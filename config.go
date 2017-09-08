package main

import (
	"bytes"
	"html/template"
	"strings"
	"time"
)

const urlSeparator = "|||"

// StatusUnknown defines valid states for Endpoint Status
const (
	StatusUnknown = iota
	StatusOK
	StatusFail
)

// Configuration defines the structure of the configuration file and values.
type Configuration struct {
	LogLevel        string
	PerformanceLogs string
	Applications    []*Application
	PerfLogChannel  chan *EndpointResult
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

// Validator defines the interface that feed result validitors need to implement.
type Validator interface {
	validate(*Endpoint, *EndpointResult, map[string]interface{}) (bool, *ValidationResult)
}

// EndpointResult contains the results from checking an Endpoint.
type EndpointResult struct {
	Endpoint  *Endpoint
	URL       string
	CheckTime time.Time
	Duration  time.Duration
	Size      int64
	Status    int
	Body      []byte
}

// ValidationResult contains the result of a validator against an Endpoint
type ValidationResult struct {
	EndpointResult *EndpointResult
	Name           string
	Valid          bool
	Errors         []string
}

// NewStaticEndpoint initializes a new StaticEndpoint using the specified values and defaults.
func NewStaticEndpoint(key string, name string, url string, checkIntervalMin int, validators []Validator) Endpoint {
	return Endpoint{Key: key, Name: name, URL: url, Method: "GET", Dynamic: false, CheckIntervalMin: checkIntervalMin, Validators: validators, lastCheckTime: time.Unix(0, 0), nextCheckTime: time.Now()}
}

// NewDynamicEndpoint initializes a new StaticEndpoint using the specified values and defaults.
func NewDynamicEndpoint(key string, name string, url string, checkIntervalMin int, validators []Validator) Endpoint {
	return Endpoint{Key: key, Name: name, URL: url, Method: "GET", Dynamic: true, CheckIntervalMin: checkIntervalMin, Validators: validators, lastCheckTime: time.Unix(0, 0), nextCheckTime: time.Now()}
}

func (c *Configuration) initialize() {
	// Setup default values
	c.LogLevel = "warn"
	c.PerformanceLogs = "performance"

	// TODO Load Config from file?

	// Override configuration file with command line parameters as needed.

	// ## TEMP Data

	configuration.Applications = []*Application{&tourApp}

	// ## END TEMP Data

	if options.LogLevel != "" {
		c.LogLevel = options.LogLevel
	}
}

func (c *Configuration) getApplication(key string) *Application {
	for _, v := range configuration.Applications {
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

var vSizeStatus = &ValidateSizeStatus{ValidStatusCodes: []int{200}, MinimumSize: 100, MaximumSize: 1000000}
var vJSON = &ValidateJSON{}

var tourse1 = NewStaticEndpoint("tours", "Tours", "http://static.pgatour.com/mobile/v2/toursV2.json", 1, []Validator{vSizeStatus, vJSON})
var tourse2 = NewStaticEndpoint("config", "Config", "http://static.pgatour.com/mobile/v2/configV2.json", 1, []Validator{vSizeStatus, vJSON})
var tourse3 = NewStaticEndpoint("maintour", "MainTour", "http://www.pgatour.com/data/de/v2/2017/r/tournament.json", 1, []Validator{vSizeStatus, vJSON})
var tourse4 = NewStaticEndpoint("maintourap", "MainTourAllPlayers", "http://www.pgatour.com/data/de/v2/2017/r/all-players.json", 1, []Validator{vSizeStatus, vJSON})

var tourde1 = NewDynamicEndpoint("broadcast", "Broadcast", "{{range .MainTour.tournaments}}http://www.pgatour.com/data/de/v2/2017/r/{{.id}}/broadcast.json|||{{end}}", 1, []Validator{vSizeStatus, vJSON})

var tourApp = Application{"PGAT", "PGA TOUR", []*Endpoint{&tourse1, &tourse2, &tourse3, &tourse4, &tourde1}}
