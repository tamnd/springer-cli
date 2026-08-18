// Package spr reads link.springer.com and the open bibliographic indexes, and
// turns what they publish into records that say where every field came from.
package spr

import (
	"bytes"
	"net/http"
	"regexp"
	"strings"
)

// Status is what a response actually is, which on this site is a different
// question from what its HTTP status code says.
//
// Three separate surfaces answer 200 with something other than the thing that
// was asked for: the search challenge, a .pdf url on subscription content that
// returns HTML, and a paywalled work page that carries full metadata and no
// body. A missing article, meanwhile, answers an honest 404 with 122 KB of
// body. Nothing here can be decided from the status code and the size alone.
type Status string

const (
	// StatusOK is the page that was asked for, served in full.
	StatusOK Status = "ok"

	// StatusRestricted is the page that was asked for, with the body withheld
	// by the publisher. It is not an error. The metadata on a restricted page
	// is complete and worth having, and only the body is missing.
	StatusRestricted Status = "restricted"

	// StatusChallenged is the Fastly client challenge: 200, 3,038 bytes, and a
	// request for a JavaScript runtime. It is triggered by request volume
	// against /search and it is never retried, because retrying is what caused
	// it.
	StatusChallenged Status = "challenged"

	// StatusWrongKind is a response whose content type is not the kind that was
	// asked for, such as a /content/pdf/ url returning text/html.
	StatusWrongKind Status = "wrong_kind"

	// StatusNotFound is a 4xx or 5xx. Here the status code is honest and is
	// taken at face value.
	StatusNotFound Status = "not_found"
)

// challengeMarker is the title and heading of the Fastly interstitial. The
// weekly drift job checks that this string still appears in a live challenge,
// because the day Springer rewords it this classifier starts calling a 3 KB
// error page a served page.
var challengeMarker = []byte("Client Challenge")

// accessMeta matches the publisher's own machine readable access statement.
// The attribute order is fixed on every capture measured, but the match is
// deliberately tolerant of extra attributes and of single quotes.
var accessMeta = regexp.MustCompile(`(?i)<meta[^>]+name=["']access["'][^>]+content=["'](Yes|No)["']`)

// headBytes bounds the access scan. Every capture measured states access in the
// head, inside the first 100 KB, and scanning a 700 KB body for it on every
// fetch buys nothing.
const headBytes = 128 << 10

// DeclaredAccess reports the publisher's access statement, "Yes", "No", or ""
// when the page does not carry one. Container pages carry no statement at all,
// which is why the empty string is a normal answer and not a parse failure.
func DeclaredAccess(body []byte) string {
	head := body
	if len(head) > headBytes {
		head = head[:headBytes]
	}
	m := accessMeta.FindSubmatch(head)
	if m == nil {
		return ""
	}
	if strings.EqualFold(string(m[1]), "yes") {
		return "Yes"
	}
	return "No"
}

// Challenged reports whether a body is the client challenge rather than a page.
func Challenged(body []byte) bool { return bytes.Contains(body, challengeMarker) }

// Classify decides what a response is, on content, before anything is parsed.
// The order of the tests is the design: the status code first because it is
// honest about not found, then the challenge because it wears a 200, then the
// content type because a .pdf that is text/html is not a pdf, then the
// publisher's own access statement.
func Classify(code int, header http.Header, body []byte, want Kind) Status {
	if code >= 400 {
		return StatusNotFound
	}
	// The challenge and the access statement are findings about html served by
	// link.springer.com. A json record from an open index that happens to carry
	// the words Client Challenge is a paper about bot detection, and running
	// either test over it would classify a real record as an interstitial.
	if want == KindJSON {
		if !want.Matches(header.Get("Content-Type"), body) {
			return StatusWrongKind
		}
		return StatusOK
	}
	if Challenged(body) {
		return StatusChallenged
	}
	if !want.Matches(header.Get("Content-Type"), body) {
		return StatusWrongKind
	}
	if DeclaredAccess(body) == "No" {
		return StatusRestricted
	}
	return StatusOK
}
