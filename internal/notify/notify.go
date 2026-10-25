// Package notify shows desktop notifications for earthquakes.
package notify

import (
	"fmt"
	"slices"
	"strings"

	"github.com/gen2brain/beeep"

	"github.com/torihazi/yure/internal/quake"
)

func init() {
	beeep.AppName = "yure"
}

// Quake shows a notification such as:
//
//	震度4 岩手県沖
//	M5.2 深さ50km · 岩手県 宮城県 青森県
func Quake(q quake.Quake, demo bool) error {
	title := q.MaxScale.Japanese()
	if q.Place != "" {
		title += " " + q.Place
	}
	if demo {
		title = "[demo] " + title
	}

	var details []string
	if q.Magnitude >= 0 {
		details = append(details, fmt.Sprintf("M%.1f", q.Magnitude))
	}
	if q.HasCenter() && q.Depth > 0 {
		details = append(details, fmt.Sprintf("深さ%dkm", q.Depth))
	}
	if prefs := strongestPrefs(q, 3); len(prefs) > 0 {
		details = append(details, strings.Join(prefs, " "))
	}
	body := strings.Join(details, " · ")
	if body == "" {
		body = q.Time.Format("15:04") + " 発生"
	}
	return beeep.Notify(title, body, "")
}

// strongestPrefs returns up to n prefectures with the strongest intensity.
func strongestPrefs(q quake.Quake, n int) []string {
	prefs := make([]string, 0, len(q.PrefScales))
	for p := range q.PrefScales {
		prefs = append(prefs, p)
	}
	slices.SortFunc(prefs, func(a, b string) int {
		if d := int(q.PrefScales[b] - q.PrefScales[a]); d != 0 {
			return d
		}
		return strings.Compare(a, b) // stable order for equal intensity
	})
	return prefs[:min(n, len(prefs))]
}
