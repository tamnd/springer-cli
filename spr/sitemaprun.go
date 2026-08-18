package spr

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Fetching sitemaps, and walking the ones that are worth walking.
//
// Two very different jobs live here. The eight static maps are eight requests
// for 39,404 container urls, which is the highest value per request anywhere in
// this tool and is over in sixteen seconds. The dated shards are 10,408
// requests, five and three quarter hours at the default pace, and up to 52
// million urls. So the static maps just run, and the shards are billed first.

// ShardThreshold is how many shards a selection may cover before the command
// asks to be told the window.
//
// A hundred is two hundred seconds and half a million urls, which is a run
// somebody can sit through and a number they can hold in their head. Above it,
// the difference between meaning to enumerate the last three days and meaning
// to enumerate everything since 1850 is one flag, and the tool should not guess
// which one was meant.
const ShardThreshold = 100

// staticMaps are the eight maps that are not dated shards.
//
// journals is three urls and not one. Springer, BMC with SpringerOpen, and
// Palgrave are three separate maps by imprint, they hold 3,043, 468 and 46
// entries, and they overlap: 3,557 entries are 3,405 distinct journals, because
// springer and bmc list 149 of the same titles. Fetching one of them and
// calling it the journal list is short by hundreds either way.
var staticMaps = map[string][]string{
	"journals": {
		"/sitemap-springer-journals.xml",
		"/sitemap-bmc-springeropen-journals.xml",
		"/sitemap-palgrave-journals.xml",
	},
	"series":      {"/sitemap-entries/sitemap_book_series.xml"},
	"collections": {"/sitemap-collections.xml"},
	"brands":      {"/sitemap-entries/sitemap_brands.xml"},
	"shops":       {"/sitemap-entries/sitemap_shops.txt"},
	"subjects":    {SubjectsIndexPath},
}

// StaticNames are the sets --static accepts, in the order they are listed in
// the help.
var StaticNames = []string{"journals", "series", "collections", "brands", "shops", "subjects"}

// StaticSet is one named set of container urls, assembled from however many
// maps that set actually lives in.
type StaticSet struct {
	Name     string   `json:"name"`
	Maps     []string `json:"maps"`
	Entries  []Entry  `json:"entries"`
	Notes    []string `json:"notes,omitempty"`
	Envelope Envelope `json:"envelope"`
}

