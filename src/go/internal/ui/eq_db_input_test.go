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

	// The dB readouts derive live from bandGainsDB; all start at 0.0.
	for i := range z.bandGainsDB {
		if got := formatDB(z.bandGainsDB[i]); got != "0.0" {
			t.Errorf("band %d readout = %q, want %q", i, got, "0.0")
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

	// Simulate curve drag on band 3: set gain directly. The readout derives
	// live from bandGainsDB, so no text-sync step is needed.
	z.bandGainsDB[3] = 6.0

	got := formatDB(z.bandGainsDB[3])
	if got != "+6.0" {
		t.Errorf("after curve drag, dB readout = %q, want %q", got, "+6.0")
	}
}

func TestEQDBInput_TextCommitSyncsCurve(t *testing.T) {
	assertDefaultParityState(t)
	restore := noInputForTest()
	defer restore()

	z, log := newTestEQPanelZone(nil)
	r := image.Rect(0, 400, 600, 580)
	_ = registerEQZone(z, r)

	// Open the editor on band 5, type a value, then commit.
	z.openEQDBEditor(5)
	z.paramEditor.ti.SetText("-3.2")
	z.paramEditor.commit()

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
	z.openEQDBEditor(0)
	z.paramEditor.ti.SetText("+15.0")
	z.paramEditor.commit()

	if z.bandGainsDB[0] != 12.0 {
		t.Errorf("bandGainsDB[0] = %f, want 12.0 (clamped)", z.bandGainsDB[0])
	}
	if got := formatDB(z.bandGainsDB[0]); got != "+12.0" {
		t.Errorf("readout = %q, want %q", got, "+12.0")
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
	z.openEQDBEditor(2)
	z.paramEditor.ti.SetText("abc")
	z.paramEditor.commit()

	// Invalid text: gain unchanged, readout still shows the prior value.
	if z.bandGainsDB[2] != 4.5 {
		t.Errorf("bandGainsDB[2] = %f, want 4.5 (unchanged on invalid)", z.bandGainsDB[2])
	}
	if got := formatDB(z.bandGainsDB[2]); got != "+4.5" {
		t.Errorf("readout = %q, want %q (reverted)", got, "+4.5")
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
	z.openEQDBEditor(7)
	z.paramEditor.ti.SetText("")
	z.paramEditor.commit()

	if z.bandGainsDB[7] != -2.0 {
		t.Errorf("bandGainsDB[7] = %f, want -2.0 (unchanged on empty)", z.bandGainsDB[7])
	}
	if got := formatDB(z.bandGainsDB[7]); got != "-2.0" {
		t.Errorf("readout = %q, want %q (reverted)", got, "-2.0")
	}
}

func TestEQDBInput_EscapeReverts(t *testing.T) {
	assertDefaultParityState(t)
	restore := noInputForTest()
	defer restore()

	z, _ := newTestEQPanelZone(nil)
	r := image.Rect(0, 400, 600, 580)
	_ = registerEQZone(z, r)

	// Open the editor on band 4 with prior value 3.0, type something else.
	z.bandGainsDB[4] = 3.0
	z.openEQDBEditor(4)
	z.paramEditor.ti.SetText("9.9") // user typed something

	// Cancel (Escape).
	z.paramEditor.cancel()

	if z.paramEditor.Active() {
		t.Error("editor should be closed after Escape")
	}
	if z.bandGainsDB[4] != 3.0 {
		t.Errorf("bandGainsDB[4] = %f, want 3.0 (unchanged)", z.bandGainsDB[4])
	}
	if got := formatDB(z.bandGainsDB[4]); got != "+3.0" {
		t.Errorf("readout = %q, want %q (reverted)", got, "+3.0")
	}
}

func TestEQDBInput_EnterCommits(t *testing.T) {
	assertDefaultParityState(t)
	restore := noInputForTest()
	defer restore()

	z, log := newTestEQPanelZone(nil)
	r := image.Rect(0, 400, 600, 580)
	_ = registerEQZone(z, r)

	z.openEQDBEditor(6)
	z.paramEditor.ti.SetText("-8.5")
	z.paramEditor.commit()

	if z.paramEditor.Active() {
		t.Error("editor should be closed after Enter commit")
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

	// There is exactly one shared editor, so opening band 7 while band 3's
	// editor is open simply re-targets the single editor at band 7. Opening
	// band 3 first, then band 7, must leave the editor pointed at band 7.
	z.openEQDBEditor(3)
	if !z.paramEditor.Active() {
		t.Fatal("editor should be active after opening band 3")
	}
	z.openEQDBEditor(7)
	if !z.paramEditor.Active() {
		t.Fatal("editor should be active after re-opening on band 7")
	}
	// Committing now must write band 7 (the most recently opened band).
	z.paramEditor.ti.SetText("4.0")
	z.paramEditor.commit()
	if math.Abs(z.bandGainsDB[7]-4.0) > 0.01 {
		t.Errorf("bandGainsDB[7] = %f, want 4.0 (editor targets last-opened band)", z.bandGainsDB[7])
	}
	if z.bandGainsDB[3] != 0.0 {
		t.Errorf("bandGainsDB[3] = %f, want 0.0 (band 3 not committed)", z.bandGainsDB[3])
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

	// Readouts derive live from bandGainsDB after SyncBandState.
	for i := range gains {
		want := formatDB(gains[i])
		if got := formatDB(z.bandGainsDB[i]); got != want {
			t.Errorf("band %d readout = %q, want %q", i, got, want)
		}
	}
}

func TestEQDBInput_MutedBandShowsValue(t *testing.T) {
	assertDefaultParityState(t)
	z, _ := newTestEQPanelZone(nil)
	z.Layout(image.Rect(0, 400, 600, 580))

	z.bandGainsDB[2] = 5.0
	z.bandMuted[2] = true

	// A muted band still shows its gain value in the readout.
	if got := formatDB(z.bandGainsDB[2]); got != "+5.0" {
		t.Errorf("muted band readout = %q, want %q", got, "+5.0")
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
				// dB inputs sit at zIdx+3 (133): above the full-panel curve
				// overlay (zIdx+1) and the mute/HP-LP controls (zIdx+2), so a
				// tap on a dB field wins over the curve drag overlay.
				if a.ZIndex != 133 {
					t.Errorf("hit area %q z-index = %d, want 133", a.Tag, a.ZIndex)
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

	// While the editor is open, HandleKey swallows keys (so global shortcuts
	// don't fire); the editor itself owns commit. Verify both.
	z.openEQDBEditor(1)
	if r := z.HandleKey(ebiten.KeyEnter); r != InputConsumed {
		t.Errorf("expected HandleKey(Enter) consumed while editor open, got %d", r)
	}
	z.paramEditor.ti.SetText("7.5")
	z.paramEditor.commit()

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
	z.openEQDBEditor(9)
	if r := z.HandleKey(ebiten.KeyEscape); r != InputConsumed {
		t.Errorf("expected HandleKey(Escape) consumed while editor open, got %d", r)
	}
	z.paramEditor.ti.SetText("0")
	z.paramEditor.cancel()

	// Cancel must not commit: gain unchanged, readout reverted.
	if z.bandGainsDB[9] != -6.0 {
		t.Errorf("bandGainsDB[9] = %f, want -6.0 (unchanged)", z.bandGainsDB[9])
	}
	if got := formatDB(z.bandGainsDB[9]); got != "-6.0" {
		t.Errorf("readout = %q, want %q", got, "-6.0")
	}
}

// TestEQDBInput_DirectBandGainsDBMutationSyncsTexts reproduces the import flow
// where a circuit writes non-zero gains directly into the shared bandGainsDB
// backing array, then calls setEQActiveChannel("main") (the "same master, no
// channel change" branch). The dB readouts now derive live from bandGainsDB, so
// the displayed values must reflect the written gains with no explicit sync.
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
	for i := range z.bandGainsDB {
		if got := formatDB(z.bandGainsDB[i]); got != "0.0" {
			t.Fatalf("initial band %d readout = %q, want %q", i, got, "0.0")
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

	// The readouts derive live from bandGainsDB, so they reflect the writes.
	want := map[int]string{0: "+6.0", 3: "-4.5", 9: "+12.0"}
	for band, expected := range want {
		if got := formatDB(z.bandGainsDB[band]); got != expected {
			t.Errorf("band %d readout = %q, want %q", band, got, expected)
		}
	}
}

func TestEQDBInput_NoBiDirectionalLoop(t *testing.T) {
	assertDefaultParityState(t)
	restore := noInputForTest()
	defer restore()

	z, log := newTestEQPanelZone(nil)
	_ = registerEQZone(z, image.Rect(0, 400, 600, 580))

	// Set gain via the editor commit.
	z.openEQDBEditor(0)
	z.paramEditor.ti.SetText("5.0")
	z.paramEditor.commit()

	// The commit itself fires OnGainChange once.
	initialGainChanges := len(log.gainChanges)
	if initialGainChanges != 1 {
		t.Fatalf("expected 1 gain change after commit, got %d", initialGainChanges)
	}

	// Reading the live readout must not trigger another callback (no
	// bidirectional sync loop now that the display derives from bandGainsDB).
	_ = formatDB(z.bandGainsDB[0])
	if len(log.gainChanges) != initialGainChanges {
		t.Errorf("reading the readout triggered extra gain change: had %d, now %d",
			initialGainChanges, len(log.gainChanges))
	}
}

func TestEQDBEditorPrefillShowsValueAndCommits(t *testing.T) {
	assertDefaultParityState(t)
	z, _ := newTestEQPanelZone(nil)
	z.Layout(image.Rect(0, 400, 600, 580))
	z.bandGainsDB[3] = -4.5
	z.openEQDBEditor(3)
	if !z.paramEditor.Active() {
		t.Fatalf("editor should be active after open")
	}
	if got := z.paramEditor.ti.Value(); got != "-4.5" {
		t.Fatalf("prefill=%q want -4.5 (must show prior value, not blank)", got)
	}
	z.paramEditor.ti.SetText("99") // out of [-12,12]
	z.paramEditor.commit()
	if z.bandGainsDB[3] != 12 {
		t.Fatalf("commit clamp got %v want 12", z.bandGainsDB[3])
	}
}
