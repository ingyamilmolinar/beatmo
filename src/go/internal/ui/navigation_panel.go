package ui

import (
	"image"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
)

// NavPanelCallbacks wires NavigationPanel's read-only state accessors and
// control actions to the owning component (the overflow menu's NAVIGATION
// section). All fields are optional; nil accessors/actions are treated as
// no-ops so the panel can be exercised in isolation (tests).
type NavPanelCallbacks struct {
	PlayheadStep  func() int
	BPM           func() int
	Div           func() int
	TimelineBeats func() int
	JumpAmount    func() int
	JumpUnitBeats func() bool
	OnPrev        func()
	OnNext        func()
	OnPickAmount  func()
	OnToggleUnit  func()
	OnGotoBeat    func()
	OnGotoTime    func()
}

// NavigationPanel is a self-contained component rendered as the reserved top
// band of the sectioned overflow ("Options") menu. It reuses the app's design-
// system controls so it matches every other menu/panel: a jog row of styled
// DropdownStyle keycaps (prev · ×N amount · next), a SegmentedControl for the
// Beats/Subdiv unit, and two drawMenuRow rows ("Go to beat" / "Go to time")
// identical in chrome to the File-section rows below it. It owns no navigation
// logic — every action is delegated via NavPanelCallbacks.
type NavigationPanel struct {
	cb      NavPanelCallbacks
	rect    image.Rectangle
	readout image.Rectangle

	prevBtn   *Button
	amountBtn *Button
	nextBtn   *Button
	unitSeg   *SegmentedControl
	gotoBeat  *Button
	gotoTime  *Button

	// segArmed latches the SegmentedControl press so a held pointer fires its
	// tap exactly once (SegmentedControl.HitTest has no edge detection of its
	// own, and the menu re-delivers pressed==true every frame while held).
	segArmed bool
}

// SetCallbacks assigns (or reassigns) the panel's callbacks. Controls are
// constructed lazily on first call so repeated calls (re-wiring live callbacks)
// don't churn control identity.
func (p *NavigationPanel) SetCallbacks(cb NavPanelCallbacks) {
	p.cb = cb
	if p.prevBtn == nil {
		p.prevBtn = NewButton("", nil, func() { p.call(p.cb.OnPrev) })
		p.prevBtn.Icon = string(IconRewind)
		p.prevBtn.Style = DropdownStyle
		p.amountBtn = NewButton("", nil, func() { p.call(p.cb.OnPickAmount) })
		p.amountBtn.Style = DropdownStyle
		p.nextBtn = NewButton("", nil, func() { p.call(p.cb.OnNext) })
		p.nextBtn.Icon = string(IconFastForward)
		p.nextBtn.Style = DropdownStyle
		// Segment 0 = Beats (unit=true), 1 = Subdiv (unit=false). OnToggleUnit
		// flips the live flag, so fire it only when the tapped segment differs
		// from the current unit (Layout re-syncs active from the source of truth).
		p.unitSeg = NewSegmentedControl([]string{"Beats", "Subdiv"}, 0, func(i int) {
			wantBeats := i == 0
			if p.cb.JumpUnitBeats != nil && p.cb.JumpUnitBeats() != wantBeats {
				p.call(p.cb.OnToggleUnit)
			}
		})
		p.gotoBeat = NewButton("", nil, func() { p.call(p.cb.OnGotoBeat) })
		p.gotoTime = NewButton("", nil, func() { p.call(p.cb.OnGotoTime) })
	}
}

func (p *NavigationPanel) call(f func()) {
	if f != nil {
		f()
	}
}

// PanelHeight returns the band's fixed total height: readout + jog row + unit
// row + go-to-beat row + go-to-time row, each touchMinTargetPx tall.
func (p *NavigationPanel) PanelHeight() int { return 5 * touchMinTargetPx }

