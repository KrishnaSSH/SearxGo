package suggest

import (
	"context"
	"encoding/json"
	"net/url"

	"searxgo/internal/httpx"
)

// Provider defines an autocomplete suggestion provider.
// Implementations must respect the provided context for cancellation/timeouts
// and return a list of suggestion strings (no persistence to disk).
type Provider interface {
	Suggest(ctx context.Context, query string) ([]string, error)
}

// Manager selects a provider (by name) and delegates Suggest calls.
// If the requested provider is unknown or empty, the default provider is used.
type Manager struct {
	providers   map[string]Provider
	defaultName string
}

// NewManager creates a new Manager with the given providers and default provider name.
func NewManager(defaultName string, providers map[string]Provider) *Manager {
	if providers == nil {
		providers = make(map[string]Provider)
	}
	return &Manager{providers: providers, defaultName: defaultName}
}

// Suggest proxies to the selected provider. Falls back to default if providerName is empty or unknown.
func (m *Manager) Suggest(ctx context.Context, providerName, query string) ([]string, error) {
	p := m.providers[providerName]
	if p == nil {
		p = m.providers[m.defaultName]
	}
	if p == nil {
		return nil, nil
	}
	return p.Suggest(ctx, query)
}

// GoogleProvider implements Provider using Google Suggest (Firefox client) endpoint.
// It only uses in-memory processing and respects the caller context.
// This provider does not perform any disk I/O.
type GoogleProvider struct{}

func NewGoogleProvider() *GoogleProvider { return &GoogleProvider{} }

func (g *GoogleProvider) Suggest(ctx context.Context, query string) ([]string, error) {
	target := "https://suggestqueries.google.com/complete/search?client=firefox&q=" + url.QueryEscape(query)
	headers := map[string]string{
		"Accept":          "application/json,text/javascript,*/*;q=0.1",
		"Accept-Language": "en-US,en;q=0.9",
		"Referer":         "https://www.google.com/",
		"User-Agent":      "SearxGO/1.0 (+autocomplete)",
	}
	body, code, err := httpx.GetWithHeaders(ctx, target, headers)
	if err != nil || code < 200 || code >= 300 {
		return nil, err
	}
	var root []any
	if err := json.Unmarshal(body, &root); err != nil || len(root) < 2 {
		return nil, nil
	}
	var out []string
	if arr, ok := root[1].([]any); ok {
		for _, v := range arr {
			if s, ok := v.(string); ok && s != "" {
				out = append(out, s)
			}
		}
	}
	return out, nil
}
