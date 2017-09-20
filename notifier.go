package main

import (
	"fmt"

	"github.com/tbruyelle/hipchat-go/hipchat"
)

// HipChatNotifer sends notifications to HipChat.
type HipChatNotifer struct {
	Client       *hipchat.Client
	Room         string
	AlwaysNotify bool
}

func (h *HipChatNotifer) initialize(data map[string]interface{}) {
	apiKey := data["apikey"].(string)
	h.Client = hipchat.NewClient(apiKey)
	h.Room = data["room"].(string)
	h.AlwaysNotify = data["alwaysnotify"].(bool)
}

func (h *HipChatNotifer) notify(n *Notification) {
	if !h.AlwaysNotify && n.EndpointResult.Valid() {
		return
	}

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
