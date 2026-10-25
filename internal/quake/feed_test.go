package quake

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The testdata feed lists 10 earthquake reports; only the 3 in testdata are served,
// the rest return 404 and should be skipped.
var served = map[string]string{
	"20261025021232_0_VXSE51_010000.xml": "testdata/vxse51.xml",
	"20261025021537_0_VXSE52_010000.xml": "testdata/vxse52.xml",
	"20261025021715_0_VXSE53_010000.xml": "testdata/vxse53.xml",
}

// fakeJMA serves the testdata feed with report links pointing at itself.
// It counts report requests so tests can check the cache.
func fakeJMA(t *testing.T) (*Source, *atomic.Int32) {
	t.Helper()
	feed, err := os.ReadFile("testdata/feed.xml")
	if err != nil {
		t.Fatal(err)
	}
	var reportRequests atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/feed" {
			w.Write([]byte(strings.ReplaceAll(string(feed),
				"https://www.data.jma.go.jp/developer/xml/data/", srv.URL+"/data/")))
			return
		}
		reportRequests.Add(1)
		name := strings.TrimPrefix(r.URL.Path, "/data/")
		if file, ok := served[name]; ok {
			http.ServeFile(w, r, file)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return &Source{
		client:    srv.Client(),
		recentURL: srv.URL + "/feed",
		longURL:   srv.URL + "/feed",
		cacheDir:  t.TempDir(),
	}, &reportRequests
}

var longAgo = time.Date(2026, 1, 1, 0, 0, 0, 0, jst)

func TestLoad(t *testing.T) {
	s, _ := fakeJMA(t)
	list, err := s.Load(context.Background(), longAgo)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("the 3 served reports are one quake; got %d entries", len(list))
	}
	if q := list[0]; q.ID != eventID || !q.HasCenter() || q.MaxScale != 30 {
		t.Errorf("unexpected quake: %+v", q)
	}
}

func TestLoadUsesCache(t *testing.T) {
	s, requests := fakeJMA(t)
	ctx := context.Background()
	if _, err := s.Load(ctx, longAgo); err != nil {
		t.Fatal(err)
	}
	first := requests.Load()
	if _, err := s.Load(ctx, longAgo); err != nil {
		t.Fatal(err)
	}
	// Only the 7 reports that 404'd are fetched again; the 3 good ones come from disk.
	if again := requests.Load() - first; again != first-3 {
		t.Errorf("second load fetched %d reports, want %d", again, first-3)
	}
}

func TestWatchSendsNewReports(t *testing.T) {
	s, _ := fakeJMA(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan Event)
	go s.Watch(ctx, longAgo, events)

	if e := <-events; !e.OK {
		t.Fatalf("first event should report the feed was read: %+v", e)
	}
	// Reports arrive oldest first: 速報, then 震源, then 詳細.
	var kinds []string
	for len(kinds) < 3 {
		select {
		case e := <-events:
			if e.Quake != nil {
				kinds = append(kinds, e.Quake.Kind)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out; got %v", kinds)
		}
	}
	want := []string{KindScalePrompt, KindDestination, KindDetail}
	for i := range want {
		if kinds[i] != want[i] {
			t.Errorf("kinds = %v, want %v", kinds, want)
			break
		}
	}
}
