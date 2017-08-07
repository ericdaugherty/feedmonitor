package main

import (
	"context"
	"sync"
)

// NotificationHandler manages the notification queue and execution.
type NotificationHandler struct {
	channel chan Notification
}

// Notification represents a notification to be sent.
type Notification struct {
	Message string
}

func (h *NotificationHandler) startNotificationHandler(ctx context.Context, wg *sync.WaitGroup) {
	log := log.WithField("module", "notification")

	go func() {
		log.Debug("Starting Notification Handler")

		wg.Add(1)
		defer wg.Done()
		for {
			select {
			case x := <-h.channel:
				log.Debug("Message: ", x.Message)
			case <-ctx.Done():
				log.Debug("Shutting down Notification Handler.")
				return
			}
		}
	}()
}
