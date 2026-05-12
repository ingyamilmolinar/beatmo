package ui

import (
	"image"
)

// Subject identifies a UI surface that the screenshot harness can crop to.
// SubjectFullScreen (the empty string) is the default and means "no crop —
// capture the entire framebuffer", preserving legacy behavior for every
// existing scene that does not opt in.
//
// New subjects are added by:
//  1. defining a const here,
//  2. adding a case in (*Game).SubjectRect that returns its bounds,
//  3. exposing the bounds publicly on the owning component if it isn't
//     already (the rule: never reach into a private field from this file —
//     promote a getter instead).
type Subject string

const (
	SubjectFullScreen     Subject = ""
	SubjectMainGrid       Subject = "main_grid"
	SubjectDrumView       Subject = "drum_view" // entire drum pane (header + rows + EQ panel)
	SubjectDrumRows       Subject = "drum_rows" // just the row scroll area, excludes header + EQ
	SubjectEQPanel        Subject = "eq_panel"
	SubjectEQTabEQ        Subject = "eq_tab_eq"
	SubjectEQTabWave      Subject = "eq_tab_wave"
	SubjectEQTabSpectrum  Subject = "eq_tab_spectrum"
	SubjectEQTabMeters    Subject = "eq_tab_meters"
	SubjectChain          Subject = "scope"
	SubjectToolbar        Subject = "toolbar" // full top header strip (transport+BPM+vol+overflow)
	SubjectFXPanel        Subject = "fx_panel"
	SubjectContextMenu    Subject = "context_menu"
	SubjectOverflowMenu   Subject = "overflow_menu"
	SubjectInstrumentMenu Subject = "instrument_menu"
)

// AllSubjects returns every named subject in declaration order. Used by
// JS bridge enumeration and tests. Excludes SubjectFullScreen because that
// is the absence-of-crop, not a surface.
func AllSubjects() []Subject {
	return []Subject{
		SubjectMainGrid,
		SubjectDrumView,
		SubjectDrumRows,
		SubjectEQPanel,
		SubjectEQTabEQ,
		SubjectEQTabWave,
		SubjectEQTabSpectrum,
		SubjectEQTabMeters,
		SubjectChain,
		SubjectToolbar,
		SubjectFXPanel,
		SubjectContextMenu,
		SubjectOverflowMenu,
		SubjectInstrumentMenu,
	}
}

// SubjectRect returns the screen-space bounding rectangle of the requested
// surface at the current laid-out frame. The second return is false when
// the subject is not currently visible (e.g. the FX panel when no row has
// it open, or the EQ panel when the mobile profile has it collapsed) — the
// caller should skip cropping and report a clear error in that case.
//
// The returned rect is always clipped to the framebuffer (0,0)-(winW,winH)
// so a too-large widget rect cannot escape the screen and produce a
// negative-sized or out-of-bounds crop.
//
// Callers must ensure layout has happened (one Layout()+Draw() cycle, or
// the SettleFrames countdown that the screenshot harness performs) before
// calling — the rects below are populated by Layout/Draw and are zero on a
// freshly-constructed Game.
func (g *Game) SubjectRect(s Subject) (image.Rectangle, bool) {
	if s == SubjectFullScreen {
		return image.Rect(0, 0, g.winW, g.winH), true
	}
	dv := g.drum
	full := image.Rect(0, 0, g.winW, g.winH)
	clamp := func(r image.Rectangle) (image.Rectangle, bool) {
		if r.Empty() {
			return image.Rectangle{}, false
		}
		c := r.Intersect(full)
		if c.Empty() {
			return image.Rectangle{}, false
		}
		return c, true
	}
	switch s {
	case SubjectMainGrid:
		if g.split == nil || g.winW == 0 || g.winH == 0 {
			return image.Rectangle{}, false
		}
		return clamp(g.split.GridRect(g.winW, g.winH))
	case SubjectDrumView:
		if dv == nil {
			return image.Rectangle{}, false
		}
		return clamp(dv.Bounds)
	case SubjectDrumRows:
		if dv == nil {
			return image.Rectangle{}, false
		}
		return clamp(dv.RowsRect())
	case SubjectEQPanel:
		if dv == nil {
			return image.Rectangle{}, false
		}
		return clamp(dv.eqRect)
	case SubjectEQTabEQ, SubjectEQTabWave, SubjectEQTabSpectrum, SubjectEQTabMeters:
		if dv == nil || dv.eqPanelZone == nil {
			return image.Rectangle{}, false
		}
		// Each EQ tab paints into the panel's content rect; the visual
		// difference is which renderer runs. We crop to the full panel
		// (including the tab header strip) so the captured PNG shows the
		// active-tab affordance, then clamp to the screen.
		return clamp(dv.eqRect)
	case SubjectChain:
		if dv == nil || dv.eqPanelZone == nil {
			return image.Rectangle{}, false
		}
		sz := dv.eqPanelZone.ChainZone()
		if sz == nil {
			return image.Rectangle{}, false
		}
		return clamp(sz.Rect())
	case SubjectToolbar:
		if dv == nil {
			return image.Rectangle{}, false
		}
		// Full top header strip — spans the entire drum-pane width and
		// includes transport cluster, BPM, length controls, master volume,
		// overflow, etc. Falling back to the bare WidgetTransport rect
		// would only capture the play/stop/record cluster.
		return clamp(dv.HeaderRect())
	case SubjectFXPanel:
		if dv == nil {
			return image.Rectangle{}, false
		}
		r, ok := dv.FXPanelRect()
		if !ok {
			return image.Rectangle{}, false
		}
		return clamp(r)
	case SubjectContextMenu:
		if dv == nil || !dv.IsContextMenuOpen() {
			return image.Rectangle{}, false
		}
		return clamp(dv.contextMenuRect)
	case SubjectOverflowMenu:
		if dv == nil || !dv.IsOverflowMenuOpen() {
			return image.Rectangle{}, false
		}
		return clamp(dv.OverflowPopupRect())
	case SubjectInstrumentMenu:
		if dv == nil || !dv.IsInstMenuOpen() {
			return image.Rectangle{}, false
		}
		return clamp(dv.InstrumentMenuRect())
	}
	return image.Rectangle{}, false
}

// SubjectByName parses a Subject from its string form. The empty string
// resolves to SubjectFullScreen. Unknown names return ok=false.
func SubjectByName(name string) (Subject, bool) {
	if name == "" {
		return SubjectFullScreen, true
	}
	for _, s := range AllSubjects() {
		if string(s) == name {
			return s, true
		}
	}
	return "", false
}
