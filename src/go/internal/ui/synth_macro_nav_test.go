package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestSynthMacroNav_RootAndDrill(t *testing.T) {
	dv := &DrumView{}
	const id = roster.Violin
	if !dv.synthAtMacroRoot(id) {
		t.Fatal("fresh instrument should start at the Basic root")
	}
	if got := dv.synthMacroOpen(id); got != "" {
		t.Fatalf("root open id = %q, want empty", got)
	}
	dv.setSynthMacroOpen(id, "brightness")
	if dv.synthAtMacroRoot(id) {
		t.Error("after drilling, should not be at root")
	}
	if got := dv.synthMacroOpen(id); got != "brightness" {
		t.Errorf("open id = %q, want brightness", got)
	}
	// A second instrument stays independent.
	if !dv.synthAtMacroRoot(roster.Cello) {
		t.Error("unrelated instrument should still be at root")
	}
	dv.setSynthMacroOpen(id, "")
	if !dv.synthAtMacroRoot(id) {
		t.Error("Basic should return to root")
	}
}
