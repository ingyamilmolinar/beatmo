package ui

import (
	"image"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
)

// NavPanelCallbacks wires NavigationPanel's read-only state accessors and
// control actions to the owning component (the navigation menu). All fields are
// optional; nil accessors/actions are treated as no-ops so the panel can be
// exercised in isolation (tests).
type NavPanelCallbacks struct {
	// Position is the transport's resolved playhead sample, the ONLY source
	// for the panel's position title. PlayheadStep is the 0-based absolute
	// subdivision step the go-to wheel opens at.
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
	// OnGotoPreview shows (on) or clears (!on) the timeline ghost marking the
	// go-to wheel's pending position. Scrolling never seeks.
	OnGotoPreview func(steps int, on bool)
	// OnGoto seeks to an absolute subdivision step (Go / Enter / centre click).
	OnGoto func(steps int)
}

// NavigationPanel is the body of the navigation menu (opened from the beat
// counter). Top to bottom: a jog row of DropdownStyle keycaps (prev · ×N amount
// · next), a Beats/Subdiv SegmentedControl for the jump unit, then the GO TO
// section — a Beat|Time SegmentedControl, the shared ValueWheel scrolling the
// pending position, and a footer with the secondary readout and the primary Go
// button. It owns no seek logic: every action is delegated via
// NavPanelCallbacks. The wheel PREVIEWS (OnGotoPreview) and only Go seeks.
type NavigationPanel struct {
	cb      NavPanelCallbacks
	rect    image.Rectangle
	readout image.Rectangle
	// HideReadout skips painting row 1: the owning menu's MenuCard title band
	// shows the readout instead (the row keeps its height for geometry).
	HideReadout bool
	// SharedJogRow puts the jog cluster and the Beats/Subdiv toggle on ONE row
	// (the desktop card is wide enough), so the card fits under the timeline
	// ribbon it drops onto and never covers the go-to ghost there.
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

	// segArmed / modeArmed latch a SegmentedControl press so a held pointer
	// fires its tap exactly once (SegmentedControl.HitTest has no edge
	// detection of its own, and the menu re-delivers pressed==true every frame
	// while held).
	segArmed  bool
	modeArmed bool
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

// PanelHeight returns the panel's fixed total height: readout + jog row + unit
// row (shared with the jog row when SharedJogRow) + GO TO header + the wheel +
// the footer.
func (p *NavigationPanel) PanelHeight() int {
	rows := 5
	if p.SharedJogRow {
		rows = 4
	}
	return rows*touchMinTargetPx + gotoWheelHeight()
}

// gotoWheelRows is how many value rows the go-to wheel shows (DESIGN.md menu
// sizing: wheels show five rows).
const gotoWheelRows = 5

// gotoWheelHeight is the wheel's height: five row pitches plus a vertical inset.
func gotoWheelHeight() int {
	gap := Profile().DensityValues().MobileWheelTickGap
	if gap <= 0 {
		gap = 34
	}
	return gotoWheelRows*gap + 2*SpaceSM
}

// Layout positions the rows top-to-bottom inside rect and refreshes the live
// amount label + unit selection from the source of truth.
func (p *NavigationPanel) Layout(rect image.Rectangle) {
	p.rect = rect
	rowH := touchMinTargetPx
	x0, w := rect.Min.X, rect.Dx()
	y := rect.Min.Y

	// Row 1 — position readout (left-aligned text + leading icon, drawn in
	// Draw). Left alignment keeps the popup's top-right close-× clear.
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

// layoutJogRows lays the jog cluster and the unit toggle on two rows (the
// mobile sheet). Returns the y below them.
func (p *NavigationPanel) layoutJogRows(x0, y, w, rowH int) int {
	// Row 2 — jog: [◀◀] [ ×N ] [▶▶] as a CENTERED cluster (bounded, not edge-to-
	// edge) so it reads as a unit on the wide mobile sheet, and never collides
	// with the close-× (which is row 1 only).
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

// ResetGoto seats the go-to wheel on the current playhead (called when the
// menu opens). The chosen mode and step rungs are kept for the session.
func (p *NavigationPanel) ResetGoto() {
	p.seated = false
	p.syncGotoModels()
}

// syncGotoModels (re)builds the models when the panel is unseated or the
// subdivision changed, and follows timeline-length changes by clamping.
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

// Go seeks to the pending position, clears the preview and re-seats the wheel
// on the new position (the menu stays open, like the jog buttons).
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

// SetTimeMode switches the wheel between beats and time, CONVERTING the pending
// position (never resetting it).
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

// AmountRect exposes the jump-amount keycap so the owning DrumView can anchor
// the jump-size picker over it.
func (p *NavigationPanel) AmountRect() image.Rectangle {
	if p.amountBtn == nil {
		return image.Rectangle{}
	}
	return p.amountBtn.Rect()
}

// Buttons returns the panel's *Button controls (NOT the SegmentedControls) so
// the owning menu can drive their hover-glow every frame.
func (p *NavigationPanel) Buttons() []*Button {
	return []*Button{p.prevBtn, p.amountBtn, p.nextBtn, p.goBtn}
}

// ReadoutText returns the panel's position title exactly as it is drawn
// ("Beat N · M:SS"), rendered from the transport's resolved PlayheadPos — the
// same sample the beat counter prints. The panel derives no beat/second values
// of its own; see PlayheadPos (playhead_position.go).
func (p *NavigationPanel) ReadoutText() string {
	if p.cb.Position == nil {
		return ""
	}
	return formatBeatTime(p.cb.Position())
}

// gotoSecondaryReadout is the footer text: the OTHER unit of the pending
// position (time in beat mode, the resolved beat in time mode).
func (p *NavigationPanel) gotoSecondaryReadout() string {
	steps := p.PendingSteps()
	if p.timeMode {
		return formatGotoBeat(steps, p.div())
	}
	return formatClock(gotoStepsToSeconds(steps, p.div(), p.bpm()))
}

// Draw renders the position readout, the jog keycaps, the unit segmented
// control and the GO TO section.
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

// HandleInput dispatches a pointer event: a gesture the wheel owns stays with
// the wheel; otherwise the segmented controls (press-edge latched), the wheel
// (a press inside it), then each *Button via Button.HandleInputResult. Returns
// the first non-Ignored result; InputIgnored when nothing claims the point.
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
	if p.beatModel == nil || (!pt.In(p.wheel.valRect) && !pt.In(p.wheel.resRect)) {
		return false
	}
	return p.wheel.HandleWheel(x, y, steps)
}
