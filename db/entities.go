package db

import "time"

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
