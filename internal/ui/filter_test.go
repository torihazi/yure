package ui

import (
	"testing"
	"time"

	"github.com/torihazi/yure/internal/quake"
)

func TestFilterApply(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	ago := func(h int) time.Time { return now.Add(-time.Duration(h) * time.Hour) }
	all := []quake.Quake{ // newest first, as the model keeps them
		{Time: ago(1), MaxScale: 10},
		{Time: ago(5), MaxScale: 30},
		{Time: ago(30), MaxScale: 40},
		{Time: ago(200), MaxScale: 50},
	}
	tests := []struct {
		f    filter
		want int
	}{
		{filter{period: 0}, 2},              // 24h, any intensity
		{filter{period: 1}, 3},              // 7 days
		{filter{period: 1, minScale: 1}, 2}, // 7 days, 2以上
		{filter{period: 1, minScale: 2}, 2}, // 7 days, 3以上
		{filter{period: 0, minScale: 3}, 0}, // 24h, 4以上
	}
	for _, tt := range tests {
		if got := len(tt.f.apply(all, now)); got != tt.want {
			t.Errorf("%s: got %d quakes, want %d", tt.f.label(), got, tt.want)
		}
	}
}

func TestRefilterKeepsSelection(t *testing.T) {
	now := time.Now()
	m := model{all: []quake.Quake{
		{Time: now.Add(-time.Hour), MaxScale: 10},
		{Time: now.Add(-2 * time.Hour), MaxScale: 30},
	}, filter: filter{period: defaultPeriod}}
	m.refilter()
	m.sel = 1 // the 震度3 one

	m.filter.minScale = 2 // 3以上 hides the first quake
	m.refilter()
	if len(m.visible) != 1 || m.sel != 0 || m.visible[m.sel].MaxScale != 30 {
		t.Errorf("selection should follow the 震度3 quake: sel=%d visible=%v", m.sel, m.visible)
	}
}

// JMA's long-term feed keeps about a week, so a longer period would silently show partial data.
func TestPeriodsFitTheFeed(t *testing.T) {
	if longest := periods[len(periods)-1].span; longest > 7*24*time.Hour {
		t.Errorf("longest period %v exceeds what the feed provides", longest)
	}
}
