package spr

import (
	"context"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Pace is not a compliance measure, it is a reliability one. The search
// challenge is the proof: the surface answered, then stopped, and the only
// thing that changed was request volume. Everything in this tool that runs for
// more than a minute depends on staying under whatever threshold the edge is
// enforcing today.
const (
	// DefaultPace is the interval between two requests to the same host.
	DefaultPace = 2 * time.Second

	// PaceFloor is the shortest interval this tool will ever use. No flag, no
	// environment variable and no config key can lower it. TestPaceFloorHolds
	// walks every path that can set a pace and asserts it, because this is
	// exactly the sort of invariant a later refactor removes by accident.
	PaceFloor = 1 * time.Second

	// SearchPace is the interval for the search surface, which gets its own
	// bucket because it is the one that trips. A slow search should not slow
	// down the work fetches that follow it, and a fast one should not spend the
	// budget that keeps search answering.
	SearchPace = 5 * time.Second
)

// searchBucket is the bucket key for /search and /search.rss on the Springer
// host. It is deliberately not the host, so search waits do not serialize the
// rest of the run.
const searchBucket = "link.springer.com/search"

// Pacer spaces requests out, one bucket per host, with the search surface
// separated out. The zero value is not usable; call NewPacer.
type Pacer struct {
	mu       sync.Mutex
	interval time.Duration
	next     map[string]time.Time

	// sleep is nil in production, where a wait is a select on a timer and the
	// context so that Ctrl-C during a five second search wait is immediate. In
	// tests it records the interval that was asked for rather than waiting it
	// out.
	sleep func(time.Duration)
	now   func() time.Time
}

// NewPacer returns a pacer at the given interval, clamped up to PaceFloor. A
// caller that asks for less gets the floor, and Interval reports what it got.
func NewPacer(interval time.Duration) *Pacer {
	if interval < PaceFloor {
		interval = PaceFloor
	}
	return &Pacer{
		interval: interval,
		next:     make(map[string]time.Time),
		now:      time.Now,
	}
}

// Interval reports the general interval in force, after the floor is applied.
func (p *Pacer) Interval() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.interval
}

// Bucket names the pace bucket a url belongs to.
func Bucket(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	if strings.EqualFold(u.Host, springerHost) && isSearchPath(u.Path) {
		return searchBucket
	}
	return strings.ToLower(u.Host)
}

func isSearchPath(path string) bool {
	return path == "/search" || path == "/search.rss" || strings.HasPrefix(path, "/search/")
}

// intervalFor returns the interval a bucket waits. The search bucket waits
// longer than everything else and never less than the general interval, so
// raising --pace raises search too.
func (p *Pacer) intervalFor(bucket string) time.Duration {
	if bucket == searchBucket && SearchPace > p.interval {
		return SearchPace
	}
	return p.interval
}

// Wait blocks until the bucket for this url is due, or until ctx is done.
func (p *Pacer) Wait(ctx context.Context, raw string) error {
	bucket := Bucket(raw)

	p.mu.Lock()
	now := p.now()
	wait := time.Duration(0)
	if due, ok := p.next[bucket]; ok && due.After(now) {
		wait = due.Sub(now)
	}
	p.next[bucket] = now.Add(wait + p.intervalFor(bucket))
	p.mu.Unlock()

	if wait <= 0 {
		return nil
	}
	if p.sleep != nil {
		p.sleep(wait)
		return ctx.Err()
	}
	select {
	case <-time.After(wait):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
