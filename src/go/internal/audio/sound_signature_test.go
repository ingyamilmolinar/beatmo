package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Reused for any document with the same signature, recomputed when anything audible moves.
func TestSoundSignatureIsTheSoundNotTheIdentity(t *testing.T) {
	d, ok := EnsureInstrumentDoc(roster.Kick808)
	if !ok {
		t.Fatal("no kick-808 document")
	}
	sig := SoundSignature(d)
	if SoundSignature(d.Clone()) != sig {
		t.Fatal("the signature is not stable for an identical document")
	}
	renamed := d.Clone()
	inst, err := instrument.Instance(roster.Kick808, 2)
	if err != nil {
		t.Fatal(err)
	}
	renamed.ID = inst
	renamed.Name = "My kick"
	renamed.Origin = "user.my-kick.abc123"
	renamed.Notes = "a note"
	if SoundSignature(renamed) != sig {
		t.Fatal("a renamed / re-identified copy of the same sound changed the signature")
	}
	edited, err := applyFieldsForTest(d, map[string]float64{"gen1_kick_pe_amt": 0.5})
	if err != nil {
		t.Fatal(err)
	}
	if SoundSignature(edited) == sig {
		t.Fatal("a changed value kept the signature")
	}
	longer := d.Clone()
	longer.Render.Seconds *= 2
	if SoundSignature(longer) == sig {
		t.Fatal("a changed note length kept the signature")
	}
}

// What the Synth tab learned about the previous circuit's instruments is re-learned for the new ones.
func TestCircuitGenerationAdvancesOnEveryCircuitChange(t *testing.T) {
	g0 := CircuitGeneration()
	snap := CircuitSnapshot()
	t.Cleanup(func() { RestoreCircuit(snap) })
	SetCircuitInstrumentDocs(map[instrument.ID]instrument.Doc{})
	g1 := CircuitGeneration()
	if g1 == g0 {
		t.Fatal("opening a circuit kept the generation")
	}
	RestoreCircuit(snap)
	if CircuitGeneration() == g1 {
		t.Fatal("restoring a circuit (undo/redo) kept the generation")
	}
	SetCircuitInstrumentName(roster.Kick808, "Boom")
	g2 := CircuitGeneration()
	SetCircuitInstrumentName(roster.Kick808, "Boom 2")
	if CircuitGeneration() != g2 {
		t.Fatal("naming a row is not a new circuit")
	}
}

// applyFieldsForTest is ApplyInstrumentFields without the skipped list.
func applyFieldsForTest(d instrument.Doc, f map[string]float64) (instrument.Doc, error) {
	out, _, err := ApplyInstrumentFields(d, f)
	return out, err
}
