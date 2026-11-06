//go:build test

// Per-frame cost discipline for the open File/Templates overflow menu.
//
// Bug (2026-09-02): opening the File menu's Templates page dropped the frame
// rate and made input feel laggy while the scrollbar menu was on screen. The
// cause was not drawing — it was that the menu's ITEM MODEL was rebuilt from
// scratch on every access:
//
//   - overflowItems() ran 12x per frame. On the template page each run built a
//     32-entry map[string]assets.Template, a ~40-element []overflowItem, 32
//     onClick closures and 32 strconv.Itoa BPM captions.
//   - overflowPopupWidth() ran 6x per frame, each re-measuring StyledTextWidth
//     over ~40 long "Artist - Song" labels (~240 text measurements per frame).
//
// Measured cost of one full Game.Update+Draw frame (1280x720, stub backend):
//
//	File page (5 items):      ~303 allocs /   9.8 KB per frame
//	Templates page (40 items): ~1515 allocs / 237.4 KB per frame   <-- 24x the bytes
//
// 237 KB/frame is ~14 MB/s of garbage at 60 fps, which is what starved the
// single-threaded WASM build.
//
// The invariant these tests pin: an open menu's per-frame cost must scale with
// what it DRAWS, not with how many times per frame something asks it for its
// item list. Rebuilding the model is a function of menu STATE (page, search
// query, selection, locale), so it must be memoized on that state.

package ui

import (
	"runtime"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"

	"github.com/hajimehoshi/ebiten/v2"
)

// Budgets are calibrated against the post-fix measurement plus headroom; they
// catch order-of-magnitude regressions, not micro-churn. See the numbers above
// for the pre-fix baseline these replaced.
const (
	// A single frame with the Templates page open must not allocate more than
	// this. Post-fix baseline 285 allocs; cap = baseline x ~1.35.
	// (Pre-fix: 1515.) RATCHET: lower this if you reduce real allocations;
	// never raise it to make a regression pass.
	templateMenuFrameAllocBudget = 385
	// Bytes per frame. Post-fix baseline ~6.6 KB; cap = baseline x ~1.8 to
	// absorb GC/host variance while still catching the 237 KB regression by 20x.
	templateMenuFrameByteBudget = 12 << 10
)

// openOverflowFrameCost drives real Game.Update + Game.Draw frames with the
// overflow menu open on the given page and reports the per-frame allocation
// count and byte churn.
func openOverflowFrameCost(t *testing.T, page int) (allocs float64, bytesPerFrame uint64) {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.drum.overflowPage = page
	g.drum.OpenOverflowMenu()
	if !g.drum.IsOverflowMenuOpen() {
		t.Fatal("overflow menu did not open; harness assumption broken")
	}

	scratch := ebiten.NewImage(1280, 720)
	// Settle: first frames pay one-off layout/portal/button-build costs that
	// would otherwise skew the steady-state measurement.
	for i := 0; i < 30; i++ {
		_ = g.Update()
		g.Draw(scratch)
	}

	frame := func() {
		_ = g.Update()
		g.Draw(scratch)
	}

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	const frames = 100
	for i := 0; i < frames; i++ {
		frame()
	}
	runtime.ReadMemStats(&after)
	bytesPerFrame = (after.TotalAlloc - before.TotalAlloc) / frames

	allocs = testing.AllocsPerRun(100, frame)
	return allocs, bytesPerFrame
}

// TestTemplateMenuFrameCost pins the per-frame cost of the open Templates page.
// It fails when the menu's item model (items, closures, BPM captions, label
// width measurements) is rebuilt per access instead of memoized on menu state.
func TestTemplateMenuFrameCost(t *testing.T) {
	allocs, bytesPerFrame := openOverflowFrameCost(t, 1)
	t.Logf("templates page: %.0f allocs/frame, %d bytes/frame", allocs, bytesPerFrame)

	if allocs > templateMenuFrameAllocBudget {
		t.Errorf("Templates page allocates %.0f objects per frame, budget %d.\n"+
			"The menu's item model is being rebuilt per access instead of memoized "+
			"on menu state (page/query/selection/locale).",
			allocs, templateMenuFrameAllocBudget)
	}
	if bytesPerFrame > templateMenuFrameByteBudget {
		t.Errorf("Templates page churns %d bytes per frame, budget %d.\n"+
			"At 60fps that is %.1f MB/s of garbage, which starves the "+
			"single-threaded WASM build.",
			bytesPerFrame, templateMenuFrameByteBudget,
			float64(bytesPerFrame)*60/(1<<20))
	}
}

// TestTemplateMenuCostNearFileMenu pins the RELATIVE invariant: the 40-row
// Templates page must not cost dramatically more per frame than the 5-row File
// page. Both draw a clipped popup of the same on-screen size; the only reason
// Templates could cost 24x more is per-frame model rebuilding.
func TestTemplateMenuCostNearFileMenu(t *testing.T) {
	fileAllocs, fileBytes := openOverflowFrameCost(t, 0)
	tmplAllocs, tmplBytes := openOverflowFrameCost(t, 1)
	t.Logf("file page: %.0f allocs/frame %d B/frame; templates page: %.0f allocs/frame %d B/frame",
		fileAllocs, fileBytes, tmplAllocs, tmplBytes)

	const maxRatio = 2.0
	if ratio := tmplAllocs / fileAllocs; ratio > maxRatio {
		t.Errorf("Templates page allocates %.1fx the File page per frame (%.0f vs %.0f), max %.1fx.\n"+
			"Per-frame cost must track what the menu DRAWS, not how many times "+
			"per frame its item list is rebuilt.", ratio, tmplAllocs, fileAllocs, maxRatio)
	}
	if fileBytes > 0 {
		if ratio := float64(tmplBytes) / float64(fileBytes); ratio > maxRatio {
			t.Errorf("Templates page churns %.1fx the File page's bytes per frame (%d vs %d), max %.1fx.",
				ratio, tmplBytes, fileBytes, maxRatio)
		}
	}
}

