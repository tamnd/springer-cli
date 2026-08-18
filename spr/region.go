package spr

import (
	"sort"
	"strings"

	"golang.org/x/net/html"
)

// Rung 3. Springer names its own regions.
//
// 53 distinct data-test values on the open access article, 52 on the
// subscription one, 56 on the journal and book pages, 29 on volumes and issues.
// Every container page in this site has a head carrying 8 or 9 meta names and
// no bibliographic vocabulary at all, so for those pages this rung is the top
// one and everything comes from here.
//
// The reason this file tracks what it read is drift. A region name Springer
// ships that this tool never asks for is a thing being left on the table, and
// the honest way to say so is to count them and name them rather than to let
// the record look complete. In 3007 this exact mechanism is what caught Amazon
// adding a coupon component to a search card.

// regions is the data-test and data-title index for one document.
type regions struct {
	byTest map[string][]*html.Node
	order  []string
	titles []*html.Node
	read   map[string]bool
	root   *html.Node
}

// parseRegions indexes every named region in the document.
func parseRegions(root *html.Node) *regions {
	r := &regions{byTest: map[string][]*html.Node{}, read: map[string]bool{}, root: root}
	walk(root, func(n *html.Node) bool {
		if v := attr(n, "data-test"); v != "" {
			if _, seen := r.byTest[v]; !seen {
				r.order = append(r.order, v)
			}
			r.byTest[v] = append(r.byTest[v], n)
		}
		if hasAttr(n, "data-title") {
			r.titles = append(r.titles, n)
		}
		return true
	})
	return r
}

// all returns every node with this data-test value and marks the name read.
func (r *regions) all(test string) []*html.Node {
	r.read[test] = true
	return r.byTest[test]
}

// first returns the first node with this data-test value, or nil, and marks
// the name read. Asking for a region that is not there still counts as having
// looked, which is the difference between a field this tool ignores and a field
// it went for and did not find.
func (r *regions) first(test string) *html.Node {
	r.read[test] = true
	if ns := r.byTest[test]; len(ns) > 0 {
		return ns[0]
	}
	return nil
}

// text returns the collapsed text of the first node with this data-test value.
func (r *regions) text(test string) string {
	return text(r.first(test))
}

// names returns every data-test value the page carried, in document order.
func (r *regions) names() []string { return append([]string(nil), r.order...) }

// unread returns every region name the extractor never asked for, sorted.
func (r *regions) unread() []string {
	var out []string
	for _, n := range r.order {
		if !r.read[n] {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// railTitle is Springer's recommender strip. It appears in the data-title
// sequence between Abstract and Introduction, and it is not a section of the
// paper. It goes to rails. A section tree that carries an advertisement is
// wrong in a way that propagates into every summary made downstream from it.
const railTitle = "Inline Recommendations"

// sectionNodes returns the article's own sections in document order.
//
// Sections are <section data-title="...">. Figures also carry data-title, on a
// <div data-test="figure">, and there are 17 of them interleaved with the 15
// real sections on the open access capture, so the tag name is what separates
// them. The recommender strip is dropped here and picked up by railNodes.
func (r *regions) sectionNodes() []*html.Node {
	var out []*html.Node
	for _, n := range r.titles {
		if n.Data != "section" {
			continue
		}
		if attr(n, "data-title") == railTitle {
			continue
		}
		out = append(out, n)
	}
	return out
}

// railNodes returns the recommender strip, if the page carried one.
func (r *regions) railNodes() []*html.Node {
	var out []*html.Node
	for _, n := range r.titles {
		if attr(n, "data-title") == railTitle {
			out = append(out, n)
		}
	}
	return out
}

// isSection reports whether a node is one of the article's own sections, which
// is what the depth walk counts ancestors with.
func isSection(n *html.Node) bool {
	return n.Data == "section" && hasAttr(n, "data-title")
}

// sectionNumber reads the printed number off a section heading, 1, 2 or 2.1.
// It is the only ordering signal the page states, and it is absent on the back
// matter sections, which are numbered by nobody and appear after the body.
func sectionNumber(n *html.Node) string {
	for _, s := range findClass(n, "c-article-section__title-number") {
		if t := strings.TrimSpace(text(s)); t != "" {
			return t
		}
	}
	return ""
}

// accessStatement is the sentence a paywalled page prints where the body would
// be. It is read from a class rather than a region because Springer does not
// name it with data-test, and it is worth having because it is the page saying
// in its own words why the body is not there.
func accessStatement(root *html.Node) string {
	for _, n := range findClass(root, "c-notes__text") {
		t := text(n)
		if strings.Contains(t, "preview of subscription content") {
			// The sentence runs into a sign in link and a stop. Everything up
			// to the first comma is the statement.
			if i := strings.Index(t, ","); i > 0 {
				return t[:i]
			}
			return t
		}
	}
	return ""
}
