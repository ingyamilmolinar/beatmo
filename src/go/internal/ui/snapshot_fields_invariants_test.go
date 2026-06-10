//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// These tests lock the building blocks that fullLayoutSnapshot's new keys
// depend on (menu/tabs/bottomNav + richer state). The snapshot function itself
// is //go:build js && !test (it needs syscall/js + a browser), so its end-to-end
// shape is asserted by the browser smoke test; here we pin the pure Go pieces it
// reads so a future refactor that breaks them fails fast in the fast path.

// snapshotNavSlugs mirrors the navSlugs list the snapshot emits under
// obj.bottomNav, in segmented-control order.
var snapshotNavSlugs = []string{"pads", "eq", "wave", "spectrum", "levels", "chain", "synth", "sampler"}

// TestViewModeSlug_MatchesBottomNavOrder asserts viewModeSlug is total over the
// viewMode enum and produces exactly the bottomNav slug list, in enum order.
// The mobile bottom-nav segmented control (Pads/EQ/Wave/Spec/Lvl/Chn/Syn/Smpl)
// maps segment i -> viewMode i, so the slug order must line up 1:1.
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

// TestPanelTabSlugs_CoverTabMap asserts AllPanelTabs() yields unique non-empty
// slugs, and that every audio-panel tab slug (except "pads", which is the Rows
// view) appears in the bottomNav slug list — i.e. the desktop `tabs` map keys
// and the mobile `bottomNav` keys describe the same surface set.
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

// TestRowMenuKebab_PresentOnDesktop pins that the per-row kebab the snapshot
// exports as rows[i].menu has a real (non-empty) rect on desktop — this is the
// only inline way an agent reaches color/edit/origin/delete now.
func TestRowMenuKebab_PresentOnDesktop(t *testing.T) {
	assertDefaultParityState(t)
	const W, H = 1280, 800
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0}}
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

// TestMobileInlineMuteSoloReachable pins that on MOBILE (390x844 portrait) the
// inline M/S buttons have non-empty rects for the top rows and that rows 1–2 are
// within the visible window. The mobile sanity test's Phase 4 drives mute/solo
// via click_ui (inline) — NOT the context menu, which contains only
// Instrument/Rename/Origin/Delete (see contextMenuItems). This guards against
// reverting Phase 4 to the (impossible) menu_click "Mute"/"Solo" path.
func TestMobileInlineMuteSoloReachable(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)
	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{
		{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0},
		{Name: "Snare", Instrument: "snare", Steps: make([]bool, 8), Volume: 1.0},
		{Name: "Hat", Instrument: "hihat", Steps: make([]bool, 8), Volume: 1.0},
		{Name: "Clap", Instrument: "clap", Steps: make([]bool, 8), Volume: 1.0},
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
	// Portrait 390x844 must show many row slots (no fragile scrolling to reach
	// the top rows the sanity test touches). 750x340 landscape showed only 1.
	if vis := dv.visibleRows(); vis < 4 {
		t.Fatalf("only %d row slots visible at %dx%d — too cramped for the mobile sanity test", vis, W, H)
	}
	// Row 0's inline mute/solo must have non-empty rects (mobile keeps M/S inline,
	// NOT in the context menu) so click_ui button="mute"/"solo" works.
	if len(dv.rowMuteBtns()) == 0 || dv.rowMuteBtns()[0] == nil || dv.rowMuteBtns()[0].Rect().Empty() {
		t.Errorf("mobile row 0 inline mute rect empty — click_ui button=\"mute\" would fail")
	}
	if len(dv.rowSoloBtns()) == 0 || dv.rowSoloBtns()[0] == nil || dv.rowSoloBtns()[0].Rect().Empty() {
		t.Errorf("mobile row 0 inline solo rect empty — click_ui button=\"solo\" would fail")
	}
}
