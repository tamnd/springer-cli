package spr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// springerHost is the site this tool reads.
	springerHost = "link.springer.com"

	// Base is the site root. Relative paths passed to Get are resolved against
	// it, so callers can pass "/article/10.1007/..." and mean the obvious thing.
	Base = "https://" + springerHost

	// maxRedirects is the redirect budget. Every first request runs the cookie
	// dance, 303 to idp.springer.com/authorize, 302 to /transit, 302 back with
	// ?error=cookies_not_supported&code=<uuid>, which is three hops before any
	// page is served. A pdf url the reader has no subscription for then runs the
	// whole thing twice: three hops to the pdf, one 303 across to the chapter
	// page, three more hops for that. Seven is therefore a normal Tuesday, and
	// the budget has to sit above it without letting a genuine loop run.
	maxRedirects = 10

	// maxBody caps a single response. The largest page measured is a 4.4 MB pdf
	// and the largest html is 719 KB, so 64 MB is a runaway guard and not a
	// limit anyone will meet.
	maxBody = 64 << 20
)

// version is stamped by the CLI at startup so the product token names the build
// that made the request. There is no flag for it and no way to make it look
// like a browser: the search challenge was measured against no user agent, this
// tool's own, and a current Chrome string, and it treated all three the same.
var version = "dev"

// SetVersion is called once from the CLI with the linker stamped version.
func SetVersion(v string) {
	if v != "" {
		version = v
	}
}

// UserAgent is the only product token this tool sends.
func UserAgent() string {
	return fmt.Sprintf("springer-cli/%s (+https://github.com/tamnd/springer-cli)", version)
}

// Response is one fetch, classified.
type Response struct {
	// URL is the url that was requested. It is the cache key, it is what goes
	// into a record, and it is deliberately not the url the redirect chain
	// landed on, which carries a per request uuid.
	URL string

	// Final is where the chain landed, kept for --debug and for nothing else.
	Final string

	Code      int
	Header    http.Header
	Body      []byte
	Status    Status
	Redirects int
	Fetched   time.Time
	FromCache bool
}

// Bytes is the size of the body, which the envelope records so that a page
// changing shape is visible in the output rather than only in a diff.
func (r *Response) Bytes() int { return len(r.Body) }

// Client fetches pages, paces itself per host, caches on the requested url, and
// classifies every response before anyone parses it.
type Client struct {
	HTTP    *http.Client
	Pacer   *Pacer
	Cache   *Cache
	Retries int

	// Mailto is the contact address sent to the Crossref and OpenAlex polite
	// pools. It is not sent to link.springer.com, which neither asks for one
	// nor does anything with it.
	Mailto string

	// Debug receives one line per request when set by --debug.
	Debug io.Writer

	// base overrides the site root, and is set only by tests so that a flow
	// which constructs its own urls, like a paged search, can be pointed at a
	// stub server. It is unexported and has no option, the same way the pacer's
	// clock is, because it is a seam for tests and not a feature.
	base string

	// rate holds the last rate limit budget seen per host, read from the
	// response rather than compiled in from a documentation page.
	rateMu sync.Mutex
	rate   map[string]RateLimit
}

// RateLimit is what a host last said about the budget it is enforcing.
type RateLimit struct {
	Limit     int
	Remaining int
	Reset     time.Time
	Seen      time.Time
}

// Option configures a Client.
type Option func(*Client)

// WithPace sets the general per host interval. Values below PaceFloor are
// raised to it, here and everywhere else, without exception.
func WithPace(d time.Duration) Option {
	return func(c *Client) { c.Pacer = NewPacer(d) }
}

// WithCache points the client at a cache directory. An empty dir disables it.
func WithCache(dir string, ttl time.Duration) Option {
	return func(c *Client) { c.Cache = NewCache(dir, ttl) }
}

// WithTimeout sets the per request timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.HTTP.Timeout = d }
}

// WithRetries sets how many times a transport error or a 5xx is retried. A
// challenge is not covered by this and is never retried.
func WithRetries(n int) Option {
	return func(c *Client) { c.Retries = n }
}

// WithMailto sets the contact address for the open index polite pools.
func WithMailto(addr string) Option {
	return func(c *Client) { c.Mailto = addr }
}

// WithDebug sends one line per request to w.
func WithDebug(w io.Writer) Option {
	return func(c *Client) { c.Debug = w }
}

// New returns a client with the defaults from the spec: 2s pace, a 30s timeout,
// two retries, and one connection per host.
func New(opts ...Option) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// One request at a time per host is the whole concurrency model. Pace only
	// controls the gap between requests, and without this a burst of goroutines
	// would open a burst of connections inside one interval.
	transport.MaxConnsPerHost = 1
	transport.MaxIdleConnsPerHost = 1

	c := &Client{
		HTTP: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
		Pacer:   NewPacer(DefaultPace),
		Retries: 2,
		rate:    make(map[string]RateLimit),
	}
	// The chain is followed by hand rather than by the http package, so the
	// number of hops is observable and the requested url stays the identity.
	c.HTTP.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Get fetches raw, which may be a full url or a path on link.springer.com, and
