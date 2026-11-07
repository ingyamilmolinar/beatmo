package ui

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
	// Zoomed out, the cache is decimated and no longer holds per-step state.
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

// Reads a snapshot taken at rebuild, never the GPU texture.
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
