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

	log.Debugf("Fetch complete with status: %v", resp.Status)
	// TODO Check HTTP STATUS

	c.Size = resp.ContentLength
	if c.Size == -1 {
		log.Debug("No ContentLength set, defaulting to body string length.")
		c.Size = int64(len(c.Body))
	}

	log.Infof("Fetched URL %v in %v and contained %d bytes.", url, c.Duration, c.Size)
	configuration.PerfLogChannel <- c

	jsonValidator := &JSON{}
	valRes := jsonValidator.validate(se, c)
	if valRes.Valid {
		log.Debug("JSON is valid.")
	} else {
		log.Debugf("JSON is INVALID! Issue: %v", valRes.Errors[0])
	}

	return valRes.Data, nil
}
