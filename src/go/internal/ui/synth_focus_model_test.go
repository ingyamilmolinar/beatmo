//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// One focused dial at a time, the one the graph explains: pressing a dial focuses it, a production card tap its first
// dial, and a focus never outlives its instrument or card set.

func TestPressingADialFocusesIt(t *testing.T) {
	for _, prod := range []bool{false, true} {
		name := "developer"
		if prod {
			name = "production"
		}
		t.Run(name, func(t *testing.T) {
			if prod {
				forceProductionUIForTest(t)
			}
			g := newDialGame(t)
			dv := g.drum
			inst := instrument.ID(dialInst(g))
			c := dv.instEditorMacroCards[1]
			if c.nDials == 0 {
				t.Fatalf("card %q has no dial", c.id)
			}
			dragFrom(g, centre(c.dialRects[0]), -24)
			card, dial, ok := dv.synthFocusedDial(inst)
			if !ok || card != c.id || dial != c.dials[0].id {
				t.Fatalf("after dragging %s/%s the focus is (%q, %q, %v)", c.id, c.dials[0].id, card, dial, ok)
			}
		})
	}
}

func TestProductionCardTapFocusesItsFirstDial(t *testing.T) {
	forceProductionUIForTest(t)
	g := newDialGame(t)
	dv := g.drum
	inst := instrument.ID(dialInst(g))
	c := dv.instEditorMacroCards[2]
	tapAtPoint(g, image.Pt(c.r.Min.X+macroCardInset()+2, c.r.Min.Y+macroCardInset()+2))
	card, dial, ok := dv.synthFocusedDial(inst)
	if !ok || card != c.id || dial != c.dials[0].id {
		t.Fatalf("a tap on card %q focused (%q, %q, %v), want its first dial %q", c.id, card, dial, ok, c.dials[0].id)
	}
	if !dv.synthAtMacroRoot(inst) {
		t.Fatal("the focusing tap drilled")
	}
}

func TestFocusDoesNotOutliveItsInstrument(t *testing.T) {
	g := newDialGame(t)
	dv := g.drum
	inst := instrument.ID(dialInst(g))
	c := dv.instEditorMacroCards[0]
	dv.setSynthFocusDial(inst, c.id, c.dials[0].id)
	if _, _, ok := dv.synthFocusedDial(inst); !ok {
		t.Fatal("setSynthFocusDial did not focus")
	}
	// A kick has no melodic "sound" card, so the focus must not carry over.
	dv.Rows[0].Instrument = roster.Kick808
	t.Cleanup(func() { audio.ResetInstrumentParams(roster.Kick808) })
	dv.eqPanelZone.SetActiveChannel(channel.Of(roster.Kick808))
	dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
	idleFrame(g)
	kick := instrument.ID(dialInst(g))
	if card, dial, ok := dv.synthFocusedDial(kick); ok {
		t.Fatalf("the violin's focus (%s/%s) carried onto %s as (%q, %q)", c.id, c.dials[0].id, kick, card, dial)
	}
	dv.clearSynthFocus()
	if _, _, ok := dv.synthFocusedDial(inst); ok {
		t.Fatal("clearSynthFocus left a focus")
	}
}

// Esc or an empty tap drops back to the overview, and the Esc does not Stop.
func TestFocusClearsOnEscAndOnAnEmptyTap(t *testing.T) {
	for _, prod := range []bool{false, true} {
		name := map[bool]string{false: "developer", true: "production"}[prod]
		t.Run(name, func(t *testing.T) {
			if prod {
				forceProductionUIForTest(t)
			}
			g := newDialGame(t)
			dv := g.drum
			inst := instrument.ID(dialInst(g))
			focus := func() {
				c := dv.instEditorMacroCards[0]
				dragFrom(g, centre(c.dialRects[0]), -10)
				if _, _, ok := dv.synthFocusedDial(inst); !ok {
					t.Fatal("precondition: the dial press did not focus")
				}
			}
			focus()
			keyTap(g, ebiten.KeyEscape)
			if _, _, ok := dv.synthFocusedDial(inst); ok {
				t.Fatal("Esc must drop the Synth graph's focus")
			}
			if dv.StopPressed() {
				t.Fatal("the Esc that dropped the focus must not also Stop")
			}
			focus()
			r := dv.instEditorMacroDetailR
			empty := image.Pt(r.Min.X+12, r.Max.Y-8)
			for _, c := range dv.instEditorMacroCards {
				if empty.In(c.r) {
					t.Skipf("no empty card space at %v (cards fill the area)", empty)
				}
			}
			tapAtPoint(g, empty)
			if _, _, ok := dv.synthFocusedDial(inst); ok {
				t.Fatal("a tap on empty card space must drop the focus")
			}
		})
	}
}
