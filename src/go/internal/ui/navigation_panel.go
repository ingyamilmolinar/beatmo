package ui

import (
	"image"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
)

// Every field is optional (nil = no-op) so the panel can be tested in isolation.
type NavPanelCallbacks struct {
	// Position is the ONLY source of the position title; PlayheadStep (0-based) is where the go-to wheel opens.
	Position      func() PlayheadPos
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
	// Scrolling never seeks; only Go does.
	OnGotoPreview func(steps int, on bool)
	// OnGoto seeks to an absolute subdivision step (Go / Enter / centre click).
	OnGoto func(steps int)
}

// Owns no seek logic: every action is delegated, and the go-to wheel only PREVIEWS until Go.
type NavigationPanel struct {
	cb      NavPanelCallbacks
	rect    image.Rectangle
	readout image.Rectangle
	// The menu's title band shows the readout instead; the row keeps its height.
	HideReadout bool
	// One row on the wide desktop card, so it fits under the ribbon and never covers the go-to ghost.
	SharedJogRow bool

	prevBtn   *Button
	amountBtn *Button
	nextBtn   *Button
	unitSeg   *SegmentedControl

	// GO TO section.
	modeSeg     *SegmentedControl // 0 = Beat, 1 = Time
	goBtn       *Button
	wheel       ValueWheel
	beatModel   *NumberModel // absolute subdivision steps
	timeModel   *NumberModel // whole seconds
	timeMode    bool
	seated      bool
	seatedDiv   int
	seatedMax   int
	previewing  bool
	captionRect image.Rectangle
	wheelRect   image.Rectangle
	footerRect  image.Rectangle

	// SegmentedControl.HitTest has no edge detection and the menu re-delivers pressed every held frame, so a held
	// press must fire once.
	segArmed  bool
	modeArmed bool
}

// Controls are built lazily on the first call so re-wiring never churns their identity.
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
		// OnToggleUnit flips the flag, so fire only when the tapped segment differs from the current unit.
		p.unitSeg = NewSegmentedControl([]string{"Beats", "Subdiv"}, 0, func(i int) {
			wantBeats := i == 0
			if p.cb.JumpUnitBeats != nil && p.cb.JumpUnitBeats() != wantBeats {
				p.call(p.cb.OnToggleUnit)
			}
		})
		p.modeSeg = NewSegmentedControl([]string{"Beat", "Time"}, 0, func(i int) {
			p.SetTimeMode(i == 1)
		})
		p.goBtn = NewSpecButton("Go", ComponentButtonPrimary, func() { p.Go() })
	}
}

func (p *NavigationPanel) call(f func()) {
	if f != nil {
		f()
	}
}

func (p *NavigationPanel) PanelHeight() int {
	rows := 5
	if p.SharedJogRow {
		rows = 4
	}
	return rows*touchMinTargetPx + gotoWheelHeight()
}

// DESIGN.md menu sizing: wheels show five rows.
const gotoWheelRows = 5

// gotoWheelHeight is the wheel's height: five row pitches plus a vertical inset.
func gotoWheelHeight() int {
	gap := Profile().DensityValues().MobileWheelTickGap
	if gap <= 0 {
		gap = 34
	}
	return gotoWheelRows*gap + 2*SpaceSM
}

func (p *NavigationPanel) Layout(rect image.Rectangle) {
	p.rect = rect
	rowH := touchMinTargetPx
	x0, w := rect.Min.X, rect.Dx()
	y := rect.Min.Y

	// Left-aligned so the close × stays clear.
	p.readout = image.Rect(x0, y, x0+w, y+rowH)
	y += rowH

	if p.SharedJogRow && p.prevBtn != nil {
		// Row 2 (shared) — jog cluster left, Beats/Subdiv toggle right.
		sq := rowH - 2*SpaceSM
		amtW := 2 * sq
		gx, gy := x0+SpaceMD, y+SpaceSM
		p.prevBtn.SetRect(image.Rect(gx, gy, gx+sq, gy+sq))
		ax := gx + sq + SpaceSM
		p.amountBtn.SetRect(image.Rect(ax, gy, ax+amtW, gy+sq))
		nx := ax + amtW + SpaceSM
		p.nextBtn.SetRect(image.Rect(nx, gy, nx+sq, gy+sq))
		if p.unitSeg != nil {
			p.unitSeg.SetRect(image.Rect(nx+sq+SpaceMD, gy, x0+w-SpaceMD, gy+sq))
		}
		y += rowH
	} else {
		y = p.layoutJogRows(x0, y, w, rowH)
	}
	p.layoutGoto(x0, y, w, rowH)
}

