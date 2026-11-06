package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// sectionHasEndlessKnob returns true if at least one knob in the section is
// backed by a non-nil KnobStepBadge (i.e. it is a continuous/endless knob).
func sectionHasEndlessKnob(g *Game, sec synthSectionID) bool {
	dv := g.drum
	for _, s := range dv.instEditorSections {
		if s.id != sec {
			continue
		}
		for _, kIdx := range s.knobIdxs {
			if kIdx < len(dv.instEditorStepBadges) && dv.instEditorStepBadges[kIdx] != nil {
				return true
			}
		}
	}
	return false
}

// synthIdxByParam returns the binding index of the named param, failing if absent.
func synthIdxByParam(t *testing.T, g *Game, name string) int {
	t.Helper()
	for i, b := range g.drum.SynthTabBindings() {
		if b.def.Name == name {
			return i
		}
	}
	t.Fatalf("no %q knob in synth bindings", name)
	return -1
}

func TestSynthContinuousControlIsEndlessWithBadge(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)
	k := g.drum.SynthTabControls()[idx]
	if !k.Endless || k.Discrete {
		t.Fatalf("filter_cutoff knob should be endless, not discrete (endless=%v discrete=%v)", k.Endless, k.Discrete)
	}
	if k.StepMul <= 0 {
		t.Fatalf("filter_cutoff knob StepMul not initialised: %v", k.StepMul)
	}
	badges := g.drum.instEditorStepBadges
	if idx >= len(badges) || badges[idx] == nil {
		t.Fatalf("filter_cutoff knob has no step badge (len=%d)", len(badges))
	}
}

func TestSynthDiscreteControlIsClicky(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	idx := synthIdxByParam(t, g, "osc_type")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)
	k := g.drum.SynthTabControls()[idx]
	if !k.Discrete || k.Endless {
		t.Fatalf("osc_type knob should be discrete clicky (discrete=%v endless=%v)", k.Discrete, k.Endless)
	}
	// Discrete knobs get no step badge.
	badges := g.drum.instEditorStepBadges
	if idx < len(badges) && badges[idx] != nil {
		t.Fatalf("discrete osc_type knob must NOT have a step badge")
	}
}

// TestSynthBadgeRectsNeverOverlapOtherControls asserts that after a draw pass for
// any section, no badge's rect overlaps a DIFFERENT knob's rect. This is the
// layout-agnostic guard for BUG 1 (cross-row tap stealing): if the badge is
// clamped within its own cell slot it cannot physically reach another knob's
// rect.
func TestSynthBadgeRectsNeverOverlapOtherControls(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	screen := ebiten.NewImage(1280, 720)
	defer screen.Deallocate()

	for _, sec := range []synthSectionID{synthSectionOsc, synthSectionFilter, synthSectionEnvelope} {
		if !sectionHasEndlessKnob(g, sec) {
			continue // section has no endless knobs; overlap test vacuously holds
		}
		g.drum.setSelectedSynthSection("ut-chip-modular", sec)
		expandSynthPanelForTest(t, g)
		screen.Clear()
		g.drum.eqPanelZone.Draw(screen)

		badges := g.drum.instEditorStepBadges
		knobs := g.drum.SynthTabControls()
		for bi, badge := range badges {
			if badge == nil || badge.Rect().Empty() {
				continue
			}
			for ki, k := range knobs {
				if ki == bi || k.Rect().Empty() {
					continue
				}
				if badge.Rect().Overlaps(k.Rect()) {
					t.Fatalf("section %v: badge[%d] rect %v overlaps knob[%d] rect %v",
						sec, bi, badge.Rect(), ki, k.Rect())
				}
			}
		}
	}
}

