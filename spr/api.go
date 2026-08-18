package spr

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The Springer Nature API, which is the publisher's own and needs a key.
//
// Three endpoints, and they are not three names for one thing. /meta/v2 is the
// current metadata service, /metadata is the older one that is still answering,
// and /openaccess is the only one that returns full text, and only for works
// that are open access.
//
// What is measured here and what is not, stated plainly because the rest of
// this package is measured throughout and this file cannot be:
//
// Measured, without a key: all three endpoints answer 401 with a JSON body, and
// the body is byte for byte the same for a missing key and a wrong one. So this
// tool cannot tell a reader which of the two happened, and it says so rather
// than guessing.
//
// Not measured: every success shape below. No key was available, the endpoints
// return nothing without one, and the record types here are written against the
// publisher's published schema. They decode leniently and the envelope says
// which fields the response actually carried, so a first run with a real key
// reports what arrived rather than what was expected to.

const (
	// SpringerAPIBase is the host all three endpoints hang off.
	SpringerAPIBase = "https://" + SpringerAPIHost

	// KeyEnv is the environment variable the key is read from.
	KeyEnv = "SPRINGER_API_KEY"
)

// SpringerEndpoint is one of the three services, named rather than spelled out
// at every call site.
type SpringerEndpoint string

const (
	// EndpointMetaV2 is the current metadata service.
	EndpointMetaV2 SpringerEndpoint = "meta/v2"

	// EndpointMetadata is the older metadata service, still answering.
	EndpointMetadata SpringerEndpoint = "metadata"

	// EndpointOpenAccess is full text, for open access works only.
	EndpointOpenAccess SpringerEndpoint = "openaccess"
)

// SpringerEndpoints is every endpoint, for a flag's help text and its
// validation, in the order a person would try them.
var SpringerEndpoints = []string{
	string(EndpointMetaV2),
	string(EndpointMetadata),
	string(EndpointOpenAccess),
}

// ParseEndpoint validates an endpoint name.
func ParseEndpoint(s string) (SpringerEndpoint, error) {
	switch e := SpringerEndpoint(strings.TrimSpace(strings.ToLower(s))); e {
	case EndpointMetaV2, EndpointMetadata, EndpointOpenAccess:
		return e, nil
	case "":
		return EndpointMetaV2, nil
	default:
		return "", fmt.Errorf("%q is not one of %s", s, strings.Join(SpringerEndpoints, ", "))
	}
}

// ErrNoKey is returned before any request is made when no key is configured.
// It is a separate error because it is the one failure here a reader can fix,
// and because spending a request to be told 401 helps nobody.
var ErrNoKey = errors.New("no Springer Nature API key: set " + KeyEnv + " or put api_key in the config file")

// APIKey returns the key, from the environment first and the config file
// second, and reports whether one was found.
//
// The key is returned and never stored on the Client, so there is no field for
// a later refactor to marshal by accident. Everything that prints, caches or
// records a url runs it through stripAPIKey first, which means there is no path
// from a configured key to any output at all.
func APIKey() (string, bool) {
	if v := strings.TrimSpace(os.Getenv(KeyEnv)); v != "" {
		return v, true
	}
	if v := strings.TrimSpace(configValue("api_key")); v != "" {
		return v, true
	}
	return "", false
}

// KeySource names where the key came from, for a --debug line and for the
// version command. It never names the key.
func KeySource() string {
	if strings.TrimSpace(os.Getenv(KeyEnv)) != "" {
		return KeyEnv
	}
	if strings.TrimSpace(configValue("api_key")) != "" {
		return ConfigPath()
	}
	return ""
}

// ConfigPath is $XDG_CONFIG_HOME/spr/config, falling back to the user config
// directory, which is the same shape DefaultCacheDir uses.
func ConfigPath() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "spr", "config")
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "spr", "config")
}

