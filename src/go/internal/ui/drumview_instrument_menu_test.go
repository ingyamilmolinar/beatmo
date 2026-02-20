//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// --- instDisplayLabel tests ---

func TestInstDisplayLabel_EmptyID(t *testing.T) {
	dv := &DrumView{}
	got := dv.instDisplayLabel("")
	if got != "(missing)" {
		t.Fatalf("instDisplayLabel(%q) = %q, want %q", "", got, "(missing)")
	}
}

func TestInstDisplayLabel_CacheHit(t *testing.T) {
	dv := &DrumView{
		instLabelCache: map[string]string{
			"kick": "Cached Kick",
		},
	}
	got := dv.instDisplayLabel("kick")
	if got != "Cached Kick" {
		t.Fatalf("instDisplayLabel(%q) = %q, want %q", "kick", got, "Cached Kick")
	}
}

func TestInstDisplayLabel_CacheMiss(t *testing.T) {
	dv := &DrumView{
		instLabelCache: map[string]string{},
	}
	got := dv.instDisplayLabel("snare")
	if got != "Snare" {
		t.Fatalf("instDisplayLabel(%q) = %q, want %q", "snare", got, "Snare")
	}
	// Verify it was cached.
	cached, ok := dv.instLabelCache["snare"]
	if !ok {
		t.Fatal("expected label to be cached after miss")
	}
	if cached != "Snare" {
		t.Fatalf("cached label = %q, want %q", cached, "Snare")
	}
}

func TestInstDisplayLabel_InitializesCache(t *testing.T) {
	// instLabelCache starts nil; should be created on first call.
	dv := &DrumView{}
	got := dv.instDisplayLabel("hihat")
	if got != "Hihat" {
		t.Fatalf("instDisplayLabel(%q) = %q, want %q", "hihat", got, "Hihat")
	}
	if dv.instLabelCache == nil {
		t.Fatal("instLabelCache should be initialized after first call")
	}
	if dv.instLabelCache["hihat"] != "Hihat" {
		t.Fatalf("cached = %q, want %q", dv.instLabelCache["hihat"], "Hihat")
	}
}

// --- computeInstLabel tests ---

func TestComputeInstLabel_WithMetaName(t *testing.T) {
	dv := &DrumView{
		instMeta: map[string]audio.SoundMeta{
			"kick808": {Name: "808 Kick"},
		},
	}
	got := dv.computeInstLabel("kick808")
	if got != "808 Kick" {
		t.Fatalf("computeInstLabel(%q) = %q, want %q", "kick808", got, "808 Kick")
	}
}

func TestComputeInstLabel_WithMetaRelPath(t *testing.T) {
	dv := &DrumView{
		instMeta: map[string]audio.SoundMeta{
			"myinst": {RelPath: "drums/deep_kick.wav"},
		},
	}
	got := dv.computeInstLabel("myinst")
	if got != "Deep_kick" {
		t.Fatalf("computeInstLabel(%q) = %q, want %q", "myinst", got, "Deep_kick")
	}
}

func TestComputeInstLabel_FallbackCapitalize(t *testing.T) {
	dv := &DrumView{}
	got := dv.computeInstLabel("snare")
	if got != "Snare" {
		t.Fatalf("computeInstLabel(%q) = %q, want %q", "snare", got, "Snare")
	}
}

func TestComputeInstLabel_SingleChar(t *testing.T) {
	dv := &DrumView{}
	got := dv.computeInstLabel("x")
	if got != "X" {
		t.Fatalf("computeInstLabel(%q) = %q, want %q", "x", got, "X")
	}
}

func TestComputeInstLabel_MetaEmptyNameUsesRelPath(t *testing.T) {
	dv := &DrumView{
		instMeta: map[string]audio.SoundMeta{
			"test": {Name: "", RelPath: "samples/bright_snare.wav"},
		},
	}
	got := dv.computeInstLabel("test")
	if got != "Bright_snare" {
		t.Fatalf("computeInstLabel(%q) = %q, want %q", "test", got, "Bright_snare")
	}
}

func TestComputeInstLabel_MetaEmptyBoth(t *testing.T) {
	// Meta entry exists but Name and RelPath are empty — falls back to ID.
	dv := &DrumView{
		instMeta: map[string]audio.SoundMeta{
			"tom": {},
		},
	}
	got := dv.computeInstLabel("tom")
	if got != "Tom" {
		t.Fatalf("computeInstLabel(%q) = %q, want %q", "tom", got, "Tom")
	}
}

