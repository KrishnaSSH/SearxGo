package aggregate

import (
	"context"
	"log"
	"net/url"
	"sort"
	"strings"
	"time"

	en "searxgo/internal/engine"
)

// Timing reports what a single engine did for a query. It is emitted for every
// engine that was queried – including ones that errored or returned nothing –
// so callers can tell "no results" apart from "engine failed".
type Timing struct {
	Engine string `json:"engine"`
	Ms     int64  `json:"ms"`
	Count  int    `json:"count"`
	Error  string `json:"error,omitempty"`
}

// entry is an aggregated result across engines, tracking every engine that
// returned it and the 1-based position it appeared at in each engine's list.
type entry struct {
	item      en.Result
	engines   map[string]struct{}
	positions []int
	score     float64
	order     int // first-seen order, used as a stable tie-breaker
}

// Run queries all engines concurrently and merges their results the way SearXNG
// does: every engine that answers within the timeout contributes, duplicates are
// detected by a normalized URL, and results are ranked by a consensus score
// (engine weight × appearances, summed as weight/position over every position).
//
// The returned slice is the full ranked set; callers slice it to the page size.
func Run(parent context.Context, engines []en.SearchEngine, query string, timeout time.Duration, page int, size int) ([]en.Result, []Timing) {
	if len(engines) == 0 || strings.TrimSpace(query) == "" {
		return nil, nil
	}

	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	type resp struct {
		engine string
		items  []en.Result
		err    error
		dur    time.Duration
	}
	ch := make(chan resp, len(engines))

	for _, e := range engines {
		e := e
		go func() {
			name := e.Name()
			start := time.Now()
			// Each engine gets the whole budget; the shared ctx bounds the total.
			items, err := e.Search(ctx, query, page, size)
			ch <- resp{engine: name, items: items, err: err, dur: time.Since(start)}
		}()
	}

	byURL := make(map[string]*entry, 128)
	var timings []Timing
	order := 0

	record := func(r resp) {
		timings = append(timings, Timing{
			Engine: r.engine,
			Ms:     r.dur.Milliseconds(),
			Count:  len(r.items),
			Error:  errString(r.err),
		})
		if r.err != nil {
			log.Printf("aggregate: engine %q failed after %dms: %v", r.engine, r.dur.Milliseconds(), r.err)
		} else if len(r.items) == 0 {
			log.Printf("aggregate: engine %q returned no results after %dms", r.engine, r.dur.Milliseconds())
		}

		for idx, it := range r.items {
			key := normalizeURL(it.URL)
			if key == "" {
				continue
			}
			pos := idx + 1
			if ent, ok := byURL[key]; ok {
				ent.engines[r.engine] = struct{}{}
				ent.positions = append(ent.positions, pos)
				// Keep the richest text and prefer an https canonical URL.
				if len(it.Title) > len(ent.item.Title) {
					ent.item.Title = it.Title
				}
				if len(it.Snippet) > len(ent.item.Snippet) {
					ent.item.Snippet = it.Snippet
				}
				if ent.item.Favicon == "" && it.Favicon != "" {
					ent.item.Favicon = it.Favicon
				}
				if strings.HasPrefix(it.URL, "https://") && !strings.HasPrefix(ent.item.URL, "https://") {
					ent.item.URL = it.URL
				}
			} else {
				byURL[key] = &entry{
					item:      it,
					engines:   map[string]struct{}{r.engine: {}},
					positions: []int{pos},
					order:     order,
				}
				order++
			}
		}
	}

	// Collect every engine response until they are all in or the deadline hits.
	// The channel is buffered to len(engines), so late goroutines never block and
	// never leak even once we stop reading.
	got := 0
collect:
	for got < len(engines) {
		select {
		case r := <-ch:
			got++
			record(r)
		case <-ctx.Done():
			for {
				select {
				case r := <-ch:
					got++
					record(r)
				default:
					break collect
				}
			}
		}
	}

	// SearXNG-style scoring:
	//   weight = product(engine weights) * number of appearances
	//   score  = sum over positions of weight / position
	weights := defaultEngineWeights()
	results := make([]*entry, 0, len(byURL))
	for _, ent := range byURL {
		weight := 1.0
		for engName := range ent.engines {
			if w, ok := weights[engName]; ok {
				weight *= w
			}
		}
		weight *= float64(len(ent.positions))

		score := 0.0
		for _, pos := range ent.positions {
			if pos > 0 {
				score += weight / float64(pos)
			}
		}
		ent.score = score
		results = append(results, ent)
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score > results[j].score
		}
		return results[i].order < results[j].order
	})

	out := make([]en.Result, 0, len(results))
	for _, ent := range results {
		out = append(out, ent.item)
	}
	return out, timings
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// normalizeURL produces a dedupe key that treats http/https, a trailing slash,
// a leading "www.", and URL fragments as equivalent – mirroring SearXNG's
// duplicate detection so consensus scoring actually merges the same page.
func normalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return strings.ToLower(raw)
	}
	host := strings.ToLower(u.Host)
	host = strings.TrimPrefix(host, "www.")
	path := strings.TrimSuffix(u.EscapedPath(), "/")
	key := host + path
	if u.RawQuery != "" {
		key += "?" + u.RawQuery
	}
	return key
}

// defaultEngineWeights returns SearXNG-style default weights. Engines without an
// explicit weight default to 1.0.
func defaultEngineWeights() map[string]float64 {
	return map[string]float64{
		"bing":          1.0,
		"google":        1.0,
		"duckduckgo":    1.3,
		"mojeek":        1.2,
		"wikipedia":     0.6,
		"openlibrary":   0.5,
		"hackernews":    0.9,
		"reddit":        0.9,
		"stackoverflow": 0.9,
	}
}
