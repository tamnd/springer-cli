package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/springer-cli/spr"
)

// Nothing in this file makes a request. Every flag check runs before the client
// is built, and the printers take an index assembled here, which is the only way
// to assert on a bill without spending five and three quarter hours to see it.

// The bad flag combinations. Each one is a run that would otherwise cost
// requests to find out it was wrong.
func TestSitemapRefusesBadFlags(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"a static map that does not exist", []string{"sitemap", "--static", "preprints"}, "is not one of journals"},
		{"one map and the whole index at once", []string{"sitemap", "--static", "journals", "--all"}, "pick one"},
		{"a kind this site has no path for", []string{"sitemap", "--kind", "preprint"}, "is not one of article"},
		{"a since that is not a date", []string{"sitemap", "--since", "last tuesday"}, "not a year, a month or a day"},
		{"an until that is not a date", []string{"sitemap", "--until", "soon"}, "not a year, a month or a day"},
		{"a window that runs backwards", []string{"sitemap", "--since", "2026", "--until", "2020"}, "is after"},
		{"no urls wanted", []string{"sitemap", "--limit", "-1"}, "asks for no urls"},
		{"filtering a list of shards by kind", []string{"sitemap", "--list", "--kind", "book"}, "nothing to filter"},
		{"resuming with the cache turned off", []string{"sitemap", "--all", "--resume", "--no-cache"}, "turned that off"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := run(t, c.args...)
			if err == nil {
				t.Fatalf("accepted, and printed:\n%s", out)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error is %q, want it to mention %q", err, c.want)
			}
			var ee *ExitError
			if !errors.As(err, &ee) || ee.Code != CodeUsage {
				t.Errorf("exit code is not %d, and nothing was fetched", CodeUsage)
			}
		})
	}
}

// The two edges of a window are read differently, and this is the test that says
// why. --until 2026 means the end of 2026. Reading both edges as the first
// instant of what they state would turn a whole year into the first of January
// and drop every shard after it without saying a word.
func TestUntilCoversTheWholeStatedPeriod(t *testing.T) {
	cases := []struct {
		in    string
		since string
		until string
	}{
		{"2026", "2026-01-01", "2026-12-31"},
		{"2026-02", "2026-02-01", "2026-02-28"},
		{"2024-02", "2024-02-01", "2024-02-29"},
		{"2026-08-18", "2026-08-18", "2026-08-18"},
	}
	for _, c := range cases {
		from, err := parseSince(c.in)
		if err != nil {
			t.Fatalf("--since %s: %v", c.in, err)
		}
		if got := from.Format("2006-01-02"); got != c.since {
			t.Errorf("--since %s starts at %s, want %s", c.in, got, c.since)
		}
		to, err := parseUntil(c.in)
		if err != nil {
			t.Fatalf("--until %s: %v", c.in, err)
		}
		if got := to.Format("2006-01-02"); got != c.until {
			t.Errorf("--until %s ends at %s, want %s", c.in, got, c.until)
		}
	}

	// An edge that was not given is an open one, and an empty string is not an
	// error, because half a window is a window.
	for _, s := range []string{"", "   "} {
		from, err := parseSince(s)
		if err != nil || !from.IsZero() {
			t.Errorf("--since %q is %v, %v, want an open edge", s, from, err)
		}
		to, err := parseUntil(s)
		if err != nil || !to.IsZero() {
			t.Errorf("--until %q is %v, %v, want an open edge", s, to, err)
		}
	}

	// And the whole point of the pair: a shard bucketed in June is inside
	// --since 2026 --until 2026.
	bucket, _, ok := spr.ParseChildName("sitemap_2026-06_3.xml")
	if !ok {
		t.Fatal("the name of a month shard did not parse")
	}
	from, _ := parseSince("2026")
	to, _ := parseUntil("2026")
	if !bucket.InWindow(from, to) {
		t.Errorf("%s is outside --since 2026 --until 2026", bucket)
	}
}

