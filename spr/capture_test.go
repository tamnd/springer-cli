package spr

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The captures are fourteen real pages and four feeds, fetched on 2026-08-18,
// stored gzipped.
//
// Gzipped because the fourteen of them are 4.5 MB of html and 933 KB compressed,
// and a repository carries its testdata forever. They are otherwise untouched
// except for one substitution: the per request uuid in
// ?error=cookies_not_supported&code=<uuid> is replaced with a run of zeroes, so
// that refetching a capture produces a diff only where the page changed. The
// uuid is not part of the page and keeping it would make every refresh look
// like a rewrite.

type capture struct {
	// file is the name under testdata/captures, without the .gz.
	file string

	// url is the address the page was fetched from, which is the requested url
	// and not the effective one.
	url string

	// record is which extractor reads this page: work, journal, book, series,
	// volumes, metrics, figure or table. It is a separate column from kind
	// because a work page has both, and every other page has a record and no
	// work type at all.
	record string

	// kind is the work type the record should carry, on a work page only.
	kind string
}

var captures = []capture{
	{"article_oa.html", "https://link.springer.com/article/10.1007/s10994-021-05946-3", "work", "article"},
	{"article_subscription.html", "https://link.springer.com/article/10.1007/s10994-024-06594-z", "work", "article"},
	{"chapter.html", "https://link.springer.com/chapter/10.1007/978-3-031-28170-9_6", "work", "chapter"},
	{"protocol.html", "https://link.springer.com/protocol/10.1007/978-1-0716-2067-0_1", "work", "protocol"},
	{"referenceworkentry.html", "https://link.springer.com/referenceworkentry/10.1007/978-3-642-27737-5_100-2", "work", "entry"},
	{"book.html", "https://link.springer.com/book/10.1007/978-3-031-28170-9", "book", ""},
	{"journal.html", "https://link.springer.com/journal/10994", "journal", ""},
	{"series.html", "https://link.springer.com/series/558", "series", ""},
	{"volumes.html", "https://link.springer.com/journal/10994/volumes-and-issues", "volumes", ""},
	{"metrics.html", "https://link.springer.com/article/10.1007/s10994-021-05946-3/metrics", "metrics", ""},
	{"metrics_subscription.html", "https://link.springer.com/article/10.1007/s10994-024-06594-z/metrics", "metrics", ""},
	{"figure.html", "https://link.springer.com/article/10.1007/s10994-021-05946-3/figures/1", "figure", ""},
	{"table.html", "https://link.springer.com/article/10.1007/s10994-021-05946-3/tables/1", "table", ""},
	{"search.html", searchQueryURL, "search", ""},
}

// The query every search capture was taken with, html and rss alike, in the
// same minute. It is one string so that the two paths cannot drift apart in the
// testdata the way they did on the live site.
const searchQuery = "query=aleatoric+uncertainty&content-type=Article&date=custom&dateFrom=2020&dateTo=2024&sortBy=relevance"

const (
	searchQueryURL = "https://link.springer.com/search?" + searchQuery
	searchFeedURL  = "https://link.springer.com/search.rss?" + searchQuery
)

// The feeds are kept out of the capture table above because that table drives
// the ledger, and the ledger reads meta names, json-ld blocks, data-test
// regions and the analytics payload. A feed has none of those, and running an
// html reader over it to produce four zeroes would be a row that looks like
// evidence and is not.
//
// The three short ones are here for one finding each. The last page of the
// result set carries 17 items rather than 20, which is what proves the
// arithmetic. Page 29 is empty and carries the four characters null. Page 200
// is empty and carries nothing at all. Both empties are 200 ok and they are
// four bytes apart in size, which is the whole argument for terminating on the
// item count.
var feeds = []capture{
	{"search.rss", searchFeedURL, "feed", ""},
	{"search_last.rss", searchFeedURL + "&page=28", "feed", ""},
	{"search_null.rss", searchFeedURL + "&page=29", "feed", ""},
	{"search_empty.rss", searchFeedURL + "&page=200", "feed", ""},
}

