package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
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

// rawBeatLen returns the length of the non-expanded beat path. When the graph
// requests a beat row with a large beat length, the loop segment is repeated to
// fill that length. This helper strips the repeated portion so callers obtain
// the actual traversal length regardless of the current beatLength setting.
func rawBeatLen(path []model.BeatInfo, isLoop bool, loopStart int) int {
	// For non-loops, the unbounded path already includes any synthesized
	// intermediate (invisible) steps between endpoints; keep the full length.
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
