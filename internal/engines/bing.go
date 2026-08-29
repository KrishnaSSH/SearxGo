package engines

import (
    "bytes"
    "context"
    "encoding/base64"
    "fmt"
    "net/url"
    "strconv"
    "strings"
    "github.com/PuerkitoBio/goquery"
    en "searxgo/internal/engine"
    "searxgo/internal/httpx"
)

type bing struct{}

func NewBing() en.SearchEngine { return &bing{} }

func (b *bing) Name() string { return "bing" }

// _pageOffset returns Bing's 1-based "first" value for the given page.
func _pageOffset(page int) int { if page < 1 { page = 1 }; return (page-1)*10 + 1 }

// buildBingURL mirrors SearXNG's request() for Bing Web.
func buildBingURL(query string, page int) (string, map[string]string) {
    if page < 1 { page = 1 }
    q := url.Values{}
    q.Set("q", query)
    q.Set("pq", query)
    // Only set &first and &FORM on pages > 1
    if page > 1 {
        q.Set("first", strconv.Itoa(_pageOffset(page)))
        if page == 2 {
            q.Set("FORM", "PERE")
        } else {
            q.Set("FORM", "PERE"+strconv.Itoa(page-2))
        }
    }
    // Base URL
    u := "https://www.bing.com/search?" + q.Encode()
    // Emulate language/region via cookies like SearXNG
    cookies := map[string]string{
        "_EDGE_CD": "m=en-US&u=en-us",
        "_EDGE_S":  "mkt=en-US&ui=en-us",
    }
    // Build Cookie header value
    var cookiePairs []string
    for k, v := range cookies {
        cookiePairs = append(cookiePairs, k+"="+v)
    }
    hdrs := map[string]string{
        "User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
        "Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
        "Accept-Language": "en-US;q=0.9,en;q=0.3",
        "DNT":             "1",
        "Connection":      "keep-alive",
        "Upgrade-Insecure-Requests": "1",
        "Sec-GPC":         "1",
        "Cache-Control":   "max-age=0",
        "Referer":         "https://www.bing.com/",
        "Cookie":          strings.Join(cookiePairs, "; "),
    }
    return u, hdrs
}

func (b *bing) Search(ctx context.Context, q string, page int, size int) ([]en.Result, error) {
    if strings.TrimSpace(q) == "" { return nil, nil }
    if page <= 0 { page = 1 }
    if size <= 0 { size = 10 }

    // Build request like SearXNG
    urlStr, headers := buildBingURL(q, page)

    body, status, err := httpx.GetWithHeaders(ctx, urlStr, headers)
    if err != nil {
        return nil, err
    }
    if status < 200 || status >= 300 {
        return nil, fmt.Errorf("bing: http %d", status)
    }
    doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
    if err != nil { return nil, err }

    // Parse strictly from ol#b_results / li.b_algo
    results := make([]en.Result, 0, size)
    doc.Find("ol#b_results li.b_algo").Each(func(i int, s *goquery.Selection) {
        if len(results) >= size { return }
        link := s.Find("h2 > a").First()
        if link.Length() == 0 { link = s.Find("h2 a").First() }
        if link.Length() == 0 { return }
        href, _ := link.Attr("href")
        title := strings.TrimSpace(link.Text())
        if href == "" { return }
        if title == "" { title = href }

        // unwrap ck/a redirects if present: https://www.bing.com/ck/a?...&u=a1<base64>
        if strings.HasPrefix(href, "https://www.bing.com/ck/") || strings.HasPrefix(href, "https://www.bing.com/aclick") {
            if u, err := url.Parse(href); err == nil {
                v := u.Query().Get("u")
                if v != "" {
                    // remove optional "a1" prefix
                    v = strings.TrimPrefix(v, "a1")
                    // add padding and base64-url decode
                    if m := len(v) % 4; m != 0 {
                        v = v + strings.Repeat("=", (4-m)%4)
                    }
                    if dec, err2 := base64.URLEncoding.DecodeString(v); err2 == nil {
                        if real := string(dec); strings.HasPrefix(real, "http://") || strings.HasPrefix(real, "https://") {
                            href = real
                        }
                    }
                }
            }
        }

        // snippet: prefer Bing caption paragraph, remove decorations
        p := s.Find("div.b_caption > p").First().Clone()
        if p.Length() == 0 {
            p = s.Find("p").First().Clone()
        }
        p.Find("span.algoSlug_icon").Each(func(i int, sp *goquery.Selection) {
            sp.Remove()
        })
        snippet := strings.TrimSpace(p.Text())
        // Guard against JSON-like blobs leaking into snippet
        if len(snippet) > 40 {
            // Heuristic: lots of braces/quotes/colons suggests JSON
            jsonish := 0
            for _, ch := range snippet {
                switch ch {
                case '{', '}', '[', ']', '"', ':', ',':
                    jsonish++
                }
            }
            if float64(jsonish)/float64(len(snippet)) > 0.12 || strings.HasPrefix(strings.TrimSpace(snippet), "{") {
                snippet = ""
            }
        }
        if snippet == "" {
            // fallback to any immediate p under b_algo
            snippet = strings.TrimSpace(s.Find("p").First().Text())
            if len(snippet) > 40 {
                jsonish := 0
                for _, ch := range snippet {
                    switch ch {
                    case '{', '}', '[', ']', '"', ':', ',':
                        jsonish++
                    }
                }
                if float64(jsonish)/float64(len(snippet)) > 0.12 || strings.HasPrefix(strings.TrimSpace(snippet), "{") {
                    snippet = ""
                }
            }
        }
        // favicon from host
        fav := ""
        if u, err := url.Parse(href); err == nil {
            if host := u.Hostname(); host != "" {
                fav = "https://icons.duckduckgo.com/ip3/" + host + ".ico"
            }
        }

        results = append(results, en.Result{Title: title, URL: href, Snippet: snippet, Engine: b.Name(), Favicon: fav})
    })

    return results, nil
}