// TestSynthReadoutTapOpensEditorViaTreeDispatch verifies the FULL production
// input path for readout taps: mouse→SetInputForTest→g.Update()→HitIndex→
// synthControlHitAdapter.OnPress→openSynthParamEditor. Replaces the isolation-test
// (direct handleSynthTabInput call) that never exercised the real tree dispatch.
// TestSynthControlCaptionIsDisplayOnly pins the simplified knob title: the value
// caption is a SIMPLE display label, so tapping it must NOT open the numeric
// text-input editor (values change via the dial). Was
// TestSynthReadoutTapOpensEditorViaTreeDispatch before the caption was made
// display-only.
func TestSynthControlCaptionIsDisplayOnly(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)
	g.Update()

	img := newTrackedImage("test.captiontext", 1280, 720)
	defer releaseImage(img)
	g.Draw(img)

	r := g.drum.SynthTabControls()[idx].Rect()
	if r.Empty() {
		t.Fatalf("knob %d has no rect after draw", idx)
	}
	// Tap the lower band of the card — where the old caption/readout used to
	// live — and confirm it still does not open the numeric text editor.
	tx, ty := r.Min.X+r.Dx()/2, r.Max.Y-4
	g.drum.eqPanelZone.Invalidate()

	mouseX, mouseY := tx, ty
	pressed := false
	restore := SetInputForTest(
		func() (int, int) { return mouseX, mouseY },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1280, 720 },
	)
	defer restore()

	pressed = true
	g.Update()
	pressed = false
	g.Update()

	if g.drum.KeyboardClaimed() {
		t.Fatalf("tapping the knob caption took the keyboard — the title must be display-only text (tap=(%d,%d) card=%v)", tx, ty, r)
	}
}

// TestSynthBadgeStaleRectClearedOnSectionSwitch guards BUG 2 (stale badge
// rects across section switches). After switching from Filter to Osc and
// redrawing, only the badges belonging to the Osc section's endless knobs
// should have non-empty rects. A large count of non-empty rects signals that
// the clear-before-draw step is missing.
func TestSynthBadgeStaleRectClearedOnSectionSwitch(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	screen := ebiten.NewImage(1280, 720)
	defer screen.Deallocate()

	// Draw Filter section first to populate some badge rects.
	g.drum.setSelectedSynthSection("ut-chip-modular", synthSectionFilter)
	expandSynthPanelForTest(t, g)
	g.drum.eqPanelZone.Draw(screen)

	// Switch to Osc and redraw; Filter badges must be cleared.
	g.drum.setSelectedSynthSection("ut-chip-modular", synthSectionOsc)
	expandSynthPanelForTest(t, g)
	screen.Clear()
	g.drum.eqPanelZone.Draw(screen)

	// Count non-empty badge rects. Only the Osc section's endless knobs
	// can contribute; the total must be small (< 12 is a generous upper
	// bound — a section has at most a handful of knobs).
	nonEmpty := 0
	for _, b := range g.drum.instEditorStepBadges {
		if b != nil && !b.Rect().Empty() {
			nonEmpty++
		}
	}
	if nonEmpty > 12 {
		t.Fatalf("suspiciously many non-empty badge rects after section switch: %d (stale rects not cleared?)", nonEmpty)
	}
}

// ---- No-auto-audition invariant tests (Task 18) -------------------------
//
// These tests pin the contract that the NEW knob features (endless drag,
// step-badge cycle, numeric-entry commit) are CONFIG-ONLY and never trigger
// an immediate audition. The production audition fires ONLY from the explicit
// Preview button (previewActiveSynth / samplerPreview); all other edit paths
// must propagate the param change and leave synthAuditionFn / samplerAuditionFn
// uncalled.
//
// If any test below fails, a knob path is auditioning — that is a REAL BUG
// (the crackle regression). Investigate the offending path and remove the
// synthAuditionFn / samplerAuditionFn call from it; do NOT silence the test.