// countMenuRowDraws runs fn with drawMenuRow intercepted and reports how many
// rows were rendered.
func countMenuRowDraws(t *testing.T, fn func()) int {
	t.Helper()
	n := 0
	orig := drawMenuRow
	drawMenuRow = func(dst *ebiten.Image, btn *Button, spec MenuRowSpec) {
		n++
		orig(dst, btn, spec)
	}
	t.Cleanup(func() { drawMenuRow = orig })
	fn()
	return n
}

// TestTemplateMenuCullsOffscreenRows pins the second half of the frame-cost
// bug: drawOverflowMenu iterated EVERY item and called drawMenuRow on it, so
// the 40-row Templates page rendered ~40 rows per frame (each a background,
// an icon blit and a label sprite blit) into a popup that only shows ~6. The
// off-screen rows were discarded by the GPU clip — every blit was wasted work,
// and it showed up as DrawImage dominating the menu's frame time.
//
// Per-frame draw work must scale with what is VISIBLE, not with list length.
func TestTemplateMenuCullsOffscreenRows(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.drum.overflowPage = 1
	g.drum.OpenOverflowMenu()

	scratch := ebiten.NewImage(1280, 720)
	for i := 0; i < 10; i++ {
		_ = g.Update()
		g.Draw(scratch)
	}

	items := g.drum.overflowItems()
	popup := g.drum.overflowPopupRect()
	pinned := overflowPinnedCount(items)
	rowH := touchMinTargetPx
	visibleRows := (popup.Dy() - pinned*rowH) / rowH
	if visibleRows < 1 {
		t.Fatalf("harness: popup shows no scrolling rows (popupH=%d pinned=%d)", popup.Dy(), pinned)
	}
	if len(items) < visibleRows*3 {
		t.Fatalf("harness: template list (%d items) does not overflow the popup (%d visible rows) "+
			"by enough to make culling observable", len(items), visibleRows)
	}

	drawn := countMenuRowDraws(t, func() { g.Draw(scratch) })

	// +2 for the partially-visible rows at the top and bottom edges of the
	// viewport, which must still render.
	budget := visibleRows + 2
	t.Logf("templates page: %d items, %d visible rows, %d menu rows drawn (budget %d)",
		len(items), visibleRows, drawn, budget)
	if drawn > budget {
		t.Errorf("drawOverflowMenu rendered %d menu rows for a viewport showing %d "+
			"(budget %d, list is %d items).\n"+
			"Rows scrolled outside the popup must be culled before drawMenuRow — "+
			"per-frame draw work has to scale with the VISIBLE window, not the list length.",
			drawn, visibleRows, budget, len(items))
	}
}

// TestOverflowItemsCacheInvalidates pins the correctness half of the memo: the
// cached item model MUST be rebuilt whenever any input in overflowItemsKey
// changes. A stale cache here would show the wrong page, ignore the search
// query, mis-mark the active template, or keep stale-locale labels on screen.
func TestOverflowItemsCacheInvalidates(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleEN)

	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	dv := g.drum
	dv.OpenOverflowMenu()

	labels := func() []string {
		out := make([]string, 0, len(dv.overflowItems()))
		for _, it := range dv.overflowItems() {
			out = append(out, it.label)
		}
		return out
	}
	joined := func() string { return strings.Join(labels(), "|") }

	// page: File -> Templates
	filePage := joined()
	dv.overflowPage = 1
	tmplPage := joined()
	if tmplPage == filePage {
		t.Error("page change did not invalidate the overflow item cache")
	}

	// query: filtering must shrink the list
	all := len(dv.overflowItems())
	dv.setTemplateSearchQuery("bach")
	filtered := len(dv.overflowItems())
	if filtered >= all {
		t.Errorf("search query did not invalidate the cache (%d items before, %d after filtering)", all, filtered)
	}
	dv.setTemplateSearchQuery("")
	if got := len(dv.overflowItems()); got != all {
		t.Errorf("clearing the query did not restore the full list: got %d want %d", got, all)
	}

	// loaded: the active row must follow templateLoadedStem
	activeCount := func() int {
		n := 0
		for _, it := range dv.overflowItems() {
			if it.active {
				n++
			}
		}
		return n
	}
	if n := activeCount(); n != 0 {
		t.Fatalf("harness: expected no active row before a template is loaded, got %d", n)
	}
	dv.templateLoadedStem = assets.Templates()[0].Genre
	if n := activeCount(); n != 1 {
		t.Errorf("templateLoadedStem change did not invalidate the cache: %d active rows, want 1", n)
	}

	// locale: File-page labels are translated, so a locale switch must rebuild
	dv.overflowPage = 0
	en := joined()
	i18n.SetLocale(i18n.LocaleES)
	es := joined()
	if es == en {
		t.Errorf("locale change did not invalidate the overflow item cache; labels stayed %q", en)
	}
}
