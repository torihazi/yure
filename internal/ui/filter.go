package ui

import (
	"time"

	"github.com/torihazi/yure/internal/quake"
)

// Periods and minimum intensities that the p and s keys cycle through.
// Based on real data: Japan has ~5-7 felt quakes a day, about 70% of them 震度1,
// so 7 days is ~30 quakes and 30 days is ~170.
var (
	periods = []struct {
		label string
		span  time.Duration
	}{
		{"24時間", 24 * time.Hour},
		{"7日", 7 * 24 * time.Hour},
		{"30日", 30 * 24 * time.Hour},
	}
	minScales = []quake.Scale{0, 20, 30, 40, 45}
)

const defaultPeriod = 1 // 7日

type filter struct {
	period   int // index into periods
	minScale int // index into minScales
}

func (f filter) since(now time.Time) time.Time {
	return now.Add(-periods[f.period].span)
}

func (f filter) label() string {
	s := periods[f.period].label + " · "
	if min := minScales[f.minScale]; min == 0 {
		s += "全震度"
	} else {
		s += min.Japanese() + "以上"
	}
	return s
}

// apply returns the quakes that pass the filter, keeping the order (newest first).
func (f filter) apply(all []quake.Quake, now time.Time) []quake.Quake {
	since := f.since(now)
	min := minScales[f.minScale]
	var out []quake.Quake
	for _, q := range all {
		if q.Time.Before(since) {
			break // all is newest first, so the rest are older too
		}
		if q.MaxScale >= min {
			out = append(out, q)
		}
	}
	return out
}
