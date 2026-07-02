// src/go/internal/ui/mobile_wheel_render.go
package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Draw renders the mobile wheel popup as a textured analogue thumbwheel: a title
// band, a shaded ridged cylinder behind scrolling value rows, a keycap-styled
// center selector showing the live value, and an optional resolution stepper.
// Guard: returns immediately when the popup is not open.
func (w *MobileWheelPopup) Draw(dst *ebiten.Image) {
	if !w.open {
		return
	}

	drawPanel(dst, w.rect)
	w.drawTitle(dst)
	w.drawCylinder(dst)  // textured rolling-wheel surface behind the rows
	w.drawSlotBezel(dst) // recessed-slot framing (top/bottom inner shadow)
	w.drawBarrel(dst)    // distinct value rows above/below center
	w.drawEdgeFade(dst)  // dissolve rows into the panel at top & bottom
	w.drawSlotRails(dst) // machined side rails with accent-lit edges
	w.drawThumbGrip(dst) // ribbed pitch-wheel thumb grip + engraved value
	w.drawResStrip(dst)  // resolution stepper (continuous params only)
}

// drawTitle centers the param name in the reserved title band with a hairline
// divider beneath it.
func (w *MobileWheelPopup) drawTitle(dst *ebiten.Image) {
	title := w.binding.Title()
	if title == "" {
		return
	}
	tr := w.titleRect
	tx := tr.Min.X + (tr.Dx()-StyledTextWidth(title, RolePanelTitle))/2
	ty := tr.Min.Y + (tr.Dy()-StyledTextHeight(RolePanelTitle))/2
	DrawTextStyled(dst, title, tx, ty, RolePanelTitle, colTextSecondary)
	divY := tr.Max.Y - 1
	drawRect(dst, image.Rect(tr.Min.X, divY, tr.Max.X, divY+1),
		WithAlphaFromColor(TokenBorderSubtle(), AlphaSubtle), true)
}

// wheelCylinderCache holds the static thumbwheel surface keyed by (w<<16|h).
var wheelCylinderCache = map[int]*ebiten.Image{}

const wheelRidgeStep = 7 // nominal px between ridges at the wheel's mid-line

// wheelCylinderSprite rasterizes the analogue thumbwheel face once per size: a
// vertical curvature gradient (dark rim → lit centre → dark rim), horizontal
// ridges bunched toward the rim via a sine map (the rolling-drum read), and a
// soft specular band above centre. Cached + blitted, mirroring the knurled knob
// cap's once-per-size strategy so the per-frame cost stays a single blit.
func wheelCylinderSprite(w, h int) *ebiten.Image {
	if w < 2 {
		w = 2
	}
	if h < 2 {
		h = 2
	}
	key := w<<16 | h
	if spr, ok := wheelCylinderCache[key]; ok {
		return spr
	}
	spr := ebiten.NewImage(w, h)
	rim := TokenSurface1()
	mid := adjustColor(TokenSurface2(), 16)
	half := h / 2
	// Vertical curvature: the face bulges toward the viewer at mid-height.
	fillVerticalGradient(spr, image.Rect(0, 0, w, half), rim, mid, 18)
	fillVerticalGradient(spr, image.Rect(0, half, w, h), mid, rim, 18)

	// Ridges bunched toward the rim (sine map of a linear index).
	cy := float64(h) / 2
	rh := float64(h) / 2
	ridges := h / wheelRidgeStep
	if ridges < 6 {
		ridges = 6
	}
	for i := -ridges; i <= ridges; i++ {
		t := float64(i) / float64(ridges) // -1..1
		y := cy + math.Sin(t*math.Pi/2)*rh
		shade := 36
		if i%2 == 0 {
			shade = -28
		}
		a := uint8(clampF64(190*(1-math.Abs(t)), 34, 190))
		col := WithAlphaFromColor(adjustColor(mid, shade), a)
		vector.StrokeLine(spr, 1, float32(y), float32(w-1), float32(y), 1, col, false)
	}
	// Soft specular band just above centre (light from above).
	specTop := half - h/8
	specBot := half - h/24
	if specBot > specTop {
		drawRect(spr, image.Rect(2, specTop, w-2, specBot), WithAlpha(colTextPrimary, 22), true)
	}
	wheelCylinderCache[key] = spr
	return spr
}