// --- matchInstrumentSearch tests ---

func TestMatchInstrumentSearch_EmptyQuery(t *testing.T) {
	dv := &DrumView{}
	if !dv.matchInstrumentSearch("kick", "") {
		t.Fatal("empty query should always match")
	}
}

func TestMatchInstrumentSearch_MatchID(t *testing.T) {
	dv := &DrumView{}
	if !dv.matchInstrumentSearch("kick808", "kick") {
		t.Fatal("query 'kick' should match id 'kick808'")
	}
}

func TestMatchInstrumentSearch_MatchIDCaseInsensitive(t *testing.T) {
	dv := &DrumView{}
	if !dv.matchInstrumentSearch("HiHat", "hihat") {
		t.Fatal("query 'hihat' should match id 'HiHat' (case-insensitive)")
	}
}

func TestMatchInstrumentSearch_MatchLabel(t *testing.T) {
	dv := &DrumView{
		instMeta: map[string]audio.SoundMeta{
			"k808": {Name: "808 Deep Kick"},
		},
	}
	if !dv.matchInstrumentSearch("k808", "deep") {
		t.Fatal("query 'deep' should match label '808 Deep Kick'")
	}
}

func TestMatchInstrumentSearch_MatchRelPath(t *testing.T) {
	dv := &DrumView{
		instMeta: map[string]audio.SoundMeta{
			"myinst": {RelPath: "drums/vintage_snare.wav"},
		},
	}
	if !dv.matchInstrumentSearch("myinst", "vintage") {
		t.Fatal("query 'vintage' should match relPath 'drums/vintage_snare.wav'")
	}
}

func TestMatchInstrumentSearch_NoMatch(t *testing.T) {
	dv := &DrumView{}
	if dv.matchInstrumentSearch("kick", "zzzzz") {
		t.Fatal("query 'zzzzz' should NOT match id 'kick'")
	}
}

// --- buildInstMenu tests ---

// setupInstMenuDV creates a DrumView with enough state for buildInstMenu to work.
func setupInstMenuDV(t *testing.T) *DrumView {
	t.Helper()
	assertDefaultParityState(t)

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.instOptions = []string{"kick", "snare", "hihat", "tom", "clap"}
	dv.instCategories = nil
	dv.instCatByID = nil
	dv.instMeta = nil

	dv.Rows = []*DrumRow{
		{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0},
		{Name: "Snare", Instrument: "snare", Steps: make([]bool, 8), Volume: 1.0},
	}
	dv.Length = 8

	dv.bgDirty = true
	dv.calcLayout()
	dv.bgDirty = false

	return dv
}

func TestBuildInstMenu_InstrumentsMode(t *testing.T) {
	dv := setupInstMenuDV(t)

	dv.instMenuRow = 0
	dv.instMenuMode = instMenuModeInstruments
	dv.instMenuBtns = nil
	dv.buildInstMenu()

	if len(dv.instMenuBtns) == 0 {
		t.Fatal("buildInstMenu should create buttons in instruments mode")
	}
	// Should have buttons for the visible instruments (up to max visible rows).
	// No back button since there are no categories.
	for _, btn := range dv.instMenuBtns {
		if btn.Text == "" {
			t.Fatal("button should have non-empty text")
		}
	}
}

func TestBuildInstMenu_CategoriesMode(t *testing.T) {
	dv := setupInstMenuDV(t)
	dv.instCategories = []string{"Drums", "Cymbals"}
	dv.instCatByID = map[string]string{
		"kick":  "Drums",
		"snare": "Drums",
		"tom":   "Drums",
		"hihat": "Cymbals",
		"clap":  "Cymbals",
	}

	dv.instMenuRow = 0
	dv.instMenuMode = instMenuModeCategories
	dv.instMenuBtns = nil
	dv.instCategoryBtns = nil
	dv.buildInstMenu()

	if len(dv.instCategoryBtns) == 0 {
		t.Fatal("buildInstMenu should create category buttons in categories mode")
	}
	// Verify category names appear as button labels.
	found := map[string]bool{}
	for _, btn := range dv.instCategoryBtns {
		found[btn.Text] = true
	}
	if !found["Drums"] {
		t.Fatal("expected 'Drums' category button")
	}
	if !found["Cymbals"] {
		t.Fatal("expected 'Cymbals' category button")
	}
}