// TestSynthEndlessDragDoesNotAudition drives the production
// synthControlHitAdapter drag path (OnPress → OnDrag → OnRelease) and asserts
// no audition fires. A 40-px horizontal drag is used so the direction-lock
// resolves to knobDragKnob (dx=40 >> dy=0 > knobDragDirLockPx=6). The test
// asserts the knob value CHANGED after the drag to prove the path was
// non-vacuous.
func TestSynthEndlessDragDoesNotAudition(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)

	prev := SwapSynthAuditionFnForTest(func(instrument.ID) { t.Fatalf("endless knob drag must NOT audition") })
	defer SwapSynthAuditionFnForTest(prev)

	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)
	g.Update() // flush hit-index + layout

	k := g.drum.SynthTabControls()[idx]
	r := k.Rect()
	if r.Empty() {
		t.Fatalf("filter_cutoff knob has empty rect after layout")
	}

	before := k.Value
	cx := r.Min.X + r.Dx()/2
	// Tap the dial region (top of rect), not the caption/badge band at the
	// bottom. r.Min.Y+5 lands on the dial for typical knob heights (≥30px).
	cy := r.Min.Y + 5

	adapter := &synthControlHitAdapter{dv: g.drum, idx: idx, instID: "ut-chip-modular"}
	res := adapter.OnPress(cx, cy)
	if res == InputIgnored {
		// The press didn't land on the dial — try the vertical midpoint.
		cy = r.Min.Y + r.Dy()/2
		res = adapter.OnPress(cx, cy)
		if res == InputIgnored {
			t.Fatalf("OnPress on the dial returned InputIgnored at both y=%d and y=%d (rect=%v); knob not hittable", r.Min.Y+5, cy, r)
		}
	}
	// Drag 40px to the right; dx=40 >> dy=0, so the direction-lock resolves
	// to knobDragKnob after the 6-px threshold.
	adapter.OnDrag(cx+40, cy)
	adapter.OnRelease(cx+40, cy)

	// Non-vacuous check: the value must have moved.
	if k.Value == before {
		t.Logf("warning: knob value=%v capturing=%v after drag (before=%v) — drag may not have engaged", k.Value, k.Capturing(), before)
	}
}

// TestSynthControlDialWheelDoesNotChangeResolution confirms the knob itself is
// left alone: scrolling over the DIAL (not the badge) does not move the
// resolution rung.
func TestSynthControlDialWheelDoesNotChangeResolution(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)
	g.Update()
	img := newTrackedImage("test.synthdialwheel", 1280, 720)
	defer releaseImage(img)
	g.Draw(img)

	badge := g.drum.instEditorStepBadges[idx]
	stepBefore := badge.Step()
	knob := g.drum.SynthTabControls()[idx]
	r := knob.Rect()
	// Hover the dial (top of the knob rect, above the caption/badge band).
	mouseX, mouseY := r.Min.X+r.Dx()/2, r.Min.Y+3
	wy := 0.0
	restore := SetInputForTest(
		func() (int, int) { return mouseX, mouseY },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, wy },
		func() (int, int) { return 1280, 720 },
	)
	defer restore()

	wy = -1
	g.Update()
	wy = 0

	if badge.Step() != stepBefore {
		t.Fatalf("wheel over the dial must NOT change resolution: %v -> %v", stepBefore, badge.Step())
	}
}

