package spr

import (
	"errors"
	"fmt"
	"strings"
)

// Every record in this tool is keyed on an identifier somebody else issued, and
// four of the five carry a check digit that says whether it was typed
// correctly. This file spends its length on those check digits, and the reason
// is worth stating once: a mistyped identifier that becomes a graph node is
// worse than an absent one, because it invents a thing that does not exist and
// then quietly joins it to real things. An ORCID that fails its checksum is not
// an ORCID. It goes to the envelope with the value that failed, and the person
// reading the output gets to see the typo rather than inherit it.
//
// Nothing here goes to the network. These functions answer what an identifier
// is and what it could point at. What it actually points at is a question for
// the client, which finds out by asking.

// ErrChecksum is returned when an identifier is the right shape and the wrong
// number. It is a distinct error because the two failures mean different
// things: a bad shape is usually the wrong field, and a bad checksum is usually
// a typo.
var ErrChecksum = errors.New("check digit does not match")

// DOI is a Digital Object Identifier in its bare, lower case form.
//
// A DOI arrives in at least five shapes across the surfaces this tool reads,
// and they are all the same DOI:
//
//	10.1007/s10994-021-05946-3
//	doi:10.1007/s10994-021-05946-3
//	https://doi.org/10.1007/s10994-021-05946-3
//	http://dx.doi.org/10.1007/s10994-021-05946-3
//	10.1007/S10994-021-05946-3
//
// The handbook makes the prefix case insensitive and tells registrants to treat
// the suffix that way too, and Crossref matches case insensitively, so
// lowercasing is safe and is what makes deduplication work at all.
type DOI string

func (d DOI) String() string { return string(d) }

// URL is the resolver form, which is what a citation should carry.
func (d DOI) URL() string { return "https://doi.org/" + string(d) }

// doiPrefixes are the leading forms stripped before parsing, longest first so
// that the https form is not left as a bare dx.doi.org.
var doiPrefixes = []string{
	"https://doi.org/",
	"http://doi.org/",
	"https://dx.doi.org/",
	"http://dx.doi.org/",
	"https://www.doi.org/",
	"http://www.doi.org/",
	"doi.org/",
	"dx.doi.org/",
	"doi:",
	"DOI:",
	"info:doi/",
}

// ParseDOI normalizes any of the shapes above to the bare lower case form.
func ParseDOI(s string) (DOI, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return "", errors.New("empty doi")
	}
	for _, p := range doiPrefixes {
		if len(raw) >= len(p) && strings.EqualFold(raw[:len(p)], p) {
			raw = raw[len(p):]
			break
		}
	}
	raw = strings.TrimSpace(raw)

	// A DOI is "10." then a registrant code, then "/", then a suffix that may
	// contain anything. Checking that much catches the common mistake of
	// passing a url or a title, and checking more would reject DOIs that are
	// unusual rather than wrong.
	if !strings.HasPrefix(raw, "10.") {
		return "", fmt.Errorf("%q is not a doi: a doi starts with the characters 10 and a dot", s)
	}
	slash := strings.Index(raw, "/")
	if slash < 0 {
		return "", fmt.Errorf("%q is not a doi, which has a / between the prefix and the suffix", s)
	}
	if slash == len("10.") {
		return "", fmt.Errorf("%q has no registrant code between 10. and /", s)
	}
	if slash == len(raw)-1 {
		return "", fmt.Errorf("%q has nothing after the /", s)
	}
	return DOI(strings.ToLower(raw)), nil
}

// Springer is whether this DOI was issued under one of Springer Nature's
// registrant codes, which is worth knowing before deciding to fetch a page for
// it on link.springer.com.
func (d DOI) Springer() bool {
	prefix, _, _ := strings.Cut(string(d), "/")
	switch prefix {
	case "10.1007", // Springer
		"10.1057", // Palgrave Macmillan
		"10.1038", // Nature
		"10.1186", // BioMed Central
		"10.1140", // EPJ, published by Springer
		"10.3758", // Psychonomic Society, on Springer Link
		"10.1361", // ASM International, on Springer Link
		"10.1245": // Annals of Surgical Oncology
		return true
	}
	return false
}