// The bill, which is the reason this command has a threshold at all.
//
// Every number in it comes from the index that was just fetched. That is what
// keeps it right in a year, when the index has grown by three hundred shards and
// a compiled in figure would still be quoting today's.
func TestTheBillIsComputedFromTheIndexInHand(t *testing.T) {
	var out bytes.Buffer
	pace := 2 * time.Second
	printCost(&out, spr.EnumCost(10408, pace), pace, 10408, true)

	got := out.String()
	for _, want := range []string{
		"10,408 child sitemaps at 2s pace is 5 hours 47 minutes",
		"5,000 urls",
		"52,040,000 urls",
		"that is the whole index",
		"narrow it with --since and --until, or pass --yes to proceed",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the bill does not say %q:\n%s", want, got)
		}
	}

	// A window that is worth billing and not worth refusing says the same
	// things without claiming to be the whole index.
	out.Reset()
	printCost(&out, spr.EnumCost(40, pace), pace, 10408, false)
	got = out.String()
	if !strings.Contains(got, "40 child sitemaps at 2s pace is 1 minute") {
		t.Errorf("a small walk is billed wrong:\n%s", got)
	}
	if strings.Contains(got, "whole index") {
		t.Errorf("a window of 40 shards called itself the whole index:\n%s", got)
	}
}

// The summary of the index is one request, and it has to spend that request
// saying the thing somebody is most likely to get wrong.
func TestPrintIndexSaysWhatABucketIs(t *testing.T) {
	var out bytes.Buffer
	printIndex(&out, indexFixture(), 2*time.Second)

	got := out.String()
	for _, want := range []string{
		"children      4 child sitemaps",
		"buckets       2 named for a day, 1 for a month, 1 for a year",
		"span          1850 to 2026-08-18",
		"lastmod       on the 2 day shards only, where it restates the bucket",
		"bucket        where a record is filed, and not when it was published",
		"static        journals, series, collections, brands, shops, subjects, read with --static",
		"full walk     4 requests at 2s, 6 seconds, bounded above by 20,000 urls",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the index summary does not say %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "envelope") {
		t.Errorf("the summary printed no envelope:\n%s", got)
	}
}

// --list is the shards themselves, one url per line, because that is what a
// caller feeds back into --resume or into their own fetcher.
func TestListPrintsShardURLs(t *testing.T) {
	var out bytes.Buffer
	idx := indexFixture()
	if err := printList(&out, idx, idx.Children, 0); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Count(out.String(), "\n"), 4; got != want {
		t.Errorf("printed %d lines, want %d, one per shard and nothing else", got, want)
	}
	if !strings.HasPrefix(out.String(), "https://link.springer.com/sitemap-entries/sitemap_2026-08-18_1.xml\n") {
		t.Errorf("the list is not the index's own order, newest first:\n%s", out.String())
	}

	// --limit takes the first n, and a limit that selects nothing is the same
	// answer as a window that selects nothing.
	out.Reset()
	if err := printList(&out, idx, idx.Children, 2); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(out.String(), "\n"); got != 2 {
		t.Errorf("--limit 2 printed %d lines", got)
	}
	var ee *ExitError
	if err := printList(&out, idx, nil, 0); !errors.As(err, &ee) || ee.Code != CodeNoData {
		t.Errorf("an empty selection exits %v, want %d", err, CodeNoData)
	}
}

// The line a walk ends on. It goes to stderr so the urls on stdout stay a pipe,
// and it counts the four things that are worth counting: what was read, what was
// skipped, what came back and what it cost.
func TestSummaryCountsWhatTheWalkDid(t *testing.T) {
	got := summary(&spr.EnumStats{
		Shards: 12, Read: 10, Skipped: 2, Empty: 1,
		Entries: 43120, Matched: 172, Bytes: 5724450,
	}, 24*time.Second)
	for _, want := range []string{
		"10 of 12 shards read",
		"2 skipped as already done",
		"43,120 urls",
		"172 kept",
		"1 empty",
		"24 seconds",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the summary does not say %q:\n%s", want, got)
		}
	}

	// A walk with no filter kept everything it read, and saying so twice would
	// read as though something had been dropped.
	plain := summary(&spr.EnumStats{Shards: 1, Read: 1, Entries: 5000, Matched: 5000}, time.Second)
	if strings.Contains(plain, "kept") || strings.Contains(plain, "skipped") {
		t.Errorf("an unfiltered walk reported a filter:\n%s", plain)
	}
}

