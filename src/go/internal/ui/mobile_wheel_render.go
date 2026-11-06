// src/go/internal/ui/mobile_wheel_render.go
package ui

import (
	"image"
	"image/color"
	"math"
	"strings"

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
	w.DrawBody(dst)
}

// DrawBody renders the wheel itself (no panel, no title): the textured
// thumbwheel cylinder, the value rows, the centre grip and the resolution
// ladder. Inline hosts draw it on their own surface.
func (w *ValueWheel) DrawBody(dst *ebiten.Image) {
	w.drawCylinder(dst)  // textured rolling-wheel surface behind the rows
	w.drawSlotBezel(dst) // recessed-slot framing (top/bottom inner shadow)
	w.drawBarrel(dst)    // distinct value rows above/below center
	w.drawEdgeFade(dst)  // dissolve rows into the panel at top & bottom
	w.drawSlotRails(dst) // machined side rails with accent-lit edges
	w.drawThumbGrip(dst) // ribbed pitch-wheel thumb grip + engraved value
	w.drawResStrip(dst)  // resolution stepper (continuous params only)
}

// wheelTitleGlossSep joins the param name and its PlainEnglish gloss in the
// title band ("Cutoff — bright or dull").
const wheelTitleGlossSep = " — "

// drawTitle centers the param name in the reserved title band with a hairline
// divider beneath it. When the binding supplies a Gloss, it is appended on the
// same band ("Title — Gloss", the gloss dim), truncated to fit; if the gloss
// truncates away entirely the bare title is drawn (never a dangling "—").
func (w *MobileWheelPopup) drawTitle(dst *ebiten.Image) {
	title := w.binding.Title()
	if title == "" {
		return
	}
	tr := w.titleRect
	ty := tr.Min.Y + (tr.Dy()-StyledTextHeight(RolePanelTitle))/2
	maxW := tr.Dx() - 2*SpaceSM

	// Try "Title — Gloss" on the same band; split the fitted string so the gloss
	// can be drawn dim. Fall back to the bare title when the gloss won't fit.
	gloss := ""
	if w.binding.Gloss != nil {
		gloss = w.binding.Gloss()
	}
	if gloss != "" {
		trunc := truncCaptionStyled(title+wheelTitleGlossSep+gloss, maxW, RolePanelTitle)
		if idx := strings.Index(trunc, wheelTitleGlossSep); idx >= 0 {
			tail := trunc[idx+len(wheelTitleGlossSep):]
			if strings.TrimSpace(strings.TrimRight(tail, "…")) != "" {
				head := trunc[:idx]
				totalW := StyledTextWidth(trunc, RolePanelTitle)
				headW := StyledTextWidth(head, RolePanelTitle)
				tx := tr.Min.X + (tr.Dx()-totalW)/2
				DrawTextStyled(dst, head, tx, ty, RolePanelTitle, colTextSecondary)
				DrawTextStyled(dst, trunc[idx:], tx+headW, ty, RolePanelTitle,
					WithAlphaFromColor(colTextSecondary, AlphaMedium))
				w.drawTitleDivider(dst)
				return
			}
		}
	}

	bare := truncCaptionStyled(title, maxW, RolePanelTitle)
	tx := tr.Min.X + (tr.Dx()-StyledTextWidth(bare, RolePanelTitle))/2
	DrawTextStyled(dst, bare, tx, ty, RolePanelTitle, colTextSecondary)
	w.drawTitleDivider(dst)
}

// drawTitleDivider draws the hairline divider beneath the title band.
func (w *MobileWheelPopup) drawTitleDivider(dst *ebiten.Image) {
	tr := w.titleRect
	divY := tr.Max.Y - 1
	drawRect(dst, image.Rect(tr.Min.X, divY, tr.Max.X, divY+1),
		WithAlphaFromColor(TokenBorderSubtle(), AlphaSubtle), true)
}

// wheelCylinderCache holds the static thumbwheel surface keyed by (w<<16|h).
var wheelCylinderCache = map[int]*ebiten.Image{}

// wheelSpriteThemeGen is the ThemeGeneration the wheel sprite caches were
// baked under. Both sprites bake surface tones, and they are package-level (a
// theme change reaches no Game cache here), so they are dropped whenever the
// theme moves — before, a wheel popup kept the boot theme's surfaces forever.
var wheelSpriteThemeGen int