// Paths returns the link.springer.com paths this DOI could be published under,
// most likely first.
//
// The prefix does not tell you what a work is. `10.1007/s10994-021-05946-3` is
// an article at /article and `10.1007/978-3-031-28170-9_5` is a chapter at
// /chapter, and both are 10.1007. The suffix does say something, though, and it
// says enough to put the likely answer first rather than to know it:
//
//   - a suffix starting with an ISBN, 978 or 979, came from a book. With a _N
//     part it is something inside the book, without one it is the book itself.
//   - a suffix starting with s and digits is Springer's journal article form,
//     where the digits after the s are the journal id.
//
// Everything else gets the full list in frequency order. The client tries them
// until one answers, which costs a request per miss and is why the order is
// worth getting right rather than worth guessing.
func (d DOI) Paths() []string {
	_, suffix, _ := strings.Cut(string(d), "/")

	inBook := []string{"/chapter/", "/protocol/", "/referenceworkentry/"}
	all := []string{"/article/", "/chapter/", "/book/", "/protocol/", "/referenceworkentry/"}

	var order []string
	switch {
	case strings.HasPrefix(suffix, "978-") || strings.HasPrefix(suffix, "979-"):
		if strings.Contains(suffix, "_") {
			order = inBook
		} else {
			order = []string{"/book/"}
		}
	case len(suffix) > 1 && suffix[0] == 's' && suffix[1] >= '0' && suffix[1] <= '9':
		order = []string{"/article/"}
	default:
		order = all
	}

	paths := make([]string, len(order))
	for i, p := range order {
		paths[i] = p + string(d)
	}
	return paths
}

// ISSN identifies a serial: a journal or a book series. It keeps its hyphen,
// because that is how it is printed, how Springer serves it, and how the ISSN
// Centre writes it.
//
// A journal usually has two, one for print and one for electronic, and they are
// different numbers for the same journal. Machine Learning is 0885-6125 in
// print and 1573-0565 electronic. Neither is more correct, so both are kept.
type ISSN string

func (i ISSN) String() string { return string(i) }

// ParseISSN accepts the printed form with or without its hyphen and validates
// the MOD 11 check digit.
func ParseISSN(s string) (ISSN, error) {
	digits := strings.ToUpper(strings.Map(keepAlnum, s))
	if len(digits) != 8 {
		return "", fmt.Errorf("%q is not an issn, which is 8 characters", s)
	}
	sum := 0
	for i := 0; i < 7; i++ {
		c := digits[i]
		if c < '0' || c > '9' {
			return "", fmt.Errorf("%q has a non digit before the check digit", s)
		}
		sum += int(c-'0') * (8 - i)
	}
	if got, want := digits[7], issnCheckDigit(sum); got != want {
		return "", fmt.Errorf("%q: %w, expected %c and found %c", s, ErrChecksum, want, got)
	}
	return ISSN(digits[:4] + "-" + digits[4:]), nil
}

func issnCheckDigit(sum int) byte {
	r := (11 - sum%11) % 11
	if r == 10 {
		return 'X'
	}
	return byte('0' + r)
}

// ISBN identifies a book, normalized to the 13 digit form with hyphens, which
// is what Springer prints and what makes an ISBN readable.
//
// The 10 digit form appears on older book records. It is converted up rather
// than kept, because 978 plus the first nine digits plus a recomputed check
// digit is the same book by definition, and one form beats two.
type ISBN string

func (i ISBN) String() string { return string(i) }

// Key is the hyphenless form, for use as a map key or a url component.
func (i ISBN) Key() string { return strings.ReplaceAll(string(i), "-", "") }

