package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// --- helpers ---------------------------------------------------------------

// openContinuousWheel opens a wheel over a continuous Hz param mid-range.
func openContinuousWheel(t *testing.T) (*MobileWheelPopup, *Knob) {
	t.Helper()
	def := audio.ParamDef{Name: "cutoff", Label: "Cutoff", Min: 20, Max: 16000, Unit: "Hz"}
	k := &Knob{Endless: true, StepMul: 50, Value: 0.5}
	k.Scale = KnobScale{Min: def.Min, Max: def.Max, Unit: def.Unit}
	b := NewKnobStepBadge(def)
	b.SetStep(50)
	k.StepMul = b.Step()
	w := NewMobileWheelPopup()
	w.Open(WheelBinding{Knob: k, Badge: b, Def: def,
		Title: func() string { return "Cutoff" },
	}, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 800, 600), 0)
	return w, k
}

// openEnumWheel opens a wheel over an FM-algorithm-style enum param.
func openEnumWheel(t *testing.T, onCommit func()) (*MobileWheelPopup, *Knob, audio.ParamDef) {
	t.Helper()
	def := audio.ParamDef{Name: "fm_algorithm", Label: "Algorithm", Min: 0, Max: 3,
		Enum: []string{"Stack", "Pair", "Bright", "Parallel"}}
	k := &Knob{Discrete: true, Value: 1.0 / 3.0} // index 1 of 4
	k.Scale = KnobScale{Min: def.Min, Max: def.Max, Enum: def.Enum}
	w := NewMobileWheelPopup()
	w.Open(WheelBinding{Knob: k, Def: def, Discrete: true,
		Title:    func() string { return "Algorithm" },
		OnCommit: onCommit,
	}, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 800, 600), 0)
	return w, k, def
}

// tapAt drives a full press+release at (x, y) with no movement — a click.
func tapAt(w *MobileWheelPopup, x, y int) {
	w.HandleInput(x, y, true)
	w.HandleInput(x, y, false)
}

// Positive n = rows ABOVE the centre selector, which show LARGER values.
func barrelRowPoint(w *MobileWheelPopup, n int) (int, int) {
	cx := (w.valRect.Min.X + w.valRect.Max.X) / 2
	cy := (w.valRect.Min.Y + w.valRect.Max.Y) / 2
	return cx, cy - n*w.tickGap()
}

// --- row click-to-select ----------------------------------------------------

// Desktop users click a value directly instead of scrolling to it.
func TestWheelBarrelRowClickSelectsLargerValue(t *testing.T) {
	w, k := openContinuousWheel(t)
	before := k.Value
	x, y := barrelRowPoint(w, 1)
	tapAt(w, x, y)
	if k.Value <= before {
		t.Fatalf("clicking the row above center must select the larger value: %v -> %v", before, k.Value)
	}
	if !w.IsOpen() {
		t.Fatal("row click selects the value but keeps the wheel open")
	}
	if !w.dirty {
		t.Fatal("row click is a live edit — dirty must be set so Accept commits it")
	}
}

// Clicking a row below the center selects the smaller value.
func TestWheelBarrelRowClickSelectsSmallerValue(t *testing.T) {
	w, k := openContinuousWheel(t)
	before := k.Value
	x, y := barrelRowPoint(w, -1)
	tapAt(w, x, y)
	if k.Value >= before {
		t.Fatalf("clicking the row below center must select the smaller value: %v -> %v", before, k.Value)
	}
}

// Enum barrels (FM algorithm & friends) select the clicked item exactly.
func TestWheelBarrelRowClickSelectsEnumItem(t *testing.T) {
	w, k, def := openEnumWheel(t, nil)
	x, y := barrelRowPoint(w, 1) // one row above = next item up
	tapAt(w, x, y)
	got := enumIndex(k.Value, len(def.Enum))
	if got != 2 {
		t.Fatalf("clicking the enum row above center must select index 2, got %d", got)
	}
	if !w.IsOpen() {
		t.Fatal("enum row click keeps the wheel open")
	}
}

func TestWheelBarrelDragDoesNotRowJumpOnRelease(t *testing.T) {
	w, k := openContinuousWheel(t)
	x, y := barrelRowPoint(w, 1)
	w.HandleInput(x, y, true)
	// Drag DOWN two full tick gaps (down = increase, per the pinned direction).
	gap := w.tickGap()
	w.HandleInput(x, y+gap, true)
	w.HandleInput(x, y+2*gap, true)
	afterDrag := k.Value
	w.HandleInput(x, y+2*gap, false) // release
	if k.Value != afterDrag {
		t.Fatalf("release after a real drag must not apply a row jump: %v -> %v", afterDrag, k.Value)
	}
}

// --- center click accepts + closes -----------------------------------------

// Clicking the middle confirms, on discrete wheels as on continuous ones.
func TestWheelCenterClickAcceptsAndClosesDiscrete(t *testing.T) {
	w, k, def := openEnumWheel(t, nil)
	before := enumIndex(k.Value, len(def.Enum))
	cx := (w.centerH.Min.X + w.centerH.Max.X) / 2
	cy := (w.centerH.Min.Y + w.centerH.Max.Y) / 2
	tapAt(w, cx, cy)
	if w.IsOpen() {
		t.Fatal("center click on a discrete wheel must close it (select the shown value)")
	}
	if got := enumIndex(k.Value, len(def.Enum)); got != before {
		t.Fatalf("center click must keep the shown value: %d -> %d", before, got)
	}
}

// A dirty discrete edit commits exactly once when the center is clicked.
func TestWheelCenterClickCommitsDirtyDiscreteOnce(t *testing.T) {
	commits := 0
	w, _, _ := openEnumWheel(t, func() { commits++ })
	// Make it dirty via a row click, then confirm via center click.
	x, y := barrelRowPoint(w, 1)
	tapAt(w, x, y)
	cx := (w.centerH.Min.X + w.centerH.Max.X) / 2
	cy := (w.centerH.Min.Y + w.centerH.Max.Y) / 2
	tapAt(w, cx, cy)
	if w.IsOpen() {
		t.Fatal("center click must close the discrete wheel")
	}
	if commits != 1 {
		t.Fatalf("center click on a dirty discrete wheel must commit exactly once, got %d", commits)
	}
}

// A centre click confirms: accept (committing once when dirty) and close.
func TestWheelCenterClickContinuousConfirms(t *testing.T) {
	w, _ := openContinuousWheel(t)
	commits := 0
	w.binding.OnCommit = func() { commits++ }
	w.StepValue(+1)
	cx := (w.centerH.Min.X + w.centerH.Max.X) / 2
	cy := (w.centerH.Min.Y + w.centerH.Max.Y) / 2
	tapAt(w, cx, cy)
	if w.IsOpen() {
		t.Fatal("continuous center click must close the wheel")
	}
	if commits != 1 {
		t.Fatalf("continuous center click on a dirty wheel must commit exactly once, got %d", commits)
	}
}
