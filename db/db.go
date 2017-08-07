package db

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/boltdb/bolt"
	"github.com/sirupsen/logrus"
)

// type DB struct {
// }

const bucketPerformanceLog = "PerformanceLog"

var db *bolt.DB
var log *logrus.Entry

// StartDatabase initializes the datastore
func StartDatabase(filePath string, logrus *logrus.Entry) error {

	log = logrus.WithField("module", "db")

	// Setup Database
	var err error
	db, err = bolt.Open(filePath, 0600, nil)

	return err
}

// StopDatabase shuts down the datastore.
func StopDatabase() {
	db.Close()
}

// WritePerformanceRecord writes a single performance log record to the database.
func WritePerformanceRecord(url string, checkTime time.Time, e PerformanceEntry) error {

	return db.Update(func(tx *bolt.Tx) error {

		b, err := tx.CreateBucketIfNotExists([]byte(bucketPerformanceLog))
		if err != nil {
			log.Errorf("Error creating PerformanceLog bucket. %v", err.Error())
			return err
		}

		fb, err := b.CreateBucketIfNotExists([]byte(url))
		if err != nil {
			log.Errorf("Error creating PerformanceLog sub-bucket: %v. %v", url, err.Error())
			return err
		}

		k := []byte(checkTime.Format(time.RFC3339))

		var buf bytes.Buffer
		json.NewEncoder(&buf).Encode(e)

		log.Debugf("Logging Performance Entry url: %v, time: %s, value: %v", url, k, buf.String())

		return fb.Put([]byte(k), buf.Bytes())
	})
}

// GetPerformanceBucketNames returns all the available performance log bucket names.
func GetPerformanceBucketNames() (names []string, err error) {
	err = db.View(func(tx *bolt.Tx) error {

		b := tx.Bucket([]byte(bucketPerformanceLog))

		b.ForEach(func(k, v []byte) error {
			if len(v) == 0 {
				names = append(names, string(k))
			}
			return nil
		})

		return nil
	})

	return
}

// GetPerformanceRecords returns all the performance records for the provided URL.
func GetPerformanceRecords(url string) (entries []PerformanceEntryResult, err error) {
	err = db.View(func(tx *bolt.Tx) error {

		b := tx.Bucket([]byte(bucketPerformanceLog))

		fb := b.Bucket([]byte(url))

		if fb == nil {
			entries = nil
			return nil
		}

		fb.ForEach(func(k, v []byte) error {
			var entry PerformanceEntry
			err = json.NewDecoder(bytes.NewReader(v)).Decode(&entry)
			if err != nil {
				log.Errorf("Error decoding JSON from record: %v", err.Error())
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
