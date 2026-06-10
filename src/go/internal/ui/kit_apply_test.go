package ui

import (
	"sync"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// TestApplyKitToDrumView_PreservesMixState is the Phase 6 ship signal:
// applying a kit must swap each row's Instrument and leave every other
// piece of per-row mix state byte-equal. Covers Volume, Pan, DelaySend,
// ReverbSend, Mute / Solo flags, EQ band gains, HPF/LPF, and the insert
// effects chain. Built without a real Game (no Ebiten); the test only
// needs the DrumView struct's row slice and the kit_binder bridge.
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

	originalEQ := []float64{0, +1.5, -2.0, 0, +0.5, 0, 0, 0, 0, 0}
	originalEQMute := []bool{false, false, true, false, false, false, false, false, false, false}
	originalEffects := []audio.EffectSlot{
		{Type: audio.EffectDistortion, Enabled: true, Params: map[string]float64{"drive": 0.6}},
	}

	dv := &DrumView{
		Rows: []*DrumRow{
			{
				Name:        "Snare",
				Instrument:  "snare",
				Volume:      0.72,
				Pan:         -0.3,
				DelaySend:   0.4,
				ReverbSend:  0.2,
				Muted:       false,
				Solo:        true,
				EQGainsDB:   append([]float64(nil), originalEQ...),
				EQBandMuted: append([]bool(nil), originalEQMute...),
				HPFEnabled:  true,
				HPFCutoffHz: 120,
				LPFEnabled:  true,
				LPFCutoffHz: 8000,
				Effects:     append([]audio.EffectSlot(nil), originalEffects...),
			},
			{
				Name:       "Kick",
				Instrument: "kick",
				Volume:     0.95,
				Pan:        0.1,
				Role:       "kick", // explicit role beats heuristic
			},
		},
	}

	kit := audio.Kit{
		ID:          "kit.fixture",
		DisplayName: "Fixture Kit",
		Members: map[string]string{
			"snare": "snare-2",
			"kick":  "kick-deep",
		},
	}
	rebound := dv.ApplyKit(kit)
	if rebound != 2 {
		t.Fatalf("rebound = %d want 2", rebound)
	}

	// Instrument changed.
	if dv.Rows[0].Instrument != "snare-2" {
		t.Errorf("row 0 instrument = %q want snare-2", dv.Rows[0].Instrument)
	}
	if dv.Rows[1].Instrument != "kick-deep" {
		t.Errorf("row 1 instrument = %q want kick-deep", dv.Rows[1].Instrument)
	}

	// Every other mix-state field on row 0 unchanged.
	if dv.Rows[0].Volume != 0.72 {
		t.Errorf("Volume drifted: %v", dv.Rows[0].Volume)
	}
	if dv.Rows[0].Pan != -0.3 {
		t.Errorf("Pan drifted: %v", dv.Rows[0].Pan)
	}
	if dv.Rows[0].DelaySend != 0.4 || dv.Rows[0].ReverbSend != 0.2 {
		t.Errorf("Sends drifted: delay=%v reverb=%v", dv.Rows[0].DelaySend, dv.Rows[0].ReverbSend)
	}
	if dv.Rows[0].Solo != true || dv.Rows[0].Muted != false {
		t.Errorf("Mute/Solo flags drifted: muted=%v solo=%v", dv.Rows[0].Muted, dv.Rows[0].Solo)
	}
	if !floatSliceEqual(dv.Rows[0].EQGainsDB, originalEQ) {
		t.Errorf("EQ gains drifted: got %v want %v", dv.Rows[0].EQGainsDB, originalEQ)
	}
	if !boolSliceEqual(dv.Rows[0].EQBandMuted, originalEQMute) {
		t.Errorf("EQ band mute drifted: got %v want %v", dv.Rows[0].EQBandMuted, originalEQMute)
	}
	if !dv.Rows[0].HPFEnabled || dv.Rows[0].HPFCutoffHz != 120 {
		t.Errorf("HPF drifted: enabled=%v cutoff=%v", dv.Rows[0].HPFEnabled, dv.Rows[0].HPFCutoffHz)
	}
	if !dv.Rows[0].LPFEnabled || dv.Rows[0].LPFCutoffHz != 8000 {
		t.Errorf("LPF drifted: enabled=%v cutoff=%v", dv.Rows[0].LPFEnabled, dv.Rows[0].LPFCutoffHz)
	}
	if len(dv.Rows[0].Effects) != 1 || dv.Rows[0].Effects[0].Type != audio.EffectDistortion {
		t.Errorf("Effects chain drifted: %+v", dv.Rows[0].Effects)
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
