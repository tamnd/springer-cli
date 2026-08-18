package spr

import (
	"net/http"
	"os"
	"testing"
)

// The challenge body is the real one, 3,038 bytes, saved from a /search request
// that had tripped the edge. It is here rather than synthesized because the one
// thing this test has to prove is that the marker the classifier looks for is
// the marker the site actually sends.
func challengeBody(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/challenge.html")
	if err != nil {
		t.Fatalf("reading the challenge capture: %v", err)
	}
	if len(b) != 3038 {
		t.Fatalf("challenge capture is %d bytes, expected the measured 3038", len(b))
	}
	return b
}

func TestClassify(t *testing.T) {
	challenge := challengeBody(t)

	html := func(body string) []byte { return []byte(body) }
	header := func(ct string) http.Header {
		h := http.Header{}
		if ct != "" {
			h.Set("Content-Type", ct)
		}
		return h
	}

	cases := []struct {
		name string
		code int
		head http.Header
		body []byte
		want Kind
		is   Status
	}{
		{
			// The one that earns the whole table: a missing article answers 404
			// with 122 KB of body, so nothing can be inferred from size.
			name: "a missing article, 404 with a large body",
			code: 404,
			head: header("text/html"),
			body: html(`<html><head><title>Page Unavailable | Springer Nature Link</title></head><body>` + string(make([]byte, 120_000)) + `</body></html>`),
			want: KindHTML,
			is:   StatusNotFound,
		},
		{
			name: "a conference 404 with no body at all",
			code: 404,
			head: header(""),
			body: nil,
			want: KindHTML,
			is:   StatusNotFound,
		},
		{
			name: "the search challenge, 200 and 3038 bytes",
			code: 200,
			head: header("text/html"),
			body: challenge,
			want: KindHTML,
			is:   StatusChallenged,
		},
		{
			// A challenge stays a challenge even when a pdf was asked for. The
			// order matters: it is not a wrong kind problem to be reported as a
			// content type mismatch, it is the edge refusing to serve.
			name: "the challenge in front of a pdf request",
			code: 200,
			head: header("text/html"),
			body: challenge,
			want: KindPDF,
			is:   StatusChallenged,
		},
		{
			name: "a subscription pdf url answering with html",
			code: 200,
			head: header("text/html; charset=utf-8"),
			body: html(`<html><head><meta name="access" content="No"></head><body>preview</body></html>`),
			want: KindPDF,
			is:   StatusWrongKind,
		},
		{
			name: "a real pdf",
			code: 200,
			head: header("application/pdf"),
			body: append([]byte("%PDF-1.6\n"), make([]byte, 100)...),
			want: KindPDF,
			is:   StatusOK,
		},
		{
			name: "a pdf mislabelled by the server but carrying the signature",
			code: 200,
			head: header("application/octet-stream"),
			body: append([]byte("%PDF-1.6\n"), make([]byte, 100)...),
			want: KindPDF,
			is:   StatusOK,
		},
		{
			name: "a chapter the publisher withholds",
			code: 200,
			head: header("text/html"),
			body: html(`<html><head><meta name="access" content="No"><meta name="citation_doi" content="10.1007/978-3-031-28170-9_5"></head></html>`),
			want: KindHTML,
			is:   StatusRestricted,
		},
		{
			name: "an open access article",
			code: 200,
			head: header("text/html"),
			body: html(`<html><head><meta name="access" content="Yes"><meta name="citation_doi" content="10.1007/s10994-021-05946-3"></head></html>`),
			want: KindHTML,
			is:   StatusOK,
		},
		{
			// Container pages carry no access statement at all, and the absence
			// is normal rather than a parse failure.
			name: "a journal home page, which states no access at all",
			code: 200,
			head: header("text/html"),
			body: html(`<html><head><title>Machine Learning</title></head></html>`),
			want: KindHTML,
			is:   StatusOK,
		},
		{
			name: "an rss feed",
			code: 200,
			head: header("application/xml"),
			body: []byte(`<?xml version="1.0"?><rss version="2.0"><channel></channel></rss>`),
			want: KindXML,
			is:   StatusOK,
		},
		{
			name: "the sitemap that is plain text among xml siblings",
			code: 200,
			head: header("text/plain"),
			body: []byte("https://link.springer.com/shop/one\nhttps://link.springer.com/shop/two\n"),
			want: KindXML,
			is:   StatusOK,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(tc.code, tc.head, tc.body, tc.want); got != tc.is {
				t.Errorf("Classify = %q, want %q", got, tc.is)
			}
		})
	}
}

func TestAccess(t *testing.T) {
	cases := []struct {
		body string
		want string
	}{
		{`<meta name="access" content="Yes">`, "Yes"},
		{`<meta name="access" content="No">`, "No"},
		{`<meta content="No" name="access">`, ""},   // attribute order as measured, nothing else claimed
		{`<meta name="access" content="no">`, "No"}, // case tolerated
		{`<meta name="citation_doi" content="10.1007/x">`, ""},
		{``, ""},
	}
	for _, tc := range cases {
		if got := DeclaredAccess([]byte(tc.body)); got != tc.want {
			t.Errorf("DeclaredAccess(%q) = %q, want %q", tc.body, got, tc.want)
		}
	}
}

// The access statement lives in the head on every capture measured, and the scan
// is bounded so a 700 KB body does not get walked on every fetch. This asserts
// the bound is real, because a bound nobody tests is a bound that gets removed.
func TestAccessScanIsBounded(t *testing.T) {
	body := make([]byte, headBytes+1024)
	for i := range body {
		body[i] = ' '
	}
	copy(body[headBytes+8:], []byte(`<meta name="access" content="No">`))
	if got := DeclaredAccess(body); got != "" {
		t.Errorf("Access read past the %d byte head bound and returned %q", headBytes, got)
	}
}
