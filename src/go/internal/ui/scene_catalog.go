//go:build !noscenes

package ui

import (
	"fmt"
	"image"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/theme"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/scope"
)

// sceneCatalog enumerates every UI surface the screenshots-all target captures.
// Names are kebab-case and stable; tooling and tests refer to them by name.
var sceneCatalog = []Scene{
	// ─── transport / baseline ─────────────────────────────────────
	{Name: "transport_idle", Description: "default boot, no playback", Mobile: true, Setup: func(g *Game) {}},
	{Name: "transport_playing", Description: "playback running",
		Setup: func(g *Game) { g.SetPlaying(true) }},
	{Name: "transport_high_bpm", Description: "BPM at upper end",
		Setup: func(g *Game) { sceneSetBPM(g, 240) }},
	{Name: "shortcuts_overlay", Description: "settings overlay open (grid gear button)",
		Setup: func(g *Game) { g.toggleSettingsOverlay() }},
	{Name: "theme_editor", Description: "theme editor overlay open (grid palette button)",
		Setup: func(g *Game) { g.toggleThemeOverlay() }},
	{Name: "menu_outside_tap_grid", Description: "theme editor opened by a real tap on its grid button, then a real tap on the empty grid outside it: menu closed, NO node created",
		Mobile: true, Setup: menuOutsideTapGridSceneSetup},
	{Name: "theme_applied_background_only", Description: "only the Background category themed (mint-500): canvas, stripes, rack bed, toolbar bed and timeline bed all follow; controls stay",
		Setup: func(g *Game) {
			g.SetCircuitTheme(theme.Doc{Categories: map[string]string{"background": "mint-500"}})
		}},
	{Name: "theme_editor_adjust", Description: "theme editor with the Adjust colors section expanded",
		Setup: func(g *Game) {
			g.toggleThemeOverlay()
			if ov, ok := g.drum.portal().TopOverlay().(*ThemeOverlay); ok {
				ov.SetAdjustExpanded(true)
			}
		}},
	{Name: "theme_editor_keynav", Description: "theme editor with the keyboard cursor on the third preset row (arrows/Enter)",
		Setup: func(g *Game) {
			g.toggleThemeOverlay()
			if ov, ok := g.drum.portal().TopOverlay().(*ThemeOverlay); ok {
				ov.MenuKeyNavForTest().BindHost(ov)
				ov.MenuKeyNavForTest().SetSelected(2)
			}
		}},
	{Name: "theme_editor_picker", Description: "theme editor with the Accent swatch picker open",
		Setup: func(g *Game) {
			g.toggleThemeOverlay()
			if ov, ok := g.drum.portal().TopOverlay().(*ThemeOverlay); ok {
				ov.SetAdjustExpanded(true)
				g.openThemeSwatchPicker(ov, "accent")
			}
		}},
	{Name: "theme_applied_orchid", Description: "whole UI recolored by a circuit theme: accent orchid-300, background electric-blue-500, viz mint-300 (Synth tab open)", SettleFrames: 60,
		Setup: func(g *Game) {
			g.SetCircuitTheme(theme.Doc{Categories: map[string]string{"accent": "orchid-300", "background": "electric-blue-500", "viz": "mint-300"}})
			_ = g.SetActiveEQTab("synth")
		},
		MobileSetup: func(g *Game) {
			g.SetCircuitTheme(theme.Doc{Categories: map[string]string{"accent": "orchid-300", "background": "electric-blue-500", "viz": "mint-300"}})
			mobileAudioPanelSetup("synth")(g)
		}},

	// ─── logs panel + date/time picker ────────────────────────────
	{Name: "logs_panel", Description: "Logs panel open — buttons, level chips, At|Range toggle, scrollable rows",
		SettleFrames: 8,
		Setup: func(g *Game) {
			g.drum.notifyInfo("Loaded demo.json")
			// Enough ERROR rows to overflow the page body and show the
			// MenuScroll scrollbar (default filter is ERROR).
			for i := 0; i < 30; i++ {
				g.drum.notifyError(fmt.Sprintf("Import failed: bad JSON near token %d", i))
			}
			g.toggleLogsPanel()
		}},
	{Name: "logs_panel_cleared", Description: "Logs panel after Clear history — empty list, page 1/1",
		SettleFrames: 8,
		Setup: func(g *Game) {
			for i := 0; i < 40; i++ {
				g.drum.notifyError(fmt.Sprintf("Import failed: bad JSON near token %d", i))
			}
			g.toggleLogsPanel()
			if c := g.drum.logsComp; c != nil {
				dst := ebiten.NewImage(1280, 720)
				c.Draw(dst) // lays out the Clear button
				// Click Clear through the real dispatch, then draw the empty state.
				cx, cy := c.clearRect.Min.X+c.clearRect.Dx()/2, c.clearRect.Min.Y+c.clearRect.Dy()/2
				c.HandleInput(cx, cy, true)
				c.HandleInput(cx, cy, false)
				c.Draw(dst)
			}
		}},
	{Name: "logs_panel_search", Description: "Logs panel with an active search — results + pagination refresh in real time (snaps to page 1)",
		SettleFrames: 8,
		Setup: func(g *Game) {
			for i := 0; i < 40; i++ {
				g.drum.notifyError(fmt.Sprintf("Import failed: bad JSON near token %d", i))
			}
			g.toggleLogsPanel()
			if c := g.drum.logsComp; c != nil {
				dst := ebiten.NewImage(1280, 720)
				c.Draw(dst)
				c.page = 2 // pretend the user had paged forward
				// Typing a query must narrow the results AND refresh pagination
				// back to page 1 in the same frame (real-time reapply).
				c.searchBox.SetText("token 3")
				c.Update()
				c.Draw(dst)
			}
		}},
	{Name: "logs_panel_leading_space", Description: "Logs search box with a LEADING SPACE (\" token\") — the space must be visible: text shifted right, caret after it",
		SettleFrames: 8,
		Setup: func(g *Game) {
			g.drum.notifyError("seed")
			g.toggleLogsPanel()
			if c := g.drum.logsComp; c != nil {
				dst := ebiten.NewImage(1280, 720)
				c.Draw(dst)                   // create + lay out the search box
				c.searchBox.SetText(" token") // leading space
				c.searchBox.FocusForTest(true)
				c.searchBox.cursor = 1 // caret right after the leading space
				c.Update()
				c.Draw(dst)
			}
		}},
	{Name: "logs_panel_search_caret", Description: "Logs panel search box focused with the caret mid-string — caret renders between characters (arrow/Home/End navigation)",
		SettleFrames: 8,
		Setup: func(g *Game) {
			for i := 0; i < 12; i++ {
				g.drum.notifyError(fmt.Sprintf("Import failed: bad JSON near token %d", i))
			}
			g.toggleLogsPanel()
			if c := g.drum.logsComp; c != nil {
				dst := ebiten.NewImage(1280, 720)
				c.Draw(dst) // creates + lays out the search box
				c.searchBox.SetText("token")
				c.searchBox.FocusForTest(true)
				c.searchBox.cursor = 2 // caret between "to" and "ken"
				c.Update()
				c.Draw(dst)
			}
		}},
	{Name: "logs_panel_scrolled", Description: "Logs panel wheel-scrolled mid-list — thumb moved off the top, rows offset",
		SettleFrames: 8,
		Setup: func(g *Game) {
			for i := 0; i < 40; i++ {
				g.drum.notifyError(fmt.Sprintf("Import failed: bad JSON near token %d", i))
			}
			g.toggleLogsPanel()
			if c := g.drum.logsComp; c != nil {
				dst := ebiten.NewImage(1280, 720)
				c.Draw(dst) // Configure the MenuScroll before wheeling
				// Drive real wheel notches through the shared smooth wheel path
				// (continuous, magnitude-respecting, no cooldown lock — the same
				// path every list menu now uses), advancing the list mid-page.
				for k := 0; k < 12; k++ {
					c.HandleWheel(0, 0, -1)
					c.Draw(dst)
				}
			}
		}},
	{Name: "logs_panel_page2", Description: "Logs panel after next-page — opens at the top of page 2 (scroll reset on page switch)",
		SettleFrames: 8,
		Setup: func(g *Game) {
			for i := 0; i < 80; i++ { // 2 pages over the fixed 50/page
				g.drum.notifyError(fmt.Sprintf("Import failed: bad JSON near token %d", i))
			}
			g.toggleLogsPanel()
			if c := g.drum.logsComp; c != nil {
				dst := ebiten.NewImage(1280, 720)
				c.Draw(dst) // Configure the scroll
				// Scroll page 1 partway down, then advance a page: the scroll must
				// snap back so page 2 opens at its first row.
				c.scroll.ScrollBehavior().VS.First = 6
				c.Draw(dst)
				c.pageNext()
				c.Draw(dst)
			}
		}},
	{Name: "logs_error_badge", Description: "grid Logs icon showing an unread-error badge (panel closed)",
		SettleFrames: 5,
		Setup: func(g *Game) {
			g.drum.notifyInfo("Loaded demo.json") // info does NOT badge
			g.drum.notifyError("Import failed: bad JSON")
		}},
	{Name: "logs_datetime_picker", Description: "Logs date/time picker open (calendar + time)",
		SettleFrames: 8,
		Setup: func(g *Game) {
			g.drum.notifyError("Import failed: bad JSON")
			g.toggleLogsPanel()
			if c := g.drum.logsComp; c != nil {
				c.Draw(ebiten.NewImage(1280, 720)) // lay out field buttons
				c.openPicker(pickAt)
			}
		}},
	{Name: "toast_circuit_loading", Description: "circuit-loading progress toast — desktop/Pads: grid pane top-centre, Play disabled by the readiness gate",
		Mobile:      true,
		Setup:       toastCircuitLoadingSceneSetup,
		MobileSetup: toastCircuitLoadingMobileSceneSetup},
	{Name: "crop_toast_circuit_loading", Description: "circuit-loading progress toast, cropped to the toast surface (label + counts + progress bar)",
		Subject: SubjectWarmupToast,
		Setup:   toastCircuitLoadingSceneSetup},

	// ─── EQ tabs ──────────────────────────────────────────────────
	// Mobile pass enters SetMobileEQMode so the panel is actually visible;
	// without it the audio surface stays collapsed and the screenshot
	// duplicates mobile_default.
	{Name: "eq_tab_eq", Description: "EQ tab active", Mobile: true,
		Setup:       func(g *Game) { _ = g.SetActiveEQTab("eq") },
		MobileSetup: mobileAudioPanelSetup("eq")},
	{Name: "eq_tab_wave", Description: "Wave tab active", Mobile: true,
		Setup:       func(g *Game) { _ = g.SetActiveEQTab("wave") },
		MobileSetup: mobileAudioPanelSetup("wave")},
	{Name: "eq_tab_spectrum", Description: "Spectrum tab active", Mobile: true,
		Setup:       func(g *Game) { _ = g.SetActiveEQTab("spectrum") },
		MobileSetup: mobileAudioPanelSetup("spectrum")},
	{Name: "eq_tab_levels", Description: "Levels tab active", Mobile: true,
		Setup:       func(g *Game) { _ = g.SetActiveEQTab("levels") },
		MobileSetup: mobileAudioPanelSetup("levels")},
	{Name: "eq_tab_synth", Description: "Synth tab active (recipe knob editor)", Mobile: true, SettleFrames: 60,
		Setup: func(g *Game) {
			_ = g.SetActiveEQTab("synth")
			settleSynthTab(g)
		}},
	{Name: "eq_tab_chain", Description: "Chain tab active", Mobile: true, SettleFrames: 120,
		Setup: func(g *Game) { _ = g.SetActiveEQTab("chain"); g.SetChainVisible(true) },
		MobileSetup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.SetMobileEQMode(true)
			_ = g.SetActiveEQTab("chain")
			g.SetChainVisible(true)
		}},
	{Name: "eq_with_band_adjusted", Description: "two EQ bands tweaked", Mobile: true,
		Setup: func(g *Game) {
			_ = g.SetActiveEQTab("eq")
			g.SetEQBandGain(channel.Master, 2, -6)
			g.SetEQBandGain(channel.Master, 5, 4)
		},
		MobileSetup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.SetMobileEQMode(true)
			_ = g.SetActiveEQTab("eq")
			g.SetEQBandGain(channel.Master, 2, -6)
			g.SetEQBandGain(channel.Master, 5, 4)
		}},

	// ─── per-row menus ────────────────────────────────────────────
	{Name: "context_menu_open", Description: "row context menu open", Mobile: true, SettleFrames: 120,
		Setup: func(g *Game) { ensureRow(g, 0); g.drum.OpenContextMenu(0) }},
	{Name: "instrument_menu_open", Description: "instrument selector open", Mobile: true, SettleFrames: 120,
		Setup: func(g *Game) { ensureRow(g, 0); g.drum.OpenInstrumentMenu(0) }},
	{Name: "instrument_menu_categories", Description: "instrument selector in categories mode", Mobile: true, SettleFrames: 120,
		Setup: func(g *Game) {
			ensureRow(g, 0)
			g.drum.instMenuForceCategories = true
			g.drum.OpenInstrumentMenu(0)
		}},
	{Name: "instrument_menu_wav_named_snare", Description: "a WAV uploaded under the name \"Snare\" is its own user-sample instrument: the search lists it beside the built-in Snare, which it no longer replaces", Mobile: true, SettleFrames: 120,
		Setup: func(g *Game) {
			ensureRow(g, 0)
			g.drum.pendingWAV = "" // a silent placeholder sample: the scene shows identity, not audio
			g.drum.registerInstrument("Snare")
			g.drum.instMenuForceCategories = true
			g.drum.OpenInstrumentMenu(0)
			if c := g.drum.instMenuComp; c != nil && c.SearchBox() != nil {
				c.SearchBox().SetText("snare")
			}
		}},
	{Name: "instrument_menu_snare_acoustic", Description: "instrument selector global-searched for \"acoustic\" — shows the reference-matched Snare Acoustic row", Mobile: true, SettleFrames: 120,
		Setup: func(g *Game) {
			ensureRow(g, 0)
			// Categories mode is the GLOBAL (cross-category) search path, so a
			// query surfaces the whole snare family rather than filtering
			// within whichever category the menu happened to open on.
			g.drum.instMenuForceCategories = true
			g.drum.OpenInstrumentMenu(0)
			if c := g.drum.instMenuComp; c != nil && c.SearchBox() != nil {
				c.SearchBox().SetText("acoustic")
			}
		}},
	{Name: "instrument_menu_keycursor", Description: "instrument selector with the KEYBOARD cursor moved off the applied instrument — the focus ring is the border highlight the arrows move, and must stay distinguishable from the active-instrument row highlight", Mobile: true, SettleFrames: 120,
		Setup:       instMenuKeyCursorSetup(false),
		MobileSetup: instMenuKeyCursorSetup(true)},
	{Name: "instrument_menu_favorites_empty", Description: "instrument selector Favorites view with no ★s", Mobile: true, SettleFrames: 120,
		Setup: func(g *Game) { openEmptyFavoritesView(g) },
		MobileSetup: func(g *Game) {
			g.SetForceMobileProfile(true)
			openEmptyFavoritesView(g)
		}},
	{Name: "color_wheel_keycursor", Description: "colour picker with the KEYBOARD cursor moved off the applied swatch — arrows navigate the palette instead of panning the grid", Mobile: true,
		Setup: func(g *Game) {
			g.drum.openColorPickerForRow(0)
			c := g.drum.colorWheelComp
			if c == nil || !c.IsOpen() {
				return
			}
			c.ClearHold()
			// Place the cursor deterministically a couple of swatches away from
			// the applied one; the scene only needs the resulting visual state.
			c.KeyNav().BindHost(colorWheelNavHost{c: c})
			c.KeyNav().SetSelected(colorSwatchCols + 2)
		}},
	{Name: "color_wheel_open", Description: "color wheel picker open", Mobile: true, SettleFrames: 120,
		Setup: func(g *Game) { ensureRow(g, 0); g.drum.OpenColorMenu(0) }},
	{Name: "subdiv_menu_open", Description: "subdivision menu open", Mobile: true, SettleFrames: 120,
		Setup: func(g *Game) { g.drum.OpenSubdivMenu() },
		MobileSetup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.OpenSubdivMenu()
		}},
	{Name: "transport_jump_menu_open", Description: "FF/RW jump-size (×N) dropdown open", SettleFrames: 120,
		Setup: func(g *Game) {
			if g.drum != nil {
				g.drum.OpenJumpMenu()
			}
		}},

	// ─── FX panel ─────────────────────────────────────────────────
	{Name: "fx_panel_open_empty", Description: "FX panel with no effects", Mobile: true, SettleFrames: 120,
		Setup: func(g *Game) { ensureRow(g, 0); g.drum.OpenFXPanel(0) }},
	{Name: "fx_add_effect_menu", Description: "FX panel add-effect picker (effect-type list) open", Mobile: true, SettleFrames: 120,
		Setup: func(g *Game) {
			ensureRow(g, 0)
			audio.AddInsertEffect(g.drum.Rows[0].Instrument, audio.EffectDelay, nil)
			g.drum.OpenFXPanel(0)
			g.drum.fxAddMenuOpen = true
			g.drum.buildFXPanel()
			g.drum.refreshFXPortalHitAreas()
		}},
	{Name: "fx_add_effect_menu_search", Description: "FX add-effect picker with an active search query (flat, filtered)", Mobile: true, SettleFrames: 120,
		Setup: func(g *Game) {
			ensureRow(g, 0)
			g.drum.OpenFXPanel(0)
			g.drum.fxAddMenuOpen = true
			g.drum.buildFXPanel()
			g.drum.setFXAddSearchQuery("re")
			g.drum.buildFXPanel()
			g.drum.refreshFXPortalHitAreas()
		}},
	{Name: "fx_panel_with_3_effects", Description: "delay+reverb+distortion stacked", Mobile: true, SettleFrames: 150,
		Setup: func(g *Game) {
			ensureRow(g, 0)
			instID := g.drum.Rows[0].Instrument
			audio.AddInsertEffect(instID, audio.EffectDelay, nil)
			audio.AddInsertEffect(instID, audio.EffectReverb, nil)
			audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
			g.drum.OpenFXPanel(0)
		}},

	// Overflowing FX list — enough effects that the content exceeds the clamped
	// panel, so the shared MenuScroll scrollbar (pixel mode) must render. Guards
	// the FX-scroll migration onto the shared list-menu engine.
	{Name: "fx_panel_scrollbar", Description: "FX list overflows so the shared scrollbar shows", SettleFrames: 150,
		Setup: func(g *Game) {
			ensureRow(g, 0)
			instID := g.drum.Rows[0].Instrument
			for i := 0; i < 8; i++ {
				audio.AddInsertEffect(instID, audio.EffectDelay, nil)
			}
			g.drum.OpenFXPanel(0)
		}},

	// ─── volume popups ────────────────────────────────────────────
	{Name: "master_vol_popup", Description: "master volume slider open", Mobile: true,
		Setup: func(g *Game) { g.drum.OpenMasterVolumePopup() },
		MobileSetup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.OpenMasterVolumePopup()
		}},

	// ─── graph / sidebar ──────────────────────────────────────────
	{Name: "node_added", Description: "extra node placed at (2,2)",
		Setup: func(g *Game) { g.tryAddNode(2, 2, model.NodeTypeRegular); g.updateBeatInfos() }},
	{Name: "edge_built", Description: "two nodes connected",
		Setup: func(g *Game) {
			a := g.tryAddNode(2, 0, model.NodeTypeRegular)
			b := g.tryAddNode(2, 2, model.NodeTypeRegular)
			if a != nil && b != nil {
				g.addEdge(a, b)
			}
			g.updateBeatInfos()
		}},
	{Name: "node_sidebar_open", Description: "node parameter sidebar open",
		Setup: func(g *Game) {
			n := g.tryAddNode(3, 1, model.NodeTypeRegular)
			g.updateBeatInfos()
			if n != nil && g.sidebar != nil {
				g.sidebar.Open(n)
			}
		}},
	{Name: "node_sidebar_low_node", Description: "node sidebar opened on a node near the bottom of the grid pane — the card must stay within the grid pane and NOT overlap the bottom drum-view panel (regression: portal spill)",
		Setup: func(g *Game) {
			n := g.tryAddNode(3, 1, model.NodeTypeRegular)
			g.updateBeatInfos()
			// Pan so the tapped node sits near the bottom of the grid pane, where a
			// window-clamped card would spill past the split into the drum panel.
			g.cam.OffsetY = float64(g.split.GridH(g.winH)) - 80
			if n != nil && g.sidebar != nil {
				g.sidebar.Open(n)
			}
		}},
	{Name: "node_menu_dragged", Description: "node pop-up moved by its top-left drag grip — card sits at the dropped position (not node-anchored), grip glyph visible, title clear of it", Mobile: true,
		Setup: func(g *Game) {
			n := g.tryAddNode(3, 1, model.NodeTypeRegular)
			g.updateBeatInfos()
			if n == nil || g.sidebar == nil {
				return
			}
			g.sidebar.Open(n)
			g.sidebar.layout()
			sceneDragMenuGrip(g, g.sidebar.rects[sbPanel], &g.sidebar.drag, image.Pt(140, 90))
			g.sidebar.layout()
		}},
	{Name: "coord_badge_fraction", Description: "coordinate badge on a node placed off the beat (grid 4,4 at subdiv 8) — badge must read beat:fraction (0:1/2, 0:1/2), NOT raw whole units", Mobile: true, KeepCoordBadge: true,
		Setup: func(g *Game) {
			_ = g.SetSubdivisions(8)
			n := g.tryAddNode(4, 4, model.NodeTypeRegular)
			g.updateBeatInfos()
			if n != nil {
				g.sel = n
				n.Selected = true
				g.coordBadgeNode = n
				g.coordBadgeFrame = g.frame
			}
		}},
	// The badge and the sidebar note chip both read nodeNoteLabel, which needs
	// the node to resolve to a row carrying a PITCHED instrument. Every other
	// node scene places an off-circuit node, so neither surface had ever been
	// captured with a note actually present.
	{Name: "coord_badge_note", Description: "coordinate badge on a pitched node — badge must lead with the sounding note (e.g. \"A3 · (0:1/2, 0:1/2)\")", Mobile: true, KeepCoordBadge: true,
		Setup:       pitchedNodeBadgeSetup(false),
		MobileSetup: pitchedNodeBadgeSetup(true)},
	{Name: "node_sidebar_pitched_row", Description: "node sidebar on a pitched node — Pitch row must show the read-only note chip beside the -/+ buttons", Mobile: true,
		Setup:       pitchedNodeSidebarSetup(false),
		MobileSetup: pitchedNodeSidebarSetup(true)},
	{Name: "note_matrix_open", Description: "note/octave matrix picker open from the Pitch section's note chip (12 pitch-class rows × octave columns, current note ringed)", Mobile: true,
		Setup:       noteMatrixSetup(false),
		MobileSetup: noteMatrixSetup(true)},
	{Name: "note_matrix_after_pick", Description: "note matrix after ONE cell pick — the picker stays open (select applies, re-select confirms) and the highlight has moved to the newly applied note", Mobile: true,
		Setup:       noteMatrixAfterPickSetup(false),
		MobileSetup: noteMatrixAfterPickSetup(true)},
	{Name: "note_matrix_keycursor", Description: "note matrix with the KEYBOARD cursor moved off the current note — focus ring must be distinguishable from the selected-note ring", Mobile: true,
		Setup:       noteMatrixKeyCursorSetup(false),
		MobileSetup: noteMatrixKeyCursorSetup(true)},
	// Chord nodes (docs/superpowers/specs/2026-10-09-chord-nodes-design.md).
	{Name: "node_chord_sidebar", Description: "node sidebar on a CHORD node (Cmaj7/E-style) — wide chord chip in the Pitch row, Chord section with Inv / Spread steppers and a Single note row, Audible row reads Chord", Mobile: true,
		Setup:       chordNodeSidebarSetup(false),
		MobileSetup: chordNodeSidebarSetup(true)},
	{Name: "node_type_rotate_to_chord", Description: "node card right after ONE tap on the Audible (type) button turned a regular node into a chord — the Chord section opened above it, the card held its place and the Audible button is still visible under the tap point (it used to be pushed off the card's bottom)", Mobile: true,
		Setup:       nodeTypeRotateToChordSetup(false),
		MobileSetup: nodeTypeRotateToChordSetup(true)},
	{Name: "chord_matrix", Description: "chord matrix open from a chord node's chip — Triads page, 12 root rows × 5 qualities, current chord tinted, page bar Triads · 7ths · Ext · Sus", Mobile: true,
		Setup:       chordMatrixSetup(false, 0),
		MobileSetup: chordMatrixSetup(true, 0)},
	{Name: "chord_matrix_ext", Description: "chord matrix on the Ext page — the widest names (maj9, m9) must stay legible inside touch-min cells", Mobile: true,
		Setup:       chordMatrixSetup(false, 2),
		MobileSetup: chordMatrixSetup(true, 2)},
	{Name: "crop_chord_matrix", Description: "chord matrix, cropped to the grid pane", Subject: SubjectMainGrid,
		Setup: chordMatrixSetup(false, 1)},
	{Name: "chord_node_badge", Description: "a chord node on the grid — stacked-notehead glyph on the node, coordinate badge leading with the chord name", Mobile: true, KeepCoordBadge: true,
		Setup:       chordNodeBadgeSetup(false),
		MobileSetup: chordNodeBadgeSetup(true)},
	{Name: "crop_chord_drum_row", Description: "drum rows with a chord node in the circuit — chord cells carry three stacked ticks", Subject: SubjectDrumRows,
		Setup: chordNodeBadgeSetup(false)},
	// Universal note display (2026-09-13): percussion names a note too. These
	// are the regression surface for it — before, every one of these rows drew
	// a bare "+0" and the note chip was absent entirely.
	{Name: "drum_note_matrix_kick", Description: "note matrix open on a KICK row — a kick's base is 55 Hz, so the columns run into the sub-contra octaves (A0 and below) that the old piano-range clamp collapsed", Mobile: true,
		Setup:       noteMatrixSetupFor(false, roster.Kick),
		MobileSetup: noteMatrixSetupFor(true, roster.Kick)},
	{Name: "drum_note_matrix_snare", Description: "note matrix open on a SNARE row — snare resolves via its declared 186 Hz head, so the picker centres near F#3", Mobile: true,
		Setup:       noteMatrixSetupFor(false, roster.Snare),
		MobileSetup: noteMatrixSetupFor(true, roster.Snare)},
	{Name: "drum_note_sidebar_808", Description: "node sidebar on an 808-KICK row — the Pitch row must show a note chip at F1 (its declared 44 Hz; the 50 Hz measurement was rejected as detector-floor-pinned), NOT the A3 the old declared-param scan reported", Mobile: true,
		Setup:       pitchedNodeSidebarSetupFor(false, roster.Kick808),
		MobileSetup: pitchedNodeSidebarSetupFor(true, roster.Kick808)},
	{Name: "unpitched_row_no_note_chip", Description: "node sidebar on a HI-HAT row — genuinely unpitched noise, so the Pitch row shows the bare -/+ stepper with NO note chip and no gap where one would sit", Mobile: true,
		Setup:       pitchedNodeSidebarSetupFor(false, roster.Hihat),
		MobileSetup: pitchedNodeSidebarSetupFor(true, roster.Hihat)},
	{Name: "cymbal_row_no_note_chip", Description: "node sidebar on a RIDE row — a cymbal reads 501 Hz at confidence 0.998 but has no energy there (lowest partial 6x), so the spectral-support gate withholds the note chip", Mobile: true,
		Setup:       pitchedNodeSidebarSetupFor(false, roster.Ride),
		MobileSetup: pitchedNodeSidebarSetupFor(true, roster.Ride)},
	{Name: "coord_badge_note_drum", Description: "coordinate badge on a kick node — badge leads with the drum's sounding note, the surface that was plain coordinates for all percussion before", Mobile: true, KeepCoordBadge: true,
		Setup:       pitchedNodeBadgeSetupFor(false, roster.Kick),
		MobileSetup: pitchedNodeBadgeSetupFor(true, roster.Kick)},
	{Name: "group_menu_open", Description: "group menu open over three adjacent grouped nodes (provisional — Save visible, member rings)", Mobile: true,
		Setup: sceneGroupMenuSetup},
	{Name: "group_menu_dragged", Description: "group menu moved by its top-left drag grip — card sits at the dropped position, member avoidance no longer applies, grip glyph visible", Mobile: true,
		Setup: func(g *Game) {
			sceneGroupMenuSetup(g)
			if g.groupMenu == nil || !g.groupMenu.IsOpen() {
				return
			}
			sceneDragMenuGrip(g, g.groupMenu.panel, &g.groupMenu.drag, image.Pt(-120, -90))
			g.groupMenu.layoutAt(g.groupMenu.anchorX, g.groupMenu.anchorY)
		}},
	{Name: "group_badges", Description: "two committed groups with distinct colors, both floating badges visible (non-overlapping), menu open for one showing tinted chrome + both ring colors", Mobile: true,
		Setup: sceneGroupBadgesSetup},
	{Name: "node_longpress_menu", Description: "long-press quick-action popup (Move/Connect/Delete) on a node", Mobile: true, SettleFrames: 60,
		Setup: func(g *Game) {
			n := g.tryAddNode(3, 1, model.NodeTypeRegular)
			g.updateBeatInfos()
			if n != nil {
				sx1, sy1, _, _ := g.nodeScreenRect(n)
				g.showLongPressPopup(n, int(sx1), int(sy1))
			}
		}},
	{Name: "paste_confirm_dialog", Description: "paste-overwrite confirmation modal", Mobile: true, SettleFrames: 90,
		Setup: func(g *Game) {
			n := g.tryAddNode(3, 1, model.NodeTypeRegular)
			g.updateBeatInfos()
			if n != nil {
				g.snapshotNodeToClipboard(n)
				g.beginPasteConfirm(3, 1)
			}
		}},
	{Name: "break_circuit_confirm", Description: "break-the-circuit confirmation modal", Mobile: true, SettleFrames: 90,
		Setup: func(g *Game) {
			n := g.tryAddNode(3, 1, model.NodeTypeRegular)
			g.updateBeatInfos()
			if n != nil {
				g.modes.Enter(GridModeMove, n)
				g.beginBreakConfirm(n, 4, 1)
			}
		}},

	// ─── multi-row state ──────────────────────────────────────────
	{Name: "multi_row_full_grid", Description: "five rows total (rack shows multiple distinct rows)", Mobile: true,
		Setup: func(g *Game) { ensureMultiRow(g, 5) }},
	{Name: "row_muted_soloed", Description: "row 0 muted, row 2 soloed",
		Setup: func(g *Game) {
			ensureRow(g, 2)
			if 0 < len(g.drum.Rows) {
				g.drum.Rows[0].Muted = true
			}
			if 2 < len(g.drum.Rows) {
				g.drum.Rows[2].Solo = true
			}
			g.drum.markRowControlsDirty()
		}},
	{Name: "solo_then_delete_past_immutable", Description: "playing: row 0 soloed, then a corner node on the solo-gated Snare row deleted (its loop breaks, so its future empties) — the Snare hits left of the playhead must stay", SettleFrames: 300,
		Setup: sceneSoloThenDeletePastImmutable},

	// ─── recording ────────────────────────────────────────────────
	{Name: "transport_recording", Description: "recording session active", Mobile: true, SettleFrames: 120,
		Setup: func(g *Game) { _ = g.StartRecording() },
		MobileSetup: func(g *Game) {
			g.SetForceMobileProfile(true)
			_ = g.StartRecording()
		}},

	// ─── mobile-only surfaces ─────────────────────────────────────
	{Name: "mobile_default", Description: "mobile profile default boot", Mobile: true,
		Setup: func(g *Game) { g.SetForceMobileProfile(true) }},
	{Name: "mobile_overflow_open", Description: "overflow menu (Upload/Import/Export)", Mobile: true, SettleFrames: 120,
		Setup: func(g *Game) { g.SetForceMobileProfile(true); g.drum.OpenOverflowMenu() }},
	{Name: "overflow_menu_open", Description: "desktop file menu (Upload/Import/Export) under the grid folder button", SettleFrames: 120,
		Setup: func(g *Game) { g.openFileMenu() }},
	{Name: "nav_menu_open", Description: "navigation menu under the beat/time counter", SettleFrames: 120,
		Setup:       func(g *Game) { g.drum.OpenNavigation() },
		MobileSetup: func(g *Game) { g.SetForceMobileProfile(true); g.drum.OpenNavigation() }},
	// Playhead parked mid-beat (30 subdivisions == beat 3.75) so the nav menu's
	// position title and the beat counter above it are both off zero and can be
	// read against each other: both must show the SAME "Beat N · M:SS", since
	// both render the transport's resolved PlayheadPos.
	{Name: "nav_menu_position_sync", Description: "navigation menu + beat counter at a mid-beat playhead — both readouts must agree", SettleFrames: 120,
		Setup: func(g *Game) {
			g.drum.RequestSeekSteps(30, hooks.SeekCounter)
			g.drum.OpenNavigation()
		},
		MobileSetup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.RequestSeekSteps(30, hooks.SeekCounter)
			g.drum.OpenNavigation()
		}},
	// Go-to wheel (docs/superpowers/specs/2026-10-03-value-wheel-and-text-field-
	// design.md §2): the navigation menu with its inline wheel scrolled four
	// beats past a mid-beat playhead — the pending position previews as a ghost
	// on the timeline, nothing seeks. Labels are 1-based ("Beat 8 · 3/4").
	{Name: "nav_goto_beat", Description: "navigation menu go-to wheel (beat mode) scrolled to a pending position — timeline ghost, no seek", SettleFrames: 120,
		Setup:       func(g *Game) { sceneGotoWheel(g, false) },
		MobileSetup: func(g *Game) { g.SetForceMobileProfile(true); sceneGotoWheel(g, false) }},
	{Name: "nav_goto_time", Description: "navigation menu go-to wheel in Time mode (M:SS) with the resolved beat in the footer", SettleFrames: 120,
		Setup:       func(g *Game) { sceneGotoWheel(g, true) },
		MobileSetup: func(g *Game) { g.SetForceMobileProfile(true); sceneGotoWheel(g, true) }},
	// The pending go-to position far enough right (two 16-beat detents) that
	// its ghost on the timeline ribbon clears the card: line + 1-based chip.
	{Name: "nav_goto_preview_ghost", Description: "go-to wheel scrolled 32 beats ahead — the timeline ghost marks the pending position (1-based chip); playhead unmoved", SettleFrames: 120,
		Setup: func(g *Game) { sceneGotoWheelAt(g, false, 3, 2) }},
	// Shared value wheel (spec §3): the BPM readout and the node sidebar's value
	// pills open it instead of a type-in box.
	{Name: "bpm_wheel_open", Description: "BPM value wheel open under the BPM readout (±1/±5/±10/±50 ladder), stepped to 124", SettleFrames: 120,
		Setup: func(g *Game) {
			if b := g.drum.bpmBox(); b != nil {
				g.drum.openBPMWheel(b.Rect)
				for i := 0; i < 4; i++ {
					g.drum.valueWheelPopup.StepValue(+1)
				}
			}
		},
		MobileSetup: func(g *Game) {
			g.SetForceMobileProfile(true)
			if b := g.drum.bpmBox(); b != nil {
				g.drum.openBPMWheel(b.Rect)
			}
		}},
	{Name: "node_volume_wheel_open", Description: "node sidebar volume value wheel open over its pill (dB ladder, Muted at the bottom)", SettleFrames: 120,
		Setup: func(g *Game) {
			n := g.tryAddNode(3, 1, model.NodeTypeRegular)
			g.updateBeatInfos()
			if n != nil && g.sidebar != nil {
				g.sidebar.Open(n)
				g.sidebar.ExpandAllSections()
				g.sidebar.layout()
				g.sidebar.wireButtons()
				g.openNodeValueWheel(fieldVol)
				g.drum.valueWheelPopup.StepValue(-1)
				g.drum.valueWheelPopup.StepValue(-1)
			}
		}},
	// The navigation card crosses the pane divider on a short drum pane and is
	// repainted above it; the jump-size picker stacked on it must stay on top.
	// Capture at a small window (e.g. -win-w 640 -win-h 480).
	{Name: "nav_jump_picker_short_pane", Description: "nav card + stacked jump-size picker in a short drum pane (card repainted above the divider, picker still on top)", SettleFrames: 120,
		Setup: func(g *Game) { g.drum.OpenNavigation(); g.drum.OpenJumpMenu() }},
	{Name: "crop_nav_goto", Description: "navigation menu card cropped (go-to wheel)", SettleFrames: 120, Subject: SubjectNavMenu,
		Setup:       func(g *Game) { sceneGotoWheel(g, false) },
		MobileSetup: func(g *Game) { g.SetForceMobileProfile(true); sceneGotoWheel(g, false) }},
	// Spanish locale with a menu open. The shipped Inter faces are SUBSETS
	// (internal/assets/font_embed.go): a codepoint dropped from the subset
	// renders as a tofu box, and es-419 is the only shipped locale whose
	// strings leave ASCII -- accents, tilde, diaeresis and the inverted
	// marks ¿ ¡. fonts_coverage_test.go asserts the glyphs EXIST; this
	// scene is the proof they actually DRAW.
	{Name: "locale_es_nav_menu", Description: "navigation menu in Spanish — accented glyph coverage of the subset font", Mobile: true, SettleFrames: 120,
		Setup: func(g *Game) {
			i18n.SetLocale(i18n.LocaleES)
			g.drum.OpenNavigation()
		},
		MobileSetup: func(g *Game) {
			g.SetForceMobileProfile(true)
			i18n.SetLocale(i18n.LocaleES)
			g.drum.OpenNavigation()
		}},
	{Name: "template_menu_open", Description: "overflow menu template page (genre circuit templates + search)", Mobile: true, SettleFrames: 120,
		Setup: func(g *Game) { g.drum.overflowPage = 1; g.drum.OpenOverflowMenu() },
		MobileSetup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.overflowPage = 1
			g.drum.OpenOverflowMenu()
		}},
	// Keyboard cursor in the subdiv picker: the SubdivMenuComponent render path
	// is separate from the overflow menu's, so it needs its own visual proof.
	{Name: "subdiv_menu_keynav", Description: "subdivision menu with the keyboard cursor on a row", SettleFrames: 120,
		Setup: func(g *Game) {
			g.drum.OpenSubdivMenu()
			nav := g.drum.subdivMenuComp.MenuKeyNavForTest()
			nav.BindHost(g.drum.subdivMenuComp)
			nav.SetSelected(2)
		}},
	// Keyboard cursor on a template row: proves the arrow-key highlight actually
	// RENDERS (a nav that moves an invisible cursor passes every unit test).
	{Name: "template_menu_keynav", Description: "template menu with the keyboard cursor on a row (arrow-key highlight)", Mobile: true, SettleFrames: 120,
		Setup:       keyNavTemplateMenuSetup(false),
		MobileSetup: keyNavTemplateMenuSetup(true)},
	// Scrolled mid-list: exercises the row-culling edges in drawOverflowMenu
	// (rows above the viewport are skipped, the partially-visible top and
	// bottom rows must still render). Regression surface for the template-menu
	// frame-cost fix — see template_menu_frame_cost_test.go.
	{Name: "template_menu_scrolled", Description: "template menu scrolled mid-list (row culling top/bottom edges)", Mobile: true, SettleFrames: 120,
		Setup:       scrolledTemplateMenuSetup(false),
		MobileSetup: scrolledTemplateMenuSetup(true)},
	// Scrolled to the Lied and French sections, which hold the LONGEST template
	// titles in the library ("Clara Schumann — Ich stand in dunklen Träumen",
	// "Lili Boulanger — D'un matin de printemps"). Title width is the one thing
	// a menu row cannot absorb: the row also right-aligns a BPM readout, so an
	// over-long title either collides with it or gets clipped. Kept as the
	// regression surface for that collision.
	{Name: "template_menu_long_titles", Description: "template menu at the longest titles (title vs BPM readout collision)", Mobile: true, SettleFrames: 120,
		Setup:       longTitleTemplateMenuSetup(false),
		MobileSetup: longTitleTemplateMenuSetup(true)},
	// A groove template loaded into the grid: the visual proof that generated
	// templates with DRUM rows lay out correctly at one grid unit per step
	// (exported subdiv 4) — rows half a beat apart, nodes on sixteenth steps,
	// swung offbeats carrying their groove. Mobile shows the row rack.
	{Name: "template_groove_funk", Description: "groove template (James Brown — Funky Drummer) loaded: drum rows under an original tune", Mobile: true, SettleFrames: 120,
		Setup:       loadTemplateScene("funky-drummer", false),
		MobileSetup: loadTemplateScene("funky-drummer", true)},
	// The widest groove template (11 rows: a seven-voice 808 pattern under
	// lead, sequencer and pad) — proof the row rack holds a full drum machine.
	{Name: "template_groove_emc2", Description: "groove template (Giorgio Moroder — E=MC²) loaded: 808 kick, rim, snare, clap, hats, congas under an original tune", Mobile: true, SettleFrames: 120,
		Setup:       loadTemplateScene("e-mc2", false),
		MobileSetup: loadTemplateScene("e-mc2", true)},
	{Name: "template_groove_salsa", Description: "groove template (Salsa — son clave 2-3) loaded, rack scrolled to the drum rows: campana, congas, clave, cáscara", SettleFrames: 120,
		Setup: func(g *Game) {
			loadTemplateScene("salsa-clave", false)(g)
			// Scroll the rack to Bass and the kit rows below it, through the
			// same pair the setRowOffset JS export writes.
			g.drum.rowOffset = 2
			if g.drum.rowRackZone != nil {
				g.drum.rowRackZone.SetRowOffset(2)
			}
		}},
	{Name: "mobile_view_audio", Description: "mobile audio view (EQ/Wave) mode", Mobile: true,
		Setup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.SetMobileEQMode(true)
		}},
	{Name: "mobile_per_row_vol_popup", Description: "mobile per-row volume popup", Mobile: true, SettleFrames: 120,
		Setup: func(g *Game) {
			g.SetForceMobileProfile(true)
			ensureRow(g, 0)
			g.drum.OpenVolumePopup(0)
		}},
	{Name: "mobile_row_inline_controls", Description: "mobile rows with vol/mute/solo/FX inline; row 0 muted, row 2 soloed", Mobile: true,
		Setup: func(g *Game) {
			g.SetForceMobileProfile(true)
			ensureRow(g, 2)
			if 0 < len(g.drum.Rows) {
				g.drum.Rows[0].Muted = true
			}
			if 2 < len(g.drum.Rows) {
				g.drum.Rows[2].Solo = true
			}
			g.drum.markRowControlsDirty()
		}},
	{Name: "mobile_transport_bottom_bar",
		Description: "mobile bottom action bar with vol/segmented-view-switch/overflow + EQ peek sparkline + horizontal BPM stepper",
		Mobile:      true, SettleFrames: 60,
		Setup: func(g *Game) {
			// Default mobile boot already shows the new bottom bar + peek strip.
			// Boost EQ band 0 by +12dB so the peek sparkline shows visible
			// motion (otherwise it's a flat line).
			g.SetForceMobileProfile(true)
			g.SetEQBandGain(channel.Master, 0, 12.0)
		}},

	// ─── mobile UI consistency pass (Themes 1–5) ──────────────────
	// One scene per audio sub-view so visual regression catches the
	// bottom-nav-driven panel state for every tab. Bottom bar must
	// be visible and the segmented selection must match the tab.
	{Name: "mobile_bottom_nav_pads", Description: "mobile Pads (rows) with bottom-nav strip visible", Mobile: true, SettleFrames: 60,
		Setup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.setViewMode(viewModeRows)
		}},
	{Name: "mobile_bottom_nav_eq", Description: "mobile EQ tab via bottom-nav strip", Mobile: true, SettleFrames: 60,
		Setup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.setViewMode(viewModeEQ)
		}},
	{Name: "mobile_bottom_nav_wave", Description: "mobile Wave tab via bottom-nav strip", Mobile: true, SettleFrames: 60,
		Setup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.setViewMode(viewModeWave)
		}},
	{Name: "mobile_bottom_nav_spectrum", Description: "mobile Spectrum tab via bottom-nav strip", Mobile: true, SettleFrames: 60,
		Setup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.setViewMode(viewModeSpectrum)
		}},
	// Spectrum banner finding states. The collision scene is a REAL one: a DnB
	// kick (49 Hz) and a sub bass (46 Hz) both sound their fundamental in ISO
	// band 1 (44-88 Hz), which is the classic low-end pile-up a beginner cannot
	// see in a bar graph but can read in a sentence. Driving actual instruments
	// rather than a synthesized analyzer state keeps the scene honest: it
	// exercises the same bridge (pitch info -> band) production uses.
	{Name: "spectrum_finding_collision", Description: "Spectrum banner: two instruments fighting in one band", SettleFrames: 150,
		Setup:       spectrumCollisionSetup,
		Mobile:      true,
		MobileSetup: spectrumCollisionSetupMobile},
	{Name: "mobile_spectrum_collision", Description: "mobile Spectrum banner: two instruments fighting", Mobile: true, SettleFrames: 150,
		Setup: spectrumCollisionSetupMobile},
	{Name: "spectrum_finding_ok", Description: "Spectrum banner: nothing fighting (the calm line)", SettleFrames: 150,
		Setup: func(g *Game) {
			_ = g.SetActiveEQTab("spectrum")
			g.SetPlaying(true)
		}},
	{Name: "mobile_spectrum_playing", Description: "mobile Spectrum tab WITH live bars (chrome-cache regression surface)", Mobile: true, SettleFrames: 150,
		Setup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.setViewMode(viewModeSpectrum)
			g.SetPlaying(true)
		}},
	{Name: "mobile_bottom_nav_levels", Description: "mobile Meters tab via bottom-nav strip", Mobile: true, SettleFrames: 60,
		Setup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.setViewMode(viewModeMeters)
		}},
	{Name: "mobile_bottom_nav_chain", Description: "mobile Chain tab via bottom-nav strip", Mobile: true, SettleFrames: 60,
		Setup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.setViewMode(viewModeChain)
		}},
	{Name: "mobile_bottom_nav_synth", Description: "mobile Synth tab via bottom-nav strip", Mobile: true, SettleFrames: 60,
		Setup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.setViewMode(viewModeSynth)
			settleSynthTab(g)
		}},
	{Name: "mobile_bottom_nav_sampler", Description: "mobile Sampler tab via bottom-nav strip", Mobile: true, SettleFrames: 60,
		Setup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.setViewMode(viewModeSampler)
		}},
	// Regression scene for the Pads-after-EQ leak (screenshot.png 2026-05-10):
	// after a visit to the EQ tab, the EQ panel's Master/HP/LP pill strip
	// must NOT be visible above the bottom nav in Pads view. Locks in the
	// tab-system-owns-visibility invariant.
	{Name: "mobile_pads_after_eq", Description: "mobile Pads tab after a visit to EQ — EQ pills must not leak", Mobile: true, SettleFrames: 60,
		Setup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.setViewMode(viewModeEQ)
			g.drum.setViewMode(viewModeRows)
		}},
	// Track / follow chip on the timeline ruler header (Theme 2).
	{Name: "mobile_track_chip_following", Description: "mobile Track chip in following state (primary border)", Mobile: true, SettleFrames: 60,
		Setup: func(g *Game) {
			g.SetForceMobileProfile(true)
			if g.drum != nil && g.drum.transportZone != nil {
				g.drum.transportZone.SetFollow(true)
			}
		}},
	{Name: "mobile_track_chip_free", Description: "mobile Track chip in free-scroll state (neutral icon)", Mobile: true, SettleFrames: 60,
		Setup: func(g *Game) {
			g.SetForceMobileProfile(true)
			if g.drum != nil && g.drum.transportZone != nil {
				g.drum.transportZone.SetFollow(false)
			}
		}},
	// Timeline-length chips — extreme states show the visible-beat range.
	// The +/− chips at the bottom of the row rack grow / shrink the
	// active drum view's beat span (mirrors the desktop length +/− pair).
	{Name: "mobile_length_max", Description: "mobile timeline expanded to its maximum visible beat span via repeated + chip taps", Mobile: true, SettleFrames: 60,
		Setup: func(g *Game) {
			g.SetForceMobileProfile(true)
			if g.drum == nil {
				return
			}
			// Drive the chip handler enough times to saturate against the
			// `clampLength` cell-width-derived max.
			for i := 0; i < 64; i++ {
				g.drum.rowZoomIncBtn.OnClick()
				_ = g.Update()
			}
		}},
	{Name: "mobile_length_min", Description: "mobile timeline shrunk to its minimum visible beat span via repeated − chip taps", Mobile: true, SettleFrames: 60,
		Setup: func(g *Game) {
			g.SetForceMobileProfile(true)
			if g.drum == nil {
				return
			}
			for i := 0; i < 64; i++ {
				g.drum.rowZoomDecBtn.OnClick()
				_ = g.Update()
			}
		}},
	// Overflow menu (Theme 5) — every entry now carries an icon.
	{Name: "mobile_overflow_menu_redesigned", Description: "mobile overflow menu with iconography on every entry", Mobile: true, SettleFrames: 120,
		Setup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.OpenOverflowMenu()
		}},

	// ─── playback overlays ────────────────────────────────────────
	// Capture menus/popups while playback is running so visual diffs
	// surface any beat-highlight or transport-state regressions that
	// hide behind overlay chrome.
	{Name: "playback_context_menu_open", Description: "playback running + context menu open", SettleFrames: 120,
		Setup: func(g *Game) {
			ensureRow(g, 0)
			g.SetPlaying(true)
			g.drum.OpenContextMenu(0)
		}},
	{Name: "playback_instrument_menu_open", Description: "playback running + instrument menu open", SettleFrames: 120,
		Setup: func(g *Game) {
			ensureRow(g, 0)
			g.SetPlaying(true)
			g.drum.OpenInstrumentMenu(0)
		}},
	{Name: "playback_color_wheel_open", Description: "playback running + color wheel open", SettleFrames: 120,
		Setup: func(g *Game) {
			ensureRow(g, 0)
			g.SetPlaying(true)
			g.drum.OpenColorMenu(0)
		}},
	{Name: "playback_subdiv_menu_open", Description: "playback running + subdivision menu open", SettleFrames: 120,
		Setup: func(g *Game) {
			g.SetPlaying(true)
			g.drum.OpenSubdivMenu()
		}},
	{Name: "playback_fx_panel_with_3_effects", Description: "playback running + FX panel with 3 effects", SettleFrames: 150,
		Setup: func(g *Game) {
			ensureRow(g, 0)
			instID := g.drum.Rows[0].Instrument
			audio.AddInsertEffect(instID, audio.EffectDelay, nil)
			audio.AddInsertEffect(instID, audio.EffectReverb, nil)
			audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
			g.SetPlaying(true)
			g.drum.OpenFXPanel(0)
		}},
	{Name: "playback_eq_tab_levels", Description: "playback running + Meters tab", SettleFrames: 120,
		Setup: func(g *Game) {
			g.SetPlaying(true)
			_ = g.SetActiveEQTab("levels")
		}},
	{Name: "playback_eq_tab_chain", Description: "playback running + Chain tab", SettleFrames: 150,
		Setup: func(g *Game) {
			g.SetPlaying(true)
			_ = g.SetActiveEQTab("chain")
			g.SetChainVisible(true)
		}},
	// Muting a row is the discontinuity the Spectrum readout has to survive:
	// the sentence, the emphasised lane, the held bars and the dB anchor all
	// have to come from the post-mute mix. When the mute leaves exactly one
	// audible row the panel also auto-focuses that instrument's channel,
	// which is the programmatic switch that used to skip the re-baseline.
	{Name: "spectrum_after_mute", Description: "Spectrum after muting a row mid-playback (readout re-based on the new mix)",
		Mobile: true, SettleFrames: 150,
		Setup: sceneSpectrumAfterMute,
		MobileSetup: func(g *Game) {
			// Setup alone leaves the mobile audio panel invisible and the
			// crop falls back to the pads view (see CLAUDE.md).
			g.SetForceMobileProfile(true)
			g.drum.SetMobileEQMode(true)
			sceneSpectrumAfterMute(g)
		}},
	// A SINGLE INSTRUMENT's channel is the hard case for the Spectrum tab:
	// it is silent between transients, so a live-tracked axis has nothing to
	// measure at the moment the channel is picked and the plot came up
	// empty. 260 settle frames so the capture lands after the channel has
	// settled on its measured level (spectrumChannelSettle).
	{Name: "spectrum_instrument_channel", Description: "Spectrum on one instrument's channel (kick) — axis pinned to that channel",
		Mobile: true, SettleFrames: 260,
		Setup: sceneSpectrumInstrumentChannel,
		MobileSetup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.SetMobileEQMode(true)
			sceneSpectrumInstrumentChannel(g)
		}},
	// THE tiny-bar trigger, cropped so the defect is one glance: mute a row,
	// then select that row's own channel. The settle window then elapses on
	// silence, and committing there froze the pre-rendered ESTIMATE (the raw
	// voice at unity, far above what the channel measures) — every bar after
	// that was a stub against a ceiling the channel never reaches, for the
	// rest of the session. Unmutes partway so the capture shows the axis it
	// finally commits to. If this ever regresses the bars go flat again and
	// the crop shows it without reading a single number.
	{Name: "crop_spectrum_muted_channel_pick", Description: "Spectrum: channel picked while silent, signal arrives after the settle",
		Mobile: true, SettleFrames: 290, Subject: SubjectEQTabSpectrum,
		Setup: sceneSpectrumMutedChannelPick,
		MobileSetup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.SetMobileEQMode(true)
			sceneSpectrumMutedChannelPick(g)
		}},
	// The same channel, CROPPED — and unlike crop_spectrum_muted_channel_pick
	// it feeds no synthetic spectrum, so it is the only subject-level scene
	// that exercises the real analyser bridge end to end. That is exactly
	// where the browser-only blank plot lived: the bridge delivered 64
	// linear bins (375 Hz apart at 48 kHz) and the four lowest ISO bands —
	// all a kick has — held no bin at all, so they reported the reference
	// floor. Desktop ran its own 43 Hz-per-bin FFT and looked correct, so
	// only the BROWSER pass of this scene can show a regression. See
	// analyzer.SpectrumFFTSize.
	//
	// READ THE BROWSER CAPTURE FOR *WHICH BANDS*, NOT FOR BAR HEIGHT. The
	// desktop pass is frame-driven and lands on the same beat every time;
	// the browser pass runs on wall-clock audio, so it lands wherever the
	// kick happens to be and a capture in the gap between hits shows
	// peak-hold ticks over near-floor bars. That is a correct reading of a
	// quiet moment. The tell is WHERE the marks are: bass (31/62/125/250)
	// means the low bands are receiving bins, and under the defect there
	// was nothing below 500 Hz at all, in any frame. The gated assertion
	// lives in xplat_spectrum_channel_data.browser.test.js, which samples
	// ten frames and keeps the loudest.
	{Name: "crop_spectrum_instrument_channel", Description: "Spectrum on one instrument's channel, live analyser (no injected data)",
		Mobile: true, SettleFrames: 260, Subject: SubjectEQTabSpectrum,
		Setup: sceneSpectrumInstrumentChannel,
		MobileSetup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.SetMobileEQMode(true)
			sceneSpectrumInstrumentChannel(g)
		}},
	{Name: "playback_eq_tab_spectrum", Description: "playback running + Spectrum tab", SettleFrames: 150,
		Setup: func(g *Game) {
			g.SetPlaying(true)
			_ = g.SetActiveEQTab("spectrum")
		}},
	{Name: "playback_eq_tab_synth", Description: "playback running + Synth tab", SettleFrames: 150,
		Setup: func(g *Game) {
			g.SetPlaying(true)
			_ = g.SetActiveEQTab("synth")
		}},

	// ─── FX panel detail (per-effect, knob drawer) ────────────────
	{Name: "fx_panel_distortion_only", Description: "FX panel with distortion only", SettleFrames: 120,
		Setup: func(g *Game) {
			ensureRow(g, 0)
			instID := g.drum.Rows[0].Instrument
			audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
			g.drum.OpenFXPanel(0)
		}},
	{Name: "fx_panel_reverb_only", Description: "FX panel with reverb only", SettleFrames: 120,
		Setup: func(g *Game) {
			ensureRow(g, 0)
			instID := g.drum.Rows[0].Instrument
			audio.AddInsertEffect(instID, audio.EffectReverb, nil)
			g.drum.OpenFXPanel(0)
		}},
	{Name: "fx_panel_delay_only", Description: "FX panel with delay only", SettleFrames: 120,
		Setup: func(g *Game) {
			ensureRow(g, 0)
			instID := g.drum.Rows[0].Instrument
			audio.AddInsertEffect(instID, audio.EffectDelay, nil)
			g.drum.OpenFXPanel(0)
		}},
	{Name: "fx_panel_knob_drawer_open", Description: "FX panel with one effect expanded showing parameter knobs", Mobile: true, SettleFrames: 150,
		Setup: func(g *Game) {
			g.SetForceMobileProfile(true)
			ensureRow(g, 0)
			instID := g.drum.Rows[0].Instrument
			audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
			g.drum.OpenFXPanel(0)
			if g.drum.fxExpandedSlots == nil {
				g.drum.fxExpandedSlots = map[int]bool{}
			}
			g.drum.fxExpandedSlots[0] = true
			g.drum.buildFXPanel()
		}},

	// ─── EQ detail (filters, band mute, scope) ────────────────────
	{Name: "eq_hpf_active", Description: "high-pass filter engaged", Mobile: true,
		Setup: func(g *Game) {
			_ = g.SetActiveEQTab("eq")
			g.drum.toggleHPF()
		},
		MobileSetup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.SetMobileEQMode(true)
			_ = g.SetActiveEQTab("eq")
			g.drum.toggleHPF()
		}},
	{Name: "eq_lpf_active", Description: "low-pass filter engaged", Mobile: true,
		Setup: func(g *Game) {
			_ = g.SetActiveEQTab("eq")
			g.drum.toggleLPF()
		},
		MobileSetup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.SetMobileEQMode(true)
			_ = g.SetActiveEQTab("eq")
			g.drum.toggleLPF()
		}},
	{Name: "eq_instrument_curve", Description: "a row instrument's own EQ (two bands + high-pass): the audio bundle's EQ part, read from the instrument", Mobile: true,
		Setup: func(g *Game) { sceneInstrumentEQ(g) },
		MobileSetup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.SetMobileEQMode(true)
			sceneInstrumentEQ(g)
		}},
	{Name: "pick_settings_question", Description: "choosing an instrument whose own saved settings differ from the row's: the must-answer question (keep the row's / use the instrument's) over the row; the row keeps playing its instrument until answered", Mobile: true,
		Setup: func(g *Game) { scenePickQuestion(g, false) }, MobileSetup: func(g *Game) {
			g.SetForceMobileProfile(true)
			scenePickQuestion(g, false)
		}},
	{Name: "kit_swap_settings_question", Description: "applying a kit whose kick carries its own saved settings to a row with settings: ONE question for the whole swap (keep each row's / use the kit's)", Mobile: true,
		Setup: func(g *Game) { scenePickQuestion(g, true) }, MobileSetup: func(g *Game) {
			g.SetForceMobileProfile(true)
			scenePickQuestion(g, true)
		}},
	{Name: "eq_band_muted", Description: "single EQ band muted", Mobile: true,
		Setup: func(g *Game) {
			_ = g.SetActiveEQTab("eq")
			muted := g.drum.eqBandMuted()
			if len(muted) > 4 {
				muted[4] = true
			}
		},
		MobileSetup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.SetMobileEQMode(true)
			_ = g.SetActiveEQTab("eq")
			muted := g.drum.eqBandMuted()
			if len(muted) > 4 {
				muted[4] = true
			}
		}},
	{Name: "eq_chain_custom_settings", Description: "Chain tab with custom taps + traces running (EQ bands also tweaked)", SettleFrames: 150,
		Setup: func(g *Game) {
			_ = g.SetActiveEQTab("chain")
			g.SetChainVisible(true)
			// EQ-band tweaks alone are invisible on the Chain tab (it renders
			// scope traces, not the EQ plot), so the variant must also change
			// the Chain surface itself: assign distinct A/B taps and run
			// playback so the trace area fills. Without this the scene
			// rendered identically to the bare eq_tab_chain base.
			if z := chainZoneOf(g); z != nil {
				z.SetTapA(scope.StageSynth)
				z.SetTapB(scope.StageMaster)
			}
			g.SetEQBandGain(channel.Master, 1, -8)
			g.SetEQBandGain(channel.Master, 7, 6)
			g.SetPlaying(true)
		}},

	// ─── graph & sidebar variants ─────────────────────────────────
	{Name: "graph_complex_3_nodes_4_edges", Description: "complex graph with 3 nodes, multiple edges",
		Setup: func(g *Game) {
			a := g.tryAddNode(2, 0, model.NodeTypeRegular)
			b := g.tryAddNode(2, 2, model.NodeTypeRegular)
			c := g.tryAddNode(4, 2, model.NodeTypeRegular)
			if a != nil && b != nil {
				g.addEdge(a, b)
			}
			if b != nil && c != nil {
				g.addEdge(b, c)
			}
			if a != nil && c != nil {
				g.addEdge(a, c)
			}
			g.updateBeatInfos()
		}},
	{Name: "node_sidebar_logic_expanded", Description: "node sidebar with Logic section expanded",
		Setup: func(g *Game) {
			n := g.tryAddNode(3, 1, model.NodeTypeRegular)
			g.updateBeatInfos()
			if n != nil && g.sidebar != nil {
				g.sidebar.Open(n)
				g.sidebar.sectionOpen[secLogic] = true
			}
		}},
	{Name: "node_sidebar_all_expanded", Description: "node sidebar with every section expanded (all main buttons visible)",
		Setup: func(g *Game) {
			n := g.tryAddNode(3, 1, model.NodeTypeRegular)
			g.updateBeatInfos()
			if n != nil && g.sidebar != nil {
				g.sidebar.Open(n)
				g.sidebar.ExpandAllSections()
			}
		}},
	{Name: "node_sidebar_groove_expanded", Description: "node sidebar with Groove section expanded",
		Setup: func(g *Game) {
			n := g.tryAddNode(3, 1, model.NodeTypeRegular)
			g.updateBeatInfos()
			if n != nil && g.sidebar != nil {
				g.sidebar.Open(n)
				g.sidebar.sectionOpen[secGroove] = true
			}
		}},
	{Name: "node_sidebar_logic_dropdown", Description: "node sidebar: the Logic rule list open on a Probability node — the selected rule row is the typed sbLogicOption of the node's rule",
		Setup: func(g *Game) {
			n := g.tryAddNode(3, 1, model.NodeTypeRegular)
			g.updateBeatInfos()
			if n != nil && g.sidebar != nil {
				if mn, ok := g.graph.GetNodeByID(n.ID); ok {
					p := mn.Params
					p.LogicKind, p.LogicP = model.LogicProbability, 0.5
					g.graph.SetNodeParams(n.ID, p)
				}
				g.sidebar.Open(n)
				g.sidebar.sectionOpen[secLogic] = true
				g.sidebar.logicDropdownOpen = true
			}
		}},
	{Name: "node_sidebar_logic_groove_badges", Description: "node sidebar, sections collapsed, on a node with Probability 50% and a 25% Delay groove — the collapsed badges name both through the typed enums (localized groove label)",
		Setup: func(g *Game) {
			n := g.tryAddNode(3, 1, model.NodeTypeRegular)
			g.updateBeatInfos()
			if n != nil && g.sidebar != nil {
				if mn, ok := g.graph.GetNodeByID(n.ID); ok {
					p := mn.Params
					p.LogicKind, p.LogicP = model.LogicProbability, 0.5
					p.GrooveKind, p.GroovePct = model.GrooveDelay, 0.25
					g.graph.SetNodeParams(n.ID, p)
				}
				g.sidebar.Open(n)
			}
		}},
	{Name: "node_sidebar_audio_expanded", Description: "node sidebar with Audio (per-node EQ) section expanded",
		Setup: func(g *Game) {
			n := g.tryAddNode(3, 1, model.NodeTypeRegular)
			g.updateBeatInfos()
			if n != nil && g.sidebar != nil {
				g.sidebar.Open(n)
				g.sidebar.sectionOpen[secAud] = true
			}
		}},
	{Name: "node_sidebar_node_muted", Description: "node sidebar inspecting a Mute-type node",
		Setup: func(g *Game) {
			n := g.tryAddNode(3, 1, model.NodeTypeMute)
			g.updateBeatInfos()
			if n != nil && g.sidebar != nil {
				g.sidebar.Open(n)
			}
		}},
	{Name: "node_sidebar_node_silent", Description: "node sidebar inspecting a Silent-type node",
		Setup: func(g *Game) {
			n := g.tryAddNode(3, 1, model.NodeTypeSilent)
			g.updateBeatInfos()
			if n != nil && g.sidebar != nil {
				g.sidebar.Open(n)
			}
		}},
	{Name: "node_sidebar_node_invisible", Description: "node sidebar inspecting an Invisible-type node",
		Setup: func(g *Game) {
			n := g.tryAddNode(3, 1, model.NodeTypeInvisible)
			g.updateBeatInfos()
			if n != nil && g.sidebar != nil {
				g.sidebar.Open(n)
			}
		}},
	{Name: "node_sidebar_groups", Description: "node sidebar Groups section: membership chips+remove buttons and an Add-to-group row", Mobile: true,
		Setup: sceneNodeSidebarGroupsSetup},

	// ─── desktop parity & recording overlays ──────────────────────
	{Name: "desktop_per_row_vol_popup", Description: "desktop per-row volume popup", SettleFrames: 120,
		Setup: func(g *Game) {
			ensureRow(g, 0)
			g.drum.OpenVolumePopup(0)
		}},
	{Name: "recording_with_context_menu", Description: "recording armed + context menu open", Mobile: true, SettleFrames: 150,
		Setup: func(g *Game) {
			ensureRow(g, 0)
			_ = g.StartRecording()
			g.drum.OpenContextMenu(0)
		},
		MobileSetup: func(g *Game) {
			g.SetForceMobileProfile(true)
			ensureRow(g, 0)
			_ = g.StartRecording()
			g.drum.OpenContextMenu(0)
		}},
	{Name: "recording_during_playback_overlay", Description: "recording armed during playback", Mobile: true, SettleFrames: 150,
		Setup: func(g *Game) {
			_ = g.StartRecording()
			g.SetPlaying(true)
		},
		MobileSetup: func(g *Game) {
			g.SetForceMobileProfile(true)
			_ = g.StartRecording()
			g.SetPlaying(true)
		}},

	// ─── armed grid modes (helper banner + blink/ghost + dialog) ──
	// SettleFrames 60 keeps the move-mode blink in an ON half-period
	// (phase 60 → index 4, even) so the subject highlight is captured.
	{Name: "mode_move_banner", Description: "move mode armed — helper banner, blinking subject, chain A->N->B", Mobile: true, SettleFrames: 60,
		Setup: func(g *Game) {
			a := g.tryAddNode(2, 2, model.NodeTypeRegular)
			n := g.tryAddNode(2, 4, model.NodeTypeRegular)
			b := g.tryAddNode(2, 6, model.NodeTypeRegular)
			if a != nil && n != nil && b != nil {
				g.addEdge(a, n)
				g.addEdge(n, b)
				g.modes.Enter(GridModeMove, n)
			}
		}},
	{Name: "mode_copy_banner", Description: "copy mode armed — pasting helper banner", Mobile: true, SettleFrames: 60,
		Setup: func(g *Game) {
			n := g.tryAddNode(2, 2, model.NodeTypeRegular)
			if n != nil && g.snapshotNodeToClipboard(n) {
				g.modes.Enter(GridModeCopy, n)
			}
		}},
	{Name: "mode_break_confirm", Description: "break-circuit confirmation dialog over a square circuit corner", Mobile: true, SettleFrames: 60,
		Setup: func(g *Game) {
			a := g.tryAddNode(2, 2, model.NodeTypeRegular)
			n := g.tryAddNode(2, 6, model.NodeTypeRegular)
			b := g.tryAddNode(6, 6, model.NodeTypeRegular)
			c := g.tryAddNode(6, 2, model.NodeTypeRegular)
			x := g.tryAddNode(10, 2, model.NodeTypeRegular)
			y := g.tryAddNode(10, 8, model.NodeTypeRegular)
			if a == nil || n == nil || b == nil || c == nil || x == nil || y == nil {
				return
			}
			g.addEdge(a, n)
			g.addEdge(n, b)
			g.addEdge(b, c)
			g.addEdge(c, a)
			g.addEdge(x, y)
			g.modes.Enter(GridModeMove, n)
			g.beginBreakConfirm(n, 10, 5)
		}},

	// ─── subject-cropped variants (Phase 5) ──────────────────────
	// Each crop_* scene declares a Subject so the screenshot harness
	// emits a PNG cropped to that surface's bounds. Setup is reused
	// from the matching full-screen scene wherever possible — the
	// only difference is the Subject field.
	{Name: "crop_theme_editor", Description: "theme editor panel, subject-cropped", Subject: SubjectThemeEditor,
		Setup: func(g *Game) { g.toggleThemeOverlay() }},
	{Name: "crop_main_grid_default", Description: "main grid pane, default circuit",
		Subject: SubjectMainGrid,
		Setup:   func(g *Game) {}},
	{Name: "crop_main_grid_with_3_nodes", Description: "main grid pane with a 3-node graph",
		Subject: SubjectMainGrid,
		Setup: func(g *Game) {
			a := g.tryAddNode(2, 0, model.NodeTypeRegular)
			b := g.tryAddNode(2, 2, model.NodeTypeRegular)
			c := g.tryAddNode(4, 2, model.NodeTypeRegular)
			if a != nil && b != nil {
				g.addEdge(a, b)
			}
			if b != nil && c != nil {
				g.addEdge(b, c)
			}
			if a != nil && c != nil {
				g.addEdge(a, c)
			}
			g.updateBeatInfos()
		}},
	{Name: "crop_group_menu", Description: "group menu panel cropped — header (name + Save + close), batch edits, rule editor",
		Subject: SubjectGroupMenu,
		Setup:   sceneGroupMenuSetup},
	{Name: "crop_drum_view_default", Description: "drum pane (header + rows + EQ panel), default rows",
		Subject: SubjectDrumView,
		Setup:   func(g *Game) {}},
	{Name: "crop_drum_view_multi_row", Description: "drum pane (header + rows + EQ panel), five rows",
		Subject: SubjectDrumView,
		Setup:   func(g *Game) { ensureMultiRow(g, 5) }},
	{Name: "crop_row_rack_states", Description: "drum pane, seven rows in every rack state (muted, soloed, FX on, low volume) with spare rack height below the rows — the add-row ghost row must sit directly under the last row, every control in a row must share one height, and no background-colored lane may separate the rack from the grid",
		Subject: SubjectDrumView, Mobile: true,
		Setup: sceneRowRackStates,
		MobileSetup: func(g *Game) {
			g.SetForceMobileProfile(true)
			sceneRowRackStates(g)
		}},
	{Name: "crop_drum_rows_default", Description: "drum row scroll area only (no header, no EQ)",
		Subject: SubjectDrumRows,
		Setup:   func(g *Game) {}},
	{Name: "crop_drum_rows_multi_row", Description: "drum row scroll area only, five rows",
		Subject: SubjectDrumRows,
		Setup:   func(g *Game) { ensureMultiRow(g, 5) }},
	{Name: "crop_eq_tab_eq", Description: "EQ panel cropped — EQ tab",
		Subject: SubjectEQTabEQ,
		Setup:   func(g *Game) { _ = g.SetActiveEQTab("eq") }},
	// The EQ tab's live backdrop bars had NO visual regression surface: the
	// bars read an audio.AnalyzerSnapshot that is empty in the screenshot
	// binary, so every EQ capture showed the curve over an empty plot. This
	// injects a deterministic spectrum so the backdrop is actually visible —
	// it is the surface for the shared dB window the bars now map through.
	{Name: "crop_eq_tab_eq_live", Description: "EQ panel with a live spectrum backdrop behind the curve",
		Mobile: true, SettleFrames: 60, Subject: SubjectEQTabEQ,
		Setup:       sceneEQLiveBackdrop,
		MobileSetup: func(g *Game) { g.SetForceMobileProfile(true); g.drum.SetMobileEQMode(true); sceneEQLiveBackdrop(g) }},
	{Name: "crop_eq_tab_wave", Description: "EQ panel cropped — Wave tab (playing for visible waveform)", SettleFrames: 150,
		Subject:     SubjectEQTabWave,
		Setup:       func(g *Game) { _ = g.SetActiveEQTab("wave"); g.SetPlaying(true) },
		MobileSetup: mobileAudioPanelPlaySetup("wave")},
	{Name: "crop_eq_tab_spectrum", Description: "EQ panel cropped — Spectrum tab (playing for visible bars)", SettleFrames: 150,
		Subject:     SubjectEQTabSpectrum,
		Setup:       func(g *Game) { _ = g.SetActiveEQTab("spectrum"); g.SetPlaying(true) },
		MobileSetup: mobileAudioPanelPlaySetup("spectrum")},
	{Name: "crop_eq_tab_levels", Description: "EQ panel cropped — Levels tab (playing for visible levels)", SettleFrames: 150,
		Subject:     SubjectEQTabLevels,
		Setup:       func(g *Game) { _ = g.SetActiveEQTab("levels"); g.SetPlaying(true) },
		MobileSetup: mobileAudioPanelPlaySetup("levels")},
	// Synth-tab redesign scenes — each opens the per-instrument pipeline
	// panel on a different recipe so the unified section layout (VOICE/OSC/
	// FM/ENVELOPE/FILTER/POST, empty sections pruned) renders realistic
	// mixes. Hihat exercises a pruned-section case; FM exercises the
	// FM-voice (FM stage collision-pruned) row.
	{Name: "crop_synth_tab_drum", Description: "Synth tab cropped — drum-snare recipe (VOICE+stage sections, unified layout)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabSceneSetup(""),
		MobileSetup: mobileSynthTabSetup("")},
	{Name: "synth_action_menu_restore_shipped", Description: "Synth ⋯ menu after a Save over the shipped kick-snap: the destructive 'Restore shipped' item appears below Reset (desktop opens the same menu at the header Reset button)",
		SettleFrames: 20,
		Setup:        restoreShippedMenuSceneSetup(false),
		MobileSetup:  restoreShippedMenuSceneSetup(true)},
	{Name: "synth_save_as_prompt", Description: "Synth tab Save As: the name prompt open over the violin, prefilled with the instrument's current name and focused, its confirm reading \"Save over Violin\" (desktop via the header Save As button, mobile via the ⋯ menu's Save As)",
		SettleFrames: 20,
		Setup:        saveAsPromptSceneSetup(false, ""),
		MobileSetup:  saveAsPromptSceneSetup(true, "")},
	{Name: "synth_save_as_new", Description: "Synth tab Save As with a new name typed: the confirm reads \"Save as new\"",
		SettleFrames: 20,
		Setup:        saveAsPromptSceneSetup(false, "Violin Dark"),
		MobileSetup:  saveAsPromptSceneSetup(true, "Violin Dark")},
	{Name: "synth_save_as_taken", Description: "Synth tab Save As with another instrument's name typed (\"Bombo\", the Kick in Spanish): refused in place, no confirm button",
		SettleFrames: 20,
		Setup:        saveAsPromptSceneSetup(false, "Bombo"),
		MobileSetup:  saveAsPromptSceneSetup(true, "Bombo")},
	{Name: "synth_saved_tone_reopened", Description: "Synth tab after Save As over the instrument, then reopen: the startup circuit's first instrument had a category dial turned to 90 and was saved over (memory-only library); reopening the circuit plays the SAVED version — the dial still reads 90 and Save As is not lit",
		SettleFrames: 20,
		Setup:        savedToneReopenedSceneSetup(false),
		MobileSetup:  savedToneReopenedSceneSetup(true)},
	{Name: "crop_synth_tab_snare_config", Description: "Synth tab cropped — the CONFIG-first snare: its source==7 voice knobs surfaced as live VOICE-stage controls (Variant, Ring Pitch, Wire Decay, …)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabSceneSetup(roster.Snare),
		MobileSetup: mobileSynthTabSetup(roster.Snare)},
	{Name: "crop_synth_tab_fm", Description: "Synth tab cropped — fm-bell recipe (brightness wired, no tone/body)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabSceneSetup(roster.FmBell),
		MobileSetup: mobileSynthTabSetup(roster.FmBell)},
	{Name: "crop_synth_tab_kick_voice", Description: "Synth tab cropped — dnb-kick VOICE stage: the kick folded into the agnostic VOICE stage (no instrument-specific KICK stage), Essential kick knobs shown + Advanced expander (P4 tiering)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabStageSceneSetup(roster.DnbKick, synthSectionVoice),
		MobileSetup: mobileSynthTabSetup(roster.DnbKick)},
	{Name: "crop_synth_advanced_divider", Description: "Synth tab cropped — dnb-kick VOICE stage showing the always-visible Advanced divider row (essentials above, fine-tuning knobs below, P4 tiering)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabStageDividerSceneSetup(roster.DnbKick, synthSectionVoice),
		MobileSetup: mobileSynthTabSetup(roster.DnbKick)},
	{Name: "crop_synth_tab_violin_osc", Description: "Synth tab cropped — violin OSC stage with Advanced open: the bowed-string physical-model knobs (Bow Position/Pressure/Speed/Brightness) appear contextually because the Bowed String oscillator is selected (P4 contextual exposure)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabStageExpandedSceneSetup(roster.Violin, synthSectionOsc),
		MobileSetup: mobileSynthTabSetup(roster.Violin)},
	{Name: "crop_synth_tab_hihat_collapsed", Description: "Synth tab cropped — drum-hihat (pruned-empty sections, brightness wired)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabSceneSetup(roster.Hihat),
		MobileSetup: mobileSynthTabSetup(roster.Hihat)},
	{Name: "crop_synth_tab_master_chooser", Description: "Synth tab cropped — channel chooser dropdown open over the Synth tab",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabMasterChooserSetup(),
		MobileSetup: mobileSynthTabMasterChooserSetup()},
	{Name: "crop_synth_tab_modular", Description: "Synth tab cropped — modular voice (OSC+FM+ENVELOPE+FILTER+POST sections, enum knobs)",
		Mobile:      true, // also captured in the mobile pass: shows the inline value pills
		Subject:     SubjectSynthPanel,
		Setup:       synthTabSceneSetup(roster.Modular),
		MobileSetup: mobileSynthTabSetup(roster.Modular)},
	{Name: "crop_synth_control_viz", Description: "Synth tab (developer) — FILTER stage drilled: its knobs, and the selected knob's graph view",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabStageSetup(roster.Modular, "FILTER", ""),
		MobileSetup: mobileSynthTabStageSetup(roster.Modular, "FILTER", "")},
	{Name: "crop_synth_tab_drum_generator", Description: "Synth tab — bespoke drum (snare) showing the Generator selector (Native) alongside its wired knobs",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabSceneSetup(roster.Snare),
		MobileSetup: mobileSynthTabSetup(roster.Snare)},
	{Name: "crop_synth_tab_drum_revoiced", Description: "Synth tab — a drum re-voiced onto a Saw waveform: full modular pipeline + per-stage enable pills",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabDrumRevoicedSetup(),
		MobileSetup: mobileSynthTabDrumRevoicedSetup()},
	{Name: "crop_synth_tab_modular_play", Description: "Synth tab — modular voice during playback (per-stage enable pills + generator step, trigger pulse)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabModularPlaySetup(),
		MobileSetup: mobileSynthTabModularPlaySetup()},
	{Name: "crop_synth_tab_modular_noise", Description: "Synth tab — modular voice with Noise Pink generator + Filter stage bypassed (dimmed) during playback",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabModularNoiseSetup(),
		MobileSetup: mobileSynthTabModularNoiseSetup()},
	{Name: "crop_synth_tab_wav", Description: "Synth tab cropped — WAV-sample instrument (no recipe): single 'does not use the synth' banner",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabNoSynthSceneSetup(),
		MobileSetup: mobileSynthTabNoSynthSetup()},
	{Name: "crop_synth_tab_chips_collapsed", Description: "Synth tab pipeline chip strip hero: full modular pipeline (VOICE/OSC/FM/PITCH/LFO/BURST/ENVELOPE/FILTER/POST) with the leftmost OSC stage expanded into the detail pane",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabStageSetup(roster.Modular, "OSC", ""),
		MobileSetup: mobileSynthTabStageSetup(roster.Modular, "OSC", "")},
	{Name: "crop_synth_control_armed", Description: "Synth tab — modular voice with a control ARMED for inline barrel editing: the card grows in-place and both the resolution lane (left) and value lane (right) render",
		Mobile:      true,
		Subject:     SubjectSynthPanel,
		Setup:       synthControlArmedSceneSetup(),
		MobileSetup: mobileSynthControlArmedSceneSetup()},
	{Name: "crop_synth_control_armed_lastrow", Description: "Synth tab — the RIGHTMOST top-row control ARMED: the grown 2×2 block stays in the clicked row (grows left) instead of dropping to the row below; the left neighbour reflows down (in-place anchor)",
		Subject: SubjectSynthPanel,
		Setup:   synthControlArmedLastInRowSceneSetup()},
	{Name: "crop_synth_control_armed_bottomrow", Description: "Synth tab — the BOTTOM-most visible control of a scrolling FM stage ARMED (capture narrow, e.g. -win-w 560): the card grows DOWN (never up) and its off-window bottom corner is dropped, not unioned into a giant rect",
		Subject: SubjectSynthPanel,
		Setup:   synthControlArmedBottomRowSceneSetup()},
	{Name: "crop_synth_snare_voice_tone2_armed", Description: "Synth tab — snare VOICE stage with Tone2 Freq ARMED: the single-grid-row stage must still EXPAND the card (2-column footprint + both barrel lanes), not collapse to one in-place cell",
		Subject: SubjectSynthPanel,
		Setup:   synthSnareVoiceTone2ArmedSceneSetup()},
	// Beginner MACRO drill-down layer (synth_macro_render.go): melodic AND
	// percussion instruments show plain-English concept cards at the Basic root;
	// tapping one drills into its params with a "Basic" pill to climb back.
	{Name: "crop_synth_macro_root", Description: "Synth tab — beginner macro layer at the Basic root: plain-English concept cards (Sound/Brightness/Attack/Length/…) for a melodic instrument (violin)",
		Subject:     SubjectSynthPanel,
		Setup:       macroSceneSetup(roster.Violin, ""),
		MobileSetup: mobileMacroSceneSetup(roster.Violin, "")},
	{Name: "crop_synth_macro_drilled", Description: "Synth tab — beginner macro layer drilled into the Brightness concept: its param cards + the '◀ Basic' pill",
		Subject:     SubjectSynthPanel,
		Setup:       macroSceneSetup(roster.Violin, "brightness"),
		MobileSetup: mobileMacroSceneSetup(roster.Violin, "brightness")},
	// Percussion macro layer: a kick row shows the drum-friendly concept set
	// (Punch/Boom/Tone/Length/Source/Grit) instead of the melodic one.
	{Name: "crop_synth_macro_perc_root", Description: "Synth tab — beginner macro layer at the Basic root for a PERCUSSION instrument (808 kick): Punch/Boom/Tone/Length/Source concept cards",
		Subject:     SubjectSynthPanel,
		Setup:       macroSceneSetup(roster.Kick808, ""),
		MobileSetup: mobileMacroSceneSetup(roster.Kick808, "")},
	{Name: "crop_synth_macro_perc_drilled", Description: "Synth tab — percussion macro layer drilled into Boom: its params + the '◀ Basic' pill",
		Subject:     SubjectSynthPanel,
		Setup:       macroSceneSetup(roster.Kick808, "boom"),
		MobileSetup: mobileMacroSceneSetup(roster.Kick808, "boom")},
	// MACRO DIALS (synth_dials.go): up to three dials per root card, each a
	// vector over existing params (internal/assets/synth_dials.json).
	// RENDERED-AUDIO GRAPH (synth_graph_pane.go, spec 2026-10-24): one view
	// per kind, each a focused dial (Turn ends as ghosts), on an instrument
	// where the dial is live.
	{Name: "crop_synth_view_overview", Description: "Synth graph — the 808 kick's overview (nothing focused): the rendered note, its colour strip, note end, summary readout",
		Subject:     SubjectSynthPanel,
		Setup:       synthViewSceneSetup(roster.Kick808, "", "", -1, false),
		MobileSetup: synthViewSceneSetup(roster.Kick808, "", "", -1, true)},
	{Name: "crop_synth_view_pitch_drop", Description: "Synth graph — Drop focused on the 808 kick: pitch over time (a 150 to 44 Hz glide) with the dial at 0/100 as ghosts",
		Subject:     SubjectSynthPanel,
		Setup:       synthViewSceneSetup(roster.Kick808, "pitchdrop", "drop", -1, false),
		MobileSetup: synthViewSceneSetup(roster.Kick808, "pitchdrop", "drop", -1, true)},
	{Name: "crop_synth_view_attack_snap", Description: "Synth graph — Snap focused on the 808 kick: the first moment (waveform + envelope), ends as ghosts",
		Subject:     SubjectSynthPanel,
		Setup:       synthViewSceneSetup(roster.Kick808, "punch", "snap", -1, false),
		MobileSetup: synthViewSceneSetup(roster.Kick808, "punch", "snap", -1, true)},
	{Name: "crop_synth_view_loudness_length", Description: "Synth graph — Length focused on the violin: loudness over time on a fixed axis, note-end marker",
		Subject:     SubjectSynthPanel,
		Setup:       synthViewSceneSetup(roster.Violin, "length", "length", -1, false),
		MobileSetup: synthViewSceneSetup(roster.Violin, "length", "length", -1, true)},
	{Name: "crop_synth_view_colour_bright", Description: "Synth graph — Bright turned from 56 to 85 on the violin: tone colour with the BEFORE ghost and difference shading",
		Subject:     SubjectSynthPanel,
		Setup:       synthViewSceneSetup(roster.Violin, "brightness", "bright", 85, false),
		MobileSetup: synthViewSceneSetup(roster.Violin, "brightness", "bright", 85, true)},
	{Name: "crop_synth_view_harmonics_richness", Description: "Synth graph — Richness focused on the FM e-piano: the harmonic ladder",
		Subject:     SubjectSynthPanel,
		Setup:       synthViewSceneSetup(roster.FmEpiano, "sound", "richness", -1, false),
		MobileSetup: synthViewSceneSetup(roster.FmEpiano, "sound", "richness", -1, true)},
	{Name: "crop_synth_view_colourtime_sweep", Description: "Synth graph — Sweep focused on the cello: tone colour over time with the brightness-centre trajectories",
		Subject:     SubjectSynthPanel,
		Setup:       synthViewSceneSetup(roster.Cello, "brightness", "sweep", -1, false),
		MobileSetup: synthViewSceneSetup(roster.Cello, "brightness", "sweep", -1, true)},
	{Name: "crop_synth_view_noise_sizzle", Description: "Synth graph — Sizzle focused on the snare: tone vs noise over time",
		Subject:     SubjectSynthPanel,
		Setup:       synthViewSceneSetup(roster.Snare, "tone", "sizzle", -1, false),
		MobileSetup: synthViewSceneSetup(roster.Snare, "tone", "sizzle", -1, true)},
	{Name: "crop_synth_view_wave_drive", Description: "Synth graph — Drive focused on the kick: a wave close-up",
		Subject:     SubjectSynthPanel,
		Setup:       synthViewSceneSetup(roster.Kick, "grit", "drive", -1, false),
		MobileSetup: synthViewSceneSetup(roster.Kick, "grit", "drive", -1, true)},
	{Name: "crop_synth_view_pitch_vibrato", Description: "Synth graph — Vibrato focused on the supersaw lead: pitch wobble over time",
		Subject:     SubjectSynthPanel,
		Setup:       synthViewSceneSetup(roster.LeadSupersaw, "humanfeel", "vibrato", -1, false),
		MobileSetup: synthViewSceneSetup(roster.LeadSupersaw, "humanfeel", "vibrato", -1, true)},
	{Name: "crop_synth_view_glow", Description: "Synth graph — the trigger glow 60 ms after the 808 kick fired (Drop focused): the NOW curve shines, the picture stays",
		Subject:     SubjectSynthPanel,
		Setup:       synthGlowSceneSetup(false),
		MobileSetup: synthGlowSceneSetup(true)},
	{Name: "crop_synth_dials_root", Description: "Synth tab — macro dials on the violin's category cards, each reading the shipped sound (0…100)",
		Subject:     SubjectSynthPanel,
		Setup:       dialSceneSetup(roster.Violin, nil, ""),
		MobileSetup: mobileDialSceneSetup(roster.Violin, nil, "")},
	{Name: "crop_synth_dials_perc_root", Description: "Synth tab — macro dials on the 808 kick's percussion cards (Snap / Weight / Drop / Bright …)",
		Subject:     SubjectSynthPanel,
		Setup:       dialSceneSetup(roster.Kick808, nil, ""),
		MobileSetup: mobileDialSceneSetup(roster.Kick808, nil, "")},
	{Name: "crop_synth_dials_turned", Description: "Synth tab — violin dials set: Bright 85, Edge 60, Length 30, Tail 20 (value arcs, graphs morphed)",
		Subject:     SubjectSynthPanel,
		Setup:       dialSceneSetup(roster.Violin, synthDialSceneTurns, ""),
		MobileSetup: mobileDialSceneSetup(roster.Violin, synthDialSceneTurns, "")},
	{Name: "synth_dials_popup", Description: "Synth tab — a tapped macro dial: the shared value wheel open on Bright (85)",
		Setup:       dialSceneSetup(roster.Violin, synthDialSceneTurns, "brightness/bright"),
		MobileSetup: mobileDialSceneSetup(roster.Violin, synthDialSceneTurns, "brightness/bright")},
	// More than three dials on a card lay out in two rows of up to three.
	{Name: "crop_synth_dials_four", Description: "Synth tab — the cello's Brightness card with four dials (Bright · Edge · Sweep · Bloom) in two rows",
		Subject:     SubjectSynthPanel,
		Setup:       dialFocusSceneSetup(roster.Cello, "brightness", false),
		MobileSetup: dialFocusSceneSetup(roster.Cello, "brightness", true)},
	{Name: "crop_synth_dials_voice", Description: "Synth tab — a formant voice's Voice card with five dials (Character · Breath · Shift · Shine · Talk)",
		Subject:     SubjectSynthPanel,
		Setup:       dialFocusSceneSetup(roster.VoiceAlien, "voice", false),
		MobileSetup: dialFocusSceneSetup(roster.VoiceAlien, "voice", true)},
	{Name: "crop_synth_ride_modal_root", Description: "Synth tab — the modal-plate RIDE at the Basic root: percussion concept cards over the cymbal plate voice",
		Subject:     SubjectSynthPanel,
		Setup:       macroSceneSetup(roster.Ride, ""),
		MobileSetup: mobileMacroSceneSetup(roster.Ride, "")},
	{Name: "crop_synth_ride_modal_tone", Description: "Synth tab — ride drilled into Tone with More open: the modal-plate knobs (Strike Tone/Width, Shimmer, Wash, Plate Modes, …) the plate actually reads",
		Subject:     SubjectSynthPanel,
		Setup:       synthMoreSceneSetup(roster.Ride, "tone"),
		MobileSetup: mobileSynthMoreSceneSetup(roster.Ride, "tone")},
	{Name: "crop_synth_hihat_rattle", Description: "Synth tab — the modal-plate HI-HAT drilled into Tone with More open: plate knobs plus the Rattle knobs (two clamped cymbals colliding) that only a hi-hat shows",
		Subject:     SubjectSynthPanel,
		Setup:       synthMoreSceneSetup(roster.Hihat, "tone"),
		MobileSetup: mobileSynthMoreSceneSetup(roster.Hihat, "tone")},
	{Name: "crop_synth_hihat_punch_more", Description: "Synth tab — hi-hat drilled into Punch with More open: Softness, Tick Corner, Sparkle and the Advanced Wire Delay (driven-plate onset) knob",
		Subject:     SubjectSynthPanel,
		Setup:       synthMoreScrolledSceneSetup(roster.Hihat, "punch"),
		MobileSetup: mobileSynthMoreSceneSetup(roster.Hihat, "punch")},
	{Name: "crop_synth_hihat_length_more", Description: "Synth tab — hi-hat drilled into Length with More open: the Rattle Length and Advanced Buzz Hold (collision-rate hold: a fused buzz instead of audible ticks) knobs",
		Subject:     SubjectSynthPanel,
		Setup:       synthMoreScrolledSceneSetup(roster.Hihat, "length"),
		MobileSetup: mobileSynthMoreSceneSetup(roster.Hihat, "length")},
	{Name: "crop_synth_kick_physical_length", Description: "Synth tab — the PHYSICAL kick (Boom Bap, variant 8) drilled into Length with More open: Body Hold, the variant-8-only knob that holds the body undamped before it decays",
		Subject:     SubjectSynthPanel,
		Setup:       synthMoreScrolledSceneSetup(roster.KickBoomBap, "length"),
		MobileSetup: mobileSynthMoreSceneSetup(roster.KickBoomBap, "length")},
	{Name: "crop_synth_kick_physical_boom", Description: "Synth tab — the PHYSICAL Muted kick drilled into Boom with More open: Air (the low 'vacuum' noise that makes a recorded kick sound real), shown only on variant-8 kicks",
		Subject:     SubjectSynthPanel,
		Setup:       synthMoreScrolledSceneSetup(roster.KickMuted, "boom"),
		MobileSetup: mobileSynthMoreSceneSetup(roster.KickMuted, "boom")},
	{Name: "crop_synth_sub_bass_root", Description: "Synth tab — Sub Bass (the analytic dual-osc voice, gen source 4) at the root: its own BASS family knobs (Harmonics / Pitch Drop / Punch / Body Decay / Drive / Level + base Pitch) claimed by the percussion cards",
		Subject:     SubjectSynthPanel,
		Setup:       macroSceneSetup(roster.SubBass, ""),
		MobileSetup: mobileMacroSceneSetup(roster.SubBass, "")},
	{Name: "crop_synth_sub_bass_punch", Description: "Synth tab — Sub Bass drilled into Punch: the analytic voice's Punch + Punch Speed essentials lead the card",
		Subject:     SubjectSynthPanel,
		Setup:       macroSceneSetup(roster.SubBass, "punch"),
		MobileSetup: mobileMacroSceneSetup(roster.SubBass, "punch")},
	{Name: "crop_synth_sub_bass_boom", Description: "Synth tab — Sub Bass drilled into Boom: base Pitch (voice_freq_hz 45 Hz) + Level",
		Subject:     SubjectSynthPanel,
		Setup:       macroSceneSetup(roster.SubBass, "boom"),
		MobileSetup: mobileMacroSceneSetup(roster.SubBass, "boom")},
	{Name: "crop_synth_sub_bass_punch_more", Description: "Synth tab — Sub Bass drilled into Punch with More open: Punch + Punch Speed (the analytic voice's attack boost)",
		Subject:     SubjectSynthPanel,
		Setup:       synthMoreScrolledSceneSetup(roster.SubBass, "punch"),
		MobileSetup: mobileSynthMoreSceneSetup(roster.SubBass, "punch")},
	{Name: "crop_synth_kick_base_boom", Description: "Synth tab — the base Kick drilled into Boom with More open: NO Body Hold / Air (only the Physical voice reads them)",
		Subject:     SubjectSynthPanel,
		Setup:       synthMoreScrolledSceneSetup(roster.Kick, "boom"),
		MobileSetup: mobileSynthMoreSceneSetup(roster.Kick, "boom")},
	{Name: "crop_synth_hihat_tone_plate_modes_zero", Description: "Synth tab — hi-hat with Plate Modes dialed to 0, drilled into Tone with More open: the plate knobs STAY (the hi-hat is always the modal plate; the engine clamps Plate Modes to >= 1)",
		Subject:     SubjectSynthPanel,
		Setup:       withInstrumentParamSceneSetup(roster.Hihat, "gen1_cym_modes", 0, synthMoreSceneSetup(roster.Hihat, "tone")),
		MobileSetup: withInstrumentParamSceneSetup(roster.Hihat, "gen1_cym_modes", 0, mobileSynthMoreSceneSetup(roster.Hihat, "tone"))},
	{Name: "crop_synth_cowbell_tone_no_plate", Description: "Synth tab — cowbell drilled into Tone with More open: NO modal-plate knobs (a cowbell is not a plate, so they would move nothing)",
		Subject:     SubjectSynthPanel,
		Setup:       synthMoreSceneSetup(roster.Cowbell, "tone"),
		MobileSetup: mobileSynthMoreSceneSetup(roster.Cowbell, "tone")},
	{Name: "crop_synth_drilled_stage", Description: "Synth tab — drilled into a pipeline STAGE (violin, FILTER via the strip pill): Back · stage title/one-liner · the stage enable pill, then the stage's essential ParamCards",
		Subject:     SubjectSynthPanel,
		Setup:       synthStageSceneSetup(roster.Violin, synthSectionFilter),
		MobileSetup: mobileSynthStageSceneSetup(roster.Violin, synthSectionFilter)},
	{Name: "crop_synth_snare_generator_voice", Description: "Synth tab — config-first snare drilled into its VOICE stage: the family-agnostic Generator (gen1_wave: Sine/Saw/Square/Triangle) beside the snare's own knobs",
		Subject:     SubjectSynthPanel,
		Setup:       synthStageSceneSetup(roster.Snare, synthSectionVoice),
		MobileSetup: mobileSynthStageSceneSetup(roster.Snare, synthSectionVoice)},
	{Name: "crop_synth_modal_root", Description: "Synth tab — a WAV-clone-shaped instrument (generated MODAL + STRIKE + NOISE document on tubular bells) at the ROOT: the MODAL / STRIKE / NOISE pills in the pipeline strip and the Ring / Mallet cards drawing the mode spectrum",
		Subject:     SubjectSynthPanel,
		Setup:       modalSceneSetup(false, ""),
		MobileSetup: modalSceneSetup(true, "")},
	{Name: "crop_synth_modal_graph", Description: "Synth tab — the same clone-shaped instrument drilled into Ring: Tune / Damping / Brightness / Partials cards and the mode-spectrum stem graph (stems = ringing modes, faded by ring time; soft bands = the NOISE rows)",
		Subject:     SubjectSynthPanel,
		Setup:       modalSceneSetup(false, "ring"),
		MobileSetup: modalSceneSetup(true, "ring")},
	{Name: "crop_synth_snare_generator_source", Description: "Synth tab — config-first snare drilled into the Source card, which claims the Generator (gen1_wave)",
		Subject:     SubjectSynthPanel,
		Setup:       macroSceneSetup(roster.Snare, "source"),
		MobileSetup: mobileMacroSceneSetup(roster.Snare, "source")},
	{Name: "crop_synth_drilled_more", Description: "Synth tab — drilled into Boom on an 808 kick with the More expander OPEN: fine-tuning cards below the More divider",
		Subject:     SubjectSynthPanel,
		Setup:       synthMoreSceneSetup(roster.Kick808, "boom"),
		MobileSetup: mobileSynthMoreSceneSetup(roster.Kick808, "boom")},
	{Name: "crop_synth_physical_kick_boom_more", Description: "Synth tab — 808 kick switched to the PHYSICAL variant (8), drilled into Boom with More open: the variant's Air knob (broadband noise floor) focused, its plain-English gloss in the graph header",
		Subject:     SubjectSynthPanel,
		Setup:       physicalKickSceneSetup(synthMoreScrolledSceneSetup(roster.Kick808, "boom"), "gen1_kick_hiss"),
		MobileSetup: physicalKickSceneSetup(mobileSynthMoreSceneSetup(roster.Kick808, "boom"), "gen1_kick_hiss")},
	{Name: "crop_synth_physical_kick_length_more", Description: "Synth tab — 808 kick on the PHYSICAL variant (8), drilled into Length with More open: the variant's Body Hold knob (undamped body before the decay) focused, its plain-English gloss in the graph header",
		Subject:     SubjectSynthPanel,
		Setup:       physicalKickSceneSetup(synthMoreScrolledSceneSetup(roster.Kick808, "length"), "gen1_kick_hold"),
		MobileSetup: physicalKickSceneSetup(mobileSynthMoreSceneSetup(roster.Kick808, "length"), "gen1_kick_hold")},
	{Name: "crop_synth_macro_armed", Description: "Synth tab — macro layer drilled into Brightness with a control ARMED: grown 2×2 card + two barrel lanes; neighbours reflowed (not hidden), desktop + mobile",
		Subject:     SubjectSynthPanel,
		Setup:       macroArmedSceneSetup(roster.Violin, "brightness"),
		MobileSetup: mobileMacroArmedSceneSetup(roster.Violin, "brightness")},
	{Name: "crop_synth_macro_arming", Description: "Synth tab — macro layer MID grow animation (armGrowT≈0.35): the armed card is partway to its 2×2 block and neighbours are sliding to their reflowed slots",
		Subject: SubjectSynthPanel,
		Setup:   macroArmingMidSceneSetup(roster.Violin, "brightness")},
	// FULL-SCREEN (no Subject) on purpose: the bug this guards against sent a
	// neighbour flying toward the SCREEN ORIGIN, i.e. outside the synth panel — a
	// panel-cropped capture could not show it. Capture short/narrow (e.g.
	// -win-w 900 -win-h 1000) so the grid has a single visible row and the
	// neighbour's reflowed row is genuinely off-window.
	{Name: "synth_macro_arming_offwindow", Description: "Synth tab — macro layer MID grow animation on a SHORT panel: the neighbour whose reflowed row is OFF-WINDOW slides DOWN under the grid's bottom edge instead of flying up-left to the screen origin",
		Setup: macroArmingMidSceneSetup(roster.Violin, "brightness")},
	{Name: "crop_synth_tab_detail_envelope", Description: "Synth tab — chip strip with the ENVELOPE stage expanded into the detail pane (full-size knobs, untruncated captions)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabStageSetup(roster.Snare, "ENVELOPE", ""),
		MobileSetup: mobileSynthTabStageSetup(roster.Snare, "ENVELOPE", "")},
	{Name: "crop_synth_tab_ghost_chip", Description: "Synth tab — modular voice with FILTER disabled AND selected: ghost chip in the strip + bypass scrim + bright enable pill in the detail header",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabStageSetup(roster.Modular, "FILTER", "filter_enabled"),
		MobileSetup: mobileSynthTabStageSetup(roster.Modular, "FILTER", "filter_enabled")},
	// Focus-graph crops: one per property-native domain. Each selects a
	// specific knob in a stage so the single big focus graph renders that
	// knob's domain (replaces the old per-knob concept bands). The mirror
	// scene exercises the de-crammed "Your sound" pane.
	{Name: "crop_synth_focus_filter", Description: "Synth tab focus graph: FILTER stage, Cutoff knob selected (frequency-domain picture)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabFocusControlSetup(roster.Modular, "FILTER", "filter_cutoff"),
		MobileSetup: mobileSynthTabFocusControlSetup(roster.Modular, "FILTER", "filter_cutoff")},
	{Name: "crop_synth_focus_env", Description: "Synth tab (developer): ENVELOPE stage, Attack knob selected — the attack view of the rendered note, knob Min/Max as ghosts",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabFocusControlSetup(roster.Modular, "ENVELOPE", "amp_attack"),
		MobileSetup: mobileSynthTabFocusControlSetup(roster.Modular, "ENVELOPE", "amp_attack")},
	{Name: "crop_synth_focus_post", Description: "Synth tab (developer): POST stage, Drive knob selected — the wave close-up view",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabFocusControlSetup(roster.Modular, "POST", "drive"),
		MobileSetup: mobileSynthTabFocusControlSetup(roster.Modular, "POST", "drive")},
	// bass-fm plays the FM OSCILLATOR (osc.fm source), whose module owns the FM
	// stage's operators; the bare modular voice's sine oscillator has none.
	{Name: "crop_synth_focus_fm", Description: "Synth tab (developer): FM stage, Op2 Depth knob selected on bass-fm (FM oscillator) — the harmonics view",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabFocusControlSetup(roster.BassFm, "FM", "fm_op2_depth"),
		MobileSetup: mobileSynthTabFocusControlSetup(roster.BassFm, "FM", "fm_op2_depth")},
	{Name: "crop_synth_focus_osc", Description: "Synth tab (developer): OSC stage, Oscillator shape knob selected — the wave close-up view",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabFocusControlSetup(roster.Modular, "OSC", "osc_type"),
		MobileSetup: mobileSynthTabFocusControlSetup(roster.Modular, "OSC", "osc_type")},
	{Name: "crop_synth_focus_pitch", Description: "Synth tab (developer): OSC stage, Octave knob selected — the pitch view",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabFocusControlSetup(roster.Modular, "OSC", "osc_octave"),
		MobileSetup: mobileSynthTabFocusControlSetup(roster.Modular, "OSC", "osc_octave")},
	{Name: "crop_synth_focus_motion", Description: "Synth tab (developer): LFO stage, Rate knob selected — the view the LFO's target names (loudness for Amp)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabFocusControlSetup(roster.Modular, "LFO", "lfo_rate"),
		MobileSetup: mobileSynthTabFocusControlSetup(roster.Modular, "LFO", "lfo_rate")},
	{Name: "crop_synth_focus_burst", Description: "Synth tab (developer): BURST stage, Hit 1 Level knob selected — the attack view",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabFocusControlSetup(roster.Modular, "BURST", "burst1_amp"),
		MobileSetup: mobileSynthTabFocusControlSetup(roster.Modular, "BURST", "burst1_amp")},
	// Config-first CONVERTED families at the Synth-tab root: the category cards
	// must claim the gen-bank knobs (which the family-stage registry, not the
	// hand-written tables, now assigns) and must show the registry's LABELS
	// rather than a prettified raw id. tom is percussive (Punch/Boom/Tone/…
	// table) and fm-bass melodic (Sound/Body/Brightness/… table), so the pair
	// covers both category vocabularies.
	{Name: "crop_synth_root_tom", Description: "Synth tab root: TOM (config-first, gen source 6) — category cards claiming the registry's gen1_tom_* knobs",
		Subject: SubjectSynthPanel,
		Setup:   synthTabSceneSetup(roster.Tom)},
	{Name: "crop_synth_root_fm_bass", Description: "Synth tab root: FM Bass (config-first, gen source 10) — melodic category cards claiming the registry's gen1_fm_* knobs",
		Subject: SubjectSynthPanel,
		Setup:   synthTabSceneSetup(roster.FmBass)},
	{Name: "crop_synth_focus_fm_decay", Description: "Synth tab (developer): FM Bass VOICE, Op 2 Decay knob selected — the colour-over-time view",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabFocusControlSetup(roster.FmBass, "VOICE", "gen1_fm_dec2"),
		MobileSetup: mobileSynthTabFocusControlSetup(roster.FmBass, "VOICE", "gen1_fm_dec2")},
	{Name: "crop_synth_focus_fm_penv", Description: "Synth tab (developer): FM Bass VOICE, Pitch Sweep knob selected — the pitch view",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabFocusControlSetup(roster.FmBass, "VOICE", "gen1_fm_pe_amt"),
		MobileSetup: mobileSynthTabFocusControlSetup(roster.FmBass, "VOICE", "gen1_fm_pe_amt")},
	{Name: "crop_synth_finish_kick", Description: "Synth tab — Kick drilled into FINISH (always-on, locked pill): Resample, Fade, Low Boost, Air, Darken, Crunch (+ Fade Speed / Limit under More)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabStageSetup(roster.Kick, "FINISH", ""),
		MobileSetup: mobileSynthTabStageSetup(roster.Kick, "FINISH", "")},
	{Name: "crop_synth_finish_violin_air", Description: "Synth tab (developer): violin FINISH stage, Air knob selected — the colour view",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabFocusControlSetup(roster.Violin, "FINISH", "post_brightness"),
		MobileSetup: mobileSynthTabFocusControlSetup(roster.Violin, "FINISH", "post_brightness")},
	{Name: "crop_synth_focus_sub_bass_pitch", Description: "Synth tab (developer): Sub Bass base Pitch (voice_freq_hz) selected — the pitch view",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabFocusControlSetup(roster.SubBass, "VOICE", "voice_freq_hz"),
		MobileSetup: mobileSynthTabFocusControlSetup(roster.SubBass, "VOICE", "voice_freq_hz")},
	{Name: "crop_synth_focus_burst_hit", Description: "Synth tab (developer): BURST stage (enabled), Hit 2 Level knob selected — the attack view",
		Mobile:      true,
		Subject:     SubjectSynthPanel,
		Setup:       synthTabFocusControlParamSetup(roster.Modular, "BURST", "burst2_amp", "burst_enabled", 1),
		MobileSetup: mobileSynthTabFocusControlParamSetup(roster.Modular, "BURST", "burst2_amp", "burst_enabled", 1)},
	{Name: "crop_synth_focus_drive", Description: "Synth tab (developer): POST stage, Drive knob at ~70% — the wave close-up view",
		Mobile:      true,
		Subject:     SubjectSynthPanel,
		Setup:       synthTabFocusControlParamSetup(roster.Modular, "POST", "drive", "drive", 0.7),
		MobileSetup: mobileSynthTabFocusControlParamSetup(roster.Modular, "POST", "drive", "drive", 0.7)},
	{Name: "crop_sampler_tab_captured", Description: "Sampler tab cropped — synth one-shot captured, trimmed to middle 70%, Normalize on",
		Subject:     SubjectSampler,
		Setup:       samplerTabSceneSetup(true),
		MobileSetup: mobileSamplerTabSetup(true)},
	{Name: "crop_sampler_tab_empty", Description: "Sampler tab cropped — no buffer loaded (capture/load placeholder state)",
		Subject:     SubjectSampler,
		Setup:       samplerTabSceneSetup(false),
		MobileSetup: mobileSamplerTabSetup(false)},
	{Name: "sampler_wheel_open", Description: "Sampler tab — value-wheel popup open on a knob: larger values above the center selector, coarsest resolution rung at the top of the ladder",
		Setup:       samplerWheelOpenSetup(samplerTabSceneSetup(true)),
		MobileSetup: samplerWheelOpenSetup(mobileSamplerTabSetup(true))},
	{Name: "sampler_wheel_keyboard_step", Description: "Sampler value-wheel after keyboard stepping: ArrowUp raised the value (barrel moved off its opening row), ArrowRight coarsened the resolution (a higher ladder rung is lit)",
		Setup:       samplerWheelKeyStepSetup(samplerTabSceneSetup(true)),
		MobileSetup: samplerWheelKeyStepSetup(mobileSamplerTabSetup(true))},
	{Name: "crop_audio_selector_bar", Description: "AudioStickyBar standalone crop", SettleFrames: 90,
		Subject: SubjectAudioStickyBar,
		Setup:   func(g *Game) { _ = g.SetActiveEQTab("eq") }},
	{Name: "crop_levels_single_channel", Description: "Levels tab single-channel detail",
		SettleFrames: 120, Subject: SubjectEQTabLevels,
		Setup: func(g *Game) { _ = g.SetActiveEQTab("levels"); g.SetPlaying(true) }},
	// The B-unset state is the one the legibility pass most changed: before it,
	// a user with only A assigned had NOTHING on screen telling them a second
	// trace existed or how to set it. The legend now shows a dimmed B slot
	// reading "tap a stage", and the hint reads NEXT TAP SETS B.
	{Name: "crop_chain_tap_b_unset", Description: "Chain panel with only Tap A set — B slot invites a tap",
		SettleFrames: 150, Subject: SubjectChain,
		Setup: func(g *Game) {
			_ = g.SetActiveEQTab("chain")
			g.SetChainVisible(true)
			if cz := chainZoneOf(g); cz != nil {
				cz.SetTapA(scope.StageSynth)
				cz.SetTapB(-1)
			}
		}},
	{Name: "crop_chain_with_two_stages", Description: "Chain panel with both Tap A and Tap B set",
		SettleFrames: 150, Subject: SubjectChain,
		Setup: func(g *Game) {
			_ = g.SetActiveEQTab("chain")
			g.SetChainVisible(true)
			if cz := chainZoneOf(g); cz != nil {
				cz.SetTapA(scope.StageSynth)
				cz.SetTapB(scope.StageEQ)
			}
		}},
	// The ONLY Chain scene that puts an actual waveform on screen. Every
	// other one captures an empty scope, because the desktop screenshot
	// binary never starts the scope service — which is how a full-height
	// trigger marker ended up being the sole mark in the viewport and read
	// as a stray object. This scene is the regression surface for BOTH
	// halves of that fix: the marker is present here (it has a trace to
	// anchor) and it is drawn exactly once.
	{Name: "crop_chain_live_traces", Description: "Chain panel with real A/B traces — trigger marker anchors a waveform",
		Mobile: true, SettleFrames: 150, Subject: SubjectChain,
		Setup: sceneChainLiveTraces,
		MobileSetup: func(g *Game) {
			// Setup alone leaves the mobile audio panel invisible, so the crop
			// silently falls back to the pads view (see CLAUDE.md).
			g.SetForceMobileProfile(true)
			g.drum.SetMobileEQMode(true)
			sceneChainLiveTraces(g)
		}},
	{Name: "crop_chain_default", Description: "Chain panel running, both traces, AG on", SettleFrames: 150,
		Subject: SubjectChain,
		Setup:   activateScopeTab,
		MobileSetup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.SetMobileEQMode(true)
			activateScopeTab(g)
		}},
	{Name: "crop_chain_frozen", Description: "Chain frozen via freeze button", SettleFrames: 150,
		Subject: SubjectChain,
		Setup: func(g *Game) {
			activateScopeTab(g)
			if z := chainZoneOf(g); z != nil {
				z.SetFrozen(true)
			}
		}},
	{Name: "crop_chain_trace_a_only", Description: "Chain with trace B hidden", SettleFrames: 150,
		Subject: SubjectChain,
		Setup: func(g *Game) {
			activateScopeTab(g)
			if z := chainZoneOf(g); z != nil {
				z.SetTraceVisible("B", false)
			}
		}},
	{Name: "crop_chain_zoomed_in", Description: "Chain with X-window narrowed to 5ms", SettleFrames: 150,
		Subject: SubjectChain,
		Setup: func(g *Game) {
			activateScopeTab(g)
			if z := chainZoneOf(g); z != nil {
				z.SetWindowMs(5)
			}
		}},
	{Name: "crop_chain_autofit_off", Description: "Chain with auto-fit off via a manual 40ms zoom (no FIT pill; manual zoom is what disables it)",
		SettleFrames: 150, Subject: SubjectChain,
		Setup: func(g *Game) {
			activateScopeTab(g)
			if z := chainZoneOf(g); z != nil {
				z.SetWindowMs(40) // manual zoom is the only way to leave auto-fit now
			}
		}},
	{Name: "crop_chain_split", Description: "Chain in split A/B display mode (legend strip)",
		SettleFrames: 150, Subject: SubjectChain,
		Setup: func(g *Game) {
			activateScopeTab(g)
			if z := chainZoneOf(g); z != nil {
				z.SetDisplayMode("split")
			}
		}},
	{Name: "crop_chain_diff", Description: "Chain in A-B difference display mode (legend strip)",
		SettleFrames: 150, Subject: SubjectChain,
		Setup: func(g *Game) {
			activateScopeTab(g)
			if z := chainZoneOf(g); z != nil {
				z.SetDisplayMode("diff")
			}
		}},
	// This scene captures the mobile-only segmented OVR/SPL/DIF control, so
	// it must be marked Mobile:true — otherwise it never enters the mobile
	// capture pass (-list-mobile-scenes filters on Mobile) and the desktop
	// Setup renders the desktop display-mode pills instead of the segmented
	// control, i.e. "renders desktop".
	{Name: "crop_chain_segmented_mobile", Description: "Chain mobile segmented OVR/SPL/DIF control",
		Mobile: true, SettleFrames: 150, Subject: SubjectChain,
		Setup: func(g *Game) {
			// Desktop fallback Setup still forces the mobile profile so even a
			// desktop-pass capture shows the segmented control rather than the
			// desktop pills.
			g.SetForceMobileProfile(true)
			g.drum.SetMobileEQMode(true)
			activateScopeTab(g)
		},
		MobileSetup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.SetMobileEQMode(true)
			activateScopeTab(g)
		}},
	{Name: "crop_fx_panel_with_3_effects", Description: "FX panel cropped (delay+reverb+distortion)", SettleFrames: 150,
		Subject: SubjectFXPanel,
		Setup: func(g *Game) {
			ensureRow(g, 0)
			instID := g.drum.Rows[0].Instrument
			audio.AddInsertEffect(instID, audio.EffectDelay, nil)
			audio.AddInsertEffect(instID, audio.EffectReverb, nil)
			audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
			g.drum.OpenFXPanel(0)
		}},
	{Name: "crop_toolbar_playing", Description: "Top toolbar/transport while playing",
		Subject: SubjectToolbar,
		Setup:   func(g *Game) { g.SetPlaying(true) }},
}

