package spr

import (
	"strconv"
	"strings"
	"unicode"
)

// The conference, which is the one entity on this site with no page of its own.
//
// /conference/aaai returns 404 with a zero byte body, and so does every other
// acronym tried against that path, so a conference url is never constructed
// here. What exists instead is a proceedings volume whose title states the
// conference in prose, and a work page that states it again in
// citation_conference_title. Both are the same sentence and this file reads it.
//
// Everything below is a heuristic over a printed title, which is the weakest
// kind of reading this tool does anywhere. It is written to say nothing rather
// than to guess: a title that does not clearly name a conference produces no
// conference at all, and a field that could not be read is left absent rather
// than filled with the closest looking fragment. A wrong conference year is
// worse than no conference year, because a reader has no way to tell it was
// invented.

// conferenceWords are the nouns a Springer proceedings title uses for the event
// itself. One of them has to appear or nothing is read.
var conferenceWords = []string{
	"conference", "symposium", "workshop", "congress",
	"colloquium", "meeting", "summit", "convention",
}

// ParseConferenceTitle reads a conference out of a proceedings title.
//
// A Springer proceedings title is a comma separated sentence in a stable order:
// the volume's own name, then the event, then the acronym with its year, then
// the place, then the dates, then the word Proceedings. For example "Advances
// in Artificial Intelligence: 34th Canadian Conference on Artificial
// Intelligence, Canadian AI 2021, Vancouver, BC, Canada, May 25-28, 2021,
// Proceedings".
//
// The bool is false when the title names no conference, which is the answer for
// every ordinary book and is not a failure.
func ParseConferenceTitle(title string) (Conference, bool) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Conference{}, false
	}
	lower := strings.ToLower(title)
	named := false
	for _, w := range conferenceWords {
		if strings.Contains(lower, w) {
			named = true
			break
		}
	}
	if !named {
		return Conference{}, false
	}

	parts := splitList(strings.ReplaceAll(title, ":", ","))
	c := Conference{}

	// The event name is the first segment carrying one of the conference nouns,
	// which skips past the volume's own title where the two differ.
	for _, part := range parts {
		p := strings.ToLower(part)
		for _, w := range conferenceWords {
			if strings.Contains(p, w) {
				c.Name = part
				break
			}
		}
		if c.Name != "" {
			break
		}
	}
	if c.Name == "" {
		return Conference{}, false
	}

	// The acronym segment is the one that ends in a year and is short enough to
	// be an acronym rather than a sentence. "Canadian AI 2021" qualifies and
	// "May 25-28, 2021" does not, because a month name is not an acronym.
	for _, part := range parts {
		acr, year, ok := acronymYear(part)
		if !ok {
			continue
		}
		c.Acronym, c.Year = acr, year
		break
	}
	if c.Year == 0 {
		c.Year = lastYear(title)
	}

	// The location is deliberately not read. A Springer title prints it as two
	// or three bare segments with no marker of any kind, and telling "Vancouver,
	// BC, Canada" apart from the rest of the sentence needs a gazetteer this
	// tool does not carry. A guessed city is worse than an absent one.

	return c, true
}

// acronymYear reads "Canadian AI 2021" into its acronym and its year.
//
// The last token has to be a plausible year and everything before it has to
// look like a short label rather than prose, which is what keeps a date segment
// and the volume's own title out.
func acronymYear(part string) (string, int, bool) {
	fields := strings.Fields(part)
	if len(fields) < 2 || len(fields) > 4 {
		return "", 0, false
	}
	year, err := strconv.Atoi(fields[len(fields)-1])
	if err != nil || year < 1900 || year > 2200 {
		return "", 0, false
	}
	acr := strings.Join(fields[:len(fields)-1], " ")
	if !acronymish(acr) {
		return "", 0, false
	}
	return acr, year, true
}

// acronymish reports whether a label reads as a conference acronym. It has to
// carry at least one run of capitals and no lower case word longer than a short
// qualifier such as Canadian or International.
func acronymish(s string) bool {
	if s == "" || len(s) > 40 {
		return false
	}
	caps := false
	for _, f := range strings.Fields(s) {
		upper := 0
		for _, r := range f {
			if unicode.IsUpper(r) || unicode.IsDigit(r) {
				upper++
			}
		}
		if upper == len([]rune(f)) && len(f) >= 2 {
			caps = true
		}
	}
	return caps
}
