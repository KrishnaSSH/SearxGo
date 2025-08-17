package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"searxgo/internal/httpx"
	yaml "gopkg.in/yaml.v3"
)

// Fact represents a key-value fact about an entity
type Fact struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ------------------- Config for facts (JSON) -------------------

type PropCfg struct {
	ID       string `json:"id"`
	Key      string `json:"key"`
	Type     string `json:"type"` // entityid, time, string, quantity, url
	Multiple bool   `json:"multiple,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

type FactsDefaults struct {
	MaxDefault int       `json:"max_default"`
	Properties []PropCfg `json:"properties"`
	Priority   []string  `json:"priority"`
}

type FactsConfig struct {
	Defaults FactsDefaults `json:"defaults"`
}

func (ks *KnowledgeService) loadFactsConfig() {
	// Load config relative to repository root. Prefer YAML, fallback to JSON.
	var cfg FactsConfig
	// Try YAML first
	ypath := filepath.Join("internal", "server", "facts.yaml")
	if b, err := os.ReadFile(ypath); err == nil {
		if err := yaml.Unmarshal(b, &cfg); err == nil {
			ks.factsCfg = cfg
			return
		}
	}
	// Fallback to JSON
	jpath := filepath.Join("internal", "server", "facts.json")
	if b, err := os.ReadFile(jpath); err == nil {
		if err := json.Unmarshal(b, &cfg); err == nil {
			ks.factsCfg = cfg
			return
		}
	}
	// If both fail, leave zero-value config
}

// KnowledgeCard represents a Wikipedia knowledge card response
type KnowledgeCard struct {
	Source      string `json:"source"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Extract     string `json:"extract,omitempty"`
	URL         string `json:"url,omitempty"`
	Thumbnail   string `json:"thumbnail,omitempty"`
	Facts       []Fact `json:"facts,omitempty"`
	Website     string `json:"website,omitempty"`
}

// WikipediaSearchResult represents the result from Wikipedia OpenSearch
type WikipediaSearchResult struct {
	Title       string
	Description string
	PageURL     string
}

// WikipediaSummaryResult represents the result from Wikipedia summary API
type WikipediaSummaryResult struct {
	PageURL string
	Desc    string
	Extract string
	Thumb   string
	Type    string
}

// KnowledgeService handles Wikipedia knowledge card operations
type KnowledgeService struct {
	timeout time.Duration
	factsCfg FactsConfig
}

// NewKnowledgeService creates a new knowledge service
func NewKnowledgeService(timeout time.Duration) *KnowledgeService {
	if timeout <= 0 {
		timeout = 2500 * time.Millisecond
	}
	ks := &KnowledgeService{timeout: timeout}
	ks.loadFactsConfig()
	return ks
}

// ------------------- Helpers -------------------

func (ks *KnowledgeService) httpGetJSON(ctx context.Context, url string, target any) error {
	// Use the service timeout for per-request operations, clamped to a
	// reasonable window to improve reliability while keeping UI responsive.
	per := ks.timeout
	if per <= 0 {
		per = 2500 * time.Millisecond
	}
	if per < 2500*time.Millisecond {
		per = 2500 * time.Millisecond
	} else if per > 3500*time.Millisecond {
		per = 3500 * time.Millisecond
	}
	sctx, cancel := context.WithTimeout(ctx, per)
	defer cancel()
	body, status, err := httpx.Get(sctx, url)
	if err != nil || status < 200 || status >= 300 {
		return err
	}
	return json.NewDecoder(bytes.NewReader(body)).Decode(target)
}

func getFirstString(arr any) string {
	if s, ok := arr.([]any); ok && len(s) > 0 {
		if str, ok2 := s[0].(string); ok2 {
			return str
		}
	}
	return ""
}

func trimDate(val string) string {
	val = strings.TrimPrefix(val, "+")
	if i := strings.IndexByte(val, 'T'); i > 0 {
		val = val[:i]
	}
	return val
}

func isQID(s string) bool {
	if len(s) < 2 || s[0] != 'Q' {
		return false
	}
	for i := 1; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func formatNumber(s string) string {
	if s == "" {
		return s
	}
	neg := false
	if s[0] == '-' {
		neg = true
		s = s[1:]
	}
	// strip decimals if present
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = s[:i]
	}
	// not a number
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return s
		}
	}
	n := len(s)
	if n <= 3 {
		if neg {
			return "-" + s
		}
		return s
	}
	rem := n % 3
	if rem == 0 {
		rem = 3
	}
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	b.WriteString(s[:rem])
	for i := rem; i < n; i += 3 {
		b.WriteByte(',')
		b.WriteString(s[i:i+3])
	}
	return b.String()
}

