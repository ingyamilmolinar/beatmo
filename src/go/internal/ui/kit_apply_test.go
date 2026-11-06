package ui

import (
	"sync"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestApplyKitToDrumView_PreservesMixState is the Phase 6 ship signal:
// applying a kit must swap each row's Instrument and leave every other piece
// of the row's mix state as it was. Volume, Mute / Solo stay on the row; the
// EQ, HPF/LPF and insert effects are the instrument's audio bundle, which
// carries over to the kit's instrument (it sets none of its own — spec
// 2026-10-31 §2.9: choosing an instrument never drops settings). Built without
// a real Game (no Ebiten); the test only needs the DrumView struct's row slice
// and the kit_binder bridge.
func TestApplyKitToDrumView_PreservesMixState(t *testing.T) {
	// Capture EventKitApplied for the hook assertion.
	var mu sync.Mutex
	var seen []hooks.Event
	unsub := hooks.Subscribe(hooks.EventKitApplied, func(e hooks.Event) {
		mu.Lock()
		seen = append(seen, e)
		mu.Unlock()
	})
	t.Cleanup(unsub)

	eq := audio.NewEQSettings()
	copy(eq.GainsDB, []float64{0, +1.5, -2.0, 0, +0.5, 0, 0, 0, 0, 0})
	eq.BandMuted[2] = true
	eq.HPF, eq.HPFHz = true, 120
	eq.LPF, eq.LPFHz = true, 8000
	settings := audio.AudioBundle{
		EQ:      eq,
		Effects: []audio.EffectSlot{{Type: audio.EffectDistortion, Enabled: true, Params: map[string]float64{"drive": 0.6}}},
	}
	for _, id := range []instrument.ID{roster.Snare, roster.SnareLofi, roster.Kick, roster.KickDeep} {
		audio.ApplyAudioBundle(id, audio.AudioBundle{})
		t.Cleanup(func() { audio.ApplyAudioBundle(id, audio.AudioBundle{}) })
	}
	audio.ApplyAudioBundle(roster.Snare, settings)
	settings = audio.CurrentAudioBundle(roster.Snare) // as the stores hold it (params sanitized)

	dv := &DrumView{
		Rows: []*DrumRow{
			{
				Name:       "Snare",
				Instrument: roster.Snare,
				Volume:     0.72,
				Muted:      false,
				Solo:       true,
			},
			{
				Name:       "Kick",
				Instrument: roster.Kick,
				Volume:     0.95,
				Role:       instrument.RoleKick, // explicit role beats heuristic
			},
		},
	}

	kit := audio.Kit{
		ID:          "kit.fixture",
		DisplayName: "Fixture Kit",
		Members: map[instrument.Role]instrument.ID{
			instrument.RoleSnare: roster.SnareLofi,
			instrument.RoleKick:  roster.KickDeep,
		},
	}
	rebound := dv.ApplyKit(kit)
	if rebound != 2 {
		t.Fatalf("rebound = %d want 2", rebound)
	}

	// Instrument changed.
	if dv.Rows[0].Instrument != roster.SnareLofi {
		t.Errorf("row 0 instrument = %q want snare-2", dv.Rows[0].Instrument)
	}
	if dv.Rows[1].Instrument != roster.KickDeep {
		t.Errorf("row 1 instrument = %q want kick-deep", dv.Rows[1].Instrument)
	}

	// Every other mix-state field on row 0 unchanged.
	if dv.Rows[0].Volume != 0.72 {
		t.Errorf("Volume drifted: %v", dv.Rows[0].Volume)
	}
	if dv.Rows[0].Solo != true || dv.Rows[0].Muted != false {
		t.Errorf("Mute/Solo flags drifted: muted=%v solo=%v", dv.Rows[0].Muted, dv.Rows[0].Solo)
	}
	if got := audio.CurrentAudioBundle(roster.SnareLofi); got.Digest() != settings.Digest() {
		t.Errorf("the row's EQ / filters / effects did not carry over to the kit's snare:\n got %+v\nwant %+v", got, settings)
	}
	if !audio.CurrentAudioBundle(roster.KickDeep).IsNeutral() {
		t.Errorf("the kick row had no settings, yet its new instrument got some: %+v", audio.CurrentAudioBundle(roster.KickDeep))
	}

	// EventKitApplied published with the kit metadata.
	for i := 0; i < 100; i++ {
		mu.Lock()
		n := len(seen)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) == 0 {
		t.Fatalf("EventKitApplied not delivered")
	}
	p, ok := seen[0].Payload.(hooks.KitPayload)
	if !ok || p.KitID != "kit.fixture" {
		t.Errorf("payload = %+v want KitID=kit.fixture", seen[0].Payload)
	}
	if len(p.Members) != 2 {
		t.Errorf("payload Members len = %d want 2", len(p.Members))
	}
}

func floatSliceEqual(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func boolSliceEqual(a, b []bool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
