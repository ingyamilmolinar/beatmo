//go:build test

package ui

import (
	"strings"
	"testing"

	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

// Chain-tab legibility: the three pure state->string helpers behind the
// redesign's header strip. All of them are plain functions so the wording can
// be tested without rendering; the PIXELS are verified by the crop_chain_*
// screenshot scenes.

// ── Next-tap hint ────────────────────────────────────────────────────────
//
// The click rule is stateful — a tap sets A, or sets B, or SILENTLY REPLACES B
// depending on hidden state (handleStageClick). The rule is deliberately
// unchanged; this hint is what makes its outcome visible BEFORE the click.

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

// The forbidden-glyph rule (DESIGN.md) bans raw chrome arrows in UI labels, so
// the hint must read as words. A regression here would also trip the
// repo-wide glyph audit, but failing HERE names the reason.
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

// ── A/B legend ───────────────────────────────────────────────────────────
//
// Nothing on screen said which colour was A and which was B, nor that B
// existed at all. The legend names both, in their own trace colours.

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

// An unset tap is the case that most needs explaining: the user cannot compare
// anything yet and nothing previously told them so.
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

// The legend colours must be the SAME ones the traces are drawn in, or the
// legend actively lies. Pinning them to the scope palette catches a future
// refactor that recolours the traces and forgets the legend.
func TestChainLegend_ColorsMatchTheTracePalette(t *testing.T) {
	got := chainLegend(scope.StageSynth, scope.StageEQ)
	if got[0].Color != colScopeA {
		t.Errorf("A legend colour %v != trace colour colScopeA %v", got[0].Color, colScopeA)
	}
	if got[1].Color != colScopeB {
		t.Errorf("B legend colour %v != trace colour colScopeB %v", got[1].Color, colScopeB)
	}
}

// ── Mode gloss ───────────────────────────────────────────────────────────
//
// OVR / SPL / DIF / FIT / AG were five unexplained codes. Chain was the only
// audio-panel tab that never used the PlainEnglish convention that Synth, EQ
// and Sampler all use.

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

// PlainEnglish is the app-wide convention for exactly this. Chain's codes must
// now answer to it like every other tab's knob labels do.
func TestPlainEnglish_CoversTheChainPillCodes(t *testing.T) {
	// FIT and AG are gone: the framing they toggled is unconditional now.
	for _, code := range []string{"OVR", "SPL", "DIF", "FRZ"} {
		if PlainEnglish(code) == "" {
			t.Errorf("PlainEnglish(%q) is empty — the Chain pill stays unexplained", code)
		}
	}
}
