package ui

import (
	"fmt"
	"image"
	"image/color"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

const (
	scopeHeaderH = 26 // 4px + 18px button + 4px
	scopePanelH  = 160
)

// scopeDisplayMode selects how A/B traces are rendered.
type scopeDisplayMode int

const (
	scopeOverlay scopeDisplayMode = iota // both traces overlaid
	scopeSplit                           // A top, B bottom
	scopeDiff                            // A-B difference waveform
)


// ScopeCallbacks contains callbacks for the ScopePanelZone to communicate
// with the DrumView and audio engine. Zones don't reference Game or each other.
type ScopeCallbacks struct {
	ScopeState     func() *scope.State
	ActiveRows     func() []*DrumRow
	OnTapAChange   func(stage scope.Stage)
	OnTapBChange   func(stage scope.Stage)
	OnClearTapA    func()
	OnClearTapB    func()
	OnFreezeToggle func() bool
	OnClose        func()
}

// ScopePanelZone implements the Zone interface for the oscilloscope panel.
// It provides A/B comparison of pipeline stages with interactive tap selection.
type ScopePanelZone struct {
	rect       image.Rectangle
	needLayout bool
	callbacks  ScopeCallbacks
	portal     *OverlayPortal

	// UI elements
	stageButtons [6]*Button // one per pipeline stage
	splitBtn     *Button    // split/overlay toggle
	autoGainBtn  *Button    // auto-gain toggle
	freezeBtn    *Button    // freeze toggle
	closeBtn     *Button    // close panel

	// Tap selection
	tapA scope.Stage // current A tap (-1 = none)
	tapB scope.Stage // current B tap (-1 = none)

	// Display state
	frozen      bool             // true when scope capture is frozen
	displayMode scopeDisplayMode // overlay, split, or diff
	windowMs    float64          // display window in ms (default 20, range [1, 500])
	yGain    float64 // Y-axis gain multiplier (default 1.0, range [0.25, 16.0])
	autoGain bool    // true = auto-scale Y to peak amplitude
	showTapA bool    // true = draw tap A trace (default true)
	showTapB bool    // true = draw tap B trace (default true)

	// WASM-only zone-local state. On desktop these stay zero-valued because
	// the audio.ScopeService() owns freeze + instrument selection.
	frozenState  *scope.State // cached snapshot held while frozen
	instrumentID string       // selected channel id, fed to snapshot calls

	// Hit areas cache (rebuilt on Layout)
	hitAreas []HitArea
}

// NewScopePanelZone creates a new ScopePanelZone with the provided callbacks.
func NewScopePanelZone(cb ScopeCallbacks) *ScopePanelZone {
	z := &ScopePanelZone{
		needLayout: true,
		callbacks:  cb,
		tapA:       -1,
		tapB:       -1,
		windowMs:   20,
		yGain:      1.0,
		showTapA:   true,
		showTapB:   true,
	}
	z.initButtons()
	return z
}

func (z *ScopePanelZone) initButtons() {
	stages := scope.AllStages()
	for i, stage := range stages {
		st := stage // capture
		z.stageButtons[i] = NewButton(scope.StageLabel(st), InstButtonStyle, func() {
			z.handleStageClick(st)
		})
	}

	z.autoGainBtn = NewButton("AG", InstButtonStyle, func() {
		z.autoGain = !z.autoGain
		if z.autoGain {
			z.yGain = 1.0 // reset manual gain when enabling auto
		}
	})
	z.splitBtn = NewButton("OVR", InstButtonStyle, func() {
		z.displayMode = (z.displayMode + 1) % 3
		switch z.displayMode {
		case scopeOverlay:
			z.splitBtn.Text = "OVR"
		case scopeSplit:
			z.splitBtn.Text = "SPL"
		case scopeDiff:
			z.splitBtn.Text = "DIF"
		}
	})
	// Freeze indicator uses single-character text per DESIGN.md §5d
	// (permitted text-glyph exception): "||" frozen, ">" resume; tinted
	// colTextSecondary inactive, colAccent when frozen.
	z.freezeBtn = NewButton("||", InstButtonStyle, func() {
		if z.callbacks.OnFreezeToggle != nil {
			z.frozen = z.callbacks.OnFreezeToggle()
			if z.frozen {
				z.freezeBtn.Text = ">"
				z.freezeBtn.TextColor = colAccent
			} else {
				z.freezeBtn.Text = "||"
				z.freezeBtn.TextColor = colTextSecondary
			}
		}
	})
	z.freezeBtn.TextColor = colTextSecondary
	// Close uses IconClose per DESIGN.md §5c (no raw "X" text in chrome).
	z.closeBtn = NewButton("", InstButtonStyle, func() {
		if z.callbacks.OnClose != nil {
			z.callbacks.OnClose()
		}
	})
	z.closeBtn.Icon = string(IconClose)
	z.closeBtn.IconColor = colTextSecondary
}

// SetTapA programmatically sets the TapA stage (use -1 to clear). Fires
// the OnTapAChange / OnClearTapA callbacks so the desktop audio service is
// kept in sync; on WASM those callbacks are no-ops and only zone state moves.
func (z *ScopePanelZone) SetTapA(stage scope.Stage) {
	if int(stage) < 0 {
		z.tapA = -1
		if z.callbacks.OnClearTapA != nil {
			z.callbacks.OnClearTapA()
		}
		return
	}
	z.tapA = stage
	if z.callbacks.OnTapAChange != nil {
		z.callbacks.OnTapAChange(stage)
	}
}

// SetTapB programmatically sets the TapB stage (use -1 to clear).
func (z *ScopePanelZone) SetTapB(stage scope.Stage) {
	if int(stage) < 0 {
		z.tapB = -1
		if z.callbacks.OnClearTapB != nil {
			z.callbacks.OnClearTapB()
		}
		return
	}
	z.tapB = stage
	if z.callbacks.OnTapBChange != nil {
		z.callbacks.OnTapBChange(stage)
	}
}

// handleStageClick implements the A/B tap selection logic:
//   - If stage == tapA, clear tapA
//   - If stage == tapB, clear tapB
//   - If tapA unset, assign tapA
//   - If tapB unset, assign tapB
//   - Otherwise, replace tapB
func (z *ScopePanelZone) handleStageClick(stage scope.Stage) {
	switch {
	case z.tapA == stage:
		z.tapA = -1
		if z.callbacks.OnClearTapA != nil {
			z.callbacks.OnClearTapA()
		}
	case z.tapB == stage:
		z.tapB = -1
		if z.callbacks.OnClearTapB != nil {
			z.callbacks.OnClearTapB()
		}
	case z.tapA < 0:
		z.tapA = stage
		if z.callbacks.OnTapAChange != nil {
			z.callbacks.OnTapAChange(stage)
		}
	case z.tapB < 0:
		z.tapB = stage
		if z.callbacks.OnTapBChange != nil {
			z.callbacks.OnTapBChange(stage)
		}
	default:
		z.tapB = stage
		if z.callbacks.OnTapBChange != nil {
			z.callbacks.OnTapBChange(stage)
		}
	}
}

// --- Zone interface ---

func (z *ScopePanelZone) ID() string { return "scope-panel" }

func (z *ScopePanelZone) NeedsLayout() bool { return z.needLayout }

func (z *ScopePanelZone) Invalidate() { z.needLayout = true }

func (z *ScopePanelZone) Layout(rect image.Rectangle) {
	z.rect = rect
	z.needLayout = false
	if rect.Dx() < 8 || rect.Dy() < 8 {
		z.hitAreas = z.hitAreas[:0]
		return
	}
	z.layoutButtons()
	z.rebuildHitAreas()
}

func (z *ScopePanelZone) Update() {}

func (z *ScopePanelZone) HitAreas() []HitArea {
	return z.hitAreas
}

func (z *ScopePanelZone) Draw(screen *ebiten.Image) {
	if z.rect.Dy() < 8 || z.rect.Dx() < 8 {
		return
	}

	// Background.
	drawRect(screen, z.rect, colEQBg, true)

	// Waveform content area.
	cr := z.contentRect()
	var state *scope.State
	if z.callbacks.ScopeState != nil {
		state = z.callbacks.ScopeState()
	}
	effectiveGain := z.yGain
	if z.autoGain && state != nil {
		peak := scopePeakAmplitude(state)
		if peak > 0.001 {
			effectiveGain = 0.9 / peak
			if effectiveGain < 1.0 {
				effectiveGain = 1.0
			}
			if effectiveGain > 16.0 {
				effectiveGain = 16.0
			}
		}
	}
	drawScopeTraces(screen, cr, state, z.windowMs, z.displayMode, z.frozen, effectiveGain, z.showTapA, z.showTapB)

	// Header.
	z.drawHeader(screen)

	// Border.
	drawRect(screen, image.Rect(z.rect.Min.X, z.rect.Min.Y, z.rect.Max.X, z.rect.Min.Y+1), colButtonBorder, true)
	drawRect(screen, image.Rect(z.rect.Min.X, z.rect.Max.Y-1, z.rect.Max.X, z.rect.Max.Y), colButtonBorder, true)
	drawRect(screen, image.Rect(z.rect.Min.X, z.rect.Min.Y, z.rect.Min.X+1, z.rect.Max.Y), colButtonBorder, true)
	drawRect(screen, image.Rect(z.rect.Max.X-1, z.rect.Min.Y, z.rect.Max.X, z.rect.Max.Y), colButtonBorder, true)
}

func (z *ScopePanelZone) HandleKey(_ ebiten.Key) InputResult {
	return InputIgnored
}

func (z *ScopePanelZone) HandleChars(_ []rune) InputResult {
	return InputIgnored
}

// contentRect returns the drawable area below the header.
func (z *ScopePanelZone) contentRect() image.Rectangle {
	return image.Rect(z.rect.Min.X, z.rect.Min.Y+scopeHeaderH, z.rect.Max.X, z.rect.Max.Y)
}

// SetPortal sets the portal reference for opening overlays.
func (z *ScopePanelZone) SetPortal(p *OverlayPortal) { z.portal = p }

// --- Header drawing ---

func (z *ScopePanelZone) drawHeader(dst *ebiten.Image) {
	captionScale := FontSizeCaption / FontSizeBody

	// Draw stage buttons with arrows between them and A/B badges.
	stages := scope.AllStages()
	for i, btn := range z.stageButtons {
		if btn == nil {
			continue
		}
		isA := z.tapA == stages[i]
		isB := z.tapB == stages[i]
		active := isA || isB
		z.drawPillButton(dst, btn, active, false)

		// A/B badge at top-right corner.
		if isA {
			z.drawBadge(dst, btn, "A", colScopeA)
		} else if isB {
			z.drawBadge(dst, btn, "B", colScopeB)
		}

		// Arrow between stages.
		if i < len(z.stageButtons)-1 {
			nextBtn := z.stageButtons[i+1]
			if nextBtn != nil {
				ax := btn.Rect().Max.X + 1
				ay := btn.Rect().Min.Y + btn.Rect().Dy()/2 - int(float64(TextHeight())*captionScale)/2
				DrawTextColorAtScale(dst, ">", ax, ay, colTextSecondary, captionScale)
			}
		}
	}

	// Zoom text between stage strip and right-side buttons.
	zoomText := formatWindowMs(z.windowMs)
	if z.yGain != 1.0 {
		zoomText += fmt.Sprintf(" Y:%.3gx", z.yGain)
	}
	zoomW := int(float64(TextWidth(zoomText)) * captionScale)
	zoomX := z.splitBtn.Rect().Min.X - zoomW - 6
	zoomY := z.rect.Min.Y + 4 + (18-int(float64(TextHeight())*captionScale))/2
	DrawTextColorAtScale(dst, zoomText, zoomX, zoomY, colTextSecondary, captionScale)

	// Split, auto-gain, freeze, and close pill buttons.
	z.drawPillButton(dst, z.splitBtn, z.displayMode != scopeOverlay, false)
	z.drawPillButton(dst, z.autoGainBtn, z.autoGain, false)
	z.drawPillButton(dst, z.freezeBtn, z.frozen, false)
	z.drawPillButton(dst, z.closeBtn, false, false)
}

// drawPillButton draws a button with pill styling (rounded-ish filled rect).
// When disabled is true the button is drawn dimmed and non-interactive.
func (z *ScopePanelZone) drawPillButton(dst *ebiten.Image, btn *Button, active, disabled bool) {
	r := btn.Rect()
	if r.Empty() {
		return
	}

	if active {
		drawRect(dst, r, colSurface2, true)
		// Accent border for active.
		drawRect(dst, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1), colAccent, true)
		drawRect(dst, image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y), colAccent, true)
		drawRect(dst, image.Rect(r.Min.X, r.Min.Y, r.Min.X+1, r.Max.Y), colAccent, true)
		drawRect(dst, image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y), colAccent, true)
	} else {
		// Outline only.
		drawRect(dst, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1), colButtonBorder, true)
		drawRect(dst, image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y), colButtonBorder, true)
		drawRect(dst, image.Rect(r.Min.X, r.Min.Y, r.Min.X+1, r.Max.Y), colButtonBorder, true)
		drawRect(dst, image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y), colButtonBorder, true)
	}

	// Center text.
	captionScale := FontSizeCaption / FontSizeBody
	tw := int(float64(TextWidth(btn.Text)) * captionScale)
	th := int(float64(TextHeight()) * captionScale)
	tx := r.Min.X + (r.Dx()-tw)/2
	ty := r.Min.Y + (r.Dy()-th)/2
	var textCol color.Color = colTextSecondary
	if active {
		textCol = colTextAccent
	}
	if disabled {
		textCol = WithAlpha(genColorScopeLabelDim, genAlphaScopeLabelDim) // dimmed
	}
	DrawTextColorAtScale(dst, btn.Text, tx, ty, textCol, captionScale)
}