// The sitemaps, kept out of the capture table for the same reason the feeds
// are: the ledger reads html regions and there are none here.
//
// Ten of the twelve maps, chosen so that every finding in sitemap.go has bytes
// behind it. The index is the whole 1.27 MB of it, because the four name
// shapes, the lastmod coverage and the 66 shards at one nominal day are claims
// about the whole file and a trimmed copy would prove none of them. The three
// journal maps are all here because the overlap between them is the finding.
// sitemap-collections.xml and sitemap_book_series.xml are the two that are left
// out: they are 3.7 MB between them, and they are one flat urlset each with
// nothing in them that the other urlsets do not already exercise.
var maps = []capture{
	{"sitemap_index.xml", Base + IndexPath, "sitemap", ""},
	{"sitemap_day.xml", Base + "/sitemap-entries/sitemap_2020-01-01_1.xml", "sitemap", ""},
	{"sitemap_year.xml", Base + "/sitemap-entries/sitemap_1850_1.xml", "sitemap", ""},
	{"sitemap_empty.xml", Base + "/sitemap-entries/sitemap_2020-01-01_99.xml", "sitemap", ""},
	{"sitemap_shops.txt", Base + "/sitemap-entries/sitemap_shops.txt", "sitemap", ""},
	{"sitemap_brands.xml", Base + "/sitemap-entries/sitemap_brands.xml", "sitemap", ""},
	{"sitemap_subjects.xml", Base + SubjectsIndexPath, "sitemap", ""},
	{"sitemap_journals_springer.xml", Base + "/sitemap-springer-journals.xml", "sitemap", ""},
	{"sitemap_journals_bmc.xml", Base + "/sitemap-bmc-springeropen-journals.xml", "sitemap", ""},
	{"sitemap_journals_palgrave.xml", Base + "/sitemap-palgrave-journals.xml", "sitemap", ""},
}

// fetchedAt is the moment the captures were taken. The extractor stamps the
// envelope from the response, so the tests hand it a fixed instant rather than
// the clock, which keeps the ledger byte for byte reproducible.
var fetchedAt = time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)

// load reads one capture into a response of the shape the client would have
// produced for it.
func load(t *testing.T, c capture) *Response {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "captures", c.file+".gz"))
	if err != nil {
		t.Fatalf("capture %s: %v", c.file, err)
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("capture %s: %v", c.file, err)
	}
	defer func() { _ = gz.Close() }()

	body, err := io.ReadAll(gz)
	if err != nil {
		t.Fatalf("capture %s: %v", c.file, err)
	}

	// The feeds and the sitemaps are served as xml, the shops map as text/plain,
	// and everything else as html. The classifier takes the kind rather than
	// guessing it, so the loader states it here the way the client would have.
	kind := KindHTML
	switch {
	case strings.HasSuffix(c.file, ".rss"),
		strings.HasSuffix(c.file, ".xml"),
		strings.HasSuffix(c.file, ".txt"):
		kind = KindXML
	}

	return &Response{
		URL:     c.url,
		Final:   c.url,
		Code:    200,
		Body:    body,
		Status:  Classify(200, nil, body, kind),
		Fetched: fetchedAt,
		// Three, the cookie dance, which is what every one of these cost.
		Redirects: 3,
	}
}

// capturedMap returns one named sitemap capture.
func capturedMap(t *testing.T, file string) *Response {
	t.Helper()
	for _, c := range maps {
		if c.file == file {
			return load(t, c)
		}
	}
	t.Fatalf("no sitemap capture named %s", file)
	return nil
}

// capturedFeed returns one named feed capture.
func capturedFeed(t *testing.T, file string) *Response {
	t.Helper()
	for _, c := range feeds {
		if c.file == file {
			return load(t, c)
		}
	}
	t.Fatalf("no feed capture named %s", file)
	return nil
}
