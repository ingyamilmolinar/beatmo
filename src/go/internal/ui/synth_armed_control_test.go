package ui

import "testing"

func TestArmSynthControl_OneAtATime(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()

	if _, armed := dv.synthControlArmed(inst); armed {
		t.Fatal("nothing should be armed initially")
	}
	dv.armSynthControl(inst, 2)
	idx, armed := dv.synthControlArmed(inst)
	if !armed || idx != 2 {
		t.Fatalf("expected armed idx 2, got idx=%d armed=%v", idx, armed)
	}
	// Arming a second control replaces the first.
	dv.armSynthControl(inst, 5)
	idx, armed = dv.synthControlArmed(inst)
	if !armed || idx != 5 {
		t.Fatalf("expected armed idx 5, got idx=%d armed=%v", idx, armed)
	}
	// Arming also selects for the focus graph.
	if got := dv.instEditorSelectedControl[inst]; got != 5 {
		t.Fatalf("arm should select: selected=%d", got)
	}
}

func TestToggleSynthControlArm_LocksOnSecondTap(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()
	dv.toggleSynthControlArm(inst, 3) // arm
	if idx, armed := dv.synthControlArmed(inst); !armed || idx != 3 {
		t.Fatalf("first toggle should arm 3, got %d/%v", idx, armed)
	}
	dv.toggleSynthControlArm(inst, 3) // lock
	if _, armed := dv.synthControlArmed(inst); armed {
		t.Fatal("second toggle on same idx should lock")
	}
	// Toggling a different idx while armed re-arms it.
	dv.toggleSynthControlArm(inst, 3)
	dv.toggleSynthControlArm(inst, 4)
	if idx, armed := dv.synthControlArmed(inst); !armed || idx != 4 {
		t.Fatalf("toggle other idx should arm it, got %d/%v", idx, armed)
	}
}
