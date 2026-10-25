package quake

import (
	"os"
	"testing"
	"time"
)

// The testdata files are real reports for one quake:
// 2026-10-25 11:09 小笠原諸島西方沖, M7.0, depth 470km, max intensity 3.
func parseFile(t *testing.T, name string) Quake {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	q, err := ParseReport(data)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return q
}

const eventID = "20261025111028"

func TestParseScalePrompt(t *testing.T) {
	q := parseFile(t, "vxse51.xml")
	if q.ID != eventID || q.Kind != KindScalePrompt {
		t.Errorf("ID/Kind = %q/%q", q.ID, q.Kind)
	}
	if q.HasCenter() || q.Magnitude != -1 {
		t.Errorf("震度速報 has no hypocenter yet: %+v", q)
	}
	if q.MaxScale != 30 || q.PrefScales["東京都"] != 30 {
		t.Errorf("intensity: max %v, 東京都 %v", q.MaxScale, q.PrefScales["東京都"])
	}
	// No origin time yet; falls back to the time of shaking.
	if want := time.Date(2026, 10, 25, 11, 10, 0, 0, jst); !q.Time.Equal(want) {
		t.Errorf("Time = %v, want %v", q.Time, want)
	}
	if q.Tsunami != TsunamiChecking {
		t.Errorf("Tsunami = %q", q.Tsunami)
	}
}

func TestParseDestination(t *testing.T) {
	q := parseFile(t, "vxse52.xml")
	if q.Kind != KindDestination || q.Place != "小笠原諸島西方沖" {
		t.Errorf("Kind/Place = %q/%q", q.Kind, q.Place)
	}
	if q.Lat != 28.0 || q.Lon != 139.9 || q.Depth != 470 || q.Magnitude != 7.0 {
		t.Errorf("hypocenter: %v,%v depth %d M%v", q.Lat, q.Lon, q.Depth, q.Magnitude)
	}
	if q.MaxScale != ScaleUnknown || len(q.PrefScales) != 0 {
		t.Errorf("震源に関する情報 has no intensity: %v %v", q.MaxScale, q.PrefScales)
	}
	if q.Tsunami != TsunamiNone {
		t.Errorf("Tsunami = %q", q.Tsunami)
	}
}

func TestParseDetail(t *testing.T) {
	q := parseFile(t, "vxse53.xml")
	if q.Kind != KindDetail || !q.HasCenter() || q.MaxScale != 30 {
		t.Errorf("unexpected: %+v", q)
	}
	if want := time.Date(2026, 10, 25, 11, 9, 0, 0, jst); !q.Time.Equal(want) {
		t.Errorf("Time = %v, want %v", q.Time, want)
	}
	if len(q.PrefScales) != 16 || q.PrefScales["東京都"] != 30 || q.PrefScales["神奈川県"] != 20 || q.PrefScales["静岡県"] != 10 {
		t.Errorf("PrefScales = %v", q.PrefScales)
	}
}

func TestParseRejectsOtherReports(t *testing.T) {
	_, err := ParseReport([]byte(`<Report><Control><Title>降灰予報（定時）</Title></Control></Report>`))
	if err == nil {
		t.Error("non-earthquake report should fail")
	}
}

func TestParseCoordinate(t *testing.T) {
	tests := []struct {
		in       string
		lat, lon float64
		depth    int
	}{
		{"+28.0+139.9-470000/", 28.0, 139.9, 470},
		{"+35.6+140.1+0/", 35.6, 140.1, 0}, // ごく浅い
		{"+37.5+137.2/", 37.5, 137.2, -1},  // depth unknown
		{"-15.3-173.2-10000/", -15.3, -173.2, 10},
	}
	for _, tt := range tests {
		lat, lon, depth, err := parseCoordinate(tt.in)
		if err != nil || lat != tt.lat || lon != tt.lon || depth != tt.depth {
			t.Errorf("parseCoordinate(%q) = %v, %v, %d, %v", tt.in, lat, lon, depth, err)
		}
	}
	if _, _, _, err := parseCoordinate("北緯28度"); err == nil {
		t.Error("garbage should fail")
	}
}

func TestMerge(t *testing.T) {
	var list []Quake
	for _, f := range []string{"vxse51.xml", "vxse52.xml", "vxse53.xml"} {
		list = Merge(list, parseFile(t, f))
	}
	if len(list) != 1 {
		t.Fatalf("reports for one quake should merge: got %d", len(list))
	}
	q := list[0]
	if q.Kind != KindDetail || !q.HasCenter() || q.MaxScale != 30 || q.Tsunami != TsunamiNone {
		t.Errorf("merged: %+v", q)
	}
	if q.Time.Minute() != 9 {
		t.Errorf("origin time from the later report should win: %v", q.Time)
	}

	older := Quake{ID: "older", Time: q.Time.Add(-time.Hour)}
	list = Merge(list, older)
	if list[0].ID != eventID || list[1].ID != "older" {
		t.Errorf("list should be newest first: %v, %v", list[0].ID, list[1].ID)
	}

	list = Merge(list, Quake{ID: eventID, Canceled: true})
	if len(list) != 1 || list[0].ID != "older" {
		t.Errorf("cancellation should remove the quake: %+v", list)
	}
}

func TestParseScale(t *testing.T) {
	for in, want := range map[string]Scale{"1": 10, "3": 30, "5-": 45, "5弱": 45, "6+": 60, "7": 70} {
		if got, err := ParseScale(in); err != nil || got != want {
			t.Errorf("ParseScale(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseScale("8"); err == nil {
		t.Error(`ParseScale("8") should fail`)
	}
}

func TestScaleString(t *testing.T) {
	for s, want := range map[Scale]string{10: "1", 45: "5-", 46: "5-", 50: "5+", 60: "6+", 70: "7", -1: "?"} {
		if got := s.String(); got != want {
			t.Errorf("Scale(%d) = %q, want %q", s, got, want)
		}
	}
}
