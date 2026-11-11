package ui

import "image"

// inputGestureState is the node-editor + pointer/touch gesture interaction
// state: the current selection, in-progress link/move/connect gestures, the
// mobile long-press quick-action popup, the coordinate badge, and pinch anchors.
//
// It is single-threaded — mutated only from the UI Update goroutine's input
// handlers (game_input_*.go, game_node_interaction.go, game_longpress_popup.go),
// never from the audio/sequencer goroutines — so it carries no lock.
//
// It is embedded anonymously in Game so existing g.sel / g.modes /
// g.longPressPopup / … keep resolving via field promotion; grouping just names
// this cohesive cluster and lifts ~31 fields out of the ~490-field Game struct
// (no behaviour change).
type inputGestureState struct {
	sel      *uiNode
	linkDrag dragLink
	marquee  marqueeDrag
	guideAim guideAimState
	// Per-frame memo for guideTargetCached (draw-time sharing between the
	// grid-guide layer and the cursor label; see grid_guide.go).
	guideMemoFrame int64
	guideMemoX     int
	guideMemoY     int
	guideMemo      GuideTarget
	camDragging    bool
	camDragged     bool
	camGesture     *cameraGesture // gesture tracker: emits one pan/zoom event per finished gesture
	leftPrev       bool
	pendingClick   bool
	clickI, clickJ int
	clickNode      *uiNode

	// Desktop mouse-hold: press-and-hold on a node (no drag) for
	// mouseHoldPopupFrames opens the long-press quick-action popup. -1 =
	// hold dead for this press (moved, or already fired).
	holdFrames int
	holdStartX int
	holdStartY int

	// modes is the centralized armed grid-mode state (move/copy/connect) —
	// see grid_mode.go. Mutated ONLY via Enter/Cancel/Commit/Tick.
	modes gridModeManager

	// Move node mode: MOVE button arms g.modes with GridModeMove, next grid
	// click places node.
	moveSkipRelease bool // skip the first mouse release after entering move mode

	// Long-press quick-action popup (mobile)
	longPressPopup          bool
	longPressPopupNode      *uiNode
	longPressPopupRect      image.Rectangle
	longPressPopupMove      image.Rectangle
	longPressPopupCopy      image.Rectangle
	longPressPopupConn      image.Rectangle
	longPressPopupDel       image.Rectangle
	longPressPopupHover     string // "move", "copy", "connect", "delete", or ""
	longPressPopupLastHover string // hover from previous frame (for release detection)

	// Coordinate badge above selected/created node
	coordBadgeNode  *uiNode // node to show badge for
	coordBadgeFrame int64   // frame when badge was set

	pinchBaseScale        float64 // camera scale when pinch started
	pinchBaseGestureScale float64 // gesture scale value on first pinch event
}
