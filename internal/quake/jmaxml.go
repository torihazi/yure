package quake

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/torihazi/yure/internal/japanmap"
)

// report mirrors the parts of a JMA XML report (VXSE51/52/53) that yure uses.
//
// The document mixes several XML namespaces (Control, Head, Body and jmx_eb each
// have their own). Struct tags without a namespace match an element by its local
// name in any namespace, so `xml:"Coordinate"` also matches <jmx_eb:Coordinate>.
// Paths like "Earthquake>OriginTime" walk down nested elements.
type report struct {
	Control struct {
		Title string `xml:"Title"`
	} `xml:"Control"`
	Head struct {
		TargetDateTime string `xml:"TargetDateTime"`
		EventID        string `xml:"EventID"`
		InfoType       string `xml:"InfoType"` // 発表, 訂正, or 取消
	} `xml:"Head"`
	Body struct {
		Earthquake struct {
			OriginTime string `xml:"OriginTime"`
			Hypocenter struct {
				Name       string `xml:"Area>Name"`
				Coordinate string `xml:"Area>Coordinate"`
			} `xml:"Hypocenter"`
			Magnitude string `xml:"Magnitude"`
		} `xml:"Earthquake"`
		Intensity struct {
			MaxInt string `xml:"Observation>MaxInt"`
			Prefs  []struct {
				Name   string `xml:"Name"`
				MaxInt string `xml:"MaxInt"`
			} `xml:"Observation>Pref"`
		} `xml:"Intensity"`
		CommentCodes []string `xml:"Comments>ForecastComment>Code"`
	} `xml:"Body"`
}

var kindByTitle = map[string]string{
	"震度速報":        KindScalePrompt,
	"震源に関する情報":    KindDestination,
	"震源・震度に関する情報": KindDetail,
}

// ParseReport parses one JMA XML earthquake report.
func ParseReport(data []byte) (Quake, error) {
	var r report
	if err := xml.Unmarshal(data, &r); err != nil {
		return Quake{}, err
	}
	kind, ok := kindByTitle[r.Control.Title]
	if !ok {
		return Quake{}, fmt.Errorf("not an earthquake report: %q", r.Control.Title)
	}
	if r.Head.EventID == "" {
		return Quake{}, fmt.Errorf("%s: missing EventID", r.Control.Title)
	}

	q := Quake{
		ID:         r.Head.EventID,
		Kind:       kind,
		Canceled:   r.Head.InfoType == "取消",
		Lat:        noCoord,
		Lon:        noCoord,
		Depth:      -1,
		Magnitude:  -1,
		MaxScale:   ScaleUnknown,
		PrefScales: make(map[string]Scale),
	}
	if q.Canceled {
		return q, nil // a cancellation carries no data
	}

	// 震度速報 has no origin time; the time of the shaking is the best we have.
	t := r.Body.Earthquake.OriginTime
	if t == "" {
		t = r.Head.TargetDateTime
	}
	var err error
	if q.Time, err = time.Parse(time.RFC3339, t); err != nil {
		return Quake{}, fmt.Errorf("bad time %q: %w", t, err)
	}
	q.Time = q.Time.In(jst)

	q.Place = printable(r.Body.Earthquake.Hypocenter.Name)
	if c := r.Body.Earthquake.Hypocenter.Coordinate; c != "" {
		if q.Lat, q.Lon, q.Depth, err = parseCoordinate(c); err != nil {
			return Quake{}, err
		}
	}
	// "NaN" when the magnitude couldn't be determined.
	// The range check also rejects Inf and absurd values, which would make the ripple huge.
	if m, err := strconv.ParseFloat(r.Body.Earthquake.Magnitude, 64); err == nil && m >= 0 && m <= 10 {
		q.Magnitude = m
	}

	q.MaxScale = parseIntensity(r.Body.Intensity.MaxInt)
	for _, p := range r.Body.Intensity.Prefs {
		// Only known prefectures: an unknown one couldn't be placed on the map anyway,
		// and this keeps arbitrary feed text out of the UI.
		if _, ok := japanmap.Prefectures[p.Name]; !ok {
			continue
		}
		if s := parseIntensity(p.MaxInt); s != ScaleUnknown {
			q.PrefScales[p.Name] = s
		}
	}

	q.Tsunami = tsunami(kind, strings.Join(r.Body.CommentCodes, " "))
	return q, nil
}

// coordinate matches ISO 6709 as JMA writes it: "+28.0+139.9-470000/"
// (latitude, longitude, then altitude in meters, so depth is negative).
// The altitude is omitted when the depth is unknown.
var coordinate = regexp.MustCompile(`^([+-][0-9]+(?:\.[0-9]+)?)([+-][0-9]+(?:\.[0-9]+)?)([+-][0-9]+)?/$`)

func parseCoordinate(s string) (lat, lon float64, depthKm int, err error) {
	m := coordinate.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, 0, fmt.Errorf("bad coordinate %q", s)
	}
	// The regexp guarantees these are numbers.
	lat, _ = strconv.ParseFloat(m[1], 64)
	lon, _ = strconv.ParseFloat(m[2], 64)
	depthKm = -1
	if m[3] != "" {
		alt, _ := strconv.Atoi(m[3])
		depthKm = -alt / 1000
	}
	return lat, lon, depthKm, nil
}

// printable drops control characters (including C1 controls like U+009B, which some
// terminals treat as escape sequences) and line separators from text shown to the user.
func printable(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsGraphic(r) {
			return r
		}
		return -1
	}, s)
}

func parseIntensity(s string) Scale {
	if s == "震度５弱以上未入電" {
		return scaleAtLeast5
	}
	if sc, err := ParseScale(s); err == nil {
		return sc
	}
	return ScaleUnknown
}

// tsunami reads JMA's fixed comment codes (固定付加文). The codes are listed in
// JMA's code table; this follows how P2P地震情報's parser maps them.
func tsunami(kind, codes string) string {
	has := func(cs ...string) bool {
		for _, c := range cs {
			if strings.Contains(codes, c) {
				return true
			}
		}
		return false
	}
	switch {
	case kind == KindScalePrompt:
		return TsunamiChecking // too early to say
	case has("0215", "0230"):
		return TsunamiNone
	case has("0212", "0213", "0214"):
		return TsunamiNonEffective
	case has("0211"):
		return TsunamiWarning
	case has("0217", "0229"):
		return TsunamiChecking
	}
	return TsunamiUnknown
}
