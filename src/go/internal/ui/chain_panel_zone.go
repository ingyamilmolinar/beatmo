package ui

import (
	"fmt"
	"image"
	"image/color"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	audio "github.com/ingyamilmolinar/beatmo/internal/audio"
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

	// StagePeak returns the latest peak/RMS dB for the given pipeline
	// stage. Used to paint per-stage mini-meters beside every stage
	// button so the column becomes a live signal-flow display (not just
	// a stage chooser). Stages with no signal should return -Inf so the
	// meter renders dark. Nil disables the per-stage display entirely.
	StagePeak func(stage scope.Stage) (peakDB, rmsDB float64)

	// IsHiddenForInput mirrors the EQPanelZone gate (see EQCallbacks
	// docstring) — when set and returning true, `HitAreas()` returns
	// nil. Belt-and-suspenders against the Pads-tab input leak, in
	// case a future refactor mounts the chain zone as its own tree
	// node (it's currently delegated through the EQ panel).
	IsHiddenForInput func() bool
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
	autoGainBtn  *Button    // auto-gain toggle (Y) — chrome row
	fitBtn       *Button    // auto-fit toggle (X) — chrome row
	freezeBtn    *Button    // freeze toggle
	closeBtn     *Button    // close panel

	// segmentedRowH is the vertical space claimed by the mobile segmented
	// OVR|SPL|DIF row (0 on desktop, where the modes are pills in the chrome
	// row). contentRect() drops the trace below it so the segmented control
	// never overlaps the waveform.
	segmentedRowH int

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
	yGain       float64          // Y-axis gain multiplier (default 1.0, range [0.25, 16.0])
	autoGain    bool             // true = auto-scale Y to peak amplitude
	autoFit     bool             // true = auto-fit the X window to the signal's active span
	showTapA    bool             // true = draw tap A trace (default true)
	showTapB    bool             // true = draw tap B trace (default true)

	// fitState is reusable scratch for the auto-fit sub-sliced snapshot so
	// Draw never heap-allocates a new State per frame (alloc budget gate).
	fitState scope.State

	// WASM-only zone-local state. On desktop these stay zero-valued because
	// the audio.ScopeService() owns freeze + instrument selection.
	frozenState  *scope.State // cached snapshot held while frozen
	instrumentID string       // selected channel id, fed to snapshot calls

	// Hit areas cache (rebuilt on Layout)
	hitAreas []HitArea
}

