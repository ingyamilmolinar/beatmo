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
// It is embedded anonymously in Game so existing g.sel / g.moveMode /
// g.longPressPopup / … keep resolving via field promotion; grouping just names
// this cohesive cluster and lifts ~31 fields out of the ~490-field Game struct
// (no behaviour change).
type inputGestureState struct {
	sel            *uiNode
	linkDrag       dragLink
	marquee        marqueeDrag
	camDragging    bool
	camDragged     bool
	camGesture     *cameraGesture // gesture tracker: emits one pan/zoom event per finished gesture
	leftPrev       bool
	pendingClick   bool
	clickI, clickJ int
	clickNode      *uiNode

	// Move node mode: MOVE button sets moveMode, next grid click places node
	moveMode        bool    // MOVE mode active, next click places node
	movingNode      *uiNode // node being moved
	moveConfirm     bool    // confirmation dialog showing
	moveConfirmI    int     // target coordinates for pending move
	moveConfirmJ    int
	moveEdgeLoss    int  // number of edges that would be dropped
	moveSkipRelease bool // skip the first mouse release after entering move mode

	// Long-press quick-action popup (mobile)
	longPressPopup          bool
	longPressPopupNode      *uiNode
	longPressPopupRect      image.Rectangle
	longPressPopupMove      image.Rectangle
	longPressPopupConn      image.Rectangle
	longPressPopupDel       image.Rectangle
	longPressPopupHover     string // "move", "connect", "delete", or ""
	longPressPopupLastHover string // hover from previous frame (for release detection)

	// Connect mode: next node tap creates edge from connectFromNode → target
	connectMode     bool
	connectFromNode *uiNode

	// Coordinate badge above selected/created node
	coordBadgeNode  *uiNode // node to show badge for
	coordBadgeFrame int64   // frame when badge was set

	pinchBaseScale        float64 // camera scale when pinch started
	pinchBaseGestureScale float64 // gesture scale value on first pinch event
}
