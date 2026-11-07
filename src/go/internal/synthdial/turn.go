package synthdial

// The weighted mean of each live term's clamped span coordinate: a pure function of the values. Absent params do not count.
func (r Resolved) Reading(values map[string]float64) float64 {
	sum, w := 0.0, 0.0
	for _, t := range r.Terms {
		v, ok := values[t.Param]
		if !ok {
			continue
		}
		sum += t.W() * clamp(t.Coord(v), 0, 1)
		w += t.W()
	}
	if w == 0 {
		return 0
	}
	return sum / w
}

// Every term moves the same fraction k of its remaining travel toward the end (u → u + (1−u)·k up, u·(1−k) down), so
// offsets are kept and the reading lands exactly on target; a term already past that end stays. Ends are exact; the
// current reading returns start unchanged.
func (r Resolved) Turn(start map[string]float64, target float64) map[string]float64 {
	target = clamp(target, 0, 1)
	out := make(map[string]float64, len(r.Terms))
	for _, t := range r.Terms {
		if v, ok := start[t.Param]; ok {
			out[t.Param] = v
		}
	}
	cur := r.Reading(start)
	if target == cur || len(out) == 0 {
		return out
	}
	up := target > cur
	us := make([]float64, len(r.Terms))
	inside := true
	for i, t := range r.Terms {
		if v, ok := start[t.Param]; ok {
			us[i] = t.Coord(v)
			if us[i] < 0 || us[i] > 1 {
				inside = false
			}
		}
	}
	moved := func(u, k float64) (float64, bool) {
		if up && u < 1 {
			return u + (1-u)*k, true
		}
		if !up && u > 0 {
			return u * (1 - k), true
		}
		return u, false
	}
	readingAt := func(k float64) float64 {
		sum, w := 0.0, 0.0
		for i, t := range r.Terms {
			if _, ok := start[t.Param]; !ok {
				continue
			}
			u, _ := moved(us[i], k)
			sum += t.W() * clamp(u, 0, 1)
			w += t.W()
		}
		return sum / w
	}
	var k float64
	switch {
	case target == 1 || target == 0:
		k = 1
	case inside && up:
		k = (target - cur) / (1 - cur)
	case inside:
		k = (cur - target) / cur
	default:
		// A term outside its span contributes its clamped end until it moves
		// back in, so the reading is piecewise in k: bisect (it is continuous
		// and monotone in k).
		lo, hi := 0.0, 1.0
		for i := 0; i < 64; i++ {
			mid := (lo + hi) / 2
			if rm := readingAt(mid); (up && rm < target) || (!up && rm > target) {
				lo = mid
			} else {
				hi = mid
			}
		}
		k = (lo + hi) / 2
	}
	for i, t := range r.Terms {
		v, ok := start[t.Param]
		if !ok {
			continue
		}
		u, m := moved(us[i], k)
		if !m {
			continue
		}
		if k == 1 {
			u = 0
			if up {
				u = 1
			}
		}
		out[t.Param] = t.Value(u, v)
	}
	return out
}
