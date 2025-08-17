package httpx

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var transport = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:   2 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          256,
	MaxIdleConnsPerHost:   64,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   3 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	ResponseHeaderTimeout: 4 * time.Second,
}

// PostForm submits application/x-www-form-urlencoded data with default headers.
func PostForm(ctx context.Context, target string, data url.Values) ([]byte, int, error) {
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(data.Encode()))
    if err != nil {
        return nil, 0, err
    }
    req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36")
    req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
    req.Header.Set("Accept-Language", "en-US,en;q=0.9")
    if req.Header.Get("Referer") == "" {
        req.Header.Set("Referer", "https://duckduckgo.com/")
    }
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    // Some sites (e.g., DuckDuckGo HTML) are less likely to show bot challenges if Referer is present
    if req.Header.Get("Referer") == "" {
        req.Header.Set("Referer", "https://duckduckgo.com/")
    }
    resp, err := defaultClient.Do(req)
    if err != nil {
        return nil, 0, err
    }
    defer resp.Body.Close()
    b, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, resp.StatusCode, err
    }
    return b, resp.StatusCode, nil
}

var defaultClient = &http.Client{
	Transport: transport,
	Timeout:   6 * time.Second,
}

// Get fetches a URL with a default User-Agent and context timeout.
func Get(ctx context.Context, url string) ([]byte, int, error) {
    return GetWithHeaders(ctx, url, nil)
}

// GetWithHeaders fetches a URL with custom headers; common defaults filled in.
func GetWithHeaders(ctx context.Context, target string, headers map[string]string) ([]byte, int, error) {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
    if err != nil { return nil, 0, err }
    // Defaults
    req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36")
    req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
    req.Header.Set("Accept-Language", "en-US,en;q=0.9")
    for k, v := range headers {
        if v == "" { continue }
        req.Header.Set(k, v)
    }
    resp, err := defaultClient.Do(req)
    if err != nil { return nil, 0, err }
    defer resp.Body.Close()
    b, err := io.ReadAll(resp.Body)
    if err != nil { return nil, resp.StatusCode, err }
    return b, resp.StatusCode, nil
}
