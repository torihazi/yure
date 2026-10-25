package quake

import "testing"

const sample = `{
  "code": 551,
  "id": "abc",
  "issue": {"type": "DetailScale"},
  "earthquake": {
    "time": "2026/10/24 21:08:00",
    "hypocenter": {"name": "薩摩半島西方沖", "latitude": 31.2, "longitude": 129.9, "depth": 0, "magnitude": 3.6},
    "maxScale": 30,
    "domesticTsunami": "None"
  },
  "points": [
    {"pref": "鹿児島県", "scale": 10},
    {"pref": "鹿児島県", "scale": 30},
    {"pref": "熊本県", "scale": 20}
  ]
}`

func TestDecode(t *testing.T) {
	q, ok, err := decode([]byte(sample))
	if err != nil || !ok {
		t.Fatalf("decode: ok=%v err=%v", ok, err)
	}
	if q.Place != "薩摩半島西方沖" || q.MaxScale != 30 || !q.HasCenter() {
		t.Errorf("unexpected quake: %+v", q)
	}
	if got := q.PrefScales["鹿児島県"]; got != 30 {
		t.Errorf("鹿児島県 should keep the strongest scale: got %v", got)
	}
	if q.Time.Hour() != 21 || q.Time.Location().String() != "JST" {
		t.Errorf("time should be parsed as JST: %v", q.Time)
	}
}

func TestDecodeIgnoresOtherCodes(t *testing.T) {
	_, ok, err := decode([]byte(`{"code": 555, "id": "x"}`))
	if err != nil || ok {
		t.Errorf("code 555 should be skipped: ok=%v err=%v", ok, err)
	}
}

func TestScaleString(t *testing.T) {
	for s, want := range map[Scale]string{10: "1", 45: "5-", 50: "5+", 60: "6+", 70: "7", -1: "?"} {
		if got := s.String(); got != want {
			t.Errorf("Scale(%d) = %q, want %q", s, got, want)
		}
	}
}

func TestMerge(t *testing.T) {
	prompt, _, _ := decode([]byte(`{"code":551,"id":"1","issue":{"type":"ScalePrompt"},
		"earthquake":{"time":"2026/10/24 21:08:00","hypocenter":{"latitude":-200,"longitude":-200,"magnitude":-1},"maxScale":30},
		"points":[{"pref":"鹿児島県","scale":30}]}`))
	detail, _, _ := decode([]byte(sample))
	older, _, _ := decode([]byte(`{"code":551,"id":"0","earthquake":{"time":"2026/10/24 12:00:00","hypocenter":{"latitude":-200,"longitude":-200}}}`))

	var list []Quake
	list = Merge(list, prompt)
	list = Merge(list, older)
	list = Merge(list, detail)

	if len(list) != 2 {
		t.Fatalf("reports for the same quake should merge: got %d entries", len(list))
	}
	if list[0].ID != "abc" || !list[0].HasCenter() || list[0].Place != "薩摩半島西方沖" {
		t.Errorf("merged entry should have the hypocenter from the later report: %+v", list[0])
	}
	if list[1].ID != "0" {
		t.Errorf("list should stay newest first: %+v", list)
	}
}

func TestParseScale(t *testing.T) {
	for in, want := range map[string]Scale{"1": 10, "3": 30, "5-": 45, "5弱": 45, "6+": 60, "7": 70} {
		if got, err := ParseScale(in); err != nil || got != want {
			t.Errorf("ParseScale(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseScale("8"); err == nil {
		t.Error("ParseScale(\"8\") should fail")
	}
}
