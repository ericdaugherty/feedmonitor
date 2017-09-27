package main

import (
	"fmt"
	"os"

	"github.com/tbruyelle/hipchat-go/hipchat"
)

// StandardErrorNotifier sends notifications to standard err (console).
type StandardErrorNotifier struct {
}

func (s *StandardErrorNotifier) initialize(data map[string]interface{}) {
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
	Client *hipchat.Client
	Room   string
}

func (h *HipChatNotifer) initialize(data map[string]interface{}) {
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
