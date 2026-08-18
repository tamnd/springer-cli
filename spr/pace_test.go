package spr

import (
	"context"
	"testing"
	"time"
)

// recorder replaces the sleep in a pacer so the test asserts the interval that
// was asked for instead of waiting it out.
func recorder(p *Pacer) *[]time.Duration {
	var waits []time.Duration
	now := time.Unix(0, 0)
	p.now = func() time.Time { return now }
	p.sleep = func(d time.Duration) {
		waits = append(waits, d)
		now = now.Add(d)
	}
	return &waits
}

// TestPaceFloorHolds walks every path that can set a pace and asserts none of
// them lands under the floor. The floor is the one setting in this tool that is
// deliberately not configurable, and it is exactly the sort of invariant a
// later refactor removes without noticing.
func TestPaceFloorHolds(t *testing.T) {
	tries := []time.Duration{
		-1 * time.Hour,
		0,
		1 * time.Nanosecond,
		999 * time.Millisecond,
		PaceFloor,
	}
	for _, d := range tries {
		if got := NewPacer(d).Interval(); got < PaceFloor {
			t.Errorf("NewPacer(%s).Interval() = %s, below the %s floor", d, got, PaceFloor)
		}
		c := New(WithPace(d))
		if got := c.Pacer.Interval(); got < PaceFloor {
			t.Errorf("New(WithPace(%s)) paces at %s, below the %s floor", d, got, PaceFloor)
		}
	}
}

func TestSearchHasItsOwnBucket(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"https://link.springer.com/search?query=x", searchBucket},
		{"https://link.springer.com/search.rss?query=x&page=3", searchBucket},
		{"https://link.springer.com/search/csv?query=x", searchBucket},
		{"https://link.springer.com/article/10.1007/s10994-021-05946-3", "link.springer.com"},
		{"https://link.springer.com/sitemap-index.xml", "link.springer.com"},
		{"https://api.crossref.org/works/10.1007/x", "api.crossref.org"},
		{"https://api.openalex.org/works", "api.openalex.org"},
	}
	for _, tc := range cases {
		if got := Bucket(tc.url); got != tc.want {
			t.Errorf("Bucket(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

// A search wait must not hold up the work fetches that follow it, and a work
// fetch must not spend the budget that keeps search answering.
func TestSearchWaitsDoNotSerializeTheRest(t *testing.T) {
	p := NewPacer(DefaultPace)
	waits := recorder(p)
	ctx := context.Background()

	urls := []string{
		"https://link.springer.com/search.rss?query=x&page=1",
		"https://link.springer.com/search.rss?query=x&page=2",
		"https://link.springer.com/article/10.1007/a",
		"https://link.springer.com/article/10.1007/b",
		"https://api.crossref.org/works/10.1007/a",
	}
	for _, u := range urls {
		if err := p.Wait(ctx, u); err != nil {
			t.Fatalf("Wait(%q): %v", u, err)
		}
	}

	// First request per bucket is free, so three of the five wait for nothing.
	// The second search request waits the search interval and the second work
	// request waits the general one.
	want := []time.Duration{SearchPace, DefaultPace}
	if len(*waits) != len(want) {
		t.Fatalf("waited %v, want %v", *waits, want)
	}
	for i, d := range *waits {
		if d != want[i] {
			t.Errorf("wait %d was %s, want %s", i, d, want[i])
		}
	}
}

// Raising --pace above the search interval has to raise search too, or the
// setting means less than it says.
func TestRaisingPaceRaisesSearch(t *testing.T) {
	p := NewPacer(10 * time.Second)
	if got := p.intervalFor(searchBucket); got != 10*time.Second {
		t.Errorf("search interval at --pace 10s is %s, want 10s", got)
	}
	if got := NewPacer(DefaultPace).intervalFor(searchBucket); got != SearchPace {
		t.Errorf("search interval at the default pace is %s, want %s", got, SearchPace)
	}
}
