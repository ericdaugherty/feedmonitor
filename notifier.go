package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/tbruyelle/hipchat-go/hipchat"
)

// StandardErrorNotifier sends notifications to standard err (console).
type StandardErrorNotifier struct {
	Name string
}

func (s *StandardErrorNotifier) initialize(name string, data map[string]interface{}) {
	s.Name = name
}

func (s *StandardErrorNotifier) notify(n *Notification) {

	var message string
	if n.EndpointResult.Valid() {
		message = fmt.Sprintf("Successfully checked %v feed %v at URL: %v in %v", n.Application.Name, n.Endpoint.Name, n.EndpointResult.URL, n.EndpointResult.Duration)
	} else {
		errors := ""
		for _, vr := range n.EndpointResult.ValidationResults {
			if !vr.Valid {
				for _, e := range vr.Errors {
					errors = fmt.Sprintf("%v%v\r\n", errors, e)
				}
			}
		}
		message = fmt.Sprintf("Validation error on %v feed name '%v' at URL: %v.\r\n Errors:\r\n%v\r\n", n.Application.Name, n.Endpoint.Name, n.EndpointResult.URL, errors)
	}
	fmt.Fprintf(os.Stderr, "Notification:\r\n%v\r\n", message)
}

// HipChatNotifer sends notifications to HipChat.
type HipChatNotifer struct {
	Name   string
	Client *hipchat.Client
	Room   string
}

func (h *HipChatNotifer) initialize(name string, data map[string]interface{}) {
	h.Name = name
	apiKey := data["apikey"].(string)
	h.Client = hipchat.NewClient(apiKey)
	h.Room = data["room"].(string)
}

func (h *HipChatNotifer) notify(n *Notification) {

	var message string
	var color hipchat.Color
	if n.EndpointResult.Valid() {
		message = fmt.Sprintf("Successfully checked %v feed %v at URL: %v in %v", n.Application.Name, n.Endpoint.Name, n.EndpointResult.URL, n.EndpointResult.Duration)
		color = hipchat.ColorGreen
	} else {
		errors := ""
		for _, vr := range n.EndpointResult.ValidationResults {
			if !vr.Valid {
				for _, e := range vr.Errors {
					errors = fmt.Sprintf("%v%v<br/>", errors, e)
				}
			}
		}
		resultURL := fmt.Sprintf("%v/app/%v/%v/", configuration.WebRoot, n.Application.Key, n.EndpointResult.EndpointKey)
		message = fmt.Sprintf("Validation error on %v feed name '%v' at URL: %v.<br/> Errors:<br/>%v<br/><a href=\"%v\">View Feed</a>", n.Application.Name, n.Endpoint.Name, n.EndpointResult.URL, errors, resultURL)
		color = hipchat.ColorRed
	}

	notifRq := &hipchat.NotificationRequest{
		Message:       message,
		MessageFormat: "html",
		Color:         color,
	}

	h.Client.Room.Notification(h.Room, notifRq)
}

// TeamsNotifer sends notifications to MS Teams.
type TeamsNotifer struct {
	Name string
	URL  string
}

func (t *TeamsNotifer) initialize(name string, data map[string]interface{}) {
	t.Name = name
	t.URL = data["url"].(string)
}

func (t *TeamsNotifer) notify(n *Notification) {
	// Useful Debugging tool: https://messagecardplayground.azurewebsites.net/
	body := `{
		"@type": "MessageCard",
		"@context": "http://schema.org/extensions",
		"summary": "Card \"Test card\"",
		"themeColor": "%v",
		"title": "%v",
		"sections": [
			%v
		],
		"potentialAction": [
			{
				"@type": "OpenUri",
				"name": "View in FeedMonitor",
				"targets": [
					{ "os": "default", "uri": "%v" }
				]
			}
		]
	}`

	successFactset := `{
		"facts": [
			{
				"name": "Application:",
				"value": "%v"
			},
			{
				"name": "Endpoint Name:",
				"value": "%v"
			},
			{
				"name": "URL:",
				"value": "%v"
			},
			{
				"name": "Duration:",
				"value": "%v"
			},
		],
		"text": "%v"
	}`

	errorFactset := `{
		"facts": [
			{
				"name": "Validator:",
				"value": "%v"
			},
			{
				"name": "Error:",
				"value": "%v"
			}
		]
	},`

	failFactset := `{
		"facts": [
			{
				"name": "Application:",
				"value": "%v"
			},
			{
				"name": "Endpoint Name:",
				"value": "%v"
			},
			{
				"name": "URL:",
				"value": "%v"
			}
		],
		"text": "%v"
	},`

	type TemplateData struct {
		Title   string
		Color   string
		Message string
		Details string
		URL     string
	}

	data := TemplateData{}
	data.URL = fmt.Sprintf("%v/app/%v/%v/", configuration.WebRoot, n.Application.Key, n.EndpointResult.EndpointKey)
	if n.EndpointResult.Valid() {
		data.Title = "FeedMonitor Fetch Successful"
		data.Message = "Feed fetched and validated successfully."
		data.Details = fmt.Sprintf(successFactset, n.Application.Name, n.Endpoint.Name, n.EndpointResult.URL, n.EndpointResult.Duration, data.Message)
		data.Color = "00FF00"
	} else {
		data.Title = "FeedMonitor Fetch Failed"
		data.Message = "Validation failed."
		errors := ""
		for _, vr := range n.EndpointResult.ValidationResults {
			if !vr.Valid {
				for _, e := range vr.Errors {
					if len(errors) > 0 {
						errors = fmt.Sprintf("%v,", errors)
					}
					errors = fmt.Sprintf("%v"+errorFactset, errors, vr.Name, e)
				}
			}
		}
		data.Details = fmt.Sprintf(failFactset+"%v", n.Application.Name, n.Endpoint.Name, n.EndpointResult.URL, data.Message, errors)
		data.Color = "FF0000"
	}

	output := fmt.Sprintf(body, data.Color, data.Title, data.Details, data.URL)

	req, err := http.NewRequest("POST", t.URL, bytes.NewBuffer([]byte(output)))

	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Errorf("Error preparing notification request to MS Teams URL: %v", t.URL)
		return
	}

	if resp.StatusCode != 200 {
		respBody, _ := ioutil.ReadAll(resp.Body)
		log.Warnf("Teams Notification Failed. %v", string(respBody))
	}
	defer resp.Body.Close()
}
