package ui

import (
	"fmt"
	"image"
	"image/color"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

// Common DAW English names, routed through i18n so the scope package needs no i18n dependency.
func chainStageKey(st scope.Stage) i18n.Key {
	switch st {
	case scope.StageSynth:
		return i18n.KeyChainStageSynth
	case scope.StageAntiPop:
		return i18n.KeyChainStageAntiPop
	case scope.StageInsertFX:
		return i18n.KeyChainStageFX
	case scope.StageEQ:
		return i18n.KeyChainStageEQ
	case scope.StageMaster:
		return i18n.KeyMaster
	}
	return ""
}

// Unknown stages fall back to the scope package's English label.
func chainStageLabelLocalized(st scope.Stage) string {
	if k := chainStageKey(st); k != "" {
		return i18n.T(k)
	}
	return scope.StageLabel(st)
}

const (
	chainHeaderH = 32 // header band: shared audioPillHeight() pill + vertical centering
	chainPanelH  = 166
)

// chainDisplayMode selects how A/B traces are rendered.
type chainDisplayMode int

const (
	chainOverlay chainDisplayMode = iota // both traces overlaid
	chainSplit                           // A top, B bottom
	chainDiff                            // A-B difference waveform
)

// Zones never reference Game or each other.
type ChainCallbacks struct {
	ScopeState     func() *scope.State
	ActiveRows     func() []*DrumRow
	OnTapAChange   func(stage scope.Stage)
	OnTapBChange   func(stage scope.Stage)
	OnClearTapA    func()
	OnClearTapB    func()
	OnFreezeToggle func() bool

	// Turns the stage column into a live signal-flow display. -Inf = dark; nil disables it.
	StagePeak func(stage scope.Stage) (peakDB, rmsDB float64)

	// Belt-and-suspenders like EQPanelZone's: true makes HitAreas() return nil.
	IsHiddenForInput func() bool
}

type ChainPanelZone struct {
	rect       image.Rectangle
	needLayout bool
	callbacks  ChainCallbacks
	portal     *OverlayPortal

	// UI elements
	stageButtons [scope.NumStages]*Button // one per pipeline stage (vertical column)
	overlayBtn   *Button                  // pill: overlay traces (top-right)
	splitBtn     *Button                  // pill: split A/B (top-right)
	diffBtn      *Button                  // pill: difference (top-right)
	freezeBtn    *Button                  // freeze toggle

	// 0 on desktop; contentRect() drops the trace below it so the segmented control never overlaps it.
	segmentedRowH int

	// Desktop only; the tooltip opens on the shared portal after ~300 ms over a control.
	hoverStartMs int64
	hoveredID    string // "stage-N" or "mode-overlay/split/diff"; empty when no hover

	// Tap selection
	tapA scope.Stage // current A tap (-1 = none)
	tapB scope.Stage // current B tap (-1 = none)

	// Display state
	frozen      bool             // true when scope capture is frozen
	displayMode chainDisplayMode // overlay, split, or diff
	windowMs    float64          // display window in ms (default 20, range [1, 500])
	yGain       float64          // Y-axis gain multiplier (default 1.0, range [0.25, 16.0])
	autoGain    bool             // true = auto-scale Y to peak amplitude
	autoFit     bool             // true = auto-fit the X window to the signal's active span
	showTapA    bool             // true = draw tap A trace (default true)
	showTapB    bool             // true = draw tap B trace (default true)

	// Reused so Draw never allocates a State per frame.
	fitState scope.State

	// The trace is the panel's hottest renderer, and the scope publishes at ~30 Hz while Draw runs at
	// 60, so render once per state/layout change and blit otherwise; that frees the single WASM thread for
	// the sequencer. Pixel-identical to a direct render.
	traceCache *ebiten.Image
	traceKey   chainTraceKey
	traceKeyOK bool
	// Pointer identity is the freshness signal: ScopeState() returns a new pointer on every refresh, and
	// the WASM path leaves Timestamp 0, so keying on it would freeze the trace. Holding it stops address reuse.
	traceState *scope.State

	// WASM only; on desktop the scope service owns freeze and instrument selection.
	frozenState  *scope.State // cached snapshot held while frozen
	instrumentID channel.ID   // selected channel, fed to snapshot calls

	// Hit areas cache (rebuilt on Layout)
	hitAreas []HitArea
}

// Tap A defaults to Synth so the tab opens on a signal, and the callback fires once so the desktop
// service mirrors it.
func NewChainPanelZone(cb ChainCallbacks) *ChainPanelZone {
	z := &ChainPanelZone{
		instrumentID: channel.Master,
		needLayout:   true,
		callbacks:    cb,
		tapA:         scope.StageSynth,
		tapB:         -1,
		windowMs:     20,
		yGain:        1.0,
		// Both framing axes default ON: opt-in pills left the default trace correct but unreadable. Manual
		// setters turn an axis back to manual.
		autoGain: true,
		autoFit:  true,
		showTapA: true,
		showTapB: true,
	}
	z.initButtons()
	if cb.OnTapAChange != nil {
		cb.OnTapAChange(scope.StageSynth)
	}
	return z
}

func (z *ChainPanelZone) initButtons() {
	stages := scope.AllStages()
	for i, stage := range stages {
		st := stage // capture
		z.stageButtons[i] = NewButton(scope.StageLabel(st), InstButtonStyle, func() {
			z.handleStageClick(st)
		})
		z.stageButtons[i].SetTextKey(chainStageKey(st))
	}

	z.overlayBtn = NewButton("OVR", InstButtonStyle, func() {
		z.displayMode = chainOverlay
	})
	z.overlayBtn.SetTextKey(i18n.KeyChainOverlay)
	z.splitBtn = NewButton("SPL", InstButtonStyle, func() {
		z.displayMode = chainSplit
	})
	z.splitBtn.SetTextKey(i18n.KeyChainSplit)
	z.diffBtn = NewButton("DIF", InstButtonStyle, func() {
		z.displayMode = chainDiff
	})
	z.diffBtn.SetTextKey(i18n.KeyChainDiff)
	// A text pill, not the icon: a pause glyph under the transport reads as transport pause.
	z.freezeBtn = NewButton(i18n.T(i18n.KeyChainFreeze), InstButtonStyle, func() {
		z.SetFrozen(!z.frozen)
	})
	applyFreezeCodeVisual(z.freezeBtn, z.frozen)
}

// -1 clears.
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

// Tapping a set stage clears it; otherwise A, then B, then B is replaced.
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

// Zero before Layout.
func (z *ChainPanelZone) Rect() image.Rectangle { return z.rect }

// ContentRect returns the trace drawing area (rect minus the header strip).
func (z *ChainPanelZone) ContentRect() image.Rectangle { return z.contentRect() }

// Frozen reports whether the scope is currently frozen.
func (z *ChainPanelZone) Frozen() bool { return z.frozen }

// AutoGain reports whether the scope is auto-gain.
func (z *ChainPanelZone) AutoGain() bool { return z.autoGain }

// 0 = overlay, 1 = split, 2 = diff.
func (z *ChainPanelZone) DisplayMode() int { return int(z.displayMode) }

// Chrome pill button accessors (rect exposure for the agent harness / screenshots).
func (z *ChainPanelZone) OverlayBtn() *Button { return z.overlayBtn }
func (z *ChainPanelZone) SplitBtn() *Button   { return z.splitBtn }
func (z *ChainPanelZone) DiffBtn() *Button    { return z.diffBtn }
func (z *ChainPanelZone) FreezeBtn() *Button  { return z.freezeBtn }

// Unknown labels return false.
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

// The UI state is authoritative: the callback lets the audio side mirror it, but cannot veto it.
func (z *ChainPanelZone) SetFrozen(frozen bool) {
	if z.frozen == frozen {
		return
	}
	if z.callbacks.OnFreezeToggle != nil {
		_ = z.callbacks.OnFreezeToggle()
	}
	z.frozen = frozen
	applyFreezeCodeVisual(z.freezeBtn, z.frozen)
}

// Enabling resets manual gain.
func (z *ChainPanelZone) SetAutoGain(on bool) {
	if z.autoGain == on {
		return
	}
	z.autoGain = on
	if z.autoGain {
		z.yGain = 1.0
	}
}

// Unknown labels are ignored.
func (z *ChainPanelZone) SetTraceVisible(label string, visible bool) {
	switch label {
	case "A":
		z.showTapA = visible
	case "B":
		z.showTapB = visible
	}
}

// The WASM equivalent of the scope service's instrument (also used by browser tests).
func (z *ChainPanelZone) SetInstrumentID(id channel.ID) {
	z.instrumentID = id
}

// Clamped to [1, 500] like the wheel; disables auto-fit so the manual window sticks.
func (z *ChainPanelZone) SetWindowMs(ms float64) {
	if ms < 1 {
		ms = 1
	}
	if ms > 500 {
		ms = 500
	}
	z.autoFit = false
	z.windowMs = ms
}

// The last manual windowMs stays the ceiling.
func (z *ChainPanelZone) SetAutoFit(on bool) {
	z.autoFit = on
}

// AutoFit reports whether auto-fit is active (test/scene introspection).
func (z *ChainPanelZone) AutoFit() bool { return z.autoFit }

// Unknown values fall back to overlay.
func (z *ChainPanelZone) SetDisplayMode(mode string) {
	switch mode {
	case "split":
		z.displayMode = chainSplit
	case "diff":
		z.displayMode = chainDiff
	default:
		z.displayMode = chainOverlay
	}
}

// Clamped to [0.25, 16]; disables auto-gain so manual zoom sticks.
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

// Mobile skips hover.
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
		// Open closes a duplicate id first, so re-opening per frame is cheap and self-correcting.
		if !z.portal.Has("chain-tt") {
			z.portal.Open(PortalEntry{
				ID:      "chain-tt",
				Owner:   z,
				Overlay: NewTooltipOverlay(text),
				Modal:   false,
				Passive: true, // display-only hint; never owns input / closes on outside press
				Anchor:  anchor,
			})
		}
	}
}

