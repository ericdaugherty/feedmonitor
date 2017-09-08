package main

import (
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

func fetchEndpoint(se *Endpoint, url string) (interface{}, error) {

	log := log.WithFields(logrus.Fields{"module": "fetcher", "url": url})

	log.Debug("Fetching Endpoint")

	c := &EndpointResult{Endpoint: se, URL: url}

	client := &http.Client{}

	var reader io.Reader
	if se.RequestBody != "" {
		reader = strings.NewReader(se.RequestBody)
	}

	req, err := http.NewRequest(se.Method, url, reader)
	if err != nil {
		return nil, err
	}

	// Request Headers: req.Header.Add("If-None-Match", `W/"wyzzy"`)

	startTime := time.Now()
	resp, err := client.Do(req)
	c.Duration = time.Now().Sub(startTime)
	c.CheckTime = startTime

	if err != nil {
		log.Errorf("Error executing HTTP Request for URL %v, %v", url, err)
		return nil, err
	}

	if resp.Body != nil {
		defer resp.Body.Close()
		c.Body, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
	}

	c.Status = resp.StatusCode
	c.Size = resp.ContentLength
	if c.Size == -1 {
		log.Debug("No ContentLength set, defaulting to body string length.")
		c.Size = int64(len(c.Body))
	}

	log.Infof("Fetched URL %v in %v with status %d and %d bytes.", url, c.Duration, c.Status, c.Size)
	configuration.PerfLogChannel <- c

	data := make(map[string]interface{})
	vresults := []*ValidationResult{}

	valid := true
	for _, v := range se.Validators {
		cont, res := v.validate(se, c, data)
		vresults = append(vresults, res)
		if !res.Valid {
			valid = false
			log.Infof("Validation Failed for %s validator. Errors: %v", res.Name, res.Errors)
		}
		if !cont {
			break
		}
	}

	// TODO Store Validation Results
	se.CurrentValidation = vresults
	if valid {
		se.CurrentStatus = StatusOK
	} else {
		se.CurrentStatus = StatusFail
	}

	return data["data"], nil
}