// Layout positions the five rows top-to-bottom inside rect and refreshes the
// live amount label + unit selection from the source of truth.
func (p *NavigationPanel) Layout(rect image.Rectangle) {
	p.rect = rect
	rowH := touchMinTargetPx
	x0, w := rect.Min.X, rect.Dx()
	y := rect.Min.Y

	// Row 1 — position readout (left-aligned text + leading icon, drawn in
	// Draw). Left alignment keeps the popup's top-right close-× clear.
	p.readout = image.Rect(x0, y, x0+w, y+rowH)
	y += rowH

	// Row 2 — jog: [◀◀] [ ×N ] [▶▶] as a CENTERED cluster (bounded, not edge-to-
	// edge) so it reads as a unit on both the narrow desktop popup and the wide
	// mobile sheet, and never collides with the close-× (which is row 1 only).
	if p.prevBtn != nil {
		sq := rowH - 2*SpaceSM // square keycap, vertically inset
		amtW := 3 * sq         // amount pill width; clamped to fit narrow popups
		if maxAmt := w - 2*SpaceMD - 2*(sq+SpaceSM); amtW > maxAmt {
			amtW = maxAmt
		}
		groupW := sq + SpaceSM + amtW + SpaceSM + sq
		gx := x0 + (w-groupW)/2
		gy := y + SpaceSM
		p.prevBtn.SetRect(image.Rect(gx, gy, gx+sq, gy+sq))
		ax := gx + sq + SpaceSM
		p.amountBtn.SetRect(image.Rect(ax, gy, ax+amtW, gy+sq))
		nx := ax + amtW + SpaceSM
		p.nextBtn.SetRect(image.Rect(nx, gy, nx+sq, gy+sq))
	}
	y += rowH

	// Row 3 — unit toggle (segmented control), inset like the jog.
	if p.unitSeg != nil {
		p.unitSeg.SetRect(image.Rect(x0+SpaceMD, y+SpaceSM, x0+w-SpaceMD, y+rowH-SpaceSM))
	}
	y += rowH

	// Rows 4 & 5 — full-width menu rows (drawn via drawMenuRow in Draw).
	if p.gotoBeat != nil {
		p.gotoBeat.SetRect(image.Rect(x0, y, x0+w, y+rowH))
	}
	y += rowH
	if p.gotoTime != nil {
		p.gotoTime.SetRect(image.Rect(x0, y, x0+w, y+rowH))
	}

	// Live labels reflecting the current jump amount + unit.
	if p.amountBtn != nil && p.cb.JumpAmount != nil {
		p.amountBtn.Text = "×" + strconv.Itoa(p.cb.JumpAmount())
	}
	if p.unitSeg != nil && p.cb.JumpUnitBeats != nil {
		if p.cb.JumpUnitBeats() {
			p.unitSeg.SetActive(0)
		} else {
			p.unitSeg.SetActive(1)
		}
	}
}

// GotoBeatRect / GotoTimeRect / AmountRect expose control bounds so the owning
// DrumView can anchor the shared position editor / jump-size picker over the
// control that triggered it (replaces index-based Buttons() lookups).
func (p *NavigationPanel) GotoBeatRect() image.Rectangle {
	if p.gotoBeat == nil {
		return image.Rectangle{}
	}
	return p.gotoBeat.Rect()
}

func (p *NavigationPanel) GotoTimeRect() image.Rectangle {
	if p.gotoTime == nil {
		return image.Rectangle{}
	}
	return p.gotoTime.Rect()
}

func (p *NavigationPanel) AmountRect() image.Rectangle {
	if p.amountBtn == nil {
		return image.Rectangle{}
	}
	return p.amountBtn.Rect()
}

// Buttons returns the panel's *Button controls (NOT the SegmentedControl) so
// the overflow menu's hoverFn can drive their hover-glow every frame.
func (p *NavigationPanel) Buttons() []*Button {
	return []*Button{p.prevBtn, p.amountBtn, p.nextBtn, p.gotoBeat, p.gotoTime}
}

