package ui

// Raw wheel units differ by runtime: native GLFW sums ±1 per detent, while the browser assigns
// the DOM's raw deltaY with deltaMode dropped (~100 per notch in Chrome, ~3 in Firefox). Only
// wheel_js.go / wheel_nonjs.go read raw units; everything else consumes DETENTS.

type WheelDeltaMode int

const (
	// Also the zero value, so a missing or garbled deltaMode degrades to the common case.
	WheelDeltaPixel WheelDeltaMode = 0
	// WheelDeltaLine is DOM_DELTA_LINE — Firefox's mouse wheel.
	WheelDeltaLine WheelDeltaMode = 1
	// WheelDeltaPage is DOM_DELTA_PAGE — rare (some assistive/kiosk setups).
	WheelDeltaPage WheelDeltaMode = 2
	// Already detents: the identity.
	WheelDeltaNotch WheelDeltaMode = 3
)

// The browsers' own wheel-click conventions: 100 px or 3 lines.
const (
	wheelPixelsPerNotch = 100.0
	wheelLinesPerNotch  = 3.0
	wheelPagesPerNotch  = 1.0
)

// FRACTIONAL on purpose: a trackpad glide is part of a notch, and rounding would delete or inflate it.
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

// Identical on browser and native: once units are detents nothing platform-specific is left to tune.
type WheelDynamics struct {
	// RowsPerNotch is how far one physical detent moves a list.
	RowsPerNotch float64
	// So a violent flick glides instead of teleporting past the list.
	MaxRowsPerFrame int
	// Bounds the remainder, so a flick cannot bank a scroll debt that pays out after it ends.
	CarryMax float64
	// So a partial notch from an old gesture never combines with a new one into a jump.
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

// Carrying the remainder makes trackpads usable (each event is a sliver of a notch) and flicks
// smooth. Not concurrent-safe, and never shared: one surface's leftover must not leak into another.
type WheelNormalizer struct {
	d     WheelDynamics
	carry float64
	idle  int
}

// NewWheelNormalizer returns a normalizer with the given calibration.
func NewWheelNormalizer(d WheelDynamics) *WheelNormalizer {
	return &WheelNormalizer{d: d}
}

// Call once per frame per surface, 0 included, so idle decay runs.
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

// Call when a surface closes, so a stale gesture cannot bleed into the next open.
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

// The only wheel accessors UI code may call. Stateless on purpose: several sites read the wheel
// each frame and must all see the same movement; smooth surfaces own a WheelNormalizer.

// Any movement yields at least one row: these are "did the user scroll?" consumers, and dropping
// a small trackpad delta would make them feel dead.
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

// A two-finger sideways scroll.
func wheelScrollStepsX() int {
	nx, _ := wheelNotches()
	return wheelRowsFromNotches(nx)
}

// Fractional, for continuous consumers such as camera zoom.
func wheelZoomDelta() float64 {
	_, ny := wheelNotches()
	return ny
}
