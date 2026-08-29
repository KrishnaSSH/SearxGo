package engines

import (
    "context"
    "encoding/json"
    "fmt"
    "math/rand"
    "net/url"
    "strconv"
    "strings"
    "time"

    en "searxgo/internal/engine"
    "searxgo/internal/httpx"
)

type wikipedia struct{}

func NewWikipedia() en.SearchEngine { return &wikipedia{} }

func (w *wikipedia) Name() string { return "wikipedia" }

// Wikipedia engine configuration and constants
const (
    wikiBaseURL          = "https://en.wikipedia.org"
    openSearchPath       = "/w/api.php"
    wikiFaviconURL       = wikiBaseURL + "/static/favicon/wikipedia.ico"

    defaultPage          = 1
    defaultSize          = 10

    jitterBaseMillis     = 100
    jitterRangeMillis    = 50
)

// sleepWithJitter gently spaces requests to avoid bursty calls
func sleepWithJitter() {
    delay := time.Duration(jitterBaseMillis+rand.Intn(jitterRangeMillis)) * time.Millisecond
    time.Sleep(delay)
}

// fetchOK wraps httpx.Get and ensures a 2xx status, returning the body or an error-equivalent signal
func fetchOK(ctx context.Context, target string) ([]byte, error) {
    body, status, err := httpx.Get(ctx, target)
    if err != nil {
        return nil, err
    }
    if status < 200 || status >= 300 {
        return nil, fmt.Errorf("wikipedia: http %d", status)
    }
    return body, nil
}

// buildOpenSearchURL constructs the OpenSearch API URL
func buildOpenSearchURL(q string, size int) string {
    // Wikipedia OpenSearch API - returns JSON array with [query, titles, descriptions, urls]
    params := url.Values{}
    params.Set("action", "opensearch")
    params.Set("limit", strconv.Itoa(size))
    params.Set("namespace", "0")
    params.Set("redirects", "resolve")
    params.Set("format", "json")
    params.Set("search", q)
    return wikiBaseURL + openSearchPath + "?" + params.Encode()
}



// Search queries Wikipedia OpenSearch API and returns article results with enhanced snippets
func (w *wikipedia) Search(ctx context.Context, q string, page int, size int) ([]en.Result, error) {
    if strings.TrimSpace(q) == "" { return nil, nil }
    if page <= 0 { page = defaultPage }
    if size <= 0 { size = defaultSize }

    // Gentle spacing to avoid bursty calls
    sleepWithJitter()

    // Wikipedia OpenSearch API - returns JSON array with [query, titles, descriptions, urls]
    searchURL := buildOpenSearchURL(q, size)

    body, err := fetchOK(ctx, searchURL)
    if err != nil { return nil, err }
    if body == nil { return nil, nil }

    // Use basic parsing for speed - summary API is too slow
    return parseWikipediaOpenSearch(body)
}



// parseWikipediaOpenSearch parses Wikipedia OpenSearch JSON response
// Format: [query, [titles...], [descriptions...], [urls...]]
func parseWikipediaOpenSearch(body []byte) ([]en.Result, error) {
    // Wikipedia OpenSearch returns: ["query", ["title1", "title2"], ["desc1", "desc2"], ["url1", "url2"]]
    var response []interface{}
    if err := json.Unmarshal(body, &response); err != nil {
        return nil, err
    }

    if len(response) < 4 {
        return nil, nil
    }

    // Extract arrays from response
    titlesArray, ok1 := response[1].([]interface{})
    descriptionsArray, ok2 := response[2].([]interface{})
    urlsArray, ok3 := response[3].([]interface{})

    if !ok1 || !ok2 || !ok3 {
        return nil, nil
    }

    // Build results
    var results []en.Result
    maxLen := len(titlesArray)
    if len(descriptionsArray) < maxLen {
        maxLen = len(descriptionsArray)
    }
    if len(urlsArray) < maxLen {
        maxLen = len(urlsArray)
    }

    for i := 0; i < maxLen; i++ {
        title, titleOk := titlesArray[i].(string)
        description, descOk := descriptionsArray[i].(string)
        url, urlOk := urlsArray[i].(string)

        if titleOk && urlOk && title != "" && url != "" {
            snippet := ""
            if descOk {
                snippet = description
            }
            
            result := en.Result{
                Title:   title,
                URL:     url,
                Snippet: snippet,
                Engine:  "wikipedia",
                Favicon: wikiFaviconURL,
            }
            results = append(results, result)
        }
    }

    return results, nil
}

