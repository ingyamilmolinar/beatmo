//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Tapping the title cycles every view; production has no picker.
func TestDeveloperViewPickerCyclesTheFocusedControlsView(t *testing.T) {
	if !devToolsBuild {
		t.Skip("release build")
	}
	g := newDialGame(t)
	dv := g.drum
	inst := instrument.ID(dialInst(g))
	c := dv.instEditorMacroCards[1]
	dragFrom(g, centre(c.dialRects[0]), -10)
	before := dv.synp().synthGraphFocus(inst)
	if before.ctl == "" {
		t.Fatal("the dial press did not focus the graph")
	}
	pick := synthGraphPickRect(dv.synthGraphPane())
	seen := map[synthView]bool{before.view: true}
	for i := 0; i < len(synthGraphPickOrder); i++ {
		tapAtPoint(g, centre(pick))
		seen[dv.synp().synthGraphFocus(inst).view] = true
	}
	for _, v := range synthGraphPickOrder {
		if !seen[v] {
			t.Errorf("cycling the picker never showed view %v", v)
		}
	}
}

func TestProductionHasNoViewPicker(t *testing.T) {
	forceProductionUIForTest(t)
	g := newDialGame(t)
	dv := g.drum
	inst := instrument.ID(dialInst(g))
	c := dv.instEditorMacroCards[1]
	dragFrom(g, centre(c.dialRects[0]), -10)
	if hits := dv.synp().synthGraphPickHitAreas(inst, 10); len(hits) != 0 {
		t.Fatal("the production UI publishes a view picker")
	}
	want := dv.synp().synthGraphFocus(inst).view
	tapAtPoint(g, centre(synthGraphPickRect(dv.synthGraphPane())))
	if got := dv.synp().synthGraphFocus(inst).view; got != want {
		t.Fatalf("a tap on the graph title changed the production view %v -> %v", want, got)
	}
}
