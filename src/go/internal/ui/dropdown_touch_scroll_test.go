//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// ── InstrumentMenuComponent touch scroll tests ──────────────────────────

// A drag scrolls instead of selecting.
func TestInstMenuCompTouchScrollCategories(t *testing.T) {
	// Mobile only: desktop selects on press and scrolls by wheel/scrollbar.
	oldM := forceSmallScreenForTest
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = oldM })

	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		Categories: audio.Categories()[:10],
		Instruments: func() []InstrumentOption {
			var opts []InstrumentOption
			for _, cat := range audio.Categories()[:10] {
				opts = append(opts, InstrumentOption{
					ID: instrument.ID(cat.String() + "_inst"), Label: cat.Label() + " Inst", Category: cat,
				})
			}
			return opts
		}(),
		AnchorRect:      image.Rect(0, 50, 200, 70),
		VertBounds:      image.Rect(0, 0, 400, 600),
		RowHeight:       24,
		ForceCategories: true,
	})
	comp.Open()
	suppressClicksUntilRelease = false

	if comp.Mode() != InstMenuModeCategories {
		t.Fatalf("expected categories mode, got %s", comp.Mode())
	}

	// Find a category button to initiate drag on.
	catBtns := comp.CategoryBtns()
	if len(catBtns) == 0 {
		t.Fatal("no category buttons rendered")
	}
	targetRect := catBtns[0].Rect()
	startX := targetRect.Min.X + 5
	startY := targetRect.Min.Y + 5

	// Touch down on category button.
	comp.HandleInput(startX, startY, true)
	if !comp.IsOpen() {
		t.Fatal("menu closed on touch down")
	}

	// Drag vertically past dead zone (8px).
	dragY := startY + 20
	comp.HandleInput(startX, dragY, true)

	// Category should NOT be selected (still in categories mode, not switched).
	if comp.Mode() != InstMenuModeCategories {
		t.Fatal("category selected during scroll — should have been suppressed")
	}

	// Release — should not fire the deferred tap since scroll was committed.
	comp.HandleInput(startX, dragY, false)
	if comp.Mode() != InstMenuModeCategories {
		t.Fatal("category selected on release after scroll")
	}
}

// A drag scrolls instead of selecting.
func TestInstMenuCompTouchScrollInstruments(t *testing.T) {
	// Touch-scroll (drag = scroll, not select) is a mobile-only affordance.
	oldM := forceSmallScreenForTest
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = oldM })

	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	var selected string
	comp := NewInstrumentMenuComponent()
	insts := make([]InstrumentOption, 20)
	for i := range insts {
		id := "inst_" + string(rune('a'+i))
		insts[i] = InstrumentOption{ID: instrument.ID(id), Label: "Inst " + id, Category: audio.CatOther}
	}
	comp.SetProps(InstrumentMenuProps{
		Instruments: insts,
		AnchorRect:  image.Rect(0, 50, 200, 70),
		VertBounds:  image.Rect(0, 0, 400, 600),
		RowHeight:   24,
		OnSelect:    func(id instrument.ID) { selected = string(id) },
	})
	comp.Open()
	suppressClicksUntilRelease = false

	// Find an instrument button.
	instBtns := comp.InstBtns()
	if len(instBtns) == 0 {
		t.Fatal("no instrument buttons")
	}
	r := instBtns[0].Rect()
	sx, sy := r.Min.X+5, r.Min.Y+5

	// Touch + drag vertically past dead zone.
	comp.HandleInput(sx, sy, true)
	comp.HandleInput(sx, sy+20, true)
	comp.HandleInput(sx, sy+20, false)

	if selected != "" {
		t.Fatalf("instrument selected during scroll: %s", selected)
	}
	if !comp.IsOpen() {
		t.Fatal("menu closed during scroll")
	}
}

func TestInstMenuCompQuickTapSelectsCategory(t *testing.T) {
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		Categories: []audio.Category{audio.CatKick, audio.CatSnare},
		Instruments: []InstrumentOption{
			{ID: roster.Kick, Label: "Kick 1", Category: audio.CatKick},
			{ID: roster.Snare, Label: "Snare 1", Category: audio.CatSnare},
		},
		AnchorRect:      image.Rect(0, 50, 200, 70),
		VertBounds:      image.Rect(0, 0, 400, 600),
		RowHeight:       24,
		ForceCategories: true,
	})
	comp.Open()
	suppressClicksUntilRelease = false

	catBtns := comp.CategoryBtns()
	if len(catBtns) < 2 {
		t.Fatal("expected at least 2 category buttons")
	}
	// Find "Snares" button.
	var target *Button
	for _, b := range catBtns {
		if b.Text == "Snare" {
			target = b
			break
		}
	}
	if target == nil {
		t.Fatal("Snares button not found")
	}
	r := target.Rect()
	cx, cy := r.Min.X+5, r.Min.Y+5

	// Quick tap: press + release without significant movement.
	comp.HandleInput(cx, cy, true)
	comp.HandleInput(cx, cy, false)

	// After tap, should switch to instruments mode.
	if comp.Mode() != InstMenuModeInstruments {
		t.Fatalf("expected instruments mode after tap, got %s", comp.Mode())
	}
}

