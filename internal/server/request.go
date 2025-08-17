package server

import (
	"net/http"
	"strconv"
	"strings"
)

// RequestParser provides utilities for parsing HTTP requests
type RequestParser struct{}

// NewRequestParser creates a new request parser
func NewRequestParser() *RequestParser {
	return &RequestParser{}
}

// SearchParams represents search request parameters
type SearchParams struct {
	Query        string
	Page         int
	Size         int
	WantTimings  bool
}

// ParseSearchParams parses search request parameters from URL query
func (rp *RequestParser) ParseSearchParams(r *http.Request, defaultSize int) SearchParams {
	params := SearchParams{
		Query:       strings.TrimSpace(r.URL.Query().Get("q")),
		Page:        1,
		Size:        defaultSize,
		WantTimings: r.URL.Query().Get("timings") == "1",
	}
	
	// Parse page parameter
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			params.Page = n
		}
	}
	
	// Parse size parameter with safety cap
	if v := r.URL.Query().Get("size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > 200 {
				n = 200 // safety cap
			}
			params.Size = n
		}
	}
	
	// Ensure non-negative size
	if params.Size < 0 {
		params.Size = 0
	}
	
	return params
}

// KnowledgeParams represents knowledge request parameters
type KnowledgeParams struct {
	Query string
}

// ParseKnowledgeParams parses knowledge request parameters
func (rp *RequestParser) ParseKnowledgeParams(r *http.Request) KnowledgeParams {
	return KnowledgeParams{
		Query: strings.TrimSpace(r.URL.Query().Get("q")),
	}
}

// ValidateRequired checks if required parameters are present
func (rp *RequestParser) ValidateRequired(params interface{}) error {
	switch p := params.(type) {
	case SearchParams:
		if p.Query == "" {
			return &ValidationError{Field: "q", Message: "missing required parameter"}
		}
	case KnowledgeParams:
		if p.Query == "" {
			return &ValidationError{Field: "q", Message: "missing required parameter"}
		}
	}
	return nil
}

// ValidationError represents a parameter validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}
