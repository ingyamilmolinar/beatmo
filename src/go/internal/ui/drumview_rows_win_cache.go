package ui

import "github.com/hajimehoshi/ebiten/v2"

// rowsWinCache is the windowed row-scroll cache (perf: avoid a per-scroll
// full-layer recompose). During steady follow-scroll playback the row CONTENT
// does not change, the window merely translates. The legacy path recomposited
// (a full-layer shift-copy + double-buffer swap → Ebiten dependency-graph churn)
// on every recenter. The windowed path renders cells into a buffer WIDER than
// the visible window, fills the leading edge incrementally as the playhead
// scrolls, and blits a moving sub-rectangle each frame — recompositing only when
// the playhead scrolls past the pad (every rowsWinPadCells cells) or when
// content/size changes. See drumview_cache_rows_window.go and
// rows_layer_scroll_recompose_test.go.
//
// It is embedded anonymously in DrumView so existing dv.rowsWinBuf / … keep
// resolving via field promotion; grouping just names this cohesive cache and
// lifts its fields out of the ~590-field DrumView struct (no behaviour change).
type rowsWinCache struct {
	rowsWinBuf          *ebiten.Image // wide cache: width = rowWidth + padPx
	rowsWinBufW         int
	rowsWinBufH         int
	rowsWinBakeOffset   int  // dv.Offset the buffer's cell 0 corresponds to
	rowsWinRenderedTo   int  // exclusive buffer-cell index rendered so far
	rowsWinRowWidth     int  // visible window width (px) the buffer pitch uses
	rowsWinLength       int  // dv.Length the buffer was baked with
	rowsWinBaseX        int  // baseX the buffer was baked with
	rowsWinRowOff       int  // dv.rowOffset (vertical scroll) the buffer was baked with
	rowsWinContentDirty bool // a real content/edit change → force re-bake
	rowsWinValid        bool
}