// ─── helpers ──────────────────────────────────────────────────────

// sceneEQLiveBackdrop opens the EQ tab over an injected analyser reading: a
// spectrum tilted down toward the treble, i.e. the shape of ordinary program
// material rather than a flat test tone.
func sceneEQLiveBackdrop(g *Game) {
	_ = g.SetActiveEQTab("eq")
	const n = 256
	spec := make([]float64, n)
	for i := range spec {
		// -22 dBFS at the bottom falling to about -46 dBFS at the top.
		spec[i] = 0.08 * math.Exp(-float64(i)/70)
	}
	snap := audio.AnalyzerSnapshot{RMS: 0.05, Peak: 0.08, Spectrum: spec}
	g.drum.eqTestSnapshot = &snap
}

// sceneSpectrumMutedChannelPick reproduces the tiny-bar defect deterministically:
// a per-instrument channel is selected while that channel is SILENT, the
// silence outlasts the axis settle window, and only then does the signal
// arrive — with no further channel change to trigger a re-pin.
//
// It scripts the analyser rather than muting a row, for two reasons found the
// hard way: muting changes the audible set, so reconcileChannelWithAudibleSet
// bounces the selector back to Master on the way out (re-picking the channel
// and hiding the bug), and zeroing the row's volume does not silence the
// per-instrument analyser tap at all. Scripting the seam removes both
// accidents and the dependence on where the beat happens to fall.
func sceneSpectrumMutedChannelPick(g *Game) {
	_ = g.SetActiveEQTab("spectrum")
	g.SetPlaying(true)
	g.QueueAction(func(g *Game) {
		g.QueueAction(func(g *Game) {
			z := g.drum.eqPanelZone
			if z == nil || len(g.drum.Rows) == 0 {
				return
			}
			g.drum.selectAudioChannel(channel.Of(g.drum.Rows[0].Instrument))

			start := time.Now()
			const silentFor = 2 * time.Second // outlasts spectrumChannelSettle
			z.callbacks.AnalyzerState = func() *analyzer.State {
				// A fresh State per call: the pointer is the zone's
				// "data refreshed" signal.
				st := &analyzer.State{}
				det := sceneKickSpectrum(time.Since(start) >= silentFor)
				st.Detail = det
				st.Master = *det
				return st
			}
		})
	})
}

