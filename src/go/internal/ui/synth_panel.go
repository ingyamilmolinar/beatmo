package ui

import (
	"strconv"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// synthPanel is the Synth audio-panel tab, extracted from the DrumView
// God-object (Phase 2b). It mirrors samplerPanel (see sampler_panel.go): the
// synth methods are grouped onto this collaborator via a `dv *DrumView`
// back-reference, and every relocated method keeps a one-line *DrumView
// forwarding shim so all 262 existing synth tests drive it through DrumView
// unchanged. Synth state fields stay on DrumView for now (Rung-A cohesion move,
// zero behavior change).
//
// Cross-tab helpers deliberately stay on DrumView (NOT relocated here) because
// non-synth code calls them: knobStepPref, persistKnobStep, masterChannelSelected,
// samplerViewDisabled, synthViewDisabled, activeInstrumentHasSynth. The synth
// methods reach them via the dv back-reference.
type synthPanel struct {
	dv *DrumView // back-reference

	// sprites holds the Synth tab's TraceSprite render caches (Task 2 of the
	// render-caching plan, synth_sprite_cache.go): the cards and the graph
	// blit a cached texture on steady frames instead of re-walking their pixel
	// loops. Released on tab-hide via EQCallbacks.ReleaseSynthSprites
	// (releaseHiddenTabSprites chokepoint in eq_panel_zone.go).
	sprites synthSpriteCache

	// graphPlot is the graph's built geometry, rebuilt only when graphPlotKey
	// (the renders, the focus, the size) changes (synth_graph_pane.go).
	graphPlot    *viewPlot
	graphPlotKey synthGraphKey
	viewMemo     dialViewMemo // the view chosen for the focused control's ends
	nowKey       nowSoundMemo // the live document's sound key (per revision)
	glow         glowLayer    // the trigger glow, baked once per picture
	// graphEnds memoises the focused control's two end documents per
	// (document revision, control) so a frame does not rebuild them.
	graphEnds graphEndsMemo

	// Persistent mobile action buttons hosted on the sticky bar (Approach B):
	// [Preview][Save][⋯]. Built once (ensureMobileActionButtons), refreshed each
	// frame (label/dirty-spec) by MobileActionButtons. The ⋯ button opens the
	// shared actionMenu (Save As / Reset). These replace the in-panel action
	// row on mobile; desktop keeps its header buttons.
	mobilePreviewBtn  *Button
	mobileSaveAsBtn   *Button
	mobileOverflowBtn *Button

	// Category-layer buttons, cached across Layouts (rects/handlers refreshed
	// every Layout, allocation-free on steady frames): the pipeline strip's
	// stage pills keyed by section id (-1 = the "+" pill), the drilled view's
	// Back button and the More divider's input button.
	pipeBtns map[synthSectionID]*Button
	pipeOn   []int // activePipelineStages scratch (reused per Layout)
	pipeOff  []int
	backBtn  *Button
	moreBtn  *Button
}

// synthParamsHash returns a stable identity for the instrument's document: its
// id plus the document revision. Any change to the document bumps the
// revision, so identical values produce an identical hash — and the per-frame
// gate is O(1) instead of merging + sorting + formatting the ~1030-param
// modular schema every frame (the Synth-tab GC-churn hang; see
// synth_live_param_edit_alloc_test.go).
func (p *synthPanel) synthParamsHash(instID instrument.ID) string {
	return string(instID) + ":" + strconv.FormatUint(audio.InstrumentDocRev(instID), 10)
}

// synp returns the DrumView's synth panel, lazily creating it if absent. The
// ctor sets dv.synthPanel eagerly; this getter is the self-healing path for the
// many test files that build *DrumView via a bare &DrumView{} literal bypassing
// the ctor. Every synth forwarding shim calls dv.synp().X() so those tests (and
// any future bare-literal construction) never nil-panic on a relocated method.
func (dv *DrumView) synp() *synthPanel {
	if dv.synthPanel == nil {
		dv.synthPanel = &synthPanel{dv: dv}
	}
	return dv.synthPanel
}
