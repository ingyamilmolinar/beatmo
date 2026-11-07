//go:build test

// An open menu's per-frame cost must scale with what it DRAWS, not with how often something asks for its item
// list: the item model is a function of menu STATE (page, query, selection, locale) and is memoised on it.

package ui

import (
	"runtime"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"

	"github.com/hajimehoshi/ebiten/v2"
)

// Calibrated with headroom: they catch order-of-magnitude regressions, not micro-churn.
const (
	// A ratchet: lower it when real allocations drop; never raise it to pass a regression.
	templateMenuFrameAllocBudget = 385
	// Headroom absorbs GC and host variance.
	templateMenuFrameByteBudget = 12 << 10
)

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
	// The first frames pay one-off layout, portal and button-build costs.
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

// Both draw a clipped popup of the same size; only per-frame model rebuilding could make Templates cost much more.
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

// Draw work scales with the VISIBLE rows: rows outside the popup are discarded by the clip, so drawing them is
// wasted blits.
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

	// Partially visible rows at both edges still render.
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

// A stale memo would show the wrong page, ignore the query, mis-mark the active template or keep old-locale labels.
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