// drawBadge draws a small colored badge ("A" or "B") at the top-right of a button.
func (z *ScopePanelZone) drawBadge(dst *ebiten.Image, btn *Button, label string, col color.Color) {
	r := btn.Rect()
	badgeW := 8
	badgeH := 8
	bx := r.Max.X - badgeW
	by := r.Min.Y
	drawRect(dst, image.Rect(bx, by, bx+badgeW, by+badgeH), col, true)

	// Badge text.
	badgeScale := FontSizeCaption / FontSizeBody * 0.7
	tw := int(float64(TextWidth(label)) * badgeScale)
	th := int(float64(TextHeight()) * badgeScale)
	DrawTextColorAtScale(dst, label, bx+(badgeW-tw)/2, by+(badgeH-th)/2, colTextPrimary, badgeScale)
}

// --- Layout ---

func (z *ScopePanelZone) layoutButtons() {
	r := z.rect
	btnH := 18
	y := r.Min.Y + 4
	x := r.Min.X + 6

	captionScale := FontSizeCaption / FontSizeBody

	// Stage buttons in a row with 3px gaps; arrows between take ~8px.
	arrowGap := int(float64(TextWidth(">"))*captionScale) + 2
	for i, btn := range z.stageButtons {
		if btn == nil {
			continue
		}
		tw := int(float64(TextWidth(btn.Text))*captionScale) + 10
		if tw < 28 {
			tw = 28
		}
		btn.SetRect(image.Rect(x, y, x+tw, y+btnH))
		x += tw
		if i < len(z.stageButtons)-1 {
			x += arrowGap
		}
	}

	// Right-aligned: close, freeze, and split buttons.
	rightEdge := r.Max.X - 6
	closeW := int(float64(TextWidth(z.closeBtn.Text))*captionScale) + 12
	if closeW < 24 {
		closeW = 24
	}
	z.closeBtn.SetRect(image.Rect(rightEdge-closeW, y, rightEdge, y+btnH))
	rightEdge -= closeW + 3

	freezeW := int(float64(TextWidth(z.freezeBtn.Text))*captionScale) + 12
	if freezeW < 24 {
		freezeW = 24
	}
	z.freezeBtn.SetRect(image.Rect(rightEdge-freezeW, y, rightEdge, y+btnH))
	rightEdge -= freezeW + 3

	agW := int(float64(TextWidth(z.autoGainBtn.Text))*captionScale) + 12
	if agW < 24 {
		agW = 24
	}
	z.autoGainBtn.SetRect(image.Rect(rightEdge-agW, y, rightEdge, y+btnH))
	rightEdge -= agW + 3

	splitW := int(float64(TextWidth(z.splitBtn.Text))*captionScale) + 12
	if splitW < 28 {
		splitW = 28
	}
	z.splitBtn.SetRect(image.Rect(rightEdge-splitW, y, rightEdge, y+btnH))
}