// ParseISBN accepts either form, with or without hyphens, and returns the 13
// digit form. Hyphens in the input are preserved in the output when the input
// was already 13 digits, because the grouping carries the registration group
// and the publisher and is not ours to invent.
func ParseISBN(s string) (ISBN, error) {
	raw := strings.TrimSpace(s)
	digits := strings.ToUpper(strings.Map(keepAlnum, raw))

	switch len(digits) {
	case 13:
		if err := checkISBN13(digits); err != nil {
			return "", fmt.Errorf("%q: %w", s, err)
		}
		// The input's own hyphenation is authoritative when it has any, because
		// the group boundaries vary by registrant and cannot be recomputed.
		if strings.Contains(raw, "-") {
			return ISBN(raw), nil
		}
		return ISBN(digits), nil

	case 10:
		if err := checkISBN10(digits); err != nil {
			return "", fmt.Errorf("%q: %w", s, err)
		}
		body := "978" + digits[:9]
		return ISBN(body + string(isbn13CheckDigit(body))), nil
	}
	return "", fmt.Errorf("%q is not an isbn, which is 10 or 13 characters", s)
}

func checkISBN13(d string) error {
	for i := 0; i < 13; i++ {
		if d[i] < '0' || d[i] > '9' {
			return errors.New("an isbn-13 is all digits")
		}
	}
	if got, want := d[12], isbn13CheckDigit(d[:12]); got != want {
		return fmt.Errorf("%w, expected %c and found %c", ErrChecksum, want, got)
	}
	return nil
}

func isbn13CheckDigit(body string) byte {
	sum := 0
	for i := 0; i < 12; i++ {
		w := 1
		if i%2 == 1 {
			w = 3
		}
		sum += int(body[i]-'0') * w
	}
	return byte('0' + (10-sum%10)%10)
}

func checkISBN10(d string) error {
	sum := 0
	for i := 0; i < 9; i++ {
		if d[i] < '0' || d[i] > '9' {
			return errors.New("an isbn-10 is 9 digits and a check character")
		}
		sum += int(d[i]-'0') * (10 - i)
	}
	last := d[9]
	switch {
	case last == 'X':
		sum += 10
	case last >= '0' && last <= '9':
		sum += int(last - '0')
	default:
		return errors.New("the check character of an isbn-10 is a digit or X")
	}
	if sum%11 != 0 {
		return ErrChecksum
	}
	return nil
}

// ORCID identifies a person, in the bare 16 digit form with hyphens.
//
// The JSON-LD on a work page gives it as http://orcid.org/0000-0002-9944-4108.
// The check digit is ISO 7064 MOD 11-2 and can be X, which is exactly the case
// a naive parser drops on the floor, so it is the case the tests lead with.
type ORCID string

func (o ORCID) String() string { return string(o) }

// URL is the resolver form ORCID itself prefers, which is https and bare.
func (o ORCID) URL() string { return "https://orcid.org/" + string(o) }

// ParseORCID accepts the bare form or either resolver url and validates the
// check digit.
func ParseORCID(s string) (ORCID, error) {
	raw := strings.TrimSpace(s)
	for _, p := range []string{"https://orcid.org/", "http://orcid.org/", "orcid.org/", "orcid:"} {
		if len(raw) >= len(p) && strings.EqualFold(raw[:len(p)], p) {
			raw = raw[len(p):]
			break
		}
	}
	d := strings.ToUpper(strings.Map(keepAlnum, raw))
	if len(d) != 16 {
		return "", fmt.Errorf("%q is not an orcid, which is 16 characters", s)
	}
	total := 0
	for i := 0; i < 15; i++ {
		if d[i] < '0' || d[i] > '9' {
			return "", fmt.Errorf("%q has a non digit before the check digit", s)
		}
		total = (total + int(d[i]-'0')) * 2
	}
	want := byte('0' + (12-total%11)%11)
	if (12-total%11)%11 == 10 {
		want = 'X'
	}
	if d[15] != want {
		return "", fmt.Errorf("%q: %w, expected %c and found %c", s, ErrChecksum, want, d[15])
	}
	return ORCID(d[0:4] + "-" + d[4:8] + "-" + d[8:12] + "-" + d[12:16]), nil
}

