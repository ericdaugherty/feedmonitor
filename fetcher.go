package main

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

func fetchEndpoint(app *Application, e *Endpoint, url string) (interface{}, error) {

	log := log.WithFields(logrus.Fields{"module": "fetcher", "app": app.Key, "endpoint": e.Key, "url": url})

	log.Debug("Fetching Endpoint")

	epr := &EndpointResult{AppKey: app.Key, EndpointKey: e.Key, URL: url}

	client := &http.Client{}

	var reader io.Reader
	if e.RequestBody != "" {
		reader = strings.NewReader(e.RequestBody)
	}

	req, err := http.NewRequest(e.Method, url, reader)
	if err != nil {
		log.Errorf("Error creating new HTTP Request: %v", err)
		return nil, err
	}

	// Request Headers: req.Header.Add("If-None-Match", `W/"wyzzy"`)

	epr.CheckTime = time.Now()
	resp, err := client.Do(req)
	epr.Duration = time.Now().Sub(epr.CheckTime)

	if err != nil {
		log.Errorf("Error executing HTTP Request: %v", err)
		return nil, err
	}

	if resp.Body != nil {
		defer resp.Body.Close()
		epr.Body, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Errorf("Error reading response body: %v", err)
			return nil, err
		}
	}
	epr.Headers = resp.Header

	epr.Status = resp.StatusCode
	epr.Size = resp.ContentLength
	if epr.Size == -1 {
		log.Debug("No ContentLength set, defaulting to body string length.")
		epr.Size = int64(len(epr.Body))
	}

	log.Infof("Fetched result in %v with status %d and %d bytes.", epr.Duration, epr.Status, epr.Size)

	data := make(map[string]interface{})
	vresults := []*ValidationResult{}

	valid := true
	for _, v := range e.Validators {
		cont, res := v.validate(e, epr, data)
		vresults = append(vresults, res)
		if !res.Valid {
			valid = false
			log.Infof("Validation Failed for %s validator. Errors: %v", res.Name, res.Errors)
		}
		if !cont {
			break
		}
	}

	prevEpr, err := GetLastEndpointResult(app.Key, e.Key, url)
	if err != nil {
		log.Warnf("Unable to compare result to previous body. %v", err.Error())
		epr.BodyChanged = true
	} else if prevEpr == nil {
		log.Infof("Unable to compare result to previous body. No Previous result in databse.")
	} else {
		if bytes.Compare(epr.Body, prevEpr.Body) != 0 {
			epr.BodyChanged = true
		} else {
			epr.BodyChanged = false
		}
	}

	epr.ValidationResults = vresults

	ResultLogChannel <- epr

	e.CurrentValidation = vresults
	if valid {
		e.CurrentStatus = StatusOK
	} else {
		e.CurrentStatus = StatusFail
	}

	return data["data"], nil
}
