//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// These tests pin the transactional Enter/Esc contract for the shared
// MobileWheelPopup (synth, sampler, EQ):
//   - live preview via OnChange while turning the wheel,
//   - the undo commit is DEFERRED (not per gesture/detent) until the edit is
//     Accepted (Enter, tap-away, navigate-away),
//   - Esc Cancels: restore the value snapshotted at Open, re-apply via OnChange,
//     and do NOT commit.
// Because all three wheels share this type, fixing it here makes them behave
// identically; TestAllControlWheels_AcceptCancelParity asserts that.

// dragWheelValue presses inside the value barrel BELOW the center box (so the
// press never lands on centerH, which would open the numeric editor on a real
// wheel whose OpenEditor is non-nil) and drags up by dpx, changing the value.
func dragWheelValue(w *MobileWheelPopup, dpx int) {
	cx := w.valRect.Min.X + 4
	y := w.valRect.Max.Y - 6
	w.HandleInput(cx, y, true)
	w.HandleInput(cx, y-dpx, true)
	w.HandleInput(cx, y-dpx, false)
}

func openTestWheel(k *Knob) (*MobileWheelPopup, *int) {
	b, commits := wheelTestBinding(k)
	w := NewMobileWheelPopup()
	w.Open(*b, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)
	return w, commits
}

// A drag changes the value live but must NOT commit; the commit is deferred
// until the edit is Accepted.
func TestWheelPopup_CommitDeferredUntilAccept(t *testing.T) {
	k := newEndlessKnobForTest(0, 200, 1.0)
	w, commits := openTestWheel(k)
	v0 := realValue(k)
	dragWheelValue(w, knobEndlessPxPerNotch*10)
	if realValue(k) == v0 {
		t.Fatalf("drag did not change value")
	}
	if *commits != 0 {
		t.Fatalf("commit must be deferred; got %d on release", *commits)
	}
	w.Accept()
	if *commits != 1 {
		t.Fatalf("Accept must commit exactly once; got %d", *commits)
	}
	if w.IsOpen() {
		t.Fatalf("Accept must close the popup")
	}
}

// Esc/Cancel reverts the value to the snapshot taken at Open, re-applies it via
// OnChange, and does not commit.
func TestWheelPopup_CancelRevertsToSnapshot(t *testing.T) {
	k := newEndlessKnobForTest(0, 200, 1.0)
	changes := 0
	applied := realValue(k)
	b := WheelBinding{
		Knob:     k,
		Def:      audio.ParamDef{Name: "test", Min: 0, Max: 200},
		OnChange: func() { changes++; applied = realValue(k) },
		OnCommit: func() {},
		Title:    func() string { return "Test" },
	}
	w := NewMobileWheelPopup()
	w.Open(b, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)

	v0 := k.Value
	a0 := realValue(k)
	dragWheelValue(w, knobEndlessPxPerNotch*10)
	if k.Value == v0 {
		t.Fatalf("drag did not change value")
	}
	changesBefore := changes
	w.Cancel()
	if k.Value != v0 {
		t.Fatalf("Cancel did not restore knob value: %v vs %v", k.Value, v0)
	}
	if changes <= changesBefore {
		t.Fatalf("Cancel did not re-apply the reverted value via OnChange")
	}
	if applied != a0 {
		t.Fatalf("Cancel re-applied the wrong value: %v want %v", applied, a0)
	}
	if w.IsOpen() {
		t.Fatalf("Cancel must close the popup")
	}
}

// Accept/Cancel with no change are no-ops (no spurious commit).
func TestWheelPopup_NotDirtyAcceptCancelNoop(t *testing.T) {
	k := newEndlessKnobForTest(0, 200, 1.0)
	w, commits := openTestWheel(k)
	w.Accept()
	if *commits != 0 {
		t.Fatalf("Accept committed with no change; got %d", *commits)
	}

	k2 := newEndlessKnobForTest(0, 200, 1.0)
	w2, commits2 := openTestWheel(k2)
	v0 := k2.Value
	w2.Cancel()
	if *commits2 != 0 {
		t.Fatalf("Cancel committed with no change")
	}
	if k2.Value != v0 {
		t.Fatalf("Cancel moved value when not dirty")
	}
}

