package ui

// input_dynamics.go — the platform input-dynamics boundary.
//
// Raw wheel units are NOT comparable across runtimes:
//
//	native GLFW  : ebiten.Wheel() is +/-1 per physical detent, ACCUMULATED
//	               across every event in the frame (input_glfw.go: WheelY += yoff)
//	browser/WASM : ebiten.Wheel() is the DOM's raw deltaY, ASSIGNED (last event
//	               in the frame wins) and with deltaMode DISCARDED upstream
//	               (input_js.go carries a "TODO: What if e.deltaMode is not
//	               DOM_DELTA_PIXEL?"). One notch is ~100 in Chrome/Safari/Edge
//	               (DOM_DELTA_PIXEL) but ~3 in Firefox (DOM_DELTA_LINE) — a ~33x
//	               spread that no single constant can serve.
//
// Letting those units reach UI code is what made one browser wheel notch scroll
// a menu through its whole list: DrumViewTree passed the raw delta to
// MenuScroll.HandleWheel, which deliberately respects magnitude (one step, one
// row). Correct on native, catastrophic in a browser.
//
// The boundary is therefore one-way and narrow:
//
//	raw platform units --[wheelNotchesFromRaw]--> DETENTS --[WheelNormalizer]--> rows
//	  wheel_js.go / wheel_nonjs.go                  ^ platform-neutral above here
//
// Only wheel_js.go and wheel_nonjs.go may read the raw wheel; everything else
// consumes detents. wheel_dynamics_discipline_test.go enforces that.
//
// deltaMode reaches Go from a tiny capture-phase listener in index.html, since
// it exists ONLY in the DOM and Ebiten drops it. Per the JS Test Charter that
// listener is dumb transport; ALL conversion logic lives here in Go, where it
// is unit-tested.

// WheelDeltaMode mirrors the DOM WheelEvent.deltaMode. Native builds always
// report WheelDeltaNotch, which is already the unit we want.
type WheelDeltaMode int

const (
	// WheelDeltaPixel is DOM_DELTA_PIXEL — Chrome, Safari, Edge, and every
	// trackpad. Also the zero value, so an absent/garbled deltaMode from JS
	// degrades to the overwhelmingly common case rather than to nonsense.
	WheelDeltaPixel WheelDeltaMode = 0
	// WheelDeltaLine is DOM_DELTA_LINE — Firefox's mouse wheel.
	WheelDeltaLine WheelDeltaMode = 1
	// WheelDeltaPage is DOM_DELTA_PAGE — rare (some assistive/kiosk setups).
	WheelDeltaPage WheelDeltaMode = 2
	// WheelDeltaNotch marks a delta that is ALREADY in detents: native GLFW,
	// and any test feeding detents directly. Converting it is the identity.
	WheelDeltaNotch WheelDeltaMode = 3
)

// Raw units that make up one physical detent, per deltaMode. The pixel and
// line figures are the de-facto web platform conventions (100px/notch,
// 3 lines/notch) that browsers themselves use to size a "wheel click".
const (
	wheelPixelsPerNotch = 100.0
	wheelLinesPerNotch  = 3.0
	wheelPagesPerNotch  = 1.0
)

// wheelNotchesFromRaw converts one platform's raw wheel delta into detents.
//
// The result is deliberately FRACTIONAL: a trackpad glide is a fraction of a
// notch, and rounding it here would either delete slow scrolling or inflate it
// to a full row. WheelNormalizer accumulates the fraction instead.
func wheelNotchesFromRaw(delta float64, mode WheelDeltaMode) float64 {
	switch mode {
	case WheelDeltaLine:
		return delta / wheelLinesPerNotch
	case WheelDeltaPage:
		return delta / wheelPagesPerNotch
	case WheelDeltaNotch:
		return delta
	default: // WheelDeltaPixel, and anything unrecognized
		return delta / wheelPixelsPerNotch
	}
}

// WheelDynamics is the detents-to-rows feel calibration.
//
// These values are deliberately IDENTICAL on browser and native. Once the raw
// units are normalized to detents there is nothing platform-specific left to
// tune, which is the point of the boundary: no divergent knob means no
// divergence to drift. (Contrast RuntimeProfile, whose fields genuinely differ
// per runtime and each carry a bench citation.)
type WheelDynamics struct {
	// RowsPerNotch is how far one physical detent moves a list.
	RowsPerNotch float64
	// MaxRowsPerFrame caps a single frame's emission so one violent flick
	// glides instead of teleporting past the whole list.
	MaxRowsPerFrame int
	// CarryMax bounds the retained remainder. It is what stops a flick from
	// banking an unbounded scroll debt that keeps paying out long after the
	// gesture ended.
	CarryMax float64
	// CarryIdleFrames drops a leftover remainder after this many consecutive
	// empty frames, so a partial notch from an old gesture cannot combine with
	// a new one into a phantom jump.
	CarryIdleFrames int
}

