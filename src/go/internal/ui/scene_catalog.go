package ui

import (
	"fmt"
	"sort"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/scope"
)

// Scene captures a reproducible UI state for the screenshot harness.
// Setup runs synchronously before the screenshot countdown starts.
//
// Mobile=true marks scenes that are also captured under MOBILE=1.
// MobileSetup, if non-nil, replaces Setup on the mobile capture pass —
// use it when a panel/overlay needs a different invocation path on
// mobile (e.g. EQ panel only renders after SetMobileEQMode(true)).
// SettleFrames overrides the default 90-frame screenshot wait when the
// scene's overlays/caches need extra time to stabilize.
type Scene struct {
	Name         string
	Description  string
	Mobile       bool
	SettleFrames int
	Setup        func(*Game)
	MobileSetup  func(*Game)

	// Subject, when non-empty, instructs the screenshot harness to crop
	// the captured PNG to that surface's on-screen bounds (resolved via
	// (*Game).SubjectRect after Setup + settle). The empty value preserves
	// the legacy full-screen behavior — every existing scene leaves this
	// unset and is unchanged.
	Subject Subject
}

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
			g.SetEQBandGain("main", 2, -6)
			g.SetEQBandGain("main", 5, 4)
		},
		MobileSetup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.SetMobileEQMode(true)
			_ = g.SetActiveEQTab("eq")
			g.SetEQBandGain("main", 2, -6)
			g.SetEQBandGain("main", 5, 4)
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
	{Name: "fx_panel_with_3_effects", Description: "delay+reverb+distortion stacked", Mobile: true, SettleFrames: 150,
		Setup: func(g *Game) {
			ensureRow(g, 0)
			instID := g.drum.Rows[0].Instrument
			audio.AddInsertEffect(instID, audio.EffectDelay, nil)
			audio.AddInsertEffect(instID, audio.EffectReverb, nil)
			audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
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
	{Name: "group_menu_open", Description: "group menu open over three adjacent grouped nodes (provisional — Save visible, member rings)", Mobile: true,
		Setup: sceneGroupMenuSetup},
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
	{Name: "overflow_menu_open", Description: "desktop overflow menu (Upload/Import/Export)", SettleFrames: 120,
		Setup: func(g *Game) { g.drum.OpenOverflowMenu() }},
	{Name: "template_menu_open", Description: "overflow menu template page (genre circuit templates + search)", Mobile: true, SettleFrames: 120,
		Setup: func(g *Game) { g.drum.overflowPage = 1; g.drum.OpenOverflowMenu() },
		MobileSetup: func(g *Game) {
			g.SetForceMobileProfile(true)
			g.drum.overflowPage = 1
			g.drum.OpenOverflowMenu()
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
			g.SetEQBandGain("main", 0, 12.0)
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
				g.Update()
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
				g.Update()
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
			g.SetEQBandGain("main", 1, -8)
			g.SetEQBandGain("main", 7, 6)
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
				g.sidebar.sectionOpen["logic"] = true
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
				g.sidebar.sectionOpen["groove"] = true
			}
		}},
	{Name: "node_sidebar_audio_expanded", Description: "node sidebar with Audio (per-node EQ) section expanded",
		Setup: func(g *Game) {
			n := g.tryAddNode(3, 1, model.NodeTypeRegular)
			g.updateBeatInfos()
			if n != nil && g.sidebar != nil {
				g.sidebar.Open(n)
				g.sidebar.sectionOpen["aud"] = true
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
	{Name: "mode_move_banner", Description: "move mode armed — helper banner, blinking subject, chain A→N→B", Mobile: true, SettleFrames: 60,
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
	{Name: "crop_drum_rows_default", Description: "drum row scroll area only (no header, no EQ)",
		Subject: SubjectDrumRows,
		Setup:   func(g *Game) {}},
	{Name: "crop_drum_rows_multi_row", Description: "drum row scroll area only, five rows",
		Subject: SubjectDrumRows,
		Setup:   func(g *Game) { ensureMultiRow(g, 5) }},
	{Name: "crop_eq_tab_eq", Description: "EQ panel cropped — EQ tab",
		Subject: SubjectEQTabEQ,
		Setup:   func(g *Game) { _ = g.SetActiveEQTab("eq") }},
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
	{Name: "crop_synth_tab_fm", Description: "Synth tab cropped — fm-bell recipe (brightness wired, no tone/body)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabSceneSetup("fm-bell"),
		MobileSetup: mobileSynthTabSetup("fm-bell")},
	{Name: "crop_synth_tab_kick_voice", Description: "Synth tab cropped — dnb-kick VOICE stage: the kick folded into the agnostic VOICE stage (no instrument-specific KICK stage), Essential kick knobs shown + Advanced expander (P4 tiering)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabStageSceneSetup("dnb-kick", synthSectionVoice),
		MobileSetup: mobileSynthTabSetup("dnb-kick")},
	{Name: "crop_synth_advanced_divider", Description: "Synth tab cropped — dnb-kick VOICE stage showing the always-visible Advanced divider row (essentials above, fine-tuning knobs below, P4 tiering)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabStageDividerSceneSetup("dnb-kick", synthSectionVoice),
		MobileSetup: mobileSynthTabSetup("dnb-kick")},
	{Name: "crop_synth_tab_violin_osc", Description: "Synth tab cropped — violin OSC stage with Advanced open: the bowed-string physical-model knobs (Bow Position/Pressure/Speed/Brightness) appear contextually because the Bowed String oscillator is selected (P4 contextual exposure)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabStageExpandedSceneSetup("violin", synthSectionOsc),
		MobileSetup: mobileSynthTabSetup("violin")},
	{Name: "crop_synth_tab_hihat_collapsed", Description: "Synth tab cropped — drum-hihat (pruned-empty sections, brightness wired)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabSceneSetup("hihat"),
		MobileSetup: mobileSynthTabSetup("hihat")},
	{Name: "crop_synth_tab_master_chooser", Description: "Synth tab cropped — channel chooser dropdown open over the Synth tab",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabMasterChooserSetup(),
		MobileSetup: mobileSynthTabMasterChooserSetup()},
	{Name: "crop_synth_tab_modular", Description: "Synth tab cropped — modular voice (OSC+FM+ENVELOPE+FILTER+POST sections, enum knobs)",
		Mobile:      true, // also captured in the mobile pass: shows the inline value pills
		Subject:     SubjectSynthPanel,
		Setup:       synthTabSceneSetup("modular"),
		MobileSetup: mobileSynthTabSetup("modular")},
	{Name: "crop_synth_knob_viz", Description: "Synth tab — FILTER stage expanded: each knob shows its concept mini-visual (the abstract 'what this knob does' picture) in the band below its caption",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabStageSetup("modular", "FILTER", ""),
		MobileSetup: mobileSynthTabStageSetup("modular", "FILTER", "")},
	{Name: "crop_synth_mirror", Description: "Synth tab — right-pane live final-output MIRROR: the real re-rendered note (with a ghost of the prior render) above the OSC/ADSR/FILTER seed plots",
		Subject:     SubjectSynthPanel,
		Setup:       synthMirrorSceneSetup(),
		MobileSetup: mobileSynthMirrorSceneSetup()},
	{Name: "crop_synth_tab_drum_generator", Description: "Synth tab — bespoke drum (snare) showing the Generator selector (Native) alongside its wired knobs",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabSceneSetup("snare"),
		MobileSetup: mobileSynthTabSetup("snare")},
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
		Setup:       synthTabStageSetup("modular", "OSC", ""),
		MobileSetup: mobileSynthTabStageSetup("modular", "OSC", "")},
	{Name: "crop_synth_tab_detail_envelope", Description: "Synth tab — chip strip with the ENVELOPE stage expanded into the detail pane (full-size knobs, untruncated captions)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabStageSetup("snare", "ENVELOPE", ""),
		MobileSetup: mobileSynthTabStageSetup("snare", "ENVELOPE", "")},
	{Name: "crop_synth_tab_ghost_chip", Description: "Synth tab — modular voice with FILTER disabled AND selected: ghost chip in the strip + bypass scrim + bright enable pill in the detail header",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabStageSetup("modular", "FILTER", "filter_enabled"),
		MobileSetup: mobileSynthTabStageSetup("modular", "FILTER", "filter_enabled")},
	// Focus-graph crops: one per property-native domain. Each selects a
	// specific knob in a stage so the single big focus graph renders that
	// knob's domain (replaces the old per-knob concept bands). The mirror
	// scene exercises the de-crammed "Your sound" pane.
	{Name: "crop_synth_focus_filter", Description: "Synth tab focus graph: FILTER stage, Cutoff knob selected (frequency-domain picture)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabFocusKnobSetup("modular", "FILTER", "filter_cutoff"),
		MobileSetup: mobileSynthTabFocusKnobSetup("modular", "FILTER", "filter_cutoff")},
	{Name: "crop_synth_focus_env", Description: "Synth tab focus graph: ENVELOPE stage, Attack knob selected (amp-envelope time-domain picture)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabFocusKnobSetup("modular", "ENVELOPE", "amp_attack"),
		MobileSetup: mobileSynthTabFocusKnobSetup("modular", "ENVELOPE", "amp_attack")},
	{Name: "crop_synth_focus_post", Description: "Synth tab focus graph: POST stage, Drive knob selected (drive/saturation transfer curve)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabFocusKnobSetup("modular", "POST", "drive"),
		MobileSetup: mobileSynthTabFocusKnobSetup("modular", "POST", "drive")},
	{Name: "crop_synth_focus_fm", Description: "Synth tab focus graph: FM stage, Op2 Depth knob selected (FM modulation-index picture)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabFocusKnobSetup("modular", "FM", "fm_op2_depth"),
		MobileSetup: mobileSynthTabFocusKnobSetup("modular", "FM", "fm_op2_depth")},
	{Name: "crop_synth_focus_osc", Description: "Synth tab focus graph: OSC stage, Oscillator shape knob selected (single-cycle waveform shape picture via conceptOsc)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabFocusKnobSetup("modular", "OSC", "osc_type"),
		MobileSetup: mobileSynthTabFocusKnobSetup("modular", "OSC", "osc_type")},
	{Name: "crop_synth_focus_pitch", Description: "Synth tab focus graph: OSC stage, Octave knob selected (real-pitch wave whose period tracks the knob, via conceptPitchWave)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabFocusKnobSetup("modular", "OSC", "osc_octave"),
		MobileSetup: mobileSynthTabFocusKnobSetup("modular", "OSC", "osc_octave")},
	{Name: "crop_synth_focus_motion", Description: "Synth tab focus graph: LFO stage, Rate knob selected (modulation-over-time wiggle curve via conceptMotion)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabFocusKnobSetup("modular", "LFO", "lfo_rate"),
		MobileSetup: mobileSynthTabFocusKnobSetup("modular", "LFO", "lfo_rate")},
	{Name: "crop_synth_focus_burst", Description: "Synth tab focus graph: BURST stage, Hit 1 Level knob selected (transient-burst modulation curve via conceptMotion)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabFocusKnobSetup("modular", "BURST", "burst1_amp"),
		MobileSetup: mobileSynthTabFocusKnobSetup("modular", "BURST", "burst1_amp")},
	{Name: "crop_synth_focus_fm_decay", Description: "Synth tab focus graph: FM stage, Op 2 Decay knob selected (FM operator decay schematic via conceptFMEnvelope)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabFocusKnobSetup("fm-bass", "FM", "fm_op2_decay"),
		MobileSetup: mobileSynthTabFocusKnobSetup("fm-bass", "FM", "fm_op2_decay")},
	{Name: "crop_synth_focus_fm_penv", Description: "Synth tab focus graph: FM stage, Pitch Sweep knob selected (FM pitch-env schematic via conceptFMEnvelope)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabFocusKnobSetup("fm-bass", "FM", "fm_pitch_env_amount"),
		MobileSetup: mobileSynthTabFocusKnobSetup("fm-bass", "FM", "fm_pitch_env_amount")},
	{Name: "crop_synth_full_note", Description: "Synth tab focus graph: ENVELOPE stage, Decay knob selected — the full-note 'Your sound' DAW-clip envelope responds to the amp envelope",
		Mobile:      true, // mobile pass verifies the stacked full-note + focus column
		Subject:     SubjectSynthPanel,
		Setup:       synthTabFocusKnobSetup("modular", "ENVELOPE", "amp_decay"),
		MobileSetup: mobileSynthTabFocusKnobSetup("modular", "ENVELOPE", "amp_decay")},
	{Name: "crop_synth_focus_burst_hit", Description: "Synth tab focus graph: BURST stage (enabled), Hit 2 Level knob selected (transient-spike timeline via conceptBurst)",
		Mobile:      true,
		Subject:     SubjectSynthPanel,
		Setup:       synthTabFocusKnobParamSetup("modular", "BURST", "burst2_amp", "burst_enabled", 1),
		MobileSetup: mobileSynthTabFocusKnobParamSetup("modular", "BURST", "burst2_amp", "burst_enabled", 1)},
	{Name: "crop_synth_focus_drive", Description: "Synth tab focus graph: POST stage, Drive knob at ~70% (before/after waveshaping via conceptPostWave)",
		Mobile:      true,
		Subject:     SubjectSynthPanel,
		Setup:       synthTabFocusKnobParamSetup("modular", "POST", "drive", "drive", 0.7),
		MobileSetup: mobileSynthTabFocusKnobParamSetup("modular", "POST", "drive", "drive", 0.7)},
	{Name: "crop_synth_mirror_clean", Description: "Synth tab de-crammed right-pane 'Your sound' MIRROR (modular voice, clean live final-output render)",
		Subject:     SubjectSynthPanel,
		Setup:       synthMirrorSceneSetup(),
		MobileSetup: mobileSynthMirrorSceneSetup()},
	// Sampler-tab scenes — the sample editor (trim/pitch/gain a synth capture
	// or loaded WAV, then Save / Save As). The "captured" scene populates the
	// working buffer so the waveform, trim handles, and trimmed-region shading
	// render; the "empty" scene exercises the no-buffer placeholder + disabled
	// affordances.
	{Name: "crop_sampler_tab_captured", Description: "Sampler tab cropped — synth one-shot captured, trimmed to middle 70%, Normalize on",
		Subject:     SubjectSampler,
		Setup:       samplerTabSceneSetup(true),
		MobileSetup: mobileSamplerTabSetup(true)},
	{Name: "crop_sampler_tab_empty", Description: "Sampler tab cropped — no buffer loaded (capture/load placeholder state)",
		Subject:     SubjectSampler,
		Setup:       samplerTabSceneSetup(false),
		MobileSetup: mobileSamplerTabSetup(false)},
	{Name: "crop_audio_selector_bar", Description: "AudioStickyBar standalone crop", SettleFrames: 90,
		Subject: SubjectAudioStickyBar,
		Setup:   func(g *Game) { _ = g.SetActiveEQTab("eq") }},
	{Name: "crop_levels_single_channel", Description: "Levels tab single-channel detail",
		SettleFrames: 120, Subject: SubjectEQTabLevels,
		Setup: func(g *Game) { _ = g.SetActiveEQTab("levels"); g.SetPlaying(true) }},
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
	{Name: "crop_chain_auto_gain_on", Description: "Chain with auto-gain enabled (AG pill active)", SettleFrames: 150,
		Subject: SubjectChain,
		Setup: func(g *Game) {
			activateScopeTab(g)
			if z := chainZoneOf(g); z != nil {
				z.SetAutoGain(true)
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
	{Name: "crop_chain_autofit_off", Description: "Chain with auto-fit off + manual 40ms window (FIT toggle inactive)",
		SettleFrames: 150, Subject: SubjectChain,
		Setup: func(g *Game) {
			activateScopeTab(g)
			if z := chainZoneOf(g); z != nil {
				z.SetWindowMs(40) // disables auto-fit; FIT pill renders inactive
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
	{Name: "crop_mobile_synth_wheel", Description: "mobile synth scroll-wheel popup open over a continuous knob",
		Mobile:      true, // captured in the mobile pass: the after-tap scroll-wheel state
		Subject:     SubjectSynthWheel,
		Setup:       synthWheelSceneSetup(),
		MobileSetup: mobileSynthWheelSceneSetup()},
}

// RunScene applies the named scene's Setup to g. Returns an error if the
// name is not in the catalog.
func RunScene(g *Game, name string) error {
	return runSceneInternal(g, name, false)
}

// RunSceneMobile applies the named scene's MobileSetup to g (or Setup if
// MobileSetup is nil). Returns an error if the name is not in the
// catalog or the scene is not marked Mobile.
func RunSceneMobile(g *Game, name string) error {
	return runSceneInternal(g, name, true)
}

func runSceneInternal(g *Game, name string, mobile bool) error {
	for _, s := range sceneCatalog {
		if s.Name == name {
			// The native -scene flag fires before Ebiten's first
			// Layout/Update. Popup-opening setups anchor against control
			// rects that only exist after a layout pass, so applying the
			// scene pre-layout mis-anchors every popup (context menu
			// clipped in a corner, instrument menu fully offscreen). Defer
			// to the update loop — two queued hops so frame 1 performs the
			// first Layout + recalc and frame 2 runs Setup against real
			// geometry (mirrors the browser runScene export, which always
			// deferred via QueueAction and never mis-anchored).
			if g.winW == 0 {
				scene := s
				g.QueueAction(func(g *Game) {
					g.QueueAction(func(g *Game) { applySceneSetup(g, scene, mobile) })
				})
				return nil
			}
			applySceneSetup(g, s, mobile)
			return nil
		}
	}
	return fmt.Errorf("unknown scene %q", name)
}

// applySceneSetup performs the scene's Setup against a laid-out game and
// arms the screenshot settle/subject state.
func applySceneSetup(g *Game, s Scene, mobile bool) {
	setup := s.Setup
	if mobile && s.MobileSetup != nil {
		setup = s.MobileSetup
	}
	if setup != nil {
		setup(g)
	}
	// Suppress the coordinate badge ("(i, j)" pill) that node
	// placement arms: tryAddNode selects the new node and sets
	// g.coordBadgeNode, which would otherwise leak a transient
	// debug-ish hover affordance into the capture (observed as a
	// stray "(4, 2)" pill in graph_complex_3_nodes_4_edges). It
	// re-arms only on real input, so a single clear after Setup
	// holds through the settle countdown.
	g.coordBadgeNode = nil
	if s.SettleFrames > 0 {
		g.SetScreenshotSettleFrames(s.SettleFrames)
	}
	g.SetScreenshotSubject(s.Subject)
	emitSceneApplied(s.Name)
}

// ListScenes returns the catalog sorted by name.
func ListScenes() []Scene {
	out := make([]Scene, len(sceneCatalog))
	copy(out, sceneCatalog)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// SceneNames returns just the scene names sorted alphabetically.
func SceneNames(includeMobile bool) []string {
	var names []string
	for _, s := range sceneCatalog {
		if !includeMobile && s.Mobile && !sceneRunsOnDesktop(s) {
			continue
		}
		names = append(names, s.Name)
	}
	sort.Strings(names)
	return names
}

// sceneRunsOnDesktop returns true if the scene is appropriate for the
// desktop pass. Mobile-prefixed scenes are mobile-only.
func sceneRunsOnDesktop(s Scene) bool {
	if len(s.Name) >= 7 && s.Name[:7] == "mobile_" {
		return false
	}
	return true
}

// MobileSceneNames returns the names of scenes captured under MOBILE=1.
func MobileSceneNames() []string {
	var names []string
	for _, s := range sceneCatalog {
		if s.Mobile {
			names = append(names, s.Name)
		}
	}
	sort.Strings(names)
	return names
}

// ─── helpers ──────────────────────────────────────────────────────

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
func synthTabStageSceneSetup(instrument string, section synthSectionID) func(*Game) {
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
func synthTabStageExpandedSceneSetup(instrument string, section synthSectionID) func(*Game) {
	return synthTabStageSceneSetup(instrument, section)
}

// synthTabStageDividerSceneSetup opens a stage and scrolls its card grid so the
// "— ADVANCED · N —" divider row (and the first advanced cards below it) are in
// view — a screenshot of the divider scene must actually show the divider, not
// just the essential row above the fold.
func synthTabStageDividerSceneSetup(instrument string, section synthSectionID) func(*Game) {
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

func synthTabSceneSetup(instrumentOverride string) func(*Game) {
	return func(g *Game) {
		g.buildDemo()
		if instrumentOverride != "" && len(g.drum.Rows) > 0 {
			g.drum.Rows[0].Instrument = instrumentOverride
		}
		_ = g.SetActiveEQTab("synth")
		if g.drum != nil && g.drum.eqPanelZone != nil && len(g.drum.Rows) > 0 {
			// Select Rows[0]'s instrument as the active channel. The Synth tab
			// requires a single-instrument context — Master is now blocked for
			// it — so pick the row explicitly (previously this relied on
			// synthTabActiveInstrument falling back to Rows[0] under Master).
			g.drum.eqPanelZone.SetActiveChannel(g.drum.Rows[0].Instrument)
		}
		// Lay the panel out through the tree so the chip strip exists, then
		// render the per-chip stage thumbnails synchronously — the chip-strip
		// watermarks are pool-rendered in production, so scene Setup must fill
		// them deterministically before capture (sibling of the
		// renderSynthMirrorNow calls in the stage/focus setups).
		if g.drum != nil && g.drum.audioTree != nil {
			g.drum.audioTree.LayoutZoneNow("eq-panel")
		}
		if g.drum != nil {
			if inst := g.drum.resolveSynthInstrument(g.drum.synthTabActiveInstrument()); inst != "" {
				g.drum.ensureStageThumbsNow(inst)
			}
		}
	}
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
		// Ensure no synth recipe is bound (a fresh id has none, but be
		// explicit so the scene is robust against demo seeding changes).
		audio.BindInstrumentToRecipe("wav-sample", "")
		_ = g.SetActiveEQTab("synth")
		if g.drum != nil && g.drum.eqPanelZone != nil {
			// Select the WAV-sample instrument itself as the active channel so
			// the Synth tab renders its "no synth" banner. Master is blocked
			// for the Synth tab, so we can't use "main" here anymore.
			g.drum.eqPanelZone.SetActiveChannel("wav-sample")
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
func mobileSynthTabSetup(instrumentOverride string) func(*Game) {
	return func(g *Game) {
		g.SetForceMobileProfile(true)
		g.drum.SetMobileEQMode(true)
		synthTabSceneSetup(instrumentOverride)(g)
	}
}

// synthTabStageSetup is synthTabSceneSetup + opening a specific stage in the
// chip strip's detail pane. disableParam (optional) toggles a stage off
// first so the scene shows a ghost chip — and, when the disabled stage is
// also the selected one, the bypass scrim. Drives SelectSynthSectionByLabel,
// the same path a real chip tap takes.
func synthTabStageSetup(instrumentOverride, stageLabel, disableParam string) func(*Game) {
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
		// Render the "Your sound" mirror synchronously so it's deterministic at
		// capture time (the live per-frame path needs a real Draw to populate).
		// The stage-thumb watermarks are pool-rendered in production, so fill
		// them synchronously too (the disableParam edit above changed the hash).
		if g.drum != nil {
			if inst := g.drum.resolveSynthInstrument(g.drum.synthTabActiveInstrument()); inst != "" {
				g.drum.renderSynthMirrorNow(inst)
				g.drum.ensureStageThumbsNow(inst)
			}
		}
	}
}

// mobileSynthTabStageSetup is the mobile counterpart of synthTabStageSetup.
func mobileSynthTabStageSetup(instrumentOverride, stageLabel, disableParam string) func(*Game) {
	return func(g *Game) {
		g.SetForceMobileProfile(true)
		g.drum.SetMobileEQMode(true)
		synthTabStageSetup(instrumentOverride, stageLabel, disableParam)(g)
	}
}

// synthTabFocusKnobSetup opens stageLabel for instrument and selects the knob
// named knobParam so the focus graph shows that knob's property-native domain.
// Builds on synthTabStageSetup (which selects the stage + renders the mirror),
// then walks the selected section's knobs to find the one whose ParamDef.Name
// matches knobParam and records it via setSynthSelectedKnob. If the knob name
// isn't present in the stage, the focus graph falls back to the section's main
// knob — still renders, but pick correct names so the intended domain shows.
func synthTabFocusKnobSetup(instrumentOverride, stageLabel, knobParam string) func(*Game) {
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
				g.drum.setSynthSelectedKnob(inst, kIdx)
				break
			}
		}
		g.drum.renderSynthMirrorNow(inst)
		g.drum.ensureStageThumbsNow(inst)
	}
}

// mobileSynthTabFocusKnobSetup is the mobile counterpart of
// synthTabFocusKnobSetup — exercises the mobile stacked focus band.
func mobileSynthTabFocusKnobSetup(instrumentOverride, stageLabel, knobParam string) func(*Game) {
	return func(g *Game) {
		g.SetForceMobileProfile(true)
		g.drum.SetMobileEQMode(true)
		synthTabFocusKnobSetup(instrumentOverride, stageLabel, knobParam)(g)
	}
}

// synthTabFocusKnobParamSetup builds on synthTabFocusKnobSetup and additionally
// pins one live param (e.g. drive to 70% of its 0..1 range) before the final
// mirror render so the focus graph and the "Your sound" pane reflect the driven
// value, not the recipe default.
func synthTabFocusKnobParamSetup(instrumentOverride, stageLabel, knobParam, setParam string, setVal float64) func(*Game) {
	return func(g *Game) {
		synthTabFocusKnobSetup(instrumentOverride, stageLabel, knobParam)(g)
		if g.drum == nil {
			return
		}
		inst := g.drum.resolveSynthInstrument(g.drum.synthTabActiveInstrument())
		if inst == "" {
			return
		}
		audio.SetInstrumentParam(inst, setParam, setVal)
		g.drum.renderSynthMirrorNow(inst)
		g.drum.ensureStageThumbsNow(inst)
	}
}

// mobileSynthTabFocusKnobParamSetup is the mobile counterpart of
// synthTabFocusKnobParamSetup.
func mobileSynthTabFocusKnobParamSetup(instrumentOverride, stageLabel, knobParam, setParam string, setVal float64) func(*Game) {
	return func(g *Game) {
		g.SetForceMobileProfile(true)
		g.drum.SetMobileEQMode(true)
		synthTabFocusKnobParamSetup(instrumentOverride, stageLabel, knobParam, setParam, setVal)(g)
	}
}

// synthMirrorSceneSetup opens the Synth tab on the modular voice and fills the
// right-pane mirror SYNCHRONOUSLY (renderSynthMirrorNow bypasses the async pool)
// so the rendered note exists deterministically at screenshot-capture time.
// Stage 4.
func synthMirrorSceneSetup() func(*Game) {
	return func(g *Game) {
		synthTabSceneSetup("modular")(g)
		if g.drum == nil {
			return
		}
		inst := g.drum.resolveSynthInstrument(g.drum.synthTabActiveInstrument())
		if inst != "" {
			g.drum.renderSynthMirrorNow(inst)
			g.drum.ensureStageThumbsNow(inst)
		}
	}
}

// mobileSynthMirrorSceneSetup is the mobile counterpart of synthMirrorSceneSetup.
func mobileSynthMirrorSceneSetup() func(*Game) {
	return func(g *Game) {
		g.SetForceMobileProfile(true)
		g.drum.SetMobileEQMode(true)
		synthMirrorSceneSetup()(g)
	}
}

// synthTabDrumRevoicedSetup points row 0 at the snare drum, switches its
// generator to a Saw waveform (Modular recipe, osc_type=1), and plays — so
// the capture shows a drum row re-voiced through the full modular pipeline
// (post gen_type-deprecation, re-voicing = rebinding to the Modular recipe).
func synthTabDrumRevoicedSetup() func(*Game) {
	return func(g *Game) {
		synthTabSceneSetup("snare")(g)
		audio.BindInstrumentToRecipe("snare", "synth-modular")
		audio.SetInstrumentParam("snare", "osc_type", 1) // Saw
		g.SetPlaying(true)
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
		synthTabSceneSetup("modular")(g)
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
		synthTabSceneSetup("modular")(g)
		audio.SetInstrumentParam("modular", "osc_type", 6) // Noise Pink
		audio.SetInstrumentParam("modular", "filter_enabled", 0)
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
			dv.eqPanelZone.SetActiveChannel(dv.Rows[0].Instrument)
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

// mobileAudioPanelSetup returns a Setup that forces mobile profile,
// expands the mobile audio (EQ/Wave/Spec/Meters/Scope) panel, and sets
// the requested tab. Used as the MobileSetup for desktop EQ scenes so
// their mobile capture exercises a real route to the panel.
// synthWheelSceneSetup opens the Synth tab, lays out the synth panel, finds
// a continuous knob (filter_cutoff on the modular recipe), and opens the
// mobile scroll-wheel popup for that knob. The popup stays open because
// MobileWheelPopup.ShouldClose() only fires on an explicit Close call.
// Works at any framebuffer size (desktop 1280×720 or mobile 390×844).
func synthWheelSceneSetup() func(*Game) {
	return func(g *Game) {
		// Use the modular recipe — it has filter_cutoff (continuous, non-enum).
		synthTabSceneSetup("modular")(g)
		if g.drum == nil {
			return
		}
		// Drive a layout pass so dv.Bounds is populated before opening the popup.
		if g.drum.audioTree != nil {
			g.drum.audioTree.LayoutZoneNow("eq-panel")
		}
		// Find filter_cutoff knob index.
		idx := -1
		for i, b := range g.drum.instEditorBindings {
			if b.def.Name == "filter_cutoff" {
				idx = i
				break
			}
		}
		if idx < 0 {
			return
		}
		// Select the section that owns this knob so the knob has a non-empty rect.
		inst := g.drum.resolveSynthInstrument(g.drum.synthTabActiveInstrument())
		for _, s := range g.drum.instEditorSections {
			for _, k := range s.knobIdxs {
				if k == idx {
					g.drum.setSelectedSynthSection(inst, s.id)
					break
				}
			}
		}
		if g.drum.audioTree != nil {
			g.drum.audioTree.LayoutZoneNow("eq-panel")
		}
		// Open the wheel popup.
		g.drum.openSynthKnobWheelPopup(idx, inst)
	}
}

// mobileSynthWheelSceneSetup is the mobile counterpart of synthWheelSceneSetup.
func mobileSynthWheelSceneSetup() func(*Game) {
	return func(g *Game) {
		g.SetForceMobileProfile(true)
		g.drum.SetMobileEQMode(true)
		// Force a layout at mobile dimensions so dv.Bounds is populated before
		// the inner setup opens the popup (the GOTCHA from CLAUDE.md: opening
		// a popup against stale/zero bounds produces an off-screen subject rect).
		g.Layout(414, 896)
		g.Update()
		synthWheelSceneSetup()(g)
	}
}

func mobileAudioPanelSetup(tab string) func(*Game) {
	return func(g *Game) {
		g.SetForceMobileProfile(true)
		g.drum.SetMobileEQMode(true)
		_ = g.SetActiveEQTab(tab)
	}
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

// ensureRow guarantees there is a row at index idx, adding rows as needed.
func ensureRow(g *Game, idx int) {
	if g.drum == nil {
		return
	}
	for len(g.drum.Rows) <= idx {
		g.drum.AddRow()
	}
}

// sceneSetBPM updates the BPM through the same path the BPM box uses.
func sceneSetBPM(g *Game, bpm int) {
	if g.drum == nil {
		return
	}
	g.drum.SetBPM(bpm)
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
	g.sidebar.sectionOpen["grp"] = true
	// The Group section is the sidebar's last section; on the short desktop
	// grid pane its rows sit below the scroll fold. Scroll to the bottom so
	// the capture shows the chips/remove/add controls (layout clamps First
	// to the real max).
	g.sidebar.scroll.VS.First = 1 << 20
}
