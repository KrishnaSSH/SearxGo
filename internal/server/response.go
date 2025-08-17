package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"searxgo/internal/aggregate"
	en "searxgo/internal/engine"
)

// ResponseHelper provides utilities for HTTP responses
type ResponseHelper struct{}

// NewResponseHelper creates a new response helper
func NewResponseHelper() *ResponseHelper {
	return &ResponseHelper{}
}

// WriteJSON writes a JSON response with optional pretty printing
func (rh *ResponseHelper) WriteJSON(w http.ResponseWriter, data interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	if os.Getenv("SEARXGO_PRETTY_JSON") != "" {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(data)
}

// WriteError writes an error response
func (rh *ResponseHelper) WriteError(w http.ResponseWriter, message string, statusCode int) {
	http.Error(w, message, statusCode)
}

// WriteNoCache sets no-cache headers and writes JSON response
func (rh *ResponseHelper) WriteNoCache(w http.ResponseWriter, data interface{}) error {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	return rh.WriteJSON(w, data)
}

// SearchResponse represents a search API response
type SearchResponse struct {
	Results []en.Result        `json:"results"`
	Timings []aggregate.Timing `json:"timings,omitempty"`
	TookMs  int64              `json:"took_ms,omitempty"`
}

// WriteSearchResponse writes a search response with timing information
func (rh *ResponseHelper) WriteSearchResponse(w http.ResponseWriter, results []en.Result, timings []aggregate.Timing, took time.Duration, wantTimings bool) error {
	// Set timing headers
	rh.setTimingHeaders(w, timings, took)
	
	if wantTimings {
		response := SearchResponse{
			Results: results,
			Timings: timings,
			TookMs:  took.Milliseconds(),
		}
		return rh.WriteJSON(w, response)
	}
	
	return rh.WriteJSON(w, results)
}

// setTimingHeaders sets Server-Timing and debug headers
func (rh *ResponseHelper) setTimingHeaders(w http.ResponseWriter, timings []aggregate.Timing, took time.Duration) {
	// Build Server-Timing: "total;dur=.., engineA;dur=.., engineB;dur=.."
	var parts []string
	parts = append(parts, "total;dur="+strconv.FormatInt(took.Milliseconds(), 10))
	
	if len(timings) > 0 {
		for _, t := range timings {
			parts = append(parts, t.Engine+";dur="+strconv.FormatInt(t.Ms, 10))
		}
	}
	
	w.Header().Set("Server-Timing", strings.Join(parts, ", "))
	
	// Also include a debug header with JSON timings for easy inspection
	if len(timings) > 0 {
		if tb, err := json.Marshal(timings); err == nil {
			w.Header().Set("X-Search-Timings", string(tb))
		}
	}
}

// WriteEmptyJSON writes an empty JSON object
func (rh *ResponseHelper) WriteEmptyJSON(w http.ResponseWriter) error {
	return rh.WriteJSON(w, struct{}{})
}
