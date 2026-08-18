package spr

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Rung 4. Class names.
//
// This is the last rung because it is presentational. A c- prefix on a Springer
// class means it belongs to their design system, and a design system is exactly
// the thing that changes without anybody thinking of it as a data change. So
// only four fields sit here, and each is here because nothing above it answers:
//
//	c-article-references__links      the resolver links, 68 of 122
//	c-article-equation               the equation count, 66
//	c-article-footnote--listed__item the listed notes, 24
//	the body text, only under --full-text
//
// Counting is done on exact class tokens, never on substrings. The article
// carries 181 occurrences of the string c-article-equation and 66 elements
// whose class list contains that exact token, and the difference is the
// modifier classes on the children.

// Link is a resolver link on a reference. kind is taken from the page's own
// data-track-action, which is Springer telling its analytics what the link is,
// and is therefore a better source for the answer than the url would be.
type Link struct {
	Kind string `json:"kind"`
	URL  string `json:"url"`
}

// linkKinds maps the page's data-track-action values to the names this tool
// prints. Every one was counted on the open access article: 68 google scholar,
// 47 math, 44 article, 29 mathscinet, 5 book.
//
// math is in this list even though it was not in the original survey of link
// kinds, because 47 of them is not a rounding error. It resolves to zbMATH.
var linkKinds = map[string]string{
	"google scholar reference": "google-scholar",
	"mathscinet reference":     "mathscinet",
	"math reference":           "math",
	"article reference":        "article",
	"book reference":           "book",
}

func linkKind(action string) string {
	if k, ok := linkKinds[strings.ToLower(strings.TrimSpace(action))]; ok {
		return k
	}
	return "external"
}

// referenceItem is one entry of the printed reference list, which is a
// different source from the citation_reference meta tags and is read for the
// resolver links the meta tags do not carry.
type referenceItem struct {
	text  string
	links []Link
}

// referenceItems reads the printed reference list.
//
// The two sources line up positionally on the captures, 122 list items against
// 122 meta tags, and the extractor pairs them only when the counts match. A
// mismatch means one of the two moved, and pairing them anyway would attach
// somebody else's resolver links to a reference, which is a wrong answer that
// looks entirely plausible.
func referenceItems(root *html.Node) []referenceItem {
	items := findClass(root, "c-article-references__item")
	out := make([]referenceItem, 0, len(items))
	for _, it := range items {
		r := referenceItem{}
		for _, t := range findClass(it, "c-article-references__text") {
			r.text = text(t)
			break
		}
		for _, blk := range findClass(it, "c-article-references__links") {
			for _, a := range findTag(blk, atom.A) {
				href := attr(a, "href")
				if href == "" {
					continue
				}
				r.links = append(r.links, Link{Kind: linkKind(attr(a, "data-track-action")), URL: href})
			}
		}
		out = append(out, r)
	}
	return out
}

// footnotes returns the listed notes in document order.
func footnotes(root *html.Node) []string {
	var out []string
	for _, n := range findClass(root, "c-article-footnote--listed__item") {
		if t := text(n); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// equationCount counts display equations.
func equationCount(root *html.Node) int {
	return countClass(root, "c-article-equation")
}