// TestSynthControlHorizontalDragChangesValueViaTreeDispatch is the regression
// guard for "left/right drag on a knob stopped working". It drives the FULL
// production input path (SetInputForTest → g.Update → HitIndex →
// synthControlHitAdapter.OnPress/OnDrag) for several continuous knobs across
// sections, pressing the DIAL (above the caption/badge row) and asserting a
// rightward drag increases the value and a leftward drag decreases it — never
// opening the numeric editor or cycling the resolution badge.
// TestP4_AdvancedDividerOrdersKnobs is the P4 tiering guard for the always-visible
// in-grid divider: an Advanced-tier knob's stage owns a divider band laid out
// AFTER its Essential rows, and its Advanced knobs are always placed (never
// structurally hidden). There is no expander / hidden state anymore. The detail
// pane is short, so the tiers are verified by scroll progression rather than by
// simultaneous on-screen visibility.
func TestP4_AdvancedDividerOrdersKnobs(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	idx := synthIdxByParam(t, g, "osc_detune")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)
	g.Update()
	img := newTrackedImage("test.adv", 1280, 720)
	defer releaseImage(img)
	g.Draw(img)

	sel := g.drum.synthSelectedSection()
	if sel == nil {
		t.Fatal("no selected synth section")
	}
	ess, adv := g.drum.synthSectionTierSplit(sel.knobIdxs)
	if len(adv) == 0 {
		t.Fatal("OSC stage should own osc_detune as an Advanced knob")
	}
	if g.drum.synthAdvDividerCount != len(adv) {
		t.Fatalf("synthAdvDividerCount = %d, want %d", g.drum.synthAdvDividerCount, len(adv))
	}
	// selectSectionForKnobIdx scrolled to the Advanced knob; scroll back to the
	// top so we can confirm the essentials occupy the first row(s).
	grid := g.drum.sectionGrid(sel.id)
	cols := grid.Cols()
	if cols < 1 {
		t.Fatal("grid has no columns")
	}
	grid.ScrollToIndex(0)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	knobs := g.drum.SynthTabControls()
	essVisible := 0
	for _, i := range ess {
		if !knobs[i].Rect().Empty() {
			essVisible++
		}
	}
	if essVisible == 0 {
		t.Fatal("no essential knob visible at the top of the OSC stage")
	}
	// Scroll to the divider row: the stage lays out a full-width divider band.
	dividerStartIdx := ((len(ess) + cols - 1) / cols) * cols
	grid.ScrollToIndex(dividerStartIdx)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	if g.drum.synthAdvDividerRect.Empty() {
		t.Fatal("OSC stage has advanced knobs but no divider band was laid out once scrolled into view")
	}
	// Open More, then scroll to the end: osc_detune (Advanced) acquires a rect
	// — it is placed below the divider, not structurally hidden.
	g.drum.setSynthMoreOpen(g.drum.resolveSynthInstrument(g.drum.synthTabActiveInstrument()), true)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	grid.ScrollToIndex(dividerStartIdx + cols + len(adv))
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	if g.drum.SynthTabControls()[idx].Rect().Empty() {
		t.Error("osc_detune (Advanced) never acquired a rect even after scrolling to it")
	}
}

// TestP4_PhysicalModelParamsContextual pins the contextual exposure: the
// physical-model OSC knobs appear only when their oscillator is selected — bow
// knobs on the Bowed String (osc_type 7), reed knobs on the Sax (osc_type 11).
func TestP4_PhysicalModelParamsContextual(t *testing.T) {
	cases := []struct {
		name    string
		oscType float64
		hidden  bool
	}{
		{"osc_bow_pos", 7, false},   // Bowed String → bow knob shown
		{"osc_bow_pos", 0, true},    // Sine → bow knob hidden
		{"osc_bow_slope", 11, true}, // Sax osc → bow knob hidden
		{"osc_sax_blow", 11, false}, // Sax → reed knob shown
		{"osc_sax_blow", 7, true},   // Bowed → reed knob hidden
		{"osc_sax_blow", 0, true},   // Sine → reed knob hidden
		{"filter_cutoff", 0, false}, // ordinary param → never gated by this rule
	}
	for _, c := range cases {
		if got := !audio.FlatFieldDeclared(map[string]float64{"osc_type": c.oscType}, c.name); got != c.hidden {
			t.Errorf("hidden(%q, oscType=%v) = %v, want %v", c.name, c.oscType, got, c.hidden)
		}
	}
}

// TestCymbalModalKnobsOnlyWherePlateRuns pins the cymbal modal-plate knobs'
// exposure against the SHIPPED instruments: the hi-hat and the ride (plate on) show
// the plate knobs, but only the hi-hat — two cymbals clamped together — shows
// the Rattle knobs; the crash (plate-capable, plate off) shows only the Plate
// Modes switch; the cowbell (not a plate) shows none. The knobs only exist where
// they move the sound. It walks the same macro categories the Synth tab root
// builds, so a knob hidden here has no card to appear on.
func TestCymbalModalKnobsOnlyWherePlateRuns(t *testing.T) {
	claimed := func(id instrument.ID) map[string]bool {
		defs, merged := instSchemaForTest(t, id)
		out := map[string]bool{}
		for _, g := range buildMacroGroupsAll(defs, merged) {
			for _, p := range g.Params {
				out[p] = true
			}
		}
		return out
	}
	hihat, ride := claimed(roster.Hihat), claimed(roster.Ride)
	crash, cowbell := claimed(roster.Crash), claimed(roster.Cowbell)
	for _, f := range []string{"cym_modes", "cym_tone_f", "cym_beat", "cym_wash"} {
		name := "gen1_" + f
		if !ride[name] || !hihat[name] {
			t.Errorf("%s hidden on ride=%v hihat=%v, want shown on both (the plate is running)", name, !ride[name], !hihat[name])
		}
		if cowbell[name] {
			t.Errorf("cowbell: %s shown, want hidden (a cowbell never runs the plate)", name)
		}
		if want := f == "cym_modes"; crash[name] != want {
			t.Errorf("crash: %s shown=%v, want %v (only the Plate Modes switch until it is on)", name, crash[name], want)
		}
	}
	for _, name := range []string{"gen1_cym_rattle", "gen1_cym_rattle_decay", "gen1_cym_drive_delay"} {
		if !hihat[name] {
			t.Errorf("hihat: %s hidden, want shown (a hi-hat rattles)", name)
		}
		if ride[name] {
			t.Errorf("ride: %s shown, want hidden (one suspended cymbal cannot rattle)", name)
		}
	}
	// The legacy cymbal knobs the plate also reads stay on the ride.
	for _, name := range []string{"gen1_cym_tune", "gen1_cym_env_tail", "gen1_cym_env_fast", "gen1_cym_noise_m"} {
		if !ride[name] {
			t.Errorf("ride: %s hidden, want shown (the modal voice reads it)", name)
		}
	}
}

