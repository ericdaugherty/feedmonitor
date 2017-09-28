package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	yaml "gopkg.in/yaml.v2"
)

const urlSeparator = "|||"

// NotificationChannel is used to send notications to the notification processor.
var NotificationChannel chan *Notification

// ResultLogChannel is used to send results to the database for storage.
var ResultLogChannel chan *EndpointResult

// StatusUnknown defines valid states for Endpoint Status
const (
	StatusUnknown = iota
	StatusOK
	StatusFail
)

// Configuration defines the structure of the configuration file and values.
type Configuration struct {
	LogLevel     string
	LogFile      string
	GitRoot      string
	WebPort      int
	WebRoot      string
	AppConfigDir string
	WebDevMode   bool
}

// ApplicationConfig represents configuration data loaded from the configuration file for a specific application
type ApplicationConfig struct {
	Key        string
	Name       string
	Validators []ValidatorConfig
	Notifiers  []NotifierConfig
	Endpoints  []EndpointConfig
}

// EndpointConfig represents configuration data loaded from the configuration file for a specific application
type EndpointConfig struct {
	Key           string
	Name          string
	URL           string
	Method        string
	RequestBody   string
	Dynamic       bool
	CheckInterval int
	Notifiers     []string
	Validators    []string
}

// NotifierConfig represents the config data for a Notification channel.
type NotifierConfig struct {
	Key     string
	Name    string
	Type    string
	Default bool
	Config  map[string]interface{}
}

// ValidatorConfig represents the
type ValidatorConfig struct {
	Key     string
	Name    string
	Type    string
	Default bool
	Config  map[string]interface{}
}

// Application defines a high level App, or set of feeds, to test
type Application struct {
	Key       string
	Name      string
	Endpoints []*Endpoint
}

// Endpoint defines an endpoint (which can be dynamic) to check.
type Endpoint struct {
	Key               string
	Name              string
	URL               string
	Method            string
	RequestBody       string
	Dynamic           bool
	CheckIntervalMin  int
	Notifiers         []Notifier
	Validators        []Validator
	CurrentURLs       []string // Most recent parsed dynamic URLs
	CurrentStatus     int
	CurrentValidation []*ValidationResult
	lastCheckTime     time.Time
	nextCheckTime     time.Time
}

// Notifier defines the interface that feed result notifiers need to implement.
type Notifier interface {
	initialize(string, map[string]interface{})
	notify(*Notification)
}

// Validator defines the interface that feed result validators need to implement.
type Validator interface {
	initialize(string, map[string]interface{})
	validate(*Endpoint, *EndpointResult, map[string]interface{}) (bool, *ValidationResult)
}

// EndpointResult contains the results from checking an Endpoint.
type EndpointResult struct {
	AppKey            string
	EndpointKey       string
	URL               string
	CheckTime         time.Time
	Duration          time.Duration
	Size              int64
	Status            int
	Headers           map[string][]string
	Body              []byte `json:"-"`
	BodyHash          string
	ValidationResults []*ValidationResult
	BodyChanged       bool
}

// Valid returns true only if all the validation results are valid.
func (er *EndpointResult) Valid() bool {
	for _, vr := range er.ValidationResults {
		if !vr.Valid {
			return false
		}
	}
	return true
}

// LoadBody loads the body of the result from storage.
func (er *EndpointResult) LoadBody() error {
	r, err := GetGitRepo(er.AppKey, er.EndpointKey, er.URL)
	if err != nil {
		return err
	}

	b, err := r.GetBody(er.BodyHash)
	if err != nil {
		return err
	}
	er.Body = b
	return nil
}

// ValidationResult contains the result of a validator against an Endpoint
type ValidationResult struct {
	Name   string
	Valid  bool
	Errors []string
}

func loadConfigFile() *Configuration {

	var c = &Configuration{}
	c.LogLevel = "warn"
	b, err := ioutil.ReadFile(options.ConfigPath)
	if err != nil {
		fmt.Println("Unable to load configuration file:", options.ConfigPath, err)
		os.Exit(1)
	}
	err = yaml.UnmarshalStrict(b, c)
	if err != nil {
		fmt.Println("Unable to parse configuration file.", err)
		os.Exit(1)
	}
	return c
}

func (c *Configuration) initialize() {

	if options.LogLevel != "" {
		c.LogLevel = options.LogLevel
	}

	c.WebDevMode = options.WebDevelopment
}

func (c *Configuration) initializeNotifier(vtype string) (Notifier, bool) {
	switch vtype {
	case "stderr":
		return &StandardErrorNotifier{}, true
	case "hipchat":
		return &HipChatNotifer{}, true
	default:
		return nil, false
	}
}

func (c *Configuration) initializeValidator(vtype string) (Validator, bool) {
	switch vtype {
	case "JSON":
		return &ValidateJSON{}, true
	case "JSONData":
		return &ValidateJSONData{}, true
	case "Status":
		return &ValidateStatus{}, true
	case "Size":
		return &ValidateSize{}, true
	default:
		return nil, false
	}
}

func (c *Configuration) initializeApplications() {

	path := filepath.Join(c.AppConfigDir, "*.yaml")
	files, err := filepath.Glob(path)
	if err != nil {
		log.Fatalf("Error parsing configuration files. %v", err)
	}

	apps := make([]*Application, len(files))

	for i, file := range files {
		log.Infof("Loading Appplication Configuration file: %v", file)
		apps[i] = c.initializeApplication(file)
	}

	applications = apps
}

