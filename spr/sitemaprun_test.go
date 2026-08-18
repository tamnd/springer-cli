package spr

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Walking the maps, against a server that serves the captures at the paths the
// site serves them at.

type mapServer struct {
	t *testing.T

	// broken are paths that answer 500 however many times they are asked.
	broken map[string]bool

	hits map[string]int
	srv  *httptest.Server
}

func newMapServer(t *testing.T, broken ...string) *mapServer {
	t.Helper()
	s := &mapServer{t: t, broken: map[string]bool{}, hits: map[string]int{}}
	for _, p := range broken {
		s.broken[p] = true
	}
	byPath := map[string]Capture{}
	for _, c := range maps {
		byPath[strings.TrimPrefix(c.URL, Base)] = c
	}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits[r.URL.Path]++
		if s.broken[r.URL.Path] {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		c, ok := byPath[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if strings.HasSuffix(c.File, ".txt") {
			w.Header().Set("Content-Type", "text/plain")
		} else {
			w.Header().Set("Content-Type", "text/xml")
		}
		_, _ = w.Write(load(s.t, c).Body)
	}))
	t.Cleanup(s.srv.Close)
	return s
}

// client returns a client pointed at this server, with no retries, because a
// test for a shard that does not answer should not spend three seconds of
// backoff proving it.
func (s *mapServer) client() *Client {
	c := fast(s.t, WithRetries(0))
	c.base = s.srv.URL
	return c
}

// children turns capture paths into the index children a walk is given.
func (s *mapServer) children(paths ...string) []Child {
	s.t.Helper()
	out := make([]Child, 0, len(paths))
	for _, p := range paths {
		name := baseName(p)
		bucket, shard, _ := ParseChildName(name)
		out = append(out, Child{URL: s.srv.URL + p, Name: name, Bucket: bucket, Shard: shard})
	}
	return out
}

// The journal list is three maps and they overlap. Fetching one of them is
// short by hundreds, and adding them up without a dedup is over by 152.
func TestStaticJournalsIsAllThreeImprints(t *testing.T) {
	srv := newMapServer(t)
	var notes []string
	set, err := srv.client().Static(context.Background(), "journals", func(s string) { notes = append(notes, s) })
	if err != nil {
		t.Fatalf("Static: %v", err)
	}
	if len(set.Maps) != 3 {
		t.Fatalf("%d maps fetched, want the three imprints", len(set.Maps))
	}
	if len(set.Entries) != 3405 {
		t.Errorf("%d journals, want the 3,405 distinct ones behind 3,557 entries", len(set.Entries))
	}
	for _, e := range set.Entries {
		if e.Kind != "journal" {
			t.Fatalf("%s is kind %q in the journal list", e.URL, e.Kind)
		}
	}

	// And it says so, because a caller who adds up the three published counts
	// and gets a different number deserves the sentence rather than a puzzle.
	var said bool
	for _, n := range notes {
		if strings.Contains(n, "3557") && strings.Contains(n, "3405") {
			said = true
		}
	}
	if !said {
		t.Errorf("the overlap went unmentioned: %v", notes)
	}
	if len(set.Notes) != len(notes) {
		t.Errorf("%d notes on the record and %d on stderr", len(set.Notes), len(notes))
	}
}

// The subjects map is an index, so what it has to give is ten sitemap
// addresses, and calling them pages would be a lie about what they are.
func TestStaticSubjectsIsAnIndexOfSitemaps(t *testing.T) {
	srv := newMapServer(t)
	set, err := srv.client().Static(context.Background(), "subjects", nil)
	if err != nil {
		t.Fatalf("Static: %v", err)
	}
	if len(set.Entries) != 10 {
		t.Fatalf("%d entries, want 10", len(set.Entries))
	}
	for _, e := range set.Entries {
		if e.Kind != "sitemap" {
			t.Errorf("%s is kind %q, and every child of an index is another sitemap", e.URL, e.Kind)
		}
	}
	var said bool
	for _, n := range set.Notes {
		if strings.Contains(n, "index rather than a list") {
			said = true
		}
	}
	if !said {
		t.Errorf("nothing said that these are sitemaps: %v", set.Notes)
	}
}

func TestStaticRefusesANameItDoesNotHave(t *testing.T) {
	srv := newMapServer(t)
	if _, err := srv.client().Static(context.Background(), "everything", nil); err == nil {
		t.Fatal("a made up static set was accepted")
	}
	if KnownStatic("everything") || !KnownStatic("Journals") {
		t.Errorf("KnownStatic disagrees with Static about which names exist")
	}
}

