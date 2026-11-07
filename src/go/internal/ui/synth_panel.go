package ui

import (
	"strconv"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// synthPanel groups the Synth tab's methods behind a DrumView back-reference; its state stays on DrumView, and
// every method keeps a *DrumView shim. Cross-tab helpers stay on DrumView because non-synth code calls them.
type synthPanel struct {
	dv *DrumView // back-reference

	// Released on tab-hide through EQCallbacks.ReleaseSynthSprites.
	sprites synthSpriteCache

	// Rebuilt only when graphPlotKey (renders, focus, size) changes.
	graphPlot    *viewPlot
	graphPlotKey synthGraphKey
	viewMemo     dialViewMemo // the view chosen for the focused control's ends
	nowKey       nowSoundMemo // the live document's sound key (per revision)
	glow         glowLayer    // the trigger glow, baked once per picture
	// Per (revision, control) so a frame does not rebuild them.
	graphEnds graphEndsMemo

	// Mobile sticky-bar actions, built once and refreshed each frame; desktop keeps its header buttons.
	mobilePreviewBtn  *Button
	mobileSaveAsBtn   *Button
	mobileOverflowBtn *Button

	// Cached across Layouts so steady frames allocate nothing; pipeBtns is keyed by section (-1 = the "+" pill).
	pipeBtns map[synthSectionID]*Button
	pipeOn   []int // activePipelineStages scratch (reused per Layout)
	pipeOff  []int
	backBtn  *Button
	moreBtn  *Button
}

// Id + document revision: any change bumps the revision, so the per-frame gate is O(1) instead of hashing the
// whole schema every frame (which caused a GC-churn hang).
func (p *synthPanel) synthParamsHash(instID instrument.ID) string {
	return string(instID) + ":" + strconv.FormatUint(audio.InstrumentDocRev(instID), 10)
}

func (dv *DrumView) synthParamsHash(instID instrument.ID) string {
	return dv.synp().synthParamsHash(instID)
}

// Lazy for DrumViews built as bare literals in tests, which skip the ctor and would nil-panic otherwise.
func (dv *DrumView) synp() *synthPanel {
	if dv.synthPanel == nil {
		dv.synthPanel = &synthPanel{dv: dv}
	}
	return dv.synthPanel
}