// sceneKickSpectrum returns a low-heavy spectrum (a kick) when live, or the
// analyser's silence reading when not. -Inf is what a real silent bin
// reports, which is also the value the band maths has to survive.
//
// Bin spacing matters: the ISO bands the tab draws start at 22-44 Hz, so a
// coarse synthetic spectrum leaves the bass bands EMPTY and the picture is
// a lie about the renderer rather than about the fixture. 1024 bins to
// Nyquist is ~23 Hz apart, the resolution a real analyser reports.
func sceneKickSpectrum(live bool) *analyzer.ChannelMetrics {
	const n = 1024
	const nyquist = 24000.0
	ch := &analyzer.ChannelMetrics{Active: true}
	for i := 0; i < n; i++ {
		hz := float64(i) * nyquist / float64(n)
		db := math.Inf(-1)
		if live && hz > 0 {
			// ~-30 dB at 60 Hz, rolling off steeply above it: a kick.
			db = -30 - 30*math.Log10(math.Max(hz, 30)/60)
			if db < -78 {
				db = math.Inf(-1)
			}
		}
		ch.FFTBins = append(ch.FFTBins, db)
		ch.FreqBins = append(ch.FreqBins, hz)
	}
	return ch
}

// sceneSpectrumInstrumentChannel plays the circuit and focuses the first
// row's own channel, the case where the analyser is silent between hits.
func sceneSpectrumInstrumentChannel(g *Game) {
	_ = g.SetActiveEQTab("spectrum")
	g.SetPlaying(true)
	g.QueueAction(func(g *Game) {
		g.QueueAction(func(g *Game) {
			if len(g.drum.Rows) > 0 {
				g.drum.selectAudioChannel(channel.Of(g.drum.Rows[0].Instrument))
			}
		})
	})
}

