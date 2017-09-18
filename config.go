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
	Port         int
	AppConfigDir string
}

// ApplicationConfig represents configuration data loaded from the configuration file for a specific application
type ApplicationConfig struct {
	Key        string
	Name       string
	Validators []ValidatorConfig
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
	Validators    []string
}

// ValidatorConfig represents the
type ValidatorConfig struct {
	Key    string
	Name   string
	Type   string
	Config map[string]interface{}
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
	Validators        []Validator
	CurrentURLs       []string // Most recent parsed dynamic URLs
	CurrentStatus     int
	CurrentValidation []*ValidationResult
	lastCheckTime     time.Time
	nextCheckTime     time.Time
}

// Validator defines the interface that feed result validators need to implement.
type Validator interface {
	initialize(map[string]interface{})
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
	Body              []byte
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

}

func (c *Configuration) initializeValidator(vtype string) (Validator, bool) {
	switch vtype {
	case "JSON":
		return &ValidateJSON{}, true
	case "SizeStatus":
		return &ValidateSizeStatus{}, true
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
	validators := make(map[string]Validator)
	for _, e := range a.Validators {
		v, ok := c.initializeValidator(e.Type)
		if !ok {
			log.Fatalf("Unknown Validator type %v", e.Type)
		}
		v.initialize(e.Config)
		validators[e.Key] = v
	}

	// Create and initialize all the endpoints.
	eps := make([]*Endpoint, len(a.Endpoints))
	for i, e := range a.Endpoints {

		ep := &Endpoint{
			Key:              e.Key,
			Name:             e.Name,
			URL:              e.URL,
			Method:           e.Method,
			RequestBody:      e.RequestBody,
			Dynamic:          e.Dynamic,
			CheckIntervalMin: e.CheckInterval,
			lastCheckTime:    time.Unix(0, 0),
			nextCheckTime:    time.Now(),
		}
		v := make([]Validator, len(e.Validators))
		for i, v1 := range e.Validators {
			v[i] = validators[v1]
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
