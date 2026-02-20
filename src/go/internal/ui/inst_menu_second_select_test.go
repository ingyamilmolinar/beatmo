//go:build test

package ui

import (
	"image"
	"testing"
)

// TestInstrumentMenuSecondSelectionUpdatesLabel verifies that selecting an
// instrument from the dropdown menu updates the row label button on both the
// first and subsequent selections.
//
// This tests a specific bug where the first selection worked correctly but the
// second selection would leave the row label showing the previous instrument
// name even though dv.Rows[idx].Instrument was updated correctly.
func TestInstrumentMenuSecondSelectionUpdatesLabel(t *testing.T) {
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	// Create a DrumView with one row
	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.instOptions = []string{"kick", "snare", "hihat"}
	dv.instCategories = nil // No categories for simplicity
	dv.instCatByID = nil

	// Initialize with a single row
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	// Force layout calculation to create the row label buttons
	dv.bgDirty = true
	dv.calcLayout()
	dv.bgDirty = false

	if len(dv.rowLabels()) == 0 {
		t.Fatal("rowLabels not created after calcLayout")
	}

	// Verify initial state
	if dv.Rows[0].Instrument != "kick" {
		t.Fatalf("expected initial instrument 'kick', got %q", dv.Rows[0].Instrument)
	}
	if dv.rowLabels()[0].Text != "Kick" {
		t.Fatalf("expected initial label 'Kick', got %q", dv.rowLabels()[0].Text)
	}

	// Create the instrument menu component
	dv.instMenuComp = NewInstrumentMenuComponent()

	// === FIRST SELECTION: Change from kick to snare ===
	t.Log("=== FIRST SELECTION: kick -> snare ===")

	// Simulate clicking the row label to open the menu
	// This triggers lbl.OnClick which sets selRow and opens the menu
	dv.selRow = 0
	dv.instMenuRow = 0

	// Build instrument options for the menu
	var instOpts []InstrumentOption
	for _, id := range dv.instOptions {
		instOpts = append(instOpts, InstrumentOption{
			ID:    id,
			Label: id,
		})
	}

	// Set props and open the menu (simulating what lbl.OnClick does)
	dv.instMenuComp.SetProps(InstrumentMenuProps{
		AnchorRect:        dv.rowLabels()[0].Rect(),
		VertBounds:        dv.Bounds,
		RowIndex:          0,
		CurrentInstrument: dv.Rows[0].Instrument,
		Instruments:       instOpts,
		RowHeight:         24,
		LabelWidth:        100,
		ControlsWidth:     100,
		OnSelect: func(instID string) {
			dv.SetInstrument(instID)
		},
	})
	dv.instMenuComp.Open()
	dv.openInstMenuPortal()
	suppressClicksUntilRelease = false

	// Find and click the "snare" button
	var snareBtn *Button
	for _, btn := range dv.instMenuComp.InstBtns() {
		if btn.Text == "snare" {
			snareBtn = btn
			break
		}
	}
	if snareBtn == nil {
		t.Fatal("could not find snare button in menu")
	}

	// Click the snare button to select it
	rect := snareBtn.Rect()
	dv.instMenuComp.HandleInput(rect.Min.X+5, rect.Min.Y+5, true)
	suppressClicksUntilRelease = false
	dv.instMenuComp.HandleInput(rect.Min.X+5, rect.Min.Y+5, false)

	// Verify the selection was applied
	if dv.Rows[0].Instrument != "snare" {
		t.Fatalf("after first selection: expected instrument 'snare', got %q", dv.Rows[0].Instrument)
	}
	if dv.Rows[0].Name != "Snare" {
		t.Fatalf("after first selection: expected Name 'Snare', got %q", dv.Rows[0].Name)
	}

	// Simulate an update cycle (bgDirty triggers calcLayout)
	if dv.bgDirty {
		dv.calcLayout()
		dv.bgDirty = false
	}

	// Verify the label was updated after calcLayout
	if dv.rowLabels()[0].Text != "Snare" {
		t.Fatalf("after first selection + calcLayout: expected label 'Snare', got %q", dv.rowLabels()[0].Text)
	}

	t.Logf("First selection successful: Rows[0].Instrument=%q, Rows[0].Name=%q, rowLabels[0].Text=%q",
		dv.Rows[0].Instrument, dv.Rows[0].Name, dv.rowLabels()[0].Text)

	// === SECOND SELECTION: Change from snare to hihat ===
	t.Log("=== SECOND SELECTION: snare -> hihat ===")

	// Close the old portal entry before reopening to avoid the OnClose
	// callback (which fires during closeByID) from closing the newly-opened comp.
	dv.closeInstMenuPortal()

	// Simulate clicking the row label again to open the menu
	dv.selRow = 0
	dv.instMenuRow = 0

	// Update props for the menu with current instrument
	dv.instMenuComp.SetProps(InstrumentMenuProps{
		AnchorRect:        dv.rowLabels()[0].Rect(),
		VertBounds:        dv.Bounds,
		RowIndex:          0,
		CurrentInstrument: dv.Rows[0].Instrument, // Should be "snare" now
		Instruments:       instOpts,
		RowHeight:         24,
		LabelWidth:        100,
		ControlsWidth:     100,
		OnSelect: func(instID string) {
			dv.SetInstrument(instID)
		},
	})
	dv.instMenuComp.Open()
	dv.openInstMenuPortal()
	suppressClicksUntilRelease = false

	// Find and click the "hihat" button
	var hihatBtn *Button
	for _, btn := range dv.instMenuComp.InstBtns() {
		if btn.Text == "hihat" {
			hihatBtn = btn
			break
		}
	}
	if hihatBtn == nil {
		t.Fatal("could not find hihat button in menu")
	}

	// Click the hihat button to select it
	rect = hihatBtn.Rect()
	dv.instMenuComp.HandleInput(rect.Min.X+5, rect.Min.Y+5, true)
	suppressClicksUntilRelease = false
	dv.instMenuComp.HandleInput(rect.Min.X+5, rect.Min.Y+5, false)

	// Verify the selection was applied to Rows
	if dv.Rows[0].Instrument != "hihat" {
		t.Fatalf("after second selection: expected instrument 'hihat', got %q", dv.Rows[0].Instrument)
	}
	if dv.Rows[0].Name != "Hihat" {
		t.Fatalf("after second selection: expected Name 'Hihat', got %q", dv.Rows[0].Name)
	}

	// Before calcLayout, the label should already be updated by SetInstrument
	if dv.rowLabels()[0].Text != "Hihat" {
		t.Logf("WARNING: rowLabels[0].Text=%q before calcLayout (expected 'Hihat')", dv.rowLabels()[0].Text)
	}

	// Simulate an update cycle (bgDirty triggers calcLayout)
	if dv.bgDirty {
		dv.calcLayout()
		dv.bgDirty = false
	}

	// THIS IS THE BUG: Verify the label was updated after calcLayout
	if dv.rowLabels()[0].Text != "Hihat" {
		t.Fatalf("BUG REPRODUCED: after second selection + calcLayout: expected label 'Hihat', got %q\n"+
			"Rows[0].Instrument=%q, Rows[0].Name=%q",
			dv.rowLabels()[0].Text, dv.Rows[0].Instrument, dv.Rows[0].Name)
	}

	t.Logf("Second selection successful: Rows[0].Instrument=%q, Rows[0].Name=%q, rowLabels[0].Text=%q",
		dv.Rows[0].Instrument, dv.Rows[0].Name, dv.rowLabels()[0].Text)
}