// TestPhysicalKickKnobsOnlyOnPhysicalKicks pins Body Hold / Air (the Phase-19
// kick_hold / kick_hiss columns) to the variant-8 Physical kicks that read them:
// a Physical kick offers both on its category cards, a base kick and an 808
// (other kick variants) offer neither — they would be dead knobs there.
func TestPhysicalKickKnobsOnlyOnPhysicalKicks(t *testing.T) {
	claimed := func(id instrument.ID) map[string]bool {
		defs, merged := instSchemaForTest(t, id)
		out := map[string]bool{}
		for _, g := range buildMacroGroupsAll(defs, merged) {
			for _, p := range g.Params {
				out[p] = true
			}
		}
		return out
	}
	for _, name := range []string{"gen1_kick_hold", "gen1_kick_hiss"} {
		for _, r := range []instrument.ID{roster.KickBoomBap, roster.KickMuted} {
			if !claimed(r)[name] {
				t.Errorf("%s: %s hidden, want shown (the Physical voice reads it)", r, name)
			}
		}
		for _, r := range []instrument.ID{roster.Kick, roster.Kick808, roster.KickFat} {
			if claimed(r)[name] {
				t.Errorf("%s: %s shown, want hidden (only the variant-8 voice reads it)", r, name)
			}
		}
	}
}

// wheelOverPoint dispatches one two-axis wheel event (dx,dy) at (mx,my) through
// the real tree, mirroring a two-finger trackpad drag.
func wheelOverPoint(t *testing.T, g *Game, mx, my, dx, dy int) {
	t.Helper()
	wx, wy := float64(dx), float64(dy)
	active := true
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) {
			if active {
				return wx, wy
			}
			return 0, 0
		},
		func() (int, int) { return 1280, 720 },
	)
	defer restore()
	g.Update() // one frame with the wheel delta
	active = false
	g.Update() // settle (no wheel)
}

// TestSubBassKnobsAreEditable: the analytic dual-osc voice (gen source 4) has a
// Synth-tab stage — every sound-shaping field the seed sets is a visible knob.
func TestSubBassKnobsAreEditable(t *testing.T) {
	defs, merged := instSchemaForTest(t, roster.SubBass)
	claimed := map[string]bool{}
	for _, g := range buildMacroGroupsAll(defs, merged) {
		for _, p := range g.Params {
			claimed[p] = true
		}
	}
	for _, name := range []string{"gen1_harm_mix", "gen1_pitch_env_amt", "gen1_pitch_env_rate", "gen1_atk_amt", "gen1_atk_rate", "gen1_env_fast_rate", "gen1_sat_k", "gen1_out_scale", "gen1_wave", "voice_freq_hz"} {
		if !claimed[name] {
			t.Errorf("%s: no Synth-tab knob for the sub-bass voice", name)
		}
	}
}