// drawCylinder blits the cached thumbwheel surface into the barrel column.
func (w *MobileWheelPopup) drawCylinder(dst *ebiten.Image) {
	vr := w.valRect
	if vr.Empty() {
		return
	}
	spr := wheelCylinderSprite(vr.Dx(), vr.Dy())
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(vr.Min.X), float64(vr.Min.Y))
	dst.DrawImage(spr, op)
	// Side rims (left/right) for a contained, machined edge.
	drawRect(dst, image.Rect(vr.Min.X, vr.Min.Y, vr.Min.X+1, vr.Max.Y),
		WithAlphaFromColor(color.Black, 70), true)
	drawRect(dst, image.Rect(vr.Max.X-1, vr.Min.Y, vr.Max.X, vr.Max.Y),
		WithAlphaFromColor(color.Black, 70), true)
}

// drawBarrel walks DISTINCT value rows outward from the center — upward shows
// larger values, downward smaller — so adjacent rows never show the same
// formatted label even when the resolution is finer than the label precision.
func (w *MobileWheelPopup) drawBarrel(dst *ebiten.Image) {
	vr := w.valRect
	if vr.Empty() {
		return
	}
	gap := w.tickGap()
	centerY := (vr.Min.Y + vr.Max.Y) / 2
	maxRows := vr.Dy()/(2*gap) + 1
	w.walkRows(dst, centerY, gap, maxRows, -1, +1) // above center, larger values
	w.walkRows(dst, centerY, gap, maxRows, +1, -1) // below center, smaller values
}

// walkRows draws up to maxRows distinct labels in one direction. screenDir is the
// Y direction (-1 up, +1 down); valueDir is the value direction (+1 larger, -1
// smaller). For enums each index is already distinct; for continuous params the
// value is advanced by StepMul until the formatted label actually changes.
func (w *MobileWheelPopup) walkRows(dst *ebiten.Image, centerY, gap, maxRows, screenDir, valueDir int) {
	def := w.binding.Def
	k := w.binding.Knob
	if k == nil {
		return
	}
	if len(def.Enum) > 0 {
		cur := enumIndex(k.Value, len(def.Enum))
		for r := 1; r <= maxRows; r++ {
			idx := cur + valueDir*r
			if idx < 0 || idx >= len(def.Enum) {
				return
			}
			w.drawRow(dst, def.Enum[idx], centerY+screenDir*r*gap, r, maxRows)
		}
		return
	}
	span := def.Max - def.Min
	step := w.valueStep()
	prev := formatSynthParamValueStep(def, def.Min+k.Value*span, step)
	val := def.Min + k.Value*span
	for r := 1; r <= maxRows; r++ {
		lbl := ""
		for guard := 0; guard < 4096; guard++ {
			val += float64(valueDir) * step
			if val < def.Min || val > def.Max {
				return
			}
			cand := formatSynthParamValueStep(def, val, step)
			if cand != prev {
				prev, lbl = cand, cand
				break
			}
		}
		if lbl == "" {
			return
		}
		w.drawRow(dst, lbl, centerY+screenDir*r*gap, r, maxRows)
	}
}

// drawRow draws one value label centered at y, plus a short engraved gradation
// tick at the left margin, fading by distance from center.
func (w *MobileWheelPopup) drawRow(dst *ebiten.Image, label string, y, row, maxRows int) {
	vr := w.valRect
	if y < vr.Min.Y+StyledTextHeight(RoleBody)/2 || y > vr.Max.Y-StyledTextHeight(RoleBody)/2 {
		return
	}
	frac := float64(row) / float64(maxRows+1)
	a := uint8(clampF64(220*(1-frac), 40, 220))
	lw := StyledTextWidth(label, RoleBody)
	lx := vr.Min.X + (vr.Dx()-lw)/2
	ly := y - StyledTextHeight(RoleBody)/2
	DrawTextStyled(dst, label, lx, ly, RoleBody, WithAlphaFromColor(colTextPrimary, a))
}