// sceneSpectrumAfterMute starts playback on the Spectrum tab and mutes the
// first row once layout exists, so the capture shows the readout re-based on
// the post-mute mix.
func sceneSpectrumAfterMute(g *Game) {
	_ = g.SetActiveEQTab("spectrum")
	g.SetPlaying(true)
	g.QueueAction(func(g *Game) {
		g.QueueAction(func(g *Game) { g.drum.toggleMute(0) })
	})
}

// sceneChainLiveTraces opens the Chain tab on two stages and feeds it a
// synthetic two-tap trace. Shared by the desktop and mobile setups.
func sceneChainLiveTraces(g *Game) {
	_ = g.SetActiveEQTab("chain")
	g.SetChainVisible(true)
	SetSceneScopeState(sceneScopeStateWithTraces())
	if cz := chainZoneOf(g); cz != nil {
		cz.SetTapA(scope.StageSynth)
		cz.SetTapB(scope.StageEQ)
		cz.SetTraceVisible("A", true)
		cz.SetTraceVisible("B", true)
	}
}

// sceneScopeStateWithTraces builds a two-tap scope.State carrying a decaying
// sine on each tap — a stand-in for what the live scope service delivers, so
// a screenshot can show the trace layer and not just the graticule.
func sceneScopeStateWithTraces() *scope.State {
	mk := func(decay, period float64) []float64 {
		out := make([]float64, 512)
		for i := range out {
			out[i] = 0.85 * math.Exp(-float64(i)/decay) * math.Sin(2*math.Pi*float64(i)/period)
		}
		return out
	}
	synth := audio.AnalyzerSnapshot{RMS: 0.3, Peak: 0.85, Waveform: mk(150, 40)}
	postEQ := audio.AnalyzerSnapshot{RMS: 0.22, Peak: 0.6, Waveform: mk(90, 64)}
	return SynthesizeScopeState(channel.Of(roster.Kick), scope.StageSynth, scope.StageEQ, ScopeStageSnapshots{
		Synth:  synth,
		PostEQ: postEQ,
	})
}