// defaultWheelDynamics is the shared calibration for every runtime.
func defaultWheelDynamics() WheelDynamics {
	return WheelDynamics{
		RowsPerNotch:    1,  // one detent, one row — matches native GLFW feel
		MaxRowsPerFrame: 3,  // ~180 rows/s at 60fps: fast, still readable
		CarryMax:        12, // a big flick glides ~4 frames, then stops
		CarryIdleFrames: 12, // ~200ms of stillness clears a partial notch
	}
}

// WheelNormalizer converts detents into whole row steps, carrying the
// fractional remainder between frames.
//
// Carrying is what makes a trackpad usable: each event is a small fraction of a
// notch, and truncating per event would round every one of them to zero. It is
// equally what keeps a flick smooth — the excess above MaxRowsPerFrame is kept,
// not discarded, so the list glides to a stop instead of clipping.
//
// Not safe for concurrent use; each owner keeps its own (carry is per-surface
// state, and sharing one would let a menu's leftover motion leak into the grid).
type WheelNormalizer struct {
	d     WheelDynamics
	carry float64
	idle  int
}

// NewWheelNormalizer returns a normalizer with the given calibration.
func NewWheelNormalizer(d WheelDynamics) *WheelNormalizer {
	return &WheelNormalizer{d: d}
}

// Rows folds one frame's detent movement into whole row steps.
// Call once per frame per surface, including with 0 so idle decay can run.
func (n *WheelNormalizer) Rows(notches float64) int {
	if notches == 0 {
		n.idle++
		if n.d.CarryIdleFrames > 0 && n.idle >= n.d.CarryIdleFrames {
			n.carry = 0
			return 0
		}
	} else {
		n.idle = 0
	}

	n.carry += notches * n.d.RowsPerNotch
	if n.d.CarryMax > 0 {
		n.carry = clampFloat(n.carry, -n.d.CarryMax, n.d.CarryMax)
	}

	// Truncate toward zero: a partial row is not movement yet, it is carry.
	rows := int(n.carry)
	if n.d.MaxRowsPerFrame > 0 {
		if rows > n.d.MaxRowsPerFrame {
			rows = n.d.MaxRowsPerFrame
		} else if rows < -n.d.MaxRowsPerFrame {
			rows = -n.d.MaxRowsPerFrame
		}
	}
	n.carry -= float64(rows)
	return rows
}

// Reset drops any retained remainder — call when a surface closes so a stale
// gesture cannot bleed into the next time it opens.
func (n *WheelNormalizer) Reset() {
	n.carry = 0
	n.idle = 0
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// --- Platform-neutral wheel readers ---------------------------------------
//
// Everything below consumes DETENTS and is therefore identical on every
// runtime. These are the ONLY wheel accessors UI code may call; the raw
// wheel() lives behind wheelNotches() in the two build-tagged shims.
//
// They are deliberately STATELESS. Several call sites read the wheel in the
// same frame (the grid dispatcher, the drum row-scroll fallback, the camera),
// and each must see the same movement, so a shared carry would be consumed by
// whichever ran first. Surfaces that want smooth sub-notch accumulation own
// their own WheelNormalizer instead (see DrumViewTree).

// wheelRowsFromNotches converts detents to whole row steps without carry.
//
// Any non-zero movement yields at least one row: these call sites are
// discrete "did the user scroll?" consumers, and swallowing a small trackpad
// delta entirely would make them feel dead. This reproduces the old native
// behavior (`d > 0.5 -> 1`) exactly, while finally bounding the browser path.
func wheelRowsFromNotches(n float64) int {
	if n == 0 {
		return 0
	}
	rows := int(n) // truncate toward zero
	if rows == 0 {
		if n > 0 {
			return 1
		}
		return -1
	}
	max := defaultWheelDynamics().MaxRowsPerFrame
	if rows > max {
		rows = max
	} else if rows < -max {
		rows = -max
	}
	return rows
}

// wheelScrollSteps returns this frame's VERTICAL wheel movement as row steps.
func wheelScrollSteps() int {
	_, ny := wheelNotches()
	return wheelRowsFromNotches(ny)
}

// wheelZoomDelta returns this frame's vertical wheel movement in detents for
// continuous (non-stepped) consumers such as camera zoom, which want the
// fractional magnitude rather than whole rows.
func wheelZoomDelta() float64 {
	_, ny := wheelNotches()
	return ny
}
