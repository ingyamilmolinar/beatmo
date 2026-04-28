package ui

import (
	"fmt"
	"sort"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// Scene captures a reproducible UI state for the screenshot harness.
// Setup runs synchronously before the screenshot countdown starts.
//
// Mobile=true marks scenes that are also captured under MOBILE=1.
// SettleFrames overrides the default 90-frame screenshot wait when the
// scene's overlays/caches need extra time to stabilize.
type Scene struct {
	Name         string
	Description  string
	Mobile       bool
	SettleFrames int
	Setup        func(*Game)
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

	// ─── EQ tabs ──────────────────────────────────────────────────
	{Name: "eq_tab_eq", Description: "EQ tab active",
		Setup: func(g *Game) { _ = g.SetActiveEQTab("eq") }},
	{Name: "eq_tab_wave", Description: "Wave tab active",
		Setup: func(g *Game) { _ = g.SetActiveEQTab("wave") }},
	{Name: "eq_tab_spectrum", Description: "Spectrum tab active",
		Setup: func(g *Game) { _ = g.SetActiveEQTab("spectrum") }},
	{Name: "eq_tab_meters", Description: "Meters tab active",
		Setup: func(g *Game) { _ = g.SetActiveEQTab("meters") }},
	{Name: "eq_tab_scope", Description: "Scope tab active", SettleFrames: 120,
		Setup: func(g *Game) { _ = g.SetActiveEQTab("scope"); g.SetScopeVisible(true) }},
	{Name: "eq_with_band_adjusted", Description: "two EQ bands tweaked",
		Setup: func(g *Game) {
			_ = g.SetActiveEQTab("eq")
			g.SetEQBandGain("main", 2, -6)
			g.SetEQBandGain("main", 5, 4)
		}},

	// ─── per-row menus ────────────────────────────────────────────
	{Name: "context_menu_open", Description: "row context menu open", Mobile: true, SettleFrames: 120,
		Setup: func(g *Game) { ensureRow(g, 0); g.drum.OpenContextMenu(0) }},
	{Name: "instrument_menu_open", Description: "instrument selector open", Mobile: true, SettleFrames: 120,
		Setup: func(g *Game) { ensureRow(g, 0); g.drum.OpenInstrumentMenu(0) }},
	{Name: "color_wheel_open", Description: "color wheel picker open", Mobile: true, SettleFrames: 120,
		Setup: func(g *Game) { ensureRow(g, 0); g.drum.OpenColorMenu(0) }},
	{Name: "subdiv_menu_open", Description: "subdivision menu open", SettleFrames: 120,
		Setup: func(g *Game) { g.drum.OpenSubdivMenu() }},

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
	{Name: "master_vol_popup", Description: "master volume slider open",
		Setup: func(g *Game) { g.drum.OpenMasterVolumePopup() }},

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

	// ─── multi-row state ──────────────────────────────────────────
	{Name: "multi_row_full_grid", Description: "four rows added", Mobile: true,
		Setup: func(g *Game) {
			for i := 0; i < 4; i++ {
				g.drum.AddRow()
			}
		}},
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
	{Name: "transport_recording", Description: "recording session active", SettleFrames: 120,
		Setup: func(g *Game) { _ = g.StartRecording() }},

	// ─── mobile-only surfaces ─────────────────────────────────────
	{Name: "mobile_default", Description: "mobile profile default boot", Mobile: true,
		Setup: func(g *Game) { g.SetForceMobileProfile(true) }},
	{Name: "mobile_overflow_open", Description: "overflow menu (Upload/Import/Export)", Mobile: true, SettleFrames: 120,
		Setup: func(g *Game) { g.SetForceMobileProfile(true); g.drum.OpenOverflowMenu() }},
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
	{Name: "playback_eq_tab_meters", Description: "playback running + Meters tab", SettleFrames: 120,
		Setup: func(g *Game) {
			g.SetPlaying(true)
			_ = g.SetActiveEQTab("meters")
		}},
	{Name: "playback_eq_tab_scope", Description: "playback running + Scope tab", SettleFrames: 150,
		Setup: func(g *Game) {
			g.SetPlaying(true)
			_ = g.SetActiveEQTab("scope")
			g.SetScopeVisible(true)
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
	{Name: "eq_hpf_active", Description: "high-pass filter engaged",
		Setup: func(g *Game) {
			_ = g.SetActiveEQTab("eq")
			g.drum.toggleHPF()
		}},
	{Name: "eq_lpf_active", Description: "low-pass filter engaged",
		Setup: func(g *Game) {
			_ = g.SetActiveEQTab("eq")
			g.drum.toggleLPF()
		}},
	{Name: "eq_band_muted", Description: "single EQ band muted",
		Setup: func(g *Game) {
			_ = g.SetActiveEQTab("eq")
			muted := g.drum.eqBandMuted()
			if len(muted) > 4 {
				muted[4] = true
			}
		}},
	{Name: "eq_scope_custom_settings", Description: "Scope tab with two bands tweaked", SettleFrames: 150,
		Setup: func(g *Game) {
			_ = g.SetActiveEQTab("scope")
			g.SetScopeVisible(true)
			g.SetEQBandGain("main", 1, -8)
			g.SetEQBandGain("main", 7, 6)
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

	// ─── desktop parity & recording overlays ──────────────────────
	{Name: "desktop_per_row_vol_popup", Description: "desktop per-row volume popup", SettleFrames: 120,
		Setup: func(g *Game) {
			ensureRow(g, 0)
			g.drum.OpenVolumePopup(0)
		}},
	{Name: "recording_with_context_menu", Description: "recording armed + context menu open", SettleFrames: 150,
		Setup: func(g *Game) {
			ensureRow(g, 0)
			_ = g.StartRecording()
			g.drum.OpenContextMenu(0)
		}},
	{Name: "recording_during_playback_overlay", Description: "recording armed during playback", SettleFrames: 150,
		Setup: func(g *Game) {
			_ = g.StartRecording()
			g.SetPlaying(true)
		}},
}

// RunScene applies the named scene's Setup to g. Returns an error if the
// name is not in the catalog.
func RunScene(g *Game, name string) error {
	for _, s := range sceneCatalog {
		if s.Name == name {
			if s.Setup != nil {
				s.Setup(g)
			}
			if s.SettleFrames > 0 {
				g.SetScreenshotSettleFrames(s.SettleFrames)
			}
			return nil
		}
	}
	return fmt.Errorf("unknown scene %q", name)
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
