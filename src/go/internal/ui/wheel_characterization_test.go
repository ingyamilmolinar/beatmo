//go:build test

package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Captured before the WheelModel / ValueWheel extraction and replayed identically after it: every drag notch,
// scroll, row click, rung tap, key, Esc restore and Enter commit leaves the knob, its resolution, the popup and the
// commit count where the old code did. Regenerate only for an INTENDED change: WHEEL_TAPE_UPDATE=1.

type wheelTapeFrame struct {
	Op      string `json:"op"`
	Value   string `json:"value"`
	StepMul string `json:"step_mul"`
	Rung    int    `json:"rung"`
	Open    bool   `json:"open"`
	Commits int    `json:"commits"`
}

const wheelTapeGolden = "testdata/wheel_characterization.golden.json"

func TestWheelCharacterizationTape(t *testing.T) {
	got := map[string][]wheelTapeFrame{}

	// Sampler: Start (idx 0, percent) and Transpose (semitones), mobile profile.
	for _, idx := range []int{samplerKnobStart, samplerKnobTranspose} {
		t.Run(fmt.Sprintf("sampler_%d", idx), func(t *testing.T) {
			g, _ := newMobileSamplerWheelGame(t)
			dv := g.drum
			open := func() {
				dv.openSamplerKnobWheelPopup(idx)
			}
			k := dv.sampler.knobs[idx]
			badge := dv.sampler.knobStepBadges[idx]
			got[fmt.Sprintf("sampler_%d", idx)] = runWheelTape(t, g, dv.samplerWheelPopup, open,
				func() float64 { return k.Value }, func() float64 { return k.StepMul },
				func() int { return badge.Index() })
		})
	}

	// EQ band 3, desktop profile.
	t.Run("eq_3", func(t *testing.T) {
		assertDefaultParityState(t)
		g := New(game_log.New(nil, game_log.LevelError))
		t.Cleanup(g.CloseForTest)
		g.Layout(1280, 720)
		dv := g.drum
		open := func() { dv.openEQKnobWheelPopup(3) }
		got["eq_3"] = runWheelTape(t, g, dv.eqWheelPopup, open,
			func() float64 { return dv.eqPanelZone.bandGainsDB[3] },
			func() float64 {
				if k := dv.eqWheelPopup.binding.Knob; k != nil {
					return k.StepMul
				}
				return 0
			},
			func() int { return dv.eqWheelStepBadge.Index() })
	})

	if os.Getenv("WHEEL_TAPE_UPDATE") == "1" {
		b, err := json.MarshalIndent(got, "", " ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(wheelTapeGolden), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(wheelTapeGolden, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", wheelTapeGolden)
		return
	}
	raw, err := os.ReadFile(wheelTapeGolden)
	if err != nil {
		t.Fatalf("read golden (regenerate with WHEEL_TAPE_UPDATE=1): %v", err)
	}
	var want map[string][]wheelTapeFrame
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	for name, wf := range want {
		gf := got[name]
		if len(gf) != len(wf) {
			t.Errorf("%s: %d frames, golden has %d", name, len(gf), len(wf))
			continue
		}
		for i := range wf {
			if gf[i] != wf[i] {
				t.Errorf("%s frame %d (%s): got %+v want %+v", name, i, wf[i].Op, gf[i], wf[i])
			}
		}
	}
	if len(got) != len(want) {
		t.Errorf("tape set changed: got %d tapes, golden %d", len(got), len(want))
	}
}

func runWheelTape(t *testing.T, g *Game, w *MobileWheelPopup, open func(),
	value, stepMul func() float64, rung func() int) []wheelTapeFrame {
	t.Helper()
	commits := 0
	reopen := func() {
		open()
		if !w.IsOpen() {
			t.Fatalf("wheel did not open")
		}
		orig := w.binding.OnCommit
		w.binding.OnCommit = func() {
			commits++
			if orig != nil {
				orig()
			}
		}
	}
	var frames []wheelTapeFrame
	rec := func(op string) {
		frames = append(frames, wheelTapeFrame{
			Op: op, Value: fmt.Sprintf("%.12g", value()), StepMul: fmt.Sprintf("%.12g", stepMul()),
			Rung: rung(), Open: w.IsOpen(), Commits: commits,
		})
	}
	h := &mobileWheelHitHandler{popup: w}
	// Left of the barrel, below the centre box, so a press never lands on the centre grip.
	bx := func() int { return w.valRect.Min.X + 4 }
	by := func() int { return w.valRect.Max.Y - 6 }
	gap := w.tickGap()

	reopen()
	rec("open")

	// Drag three notches down (finger down = value up).
	x, y := bx(), by()
	h.OnPress(x, y-3*gap)
	h.OnDrag(x, y-3*gap+gap*3/2)
	h.OnDrag(x, y)
	h.OnRelease(x, y)
	rec("drag-3-down")

	// Click the row two above centre (no movement = click).
	cy := (w.valRect.Min.Y + w.valRect.Max.Y) / 2
	h.OnPress(bx(), cy-2*gap)
	h.OnRelease(bx(), cy-2*gap)
	rec("click-row+2")

	// Seven scroll events down over the barrel (3 events per notch).
	for i := 0; i < 7; i++ {
		h.OnWheel(bx(), cy, 1)
	}
	rec("wheel-7-down")

	if !w.resRect.Empty() {
		rx := (w.resRect.Min.X + w.resRect.Max.X) / 2
		ry := (w.resRect.Min.Y + w.resRect.Max.Y) / 2
		for i := 0; i < 3; i++ {
			h.OnWheel(rx, ry, 1)
		}
		rec("wheel-3-strip")
		if rr := w.rungRects(); len(rr) > 1 {
			p := rr[1].Min.Add(rr[1].Max).Div(2)
			h.OnPress(p.X, p.Y)
			h.OnRelease(p.X, p.Y)
			rec("rung-tap-1")
		}
	}

	pressKeyThroughShortcuts(g, ebiten.KeyArrowUp)
	pressKeyThroughShortcuts(g, ebiten.KeyArrowUp)
	rec("key-up-2")
	pressKeyThroughShortcuts(g, ebiten.KeyArrowRight)
	rec("key-right")

	// Shift + three scroll events: one notch at the next-finer rung.
	withStubbedKeys([]ebiten.Key{ebiten.KeyShiftLeft}, nil, func() {
		for i := 0; i < 3; i++ {
			h.OnWheel(bx(), cy, 1)
		}
	})
	rec("shift-wheel-3")

	w.Cancel() // Esc path: restore the snapshot, no commit
	rec("esc")

	reopen()
	rec("reopen")
	pressKeyThroughShortcuts(g, ebiten.KeyArrowDown)
	rec("key-down")
	w.Accept() // Enter path: commit once
	rec("enter")
	return frames
}
