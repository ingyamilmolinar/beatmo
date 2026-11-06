package ui

// barrelBinding is the value model a BarrelWheel scrolls. The wheel owns no
// value state; it reads labels and asks the binding to step by whole detents.
// Implementations resolve their underlying widget by index every call so a
// panel re-layout can never leave a dangling pointer.
type barrelBinding interface {
	// labelAt returns the display label offset detents from the current
	// selection (0 == current). Used only for the fade stack render.
	labelAt(offset int) string
	// stepBy moves the current selection by detents (signed). Returns whether
	// the underlying value changed (false at a hard clamp).
	stepBy(detents int) bool
	// canStep reports whether one more step in dir (+1/-1) is possible.
	canStep(dir int) bool
}

// Inertia tunables. Starting values; refined in the screenshot loop (Task 14).
const (
	barrelFriction    = 0.75 // velocity retained per tick
	barrelMinVel      = 0.6  // px/frame below which we settle
	barrelSnapEase    = 0.30 // fraction of remaining offset eased away per tick
	barrelNotchVel    = 9.0  // px/frame impulse per wheel notch
	barrelDragSmooth  = 0.5  // velocity smoothing on drag samples
	barrelMaxVelScale = 3.0  // cap fling velocity at detentPx * this
)

// BarrelWheel is an infinite, inertial scroll wheel: a sub-detent pixel offset
// plus a velocity that decays with friction and snaps to the nearest detent at
// rest. Direction: a DOWNWARD gesture (y increasing / positive notch) advances
// the binding in the +1 direction (see plan Global Constraints).
type BarrelWheel struct {
	b        barrelBinding
	detentPx int

	offsetPx float64 // (-detentPx, detentPx); 0 == centered on current detent
	velocity float64 // px/frame

	dragging  bool
	lastDragY int
	settling  bool
}

func NewBarrelWheel() *BarrelWheel { return &BarrelWheel{detentPx: 1} }

func (w *BarrelWheel) Bind(b barrelBinding, detentPx int) {
	w.b = b
	if detentPx < 1 {
		detentPx = 1
	}
	w.detentPx = detentPx
}

func (w *BarrelWheel) Animating() bool {
	return w.dragging || w.settling || w.velocity != 0 || w.offsetPx != 0
}
func (w *BarrelWheel) OffsetPx() float64 { return w.offsetPx }

func (w *BarrelWheel) BeginDrag(y int) {
	w.dragging = true
	w.settling = false
	w.velocity = 0
	w.lastDragY = y
}

// DragTo shifts by the pointer delta (natural: offset follows the finger) and
// crosses whole detents. Returns whether a detent crossed.
func (w *BarrelWheel) DragTo(y int) bool {
	if !w.dragging {
		return false
	}
	dy := float64(y - w.lastDragY)
	w.lastDragY = y
	// Smooth the velocity estimate so EndDrag flings on recent motion.
	w.velocity = w.velocity*(1-barrelDragSmooth) + dy*barrelDragSmooth
	return w.advance(dy)
}

func (w *BarrelWheel) EndDrag() {
	w.dragging = false
	w.settling = true
	w.clampVel()
}

// ImpulseNotch adds a velocity impulse in dir (+1 down/increase, -1 up).
func (w *BarrelWheel) ImpulseNotch(dir int) bool {
	if dir == 0 {
		return false
	}
	w.settling = true
	w.velocity += float64(sign(dir)) * barrelNotchVel
	w.clampVel()
	return true
}

// StepImmediate performs one synchronous, full-detent step against the bound
// value — for discrete keyboard input, where the caller (and its test)
// expects the underlying value to have already changed by the time this call
// returns, unlike ImpulseNotch's kinetic impulse which only resolves the step
// across subsequent Tick calls. It still seeds a residual pixel offset so the
// fade stack eases back to center like a kinetic step would.
func (w *BarrelWheel) StepImmediate(dir int) bool {
	if w.b == nil || dir == 0 {
		return false
	}
	d := sign(dir)
	if !w.b.canStep(d) {
		return false
	}
	changed := w.b.stepBy(d)
	if changed {
		w.velocity = 0
		w.offsetPx = float64(-d) * float64(w.detentPx)
		w.settling = true
	}
	return changed
}

// Tick integrates one frame of motion. Returns true while still animating.
func (w *BarrelWheel) Tick() bool {
	if w.dragging {
		return true
	}
	if abs(int(w.velocity)) != 0 || w.velocity > barrelMinVel || w.velocity < -barrelMinVel {
		w.advance(w.velocity)
		w.velocity *= barrelFriction
		if w.velocity < barrelMinVel && w.velocity > -barrelMinVel {
			w.velocity = 0
		}
		return true
	}
	// Settle: ease the residual offset to the nearest detent.
	if w.offsetPx != 0 {
		w.offsetPx -= w.offsetPx * barrelSnapEase
		if w.offsetPx < 0.5 && w.offsetPx > -0.5 {
			w.offsetPx = 0
			w.settling = false
		}
		return w.offsetPx != 0
	}
	w.settling = false
	return false
}

// advance shifts offsetPx by dpx and applies whole-detent crossings, honoring
// clamps (a clamp kills velocity and pins offset to 0).
func (w *BarrelWheel) advance(dpx float64) bool {
	if w.b == nil || w.detentPx <= 0 {
		return false
	}
	w.offsetPx += dpx
	crossed := false
	d := float64(w.detentPx)
	for w.offsetPx >= d {
		if !w.b.canStep(+1) {
			w.offsetPx = 0
			w.velocity = 0
			break
		}
		if w.b.stepBy(+1) {
			crossed = true
		}
		w.offsetPx -= d
	}
	for w.offsetPx <= -d {
		if !w.b.canStep(-1) {
			w.offsetPx = 0
			w.velocity = 0
			break
		}
		if w.b.stepBy(-1) {
			crossed = true
		}
		w.offsetPx += d
	}
	return crossed
}

func (w *BarrelWheel) clampVel() {
	max := float64(w.detentPx) * barrelMaxVelScale
	if w.velocity > max {
		w.velocity = max
	}
	if w.velocity < -max {
		w.velocity = -max
	}
}

func sign(n int) int {
	if n > 0 {
		return 1
	}
	if n < 0 {
		return -1
	}
	return 0
}