// The mobile sheet's two-row layout.
func (p *NavigationPanel) layoutJogRows(x0, y, w, rowH int) int {
	// A centred, bounded cluster so it reads as a unit on the wide sheet.
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
	return y + rowH
}

// layoutGoto lays the GO TO section from y down and refreshes the live labels.
func (p *NavigationPanel) layoutGoto(x0, y, w, rowH int) {
	// GO TO header: section caption left, Beat|Time tabs right.
	p.captionRect = image.Rect(x0+SpaceMD, y, x0+w/2, y+rowH)
	if p.modeSeg != nil {
		segW := w / 2
		if segW > 180 {
			segW = 180
		}
		p.modeSeg.SetRect(image.Rect(x0+w-SpaceMD-segW, y+SpaceSM, x0+w-SpaceMD, y+rowH-SpaceSM))
	}
	y += rowH

	// The wheel (barrel + step ladder).
	p.wheelRect = image.Rect(x0+SpaceMD, y, x0+w-SpaceMD, y+gotoWheelHeight())
	y += gotoWheelHeight()

	// Footer: secondary readout left, primary Go right.
	p.footerRect = image.Rect(x0+SpaceMD, y, x0+w-SpaceMD, y+rowH)
	if p.goBtn != nil {
		goW := 2 * rowH
		p.goBtn.SetRect(image.Rect(p.footerRect.Max.X-goW, y+SpaceSM, p.footerRect.Max.X, y+rowH-SpaceSM))
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
	if p.modeSeg != nil {
		if p.timeMode {
			p.modeSeg.SetActive(1)
		} else {
			p.modeSeg.SetActive(0)
		}
	}
	p.syncGotoModels()
	p.wheel.Layout(p.wheelRect)
}

// ── GO TO ────────────────────────────────────────────────────────────────

func (p *NavigationPanel) div() int {
	if p.cb.Div != nil {
		if d := p.cb.Div(); d > 0 {
			return d
		}
	}
	return 1
}

func (p *NavigationPanel) bpm() int {
	if p.cb.BPM != nil {
		if b := p.cb.BPM(); b > 0 {
			return b
		}
	}
	return 120
}

func (p *NavigationPanel) maxSteps() int {
	beats := 0
	if p.cb.TimelineBeats != nil {
		beats = p.cb.TimelineBeats()
	}
	if beats < 0 {
		beats = 0
	}
	return beats * p.div()
}

// Called on open; the mode and step rungs persist for the session.
func (p *NavigationPanel) ResetGoto() {
	p.seated = false
	p.syncGotoModels()
}

// Rebuilt when unseated or the subdivision changed; a timeline-length change only clamps.
func (p *NavigationPanel) syncGotoModels() {
	div, maxSteps := p.div(), p.maxSteps()
	if !p.seated || div != p.seatedDiv {
		start := 0
		if p.cb.PlayheadStep != nil {
			start = p.cb.PlayheadStep()
		}
		p.seatAt(start, div, maxSteps)
		return
	}
	if maxSteps != p.seatedMax {
		before := p.PendingSteps()
		p.seatedMax = maxSteps
		p.beatModel.Max = float64(maxSteps)
		p.beatModel.Value = p.beatModel.clamp(p.beatModel.Value)
		p.timeModel.Max = float64(gotoStepsToSeconds(maxSteps, div, p.bpm()))
		p.timeModel.Value = p.timeModel.clamp(p.timeModel.Value)
		if p.previewing && p.PendingSteps() != before {
			p.preview()
		}
	}
}

// seatAt rebuilds both models around steps, keeping each model's chosen rung.
func (p *NavigationPanel) seatAt(steps, div, maxSteps int) {
	beatRung, timeRung := -1, -1
	if p.beatModel != nil && p.seatedDiv == div {
		beatRung = p.beatModel.Rung
	}
	if p.timeModel != nil {
		timeRung = p.timeModel.Rung
	}
	bpm := p.bpm()
	p.beatModel = newGotoBeatModel(div, maxSteps, steps)
	p.timeModel = newGotoTimeModel(gotoStepsToSeconds(maxSteps, div, bpm), gotoStepsToSeconds(steps, div, bpm))
	if beatRung >= 0 && beatRung < len(p.beatModel.Ladder) {
		p.beatModel.Rung = beatRung
	}
	if timeRung >= 0 && timeRung < len(p.timeModel.Ladder) {
		p.timeModel.Rung = timeRung
	}
	p.seated, p.seatedDiv, p.seatedMax = true, div, maxSteps
	p.bindWheel()
}

func (p *NavigationPanel) activeModel() *NumberModel {
	if p.timeMode {
		return p.timeModel
	}
	return p.beatModel
}

func (p *NavigationPanel) bindWheel() {
	p.wheel.Bind(WheelBinding{
		Model:    p.activeModel(),
		OnChange: p.preview,
	})
	p.wheel.SetConfirm(p.Go)
	if !p.wheelRect.Empty() {
		p.wheel.Layout(p.wheelRect)
	}
}

// PendingSteps is the absolute subdivision step Go would seek to.
func (p *NavigationPanel) PendingSteps() int {
	if p.beatModel == nil {
		return 0
	}
	if p.timeMode {
		return timeToSteps(p.timeModel.Value, p.bpm(), p.div(), p.maxSteps())
	}
	return int(p.beatModel.Value)
}

func (p *NavigationPanel) preview() {
	p.previewing = true
	if p.cb.OnGotoPreview != nil {
		p.cb.OnGotoPreview(p.PendingSteps(), true)
	}
}

// CancelGoto drops the pending position's preview (Esc, tap-away, close).
func (p *NavigationPanel) CancelGoto() {
	if p.previewing && p.cb.OnGotoPreview != nil {
		p.cb.OnGotoPreview(0, false)
	}
	p.previewing = false
}

// The menu stays open, like the jog buttons.
func (p *NavigationPanel) Go() {
	if p.beatModel == nil {
		return
	}
	steps := p.PendingSteps()
	p.CancelGoto()
	if p.cb.OnGoto != nil {
		p.cb.OnGoto(steps)
	}
	p.seatAt(steps, p.div(), p.maxSteps())
}

// TimeMode reports whether the wheel scrolls time (M:SS) instead of beats.
func (p *NavigationPanel) TimeMode() bool { return p.timeMode }

// CONVERTS the pending position, never resets it.
func (p *NavigationPanel) SetTimeMode(on bool) {
	if on == p.timeMode || p.beatModel == nil {
		return
	}
	steps := p.PendingSteps()
	p.timeMode = on
	if on {
		p.timeModel.Value = p.timeModel.clamp(float64(gotoStepsToSeconds(steps, p.div(), p.bpm())))
	} else {
		p.beatModel.Value = p.beatModel.clamp(float64(steps))
	}
	p.bindWheel()
	if p.previewing {
		p.preview()
	}
}

// GotoWheel exposes the go-to ValueWheel (keys, tests).
func (p *NavigationPanel) GotoWheel() *ValueWheel { return &p.wheel }

// GotoWheelRect / GotoModeRect / GoRect expose the GO TO controls' bounds.
func (p *NavigationPanel) GotoWheelRect() image.Rectangle { return p.wheelRect }

func (p *NavigationPanel) GotoModeRect() image.Rectangle {
	if p.modeSeg == nil {
		return image.Rectangle{}
	}
	return p.modeSeg.Rect()
}

func (p *NavigationPanel) GoRect() image.Rectangle {
	if p.goBtn == nil {
		return image.Rectangle{}
	}
	return p.goBtn.Rect()
}

func (p *NavigationPanel) AmountRect() image.Rectangle {
	if p.amountBtn == nil {
		return image.Rectangle{}
	}
	return p.amountBtn.Rect()
}

// Not the SegmentedControls; the menu drives their hover glow.
func (p *NavigationPanel) Buttons() []*Button {
	return []*Button{p.prevBtn, p.amountBtn, p.nextBtn, p.goBtn}
}

// From the transport's PlayheadPos, the beat counter's own sample: the panel derives no numbers itself.
func (p *NavigationPanel) ReadoutText() string {
	if p.cb.Position == nil {
		return ""
	}
	return formatBeatTime(p.cb.Position())
}

// The OTHER unit of the pending position.
func (p *NavigationPanel) gotoSecondaryReadout() string {
	steps := p.PendingSteps()
	if p.timeMode {
		return formatGotoBeat(steps, p.div())
	}
	return formatClock(gotoStepsToSeconds(steps, p.div(), p.bpm()))
}

func (p *NavigationPanel) Draw(dst *ebiten.Image) {
	// Row 1 — position readout: leading target icon + "Beat N · M:SS".
	if txt := p.ReadoutText(); txt != "" && !p.HideReadout {
		icSz := IconSizeSM
		iy := p.readout.Min.Y + (p.readout.Dy()-icSz)/2
		DrawIcon(dst, IconTarget, image.Rect(p.readout.Min.X+SpaceMD, iy, p.readout.Min.X+SpaceMD+icSz, iy+icSz), colMenuIcon)
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
	// GO TO header: hairline divider + uppercase caption + mode tabs.
	if !p.captionRect.Empty() {
		drawRect(dst, image.Rect(p.rect.Min.X+SpaceMD, p.captionRect.Min.Y, p.rect.Max.X-SpaceMD, p.captionRect.Min.Y+1),
			WithAlphaFromColor(TokenBorderSubtle(), AlphaSubtle), true)
		th := StyledTextHeight(RoleCaption)
		DrawTextStyled(dst, "GO TO", p.captionRect.Min.X, p.captionRect.Min.Y+(p.captionRect.Dy()-th)/2, RoleCaption, colTextSecondary)
	}
	if p.modeSeg != nil {
		p.modeSeg.Draw(dst)
	}
	if !p.wheelRect.Empty() && p.beatModel != nil {
		p.wheel.DrawBody(dst)
	}
	if !p.footerRect.Empty() && p.beatModel != nil {
		th := StyledTextHeight(RoleBody)
		DrawTextStyled(dst, p.gotoSecondaryReadout(), p.footerRect.Min.X, p.footerRect.Min.Y+(p.footerRect.Dy()-th)/2, RoleBody, colTextSecondary)
	}
	if p.goBtn != nil {
		p.goBtn.Draw(dst)
	}
}

// A gesture the wheel owns stays with it; then the segments, the wheel, then each Button.
func (p *NavigationPanel) HandleInput(x, y int, pressed bool) InputResult {
	pt := image.Pt(x, y)
	if p.wheel.IsDraggingAny() {
		p.wheel.HandleInput(x, y, pressed)
		return InputConsumed
	}
	if seg, armed := p.segAt(pt); seg != nil {
		if pressed {
			if !*armed {
				*armed = true
				seg.HitTest(x, y) // fire the segment tap once, on the press edge
			}
		} else {
			*armed = false
		}
		return InputConsumed
	}
	if !pressed {
		p.segArmed, p.modeArmed = false, false
	}
	if pressed && p.beatModel != nil && (pt.In(p.wheel.valRect) || pt.In(p.wheel.resRect)) {
		p.wheel.HandleInput(x, y, true)
		return InputConsumed
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

// segAt returns the segmented control under pt and its press latch.
func (p *NavigationPanel) segAt(pt image.Point) (*SegmentedControl, *bool) {
	if p.unitSeg != nil && pt.In(p.unitSeg.Rect()) {
		return p.unitSeg, &p.segArmed
	}
	if p.modeSeg != nil && pt.In(p.modeSeg.Rect()) {
		return p.modeSeg, &p.modeArmed
	}
	return nil, nil
}

// HandleWheel scrolls the go-to wheel when the pointer is over it.
func (p *NavigationPanel) HandleWheel(x, y, steps int) bool {
	pt := image.Pt(x, y)
	if p.beatModel == nil || !(pt.In(p.wheel.valRect) || pt.In(p.wheel.resRect)) {
		return false
	}
	return p.wheel.HandleWheel(x, y, steps)
}
