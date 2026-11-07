package ui

import (
	"image"
	"sync/atomic"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// recalcButtons runs every frame; these gates skip a forced layout whose inputs did not move.

// An export can change any UI state between frames, so a gated layout re-runs after one.
var jsExportSeq atomic.Uint64

// Every input TimelineZone.Layout reads, so an equal key lays out identically.
type timelineLayoutKey struct {
	zone, bar, beatCounter, track, lenDec, lenInc image.Rectangle
	barH                                          int
}

// Bounds how long a change that neither moves eqLayoutKey nor arrives as input can stay stale.
const eqPanelRelayoutEvery = 8

// Panel state that can change without input to the panel itself.
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

// The editor tabs lay out every frame because they read live instrument state; analysis tabs
// only when the key moves, after input or a bridge call, or every eqPanelRelayoutEvery frames.
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