func (z *ChainPanelZone) HitAreas() []HitArea {
	if z.callbacks.IsHiddenForInput != nil && z.callbacks.IsHiddenForInput() {
		return nil
	}
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
	z.drawTracesCached(screen, cr, state)

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

// segmentedRowH is 0 on desktop.
func (z *ChainPanelZone) contentRect() image.Rectangle {
	left := z.rect.Min.X + Profile().DensityValues().ChainStageColW + Profile().DensityValues().ChainContentGap
	top := z.rect.Min.Y + chainHeaderH + z.segmentedRowH
	if left >= z.rect.Max.X || top >= z.rect.Max.Y {
		return image.Rectangle{}
	}
	return image.Rect(left, top, z.rect.Max.X, z.rect.Max.Y)
}

// SetPortal sets the portal reference for opening overlays.
func (z *ChainPanelZone) SetPortal(p *OverlayPortal) { z.portal = p }

// --- Header drawing ---

func (z *ChainPanelZone) drawHeader(dst *ebiten.Image) {
	// No arrows: the column itself is the chain.
	stages := scope.AllStages()
	for i, btn := range z.stageButtons {
		if btn == nil {
			continue
		}
		isA := i < len(stages) && z.tapA == stages[i]
		isB := i < len(stages) && z.tapB == stages[i]
		active := isA || isB
		// Room for the badge, so the label centres left of it instead of running under it.
		badgeReserve := 0
		if isA {
			bw, _ := chainBadgeSize("A")
			badgeReserve = bw + chainBadgeInset + 3
		} else if isB {
			bw, _ := chainBadgeSize("B")
			badgeReserve = bw + chainBadgeInset + 3
		}
		z.drawStagePill(dst, btn, active, badgeReserve)
		if isA {
			z.drawBadge(dst, btn, "A", colScopeA)
		} else if isB {
			z.drawBadge(dst, btn, "B", colScopeB)
		}
		// Silent stages read -Inf → background only, so live vs silent shows at a glance.
		if i < len(stages) && z.callbacks.StagePeak != nil {
			peakDB, _ := z.callbacks.StagePeak(stages[i])
			z.drawStageMeter(dst, btn.Rect(), peakDB)
		}

		z.drawChainStageCardDecorations(dst, btn, i, isA, isB)
	}

	// Zoom readout: tucked just left of the OVR pill on the top-right strip.
	zoomText := formatWindowMs(z.windowMs)
	if z.yGain != 1.0 {
		zoomText += fmt.Sprintf(" Y:%.3gx", z.yGain)
	}
	zoomW := StyledTextWidth(zoomText, RoleMicro)
	// Mobile moves the mode pills to their own row, so FIT is the leftmost chrome there.
	chromeLeft := z.overlayBtn.Rect().Min.X
	if Profile().ChainModeSegmented {
		chromeLeft = z.freezeBtn.Rect().Min.X
	}
	zoomX := chromeLeft - zoomW - 6
	zoomY := z.rect.Min.Y + 4 + (18-StyledTextHeight(RoleMicro))/2
	DrawTextStyled(dst, zoomText, zoomX, zoomY, RoleMicro, colTextSecondary)

	// That span was empty, so the explanatory layer costs no layout.
	stripLeft := z.rect.Min.X + Profile().DensityValues().ChainStageColW + Profile().DensityValues().ChainContentGap + SpaceXS
	strip := image.Rect(stripLeft, z.rect.Min.Y+4, zoomX-SpaceSM, z.rect.Min.Y+4+18)
	drawChainHeaderStrip(dst, strip, z.tapA, z.tapB, z.displayMode)

	// Separate pills on desktop, a segmented row on mobile; always visible.
	drawPillTabAt(dst, z.overlayBtn, z.displayMode == chainOverlay)
	drawPillTabAt(dst, z.splitBtn, z.displayMode == chainSplit)
	drawPillTabAt(dst, z.diffBtn, z.displayMode == chainDiff)

	drawPillTabAt(dst, z.freezeBtn, z.frozen)
}

// Shared so chrome pills and stage cards look identical.
func (z *ChainPanelZone) drawPillChrome(dst *ebiten.Image, r image.Rectangle, active bool) {
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
}

// pillGlyphColor resolves the glyph color shared by the icon + text paths.
func pillGlyphColor(active, disabled bool) color.Color {
	switch {
	case disabled:
		return WithAlpha(genColorScopeLabelDim, genAlphaScopeLabelDim)
	case active:
		return colTextAccent
	default:
		return colTextSecondary
	}
}

// The label centres left of the badge reservation and elides when it still overflows.
func (z *ChainPanelZone) drawStagePill(dst *ebiten.Image, btn *Button, active bool, badgeReserve int) {
	r := btn.Rect()
	if r.Empty() {
		return
	}
	z.drawPillChrome(dst, r, active)

	glyphCol := pillGlyphColor(active, false)
	labelR := image.Rect(r.Min.X+2, r.Min.Y, r.Max.X-2-badgeReserve, r.Max.Y)
	if labelR.Dx() <= 0 {
		return
	}
	text := chainElideLabel(btn.Text, labelR.Dx())
	tw := StyledTextWidth(text, RoleCaption)
	th := StyledTextHeight(RoleCaption)
	tx := labelR.Min.X + (labelR.Dx()-tw)/2
	if tx < labelR.Min.X {
		tx = labelR.Min.X
	}
	ty := r.Min.Y + (r.Dy()-th)/2
	DrawTextStyled(dst, text, tx, ty, RoleCaption, glyphCol)
}

// The full text when it fits; "…" (or "") when nothing does.
func chainElideLabel(text string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	if StyledTextWidth(text, RoleCaption) <= maxW {
		return text
	}
	const ell = "…"
	if StyledTextWidth(ell, RoleCaption) > maxW {
		return ""
	}
	runes := []rune(text)
	for n := len(runes) - 1; n >= 1; n-- {
		cand := string(runes[:n]) + ell
		if StyledTextWidth(cand, RoleCaption) <= maxW {
			return cand
		}
	}
	return ell
}

// Silent stages get only the dim rail.
func (z *ChainPanelZone) drawStageMeter(dst *ebiten.Image, btnRect image.Rectangle, peakDB float64) {
	if btnRect.Empty() {
		return
	}
	// Wider at Spacious (mobile) so the rail is visible there.
	dv := Profile().DensityValues()
	meterW := dv.ChainMiniMeterW
	meterGap := dv.ChainMiniMeterGap
	railX0 := btnRect.Max.X + meterGap
	railX1 := railX0 + meterW
	railY0 := btnRect.Min.Y
	railY1 := btnRect.Max.Y
	if railY1-railY0 < 4 {
		return
	}
	// Dim background rail so the user can always see the meter slot.
	drawRect(dst, image.Rect(railX0, railY0, railX1, railY1), meterBg, true)
	if peakDB <= meterDBFloor {
		return
	}
	frac := dbToFrac(peakDB)
	if frac <= 0 {
		return
	}
	fillH := int(frac * float64(railY1-railY0))
	if fillH <= 0 {
		return
	}
	col := meterColor(peakDB)
	drawRect(dst, image.Rect(railX0, railY1-fillH, railX1, railY1), col, true)
}

// Keeps the badge off the active pill's 2px selection stroke.
const chainBadgeInset = 2

// Shared by the badge renderer and the label reservation so they agree.
func chainBadgeSize(label string) (w, h int) {
	tw := StyledTextWidth(label, RoleCaption)
	th := StyledTextHeight(RoleCaption)
	const padX, padY = 3, 1
	return tw + 2*padX, th + 2*padY
}

// Sized to the density text so A/B is legible, inset inside the selection stroke.
func (z *ChainPanelZone) drawBadge(dst *ebiten.Image, btn *Button, label string, col color.Color) {
	r := btn.Rect()
	badgeW, badgeH := chainBadgeSize(label)
	const padX, padY = 3, 1
	bx := r.Max.X - badgeW - chainBadgeInset
	by := r.Min.Y + chainBadgeInset
	drawRect(dst, image.Rect(bx, by, bx+badgeW, by+badgeH), col, true)
	DrawTextStyled(dst, label, bx+padX, by+padY, RoleCaption, colTextPrimary)
}

// --- Layout ---

const (
	// Comfortable-density anchors for static sites; runtime reads the density values.
	chainStageColW = 56
	chainStageRowH = 22
)

func (z *ChainPanelZone) layoutButtons() {
	r := z.rect
	dv := Profile().DensityValues()
	btnH := audioPillHeight()

	// --- Vertical stage column on the LEFT — density-driven width ---
	stageW := dv.ChainStageColW
	stageH := dv.ChainStageRowH
	colX := r.Min.X + 4
	colY0 := r.Min.Y + 4
	for i, btn := range z.stageButtons {
		if btn == nil {
			continue
		}
		yTop := colY0 + i*(stageH+2)
		btn.SetRect(image.Rect(colX, yTop, colX+stageW, yTop+stageH))
	}

	bandTop := r.Min.Y + 4
	if pad := (chainHeaderH - btnH) / 2; pad > 0 {
		bandTop = r.Min.Y + pad
	}
	y := bandTop
	gap := Profile().DensityValues().AudioPillGap
	rightEdge := r.Max.X - SpaceSM

	// Freeze (right-most chrome pill).
	freezeW := audioPillWidth(z.freezeBtn)
	z.freezeBtn.SetRect(image.Rect(rightEdge-freezeW, y, rightEdge, y+btnH))
	rightEdge -= freezeW + gap

	z.segmentedRowH = 0
	if Profile().ChainModeSegmented {
		// Mobile: a dedicated segmented row below the chrome strip, never squeezed into the right edge;
		// segmentedRowH drops the trace below it.
		segH := ExpandHitArea(btnH)
		segY := y + btnH + 2
		x0 := colX + stageW + Profile().DensityValues().ChainContentGap
		for _, btn := range []*Button{z.overlayBtn, z.splitBtn, z.diffBtn} {
			pillW := audioPillWidth(btn)
			btn.SetRect(image.Rect(x0, segY, x0+pillW, segY+segH))
			x0 += pillW // contiguous — segmented, no inter-pill gap
		}
		z.segmentedRowH = segH + 4
	} else {
		for _, btn := range []*Button{z.diffBtn, z.splitBtn, z.overlayBtn} {
			pillW := audioPillWidth(btn)
			btn.SetRect(image.Rect(rightEdge-pillW, y, rightEdge, y+btnH))
			rightEdge -= pillW + gap
		}
	}
}

// --- Hit areas ---

func (z *ChainPanelZone) rebuildHitAreas() {
	z.hitAreas = z.hitAreas[:0]

	const zIdx = 140

	// Its own catch-all at its nominal z, so isolation holds even if it is ever mounted outside the EQ panel.
	if !z.rect.Empty() {
		z.hitAreas = append(z.hitAreas, NewInputCaptureHitArea(z.rect, zIdx, "chain-panel-capture"))
	}

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

	// The split pill keeps the "scope-split-btn" tag for callers that expect it.
	if sr := z.overlayBtn.Rect(); !sr.Empty() {
		ha := HitArea{
			Rect:    sr,
			ZIndex:  zIdx + 1,
			Handler: &buttonHitAdapter{btn: z.overlayBtn},
			Tag:     "scope-overlay-btn",
		}
		if Profile().IsMobile() {
			ha.Touch = true
			ha.ClipRect = expandToTouchMin(sr)
		}
		z.hitAreas = append(z.hitAreas, ha)
	}
	if sr := z.splitBtn.Rect(); !sr.Empty() {
		ha := HitArea{
			Rect:    sr,
			ZIndex:  zIdx + 1,
			Handler: &buttonHitAdapter{btn: z.splitBtn},
			Tag:     "scope-split-btn",
		}
		if Profile().IsMobile() {
			ha.Touch = true
			ha.ClipRect = expandToTouchMin(sr)
		}
		z.hitAreas = append(z.hitAreas, ha)
	}
	if sr := z.diffBtn.Rect(); !sr.Empty() {
		ha := HitArea{
			Rect:    sr,
			ZIndex:  zIdx + 1,
			Handler: &buttonHitAdapter{btn: z.diffBtn},
			Tag:     "scope-diff-btn",
		}
		if Profile().IsMobile() {
			ha.Touch = true
			ha.ClipRect = expandToTouchMin(sr)
		}
		z.hitAreas = append(z.hitAreas, ha)
	}

	// Freeze button.
	if fr := z.freezeBtn.Rect(); !fr.Empty() {
		ha := HitArea{
			Rect:    fr,
			ZIndex:  zIdx + 1,
			Handler: &buttonHitAdapter{btn: z.freezeBtn},
			Tag:     "scope-freeze-btn",
		}
		if Profile().IsMobile() {
			ha.Touch = true
			ha.ClipRect = expandToTouchMin(fr)
		}
		z.hitAreas = append(z.hitAreas, ha)
	}

	// The legend readouts below the waveform are the click targets; chainLegendHalves is shared with
	// the draw so clicks land on the rendered text.
	contentR := z.contentRect()
	if !contentR.Empty() {
		_, legendStrip := chainTraceRects(contentR)
		segA, segB := chainLegendHalves(legendStrip, z.displayMode)
		if !segA.Empty() && z.displayMode != chainDiff {
			z.hitAreas = append(z.hitAreas,
				HitArea{Rect: segA, ZIndex: zIdx + 2, Handler: &chainSwatchHandler{zone: z, tap: "A"}, Tag: "scope-swatch-a"})
		}
		if !segB.Empty() {
			z.hitAreas = append(z.hitAreas,
				HitArea{Rect: segB, ZIndex: zIdx + 2, Handler: &chainSwatchHandler{zone: z, tap: "B"}, Tag: "scope-swatch-b"})
		}
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
func (h *chainSwatchHandler) OnDrag(x, y int)                     {}
func (h *chainSwatchHandler) OnRelease(x, y int)                  {}
func (h *chainSwatchHandler) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// --- Scroll wheel zoom handler ---

// Scroll zooms time, Shift+scroll the gain; double-click resets both.
type chainZoomHandler struct {
	zone          *ChainPanelZone
	lastPressTime int64 // monotonic ms for double-click detection
}

func (h *chainZoomHandler) OnPress(x, y int) InputResult {
	now := time.Now().UnixMilli()
	if now-h.lastPressTime < 300 {
		// Double-click: reset both axes and restore auto-fit.
		h.zone.windowMs = 20
		h.zone.yGain = 1.0
		h.zone.autoFit = true
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
		// Disables auto-fit so the manual window sticks.
		h.zone.autoFit = false
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
