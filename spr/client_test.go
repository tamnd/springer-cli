package spr

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fast returns a client whose pacer does not actually wait, so a test that
// makes four requests takes microseconds rather than eight seconds. The pace
// logic itself is tested in pace_test.go.
func fast(t *testing.T, opts ...Option) *Client {
	t.Helper()
	c := New(append([]Option{WithCache("", 0)}, opts...)...)
	c.Pacer.sleep = func(time.Duration) {}
	c.Pacer.now = func() time.Time { return time.Unix(0, 0) }
	return c
}

// The site answers every first request with 303, then 302, then 200, and the
// last hop carries ?error=cookies_not_supported&code=<uuid> with a fresh uuid
// every time. The client has to follow that, count it, and keep the url that
// was asked for as the identity of the response.
func TestRedirectChainIsFollowedAndCounted(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Query().Get("code") != "":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<html><head><meta name="access" content="Yes"></head></html>`)
		case r.URL.Query().Get("error") != "":
			http.Redirect(w, r, srv.URL+r.URL.Path+"?error=cookies_not_supported&code=1a2b3c", http.StatusFound)
		default:
			http.Redirect(w, r, srv.URL+r.URL.Path+"?error=cookies_not_supported", http.StatusSeeOther)
		}
	}))
	defer srv.Close()

	target := srv.URL + "/article/10.1007/s10994-021-05946-3"
	resp, err := fast(t).Get(context.Background(), target, KindHTML)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.Redirects != 2 {
		t.Errorf("Redirects = %d, want 2", resp.Redirects)
	}
	if resp.URL != target {
		t.Errorf("URL = %q, want the requested url %q", resp.URL, target)
	}
	if !strings.Contains(resp.Final, "code=1a2b3c") {
		t.Errorf("Final = %q, want the last hop of the chain", resp.Final)
	}
	if resp.Status != StatusOK {
		t.Errorf("Status = %q, want %q", resp.Status, StatusOK)
	}
}

// A pdf url the reader has no subscription for runs the cookie dance, gets sent
// across to the chapter page, and runs the whole dance again: seven hops, then
// html from a url that asked for a pdf. Both halves of that are the point. The
// budget has to survive it, and the answer has to be WrongKind rather than OK.
func TestRestrictedPDFTakesTheLongWayAndLandsOnHTML(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dance := func(path string) bool {
			switch r.URL.Query().Get("hop") {
			case "":
				http.Redirect(w, r, srv.URL+"/authorize?redirect_uri="+path+"&hop=1", http.StatusSeeOther)
			case "1":
				http.Redirect(w, r, srv.URL+"/transit?redirect_uri="+path+"&hop=2", http.StatusFound)
			case "2":
				http.Redirect(w, r, srv.URL+path+"?error=cookies_not_supported&hop=3", http.StatusFound)
			default:
				return false
			}
			return true
		}
		switch r.URL.Path {
		case "/authorize", "/transit":
			dance(r.URL.Query().Get("redirect_uri"))
		case "/content/pdf/10.1007/x.pdf":
			if dance(r.URL.Path) {
				return
			}
			// The dance is done and there is still no subscription, so the site
			// sends the reader to the chapter page, which starts a new dance.
			http.Redirect(w, r, srv.URL+"/chapter/10.1007/x", http.StatusSeeOther)
		case "/chapter/10.1007/x":
			if dance(r.URL.Path) {
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, `<html><head><meta name="access" content="No"></head><body>preview</body></html>`)
		}
	}))
	defer srv.Close()

	resp, err := fast(t).Get(context.Background(), srv.URL+"/content/pdf/10.1007/x.pdf", KindPDF)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.Redirects != 7 {
		t.Errorf("Redirects = %d, want the measured 7", resp.Redirects)
	}
	if resp.Status != StatusWrongKind {
		t.Errorf("Status = %q, want %q, because a pdf url that answers html has not served a pdf", resp.Status, StatusWrongKind)
	}
	if !strings.HasSuffix(resp.URL, "/content/pdf/10.1007/x.pdf") {
		t.Errorf("URL = %q, want the pdf url that was asked for", resp.URL)
	}
	if !strings.Contains(resp.Final, "/chapter/") {
		t.Errorf("Final = %q, want the chapter page the chain landed on", resp.Final)
	}
}

func TestRedirectBudgetIsFinite(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.Path+"x", http.StatusFound)
	}))
	defer srv.Close()

	_, err := fast(t, WithRetries(0)).Get(context.Background(), srv.URL+"/loop", KindHTML)
	if err == nil {
		t.Fatal("a redirect loop returned no error")
	}
	if !strings.Contains(err.Error(), "redirects") {
		t.Errorf("error is %q, want it to name the redirect budget", err)
	}
}

// The cache key is the requested url. Keying on the effective url would put the
// per request uuid into every key, and no key would ever be seen twice, so the
// cache would silently never hit.
func TestCacheKeyIsTheRequestedURL(t *testing.T) {
	requested := "https://link.springer.com/article/10.1007/s10994-021-05946-3"
	effective := requested + "?error=cookies_not_supported&code=1a2b3c"

	if Key(requested) == Key(effective) {
		t.Fatal("two different urls hashed to one key")
	}

	dir := t.TempDir()
	c := NewCache(dir, time.Hour)
	if err := c.Put(&Response{
		URL:     requested,
		Final:   effective,
		Code:    200,
		Header:  http.Header{"Content-Type": []string{"text/html"}},
		Body:    []byte("<html></html>"),
		Fetched: time.Now(),
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, ok := c.Get(requested); !ok {
		t.Error("the requested url did not hit the cache it was stored under")
	}
	if _, ok := c.Get(effective); ok {
		t.Error("the effective url hit the cache, which means a uuid reached a key")
	}
}

func TestCacheServesTheSecondRequest(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><head><meta name="access" content="Yes"></head></html>`)
	}))
	defer srv.Close()

	c := fast(t, WithCache(t.TempDir(), time.Hour))
	for i := 0; i < 3; i++ {
		resp, err := c.Get(context.Background(), srv.URL+"/article/10.1007/x", KindHTML)
		if err != nil {
			t.Fatalf("Get %d: %v", i, err)
		}
		if i > 0 && !resp.FromCache {
			t.Errorf("request %d went to the network", i)
		}
	}
	if hits != 1 {
		t.Errorf("the server saw %d requests, want 1", hits)
	}
}