// drawEdgeFade overlays panel-colored gradients at the top and bottom of the
// barrel so rows dissolve into the panel — reinforcing the cylinder curve.
func (w *MobileWheelPopup) drawEdgeFade(dst *ebiten.Image) {
	vr := w.valRect
	if vr.Empty() {
		return
	}
	h := vr.Dy() / 5
	if h < 8 {
		return
	}
	clear := WithAlphaFromColor(colPanelBG, 0)
	fillVerticalGradient(dst, image.Rect(vr.Min.X, vr.Min.Y, vr.Max.X, vr.Min.Y+h), colPanelBG, clear, 8)
	fillVerticalGradient(dst, image.Rect(vr.Min.X, vr.Max.Y-h, vr.Max.X, vr.Max.Y), clear, colPanelBG, 8)
}

// wheelRailW is the width of each machined side rail flanking the wheel slot.
const wheelRailW = 7

// drawSlotBezel sinks the barrel into a recessed slot: a dark inner-shadow band
// at the top edge and a thin lit lip at the bottom (light from above), so the
// rolling wheel reads as mounted in a panel cutout.
func (w *MobileWheelPopup) drawSlotBezel(dst *ebiten.Image) {
	vr := w.valRect
	if vr.Empty() {
		return
	}
	band := vr.Dy() / 9
	if band < 4 {
		band = 4
	}
	clear := WithAlphaFromColor(color.Black, 0)
	// Top inner shadow (recessed lip catches no light).
	fillVerticalGradient(dst, image.Rect(vr.Min.X, vr.Min.Y, vr.Max.X, vr.Min.Y+band),
		WithAlphaFromColor(color.Black, 150), clear, 6)
	// Bottom lit lip.
	drawRect(dst, image.Rect(vr.Min.X, vr.Max.Y-1, vr.Max.X, vr.Max.Y),
		WithAlpha(colTextPrimary, 28), true)
}

// drawSlotRails draws the two machined side rails that flank the wheel, each with
// a thin accent-lit inner edge — the analog pitch-wheel's side-LED glow, colored
// by the active row's accent.
func (w *MobileWheelPopup) drawSlotRails(dst *ebiten.Image) {
	vr := w.valRect
	if vr.Empty() {
		return
	}
	accent := w.resolveAccent()
	rail := func(x0, x1, edgeX int) {
		// Dark machined rail with a top-down sheen.
		fillVerticalGradient(dst, image.Rect(x0, vr.Min.Y, x1, vr.Max.Y),
			adjustColor(TokenSurface1(), 10), TokenSurface1(), 8)
		// Accent-lit inner edge (1px) + a soft glow falloff beside it.
		drawRect(dst, image.Rect(edgeX, vr.Min.Y, edgeX+1, vr.Max.Y),
			WithAlphaFromColor(accent, AlphaStrong), true)
		drawRect(dst, image.Rect(edgeX-1, vr.Min.Y, edgeX, vr.Max.Y),
			WithAlphaFromColor(accent, AlphaFaint), true)
	}
	rail(vr.Min.X, vr.Min.X+wheelRailW, vr.Min.X+wheelRailW)       // left rail, edge on its right
	rail(vr.Max.X-wheelRailW, vr.Max.X, vr.Max.X-wheelRailW-1)     // right rail, edge on its left
}

// wheelGripCache holds the static, neutral thumb-grip texture (dome + rubber
// ridges + specular) keyed by (w<<16|h). The accent detent and the engraved
// value are drawn on top per frame, so one sprite serves every instrument color.
var wheelGripCache = map[int]*ebiten.Image{}

