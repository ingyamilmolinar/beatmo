//go:build test

package ui

import (
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// pressFooterTag drives a press at the visible center of the footer button
// carrying the given sentinel tag through the synth hit-area dispatcher.
// Routes through synthTabHitAreas, picking the highest-z hit that contains
// the point and returns its result.
func pressFooterTag(t *testing.T, env *synthFooterClickTestEnv, tag string) InputResult {
	t.Helper()
	btn := findFooterButton(t, env.g.drum, tag)
	r := btn.Rect()
	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2
	// Refresh hits before dispatch — earlier setup may have changed state.
	rawHits := env.g.drum.eqPanelZone.HitAreas()
	indexed := make([]indexedHitArea, 0, len(rawHits))
	for _, h := range rawHits {
		indexed = append(indexed, indexedHitArea{HitArea: h})
	}
	winner := topHitAt(indexed, cx, cy)
	if winner == nil || winner.Handler == nil {
		t.Fatalf("no hit area with handler at (%d,%d) for tag=%q rect=%v", cx, cy, tag, r)
	}
	return winner.Handler.OnPress(cx, cy)
}

// TestSaveAsDialog_OpensOnSaveAsPress — pressing the Save As footer button
// opens the inline name dialog (does NOT directly call SaveActiveRecipeAs).
// The dialog's TextInput must be focused with the suggested default name.
func TestSaveAsDialog_OpensOnSaveAsPress(t *testing.T) {
	env := setupSynthFooterEnv(t, 1280, 800, false)
	dv := env.g.drum
	if dv.SynthSaveAsDialog() != nil {
		t.Fatal("dialog should be nil before Save As press")
	}
	_ = pressFooterTag(t, env, synthSaveAsButtonTag)
	dlg := dv.SynthSaveAsDialog()
	if dlg == nil {
		t.Fatal("Save As press did not open the name dialog")
	}
	if !dlg.Focused() {
		t.Errorf("dialog TextInput should be focused on open")
	}
	if dlg.Value() == "" {
		t.Errorf("dialog should suggest a default name, got empty string")
	}
}

// TestSaveAsDialog_SubmitPersistsTypedName — typing a name into the dialog
// and confirming routes through SaveActiveRecipeAs with the typed string as
// the displayName. The persisted document must carry that name.
func TestSaveAsDialog_SubmitPersistsTypedName(t *testing.T) {
	env := setupSynthFooterEnv(t, 1280, 800, false)
	dv := env.g.drum
	lib := withPrivateInstrumentLibrary(t)

	audio.SetInstrumentParam(env.instID, "post_decay", 1.8)
	_ = pressFooterTag(t, env, synthSaveAsButtonTag)
	dlg := dv.SynthSaveAsDialog()
	if dlg == nil {
		t.Fatal("dialog not open after Save As press")
	}
	const typed = "Punchy Snare"
	dlg.SetValue(typed)
	dv.ConfirmSaveAsDialog()

	// Dialog dismissed.
	if dv.SynthSaveAsDialog() != nil {
		t.Errorf("dialog should close after confirm")
	}
	// Find the new user instrument whose document reached the library sink.
	var newID string
	for id := range lib.docs {
		if strings.HasPrefix(string(id), "user.") {
			newID = string(id)
		}
	}
	if newID == "" {
		t.Fatal("no user.* instrument reached the library sink")
	}
	d, err := instrument.ParseFile(lib.docs[instrument.ID(newID)])
	if err != nil {
		t.Fatalf("doc decode: %v", err)
	}
	if d.Name != typed {
		t.Errorf("saved document name = %q, want %q (typed name not propagated)", d.Name, typed)
	}
}

// TestSaveAsDialog_CancelDiscards — cancel must close the dialog without
// invoking the sink.
func TestSaveAsDialog_CancelDiscards(t *testing.T) {
	env := setupSynthFooterEnv(t, 1280, 800, false)
	dv := env.g.drum
	lib := withPrivateInstrumentLibrary(t)

	audio.SetInstrumentParam(env.instID, "post_decay", 1.8)
	_ = pressFooterTag(t, env, synthSaveAsButtonTag)
	if dv.SynthSaveAsDialog() == nil {
		t.Fatal("dialog not open")
	}
	dv.CancelSaveAsDialog()

	if dv.SynthSaveAsDialog() != nil {
		t.Errorf("dialog should close after cancel")
	}
	for id := range lib.docs {
		t.Errorf("library sink received %q after cancel — should be a no-op", id)
	}
}

// TestSaveAsDialog_EscapeKeyDismisses — pressing Escape while the dialog
// is open closes it without saving. Drives via the dialog's Update poll.
func TestSaveAsDialog_EscapeKeyDismisses(t *testing.T) {
	env := setupSynthFooterEnv(t, 1280, 800, false)
	dv := env.g.drum
	lib := withPrivateInstrumentLibrary(t)

	audio.SetInstrumentParam(env.instID, "post_decay", 1.8)
	_ = pressFooterTag(t, env, synthSaveAsButtonTag)
	if dv.SynthSaveAsDialog() == nil {
		t.Fatal("dialog not open")
	}
	// Esc is owned by the universal handler (Game.handleEscape → rung 2b
	// CancelActiveTextDialog), which is edge-triggered. Drive isKeyJustPressed
	// via stubKeys — SetInputForTest only sets isKeyPressed.
	restoreInput := stubKeys(
		map[ebiten.Key]bool{ebiten.KeyEscape: true},
		map[ebiten.Key]bool{ebiten.KeyEscape: true},
	)
	defer restoreInput()
	env.g.Update()

	if dv.SynthSaveAsDialog() != nil {
		t.Errorf("dialog should close after Escape")
	}
	for id := range lib.docs {
		t.Errorf("library sink received %q after Escape — should be a no-op", id)
	}
}
