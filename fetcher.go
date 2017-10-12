package main

import (
	"bytes"
	"html/template"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

func fetchEndpoint(app *Application, e *Endpoint, url string, data map[string]interface{}) (map[string]interface{}, error) {

	log := log.WithFields(logrus.Fields{"module": "fetcher", "app": app.Key, "endpoint": e.Key, "url": url})

	log.Debug("Fetching Endpoint")

	epr := &EndpointResult{AppKey: app.Key, EndpointKey: e.Key, URL: url}

	client := &http.Client{}
	if e.IgnoreRedirects {
		client = &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}

	req, err := http.NewRequest(e.Method, url, strings.NewReader(executeTemplate(e.RequestBody, data)))
	if err != nil {
		log.Errorf("Error creating new HTTP Request: %v", err)
		return nil, err
	}

	for k, v := range e.Headers {
		req.Header.Add(executeTemplate(k, data), executeTemplate(v, data))
	}

	epr.CheckTime = time.Now()
	resp, err := client.Do(req)
	if err != nil {
		webLog.Warnf("Error Performing Endpoint Query. %v", err)
	}
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

	resultData := make(map[string]interface{})
	resultData["headers"] = resp.Header
	vresults := []*ValidationResult{}

	valid := true
	for _, v := range e.Validators {
		cont, res := v.validate(e, epr, resultData)
		vresults = append(vresults, res)
		if !res.Valid {
			valid = false
			log.Infof("Validation Failed for %s validator. Errors: %v", res.Name, res.Errors)
		}
		if !cont {
			break
		}
	}

	epr.ValidationResults = vresults

	ResultLogChannel <- epr
	NotificationChannel <- &Notification{Application: app, Endpoint: e, EndpointResult: epr}

	e.CurrentValidation = vresults
	if valid {
		e.CurrentStatus = StatusOK
	} else {
		e.CurrentStatus = StatusFail
	}

	return resultData, nil
}

func executeTemplate(value string, data map[string]interface{}) string {
	funcMap := template.FuncMap{
		"TrimPrefix": strings.TrimPrefix,
		"TrimSuffix": strings.TrimSuffix,
		"Split":      strings.Split,
		"TrimAt": func(s string, sep string) string {
			i := strings.Index(s, sep)
			if i >= 0 {
				return s[:i]
			}
			return s
		},
	}

	t, err := template.New("").Funcs(funcMap).Parse(value)
	if err != nil {
		log.Warnf("Unable to parse template %v. Error: %v", value, err)
		return value
	}

	buf := new(bytes.Buffer)
	err = t.Execute(buf, data)
	if err != nil {
		log.Warnf("Unable to execute template %v. Error: %v", value, err)
		return value
	}
	return buf.String()
}
