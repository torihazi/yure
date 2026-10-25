package quake

// Merge adds q to list (newest first) and returns the new list.
//
// JMA issues several reports for one earthquake: first 震度速報 (intensity only),
// then 震源に関する情報 (hypocenter only), then 各地の震度 (both, in detail).
// Reports with the same origin time are combined into one entry,
// keeping whatever each report knows.
func Merge(list []Quake, q Quake) []Quake {
	for i := range list {
		if !list[i].Time.Equal(q.Time) {
			continue
		}
		cur := &list[i]
		cur.ID, cur.Type = q.ID, q.Type
		if q.HasCenter() {
			cur.Place, cur.Lat, cur.Lon = q.Place, q.Lat, q.Lon
			cur.Depth, cur.Magnitude = q.Depth, q.Magnitude
		}
		if len(q.PrefScales) > 0 {
			cur.PrefScales = q.PrefScales
		}
		if q.MaxScale > cur.MaxScale {
			cur.MaxScale = q.MaxScale
		}
		if q.Tsunami != "" && q.Tsunami != "Unknown" {
			cur.Tsunami = q.Tsunami
		}
		return list
	}

	// Insert keeping newest first. New reports are almost always the newest,
	// but history and the live feed can arrive out of order.
	i := 0
	for i < len(list) && list[i].Time.After(q.Time) {
		i++
	}
	list = append(list, Quake{})
	copy(list[i+1:], list[i:])
	list[i] = q
	return list
}
