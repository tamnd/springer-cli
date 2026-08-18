package spr

import (
	"errors"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// The volumes and issues page.
//
// 472 KB of html, 8 meta names, no JSON-LD at all, and the complete volume and
// issue index of the journal in one response. For Machine Learning that is 38
// volumes and every issue in each of them, which is the entire back catalogue
// for the price of a single request, and it is the reason this page has a
// record of its own rather than being a flag on the journal command.
//
// It also ships the same analytics payload the journal home page does, so it
// can say which journal it belongs to without a second fetch.

// ErrNotVolumes is returned for a page that is not a volumes and issues page.
var ErrNotVolumes = errors.New("this page is not a volumes and issues page")

// VolumesPath reports whether a url addresses a volumes and issues page.
func VolumesPath(raw string) bool {
	_, sub := journalParts(raw)
	return sub == "volumes-and-issues"
}

// VolumesURL is the volumes and issues page for a journal id.
func VolumesURL(id SpringerID) string { return Base + "/journal/" + string(id) + "/volumes-and-issues" }

// ExtractVolumes reads the volume and issue index out of a fetched page.
func ExtractVolumes(resp *Response) (*Volumes, error) {
	if resp == nil {
		return nil, errors.New("no response to extract from")
	}
	if !VolumesPath(resp.URL) && !VolumesPath(resp.Final) {
		return nil, ErrNotVolumes
	}
	p, err := newPage(resp)
	if err != nil {
		return nil, err
	}

	v := &Volumes{URL: resp.URL}

	if id := p.layer("journal", "Journal Id"); id != "" {
		v.Journal = &Ref{Kind: "journal", ID: id, Name: p.dl.str("Journal Title"), URL: Base + "/journal/" + id}
	}

	root := p.reg.first("volumes-and-issues")
	if root == nil {
		p.env.miss("volumes", "the page carried no [data-test=volumes-and-issues] list, which is the only place the index is printed")
		v.Envelope = p.finish()
		return v, nil
	}

	// One <li class="app-vol-and-issues-item"> per volume, each holding a
	// heading with the volume number and its printed span, and a nested list of
	// issues. The volume year is read off the span rather than off the issues,
	// because volume 115 runs January to August 2026 and a volume that spans a
	// year boundary would otherwise take the year of whichever issue happened
	// to be first.
	for _, item := range findClass(root, "app-vol-and-issues-item") {
		vol := Volume{}
		// The heading holds a span with the number and then two time elements
		// with the same span in two lengths, one shown on a wide viewport and
		// one on a narrow. The first of each is taken, which is the number and
		// the unabbreviated span.
		if h := firstTag(item, atom.H2); h != nil {
			if s := firstTag(h, atom.Span); s != nil {
				vol.Number = strings.TrimSpace(strings.TrimPrefix(text(s), "Volume"))
			}
			if t := firstTag(h, atom.Time); t != nil {
				vol.Label = text(t)
			}
		}
		if vol.Number == "" {
			continue
		}
		vol.Year = lastYear(vol.Label)
		vol.Issue = issuesOf(item)
		v.Volumes = append(v.Volumes, vol)
	}
	if len(v.Volumes) > 0 {
		p.env.via("volumes", LevelRegion, "[data-test=volumes-and-issues] .app-vol-and-issues-item")
	}

	v.Envelope = p.finish()
	return v, nil
}

// issuesOf reads the issues under one volume.
func issuesOf(item *html.Node) []Issue {
	var out []Issue
	for _, li := range findClass(item, "c-list-group__item") {
		iss := Issue{}
		if a := firstTag(li, atom.A); a != nil {
			iss.Label = text(a)
			iss.URL = trimQuery(attr(a, "href"))
		}
		if iss.Label == "" {
			continue
		}
		iss.Number = strings.TrimSpace(strings.TrimPrefix(iss.Label, "Issue"))

		// The issue date is in the time element's datetime attribute as
		// 2026-08, which is a month and is recorded as one. The printed text
		// beside it says August 2026 and is the same fact in prose.
		if t := firstTag(li, atom.Time); t != nil {
			if d, err := ParseDate(attr(t, "datetime")); err == nil {
				iss.Date = &d
			}
		}

		// The themed collection line, where the issue has one. It is the only
		// place the page says what an issue is about, and 86 of the 348
		// measured issues carry one.
		for _, para := range findTag(li, atom.P) {
			if s := text(para); s != "" {
				iss.SpecialTitle = s
				break
			}
		}
		if iss.URL != "" {
			iss.Articles = &Conn{URL: iss.URL}
		}
		out = append(out, iss)
	}
	return out
}

// lastYear reads the last four digit year out of a printed span, so that
// "January - August 2026" and "Jan - Dec 2025" both give the year the volume
// closes in.
func lastYear(s string) int {
	year := 0
	for i := 0; i+4 <= len(s); i++ {
		chunk := s[i : i+4]
		n := 0
		ok := true
		for j := 0; j < 4; j++ {
			if chunk[j] < '0' || chunk[j] > '9' {
				ok = false
				break
			}
			n = n*10 + int(chunk[j]-'0')
		}
		if ok && n >= 1800 && n <= 2200 {
			year = n
		}
	}
	return year
}

// Count returns how many issues this index holds across every volume, which is
// what a Conn on the journal record reports.
func (v *Volumes) Count() int {
	n := 0
	for _, vol := range v.Volumes {
		n += len(vol.Issue)
	}
	return n
}
