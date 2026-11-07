package ui

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
)

func makeBeatKey(row, idx int) int { return (row << 16) | (idx & 0xFFFF) }

func splitBeatKey(key int) (row, idx int) { return key >> 16, key & 0xFFFF }

func loopSegmentLen(path []model.BeatInfo, start int) int {
	if start < 0 || start >= len(path) {
		return 0
	}
	origin := path[start].NodeID
	for i := start + 1; i < len(path); i++ {
		if path[i].NodeID == origin {
			return i - start
		}
	}
	return len(path) - start
}

// Strips the repeated loop segment, so callers get the real traversal length.
func rawBeatLen(path []model.BeatInfo, isLoop bool, loopStart int) int {
	// A non-loop's path already holds its synthesized steps.
	if !isLoop {
		return len(path)
	}
	if loopStart < 0 || loopStart >= len(path) {
		return len(path)
	}
	origin := path[loopStart].NodeID
	for i := loopStart + 1; i < len(path); i++ {
		if path[i].NodeID == origin {
			return i
		}
	}
	return len(path)
}
