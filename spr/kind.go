package spr

import (
	"bytes"
	"strings"
)

// Kind is the sort of document a request expects back. It exists because the
// url is not evidence: /content/pdf/<doi>.pdf answers 200 with 282,045 bytes of
// text/html when the content is behind a subscription, and a client that trusts
// the .pdf in the path writes an HTML error page to disk with a .pdf name.
type Kind int

const (
	// KindAny accepts whatever comes back. Used by the cache layer and by
	// commands that are inspecting rather than parsing.
	KindAny Kind = iota

	// KindHTML is a page.
	KindHTML

	// KindPDF is the full text pdf.
	KindPDF

	// KindXML is a sitemap or an rss feed. Springer serves both as
	// application/xml, and serves one sitemap as text/plain.
	KindXML
)

// pdfMagic is the file signature every pdf starts with. The content type is
// checked first, but a server that mislabels a real pdf is a smaller problem
// than a server that labels an error page as one, so the bytes get a say.
var pdfMagic = []byte("%PDF-")

// Matches reports whether a response of this content type and these bytes is
// the kind that was asked for.
func (k Kind) Matches(contentType string, body []byte) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	switch k {
	case KindAny:
		return true
	case KindHTML:
		return ct == "" || strings.Contains(ct, "html")
	case KindPDF:
		return ct == "application/pdf" || bytes.HasPrefix(body, pdfMagic)
	case KindXML:
		// sitemap_shops.txt is a list of urls served as text/plain among a
		// directory of xml siblings, so text/plain counts here. The parser
		// sniffs the first bytes rather than trusting either one.
		return strings.Contains(ct, "xml") || strings.Contains(ct, "text/plain") || ct == ""
	}
	return false
}

// String names the kind for error messages.
func (k Kind) String() string {
	switch k {
	case KindHTML:
		return "html"
	case KindPDF:
		return "pdf"
	case KindXML:
		return "xml"
	default:
		return "any"
	}
}
