package spr

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// A date on a Springer page is one of at least seven things, and which one you
// get depends on which tag you read it from and what kind of work it is.
// Measured across the captures:
//
//	2021-03-08T00:00:00Z   prism.publicationDate on an article
//	2021/03/08             citation_online_date, slashes, on the same page
//	2021/03                citation_publication_date, slashes and no day
//	25 November 2024       the printed date in the article header region
//	March 2021             a book chapter with no day
//	2022                   a book, and most series
//	2020-01-01             a sitemap bucket, which is not a publication date
//	Mon, 02 Jan 2006       what an rss pubDate is supposed to be, and is not
//
// Storing all of those as a time.Time would make a year and a first of January
// the same value, and they are not: one is a book published in 2022 and the
// other is a claim about the second of January that nobody made. So a Date
// keeps three things. What the page said, how much of it was real, and the
// parse.
//
// The precision is the field that earns this type. A caller sorting by date can
// use Value and be right. A caller printing a date to a person should look at
// Precision first, or it will tell them a book came out on the first of January
// when the book says 2022.
type Date struct {
	// Raw is the string exactly as the page printed it. It survives because a
	// wrong parse has to be recoverable, and because the string is evidence.
	Raw string `json:"raw"`

	// Precision is how much of Value was actually stated: year, month or day.
	Precision Precision `json:"precision"`

	// Value is the parse, always a real calendar date. The parts that were not
	// stated are filled with the first of the period, so a year becomes its
	// first of January. That is a convention for sorting and not a claim.
	Value time.Time `json:"-"`
}

// Precision says how much of a date the page actually stated.
type Precision string

const (
	PrecisionYear  Precision = "year"
	PrecisionMonth Precision = "month"
	PrecisionDay   Precision = "day"
)

// String prints the date at the precision it was stated at, which is the only
// honest way to print it: 2022 for a book, 2021-03 for a chapter, 2021-03-08
// for an article.
func (d Date) String() string {
	switch d.Precision {
	case PrecisionYear:
		return d.Value.Format("2006")
	case PrecisionMonth:
		return d.Value.Format("2006-01")
	default:
		return d.Value.Format("2006-01-02")
	}
}

// Zero reports whether nothing was parsed, so that an absent date can be
// omitted from a record rather than emitted as a first of January in year one.
func (d Date) Zero() bool { return d.Value.IsZero() }

// MarshalJSON writes the three fields the spec calls for. Value is rendered at
// the stated precision rather than as a timestamp, because a timestamp on a
// year precision date is the exact false claim this type exists to avoid.
func (d Date) MarshalJSON() ([]byte, error) {
	if d.Zero() {
		return []byte("null"), nil
	}
	return json.Marshal(struct {
		Raw       string    `json:"raw"`
		Precision Precision `json:"precision"`
		Value     string    `json:"value"`
	}{d.Raw, d.Precision, d.String()})
}

// UnmarshalJSON reads back what MarshalJSON wrote, so a cached record round
// trips.
func (d *Date) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*d = Date{}
		return nil
	}
	var v struct {
		Raw       string    `json:"raw"`
		Precision Precision `json:"precision"`
		Value     string    `json:"value"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	parsed, err := ParseDate(v.Value)
	if err != nil {
		return fmt.Errorf("date value %q: %w", v.Value, err)
	}
	parsed.Raw = v.Raw
	if v.Precision != "" {
		parsed.Precision = v.Precision
	}
	*d = parsed
	return nil
}

// dateFormats are tried in order. Every one of them was measured on a page this
// tool reads, and the order is longest match first so that a full timestamp is
// not read as a bare year by a format that happens to accept a prefix.
var dateFormats = []struct {
	layout    string
	precision Precision
}{
	{time.RFC3339, PrecisionDay},          // 2021-03-08T00:00:00Z, prism.publicationDate
	{"2006-01-02T15:04:05", PrecisionDay}, // the same without a zone, seen in JSON-LD
	{"2006-01-02", PrecisionDay},          // an rss pubDate
	{"2006/01/02", PrecisionDay},          // citation_online_date and citation_cover_date
	{"2006/01", PrecisionMonth},           // citation_publication_date on a monthly issue
	{"2 January 2006", PrecisionDay},      // the printed date in an article header
	{"02 January 2006", PrecisionDay},     // the same, zero padded
	{"January 2, 2006", PrecisionDay},     // the American form, seen in JSON-LD
	{"2 Jan 2006", PrecisionDay},          // abbreviated, seen in reference lists
	{time.RFC1123, PrecisionDay},          // what an rss pubDate is supposed to be
	{time.RFC1123Z, PrecisionDay},         //
	{"January 2006", PrecisionMonth},      // a chapter with no day
	{"Jan 2006", PrecisionMonth},          //
	{"2006-01", PrecisionMonth},           //
	{"2006", PrecisionYear},               // a book, and most series
}

// ParseDate reads any of the forms this site prints and keeps the string it
// read. An empty input is not an error: a page that carried no date is a normal
// page, and the caller decides whether that is worth naming as missing.
func ParseDate(s string) (Date, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return Date{}, nil
	}
	for _, f := range dateFormats {
		t, err := time.Parse(f.layout, raw)
		if err != nil {
			continue
		}
		return Date{
			Raw:       s,
			Precision: f.precision,
			Value:     time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC),
		}, nil
	}
	return Date{}, fmt.Errorf("%q is not a date in any form this site is known to print", s)
}

// ParseRSSDate reads the pubDate out of the search feed.
//
// It exists as its own function because the field is misnamed rather than
// merely inconsistent. RSS 2.0 specifies RFC 822 for pubDate, so a reader is
// entitled to expect "Mon, 02 Jan 2006 15:04:05 GMT". Springer sends a bare
// "2024-11-25". Every off the shelf feed parser therefore reads this field as
// unparseable and drops it, and a date silently dropped by a library is the
// kind of bug that gets found six months later in a chart.
//
// Both forms are accepted, because being right about the spec would mean losing
// the data.
func ParseRSSDate(s string) (Date, error) {
	return ParseDate(s)
}
