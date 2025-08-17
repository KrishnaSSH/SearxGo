package engines

import (
    "context"
    "encoding/json"
    "net/url"
    "strconv"
    "strings"

    en "searxgo/internal/engine"
    "searxgo/internal/httpx"
)

type reddit struct{}

func NewReddit() en.SearchEngine { return &reddit{} }

func (r *reddit) Name() string { return "reddit" }

// Search queries Reddit's public JSON search endpoint.
// Note: pagination beyond the first page requires the `after` cursor.
// This implementation only serves the first page for simplicity and reliability.
func (r *reddit) Search(ctx context.Context, q string, page int, size int) ([]en.Result, error) {
    if strings.TrimSpace(q) == "" { return nil, nil }
    if size <= 0 { size = 10 }

    // reddit requires a real-ish UA; request raw_json=1 to avoid escaped entities
    // Note: pagination would require the `after` parameter; we only fetch the first page.
    searchURL := "https://www.reddit.com/search.json?q=" + url.QueryEscape(q) +
        "&type=link&sort=relevance&limit=" + strconv.Itoa(size) + "&raw_json=1"

    headers := map[string]string{
        "User-Agent":      "SearxGO/1.0 (+https://github.com/kwynx/searxgo)",
        "Accept":          "application/json,text/javascript;q=0.9,*/*;q=0.8",
        "Accept-Language": "en-US,en;q=0.9",
        "Referer":         "https://www.reddit.com/",
    }

    body, status, err := httpx.GetWithHeaders(ctx, searchURL, headers)
    if err != nil || status < 200 || status >= 300 {
        return nil, nil
    }

    // Minimal structures to parse reddit JSON
    type postData struct {
        Title      string  `json:"title"`
        Permalink  string  `json:"permalink"`
        Selftext   string  `json:"selftext"`
        URL        string  `json:"url"`
        CreatedUTC float64 `json:"created_utc"`
        Thumbnail  string  `json:"thumbnail"`
    }
    type child struct {
        Data postData `json:"data"`
    }
    var payload struct {
        Data struct {
            Children []child `json:"children"`
        } `json:"data"`
    }

    if err := json.Unmarshal(body, &payload); err != nil {
        return nil, nil
    }

    children := payload.Data.Children
    if len(children) == 0 {
        return nil, nil
    }

    results := make([]en.Result, 0, size)
    for _, c := range children {
        d := c.Data
        title := strings.TrimSpace(d.Title)
        if title == "" { continue }
        permalink := strings.TrimSpace(d.Permalink)
        urlStr := d.URL
        // Prefer the Reddit permalink to keep a consistent destination
        if permalink != "" {
            urlStr = "https://www.reddit.com" + permalink
        }
        snippet := strings.TrimSpace(d.Selftext)
        if snippet == "" {
            // fallback to show the external URL if selftext is empty
            snippet = urlStr
        }
        if len(snippet) > 300 { snippet = snippet[:300] + "..." }

        results = append(results, en.Result{
            Title:   title,
            URL:     urlStr,
            Snippet: snippet,
            Engine:  r.Name(),
            Favicon: "https://www.redditstatic.com/desktop2x/img/favicon/favicon-32x32.png",
        })
        if len(results) >= size { break }
    }

    return results, nil
}
