//go:build test

package ui

import (
	"image"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	gamelog "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestMobileRenameUpdatesLabel reproduces the real mobile rename timing issue:
// on mobile, mobileInputActive() returns false when Open() is called synchronously
// from the edit button's OnClick handler because the native HTML input isn't
// registered/active yet. So Open() falls through to desktop mode (TextInput),
// and the mobile input result is never polled.
func TestMobileRenameUpdatesLabel(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)
	withSmallScreen(t, true)

	logger := gamelog.New(testLogOutput(), gamelog.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 600), nil, logger)

	// Confirm initial row state.
	if len(dv.Rows) == 0 {
		t.Fatal("expected at least one row")
	}
	origName := dv.Rows[0].Name
	origInst := dv.Rows[0].Instrument

	// Run an initial Update to build layout and buttons.
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 400, 600 },
	)
	dv.Update()
	restore()

	// Verify edit button exists.
	if len(dv.rowEditBtns()) == 0 {
		t.Fatal("no edit buttons after initial Update")
	}

	// ── Step 1: Click the edit button WITHOUT setting testMobileInputActive ──
	// This reproduces the real mobile timing: native input isn't active yet
	// when Open() runs, so mobileInputActive("rename-0") returns false and
	// Open() falls through to desktop mode.
	dv.rowEditBtns()[0].OnClick()

	// ── Step 2: Simulate the native input becoming active AFTER Open() ──
	// On real mobile, the HTML input is created asynchronously. Here we inject
	// the active state and the committed result that the user would type.
	testMobileInputActive = map[string]bool{"rename-0": true}
	testMobileInputResult = map[string]*struct {
		Value     string
		Committed bool
	}{
		"rename-0": {Value: "NewKick", Committed: true},
	}
	defer func() {
		testMobileInputActive = nil
		testMobileInputResult = nil
	}()

	// ── Step 3: Run several Update frames with no keyboard ──
	// On mobile there is no physical keyboard, so no Enter key is pressed.
	// The rename should be handled by polling the mobile input result.
	for i := 0; i < 10; i++ {
		restore = SetInputForTest(
			func() (int, int) { return 0, 0 },
			func(b ebiten.MouseButton) bool { return false },
			func(k ebiten.Key) bool { return false },
			func() []rune { return nil },
			func() (float64, float64) { return 0, 0 },
			func() (int, int) { return 400, 600 },
		)
		dv.Update()
		restore()
	}

	// ── Step 4: Assert the rename took effect ──
	// These should all FAIL because Open() entered desktop mode and never
	// polls the mobile input result.
	if dv.Rows[0].Name != "NewKick" {
		t.Errorf("Rows[0].Name = %q, want %q (was %q)", dv.Rows[0].Name, "NewKick", origName)
	}
	if len(dv.rowLabels()) > 0 && dv.rowLabels()[0].Text != "NewKick" {
		t.Errorf("rowLabels[0].Text = %q, want %q", dv.rowLabels()[0].Text, "NewKick")
	}
	if dv.Rows[0].Instrument != "newkick" {
		t.Errorf("Rows[0].Instrument = %q, want %q (was %q)", dv.Rows[0].Instrument, "newkick", origInst)
	}

	// Verify notification was shown.
	found := false
	for _, n := range dv.notifStore.History() {
		if strings.Contains(n.text, "NewKick") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected info notification containing 'NewKick', got %v", dv.notifStore.History())
	}
}

// TestRenameInvalidCharsShowsError verifies that instrument names containing
// invalid characters (slashes, angle brackets, null bytes, backslashes) are
// rejected with an error notification. Currently, the OnCommit callback only
// does TrimSpace + empty check, so invalid names are silently accepted.
func TestRenameInvalidCharsShowsError(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)

	// Use a known instrument so the rename flow works.
	audio.ResetCatalogForTest([]audio.SoundMeta{{ID: "snare", Name: "Snare"}})

	invalidNames := []struct {
		name string
		desc string
	}{
		{"Kick/2", "forward slash"},
		{"Kick<>", "angle brackets"},
		{"Hi\x00Hat", "null byte"},
		{"Snare\\1", "backslash"},
	}

	for _, tc := range invalidNames {
		t.Run(tc.desc, func(t *testing.T) {
			logger := gamelog.New(testLogOutput(), gamelog.LevelError)
			dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, logger)

			origName := dv.Rows[0].Name
			origInst := dv.Rows[0].Instrument

			// Initial Update to build layout/buttons.
			restore := SetInputForTest(
				func() (int, int) { return 0, 0 },
				func(b ebiten.MouseButton) bool { return false },
				func(k ebiten.Key) bool { return false },
				func() []rune { return nil },
				func() (float64, float64) { return 0, 0 },
				func() (int, int) { return 800, 600 },
			)
			dv.Update()
			restore()

			if len(dv.rowEditBtns()) == 0 {
				t.Fatal("no edit buttons after initial Update")
			}

			// Click edit button to open rename dialog (desktop mode).
			dv.rowEditBtns()[0].OnClick()

			if dv.renameBox == nil {
				t.Fatal("renameBox not created after edit click")
			}

			// Set the text to an invalid name.
			dv.renameBox.SetText(tc.name)

			// Simulate Enter key press to commit.
			enterPressed := true
			restore = SetInputForTest(
				func() (int, int) {
					if dv.renameBox == nil {
						return 0, 0
					}
					return dv.renameBox.Rect.Min.X + 1, dv.renameBox.Rect.Min.Y + 1
				},
				func(b ebiten.MouseButton) bool { return false },
				func(k ebiten.Key) bool {
					if k == ebiten.KeyEnter && enterPressed {
						return true
					}
					return false
				},
				func() []rune { return nil },
				func() (float64, float64) { return 0, 0 },
				func() (int, int) { return 800, 600 },
			)
			dv.Update()
			enterPressed = false
			restore()

			// Run another update to process the commit.
			restore = SetInputForTest(
				func() (int, int) { return 0, 0 },
				func(b ebiten.MouseButton) bool { return false },
				func(k ebiten.Key) bool { return false },
				func() []rune { return nil },
				func() (float64, float64) { return 0, 0 },
				func() (int, int) { return 800, 600 },
			)
			dv.Update()
			restore()

			// The name should NOT have changed — invalid chars should be rejected.
			if dv.Rows[0].Name != origName {
				t.Errorf("Rows[0].Name = %q, want %q (invalid name %q was accepted)", dv.Rows[0].Name, origName, tc.name)
			}
			if dv.Rows[0].Instrument != origInst {
				t.Errorf("Rows[0].Instrument = %q, want %q (invalid name %q was accepted)", dv.Rows[0].Instrument, origInst, tc.name)
			}

			// An error notification should have been shown.
			hasErr := false
			for _, n := range dv.notifStore.History() {
				if n.isErr {
					hasErr = true
					break
				}
			}
			if !hasErr {
				t.Errorf("expected error notification for invalid name %q, got notifs=%v", tc.name, dv.notifStore.History())
			}
		})
	}
}