// A walk reads shards in order, filters on the url's own word, and hands
// entries out as it goes rather than holding 52 million of them.
func TestEnumerateWalksAndFilters(t *testing.T) {
	srv := newMapServer(t)
	children := srv.children(
		"/sitemap-entries/sitemap_2020-01-01_1.xml",
		"/sitemap-entries/sitemap_1850_1.xml",
		"/sitemap-entries/sitemap_2020-01-01_99.xml",
	)

	var got []Entry
	stats, err := srv.client().Enumerate(context.Background(), children, EnumOptions{
		Kinds: []string{"book"},
		Each:  func(e Entry) error { got = append(got, e); return nil },
	})
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	if stats.Read != 3 || stats.Shards != 3 {
		t.Errorf("read %d of %d shards, want 3 of 3", stats.Read, stats.Shards)
	}
	if stats.Entries != 6548 {
		t.Errorf("%d urls read, want 5,000 plus 1,548 plus the empty one", stats.Entries)
	}
	// 136 books in the day shard and 36 in the year shard.
	if stats.Matched != 172 || len(got) != 172 {
		t.Errorf("%d books matched and %d handed out, want 172", stats.Matched, len(got))
	}
	for _, e := range got {
		if e.Kind != "book" {
			t.Fatalf("%s came through a --kind book walk", e.URL)
		}
	}

	// The shard that does not exist answered 200 with an empty urlset, and that
	// is counted rather than passed off as a shard with nothing published in it.
	if stats.Empty != 1 {
		t.Errorf("%d empty shards, want 1", stats.Empty)
	}
}

func TestEnumerateStopsAtTheLimit(t *testing.T) {
	srv := newMapServer(t)
	children := srv.children(
		"/sitemap-entries/sitemap_2020-01-01_1.xml",
		"/sitemap-entries/sitemap_1850_1.xml",
	)
	var n int
	stats, err := srv.client().Enumerate(context.Background(), children, EnumOptions{
		Limit: 10,
		Each:  func(Entry) error { n++; return nil },
	})
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	if n != 10 || stats.Matched != 10 {
		t.Errorf("%d entries handed out and %d matched, want 10", n, stats.Matched)
	}
	// Ten urls is one shard, and the second one is never asked for.
	if stats.Read != 1 {
		t.Errorf("%d shards read for a limit of 10", stats.Read)
	}
}

