package spr

import (
	"errors"
	"reflect"
	"testing"
)

func TestParseDOI(t *testing.T) {
	const want = "10.1007/s10994-021-05946-3"
	cases := []struct {
		in   string
		want DOI
		err  bool
	}{
		// The five shapes measured across the surfaces, all the same DOI.
		{"10.1007/s10994-021-05946-3", want, false},
		{"doi:10.1007/s10994-021-05946-3", want, false},
		{"https://doi.org/10.1007/s10994-021-05946-3", want, false},
		{"http://dx.doi.org/10.1007/s10994-021-05946-3", want, false},
		{"10.1007/S10994-021-05946-3", want, false},

		{"  10.1007/s10994-021-05946-3  ", want, false},
		{"info:doi/10.1007/s10994-021-05946-3", want, false},
		{"https://www.doi.org/10.1007/s10994-021-05946-3", want, false},

		// A book chapter DOI, where the suffix is an ISBN and a part number.
		{"10.1007/978-3-031-28170-9_5", "10.1007/978-3-031-28170-9_5", false},

		{"", "", true},
		{"s10994-021-05946-3", "", true},
		{"10.1007", "", true},
		{"10./suffix", "", true},
		{"10.1007/", "", true},
		{"https://link.springer.com/article/x", "", true},
	}
	for _, tc := range cases {
		got, err := ParseDOI(tc.in)
		if tc.err {
			if err == nil {
				t.Errorf("ParseDOI(%q) = %q, want an error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseDOI(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseDOI(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// The prefix does not say what a work is, but the suffix says enough to put the
// likely answer first, and the difference is a wasted request per miss.
func TestDOIPaths(t *testing.T) {
	cases := []struct {
		doi   string
		first string
		n     int
	}{
		{"10.1007/s10994-021-05946-3", "/article/10.1007/s10994-021-05946-3", 1},
		{"10.1007/978-3-031-28170-9_5", "/chapter/10.1007/978-3-031-28170-9_5", 3},
		{"10.1007/978-3-031-28170-9", "/book/10.1007/978-3-031-28170-9", 1},
		{"10.1057/9781137025197", "/article/10.1057/9781137025197", 5},
	}
	for _, tc := range cases {
		got := DOI(tc.doi).Paths()
		if len(got) != tc.n {
			t.Errorf("DOI(%q).Paths() has %d entries %v, want %d", tc.doi, len(got), got, tc.n)
			continue
		}
		if got[0] != tc.first {
			t.Errorf("DOI(%q).Paths()[0] = %q, want %q", tc.doi, got[0], tc.first)
		}
	}
}

func TestDOISpringer(t *testing.T) {
	for _, d := range []string{"10.1007/x", "10.1038/x", "10.1186/x", "10.1057/x"} {
		if !DOI(d).Springer() {
			t.Errorf("DOI(%q).Springer() = false, want true", d)
		}
	}
	for _, d := range []string{"10.1145/x", "10.1109/x", "10.48550/arxiv.2301.00001"} {
		if DOI(d).Springer() {
			t.Errorf("DOI(%q).Springer() = true, want false", d)
		}
	}
}

func TestParseISSN(t *testing.T) {
	cases := []struct {
		in   string
		want ISSN
		err  error
	}{
		// Machine Learning, both of them, measured off the journal page.
		{"1573-0565", "1573-0565", nil},
		{"0885-6125", "0885-6125", nil},
		{"15730565", "1573-0565", nil},
		{"1573 0565", "1573-0565", nil},

		// An X check digit, which is the case a naive parser drops.
		{"0378-5955", "0378-5955", nil},

		{"1573-0566", "", ErrChecksum},
		{"0885-6126", "", ErrChecksum},
		{"1573-056", "", errShape},
		{"157A-0565", "", errShape},
		{"", "", errShape},
	}
	for _, tc := range cases {
		got, err := ParseISSN(tc.in)
		switch {
		case tc.err == nil && err != nil:
			t.Errorf("ParseISSN(%q): %v", tc.in, err)
		case tc.err == ErrChecksum && !errors.Is(err, ErrChecksum):
			t.Errorf("ParseISSN(%q) = %v, want a checksum error", tc.in, err)
		case tc.err == errShape && err == nil:
			t.Errorf("ParseISSN(%q) = %q, want a shape error", tc.in, got)
		case tc.err == nil && got != tc.want:
			t.Errorf("ParseISSN(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseISBN(t *testing.T) {
	cases := []struct {
		in   string
		want ISBN
		err  error
	}{
		// The book behind the chapter capture.
		{"978-3-031-28170-9", "978-3-031-28170-9", nil},
		{"9783031281709", "9783031281709", nil},

		// A 10 digit isbn is converted up, because 978 plus the first nine
		// digits is the same book by definition.
		{"0-306-40615-2", "9780306406157", nil},
		{"0306406152", "9780306406157", nil},

		// An X check digit on the 10 digit form.
		{"097522980X", "9780975229804", nil},

		{"978-3-031-28170-8", "", ErrChecksum},
		{"0-306-40615-3", "", ErrChecksum},
		{"978-3-031-2817", "", errShape},
		{"", "", errShape},
	}
	for _, tc := range cases {
		got, err := ParseISBN(tc.in)
		switch {
		case tc.err == nil && err != nil:
			t.Errorf("ParseISBN(%q): %v", tc.in, err)
		case tc.err == ErrChecksum && !errors.Is(err, ErrChecksum):
			t.Errorf("ParseISBN(%q) = %v, want a checksum error", tc.in, err)
		case tc.err == errShape && err == nil:
			t.Errorf("ParseISBN(%q) = %q, want a shape error", tc.in, got)
		case tc.err == nil && got != tc.want:
			t.Errorf("ParseISBN(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	if got := ISBN("978-3-031-28170-9").Key(); got != "9783031281709" {
		t.Errorf("Key() = %q, want the hyphenless form", got)
	}
}

func TestParseORCID(t *testing.T) {
	cases := []struct {
		in   string
		want ORCID
		err  error
	}{
		// From the JSON-LD on the open access article, in the shape it is served.
		{"http://orcid.org/0000-0002-9944-4108", "0000-0002-9944-4108", nil},
		{"https://orcid.org/0000-0002-9944-4108", "0000-0002-9944-4108", nil},
		{"0000-0002-9944-4108", "0000-0002-9944-4108", nil},
		{"0000000299444108", "0000-0002-9944-4108", nil},

		// Josiah Carberry, the orcid everyone tests with.
		{"0000-0002-1825-0097", "0000-0002-1825-0097", nil},

		// The X check digit, which is a tenth of all orcids and the case a
		// digits-only parser silently discards.
		{"0000-0001-8250-009X", "0000-0001-8250-009X", nil},
		{"0000-0001-8250-009x", "0000-0001-8250-009X", nil},

		{"0000-0002-9944-4107", "", ErrChecksum},
		{"0000-0002-1825-0098", "", ErrChecksum},
		{"0000-0001-8250-0090", "", ErrChecksum},
		{"0000-0002-9944-410", "", errShape},
		{"", "", errShape},
	}
	for _, tc := range cases {
		got, err := ParseORCID(tc.in)
		switch {
		case tc.err == nil && err != nil:
			t.Errorf("ParseORCID(%q): %v", tc.in, err)
		case tc.err == ErrChecksum && !errors.Is(err, ErrChecksum):
			t.Errorf("ParseORCID(%q) = %v, want a checksum error", tc.in, err)
		case tc.err == errShape && err == nil:
			t.Errorf("ParseORCID(%q) = %q, want a shape error", tc.in, got)
		case tc.err == nil && got != tc.want:
			t.Errorf("ParseORCID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseROR(t *testing.T) {
	cases := []struct {
		in   string
		want ROR
		err  error
	}{
		{"https://ror.org/042nb2s44", "042nb2s44", nil}, // MIT
		{"042nb2s44", "042nb2s44", nil},                 //
		{"https://ror.org/00hx57361", "00hx57361", nil}, // Princeton
		{"https://ror.org/00f54p054", "00f54p054", nil}, // Stanford

		{"042nb2s45", "", ErrChecksum},
		{"142nb2s44", "", errShape}, // a ror id starts with 0
		{"04lnb2s44", "", errShape}, // l is not in crockford's alphabet
		{"042nb2s4", "", errShape},
		{"", "", errShape},
	}
	for _, tc := range cases {
		got, err := ParseROR(tc.in)
		switch {
		case tc.err == nil && err != nil:
			t.Errorf("ParseROR(%q): %v", tc.in, err)
		case tc.err == ErrChecksum && !errors.Is(err, ErrChecksum):
			t.Errorf("ParseROR(%q) = %v, want a checksum error", tc.in, err)
		case tc.err == errShape && err == nil:
			t.Errorf("ParseROR(%q) = %q, want a shape error", tc.in, got)
		case tc.err == nil && got != tc.want:
			t.Errorf("ParseROR(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSpringerID(t *testing.T) {
	cases := []struct {
		in   string
		want SpringerID
		err  bool
	}{
		{"10994", "10994", false},
		{"/journal/10994", "10994", false},
		{"https://link.springer.com/journal/10994", "10994", false},
		{"https://link.springer.com/journal/10994/volumes-and-issues", "10994", false},
		{"10994?foo=bar", "10994", false},
		{"machine-learning", "", true},
		{"", "", true},
	}
	for _, tc := range cases {
		got, err := ParseSpringerID(tc.in)
		if tc.err {
			if err == nil {
				t.Errorf("ParseSpringerID(%q) = %q, want an error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseSpringerID(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseSpringerID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	if got := SpringerID("10994").URL(); got != Base+"/journal/10994" {
		t.Errorf("URL() = %q", got)
	}
}

// Springer builds an article DOI suffix as s<journal id>-<year>-<sequence>, so
// the journal id is readable from the DOI. It is a convenience and not a source,
// and the test says which cases it declines rather than guesses at.
func TestSpringerIDFromDOI(t *testing.T) {
	cases := []struct {
		doi  string
		want SpringerID
		ok   bool
	}{
		{"10.1007/s10994-021-05946-3", "10994", true},
		{"10.1007/s00193-024-01208-y", "00193", true},
		{"10.1007/978-3-031-28170-9_5", "", false},
		{"10.1007/s-no-digits", "", false},
		{"10.1007/nothing", "", false},
	}
	for _, tc := range cases {
		got, ok := SpringerIDFromDOI(DOI(tc.doi))
		if ok != tc.ok || got != tc.want {
			t.Errorf("SpringerIDFromDOI(%q) = %q %v, want %q %v", tc.doi, got, ok, tc.want, tc.ok)
		}
	}
}

// Every identifier that carries a resolver form should produce one, because a
// record that stores a bare id and prints a bare id is not citable.
func TestResolverForms(t *testing.T) {
	got := []string{
		DOI("10.1007/s10994-021-05946-3").URL(),
		ORCID("0000-0002-9944-4108").URL(),
		ROR("042nb2s44").URL(),
	}
	want := []string{
		"https://doi.org/10.1007/s10994-021-05946-3",
		"https://orcid.org/0000-0002-9944-4108",
		"https://ror.org/042nb2s44",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("resolver forms = %q, want %q", got, want)
	}
}

// errShape is a marker for the table above: this input is the wrong shape
// rather than the wrong number, and the two are worth telling apart because a
// bad shape usually means the wrong field was read and a bad checksum usually
// means a typo.
var errShape = errors.New("shape")