// chainZoneOf returns the scope panel zone reachable from the EQ panel zone,
// or nil if either is not yet wired. Used by crop_scope_* setups to drive
// the same state the AG/freeze/swatch buttons toggle, without synthesizing
// click events.
func chainZoneOf(g *Game) *ChainPanelZone {
	if g == nil || g.drum == nil || g.drum.eqPanelZone == nil {
		return nil
	}
	return g.drum.eqPanelZone.ChainZone()
}

// activateScopeTab applies the canonical Scope-tab visibility setup (tab +
// service visible), assigns default A/B taps so traces actually draw, and
// starts playback so the scope ring buffers fill during the screenshot
// settle countdown. Without taps the trace area renders empty regardless
// of toggle state, making variants visually indistinguishable.
func activateScopeTab(g *Game) {
	_ = g.SetActiveEQTab("chain")
	g.SetChainVisible(true)
	if z := chainZoneOf(g); z != nil {
		z.SetTapA(scope.StageMaster)
		z.SetTapB(scope.StageEQ)
	}
	g.SetPlaying(true)
}

// synthTabSceneSetup returns a Setup function for the synth-tab crop
// scenes. The optional override forces row 0's instrument to a specific
// builtin id (e.g. "fm-bell", "hihat") so the section layout exercises a
// recipe other than the embedded startup-demo's row 0 default. Empty
// override uses whatever the demo's row 0 carries (snare in the embedded
// demo).
//
// The demo is built synchronously via g.buildDemo() before the row
// rewrite. Without that, the scene Setup runs against an empty Rows[]
// because buildDemo is normally deferred to the first Update() — the
// scene Setup runs BEFORE the first Update(), so without forcing the
// demo here the row rewrite is a no-op and the captured PNG shows the
// embedded demo's row 0 instead of the requested override.
// synthTabStageSceneSetup extends synthTabSceneSetup by opening a specific stage,
// so a screenshot can show that stage's knobs (and its Advanced expander) instead
// of the default landing stage.
func synthTabStageSceneSetup(instrument instrument.ID, section synthSectionID) func(*Game) {
	base := synthTabSceneSetup(instrument)
	return func(g *Game) {
		base(g)
		if g.drum != nil && len(g.drum.Rows) > 0 {
			inst := g.drum.resolveSynthInstrument(g.drum.Rows[0].Instrument)
			g.drum.setSelectedSynthSection(inst, section)
			if g.drum.eqPanelZone != nil {
				g.drum.eqPanelZone.Invalidate()
			}
		}
	}
}

