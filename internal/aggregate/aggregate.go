package aggregate

import (
	"context"
	"sort"
	"time"

	en "searxgo/internal/engine"
)

// Run queries all engines concurrently with a per-request timeout and returns
// a deduplicated slice of results. Dedupe key is normalized URL.
type Timing struct {
	Engine string `json:"engine"`
	Ms     int64  `json:"ms"`
	Count  int    `json:"count"`
}

func Run(parent context.Context, engines []en.SearchEngine, query string, timeout time.Duration, page int, size int) ([]en.Result, []Timing) {
	if len(engines) == 0 || query == "" {
		return nil, nil
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
	// per-engine sub-timeout: respect overall timeout, cap to 3s for snappier UX
	perEngineTO := timeout
	if perEngineTO > 3*time.Second {
		perEngineTO = 3 * time.Second
	}
	if perEngineTO <= 200*time.Millisecond {
		perEngineTO = 200 * time.Millisecond
	}
	for _, e := range engines {
		e := e
		go func() {
			name := e.Name()
			// child ctx to avoid lingering slow engine
			cctx, ccancel := context.WithTimeout(ctx, perEngineTO)
			start := time.Now()
			items, err := e.Search(cctx, query, page, size)
			dur := time.Since(start)
			ccancel()
			ch <- resp{engine: name, items: items, err: err, dur: dur}
		}()
	}

	// SearXNG-like consensus merge with scoring
	type entry struct {
		item      en.Result
		engines   map[string]struct{}
		positions []int // 1-based positions where it appeared across engines
		score     float64
	}
	byURL := make(map[string]*entry, 128)
	// Early cutoff target: if size>0 use it, else a slightly larger default for better coverage
	cutoff := size
	if cutoff <= 0 {
		cutoff = 20
	}

	var timings []Timing
	// capture results with position within each engine list
	// Do not cancel the context when cutoff is reached; instead, keep draining
	// the channel to avoid canceling in-flight engine requests (which caused
	// context canceled errors). Optionally skip merging after cutoff for speed.
	cutoffReached := false
	for i := 0; i < len(engines); i++ {
		select {
		case r := <-ch:
			// collect timing only if engine produced results
			if len(r.items) > 0 {
				timings = append(timings, Timing{Engine: r.engine, Ms: r.dur.Milliseconds(), Count: len(r.items)})
			}
			// If we've already reached cutoff, skip further merging but continue to drain
			if cutoffReached {
				continue
			}
			for idx, it := range r.items {
				if it.URL == "" {
					continue
				}
				if ent, ok := byURL[it.URL]; ok {
					// merge: prefer longer title/snippet
					if len(it.Title) > len(ent.item.Title) {
						ent.item.Title = it.Title
					}
					if len(it.Snippet) > len(ent.item.Snippet) {
						ent.item.Snippet = it.Snippet
					}
					// track engines
					ent.engines[r.engine] = struct{}{}
					// positions are 1-based
					ent.positions = append(ent.positions, idx+1)
				} else {
					byURL[it.URL] = &entry{
						item:      it,
						engines:   map[string]struct{}{r.engine: {}},
						positions: []int{idx + 1},
					}
				}
			}
			// Mark cutoff reached but do not cancel contexts; drain remaining responses
			if len(byURL) >= cutoff {
				cutoffReached = true
			}
		case <-ctx.Done():
			// break early and compute from what we have
			// parent timeout reached; remaining goroutines will exit via context
			// We still need to drain any ready items quickly to avoid goroutine leaks
			// Drain non-blocking
			drainLoop:
			for drained := 1; drained < len(engines)-i; drained++ {
				select {
				case <-ch:
				default:
					break drainLoop
				}
			}
			i = len(engines) // exit loop
		}
	}

	// compute scores using SearXNG defaults
	// calculate_score:
	//   weight = product(engine.weight) * len(positions)
	//   score  = sum over positions (weight/pos) [priority neutral]
	weights := defaultEngineWeights()
	results := make([]*entry, 0, len(byURL))
	for _, ent := range byURL {
		weight := 1.0
		for engName := range ent.engines {
			if w, ok := weights[engName]; ok {
				weight *= w
			} else {
				weight *= 1.0
			}
		}
		if n := len(ent.positions); n > 0 {
			weight *= float64(n)
		}
		score := 0.0
		for _, pos := range ent.positions {
			if pos <= 0 {
				continue
			}
			score += weight / float64(pos)
		}
		ent.score = score
		results = append(results, ent)
	}

	// stable sort by score desc; if tie, keep deterministic order by URL
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].score == results[j].score {
			return results[i].item.URL < results[j].item.URL
		}
		return results[i].score > results[j].score
	})

	out := make([]en.Result, 0, len(results))
	for _, ent := range results {
		out = append(out, ent.item)
	}
	return out, timings
}

// defaultEngineWeights returns SearXNG default weights for our engines of interest.
// If an engine has no explicit weight in settings.yml, it defaults to 1.0.
func defaultEngineWeights() map[string]float64 {
	return map[string]float64{
		// Core web engines
		"bing":       1.0,
		"duckduckgo": 1.3,
		"mojeek":     1.2,
		// Knowledge / APIs
		"wikipedia":  0.6,
		"openlibrary": 0.5,
		// Communities
		"hackernews":   0.9,
		"reddit":       0.9,
		"stackoverflow": 0.9,
	}
}
