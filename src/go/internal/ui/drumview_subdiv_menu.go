package ui

import (
	"fmt"
	"image"
)

func (dv *DrumView) buildSubdivMenu() {
	dv.subdivMenuBtns = dv.subdivMenuBtns[:0]
	vals := []int{4, 8, 16, 32}
	base := dv.subdivBtn().Rect()
	for i, v := range vals {
		r := image.Rect(base.Min.X, base.Max.Y+i*dv.rowHeight(), base.Max.X, base.Max.Y+(i+1)*dv.rowHeight())
		text := fmt.Sprintf("%d", v)
		vv := v
		btn := NewButton(text, DropdownStyle, func() {
			if dv.onChangeSubdiv != nil {
				if err := dv.onChangeSubdiv(vv); err != nil {
					return
				}
			}
			dv.subdivBtn().Text = text
			dv.timelineUnitsPerBeat = vv
			dv.closeSubdivMenuPortal()
		})
		btn.ConsumeOnPress = true
		btn.SetRect(insetRect(r, buttonPad))
		dv.subdivMenuBtns = append(dv.subdivMenuBtns, btn)
	}
}