// syncWheelSpriteTheme drops both wheel sprite caches after a theme change.
func syncWheelSpriteTheme() {
	tg := ThemeGeneration()
	if tg == wheelSpriteThemeGen {
		return
	}
	wheelSpriteThemeGen = tg
	for k, img := range wheelCylinderCache {
		releaseImage(img)
		delete(wheelCylinderCache, k)
	}
	for k, img := range wheelGripCache {
		releaseImage(img)
		delete(wheelGripCache, k)
	}
}

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
	syncWheelSpriteTheme()
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
func (w *ValueWheel) drawCylinder(dst *ebiten.Image) {
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
func (w *ValueWheel) drawBarrel(dst *ebiten.Image) {
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
// smaller). The labels come from the model's distinct-row walk (WheelModel.RowLabel),
// so a click on a row lands exactly on the value it shows.
func (w *ValueWheel) walkRows(dst *ebiten.Image, centerY, gap, maxRows, screenDir, valueDir int) {
	m := w.model()
	if m == nil {
		return
	}
	for r := 1; r <= maxRows; r++ {
		lbl, ok := m.RowLabel(valueDir * r)
		if !ok {
			return
		}
		w.drawRow(dst, lbl, centerY+screenDir*r*gap, r, maxRows)
	}
}

// drawRow draws one value label centered at y, plus a short engraved gradation
// tick at the left margin, fading by distance from center.
func (w *ValueWheel) drawRow(dst *ebiten.Image, label string, y, row, maxRows int) {
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
func (w *ValueWheel) drawEdgeFade(dst *ebiten.Image) {
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
func (w *ValueWheel) drawSlotBezel(dst *ebiten.Image) {
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
func (w *ValueWheel) drawSlotRails(dst *ebiten.Image) {
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
	rail(vr.Min.X, vr.Min.X+wheelRailW, vr.Min.X+wheelRailW)   // left rail, edge on its right
	rail(vr.Max.X-wheelRailW, vr.Max.X, vr.Max.X-wheelRailW-1) // right rail, edge on its left
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
	syncWheelSpriteTheme()
	key := wpx<<16 | hpx
	if spr, ok := wheelGripCache[key]; ok {
		return spr
	}
	spr := ebiten.NewImage(wpx, hpx)
	top := adjustColor(TokenSurface3(), 24)  // lit upper face
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
func (w *ValueWheel) drawThumbGrip(dst *ebiten.Image) {
	// Inset the pad to sit BETWEEN the side rails, like a thumb grip in its slot.
	r := image.Rect(w.centerH.Min.X+wheelRailW, w.centerH.Min.Y, w.centerH.Max.X-wheelRailW, w.centerH.Max.Y)
	if r.Empty() {
		return
	}
	rad := fitRadius(RadiusControl, r)
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
	// Hover brighten: cursor-over feedback for the click-to-confirm grip
	// (portal input handlers only fire on press edges — see HoverAt).
	if w.hoverCenter {
		drawRoundedRect(dst, r, WithAlpha(colTextPrimary, AlphaFaint), rad, true)
	}
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

// drawResStrip renders the resolution control as a visible rung LADDER: every
// stepLadder rung is a tappable pill stacked vertically, finest at the TOP
// (matching "scroll up = finer"), the active rung highlighted with the accent.
// Tapping a pill jumps straight to it; scrolling/dragging still steps rung-by-
// rung. rungRects() is the single source of truth for the pill geometry so the
// hit-test and the draw stay in lockstep.
func (w *ValueWheel) drawResStrip(dst *ebiten.Image) {
	m := w.model()
	if w.resRect.Empty() || m == nil || !m.HasLadder() {
		return
	}
	r := w.resRect
	drawRoundedRect(dst, r, TokenSurface2(), RadiusContainer, true)
	drawRoundedRect(dst, r, WithAlphaFromColor(TokenBorderSubtle(), AlphaMedium), RadiusContainer, false)
	accent := w.resolveAccent()
	for i, rr := range w.rungRects() {
		lbl := m.LadderLabel(i)
		active := i == m.LadderIndex()
		switch {
		case active:
			drawRoundedRect(dst, rr, WithAlphaFromColor(accent, AlphaMedium), RadiusXS, true)
			drawRoundedRect(dst, rr, accent, RadiusXS, false)
		case i == w.hoverRung:
			// Brighten (not the accent fill — that's reserved for the active
			// rung) so a hovered-but-inactive pill reads as interactive.
			drawRoundedRect(dst, rr, WithAlpha(TokenSurface3(), AlphaStrong), RadiusXS, true)
		default:
			drawRoundedRect(dst, rr, WithAlpha(TokenSurface3(), AlphaMedium), RadiusXS, true)
		}
		col := TokenTextSecondary()
		if active {
			col = colTextPrimary
		}
		disp := truncCaptionStyled(lbl, rr.Dx()-4, RoleMicro)
		lw := StyledTextWidth(disp, RoleMicro)
		lh := StyledTextHeight(RoleMicro)
		DrawTextStyled(dst, disp, rr.Min.X+(rr.Dx()-lw)/2, rr.Min.Y+(rr.Dy()-lh)/2, RoleMicro, col)
	}
}

func (w *ValueWheel) tickGap() int {
	gap := Profile().DensityValues().MobileWheelTickGap
	if gap <= 0 {
		gap = 34
	}
	return gap
}

// resolveAccent returns the accent fill for the selection lane. When the binding
// provides a non-nil Accent color, that color is used; otherwise the primary
// token is returned. No raw color literals.
func (w *ValueWheel) resolveAccent() color.Color {
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
func (w *ValueWheel) liveValueLabel() string {
	if m := w.model(); m != nil {
		return m.CenterLabel()
	}
	return ""
}

// The knob value-formatting helpers (enumIndex, formatKnobDisplay,
// formatKnobDisplayStep, knobStepResolution, knobPillLabel) live in
// knob_format.go — they are view-independent and outlive this popup.
