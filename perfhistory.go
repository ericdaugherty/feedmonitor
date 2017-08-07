package main

import (
	"context"
	"sync"
	"time"

	"github.com/ericdaugherty/feedmonitor/db"
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
	// log := log.WithField("module", "perfwriter")

	entry := db.PerformanceEntry{Duration: e.Duration.Nanoseconds() / int64(time.Millisecond), Size: e.Size}
	err := db.WritePerformanceRecord(e.URL, e.CheckTime, entry)

	return err

	// // fileName := html.EscapeString(e.URL)
	// fileName := "test"

	// f, err := os.OpenFile("./"+fileName, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0755)
	// if err != nil {
	// 	log.Errorf("Error opening performance log for URL %v. Error: %v", e.URL, err)
	// 	return err
	// }
	// defer f.Close()

	// f.WriteString(time.Now().String() + "\n")

	// return nil
}
