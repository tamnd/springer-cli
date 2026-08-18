package spr

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// The extraction layer works on a parsed tree rather than on regular
// expressions. A Springer article is 700 KB of nested markup with attributes in
// no fixed order, and a pattern that reads data-test="figure" out of it is one
// attribute reorder away from silently matching nothing. The parser costs about
// ten milliseconds on the largest capture and removes a whole class of bug.
//
// These helpers are deliberately small. There is no selector engine here,
// because the four rungs need exactly three things: find nodes by attribute,
// find nodes by class token, and read the text under a node.

// parseDoc parses a response body into a document tree.
func parseDoc(b []byte) (*html.Node, error) {
	return html.Parse(bytes.NewReader(b))
}

// attr returns the value of an attribute, or the empty string.
func attr(n *html.Node, key string) string {
	if n == nil {
		return ""
	}
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// hasAttr reports whether the attribute is present, which is a different
// question from whether it has a value. citation_fulltext_world_readable is
// present and empty on both article captures, and the presence is the fact.
func hasAttr(n *html.Node, key string) bool {
	if n == nil {
		return false
	}
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

// hasClass reports whether the class attribute carries this exact token.
// Substring matching would count c-article-equation__content as an equation and
// report 181 equations on a page that has 66.
func hasClass(n *html.Node, class string) bool {
	if n == nil {
		return false
	}
	for _, f := range strings.Fields(attr(n, "class")) {
		if f == class {
			return true
		}
	}
	return false
}

// walk visits every element node in document order. Returning false from fn
// skips that node's children, which is how a section walk avoids descending
// into a nested section it will visit on its own.
func walk(n *html.Node, fn func(*html.Node) bool) {
	if n == nil {
		return
	}
	if n.Type == html.ElementNode && !fn(n) {
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, fn)
	}
}

// findClass returns every element carrying the class token, in document order.
func findClass(root *html.Node, class string) []*html.Node {
	var out []*html.Node
	walk(root, func(n *html.Node) bool {
		if hasClass(n, class) {
			out = append(out, n)
		}
		return true
	})
	return out
}

// firstClass returns the first element carrying the class token, or nil.
func firstClass(root *html.Node, class string) *html.Node {
	var found *html.Node
	walk(root, func(n *html.Node) bool {
		if found != nil {
			return false
		}
		if hasClass(n, class) {
			found = n
			return false
		}
		return true
	})
	return found
}

// countClass counts elements carrying the class token.
func countClass(root *html.Node, class string) int {
	return len(findClass(root, class))
}

// findTag returns every element with this tag name, in document order.
func findTag(root *html.Node, a atom.Atom) []*html.Node {
	var out []*html.Node
	walk(root, func(n *html.Node) bool {
		if n.DataAtom == a {
			out = append(out, n)
		}
		return true
	})
	return out
}

// firstTag returns the first element with this tag name, or nil. The container
// extractors want the first of something far more often than all of it, and
// writing that as a loop that breaks on its first pass reads as though the rest
// mattered.
func firstTag(root *html.Node, a atom.Atom) *html.Node {
	var found *html.Node
	walk(root, func(n *html.Node) bool {
		if found != nil {
			return false
		}
		if n.DataAtom == a {
			found = n
			return false
		}
		return true
	})
	return found
}

// blocks are the elements that end a line when a browser lays them out.
//
// A space is written at their boundaries and nowhere else. Without it a heading
// and the paragraph under it run together as "AbstractThe notion of", and with
// it everywhere H<sub>2</sub>O comes out as "H 2 O". Block level is the line
// this splits on, and it is the same line the browser draws.
var blocks = map[atom.Atom]bool{
	atom.P: true, atom.Div: true, atom.Section: true, atom.Article: true,
	atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true, atom.H5: true, atom.H6: true,
	atom.Li: true, atom.Ul: true, atom.Ol: true, atom.Dl: true, atom.Dt: true, atom.Dd: true,
	atom.Table: true, atom.Tr: true, atom.Td: true, atom.Th: true, atom.Caption: true,
	atom.Figure: true, atom.Figcaption: true, atom.Blockquote: true, atom.Pre: true,
	atom.Br: true, atom.Header: true, atom.Footer: true, atom.Nav: true, atom.Aside: true,
}

// text returns the visible text under a node with whitespace collapsed to
// single spaces. Springer indents its markup, so the raw concatenation carries
// runs of newlines and tabs that no consumer wants and every diff notices.
func text(n *html.Node) string {
	var b strings.Builder
	var rec func(*html.Node)
	rec = func(n *html.Node) {
		if n == nil {
			return
		}
		switch {
		case n.Type == html.TextNode:
			b.WriteString(n.Data)
		case n.Type == html.ElementNode && (n.DataAtom == atom.Script || n.DataAtom == atom.Style):
			return
		case n.Type == html.ElementNode && blocks[n.DataAtom]:
			b.WriteString(" ")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			rec(c)
		}
		if n.Type == html.ElementNode && blocks[n.DataAtom] {
			b.WriteString(" ")
		}
	}
	rec(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

// scriptText returns the source of every script under a node, joined by
// newlines and otherwise untouched.
//
// text skips script elements on purpose, because a page's javascript is not
// its prose and folding it into a section body would be nonsense. The analytics
// blob is the one thing that is deliberately wanted as source, so it gets its
// own reader rather than a flag on text that every other caller has to think
// about. Nothing here collapses whitespace, since the point is to carry the
// payload exactly as it was served.
func scriptText(n *html.Node) string {
	var parts []string
	walk(n, func(e *html.Node) bool {
		if e.DataAtom != atom.Script {
			return true
		}
		var b strings.Builder
		for c := e.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.TextNode {
				b.WriteString(c.Data)
			}
		}
		if s := strings.TrimSpace(b.String()); s != "" {
			parts = append(parts, s)
		}
		return false
	})
	return strings.Join(parts, "\n")
}

// ownText returns the text of a node without descending into child elements
// that carry their own data-title, so that a section's own prose does not
// swallow a nested section's.
func ownText(n *html.Node) string {
	var b strings.Builder
	var rec func(*html.Node)
	rec = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.TextNode {
				b.WriteString(c.Data)
				continue
			}
			if c.Type != html.ElementNode {
				continue
			}
			if c.DataAtom == atom.Script || c.DataAtom == atom.Style {
				continue
			}
			if hasAttr(c, "data-title") {
				continue
			}
			if blocks[c.DataAtom] {
				b.WriteString(" ")
			}
			rec(c)
			if blocks[c.DataAtom] {
				b.WriteString(" ")
			}
		}
	}
	rec(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

// depth counts how many ancestors of this node are also sections, which is how
// the section tree gets its levels without any id attribute to hang them on.
func depth(n *html.Node, isParent func(*html.Node) bool) int {
	d := 0
	for p := n.Parent; p != nil; p = p.Parent {
		if isParent(p) {
			d++
		}
	}
	return d
}
