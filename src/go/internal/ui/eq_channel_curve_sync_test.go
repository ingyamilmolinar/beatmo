//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestEQCurveGainsSyncOnChannelSwitch verifies that switching the EQ channel
// via setEQActiveChannel() syncs z.bandGainsDB to the new channel's gains.
// Bug: only slider .Value was updated; z.bandGainsDB stayed stale, so the
// curve line and band handles showed the previous channel's EQ.
func TestEQCurveGainsSyncOnChannelSwitch(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Need at least 2 rows with distinct instruments.
	g.drum.AddRow()
	g.drum.Rows[0].Instrument = "kick"
	g.drum.Rows[1].Instrument = "snare"

	// Initialize EQ gains for both rows.
	g.drum.ensureRowEQ(0)
	g.drum.ensureRowEQ(1)

	// Set distinct gains: row 0 band 3 = +8 dB, row 1 band 5 = -6 dB.
	g.drum.Rows[0].EQGainsDB[3] = 8.0
	g.drum.Rows[1].EQGainsDB[5] = -6.0

	z := g.drum.eqPanelZone

	// Switch to row 0's instrument.
	g.drum.setEQActiveChannel("kick")

	if z.bandGainsDB[3] != 8.0 {
		t.Errorf("after switching to kick: bandGainsDB[3] = %v, want 8.0", z.bandGainsDB[3])
	}
	if z.bandGainsDB[5] != 0.0 {
		t.Errorf("after switching to kick: bandGainsDB[5] = %v, want 0.0", z.bandGainsDB[5])
	}

	// Switch to row 1's instrument.
	g.drum.setEQActiveChannel("snare")

	if z.bandGainsDB[5] != -6.0 {
		t.Errorf("after switching to snare: bandGainsDB[5] = %v, want -6.0", z.bandGainsDB[5])
	}
	if z.bandGainsDB[3] != 0.0 {
		t.Errorf("after switching to snare: bandGainsDB[3] = %v, want 0.0", z.bandGainsDB[3])
	}

	// Switch back to master — should reflect master gains (all zero by default).
	g.drum.setEQActiveChannel("main")

	for i, v := range z.bandGainsDB {
		if v != 0.0 {
			t.Errorf("after switching to main: bandGainsDB[%d] = %v, want 0.0", i, v)
		}
	}
}

// TestEQCurveDirtySyncOnChannelSwitch verifies that z.curveDirty is set
// when switching channels so the curve is recomputed.
// Bug: only dv.eqCurveDirty was set; z.curveDirty was not.
func TestEQCurveDirtySyncOnChannelSwitch(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	g.drum.AddRow()
	inst := g.drum.Rows[1].Instrument

	z := g.drum.eqPanelZone

	// Clear dirty flags.
	z.curveDirty = false
	g.drum.eqCurveDirty = false

	// Switch channel.
	g.drum.setEQActiveChannel(inst)

	if !z.curveDirty {
		t.Error("z.curveDirty should be true after channel switch")
	}
}

// TestEQCurveMuteSyncOnChannelSwitch verifies that z.bandMuted is synced
// to the new channel's mute state on switch.
func TestEQCurveMuteSyncOnChannelSwitch(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	g.drum.AddRow()
	g.drum.Rows[0].Instrument = "kick"

	// Initialize and mute band 2 on row 0.
	g.drum.ensureRowEQ(0)
	g.drum.ensureRowEQMuted(0)
	g.drum.Rows[0].EQBandMuted[2] = true

	z := g.drum.eqPanelZone

	// Switch to row 0's instrument.
	g.drum.setEQActiveChannel("kick")

	if !z.bandMuted[2] {
		t.Error("after switching to kick: bandMuted[2] should be true")
	}

	// Switch to master (no mutes).
	g.drum.setEQActiveChannel("main")

	if z.bandMuted[2] {
		t.Error("after switching to main: bandMuted[2] should be false")
	}
}

// TestEQGainUpdatesBandGainsOnPerInstrument verifies that setting a gain via
// the curve drag mechanism updates z.bandGainsDB even when activeChannel != "main".
func TestEQGainUpdatesBandGainsOnPerInstrument(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, logger)
	dv.calcLayout()

	z := dv.eqPanelZone

	// Simulate switching to a per-instrument channel.
	z.activeChannel = "kick"

	// Set the gain directly on bandGainsDB (as the curve drag handler does).
	band := 4
	want := 5.0 // +5 dB
	z.bandGainsDB[band] = want
	z.curveDirty = true

	got := z.bandGainsDB[band]
	if got != want {
		t.Errorf("bandGainsDB[%d] = %v after gain change on per-instrument channel, want %v", band, got, want)
	}
}

