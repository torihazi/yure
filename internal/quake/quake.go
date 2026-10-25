// Package quake fetches earthquake reports from the P2P地震情報 API.
//
// API spec: https://www.p2pquake.net/develop/json_api_v2/
// The data originates from the Japan Meteorological Agency (気象庁).
package quake

import (
	"encoding/json"
	"fmt"
	"time"
)

// codeQuake is the API's code for 地震情報 (earthquake reports).
// Other codes (tsunami, early warnings, user reports...) are ignored for now.
const codeQuake = 551

// Scale is the JMA seismic intensity (震度) as encoded by the API:
// 10 = 震度1, 20 = 震度2, ..., 45 = 5弱, 50 = 5強, 55 = 6弱, 60 = 6強, 70 = 7.
type Scale int

const ScaleUnknown Scale = -1

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
	case 45, 46: // 46 = "5弱以上と推定されるが震度情報を入手していない"
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
	case 45, 46:
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

// Quake is one earthquake report.
type Quake struct {
	ID         string
	Type       string // ScalePrompt, Destination, DetailScale, ...
	Time       time.Time
	Place      string  // name of the hypocenter area, e.g. 能登半島沖
	Lat, Lon   float64 // hypocenter; HasCenter reports whether they are valid
	Depth      int     // km; 0 means very shallow (ごく浅い)
	Magnitude  float64 // -1 if unknown
	MaxScale   Scale
	Tsunami    string
	PrefScales map[string]Scale // strongest intensity per prefecture
}

// HasCenter reports whether the hypocenter is known.
// 震度速報 (ScalePrompt) is issued before the hypocenter is determined.
func (q Quake) HasCenter() bool {
	return q.Lat > -200 && q.Lon > -200
}

// apiQuake mirrors the JSON shape of the API.
type apiQuake struct {
	ID    string `json:"id"`
	Code  int    `json:"code"`
	Issue struct {
		Type string `json:"type"`
	} `json:"issue"`
	Earthquake struct {
		Time       string `json:"time"`
		Hypocenter struct {
			Name      string  `json:"name"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
			Depth     int     `json:"depth"`
			Magnitude float64 `json:"magnitude"`
		} `json:"hypocenter"`
		MaxScale        int    `json:"maxScale"`
		DomesticTsunami string `json:"domesticTsunami"`
	} `json:"earthquake"`
	Points []struct {
		Pref  string `json:"pref"`
		Scale int    `json:"scale"`
	} `json:"points"`
}

// jst is fixed rather than loaded from tzdata so the binary works anywhere.
var jst = time.FixedZone("JST", 9*60*60)

func (a apiQuake) toQuake() Quake {
	h := a.Earthquake.Hypocenter
	q := Quake{
		ID:         a.ID,
		Type:       a.Issue.Type,
		Place:      h.Name,
		Lat:        h.Latitude,
		Lon:        h.Longitude,
		Depth:      h.Depth,
		Magnitude:  h.Magnitude,
		MaxScale:   Scale(a.Earthquake.MaxScale),
		Tsunami:    a.Earthquake.DomesticTsunami,
		PrefScales: make(map[string]Scale),
	}
	// A bad timestamp leaves Time zero; the rest of the report is still useful.
	q.Time, _ = time.ParseInLocation("2006/01/02 15:04:05", a.Earthquake.Time, jst)
	for _, p := range a.Points {
		if s := Scale(p.Scale); s > q.PrefScales[p.Pref] {
			q.PrefScales[p.Pref] = s
		}
	}
	return q
}

// decode parses one API object. ok is false for non-earthquake messages.
func decode(data []byte) (q Quake, ok bool, err error) {
	var a apiQuake
	if err := json.Unmarshal(data, &a); err != nil {
		return Quake{}, false, err
	}
	if a.Code != codeQuake {
		return Quake{}, false, nil
	}
	return a.toQuake(), true, nil
}

// ParseScale parses an intensity written as on the command line: 1, 2, 3, 4, 5-, 5+, 6-, 6+, 7.
// "5弱" style is accepted too.
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
