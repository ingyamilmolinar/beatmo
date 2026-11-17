package ui

import (
	"image"
	"sync/atomic"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// recalcButtons runs every frame and used to force a full re-layout of the
// timeline and audio-panel zones every frame too — on WASM the panel's alone
// was ~10% of Update while nothing on screen changed. These gates skip a
// forced layout whose inputs did not move.

// jsExportSeq counts bridge calls (jsFn): an export can change any UI state
// between frames, so a gated layout re-runs after one.
var jsExportSeq atomic.Uint64

// timelineLayoutKey is every input TimelineZone.Layout reads — all of it set
// by recalcButtons just before — so an equal key lays out identically.
type timelineLayoutKey struct {
	zone, bar, beatCounter, track, lenDec, lenInc image.Rectangle
	barH                                          int
}

// eqPanelRelayoutEvery bounds how long an audio-panel change that neither
// moves eqLayoutKey nor arrives as input can leave the layout stale: the
// panel is laid out at least every this many frames.
const eqPanelRelayoutEvery = 8

// eqLayoutKey is the audio panel's layout state that can change without input
// to the panel itself: its rect and tab, the profile, theme, font and locale
// (labels), and the channel pill, which follows the selected row and its name.
type eqLayoutKey struct {
	rect              image.Rectangle
	tab               PanelTab
	mobile            bool
	density           Density
	theme             int
	font              int64
	locale            i18n.Locale
	pill              string
	synthOff, sampOff bool
	jsSeq             uint64
}

type eqLayoutGate struct {
	key         eqLayoutKey
	have        bool
	sinceForced int
}

// timelineLayoutDue reports whether the timeline zone must be laid out: its
// inputs moved or it asked for a layout.
func (dv *DrumView) timelineLayoutDue(zoneRect image.Rectangle) bool {
	z := dv.timelineZone
	key := timelineLayoutKey{
		zone: zoneRect, bar: dv.timelineRect, beatCounter: dv.beatCounterRect,
		track: dv.trackBtn().Rect(), lenDec: dv.lenDecBtn.Rect(), lenInc: dv.lenIncBtn.Rect(),
		barH: tlBarHeight(),
	}
	if dv.timelineLayoutHave && key == dv.timelineLayoutKey && !z.NeedsLayout() {
		return false
	}
	dv.timelineLayoutKey, dv.timelineLayoutHave = key, true
	return true
}

// eqPanelLayoutDue reports whether the audio panel must be laid out this
// frame. The editor tabs (Synth, Sampler, Chain) lay out every frame: their
// layout reads live instrument / stage state. The analysis tabs re-lay out
// when eqLayoutKey moves, after the panel handled input (its pills and
// toggles change it), after a bridge call, and at least every
// eqPanelRelayoutEvery frames.
func (dv *DrumView) eqPanelLayoutDue() bool {
	z := dv.eqPanelZone
	tab := z.ActiveTab()
	p := Profile()
	key := eqLayoutKey{
		rect: dv.eqRect, tab: tab, mobile: p.IsMobile(), density: p.Density(),
		theme: ThemeGeneration(), font: i18n.FontGeneration(), locale: i18n.ActiveLocale(),
		pill: z.channelPillLabel(), synthOff: z.synthTabDisabled(), sampOff: z.samplerTabDisabled(),
		jsSeq: jsExportSeq.Load(),
	}
	g := &dv.eqLayoutGate
	due := !g.have || key != g.key || z.NeedsLayout() || g.sinceForced+1 >= eqPanelRelayoutEvery ||
		tab == TabSynth || tab == TabSampler || tab == TabScope ||
		dv.audioTree.handledInputLastFrame()
	if due {
		g.key, g.have, g.sinceForced = key, true, 0
	} else {
		g.sinceForced++
	}
	return due
}