// classifies the result against the kind the caller expects.
func (c *Client) Get(ctx context.Context, raw string, want Kind) (*Response, error) {
	target, err := c.resolve(raw)
	if err != nil {
		return nil, err
	}
	if r, ok := c.Cache.Get(target); ok {
		r.Status = Classify(r.Code, r.Header, r.Body, want)
		c.debugf("cache %s %s", r.Status, target)
		return r, nil
	}

	var last error
	for attempt := 0; ; attempt++ {
		if err := c.Pacer.Wait(ctx, target); err != nil {
			return nil, err
		}
		r, err := c.fetch(ctx, target)
		switch {
		case err != nil:
			last = err
		case r.Code >= 500:
			last = fmt.Errorf("%s: upstream returned %d", target, r.Code)
		default:
			r.Status = Classify(r.Code, r.Header, r.Body, want)
			c.debugf("%d %s %d bytes %d redirects %s", r.Code, r.Status, len(r.Body), r.Redirects, target)
			// A challenge is stored so that a repeat run does not go and ask
			// for another one, and everything else is stored because it is the
			// page. A 404 is cached too: a work that does not exist still does
			// not exist five minutes later.
			if err := c.Cache.Put(r); err != nil {
				c.debugf("cache write failed: %v", err)
			}
			return r, nil
		}
		if attempt >= c.Retries {
			return nil, last
		}
		wait := backoff(attempt)
		c.debugf("retry %d in %s after %v", attempt+1, wait, last)
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// fetch performs one request and follows the redirect chain by hand.
func (c *Client) fetch(ctx context.Context, target string) (*Response, error) {
	current := target
	redirects := 0
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, current, nil)
		if err != nil {
			return nil, err
		}
		c.setHeaders(req)

		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, err
		}
		c.readRateLimit(req.URL.Host, resp.Header)

		if loc := resp.Header.Get("Location"); isRedirect(resp.StatusCode) && loc != "" {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
			redirects++
			if redirects > maxRedirects {
				return nil, fmt.Errorf("%s: more than %d redirects, which is longer than the seven hop worst case this site is known to use", target, maxRedirects)
			}
			next, err := req.URL.Parse(loc)
			if err != nil {
				return nil, fmt.Errorf("%s: redirect to an unparseable location %q: %w", target, loc, err)
			}
			current = next.String()
			continue
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: reading the body: %w", target, err)
		}
		return &Response{
			URL:       target,
			Final:     current,
			Code:      resp.StatusCode,
			Header:    resp.Header,
			Body:      body,
			Redirects: redirects,
			Fetched:   time.Now().UTC(),
		}, nil
	}
}

// setHeaders sends three headers and no more. There is no header set here that
// exists to look like a browser, because the one surface that scores clients
// was measured to ignore the user agent entirely.
func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", UserAgent())
	req.Header.Set("Accept", "*/*")
	// Accept-Encoding is left unset on purpose. Setting it by hand turns off
	// the transport's transparent gzip and makes the decode ours to get wrong.
	if c.Mailto != "" && req.URL.Host != springerHost {
		q := req.URL.Query()
		if !q.Has("mailto") {
			q.Set("mailto", c.Mailto)
			req.URL.RawQuery = q.Encode()
		}
	}
}

// readRateLimit records what the host says about the budget it is enforcing.
// No limit is compiled in: the Springer Nature API's published figure could not
// be verified without a key, and a number read from a docs page is correct
// until the day it is not.
func (c *Client) readRateLimit(host string, h http.Header) {
	limit, hasLimit := headerInt(h, "X-RateLimit-Limit")
	remaining, hasRemaining := headerInt(h, "X-RateLimit-Remaining")
	if !hasLimit && !hasRemaining {
		return
	}
	rl := RateLimit{Limit: limit, Remaining: remaining, Seen: time.Now()}
	if reset, ok := headerInt(h, "X-RateLimit-Reset"); ok {
		rl.Reset = time.Unix(int64(reset), 0)
	}
	c.rateMu.Lock()
	c.rate[strings.ToLower(host)] = rl
	c.rateMu.Unlock()
	c.debugf("rate %s %d/%d", host, remaining, limit)
}

// RateLimit reports the last budget a host stated, and whether it stated one.
func (c *Client) RateLimit(host string) (RateLimit, bool) {
	c.rateMu.Lock()
	defer c.rateMu.Unlock()
	rl, ok := c.rate[strings.ToLower(host)]
	return rl, ok
}

func (c *Client) debugf(format string, args ...any) {
	if c.Debug == nil {
		return
	}
	fmt.Fprintf(c.Debug, "spr: "+format+"\n", args...)
}

// backoff is exponential from one second. Retry-After takes precedence when a
// server sends one, which RetryAfter reads.
func backoff(attempt int) time.Duration {
	d := time.Second << attempt
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

// RetryAfter reads the header of the same name, in either of its two forms.
func RetryAfter(h http.Header) (time.Duration, bool) {
	v := h.Get("Retry-After")
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d, true
		}
		return 0, true
	}
	return 0, false
}

func headerInt(h http.Header, name string) (int, bool) {
	v := h.Get(name)
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, false
	}
	return n, true
}

func isRedirect(code int) bool {
	switch code {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	}
	return false
}

// resolve turns a path or a url into an absolute url on this site.
func (c *Client) resolve(raw string) (string, error) {
	base := Base
	if c != nil && c.base != "" {
		base = c.base
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("empty url")
	}
	if strings.HasPrefix(raw, "/") {
		return base + raw, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%q is not a url: %w", raw, err)
	}
	if u.Scheme == "" {
		return "", fmt.Errorf("%q has no scheme, pass a full url or a path starting with /", raw)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("%q: only http and https are fetched", raw)
	}
	return u.String(), nil
}