func TestCacheExpires(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(dir, time.Millisecond)
	url := "https://link.springer.com/article/10.1007/x"
	if err := c.Put(&Response{URL: url, Code: 200, Body: []byte("x"), Fetched: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, ok := c.Get(url); ok {
		t.Error("an hour old entry survived a one millisecond ttl")
	}
}

// A challenge is never retried. Retrying is what caused it, and a client that
// answers a rate limit with more requests is the reason the limit exists.
func TestChallengeIsNotRetried(t *testing.T) {
	body := challengeBody(t)
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.Header().Set("Content-Type", "text/html")
		w.Write(body)
	}))
	defer srv.Close()

	resp, err := fast(t, WithRetries(3)).Get(context.Background(), srv.URL+"/search?query=x", KindHTML)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.Status != StatusChallenged {
		t.Fatalf("Status = %q, want %q", resp.Status, StatusChallenged)
	}
	if hits != 1 {
		t.Errorf("a challenge was requested %d times, want 1", hits)
	}
}

// A 5xx is a different thing: it is worth asking again, up to the retry budget.
func TestServerErrorIsRetried(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	c := fast(t, WithRetries(2))
	// The retry backoff is real time, so keep it out of the test by cancelling
	// the context as soon as the attempts are spent.
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	_, err := c.Get(ctx, srv.URL+"/article/10.1007/x", KindHTML)
	if err == nil {
		t.Fatal("a 502 that outlived the retries returned no error")
	}
	if hits < 2 {
		t.Errorf("the server saw %d requests, want at least 2", hits)
	}
}

