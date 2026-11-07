package ui

import (
	"fmt"
)

// Mirrors the component's resolved button rects instead of recomputing them, so JS exports and
// tests report what the user sees.
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
