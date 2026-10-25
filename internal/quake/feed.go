package quake

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// JMA publishes Atom feeds that list recent reports, each linking to its XML.
// The high-frequency feed is updated every minute; the long-term feed keeps about a week.
const (
	feedRecent = "https://www.data.jma.go.jp/developer/xml/feed/eqvol.xml"
	feedLong   = "https://www.data.jma.go.jp/developer/xml/feed/eqvol_l.xml"

	pollInterval = time.Minute // the feed is cached for 60 seconds, so polling faster is pointless
	userAgent    = "yure (+https://github.com/torihazi/yure)"
)

// Source fetches reports from the JMA feeds, caching report files on disk.
// Published reports never change, so a cached file is valid forever.
type Source struct {
	client             *http.Client
	recentURL, longURL string
	cacheDir           string // empty disables caching
}

func NewSource() *Source {
	s := &Source{
		client:    &http.Client{Timeout: 20 * time.Second},
		recentURL: feedRecent,
		longURL:   feedLong,
	}
	if dir, err := os.UserCacheDir(); err == nil {
		s.cacheDir = filepath.Join(dir, "yure", "jmaxml")
		if os.MkdirAll(s.cacheDir, 0o755) != nil {
			s.cacheDir = ""
		}
		s.pruneCache(8 * 24 * time.Hour)
	}
	return s
}

type entry struct {
	Title   string    `xml:"title"`
	Updated time.Time `xml:"updated"`
	Link    struct {
		Href string `xml:"href,attr"`
	} `xml:"link"`
}

// feed fetches a feed and returns its earthquake entries, oldest first.
func (s *Source) feed(ctx context.Context, url string) ([]entry, error) {
	body, err := s.get(ctx, url)
	if err != nil {
		return nil, err
	}
	var f struct {
		Entries []entry `xml:"entry"`
	}
	if err := xml.Unmarshal(body, &f); err != nil {
		return nil, fmt.Errorf("feed: %w", err)
	}
	var out []entry
	for _, e := range f.Entries {
		if _, ok := kindByTitle[e.Title]; ok {
			out = append(out, e)
		}
	}
	// Reports must be merged in the order they were issued (速報 before 詳細).
	slices.SortStableFunc(out, func(a, b entry) int { return a.Updated.Compare(b.Updated) })
	return out, nil
}

// report fetches and parses one report, using the disk cache when possible.
func (s *Source) report(ctx context.Context, url string) (Quake, error) {
	cached := ""
	if s.cacheDir != "" {
		cached = filepath.Join(s.cacheDir, path.Base(url))
		if data, err := os.ReadFile(cached); err == nil {
			return ParseReport(data)
		}
	}
	data, err := s.get(ctx, url)
	if err != nil {
		return Quake{}, err
	}
	q, err := ParseReport(data)
	if err == nil && cached != "" {
		// Only cache what parsed, so a truncated download doesn't stick around.
		_ = os.WriteFile(cached, data, 0o644)
	}
	return q, err
}

func (s *Source) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("気象庁 %s: %s", path.Base(url), resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// reports fetches the reports for entries concurrently and returns them in entry order.
// Reports that fail to load are skipped: one odd report shouldn't hide all the others.
func (s *Source) reports(ctx context.Context, entries []entry) []Quake {
	results := make([]*Quake, len(entries))
	sem := make(chan struct{}, 4) // be gentle with JMA's server
	var wg sync.WaitGroup
	for i, e := range entries {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if q, err := s.report(ctx, e.Link.Href); err == nil {
				results[i] = &q
			}
		}()
	}
	wg.Wait()

	var out []Quake
	for _, q := range results {
		if q != nil {
			out = append(out, *q)
		}
	}
	return out
}

// Load returns earthquakes from about the past week (as far as the long-term feed goes)
// that occurred at or after since, newest first.
func (s *Source) Load(ctx context.Context, since time.Time) ([]Quake, error) {
	entries, err := s.feed(ctx, s.longURL)
	if err != nil {
		return nil, err
	}
	// The report is issued after the quake, so this keeps everything that might qualify.
	entries = slices.DeleteFunc(entries, func(e entry) bool { return e.Updated.Before(since) })

	var list []Quake
	for _, q := range s.reports(ctx, entries) {
		list = Merge(list, q)
	}
	list = slices.DeleteFunc(list, func(q Quake) bool { return q.Time.Before(since) })
	return list, nil
}

// Event is what Watch and Replay send: a new report, or a change in feed state.
type Event struct {
	Quake *Quake
	OK    bool  // the feed was read successfully
	Err   error // reading the feed failed; Watch keeps trying
}

// Watch polls the high-frequency feed and sends reports issued after `after`
// until ctx is canceled. Earlier reports are left to Load.
func (s *Source) Watch(ctx context.Context, after time.Time, events chan<- Event) {
	seen := make(map[string]bool)
	for {
		entries, err := s.feed(ctx, s.recentURL)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			send(ctx, events, Event{Err: err})
		} else {
			send(ctx, events, Event{OK: true})
			current := make(map[string]bool, len(entries))
			for _, e := range entries {
				current[e.Link.Href] = true
				if seen[e.Link.Href] || !e.Updated.After(after) {
					continue
				}
				q, err := s.report(ctx, e.Link.Href)
				if err != nil {
					continue // try again on the next poll
				}
				seen[e.Link.Href] = true
				send(ctx, events, Event{Quake: &q})
			}
			// Forget entries that dropped off the feed so the map doesn't grow forever.
			for href := range seen {
				if !current[href] {
					delete(seen, href)
				}
			}
		}

		select {
		case <-time.After(pollInterval):
		case <-ctx.Done():
			return
		}
	}
}

// Replay sends recent real reports one at a time, oldest first, as if they were
// arriving live. Useful for trying the app when nothing is happening.
func (s *Source) Replay(ctx context.Context, n int, interval time.Duration, events chan<- Event) {
	entries, err := s.feed(ctx, s.longURL)
	if err != nil {
		send(ctx, events, Event{Err: err})
		return
	}
	entries = entries[max(0, len(entries)-n):]
	quakes := s.reports(ctx, entries)
	send(ctx, events, Event{OK: true})
	for i := range quakes {
		select {
		case <-time.After(interval):
		case <-ctx.Done():
			return
		}
		send(ctx, events, Event{Quake: &quakes[i]})
	}
}

func send(ctx context.Context, events chan<- Event, e Event) {
	select {
	case events <- e:
	case <-ctx.Done():
	}
}

// pruneCache deletes cached reports older than maxAge. Errors are ignored:
// the cache is only an optimization.
func (s *Source) pruneCache(maxAge time.Duration) {
	files, err := os.ReadDir(s.cacheDir)
	if err != nil {
		return
	}
	for _, f := range files {
		info, err := f.Info()
		if err == nil && time.Since(info.ModTime()) > maxAge {
			_ = os.Remove(filepath.Join(s.cacheDir, f.Name()))
		}
	}
}
