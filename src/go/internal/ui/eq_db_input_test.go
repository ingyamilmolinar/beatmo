//go:build test

package ui

import (
	"image"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// --- EQ dB text input tests ---

func TestEQDBInput_InitialValues(t *testing.T) {
	assertDefaultParityState(t)
	z, _ := newTestEQPanelZone(nil)
	z.Layout(image.Rect(0, 400, 600, 580))

	for i, ti := range z.eqDBInputs {
		if ti == nil {
			t.Fatalf("eqDBInputs[%d] is nil", i)
		}
		if got := ti.Value(); got != "0.0" {
			t.Errorf("eqDBInputs[%d] = %q, want %q", i, got, "0.0")
		}
	}
}

func TestEQDBInput_CurveDragSyncsText(t *testing.T) {
	assertDefaultParityState(t)
	restore := noInputForTest()
	defer restore()

	z, _ := newTestEQPanelZone(nil)
	r := image.Rect(0, 400, 600, 580)
	_ = registerEQZone(z, r)

	// Simulate curve drag on band 3: set gain directly and call syncDBInputText.
	z.bandGainsDB[3] = 6.0
	z.syncDBInputText(3)

	got := z.eqDBInputs[3].Value()
	if got != "+6.0" {
		t.Errorf("after curve drag sync, dB text = %q, want %q", got, "+6.0")
	}
}

func TestEQDBInput_TextCommitSyncsCurve(t *testing.T) {
	assertDefaultParityState(t)
	restore := noInputForTest()
	defer restore()

	z, log := newTestEQPanelZone(nil)
	r := image.Rect(0, 400, 600, 580)
	_ = registerEQZone(z, r)

	// Simulate: focus gained, type value, then commit.
	z.eqDBInputs[5].SetText("-3.2")
	z.dbInputPrev = 0
	z.commitDBText(5)

	if math.Abs(z.bandGainsDB[5]-(-3.2)) > 0.01 {
		t.Errorf("bandGainsDB[5] = %f, want -3.2", z.bandGainsDB[5])
	}
	if len(log.gainChanges) == 0 {
		t.Fatal("expected OnGainChange callback to fire")
	}
	if log.applyCount == 0 {
		t.Fatal("expected OnApplyEQ callback to fire")
	}
}

func TestEQDBInput_ValidationClamp(t *testing.T) {
	assertDefaultParityState(t)
	restore := noInputForTest()
	defer restore()

	z, _ := newTestEQPanelZone(nil)
	r := image.Rect(0, 400, 600, 580)
	_ = registerEQZone(z, r)

	// Enter +15.0 → should clamp to +12.0.
	z.eqDBInputs[0].SetText("+15.0")
	z.dbInputPrev = 0
	z.commitDBText(0)

	if z.bandGainsDB[0] != 12.0 {
		t.Errorf("bandGainsDB[0] = %f, want 12.0 (clamped)", z.bandGainsDB[0])
	}
	if z.eqDBInputs[0].Value() != "+12.0" {
		t.Errorf("text = %q, want %q", z.eqDBInputs[0].Value(), "+12.0")
	}
}

func TestEQDBInput_InvalidTextReverts(t *testing.T) {
	assertDefaultParityState(t)
	restore := noInputForTest()
	defer restore()

	z, _ := newTestEQPanelZone(nil)
	r := image.Rect(0, 400, 600, 580)
	_ = registerEQZone(z, r)

	z.bandGainsDB[2] = 4.5
	z.dbInputPrev = 4.5
	z.eqDBInputs[2].SetText("abc")
	z.commitDBText(2)

	// Should revert to previous.
	if z.eqDBInputs[2].Value() != "+4.5" {
		t.Errorf("text = %q, want %q (reverted)", z.eqDBInputs[2].Value(), "+4.5")
	}
}

func TestEQDBInput_EmptyTextReverts(t *testing.T) {
	assertDefaultParityState(t)
	restore := noInputForTest()
	defer restore()

	z, _ := newTestEQPanelZone(nil)
	r := image.Rect(0, 400, 600, 580)
	_ = registerEQZone(z, r)

	z.bandGainsDB[7] = -2.0
	z.dbInputPrev = -2.0
	z.eqDBInputs[7].SetText("")
	z.commitDBText(7)

	if z.eqDBInputs[7].Value() != "-2.0" {
		t.Errorf("text = %q, want %q (reverted)", z.eqDBInputs[7].Value(), "-2.0")
	}
}

func TestEQDBInput_EscapeReverts(t *testing.T) {
	assertDefaultParityState(t)
	restore := noInputForTest()
	defer restore()

	z, _ := newTestEQPanelZone(nil)
	r := image.Rect(0, 400, 600, 580)
	_ = registerEQZone(z, r)

	// Simulate focus on band 4.
	z.bandGainsDB[4] = 3.0
	z.syncDBInputText(4)
	z.dbInputPrev = 3.0
	z.dbInputFocused = 4
	z.eqDBInputs[4].focused = true
	z.eqDBInputs[4].SetText("9.9") // user typed something

	// Press Escape.
	result := z.HandleKey(ebiten.KeyEscape)
	if result != InputConsumed {
		t.Errorf("HandleKey(Escape) = %d, want InputConsumed", result)
	}
	if z.eqDBInputs[4].Focused() {
		t.Error("input should be blurred after Escape")
	}
	if z.eqDBInputs[4].Value() != "+3.0" {
		t.Errorf("text = %q, want %q (reverted)", z.eqDBInputs[4].Value(), "+3.0")
	}
}

func TestEQDBInput_EnterCommits(t *testing.T) {
	assertDefaultParityState(t)
	restore := noInputForTest()
	defer restore()

	z, log := newTestEQPanelZone(nil)
	r := image.Rect(0, 400, 600, 580)
	_ = registerEQZone(z, r)

	z.dbInputPrev = 0
	z.dbInputFocused = 6
	z.eqDBInputs[6].focused = true
	z.eqDBInputs[6].SetText("-8.5")

	result := z.HandleKey(ebiten.KeyEnter)
	if result != InputConsumed {
		t.Errorf("HandleKey(Enter) = %d, want InputConsumed", result)
	}
	if z.eqDBInputs[6].Focused() {
		t.Error("input should be blurred after Enter")
	}
	if math.Abs(z.bandGainsDB[6]-(-8.5)) > 0.01 {
		t.Errorf("bandGainsDB[6] = %f, want -8.5", z.bandGainsDB[6])
	}
	if len(log.gainChanges) == 0 {
		t.Fatal("expected OnGainChange to fire on Enter commit")
	}
}

func TestEQDBInput_OnlyOneFocusedAtOnce(t *testing.T) {
	assertDefaultParityState(t)
	restore := noInputForTest()
	defer restore()

	z, _ := newTestEQPanelZone(nil)
	r := image.Rect(0, 400, 600, 580)
	_ = registerEQZone(z, r)

	// Manually focus input 3.
	z.eqDBInputs[3].focused = true
	z.dbInputFocused = 3
	z.dbInputPrev = 0

	// Now focus input 7 — input 3 should be committed and blurred.
	z.eqDBInputs[7].focused = true
	z.updateDBInputs()

	if z.eqDBInputs[3].Focused() {
		t.Error("input 3 should be blurred when input 7 gains focus")
	}
	if z.dbInputFocused != 7 {
		t.Errorf("dbInputFocused = %d, want 7", z.dbInputFocused)
	}
}

func TestEQDBInput_SyncBandStateUpdatesTexts(t *testing.T) {
	assertDefaultParityState(t)
	z, _ := newTestEQPanelZone(nil)
	z.Layout(image.Rect(0, 400, 600, 580))

	gains := make([]float64, 10)
	muted := make([]bool, 10)
	for i := range gains {
		gains[i] = float64(i) - 5
	}
	z.SyncBandState(gains, muted)

	for i, ti := range z.eqDBInputs {
		want := formatDB(gains[i])
		if ti.Value() != want {
			t.Errorf("eqDBInputs[%d] = %q, want %q", i, ti.Value(), want)
		}
	}
}

func TestEQDBInput_MutedBandShowsValue(t *testing.T) {
	assertDefaultParityState(t)
	z, _ := newTestEQPanelZone(nil)
	z.Layout(image.Rect(0, 400, 600, 580))

	z.bandGainsDB[2] = 5.0
	z.bandMuted[2] = true
	z.syncDBInputText(2)

	if z.eqDBInputs[2].Value() != "+5.0" {
		t.Errorf("muted band text = %q, want %q", z.eqDBInputs[2].Value(), "+5.0")
	}
}

func TestEQDBInput_HitAreaRegistered(t *testing.T) {
	assertDefaultParityState(t)
	z, _ := newTestEQPanelZone(nil)
	r := image.Rect(0, 400, 600, 580)
	z.Layout(r)

	found := 0
	for _, a := range z.HitAreas() {
		for i := range eqCenterLabels {
			if a.Tag == "eq-db-"+eqCenterLabels[i] {
				found++
				if a.ZIndex != 132 {
					t.Errorf("hit area %q z-index = %d, want 132", a.Tag, a.ZIndex)
				}
			}
		}
	}
	if found != 10 {
		t.Errorf("found %d eq-db-* hit areas, want 10", found)
	}
}

func TestEQDBInput_HandleKeyEnterCommits(t *testing.T) {
	assertDefaultParityState(t)
	restore := noInputForTest()
	defer restore()

	z, log := newTestEQPanelZone(nil)
	_ = registerEQZone(z, image.Rect(0, 400, 600, 580))

	z.dbInputPrev = 0
	z.dbInputFocused = 1
	z.eqDBInputs[1].focused = true
	z.eqDBInputs[1].SetText("7.5")

	r := z.HandleKey(ebiten.KeyEnter)
	if r != InputConsumed {
		t.Errorf("expected InputConsumed, got %d", r)
	}
	if math.Abs(z.bandGainsDB[1]-7.5) > 0.01 {
		t.Errorf("bandGainsDB[1] = %f, want 7.5", z.bandGainsDB[1])
	}
	if log.applyCount == 0 {
		t.Fatal("expected OnApplyEQ to fire")
	}
}

func TestEQDBInput_HandleKeyEscapeReverts(t *testing.T) {
	assertDefaultParityState(t)
	restore := noInputForTest()
	defer restore()

	z, _ := newTestEQPanelZone(nil)
	_ = registerEQZone(z, image.Rect(0, 400, 600, 580))

	z.bandGainsDB[9] = -6.0
	z.dbInputPrev = -6.0
	z.dbInputFocused = 9
	z.eqDBInputs[9].focused = true
	z.eqDBInputs[9].SetText("0")

	r := z.HandleKey(ebiten.KeyEscape)
	if r != InputConsumed {
		t.Errorf("expected InputConsumed, got %d", r)
	}
	if z.eqDBInputs[9].Value() != "-6.0" {
		t.Errorf("text = %q, want %q", z.eqDBInputs[9].Value(), "-6.0")
	}
	// bandGainsDB should not have changed (no commit happened).
	if z.bandGainsDB[9] != -6.0 {
		t.Errorf("bandGainsDB[9] = %f, want -6.0 (unchanged)", z.bandGainsDB[9])
	}
}

// TestEQDBInput_DirectBandGainsDBMutationSyncsTexts reproduces the bug where
// importing a circuit writes non-zero gains directly into the shared bandGainsDB
// backing array, then calls setEQActiveChannel("main") which hits the
// "same master, no channel change" branch. That branch reads the zone's own
// bandGainsDB (which now has non-zero values) but never calls SyncBandState
// or syncAllDBInputTexts, leaving dB texts stale at "0.0".
func TestEQDBInput_DirectBandGainsDBMutationSyncsTexts(t *testing.T) {
	assertDefaultParityState(t)
	restore := noInputForTest()
	defer restore()

	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	z := g.drum.eqPanelZone
	if z == nil {
		t.Fatal("eqPanelZone is nil")
	}

	// Verify initial state: all "0.0".
	for i, ti := range z.eqDBInputs {
		if ti.Value() != "0.0" {
			t.Fatalf("initial eqDBInputs[%d] = %q, want %q", i, ti.Value(), "0.0")
		}
	}

	// Simulate the import path: write non-zero gains directly into the
	// zone's bandGainsDB via the shared backing array (import.go:178).
	z.bandGainsDB[0] = 6.0
	z.bandGainsDB[3] = -4.5
	z.bandGainsDB[9] = 12.0

	// Now call setEQActiveChannel("main") — same channel, no change.
	// This is what import.go:203 does after writing the gains.
	g.drum.setEQActiveChannel("main")

	// Bug: dB texts still show "0.0" because the "same master" branch
	// never syncs the text inputs.
	want := map[int]string{0: "+6.0", 3: "-4.5", 9: "+12.0"}
	for band, expected := range want {
		if got := z.eqDBInputs[band].Value(); got != expected {
			t.Errorf("eqDBInputs[%d] = %q, want %q", band, got, expected)
		}
	}
}

func TestEQDBInput_NoBiDirectionalLoop(t *testing.T) {
	assertDefaultParityState(t)
	restore := noInputForTest()
	defer restore()

	z, log := newTestEQPanelZone(nil)
	_ = registerEQZone(z, image.Rect(0, 400, 600, 580))

	// Set gain via text commit.
	z.dbInputPrev = 0
	z.eqDBInputs[0].SetText("5.0")
	z.commitDBText(0)

	// The commit itself fires OnGainChange once.
	initialGainChanges := len(log.gainChanges)
	if initialGainChanges != 1 {
		t.Fatalf("expected 1 gain change after commit, got %d", initialGainChanges)
	}

	// Sync back should not trigger another callback.
	z.syncDBInputText(0)
	if len(log.gainChanges) != initialGainChanges {
		t.Errorf("syncDBInputText triggered extra gain change: had %d, now %d",
			initialGainChanges, len(log.gainChanges))
	}
}