// ROR identifies an institution. It only ever arrives from OpenAlex, because
// Springer's own pages name affiliations as strings and nothing else.
//
// The form is nine characters: a leading 0, six Crockford base32 characters,
// and two check digits under ISO 7064 MOD 97-10. Crockford's alphabet leaves
// out I, L, O and U so that a person reading an id aloud cannot turn a 1 into
// an l, which is a nice touch and also means those four letters are a shape
// error rather than a checksum one.
type ROR string

func (r ROR) String() string { return string(r) }

// URL is the resolver form, which is how ROR ids are written in citations.
func (r ROR) URL() string { return "https://ror.org/" + string(r) }

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// ParseROR accepts the bare id or the resolver url and validates the checksum.
func ParseROR(s string) (ROR, error) {
	raw := strings.TrimSpace(s)
	for _, p := range []string{"https://ror.org/", "http://ror.org/", "ror.org/"} {
		if len(raw) >= len(p) && strings.EqualFold(raw[:len(p)], p) {
			raw = raw[len(p):]
			break
		}
	}
	id := strings.ToLower(raw)
	if len(id) != 9 {
		return "", fmt.Errorf("%q is not a ror id, which is 9 characters", s)
	}
	if id[0] != '0' {
		return "", fmt.Errorf("%q is not a ror id, which starts with 0", s)
	}

	var value int64
	for i := 1; i < 7; i++ {
		n := strings.IndexByte(crockford, strings.ToUpper(id[i : i+1])[0])
		if n < 0 {
			return "", fmt.Errorf("%q has %c, which is not in the base32 alphabet ror uses", s, id[i])
		}
		value = value*32 + int64(n)
	}
	if id[7] < '0' || id[7] > '9' || id[8] < '0' || id[8] > '9' {
		return "", fmt.Errorf("%q ends in something other than two check digits", s)
	}
	got := int64(id[7]-'0')*10 + int64(id[8]-'0')
	if want := 98 - (value*100)%97; got != want {
		return "", fmt.Errorf("%q: %w, expected %02d and found %02d", s, ErrChecksum, want, got)
	}
	return ROR(id), nil
}

// SpringerID is Springer's own journal number: 10994 for Machine Learning.
//
// It is a real identifier. It is in the url, it is in the DOI suffix as the
// digits after the s, and it is in the analytics payload, and it is stable. It
// is also publisher scoped and means nothing to anyone outside Springer, which
// is why a journal record stores it as springer_id and keys itself on the ISSN.
type SpringerID string

func (s SpringerID) String() string { return string(s) }

// URL is the journal home page this id names.
func (s SpringerID) URL() string { return Base + "/journal/" + string(s) }

// ParseSpringerID accepts the number, a /journal/ path, or a full url.
func ParseSpringerID(s string) (SpringerID, error) {
	raw := strings.TrimSpace(s)
	if i := strings.Index(raw, "/journal/"); i >= 0 {
		raw = raw[i+len("/journal/"):]
	}
	raw, _, _ = strings.Cut(raw, "/")
	raw, _, _ = strings.Cut(raw, "?")
	if raw == "" {
		return "", fmt.Errorf("%q has no journal id in it", s)
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < '0' || raw[i] > '9' {
			return "", fmt.Errorf("%q is not a springer journal id, which is all digits", s)
		}
	}
	return SpringerID(raw), nil
}

// SpringerIDFromDOI reads the journal id out of an article DOI suffix, which
// Springer builds as s<journal id>-<year>-<sequence>. It is a convenience and
// not a source: a journal id read this way is worth checking against the page
// before it is treated as one.
func SpringerIDFromDOI(d DOI) (SpringerID, bool) {
	_, suffix, _ := strings.Cut(string(d), "/")
	if len(suffix) < 2 || suffix[0] != 's' {
		return "", false
	}
	rest := suffix[1:]
	end := strings.IndexByte(rest, '-')
	if end <= 0 {
		return "", false
	}
	id, err := ParseSpringerID(rest[:end])
	if err != nil {
		return "", false
	}
	return id, true
}

// keepAlnum drops the separators people and publishers put in identifiers, so
// that 0000-0002-9944-4108 and 0000000299444108 parse to the same thing.
func keepAlnum(r rune) rune {
	switch {
	case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		return r
	}
	return -1
}