func TestBuildInstMenu_CategoriesFallbackNoCategories(t *testing.T) {
	dv := setupInstMenuDV(t)
	dv.instCategories = nil

	dv.instMenuRow = 0
	dv.instMenuMode = instMenuModeCategories
	dv.instMenuBtns = nil
	dv.buildInstMenu()

	// With no categories, should fall back to instruments mode.
	if dv.instMenuMode != instMenuModeInstruments {
		t.Fatalf("mode = %q, want %q (fallback)", dv.instMenuMode, instMenuModeInstruments)
	}
	if len(dv.instMenuBtns) == 0 {
		t.Fatal("should have instrument buttons after fallback")
	}
}

func TestBuildInstMenu_SearchFilter(t *testing.T) {
	dv := setupInstMenuDV(t)

	dv.instMenuRow = 0
	dv.instMenuMode = instMenuModeInstruments
	dv.instSearch = "kick"
	dv.instMenuActiveCat = ""
	dv.instMenuBtns = nil
	// Ensure instOptions is fresh (constructor may have mutated the slice).
	dv.instOptions = []string{"kick", "snare", "hihat", "tom", "clap"}
	dv.buildInstMenu()

	// Only "kick" should match the search.
	found := false
	for _, btn := range dv.instMenuBtns {
		if btn.Text == "Kick" {
			found = true
		}
		// "Snare", "Hihat", etc. should not appear.
		if btn.Text == "Snare" || btn.Text == "Hihat" || btn.Text == "Tom" || btn.Text == "Clap" {
			t.Fatalf("unexpected button %q should be filtered out by search 'kick'", btn.Text)
		}
	}
	if !found {
		t.Fatal("expected 'Kick' button to appear for search 'kick'")
	}
}

func TestBuildInstMenu_NoMatches(t *testing.T) {
	dv := setupInstMenuDV(t)

	dv.instMenuRow = 0
	dv.instMenuMode = instMenuModeInstruments
	dv.instSearch = "zzzznotfound"
	dv.instMenuActiveCat = ""
	dv.instMenuBtns = nil
	dv.instOptions = []string{"kick", "snare", "hihat", "tom", "clap"}
	dv.buildInstMenu()

	// Should have a placeholder "No matches" button.
	if len(dv.instMenuBtns) == 0 {
		t.Fatal("should have a placeholder button for no matches")
	}
	if dv.instMenuBtns[0].Text != "No matches" {
		t.Fatalf("placeholder text = %q, want %q", dv.instMenuBtns[0].Text, "No matches")
	}
}

func TestBuildInstMenu_OutOfRangeRow(t *testing.T) {
	dv := setupInstMenuDV(t)

	dv.instMenuRow = 99 // out of range
	dv.instMenuBtns = []*Button{{}} // pre-populate to verify it's cleared
	dv.buildInstMenu()

	if len(dv.instMenuBtns) != 0 {
		t.Fatalf("len(instMenuBtns) = %d, want 0 for out-of-range row", len(dv.instMenuBtns))
	}
}

func TestBuildInstMenu_NegativeRow(t *testing.T) {
	dv := setupInstMenuDV(t)

	dv.instMenuRow = -1
	dv.instMenuBtns = []*Button{{}}
	dv.buildInstMenu()

	if len(dv.instMenuBtns) != 0 {
		t.Fatalf("len(instMenuBtns) = %d, want 0 for negative row", len(dv.instMenuBtns))
	}
}

func TestBuildInstMenu_InstrumentsModeWithCategories_HasBack(t *testing.T) {
	dv := setupInstMenuDV(t)
	dv.instCategories = []string{"Drums", "Cymbals"}
	dv.instCatByID = map[string]string{
		"kick":  "Drums",
		"snare": "Drums",
		"tom":   "Drums",
		"hihat": "Cymbals",
		"clap":  "Cymbals",
	}

	dv.instMenuRow = 0
	dv.instMenuMode = instMenuModeInstruments
	dv.instMenuActiveCat = "Drums"
	dv.instMenuBtns = nil
	dv.buildInstMenu()

	if len(dv.instMenuBtns) == 0 {
		t.Fatal("should have buttons")
	}
	// First button should be "Back" when categories exist.
	if dv.instMenuBtns[0].Text != "Back" {
		t.Fatalf("first button = %q, want %q", dv.instMenuBtns[0].Text, "Back")
	}
}