// configValue reads one key out of the config file, which is key=value, one per
// line, with # for a comment. It is deliberately the smallest format that can
// hold a credential: a file this tool reads a secret out of is not the place
// for an expressive syntax with an edge case in it.
func configValue(want string) string {
	path := ConfigPath()
	if path == "" {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	scan := bufio.NewScanner(f)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != want {
			continue
		}
		return strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return ""
}

// SpringerQuery is a query against any of the three endpoints. The q parameter
// is the publisher's own mini language, and the fielded forms below are the
// ones this tool builds rather than the whole of it.
type SpringerQuery struct {
	Endpoint SpringerEndpoint

	DOI  DOI
	ISSN ISSN
	ISBN string

	// Title, Keyword and Free are the three text forms: title:"...",
	// keyword:"..." and a bare term. All three are quoted when they contain a
	// space, because an unquoted phrase is silently read as one term and then
	// the next.
	Title   string
	Keyword string
	Free    string

	// Year and DateFrom and DateTo are the date forms the endpoints accept.
	Year     string
	DateFrom string
	DateTo   string

	// Subject and Type are the publisher's own vocabularies.
	Subject string
	Type    string

	// Start is one based, not zero based, which is the one thing about this
	// API's paging that will catch a reader out.
	Start int
	Rows  int
}

// Q builds the q parameter, which is space separated field:value terms.
func (q SpringerQuery) Q() string {
	var terms []string
	add := func(field, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if field == "" {
			terms = append(terms, quoteTerm(value))
			return
		}
		terms = append(terms, field+":"+quoteTerm(value))
	}
	add("doi", string(q.DOI))
	add("issn", string(q.ISSN))
	add("isbn", q.ISBN)
	add("title", q.Title)
	add("keyword", q.Keyword)
	add("subject", q.Subject)
	add("type", q.Type)
	add("year", q.Year)
	add("datefrom", q.DateFrom)
	add("dateto", q.DateTo)
	add("", q.Free)
	return strings.Join(terms, " ")
}

// quoteTerm quotes a value that contains a space, because the q language reads
// an unquoted phrase as separate terms and returns a much larger result set
// without saying that it did.
func quoteTerm(s string) string {
	if strings.ContainsAny(s, " \t") && !strings.HasPrefix(s, `"`) {
		return `"` + s + `"`
	}
	return s
}

// URL builds the request url, key included. The key is in the url because that
// is the only way these endpoints accept one, and nothing downstream of the
// request stores this string: Get caches, prints and records the stripped form.
func (q SpringerQuery) URL(key string) string {
	endpoint := q.Endpoint
	if endpoint == "" {
		endpoint = EndpointMetaV2
	}
	v := url.Values{}
	v.Set("q", q.Q())
	if q.Start > 0 {
		v.Set("s", strconv.Itoa(q.Start))
	}
	if q.Rows > 0 {
		v.Set("p", strconv.Itoa(q.Rows))
	}
	v.Set("api_key", key)
	return SpringerAPIBase + "/" + string(endpoint) + "/json?" + v.Encode()
}

// SpringerResult is one page from any of the three endpoints.
type SpringerResult struct {
	Endpoint SpringerEndpoint `json:"endpoint"`

	// Total is the size of the whole match. The API returns it as a string in
	// a result block, which is why it is parsed here rather than decoded.
	Total int `json:"total"`

	Start int `json:"start"`

	Records []SpringerRecord `json:"records,omitempty"`

	Envelope Envelope `json:"envelope"`
}

// SpringerRecord is one work as the publisher's own API returns it.
//
// Every field here is decoded from the published schema and none of it has been
// seen on the wire, because no key was available to see it with. The envelope
// on the result records which of these fields the first real response actually
// carried, so the gap between the schema and the service is reported rather
// than assumed away.
type SpringerRecord struct {
	DOI      DOI    `json:"doi,omitempty"`
	Title    string `json:"title,omitempty"`
	Abstract string `json:"abstract,omitempty"`

	Publisher      string `json:"publisher,omitempty"`
	ContainerTitle string `json:"container_title,omitempty"`
	Volume         string `json:"volume,omitempty"`
	Issue          string `json:"issue,omitempty"`
	StartingPage   string `json:"starting_page,omitempty"`
	EndingPage     string `json:"ending_page,omitempty"`

	PublicationDate string `json:"publication_date,omitempty"`
	OnlineDate      string `json:"online_date,omitempty"`

	ISSNs []TypedISSN `json:"issns,omitempty"`
	ISBNs []string    `json:"isbns,omitempty"`

	Authors  []string `json:"authors,omitempty"`
	Subjects []string `json:"subjects,omitempty"`
	Keywords []string `json:"keywords,omitempty"`

	// OpenAccess is the publisher's own flag, which arrives as the string
	// "true" rather than as a boolean.
	OpenAccess bool `json:"open_access,omitempty"`

	URL string `json:"url,omitempty"`
}

// springerWire is the wire form. Almost everything in it is a string, including
// the numbers and the booleans, which is why there is a wire form at all.
type springerWire struct {
	Result []struct {
		Total   string `json:"total"`
		Start   string `json:"start"`
		Records string `json:"recordsDisplayed"`
	} `json:"result"`

	Records []struct {
		DOI             string `json:"doi"`
		Title           string `json:"title"`
		Abstract        string `json:"abstract"`
		Publisher       string `json:"publisher"`
		PublicationName string `json:"publicationName"`
		Volume          string `json:"volume"`
		Number          string `json:"number"`
		StartingPage    string `json:"startingPage"`
		EndingPage      string `json:"endingPage"`
		PublicationDate string `json:"publicationDate"`
		OnlineDate      string `json:"onlineDate"`
		OpenAccess      string `json:"openaccess"`

		ISSN      string `json:"issn"`
		EISSN     string `json:"eIssn"`
		PrintISBN string `json:"printIsbn"`
		EISBN     string `json:"electronicIsbn"`

		Creators []struct {
			Creator string `json:"creator"`
		} `json:"creators"`

		Subjects []string `json:"subjects"`

		Keyword []string `json:"keyword"`

		URL []struct {
			Value  string `json:"value"`
			Format string `json:"format"`
		} `json:"url"`
	} `json:"records"`

	// The failure shape, which is the only one measured. Both a missing key and
	// a wrong key answer 401 with these three fields and identical text.
	Status  string `json:"status"`
	Message string `json:"message"`
}

// SpringerSearch runs one query against the publisher's own API.
func (c *Client) SpringerSearch(ctx context.Context, q SpringerQuery) (*SpringerResult, error) {
	key, ok := APIKey()
	if !ok {
		return nil, ErrNoKey
	}
	if q.Endpoint == "" {
		q.Endpoint = EndpointMetaV2
	}
	target := q.URL(key)

	var wire springerWire
	resp, err := c.getJSON(ctx, target, &wire)
	if err != nil {
		var missing *NoRecord
		if errors.As(err, &missing) && resp != nil && resp.Code == 401 {
			// The body says "API key is invalid or missing" for both cases and
			// there is no field anywhere in the response that separates them,
			// so neither does this.
			return nil, fmt.Errorf("%s rejected the key from %s, which it reports the same way for a wrong key and a missing one", SpringerAPIHost, KeySource())
		}
		return nil, err
	}

	out := &SpringerResult{Endpoint: q.Endpoint}
	if len(wire.Result) > 0 {
		out.Total, _ = strconv.Atoi(wire.Result[0].Total)
		out.Start, _ = strconv.Atoi(wire.Result[0].Start)
	}
	for i := range wire.Records {
		r := &wire.Records[i]
		rec := SpringerRecord{
			Title:           r.Title,
			Abstract:        r.Abstract,
			Publisher:       r.Publisher,
			ContainerTitle:  r.PublicationName,
			Volume:          r.Volume,
			Issue:           r.Number,
			StartingPage:    r.StartingPage,
			EndingPage:      r.EndingPage,
			PublicationDate: r.PublicationDate,
			OnlineDate:      r.OnlineDate,
			Subjects:        r.Subjects,
			Keywords:        r.Keyword,
			OpenAccess:      strings.EqualFold(strings.TrimSpace(r.OpenAccess), "true"),
		}
		if d, err := ParseDOI(r.DOI); err == nil {
			rec.DOI = d
		}
		for _, pair := range []struct{ raw, kind string }{{r.ISSN, "print"}, {r.EISSN, "electronic"}} {
			if issn, err := ParseISSN(pair.raw); err == nil {
				rec.ISSNs = append(rec.ISSNs, TypedISSN{Value: issn, Type: pair.kind})
			}
		}
		for _, isbn := range []string{r.PrintISBN, r.EISBN} {
			if strings.TrimSpace(isbn) != "" {
				rec.ISBNs = append(rec.ISBNs, isbn)
			}
		}
		for _, cr := range r.Creators {
			if cr.Creator != "" {
				rec.Authors = append(rec.Authors, cr.Creator)
			}
		}
		for _, u := range r.URL {
			if u.Value != "" {
				rec.URL = u.Value
				break
			}
		}
		out.Records = append(out.Records, rec)
	}

	out.Envelope.Tier = "api"
	out.Envelope.record(resp)
	out.Envelope.carry("total", "api:result[0].total")
	if len(out.Records) > 0 {
		out.Envelope.carry("records", "api:records[]")
	}
	springerProvenance(&out.Envelope, resp.Body, len(wire.Records))
	out.Envelope.sortMissed()
	return out, nil
}

// springerProvenance reports the gap between the schema this reader was written
// against and the response that actually arrived.
//
// It exists because the success path here is the one thing in this package
// nobody has seen. Rather than silently returning empty fields, the first run
// with a real key names every top level key in the response that this reader
// does not read, which is how the schema gets corrected from evidence.
func springerProvenance(e *Envelope, body []byte, records int) {
	if records == 0 {
		e.miss("records", "the response carried no records block, so nothing here has been read from it")
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(body, &top); err != nil {
		return
	}
	known := map[string]bool{"result": true, "records": true, "query": true, "apiMessage": true, "facets": true}
	var unread []string
	for k := range top {
		if !known[k] {
			unread = append(unread, k)
		}
	}
	if len(unread) > 0 {
		e.Unread = append(e.Unread, unread...)
	}
}
