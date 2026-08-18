package spr

import (
	"encoding/json"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// The analytics payload, which is the one place a container page states facts
// about itself that no meta tag and no schema.org block carries.
//
// Every page on this site ships its analytics state twice, in two different
// forms, and the difference between them is the whole reason this file exists.
// Measured on all nine captures:
//
//   - The assignment form, window.dataLayer = [{...}]; is strict JSON and
//     parses on every page that ships one. It is the first script in the head
//     and it is named data-test="datalayer" on a journal and a volumes page and
//     unnamed on an article, a book and a series page.
//   - The push form, window.dataLayer.push({...}), is javascript. Single quoted
//     keys, Math.random() calls, string concatenation. It does not parse
//     anywhere and it never will. It is the one named data-test=
//     "springer-link-article-datalayer", "springer-link-datalayer" or
//     "springer-datalayer".
//
// So the split is by form and not by page. This file parses the assignment form
// and reads real fields out of it, and it carries the push form verbatim to
// Envelope.Extra without reading a byte of it. The tempting middle road is a
// lenient parser that recovers most of the push form most of the time, and a
// parser that half works on a blob the publisher can change without notice is a
// permanent source of quiet wrongness.
//
// What the assignment form is worth, per page type, all measured:
//
//	journal   Journal Id, Journal Title, publisherBrand, imprint,
//	          Publishing Model, continuousArticlePublishing, content.category.snt
//	volumes   the same seven, so a volumes page identifies its own journal
//	book      content.book.pisbn, content.book.eisbn, content.book.seriesId,
//	          content.book.seriesTitle, content.book.bookProductType,
//	          content.serial.pissn, content.category.snt, Open Access
//	series    Book Series Id and Book Series Title, and nothing else
//	article   the same shape as a book, and every field it states is already
//	          declared by a meta tag that outranks it, so nothing reads it
//
// Publishing model is the field that justifies the file on its own. It is
// Hybrid Access on Machine Learning, it decides how every article in the
// journal should be read, and there is no meta tag, no schema.org key and no
// visible region on the page that states it in machine readable form.

// dataLayer is one page's analytics state: the parsed assignment entries and
// the unparsed push payloads, kept apart.
type dataLayer struct {
	entries []map[string]json.RawMessage

	// pushes is keyed by the script's data-test name, so that a record can say
	// which block it is carrying rather than emitting an anonymous blob. Only
	// named scripts are collected, because the unnamed ones on these pages are
	// the cookie banner and the tag manager bootstrap and neither is about the
	// content.
	pushes map[string]string

	// broken counts assignment blocks that did not parse. It is a count rather
	// than an error because a page with a broken payload still has a head full
	// of good metadata, and the count reaches the envelope so the day the form
	// changes is a day somebody hears about.
	broken int
}

const (
	assignPrefix = "window.dataLayer = ["
	pushMarker   = "dataLayer.push("
)

// parseDataLayer reads every analytics block in the document.
func parseDataLayer(root *html.Node) *dataLayer {
	d := &dataLayer{pushes: map[string]string{}}
	walk(root, func(n *html.Node) bool {
		if n.DataAtom != atom.Script {
			return true
		}
		src := scriptSource(n)
		switch {
		case strings.HasPrefix(src, assignPrefix):
			body := strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(src, "window.dataLayer = ")), ";")
			var entries []map[string]json.RawMessage
			if err := json.Unmarshal([]byte(body), &entries); err != nil {
				d.broken++
				return false
			}
			d.entries = append(d.entries, entries...)
		case strings.Contains(src, pushMarker):
			if name := attr(n, "data-test"); name != "" {
				d.pushes[name] = src
			}
		}
		return false
	})
	return d
}

// scriptSource is the text of one script element, untouched.
func scriptSource(n *html.Node) string {
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
		}
	}
	return strings.TrimSpace(b.String())
}

// ok reports whether anything parsed, which is what an extractor checks before
// it decides whether an absent field is absent or unreadable.
func (d *dataLayer) ok() bool { return d != nil && len(d.entries) > 0 }

// raw returns the value at a path, descending through nested objects, from the
// first entry that has it.
func (d *dataLayer) raw(path ...string) json.RawMessage {
	if d == nil {
		return nil
	}
	for _, e := range d.entries {
		if v := lookup(e, path); v != nil {
			return v
		}
	}
	return nil
}

func lookup(m map[string]json.RawMessage, path []string) json.RawMessage {
	cur := m
	for i, key := range path {
		v, ok := cur[key]
		if !ok {
			return nil
		}
		if i == len(path)-1 {
			return v
		}
		var next map[string]json.RawMessage
		if err := json.Unmarshal(v, &next); err != nil {
			return nil
		}
		cur = next
	}
	return nil
}

// str reads a string at a path.
//
// A number is accepted and returned in its printed form, because the same fact
// arrives as both: Journal Id is the string "10994" on a journal page and Book
// Series Id is the number 558 on a series page. Both are identifiers and
// neither is arithmetic, so both come back as the digits they were written as.
func (d *dataLayer) str(path ...string) string {
	v := d.raw(path...)
	if v == nil {
		return ""
	}
	var s string
	if err := json.Unmarshal(v, &s); err == nil {
		return s
	}
	var n json.Number
	if err := json.Unmarshal(v, &n); err == nil {
		return n.String()
	}
	return ""
}

// boolean reads a bool at a path, as a pointer, so that a page that said
// nothing stays distinguishable from a page that said false.
func (d *dataLayer) boolean(path ...string) *bool {
	v := d.raw(path...)
	if v == nil {
		return nil
	}
	var b bool
	if err := json.Unmarshal(v, &b); err != nil {
		return nil
	}
	return &b
}

// list reads a list of strings at a path.
func (d *dataLayer) list(path ...string) []string {
	v := d.raw(path...)
	if v == nil {
		return nil
	}
	var out []string
	if err := json.Unmarshal(v, &out); err != nil {
		return nil
	}
	return out
}
