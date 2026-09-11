package main

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"time"
)

const maxMalformedSamples = 10

// ClientReport is the per-client section of the output.
type ClientReport struct {
	ClientID            string         `json:"client_id"`
	RequestCount        int            `json:"request_count"`
	FirstSeen           string         `json:"first_seen"`
	LastSeen            string         `json:"last_seen"`
	Endpoints           map[string]int `json:"endpoints"`
	RateLimitViolations int            `json:"rate_limit_violations"`
	MaxRequestsInWindow int            `json:"max_requests_in_window"`
}

// Report is the top-level JSON traffic report emitted by the tool.
type Report struct {
	GeneratedAt string `json:"generated_at"`
	Summary     struct {
		TotalLines     int `json:"total_lines"`
		ValidCount     int `json:"valid_count"`
		MalformedCount int `json:"malformed_count"`
	} `json:"summary"`
	MalformedSamples []string `json:"malformed_samples,omitempty"`
	RateLimitRule    struct {
		MaxRequests   int    `json:"max_requests"`
		WindowSeconds int    `json:"window_seconds"`
		Description   string `json:"description"`
	} `json:"rate_limit_rule"`
	StatusCodeCounts map[string]int `json:"status_code_counts"`
	Clients          []ClientReport `json:"clients"`
}

// buildReport reads JSONL records from in and produces the full traffic
// report, applying the given rate-limit rule (limit requests per window).
func buildReport(in io.Reader, limit int, window time.Duration) Report {
	var rpt Report
	rpt.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	rpt.RateLimitRule.MaxRequests = limit
	rpt.RateLimitRule.WindowSeconds = int(window.Seconds())
	rpt.RateLimitRule.Description = fmt.Sprintf(
		"A client violates the rate limit if it makes more than %d requests within any sliding %s window.",
		limit, window,
	)
	rpt.StatusCodeCounts = map[string]int{}

	byClient := map[string][]Record{}

	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytesTrimSpace(line)) == 0 {
			continue // skip blank lines silently; not a data line
		}
		rpt.Summary.TotalLines++
		rec, err := parseLine(line)
		if err != nil {
			rpt.Summary.MalformedCount++
			if len(rpt.MalformedSamples) < maxMalformedSamples {
				rpt.MalformedSamples = append(rpt.MalformedSamples, err.Error())
			}
			continue
		}
		rpt.Summary.ValidCount++
		byClient[rec.ClientID] = append(byClient[rec.ClientID], rec)

		bucket := statusBucket(rec.StatusCode)
		rpt.StatusCodeCounts[bucket]++
	}

	clientIDs := make([]string, 0, len(byClient))
	for id := range byClient {
		clientIDs = append(clientIDs, id)
	}
	sort.Strings(clientIDs)

	for _, id := range clientIDs {
		recs := byClient[id]
		sort.Slice(recs, func(i, j int) bool { return recs[i].Timestamp.Before(recs[j].Timestamp) })

		cr := ClientReport{
			ClientID:     id,
			RequestCount: len(recs),
			FirstSeen:    recs[0].Timestamp.UTC().Format(time.RFC3339),
			LastSeen:     recs[len(recs)-1].Timestamp.UTC().Format(time.RFC3339),
			Endpoints:    map[string]int{},
		}
		for _, r := range recs {
			cr.Endpoints[r.Endpoint]++
		}

		violations, maxInWindow := detectRateLimitViolations(recs, limit, window)
		cr.RateLimitViolations = violations
		cr.MaxRequestsInWindow = maxInWindow

		rpt.Clients = append(rpt.Clients, cr)
	}

	return rpt
}

// detectRateLimitViolations uses a sliding window (two-pointer) scan over
// timestamps sorted ascending. For every request, it counts how many
// requests (including itself) fall within [t-window, t]. Any request that
// pushes the window count above limit is counted as one violation. It also
// returns the largest window count observed, for visibility.
func detectRateLimitViolations(recs []Record, limit int, window time.Duration) (violations int, maxInWindow int) {
	left := 0
	for right := 0; right < len(recs); right++ {
		for recs[right].Timestamp.Sub(recs[left].Timestamp) > window {
			left++
		}
		count := right - left + 1
		if count > maxInWindow {
			maxInWindow = count
		}
		if count > limit {
			violations++
		}
	}
	return violations, maxInWindow
}

func statusBucket(code int) string {
	switch {
	case code >= 200 && code < 300:
		return "2xx"
	case code >= 300 && code < 400:
		return "3xx"
	case code >= 400 && code < 500:
		return "4xx"
	case code >= 500 && code < 600:
		return "5xx"
	default:
		return "other"
	}
}