// TestInstrumentMenuThirdSelectionUpdatesLabel extends the test to a third
// selection to ensure the pattern holds for multiple subsequent selections.
func TestInstrumentMenuThirdSelectionUpdatesLabel(t *testing.T) {
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.instOptions = []string{"kick", "snare", "hihat", "tom"}
	dv.instCategories = nil
	dv.instCatByID = nil

	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	dv.bgDirty = true
	dv.calcLayout()
	dv.bgDirty = false

	if len(dv.rowLabels()) == 0 {
		t.Fatal("rowLabels not created after calcLayout")
	}

	dv.instMenuComp = NewInstrumentMenuComponent()

	var instOpts []InstrumentOption
	for _, id := range dv.instOptions {
		instOpts = append(instOpts, InstrumentOption{
			ID:    id,
			Label: id,
		})
	}

	// Helper to select an instrument
	selectInstrument := func(instID string) {
		// Close old portal entry before reopening to avoid OnClose callback
		// from closing the newly-opened comp (closeByID fires old OnClose).
		dv.closeInstMenuPortal()

		dv.selRow = 0
		dv.instMenuRow = 0

		dv.instMenuComp.SetProps(InstrumentMenuProps{
			AnchorRect:        dv.rowLabels()[0].Rect(),
			VertBounds:        dv.Bounds,
			RowIndex:          0,
			CurrentInstrument: dv.Rows[0].Instrument,
			Instruments:       instOpts,
			RowHeight:         24,
			LabelWidth:        100,
			ControlsWidth:     100,
			OnSelect: func(id string) {
				dv.SetInstrument(id)
			},
		})
		dv.instMenuComp.Open()
		dv.openInstMenuPortal()
		suppressClicksUntilRelease = false

		var btn *Button
		for _, b := range dv.instMenuComp.InstBtns() {
			if b.Text == instID {
				btn = b
				break
			}
		}
		if btn == nil {
			t.Fatalf("could not find %s button in menu", instID)
		}

		rect := btn.Rect()
		dv.instMenuComp.HandleInput(rect.Min.X+5, rect.Min.Y+5, true)
		suppressClicksUntilRelease = false
		dv.instMenuComp.HandleInput(rect.Min.X+5, rect.Min.Y+5, false)

		if dv.bgDirty {
			dv.calcLayout()
			dv.bgDirty = false
		}
	}

	// Selection 1: kick -> snare
	selectInstrument("snare")
	if dv.rowLabels()[0].Text != "Snare" {
		t.Fatalf("after selection 1: expected label 'Snare', got %q", dv.rowLabels()[0].Text)
	}

	// Selection 2: snare -> hihat
	selectInstrument("hihat")
	if dv.rowLabels()[0].Text != "Hihat" {
		t.Fatalf("after selection 2: expected label 'Hihat', got %q", dv.rowLabels()[0].Text)
	}

	// Selection 3: hihat -> tom
	selectInstrument("tom")
	if dv.rowLabels()[0].Text != "Tom" {
		t.Fatalf("after selection 3: expected label 'Tom', got %q", dv.rowLabels()[0].Text)
	}

	// Selection 4: tom -> kick (back to original)
	selectInstrument("kick")
	if dv.rowLabels()[0].Text != "Kick" {
		t.Fatalf("after selection 4: expected label 'Kick', got %q", dv.rowLabels()[0].Text)
	}

	t.Log("All selections successful")
}

