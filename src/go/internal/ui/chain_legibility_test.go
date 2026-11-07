//go:build test

package ui

import (
	"strings"
	"testing"

	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

// Pure state→string helpers, so the wording tests without rendering; pixels come from the crop_chain_* scenes.

// A tap sets A, sets B or silently replaces B depending on hidden state; the hint shows the outcome before the
// click.

func TestChainNextTapHint_TracksTheSelectionState(t *testing.T) {
	const unset = scope.Stage(-1)
	for _, tc := range []struct {
		name     string
		a, b     scope.Stage
		wantWord string
	}{
		{"nothing set yet", unset, unset, "A"},
		{"only A set", scope.StageSynth, unset, "B"},
		{"only B set", unset, scope.StageEQ, "A"},
		{"both set - the surprising case", scope.StageSynth, scope.StageEQ, "REPLACES"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := chainNextTapHint(tc.a, tc.b)
			if got == "" {
				t.Fatal("hint is empty — the next tap's outcome stays invisible")
			}
			if !strings.Contains(strings.ToUpper(got), tc.wantWord) {
				t.Errorf("hint %q does not convey %q", got, tc.wantWord)
			}
		})
	}
}

// DESIGN.md bans raw chrome arrows, so it reads as words; failing here names the reason.
func TestChainNextTapHint_UsesWordsNotArrowGlyphs(t *testing.T) {
	for _, s := range []string{
		chainNextTapHint(-1, -1),
		chainNextTapHint(scope.StageSynth, -1),
		chainNextTapHint(scope.StageSynth, scope.StageEQ),
	} {
		for _, bad := range []string{"→", "←", "▶", "▼"} {
			if strings.Contains(s, bad) {
				t.Errorf("hint %q contains forbidden chrome glyph %q", s, bad)
			}
		}
	}
}

// The legend names A and B in their trace colours.

func TestChainLegend_NamesBothTapsWithTheirStage(t *testing.T) {
	got := chainLegend(scope.StageSynth, scope.StageEQ)
	if len(got) != 2 {
		t.Fatalf("legend has %d entries, want one for A and one for B", len(got))
	}
	if got[0].Slot != "A" || got[1].Slot != "B" {
		t.Fatalf("legend slots = %q/%q, want A then B", got[0].Slot, got[1].Slot)
	}
	if !got[0].Set || !got[1].Set {
		t.Error("both taps are assigned but the legend reports one unset")
	}
	if got[0].Stage == "" || got[1].Stage == "" {
		t.Error("legend does not name the stage each trace comes from")
	}
	if got[0].Color == got[1].Color {
		t.Error("A and B share a legend colour — the legend cannot tell them apart")
	}
}

// The case that most needs explaining: nothing can be compared yet.
func TestChainLegend_UnsetTapInvitesTheUserToSetIt(t *testing.T) {
	got := chainLegend(scope.StageSynth, -1)
	if len(got) != 2 {
		t.Fatalf("legend has %d entries, want both slots represented", len(got))
	}
	b := got[1]
	if b.Set {
		t.Fatal("B is unset but the legend reports it as assigned")
	}
	if b.Stage == "" {
		t.Error("the unset B slot shows nothing — it must invite the user to set it")
	}
}

// The traces' own palette, or the legend lies.
func TestChainLegend_ColorsMatchTheTracePalette(t *testing.T) {
	got := chainLegend(scope.StageSynth, scope.StageEQ)
	if got[0].Color != colScopeA {
		t.Errorf("A legend colour %v != trace colour colScopeA %v", got[0].Color, colScopeA)
	}
	if got[1].Color != colScopeB {
		t.Errorf("B legend colour %v != trace colour colScopeB %v", got[1].Color, colScopeB)
	}
}

// Every display-mode code has a PlainEnglish gloss, as on the other tabs.

func TestChainModeGloss_ExplainsEveryDisplayMode(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range []chainDisplayMode{chainOverlay, chainSplit, chainDiff} {
		g := chainModeGloss(m)
		if g == "" {
			t.Errorf("display mode %v has no plain-English gloss", m)
			continue
		}
		if seen[g] {
			t.Errorf("display mode %v reuses another mode's gloss %q", m, g)
		}
		seen[g] = true
	}
}

// A gloss that just repeats the code teaches nothing.
func TestChainModeGloss_DoesNotMerelyRestateTheCode(t *testing.T) {
	for m, code := range map[chainDisplayMode]string{
		chainOverlay: "OVR",
		chainSplit:   "SPL",
		chainDiff:    "DIF",
	} {
		g := chainModeGloss(m)
		if strings.EqualFold(strings.TrimSpace(g), code) {
			t.Errorf("gloss for %s is just the code again: %q", code, g)
		}
		if len(strings.Fields(g)) < 3 {
			t.Errorf("gloss for %s is too terse to explain anything: %q", code, g)
		}
	}
}

func TestPlainEnglish_CoversTheChainPillCodes(t *testing.T) {
	// FIT and AG are gone: the framing they toggled is unconditional now.
	for _, code := range []string{"OVR", "SPL", "DIF", "FRZ"} {
		if PlainEnglish(code) == "" {
			t.Errorf("PlainEnglish(%q) is empty — the Chain pill stays unexplained", code)
		}
	}
}