func TestInstMenuCompQuickTapSelectsInstrument(t *testing.T) {
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	var selected string
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		Instruments: []InstrumentOption{
			{ID: roster.Kick, Label: "Kick", Category: audio.CatNone},
			{ID: roster.Snare, Label: "Snare", Category: audio.CatNone},
		},
		AnchorRect: image.Rect(0, 50, 200, 70),
		VertBounds: image.Rect(0, 0, 400, 600),
		RowHeight:  24,
		OnSelect:   func(id instrument.ID) { selected = string(id) },
	})
	comp.Open()
	suppressClicksUntilRelease = false

	instBtns := comp.InstBtns()
	if len(instBtns) == 0 {
		t.Fatal("no instrument buttons")
	}
	r := instBtns[0].Rect()
	cx, cy := r.Min.X+5, r.Min.Y+5

	comp.HandleInput(cx, cy, true)
	comp.HandleInput(cx, cy, false)

	if selected == "" {
		t.Fatal("instrument not selected after quick tap")
	}
}

// ── Context menu deferred tap tests ─────────────────────────────────────

// Fire on release, not press.
func TestContextMenuDeferredTap(t *testing.T) {
	old := forceSmallScreenForTest
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = old })

	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 800), nil, logger)
	dv.Rows = append(dv.Rows, &DrumRow{Name: "Row 1", Instrument: roster.Kick})
	dv.calcLayout()
	dv.openContextMenu(0)

	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu not open")
	}
	if len(dv.contextMenuBtns) == 0 {
		t.Fatal("no context menu buttons")
	}

	// Released after the kebab, as in real use.
	suppressClicksUntilRelease = false

	// Find a button inside the menu.
	btn := dv.contextMenuBtns[0]
	r := btn.Rect()
	cx, cy := r.Min.X+2, r.Min.Y+2

	// Touch down — should not fire immediately.
	consumed := dv.handleContextMenuInput(cx, cy, true)
	if !consumed {
		t.Fatal("not consumed on press")
	}
	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu closed on press")
	}
	if !dv.contextMenuDeferredTap.Active() {
		t.Fatal("deferred tap not active")
	}

	// Release — fires the deferred tap.
	dv.handleContextMenuInput(cx, cy, false)
	// The first button is "Rename", which closes the context menu when fired.
	if dv.IsContextMenuOpen() {
		t.Fatal("context menu still open after tap release")
	}
}

// ── Overflow menu deferred tap tests ────────────────────────────────────

// Fire on release, not press.
func TestOverflowMenuDeferredTap(t *testing.T) {
	old := forceSmallScreenForTest
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = old })

	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 800), nil, logger)
	dv.Rows = append(dv.Rows, &DrumRow{Name: "Row 1", Instrument: roster.Kick})
	dv.calcLayout()

	// With no Game, the anchor falls back to the drum pane's top-right.
	dv.openOverflowMenuPortal()

	popupRect := dv.overflowPopupRect()
	if popupRect.Empty() {
		t.Fatal("overflow popup rect is empty")
	}
	cx := popupRect.Min.X + 10
	// Row 0 is the header; a couple of rows down is a file action, which closes the menu.
	cy := popupRect.Min.Y + 2*touchMinTargetPx + 10

	// Touch down — should not fire immediately.
	consumed := dv.handleOverflowMenuInput(cx, cy, true)
	if !consumed {
		t.Fatal("not consumed on press")
	}
	if !dv.IsOverflowMenuOpen() {
		t.Fatal("overflow menu closed on press")
	}
	// The shared MenuScroll now owns the overflow menu's deferred tap.
	if dv.overflowMenuScroll == nil || !dv.overflowMenuScroll.TapActive() {
		t.Fatal("deferred tap not active")
	}

	// Release — fires the deferred tap.
	dv.handleOverflowMenuInput(cx, cy, false)
	// The first action button is "Upload" which closes the overflow menu.
	if dv.IsOverflowMenuOpen() {
		t.Fatal("overflow menu still open after tap release")
	}
}

// ── openInstMenuForRow uses component path ──────────────────────────────

func TestInstMenuOpenForRowUsesComponent(t *testing.T) {
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, logger)
	dv.instOptions = []instrument.ID{roster.Kick, roster.Snare}
	dv.Rows = append(dv.Rows, &DrumRow{Name: "Row 1", Instrument: roster.Kick})
	dv.calcLayout()

	comp := NewInstrumentMenuComponent()
	dv.instMenuComp = comp

	dv.openInstMenuForRow(0)

	if !comp.IsOpen() {
		t.Fatal("instMenuComp.IsOpen() should be true after openInstMenuForRow")
	}
	if !dv.IsInstMenuOpen() {
		t.Fatal("instMenuOpen should be true")
	}
}

// ── Desktop immediate fire (no deferred tap) ────────────────────────────

// A quick press+release fires on desktop too.
func TestInstMenuDesktopImmediateFire(t *testing.T) {
	old := forceSmallScreenForTest
	forceSmallScreenForTest = false
	t.Cleanup(func() { forceSmallScreenForTest = old })

	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	var selected string
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		Instruments: []InstrumentOption{
			{ID: roster.Kick, Label: "Kick", Category: audio.CatNone},
			{ID: roster.Snare, Label: "Snare", Category: audio.CatNone},
		},
		AnchorRect: image.Rect(0, 50, 200, 70),
		VertBounds: image.Rect(0, 0, 400, 600),
		RowHeight:  24,
		OnSelect:   func(id instrument.ID) { selected = string(id) },
	})
	comp.Open()
	suppressClicksUntilRelease = false

	instBtns := comp.InstBtns()
	if len(instBtns) == 0 {
		t.Fatal("no instrument buttons")
	}

	// The deferred tap works on desktop as well.
	r := instBtns[0].Rect()
	cx, cy := r.Min.X+5, r.Min.Y+5

	comp.HandleInput(cx, cy, true)
	comp.HandleInput(cx, cy, false)

	if selected == "" {
		t.Fatal("instrument not selected on desktop quick tap")
	}
}
