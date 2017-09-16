package main

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/boltdb/bolt"
	"github.com/sirupsen/logrus"
)

// type DB struct {
// }

const bucketPerformanceLog = "PerformanceLog"
const bucketEndpointResults = "EndpointResults"

var db *bolt.DB
var dbLog *logrus.Entry

// PerformanceEntry represents the value of a performance log entry.
type PerformanceEntry struct {
	Duration int64
	Size     int64
}

// PerformanceEntryResult represents the value of a performance log entry including the time key.
type PerformanceEntryResult struct {
	CheckTime time.Time
	PerformanceEntry
}

// StartDatabase initializes the datastore
func StartDatabase(filePath string, logrus *logrus.Entry) error {

	dbLog = logrus.WithField("module", "db")

	// Setup Database
	var err error
	db, err = bolt.Open(filePath, 0600, nil)

	return err
}

// StopDatabase shuts down the datastore.
func StopDatabase() {
	db.Close()
}

// StartResultWriter creates and returns a channel that can be used to send results to be written to the database.
func StartResultWriter(ctx context.Context, wg *sync.WaitGroup) chan *EndpointResult {
	log := log.WithField("module", "perfwriter")

	c := make(chan *EndpointResult, 100)

	log.Debug("Started Result Writer.")

	go func() {
		wg.Add(1)
		defer wg.Done()
		for {
			select {
			case res := <-c:
				recordResult(res)
			case <-ctx.Done():
				log.Debug("Shutting down Result Writer.")
				return
			}
		}
	}()

	return c
}

func recordResult(e *EndpointResult) {
	entry := PerformanceEntry{Duration: e.Duration.Nanoseconds() / int64(time.Millisecond), Size: e.Size}
	WritePerformanceRecord(e, entry)
	WriteEndpointResult(e)
}

// WritePerformanceRecord writes a single performance log record to the database.
func WritePerformanceRecord(epr *EndpointResult, e PerformanceEntry) {

	db.Update(func(tx *bolt.Tx) error {

		b, err := getOrCreateBucket(tx, bucketPerformanceLog, epr.AppKey, epr.EndpointKey, epr.URL)
		if err != nil {
			dbLog.Errorf("Error getting PerformanceBucket for App: %v, Endpoint: %v, URL: %v - %v", epr.AppKey, epr.EndpointKey, epr.URL, err.Error())
			return err
		}

		k := getTimeKey(epr.CheckTime)

		var buf bytes.Buffer
		json.NewEncoder(&buf).Encode(e)

		dbLog.Debugf("Logging Performance Entry url: %v, time: %s, value: %v", epr.URL, k, buf.String())

		err = b.Put([]byte(k), buf.Bytes())
		if err != nil {
			dbLog.Errorf("Error writing PerformanceLog entry to db for App: %v, Endpoint: %v, URL: %v - %v", epr.AppKey, epr.EndpointKey, epr.URL, err.Error())
			return err
		}

		return nil
	})
}

// GetPerformanceRecords returns all the performance records for the provided URL.
func GetPerformanceRecords(appKey string, endpointKey string, url string) (entries []PerformanceEntryResult, err error) {
	err = db.View(func(tx *bolt.Tx) error {

		b := getBucket(tx, bucketPerformanceLog, appKey, endpointKey, url)

		if b == nil {
			entries = nil
			return nil
		}

		b.ForEach(func(k, v []byte) error {
			var entry PerformanceEntry
			err = json.NewDecoder(bytes.NewReader(v)).Decode(&entry)
			if err != nil {
				dbLog.Errorf("Error decoding JSON from record: %v", err.Error())
				return nil
			}
			time, _ := time.Parse(time.RFC3339, string(k))
			res := PerformanceEntryResult{CheckTime: time, PerformanceEntry: entry}
			entries = append(entries, res)

			return nil
		})

		return nil
	})

	return
}

// GetPerformanceRecordsForDate returns all the performance records for the provided URL and date.
func GetPerformanceRecordsForDate(appKey string, endpointKey string, url string, date time.Time) ([]PerformanceEntryResult, error) {

	entries := make([]PerformanceEntryResult, 0, 100)

	err := db.View(func(tx *bolt.Tx) error {

		b := getBucket(tx, bucketPerformanceLog, appKey, endpointKey, url)

		if b == nil {
			entries = nil
			return nil
		}

		c := b.Cursor()

		year, month, day := date.Date()
		d := time.Date(year, month, day, 0, 0, 0, 0, date.Location())
		min := []byte(d.Format(time.RFC3339))
		max := []byte(d.Add(24 * time.Hour).Format(time.RFC3339))

		for k, v := c.Seek(min); k != nil && bytes.Compare(k, max) <= 0; k, v = c.Next() {
			var entry PerformanceEntry
			err := json.NewDecoder(bytes.NewReader(v)).Decode(&entry)
			if err != nil {
				dbLog.Errorf("Error decoding JSON from record: %v", err.Error())
				return err
			}
			time, _ := time.Parse(time.RFC3339, string(k))
			res := PerformanceEntryResult{CheckTime: time, PerformanceEntry: entry}
			entries = append(entries, res)
		}
		return nil
	})

	return entries, err
}