// wheelGripSprite rasterizes the matte-rubber thumb pad once per size: a domed
// vertical gradient (lit top → shadow bottom), tight horizontal rubber ridges,
// and a crisp top specular line. Mirrors the knurled cap / cylinder caching.
func wheelGripSprite(wpx, hpx int) *ebiten.Image {
	if wpx < 2 {
		wpx = 2
	}
	if hpx < 2 {
		hpx = 2
	}
	key := wpx<<16 | hpx
	if spr, ok := wheelGripCache[key]; ok {
		return spr
	}
	spr := ebiten.NewImage(wpx, hpx)
	top := adjustColor(TokenSurface3(), 24) // lit upper face
	bot := adjustColor(TokenSurface1(), -14) // shadowed lower face
	fillVerticalGradient(spr, image.Rect(0, 0, wpx, hpx), top, bot, 14)
	// Rubber ribs: a dark groove with a lit ridge just below it (light from above),
	// repeated so the pad reads as molded grip ribbing.
	for y := 4; y < hpx-3; y += 4 {
		drawRect(spr, image.Rect(2, y, wpx-2, y+1), WithAlphaFromColor(color.Black, 150), true)
		drawRect(spr, image.Rect(2, y+1, wpx-2, y+2), WithAlpha(colTextPrimary, 36), true)
	}
	// Crisp top specular line + bottom shadow line (raised dome).
	drawRect(spr, image.Rect(2, 1, wpx-2, 2), WithAlpha(colTextPrimary, 90), true)
	drawRect(spr, image.Rect(2, hpx-2, wpx-2, hpx-1), WithAlphaFromColor(color.Black, 110), true)
	wheelGripCache[key] = spr
	return spr
}

// drawThumbGrip renders the pitch-wheel thumb grip (the "main button"): a raised
// ribbed matte-rubber pad with a contact shadow, a central accent detent notch,
// and the live value as an embossed legend printed on the rubber.
func (w *MobileWheelPopup) drawThumbGrip(dst *ebiten.Image) {
	// Inset the pad to sit BETWEEN the side rails, like a thumb grip in its slot.
	r := image.Rect(w.centerH.Min.X+wheelRailW, w.centerH.Min.Y, w.centerH.Max.X-wheelRailW, w.centerH.Max.Y)
	if r.Empty() {
		return
	}
	rad := RadiusXS
	accent := w.resolveAccent()
	// Raised pad: a 2px contact shadow below sells the protrusion from the slot.
	socket := image.Rect(r.Min.X, r.Min.Y+2, r.Max.X, r.Max.Y+2)
	drawRoundedRect(dst, socket, WithAlphaFromColor(color.Black, 120), rad, true)
	// Pad body: blit the cached rubber texture clipped to the rounded face.
	drawRoundedRect(dst, r, TokenSurface2(), rad, true) // rounded base (corners)
	spr := wheelGripSprite(r.Dx()-2, r.Dy()-2)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(r.Min.X+1), float64(r.Min.Y+1))
	dst.DrawImage(spr, op)
	// Rounded rim: a 1px accent border ties it to the rail glow.
	drawRoundedRect(dst, r, WithAlphaFromColor(accent, AlphaMedium), rad, false)

	// Center detent notch: a dark groove with an accent glow line — the wheel's
	// rest indicator and the in-focus marker.
	cyMid := (r.Min.Y + r.Max.Y) / 2
	drawRect(dst, image.Rect(r.Min.X+3, cyMid-1, r.Max.X-3, cyMid), WithAlphaFromColor(color.Black, 150), true)
	drawRect(dst, image.Rect(r.Min.X+3, cyMid, r.Max.X-3, cyMid+1), WithAlphaFromColor(accent, AlphaStrong), true)

	// Engraved value legend: a dark drop-shadow under bright glyphs reads as a
	// raised label printed on the rubber.
	val := w.liveValueLabel()
	vw := StyledTextWidth(val, RoleSectionHeader)
	vh := StyledTextHeight(RoleSectionHeader)
	vx := r.Min.X + (r.Dx()-vw)/2
	vy := r.Min.Y + (r.Dy()-vh)/2
	DrawTextStyled(dst, val, vx, vy+1, RoleSectionHeader, WithAlphaFromColor(color.Black, 160))
	DrawTextStyled(dst, val, vx, vy, RoleSectionHeader, colTextPrimary)
}

