package spr

import (
	"encoding/json"
	"testing"
)

func TestParseDate(t *testing.T) {
	cases := []struct {
		in        string
		value     string // what String() should print, at the stated precision
		precision Precision
		err       bool
	}{
		// Every form below was measured on a page this tool reads.
		{"2021-03-08T00:00:00Z", "2021-03-08", PrecisionDay, false}, // prism.publicationDate
		{"2021-03-08", "2021-03-08", PrecisionDay, false},           // citation_online_date
		{"25 November 2024", "2024-11-25", PrecisionDay, false},     // the article header
		{"05 November 2024", "2024-11-05", PrecisionDay, false},
		{"March 2021", "2021-03", PrecisionMonth, false}, // a chapter with no day
		{"2021-03", "2021-03", PrecisionMonth, false},
		{"2022", "2022", PrecisionYear, false}, // a book, and most series
		{"2020-01-01", "2020-01-01", PrecisionDay, false},
		{"Mon, 02 Jan 2006 15:04:05 GMT", "2006-01-02", PrecisionDay, false},

		{"  2022  ", "2022", PrecisionYear, false},

		{"forthcoming", "", "", true},
		{"2021-13-45", "", "", true},
		{"in press", "", "", true},
	}
	for _, tc := range cases {
		got, err := ParseDate(tc.in)
		if tc.err {
			if err == nil {
				t.Errorf("ParseDate(%q) = %v, want an error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseDate(%q): %v", tc.in, err)
			continue
		}
		if got.String() != tc.value {
			t.Errorf("ParseDate(%q).String() = %q, want %q", tc.in, got.String(), tc.value)
		}
		if got.Precision != tc.precision {
			t.Errorf("ParseDate(%q).Precision = %q, want %q", tc.in, got.Precision, tc.precision)
		}
		if got.Raw != tc.in {
			t.Errorf("ParseDate(%q).Raw = %q, want the input unchanged", tc.in, got.Raw)
		}
	}
}

// A year and a first of January are not the same date, and a type that cannot
// tell them apart will eventually tell someone a book came out on New Year's
// Day. This is the reason Date is a struct and not a time.Time.
func TestPrecisionSurvives(t *testing.T) {
	year, err := ParseDate("2022")
	if err != nil {
		t.Fatal(err)
	}
	day, err := ParseDate("2022-01-01")
	if err != nil {
		t.Fatal(err)
	}
	if !year.Value.Equal(day.Value) {
		t.Fatal("the two parse to different instants, so this test is not testing what it thinks")
	}
	if year.Precision == day.Precision {
		t.Error("a bare year and a full date came out at the same precision")
	}
	if year.String() == day.String() {
		t.Errorf("both print as %q, so the precision is not reaching the output", year.String())
	}
}

// An empty date is not an error. A page that carried no date is a normal page,
// and it is the caller who decides whether that is worth naming as missing.
func TestEmptyDateIsNotAnError(t *testing.T) {
	got, err := ParseDate("")
	if err != nil {
		t.Fatalf("ParseDate(\"\"): %v", err)
	}
	if !got.Zero() {
		t.Error("an empty string parsed to something")
	}
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "null" {
		t.Errorf("an absent date marshalled as %s, want null", b)
	}
}

// RSS 2.0 says pubDate is RFC 822. Springer sends a bare YYYY-MM-DD, which every
// off the shelf feed parser reads as unparseable and drops. Both forms have to
// work, because being right about the spec would mean losing the data.
func TestRSSPubDateIsNotRFC822(t *testing.T) {
	measured, err := ParseRSSDate("2024-11-25")
	if err != nil {
		t.Fatalf("the form Springer actually sends did not parse: %v", err)
	}
	if measured.String() != "2024-11-25" || measured.Precision != PrecisionDay {
		t.Errorf("got %q at %q precision", measured.String(), measured.Precision)
	}

	// And the form the format asks for, in case it ever starts arriving.
	spec, err := ParseRSSDate("Mon, 25 Nov 2024 00:00:00 GMT")
	if err != nil {
		t.Fatalf("the form rss specifies did not parse: %v", err)
	}
	if !spec.Value.Equal(measured.Value) {
		t.Errorf("the two forms of the same date parsed to %s and %s", spec.Value, measured.Value)
	}
}

func TestDateJSONRoundTrip(t *testing.T) {
	for _, in := range []string{"2021-03-08T00:00:00Z", "March 2021", "2022"} {
		want, err := ParseDate(in)
		if err != nil {
			t.Fatal(err)
		}
		b, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		var got Date
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("unmarshalling %s: %v", b, err)
		}
		if got.Raw != want.Raw || got.Precision != want.Precision || !got.Value.Equal(want.Value) {
			t.Errorf("%q round tripped through %s to %+v, want %+v", in, b, got, want)
		}
	}
}

// The json is the three fields the spec names, and value is rendered at the
// stated precision. Writing a full timestamp for a book that only said 2022 is
// the exact false claim this type exists to avoid.
func TestDateJSONShape(t *testing.T) {
	d, err := ParseDate("2022")
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"raw":"2022","precision":"year","value":"2022"}`
	if string(b) != want {
		t.Errorf("marshalled to %s, want %s", b, want)
	}
}