// The resume state is keyed on the selection. Resuming a walk of the last three
// days against the state of a walk of everything since 1850 would skip most of
// what was asked for and report a clean run, so two different selections have to
// be two different keys.
func TestTheResumeKeyFollowsTheSelection(t *testing.T) {
	idx := indexFixture()
	all := selectionOf(idx.Children)
	narrow := selectionOf(idx.Children[:2])
	if all == narrow {
		t.Fatalf("two selections named the same thing: %q", all)
	}
	if !strings.Contains(all, "/4") || !strings.Contains(narrow, "/2") {
		t.Errorf("a selection name does not say how many shards it covers: %q and %q", all, narrow)
	}
	if selectionOf(nil) != "empty" {
		t.Errorf("an empty selection is %q", selectionOf(nil))
	}

	base := spr.ResumeKey("https://link.springer.com/sitemap-index.xml", all, "")
	for _, other := range []string{
		spr.ResumeKey("https://link.springer.com/sitemap-index.xml", narrow, ""),
		spr.ResumeKey("https://link.springer.com/sitemap-index.xml", all, "book"),
	} {
		if other == base {
			t.Error("a different selection produced the same resume key")
		}
	}
	if spr.ResumeKey("https://link.springer.com/sitemap-index.xml", all, "") != base {
		t.Error("the same selection produced two different resume keys")
	}
}

// --kind is a filter on the url's own first path segment, so it costs nothing
// and it happens before anything is printed. The aliases are there because
// nobody types rwe.
func TestFilterKinds(t *testing.T) {
	entries := []spr.Entry{
		{URL: "https://link.springer.com/article/10.1007/a", Kind: "article"},
		{URL: "https://link.springer.com/book/978-3", Kind: "book"},
		{URL: "https://link.springer.com/rwe/10.1007/b", Kind: "entry"},
	}
	got := filterKinds(append([]spr.Entry(nil), entries...), []string{"books", "rwe"})
	if len(got) != 2 || got[0].Kind != "book" || got[1].Kind != "entry" {
		t.Errorf("filterKinds kept %v", got)
	}
	if none := filterKinds(append([]spr.Entry(nil), entries...), []string{"journal"}); len(none) != 0 {
		t.Errorf("filterKinds kept %v for a kind that is not there", none)
	}
}

// A pace below the floor is applied as the floor, so the bill has to be quoted
// at the pace the run will actually keep rather than the one that was asked for.
func TestTheBillIsQuotedAtThePaceTheRunWillKeep(t *testing.T) {
	defer func(d time.Duration) { g.pace = d }(g.pace)

	g.pace = 100 * time.Millisecond
	if got := effectivePace(); got != spr.PaceFloor {
		t.Errorf("effectivePace() = %s under the floor, want %s", got, spr.PaceFloor)
	}
	g.pace = 5 * time.Second
	if got := effectivePace(); got != 5*time.Second {
		t.Errorf("effectivePace() = %s, want the pace that was asked for", got)
	}
}

// indexFixture is four children in the three name shapes, in the order the real
// index lists them, which is newest first.
func indexFixture() *spr.Sitemap {
	idx := &spr.Sitemap{
		URL:      "https://link.springer.com/sitemap-index.xml",
		Form:     spr.FormIndex,
		Envelope: spr.Envelope{Tier: "sitemap", Status: spr.StatusOK, Bytes: 1267777},
	}
	for _, c := range []struct{ name, lastmod string }{
		{"sitemap_2026-08-18_1.xml", "2026-08-18"},
		{"sitemap_2020-01-01_1.xml", "2020-01-01"},
		{"sitemap_1900-01_1.xml", ""},
		{"sitemap_1850_1.xml", ""},
	} {
		bucket, shard, ok := spr.ParseChildName(c.name)
		if !ok {
			panic("the fixture holds a name that does not parse: " + c.name)
		}
		idx.Children = append(idx.Children, spr.Child{
			URL:     "https://link.springer.com/sitemap-entries/" + c.name,
			Name:    c.name,
			Bucket:  bucket,
			Shard:   shard,
			LastMod: c.lastmod,
		})
	}
	return idx
}
