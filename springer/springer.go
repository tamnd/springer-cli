// Package springer is the library behind the springer command line:
// the HTTP client, request shaping, and the typed data models for Springer Nature
// academic papers fetched via the CrossRef API.
package springer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const DefaultUserAgent = "Mozilla/5.0 (compatible; springer-cli/0.1; +https://github.com/tamnd/springer-cli)"

// Config holds constructor parameters.
type Config struct {
	BaseURL   string
	UserAgent string
	Rate      time.Duration
	Retries   int
	Timeout   time.Duration
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:   "https://api.crossref.org",
		UserAgent: DefaultUserAgent,
		Rate:      500 * time.Millisecond,
		Retries:   3,
		Timeout:   30 * time.Second,
	}
}

// Client talks to the CrossRef API for Springer Nature papers.
type Client struct {
	cfg        Config
	httpClient *http.Client
	mu         sync.Mutex
	last       time.Time
}

// NewClient returns a Client with the given config.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: cfg.Timeout},
	}
}

func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		b, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return b, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get: %w", lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) ([]byte, bool, error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

func (c *Client) pace() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cfg.Rate <= 0 {
		return
	}
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// Recent fetches recently published Springer Nature papers.
func (c *Client) Recent(ctx context.Context, limit int) ([]Paper, error) {
	if limit <= 0 {
		limit = 20
	}
	apiURL := fmt.Sprintf("%s/members/297/works?rows=%d&sort=published&order=desc", c.cfg.BaseURL, limit)
	return c.fetch(ctx, apiURL, limit)
}

// Search searches Springer Nature papers by keyword.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]Paper, error) {
	if limit <= 0 {
		limit = 20
	}
	apiURL := fmt.Sprintf(
		"%s/members/297/works?rows=%d&sort=published&order=desc&query=%s",
		c.cfg.BaseURL, limit, url.QueryEscape(query),
	)
	return c.fetch(ctx, apiURL, limit)
}

func (c *Client) fetch(ctx context.Context, apiURL string, limit int) ([]Paper, error) {
	raw, err := c.get(ctx, apiURL)
	if err != nil {
		return nil, err
	}
	var resp wireResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	papers := make([]Paper, 0, len(resp.Message.Items))
	for i, w := range resp.Message.Items {
		if limit > 0 && i >= limit {
			break
		}
		papers = append(papers, wireToPaper(w, i+1))
	}
	return papers, nil
}

func wireToPaper(w wireItem, rank int) Paper {
	title := ""
	if len(w.Title) > 0 {
		title = w.Title[0]
	}
	journal := ""
	if len(w.ContainerTitle) > 0 {
		journal = w.ContainerTitle[0]
	}
	authors := formatAuthors(w.Author)
	published := formatDate(w.Published.DateParts)
	return Paper{
		Rank:      rank,
		DOI:       w.DOI,
		Title:     title,
		Authors:   authors,
		Journal:   journal,
		Published: published,
		Cited:     w.IsReferencedBy,
		URL:       "https://doi.org/" + w.DOI,
	}
}

func formatAuthors(authors []wireAuthor) string {
	if len(authors) == 0 {
		return ""
	}
	max := len(authors)
	suffix := ""
	if max > 3 {
		max = 3
		suffix = " et al."
	}
	parts := make([]string, 0, max)
	for _, a := range authors[:max] {
		given := ""
		if len(a.Given) > 0 {
			given = " " + string([]rune(a.Given)[:1])
		}
		parts = append(parts, a.Family+given)
	}
	s := strings.Join(parts, ", ") + suffix
	if len([]rune(s)) > 80 {
		s = string([]rune(s)[:79]) + "…"
	}
	return s
}

func formatDate(parts [][]int) string {
	if len(parts) == 0 || len(parts[0]) == 0 {
		return ""
	}
	row := parts[0]
	if len(row) >= 2 {
		return fmt.Sprintf("%04d-%02d", row[0], row[1])
	}
	return fmt.Sprintf("%04d", row[0])
}
