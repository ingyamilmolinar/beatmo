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

func (w *MobileWheelPopup) Draw(dst *ebiten.Image) {
	if !w.open {
		return
	}

	drawPanel(dst, w.rect)
	w.drawTitle(dst)
	w.DrawBody(dst)
}

// For inline hosts, which draw it on their own surface.
func (w *ValueWheel) DrawBody(dst *ebiten.Image) {
	w.drawCylinder(dst)  // textured rolling-wheel surface behind the rows
	w.drawSlotBezel(dst) // recessed-slot framing (top/bottom inner shadow)
	w.drawBarrel(dst)    // distinct value rows above/below center
	w.drawEdgeFade(dst)  // dissolve rows into the panel at top & bottom
	w.drawSlotRails(dst) // machined side rails with accent-lit edges
	w.drawThumbGrip(dst) // ribbed pitch-wheel thumb grip + engraved value
	w.drawResStrip(dst)  // resolution stepper (continuous params only)
}

const wheelTitleGlossSep = " — "

// The gloss is appended dim when it fits; never a dangling "—".
func (w *MobileWheelPopup) drawTitle(dst *ebiten.Image) {
	title := w.binding.Title()
	if title == "" {
		return
	}
	tr := w.titleRect
	ty := tr.Min.Y + (tr.Dy()-StyledTextHeight(RolePanelTitle))/2
	maxW := tr.Dx() - 2*SpaceSM

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

// Package-level sprites bake surface tones and no Game cache reaches them, so drop them when the
// theme moves.
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

// Rasterized once per size and blitted, so the per-frame cost is one blit.
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

// DISTINCT values outward from centre, so adjacent rows never repeat a label at a fine resolution.
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

// Labels come from the model's distinct-row walk, so a click lands on exactly the value shown.
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

// Lit in the active row's accent.
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

// Neutral, so one sprite serves every instrument color; the accent and value draw on top.
var wheelGripCache = map[int]*ebiten.Image{}

// Once per size, like the cylinder.
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
	// Portal input only fires on press edges, so hover comes from HoverAt.
	if w.hoverCenter {
		drawRoundedRect(dst, r, WithAlpha(colTextPrimary, AlphaFaint), rad, true)
	}
	// Rounded rim: a 1px accent border ties it to the rail glow.
	drawRoundedRect(dst, r, WithAlphaFromColor(accent, AlphaMedium), rad, false)

	cyMid := (r.Min.Y + r.Max.Y) / 2
	drawRect(dst, image.Rect(r.Min.X+3, cyMid-1, r.Max.X-3, cyMid), WithAlphaFromColor(color.Black, 150), true)
	drawRect(dst, image.Rect(r.Min.X+3, cyMid, r.Max.X-3, cyMid+1), WithAlphaFromColor(accent, AlphaStrong), true)

	val := w.liveValueLabel()
	vw := StyledTextWidth(val, RoleSectionHeader)
	vh := StyledTextHeight(RoleSectionHeader)
	vx := r.Min.X + (r.Dx()-vw)/2
	vy := r.Min.Y + (r.Dy()-vh)/2
	DrawTextStyled(dst, val, vx, vy+1, RoleSectionHeader, WithAlphaFromColor(color.Black, 160))
	DrawTextStyled(dst, val, vx, vy, RoleSectionHeader, colTextPrimary)
}

// Every rung is a tappable pill; rungRects() is the one geometry source so hit-test and draw agree.
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
			// A brighten, not the accent fill reserved for the active rung.
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

func (w *ValueWheel) resolveAccent() color.Color {
	if w.binding.Accent != nil {
		if c := w.binding.Accent(); c != nil {
			return c
		}
	}
	return genColorPrimary // package-level RGBA token
}

// At the active resolution, so a fine step still changes the displayed number.
func (w *ValueWheel) liveValueLabel() string {
	if m := w.model(); m != nil {
		return m.CenterLabel()
	}
	return ""
}