// WriteEndpointResult writes a single endpoint result record to the database.
func WriteEndpointResult(epr *EndpointResult) {

	db.Update(func(tx *bolt.Tx) error {

		b, err := getOrCreateBucket(tx, bucketEndpointResults, epr.AppKey, epr.EndpointKey, epr.URL)
		if err != nil {
			dbLog.Errorf("Error getting EndpointResult Bucket for App: %v, Endpoint: %v, URL: %v - %v", epr.AppKey, epr.EndpointKey, epr.URL, err.Error())
			return err
		}

		k := getTimeKey(epr.CheckTime)

		var buf bytes.Buffer
		json.NewEncoder(&buf).Encode(epr)

		dbLog.Debugf("Logging EndpointResult Entry App: %v, Endpoint: %v, URL: %v", epr.AppKey, epr.EndpointKey, epr.URL)

		err = b.Put([]byte(k), buf.Bytes())
		if err != nil {
			dbLog.Errorf("Error writing EndpointResult entry to db for App: %v, Endpoint: %v, URL: %v - %v", epr.AppKey, epr.EndpointKey, epr.URL, err.Error())
			return err
		}

		return nil
	})
}

// GetEndpointResult returns a single EndPointResult.
func GetEndpointResult(appKey string, endpointKey string, url string, date time.Time) (epr *EndpointResult, err error) {

	err = db.View(func(tx *bolt.Tx) error {

		b := getBucket(tx, bucketEndpointResults, appKey, endpointKey, url)

		if b == nil {
			return nil
		}

		v := b.Get(getTimeKey(date))
		if len(v) == 0 {
			return nil
		}

		err := json.NewDecoder(bytes.NewReader(v)).Decode(&epr)
		if err != nil {
			dbLog.Errorf("Error decoding JSON from record: %v", err.Error())
			return err
		}
		return nil
	})

	return
}

// GetEndpointResultsForDate returns all the performance records for the provided URL and date.
func GetEndpointResultsForDate(appKey string, endpointKey string, url string, date time.Time) ([]EndpointResult, error) {

	entries := make([]EndpointResult, 0, 100)

	err := db.View(func(tx *bolt.Tx) error {

		b := getBucket(tx, bucketEndpointResults, appKey, endpointKey, url)

		if b == nil {
			entries = nil
			return nil
		}

		c := b.Cursor()

		year, month, day := date.Date()
		d := time.Date(year, month, day, 0, 0, 0, 0, date.Location())
		min := []byte(d.Format(time.RFC3339))
		max := []byte(d.Add(24 * time.Hour).Format(time.RFC3339))

		for k, v := c.Seek(min); k != nil && bytes.Compare(k, max) <= 0; k, v = c.Next() {
			var entry EndpointResult
			err := json.NewDecoder(bytes.NewReader(v)).Decode(&entry)
			if err != nil {
				dbLog.Errorf("Error decoding JSON from record: %v", err.Error())
				return err
			}
			entries = append(entries, entry)
		}
		return nil
	})

	return entries, err
}

func getBucket(tx *bolt.Tx, bucketType string, appKey string, endpointKey string, url string) *bolt.Bucket {

	b := tx.Bucket([]byte(bucketType))

	appb := b.Bucket([]byte(appKey))

	epb := appb.Bucket([]byte(endpointKey))

	return epb.Bucket([]byte(url))
}

func getOrCreateBucket(tx *bolt.Tx, bucketType string, appKey string, endpointKey string, url string) (*bolt.Bucket, error) {

	b, err := tx.CreateBucketIfNotExists([]byte(bucketType))
	if err != nil {
		return nil, err
	}

	appb, err := b.CreateBucketIfNotExists([]byte(appKey))
	if err != nil {
		return nil, err
	}

	epb, err := appb.CreateBucketIfNotExists([]byte(endpointKey))
	if err != nil {
		return nil, err
	}

	fb, err := epb.CreateBucketIfNotExists([]byte(url))
	if err != nil {
		return nil, err
	}

	return fb, nil
}

func getTimeKey(t time.Time) []byte {
	return []byte(t.Format(time.RFC3339))
}