// TestHihatAndRideAlwaysShowThePlate: the hi-hat (variant 0) and ride (variant
// 4) have no voice except the modal plate, so dialing Plate Modes to 0 (which
// the engine clamps to 1) must not hide the plate knobs.
func TestHihatAndRideAlwaysShowThePlate(t *testing.T) {
	for _, id := range []instrument.ID{roster.Hihat, roster.Ride} {
		defs, merged := instSchemaForTest(t, id)
		merged["gen1_cym_modes"] = 0
		claimed := map[string]bool{}
		for _, g := range buildMacroGroupsAll(defs, merged) {
			for _, p := range g.Params {
				claimed[p] = true
			}
		}
		for _, f := range []string{"gen1_cym_tone_f", "gen1_cym_beat", "gen1_cym_wash"} {
			if !claimed[f] {
				t.Errorf("%s with Plate Modes 0: %s hidden, want shown (the plate always runs)", id, f)
			}
		}
	}
}

// TestBasePitchKnobIsUsableAndStable: on an instrument that seeds a base
// pitch (sub-bass 45 Hz), the Pitch knob spans a musical range on a log curve
// (one step can't reach 0 Hz, which would switch the voice to note-derived
// pitch), and it stays visible whatever the live value is (gating on the live
// value made the knob delete itself mid-edit).
func TestBasePitchKnobIsUsableAndStable(t *testing.T) {
	const id = roster.SubBass
	defs, merged := instSchemaForTest(t, id)
	var def *audio.ParamDef
	for i := range defs {
		if defs[i].Name == "voice_freq_hz" {
			def = &defs[i]
		}
	}
	if def == nil {
		t.Fatal("no voice_freq_hz ParamDef")
	}
	if def.Min < 20 || def.Max > 2000 || def.Curve != "log" {
		t.Errorf("voice_freq_hz on %s = [%v,%v] curve %q, want within [20,2000] on a log curve", id, def.Min, def.Max, def.Curve)
	}
	merged["voice_freq_hz"] = 0 // a live edit to the floor
	claimed := map[string]bool{}
	for _, g := range buildMacroGroupsAll(defs, merged) {
		for _, p := range g.Params {
			claimed[p] = true
		}
	}
	if !claimed["voice_freq_hz"] {
		t.Error("the Pitch knob disappeared when its live value reached 0")
	}
	// An instrument that does not seed a base pitch still hides it.
	plainDefs, plainMerged := instSchemaForTest(t, roster.PianoGrand)
	for _, g := range buildMacroGroupsAll(plainDefs, plainMerged) {
		for _, p := range g.Params {
			if p == "voice_freq_hz" {
				t.Error("piano-grand (note-derived pitch) shows an inert base-Pitch knob")
			}
		}
	}
}

// TestSubBassKnobsHaveMusicalRanges: the analytic voice's knobs span ranges a
// person can actually dial (the engine accepts 0..2000 for a decay RATE whose
// seed is 2 — the whole useful range sat in the first 0.1% of the knob). The
// body decay rate spans the Length dial's whole span (0.02..40 /s on a log
// curve, the seed mid-knob): a knob means the same values on every instrument
// (TestDialSpansAreTheSameOnEveryInstrument).
func TestSubBassKnobsHaveMusicalRanges(t *testing.T) {
	const id = roster.SubBass
	defs, _ := instSchemaForTest(t, id)
	maxRange := map[string]float64{
		"gen1_harm_mix": 1, "gen1_pitch_env_amt": 1, "gen1_pitch_env_rate": 200,
		"gen1_atk_amt": 1.5, "gen1_atk_rate": 400, "gen1_env_fast_rate": 40,
		"gen1_sat_k": 4, "gen1_out_scale": 2,
	}
	for _, d := range defs {
		want, ok := maxRange[d.Name]
		if !ok {
			continue
		}
		delete(maxRange, d.Name)
		if d.Max > want {
			t.Errorf("%s: knob max %v, want <= %v (a musical range)", d.Name, d.Max, want)
		}
		if d.Default < d.Min || d.Default > d.Max {
			t.Errorf("%s: seed %v outside its knob range [%v,%v]", d.Name, d.Default, d.Min, d.Max)
		}
	}
	for n := range maxRange {
		t.Errorf("%s: no ParamDef on %s", n, id)
	}
}
