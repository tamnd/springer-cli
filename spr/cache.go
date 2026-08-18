package spr

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// DefaultTTL is how long a cached page is considered current. A work page does
// not change hourly and a sitemap changes daily, so a day is long enough to
// make a re-run cheap and short enough that a record is not quietly a week old.
const DefaultTTL = 24 * time.Hour

// Cache is a directory of fetched responses keyed on the requested url.
//
// The key is the requested url and never the effective one. Every first request
// to link.springer.com is redirected twice and lands on the same path with
// ?error=cookies_not_supported&code=<uuid> appended, and that uuid is different
// on every request. Key on where the response came from and no key is ever seen
// twice, so the cache silently never hits and every run pays full price.
type Cache struct {
	Dir string
	TTL time.Duration
}

// entry is the sidecar written next to a cached body.
type entry struct {
	URL       string      `json:"url"`   // the requested url, which is the key
	Final     string      `json:"final"` // where the redirect chain landed
	Code      int         `json:"code"`
	Header    http.Header `json:"header"`
	Redirects int         `json:"redirects"`
	Fetched   time.Time   `json:"fetched"`
}

// NewCache returns a cache rooted at dir. An empty dir disables caching, which
// is what --no-cache does.
func NewCache(dir string, ttl time.Duration) *Cache {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &Cache{Dir: dir, TTL: ttl}
}

// DefaultCacheDir is $XDG_CACHE_HOME/spr, falling back to the user cache dir.
func DefaultCacheDir() string {
	if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
		return filepath.Join(dir, "spr")
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "spr")
}

// Key is the cache key for a url: the sha256 of the requested url, hex encoded.
// It is a hash rather than a sanitized path because urls carry query strings
// that are longer than a file name is allowed to be.
func Key(url string) string {
	sum := sha256.Sum256([]byte(url))
	return hex.EncodeToString(sum[:])
}

func (c *Cache) enabled() bool { return c != nil && c.Dir != "" }

func (c *Cache) paths(url string) (meta, body string) {
	k := Key(url)
	// One level of fan out keeps a directory listing usable after a large run.
	dir := filepath.Join(c.Dir, k[:2])
	return filepath.Join(dir, k+".json"), filepath.Join(dir, k+".body")
}

// Get returns the cached response for a url when one is present and current.
func (c *Cache) Get(url string) (*Response, bool) {
	if !c.enabled() {
		return nil, false
	}
	metaPath, bodyPath := c.paths(url)
	raw, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, false
	}
	var e entry
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, false
	}
	if time.Since(e.Fetched) > c.TTL {
		return nil, false
	}
	body, err := os.ReadFile(bodyPath)
	if err != nil {
		return nil, false
	}
	return &Response{
		URL:       e.URL,
		Final:     e.Final,
		Code:      e.Code,
		Header:    e.Header,
		Body:      body,
		Redirects: e.Redirects,
		Fetched:   e.Fetched,
		FromCache: true,
	}, true
}

// Put stores a response under the url that was requested for it.
func (c *Cache) Put(r *Response) error {
	if !c.enabled() || r == nil {
		return nil
	}
	metaPath, bodyPath := c.paths(r.URL)
	if err := os.MkdirAll(filepath.Dir(metaPath), 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(entry{
		URL:       r.URL,
		Final:     r.Final,
		Code:      r.Code,
		Header:    r.Header,
		Redirects: r.Redirects,
		Fetched:   r.Fetched,
	})
	if err != nil {
		return err
	}
	if err := os.WriteFile(bodyPath, r.Body, 0o644); err != nil {
		return err
	}
	return os.WriteFile(metaPath, raw, 0o644)
}

// Stats reports the number of cached responses and the bytes they occupy.
func (c *Cache) Stats() (n int, bytes int64, err error) {
	if !c.enabled() {
		return 0, 0, nil
	}
	err = filepath.WalkDir(c.Dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		bytes += info.Size()
		if filepath.Ext(path) == ".json" {
			n++
		}
		return nil
	})
	return n, bytes, err
}

// Clear removes every cached response.
func (c *Cache) Clear() error {
	if !c.enabled() {
		return nil
	}
	return os.RemoveAll(c.Dir)
}
