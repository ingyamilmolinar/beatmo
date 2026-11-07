package ui

import (
	"encoding/json"
	"image"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestEQSliderLinearMapping(t *testing.T) {
	assertDefaultParityState(t)

	// 0% = -12 dB, 50% = 0 dB, 100% = +12 dB.
	tests := []struct {
		slider  float64
		minGain float64
		maxGain float64
		desc    string
	}{
		{0.0, -12.1, -11.9, "0% should be -12 dB"},
		{0.25, -6.1, -5.9, "25% should be -6 dB"},
		{0.5, -0.1, 0.1, "50% should be unity (0 dB)"},
		{0.75, 5.9, 6.1, "75% should be +6 dB"},
		{1.0, 11.9, 12.1, "100% should be +12 dB boost"},
	}

	for _, tc := range tests {
		gain := sliderToGainDB(tc.slider)
		if gain < tc.minGain || gain > tc.maxGain {
			t.Errorf("%s: slider %.2f produced gain %.1f dB, want [%.1f, %.1f]",
				tc.desc, tc.slider, gain, tc.minGain, tc.maxGain)
		}
	}

	// Test round-trip: slider -> gain -> slider
	for _, v := range []float64{0, 0.1, 0.25, 0.5, 0.75, 1.0} {
		g := sliderToGainDB(v)
		back := gainDBToSlider(g)
		if math.Abs(back-v) > 0.01 {
			t.Errorf("round-trip failed: slider %.2f -> gain %.1f dB -> slider %.2f",
				v, g, back)
		}
	}
}

func TestPerInstrumentEQInitialization(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Initial row should have EQ gains initialized
	if len(g.drum.Rows) == 0 {
		t.Fatal("expected at least one row")
	}
	row := g.drum.Rows[0]
	if len(rowEQ(row).GainsDB) != len(eqBandDefs) {
		t.Errorf("EQGainsDB not initialized: got %d, want %d", len(rowEQ(row).GainsDB), len(eqBandDefs))
	}

	// Add a new row and verify EQ is initialized
	g.drum.AddRow()
	if len(g.drum.Rows) != 2 {
		t.Fatal("expected 2 rows after AddRow")
	}
	row2 := g.drum.Rows[1]
	if len(rowEQ(row2).GainsDB) != len(eqBandDefs) {
		t.Errorf("new row EQGainsDB not initialized: got %d, want %d", len(rowEQ(row2).GainsDB), len(eqBandDefs))
	}
}

func TestPerInstrumentEQChannelSelection(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Default should be master
	if g.drum.activeEQChannel() != channel.Master {
		t.Errorf("default active channel should be 'main', got %q", g.drum.activeEQChannel())
	}

	// Add a row to have something to select
	g.drum.AddRow()
	inst := g.drum.Rows[1].Instrument

	// Switch to per-instrument channel
	g.drum.setEQActiveChannel(channel.Of(inst))
	if g.drum.activeEQChannel() != channel.Of(inst) {
		t.Errorf("active channel should be %q, got %q", inst, g.drum.activeEQChannel())
	}

	// Switch back to master
	g.drum.setEQActiveChannel(channel.Master)
	if g.drum.activeEQChannel() != channel.Master {
		t.Errorf("active channel should be 'main', got %q", g.drum.activeEQChannel())
	}
}

func TestPerInstrumentEQCycle(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Start at master
	g.drum.setEQActiveChannel(channel.Master)

	// Add two rows with different instruments
	g.drum.AddRow()
	g.drum.AddRow()

	// Rename instruments to be unique for the test
	g.drum.Rows[0].Instrument = roster.Kick
	g.drum.Rows[1].Instrument = roster.Hihat
	g.drum.Rows[2].Instrument = roster.Snare

	inst0 := g.drum.Rows[0].Instrument
	inst1 := g.drum.Rows[1].Instrument
	inst2 := g.drum.Rows[2].Instrument

	// Cycle: main -> inst0
	g.drum.cycleEQChannel()
	if g.drum.activeEQChannel() != channel.Of(inst0) {
		t.Errorf("after first cycle, expected %q, got %q", inst0, g.drum.activeEQChannel())
	}

	// Cycle: inst0 -> inst1
	g.drum.cycleEQChannel()
	if g.drum.activeEQChannel() != channel.Of(inst1) {
		t.Errorf("after second cycle, expected %q, got %q", inst1, g.drum.activeEQChannel())
	}

	// Cycle: inst1 -> inst2
	g.drum.cycleEQChannel()
	if g.drum.activeEQChannel() != channel.Of(inst2) {
		t.Errorf("after third cycle, expected %q, got %q", inst2, g.drum.activeEQChannel())
	}

	// Cycle: inst2 -> main (wrap around)
	g.drum.cycleEQChannel()
	if g.drum.activeEQChannel() != channel.Master {
		t.Errorf("after fourth cycle, expected 'main', got %q", g.drum.activeEQChannel())
	}
}

func TestPerInstrumentEQDeleteRowResetsChannel(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Add a row
	g.drum.AddRow()
	inst := g.drum.Rows[1].Instrument

	// Select that row's channel
	g.drum.setEQActiveChannel(channel.Of(inst))
	if g.drum.activeEQChannel() != channel.Of(inst) {
		t.Errorf("active channel should be %q, got %q", inst, g.drum.activeEQChannel())
	}

	// Delete the row
	g.drum.DeleteRow(1)

	// Channel should reset to master
	if g.drum.activeEQChannel() != channel.Master {
		t.Errorf("after deleting active channel row, should reset to 'main', got %q", g.drum.activeEQChannel())
	}
}

func TestPerInstrumentEQExportImport(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Create a node for the row origin
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = a.ID
	g.updateBeatInfos()

	// Set non-zero EQ on the first row
	rowEQ(g.drum.Rows[0]).GainsDB = make([]float64, len(eqBandDefs))
	rowEQ(g.drum.Rows[0]).GainsDB[0] = 6.0  // +6dB on first band
	rowEQ(g.drum.Rows[0]).GainsDB[5] = -3.0 // -3dB on sixth band

	// Export
	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	// Parse and verify
	var f projectFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(f.Instruments) == 0 {
		t.Fatal("no instruments in export")
	}

	inst := f.Instruments[0]
	if inst.EQ == nil {
		t.Fatal("per-instrument EQ not exported")
	}
	if len(inst.EQ.GainsDB) != len(eqBandDefs) {
		t.Errorf("EQ gains count mismatch: got %d, want %d", len(inst.EQ.GainsDB), len(eqBandDefs))
	}
	if inst.EQ.GainsDB[0] != 6.0 {
		t.Errorf("EQ band 0 mismatch: got %f, want 6.0", inst.EQ.GainsDB[0])
	}
	if inst.EQ.GainsDB[5] != -3.0 {
		t.Errorf("EQ band 5 mismatch: got %f, want -3.0", inst.EQ.GainsDB[5])
	}
}

func TestPerInstrumentEQNoExportWhenZero(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Create a node for the row origin
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = a.ID
	g.updateBeatInfos()

	// Ensure all EQ bands are zero
	rowEQ(g.drum.Rows[0]).GainsDB = make([]float64, len(eqBandDefs))

	// Export
	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	// Parse and verify EQ is not included
	var f projectFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(f.Instruments) == 0 {
		t.Fatal("no instruments in export")
	}

	inst := f.Instruments[0]
	if inst.EQ != nil {
		t.Error("per-instrument EQ should not be exported when all bands are zero")
	}
}

func TestEQChannelMenuBuild(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Add a few rows
	g.drum.AddRow()
	g.drum.AddRow()

	// Open the dropdown via portal path.
	z := g.drum.eqPanelZone
	z.stickyBar.ChannelBtn().OnClick()

	// Total items should be Master + 3 rows = 4
	scroll := z.channelScroll
	if scroll.VS.Total != 4 {
		t.Errorf("expected 4 total channel items (Master + 3 rows), got %d", scroll.VS.Total)
	}

	if !g.drum.portal().Has("eq-channel-dropdown") {
		t.Error("expected eq-channel-dropdown portal to be open")
	}

	// Channel button should show "Master" initially.
	if z.stickyBar.ChannelBtn().Text != "Master" {
		t.Errorf("channel button should show 'Master', got %q", z.stickyBar.ChannelBtn().Text)
	}

	// If total > visible, scrollbar should be present
	if scroll.VS.Total > scroll.VS.Visible {
		if !scroll.HasScroll() {
			t.Errorf("expected scrollbar when total (%d) > visible (%d)",
				scroll.VS.Total, scroll.VS.Visible)
		}
	}
}

// TestEQChannelDropdownButtonsAreVisible ensures dropdown buttons are within visible bounds.
func TestEQChannelDropdownButtonsAreVisible(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	dv := g.drum
	dv.recalcButtons()

	// Add rows
	dv.AddRow()
	dv.AddRow()

	// Open the dropdown via portal path
	z := dv.eqPanelZone
	z.stickyBar.ChannelBtn().OnClick()

	// Check that the trigger button is visible within the drum view bounds
	if dv.eqChannelBtn() == nil {
		t.Fatalf("eqChannelBtn is nil")
	}

	triggerRect := dv.eqChannelBtn().Rect()
	t.Logf("Trigger button rect: %v", triggerRect)
	t.Logf("DrumView bounds: %v", dv.Bounds)
	t.Logf("EQ rect: %v", dv.eqRect)

	// Trigger button should be within EQ panel
	if !triggerRect.In(dv.eqRect) && !triggerRect.Overlaps(dv.eqRect) {
		t.Errorf("trigger button %v is not within EQ panel %v", triggerRect, dv.eqRect)
	}

	// Portal should be open with the dropdown (in the audio-panel subtree).
	if !dv.portal().Has("eq-channel-dropdown") {
		t.Error("expected eq-channel-dropdown portal to be open")
	}

	// Menu view rect from portal scroll should be non-empty and below the trigger
	menuRect := z.channelScroll.VS.View
	t.Logf("Menu rect: %v", menuRect)

	if menuRect.Empty() {
		t.Error("menu view rect should not be empty")
	}
	if menuRect.Min.Y <= triggerRect.Min.Y {
		t.Errorf("menu rect %v should be below trigger %v", menuRect, triggerRect)
	}
}

// Through DrumView.Update.
func TestEQChannelDropdownOpensAndSelects(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	dv := g.drum
	dv.recalcButtons()

	// Add rows with different instruments for the test
	dv.AddRow()
	dv.Rows[0].Instrument = roster.Kick
	dv.Rows[0].Name = "Kick"
	dv.Rows[1].Instrument = roster.Snare
	dv.Rows[1].Name = "Snare"

	// Verify starting state
	if dv.eqActiveChannel != channel.Master {
		t.Fatalf("initial active channel should be 'main', got %q", dv.eqActiveChannel)
	}
	if dv.eqChannelBtn() == nil {
		t.Fatalf("eqChannelBtn is nil")
	}

	// Click the EQ channel button to open the dropdown
	center := dv.eqChannelBtn().Rect()
	cx, cy := (center.Min.X+center.Max.X)/2, (center.Min.Y+center.Max.Y)/2
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	t.Cleanup(restore)
	dv.Update()
	restore()

	if !dv.IsEQChannelOpen() && (dv.eqPanelZone == nil || !dv.eqPanelZone.ChannelDropdownOpen()) {
		t.Fatalf("EQ channel menu did not open")
	}

	// Release frame to clear suppress.
	restore = SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	dv.Update()
	restore()

	// Item 2 is the first row; items sit at anchor.Max.Y + i·btnH.
	anchor := dv.eqChannelBtn().Rect()
	btnH := 24 // dropdown button height
	rx := (anchor.Min.X + anchor.Max.X) / 2
	ry := anchor.Max.Y + btnH + btnH/2 // center of second button (index 1)
	// Press frame
	restore = SetInputForTest(
		func() (int, int) { return rx, ry },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	dv.Update()
	restore()

	// Release frame — deferred tap fires the button
	restore = SetInputForTest(
		func() (int, int) { return rx, ry },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	dv.Update()
	restore()

	if dv.IsEQChannelOpen() || (dv.eqPanelZone != nil && dv.eqPanelZone.ChannelDropdownOpen()) {
		t.Fatalf("EQ channel menu did not close after selection")
	}
	if dv.eqActiveChannel != channel.Of(roster.Kick) {
		t.Fatalf("active channel should be 'kick', got %q", dv.eqActiveChannel)
	}
	if dv.eqChannelBtn().Text != "Kick" {
		t.Fatalf("button text should be 'Kick', got %q", dv.eqChannelBtn().Text)
	}
}

// Through the full Game.Update flow.
func TestEQChannelDropdownViaGameUpdate(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	dv := g.drum
	dv.recalcButtons()

	// Add rows with different instruments for the test
	dv.AddRow()
	dv.Rows[0].Instrument = roster.Kick
	dv.Rows[0].Name = "Kick"
	dv.Rows[1].Instrument = roster.Snare
	dv.Rows[1].Name = "Snare"

	// Verify starting state
	if dv.eqActiveChannel != channel.Master {
		t.Fatalf("initial active channel should be 'main', got %q", dv.eqActiveChannel)
	}
	if dv.eqChannelBtn() == nil {
		t.Fatalf("eqChannelBtn is nil")
	}

	// Click the EQ channel button to open the dropdown (press + release).
	center := dv.eqChannelBtn().Rect()
	cx, cy := (center.Min.X+center.Max.X)/2, (center.Min.Y+center.Max.Y)/2
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	_ = g.Update()
	restore()
	// Release frame to clear suppress.
	restore = SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	_ = g.Update()
	restore()

	if !dv.IsEQChannelOpen() && (dv.eqPanelZone == nil || !dv.eqPanelZone.ChannelDropdownOpen()) {
		t.Fatalf("EQ channel menu did not open via Game.Update()")
	}

	// Item 2 is the first row; items sit at anchor.Max.Y + i·btnH.
	anchor := dv.eqChannelBtn().Rect()
	btnH := 24 // dropdown button height
	rx := (anchor.Min.X + anchor.Max.X) / 2
	ry := anchor.Max.Y + btnH + btnH/2 // center of second button (index 1)
	// Press frame
	restore = SetInputForTest(
		func() (int, int) { return rx, ry },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	_ = g.Update()
	restore()

	// Release frame — deferred tap fires the button
	restore = SetInputForTest(
		func() (int, int) { return rx, ry },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	_ = g.Update()
	restore()

	if dv.IsEQChannelOpen() || (dv.eqPanelZone != nil && dv.eqPanelZone.ChannelDropdownOpen()) {
		t.Fatalf("EQ channel menu did not close after selection via Game.Update()")
	}
	if dv.eqActiveChannel != channel.Of(roster.Kick) {
		t.Fatalf("active channel should be 'kick', got %q", dv.eqActiveChannel)
	}
	if dv.eqChannelBtn().Text != "Kick" {
		t.Fatalf("button text should be 'Kick', got %q", dv.eqChannelBtn().Text)
	}
}

// So the layout handler cannot consume input and flicker the menu.
func TestEQChannelDropdownBlocksLayoutHandler(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	dv := g.drum

	// Menu closed - should not be capturing or blocking
	dv.CloseAllPopups()
	if dv.Capturing() {
		t.Error("Capturing() should be false when eqChannelOpen is false")
	}
	// BlocksAt requires point within bounds
	cx := (dv.Bounds.Min.X + dv.Bounds.Max.X) / 2
	cy := (dv.Bounds.Min.Y + dv.Bounds.Max.Y) / 2
	if dv.BlocksAt(cx, cy) {
		t.Error("BlocksAt() should be false when eqChannelOpen is false")
	}

	// Menu open via portal path - should be capturing and blocking
	dv.eqPanelZone.stickyBar.ChannelBtn().OnClick()
	if !dv.Capturing() {
		t.Error("Capturing() should be true when EQ channel dropdown is open")
	}
	if !dv.BlocksAt(cx, cy) {
		t.Error("BlocksAt() should be true when EQ channel dropdown is open")
	}
	dv.CloseAllPopups()
}

// The menu stays open across Update frames with the button held.
func TestEQChannelDropdownNoFlickerOnClick(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	dv := g.drum
	dv.recalcButtons()

	// Add a row
	dv.AddRow()
	dv.Rows[0].Instrument = roster.Kick
	dv.Rows[0].Name = "Kick"

	if dv.eqChannelBtn() == nil {
		t.Fatalf("eqChannelBtn is nil")
	}

	// Click and hold on the EQ channel button to open the dropdown
	center := dv.eqChannelBtn().Rect()
	cx, cy := (center.Min.X+center.Max.X)/2, (center.Min.Y+center.Max.Y)/2

	// Simulate press down
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	dv.Update()

	eqChOpen := dv.IsEQChannelOpen() || (dv.eqPanelZone != nil && dv.eqPanelZone.ChannelDropdownOpen())
	if !eqChOpen {
		restore()
		t.Fatalf("EQ channel menu did not open on first Update()")
	}

	for i := 0; i < 5; i++ {
		dv.Update()
		eqChOpen = dv.IsEQChannelOpen() || (dv.eqPanelZone != nil && dv.eqPanelZone.ChannelDropdownOpen())
		if !eqChOpen {
			restore()
			t.Fatalf("EQ channel menu closed unexpectedly on Update() iteration %d (flickering bug)", i)
		}
	}

	restore()

	// Release mouse, then click outside to close - menu should close
	restore = SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	dv.Update() // Release frame
	restore()

	// Menu should still be open after release (only closes on click outside or selection)
	eqChOpen = dv.IsEQChannelOpen() || (dv.eqPanelZone != nil && dv.eqPanelZone.ChannelDropdownOpen())
	if !eqChOpen {
		t.Fatalf("EQ channel menu should remain open after mouse release")
	}
}

func TestEQBandMuteProducesSilence(t *testing.T) {
	assertDefaultParityState(t)

	// The Muted flag reaches the audio.EQBand slice.

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Ensure we have EQ band arrays initialized
	if len(g.drum.eqBandMuted()) != len(eqBandDefs) {
		// Initialize if not already done
		g.drum.eqPanelZone.bandMuted = make([]bool, len(eqBandDefs))
	}
	if len(g.drum.eqBandGainsDB()) != len(eqBandDefs) {
		g.drum.eqPanelZone.bandGainsDB = make([]float64, len(eqBandDefs))
	}

	// Mute the first band (low frequencies)
	g.drum.eqBandMuted()[0] = true
	g.drum.applyMasterEQ()

	// Verify the band is marked as muted in the applied EQ
	if len(g.drum.eqApplied) == 0 {
		t.Fatal("no EQ bands applied")
	}
	if !g.drum.eqApplied[0].Muted {
		t.Error("first band should be muted in applied EQ")
	}

	// Other bands should not be muted
	for i := 1; i < len(g.drum.eqApplied); i++ {
		if g.drum.eqApplied[i].Muted {
			t.Errorf("band %d should not be muted", i)
		}
	}
}

func TestEQBandMuteToggle(t *testing.T) {
	assertDefaultParityState(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Initialize mute state
	if len(g.drum.eqBandMuted()) != len(eqBandDefs) {
		g.drum.eqPanelZone.bandMuted = make([]bool, len(eqBandDefs))
	}

	// Initial state: no bands muted
	for i, muted := range g.drum.eqBandMuted() {
		if muted {
			t.Errorf("band %d should not be muted initially", i)
		}
	}

	// Toggle mute on band 2
	g.drum.eqBandMuted()[2] = true

	// Verify state
	if !g.drum.eqBandMuted()[2] {
		t.Error("band 2 should be muted after toggle")
	}

	// Toggle off
	g.drum.eqBandMuted()[2] = false
	if g.drum.eqBandMuted()[2] {
		t.Error("band 2 should not be muted after second toggle")
	}
}

func TestEQBandMuteExport(t *testing.T) {
	assertDefaultParityState(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Create a node for the row origin
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = a.ID
	g.updateBeatInfos()

	// Initialize and set mute state
	g.drum.eqPanelZone.bandMuted = make([]bool, len(eqBandDefs))
	g.drum.eqPanelZone.bandGainsDB = make([]float64, len(eqBandDefs))
	g.drum.eqBandMuted()[0] = true // Mute first band
	g.drum.eqBandMuted()[5] = true // Mute sixth band

	// Export
	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	// Parse and verify
	var f projectFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if f.EQ == nil {
		t.Fatal("master EQ not exported")
	}
	if len(f.EQ.BandMuted) == 0 {
		t.Fatal("band muted state not exported")
	}
	if !f.EQ.BandMuted[0] {
		t.Error("band 0 should be muted in export")
	}
	if !f.EQ.BandMuted[5] {
		t.Error("band 5 should be muted in export")
	}
	// Other bands should not be muted
	for i := 1; i < 5; i++ {
		if i < len(f.EQ.BandMuted) && f.EQ.BandMuted[i] {
			t.Errorf("band %d should not be muted in export", i)
		}
	}
}

func TestPerInstrumentEQBandMute(t *testing.T) {
	assertDefaultParityState(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Add a row and set up per-instrument mute state
	g.drum.AddRow()
	row := g.drum.Rows[1]
	row.Instrument = roster.Snare

	// Initialize EQ for the row

	// Mute a band on the row
	rowEQ(row).BandMuted[3] = true

	// Build bands for this row
	bands := g.drum.buildFullEQBands(rowEQ(row).GainsDB, rowEQ(row).BandMuted, false, 20, false, 20000)

	// Verify the muted band
	if !bands[3].Muted {
		t.Error("band 3 should be muted")
	}
	for i := 0; i < len(bands); i++ {
		if i != 3 && bands[i].Muted {
			t.Errorf("band %d should not be muted", i)
		}
	}
}

// Once, not per held frame.
func TestEQBandMuteButtonSingleClickToggle(t *testing.T) {
	assertDefaultParityState(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	dv := g.drum
	dv.recalcButtons()

	// Initialize mute state
	if len(dv.eqBandMuted()) != len(eqBandDefs) {
		dv.eqPanelZone.bandMuted = make([]bool, len(eqBandDefs))
	}
	if len(dv.eqBandGainsDB()) != len(eqBandDefs) {
		dv.eqPanelZone.bandGainsDB = make([]float64, len(eqBandDefs))
	}

	// Verify band 0 starts unmuted
	if dv.eqBandMuted()[0] {
		t.Fatal("band 0 should be unmuted initially")
	}

	// Ensure EQ mute buttons are created with OnClick callbacks
	if len(dv.eqMuteBtns()) == 0 || dv.eqMuteBtns()[0] == nil {
		t.Fatal("EQ mute buttons not initialized")
	}
	if dv.eqMuteBtns()[0].OnClick == nil {
		t.Fatal("EQ mute button OnClick callback is nil - this is the bug we're fixing")
	}

	// What OnClick does.
	dv.toggleEQBandMute(0)

	// Verify mute toggled ON
	if !dv.eqBandMuted()[0] {
		t.Error("band 0 should be muted after first toggle")
	}

	// Click again to unmute
	dv.toggleEQBandMute(0)

	// Verify mute toggled OFF
	if dv.eqBandMuted()[0] {
		t.Error("band 0 should be unmuted after second toggle")
	}
}

// Many held frames toggle once.
func TestEQBandMuteHoldNoMultipleToggles(t *testing.T) {
	assertDefaultParityState(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	dv := g.drum
	dv.recalcButtons()
	dv.calcLayout()

	// Initialize mute state
	if len(dv.eqBandMuted()) != len(eqBandDefs) {
		dv.eqPanelZone.bandMuted = make([]bool, len(eqBandDefs))
	}
	if len(dv.eqBandGainsDB()) != len(eqBandDefs) {
		dv.eqPanelZone.bandGainsDB = make([]float64, len(eqBandDefs))
	}

	// Verify band 0 starts unmuted
	if dv.eqBandMuted()[0] {
		t.Fatal("band 0 should be unmuted initially")
	}

	// Ensure mute buttons are initialized
	if len(dv.eqMuteBtns()) == 0 || dv.eqMuteBtns()[0] == nil {
		t.Fatal("EQ mute buttons not initialized")
	}

	// Get button position (use center of button rect)
	btn := dv.eqMuteBtns()[0]
	r := btn.Rect()
	// If button rect is empty, skip this part of the test
	if r.Empty() {
		t.Skip("EQ mute button has no rect (EQ panel may be disabled in test mode)")
	}
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2

	// Simulate pressing and holding the button for many frames
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return true }, // Mouse pressed
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)

	// Run many Update() frames with button held
	for i := 0; i < 10; i++ {
		dv.Update()
	}
	restore()

	// Exactly once, or an even count would leave it unmuted.
	if !dv.eqBandMuted()[0] {
		t.Error("band 0 should be muted after holding button (single toggle, not per-frame)")
	}

	// Release button
	restore = SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return false }, // Mouse released
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	dv.Update()
	restore()

	// State should still be muted (release doesn't toggle)
	if !dv.eqBandMuted()[0] {
		t.Error("band 0 should still be muted after button release")
	}
}

func TestEQBandMuteUnmutePreservesGain(t *testing.T) {
	assertDefaultParityState(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	dv := g.drum

	// Initialize EQ state
	if len(dv.eqBandMuted()) != len(eqBandDefs) {
		dv.eqPanelZone.bandMuted = make([]bool, len(eqBandDefs))
	}
	if len(dv.eqBandGainsDB()) != len(eqBandDefs) {
		dv.eqPanelZone.bandGainsDB = make([]float64, len(eqBandDefs))
	}

	// Set band 0 gain to +6dB
	dv.eqBandGainsDB()[0] = 6.0
	dv.applyMasterEQ()

	// Verify gain is +6dB
	if dv.eqApplied[0].GainDB != 6.0 {
		t.Errorf("initial gain should be 6.0, got %f", dv.eqApplied[0].GainDB)
	}

	// Mute the band
	dv.toggleEQBandMute(0)
	if !dv.eqBandMuted()[0] {
		t.Error("band should be muted")
	}

	// Verify gain is still +6dB (muting doesn't change the gain value)
	if dv.eqBandGainsDB()[0] != 6.0 {
		t.Errorf("gain should still be 6.0 after mute, got %f", dv.eqBandGainsDB()[0])
	}

	// Unmute the band
	dv.toggleEQBandMute(0)
	if dv.eqBandMuted()[0] {
		t.Error("band should be unmuted")
	}

	// Verify gain is still +6dB after unmute
	if dv.eqBandGainsDB()[0] != 6.0 {
		t.Errorf("gain should still be 6.0 after unmute, got %f", dv.eqBandGainsDB()[0])
	}

	// Apply and verify in applied EQ
	dv.applyMasterEQ()
	if dv.eqApplied[0].GainDB != 6.0 {
		t.Errorf("applied gain should be 6.0 after unmute, got %f", dv.eqApplied[0].GainDB)
	}
	if dv.eqApplied[0].Muted {
		t.Error("band should not be muted in applied EQ after unmute")
	}
}

func TestPerInstrumentEQBandMuteToggle(t *testing.T) {
	assertDefaultParityState(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	dv := g.drum

	// Unique: the first row already plays "snare".
	dv.AddRow()
	dv.Rows[1].Instrument = roster.Hihat
	dv.Rows[1].Name = "HiHat"

	// Initialize EQ for the row

	// Switch to per-instrument channel
	dv.setEQActiveChannel(channel.Of(roster.Hihat))
	if dv.activeEQChannel() != channel.Of(roster.Hihat) {
		t.Fatalf("active channel should be 'hihat', got %q", dv.activeEQChannel())
	}

	// Verify band 2 starts unmuted
	if rowEQ(dv.Rows[1]).BandMuted[2] {
		t.Fatal("band 2 should be unmuted initially")
	}

	// Toggle mute on band 2 via the method
	dv.toggleEQBandMute(2)

	// Verify mute toggled ON for the per-instrument channel
	if !rowEQ(dv.Rows[1]).BandMuted[2] {
		t.Error("band 2 should be muted after toggle for hihat channel")
	}

	// Master channel should be unaffected
	if len(dv.eqBandMuted()) > 2 && dv.eqBandMuted()[2] {
		t.Error("master channel band 2 should not be muted")
	}

	// Row 0 (snare) should be unaffected
	if rowEQ(dv.Rows[0]).BandMuted[2] {
		t.Error("row 0 band 2 should not be muted")
	}

	// Toggle again to unmute
	dv.toggleEQBandMute(2)
	if rowEQ(dv.Rows[1]).BandMuted[2] {
		t.Error("band 2 should be unmuted after second toggle")
	}
}

// The mute buttons follow the new channel, as the sliders do.
func TestEQMuteButtonStateSyncsOnChannelSwitch(t *testing.T) {
	assertDefaultParityState(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	dv := g.drum
	dv.recalcButtons()

	// Add a row with a unique instrument
	dv.AddRow()
	dv.Rows[0].Instrument = roster.Kick
	dv.Rows[0].Name = "Kick"
	dv.Rows[1].Instrument = roster.Snare
	dv.Rows[1].Name = "Snare"

	// Initialize EQ state for master and rows
	if len(dv.eqBandMuted()) != len(eqBandDefs) {
		dv.eqPanelZone.bandMuted = make([]bool, len(eqBandDefs))
	}

	// Skip if mute buttons not initialized
	if len(dv.eqMuteBtns()) == 0 || dv.eqMuteBtns()[0] == nil {
		t.Skip("EQ mute buttons not initialized")
	}

	// Start on Master channel
	dv.setEQActiveChannel(channel.Master)
	if dv.activeEQChannel() != channel.Master {
		t.Fatalf("active channel should be 'main', got %q", dv.activeEQChannel())
	}

	// Mute band 0 on Master
	dv.eqBandMuted()[0] = true
	// Re-sync after manual change
	dv.setEQActiveChannel(channel.Master)

	// Verify button 0 shows muted (active style)
	if dv.eqMuteBtns()[0].Style != EQMuteButtonActiveStyle {
		t.Error("band 0 button should show active style when Master band 0 is muted")
	}

	// Switch to Kick channel (which has band 0 unmuted)
	dv.setEQActiveChannel(channel.Of(roster.Kick))
	if dv.activeEQChannel() != channel.Of(roster.Kick) {
		t.Fatalf("active channel should be 'kick', got %q", dv.activeEQChannel())
	}

	// Verify button 0 now shows unmuted (Kick's state)
	if dv.eqMuteBtns()[0].Style != EQMuteButtonStyle {
		t.Error("band 0 button should show normal style when Kick band 0 is unmuted")
	}

	// Mute band 1 on Kick
	rowEQ(dv.Rows[0]).BandMuted[1] = true
	// Re-sync after manual change
	dv.setEQActiveChannel(channel.Of(roster.Kick))

	// Verify button 1 shows muted on Kick
	if dv.eqMuteBtns()[1].Style != EQMuteButtonActiveStyle {
		t.Error("band 1 button should show active style when Kick band 1 is muted")
	}

	// Switch back to Master
	dv.setEQActiveChannel(channel.Master)

	// Verify band 0 still shows muted (Master's state)
	if dv.eqMuteBtns()[0].Style != EQMuteButtonActiveStyle {
		t.Error("band 0 button should show active style when back on Master (band 0 muted)")
	}

	// Verify band 1 shows unmuted (Master band 1 is not muted)
	if dv.eqMuteBtns()[1].Style != EQMuteButtonStyle {
		t.Error("band 1 button should show normal style when back on Master (band 1 unmuted)")
	}
}

// Inside the short-panel overlap the mute (zIdx+4) wins over the dB editor (zIdx+3).
func TestEQ_MutePressWinsOverDBReadoutOverlap(t *testing.T) {
	assertDefaultParityState(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	dv := g.drum
	dv.recalcButtons()
	dv.calcLayout()

	z := dv.eqPanelZone
	if z == nil {
		t.Fatal("eqPanelZone nil")
	}
	// A band whose mute centre falls inside its dB readout.
	band := -1
	var cx, cy int
	btns := dv.eqMuteBtns()
	for i := range btns {
		if btns[i] == nil {
			continue
		}
		mr := btns[i].Rect()
		dr := z.dbReadoutRect(i)
		if mr.Empty() || dr.Empty() {
			continue
		}
		c := image.Pt((mr.Min.X+mr.Max.X)/2, (mr.Min.Y+mr.Max.Y)/2)
		if c.In(dr) {
			band, cx, cy = i, c.X, c.Y
			break
		}
	}
	if band < 0 {
		t.Skip("no mute/dB overlap in this layout")
	}
	if dv.eqBandMuted()[band] {
		t.Fatalf("band %d should start unmuted", band)
	}

	rp := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	for i := 0; i < 5; i++ {
		dv.Update()
	}
	rp()

	if !dv.eqBandMuted()[band] {
		t.Errorf("press in mute/dB overlap must toggle mute, not open the dB wheel (band %d)", band)
	}
	if dv.eqWheelPopup != nil && dv.eqWheelPopup.IsOpen() {
		t.Errorf("dB wheel must NOT open on a mute-button press in the overlap region")
	}
}