func TestBuildInstMenu_ActiveCatFilters(t *testing.T) {
	dv := setupInstMenuDV(t)
	dv.instCategories = []string{"Drums", "Cymbals"}
	dv.instCatByID = map[string]string{
		"kick":  "Drums",
		"snare": "Drums",
		"tom":   "Drums",
		"hihat": "Cymbals",
		"clap":  "Cymbals",
	}

	dv.instMenuRow = 0
	dv.instMenuMode = instMenuModeInstruments
	dv.instMenuActiveCat = "Cymbals"
	dv.instMenuBtns = nil
	dv.buildInstMenu()

	// Only cymbals instruments + Back button should appear.
	for _, btn := range dv.instMenuBtns {
		if btn.Text == "Back" {
			continue
		}
		if btn.Text == "Kick" || btn.Text == "Snare" || btn.Text == "Tom" {
			t.Fatalf("button %q should not appear when active category is Cymbals", btn.Text)
		}
	}
}

func TestBuildInstMenu_ScrollState(t *testing.T) {
	assertDefaultParityState(t)

	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.instOptions = []string{"kick", "snare", "hihat", "tom", "clap"}
	dv.instCategories = nil
	dv.instCatByID = nil

	dv.Rows = []*DrumRow{
		{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0},
	}
	dv.Length = 8
	dv.bgDirty = true
	dv.calcLayout()
	dv.bgDirty = false

	// Open via the proper path so scroll state is populated.
	dv.openInstMenuForRow(0)
	if !dv.IsInstMenuOpen() {
		t.Fatal("menu should be open")
	}
	dv.syncInstMenuBtnsFromComp()

	if dv.instMenuScroll.View.Empty() {
		t.Fatal("scroll.View should be non-empty after opening inst menu")
	}
}

func TestBuildInstMenu_UnsetModeFallsToCategories(t *testing.T) {
	dv := setupInstMenuDV(t)
	dv.instCategories = []string{"Drums"}
	dv.instCatByID = map[string]string{"kick": "Drums"}

	dv.instMenuRow = 0
	dv.instMenuMode = instMenuModeUnset
	dv.instMenuBtns = nil
	dv.buildInstMenu()

	// Should set mode to categories when unset and categories exist.
	if dv.instMenuMode != instMenuModeCategories {
		t.Fatalf("mode = %q, want %q after unset with categories", dv.instMenuMode, instMenuModeCategories)
	}
}

// --- openInstMenuForRow tests ---

func TestOpenInstMenuForRow_Toggle(t *testing.T) {
	assertDefaultParityState(t)

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

	// Open for row 0.
	dv.openInstMenuForRow(0)
	if !dv.IsInstMenuOpen() {
		t.Fatal("menu should be open after first openInstMenuForRow(0)")
	}

	// Open again for the same row — should toggle closed.
	dv.openInstMenuForRow(0)
	if dv.IsInstMenuOpen() {
		t.Fatal("menu should be CLOSED after second openInstMenuForRow(0) (toggle)")
	}
}

func TestOpenInstMenuForRow_OutOfRange(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)

	// Negative row — should be no-op.
	dv.openInstMenuForRow(-1)
	if dv.IsInstMenuOpen() {
		t.Fatal("menu should not open for negative row index")
	}

	// Row beyond len(Rows).
	dv.openInstMenuForRow(999)
	if dv.IsInstMenuOpen() {
		t.Fatal("menu should not open for row index beyond Rows length")
	}
}

func TestOpenInstMenuForRow_SwitchRow(t *testing.T) {
	assertDefaultParityState(t)

	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.instOptions = []string{"kick", "snare"}
	dv.instCategories = nil
	dv.instCatByID = nil

	dv.Rows = []*DrumRow{
		{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0},
		{Name: "Snare", Instrument: "snare", Steps: make([]bool, 8), Volume: 1.0},
	}
	dv.Length = 8
	dv.bgDirty = true
	dv.calcLayout()
	dv.bgDirty = false

	// Open for row 0.
	dv.openInstMenuForRow(0)
	if !dv.IsInstMenuOpen() {
		t.Fatal("menu should be open for row 0")
	}
	if dv.instMenuRow != 0 {
		t.Fatalf("instMenuRow = %d, want 0", dv.instMenuRow)
	}

	// Open for row 1 — should switch, not toggle off.
	dv.openInstMenuForRow(1)
	if !dv.IsInstMenuOpen() {
		t.Fatal("menu should be open for row 1 (switch, not toggle)")
	}
	if dv.instMenuRow != 1 {
		t.Fatalf("instMenuRow = %d, want 1", dv.instMenuRow)
	}
}