func (ks *KnowledgeService) resolveEntityLabels(ctx context.Context, ids []string) map[string]string {
	labels := map[string]string{}
	if len(ids) == 0 {
		return labels
	}
	// de-duplicate
	uniq := make([]string, 0, len(ids))
	seen := map[string]struct{}{}
	for _, id := range ids {
		if !isQID(id) {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	// chunk requests to keep URLs reasonable
	const chunkSize = 40
	for i := 0; i < len(uniq); i += chunkSize {
		end := i + chunkSize
		if end > len(uniq) {
			end = len(uniq)
		}
		idChunk := strings.Join(uniq[i:end], "|")
		api := "https://www.wikidata.org/w/api.php?action=wbgetentities&props=labels&languages=en&format=json&ids=" + url.QueryEscape(idChunk)
		var resp struct {
			Entities map[string]struct {
				Labels map[string]struct {
					Value string `json:"value"`
				} `json:"labels"`
			} `json:"entities"`
		}
		_ = ks.httpGetJSON(ctx, api, &resp)
		for id, ent := range resp.Entities {
			if l, ok := ent.Labels["en"]; ok && l.Value != "" {
				labels[id] = l.Value
			}
		}
	}
	return labels
}

// ------------------- Wikipedia -------------------

func (ks *KnowledgeService) SearchWikipedia(ctx context.Context, query string) (*WikipediaSearchResult, error) {
	searchURL := "https://en.wikipedia.org/w/api.php?action=opensearch&limit=1&namespace=0&redirects=resolve&format=json&search=" + url.QueryEscape(query)
	var arr []any
	if err := ks.httpGetJSON(ctx, searchURL, &arr); err != nil || len(arr) < 4 {
		return nil, err
	}
	return &WikipediaSearchResult{
		Title:       getFirstString(arr[1]),
		Description: getFirstString(arr[2]),
		PageURL:     getFirstString(arr[3]),
	}, nil
}

func (ks *KnowledgeService) FetchSummary(ctx context.Context, title string) (*WikipediaSummaryResult, error) {
	sumURL := "https://en.wikipedia.org/api/rest_v1/page/summary/" + url.PathEscape(title) + "?redirect=true"
	var sum struct {
		ContentURLs struct {
			Desktop struct {
				Page string `json:"page"`
			} `json:"desktop"`
		} `json:"content_urls"`
		Extract     string  `json:"extract"`
		Description string  `json:"description"`
		Thumbnail   *struct {
			Source string `json:"source"`
		} `json:"thumbnail"`
		Type string `json:"type"`
	}
	if err := ks.httpGetJSON(ctx, sumURL, &sum); err != nil {
		return &WikipediaSummaryResult{}, nil
	}

	thumb := ""
	if sum.Thumbnail != nil {
		thumb = sum.Thumbnail.Source
	}

	return &WikipediaSummaryResult{
		PageURL: sum.ContentURLs.Desktop.Page,
		Desc:    sum.Description,
		Extract: sum.Extract,
		Thumb:   thumb,
		Type:    sum.Type,
	}, nil
}

func (ks *KnowledgeService) QueryBestTitle(ctx context.Context, query string) (string, error) {
	type searchResp struct {
		Query struct {
			Search []struct {
				Title   string `json:"title"`
				Snippet string `json:"snippet"`
			} `json:"search"`
		} `json:"query"`
	}
	api := "https://en.wikipedia.org/w/api.php?action=query&list=search&srnamespace=0&srlimit=5&srwhat=nearmatch&format=json&srsearch=" + url.QueryEscape(query)
	var resp searchResp
	if err := ks.httpGetJSON(ctx, api, &resp); err != nil {
		return "", err
	}
	// Prefer first non-disambiguation result
	for _, s := range resp.Query.Search {
		lt := strings.ToLower(s.Title)
		ls := strings.ToLower(s.Snippet)
		if strings.Contains(lt, "(disambiguation)") || strings.Contains(lt, "disambiguation") || strings.Contains(ls, "disambiguation") {
			continue
		}
		return s.Title, nil
	}
	// Fallback to first result if nothing better
	if len(resp.Query.Search) > 0 {
		return resp.Query.Search[0].Title, nil
	}
	return "", nil
}

// ------------------- Wikidata -------------------

func (ks *KnowledgeService) FetchWikidataFacts(ctx context.Context, title string) ([]Fact, error) {
	if title == "" {
		return nil, nil
	}

	var qres struct {
		Query struct {
			Pages map[string]struct {
				PageProps struct {
					Item string `json:"wikibase_item"`
				} `json:"pageprops"`
			} `json:"pages"`
		} `json:"query"`
	}
	qURL := "https://en.wikipedia.org/w/api.php?action=query&prop=pageprops&ppprop=wikibase_item&format=json&titles=" + url.QueryEscape(title)
	if err := ks.httpGetJSON(ctx, qURL, &qres); err != nil {
		return nil, nil
	}

	var qid string
	for _, p := range qres.Query.Pages {
		if p.PageProps.Item != "" {
			qid = p.PageProps.Item
			break
		}
	}
	if qid == "" {
		return nil, nil
	}

	facts, _ := ks.fetchEntityData(ctx, qid)
	// Collect Q-IDs to resolve into human-readable labels
	idSet := map[string]struct{}{}
	for _, f := range facts {
		// Split on comma+space as we join multiples that way
		parts := strings.Split(f.Value, ", ")
		for _, p := range parts {
			if isQID(p) {
				idSet[p] = struct{}{}
			}
		}
	}
	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	labelMap := ks.resolveEntityLabels(ctx, ids)
	// Replace any Q-IDs in fact values with labels
	for i := range facts {
		parts := strings.Split(facts[i].Value, ", ")
		changed := false
		for j, p := range parts {
			if lbl, ok := labelMap[p]; ok {
				parts[j] = lbl
				changed = true
			} else if facts[i].Key == "Employees" { // format quantities nicely
				parts[j] = formatNumber(p)
			}
		}
		if changed || facts[i].Key == "Employees" {
			facts[i].Value = strings.Join(parts, ", ")
		}
	}
	return facts, nil
}

func (ks *KnowledgeService) fetchEntityData(ctx context.Context, qid string) ([]Fact, error) {
	var ed struct {
		Entities map[string]struct {
			Claims map[string][]struct {
				Mainsnak struct {
					Datavalue struct {
						Type  string `json:"type"`
						Value any    `json:"value"`
					} `json:"datavalue"`
				} `json:"mainsnak"`
			} `json:"claims"`
		} `json:"entities"`
	}
	wdURL := "https://www.wikidata.org/wiki/Special:EntityData/" + url.PathEscape(qid) + ".json"
	if err := ks.httpGetJSON(ctx, wdURL, &ed); err != nil {
		return nil, nil
	}

	ent, ok := ed.Entities[qid]
	if !ok {
		return nil, nil
	}

	facts := ks.extractFacts(ent.Claims)
	// Try to pull a logo image (P154) and expose as a pseudo-fact so caller can prefer it as thumbnail
	if cs, ok := ent.Claims["P154"]; ok && len(cs) > 0 {
		for _, claim := range cs {
			// Datavalue type for P154 is usually "commonsMedia" with a filename string
			if m, ok2 := claim.Mainsnak.Datavalue.Value.(map[string]any); ok2 {
				if name, ok3 := m["title"].(string); ok3 && name != "" {
					logo := "https://commons.wikimedia.org/wiki/Special:FilePath/" + url.PathEscape(name) + "?width=320"
					facts = append([]Fact{{Key: "LogoURL", Value: logo}}, facts...)
					break
				}
			} else if s, ok3 := claim.Mainsnak.Datavalue.Value.(string); ok3 && s != "" {
				logo := "https://commons.wikimedia.org/wiki/Special:FilePath/" + url.PathEscape(s) + "?width=320"
				facts = append([]Fact{{Key: "LogoURL", Value: logo}}, facts...)
				break
			}
		}
	}
	return facts, nil
}

// ------------------- Fact Extraction (Config-driven) -------------------

func (ks *KnowledgeService) extractFacts(claims map[string][]struct {
	Mainsnak struct {
		Datavalue struct {
			Type  string `json:"type"`
			Value any    `json:"value"`
		} `json:"datavalue"`
	} `json:"mainsnak"`
}) []Fact {

	getVal := func(c struct {
		Mainsnak struct {
			Datavalue struct {
				Type  string `json:"type"`
				Value any    `json:"value"`
			} `json:"datavalue"`
		} `json:"mainsnak"`
	}, t string) string {
		switch t {
		case "entityid":
			if c.Mainsnak.Datavalue.Type == "wikibase-entityid" {
				if m, ok := c.Mainsnak.Datavalue.Value.(map[string]any); ok {
					if id, ok2 := m["id"].(string); ok2 {
						return id
					}
				}
			}
		case "time":
			if c.Mainsnak.Datavalue.Type == "time" {
				if m, ok := c.Mainsnak.Datavalue.Value.(map[string]any); ok {
					if t, ok2 := m["time"].(string); ok2 {
						return trimDate(t)
					}
				}
			}
		case "string", "url":
			if c.Mainsnak.Datavalue.Type == t {
				if s, ok := c.Mainsnak.Datavalue.Value.(string); ok {
					return s
				}
			}
		case "quantity":
			if c.Mainsnak.Datavalue.Type == "quantity" {
				if m, ok := c.Mainsnak.Datavalue.Value.(map[string]any); ok {
					if a, ok2 := m["amount"].(string); ok2 {
						return strings.TrimPrefix(a, "+")
					}
				}
			}
		}
		return ""
	}

	var facts []Fact
	props := ks.factsCfg.Defaults.Properties
	// Build a quick lookup for which keys are quantities (for formatting)
	qtyKeys := map[string]bool{}
	for _, p := range props {
		if p.Type == "quantity" {
			qtyKeys[p.Key] = true
		}
	}
	for _, p := range props {
		cs := claims[p.ID]
		if len(cs) == 0 {
			continue
		}
		if p.Multiple {
			vals := []string{}
			for _, c := range cs {
				v := getVal(c, p.Type)
				if v != "" {
					vals = append(vals, v)
				}
				if p.Limit > 0 && len(vals) >= p.Limit {
					break
				}
			}
			if len(vals) > 0 {
				facts = append(facts, Fact{Key: p.Key, Value: strings.Join(vals, ", ")})
			}
		} else {
			v := getVal(cs[0], p.Type)
			if v != "" {
				facts = append(facts, Fact{Key: p.Key, Value: v})
			}
		}
	}
	// Post-processing for quantity formatting
	for i := range facts {
		if qtyKeys[facts[i].Key] {
			facts[i].Value = formatNumber(facts[i].Value)
		}
	}
	return facts
}

// ------------------- Build Knowledge Card -------------------
func (ks *KnowledgeService) BuildKnowledgeCard(search *WikipediaSearchResult, summary *WikipediaSummaryResult, facts []Fact) *KnowledgeCard {
	card := &KnowledgeCard{
		Source: "wikipedia",
		Title:  search.Title,
		URL:    search.PageURL,
	}

	if summary != nil {
		if summary.PageURL != "" {
			card.URL = summary.PageURL
		}
		if summary.Desc != "" {
			card.Description = summary.Desc
		} else {
			card.Description = search.Description
		}
		card.Extract = summary.Extract
		card.Thumbnail = summary.Thumb
	} else {
		card.Description = search.Description
	}

	hasLogo := false
	for _, f := range facts {
		if strings.EqualFold(f.Key, "Website") {
			card.Website = f.Value
		}
		if strings.EqualFold(f.Key, "LogoURL") && f.Value != "" {
			// Prefer logo image over page screenshot
			card.Thumbnail = f.Value
			hasLogo = true
		}
	}
	// If no Wikidata logo, fall back to the site's favicon derived from Website
	if !hasLogo && card.Website != "" {
		if u, err := url.Parse(card.Website); err == nil {
			host := u.Hostname()
			if host != "" {
				// Use DuckDuckGo icons service – lightweight and reliable
				card.Thumbnail = "https://icons.duckduckgo.com/ip3/" + host + ".ico"
			}
		}
	}
	card.Facts = ks.condenseFacts(facts)
	return card
}

func (ks *KnowledgeService) condenseFacts(facts []Fact) []Fact {
    // Priority order is config-driven. Frontend collapses to top N.
    order := ks.factsCfg.Defaults.Priority
    // Label tweaks for brevity
    labelMap := map[string]string{
        "Headquarters": "HQ",
        "Founded by":   "Founders",
        "Initial release": "Initial",
    }
    // Build lookup from key -> value
    kv := map[string]string{}
    for _, f := range facts {
        if f.Key == "Website" || f.Key == "LogoURL" { // shown separately / internal only
            continue
        }
        if f.Value == "" {
            continue
        }
        // prefer first occurrence
        if _, ok := kv[f.Key]; !ok {
            kv[f.Key] = f.Value
        }
    }
    // Assemble in order with no cap (frontend will collapse to top N by default)
    out := make([]Fact, 0, len(order))
    used := map[string]struct{}{}
    for _, k := range order {
        if v, ok := kv[k]; ok && v != "" {
            origK := k
            if short, ok2 := labelMap[k]; ok2 { k = short }
            out = append(out, Fact{Key: k, Value: v})
            // mark the original key as used so it won't appear in the remainder
            used[origK] = struct{}{}
        }
    }
    // Append any remaining facts not covered by priority, alphabetically by key
    rest := make([]string, 0, len(kv))
    for k := range kv {
        if _, ok := used[k]; !ok {
            rest = append(rest, k)
        }
    }
    if len(rest) > 0 {
        // simple insertion sort to avoid importing sort for small slices
        for i := 1; i < len(rest); i++ {
            j := i
            for j > 0 && rest[j] < rest[j-1] {
                rest[j], rest[j-1] = rest[j-1], rest[j]
                j--
            }
        }
        for _, k := range rest {
            v := kv[k]
            if short, ok2 := labelMap[k]; ok2 { k = short }
            out = append(out, Fact{Key: k, Value: v})
        }
    }
    return out
}
