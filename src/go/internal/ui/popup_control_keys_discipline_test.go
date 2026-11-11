//go:build test

package ui

// popup_control_keys_discipline_test.go — structural guards for the partial
// keyboard-claim contract (keyboard_key_claim.go).
//
// The reported bug was a MISSING pair, not a wrong one: the Sampler wheel popup
// neither handled the arrows nor claimed them, so a press did the grid's job.
// These guards make both halves impossible to omit independently.

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Every popup value control must satisfy KeyboardKeyClaimant. A compile-time
// assertion, so adding a fourth family without the interface fails the build of
// this test rather than shipping arrows that pan the camera.
var (
	_ KeyboardKeyClaimant = (*MobileWheelPopup)(nil)
	_ KeyboardKeyClaimant = (*SliderPopup)(nil)
	_ KeyboardKeyClaimant = (*mobileWheelPopupPortalOverlay)(nil)
	_ KeyboardKeyClaimant = (*sliderPopupPortalOverlay)(nil)
	_ KeyboardKeyClaimant = armedControlKeys{}
)

// A claim must not outlive its control: a CLOSED popup owns nothing, or the
// arrows would stay dead for the rest of the session.
func TestClosedPopupControlsClaimNoKeys(t *testing.T) {
	for _, tc := range []struct {
		name string
		c    KeyboardKeyClaimant
	}{
		{"wheel popup", NewMobileWheelPopup()},
		{"slider popup", NewSliderPopup(SliderPopupConfig{})},
		{"wheel portal overlay", &mobileWheelPopupPortalOverlay{popup: NewMobileWheelPopup()}},
		{"slider portal overlay", &sliderPopupPortalOverlay{popup: NewSliderPopup(SliderPopupConfig{})}},
	} {
		if keys := tc.c.ClaimedKeys(); len(keys) != 0 {
			t.Errorf("%s: closed control claims %d keys; must claim none", tc.name, len(keys))
		}
		if tc.c.HandleClaimedKeys() {
			t.Errorf("%s: closed control handled a key", tc.name)
		}
	}
}

// A claimant may only claim keys it actually handles. HandleClaimedKeys reads
// the arrows through anyKeyJustPressed(keysStep*), so every claimed key must
// appear in one of those shared slices — a claim on, say, Space would silently
// kill play/pause with no handler to replace it.
func TestClaimedKeysAreOnlyTheHandledArrows(t *testing.T) {
	handled := map[ebiten.Key]bool{}
	for _, ks := range [][]ebiten.Key{keysStepUp, keysStepDown, keysStepLeft, keysStepRight} {
		for _, k := range ks {
			handled[k] = true
		}
	}
	k := newEndlessKnobForTest(0, 200, 1.0)
	w := NewMobileWheelPopup()
	w.Open(WheelBinding{Knob: k}, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)
	claimed := w.ClaimedKeys()
	if len(claimed) == 0 {
		t.Fatal("an open value popup must claim the arrows it handles")
	}
	for _, key := range claimed {
		if !handled[key] {
			t.Errorf("wheel popup claims %v, which HandleClaimedKeys never reads", key)
		}
	}
}

// The arrow keys may only be READ at the sanctioned sites. A popup or panel that
// reads them in its own Update re-introduces the double-fire the claim contract
// removes (isKeyJustPressed is non-consuming, so both sites would act on the
// same press) — and, if it forgets the claim, the camera pans too.
func TestArrowKeysReadOnlyAtSanctionedSites(t *testing.T) {
	// Sanctioned: the shared alias slices, the grid shortcut NODE that owns
	// arrow-pan, the router's key declaration, and the menu keyboard navigator.
	allowed := map[string]bool{
		"keyboard_key_claim.go":   true, // declares keysStep* — the one alias table
		"keyboard_shortcuts.go":   true, // declares the grid node's key set
		"game_input_shortcuts.go": true, // gridShortcut: the arrow-pan node itself
		"menu_keynav.go":          true, // shared list/grid menu navigator
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || allowed[f] {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, spelling := range []string{
			"ebiten.KeyArrowUp", "ebiten.KeyArrowDown",
			"ebiten.KeyArrowLeft", "ebiten.KeyArrowRight",
		} {
			if strings.Contains(string(src), spelling) {
				t.Errorf("%s reads %s directly — route it through KeyboardKeyClaimant "+
					"(keyboard_key_claim.go) so the same press cannot also pan the grid", f, spelling)
			}
		}
	}
}
