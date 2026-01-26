package ui

import "github.com/ingyamilmolinar/tunkul/core/model"

// RowWindow captures a snapshot of the visible portion of a drum row.
type RowWindow struct {
	Offset int
	Steps  []bool
	Types  []model.NodeType
}

func cloneRowWindow(src RowWindow) RowWindow {
	dst := RowWindow{Offset: src.Offset}
	if len(src.Steps) > 0 {
		dst.Steps = make([]bool, len(src.Steps))
		copy(dst.Steps, src.Steps)
	}
	if len(src.Types) > 0 {
		dst.Types = make([]model.NodeType, len(src.Types))
		copy(dst.Types, src.Types)
	}
	return dst
}
