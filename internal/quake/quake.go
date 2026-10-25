// Package quake reads earthquake reports from the Japan Meteorological Agency (気象庁)
// through its official XML feed (気象庁防災情報XML).
//
// Feed and format: https://xml.kishou.go.jp/
package quake

import (
	"fmt"
	"time"
)

// Scale is the JMA seismic intensity (震度):
// 10 = 震度1, 20 = 震度2, ..., 45 = 5弱, 50 = 5強, 55 = 6弱, 60 = 6強, 70 = 7.
// Spacing the values out keeps 5弱 < 5強 < 6弱 comparable with plain integers.
type Scale int

const (
	ScaleUnknown Scale = -1
	// scaleAtLeast5 is "震度5弱以上と推定されるが、まだ観測値が届いていない".
	scaleAtLeast5 Scale = 46
)

func (s Scale) String() string {
	switch s {
	case 10:
		return "1"
	case 20:
		return "2"
	case 30:
		return "3"
	case 40:
		return "4"
	case 45, scaleAtLeast5:
		return "5-"
	case 50:
		return "5+"
	case 55:
		return "6-"
	case 60:
		return "6+"
	case 70:
		return "7"
	default:
		return "?"
	}
}

// Japanese returns the name used in Japanese, e.g. "震度5弱".
func (s Scale) Japanese() string {
	switch s {
	case 45, scaleAtLeast5:
		return "震度5弱"
	case 50:
		return "震度5強"
	case 55:
		return "震度6弱"
	case 60:
		return "震度6強"
	case ScaleUnknown:
		return "震度不明"
	default:
		return "震度" + s.String()
	}
}

// ParseScale parses an intensity as written in JMA XML:
// 1, 2, 3, 4, 5-, 5+, 6-, 6+, 7. "5弱" style is accepted too.
func ParseScale(s string) (Scale, error) {
	switch s {
	case "1":
		return 10, nil
	case "2":
		return 20, nil
	case "3":
		return 30, nil
	case "4":
		return 40, nil
	case "5-", "5弱":
		return 45, nil
	case "5+", "5強":
		return 50, nil
	case "6-", "6弱":
		return 55, nil
	case "6+", "6強":
		return 60, nil
	case "7":
		return 70, nil
	}
	return 0, fmt.Errorf("unknown intensity %q (use 1, 2, 3, 4, 5-, 5+, 6-, 6+ or 7)", s)
}

// Kinds of report. JMA issues them in this order for one earthquake.
const (
	KindScalePrompt = "ScalePrompt" // 震度速報: intensity only, ~1.5 minutes after
	KindDestination = "Destination" // 震源に関する情報: hypocenter only
	KindDetail      = "DetailScale" // 震源・震度に関する情報: both, in detail
)

// Tsunami outlook in Japan, as stated in the report's fixed comments.
const (
	TsunamiNone         = "None"         // 津波の心配なし
	TsunamiNonEffective = "NonEffective" // 若干の海面変動 (no damage expected)
	TsunamiWarning      = "Warning"      // 津波警報・注意報などを発表中
	TsunamiChecking     = "Checking"     // 今後の情報に注意
	TsunamiUnknown      = ""
)

// noCoord is used for Lat and Lon when the hypocenter isn't known yet.
const noCoord = -200

// Quake is one earthquake, built from one or more reports.
type Quake struct {
	ID         string // JMA's EventID, shared by all reports about the same quake
	Kind       string // kind of the latest report merged in
	Canceled   bool   // the report retracts an earlier one (取消)
	Time       time.Time
	Place      string  // name of the hypocenter area, e.g. 能登半島沖
	Lat, Lon   float64 // hypocenter; see HasCenter
	Depth      int     // km; 0 means very shallow (ごく浅い), -1 unknown
	Magnitude  float64 // -1 if unknown
	MaxScale   Scale
	Tsunami    string
	PrefScales map[string]Scale // strongest intensity per prefecture
}

// HasCenter reports whether the hypocenter is known.
// 震度速報 is issued before the hypocenter is determined.
func (q Quake) HasCenter() bool {
	return q.Lat > noCoord && q.Lon > noCoord
}

// jst is fixed rather than loaded from tzdata so the binary works anywhere.
var jst = time.FixedZone("JST", 9*60*60)
