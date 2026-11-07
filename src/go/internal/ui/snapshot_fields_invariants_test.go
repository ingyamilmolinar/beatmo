//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// fullLayoutSnapshot needs syscall/js, so the browser smoke test checks its shape; these pin the pure Go pieces
// it reads.

// In segmented-control order.
var snapshotNavSlugs = []string{"pads", "eq", "wave", "spectrum", "levels", "chain", "synth", "sampler"}

// Segment i maps to viewMode i, so the slug order must line up 1:1.
func TestViewModeSlug_MatchesBottomNavOrder(t *testing.T) {
	modes := []viewMode{
		viewModeRows, viewModeEQ, viewModeWave, viewModeSpectrum,
		viewModeMeters, viewModeChain, viewModeSynth, viewModeSampler,
	}
	if len(modes) != len(snapshotNavSlugs) {
		t.Fatalf("viewMode count %d != navSlugs count %d", len(modes), len(snapshotNavSlugs))
	}
	for i, m := range modes {
		if got := viewModeSlug(m); got != snapshotNavSlugs[i] {
			t.Errorf("viewModeSlug(%d)=%q, want %q", int(m), got, snapshotNavSlugs[i])
		}
	}
}

// The desktop `tabs` keys and the mobile `bottomNav` keys describe the same surfaces ("pads" is the Rows view).
func TestPanelTabSlugs_CoverTabMap(t *testing.T) {
	seen := map[string]bool{}
	for _, tab := range AllPanelTabs() {
		s := PanelTabSlug(tab)
		if s == "" {
			t.Errorf("PanelTabSlug(%d) empty", int(tab))
		}
		if seen[s] {
			t.Errorf("duplicate tab slug %q", s)
		}
		seen[s] = true
	}
	if len(seen) != 7 {
		t.Errorf("expected 7 distinct tab slugs, got %d (%v)", len(seen), seen)
	}
	// Every tab slug must be a bottomNav slug (minus "pads").
	navSet := map[string]bool{}
	for _, s := range snapshotNavSlugs {
		navSet[s] = true
	}
	for s := range seen {
		if !navSet[s] {
			t.Errorf("tab slug %q missing from bottomNav slug list %v", s, snapshotNavSlugs)
		}
	}
}

// The kebab is the only inline way to reach colour / edit / origin / delete.
func TestRowMenuKebab_PresentOnDesktop(t *testing.T) {
	assertDefaultParityState(t)
	const W, H = 1280, 800
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: roster.Kick, Steps: make([]bool, 8), Volume: 1.0}}
	dv.Length = 8

	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	restore()

	btns := dv.rowMenuBtns()
	if len(btns) == 0 || btns[0] == nil {
		t.Fatalf("expected a row-menu kebab button, got none")
	}
	if btns[0].Rect().Empty() {
		t.Errorf("row kebab rect empty on desktop — snapshot rows[0].menu would be unreachable")
	}
}

// Mobile keeps M/S inline: the context menu holds only Instrument / Rename / Origin / Delete.
func TestMobileInlineMuteSoloReachable(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)
	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{
		{Name: "Kick", Instrument: roster.Kick, Steps: make([]bool, 8), Volume: 1.0},
		{Name: "Snare", Instrument: roster.Snare, Steps: make([]bool, 8), Volume: 1.0},
		{Name: "Hat", Instrument: roster.Hihat, Steps: make([]bool, 8), Volume: 1.0},
		{Name: "Clap", Instrument: roster.Clap, Steps: make([]bool, 8), Volume: 1.0},
	}
	dv.Length = 8
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	restore()

	if !Profile().IsMobile() {
		t.Fatalf("expected mobile profile at %dx%d", W, H)
	}
	// Enough row slots that the top rows need no fragile scrolling.
	if vis := dv.visibleRows(); vis < 4 {
		t.Fatalf("only %d row slots visible at %dx%d — too cramped for the mobile sanity test", vis, W, H)
	}
	if len(dv.rowMuteBtns()) == 0 || dv.rowMuteBtns()[0] == nil || dv.rowMuteBtns()[0].Rect().Empty() {
		t.Errorf("mobile row 0 inline mute rect empty — click_ui button=\"mute\" would fail")
	}
	if len(dv.rowSoloBtns()) == 0 || dv.rowSoloBtns()[0] == nil || dv.rowSoloBtns()[0].Rect().Empty() {
		t.Errorf("mobile row 0 inline solo rect empty — click_ui button=\"solo\" would fail")
	}
}
