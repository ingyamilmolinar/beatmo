package timeline

import "github.com/ingyamilmolinar/tunkul/core/model"

func cloneBoolSlice(src []bool) []bool {
	if len(src) == 0 {
		return nil
	}
	out := make([]bool, len(src))
	copy(out, src)
	return out
}

func cloneNodeTypeSlice(src []model.NodeType) []model.NodeType {
	if len(src) == 0 {
		return nil
	}
	out := make([]model.NodeType, len(src))
	copy(out, src)
	return out
}
