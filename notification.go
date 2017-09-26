package main

import (
	"context"
	"sync"
)

// Notification represents a notification to be sent.
type Notification struct {
	Application    *Application
	Endpoint       *Endpoint
	EndpointResult *EndpointResult
}

// StartNotificationHandler starts the goroutine to process notifications.
func StartNotificationHandler(ctx context.Context, wg *sync.WaitGroup) chan *Notification {
	log := log.WithField("module", "notification")

	n := make(chan *Notification, 100)

	go func() {
		log.Debug("Starting Notification Handler")

		wg.Add(1)
		defer wg.Done()
		for {
			select {
			case nf := <-n:
				if shouldNotify(nf) {
					for _, nh := range nf.Endpoint.Notifiers {
						nh.notify(nf)
					}
				}
			case <-ctx.Done():
				log.Debug("Shutting down Notification Handler.")
				return
			}
		}
	}()

	return n
}

func shouldNotify(n *Notification) bool {

	prevEpr, _ := GetEndpointResultPrev(n.EndpointResult.AppKey, n.EndpointResult.EndpointKey, n.EndpointResult.URL, n.EndpointResult.CheckTime)

	if !n.EndpointResult.Valid() && (prevEpr == nil || prevEpr.Valid()) {
		return true
	}

	if n.EndpointResult.Valid() && (prevEpr != nil && !prevEpr.Valid()) {
		return true
	}

	return false
}