// An interrupted walk does not start over. The state is written as each shard
// finishes, so what is skipped is exactly what was finished.
func TestEnumerateResumesWhereItStopped(t *testing.T) {
	srv := newMapServer(t)
	dir := t.TempDir()
	children := srv.children(
		"/sitemap-entries/sitemap_2020-01-01_1.xml",
		"/sitemap-entries/sitemap_1850_1.xml",
	)

	// A first walk that is cut short in the middle of the first shard. The
	// shard is not finished, so it is not marked, so the second walk reads it
	// again rather than skipping past urls nobody ever saw.
	stop := errors.New("stopped")
	r, err := OpenResume(dir, ResumeKey("test"))
	if err != nil {
		t.Fatalf("OpenResume: %v", err)
	}
	var seen int
	_, err = srv.client().Enumerate(context.Background(), children, EnumOptions{
		Resume: r,
		Each: func(Entry) error {
			seen++
			if seen == 5 {
				return stop
			}
			return nil
		},
	})
	if !errors.Is(err, stop) {
		t.Fatalf("Enumerate: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if r.Count() != 0 {
		t.Fatalf("%d shards marked done, and the one that was interrupted is not done", r.Count())
	}

	// A second walk with the same key finishes both shards.
	r, err = OpenResume(dir, ResumeKey("test"))
	if err != nil {
		t.Fatalf("OpenResume: %v", err)
	}
	stats, err := srv.client().Enumerate(context.Background(), children, EnumOptions{Resume: r})
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	if stats.Read != 2 || stats.Skipped != 0 {
		t.Errorf("read %d and skipped %d, want 2 and 0", stats.Read, stats.Skipped)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// And a third walk has nothing left to do.
	r, err = OpenResume(dir, ResumeKey("test"))
	if err != nil {
		t.Fatalf("OpenResume: %v", err)
	}
	defer func() { _ = r.Close() }()
	stats, err = srv.client().Enumerate(context.Background(), children, EnumOptions{Resume: r})
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	if stats.Read != 0 || stats.Skipped != 2 {
		t.Errorf("read %d and skipped %d, want 0 and 2", stats.Read, stats.Skipped)
	}

	// A different selection is a different key, and it starts from nothing. A
	// walk of the last three days resuming the state of a walk of everything
	// would skip most of what it was asked for and call it done.
	other, err := OpenResume(dir, ResumeKey("test", "--kind", "book"))
	if err != nil {
		t.Fatalf("OpenResume: %v", err)
	}
	defer func() { _ = other.Close() }()
	if other.Count() != 0 {
		t.Errorf("a different selection inherited %d finished shards", other.Count())
	}
}

// One shard that will not answer is a gap in a long walk and not the end of it.
// The first one is different: nothing has been produced yet, so there is
// nothing to salvage by carrying on.
func TestEnumerateSurvivesAShardThatDoesNotAnswer(t *testing.T) {
	broken := "/sitemap-entries/sitemap_1850_1.xml"
	srv := newMapServer(t, broken)
	dir := t.TempDir()
	r, err := OpenResume(dir, ResumeKey("test"))
	if err != nil {
		t.Fatalf("OpenResume: %v", err)
	}
	defer func() { _ = r.Close() }()

	children := srv.children("/sitemap-entries/sitemap_2020-01-01_1.xml", broken)
	var notes []string
	stats, err := srv.client().Enumerate(context.Background(), children, EnumOptions{
		Resume: r,
		Note:   func(s string) { notes = append(notes, s) },
	})
	if err != nil {
		t.Fatalf("one bad shard ended a two shard walk: %v", err)
	}
	if stats.Read != 1 || stats.Failed != 1 {
		t.Errorf("read %d and failed %d, want 1 and 1", stats.Read, stats.Failed)
	}
	if r.Done(children[1].URL) {
		t.Errorf("a shard that did not answer was marked done, so --resume would never come back for it")
	}
	var said bool
	for _, n := range notes {
		if strings.Contains(n, "did not answer") {
			said = true
		}
	}
	if !said {
		t.Errorf("a shard was lost without a word: %v", notes)
	}

	// And the same failure on the first shard is the walk failing, because
	// there is no partial result to protect.
	srv2 := newMapServer(t, broken)
	if _, err := srv2.client().Enumerate(context.Background(), srv2.children(broken), EnumOptions{}); err == nil {
		t.Error("the only shard in the walk failed and Enumerate returned success")
	}
}

// The bill is computed from the index that was just fetched. These are the
// numbers the guard prints, and they are arithmetic on a measured shard rather
// than a figure typed into a help string.
func TestEnumCostIsArithmeticOnTheIndex(t *testing.T) {
	cost := EnumCost(10408, 2*time.Second)
	if cost.Requests != 10408 {
		t.Errorf("requests = %d", cost.Requests)
	}
	if want := 10407 * 2 * time.Second; cost.Duration != want {
		t.Errorf("duration = %s, want %s, which is the gaps and not the requests", cost.Duration, want)
	}
	if cost.Duration.Round(time.Minute) != 5*time.Hour+47*time.Minute {
		t.Errorf("duration rounds to %s, want 5h47m", cost.Duration.Round(time.Minute))
	}
	if cost.MaxURLs != 52040000 {
		t.Errorf("max urls = %d, want 52,040,000", cost.MaxURLs)
	}
	if cost.MaxBytes != 10408*ShardBytes {
		t.Errorf("max bytes = %d", cost.MaxBytes)
	}
	if (Cost{}) != EnumCost(0, time.Second) {
		t.Errorf("a walk of nothing costs something")
	}
}

// Selecting is done on the index in hand, so the count that is billed and the
// count that is walked are the same list.
func TestSelectNarrowsTheIndex(t *testing.T) {
	s, err := ParseSitemap(capturedMap(t, "sitemap_index.xml"))
	if err != nil {
		t.Fatalf("ParseSitemap: %v", err)
	}
	since := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	sel := s.Select(since, time.Time{})
	if len(sel) == 0 || len(sel) >= len(s.Children) {
		t.Fatalf("%d of %d children selected for one month", len(sel), len(s.Children))
	}
	for _, c := range sel {
		if !c.Bucket.End().After(since) {
			t.Fatalf("%s is entirely before the window", c.Name)
		}
	}
	// The index's own order is kept, which is newest first, so a walk that is
	// interrupted has done the most recent shards rather than a random half.
	for i := 1; i < len(sel); i++ {
		if sel[i].Bucket.Start.After(sel[i-1].Bucket.Start) {
			t.Fatalf("%s comes after %s, and the index is newest first", sel[i].Name, sel[i-1].Name)
		}
	}
}
