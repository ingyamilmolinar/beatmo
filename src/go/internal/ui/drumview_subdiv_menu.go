package ui

import (
	"fmt"
)

// buildSubdivMenu populates dv.subdivMenuBtns — the legacy hit-rect mirror used
// by JS exports (subdivMenuItemRects / applySubdivValue) and tests. The actual
// menu surface is rendered by dv.subdivMenuComp (drumview_overlay_subdiv_comp.go),
// which positions its card via the shared AnchorPopupRect primitive. To keep the
// reported rects in sync with what the user sees, this mirror reuses the
// component's resolved button rects instead of recomputing a fixed offset (the
// old fixed math placed the rects ~250px below the anchor over the EQ panel).
func (dv *DrumView) buildSubdivMenu() {
	dv.subdivMenuBtns = dv.subdivMenuBtns[:0]
	if dv.subdivMenuComp == nil {
		return
	}
	for _, src := range dv.subdivMenuComp.Buttons() {
		v, _ := atoiSafe(src.Text)
		vv := v
		text := src.Text
		btn := NewButton(text, src.Style, func() {
			if dv.onChangeSubdiv != nil {
				if err := dv.onChangeSubdiv(vv); err != nil {
					return
				}
			}
			dv.subdivBtn().Text = fmt.Sprintf("÷%d", vv)
			dv.timelineUnitsPerBeat = vv
			dv.closeSubdivMenuPortal()
		})
		btn.ConsumeOnPress = true
		btn.SetRect(src.Rect())
		dv.subdivMenuBtns = append(dv.subdivMenuBtns, btn)
	}
}

// atoiSafe parses a base-10 int, returning 0 on error (the subdiv labels are
// always small positive integers, so this never legitimately fails).
func atoiSafe(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return n, fmt.Errorf("non-digit %q", c)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
