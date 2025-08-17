package engines

import (
    "bytes"
    "context"
    "net/url"
    "strconv"
    "strings"

    "github.com/PuerkitoBio/goquery"
    en "searxgo/internal/engine"
    "searxgo/internal/httpx"
)

type google struct{}

func NewGoogle() en.SearchEngine { return &google{} }

func (g *google) Name() string { return "google" }

// buildGoogleURL builds a Google Web search URL similar to SearXNG behavior.
// We keep it simple and robust to avoid tripping bot defenses.
func buildGoogleURL(query string, page int) (string, map[string]string) {
    if page < 1 { page = 1 }
    start := (page - 1) * 10

    q := url.Values{}
    q.Set("q", query)
    q.Set("hl", "en-US")      // interface language
    q.Set("lr", "lang_en")    // restrict to English (keeps results consistent)
    q.Set("ie", "utf8")
    q.Set("oe", "utf8")
    q.Set("filter", "0")      // disable near-dup filtering to match SearXNG defaults
    q.Set("start", strconv.Itoa(start))

    u := "https://www.google.com/search?" + q.Encode()

    // Basic hardening headers and CONSENT cookie to reduce interstitials.
    hdrs := map[string]string{
        "User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
        "Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
        "Accept-Language": "en-US,en;q=0.8",
        "Connection":      "keep-alive",
        "Referer":         "https://www.google.com/",
        // SearXNG sets CONSENT=YES+ to avoid the consent page
        "Cookie":          "CONSENT=YES+",
    }
    return u, hdrs
}

func (g *google) Search(ctx context.Context, q string, page int, size int) ([]en.Result, error) {
    if strings.TrimSpace(q) == "" { return nil, nil }
    if page <= 0 { page = 1 }
    if size <= 0 { size = 10 }

    urlStr, headers := buildGoogleURL(q, page)

    body, status, err := httpx.GetWithHeaders(ctx, urlStr, headers)
    if err != nil || status < 200 || status >= 300 {
        return nil, nil
    }

    doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
    if err != nil { return nil, nil }

    // Detect captcha/sorry page quickly
    if strings.Contains(strings.ToLower(string(body)), "sorry") && strings.Contains(strings.ToLower(string(body)), "google") {
        // Avoid surfacing captcha pages as results; just return empty
        return nil, nil
    }

    out := make([]en.Result, 0, size)

    // Google web results: robust selectors
    // Primary cards: div[jscontroller*="SC7lYd"] containing a title link with <h3>
    doc.Find("div[jscontroller*='SC7lYd']").Each(func(i int, s *goquery.Selection) {
        if len(out) >= size { return }
        // Title and URL
        a := s.Find("a:has(h3)").First()
        if a.Length() == 0 {
            // Sometimes h3 is a direct child of a div and the <a> wraps it indirectly
            a = s.Find("a h3").ParentsFiltered("a").First()
        }
        if a.Length() == 0 { return }
        href, _ := a.Attr("href")
        h3 := a.Find("h3").First()
        title := strings.TrimSpace(h3.Text())
        if href == "" || title == "" { return }

        // Snippet/content area: div[data-sncf="1"] is common in the lightweight UI
        snippet := strings.TrimSpace(s.Find("div[data-sncf='1']").First().Text())
        if snippet == "" {
            // Fallback to generic snippet container
            snippet = strings.TrimSpace(s.Find("div.VwiC3b, div.yXK7lf, span.aCOpRe").First().Text())
        }

        // Build favicon from URL host
        fav := ""
        if u, err := url.Parse(href); err == nil {
            if host := u.Hostname(); host != "" {
                fav = "https://icons.duckduckgo.com/ip3/" + host + ".ico"
            }
        }

        out = append(out, en.Result{Title: title, URL: href, Snippet: snippet, Engine: g.Name(), Favicon: fav})
    })

    return out, nil
}
