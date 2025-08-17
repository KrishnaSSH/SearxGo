package server

import (
    "context"
    "net/http"
    "time"
    "strings"

    "searxgo/internal/aggregate"
    en "searxgo/internal/engine"
)

// Server bundles dependencies for HTTP handlers.
type Server struct {
    Engines         []en.SearchEngine
    Timeout         time.Duration
    StaticDir       string
    DefaultSize     int // default page size when client doesn't provide ?size
    
    // Service dependencies
    knowledgeService *KnowledgeService
    responseHelper   *ResponseHelper
    requestParser    *RequestParser
}

// NewServer creates a new server with default configuration
func NewServer(engines []en.SearchEngine) *Server {
    timeout := 5 * time.Second
    return &Server{
        Engines:          engines,
        Timeout:          timeout,
        StaticDir:        "static",
        DefaultSize:      30,
        knowledgeService: NewKnowledgeService(timeout),
        responseHelper:   NewResponseHelper(),
        requestParser:    NewRequestParser(),
    }
}

// Handler returns an http.Handler with routes registered.
func (s *Server) Handler() http.Handler {
    mux := http.NewServeMux()

    // Static files
    s.setupStaticRoutes(mux)
    
    // API routes
    mux.HandleFunc("/search", s.handleSearch)
    mux.HandleFunc("/knowledge", s.handleKnowledge)

    return mux
}

// setupStaticRoutes configures static file serving
func (s *Server) setupStaticRoutes(mux *http.ServeMux) {
    if s.StaticDir == "" {
        s.StaticDir = "static"
    }
    fs := http.FileServer(http.Dir(s.StaticDir))
    mux.Handle("/static/", http.StripPrefix("/static/", fs))
    // Pretty route for results page (avoid exposing /static/results.html in URL)
    mux.HandleFunc("/results", func(w http.ResponseWriter, r *http.Request) {
        http.ServeFile(w, r, s.StaticDir+"/results.html")
    })
    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        http.ServeFile(w, r, s.StaticDir+"/index.html")
    })
}

// handleSearch handles search requests
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
    // Parse and validate request parameters
    params := s.requestParser.ParseSearchParams(r, s.DefaultSize)
    if err := s.requestParser.ValidateRequired(params); err != nil {
        s.responseHelper.WriteError(w, err.Error(), http.StatusBadRequest)
        return
    }

    // Setup context with timeout
    ctx, cancel := s.createTimeoutContext(r.Context())
    defer cancel()

    // Execute search
    results, timings, took := s.executeSearch(ctx, params)
    
    // Write response
    if err := s.responseHelper.WriteSearchResponse(w, results, timings, took, params.WantTimings); err != nil {
        s.responseHelper.WriteError(w, "failed to encode response", http.StatusInternalServerError)
    }
}

// executeSearch performs the actual search operation
func (s *Server) executeSearch(ctx context.Context, params SearchParams) ([]en.Result, []aggregate.Timing, time.Duration) {
    // Engines need a deterministic page size to fetch offsets.
    // DuckDuckGo HTML effectively paginates in ~30-size chunks; use 30 for consistency.
    engineSize := 30
    totalStart := time.Now()
    results, timings := aggregate.Run(ctx, s.Engines, params.Query, s.Timeout, params.Page, engineSize)
    took := time.Since(totalStart)
    
    // Slice to requested size for the client (stable subset of the engine page)
    if params.Size > 0 {
        start := 0
        // within the engine page, we expose only the first N requested by client
        end := params.Size
        if start > len(results) {
            start = len(results)
        }
        if end > len(results) {
            end = len(results)
        }
        results = results[start:end]
    }
    
    return results, timings, took
}

// createTimeoutContext creates a context with the appropriate timeout
func (s *Server) createTimeoutContext(parent context.Context) (context.Context, context.CancelFunc) {
    timeout := s.Timeout
    if timeout <= 0 {
        timeout = 5 * time.Second
    }
    return context.WithTimeout(parent, timeout)
}

