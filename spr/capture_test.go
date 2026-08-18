package spr

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The captures are thirteen real pages, fetched on 2026-08-18, stored gzipped.
//
// Gzipped because the thirteen of them are 4.2 MB of html and 883 KB compressed,
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

	return &Response{
		URL:     c.url,
		Final:   c.url,
		Code:    200,
		Body:    body,
		Status:  Classify(200, nil, body, KindHTML),
		Fetched: fetchedAt,
		// Three, the cookie dance, which is what every one of these cost.
		Redirects: 3,
	}
}
