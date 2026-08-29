package aggregate

import (
	"context"
	"errors"
	"testing"
	"time"

	en "searxgo/internal/engine"
)

type fakeEngine struct {
	name  string
	items []en.Result
	err   error
	delay time.Duration
}

func (f *fakeEngine) Name() string { return f.name }
func (f *fakeEngine) Search(ctx context.Context, q string, page, size int) ([]en.Result, error) {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.items, f.err
}

// A fast engine returning a full page must not cause a slower engine's results
// to be dropped – that was the "results don't show from certain engines" bug.
func TestSlowEngineStillMerged(t *testing.T) {
	fast := &fakeEngine{name: "fast", items: mkResults("https://fast.test/", 30)}
	slow := &fakeEngine{name: "slow", delay: 50 * time.Millisecond, items: mkResults("https://slow.test/", 5)}

	out, timings := Run(context.Background(), []en.SearchEngine{fast, slow}, "q", 2*time.Second, 1, 10)

	if len(timings) != 2 {
		t.Fatalf("want a timing entry per engine, got %d", len(timings))
	}
	var sawSlow bool
	for _, r := range out {
		if r.Engine == "slow" || containsHost(r.URL, "slow.test") {
			sawSlow = true
		}
	}
	if !sawSlow {
		t.Fatalf("slow engine results were dropped from merged output")
	}
}

// A failing engine is reported in timings with its error, not silently swallowed.
func TestFailingEngineReported(t *testing.T) {
	ok := &fakeEngine{name: "ok", items: mkResults("https://ok.test/", 3)}
	bad := &fakeEngine{name: "bad", err: errors.New("boom")}

	_, timings := Run(context.Background(), []en.SearchEngine{ok, bad}, "q", time.Second, 1, 10)

	var badTiming *Timing
	for i := range timings {
		if timings[i].Engine == "bad" {
			badTiming = &timings[i]
		}
	}
	if badTiming == nil || badTiming.Error == "" {
		t.Fatalf("failing engine not reported with error: %+v", timings)
	}
}

// The same page from two engines is deduped and ranked above a single-engine hit.
func TestConsensusRanking(t *testing.T) {
	a := &fakeEngine{name: "bing", items: []en.Result{
		{Title: "Shared", URL: "https://example.com/page"},
		{Title: "OnlyA", URL: "https://a-only.com/x"},
	}}
	b := &fakeEngine{name: "duckduckgo", items: []en.Result{
		{Title: "Shared longer title", URL: "http://www.example.com/page/"},
	}}

	out, _ := Run(context.Background(), []en.SearchEngine{a, b}, "q", time.Second, 1, 10)

	if len(out) != 2 {
		t.Fatalf("want 2 deduped results, got %d: %+v", len(out), out)
	}
	if out[0].URL != "https://example.com/page" && normalizeURL(out[0].URL) != normalizeURL("https://example.com/page") {
		t.Fatalf("consensus result should rank first, got %+v", out[0])
	}
	if out[0].Title != "Shared longer title" {
		t.Fatalf("merge should keep the longer title, got %q", out[0].Title)
	}
}

func TestNormalizeURL(t *testing.T) {
	cases := [][2]string{
		{"https://www.example.com/page/", "https://example.com/page"},
		{"http://example.com/page", "https://example.com/page/"},
		{"https://example.com/a#frag", "https://example.com/a"},
	}
	for _, c := range cases {
		if normalizeURL(c[0]) != normalizeURL(c[1]) {
			t.Errorf("normalizeURL(%q)=%q != normalizeURL(%q)=%q", c[0], normalizeURL(c[0]), c[1], normalizeURL(c[1]))
		}
	}
}

func mkResults(base string, n int) []en.Result {
	out := make([]en.Result, n)
	for i := 0; i < n; i++ {
		out[i] = en.Result{Title: "t", URL: base + string(rune('a'+i%26)) + itoa(i), Engine: ""}
	}
	return out
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

func containsHost(u, host string) bool {
	return len(u) > 0 && (indexOf(u, host) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
