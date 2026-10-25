package quake

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

// Endpoints of the API. A struct rather than constants so tests can point it elsewhere.
type Endpoints struct {
	History   string // latest reports of any kind (about the past week)
	Search    string // earthquake reports since 2015, filterable by date
	WebSocket string
}

var Production = Endpoints{
	History:   "https://api.p2pquake.net/v2/history",
	Search:    "https://api.p2pquake.net/v2/jma/quake",
	WebSocket: "wss://api.p2pquake.net/v2/ws",
}

const userAgent = "yure (+https://github.com/torihazi/yure)"

// Recent returns up to limit of the latest earthquake reports, newest first.
func Recent(ctx context.Context, ep Endpoints, limit int) ([]Quake, error) {
	return get(ctx, fmt.Sprintf("%s?codes=%d&limit=%d", ep.History, codeQuake, limit))
}

// The search API allows only 10 requests a minute,
// so at most this many pages (of 100) are fetched per search.
const maxSearchPages = 3

// Since returns earthquakes that occurred at or after since, newest first.
// Only the final detailed report (各地の震度) of each quake is fetched,
// so there is one entry per quake. Results are capped at 300 quakes;
// truncated reports whether there were more.
func Since(ctx context.Context, ep Endpoints, since time.Time) (quakes []Quake, truncated bool, err error) {
	// The API filters by date (JST), so fetch from the start of that day and trim below.
	date := since.In(jst).Format("20060102")
	for page := 0; page < maxSearchPages; page++ {
		url := fmt.Sprintf("%s?quake_type=DetailScale&since_date=%s&limit=100&offset=%d", ep.Search, date, page*100)
		batch, err := get(ctx, url)
		if err != nil {
			return nil, false, err
		}
		for _, q := range batch {
			if !q.Time.Before(since) {
				quakes = append(quakes, q)
			}
		}
		if len(batch) < 100 {
			return quakes, false, nil
		}
	}
	return quakes, true, nil
}

func get(ctx context.Context, url string) ([]Quake, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("P2P地震情報 API: %s", resp.Status)
	}

	var raw []json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	var quakes []Quake
	for _, r := range raw {
		q, ok, err := decode(r)
		if err != nil {
			return nil, err
		}
		if ok {
			quakes = append(quakes, q)
		}
	}
	return quakes, nil
}

// Event is what Stream sends: a new report, or a change in connection state.
type Event struct {
	Quake     *Quake
	Connected bool
	Err       error // set when the connection dropped; Stream will reconnect
}

// Stream connects to the WebSocket API and sends events until ctx is canceled.
// It reconnects with backoff when the connection drops.
func Stream(ctx context.Context, ep Endpoints, events chan<- Event) {
	backoff := time.Second
	for ctx.Err() == nil {
		err := streamOnce(ctx, ep, events)
		if ctx.Err() != nil {
			return
		}
		send(ctx, events, Event{Err: err})

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}
		backoff = min(backoff*2, time.Minute)
	}
}

func streamOnce(ctx context.Context, ep Endpoints, events chan<- Event) error {
	conn, _, err := websocket.Dial(ctx, ep.WebSocket, &websocket.DialOptions{
		HTTPHeader: http.Header{"User-Agent": {userAgent}},
	})
	if err != nil {
		return err
	}
	defer conn.CloseNow()
	// Reports with many observation points can exceed the default 32KB limit.
	conn.SetReadLimit(4 << 20)
	send(ctx, events, Event{Connected: true})

	// The API may deliver the same report more than once.
	seen := make(map[string]bool)
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		q, ok, err := decode(data)
		if err != nil || !ok || seen[q.ID] {
			continue
		}
		seen[q.ID] = true
		send(ctx, events, Event{Quake: &q})
	}
}

func send(ctx context.Context, events chan<- Event, e Event) {
	select {
	case events <- e:
	case <-ctx.Done():
	}
}

// Replay fetches recent real reports and sends them one at a time, oldest first,
// as if they were arriving live. Useful for trying the app when nothing is happening.
func Replay(ctx context.Context, ep Endpoints, interval time.Duration, events chan<- Event) {
	quakes, err := Recent(ctx, ep, 20)
	if err != nil {
		send(ctx, events, Event{Err: err})
		return
	}
	send(ctx, events, Event{Connected: true})
	for i := len(quakes) - 1; i >= 0; i-- {
		select {
		case <-time.After(interval):
		case <-ctx.Done():
			return
		}
		send(ctx, events, Event{Quake: &quakes[i]})
	}
}
