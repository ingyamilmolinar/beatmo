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
	chainHeaderH = 26 // 4px + 18px button + 4px
	chainPanelH  = 160
)

// chainDisplayMode selects how A/B traces are rendered.
type chainDisplayMode int

const (
	chainOverlay chainDisplayMode = iota // both traces overlaid
	chainSplit                           // A top, B bottom
	chainDiff                            // A-B difference waveform
)


// ChainCallbacks contains callbacks for the ChainPanelZone to communicate
// with the DrumView and audio engine. Zones don't reference Game or each other.
type ChainCallbacks struct {
	ScopeState     func() *scope.State
	ActiveRows     func() []*DrumRow
	OnTapAChange   func(stage scope.Stage)
	OnTapBChange   func(stage scope.Stage)
	OnClearTapA    func()
	OnClearTapB    func()
	OnFreezeToggle func() bool
	OnClose        func()
}

// ChainPanelZone implements the Zone interface for the oscilloscope panel.
// It provides A/B comparison of pipeline stages with interactive tap selection.
type ChainPanelZone struct {
	rect       image.Rectangle
	needLayout bool
	callbacks  ChainCallbacks
	portal     *OverlayPortal

	// UI elements
	stageButtons [6]*Button // one per pipeline stage (vertical column)
	overlayBtn   *Button    // pill: overlay traces (top-right)
	splitBtn     *Button    // pill: split A/B (top-right)
	diffBtn      *Button    // pill: difference (top-right)
	autoGainBtn  *Button    // auto-gain toggle (corner of trace area)
	freezeBtn    *Button    // freeze toggle
	closeBtn     *Button    // close panel

	// Hover-dwell tooltip state (desktop only). The tooltip itself lives
	// in a TooltipOverlay opened on the shared OverlayPortal once the
	// cursor has lingered ~300ms over a control.
	hoverStartMs int64
	hoveredID    string // "stage-N" or "mode-overlay/split/diff"; empty when no hover

	// Tap selection
	tapA scope.Stage // current A tap (-1 = none)
	tapB scope.Stage // current B tap (-1 = none)

	// Display state
	frozen      bool             // true when scope capture is frozen
	displayMode chainDisplayMode // overlay, split, or diff
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

// NewChainPanelZone creates a new ChainPanelZone with the provided callbacks.
func NewChainPanelZone(cb ChainCallbacks) *ChainPanelZone {
	z := &ChainPanelZone{
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

func (z *ChainPanelZone) initButtons() {
	stages := scope.AllStages()
	for i, stage := range stages {
		st := stage // capture
		z.stageButtons[i] = NewButton(scope.StageLabel(st), InstButtonStyle, func() {
			z.handleStageClick(st)
		})
	}

	z.autoGainBtn = NewButton("AG", InstButtonStyle, func() {
		z.SetAutoGain(!z.autoGain)
	})
	// Three separate mode pills replace the previous OVR/SPL/DIF cycle button.
	// Clicking each sets the displayMode directly (no implicit cycle).
	z.overlayBtn = NewButton("OVR", InstButtonStyle, func() {
		z.displayMode = chainOverlay
	})
	z.splitBtn = NewButton("SPL", InstButtonStyle, func() {
		z.displayMode = chainSplit
	})
	z.diffBtn = NewButton("DIF", InstButtonStyle, func() {
		z.displayMode = chainDiff
	})
	// Freeze indicator uses single-character text per DESIGN.md §5d
	// (permitted text-glyph exception): "||" frozen, ">" resume; tinted
	// colTextSecondary inactive, colAccent when frozen.
	z.freezeBtn = NewButton("||", InstButtonStyle, func() {
		z.SetFrozen(!z.frozen)
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
func (z *ChainPanelZone) SetTapA(stage scope.Stage) {
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
func (z *ChainPanelZone) SetTapB(stage scope.Stage) {
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
func (z *ChainPanelZone) handleStageClick(stage scope.Stage) {
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

func (z *ChainPanelZone) ID() string { return "chain-panel" }

// Rect returns the full scope-panel rectangle (header + content area).
// Returns the zero Rectangle before Layout has run.
func (z *ChainPanelZone) Rect() image.Rectangle { return z.rect }

// ContentRect returns the trace drawing area (rect minus the header strip).
func (z *ChainPanelZone) ContentRect() image.Rectangle { return z.contentRect() }

// Frozen reports whether the scope is currently frozen.
func (z *ChainPanelZone) Frozen() bool { return z.frozen }

// AutoGain reports whether the scope is auto-gain.
func (z *ChainPanelZone) AutoGain() bool { return z.autoGain }

// TraceVisible reports whether the named trace ("A" or "B") is currently
// drawn. Unknown labels return false.
func (z *ChainPanelZone) TraceVisible(label string) bool {
	switch label {
	case "A":
		return z.showTapA
	case "B":
		return z.showTapB
	}
	return false
}

// WindowMs returns the current X-axis time window (ms).
func (z *ChainPanelZone) WindowMs() float64 { return z.windowMs }

// YGain returns the current Y-axis gain factor.
func (z *ChainPanelZone) YGain() float64 { return z.yGain }

// SetFrozen drives the same freeze flow the freeze-button click takes,
// including the audio-service callback and the button label/color update.
// Idempotent: SetFrozen(z.Frozen()) is a no-op.
//
// UI state is authoritative: z.frozen always ends up matching `frozen`,
// regardless of what OnFreezeToggle reports. The callback is fired so
// the audio service can mirror the change, but if it short-circuits or
// the audio side is unavailable (test stub, WASM-fallback with no
// snapshots) the UI still latches the requested state.
func (z *ChainPanelZone) SetFrozen(frozen bool) {
	if z.frozen == frozen {
		return
	}
	if z.callbacks.OnFreezeToggle != nil {
		// Fire-and-best-effort: ask the callback to flip the audio
		// side, but ignore its return — we don't let the audio side
		// veto the requested UI state.
		_ = z.callbacks.OnFreezeToggle()
	}
	z.frozen = frozen
	if z.freezeBtn != nil {
		if z.frozen {
			z.freezeBtn.Text = ">"
			z.freezeBtn.TextColor = colAccent
		} else {
			z.freezeBtn.Text = "||"
			z.freezeBtn.TextColor = colTextSecondary
		}
	}
}

// SetAutoGain drives the same auto-gain flow the AG-button click takes
// (resets manual gain when enabling). Idempotent.
func (z *ChainPanelZone) SetAutoGain(on bool) {
	if z.autoGain == on {
		return
	}
	z.autoGain = on
	if z.autoGain {
		z.yGain = 1.0
	}
}

// SetTraceVisible toggles the visibility of trace A or B. Matches the
// per-swatch click handler (chainSwatchHandler) so scene Setup can produce
// the same visual state as a user click without synthesizing input.
// Unknown labels are ignored.
func (z *ChainPanelZone) SetTraceVisible(label string, visible bool) {
	switch label {
	case "A":
		z.showTapA = visible
	case "B":
		z.showTapB = visible
	}
}

// SetWindowMs sets the X-axis time window directly, clamped to the same
// [1, 500] range the wheel handler enforces.
func (z *ChainPanelZone) SetWindowMs(ms float64) {
	if ms < 1 {
		ms = 1
	}
	if ms > 500 {
		ms = 500
	}
	z.windowMs = ms
}

// SetYGain sets the Y-axis gain directly, clamped to [0.25, 16.0]. Like
// the shift-scroll path, calling this disables auto-gain so manual zoom
// sticks until the user re-enables AG.
func (z *ChainPanelZone) SetYGain(gain float64) {
	if gain < 0.25 {
		gain = 0.25
	}
	if gain > 16.0 {
		gain = 16.0
	}
	z.autoGain = false
	z.yGain = gain
}

func (z *ChainPanelZone) NeedsLayout() bool { return z.needLayout }

func (z *ChainPanelZone) Invalidate() { z.needLayout = true }

func (z *ChainPanelZone) Layout(rect image.Rectangle) {
	z.rect = rect
	z.needLayout = false
	if rect.Dx() < 8 || rect.Dy() < 8 {
		z.hitAreas = z.hitAreas[:0]
		return
	}
	z.layoutButtons()
	z.rebuildHitAreas()
}

// Update drives the hover-dwell tooltip: when the cursor sits over a stage
// thumbnail or a mode pill for ~300ms we open a TooltipOverlay through the
// shared OverlayPortal. Mobile profiles skip hover entirely (no cursor).
func (z *ChainPanelZone) Update() {
	if Profile().IsMobile() {
		return
	}
	if z.portal == nil {
		return
	}
	mx, my := cursorPosition()
	pt := image.Pt(mx, my)

	cur := ""
	var anchor image.Rectangle
	var text string

	for i, btn := range z.stageButtons {
		if btn == nil {
			continue
		}
		if pt.In(btn.Rect()) {
			cur = fmt.Sprintf("stage-%d", i)
			anchor = btn.Rect()
			stages := scope.AllStages()
			if i < len(stages) {
				text = scope.StageDescription(stages[i])
			}
			break
		}
	}
	if cur == "" {
		modePills := []struct {
			id  string
			btn *Button
			tip string
		}{
			{"mode-overlay", z.overlayBtn, "Overlay traces"},
			{"mode-split", z.splitBtn, "Split top/bottom"},
			{"mode-diff", z.diffBtn, "Show difference"},
		}
		for _, mp := range modePills {
			if mp.btn == nil {
				continue
			}
			if pt.In(mp.btn.Rect()) {
				cur = mp.id
				anchor = mp.btn.Rect()
				text = mp.tip
				break
			}
		}
	}

	now := time.Now().UnixMilli()
	if cur == "" {
		if z.hoveredID != "" {
			z.portal.Close("chain-tt")
			z.hoveredID = ""
			z.hoverStartMs = 0
		}
		return
	}
	if cur != z.hoveredID {
		// Cursor moved to a different control: close any prior tooltip
		// and re-arm the dwell timer for the new control.
		z.portal.Close("chain-tt")
		z.hoveredID = cur
		z.hoverStartMs = now
		return
	}
	if z.hoverStartMs == 0 {
		// Just entered this control — start the dwell timer.
		z.hoverStartMs = now
		return
	}
	if now-z.hoverStartMs >= 300 {
		// Dwell threshold crossed: open the tooltip. Open is idempotent
		// against duplicate IDs (closeByID happens first in OverlayPortal.Open),
		// so re-open per-frame is cheap and self-correcting.
		if !z.portal.Has("chain-tt") {
			z.portal.Open(PortalEntry{
				ID:      "chain-tt",
				Owner:   z,
				Overlay: NewTooltipOverlay(text),
				Modal:   false,
				Anchor:  anchor,
			})
		}
	}
}

func (z *ChainPanelZone) HitAreas() []HitArea {
	return z.hitAreas
}

func (z *ChainPanelZone) Draw(screen *ebiten.Image) {
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
		peak := chainPeakAmplitude(state)
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
	drawChainTraces(screen, cr, state, z.windowMs, z.displayMode, z.frozen, effectiveGain, z.showTapA, z.showTapB)

	// Header.
	z.drawHeader(screen)

	// Border.
	drawRect(screen, image.Rect(z.rect.Min.X, z.rect.Min.Y, z.rect.Max.X, z.rect.Min.Y+1), colButtonBorder, true)
	drawRect(screen, image.Rect(z.rect.Min.X, z.rect.Max.Y-1, z.rect.Max.X, z.rect.Max.Y), colButtonBorder, true)
	drawRect(screen, image.Rect(z.rect.Min.X, z.rect.Min.Y, z.rect.Min.X+1, z.rect.Max.Y), colButtonBorder, true)
	drawRect(screen, image.Rect(z.rect.Max.X-1, z.rect.Min.Y, z.rect.Max.X, z.rect.Max.Y), colButtonBorder, true)
}

func (z *ChainPanelZone) HandleKey(_ ebiten.Key) InputResult {
	return InputIgnored
}

func (z *ChainPanelZone) HandleChars(_ []rune) InputResult {
	return InputIgnored
}

// contentRect returns the drawable trace area: to the right of the vertical
// stage column and below the chrome strip at the top.
func (z *ChainPanelZone) contentRect() image.Rectangle {
	left := z.rect.Min.X + chainStageColW + 8
	top := z.rect.Min.Y + chainHeaderH
	if left >= z.rect.Max.X || top >= z.rect.Max.Y {
		return image.Rectangle{}
	}
	return image.Rect(left, top, z.rect.Max.X, z.rect.Max.Y)
}

// SetPortal sets the portal reference for opening overlays.
func (z *ChainPanelZone) SetPortal(p *OverlayPortal) { z.portal = p }

// --- Header drawing ---

func (z *ChainPanelZone) drawHeader(dst *ebiten.Image) {
	captionScale := FontSizeCaption / FontSizeBody

	// Draw vertical stage thumbnails on the left, with A/B badges. No arrows
	// between rows — the column itself is the visual chain.
	stages := scope.AllStages()
	for i, btn := range z.stageButtons {
		if btn == nil {
			continue
		}
		isA := i < len(stages) && z.tapA == stages[i]
		isB := i < len(stages) && z.tapB == stages[i]
		active := isA || isB
		z.drawPillButton(dst, btn, active, false)
		if isA {
			z.drawBadge(dst, btn, "A", colScopeA)
		} else if isB {
			z.drawBadge(dst, btn, "B", colScopeB)
		}
	}

	// Zoom readout: tucked just left of the OVR pill on the top-right strip.
	zoomText := formatWindowMs(z.windowMs)
	if z.yGain != 1.0 {
		zoomText += fmt.Sprintf(" Y:%.3gx", z.yGain)
	}
	zoomW := int(float64(TextWidth(zoomText)) * captionScale)
	zoomX := z.overlayBtn.Rect().Min.X - zoomW - 6
	zoomY := z.rect.Min.Y + 4 + (18-int(float64(TextHeight())*captionScale))/2
	DrawTextColorAtScale(dst, zoomText, zoomX, zoomY, colTextSecondary, captionScale)

	// Three separate mode pills on the top-right.
	z.drawPillButton(dst, z.overlayBtn, z.displayMode == chainOverlay, false)
	z.drawPillButton(dst, z.splitBtn, z.displayMode == chainSplit, false)
	z.drawPillButton(dst, z.diffBtn, z.displayMode == chainDiff, false)
	z.drawPillButton(dst, z.autoGainBtn, z.autoGain, false)
	z.drawPillButton(dst, z.freezeBtn, z.frozen, false)
	z.drawPillButton(dst, z.closeBtn, false, false)
}

// drawPillButton draws a button with pill styling (rounded-ish filled rect).
// When disabled is true the button is drawn dimmed and non-interactive.
func (z *ChainPanelZone) drawPillButton(dst *ebiten.Image, btn *Button, active, disabled bool) {
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
func (z *ChainPanelZone) drawBadge(dst *ebiten.Image, btn *Button, label string, col color.Color) {
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

const (
	// chainStageColW is the width of the left-side vertical stage column.
	// chainStageRowH is the height of one stage thumbnail; with a 2px gap
	// between rows, six stages stack to ~144px (fits the 160px panel).
	chainStageColW = 56
	chainStageRowH = 22
)

func (z *ChainPanelZone) layoutButtons() {
	r := z.rect
	btnH := 18

	// --- Vertical stage column on the LEFT ---
	colX := r.Min.X + 4
	colY0 := r.Min.Y + 4
	for i, btn := range z.stageButtons {
		if btn == nil {
			continue
		}
		yTop := colY0 + i*(chainStageRowH+2)
		btn.SetRect(image.Rect(colX, yTop, colX+chainStageColW, yTop+chainStageRowH))
	}

	// --- Right-aligned chrome row at TOP, just above the trace area ---
	y := r.Min.Y + 4
	rightEdge := r.Max.X - 6

	// Close (icon-only) furthest right.
	closeW := 24
	z.closeBtn.SetRect(image.Rect(rightEdge-closeW, y, rightEdge, y+btnH))
	rightEdge -= closeW + 3

	// Freeze.
	freezeW := 24
	z.freezeBtn.SetRect(image.Rect(rightEdge-freezeW, y, rightEdge, y+btnH))
	rightEdge -= freezeW + 3

	// Three mode pills (DIF rightmost, then SPL, then OVR — drawn so OVR is leftmost).
	const pillW = 32
	for _, btn := range []*Button{z.diffBtn, z.splitBtn, z.overlayBtn} {
		btn.SetRect(image.Rect(rightEdge-pillW, y, rightEdge, y+btnH))
		rightEdge -= pillW + 3
	}

	// Auto-gain button now sits in the lower-right corner of the trace area
	// (DESIGN.md tooltip-anchor pattern: corner-of-content rather than chrome).
	cr := z.contentRect()
	if !cr.Empty() {
		const agW = 26
		const agH = 16
		z.autoGainBtn.SetRect(image.Rect(cr.Max.X-agW-4, cr.Max.Y-agH-4, cr.Max.X-4, cr.Max.Y-4))
	} else {
		// Fallback when the trace area is too small to host the AG chip:
		// put it next to freeze so it stays clickable for tests/scenes.
		agW := 24
		z.autoGainBtn.SetRect(image.Rect(rightEdge-agW, y, rightEdge, y+btnH))
	}
}

// --- Hit areas ---

func (z *ChainPanelZone) rebuildHitAreas() {
	z.hitAreas = z.hitAreas[:0]

	const zIdx = 140

	// Scroll wheel zoom area on content rect.
	cr := z.contentRect()
	if !cr.Empty() {
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:    cr,
			ZIndex:  zIdx,
			Handler: &chainZoomHandler{zone: z},
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

	// Three separate mode pills (overlay, split, diff). The legacy
	// "scope-split-btn" tag is preserved on the split pill so older callers
	// and discipline tests that expect it still resolve.
	if sr := z.overlayBtn.Rect(); !sr.Empty() {
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:    sr,
			ZIndex:  zIdx + 1,
			Handler: &buttonHitAdapter{btn: z.overlayBtn},
			Tag:     "scope-overlay-btn",
		})
	}
	if sr := z.splitBtn.Rect(); !sr.Empty() {
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:    sr,
			ZIndex:  zIdx + 1,
			Handler: &buttonHitAdapter{btn: z.splitBtn},
			Tag:     "scope-split-btn",
		})
	}
	if sr := z.diffBtn.Rect(); !sr.Empty() {
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:    sr,
			ZIndex:  zIdx + 1,
			Handler: &buttonHitAdapter{btn: z.diffBtn},
			Tag:     "scope-diff-btn",
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
	// Positioned to align with the legend text drawn by drawChainOverlay at
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
			HitArea{Rect: swatchA, ZIndex: zIdx + 2, Handler: &chainSwatchHandler{zone: z, tap: "A"}, Tag: "scope-swatch-a"},
			HitArea{Rect: swatchB, ZIndex: zIdx + 2, Handler: &chainSwatchHandler{zone: z, tap: "B"}, Tag: "scope-swatch-b"},
		)
	}
}

// chainSwatchHandler toggles visibility of a trace when its legend swatch is clicked.
type chainSwatchHandler struct {
	zone *ChainPanelZone
	tap  string // "A" or "B"
}

func (h *chainSwatchHandler) OnPress(x, y int) InputResult {
	h.zone.SetTraceVisible(h.tap, !h.zone.TraceVisible(h.tap))
	return InputConsumed
}
func (h *chainSwatchHandler) OnDrag(x, y int)               {}
func (h *chainSwatchHandler) OnRelease(x, y int)            {}
func (h *chainSwatchHandler) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// --- Scroll wheel zoom handler ---

// chainZoomHandler implements HitHandler for scroll-wheel zoom on the scope content area.
// Plain scroll adjusts time window; Shift+scroll adjusts Y-axis gain.
// Double-click resets both axes to defaults.
type chainZoomHandler struct {
	zone          *ChainPanelZone
	lastPressTime int64 // monotonic ms for double-click detection
}

func (h *chainZoomHandler) OnPress(x, y int) InputResult {
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
func (h *chainZoomHandler) OnDrag(x, y int)    {}
func (h *chainZoomHandler) OnRelease(x, y int) {}
func (h *chainZoomHandler) OnWheel(x, y, steps int) InputResult {
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
