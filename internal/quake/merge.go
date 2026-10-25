package quake

import "slices"

// Merge adds a report to list (newest first) and returns the new list.
//
// JMA issues several reports for one earthquake: first 震度速報 (intensity only),
// then 震源に関する情報 (hypocenter only), then 震源・震度に関する情報 (both).
// Reports share an EventID and are combined into one entry,
// keeping whatever each report knows. A cancellation removes the entry.
func Merge(list []Quake, q Quake) []Quake {
	i := slices.IndexFunc(list, func(e Quake) bool { return e.ID == q.ID })
	if q.Canceled {
		if i >= 0 {
			list = slices.Delete(list, i, i+1)
		}
		return list
	}
	if i < 0 {
		list = append(list, q)
	} else {
		mergeInto(&list[i], q)
	}
	// 震度速報 carries the time of shaking rather than the origin time,
	// so a later report can move an entry slightly. Sort to stay newest first.
	slices.SortStableFunc(list, func(a, b Quake) int { return b.Time.Compare(a.Time) })
	return list
}

func mergeInto(cur *Quake, q Quake) {
	cur.Kind = q.Kind
	if q.Kind != KindScalePrompt {
		cur.Time = q.Time // the real origin time
	}
	if q.HasCenter() {
		cur.Place, cur.Lat, cur.Lon = q.Place, q.Lat, q.Lon
		cur.Depth, cur.Magnitude = q.Depth, q.Magnitude
	}
	if len(q.PrefScales) > 0 {
		cur.PrefScales = q.PrefScales
	}
	if q.MaxScale != ScaleUnknown {
		cur.MaxScale = q.MaxScale
	}
	if q.Tsunami != TsunamiUnknown {
		cur.Tsunami = q.Tsunami
	}
}