// synthTabStageExpandedSceneSetup opens a stage. Advanced (fine-tuning) knobs
// are now always laid out below an in-grid divider row — there is no expander to
// open — so this is just the stage-selection setup (kept as a distinct name for
// the scenes that specifically showcase the Advanced tier, e.g. violin OSC).
func synthTabStageExpandedSceneSetup(instrument instrument.ID, section synthSectionID) func(*Game) {
	return synthTabStageSceneSetup(instrument, section)
}

// synthTabStageDividerSceneSetup opens a stage and scrolls its card grid so the
// "— ADVANCED · N —" divider row (and the first advanced cards below it) are in
// view — a screenshot of the divider scene must actually show the divider, not
// just the essential row above the fold.
func synthTabStageDividerSceneSetup(instrument instrument.ID, section synthSectionID) func(*Game) {
	base := synthTabStageSceneSetup(instrument, section)
	return func(g *Game) {
		base(g)
		dv := g.drum
		if dv == nil {
			return
		}
		sel := dv.synthSelectedSection()
		if sel == nil {
			return
		}
		ess, adv := dv.synthSectionTierSplit(sel.knobIdxs)
		if len(adv) == 0 {
			return
		}
		grid := dv.sectionGrid(sel.id)
		cols := grid.Cols()
		if cols < 1 {
			cols = 1
		}
		dividerStart := ((len(ess) + cols - 1) / cols) * cols
		grid.ScrollToIndex(dividerStart)
		if dv.eqPanelZone != nil {
			dv.eqPanelZone.Invalidate()
		}
	}
}

// spectrumCollisionSetup points the first two rows at instruments whose
// fundamentals share one ISO band, so the Spectrum banner reports a real
// collision. Instruments are set through SetInstrument (never a raw
// Rows[i].Instrument write) so the row label and every dependent of the row's
// instrument id follow — a direct field write leaves them on the previous
// instrument and the capture stops showing what it claims.
// spectrumControlsOf reaches the Spectrum tab's control header, or nil.
func spectrumControlsOf(g *Game) *spectrumControls {
	if g == nil || g.drum == nil || g.drum.eqPanelZone == nil {
		return nil
	}
	return g.drum.eqPanelZone.spectrumControls
}

func spectrumCollisionSetup(g *Game) {
	if g == nil || g.drum == nil || len(g.drum.Rows) < 2 {
		return
	}
	for row, id := range map[int]instrument.ID{0: roster.DnbKick, 1: roster.SubBass} {
		g.drum.selRow = row
		g.drum.SetInstrument(id)
	}
	g.drum.selRow = 0
	_ = g.SetActiveEQTab("spectrum")
	g.SetPlaying(true)
}

func spectrumCollisionSetupMobile(g *Game) {
	if g == nil {
		return
	}
	g.SetForceMobileProfile(true)
	spectrumCollisionSetup(g)
	if g.drum != nil {
		g.drum.setViewMode(viewModeSpectrum)
	}
}

func synthTabSceneSetup(instrumentOverride instrument.ID) func(*Game) {
	return func(g *Game) {
		g.buildDemo()
		if instrumentOverride != "" && len(g.drum.Rows) > 0 {
			// Go through SetInstrument, not a raw Rows[0].Instrument write: the
			// direct field assignment skips the sync notifier, so the row label
			// (and anything else keyed off the row's instrument) stays on the
			// PREVIOUS instrument while the panel shows the new one — the
			// capture then can't be trusted to show what it claims.
			g.drum.selRow = 0
			g.drum.SetInstrument(instrumentOverride)
		}
		_ = g.SetActiveEQTab("synth")
		if g.drum != nil && g.drum.eqPanelZone != nil && len(g.drum.Rows) > 0 {
			// Select Rows[0]'s instrument as the active channel. The Synth tab
			// requires a single-instrument context — Master is now blocked for
			// it — so pick the row explicitly (previously this relied on
			// synthTabActiveInstrument falling back to Rows[0] under Master).
			g.drum.eqPanelZone.SetActiveChannel(channel.Of(g.drum.Rows[0].Instrument))
		}
		settleSynthTab(g)
	}
}

// settleSynthTab lays the audio panel out through the tree, then renders the
// Synth graph synchronously. Its renders are pool-rendered (async) in
// production, so a scene that shows the Synth tab must land them before
// capture or two captures of the same scene differ by whether the worker
// finished — which the theme-drift guard
// (TestRuntimeThemeSwitchMatchesBootEveryScene) reads as a theme diff.
func settleSynthTab(g *Game) {
	if g.drum == nil {
		return
	}
	if g.drum.audioTree != nil {
		g.drum.audioTree.LayoutZoneNow("eq-panel")
	}
	g.drum.settleSynthGraphNow()
}

// synthTabMasterChooserSetup opens the Synth tab and then opens the
// channel chooser dropdown so the capture actually shows the chooser
// surface (the scene's whole point). The panel must be laid out through
// the tree first so its portal exists before OpenChannelDropdown builds
// the dropdown entry.
func synthTabMasterChooserSetup() func(*Game) {
	return func(g *Game) {
		synthTabSceneSetup("")(g)
		if g.drum == nil || g.drum.eqPanelZone == nil {
			return
		}
		if g.drum.audioTree != nil {
			g.drum.audioTree.LayoutZoneNow("eq-panel")
		}
		g.drum.eqPanelZone.OpenChannelDropdown()
	}
}

func mobileSynthTabMasterChooserSetup() func(*Game) {
	return func(g *Game) {
		g.SetForceMobileProfile(true)
		g.drum.SetMobileEQMode(true)
		synthTabMasterChooserSetup()(g)
	}
}

// synthTabNoSynthSceneSetup points the active row at a WAV-sample
// instrument with no synth recipe so the synth tab renders its single
// "this instrument does not use the synth" banner instead of the section
// grid. Mirrors the user-reported state for a loaded WAV.
func synthTabNoSynthSceneSetup() func(*Game) {
	return func(g *Game) {
		g.buildDemo()
		if len(g.drum.Rows) > 0 {
			g.drum.Rows[0].Instrument = "wav-sample"
			g.drum.Rows[0].Name = "Kick-1"
		}
		// A fresh id has no instrument document, so the tab shows the
		// "does not use the synth" banner.
		_ = g.SetActiveEQTab("synth")
		if g.drum != nil && g.drum.eqPanelZone != nil {
			// Select the WAV-sample instrument itself as the active channel so
			// the Synth tab renders its "no synth" banner. Master is blocked
			// for the Synth tab, so we can't use "main" here anymore.
			g.drum.eqPanelZone.SetActiveChannel(channel.Of("wav-sample"))
		}
	}
}

// mobileSynthTabNoSynthSetup is the mobile counterpart of
// synthTabNoSynthSceneSetup.
func mobileSynthTabNoSynthSetup() func(*Game) {
	return func(g *Game) {
		g.SetForceMobileProfile(true)
		g.drum.SetMobileEQMode(true)
		synthTabNoSynthSceneSetup()(g)
	}
}

// mobileSynthTabSetup is the mobile counterpart. Forces mobile profile,
// expands the mobile audio panel, then applies the synth-tab setup.
func mobileSynthTabSetup(instrumentOverride instrument.ID) func(*Game) {
	return func(g *Game) {
		g.SetForceMobileProfile(true)
		g.drum.SetMobileEQMode(true)
		synthTabSceneSetup(instrumentOverride)(g)
	}
}

// restoreShippedMenuSceneSetup saves an edit over the shipped kick-snap (into a
// memory-only library, so a capture never writes the developer's
// instruments.json) and opens the Synth ⋯ menu, which then offers Restore
// shipped. Mobile opens it through the sticky bar's ⋯ button, desktop through
// the Synth header's ⋯ button.
func restoreShippedMenuSceneSetup(mobile bool) func(*Game) {
	return func(g *Game) {
		if mobile {
			mobileSynthTabSetup(roster.KickSnap)(g)
		} else {
			synthTabSceneSetup(roster.KickSnap)(g)
		}
		audio.SetInstrumentLibrarySink(nil)
		audio.SetInstrumentParam(roster.KickSnap, "gen1_kick_click", 0.9)
		saveRecipeForInstrument(roster.KickSnap)
		if mobile {
			if acts := g.drum.synthPanelMobileActions(); len(acts) == 3 && acts[2].OnClick != nil {
				acts[2].OnClick()
			}
			return
		}
		// Desktop: press the header's ⋯ button through its own OnClick (the
		// same handler a click reaches).
		for _, b := range g.drum.SynthTabButtons() {
			if b != nil && b.Icon == string(IconOverflow) && b.OnClick != nil {
				b.OnClick()
				return
			}
		}
		panic("scene: desktop Synth header has no ⋯ button")
	}
}

// savedToneReopenedSceneSetup turns the first category dial of the startup
// circuit's row-0 instrument to 90, presses Save (into a memory-only library,
// so a capture never writes the developer's instruments.json), then reopens
// the startup circuit — the user's Save must trump the circuit's document.
func savedToneReopenedSceneSetup(mobile bool) func(*Game) {
	return func(g *Game) {
		open := synthTabSceneSetup("")
		if mobile {
			open = mobileSynthTabSetup("")
		}
		open(g)
		audio.SetInstrumentLibrarySink(nil)
		inst := g.drum.resolveSynthInstrument(g.drum.synthTabActiveInstrument())
		var turned bool
		for _, c := range g.drum.instEditorMacroCards {
			if c.nDials == 0 {
				continue
			}
			d, ok := g.drum.synthDialByID(c.id, c.dials[0].id)
			if !ok {
				continue
			}
			start := map[string]float64{}
			cur := audio.MergedInstrumentParamsRO(inst)
			for _, tm := range d.Terms {
				start[tm.Param] = cur[tm.Param]
			}
			audio.SetInstrumentParamsLive(inst, d.Turn(start, 0.9))
			turned = true
			break
		}
		if !turned {
			panic("scene: the startup circuit's row 0 has no macro dial")
		}
		saveRecipeForInstrument(inst)
		if err := g.Import(assets.StartupDemoJSON); err != nil {
			panic("scene: reopen the startup circuit: " + err.Error())
		}
		open(g)
	}
}

// saveAsPromptSceneSetup opens the Synth tab on the violin (a memory-only
// library, so a capture never writes the developer's instruments.json) and
// presses Save As: the desktop header button's own OnClick, or on mobile the
// ⋯ menu's Save As item (the same handler a tap reaches).
// saveAsPromptSceneSetup opens Save As over the violin; typed ("" = keep the
// prefilled name) is then typed into it.
func saveAsPromptSceneSetup(mobile bool, typed string) func(*Game) {
	return func(g *Game) {
		if mobile {
			mobileSynthTabSetup(roster.Violin)(g)
		} else {
			synthTabSceneSetup(roster.Violin)(g)
		}
		audio.SetInstrumentLibrarySink(nil)
		if mobile {
			g.drum.openSaveAsDialog()
		} else {
			opened := false
			for _, b := range g.drum.SynthTabButtons() {
				if b != nil && b.Text == synthSaveAsButtonTag && b.OnClick != nil {
					b.OnClick()
					opened = true
					break
				}
			}
			if !opened {
				panic("scene: desktop Synth header has no Save As button")
			}
		}
		if dlg := g.drum.SynthSaveAsDialog(); dlg != nil && typed != "" {
			dlg.SetValue(typed)
			dlg.classify()
		}
	}
}

// macroSceneSetup / mobileMacroSceneSetup drive the Synth tab into the beginner
// macro layer for `instrument`, optionally drilled into concept `open` ("" = the
// Basic root of concept cards). Shared by the crop_synth_macro_* scenes so desktop
// and the mobile pass exercise the same state.
// modalSceneSetup shows the WAV-clone stages: tubular bells' project copy
// becomes the generated demo modal document (synth_modal_demo.go), then the
// root (open == "") or a drilled concept.
func modalSceneSetup(mobile bool, open string) func(*Game) {
	return func(g *Game) {
		if mobile {
			mobileSynthTabSetup(roster.TubularBells)(g)
		} else {
			synthTabSceneSetup(roster.TubularBells)(g)
		}
		if err := installDemoModalDoc(roster.TubularBells); err != nil {
			panic("modal scene: " + err.Error())
		}
		settleSynthTab(g)
		applyMacroSceneState(g, open)
	}
}

func macroSceneSetup(instrument instrument.ID, open string) func(*Game) {
	return func(g *Game) {
		synthTabSceneSetup(instrument)(g)
		applyMacroSceneState(g, open)
	}
}

func mobileMacroSceneSetup(instrument instrument.ID, open string) func(*Game) {
	return func(g *Game) {
		mobileSynthTabSetup(instrument)(g)
		applyMacroSceneState(g, open)
	}
}

func applyMacroSceneState(g *Game, open string) {
	if g.drum == nil {
		return
	}
	if open != "" {
		inst := g.drum.resolveSynthInstrument(g.drum.synthTabActiveInstrument())
		g.drum.setSynthMacroOpen(inst, open)
	}
	if g.drum.audioTree != nil {
		g.drum.audioTree.LayoutZoneNow("eq-panel")
	}
	g.drum.settleSynthGraphNow()
}

// synthDialSceneTurns is the "turned" dial scenes' state: card/dial → the
// ABSOLUTE reading (0…100) to set, through the same Turn a drag uses.
var synthDialSceneTurns = map[string]int{
	"brightness/bright": 85, "brightness/edge": 60, "length/length": 30, "length/tail": 20,
}

func dialSceneSetup(instrument instrument.ID, turns map[string]int, open string) func(*Game) {
	return func(g *Game) {
		synthTabSceneSetup(instrument)(g)
		applyDialSceneState(g, turns, open)
	}
}

func mobileDialSceneSetup(instrument instrument.ID, turns map[string]int, open string) func(*Game) {
	return func(g *Game) {
		mobileSynthTabSetup(instrument)(g)
		applyDialSceneState(g, turns, open)
	}
}

// dialFocusSceneSetup opens the Synth root on instrument with card scrolled
// into the root grid's window (untouched values: every dial reads the shipped
// sound).
func dialFocusSceneSetup(instrument instrument.ID, card string, mobile bool) func(*Game) {
	return func(g *Game) {
		if mobile {
			mobileSynthTabSetup(instrument)(g)
		} else {
			synthTabSceneSetup(instrument)(g)
		}
		applyMacroSceneState(g, "")
		dv := g.drum
		if dv == nil {
			return
		}
		for i, cg := range dv.instEditorMacros {
			if cg.ID == card {
				dv.synp().sectionGrid(-1).ScrollToIndex(i)
			}
		}
		applyMacroSceneState(g, "")
	}
}

// applyDialSceneState turns dials and optionally opens one dial's wheel, both
// through the same calls a drag / tap makes.
func applyDialSceneState(g *Game, turns map[string]int, open string) {
	dv := g.drum
	if dv == nil {
		return
	}
	applyMacroSceneState(g, "")
	inst := dv.resolveSynthInstrument(dv.synthTabActiveInstrument())
	keys := make([]string, 0, len(turns))
	for k := range turns {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		card, dial, _ := strings.Cut(k, "/")
		if d, ok := dv.synthDialByID(card, dial); ok {
			dv.setSynthDialReading(inst, d, turns[k])
		}
	}
	// Bring the scene's subject card into the root grid's window: the mobile
	// root shows one card at a time, so without this the turned dials and the
	// tapped one sit below the fold (and a tap on an off-window card is a no-op).
	focus := open
	if focus == "" && len(keys) > 0 {
		focus = keys[0]
	}
	if card, _, _ := strings.Cut(focus, "/"); card != "" {
		for i, cg := range dv.instEditorMacros {
			if cg.ID == card {
				dv.synp().sectionGrid(-1).ScrollToIndex(i)
			}
		}
	}
	applyMacroSceneState(g, "")
	if open != "" {
		card, dial, _ := strings.Cut(open, "/")
		for _, c := range dv.instEditorMacroCards {
			for j := 0; j < c.nDials; j++ {
				if c.id == card && c.dials[j].id == dial {
					dv.openSynthDialWheel(inst, card, dial, c.dialRects[j])
				}
			}
		}
	}
}

// synthStageSceneSetup drills into pipeline stage `section` (what a tap on the
// strip's pill does); mobileSynthStageSceneSetup is the mobile pass.
func synthStageSceneSetup(instrument instrument.ID, section synthSectionID) func(*Game) {
	return func(g *Game) {
		synthTabSceneSetup(instrument)(g)
		applyStageSceneState(g, section)
	}
}

func mobileSynthStageSceneSetup(instrument instrument.ID, section synthSectionID) func(*Game) {
	return func(g *Game) {
		mobileSynthTabSetup(instrument)(g)
		applyStageSceneState(g, section)
	}
}

func applyStageSceneState(g *Game, section synthSectionID) {
	if g.drum == nil {
		return
	}
	inst := g.drum.resolveSynthInstrument(g.drum.synthTabActiveInstrument())
	g.drum.setSynthStageOpen(inst, section)
	if g.drum.audioTree != nil {
		g.drum.audioTree.LayoutZoneNow("eq-panel")
	}
}

// synthMoreSceneSetup drills into `concept` and opens its More expander.
func synthMoreSceneSetup(instrument instrument.ID, concept string) func(*Game) {
	return func(g *Game) {
		macroSceneSetup(instrument, concept)(g)
		applyMoreSceneState(g)
	}
}

// synthMoreScrolledSceneSetup is synthMoreSceneSetup scrolled to the END of the
// drilled grid, so the Advanced cards below the More divider are on screen.
func synthMoreScrolledSceneSetup(instrument instrument.ID, concept string) func(*Game) {
	return func(g *Game) {
		synthMoreSceneSetup(instrument, concept)(g)
		if g.drum == nil {
			return
		}
		g.drum.synp().sectionGrid(-1).ScrollToIndex(1 << 20)
		if g.drum.audioTree != nil {
			g.drum.audioTree.LayoutZoneNow("eq-panel")
		}
	}
}

// withInstrumentParamSceneSetup sets one synth param on inst BEFORE the wrapped
// setup lays the Synth tab out, so a scene can show a user edit's effect.
func withInstrumentParamSceneSetup(inst instrument.ID, param string, value float64, next func(*Game)) func(*Game) {
	return func(g *Game) {
		audio.SetInstrumentParam(inst, param, value)
		next(g)
	}
}

// physicalKickSceneSetup switches the 808 kick to the physical drumhead
// variant (gen1_kick_variant 8) BEFORE the drill, so the variant-only knobs
// (Body Hold, Air) are laid out. No shipped instrument uses variant 8 yet; a
// user reaches it through the Variant control. `focus` selects that knob so
// the signal-graph header shows its plain-English gloss.
func physicalKickSceneSetup(next func(*Game), focus string) func(*Game) {
	return func(g *Game) {
		audio.SetInstrumentParam(roster.Kick808, "gen1_kick_variant", 8)
		next(g)
		if g.drum == nil {
			return
		}
		inst := g.drum.resolveSynthInstrument(g.drum.synthTabActiveInstrument())
		for kIdx, b := range g.drum.instEditorBindings {
			if b.def.Name == focus {
				g.drum.setSynthSelectedControl(inst, kIdx)
				break
			}
		}
		if g.drum.audioTree != nil {
			g.drum.audioTree.LayoutZoneNow("eq-panel")
		}
	}
}

func mobileSynthMoreSceneSetup(instrument instrument.ID, concept string) func(*Game) {
	return func(g *Game) {
		mobileMacroSceneSetup(instrument, concept)(g)
		applyMoreSceneState(g)
	}
}

func applyMoreSceneState(g *Game) {
	if g.drum == nil {
		return
	}
	inst := g.drum.resolveSynthInstrument(g.drum.synthTabActiveInstrument())
	g.drum.setSynthMoreOpen(inst, true)
	if g.drum.audioTree != nil {
		g.drum.audioTree.LayoutZoneNow("eq-panel")
	}
}

// macroArmedSceneSetup drills into `concept` and ARMS its first on-window
// continuous control so the capture shows the grown card + two barrel lanes —
// the edit affordance a tap/click produces on desktop and mobile.
func macroArmedSceneSetup(instrument instrument.ID, concept string) func(*Game) {
	return func(g *Game) {
		macroSceneSetup(instrument, concept)(g)
		armFirstDrilledControl(g, concept)
	}
}

func mobileMacroArmedSceneSetup(instrument instrument.ID, concept string) func(*Game) {
	return func(g *Game) {
		mobileMacroSceneSetup(instrument, concept)(g)
		armFirstDrilledControl(g, concept)
	}
}

func armFirstDrilledControl(g *Game, concept string) {
	if g.drum == nil {
		return
	}
	inst := g.drum.resolveSynthInstrument(g.drum.synthTabActiveInstrument())
	idxs := g.drum.macroConceptKnobIdxs(concept)
	if len(idxs) == 0 {
		return
	}
	// Prefer a continuous control (non-nil step badge) that is on-window (non-empty
	// rect) so both the resolution and value lanes render; fall back to the first.
	idx := idxs[0]
	for _, kIdx := range idxs {
		if kIdx >= 0 && kIdx < len(g.drum.instEditorStepBadges) &&
			g.drum.instEditorStepBadges[kIdx] != nil && !g.drum.instEditorControls[kIdx].Rect().Empty() {
			idx = kIdx
			break
		}
	}
	g.drum.armSynthControl(inst, idx)
	// Settle the grow animation so the capture shows the fully-grown 2×2 block +
	// the reflowed neighbours (the animation advances in synthTabUpdate in the app;
	// scenes drive it directly).
	for i := 0; i < 50 && g.drum.advanceArmGrow(); i++ {
		if g.drum.audioTree != nil {
			g.drum.audioTree.LayoutZoneNow("eq-panel")
		}
	}
	if g.drum.audioTree != nil {
		g.drum.audioTree.LayoutZoneNow("eq-panel")
	}
	g.drum.settleSynthGraphNow()
}

// macroArmingMidSceneSetup captures a MID-animation frame: the armed control is
// partway grown (armGrowT ≈ 0.35) so the neighbours are caught sliding to their
// reflowed slots.
func macroArmingMidSceneSetup(instrument instrument.ID, concept string) func(*Game) {
	return func(g *Game) {
		macroSceneSetup(instrument, concept)(g)
		if g.drum == nil {
			return
		}
		inst := g.drum.resolveSynthInstrument(g.drum.synthTabActiveInstrument())
		idxs := g.drum.macroConceptKnobIdxs(concept)
		if len(idxs) == 0 {
			return
		}
		idx := idxs[0]
		for _, kIdx := range idxs {
			if kIdx >= 0 && kIdx < len(g.drum.instEditorStepBadges) &&
				g.drum.instEditorStepBadges[kIdx] != nil && !g.drum.instEditorControls[kIdx].Rect().Empty() {
				idx = kIdx
				break
			}
		}
		g.drum.armSynthControl(inst, idx)
		// Freeze mid-grow for a deterministic capture: T == target means
		// advanceArmGrow (run by the harness's settle frames) leaves it put.
		g.drum.armGrowT = 0.35
		g.drum.armGrowTarget = 0.35
		if g.drum.audioTree != nil {
			g.drum.audioTree.LayoutZoneNow("eq-panel")
		}
		g.drum.settleSynthGraphNow()
	}
}

// synthTabStageSetup is synthTabSceneSetup + opening a specific stage in the
// chip strip's detail pane. disableParam (optional) toggles a stage off
// first so the scene shows a ghost chip — and, when the disabled stage is
// also the selected one, the bypass scrim. Drives SelectSynthSectionByLabel,
// the same path a real chip tap takes.
func synthTabStageSetup(instrumentOverride instrument.ID, stageLabel, disableParam string) func(*Game) {
	return func(g *Game) {
		synthTabSceneSetup(instrumentOverride)(g)
		if disableParam != "" && len(g.drum.Rows) > 0 {
			inst := g.drum.resolveSynthInstrument(g.drum.Rows[0].Instrument)
			if inst != "" {
				audio.SetInstrumentParam(inst, disableParam, 0)
			}
		}
		// Layout once so the chip strip exists, then select + re-layout. Route
		// the forced layouts through the tree (LayoutZoneNow) — a direct
		// eqPanelZone.Layout bypasses the hit-area republish and trips the
		// chokepoint discipline guard (TestZoneLayoutRoutesThroughTreeDiscipline).
		if g.drum != nil && g.drum.audioTree != nil {
			g.drum.audioTree.LayoutZoneNow("eq-panel")
			g.drum.SelectSynthSectionByLabel(stageLabel)
			g.drum.audioTree.LayoutZoneNow("eq-panel")
		}
		// Land the graph synchronously so it is deterministic at capture time
		// (it is pool-rendered in production).
		if g.drum != nil {
			g.drum.settleSynthGraphNow()
		}
	}
}

// mobileSynthTabStageSetup is the mobile counterpart of synthTabStageSetup.
func mobileSynthTabStageSetup(instrumentOverride instrument.ID, stageLabel, disableParam string) func(*Game) {
	return func(g *Game) {
		g.SetForceMobileProfile(true)
		g.drum.SetMobileEQMode(true)
		synthTabStageSetup(instrumentOverride, stageLabel, disableParam)(g)
	}
}