// A 404 is not retried and is not an error at this layer. It is an answer, and
// the command above decides what to say about it.
func TestNotFoundIsAnAnswerRatherThanAnError(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "<html><title>Page Unavailable | Springer Nature Link</title></html>")
	}))
	defer srv.Close()

	resp, err := fast(t, WithRetries(3)).Get(context.Background(), srv.URL+"/article/10.1007/nope", KindHTML)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.Status != StatusNotFound {
		t.Errorf("Status = %q, want %q", resp.Status, StatusNotFound)
	}
	if hits != 1 {
		t.Errorf("a 404 was requested %d times, want 1", hits)
	}
}

func TestRateLimitIsReadFromTheResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Limit", "100")
		w.Header().Set("X-RateLimit-Remaining", "97")
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html></html>")
	}))
	defer srv.Close()

	c := fast(t)
	resp, err := c.Get(context.Background(), srv.URL+"/metadata", KindHTML)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	host := strings.TrimPrefix(resp.Final, "http://")
	host = host[:strings.IndexByte(host, '/')]

	rl, ok := c.RateLimit(host)
	if !ok {
		t.Fatalf("no rate limit recorded for %q", host)
	}
	if rl.Limit != 100 || rl.Remaining != 97 {
		t.Errorf("rate limit is %d/%d, want 97/100", rl.Remaining, rl.Limit)
	}
}

func TestRetryAfter(t *testing.T) {
	h := http.Header{}
	if _, ok := RetryAfter(h); ok {
		t.Error("an absent Retry-After was read as present")
	}
	h.Set("Retry-After", "30")
	if d, ok := RetryAfter(h); !ok || d != 30*time.Second {
		t.Errorf("RetryAfter = %s %v, want 30s true", d, ok)
	}
	// The other legal form is an HTTP date. One in the past means wait for
	// nothing rather than wait forever.
	h.Set("Retry-After", "Mon, 02 Jan 2006 15:04:05 GMT")
	if d, ok := RetryAfter(h); !ok || d != 0 {
		t.Errorf("RetryAfter on a past date = %s %v, want 0s true", d, ok)
	}
	h.Set("Retry-After", "soon")
	if _, ok := RetryAfter(h); ok {
		t.Error("an unparseable Retry-After was read as present")
	}
}

// The only headers this tool sends are a product token and an Accept. There is
// no header here that exists to look like a browser, and Accept-Encoding is
// left to the transport so the gzip decode does not become ours to get wrong.
func TestRequestHeadersAreThree(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html></html>")
	}))
	defer srv.Close()

	if _, err := fast(t).Get(context.Background(), srv.URL+"/x", KindHTML); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ua := got.Get("User-Agent"); !strings.HasPrefix(ua, "springer-cli/") {
		t.Errorf("User-Agent is %q, want the tool's own product token", ua)
	}
	for _, name := range []string{"Cookie", "Accept-Language", "Sec-Ch-Ua", "Referer"} {
		if got.Get(name) != "" {
			t.Errorf("%s was sent, and nothing in this tool should be setting it", name)
		}
	}
}

func TestResolve(t *testing.T) {
	cases := []struct {
		in   string
		want string
		err  bool
	}{
		{"/article/10.1007/x", Base + "/article/10.1007/x", false},
		{"https://link.springer.com/journal/10994", "https://link.springer.com/journal/10994", false},
		{"https://api.crossref.org/works", "https://api.crossref.org/works", false},
		{"link.springer.com/journal/10994", "", true},
		{"ftp://example.com/x", "", true},
		{"", "", true},
	}
	for _, tc := range cases {
		got, err := resolve(tc.in)
		if tc.err {
			if err == nil {
				t.Errorf("resolve(%q) returned no error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("resolve(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("resolve(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