// The portal overlay routes Esc -> Cancel (returning false so handleEscape's
// CloseTop still removes the entry) and Enter -> Accept.
func TestWheelPopupOverlay_KeyHandlers(t *testing.T) {
	k := newEndlessKnobForTest(0, 200, 1.0)
	w, commits := openTestWheel(k)
	o := &mobileWheelPopupPortalOverlay{popup: w, tag: "t"}
	var _ portalEscapeHandler = o
	var _ portalEnterHandler = o

	v0 := k.Value
	dragWheelValue(w, knobEndlessPxPerNotch*10)
	if got := o.HandleEscape(); got != false {
		t.Fatalf("HandleEscape must return false to let CloseTop run; got %v", got)
	}
	if k.Value != v0 || *commits != 0 {
		t.Fatalf("Esc did not cancel: value=%v commits=%d", k.Value, *commits)
	}

	k2 := newEndlessKnobForTest(0, 200, 1.0)
	w2, commits2 := openTestWheel(k2)
	o2 := &mobileWheelPopupPortalOverlay{popup: w2, tag: "t"}
	dragWheelValue(w2, knobEndlessPxPerNotch*10)
	changed := w2.binding.Knob.Value
	if got := o2.HandleEnter(); got != true {
		t.Fatalf("HandleEnter must return true; got %v", got)
	}
	if *commits2 != 1 || w2.IsOpen() {
		t.Fatalf("Enter did not accept: commits=%d open=%v", *commits2, w2.IsOpen())
	}
	if w2.binding.Knob.Value != changed {
		t.Fatalf("Enter changed the value")
	}
}

// Game-level Enter/Esc routing on a live EQ wheel: Esc reverts the source of
// truth (bandGainsDB), Enter persists it.
func TestWheelPopup_GameRouting_EQ(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	setupEQTabForTest(g, 1200, 800)
	dv := g.drum

	changeWheel := func() float64 {
		dragWheelValue(dv.eqWheelPopup, knobEndlessPxPerNotch*8)
		return dv.eqPanelZone.bandGainsDB[0]
	}

	// Esc reverts.
	dv.openEQKnobWheelPopup(0)
	before := dv.eqPanelZone.bandGainsDB[0]
	if changeWheel() == before {
		t.Fatalf("wheel drag did not change bandGainsDB")
	}
	r := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
	g.handleGlobalShortcuts()
	r()
	if dv.eqWheelPopup.IsOpen() {
		t.Fatalf("Esc did not close the EQ wheel")
	}
	if dv.eqPanelZone.bandGainsDB[0] != before {
		t.Fatalf("Esc did not revert bandGainsDB: %v vs %v", dv.eqPanelZone.bandGainsDB[0], before)
	}

	// Enter persists.
	dv.openEQKnobWheelPopup(0)
	before2 := dv.eqPanelZone.bandGainsDB[0]
	changed := changeWheel()
	if changed == before2 {
		t.Fatalf("wheel drag did not change bandGainsDB (enter case)")
	}
	r = stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEnter: true})
	g.handleGlobalShortcuts()
	r()
	if dv.eqWheelPopup.IsOpen() {
		t.Fatalf("Enter did not close the EQ wheel")
	}
	if dv.eqPanelZone.bandGainsDB[0] != changed {
		t.Fatalf("Enter did not persist bandGainsDB: %v vs %v", dv.eqPanelZone.bandGainsDB[0], changed)
	}
}

// Tap-outside dismissal (a modal popup's click-outside routes through the tree
// to portal.CloseTop → the entry's OnClose) must PERSIST the edit: value kept
// AND exactly one undo step recorded. Regression for the OnClose→Close bug that
// dropped the deferred commit on the most common dismissal.
func TestWheelPopup_TapOutsidePersists_EQ(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	setupEQTabForTest(g, 1200, 800)
	dv := g.drum

	dv.openEQKnobWheelPopup(0)
	g.undoManager.OnExternalLoad()
	depth0 := len(g.undoManager.undo)
	before := dv.eqPanelZone.bandGainsDB[0]
	dragWheelValue(dv.eqWheelPopup, knobEndlessPxPerNotch*8)
	changed := dv.eqPanelZone.bandGainsDB[0]
	if changed == before {
		t.Fatalf("drag did not change bandGainsDB")
	}

	dv.portal().CloseTop() // the tree's click-outside dismissal for a modal popup

	if dv.eqWheelPopup.IsOpen() {
		t.Fatalf("tap-outside did not close the wheel")
	}
	if dv.eqPanelZone.bandGainsDB[0] != changed {
		t.Fatalf("tap-outside must persist the value: %v vs %v", dv.eqPanelZone.bandGainsDB[0], changed)
	}
	if steps := len(g.undoManager.undo) - depth0; steps != 1 {
		t.Fatalf("tap-outside must commit exactly one undo step, got %d", steps)
	}
}