// TestInstrumentMenuSecondSelectionViaRowLabelClick tests the bug using the
// actual click flow through the row label buttons rather than directly
// manipulating the component state. This more closely matches the real user
// interaction path.
func TestInstrumentMenuSecondSelectionViaRowLabelClick(t *testing.T) {
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.instOptions = []string{"kick", "snare", "hihat"}
	dv.instCategories = nil
	dv.instCatByID = nil

	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	dv.bgDirty = true
	dv.calcLayout()
	dv.bgDirty = false

	if len(dv.rowLabels()) == 0 {
		t.Fatal("rowLabels not created after calcLayout")
	}

	// Create the instrument menu component
	dv.instMenuComp = NewInstrumentMenuComponent()

	// Helper to simulate clicking a row label and selecting an instrument
	// This follows the actual code path from drumview_layout.go
	// buttonLabel is the display label on the button (e.g., "Snare")
	// expectedID is the instrument ID (e.g., "snare")
	// expectedLabel is what the row label should show after selection
	selectInstrumentViaUI := func(buttonLabel, expectedID, expectedLabel string) {
		t.Helper()

		// Get the row label button and simulate clicking it
		// This triggers lbl.OnClick (from drumview_layout.go:387)
		labelBtn := dv.rowLabels()[0]
		if labelBtn.OnClick == nil {
			t.Fatal("row label button has no OnClick handler")
		}

		// Call OnClick directly - this is what happens when user clicks the label
		labelBtn.OnClick()
		suppressClicksUntilRelease = false

		if !dv.IsInstMenuOpen() && (dv.instMenuComp == nil || !dv.instMenuComp.IsOpen()) {
			t.Fatal("menu should be open after clicking row label")
		}

		// Find the target instrument button in the menu by its display label
		var targetBtn *Button
		if dv.instMenuComp != nil && dv.instMenuComp.IsOpen() {
			btns := dv.instMenuComp.InstBtns()
			for _, btn := range btns {
				if btn.Text == buttonLabel {
					targetBtn = btn
					break
				}
			}
		} else {
			for _, btn := range dv.instMenuBtns {
				if btn.Text == buttonLabel {
					targetBtn = btn
					break
				}
			}
		}
		if targetBtn == nil {
			t.Fatalf("could not find button with label %q in menu", buttonLabel)
		}

		// Click the instrument button
		rect := targetBtn.Rect()
		if dv.instMenuComp != nil && dv.instMenuComp.IsOpen() {
			dv.instMenuComp.HandleInput(rect.Min.X+5, rect.Min.Y+5, true)
			suppressClicksUntilRelease = false
			dv.instMenuComp.HandleInput(rect.Min.X+5, rect.Min.Y+5, false)
		} else if targetBtn.OnClick != nil {
			targetBtn.OnClick()
		}

		// Process any pending layout updates (simulates Update() cycle)
		if dv.bgDirty {
			dv.calcLayout()
			dv.bgDirty = false
		}

		// Verify the selection
		if dv.Rows[0].Instrument != expectedID {
			t.Fatalf("expected instrument %q, got %q", expectedID, dv.Rows[0].Instrument)
		}
		if dv.rowLabels()[0].Text != expectedLabel {
			t.Fatalf("expected label %q, got %q (Rows[0].Name=%q)",
				expectedLabel, dv.rowLabels()[0].Text, dv.Rows[0].Name)
		}
	}

	// First selection: kick -> snare
	// Button label is "Snare", ID is "snare", row label should show "Snare"
	t.Log("=== Selection 1: kick -> snare ===")
	selectInstrumentViaUI("Snare", "snare", "Snare")
	t.Logf("After selection 1: label=%q, instrument=%q", dv.rowLabels()[0].Text, dv.Rows[0].Instrument)

	// Second selection: snare -> hihat
	t.Log("=== Selection 2: snare -> hihat ===")
	selectInstrumentViaUI("Hihat", "hihat", "Hihat")
	t.Logf("After selection 2: label=%q, instrument=%q", dv.rowLabels()[0].Text, dv.Rows[0].Instrument)

	// Third selection: hihat -> kick
	t.Log("=== Selection 3: hihat -> kick ===")
	selectInstrumentViaUI("Kick", "kick", "Kick")
	t.Logf("After selection 3: label=%q, instrument=%q", dv.rowLabels()[0].Text, dv.Rows[0].Instrument)

	t.Log("All selections via UI successful")
}