// drawResStrip renders the resolution control as a vertical stepper: a rounded
// rail with up/down chevrons framing the current step label.
func (w *MobileWheelPopup) drawResStrip(dst *ebiten.Image) {
	if w.resRect.Empty() || w.binding.Badge == nil {
		return
	}
	r := w.resRect
	drawRoundedRect(dst, r, TokenSurface2(), RadiusMD, true)
	drawRoundedRect(dst, r, WithAlphaFromColor(TokenBorderSubtle(), AlphaMedium), RadiusMD, false)

	chW := r.Dx() - SpaceSM*2
	if chW > 18 {
		chW = 18
	}
	cx := r.Min.X + (r.Dx()-chW)/2
	up := image.Rect(cx, r.Min.Y+SpaceXS, cx+chW, r.Min.Y+SpaceXS+chW)
	DrawIcon(dst, IconChevronUp, up, colTextSecondary)
	dn := image.Rect(cx, r.Max.Y-SpaceXS-chW, cx+chW, r.Max.Y-SpaceXS)
	DrawIcon(dst, IconChevronDown, dn, colTextSecondary)

	lbl := w.binding.Badge.Label()
	lw := StyledTextWidth(lbl, RoleCaption)
	lh := StyledTextHeight(RoleCaption)
	lx := r.Min.X + (r.Dx()-lw)/2
	ly := r.Min.Y + (r.Dy()-lh)/2
	DrawTextStyled(dst, lbl, lx, ly, RoleCaption, colTextAccent)
}

func (w *MobileWheelPopup) tickGap() int {
	gap := Profile().DensityValues().MobileWheelTickGap
	if gap <= 0 {
		gap = 34
	}
	return gap
}

// resolveAccent returns the accent fill for the selection lane. When the binding
// provides a non-nil Accent color, that color is used; otherwise the primary
// token is returned. No raw color literals.
func (w *MobileWheelPopup) resolveAccent() color.Color {
	if w.binding.Accent != nil {
		if c := w.binding.Accent(); c != nil {
			return c
		}
	}
	return genColorPrimary // package-level RGBA token
}

// liveValueLabel returns the formatted current value string for the selection
// lane, rendered at the active step resolution so a fine resolution (e.g.
// x0.005) keeps changing the displayed number after a single-step nudge.
func (w *MobileWheelPopup) liveValueLabel() string {
	return formatKnobDisplayStep(w.binding.Def, w.binding.Knob.Value, w.valueStep())
}

// valueStep returns the active step in real param units — the knob's StepMul,
// falling back to span/200 when unset. Shared by the center readout and the
// barrel row walk so both render at the same resolution.
func (w *MobileWheelPopup) valueStep() float64 {
	return knobStepResolution(w.binding.Knob, w.binding.Def)
}

// enumIndex maps a normalized [0,1] value to a clamped enum index.
func enumIndex(norm float64, n int) int {
	if n <= 0 {
		return 0
	}
	idx := int(math.Round(norm * float64(n-1)))
	if idx < 0 {
		return 0
	}
	if idx >= n {
		return n - 1
	}
	return idx
}

// formatKnobDisplay formats a normalized knob value for display: the enum label
// for discrete params, otherwise the unit-aware numeric label at natural
// precision. Shared by the inline mobile value pill.
func formatKnobDisplay(def audio.ParamDef, norm float64) string {
	return formatKnobDisplayStep(def, norm, math.Inf(1))
}

// formatKnobDisplayStep is the resolution-aware sibling of formatKnobDisplay: it
// renders the numeric value with the decimals the active `step` (real param
// units) requires. An infinite step reproduces formatKnobDisplay.
func formatKnobDisplayStep(def audio.ParamDef, norm, step float64) string {
	if len(def.Enum) > 0 {
		return def.Enum[enumIndex(norm, len(def.Enum))]
	}
	return formatSynthParamValueStep(def, def.Min+norm*(def.Max-def.Min), step)
}

// knobStepResolution returns a knob's active step in real param units — its
// StepMul, falling back to span/200 when unset — matching the wheel's value
// step. Shared so the inline pill and the wheel center render at one resolution.
func knobStepResolution(k *Knob, def audio.ParamDef) float64 {
	if k == nil {
		return 0
	}
	step := k.StepMul
	if step <= 0 {
		step = (def.Max - def.Min) / 200
	}
	return step
}

// knobPillLabel formats a knob's current value for the inline mobile pill/button
// at the knob's active step resolution, so a fine resolution shows the extra
// decimals the user dialed in (matching the wheel center). Enum-aware.
func knobPillLabel(k *Knob, def audio.ParamDef) string {
	if k == nil {
		return formatKnobDisplay(def, 0)
	}
	return formatKnobDisplayStep(def, k.Value, knobStepResolution(k, def))
}