// KnownStatic reports whether a name is one of the static sets.
func KnownStatic(name string) bool {
	_, ok := staticMaps[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

// SitemapIndex fetches the root index, which is one request for the shape of
// everything this site has published.
func (c *Client) SitemapIndex(ctx context.Context) (*Sitemap, error) {
	return c.Sitemap(ctx, IndexPath)
}

// Sitemap fetches and parses one map, at a path or a full url.
func (c *Client) Sitemap(ctx context.Context, raw string) (*Sitemap, error) {
	resp, err := c.Get(ctx, raw, KindXML)
	if err != nil {
		return nil, err
	}
	return ParseSitemap(resp)
}

// Static fetches a named set and returns its urls, deduplicated.
//
// The dedup is not tidiness. It is the whole reason this function exists rather
// than a caller fetching a url: three of these sets are one map, one of them is
// three maps that overlap, and the only way to get the journal list right is to
// take the union and say how many titles were listed twice.
func (c *Client) Static(ctx context.Context, name string, onNote func(string)) (*StaticSet, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	paths, ok := staticMaps[name]
	if !ok {
		return nil, fmt.Errorf("%q is not a static sitemap, and the ones there are is %s", name, strings.Join(StaticNames, ", "))
	}

	set := &StaticSet{Name: name, Envelope: Envelope{Tier: "sitemap"}}
	note := func(format string, args ...any) {
		s := fmt.Sprintf(format, args...)
		set.Notes = append(set.Notes, s)
		if onNote != nil {
			onNote(s)
		}
	}

	seen := map[string]bool{}
	var listed, duplicates int
	for _, p := range paths {
		m, err := c.Sitemap(ctx, p)
		if err != nil {
			return nil, err
		}
		set.Maps = append(set.Maps, m.URL)
		set.Envelope.URLs = append(set.Envelope.URLs, m.Envelope.URLs...)
		set.Envelope.Bytes += m.Envelope.Bytes
		set.Envelope.Redirects += m.Envelope.Redirects
		if set.Envelope.Fetched.IsZero() {
			set.Envelope.Fetched = m.Envelope.Fetched
			set.Envelope.Status = m.Envelope.Status
		}
		for field, v := range m.Envelope.Via {
			set.Envelope.carry(field, v)
		}
		for _, miss := range m.Envelope.Missed {
			set.Envelope.miss(miss.Field, miss.Why)
		}
		set.Envelope.Unread = append(set.Envelope.Unread, m.Envelope.Unread...)

		entries := m.Entries
		// The subjects map is an index of ten more sitemaps rather than a list
		// of pages, so what it has to give is those ten addresses. Fetching them
		// is a separate decision and the caller gets to make it.
		if m.Form == FormIndex {
			entries = nil
			for _, child := range m.Children {
				entries = append(entries, Entry{URL: child.URL, Kind: "sitemap", LastMod: child.LastMod})
			}
			note("%s is an index rather than a list, so these %d urls are sitemaps and not pages", baseName(m.URL), len(entries))
		}

		listed += len(entries)
		for _, e := range entries {
			if seen[e.URL] {
				duplicates++
				continue
			}
			seen[e.URL] = true
			set.Entries = append(set.Entries, e)
		}
	}

	if duplicates > 0 {
		note("%d urls across %d maps are %d distinct ones, because the imprint maps list %d of the same titles", listed, len(paths), len(set.Entries), duplicates)
	}
	set.Envelope.sortMissed()
	return set, nil
}

// EnumOptions are the knobs on a walk of the dated shards.
type EnumOptions struct {
	// Kinds filters on the url's own first path segment, so it costs nothing
	// and it happens before anything is printed. Empty keeps everything.
	Kinds []string

	// Limit stops the walk after this many entries have been passed to Each.
	// Zero means no limit.
	Limit int

	// Resume is the record of shards already read. Nil walks all of them.
	Resume *Resume

	// Note receives what the caller has a right to know while the walk is still
	// running. It is stderr in the command.
	Note func(string)

	// Each receives every entry that passed the filter, in shard order, as it
	// is read. It is a callback because the upper bound on this walk is 52
	// million urls and holding them is not a thing this tool will do.
	Each func(Entry) error
}

// EnumStats is what a walk cost and what it found.
type EnumStats struct {
	// Shards is how many were selected, Read is how many were fetched, Skipped
	// is how many a resumed run already had, and Failed is how many did not
	// answer and are therefore still not done.
	Shards  int `json:"shards"`
	Read    int `json:"read"`
	Skipped int `json:"skipped,omitempty"`
	Failed  int `json:"failed,omitempty"`

	// Empty is shards that answered 200 with an empty urlset.
	Empty int `json:"empty,omitempty"`

	// Entries is every url read and Matched is the ones that passed --kind.
	Entries int `json:"entries"`
	Matched int `json:"matched"`

	Bytes int `json:"bytes"`
}

// Enumerate walks the given shards and hands every entry that passes the filter
// to Each.
//
// The children are passed in rather than selected here so that the caller bills
// the run from the same list it walks. A cost that is computed from one number
// and paid against another is not a cost anybody should trust.
func (c *Client) Enumerate(ctx context.Context, children []Child, opts EnumOptions) (*EnumStats, error) {
	stats := &EnumStats{Shards: len(children)}
	note := func(format string, args ...any) {
		if opts.Note != nil {
			opts.Note(fmt.Sprintf(format, args...))
		}
	}

	want := map[string]bool{}
	for _, k := range opts.Kinds {
		if norm, ok := NormalizeKind(k); ok {
			want[norm] = true
		}
	}

	for _, child := range children {
		if opts.Resume != nil && opts.Resume.Done(child.URL) {
			stats.Skipped++
			continue
		}
		m, err := c.Sitemap(ctx, child.URL)
		if err != nil {
			if ctx.Err() != nil {
				return stats, err
			}
			// The first shard failing is the run failing. A shard failing after
			// urls have been printed is one gap in a long walk, and stopping
			// there would throw away the rest of a five hour job over it. It is
			// not marked done either way, so a resumed run comes back for it.
			if stats.Read == 0 {
				return stats, err
			}
			stats.Failed++
			note("%s did not answer, %q, and it is left unread so that --resume comes back for it", child.Name, err)
			continue
		}
		stats.Read++
		stats.Bytes += m.Envelope.Bytes
		if len(m.Entries) == 0 {
			stats.Empty++
		}

		for _, e := range m.Entries {
			stats.Entries++
			if len(want) > 0 && !want[e.Kind] {
				continue
			}
			stats.Matched++
			if opts.Each != nil {
				if err := opts.Each(e); err != nil {
					return stats, err
				}
			}
			if opts.Limit > 0 && stats.Matched >= opts.Limit {
				if opts.Resume != nil {
					// The shard is finished as far as this run is concerned,
					// but it is not finished, so it stays unmarked.
					_ = opts.Resume.Close()
				}
				return stats, nil
			}
		}

		if opts.Resume != nil {
			if err := opts.Resume.Mark(child.URL); err != nil {
				note("the resume file could not be written, %q, so this run is not resumable", err)
				opts.Resume = nil
			}
		}
		// Often enough to know it is alive on a five hour walk, rarely enough
		// that it is not the output.
		if stats.Read%50 == 0 && stats.Read < len(children) {
			note("%d of %d shards, %d urls", stats.Read+stats.Skipped, len(children), stats.Matched)
		}
	}
	if stats.Failed > 0 {
		note("%d of %d shards did not answer and were left unread", stats.Failed, stats.Shards)
	}
	return stats, nil
}

// Cost is what a walk will cost, computed from an index that was just fetched
// and never from a number compiled in here. The index grows daily and a bill
// that is right today and wrong in a month is worse than no bill.
type Cost struct {
	Requests int
	Duration time.Duration
	MaxURLs  int
	MaxBytes int64
}

// EnumCost bills a walk of n shards at a pace.
//
// The duration counts the gaps and not the requests, because the pacer waits
// between requests, and the url and byte figures are upper bounds from the
// largest shard measured rather than an average that would read as a forecast.
func EnumCost(n int, pace time.Duration) Cost {
	if n <= 0 {
		return Cost{}
	}
	return Cost{
		Requests: n,
		Duration: time.Duration(n-1) * pace,
		MaxURLs:  n * ShardCeiling,
		MaxBytes: int64(n) * ShardBytes,
	}
}

// Resume is the list of shards a walk has already finished.
//
// One url per line, appended as each shard is done, in a file keyed on the
// selection that produced it. The key matters: resuming a walk of the last
// three days against the state of a walk of everything since 1850 would skip
// most of what was asked for and report a clean run.
type Resume struct {
	Path string

	done map[string]bool
	w    *os.File
}

// ResumeKey names the state file for one selection. It is a hash because a
// selection contains dates, kinds and an index url, and none of that belongs in
// a file name.
func ResumeKey(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])[:16]
}