// Navigate-away (CloseAllPopups drains the portal via CloseTop) must PERSIST the
// edit — value kept AND one undo step recorded.
func TestWheelPopup_NavigateAwayPersists_EQ(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	setupEQTabForTest(g, 1200, 800)
	dv := g.drum

	dv.openEQKnobWheelPopup(0)
	g.undoManager.OnExternalLoad()
	depth0 := len(g.undoManager.undo)
	before := dv.eqPanelZone.bandGainsDB[0]
	dragWheelValue(dv.eqWheelPopup, knobEndlessPxPerNotch*8)
	changed := dv.eqPanelZone.bandGainsDB[0]
	if changed == before {
		t.Fatalf("drag did not change bandGainsDB")
	}

	dv.CloseAllPopups()

	if dv.eqWheelPopup.IsOpen() {
		t.Fatalf("CloseAllPopups did not close the wheel")
	}
	if dv.eqPanelZone.bandGainsDB[0] != changed {
		t.Fatalf("navigate-away must persist the value: %v vs %v", dv.eqPanelZone.bandGainsDB[0], changed)
	}
	if steps := len(g.undoManager.undo) - depth0; steps != 1 {
		t.Fatalf("navigate-away must commit exactly one undo step, got %d", steps)
	}
}

// Discrete/enum knobs revert on Cancel too (M1): the restore is not continuous-only.
func TestWheelPopup_DiscreteCancelReverts(t *testing.T) {
	k := NewKnob(1.0) // start near the top enum index so an up-drag can move down
	k.Scale = KnobScale{Min: 0, Max: 3, Enum: []string{"a", "b", "c", "d"}}
	changes := 0
	b := WheelBinding{
		Knob:     k,
		Def:      audio.ParamDef{Name: "wave", Min: 0, Max: 3, Enum: []string{"a", "b", "c", "d"}},
		Discrete: true,
		OnChange: func() { changes++ },
		OnCommit: func() {},
		Title:    func() string { return "Wave" },
	}
	w := NewMobileWheelPopup()
	w.Open(b, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)
	v0 := k.Value
	dragWheelValue(w, knobEndlessPxPerNotch*20) // advance the enum index
	if k.Value == v0 {
		t.Fatalf("discrete drag did not change the enum index")
	}
	w.Cancel()
	if k.Value != v0 {
		t.Fatalf("discrete Cancel did not restore the enum index: %v vs %v", k.Value, v0)
	}
}

// All three control wheels accept user input the exact same way: a change then
// Cancel reverts to the snapshot; a change then Accept keeps it and closes.
func TestAllControlWheels_AcceptCancelParity(t *testing.T) {
	type wheelCase struct {
		name string
		open func(t *testing.T) *MobileWheelPopup
	}
	cases := []wheelCase{
		{"eq", func(t *testing.T) *MobileWheelPopup {
			g := New(testLogger)
			t.Cleanup(g.CloseForTest)
			setupEQTabForTest(g, 1200, 800)
			g.drum.openEQKnobWheelPopup(0)
			return g.drum.eqWheelPopup
		}},
		{"synth", func(t *testing.T) *MobileWheelPopup {
			g := New(testLogger)
			t.Cleanup(g.CloseForTest)
			mobileSynthWheelSceneSetup()(g)
			return g.drum.synthWheelPopup
		}},
		{"sampler", func(t *testing.T) *MobileWheelPopup {
			g, _ := newMobileSamplerWheelGame(t)
			g.drum.openSamplerKnobWheelPopup(samplerKnobGain)
			return g.drum.samplerWheelPopup
		}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name+"/cancel", func(t *testing.T) {
			w := tc.open(t)
			if w == nil || !w.IsOpen() {
				t.Fatalf("%s wheel not open", tc.name)
			}
			v0 := w.binding.Knob.Value
			dragWheelValue(w, knobEndlessPxPerNotch*8)
			if w.binding.Knob.Value == v0 {
				t.Fatalf("%s: value did not change on drag", tc.name)
			}
			w.Cancel()
			if w.IsOpen() {
				t.Fatalf("%s: Cancel did not close", tc.name)
			}
			if w.binding.Knob.Value != v0 {
				t.Fatalf("%s: Cancel did not revert: %v vs %v", tc.name, w.binding.Knob.Value, v0)
			}
		})
		t.Run(tc.name+"/accept", func(t *testing.T) {
			w := tc.open(t)
			if w == nil || !w.IsOpen() {
				t.Fatalf("%s wheel not open", tc.name)
			}
			v0 := w.binding.Knob.Value
			dragWheelValue(w, knobEndlessPxPerNotch*8)
			changed := w.binding.Knob.Value
			if changed == v0 {
				t.Fatalf("%s: value did not change on drag", tc.name)
			}
			w.Accept()
			if w.IsOpen() {
				t.Fatalf("%s: Accept did not close", tc.name)
			}
			if w.binding.Knob.Value != changed {
				t.Fatalf("%s: Accept changed the value", tc.name)
			}
		})
	}
}
