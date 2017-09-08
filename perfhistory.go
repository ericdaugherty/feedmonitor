package main

import (
	"context"
	"sync"
	"time"
)

func startPerformanceWriter(ctx context.Context, wg *sync.WaitGroup) chan *EndpointResult {
	log := log.WithField("module", "perfwriter")

	c := make(chan *EndpointResult, 100)

	log.Debug("Started Performance Writer.")

	go func() {
		wg.Add(1)
		defer wg.Done()
		for {
			select {
			case res := <-c:
				log.Debugf("TODO Write result for URL: %v, time: %s duration: %v, size: %d", res.Endpoint.URL, res.CheckTime, res.Duration, res.Size)
				recordPerformance(res)
			case <-ctx.Done():
				log.Debug("Shutting down Performance Writer.")
				return
			}
		}
	}()

	return c
}

func recordPerformance(e *EndpointResult) error {
	entry := PerformanceEntry{Duration: e.Duration.Nanoseconds() / int64(time.Millisecond), Size: e.Size}
	err := WritePerformanceRecord(e.URL, e.CheckTime, entry)

	return err
}
