package ui

import (
	"image/color"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Every alias is DERIVED in rebuildThemeAliases from the live tokens, at init and after ApplyTheme: a var initialiser
// runs once and goes stale. Hex values are edited in DESIGN.md, never here.
var (
	colBGTop                    color.RGBA
	colBGBottom                 color.RGBA
	colGridHorizon              color.RGBA
	colSurface1                 color.RGBA
	colSurface2                 color.RGBA
	colSurface3                 color.RGBA
	colGridLine                 color.RGBA
	colGridHalf                 color.RGBA
	colGridQuarter              color.RGBA
	colGridEighth               color.RGBA
	colGridSixteenth            color.RGBA
	colGridThirtySecond         color.RGBA
	colBorderSubtle             color.NRGBA
	colBorderMedium             color.NRGBA
	colBorderStrong             color.NRGBA
	colButtonBorder             color.NRGBA
	colSubtleBorder             color.NRGBA
	colAccent                   color.RGBA
	colAccentBright             color.RGBA
	colAccentDim                color.RGBA
	colAccentSubtle             color.NRGBA
	colPlayGreen                color.RGBA
	colStopRed                  color.RGBA
	colMuteRed                  color.RGBA
	colTextPrimary              color.RGBA
	colTextSecondary            color.RGBA
	colTextDisabled             color.RGBA
	colTextAccent               color.RGBA
	colPlayButton               color.RGBA
	colStopButton               color.RGBA
	colBPMBox                   color.RGBA
	colDropdown                 color.RGBA
	colDropdownEdge             color.NRGBA
	colError                    color.RGBA
	colSecondaryBtn             color.RGBA
	colRowBtnInactive           color.RGBA
	colRowBtnInactiveText       color.RGBA
	colRowLabel                 color.RGBA
	colSplitterHandle           color.NRGBA
	colSplitterHandleHover      color.NRGBA
	colRackSurface              color.RGBA
	colPanelBG                  color.NRGBA
	colPanelBorder              color.NRGBA
	colScrim                    color.NRGBA
	colDeleteFill               color.RGBA
	colDeleteBorder             color.RGBA
	colDeleteText               color.RGBA
	colMenuGroupBG              color.NRGBA
	colMenuGroupDeleteBG        color.NRGBA
	colMenuIcon                 color.RGBA
	colStep                     color.RGBA
	colStepOff                  color.RGBA
	colStepBorder               color.NRGBA
	colHighlight                color.RGBA
	colMuteCell                 color.RGBA
	colMuteHighlight            color.NRGBA
	colBeatGroupAlt             color.NRGBA
	colTimelineTotal            color.RGBA
	colTimelineView             color.NRGBA
	colTimelineViewHi           color.RGBA
	colTimelineCursor           color.RGBA
	colTimelineBeat             color.RGBA
	colEQBg                     color.NRGBA
	colEQBar                    color.RGBA
	colEQBarPeak                color.RGBA
	colWaveTrace                color.RGBA
	colWaveTraceDry             color.NRGBA
	colWaveMid                  color.NRGBA
	colEQCurve                  color.NRGBA
	colEQCurveFill              color.NRGBA
	colEQHandle                 color.RGBA
	colEQHandleActive           color.RGBA
	colEQHandleBorder           color.RGBA
	colEQZeroLine               color.NRGBA
	colEQFilterHandle           color.RGBA
	colEQFilterHandleActive     color.RGBA
	colEQFilterLine             color.NRGBA
	colVizBass                  color.RGBA
	colVizMids                  color.RGBA
	colVizTreble                color.RGBA
	NodeUI                      NodeStyle
	SignalUI                    SignalStyle
	EdgeUI                      EdgeStyle
	colTransportSurface         color.RGBA
	colTransportBorder          color.NRGBA
	colTransportContainerBorder color.NRGBA
	colTransportDivider         color.NRGBA
	colTransportGroupBG         color.NRGBA
	colPlayIconTint             color.RGBA
	colStopIconTint             color.RGBA
	colIncDecIcon               color.RGBA
	colVolumeIconOn             color.RGBA
	colVolumeIconOff            color.RGBA
	colFollowActive             color.RGBA
	colRecordIdle               color.RGBA
	colRecordActive             color.RGBA
	colMuteActive               color.RGBA
	colMuteActiveBdr            color.RGBA
	colSoloActive               color.RGBA
	colSoloActiveBdr            color.RGBA
	PlayButtonStyle             ButtonStyle
	StopButtonStyle             ButtonStyle
	BPMBoxStyle                 TextInputStyle
	BPMDecStyle                 ButtonStyle
	BPMIncStyle                 ButtonStyle
	LenDecStyle                 ButtonStyle
	LenIncStyle                 ButtonStyle
	InstButtonStyle             ButtonStyle
	UploadBtnStyle              ButtonStyle
	PopupButtonStyle            ButtonStyle
	DropdownStyle               ButtonStyle
	DisabledButtonStyle         ButtonStyle
	DeleteButtonStyle           ButtonStyle
	DeleteConfirmButtonStyle    ButtonStyle
	MissingInstStyle            ButtonStyle
	MuteActiveStyle             ButtonStyle
	SoloActiveStyle             ButtonStyle
	FXActiveStyle               ButtonStyle
	ContextMenuItemStyle        ButtonStyle
	EQMuteButtonStyle           ButtonStyle
	EQMuteButtonActiveStyle     ButtonStyle
	EQFilterButtonActiveStyle   ButtonStyle
	EQDBBoxStyle                TextInputStyle
	colEQDBLabel                color.RGBA
	colEQDBLabelMuted           color.RGBA
	TransportPlayStyle          ButtonStyle
	TransportStopStyle          ButtonStyle
	TransportIncStyle           ButtonStyle
	TransportDecStyle           ButtonStyle
	TransportMiscStyle          ButtonStyle
	TimelineKeycapStyle         ButtonStyle
	TransportFollowOnStyle      ButtonStyle
	FABStyle                    ButtonStyle
	MobileRowLabelStyle         ButtonStyle
	DrumCellUI                  DrumCellStyle
)

// Runtime STATE, never touched by rebuildThemeAliases: a theme change must not wipe custom instrument colours.
var (
	instColors      = buildInstColors()
	customPalette   = buildCustomPalette()
	customColors    = map[instrument.ID]color.Color{}
	nextCustomColor int
)

func init() { rebuildThemeAliases() }

// rebuildComponentSpecs() must have run first: the *StyleFromSpec calls copy from the spec table.
func rebuildThemeAliases() {
	// Level 0 (base): deepest background — grid pane, timeline
	colBGTop = genColorBackground
	colBGBottom = genColorBackground
	colGridHorizon = genColorGridHorizon

	// Level 1 (surface): main surfaces — row rack, transport bg
	colSurface1 = genColorSurface1

	// Level 2 (elevated): cards, buttons, input fields
	colSurface2 = genColorSurface2

	// Level 3 (raised): hover states, active surfaces
	colSurface3 = genColorSurface3

	colGridLine = genColorGridLine
	colGridHalf = genColorGridHalf
	colGridQuarter = genColorGridQuarter
	colGridEighth = genColorGridEighth
	colGridSixteenth = genColorGridSixteenth
	colGridThirtySecond = genColorGridThirtySecond

	// ── Borders (structured hierarchy) — sourced from genColorBorder + named alpha buckets ──
	colBorderSubtle = WithAlpha(genColorBorder, genAlphaBorderThin)     // section dividers
	colBorderMedium = WithAlpha(genColorBorder, genAlphaBorderDefault)  // control borders
	colBorderStrong = WithAlpha(genColorBorder, genAlphaBorderEmphasis) // focused input borders

	colButtonBorder = colBorderMedium // alias for control borders
	colSubtleBorder = colBorderSubtle // alias for subtle borders

	// ── Accent (Primary — Cyan) ─────────────────────────────────────────
	colAccent = genColorPrimary
	colAccentBright = genColorPrimaryBright
	colAccentDim = genColorPrimaryDim
	// The panel-border bucket (20) keeps every "20-alpha overlay" in one place.
	colAccentSubtle = WithAlpha(genColorPrimary, genAlphaBorderPanel)

	// ── Accent (Secondary — Semantic) ───────────────────────────────────
	colPlayGreen = genColorSuccess
	colStopRed = genColorError
	colMuteRed = genColorMute

	// ── Text colors ─────────────────────────────────────────────────────
	colTextPrimary = genColorOnSurface
	colTextSecondary = genColorOnSurfaceMuted
	colTextDisabled = genColorOnSurfaceDisabled
	colTextAccent = genColorOnSurfaceAccent

	// ── Legacy aliases (mapped to new surface system) ───────────────────
	colPlayButton = genColorPlayDesktopFill                          // lime play fill (DESIGN.md play-desktop-fill)
	colStopButton = genColorStopDesktopFill                          // wine stop fill (DESIGN.md stop-desktop-fill)
	colBPMBox = colSurface2                                          // elevated surface for inputs
	colDropdown = colSurface2                                        // elevated surface for dropdowns
	colDropdownEdge = WithAlpha(genColorPrimary, genAlphaAccentTint) // magenta accent border @ alpha.accent-tint (50)
	colError = genColorError                                         // DESIGN.md error

	// Secondary button fill for file-ops, overflow, etc.
	colSecondaryBtn = colSurface2

	// Row control button inactive state
	colRowBtnInactive = colSurface1
	colRowBtnInactiveText = colTextSecondary

	// Row label brighter color
	colRowLabel = colTextPrimary

	// One handle colour on every platform; the hover reuses the white `border` token at its own bucket.
	colSplitterHandle = WithAlpha(genColorSplitterHandle, genAlphaStrong)
	colSplitterHandleHover = WithAlpha(genColorBorder, genAlphaSplitterHover)

	// Rack surface: Level 1 for visual separation
	colRackSurface = genColorRackBed // Background-category bed (DESIGN.md rack-bed), NOT the surface-1 control surface

	colPanelBG = WithAlpha(genColorSurfaceOverlay, genAlphaPanelNearOpaque) // surface-overlay @ alpha.panel-near-opaque (250)
	colPanelBorder = WithAlpha(genColorBorder, genAlphaBorderPanel)         // panel chrome border @ alpha.border-panel (20)
	colScrim = WithAlpha(genColorDimBlack, genAlphaScrim)                   // backdrop scrim @ alpha.scrim (150)

	// Delete/destructive action colors
	colDeleteFill = genColorDestructive
	colDeleteBorder = genColorDestructiveBorder
	colDeleteText = colStopRed // red text for delete items (less aggressive than red bg)

	colMenuGroupBG = WithAlpha(genColorBorder, genAlphaBorderThin)                  // group container bg @ alpha.border-thin (8)
	colMenuGroupDeleteBG = WithAlpha(genColorMenuDeleteTint, genAlphaBorderDefault) // delete-row warning tint @ alpha.border-default (15)
	colMenuIcon = colTextDisabled                                                   // icon tint

	colStep = colAccent                                                            // primary-magenta active cells
	colStepOff = genColorDrumCellOff                                               // cell-off resting fill (DESIGN.md drum-cell-off)
	colStepBorder = WithAlpha(genColorBorder, AlphaRowRackZebra)                   // very subtle cell borders (border@10)
	colHighlight = genColorDrumCellHighlight                                       // cool-white per-beat flash (DESIGN.md drum-cell-highlight)
	colMuteCell = genColorDrumMuteCell                                             // muted-row resting fill (DESIGN.md drum-mute-cell)
	colMuteHighlight = WithAlpha(genColorDrumMuteHighlight, genAlphaMuteHighlight) // muted-row beat flash @ alpha.mute-highlight (160)

	// Beat grouping: alternate column tint for visual beat separation
	colBeatGroupAlt = WithAlpha(genColorBorder, genAlphaBeatGroupAlt)

	// Amber on purpose: a data canvas, exempt from the single chrome accent.
	colTimelineTotal = genColorTimelineTotalBg
	colTimelineView = WithAlpha(genColorTimelineView, genAlphaSubtle) // view region @ alpha.subtle (60)
	colTimelineViewHi = genColorTimelineViewHi
	colTimelineCursor = genColorTimelineCursor
	colTimelineBeat = genColorTimelineBeat
	colEQBg = WithAlpha(genColorVizBg, genAlphaOverlay) // viz-bg @ alpha.overlay (220)
	colEQBar = genColorVizBar
	colEQBarPeak = genColorVizBarPeak
	colWaveTrace = genColorVizWaveA
	colWaveTraceDry = WithAlpha(genColorWaveTraceDry, genAlphaWaveTraceDry) // wave dry trace @ alpha.wave-trace-dry (100)
	colWaveMid = WithAlpha(genColorVizWaveB, genAlphaSidebarChip)           // wave-b @ alpha.sidebar-chip (200)
	colEQCurve = WithAlpha(genColorVizCurve, 230)                           // curve stroke at intentional 230
	colEQCurveFill = WithAlpha(genColorTimelineView, genAlphaEqCurveFill)   // amber fill @ alpha.eq-curve-fill (20); shares hex with timeline-view by design
	colEQHandle = colAccent
	colEQHandleActive = colAccentBright
	colEQHandleBorder = colAccentDim
	colEQZeroLine = WithAlpha(genColorEqZeroLine, genAlphaEqZeroLine) // 0 dB ruler @ alpha.eq-zero-line (80)
	colEQFilterHandle = genColorVizBar                                // filter handle reuses viz-bar (cyan, opaque)
	colEQFilterHandleActive = genColorVizBarPeak                      // active state reuses viz-bar-peak
	colEQFilterLine = WithAlpha(genColorVizBar, genAlphaSubtle)       // filter line @ alpha.subtle (60)

	// Warm bass, neutral mids, cool treble.
	colVizBass = genColorVizBass
	colVizMids = genColorVizMids
	colVizTreble = genColorVizTreble

	NodeUI = NodeStyle{Radius: float32(genGeomNodeRadius), Fill: genColorNodeFill, Border: genColorNodeBorder}
	SignalUI = SignalStyle{Radius: float32(genGeomSignalRadius), Color: colAccent}
	EdgeUI = EdgeStyle{Color: WithAlpha(genColorEdgeColor, genAlphaEdgeDefault)} // edge @ alpha.edge-default (100)

	// Transport surface: Level 1 for visual hierarchy
	colTransportSurface = genColorToolbarBed // Background-category bed (DESIGN.md toolbar-bed)
	// Transport border: subtle section divider
	colTransportBorder = colBorderSubtle
	// Transport container border (visible grouping)
	colTransportContainerBorder = colBorderMedium
	// Transport group divider — white@20 = alpha.border-panel
	colTransportDivider = WithAlpha(genColorBorder, genAlphaBorderPanel)

	// Pill container colors for transport button groups
	colTransportGroupBG = WithAlpha(genColorBorder, genAlphaRowRackZebra) // pill container fill — white@10 = alpha.row-rack-zebra

	// Icon tint colors for mobile transport buttons
	colPlayIconTint = colPlayGreen         // green for play icon
	colStopIconTint = colStopRed           // red for stop icon
	colIncDecIcon = colTextPrimary         // primary text for +/- icons (mobile)
	colVolumeIconOn = colTextSecondary     // muted grey when volume > 0 (nil-tint / master speaker; per-row icons pass their instrument color)
	colVolumeIconOff = colTextDisabled     // disabled when muted/zero
	colFollowActive = colTextPrimary       // neutral bright tint for follow-on (track lit); free state dims to colTextDisabled
	colRecordIdle = genColorRecordIdle     // red circle for record button (DESIGN.md record-idle, opaque)
	colRecordActive = genColorRecordActive // bright red when recording (DESIGN.md record-active, opaque)

	// Mute/Solo active state colors
	colMuteActive = colMuteRed
	colMuteActiveBdr = genColorMuteActiveBorder // mute-active-border (DESIGN.md mute-active-border #D87848)
	colSoloActive = genColorPrimaryBright       // primary-bright coral #FFB498
	colSoloActiveBdr = genColorSoloActiveBorder // solo-active-border #FFB498

	// Spec-backed; TestComponentSpecsDrift asserts byte-equivalence with DESIGN.md.
	PlayButtonStyle = ButtonStyleFromSpec(ComponentButtonPlayDesktop)
	StopButtonStyle = ButtonStyleFromSpec(ComponentButtonStopDesktop)
	BPMBoxStyle = TextInputStyleFromSpec(ComponentInputField, color.White)
	// All +/- buttons share the unified blue stepper spec.
	BPMDecStyle = ButtonStyleFromSpec(ComponentButtonStepper)
	BPMIncStyle = ButtonStyleFromSpec(ComponentButtonStepper)
	LenDecStyle = ButtonStyleFromSpec(ComponentButtonStepper)
	LenIncStyle = ButtonStyleFromSpec(ComponentButtonStepper)
	// ButtonStyleFromSpec leaves Radius 0, which draws square.
	LenDecStyle.Radius = RadiusControl
	LenIncStyle.Radius = RadiusControl
	InstButtonStyle = ButtonStyleFromSpec(ComponentButtonRowControl)
	UploadBtnStyle = ButtonStyleFromSpec(ComponentButtonSecondary)
	PopupButtonStyle = ButtonStyleFromSpec(ComponentButtonSecondary)
	DropdownStyle = ButtonStyleFromSpec(ComponentButtonDropdown)
	DisabledButtonStyle = ButtonStyleFromSpec(ComponentButtonDisabled)
	// Delete / destructive
	DeleteButtonStyle = ButtonStyleFromSpec(ComponentButtonDestructive)
	DeleteConfirmButtonStyle = ButtonStyleFromSpec(ComponentButtonDestructiveConfirm)
	// Missing instrument row label
	MissingInstStyle = ButtonStyleFromSpec(ComponentButtonMissingInstrument)
	// Mute/Solo/FX active styles for drum row buttons
	MuteActiveStyle = ButtonStyleFromSpec(ComponentButtonRowControlMuteActive)
	SoloActiveStyle = ButtonStyleFromSpec(ComponentButtonRowControlSoloActive)
	FXActiveStyle = ButtonStyleFromSpec(ComponentButtonRowControlFxActive)

	// No spec is a fully transparent fill, so this stays hand-coded.
	ContextMenuItemStyle = ButtonStyle{Fill: color.RGBA{0, 0, 0, 0}, Border: color.NRGBA{0, 0, 0, 0}}

	// EQ band mute / filter buttons.
	EQMuteButtonStyle = ButtonStyleFromSpec(ComponentButtonEqMute)
	EQMuteButtonActiveStyle = ButtonStyleFromSpec(ComponentButtonEqMuteActive)
	EQFilterButtonActiveStyle = ButtonStyleFromSpec(ComponentButtonEqFilterActive)

	// EQ per-band dB text input.
	EQDBBoxStyle = TextInputStyleFromSpec(ComponentInputField, color.White)

	colEQDBLabel = colTextPrimary
	colEQDBLabelMuted = colTextSecondary

	// ── Transport button styles (all share button-secondary recipe) ──
	TransportPlayStyle = ButtonStyleFromSpec(ComponentButtonSecondary)
	TransportStopStyle = ButtonStyleFromSpec(ComponentButtonSecondary)
	TransportIncStyle = ButtonStyleFromSpec(ComponentButtonSecondary)
	TransportDecStyle = ButtonStyleFromSpec(ComponentButtonSecondary)
	TransportMiscStyle = ButtonStyleFromSpec(ComponentButtonSecondary)
	TimelineKeycapStyle = TransportMiscStyle
	TimelineKeycapStyle.Radius = RadiusControl
	TransportFollowOnStyle = ButtonStyleFromSpec(ComponentButtonTransportFollowOn)
	FABStyle = ButtonStyleFromSpec(ComponentButtonPrimary)
	MobileRowLabelStyle = ButtonStyleFromSpec(ComponentButtonRowLabelMobile)

	DrumCellUI = DrumCellStyle{
		On:        colStep,
		Off:       colStepOff,
		Highlight: colHighlight,
		Border:    colStepBorder,
	}

	// The historical map/slice shapes over the generated palettes, so consumers are unchanged.

	// Package-level derivations from other files live here so they re-derive after ApplyTheme.

	// render_chain.go — scope traces / grid / trigger / frozen border.
	colScopeA = WithAlpha(genColorVizScopeTraceA, 180)
	colScopeB = WithAlpha(genColorVizScopeTraceB, 150)
	colScopeAFill = WithAlpha(genColorVizScopeTraceA, AlphaSubtle)
	colScopeBFill = WithAlpha(genColorVizScopeTraceB, AlphaSubtle)
	colScopeDiff = WithAlpha(genColorVizScopeTraceDiff, 200)
	colScopeDiffFill = WithAlpha(genColorVizScopeTraceDiff, AlphaSubtle)
	colScopeBg = genColorVizScopeBg
	colScopeGrid = WithAlpha(genColorBorder, 20)
	colScopeGridMid = WithAlpha(genColorBorder, 35)
	colScopeFrozenBorder = WithAlpha(genColorPrimary, AlphaMedium)

	// render_meters.go — VU zones.
	meterLow = genColorVizMeterLow
	meterMid = genColorVizMeterMid
	meterHigh = genColorVizMeterHigh
	meterBg = genColorVizMeterBg
	meterClip = genColorVizMeterClip

	// render_spectrum.go / render_waveform.go — trace-derived fills.
	colSpectrumCurveFill = WithAlpha(colWaveTrace, AlphaFaint)
	// Visualizers family, never the chrome accent: it must move with the bars it highlights.
	colSpectrumBandEmphasis = WithAlpha(colWaveTrace, AlphaFaint)
	colSpectrumBarTop = WithAlpha(colWaveTrace, AlphaOverlay)
	colSpectrumBarMid = WithAlpha(colWaveTrace, AlphaStrong)
	colSpectrumBarBase = WithAlpha(colWaveTrace, AlphaMedium)
	colWaveTraceFill = WithAlpha(colWaveTrace, AlphaSubtle)

	// scrollbar_style.go — shared scrollbar palettes.
	DefaultScrollbarStyle = ScrollbarStyle{
		TrackColor: WithAlpha(genColorBorder, genAlphaBorderThin),
		ThumbColor: WithAlpha(genColorPrimary, genAlphaScrollbarThumb),
		Width:      6,
		MinThumbH:  10,
		live:       scrollbarLiveDefault,
	}
	MobileScrollbarStyle = ScrollbarStyle{
		TrackColor: WithAlpha(genColorBorder, genAlphaBorderThin),
		ThumbColor: WithAlpha(genColorPrimary, genAlphaScrollbarThumbMobile),
		Width:      mobileScrollbarWidth,
		MinThumbH:  44,
		live:       scrollbarLiveMobile,
	}
	DropdownScrollbarStyle = ScrollbarStyle{
		TrackColor: WithAlpha(genColorBorder, genAlphaBorderThin),
		ThumbColor: WithAlpha(genColorPrimary, genAlphaScrollbarThumb),
		Width:      10,
		MinThumbH:  12,
		live:       scrollbarLiveDropdown,
	}

	// A data canvas: the viz family, never the chrome accent.
	colConceptFill = WithAlpha(colWaveTrace, AlphaSubtle)
	colConceptStroke = WithAlpha(colWaveTrace, AlphaStrong)

	// synth_view_draw.go — Synth-tab graph roles (data canvases: Visualizers family).
	colGraphLo = WithAlpha(genColorVizTreble, AlphaStrong)
	colGraphHi = WithAlpha(genColorVizBass, AlphaStrong)
	colGraphBefore = WithAlpha(genColorVizWaveB, AlphaStrong)
	colGraphDiff = WithAlpha(genColorVizGlow, AlphaFaint)
	colGraphGrid = WithAlpha(genColorBorder, AlphaFaint)
	colGraphPlotBg = WithAlpha(genColorVizScopeBg, AlphaOverlay)
	colGraphGlow = WithAlpha(genColorVizGlow, AlphaOverlay)
	colGraphGlowRing = WithAlpha(genColorVizGlow, AlphaStrong)

	// uigrid.go — fuzzy-match highlight.
	colFuzzyHighlight = WithAlpha(genColorPrimary, genAlphaSubtle)

	// Pre-boxed so drawPillTabAt allocates nothing.
	pillCapFillInactive = colSurface2
	pillCapBorderInact = colButtonBorder
	pillTextInactive = colTextSecondary
	pillCapFillActive = genColorSunsetGold300
	pillCapBorderActive = genColorSunsetGold300
	pillTextActive = genColorBackground
	pillShellInactive = adjustColor(colSurface2, -45)
	pillShellActive = adjustColor(genColorSunsetGold300, -45)
	pillCapFillDisabled = adjustColor(colSurface2, -20)
	pillCapBorderDisab = adjustColor(colButtonBorder, -20)
	pillTextDisabled = colTextDisabled
	pillShellDisabled = adjustColor(colSurface2, -55)

	// button_styles.go — hand-composed styles on top of specs.
	PrimaryActionStyle = primaryActionStyle()

}

// One map copy at startup.
func buildInstColors() map[instrument.ID]color.Color {
	out := make(map[instrument.ID]color.Color, len(genInstrumentDefaults))
	for id, c := range genInstrumentDefaults {
		out[id] = c
	}
	return out
}

func buildCustomPalette() []color.Color {
	out := make([]color.Color, len(genInstrumentFallbackPalette))
	for i, c := range genInstrumentFallbackPalette {
		out[i] = c
	}
	return out
}