// TestInstrumentMenuSelectionWithMultipleUpdateCycles tests the selection flow
// with explicit Update() calls to more closely simulate the real runtime behavior.
func TestInstrumentMenuSelectionWithMultipleUpdateCycles(t *testing.T) {
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.instOptions = []string{"kick", "snare", "hihat"}
	dv.instCategories = nil
	dv.instCatByID = nil

	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	// Initial layout
	dv.bgDirty = true
	dv.calcLayout()
	dv.bgDirty = false

	dv.instMenuComp = NewInstrumentMenuComponent()

	// Helper to run a partial update cycle (just the layout part)
	runLayoutUpdate := func() {
		dv.recalcButtons()
		if dv.bgDirty {
			dv.calcLayout()
			dv.bgDirty = false
		}
	}

	// Helper to select an instrument
	selectInstrument := func(buttonLabel, expectedID, expectedLabel string) {
		t.Helper()

		// Open the menu via label click
		labelBtn := dv.rowLabels()[0]
		labelBtn.OnClick()
		suppressClicksUntilRelease = false

		// Find and click the target instrument
		var targetBtn *Button
		for _, btn := range dv.instMenuComp.InstBtns() {
			if btn.Text == buttonLabel {
				targetBtn = btn
				break
			}
		}
		if targetBtn == nil {
			t.Fatalf("could not find button %q", buttonLabel)
		}

		rect := targetBtn.Rect()
		dv.instMenuComp.HandleInput(rect.Min.X+5, rect.Min.Y+5, true)
		suppressClicksUntilRelease = false
		dv.instMenuComp.HandleInput(rect.Min.X+5, rect.Min.Y+5, false)

		// Run update cycle
		runLayoutUpdate()

		// Verify
		if dv.Rows[0].Instrument != expectedID {
			t.Fatalf("instrument mismatch: got %q, want %q", dv.Rows[0].Instrument, expectedID)
		}
		if dv.Rows[0].Name != expectedLabel {
			t.Fatalf("name mismatch: got %q, want %q", dv.Rows[0].Name, expectedLabel)
		}
		if dv.rowLabels()[0].Text != expectedLabel {
			t.Fatalf("label mismatch: got %q, want %q (instrument=%q, name=%q)",
				dv.rowLabels()[0].Text, expectedLabel, dv.Rows[0].Instrument, dv.Rows[0].Name)
		}
	}

	// Selection 1
	t.Log("Selection 1: kick -> snare")
	selectInstrument("Snare", "snare", "Snare")

	// Run multiple update cycles between selections
	for i := 0; i < 5; i++ {
		runLayoutUpdate()
	}

	// Selection 2
	t.Log("Selection 2: snare -> hihat")
	selectInstrument("Hihat", "hihat", "Hihat")

	// Run multiple update cycles
	for i := 0; i < 5; i++ {
		runLayoutUpdate()
	}

	// Selection 3
	t.Log("Selection 3: hihat -> kick")
	selectInstrument("Kick", "kick", "Kick")

	t.Log("All selections with update cycles successful")
}

// TestSetInstrumentInvalidatesRowControlsCache verifies that SetInstrument
// marks the row controls cache as dirty so the label is redrawn.
func TestSetInstrumentInvalidatesRowControlsCache(t *testing.T) {
	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.instOptions = []string{"kick", "snare", "hihat"}

	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8
	dv.selRow = 0

	// Initial layout
	dv.bgDirty = true
	dv.calcLayout()
	dv.bgDirty = false

	// Clear the row controls cache dirty flag to simulate a "clean" state
	// after the initial draw has occurred
	dv.rowControlsCacheDirty = false

	// Now call SetInstrument
	dv.SetInstrument("snare")

	// Verify that the row controls cache is marked dirty
	if !dv.rowControlsCacheDirty {
		t.Fatal("SetInstrument should mark rowControlsCacheDirty = true")
	}

	// Also verify the label text was updated
	if dv.rowLabels()[0].Text != "Snare" {
		t.Fatalf("expected label 'Snare', got %q", dv.rowLabels()[0].Text)
	}
	if dv.Rows[0].Name != "Snare" {
		t.Fatalf("expected name 'Snare', got %q", dv.Rows[0].Name)
	}
}
