package main

import (
	"encoding/json"
	"fmt"
	"time"
)

// RawRecord mirrors the on-disk JSON shape. StatusCode is parsed as
// json.RawMessage so we can validate it's an integer without losing
// precision.
type RawRecord struct {
	RequestID  *string         `json:"request_id"`
	Timestamp  *string         `json:"timestamp"`
	ClientID   *string         `json:"client_id"`
	Endpoint   *string         `json:"endpoint"`
	StatusCode json.RawMessage `json:"status_code"`
}

// Record is a validated, parsed log entry.
type Record struct {
	RequestID  string
	Timestamp  time.Time
	ClientID   string
	Endpoint   string
	StatusCode int
}

// parseLine validates and converts one raw JSON line into a record. It
// returns an error describing why the line is malformed, if any.
func parseLine(line []byte) (Record, error) {
	var raw RawRecord
	if err := json.Unmarshal(line, &raw); err != nil {
		return Record{}, fmt.Errorf("invalid JSON: %w", err)
	}
	if raw.RequestID == nil || *raw.RequestID == "" {
		return Record{}, fmt.Errorf("missing request_id")
	}
	if raw.ClientID == nil || *raw.ClientID == "" {
		return Record{}, fmt.Errorf("missing client_id")
	}
	if raw.Endpoint == nil || *raw.Endpoint == "" {
		return Record{}, fmt.Errorf("missing endpoint")
	}
	if raw.Timestamp == nil || *raw.Timestamp == "" {
		return Record{}, fmt.Errorf("missing timestamp")
	}
	ts, err := time.Parse(time.RFC3339, *raw.Timestamp)
	if err != nil {
		return Record{}, fmt.Errorf("invalid timestamp: %w", err)
	}
	if len(raw.StatusCode) == 0 {
		return Record{}, fmt.Errorf("missing status_code")
	}
	var code int
	if err := json.Unmarshal(raw.StatusCode, &code); err != nil {
		return Record{}, fmt.Errorf("status_code must be an integer: %w", err)
	}
	if code < 100 || code > 599 {
		return Record{}, fmt.Errorf("status_code out of range: %d", code)
	}
	return Record{
		RequestID:  *raw.RequestID,
		Timestamp:  ts,
		ClientID:   *raw.ClientID,
		Endpoint:   *raw.Endpoint,
		StatusCode: code,
	}, nil
}

// bytesTrimSpace trims leading and trailing ASCII whitespace from b.
func bytesTrimSpace(b []byte) []byte {
	start := 0
	for start < len(b) && isSpace(b[start]) {
		start++
	}
	end := len(b)
	for end > start && isSpace(b[end-1]) {
		end--
	}
	return b[start:end]
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}
