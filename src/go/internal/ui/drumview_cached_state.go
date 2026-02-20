package ui

// cachedRowWindow returns the cached window offset/length for a row when the
// row sprite cache can be sampled at full resolution.
func (dv *DrumView) cachedRowWindow(row int) (offset, length int, ok bool) {
	if dv == nil || row < 0 || row >= len(dv.Rows) {
		return 0, 0, false
	}
	if row < len(dv.rowDirty) && dv.rowDirty[row] {
		return 0, 0, false
	}
	if row < len(dv.rowFullDirty) && dv.rowFullDirty[row] {
		return 0, 0, false
	}
	if row >= len(dv.rowCache) || dv.rowCache[row] == nil {
		return 0, 0, false
	}
	if dv.rowCacheW <= 0 || dv.rowCacheH <= 0 || dv.rowCacheLen <= 0 {
		return 0, 0, false
	}
	// When zoomed out (more steps than pixels), the cache is decimated and no
	// longer represents per-step on/off state.
	if dv.rowCacheLen > dv.rowCacheW {
		return 0, 0, false
	}
	offset = dv.Offset
	if row < len(dv.rowCacheOff) {
		offset = dv.rowCacheOff[row]
	}
	length = dv.rowCacheLen
	return offset, length, true
}

// cachedRowState returns the cached row step state without sampling GPU textures.
// It relies on a snapshot captured when the row cache was rebuilt.
func (dv *DrumView) cachedRowState(row, abs int) (val bool, ok bool) {
	offset, length, ok := dv.cachedRowWindow(row)
	if !ok {
		return false, false
	}
	if abs < offset || abs >= offset+length {
		return false, false
	}
	if row < 0 || row >= len(dv.rowCacheSteps) || dv.rowCacheSteps[row] == nil {
		return false, false
	}
	rel := abs - offset
	if rel < 0 || rel >= len(dv.rowCacheSteps[row]) {
		return false, false
	}
	return dv.rowCacheSteps[row][rel], true
}