// NewChainPanelZone creates a new ChainPanelZone with the provided callbacks.
// Tap A defaults to StageSynth so opening the Chain tab shows the synth signal
// immediately instead of a blank panel; the OnTapAChange callback fires once
// here so the desktop scope service mirrors the UI default. (On WASM the
// callback is a no-op and the zone-local tapA field is authoritative.)
func NewChainPanelZone(cb ChainCallbacks) *ChainPanelZone {
	z := &ChainPanelZone{
		needLayout: true,
		callbacks:  cb,
		tapA:       scope.StageSynth,
		tapB:       -1,
		windowMs:   20,
		yGain:      1.0,
		autoFit:    true,
		showTapA:   true,
		showTapB:   true,
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
	}

	z.autoGainBtn = NewButton("AG", InstButtonStyle, func() {
		z.SetAutoGain(!z.autoGain)
	})
	// FIT toggles X auto-fit (frame the signal's active span). Active by
	// default; manual zoom turns it off, this turns it back on.
	z.fitBtn = NewButton("FIT", InstButtonStyle, func() {
		z.SetAutoFit(!z.autoFit)
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

// DisplayMode reports the current trace display mode (0=overlay, 1=split, 2=diff).
// Exposed for agent read-back checkpoints.
func (z *ChainPanelZone) DisplayMode() int { return int(z.displayMode) }

// Chrome pill button accessors (rect exposure for the agent harness / screenshots).
func (z *ChainPanelZone) OverlayBtn() *Button { return z.overlayBtn }
func (z *ChainPanelZone) SplitBtn() *Button   { return z.splitBtn }
func (z *ChainPanelZone) DiffBtn() *Button    { return z.diffBtn }
func (z *ChainPanelZone) AGBtn() *Button      { return z.autoGainBtn }
func (z *ChainPanelZone) FreezeBtn() *Button  { return z.freezeBtn }

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

// SetInstrumentID assigns the zone-local instrument id used by the WASM
// ScopeState fallback (BuildScopeStateFromSnapshots). Desktop reads the
// id from audio.ScopeService instead; this setter is the WASM equivalent
// and is also used by setEQChannel(id) browser tests to make
// probeScopeState() reflect the new channel without going through a
// click on the EQ panel's channel-cycle button.
func (z *ChainPanelZone) SetInstrumentID(id string) {
	z.instrumentID = id
}

// SetWindowMs sets the X-axis time window directly, clamped to the same
// [1, 500] range the wheel handler enforces. Like the plain-scroll path,
// calling this disables auto-fit so the manual window sticks until the user
// re-enables FIT.
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

// SetAutoFit drives the same flow the FIT-button click takes: when enabled,
// the X window auto-frames the signal's active span every frame; the last
// manual windowMs is preserved as the ceiling. Mirrors SetAutoGain.
func (z *ChainPanelZone) SetAutoFit(on bool) {
	z.autoFit = on
}

// AutoFit reports whether auto-fit is active (test/scene introspection).
func (z *ChainPanelZone) AutoFit() bool { return z.autoFit }

// SetDisplayMode selects the A/B display mode ("overlay" | "split" | "diff").
// Mirrors clicking the OVR/SPL/DIF pill; used by scene Setup and tests to
// drive the same visual state without synthesizing a click. Unknown values
// fall back to overlay.
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
	// Self-defense input gate — see EQPanelZone.HitAreas for the
	// architectural rationale.
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

	// Auto-fit: frame the X window to the signal's active span so the
	// waveform fills the trace. The scope captures ~500ms but a transient is
	// ~2ms; without this the trace is ~98% dead width and the time-axis
	// labels (driven by windowMs) disagree with what's rendered. We sub-slice
	// each tap's samples (alloc-free, into reusable scratch) and derive an
	// effective window so labels match. Manual zoom (z.windowMs) is the
	// ceiling. See [[project_chain_tab_redesign]] / scope.ActiveSpan.
	drawState := state
	drawWindowMs := z.windowMs
	if z.autoFit && state != nil {
		if start, end := chainFitSpan(state, z.showTapA, z.showTapB); end > start {
			z.fitState = *state
			z.fitState.TapA.Samples = chainSliceSpan(state.TapA.Samples, start, end)
			z.fitState.TapB.Samples = chainSliceSpan(state.TapB.Samples, start, end)
			drawState = &z.fitState
			if sr := audio.SampleRate(); sr > 0 {
				spanMs := float64(end-start) * 1000.0 / float64(sr)
				if minMs := float64(Profile().DensityValues().ChainAutoFitMinMs); spanMs < minMs {
					spanMs = minMs
				}
				if spanMs > z.windowMs {
					spanMs = z.windowMs // manual zoom ceiling
				}
				drawWindowMs = spanMs
			}
		}
	}
	drawChainTraces(screen, cr, drawState, drawWindowMs, z.displayMode, z.frozen, effectiveGain, z.showTapA, z.showTapB)

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
// stage column (density-driven width) and below the chrome strip plus the
// mobile segmented mode row (segmentedRowH, 0 on desktop).
func (z *ChainPanelZone) contentRect() image.Rectangle {
	left := z.rect.Min.X + Profile().DensityValues().ChainStageColW + 8
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
	// Density-aware: the zoom readout grows on mobile (Spacious) instead of
	// the old hardcoded FontSizeCaption/FontSizeBody.
	captionScale := chainLabelScale()

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
		// Reserve horizontal room for the A/B badge so the centered stage
		// label is squeezed into the area LEFT of the badge instead of being
		// centered under it (bug #1: "AntiPop"/"Master" ran under the badge).
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
		// Per-stage mini-meter: lit fill height = dB-fraction of the
		// stage's latest peak. Silent stages return -Inf → frac=0 →
		// background only, so the column reads as "this stage is live
		// vs. this stage is silent" at a glance. Phase 3.
		if i < len(stages) && z.callbacks.StagePeak != nil {
			peakDB, _ := z.callbacks.StagePeak(stages[i])
			z.drawStageMeter(dst, btn.Rect(), peakDB)
		}

		// Phase 3 audio-panel redesign: per-stage mini-waveform + FX
		// badge count. drawChainStageCardDecorations centralises both
		// renders so the per-stage loop stays focused on the meter.
		z.drawChainStageCardDecorations(dst, btn, i, isA, isB)
	}

	// Zoom readout: tucked just left of the OVR pill on the top-right strip.
	zoomText := formatWindowMs(z.windowMs)
	if z.yGain != 1.0 {
		zoomText += fmt.Sprintf(" Y:%.3gx", z.yGain)
	}
	zoomW := int(float64(TextWidth(zoomText)) * captionScale)
	// Anchor the zoom readout left of the leftmost chrome-row control. On
	// desktop that's the OVR pill; on mobile the mode pills moved to their own
	// row, so anchor to FIT (the leftmost remaining chrome control).
	chromeLeft := z.overlayBtn.Rect().Min.X
	if Profile().ChainModeSegmented {
		chromeLeft = z.fitBtn.Rect().Min.X
	}
	zoomX := chromeLeft - zoomW - 6
	zoomY := z.rect.Min.Y + 4 + (18-int(float64(TextHeight())*captionScale))/2
	DrawTextColorAtScale(dst, zoomText, zoomX, zoomY, colTextSecondary, captionScale)

	// Mode selector — three pills. Desktop: separated, in the chrome row.
	// Mobile: a contiguous segmented control on its own row (rects set in
	// layoutButtons); always-visible either way.
	z.drawPillButton(dst, z.overlayBtn, z.displayMode == chainOverlay, false)
	z.drawPillButton(dst, z.splitBtn, z.displayMode == chainSplit, false)
	z.drawPillButton(dst, z.diffBtn, z.displayMode == chainDiff, false)

	z.drawPillButton(dst, z.fitBtn, z.autoFit, false)
	z.drawPillButton(dst, z.autoGainBtn, z.autoGain, false)
	z.drawPillButton(dst, z.freezeBtn, z.frozen, false)
	z.drawPillButton(dst, z.closeBtn, false, false)
}

// drawPillChrome paints just the pill background + border (active = filled
// surface with azure stroke; inactive = outline only). Shared by every pill
// renderer so the chrome stays identical across chrome pills and stage cards.
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

// drawPillButton draws a button with pill styling (rounded-ish filled rect).
// When disabled is true the button is drawn dimmed and non-interactive.
func (z *ChainPanelZone) drawPillButton(dst *ebiten.Image, btn *Button, active, disabled bool) {
	r := btn.Rect()
	if r.Empty() {
		return
	}
	z.drawPillChrome(dst, r, active)

	glyphCol := pillGlyphColor(active, disabled)

	// Icon-only pills (e.g. the close button) carry an Icon + empty Text;
	// drawPillButton paints its own chrome so it must render the icon here —
	// it never calls btn.Draw(). Without this the close pill drew an empty
	// outlined box (bug: missing glyph). The icon is inset inside the pill so
	// it doesn't touch the border.
	if btn.Icon != "" {
		ic := glyphCol
		if btn.IconColor != nil {
			ic = btn.IconColor
		}
		iconR := r.Inset(4)
		if iconR.Dx() > 0 && iconR.Dy() > 0 {
			DrawIcon(dst, IconID(btn.Icon), iconR, ic)
		}
		return
	}

	// Center text. Density-aware pill text grows on mobile (Spacious).
	captionScale := chainPillScale()
	tw := int(float64(TextWidth(btn.Text)) * captionScale)
	th := int(float64(TextHeight()) * captionScale)
	tx := r.Min.X + (r.Dx()-tw)/2
	ty := r.Min.Y + (r.Dy()-th)/2
	DrawTextColorAtScale(dst, btn.Text, tx, ty, glyphCol, captionScale)
}

// drawStagePill draws a stage button's chrome plus its label, reserving
// badgeReserve px on the right for the A/B badge so the label centers in the
// area left of the badge (bug #1) and elides with "…" when it still overflows.
func (z *ChainPanelZone) drawStagePill(dst *ebiten.Image, btn *Button, active bool, badgeReserve int) {
	r := btn.Rect()
	if r.Empty() {
		return
	}
	z.drawPillChrome(dst, r, active)

	captionScale := chainPillScale()
	glyphCol := pillGlyphColor(active, false)
	// Inner label rect: inset 2px each side, then drop badgeReserve from the
	// right edge so the centered label never runs under the badge.
	labelR := image.Rect(r.Min.X+2, r.Min.Y, r.Max.X-2-badgeReserve, r.Max.Y)
	if labelR.Dx() <= 0 {
		return
	}
	text := chainElideLabel(btn.Text, labelR.Dx(), captionScale)
	tw := int(float64(TextWidth(text)) * captionScale)
	th := int(float64(TextHeight()) * captionScale)
	tx := labelR.Min.X + (labelR.Dx()-tw)/2
	if tx < labelR.Min.X {
		tx = labelR.Min.X
	}
	ty := r.Min.Y + (r.Dy()-th)/2
	DrawTextColorAtScale(dst, text, tx, ty, glyphCol, captionScale)
}

// chainElideLabel truncates `text` with a trailing "…" so its scaled width
// fits within maxW px. Returns the full text untouched when it already fits;
// returns "…" (or "") when nothing fits.
func chainElideLabel(text string, maxW int, scale float64) string {
	if maxW <= 0 {
		return ""
	}
	if int(float64(TextWidth(text))*scale) <= maxW {
		return text
	}
	const ell = "…"
	ellW := int(float64(TextWidth(ell)) * scale)
	if ellW > maxW {
		return ""
	}
	runes := []rune(text)
	for n := len(runes) - 1; n >= 1; n-- {
		cand := string(runes[:n]) + ell
		if int(float64(TextWidth(cand))*scale) <= maxW {
			return cand
		}
	}
	return ell
}

// drawStageMeter paints a 3 px-wide vertical fill flush to the right
// edge of the stage button rect, height proportional to peakDB (clamped
// to meter floor / ceil). Silent stages get only the dim background
// rail; active stages fill in meterColor(peakDB). This turns the stage
// column into a live signal-flow display rather than a chooser-only
// strip. Phase 3.
func (z *ChainPanelZone) drawStageMeter(dst *ebiten.Image, btnRect image.Rectangle, peakDB float64) {
	if btnRect.Empty() {
		return
	}
	// Phase 3 audio-panel redesign: mini-meter width follows density
	// so the 4-px Compact rail expands to 10 px at Spacious (mobile)
	// where the user can actually see "signal flowing here vs not".
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

// chainBadgeInset is the gap (px) the A/B badge is pulled in from the stage
// button's edges so it never sits on top of the 2px azure selection stroke
// drawn around an active pill (bug #2: badge overlapped the selection border).
const chainBadgeInset = 2

// chainBadgeSize returns the rendered width/height of an A/B badge for the
// given label at the current density. Shared by drawBadge (the renderer) and
// the stage-label centering reservation (bug #1) so the two stay in lockstep.
func chainBadgeSize(label string) (w, h int) {
	badgeScale := chainBadgeScale()
	tw := int(float64(TextWidth(label)) * badgeScale)
	th := int(float64(TextHeight()) * badgeScale)
	const padX, padY = 3, 1
	return tw + 2*padX, th + 2*padY
}

// drawBadge draws a colored A/B pill at the top-right of a stage button.
// The pill is sized to the density-aware badge text (was a fixed 8×8 box
// holding ~7px text — unreadable) so the A/B assignment is legible at a
// glance, especially on mobile (Spacious). The badge is inset by
// chainBadgeInset from the button's top/right edges so it sits inside the
// 2px azure selection stroke of an active pill (bug #2) instead of on it.
func (z *ChainPanelZone) drawBadge(dst *ebiten.Image, btn *Button, label string, col color.Color) {
	r := btn.Rect()
	badgeScale := chainBadgeScale()
	badgeW, badgeH := chainBadgeSize(label)
	const padX, padY = 3, 1
	bx := r.Max.X - badgeW - chainBadgeInset
	by := r.Min.Y + chainBadgeInset
	drawRect(dst, image.Rect(bx, by, bx+badgeW, by+badgeH), col, true)
	DrawTextColorAtScale(dst, label, bx+padX, by+padY, colTextPrimary, badgeScale)
}

// --- Layout ---

const (
	// chainStageColW + chainStageRowH are Comfortable-density anchors
	// preserved as constants for static call sites that don't have a
	// Profile() handy at file scope. The runtime path
	// (chainStageDims()) reads density values directly so Compact /
	// Spacious shrink/expand the stage column appropriately.
	chainStageColW = 56
	chainStageRowH = 22
)

func (z *ChainPanelZone) layoutButtons() {
	r := z.rect
	dv := Profile().DensityValues()
	btnH := 18

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

	// --- Right-aligned chrome row at TOP, just above the trace area ---
	y := r.Min.Y + 4
	rightEdge := r.Max.X - 6

	// Close (icon-only) furthest right.
	closeW := dv.CloseButtonSize
	if closeW < 20 {
		closeW = 20
	}
	z.closeBtn.SetRect(image.Rect(rightEdge-closeW, y, rightEdge, y+btnH))
	rightEdge -= closeW + 3

	// Freeze.
	freezeW := closeW
	z.freezeBtn.SetRect(image.Rect(rightEdge-freezeW, y, rightEdge, y+btnH))
	rightEdge -= freezeW + 3

	// Auto-gain (Y) + auto-fit (X) toggles live IN the chrome row. They used
	// to sit in the trace's lower-right corner where AG collided with the
	// time-axis labels; the row keeps them out of the waveform entirely.
	agW := dv.ChainAggregateW
	z.autoGainBtn.SetRect(image.Rect(rightEdge-agW, y, rightEdge, y+btnH))
	rightEdge -= agW + 3
	fitW := agW
	z.fitBtn.SetRect(image.Rect(rightEdge-fitW, y, rightEdge, y+btnH))
	rightEdge -= fitW + 3

	z.segmentedRowH = 0
	if Profile().ChainModeSegmented {
		// Mobile / narrow layout: OVR|SPL|DIF as one always-visible
		// contiguous segmented control on a DEDICATED row below the chrome
		// strip — never in the cramped right edge, never overlapping the
		// trace/legend. Each segment is touch-min tall. The row claims
		// vertical space via segmentedRowH so contentRect() drops the trace
		// below it.
		pillW := dv.ChainModePillW
		segH := ExpandHitArea(btnH)
		segY := y + btnH + 2
		x0 := colX + stageW + 8
		for _, btn := range []*Button{z.overlayBtn, z.splitBtn, z.diffBtn} {
			btn.SetRect(image.Rect(x0, segY, x0+pillW, segY+segH))
			x0 += pillW // contiguous — segmented, no inter-pill gap
		}
		z.segmentedRowH = segH + 4
	} else {
		// Desktop: three separate mode pills in the chrome row, right-aligned.
		// Density-driven width (Compact 28 / Comfortable 36 / Spacious 48).
		pillW := dv.ChainModePillW
		for _, btn := range []*Button{z.diffBtn, z.splitBtn, z.overlayBtn} {
			btn.SetRect(image.Rect(rightEdge-pillW, y, rightEdge, y+btnH))
			rightEdge -= pillW + 3
		}
	}
}

// --- Hit areas ---

func (z *ChainPanelZone) rebuildHitAreas() {
	z.hitAreas = z.hitAreas[:0]

	const zIdx = 140

	// Chain panel input isolation. Mirrors the EQPanelZone catch-all
	// pattern (see input_capture.go file-level docstring). Today the
	// chain panel is mounted as a delegated zone inside EQPanelZone so
	// the EQ-level catch-all already covers its rect, but registering
	// our own catch-all at the chain zone's nominal z keeps the
	// contract local — a future refactor that promotes chain to a
	// standalone tree node still has input isolation, with no follow-
	// up change required.
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

	// Auto-gain button.
	if ar := z.autoGainBtn.Rect(); !ar.Empty() {
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:    ar,
			ZIndex:  zIdx + 1,
			Handler: &buttonHitAdapter{btn: z.autoGainBtn},
			Tag:     "scope-ag-btn",
		})
	}

	// Auto-fit (FIT) button.
	if fr := z.fitBtn.Rect(); !fr.Empty() {
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:    fr,
			ZIndex:  zIdx + 1,
			Handler: &buttonHitAdapter{btn: z.fitBtn},
			Tag:     "scope-fit-btn",
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

	// Trace visibility toggles: the A/B legend segments in the reserved strip
	// below the waveform are the click targets (tap the A readout to hide A).
	// chainLegendHalves is the single source of truth shared with
	// drawChainLegend so clicks land exactly on the rendered readout.
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
		// Plain scroll: time-axis zoom. Disables auto-fit so the manual
		// window sticks (double-click restores it).
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