// handleKnowledge handles knowledge card requests
func (s *Server) handleKnowledge(w http.ResponseWriter, r *http.Request) {
    // Parse and validate request parameters
    params := s.requestParser.ParseKnowledgeParams(r)
    if err := s.requestParser.ValidateRequired(params); err != nil {
        s.responseHelper.WriteError(w, err.Error(), http.StatusBadRequest)
        return
    }

    // Setup context with a slightly higher timeout for better reliability
    timeout := 3500 * time.Millisecond
    if s.Timeout > 0 && s.Timeout < timeout {
        timeout = s.Timeout
    }
    ctx, cancel := context.WithTimeout(r.Context(), timeout)
    defer cancel()

    // Fetch knowledge card data
    card, err := s.fetchKnowledgeCard(ctx, params.Query)
    if err != nil {
        // Prefer returning empty object on transient errors (e.g., timeouts)
        if err := s.responseHelper.WriteNoCache(w, struct{}{}); err != nil {
            s.responseHelper.WriteError(w, "failed to encode response", http.StatusInternalServerError)
        }
        return
    }

    // Return empty object if no results found
    if card == nil {
        if err := s.responseHelper.WriteNoCache(w, struct{}{}); err != nil {
            s.responseHelper.WriteError(w, "failed to encode response", http.StatusInternalServerError)
        }
        return
    }

    // Write successful response with no-cache headers
    if err := s.responseHelper.WriteNoCache(w, card); err != nil {
        s.responseHelper.WriteError(w, "failed to encode response", http.StatusInternalServerError)
    }
}

// fetchKnowledgeCard fetches and builds a complete knowledge card
func (s *Server) fetchKnowledgeCard(ctx context.Context, query string) (*KnowledgeCard, error) {
    // Step 1: Search Wikipedia for the best matching article
    searchResult, err := s.knowledgeService.SearchWikipedia(ctx, query)
    if err != nil {
        return nil, err
    }

    // Check if we found a valid result
    if searchResult.Title == "" && searchResult.PageURL == "" {
        return nil, nil // No good hit
    }

    // Step 2 & 3: Fetch summary and facts in parallel
    summaryCh := make(chan *WikipediaSummaryResult, 1)
    factsCh := make(chan []Fact, 1)

    // Summary goroutine
    go func() {
        defer func() { recover() }()
        summary, _ := s.knowledgeService.FetchSummary(ctx, searchResult.Title)
        summaryCh <- summary
    }()

    // Facts goroutine
    go func() {
        defer func() { recover() }()
        facts, _ := s.knowledgeService.FetchWikidataFacts(ctx, searchResult.Title)
        factsCh <- facts
    }()

    // Collect results from both goroutines
    var summary *WikipediaSummaryResult
    var facts []Fact
    done := 0
    for done < 2 {
        select {
        case ssum := <-summaryCh:
            summary = ssum
            done++
        case f := <-factsCh:
            facts = f
            done++
        case <-ctx.Done():
            done = 2 // Force exit on timeout
        }
    }

    // Disambiguation handling: if summary indicates disambiguation, try to resolve to best title
    if summary != nil {
        t := strings.ToLower(summary.Type)
        ex := strings.ToLower(summary.Extract)
        if t == "disambiguation" || strings.Contains(ex, "may refer to") || strings.Contains(ex, "disambiguation") {
            if best, _ := s.knowledgeService.QueryBestTitle(ctx, query); best != "" && best != searchResult.Title {
                // Refetch summary and facts for the resolved title
                ssum, _ := s.knowledgeService.FetchSummary(ctx, best)
                sfacts, _ := s.knowledgeService.FetchWikidataFacts(ctx, best)
                if ssum != nil {
                    summary = ssum
                    // Also update the search result title/url to match best
                    searchResult.Title = best
                }
                if sfacts != nil {
                    facts = sfacts
                }
            }
        }
    }

    // If still disambiguation after fallback, suppress the card
    if summary != nil {
        t := strings.ToLower(summary.Type)
        ex := strings.ToLower(summary.Extract)
        if t == "disambiguation" || strings.Contains(ex, "may refer to") || strings.Contains(ex, "disambiguation") {
            return nil, nil
        }
    }

    // Build and return the final knowledge card
    card := s.knowledgeService.BuildKnowledgeCard(searchResult, summary, facts)
    return card, nil
}
