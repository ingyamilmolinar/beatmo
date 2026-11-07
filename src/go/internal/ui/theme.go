package ui

import (
	"image/color"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// ── Surface hierarchy (3 distinct background levels + raised) ────────────
//
// Surface, accent, text, semantic-state, and viz colors are aliased from
// design_tokens.gen.go (which mirrors DESIGN.md). To change a hex value,
// edit DESIGN.md and run `make gen-design-tokens` — never edit the
// genColor* symbols or these aliases directly.
// Every semantic alias / style below is DECLARED here and DERIVED in
// rebuildThemeAliases() from the live genColor* tokens. The derivation runs
// once at init and again after ApplyTheme (theme_runtime.go) overwrites the
// tokens — a var initialiser would be evaluated only once and go stale.
var (
	colBGTop                  color.RGBA
	colBGBottom               color.RGBA
	colGridHorizon            color.RGBA
	colSurface1               color.RGBA
	colSurface2               color.RGBA
	colSurface3               color.RGBA
	colGridLine               color.RGBA
	colGridHalf               color.RGBA
	colGridQuarter            color.RGBA
	colGridEighth             color.RGBA
	colGridSixteenth          color.RGBA
	colGridThirtySecond       color.RGBA
	colBorderSubtle           color.NRGBA
	colBorderMedium           color.NRGBA
	colBorderStrong           color.NRGBA
	colButtonBorder           color.NRGBA
	colAccent                 color.RGBA
	colAccentBright           color.RGBA
	colAccentDim              color.RGBA
	colAccentSubtle           color.NRGBA
	colPlayGreen              color.RGBA
	colStopRed                color.RGBA
	colMuteRed                color.RGBA
	colTextPrimary            color.RGBA
	colTextSecondary          color.RGBA
	colTextDisabled           color.RGBA
	colTextAccent             color.RGBA
	colPlayButton             color.RGBA
	colStopButton             color.RGBA
	colDropdown               color.RGBA
	colDropdownEdge           color.NRGBA
	colError                  color.RGBA
	colSplitterHandle         color.NRGBA
	colSplitterHandleHover    color.NRGBA
	colRackSurface            color.RGBA
	colPanelBG                color.NRGBA
	colPanelBorder            color.NRGBA
	colScrim                  color.NRGBA
	colDeleteFill             color.RGBA
	colDeleteBorder           color.RGBA
	colDeleteText             color.RGBA
	colMenuGroupDeleteBG      color.NRGBA
	colMenuIcon               color.RGBA
	colStep                   color.RGBA
	colStepOff                color.RGBA
	colStepBorder             color.NRGBA
	colHighlight              color.RGBA
	colMuteCell               color.RGBA
	colMuteHighlight          color.NRGBA
	colBeatGroupAlt           color.NRGBA
	colTimelineTotal          color.RGBA
	colTimelineView           color.NRGBA
	colTimelineViewHi         color.RGBA
	colTimelineCursor         color.RGBA
	colTimelineBeat           color.RGBA
	colEQBg                   color.NRGBA
	colEQBar                  color.RGBA
	colEQBarPeak              color.RGBA
	colWaveTrace              color.RGBA
	colWaveTraceDry           color.NRGBA
	colWaveMid                color.NRGBA
	colEQCurve                color.NRGBA
	colEQCurveFill            color.NRGBA
	colEQHandle               color.RGBA
	colEQHandleActive         color.RGBA
	colEQHandleBorder         color.RGBA
	colEQZeroLine             color.NRGBA
	colEQFilterHandle         color.RGBA
	colEQFilterHandleActive   color.RGBA
	colEQFilterLine           color.NRGBA
	colVizBass                color.RGBA
	colVizMids                color.RGBA
	colVizTreble              color.RGBA
	NodeUI                    NodeStyle
	SignalUI                  SignalStyle
	EdgeUI                    EdgeStyle
	colTransportSurface       color.RGBA
	colTransportBorder        color.NRGBA
	colTransportDivider       color.NRGBA
	colTransportGroupBG       color.NRGBA
	colPlayIconTint           color.RGBA
	colStopIconTint           color.RGBA
	colIncDecIcon             color.RGBA
	colVolumeIconOn           color.RGBA
	colFollowActive           color.RGBA
	colRecordIdle             color.RGBA
	colRecordActive           color.RGBA
	colMuteActive             color.RGBA
	colMuteActiveBdr          color.RGBA
	colSoloActive             color.RGBA
	colSoloActiveBdr          color.RGBA
	PlayButtonStyle           ButtonStyle
	StopButtonStyle           ButtonStyle
	BPMBoxStyle               TextInputStyle
	BPMDecStyle               ButtonStyle
	BPMIncStyle               ButtonStyle
	LenDecStyle               ButtonStyle
	LenIncStyle               ButtonStyle
	InstButtonStyle           ButtonStyle
	UploadBtnStyle            ButtonStyle
	PopupButtonStyle          ButtonStyle
	DropdownStyle             ButtonStyle
	DisabledButtonStyle       ButtonStyle
	DeleteButtonStyle         ButtonStyle
	DeleteConfirmButtonStyle  ButtonStyle
	MissingInstStyle          ButtonStyle
	MuteActiveStyle           ButtonStyle
	SoloActiveStyle           ButtonStyle
	FXActiveStyle             ButtonStyle
	ContextMenuItemStyle      ButtonStyle
	EQMuteButtonStyle         ButtonStyle
	EQMuteButtonActiveStyle   ButtonStyle
	EQFilterButtonActiveStyle ButtonStyle
	EQDBBoxStyle              TextInputStyle
	colEQDBLabelMuted         color.RGBA
	TransportPlayStyle        ButtonStyle
	TransportStopStyle        ButtonStyle
	TransportIncStyle         ButtonStyle
	TransportDecStyle         ButtonStyle
	TransportMiscStyle        ButtonStyle
	TimelineKeycapStyle       ButtonStyle
	TransportFollowOnStyle    ButtonStyle
	FABStyle                  ButtonStyle
	MobileRowLabelStyle       ButtonStyle
	DrumCellUI                DrumCellStyle
)

// Runtime STATE, not theme derivation — initialised once and NEVER touched
// by rebuildThemeAliases (a theme change must not wipe user-assigned custom
// instrument colors).
var (
	instColors      = buildInstColors()
	customPalette   = buildCustomPalette()
	customColors    = map[instrument.ID]color.Color{}
	nextCustomColor int
)

func init() { rebuildThemeAliases() }

// rebuildThemeAliases derives every semantic color alias and every
// spec-backed style from the CURRENT genColor* token variables. Runs once at
// init and again from ApplyTheme after a theme overwrites tokens. Order
// matters: rebuildComponentSpecs() must have run first, since the
// *StyleFromSpec calls below copy from the spec table.
func rebuildThemeAliases() {
	// Level 0 (base): deepest background — grid pane, timeline
	colBGTop = genColorBackground
	colBGBottom = genColorBackground
	// Sunset horizon: warm magenta-violet at the bottom edge of the grid
	// pane; the pane grades from colBGTop (top) to this (bottom). See
	// DESIGN.md colors.grid-horizon.
	colGridHorizon = genColorGridHorizon

	// Level 1 (surface): main surfaces — row rack, transport bg
	colSurface1 = genColorSurface1

	// Level 2 (elevated): cards, buttons, input fields
	colSurface2 = genColorSurface2

	// Level 3 (raised): hover states, active surfaces
	colSurface3 = genColorSurface3

	// ── Grid colors (brightened slightly for visibility against Level 0) ──
	// Sourced from DESIGN.md grid-* color tokens.
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

	// ── Accent (Primary — Cyan) ─────────────────────────────────────────
	colAccent = genColorPrimary
	colAccentBright = genColorPrimaryBright
	colAccentDim = genColorPrimaryDim
	// colAccentSubtle composes primary with the panel-border bucket (20).
	// 20 sits between subtle (8) and faint (25); reusing the panel-border
	// bucket here keeps every "20-alpha overlay" decision in one place.
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
	colDropdown = colSurface2                                        // elevated surface for dropdowns
	colDropdownEdge = WithAlpha(genColorPrimary, genAlphaAccentTint) // magenta accent border @ alpha.accent-tint (50)
	colError = genColorError                                         // DESIGN.md error

	// Secondary button fill for file-ops, overflow, etc.

	// Row control button inactive state

	// Row label brighter color

	// Splitter handle colors (handle indicator on divider lines). One cyan
	// color on every platform (desktop + mobile share it). Sourced from the
	// DESIGN.md splitter-handle token at the strong alpha bucket.
	// `splitter-handle-hover` reuses the universal white `border` token at
	// the splitter-hover bucket; the role is "splitter handle at hover", the
	// hex is white (no separate token needed since border already names it).
	colSplitterHandle = WithAlpha(genColorSplitterHandle, genAlphaStrong)
	colSplitterHandleHover = WithAlpha(genColorBorder, genAlphaSplitterHover)

	// Rack surface: Level 1 for visual separation
	colRackSurface = genColorRackBed // Background-category bed (DESIGN.md rack-bed), NOT the surface-1 control surface

	// Panel/popup surface colors — unified component kit.
	// Both panel BG and modal scrim now name their own alpha buckets
	// (`alpha.panel-near-opaque`, `alpha.scrim`) — no more inline 250/150.
	colPanelBG = WithAlpha(genColorSurfaceOverlay, genAlphaPanelNearOpaque) // surface-overlay @ alpha.panel-near-opaque (250)
	colPanelBorder = WithAlpha(genColorBorder, genAlphaBorderPanel)         // panel chrome border @ alpha.border-panel (20)
	colScrim = WithAlpha(genColorDimBlack, genAlphaScrim)                   // backdrop scrim @ alpha.scrim (150)

	// Delete/destructive action colors
	colDeleteFill = genColorDestructive
	colDeleteBorder = genColorDestructiveBorder
	colDeleteText = colStopRed // red text for delete items (less aggressive than red bg)

	// Context menu group container backgrounds.
	// Both groups now use named tokens × named alpha buckets — the
	// menu-delete-tint TODO that previously kept this inline is resolved
	// (see DESIGN.md `colors.menu-delete-tint`).
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

	// Timeline strip palette — sourced from DESIGN.md timeline-* tokens.
	// Amber accent family is intentional (data-canvas color, not chrome —
	// see "Single chrome accent" invariant exemption for `viz-*` family).
	colTimelineTotal = genColorTimelineTotalBg
	colTimelineView = WithAlpha(genColorTimelineView, genAlphaSubtle) // view region @ alpha.subtle (60)
	colTimelineViewHi = genColorTimelineViewHi
	colTimelineCursor = genColorTimelineCursor
	colTimelineBeat = genColorTimelineBeat
	// viz-bg/curve/wave-b carry intentional non-255 alpha; sourced from
	// generated symbols and composed via WithAlpha + named buckets. The
	// previous "stays inline" exemption for WaveTraceDry / EQ zero-line /
	// EQ curve-fill is now resolved (alpha.wave-trace-dry, alpha.eq-zero-line,
	// alpha.eq-curve-fill in DESIGN.md).
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

	// Spectrum frequency-group tinting (warm = bass, neutral = mids, cool
	// = treble). Sourced from DESIGN.md `viz-bass` / `viz-mids` /
	// `viz-treble`. Used by the Spectrum tab's Bass/Mids/Treble bracket
	// strip + group highlight fills.
	colVizBass = genColorVizBass
	colVizMids = genColorVizMids
	colVizTreble = genColorVizTreble

	// Graph-pane node / signal / edge — colors + geometry both sourced
	// from DESIGN.md (Phase 1 colors + Phase 4 geometry now extended).
	NodeUI = NodeStyle{Radius: float32(genGeomNodeRadius), Fill: genColorNodeFill, Border: genColorNodeBorder}
	SignalUI = SignalStyle{Radius: float32(genGeomSignalRadius), Color: colAccent}
	EdgeUI = EdgeStyle{Color: WithAlpha(genColorEdgeColor, genAlphaEdgeDefault)} // edge @ alpha.edge-default (100)

	// Transport surface: Level 1 for visual hierarchy
	colTransportSurface = genColorToolbarBed // Background-category bed (DESIGN.md toolbar-bed)
	// Transport border: subtle section divider
	colTransportBorder = colBorderSubtle
	// Transport container border (visible grouping)
	// Transport group divider — white@20 = alpha.border-panel
	colTransportDivider = WithAlpha(genColorBorder, genAlphaBorderPanel)

	// Pill container colors for transport button groups
	colTransportGroupBG = WithAlpha(genColorBorder, genAlphaRowRackZebra) // pill container fill — white@10 = alpha.row-rack-zebra

	// Icon tint colors for mobile transport buttons
	colPlayIconTint = colPlayGreen         // green for play icon
	colStopIconTint = colStopRed           // red for stop icon
	colIncDecIcon = colTextPrimary         // primary text for +/- icons (mobile)
	colVolumeIconOn = colTextSecondary     // muted grey when volume > 0 (nil-tint / master speaker; per-row icons pass their instrument color)
	colFollowActive = colTextPrimary       // neutral bright tint for follow-on (track lit); free state dims to colTextDisabled
	colRecordIdle = genColorRecordIdle     // red circle for record button (DESIGN.md record-idle, opaque)
	colRecordActive = genColorRecordActive // bright red when recording (DESIGN.md record-active, opaque)

	// Mute/Solo active state colors
	colMuteActive = colMuteRed
	colMuteActiveBdr = genColorMuteActiveBorder // mute-active-border (DESIGN.md mute-active-border #D87848)
	colSoloActive = genColorPrimaryBright       // primary-bright coral #FFB498
	colSoloActiveBdr = genColorSoloActiveBorder // solo-active-border #FFB498

	// Phase M2 (post-refactor cleanup): every var below sources its
	// fill/border from a generated ComponentSpec. The hand-coded literals
	// they used to carry are now in DESIGN.md; this file is the legacy-
	// surface compatibility shim until call sites migrate to NewSpecButton
	// directly. Drift between DESIGN.md and the runtime is caught by
	// TestComponentSpecsDrift; byte-equivalence is asserted there.
	PlayButtonStyle = ButtonStyleFromSpec(ComponentButtonPlayDesktop)
	StopButtonStyle = ButtonStyleFromSpec(ComponentButtonStopDesktop)
	BPMBoxStyle = TextInputStyleFromSpec(ComponentInputField, color.White)
	// All +/- buttons share the unified blue stepper spec.
	BPMDecStyle = ButtonStyleFromSpec(ComponentButtonStepper)
	BPMIncStyle = ButtonStyleFromSpec(ComponentButtonStepper)
	LenDecStyle = ButtonStyleFromSpec(ComponentButtonStepper)
	LenIncStyle = ButtonStyleFromSpec(ComponentButtonStepper)
	// The timeline band's keycaps (length ±, lock) carry the control radius:
	// ButtonStyleFromSpec leaves Radius 0, which draws a legacy style square.
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

	// Context menu items: transparent — container provides visual boundary.
	// No spec is byte-equivalent to a fully-transparent fill, so this stays
	// hand-coded; future migration could add a `transparent-button` spec.
	ContextMenuItemStyle = ButtonStyle{Fill: color.RGBA{0, 0, 0, 0}, Border: color.NRGBA{0, 0, 0, 0}}

	// EQ band mute / filter buttons.
	EQMuteButtonStyle = ButtonStyleFromSpec(ComponentButtonEqMute)
	EQMuteButtonActiveStyle = ButtonStyleFromSpec(ComponentButtonEqMuteActive)
	EQFilterButtonActiveStyle = ButtonStyleFromSpec(ComponentButtonEqFilterActive)

	// EQ per-band dB text input.
	EQDBBoxStyle = TextInputStyleFromSpec(ComponentInputField, color.White)

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

	// instColors and customPalette are now sourced from DESIGN.md's
	// `instrumentDefaults:` and `instrumentFallbackPalette:` blocks
	// (Phase 3 PR2). Editing the palette is now a one-file edit in
	// DESIGN.md; the runtime aliases below preserve the historical
	// `map[string]color.Color` / `[]color.Color` shape so consumers
	// (drumview.go, drumview_colors.go) don't need to change.

	// ── Folded per-file derivations (Task 3 of the theme-editor plan) ──
	// These used to be package-level initialisers in render_chain.go,
	// render_meters.go, render_spectrum.go, render_waveform.go,
	// scrollbar_style.go, synth_view_draw.go, uigrid.go, audio_sticky_bar.go
	// and button_styles.go. Evaluated once at init they went stale after
	// ApplyTheme; here they re-derive with the rest. Guard:
	// TestNoTopLevelDerivedColorsOutsideTheme.

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
	// Emphasis lane behind the ISO band the Spectrum banner's sentence refers
	// to, so the words and the picture point at the same place. Visualizers
	// family, never the chrome accent — a theme that separates Accent from
	// Visualizers must keep this with the bars it highlights.
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

	// synth_view_draw.go — the Synth tab's Visualizers accent (graphs, dials,
	// cards): a data canvas, so the viz family, never the chrome accent.
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

	// audio_sticky_bar.go — pill tab palette (pre-boxed color.Color so
	// drawPillTabAt stays allocation-free).
	pillCapFillInactive = colSurface2
	pillTextInactive = colTextSecondary
	pillCapFillActive = genColorSunsetGold300
	pillTextActive = genColorBackground
	pillCapFillDisabled = adjustColor(colSurface2, -20)
	pillTextDisabled = colTextDisabled

	// button_styles.go — hand-composed styles on top of specs.
	PrimaryActionStyle = primaryActionStyle()

}

// buildInstColors lifts genInstrumentDefaults (typed as
// map[string]color.RGBA by the generator) into the historical
// map[string]color.Color shape that drumview.go expects. Done at
// package init via the var initialiser above; cost is one map copy
// at startup.
func buildInstColors() map[instrument.ID]color.Color {
	out := make(map[instrument.ID]color.Color, len(genInstrumentDefaults))
	for id, c := range genInstrumentDefaults {
		out[id] = c
	}
	return out
}

// buildCustomPalette lifts genInstrumentFallbackPalette ([]color.RGBA)
// into []color.Color so drumview.go's existing fallback-cycle logic
// keeps working unchanged.
func buildCustomPalette() []color.Color {
	out := make([]color.Color, len(genInstrumentFallbackPalette))
	for i, c := range genInstrumentFallbackPalette {
		out[i] = c
	}
	return out
}