func (c *Configuration) initializeApplication(file string) *Application {

	var a = &ApplicationConfig{}
	b, err := ioutil.ReadFile(file)
	if err != nil {
		log.Fatalf("Unable to load application configuration file: %v - %v", file, err)
	}
	err = yaml.UnmarshalStrict(b, a)
	if err != nil {
		log.Fatalf("Unable to parse application configuration file: %v - %v", file, err)
	}

	app := &Application{Key: a.Key, Name: a.Name}

	// Create and initialize the validators needed in the Endpoints.
	notifiers := make(map[string]Notifier)
	var defaultNotifiers []Notifier
	for _, e := range a.Notifiers {
		n, ok := c.initializeNotifier(e.Type)
		if !ok {
			log.Fatalf("Unknown Notifier type %v", e.Type)
		}
		n.initialize(e.Name, e.Config)
		notifiers[e.Key] = n
		if e.Default {
			defaultNotifiers = append(defaultNotifiers, n)
		}
	}

	// Create and initialize the validators needed in the Endpoints.
	validators := make(map[string]Validator)
	var defaultValidators []Validator
	for _, e := range a.Validators {
		v, ok := c.initializeValidator(e.Type)
		if !ok {
			log.Fatalf("Unknown Validator type %v", e.Type)
		}
		v.initialize(e.Name, e.Config)
		validators[e.Key] = v
		if e.Default {
			defaultValidators = append(defaultValidators, v)
		}
	}

	// Create and initialize all the endpoints.
	eps := make([]*Endpoint, len(a.Endpoints))
	for i, e := range a.Endpoints {

		method := "GET"
		if e.Method != "" {
			method = e.Method
		}

		ep := &Endpoint{
			Key:              e.Key,
			Name:             e.Name,
			URL:              e.URL,
			Method:           method,
			RequestBody:      e.RequestBody,
			Dynamic:          e.Dynamic,
			CheckIntervalMin: e.CheckInterval,
			lastCheckTime:    time.Unix(0, 0),
			nextCheckTime:    time.Now(),
		}

		n := make([]Notifier, len(e.Notifiers)+len(defaultNotifiers))
		// Add Default Notifiers
		copy(n, defaultNotifiers)
		indexOffset := len(defaultNotifiers)
		// Then add notifiers specified in endpoint config.
		for i, n1 := range e.Notifiers {
			not, ok := notifiers[n1]
			if !ok {
				log.Fatalf("Unable to find Notifier %v for Endpoint %v (%v) in app %v", n1, e.Name, e.Key, a.Name)
			}
			n[i+indexOffset] = not
		}
		ep.Notifiers = n

		v := make([]Validator, len(e.Validators)+len(defaultValidators))
		// Add Default Validators
		copy(v, defaultValidators)
		indexOffset = len(defaultValidators)
		// Then add Validators specified in endpoint config.
		for i, v1 := range e.Validators {
			val, ok := validators[v1]
			if !ok {
				log.Fatalf("Unable to find Validator %v for Endpoint %v (%v) in app %v", v1, e.Name, e.Key, a.Name)
			}
			v[i+indexOffset] = val
		}
		ep.Validators = v

		eps[i] = ep
	}
	app.Endpoints = eps

	return app

}

func (c *Configuration) getApplication(key string) *Application {
	for _, v := range applications {
		if strings.EqualFold(v.Key, key) {
			return v
		}
	}
	return nil
}

func (a *Application) getEndpoint(key string) *Endpoint {
	for _, v := range a.Endpoints {
		if strings.EqualFold(v.Key, key) {
			return v
		}
	}
	return nil
}

func (e *Endpoint) scheduleNextCheck() {
	if e.lastCheckTime.Unix() == 0 {
		e.lastCheckTime = time.Now()
		e.nextCheckTime = e.lastCheckTime.Add(time.Duration(e.CheckIntervalMin) * time.Minute)
	} else {
		e.lastCheckTime = e.nextCheckTime
		e.nextCheckTime = e.lastCheckTime.Add(time.Duration(e.CheckIntervalMin) * time.Minute)
		// Make sure we are not getting backed up.
		if e.nextCheckTime.Before(time.Now()) {
			e.nextCheckTime = time.Now()
		}
	}
}

func (e *Endpoint) shouldCheckNow() bool {
	return e.nextCheckTime.Before(time.Now())
}

func (e *Endpoint) parseURLs(data interface{}) ([]string, error) {
	log := log.WithField("endpoint", e.Name)

	t := template.New("URL Template")
	t, err := t.Parse(e.URL)
	if err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	err = t.Execute(buf, data)
	if err != nil {
		return nil, err
	}

	templateResult := buf.String()
	if strings.HasSuffix(templateResult, urlSeparator) {
		templateResult = templateResult[:(len(templateResult) - 3)]
	}

	if len(strings.TrimSpace(templateResult)) == 0 {
		log.Warnf("Dynamic Endpoint %s did not produce any URLs to query.", e.URL)
		return nil, nil
	}

	urls := strings.Split(templateResult, urlSeparator)
	for _, v := range urls {
		log.Infof("Parsed Dynamic URL: %v", v)
	}

	e.CurrentURLs = urls

	return urls, nil
}