// synthTabFocusControlSetup opens stageLabel for instrument and selects the knob
// named knobParam so the focus graph shows that knob's property-native domain.
// Builds on synthTabStageSetup (which selects the stage + renders the mirror),
// then walks the selected section's knobs to find the one whose ParamDef.Name
// matches knobParam and records it via setSynthSelectedControl. If the knob name
// isn't present in the stage, the focus graph falls back to the section's main
// knob — still renders, but pick correct names so the intended domain shows.
func synthTabFocusControlSetup(instrumentOverride instrument.ID, stageLabel, knobParam string) func(*Game) {
	return func(g *Game) {
		synthTabStageSetup(instrumentOverride, stageLabel, "")(g)
		if g.drum == nil {
			return
		}
		inst := g.drum.resolveSynthInstrument(g.drum.synthTabActiveInstrument())
		if inst == "" {
			return
		}
		sec := g.drum.synthSelectedSection()
		if sec == nil {
			return
		}
		for _, kIdx := range sec.knobIdxs {
			if kIdx >= 0 && kIdx < len(g.drum.instEditorBindings) &&
				g.drum.instEditorBindings[kIdx].def.Name == knobParam {
				g.drum.setSynthSelectedControl(inst, kIdx)
				break
			}
		}
		g.drum.settleSynthGraphNow()
	}
}

// mobileSynthTabFocusControlSetup is the mobile counterpart of
// synthTabFocusControlSetup — exercises the mobile stacked focus band.
func mobileSynthTabFocusControlSetup(instrumentOverride instrument.ID, stageLabel, knobParam string) func(*Game) {
	return func(g *Game) {
		g.SetForceMobileProfile(true)
		g.drum.SetMobileEQMode(true)
		synthTabFocusControlSetup(instrumentOverride, stageLabel, knobParam)(g)
	}
}

// synthTabFocusControlParamSetup builds on synthTabFocusControlSetup and additionally
// pins one live param (e.g. drive to 70% of its 0..1 range) before the final
// mirror render so the focus graph and the "Your sound" pane reflect the driven
// value, not the recipe default.
func synthTabFocusControlParamSetup(instrumentOverride instrument.ID, stageLabel, knobParam, setParam string, setVal float64) func(*Game) {
	return func(g *Game) {
		synthTabFocusControlSetup(instrumentOverride, stageLabel, knobParam)(g)
		if g.drum == nil {
			return
		}
		inst := g.drum.resolveSynthInstrument(g.drum.synthTabActiveInstrument())
		if inst == "" {
			return
		}
		audio.SetInstrumentParam(inst, setParam, setVal)
		g.drum.settleSynthGraphNow()
	}
}

// synthControlArmedSceneSetup opens the Synth tab on the modular voice and
// arms a control in the currently-selected section so the card expands
// in-place with the kinetic barrel editor. Prefers a knob that carries a
// resolution lane (a non-nil step badge, i.e. a continuous param) over the
// section's first knob so BOTH lanes render — a discrete param would only
// show the value lane.
func synthControlArmedSceneSetup() func(*Game) {
	return func(g *Game) {
		synthTabSceneSetup(roster.Modular)(g)
		if g.drum == nil {
			return
		}
		inst := g.drum.resolveSynthInstrument(g.drum.synthTabActiveInstrument())
		if inst == "" {
			return
		}
		relayout := func() {
			if g.drum.audioTree != nil {
				g.drum.audioTree.LayoutZoneNow("eq-panel")
			}
		}
		// Select the ENVELOPE stage: its essential (on-window) controls are
		// continuous (they carry a resolution ladder), so an armed control there
		// renders BOTH barrels — the left resolution lane and the right value
		// lane. (The default OSC stage's on-window controls are discrete, so
		// arming one would show only the value lane.)
		g.drum.setSelectedSynthSection(inst, synthSectionEnvelope)
		relayout()
		sec := g.drum.synthSelectedSection()
		if sec == nil || len(sec.knobIdxs) == 0 {
			return
		}
		// Arm the first control that is BOTH on-window (non-empty rect, so its
		// grown card actually renders) AND continuous (non-nil step badge, so the
		// resolution lane exists). Fall back to the section's main knob.
		idx := sec.knobIdxs[0]
		for _, kIdx := range sec.knobIdxs {
			if kIdx < 0 || kIdx >= len(g.drum.instEditorStepBadges) {
				continue
			}
			if g.drum.instEditorStepBadges[kIdx] != nil && !g.drum.instEditorControls[kIdx].Rect().Empty() {
				idx = kIdx
				break
			}
		}
		g.drum.armSynthControl(inst, idx)
		// Layout again through the tree so the armed card's grown bounds and
		// lane rects are published before capture.
		relayout()
		g.drum.settleSynthGraphNow()
	}
}

// mobileSynthControlArmedSceneSetup is the mobile counterpart of
// synthControlArmedSceneSetup.
// synthControlArmedLastInRowSceneSetup drills the violin "brightness" macro (its
// controls fill the grid's columns) and ARMS the RIGHTMOST control in the top row
// — the last-column case the in-place-anchor fix targets: before, the grown 2×2
// block dropped to the row below; now the clicked cell stays put as the block's
// right column (block grows LEFT) and the left neighbour reflows down.
func synthControlArmedLastInRowSceneSetup() func(*Game) {
	return func(g *Game) {
		macroSceneSetup(roster.Violin, "brightness")(g)
		armLastInRowDrilledControl(g, "brightness")
	}
}

// synthSnareVoiceTone2ArmedSceneSetup reproduces the reported bug exactly: the
// snare VOICE stage with "Tone2 Freq" (a continuous two-lane control) ARMED. This
// stage renders a single grid row, so the armed 2×2 block's lower cells are
// off-window — the card must still EXPAND (grow to the 2-column, taller footprint
// with both barrel lanes) rather than collapse to a single in-place cell.
func synthSnareVoiceTone2ArmedSceneSetup() func(*Game) {
	return func(g *Game) {
		synthTabStageSceneSetup(roster.Snare, synthSectionVoice)(g)
		if g.drum == nil {
			return
		}
		inst := g.drum.resolveSynthInstrument(g.drum.synthTabActiveInstrument())
		if inst == "" {
			return
		}
		idx := -1
		for i, b := range g.drum.SynthTabBindings() {
			if b.def.Name == "snare_tone2_freq" {
				idx = i
				break
			}
		}
		if idx < 0 {
			return
		}
		g.drum.armSynthControl(inst, idx)
		for i := 0; i < 50 && g.drum.advanceArmGrow(); i++ {
			if g.drum.audioTree != nil {
				g.drum.audioTree.LayoutZoneNow("eq-panel")
			}
		}
		if g.drum.audioTree != nil {
			g.drum.audioTree.LayoutZoneNow("eq-panel")
		}
		g.drum.settleSynthGraphNow()
	}
}

// synthControlArmedBottomRowSceneSetup ARMS the bottom-most visible control of a
// SCROLLING stage (the FM stage of the modular voice, whose op knobs overflow a
// short/narrow panel) — the "Voice Tone2 Freq expands the control above and much
// larger" case: the 2×2 block's lower row is off-window, so the block must grow
// UPWARD (clicked control = block's bottom row) instead of unioning an off-window
// corner into a giant origin-anchored rect. Capture at a narrow window (e.g.
// -win-w 560) so the FM stage scrolls.
func synthControlArmedBottomRowSceneSetup() func(*Game) {
	return func(g *Game) {
		synthTabSceneSetup(roster.Modular)(g)
		if g.drum == nil {
			return
		}
		inst := g.drum.resolveSynthInstrument(g.drum.synthTabActiveInstrument())
		if inst == "" {
			return
		}
		relayout := func() {
			if g.drum.audioTree != nil {
				g.drum.audioTree.LayoutZoneNow("eq-panel")
			}
		}
		g.drum.setSelectedSynthSection(inst, synthSectionFM)
		relayout()
		var sec *synthSection
		for i := range g.drum.instEditorSections {
			if g.drum.instEditorSections[i].id == synthSectionFM {
				sec = &g.drum.instEditorSections[i]
				break
			}
		}
		if sec == nil {
			return
		}
		bottomK, bottomY := -1, -(1 << 30)
		for _, k := range sec.knobIdxs {
			if k < 0 || k >= len(g.drum.instEditorControls) {
				continue
			}
			if r := g.drum.instEditorControls[k].Rect(); !r.Empty() && r.Min.Y > bottomY {
				bottomY, bottomK = r.Min.Y, k
			}
		}
		if bottomK < 0 {
			return
		}
		g.drum.armSynthControl(inst, bottomK)
		for i := 0; i < 50 && g.drum.advanceArmGrow(); i++ {
			relayout()
		}
		relayout()
		g.drum.settleSynthGraphNow()
	}
}

// armLastInRowDrilledControl arms the rightmost on-window control in the top row
// of the drilled concept grid, then settles the grow animation (mirrors
// armFirstDrilledControl's settle loop).
func armLastInRowDrilledControl(g *Game, concept string) {
	if g.drum == nil {
		return
	}
	inst := g.drum.resolveSynthInstrument(g.drum.synthTabActiveInstrument())
	idxs := g.drum.macroConceptKnobIdxs(concept)
	if len(idxs) == 0 {
		return
	}
	minY := 1 << 30
	for _, kIdx := range idxs {
		if kIdx < 0 || kIdx >= len(g.drum.instEditorControls) {
			continue
		}
		if r := g.drum.instEditorControls[kIdx].Rect(); !r.Empty() && r.Min.Y < minY {
			minY = r.Min.Y
		}
	}
	idx, bestX := idxs[0], -(1 << 30)
	for _, kIdx := range idxs {
		if kIdx < 0 || kIdx >= len(g.drum.instEditorControls) {
			continue
		}
		r := g.drum.instEditorControls[kIdx].Rect()
		if r.Empty() || r.Min.Y != minY {
			continue
		}
		if r.Min.X > bestX {
			bestX, idx = r.Min.X, kIdx
		}
	}
	g.drum.armSynthControl(inst, idx)
	for i := 0; i < 50 && g.drum.advanceArmGrow(); i++ {
		if g.drum.audioTree != nil {
			g.drum.audioTree.LayoutZoneNow("eq-panel")
		}
	}
	if g.drum.audioTree != nil {
		g.drum.audioTree.LayoutZoneNow("eq-panel")
	}
	g.drum.settleSynthGraphNow()
}

func mobileSynthControlArmedSceneSetup() func(*Game) {
	return func(g *Game) {
		g.SetForceMobileProfile(true)
		g.drum.SetMobileEQMode(true)
		synthControlArmedSceneSetup()(g)
	}
}

// mobileSynthTabFocusControlParamSetup is the mobile counterpart of
// synthTabFocusControlParamSetup.
func mobileSynthTabFocusControlParamSetup(instrumentOverride instrument.ID, stageLabel, knobParam, setParam string, setVal float64) func(*Game) {
	return func(g *Game) {
		g.SetForceMobileProfile(true)
		g.drum.SetMobileEQMode(true)
		synthTabFocusControlParamSetup(instrumentOverride, stageLabel, knobParam, setParam, setVal)(g)
	}
}

// synthTabDrumRevoicedSetup points row 0 at the snare drum, re-voices it with
// the bare modular voice's document switched to a Saw waveform (osc_type=1),
// and plays — so the capture shows a drum row re-voiced through the full
// modular pipeline.
func synthTabDrumRevoicedSetup() func(*Game) {
	return func(g *Game) {
		synthTabSceneSetup(roster.Snare)(g)
		revoiceWithModularForScene(roster.Snare)
		audio.SetInstrumentParam(roster.Snare, "osc_type", 1) // Saw
		g.SetPlaying(true)
	}
}

// revoiceWithModularForScene gives id a copy of the bare modular voice's
// document (Origin = modular): the drum keeps its row, its sound is the
// modular pipeline.
func revoiceWithModularForScene(id instrument.ID) {
	if d, ok := audio.LibraryDoc(roster.Modular); ok {
		d.Origin = roster.Modular
		_ = audio.SetInstrumentDoc(id, d)
	}
}

func mobileSynthTabDrumRevoicedSetup() func(*Game) {
	return func(g *Game) {
		g.SetForceMobileProfile(true)
		g.drum.SetMobileEQMode(true)
		synthTabDrumRevoicedSetup()(g)
	}
}

// synthTabModularPlaySetup opens the modular voice's Synth tab during playback
// so the capture shows the per-stage enable pills + generator step with the
// live trigger-pulse borders. Mirrors mobileAudioPanelPlaySetup's SetPlaying.
func synthTabModularPlaySetup() func(*Game) {
	return func(g *Game) {
		synthTabSceneSetup(roster.Modular)(g)
		g.SetPlaying(true)
	}
}

func mobileSynthTabModularPlaySetup() func(*Game) {
	return func(g *Game) {
		g.SetForceMobileProfile(true)
		g.drum.SetMobileEQMode(true)
		synthTabModularPlaySetup()(g)
	}
}

// synthTabModularNoiseSetup additionally selects the Noise Pink generator and
// bypasses the Filter stage so the capture exercises the new noise oscillator
// and a dimmed/bypassed section card + preview plot.
func synthTabModularNoiseSetup() func(*Game) {
	return func(g *Game) {
		synthTabSceneSetup(roster.Modular)(g)
		audio.SetInstrumentParam(roster.Modular, "osc_type", 6) // Noise Pink
		audio.SetInstrumentParam(roster.Modular, "filter_enabled", 0)
		g.SetPlaying(true)
	}
}

func mobileSynthTabModularNoiseSetup() func(*Game) {
	return func(g *Game) {
		g.SetForceMobileProfile(true)
		g.drum.SetMobileEQMode(true)
		synthTabModularNoiseSetup()(g)
	}
}

// samplerTabSceneSetup returns a Setup function for the sampler-tab crop
// scenes. It builds the demo synchronously (the scene Setup runs before the
// first Update), activates the Sampler tab, and captures the active
// instrument's one-shot into the working buffer so the waveform card renders
// a real trace instead of the empty placeholder. The optional capture flags
// drive trim + a toggle so the trimmed-region shading and active-toggle
// chrome are exercised by the screenshot.
func samplerTabSceneSetup(captured bool) func(*Game) {
	return func(g *Game) {
		g.buildDemo()
		_ = g.SetActiveEQTab("sampler")
		dv := g.drum
		if dv == nil {
			return
		}
		if dv.eqPanelZone != nil && len(dv.Rows) > 0 {
			// The Sampler tab needs a single-instrument context (Master is now
			// blocked for it); select Rows[0]'s instrument explicitly rather
			// than relying on the old Master→Rows[0] fallback.
			dv.eqPanelZone.SetActiveChannel(channel.Of(dv.Rows[0].Instrument))
		}
		if !captured {
			return
		}
		dv.sampler.captureFromSynth(dv.samplerActiveInstrument())
		// Trim to the middle 70% and enable Normalize so the scene shows the
		// trimmed-away shading, both drag handles, and an active toggle.
		dv.sampler.startFrac = 0.15
		dv.sampler.endFrac = 0.85
		dv.sampler.normalize = true
	}
}

// mobileSamplerTabSetup is the mobile counterpart. Forces mobile profile,
// expands the mobile audio panel, then applies the sampler-tab setup.
func mobileSamplerTabSetup(captured bool) func(*Game) {
	return func(g *Game) {
		g.SetForceMobileProfile(true)
		g.drum.SetMobileEQMode(true)
		samplerTabSceneSetup(captured)(g)
	}
}

// samplerWheelOpenSetup captures a sample, then opens the value-wheel popup on
// one sampler knob — the surface that owns BOTH wheel directions: the value
// barrel (larger values above the center selector) and the resolution ladder
// (coarsest rung at the top). Deliberately NOT subject-cropped: the popup is a
// modal over the whole screen, and the point of the capture is that the two
// stacks read in the same direction.
func samplerWheelOpenSetup(base func(*Game)) func(*Game) {
	return func(g *Game) {
		base(g)
		dv := g.drum
		if dv == nil {
			return
		}
		// The popup anchors on the knob's rect, which only exists once the panel
		// has laid out — mirrors armFirstDrilledControl's LayoutZoneNow priming.
		if dv.audioTree != nil {
			dv.audioTree.LayoutZoneNow("eq-panel")
		}
		dv.openSamplerKnobWheelPopup(0)
	}
}

// samplerWheelKeyStepSetup opens the same popup and then applies the steps the
// ARROW KEYS apply (StepValue / StepResolution — the exact methods
// MobileWheelPopup.HandleClaimedKeys calls), so the capture shows that keyboard
// editing actually moves what is rendered: the value barrel off its opening row
// and the resolution ladder onto a coarser rung. The key ROUTING that gets those
// methods called instead of panning the grid is pinned in Go by
// popup_control_keys_test.go.
func samplerWheelKeyStepSetup(base func(*Game)) func(*Game) {
	open := samplerWheelOpenSetup(base)
	return func(g *Game) {
		open(g)
		w := g.drum.samplerWheelPopup
		if w == nil || !w.IsOpen() {
			return
		}
		for i := 0; i < 3; i++ {
			w.StepValue(+1)
		}
		w.StepResolution(+1)
	}
}

// mobileAudioPanelSetup returns a Setup that forces mobile profile,
// expands the mobile audio (EQ/Wave/Spec/Meters/Scope) panel, and sets
// the requested tab. Used as the MobileSetup for desktop EQ scenes so
// their mobile capture exercises a real route to the panel.
func mobileAudioPanelSetup(tab string) func(*Game) {
	return func(g *Game) {
		g.SetForceMobileProfile(true)
		g.drum.SetMobileEQMode(true)
		_ = g.SetActiveEQTab(tab)
	}
}

// blockedWarmBackend reports nothing cached and never completes a warm, so a
// screenshot scene can hold the readiness gate open (Play button disabled)
// and the progress toast frozen at a stable frame — see
// newBlockedWarmBackendForScene / toastCircuitLoadingSceneSetup.
type blockedWarmBackend struct{}

func (blockedWarmBackend) Cached(instrument.ID, float64) bool  { return false }
func (blockedWarmBackend) Warm(instrument.ID, float64, func()) {}

func newBlockedWarmBackendForScene() warmBackend { return blockedWarmBackend{} }

// toastCircuitLoadingSceneSetup drives the circuit-loading progress toast
// into a stable mid-progress frame for both the full-screen and cropped
// scenes. The demo circuit is built synchronously first (buildWarmSet reads
// beatInfosByRow, which buildDemo populates via updateBeatInfos — Setup runs
// before the first Update(), so without this the warm set would be empty).
// The readiness gate is then armed against a backend that never completes,
// so Play stays visibly disabled for the whole capture window. Finally the
// toast's own done/total are pinned to 42/100 directly — independent of
// whatever the demo's actual warm-set size turns out to be — so the capture
// reads a deterministic "42/100" regardless of which built-in demo loads.
func toastCircuitLoadingSceneSetup(g *Game) {
	g.buildDemo()
	g.warmup = newWarmupController(g, newBlockedWarmBackendForScene())
	g.wireWarmupNotifications()
	g.warmup.Begin()
	g.warmup.onUpdate = nil // freeze the bar mid-progress for a stable capture
	g.drum.logsToast.beginTask(i18n.KeyNotifCircuitLoading, 100)
	g.drum.logsToast.updateTask(42)
}

// toastCircuitLoadingMobileSceneSetup is the MobileSetup for
// toast_circuit_loading: put the bottom nav on an audio tab (NOT Pads) so
// the grid pane is offscreen and the DrumToastLayer fallback
// (layer_drum_toast.go) is the ONLY thing that can show the toast — proves
// the mobile placement rule Task 9 added — then apply the same shared
// warm-up + toast state as the desktop scene.
func toastCircuitLoadingMobileSceneSetup(g *Game) {
	g.SetForceMobileProfile(true)
	g.drum.SetMobileEQMode(true)
	_ = g.SetActiveEQTab("eq")
	toastCircuitLoadingSceneSetup(g)
}

// mobileAudioPanelPlaySetup is mobileAudioPanelSetup + SetPlaying(true),
// for crop_* scenes that need both the panel exposed AND playback running
// so the analyzer ring buffers fill before the screenshot fires.
func mobileAudioPanelPlaySetup(tab string) func(*Game) {
	return func(g *Game) {
		g.SetForceMobileProfile(true)
		g.drum.SetMobileEQMode(true)
		_ = g.SetActiveEQTab(tab)
		g.SetPlaying(true)
	}
}

// ensureMultiRow guarantees the rack has at least n distinct rows for the
// multi-row scenes. It builds the demo synchronously first: buildDemo is
// otherwise deferred to the first Update(), and if it ran AFTER the scene
// appended rows it would import-replace them, collapsing the rack back to
// the demo's row count (the "shows one row" capture bug). Building it here
// flips demoBuilt so the subsequent settle Updates leave the added rows
// intact.
func ensureMultiRow(g *Game, n int) {
	if g.drum == nil {
		return
	}
	g.buildDemo()
	for len(g.drum.Rows) < n {
		g.drum.AddRow()
	}
}

// sceneRowRackStates seeds seven rows covering every rack-control state and
// gives the drum pane more height than the rows need, so the add-row slot and
// any background lane around the rack are visible.
func sceneRowRackStates(g *Game) {
	ensureMultiRow(g, 7)
	if g.drum == nil || len(g.drum.Rows) < 7 {
		return
	}
	g.drum.Rows[3].Muted = true
	g.drum.Rows[4].Solo = true
	g.drum.Rows[5].Volume = 0.35
	g.SetSplitterFrac(0.2)
	g.drum.markRowControlsDirty()
}

// ensureRow guarantees there is a row at index idx, adding rows as needed.
func ensureRow(g *Game, idx int) {
	if g.drum == nil {
		return
	}
	for len(g.drum.Rows) <= idx {
		g.drum.AddRow()
	}
}

// openEmptyFavoritesView drives the instrument menu into the state that
// exposed the stale-star bug: browse a populated category (one star laid out
// per row), pop back to the category list, then open Favorites with an empty
// ★ store so the list collapses to the onboarding placeholder. The stars from
// the category listing must not survive into this frame.
//
// Note it swaps the process-global FavoritesStore for an empty one — the menu
// reads props.Favorites from Favorites(), so a dev with saved ★s would
// otherwise capture a populated list. Equivalent to the package default
// (an empty InMemoryFavoritesStore); tests that need their own store install
// it with a t.Cleanup restore.
func openEmptyFavoritesView(g *Game) {
	ensureRow(g, 0)
	if g.drum == nil {
		return
	}
	SetFavoritesStore(NewInMemoryFavoritesStore())
	g.drum.instMenuForceCategories = true
	g.drum.OpenInstrumentMenu(0)
	comp := g.drum.instMenuComp
	if comp == nil {
		return
	}
	if cats := comp.CategoryBtns(); len(cats) > 1 && cats[1].OnClick != nil {
		cats[1].OnClick()
	}
	if b := comp.BackBtn(); b != nil && b.OnClick != nil {
		b.OnClick()
	}
	if cats := comp.CategoryBtns(); len(cats) > 0 && cats[0].OnClick != nil {
		cats[0].OnClick()
	}
}

// sceneSetBPM updates the BPM through the same path the BPM box uses.
func sceneSetBPM(g *Game, bpm int) {
	if g.drum == nil {
		return
	}
	g.drum.SetBPM(bpm)
}

// sceneDragMenuGrip drives a grid-pane menu card through a grip drag by delta,
// using the same PopupDragHandle entry points the real press/move/release path
// uses — so the captured scene reflects genuine drag state (pinned placement,
// dragging tint) rather than a hand-set origin. Mirrors the ScopePanelZone
// SetFrozen/SetAutoGain scene-setter pattern.
//
// Pick delta with the card's clamped edges in mind: both cards can open already
// welded to a pane edge, and a delta pushing further into that edge nets zero
// movement — the capture would then show an undragged card and silently prove
// nothing.
func sceneDragMenuGrip(g *Game, panel image.Rectangle, h *PopupDragHandle, delta image.Point) {
	if panel.Empty() || h == nil {
		return
	}
	grip := h.GripRect(panel)
	from := image.Pt(grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2)
	pane := image.Rect(0, 0, g.split.GridW(g.winW), g.split.GridH(g.winH))
	h.Begin(from, panel)
	h.MoveTo(from.Add(delta), pane, g.gridTopButtonsReservedRect(), image.Pt(panel.Dx(), panel.Dy()))
	h.End()
}

// sceneGroupMenuSetup builds three adjacent connected nodes, groups them the
// way a marquee release does (provisional group + one pitch rule), and opens
// the GroupMenu beside the group — the exact post-marquee UI including the
// Save button and member rings.
func sceneGroupMenuSetup(g *Game) {
	a := g.tryAddNode(2, 1, model.NodeTypeRegular)
	b := g.tryAddNode(3, 1, model.NodeTypeRegular)
	c := g.tryAddNode(4, 1, model.NodeTypeRegular)
	if a == nil || b == nil || c == nil || g.groupMenu == nil {
		return
	}
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.updateBeatInfos()
	// A committed group elsewhere gives the provisional menu one merge-chip
	// candidate, so the capture shows the "Add to group" section too.
	if d := g.tryAddNode(8, 6, model.NodeTypeRegular); d != nil {
		_, _ = g.graph.CreateGroup("Drums", []model.NodeID{d.ID})
	}
	gid, err := g.graph.CreateGroup("", []model.NodeID{a.ID, b.ID, c.ID})
	if err != nil {
		return
	}
	_ = g.graph.SetGroupRules(gid, []model.GroupRule{{Param: model.GroupParamPitch, Delta: 2, EveryN: 4}})
	_, sy1, sx2, _ := g.nodeScreenRect(c)
	g.groupMenu.OpenAtProvisional(gid, int(sx2)+24, int(sy1))
}