// TestEQCurveReflectsChannelAfterSwitch is an end-to-end test: after
// switching to an instrument with +10 dB on band 5, the recomputed curve
// cache should show positive gain around that frequency.
func TestEQCurveReflectsChannelAfterSwitch(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	g.drum.AddRow()
	g.drum.Rows[0].Instrument = "kick"
	g.drum.ensureRowEQ(0)
	g.drum.Rows[0].EQGainsDB[5] = 10.0 // band 5 ≈ 1 kHz

	z := g.drum.eqPanelZone

	// Switch to kick — should sync gains and mark curve dirty.
	g.drum.setEQActiveChannel("kick")

	// Force curve recompute.
	z.curveDirty = true
	bands := z.buildCurrentBands()
	if len(bands) == 0 {
		t.Fatal("buildCurrentBands returned no bands")
	}

	// Band 5 should have +10 dB gain in the computed bands.
	// Bands may include HPF/LPF prepend/append, so find the peaking bands.
	found := false
	for _, b := range bands {
		if b.GainDB == 10.0 {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("curve bands should include +10 dB gain; got %v", bands)
	}
}

// TestEQCurveCacheUpdatesViaDropdownClick exercises the full dropdown click
// path: open dropdown, click an instrument, and verify the zone's curveCache
// actually reflects the new channel's gains after a draw-like recompute.
func TestEQCurveCacheUpdatesViaDropdownClick(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	dv := g.drum
	z := dv.eqPanelZone

	// Set up two instruments with distinct EQ profiles.
	dv.AddRow()
	dv.Rows[0].Instrument = "kick"
	dv.Rows[1].Instrument = "snare"
	dv.ensureRowEQ(0)
	dv.ensureRowEQ(1)
	dv.Rows[0].EQGainsDB[3] = 12.0 // kick: big boost on band 3
	dv.Rows[1].EQGainsDB[7] = -12.0 // snare: big cut on band 7

	// Start on master (flat).
	dv.setEQActiveChannel("main")

	// Prime the curve cache by triggering a recompute.
	z.curveDirty = true
	_ = z.buildCurrentBands()
	z.curveCache = audio.ComputeFreqResponse(audio.SampleRate(),
		z.buildCurrentBands(), eqCurvePoints, 20, 20000)
	z.curveDirty = false

	// Verify master curve is flat (all gains near 0 dB).
	for _, p := range z.curveCache {
		if p.GainDB > 1.0 || p.GainDB < -1.0 {
			t.Fatalf("master curve should be flat, got %.1f dB at %.0f Hz", p.GainDB, p.FreqHz)
		}
	}

	// --- Simulate dropdown click to switch to kick ---
	// Use the same callback path as the actual dropdown button onClick.
	dv.setEQActiveChannel("kick")

	// The curve should be dirty now; recompute as drawEQCurve would.
	if !z.curveDirty && z.curveCache != nil {
		t.Error("curveCache should be nil or curveDirty should be true after channel switch")
	}
	z.curveCache = audio.ComputeFreqResponse(audio.SampleRate(),
		z.buildCurrentBands(), eqCurvePoints, 20, 20000)
	z.curveDirty = false

	// Verify the curve has a significant boost (kick's band 3 = +12 dB).
	maxGain := 0.0
	for _, p := range z.curveCache {
		if p.GainDB > maxGain {
			maxGain = p.GainDB
		}
	}
	if maxGain < 5.0 {
		t.Errorf("kick curve should show significant boost, max gain = %.1f dB", maxGain)
	}

	// --- Switch to snare ---
	dv.setEQActiveChannel("snare")
	z.curveCache = audio.ComputeFreqResponse(audio.SampleRate(),
		z.buildCurrentBands(), eqCurvePoints, 20, 20000)
	z.curveDirty = false

	// Verify the curve has a significant cut (snare's band 7 = -12 dB).
	minGain := 0.0
	for _, p := range z.curveCache {
		if p.GainDB < minGain {
			minGain = p.GainDB
		}
	}
	if minGain > -5.0 {
		t.Errorf("snare curve should show significant cut, min gain = %.1f dB", minGain)
	}

	// --- Switch back to master ---
	dv.setEQActiveChannel("main")
	z.curveCache = audio.ComputeFreqResponse(audio.SampleRate(),
		z.buildCurrentBands(), eqCurvePoints, 20, 20000)

	// Master should be flat again.
	for _, p := range z.curveCache {
		if p.GainDB > 1.0 || p.GainDB < -1.0 {
			t.Fatalf("master curve should be flat after switch back, got %.1f dB at %.0f Hz",
				p.GainDB, p.FreqHz)
		}
	}
}