// --- Hit areas ---

func (z *ScopePanelZone) rebuildHitAreas() {
	z.hitAreas = z.hitAreas[:0]

	const zIdx = 140

	// Scroll wheel zoom area on content rect.
	cr := z.contentRect()
	if !cr.Empty() {
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:    cr,
			ZIndex:  zIdx,
			Handler: &scopeZoomHandler{zone: z},
			Tag:     "scope-zoom",
		})
	}

	// Stage buttons.
	for i, btn := range z.stageButtons {
		if btn == nil {
			continue
		}
		if br := btn.Rect(); !br.Empty() {
			z.hitAreas = append(z.hitAreas, HitArea{
				Rect:    br,
				ZIndex:  zIdx + 1,
				Handler: &buttonHitAdapter{btn: btn},
				Tag:     fmt.Sprintf("scope-stage-%d", i),
			})
		}
	}

	// Auto-gain button.
	if ar := z.autoGainBtn.Rect(); !ar.Empty() {
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:    ar,
			ZIndex:  zIdx + 1,
			Handler: &buttonHitAdapter{btn: z.autoGainBtn},
			Tag:     "scope-ag-btn",
		})
	}

	// Split/overlay button.
	if sr := z.splitBtn.Rect(); !sr.Empty() {
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:    sr,
			ZIndex:  zIdx + 1,
			Handler: &buttonHitAdapter{btn: z.splitBtn},
			Tag:     "scope-split-btn",
		})
	}

	// Freeze button.
	if fr := z.freezeBtn.Rect(); !fr.Empty() {
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:    fr,
			ZIndex:  zIdx + 1,
			Handler: &buttonHitAdapter{btn: z.freezeBtn},
			Tag:     "scope-freeze-btn",
		})
	}

	// Close button.
	if cr := z.closeBtn.Rect(); !cr.Empty() {
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:    cr,
			ZIndex:  zIdx + 1,
			Handler: &buttonHitAdapter{btn: z.closeBtn},
			Tag:     "scope-close-btn",
		})
	}

	// Trace visibility swatch hit areas (fixed position in top-right of content rect).
	// Positioned to align with the legend text drawn by drawScopeOverlay at
	// waveRect.Min.Y + 2 (legend line A) and + lh + 4 (legend line B).
	contentR := z.contentRect()
	if !contentR.Empty() {
		swatchW, swatchH := 20, 12
		captionScale := FontSizeCaption / FontSizeBody
		lh := int(float64(TextHeight()) * captionScale)
		sx := contentR.Max.X - swatchW - 4
		syA := contentR.Min.Y + 2
		syB := syA + lh + 2
		swatchA := image.Rect(sx, syA, sx+swatchW, syA+swatchH)
		swatchB := image.Rect(sx, syB, sx+swatchW, syB+swatchH)
		z.hitAreas = append(z.hitAreas,
			HitArea{Rect: swatchA, ZIndex: zIdx + 2, Handler: &scopeSwatchHandler{zone: z, tap: "A"}, Tag: "scope-swatch-a"},
			HitArea{Rect: swatchB, ZIndex: zIdx + 2, Handler: &scopeSwatchHandler{zone: z, tap: "B"}, Tag: "scope-swatch-b"},
		)
	}
}

