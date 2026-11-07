//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Adding a menu to the kit is one row here.
type menuUnderTest struct {
	name     string
	open     func(g *Game)
	isOpen   func(g *Game) bool
	panel    func(g *Game) image.Rectangle
	closeBtn func(g *Game) *Button
	// mustAnswer menus (confirm dialogs) do NOT close on an outside tap.
	mustAnswer bool
}

func cardMenus() []menuUnderTest {
	return []menuUnderTest{
		{name: "theme", open: func(g *Game) { g.toggleThemeOverlay() }, isOpen: func(g *Game) bool { return g.drum.portal().Has(themeOverlayID) },
			panel: func(g *Game) image.Rectangle {
				return g.drum.portal().OverlayByID(themeOverlayID).(*ThemeOverlay).Rect()
			},
			closeBtn: func(g *Game) *Button {
				return g.drum.portal().OverlayByID(themeOverlayID).(*ThemeOverlay).Card().CloseBtn()
			}},
		{name: "settings", open: func(g *Game) { g.toggleSettingsOverlay() }, isOpen: func(g *Game) bool { return g.drum.portal().Has(settingsOverlayID) },
			panel: func(g *Game) image.Rectangle {
				return g.drum.portal().OverlayByID(settingsOverlayID).(*SettingsOverlay).rect
			},
			closeBtn: func(g *Game) *Button {
				return g.drum.portal().OverlayByID(settingsOverlayID).(*SettingsOverlay).card.CloseBtn()
			}},
		{name: "logs", open: func(g *Game) { g.toggleLogsPanel() }, isOpen: func(g *Game) bool { return g.drum.portal().Has(logsPanelID) },
			panel:    func(g *Game) image.Rectangle { return g.drum.ensureLogsComp().panelRect() },
			closeBtn: func(g *Game) *Button { return g.drum.ensureLogsComp().card.CloseBtn() }},
		{name: "file", open: func(g *Game) { g.toggleFileMenu() }, isOpen: func(g *Game) bool { return g.drum.portal().Has(overflowMenuID) },
			panel:    func(g *Game) image.Rectangle { return g.drum.overflowPopupRect() },
			closeBtn: func(g *Game) *Button { return g.drum.overflowCard.CloseBtn() }},
		{name: "context", open: func(g *Game) { ensureRow(g, 0); g.drum.OpenContextMenu(0) }, isOpen: func(g *Game) bool { return g.drum.portal().Has("context-menu") },
			panel:    func(g *Game) image.Rectangle { return g.drum.contextMenuRect },
			closeBtn: func(g *Game) *Button { return g.drum.contextMenuCloseButton }},
		{name: "nav", open: func(g *Game) { g.drum.OpenNavigation() }, isOpen: func(g *Game) bool { return g.drum.portal().Has(navMenuPortalID) },
			panel:    func(g *Game) image.Rectangle { return g.drum.navMenuComp.card },
			closeBtn: func(g *Game) *Button { return g.drum.navMenuComp.closeBtn }},
		{name: "paste-confirm", open: func(g *Game) { g.openPasteConfirmPortal() }, isOpen: func(g *Game) bool { return g.drum.portal().Has(pasteConfirmPortalID) },
			panel: func(g *Game) image.Rectangle {
				return g.drum.portal().OverlayByID(pasteConfirmPortalID).(*ConfirmDialog).rect
			},
			closeBtn: func(g *Game) *Button {
				return g.drum.portal().OverlayByID(pasteConfirmPortalID).(*ConfirmDialog).closeBtn
			}},
	}
}

// Real input through Game.Update: ×, Esc and an outside tap (unless it must be answered) close it, and it
// reopens afterwards.
func TestEveryMenuClosesTheSameWay(t *testing.T) {
	for _, m := range cardMenus() {
		t.Run(m.name, func(t *testing.T) {
			assertDefaultParityState(t)
			g := New(testLogger)
			t.Cleanup(g.CloseForTest)
			g.Layout(1280, 720)
			advanceFrames(g, 2)
			tap := makeHoldTap(g, 1280, 720)
			screen := ebiten.NewImage(1280, 720)
			openIt := func() {
				m.open(g)
				advanceFrames(g, 2)
				g.Draw(screen) // comp-style menus lay out in Draw
				advanceFrames(g, 1)
				if !m.isOpen(g) {
					t.Fatalf("%s did not open", m.name)
				}
			}
			// 1. close ×
			openIt()
			if desktopCloseVisible() {
				cb := m.closeBtn(g)
				if cb == nil || cb.Rect().Empty() {
					t.Fatalf("%s: no close × on desktop", m.name)
				}
				if !cb.Rect().In(m.panel(g)) {
					t.Fatalf("%s: close × %v outside panel %v", m.name, cb.Rect(), m.panel(g))
				}
				cx, cy := center(cb.Rect())
				tap(cx, cy, 3)
				advanceFrames(g, 2)
				if m.isOpen(g) {
					t.Fatalf("%s: close × did not close it", m.name)
				}
			}
			// 2. Esc
			openIt()
			restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
			advanceFrames(g, 1)
			restore()
			advanceFrames(g, 1)
			if m.isOpen(g) {
				t.Fatalf("%s: Esc did not close it", m.name)
			}
			// A drum-pane cell: the click-away consumes the press, so no step toggles.
			openIt()
			panel := m.panel(g)
			ox, oy := 700, 500
			if image.Pt(ox, oy).In(panel) {
				ox, oy = 200, 700
			}
			if image.Pt(ox, oy).In(panel) {
				t.Fatalf("%s: could not find an outside point for panel %v", m.name, panel)
			}
			tap(ox, oy, 3)
			advanceFrames(g, 2)
			if m.isOpen(g) == !m.mustAnswer {
				t.Fatalf("%s: outside tap: open=%v mustAnswer=%v", m.name, m.isOpen(g), m.mustAnswer)
			}
			if m.isOpen(g) {
				g.drum.portal().CloseTop()
			}
			// 4. reopens cleanly
			openIt()
		})
	}
}
