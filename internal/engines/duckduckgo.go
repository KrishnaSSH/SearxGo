package engines

import (
    "bytes"
    "context"
    "net/url"
    "regexp"
    "strconv"
    "strings"
    "sync"

    "github.com/PuerkitoBio/goquery"
    en "searxgo/internal/engine"
    "searxgo/internal/httpx"
)

type duckduckgo struct{}

func NewDuckDuckGo() en.SearchEngine { return &duckduckgo{} }

func (d *duckduckgo) Name() string { return "duckduckgo" }

// vqd cache (in-memory) keyed by query+region
type vqdEntry struct{ v string }

var ddgVQD sync.Map // key: query+"//"+region, value: vqdEntry

var vqdRe = regexp.MustCompile(`vqd="([^"]+)"`)

func getVQD(ctx context.Context, query string) string {
    // region hardcoded to us-en for now
    key := query + "//us-en"
    if v, ok := ddgVQD.Load(key); ok {
        if e, ok2 := v.(vqdEntry); ok2 && e.v != "" {
            return e.v
        }
    }
    // fetch from main site
    u := "https://duckduckgo.com/?q=" + url.QueryEscape(query)
    body, status, err := httpx.GetWithHeaders(ctx, u, map[string]string{"Referer": "https://duckduckgo.com/"})
    if err != nil || status < 200 || status >= 300 {
        return ""
    }
    // extract vqd
    m := vqdRe.FindSubmatch(body)
    if len(m) >= 2 {
        v := string(m[1])
        ddgVQD.Store(key, vqdEntry{v: v})
        return v
    }
    return ""
}

func (d *duckduckgo) Search(ctx context.Context, q string, page int, size int) ([]en.Result, error) {
    if strings.TrimSpace(q) == "" { return nil, nil }
    if page <= 0 { page = 1 }
    if size <= 0 { size = 10 }

    host := "https://html.duckduckgo.com/html/"

    form := url.Values{}
    form.Set("q", q)
    // region/language kl like SearXNG; use us-en
    form.Set("kl", "us-en")

    if page == 1 {
        // First page: set b=""
        form.Set("b", "")
    } else {
        // Page 2 = 10, Page 3+ = 10 + (n-2)*15
        offset := 10 + (page-2)*15
        form.Set("s", strconv.Itoa(offset))
        form.Set("dc", strconv.Itoa(offset+1))
        form.Set("v", "l")
        form.Set("o", "json")
        form.Set("api", "d.js")
        // vqd required for follow-up pages
        if vqd := getVQD(ctx, q); vqd != "" {
            form.Set("vqd", vqd)
        } else {
            // don't hit DDG without vqd
            return nil, nil
        }
    }

    body, status, err := httpx.PostForm(ctx, host, form)
    if err != nil || status < 200 || status >= 300 { return nil, nil }
    doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
    if err != nil { return nil, nil }

    // Detect CAPTCHA/challenge form
    if doc.Find("form#challenge-form").Length() > 0 {
        return nil, nil
    }
    // Optionally capture vqd from hidden input for caching
    if v := doc.Find("input[name='vqd']").First(); v.Length() > 0 {
        if val, ok := v.Attr("value"); ok && val != "" {
            ddgVQD.Store(q+"//us-en", vqdEntry{v: val})
        }
    }

    // Parse results: #links > div.web-result
    out := make([]en.Result, 0, size)
    doc.Find("#links > div.web-result").Each(func(i int, s *goquery.Selection) {
        if len(out) >= size { return }
        a := s.Find("h2 a").First()
        if a.Length() == 0 { return }
        href, _ := a.Attr("href")
        title := strings.TrimSpace(a.Text())
        if href == "" || title == "" { return }

        snippet := strings.TrimSpace(s.Find("a.result__snippet").First().Text())
        if snippet == "" {
            snippet = strings.TrimSpace(s.Find(".result__snippet").First().Text())
        }
        // derive favicon
        fav := ""
        if u, err := url.Parse(href); err == nil {
            if host := u.Hostname(); host != "" {
                fav = "https://icons.duckduckgo.com/ip3/" + host + ".ico"
            }
        }
        out = append(out, en.Result{Title: title, URL: href, Snippet: snippet, Engine: d.Name(), Favicon: fav})
    })
    return out, nil
}