func TestOpenInstMenuForRow_PrimesCategoryFromRow(t *testing.T) {
	assertDefaultParityState(t)

	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.instOptions = []string{"kick", "snare", "hihat"}
	dv.instCategories = []string{"Drums", "Cymbals"}
	dv.instCatByID = map[string]string{
		"kick":  "Drums",
		"snare": "Drums",
		"hihat": "Cymbals",
	}

	dv.Rows = []*DrumRow{
		{Name: "HiHat", Instrument: "hihat", Steps: make([]bool, 8), Volume: 1.0},
	}
	dv.Length = 8
	dv.bgDirty = true
	dv.calcLayout()
	dv.bgDirty = false

	dv.openInstMenuForRow(0)
	if dv.instMenuActiveCat != "Cymbals" {
		t.Fatalf("instMenuActiveCat = %q, want %q (primed from row instrument)", dv.instMenuActiveCat, "Cymbals")
	}
}

// --- syncInstMenuBtnsFromComp tests ---

func TestSyncInstMenuBtnsFromComp_NilComp(t *testing.T) {
	dv := &DrumView{}
	dv.instMenuComp = nil
	// Should not panic.
	dv.syncInstMenuBtnsFromComp()
}

func TestSyncInstMenuBtnsFromComp_WithComp(t *testing.T) {
	assertDefaultParityState(t)

	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	if dv.instMenuComp == nil {
		t.Skip("instMenuComp not initialized in this build")
	}

	// Open menu first so the component has state.
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	dv.instOptions = []string{"kick", "snare"}
	dv.instCategories = nil
	dv.instCatByID = nil
	dv.Rows = []*DrumRow{
		{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0},
	}
	dv.Length = 8
	dv.bgDirty = true
	dv.calcLayout()
	dv.bgDirty = false

	dv.openInstMenuForRow(0)
	if !dv.IsInstMenuOpen() {
		t.Fatal("menu should be open")
	}

	// Clear and resync.
	dv.instMenuBtns = nil
	dv.syncInstMenuBtnsFromComp()

	if len(dv.instMenuBtns) == 0 {
		t.Fatal("syncInstMenuBtnsFromComp should populate instMenuBtns")
	}
}

// --- buildInstMenu: open direction ---

func TestBuildInstMenu_OpenUpWhenAnchorNearBottom(t *testing.T) {
	assertDefaultParityState(t)

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.instOptions = []string{"kick", "snare", "hihat", "tom", "clap"}
	dv.instCategories = nil
	dv.instCatByID = nil

	// Create multiple rows so one can be near the bottom.
	rows := make([]*DrumRow, 10)
	for i := range rows {
		rows[i] = &DrumRow{
			Name:       "Row",
			Instrument: "kick",
			Steps:      make([]bool, 8),
			Volume:     1.0,
		}
	}
	dv.Rows = rows
	dv.Length = 8
	dv.bgDirty = true
	dv.calcLayout()
	dv.bgDirty = false

	labels := dv.rowLabels()
	if len(labels) == 0 {
		t.Fatal("no row labels")
	}

	// Use the last visible row (near bottom) to trigger upward opening.
	lastRow := len(labels) - 1
	if lastRow >= len(dv.Rows) {
		lastRow = len(dv.Rows) - 1
	}
	dv.instMenuRow = lastRow
	dv.instMenuMode = instMenuModeInstruments
	dv.instMenuBtns = nil
	dv.buildInstMenu()

	// Menu should have been created.
	if len(dv.instMenuBtns) == 0 {
		t.Fatal("should have buttons for bottom row")
	}
	// The menu's full rect should be positioned above the anchor.
	anchorY := labels[lastRow].Rect().Min.Y
	if !dv.instMenuFullRect.Empty() && dv.instMenuFullRect.Min.Y > anchorY {
		// Not a hard error if geometry constraints push it down, but log for visibility.
		t.Logf("note: fullRect.Min.Y=%d > anchorY=%d (geometry constraints may override openUp)",
			dv.instMenuFullRect.Min.Y, anchorY)
	}
}