// OpenResume opens the state file for a selection, reading what an earlier run
// finished and opening the file to append what this one finishes.
func OpenResume(dir, key string) (*Resume, error) { return openResumeIn(dir, "sitemap", key) }

// OpenGraphResume is the same file for a graph walk, kept in its own directory
// so that a resumed sitemap walk and a resumed graph walk cannot collide on a
// key. The two record different things about different urls.
func OpenGraphResume(dir, key string) (*Resume, error) { return openResumeIn(dir, "graph", key) }

func openResumeIn(dir, sub, key string) (*Resume, error) {
	if dir == "" {
		return nil, errors.New("resuming needs a cache directory, and --no-cache turned it off")
	}
	path := filepath.Join(dir, sub, key+".resume")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	r := &Resume{Path: path, done: map[string]bool{}}

	if f, err := os.Open(path); err == nil {
		scan := bufio.NewScanner(f)
		// A url is well under this and a corrupt line should not take the run
		// down with a token too long.
		scan.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scan.Scan() {
			if line := strings.TrimSpace(scan.Text()); line != "" {
				r.done[line] = true
			}
		}
		_ = f.Close()
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	w, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	r.w = w
	return r, nil
}

// Done reports whether a shard was finished by this run or an earlier one.
func (r *Resume) Done(url string) bool { return r != nil && r.done[url] }

// Count is how many shards were already done when this run started plus the
// ones it has finished since.
func (r *Resume) Count() int {
	if r == nil {
		return 0
	}
	return len(r.done)
}

// Mark records a shard as finished, on disk, immediately. Buffering this would
// mean that the interruption the file exists for is the one thing that loses
// it.
func (r *Resume) Mark(url string) error {
	if r == nil || r.w == nil {
		return nil
	}
	r.done[url] = true
	if _, err := fmt.Fprintln(r.w, url); err != nil {
		return err
	}
	return r.w.Sync()
}

// Close releases the state file. The file stays: it is what the next --resume
// reads, and spr cache clear is what removes it.
func (r *Resume) Close() error {
	if r == nil || r.w == nil {
		return nil
	}
	err := r.w.Close()
	r.w = nil
	return err
}

// SortEntries puts a set of entries in a stable order, by kind and then by url,
// so that two runs of the same command produce the same bytes.
func SortEntries(entries []Entry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Kind != entries[j].Kind {
			return entries[i].Kind < entries[j].Kind
		}
		return entries[i].URL < entries[j].URL
	})
}