// scopeSwatchHandler toggles visibility of a trace when its legend swatch is clicked.
type scopeSwatchHandler struct {
	zone *ScopePanelZone
	tap  string // "A" or "B"
}

func (h *scopeSwatchHandler) OnPress(x, y int) InputResult {
	switch h.tap {
	case "A":
		h.zone.showTapA = !h.zone.showTapA
	case "B":
		h.zone.showTapB = !h.zone.showTapB
	}
	return InputConsumed
}
func (h *scopeSwatchHandler) OnDrag(x, y int)               {}
func (h *scopeSwatchHandler) OnRelease(x, y int)            {}
func (h *scopeSwatchHandler) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// --- Scroll wheel zoom handler ---

// scopeZoomHandler implements HitHandler for scroll-wheel zoom on the scope content area.
// Plain scroll adjusts time window; Shift+scroll adjusts Y-axis gain.
// Double-click resets both axes to defaults.
type scopeZoomHandler struct {
	zone          *ScopePanelZone
	lastPressTime int64 // monotonic ms for double-click detection
}

func (h *scopeZoomHandler) OnPress(x, y int) InputResult {
	now := time.Now().UnixMilli()
	if now-h.lastPressTime < 300 {
		// Double-click: reset both axes.
		h.zone.windowMs = 20
		h.zone.yGain = 1.0
		h.lastPressTime = 0
		return InputConsumed
	}
	h.lastPressTime = now
	return InputIgnored
}
func (h *scopeZoomHandler) OnDrag(x, y int)    {}
func (h *scopeZoomHandler) OnRelease(x, y int) {}
func (h *scopeZoomHandler) OnWheel(x, y, steps int) InputResult {
	if steps == 0 {
		return InputIgnored
	}
	shift := isKeyPressed(ebiten.KeyShiftLeft) || isKeyPressed(ebiten.KeyShiftRight)
	if shift {
		// Shift+Scroll: Y-axis gain zoom. Disables auto-gain.
		h.zone.autoGain = false
		factor := 1 + float64(steps)*0.15
		h.zone.yGain *= factor
		if h.zone.yGain < 0.25 {
			h.zone.yGain = 0.25
		}
		if h.zone.yGain > 16.0 {
			h.zone.yGain = 16.0
		}
	} else {
		// Plain scroll: time-axis zoom.
		factor := 1 + float64(steps)*0.15
		h.zone.windowMs *= factor
		if h.zone.windowMs < 1 {
			h.zone.windowMs = 1
		}
		if h.zone.windowMs > 500 {
			h.zone.windowMs = 500
		}
	}
	return InputConsumed
}
