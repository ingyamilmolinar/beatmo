package ui

import "image"

// UI Update goroutine only, so no lock. Embedded in Game; fields resolve by promotion.
type inputGestureState struct {
	sel      *uiNode
	linkDrag dragLink
	marquee  marqueeDrag
	guideAim guideAimState
	// Shared per frame by the guide layer and the cursor label.
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

	// -1: the hold is dead for this press (moved, or already fired).
	holdFrames int
	holdStartX int
	holdStartY int

	// Mutated ONLY via Enter/Cancel/Commit/Tick.
	modes gridModeManager

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