// beatOfPlayhead returns the current whole-beat index (playhead step / div).
func (p *NavigationPanel) beatOfPlayhead() int {
	if p.cb.PlayheadStep == nil || p.cb.Div == nil {
		return 0
	}
	div := p.cb.Div()
	if div <= 0 {
		div = 1
	}
	return p.cb.PlayheadStep() / div
}

// Draw renders the position readout, the jog keycaps, the unit segmented
// control, and the two go-to menu rows (current beat/time as trailing caption).
func (p *NavigationPanel) Draw(dst *ebiten.Image) {
	// Row 1 — position readout: leading target icon + "Beat N · M:SS".
	if p.cb.PlayheadStep != nil && p.cb.Div != nil && p.cb.BPM != nil {
		icSz := IconSizeSM
		iy := p.readout.Min.Y + (p.readout.Dy()-icSz)/2
		DrawIcon(dst, IconTarget, image.Rect(p.readout.Min.X+SpaceMD, iy, p.readout.Min.X+SpaceMD+icSz, iy+icSz), colMenuIcon)
		txt := formatBeatTime(p.cb.PlayheadStep(), p.cb.Div(), p.cb.BPM())
		th := StyledTextHeight(RolePanelTitle)
		ty := p.readout.Min.Y + (p.readout.Dy()-th)/2
		DrawTextStyled(dst, txt, p.readout.Min.X+SpaceMD+icSz+SpaceSM, ty, RolePanelTitle, colTextPrimary)
	}
	if p.prevBtn != nil {
		p.prevBtn.Draw(dst)
		p.amountBtn.Draw(dst)
		p.nextBtn.Draw(dst)
	}
	if p.unitSeg != nil {
		p.unitSeg.Draw(dst)
	}
	// Go-to rows: current position shown as the trailing caption, a chevron
	// signals the row opens an editor — same chrome as the File-section rows.
	beatTrail := strconv.Itoa(p.beatOfPlayhead())
	timeTrail := ""
	if p.cb.PlayheadStep != nil && p.cb.Div != nil && p.cb.BPM != nil {
		div := p.cb.Div()
		if div <= 0 {
			div = 1
		}
		bpm := p.cb.BPM()
		if bpm <= 0 {
			bpm = 120
		}
		timeTrail = formatClock((p.beatOfPlayhead() * 60) / bpm)
	}
	p.drawGotoRow(dst, p.gotoBeat, "Go to beat", beatTrail)
	p.drawGotoRow(dst, p.gotoTime, "Go to time", timeTrail)
}

func (p *NavigationPanel) drawGotoRow(dst *ebiten.Image, btn *Button, label, trail string) {
	if btn == nil {
		return
	}
	state := menuItemRest
	if btn.hovered || btn.pressed {
		state = menuItemHover
	}
	drawMenuRow(dst, btn, MenuRowSpec{
		Accent:      colAccent,
		State:       state,
		Label:       label,
		TrailLabel:  trail,
		TrailIconID: IconChevronRight,
	})
}

// HandleInput dispatches a pointer event to the unit segmented control (press-
// edge latched) then to each *Button via Button.HandleInputResult (which owns
// edge-firing, telemetry, press animation). Returns the first non-Ignored
// result; InputIgnored when nothing in the band claims the point.
func (p *NavigationPanel) HandleInput(x, y int, pressed bool) InputResult {
	if p.unitSeg != nil && image.Pt(x, y).In(p.unitSeg.Rect()) {
		if pressed {
			if !p.segArmed {
				p.segArmed = true
				p.unitSeg.HitTest(x, y) // fire the segment tap once, on the press edge
			}
		} else {
			p.segArmed = false
		}
		return InputConsumed
	}
	if !pressed {
		p.segArmed = false
	}
	for _, b := range p.Buttons() {
		if b == nil {
			continue
		}
		if res := b.HandleInputResult(x, y, pressed); res != InputIgnored {
			return res
		}
	}
	return InputIgnored
}