// sceneGroupBadgesSetup builds two spatially-separated committed groups
// ("Alpha" {a,b} near the top, "Beta" {c,d} several rows below) so their
// floating badges (task-group-badges) render without overlapping, then opens
// the GroupMenu for the second group so the capture shows: both badges (each
// tinted with its own group's color), the open menu's tinted chrome, and both
// ring colors (Alpha faint, Beta strong on its own members).
func sceneGroupBadgesSetup(g *Game) {
	a := g.tryAddNode(2, 1, model.NodeTypeRegular)
	b := g.tryAddNode(3, 1, model.NodeTypeRegular)
	c := g.tryAddNode(2, 4, model.NodeTypeRegular)
	d := g.tryAddNode(3, 4, model.NodeTypeRegular)
	if a == nil || b == nil || c == nil || d == nil || g.groupMenu == nil {
		return
	}
	g.addEdge(a, b)
	g.addEdge(c, d)
	g.updateBeatInfos()
	if _, err := g.graph.CreateGroup("Alpha", []model.NodeID{a.ID, b.ID}); err != nil {
		return
	}
	gid2, err := g.graph.CreateGroup("Beta", []model.NodeID{c.ID, d.ID})
	if err != nil {
		return
	}
	_, sy1, sx2, _ := g.nodeScreenRect(d)
	g.groupMenu.OpenAt(gid2, int(sx2)+24, int(sy1))
}

// sceneNodeSidebarGroupsSetup builds a→b→c with node b in two committed
// groups ("Drums" {a,b}, "Melody" {b,c}) — 2 full-width open-only chip
// rows — plus a third group ("Extra" {a} only) so b also has exactly one
// add-candidate and the "Add to group" row shows, alongside the "Remove
// from group" row (b has >=1 membership). Both dropdowns are left closed
// (portal/dropdown overlay screenshots are flaky per CLAUDE.md); the closed
// grpadd/grpremove rows are enough for visual verification.
func sceneNodeSidebarGroupsSetup(g *Game) {
	a := g.tryAddNode(2, 1, model.NodeTypeRegular)
	b := g.tryAddNode(3, 1, model.NodeTypeRegular)
	c := g.tryAddNode(4, 1, model.NodeTypeRegular)
	if a == nil || b == nil || c == nil || g.sidebar == nil {
		return
	}
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.updateBeatInfos()
	if _, err := g.graph.CreateGroup("Drums", []model.NodeID{a.ID, b.ID}); err != nil {
		return
	}
	if _, err := g.graph.CreateGroup("Melody", []model.NodeID{b.ID, c.ID}); err != nil {
		return
	}
	if _, err := g.graph.CreateGroup("Extra", []model.NodeID{a.ID}); err != nil {
		return
	}
	g.sidebar.Open(b)
	g.sidebar.sectionOpen[secGrp] = true
	// The Group section is the sidebar's last section; on the short desktop
	// grid pane its rows sit below the scroll fold. Scroll to the bottom so
	// the capture shows the chips/remove/add controls (layout clamps First
	// to the real max).
	g.sidebar.scroll.VS.First = 1 << 20
}

// scrolledTemplateMenuSetup opens the template page and wheel-scrolls it well
// into the list so both culling edges are on screen.
// longTitleTemplateMenuSetup scrolls far enough to reach the Lied · Song and
// French · Americas groups, where the longest titles live.
func longTitleTemplateMenuSetup(mobile bool) func(*Game) {
	return func(g *Game) {
		if mobile {
			g.SetForceMobileProfile(true)
		}
		g.drum.overflowPage = 1
		g.drum.OpenOverflowMenu()
		g.drum.configureOverflowScroll()
		if sc := g.drum.overflowMenuScroll; sc != nil {
			sc.HandleWheel(-22)
		}
	}
}

// loadTemplateScene imports a built-in template exactly as the template menu
// does (the same embedded bytes through Game.Import).
func loadTemplateScene(stem string, mobile bool) func(*Game) {
	return func(g *Game) {
		if mobile {
			g.SetForceMobileProfile(true)
		}
		for _, tp := range assets.Templates() {
			if tp.Genre == stem {
				g.drum.templateLoadedStem = tp.Genre
				if err := g.Import(tp.Bytes); err != nil {
					panic(fmt.Sprintf("scene: import template %s: %v", stem, err))
				}
				return
			}
		}
		panic("scene: no template " + stem)
	}
}

// pitchedNode places a node on row 0 and points that row at a melodic
// instrument, so nodeNoteLabel can resolve node → row → instrument and yield a
// real note. organ-church is a modular melodic instrument present in the
// default demo, so this needs no synthetic recipe binding.
func pitchedNode(g *Game, i, j int) *uiNode {
	return pitchedNodeWithInstrument(g, i, j, roster.OrganChurch)
}

// pitchedNodeWithInstrument is pitchedNode for an explicit instrument, so the
// drum scenes can prove that percussion names a note too — the whole point of
// making note display universal. A kick is the sharpest subject: its
// fundamental is 55 Hz, so it exercises the octave range below A0 where the
// old piano-range clamp used to collapse every note to "A0".
func pitchedNodeWithInstrument(g *Game, i, j int, instrumentID instrument.ID) *uiNode {
	n := g.tryAddNode(i, j, model.NodeTypeRegular)
	g.updateBeatInfos()
	if n == nil || len(g.drum.Rows) == 0 {
		return nil
	}
	// Route through SetInstrument rather than assigning Rows[0].Instrument:
	// the setter also refreshes the row's display name and runs the sync
	// notifier that dependents key off. Writing the field directly left the
	// sidebar header still reading "Dnb Kick" while the note chip already
	// showed the organ's pitch — exactly the stale-dependent split CLAUDE.md
	// warns about for this field.
	prev := g.drum.selRow
	g.drum.selRow = 0
	g.drum.SetInstrument(instrumentID)
	g.drum.selRow = prev
	// Map the node onto row 0 directly. The node is off-circuit, so the real
	// path walk would not assign it a row and nodeNoteLabel would return "";
	// this is a fixture shortcut that exercises the identical render path a
	// genuinely on-circuit node takes, without building a whole loop here.
	// Same shortcut noteTestNode uses in the unit tests.
	g.nodeRows[n.ID] = 0
	return n
}

func pitchedNodeBadgeSetup(mobile bool) func(*Game) {
	return pitchedNodeBadgeSetupFor(mobile, roster.OrganChurch)
}

// pitchedNodeBadgeSetupFor is pitchedNodeBadgeSetup for an explicit instrument.
func pitchedNodeBadgeSetupFor(mobile bool, instrumentID instrument.ID) func(*Game) {
	return func(g *Game) {
		if mobile {
			g.SetForceMobileProfile(true)
		}
		_ = g.SetSubdivisions(8)
		n := pitchedNodeWithInstrument(g, 4, 4, instrumentID)
		if n == nil {
			return
		}
		g.sel = n
		n.Selected = true
		g.coordBadgeNode = n
		g.coordBadgeFrame = g.frame
		// drawGridCoordBadge gates the badge on OPPOSITE sidebar states per
		// platform: desktop shows it only while the node's sidebar is open,
		// mobile only while it is closed (there the sidebar sheet would cover
		// it). So the desktop capture has to open the sidebar to see anything.
		if !mobile && g.sidebar != nil {
			g.sidebar.Open(n)
			g.sidebar.layout()
			// The sidebar is node-anchored, so with it open it lands directly
			// over the badge — correct z-order (grid_coord_badge_zorder_test
			// pins the sidebar painting above it) but a capture showing only
			// the panel. Drag the panel clear so the badge is actually visible.
			sceneDragMenuGrip(g, g.sidebar.rects[sbPanel], &g.sidebar.drag, image.Pt(150, 120))
			g.sidebar.layout()
		}
	}
}

// noteMatrixAfterPickSetup opens the picker and applies ONE note, so the
// capture shows the post-pick state: still open, highlight moved.
func noteMatrixAfterPickSetup(mobile bool) func(*Game) {
	return func(g *Game) {
		noteMatrixSetup(mobile)(g)
		sb := g.sidebar
		if sb == nil || !sb.noteMatrixIsOpen() {
			return
		}
		// Apply a note two semitones up from the current one.
		if mn, ok := g.graph.GetNodeByID(sb.node.ID); ok {
			sb.applyNoteMatrixPitch(mn.Params.Pitch + 2)
		}
	}
}

// noteMatrixKeyCursorSetup opens the picker and walks the keyboard cursor a
// few cells off the current note, so the capture shows the focus ring and the
// selected-note ring at once.
// instMenuKeyCursorSetup opens the instrument menu and parks the keyboard
// cursor on a row that is NOT the applied instrument, which is the state that
// makes the two highlights distinguishable: the applied row keeps its active
// treatment while the cursor row carries the focus ring.
func instMenuKeyCursorSetup(mobile bool) func(*Game) {
	return func(g *Game) {
		if mobile {
			g.SetForceMobileProfile(true)
		}
		ensureRow(g, 0)
		g.drum.OpenInstrumentMenu(0)
		c := g.drum.instMenuComp
		if c == nil || !c.IsOpen() {
			return
		}
		// The row list is built by rebuildMenu, which needs the laid-out anchor,
		// so it is still EMPTY at this point. Drive real frames first or
		// SetSelected has nothing to select and the scene captures no ring.
		for i := 0; i < 4 && len(c.FilteredInstIDsForTest()) == 0; i++ {
			_ = g.Update()
		}
		nav := c.MenuKeyNavForTest()
		nav.BindHost(c)
		// Place the cursor deterministically rather than synthesising key edges:
		// the scene only needs the resulting visual state.
		//
		// It must land NEXT TO the applied instrument, not at row 0: the menu
		// auto-scrolls to reveal the current instrument on open, so an absolute
		// index near the top is scrolled out of the viewport and draws no ring
		// at all. Adjacent also puts both treatments — the active-row highlight
		// and the focus ring — on screen together, which is the whole point of
		// the capture.
		ids := c.FilteredInstIDsForTest()
		applied := c.ActiveInstrumentID()
		at := -1
		for i, id := range ids {
			if id == applied {
				at = i
				break
			}
		}
		switch {
		case at < 0 && len(ids) > 0:
			nav.SetSelected(0)
		case at+1 < len(ids):
			nav.SetSelected(at + 1)
		case at > 0:
			nav.SetSelected(at - 1)
		}
		// SetSelected does not scroll (it is the hover entry point), and the
		// menu opens auto-scrolled to the current instrument — so the row next
		// to it can still be one past the visible page. Reveal brings the cursor
		// onto the page and the rebuild refreshes the rows the ring is drawn
		// against, exactly as the live keyboard path does — including the
		// userScrolled flag, without which rebuildMenu re-biases the scroll
		// straight back onto the current instrument.
		nav.Reveal(c)
		c.state.userScrolled = true
		c.rebuildMenu()
	}
}

func noteMatrixKeyCursorSetup(mobile bool) func(*Game) {
	return func(g *Game) {
		noteMatrixSetup(mobile)(g)
		if g.sidebar == nil || !g.sidebar.noteMatrixIsOpen() {
			return
		}
		nav := g.sidebar.noteMatrixNav()
		host := noteMatrixNavHost{sb: g.sidebar}
		// Place the cursor deterministically rather than synthesising key
		// edges: the scene only needs the resulting visual state.
		for i := 0; i < host.MenuItemCount(); i++ {
			if host.MenuItemSelectable(i) {
				nav.SetSelected(i)
				break
			}
		}
	}
}

// sceneMakeChord turns n into a chord node (Cmaj7, first inversion).
func sceneMakeChord(g *Game, n *uiNode) {
	g.setNodeType(n.ID, model.NodeTypeChord)
	g.setNodeChord(n.ID, model.ChordSpec{Quality: "maj7", Inversion: 1})
}

// sceneCircuitNode is a non-origin node on row 0's circuit (the demo's
// pitched Organ Church row), so the node keeps its row — and its notes —
// across the type change's updateBeatInfos. (The pitched-node helpers fake
// an off-circuit node's row, which a structural rebuild drops.)
func sceneCircuitNode(g *Game) *uiNode {
	if len(g.drum.Rows) == 0 {
		return nil
	}
	var best *uiNode
	for _, n := range g.nodes {
		if row, ok := g.nodeRows[n.ID]; ok && row == 0 && !n.Start {
			if best == nil || n.I > best.I || (n.I == best.I && n.J < best.J) {
				best = n
			}
		}
	}
	return best
}

// chordNodeSidebarSetup opens the node sidebar on a chord node.
func chordNodeSidebarSetup(mobile bool) func(*Game) {
	return func(g *Game) {
		if mobile {
			g.SetForceMobileProfile(true)
		}
		n := sceneCircuitNode(g)
		if n == nil || g.sidebar == nil {
			return
		}
		sceneMakeChord(g, n)
		g.sidebar.Open(n)
		g.sidebar.ExpandAllSections()
		g.sidebar.layout()
	}
}

// nodeTypeRotateToChordSetup opens a regular node's card with only Audible
// expanded, then presses the type button once (Regular → Chord) through the
// same keepUnderPointer path the button's click runs.
func nodeTypeRotateToChordSetup(mobile bool) func(*Game) {
	return func(g *Game) {
		if mobile {
			g.SetForceMobileProfile(true)
		}
		n := sceneCircuitNode(g)
		if n == nil || g.sidebar == nil {
			return
		}
		g.sidebar.Open(n)
		g.sidebar.sectionOpen[secAud] = true
		g.sidebar.layout()
		g.sidebar.keepUnderPointer(sbAudible, func() { g.cycleNodeType(n.ID) })
	}
}

// chordMatrixSetup opens the chord matrix on page `page`.
func chordMatrixSetup(mobile bool, page int) func(*Game) {
	return func(g *Game) {
		chordNodeSidebarSetup(mobile)(g)
		if g.sidebar == nil || !g.sidebar.IsOpen() {
			return
		}
		g.sidebar.openChordMatrix(sbChordName)
		if g.sidebar.chordMatrixIsOpen() {
			g.sidebar.chordMatrix.SetPage(page)
			g.sidebar.layout()
		}
	}
}

// chordNodeBadgeSetup selects a chord node so the coordinate badge shows
// (desktop: with the sidebar open and dragged clear; mobile: sidebar closed).
func chordNodeBadgeSetup(mobile bool) func(*Game) {
	return func(g *Game) {
		if mobile {
			g.SetForceMobileProfile(true)
		}
		n := sceneCircuitNode(g)
		if n == nil {
			return
		}
		sceneMakeChord(g, n)
		g.sel = n
		n.Selected = true
		g.coordBadgeNode = n
		g.coordBadgeFrame = g.frame
		if !mobile && g.sidebar != nil {
			g.sidebar.Open(n)
			g.sidebar.layout()
			sceneDragMenuGrip(g, g.sidebar.rects[sbPanel], &g.sidebar.drag, image.Pt(150, 120))
		}
	}
}

// noteMatrixSetup opens the node sidebar on a pitched node and then opens the
// note-matrix picker from its Pitch-section chip.
func noteMatrixSetup(mobile bool) func(*Game) {
	return noteMatrixSetupFor(mobile, roster.OrganChurch)
}

// noteMatrixSetupFor opens the picker on a row carrying instrumentID.
func noteMatrixSetupFor(mobile bool, instrumentID instrument.ID) func(*Game) {
	return func(g *Game) {
		pitchedNodeSidebarSetupFor(mobile, instrumentID)(g)
		if g.sidebar == nil || !g.sidebar.IsOpen() {
			return
		}
		g.sidebar.layout()
		g.sidebar.openNoteMatrix()
	}
}

func pitchedNodeSidebarSetup(mobile bool) func(*Game) {
	return pitchedNodeSidebarSetupFor(mobile, roster.OrganChurch)
}

// pitchedNodeSidebarSetupFor is pitchedNodeSidebarSetup for an explicit
// instrument.
func pitchedNodeSidebarSetupFor(mobile bool, instrumentID instrument.ID) func(*Game) {
	return func(g *Game) {
		if mobile {
			g.SetForceMobileProfile(true)
		}
		n := pitchedNodeWithInstrument(g, 3, 1, instrumentID)
		if n == nil || g.sidebar == nil {
			return
		}
		g.sidebar.Open(n)
		g.sidebar.ExpandAllSections()
		g.sidebar.layout()
	}
}

func scrolledTemplateMenuSetup(mobile bool) func(*Game) {
	return func(g *Game) {
		if mobile {
			g.SetForceMobileProfile(true)
		}
		g.drum.overflowPage = 1
		g.drum.OpenOverflowMenu()
		g.drum.configureOverflowScroll()
		if sc := g.drum.overflowMenuScroll; sc != nil {
			sc.HandleWheel(-10)
		}
	}
}

// keyNavTemplateMenuSetup opens the template menu and parks the shared keyboard
// cursor on a template row, so the screenshot shows the arrow-key highlight.
func keyNavTemplateMenuSetup(mobile bool) func(*Game) {
	return func(g *Game) {
		if mobile {
			g.SetForceMobileProfile(true)
		}
		g.drum.overflowPage = 1
		g.drum.OpenOverflowMenu()
		g.drum.configureOverflowScroll()
		nav := g.drum.OverflowKeyNavForTest()
		nav.BindHost(overflowNavHost{dv: g.drum})
		// Step down to a real template row (past the pinned title/search/Back
		// rows and the first group header).
		items := g.drum.overflowItems()
		picked := 0
		for i := range items {
			if !items[i].header && !items[i].search && items[i].iconID == IconTemplate {
				picked++
				// Second template row, so the highlight is clearly distinct from
				// the "Back" row above the first group header.
				if picked == 2 {
					nav.SetSelected(i)
					break
				}
			}
		}
	}
}

// sceneAtDraw runs fn on the first Update once the screenshot harness has
// counted n draws — the same counter that times the capture, so a step can be
// placed a fixed number of frames before the shot. Updates outpace draws under
// xvfb, so an Update count alone drifts. Outside screenshot mode no draws are
// counted; fn then fires after 4n updates.
func sceneAtDraw(g *Game, n int, fn func(*Game)) {
	var updates int
	var poll func(*Game)
	poll = func(g *Game) {
		updates++
		if g.screenshotDraws >= n || (g.screenshotPath == "" && updates >= 4*n) {
			fn(g)
			return
		}
		g.QueueAction(poll)
	}
	g.QueueAction(poll)
}

// sceneSoloThenDeletePastImmutable plays the circuit, solos row 0, keeps
// playing, then — shortly before the capture — deletes a corner node on the
// solo-gated row 1. That breaks the row's loop, so everything right of the
// playhead empties; the hits left of it already played and must stay.
func sceneSoloThenDeletePastImmutable(g *Game) {
	g.SetPlaying(true)
	sceneAtDraw(g, 100, func(g *Game) {
		if len(g.drum.Rows) < 2 {
			return
		}
		g.drum.toggleSolo(0)
	})
	// SettleFrames is 300: delete ~15 draws before the capture so the
	// pre-delete history is still inside the visible window.
	sceneAtDraw(g, 285, func(g *Game) {
		if victim := sceneLoopBreakingNode(g, 1); victim != nil {
			g.deleteNode(victim)
		}
	})
}

// sceneLoopBreakingNode returns a node on row's beat path (not its origin)
// whose predecessor and successor are NOT on a shared grid line, so deleting
// it cannot reconnect them and the row's loop breaks.
func sceneLoopBreakingNode(g *Game, row int) *uiNode {
	if row < 0 || row >= len(g.beatInfosByRow) || row >= len(g.drum.Rows) {
		return nil
	}
	origin := g.drum.Rows[row].Origin
	seen := map[model.NodeID]bool{}
	for _, bi := range g.beatInfosByRow[row] {
		id := bi.NodeID
		if bi.NodeType != model.NodeTypeRegular || id == origin || seen[id] {
			continue
		}
		seen[id] = true
		bypassable := false
		for e := range g.graph.Edges {
			if e[1] != id {
				continue
			}
			for f := range g.graph.Edges {
				if f[0] != id {
					continue
				}
				p, s := g.nodesByID[e[0]], g.nodesByID[f[1]]
				if p != nil && s != nil && p.ID != s.ID && (p.I == s.I || p.J == s.J) {
					bypassable = true
				}
			}
		}
		if !bypassable {
			return g.nodesByID[id]
		}
	}
	return nil
}

// sceneGotoWheel parks the playhead mid-beat (step 30 = Beat 4 · 3/4), opens
// the navigation menu and scrolls its go-to wheel `detents` detents up at rung
// `rung` (-1 = default) so the pending position shows its timeline ghost;
// timeMode switches the wheel to M:SS first. The seek is a request applied on
// the next frame, so the wheel is seated on step 30 explicitly.
func sceneGotoWheel(g *Game, timeMode bool) { sceneGotoWheelAt(g, timeMode, -1, 4) }

func sceneGotoWheelAt(g *Game, timeMode bool, rung, detents int) {
	g.drum.RequestSeekSteps(30, hooks.SeekCounter)
	g.drum.OpenNavigation()
	p := g.drum.navPanel
	if p == nil {
		return
	}
	p.seatAt(30, p.div(), p.maxSteps())
	if timeMode {
		p.SetTimeMode(true)
	}
	if rung >= 0 {
		p.GotoWheel().model().SetLadderIndex(rung)
	}
	for i := 0; i < detents; i++ {
		p.GotoWheel().StepValue(+1)
	}
}

// synthViewSceneSetup opens the Synth root on inst and focuses card/dial
// through the same call a dial press makes (which snapshots the BEFORE
// ghost); turn >= 0 then turns the dial there through the drag's own Turn and
// commit, so the graph shows NOW against BEFORE. card == "" leaves nothing
// focused (the overview). Every render is settled before capture.
func synthViewSceneSetup(inst instrument.ID, card, dial string, turn int, mobile bool) func(*Game) {
	return func(g *Game) {
		if mobile {
			mobileSynthTabSetup(inst)(g)
		} else {
			synthTabSceneSetup(inst)(g)
		}
		dv := g.drum
		if dv == nil {
			return
		}
		applyMacroSceneState(g, "")
		if card == "" {
			return
		}
		resolved := dv.resolveSynthInstrument(dv.synthTabActiveInstrument())
		dv.setSynthFocusDial(resolved, card, dial)
		if turn >= 0 {
			if d, ok := dv.synthDialByID(card, dial); ok {
				gs := newSynthDialGesture(resolved, card, d)
				dv.turnSynthDial(gs, turn)
				dv.endSynthDialGesture(gs, true)
			}
		}
		applyMacroSceneState(g, "")
	}
}

// synthGlowSceneSetup is the pitch-drop scene frozen 60 ms after a trigger
// (an injected clock), so the capture shows the glow at full strength.
func synthGlowSceneSetup(mobile bool) func(*Game) {
	return func(g *Game) {
		synthViewSceneSetup(roster.Kick808, "pitchdrop", "drop", -1, mobile)(g)
		if g.drum == nil {
			return
		}
		frozen := time.Unix(1_000_000, 0)
		sg := g.drum.graphGlow()
		sg.now = func() time.Time { return frozen }
		sg.start()
		sg.glowStart = frozen.Add(-60 * time.Millisecond)
	}
}

// sceneInstrumentEQ opens the EQ tab on row 0's instrument and gives that
// instrument two band moves and a high-pass.
func sceneInstrumentEQ(g *Game) {
	_ = g.SetActiveEQTab("eq")
	id := g.drum.Rows[0].Instrument
	g.drum.setEQActiveChannel(channel.Of(id))
	eq := audio.EQSettingsOf(id)
	eq.GainsDB[2], eq.GainsDB[6] = -6, 5
	eq.HPF, eq.HPFHz = true, 180
	audio.ApplyEQSettings(id)
	g.drum.setEQActiveChannel(channel.Of(id)) // reload the panel from the instrument
}

// scenePickQuestion gives row 0's instrument an EQ and saves a version of
// another instrument with a different EQ (memory-only library), then picks
// that instrument for row 0 — or applies a kit holding it.
func scenePickQuestion(g *Game, kit bool) {
	audio.SetInstrumentLibrarySink(nil)
	row := g.drum.Rows[0].Instrument
	audio.EQSettingsOf(row).GainsDB[2] = -6
	audio.ApplyEQSettings(row)
	target := roster.KickDeep
	if row == target {
		target = roster.Kick
	}
	d, _ := audio.ShippedDoc(target)
	eq := audio.NewEQSettings()
	eq.GainsDB[8] = 5
	_ = audio.SaveInstrumentVersion(target, d, audio.AudioBundle{EQ: eq})
	if !kit {
		g.drum.pickInstrument(0, target)
		return
	}
	g.drum.Rows[0].Role = instrument.RoleKick
	g.drum.ApplyKit(audio.Kit{ID: "kit.scene", DisplayName: "Scene Kit",
		Members: map[instrument.Role]instrument.ID{instrument.RoleKick: target}})
}
