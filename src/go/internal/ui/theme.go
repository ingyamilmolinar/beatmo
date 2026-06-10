package ui

import "image/color"

// ── Surface hierarchy (3 distinct background levels + raised) ────────────
//
// Surface, accent, text, semantic-state, and viz colors are aliased from
// design_tokens.gen.go (which mirrors DESIGN.md). To change a hex value,
// edit DESIGN.md and run `make gen-design-tokens` — never edit the
// genColor* symbols or these aliases directly.
var (
	// Level 0 (base): deepest background — grid pane, timeline
	colBGTop    = genColorBackground
	colBGBottom = genColorBackground

	// Level 1 (surface): main surfaces — row rack, transport bg
	colSurface1 = genColorSurface1

	// Level 2 (elevated): cards, buttons, input fields
	colSurface2 = genColorSurface2

	// Level 3 (raised): hover states, active surfaces
	colSurface3 = genColorSurface3

	// ── Grid colors (brightened slightly for visibility against Level 0) ──
	// Sourced from DESIGN.md grid-* color tokens.
	colGridLine         = genColorGridLine
	colGridHalf         = genColorGridHalf
	colGridQuarter      = genColorGridQuarter
	colGridEighth       = genColorGridEighth
	colGridSixteenth    = genColorGridSixteenth
	colGridThirtySecond = genColorGridThirtySecond

	// ── Borders (structured hierarchy) — sourced from genColorBorder + named alpha buckets ──
	colBorderSubtle = WithAlpha(genColorBorder, genAlphaBorderThin)     // section dividers
	colBorderMedium = WithAlpha(genColorBorder, genAlphaBorderDefault)  // control borders
	colBorderStrong = WithAlpha(genColorBorder, genAlphaBorderEmphasis) // focused input borders

	colButtonBorder = colBorderMedium // alias for control borders
	colSubtleBorder = colBorderSubtle // alias for subtle borders

	// ── Accent (Primary — Cyan) ─────────────────────────────────────────
	colAccent      = genColorPrimary
	colAccentBright = genColorPrimaryBright
	colAccentDim   = genColorPrimaryDim
	// colAccentSubtle composes primary with the panel-border bucket (20).
	// 20 sits between subtle (8) and faint (25); reusing the panel-border
	// bucket here keeps every "20-alpha overlay" decision in one place.
	colAccentSubtle = WithAlpha(genColorPrimary, genAlphaBorderPanel)

	// ── Accent (Secondary — Semantic) ───────────────────────────────────
	colPlayGreen = genColorSuccess
	colStopRed   = genColorError
	colMuteRed   = genColorMute

	// ── Text colors ─────────────────────────────────────────────────────
	colTextPrimary   = genColorOnSurface
	colTextSecondary = genColorOnSurfaceMuted
	colTextDisabled  = genColorOnSurfaceDisabled
	colTextAccent    = genColorOnSurfaceAccent

	// ── Legacy aliases (mapped to new surface system) ───────────────────
	colPlayButton = genColorPlayDesktopFill                           // lime play fill (DESIGN.md play-desktop-fill)
	colStopButton = genColorStopDesktopFill                           // wine stop fill (DESIGN.md stop-desktop-fill)
	colBPMBox     = colSurface2                                       // elevated surface for inputs
	colDropdown   = colSurface2                                       // elevated surface for dropdowns
	colDropdownEdge = WithAlpha(genColorPrimary, genAlphaAccentTint)  // magenta accent border @ alpha.accent-tint (50)
	colError      = genColorError                                     // DESIGN.md error

	// Secondary button fill for file-ops, overflow, etc.
	colSecondaryBtn = colSurface2

	// Row control button inactive state
	colRowBtnInactive     = colSurface1
	colRowBtnInactiveText = colTextSecondary

	// Row label brighter color
	colRowLabel = colTextPrimary

	// Splitter handle colors (pill indicator on divider lines).
	// Sourced from DESIGN.md splitter-* color tokens × alpha buckets.
	// `splitter-handle-hover` reuses the universal white `border` token at
	// the splitter-hover bucket; the role is "splitter pill at hover", the
	// hex is white (no separate token needed since border already names it).
	colSplitterHandle        = WithAlpha(genColorSplitterHandle, genAlphaOverlay)
	colSplitterHandleMobile  = WithAlpha(genColorSplitterHandleMobile, genAlphaStrong)
	colSplitterHandleHover   = WithAlpha(genColorBorder, genAlphaSplitterHover)
	colSplitterGripLine      = WithAlpha(genColorSplitterGripLine, genAlphaStrong)
	colSplitterGripLineHover = WithAlpha(genColorSplitterGripLineHover, genAlphaSplitterGripHover)

	// Rack surface: Level 1 for visual separation
	colRackSurface = colSurface1

	// Panel/popup surface colors — unified component kit.
	// Both panel BG and modal scrim now name their own alpha buckets
	// (`alpha.panel-near-opaque`, `alpha.scrim`) — no more inline 250/150.
	colPanelBG     = WithAlpha(genColorSurfaceOverlay, genAlphaPanelNearOpaque) // surface-overlay @ alpha.panel-near-opaque (250)
	colPanelBorder = WithAlpha(genColorBorder, genAlphaBorderPanel)             // panel chrome border @ alpha.border-panel (20)
	colScrim       = WithAlpha(genColorDimBlack, genAlphaScrim)                 // backdrop scrim @ alpha.scrim (150)

	// Delete/destructive action colors
	colDeleteFill   = genColorDestructive
	colDeleteBorder = genColorDestructiveBorder
	colDeleteText   = colStopRed // red text for delete items (less aggressive than red bg)

	// Context menu group container backgrounds.
	// Both groups now use named tokens × named alpha buckets — the
	// menu-delete-tint TODO that previously kept this inline is resolved
	// (see DESIGN.md `colors.menu-delete-tint`).
	colMenuGroupBG       = WithAlpha(genColorBorder, genAlphaBorderThin)        // group container bg @ alpha.border-thin (8)
	colMenuGroupDeleteBG = WithAlpha(genColorMenuDeleteTint, genAlphaBorderDefault) // delete-row warning tint @ alpha.border-default (15)
	colMenuIcon          = colTextDisabled                  // icon tint

	colStep          = colAccent                                       // primary-magenta active cells
	colStepOff       = genColorDrumCellOff                             // cell-off resting fill (DESIGN.md drum-cell-off)
	colStepBorder    = WithAlpha(genColorBorder, AlphaRowRackZebra)    // very subtle cell borders (border@10)
	colHighlight     = genColorDrumCellHighlight                       // cool-white per-beat flash (DESIGN.md drum-cell-highlight)
	colMuteCell      = genColorDrumMuteCell                            // muted-row resting fill (DESIGN.md drum-mute-cell)
	colMuteHighlight = WithAlpha(genColorDrumMuteHighlight, genAlphaMuteHighlight) // muted-row beat flash @ alpha.mute-highlight (160)

	// Beat grouping: alternate column tint for visual beat separation
	colBeatGroupAlt = WithAlpha(genColorBorder, genAlphaBeatGroupAlt)

	// Timeline strip palette — sourced from DESIGN.md timeline-* tokens.
	// Amber accent family is intentional (data-canvas color, not chrome —
	// see "Single chrome accent" invariant exemption for `viz-*` family).
	colTimelineTotal        = genColorTimelineTotalBg
	colTimelineView         = WithAlpha(genColorTimelineView, genAlphaSubtle) // view region @ alpha.subtle (60)
	colTimelineViewHi       = genColorTimelineViewHi
	colTimelineCursor       = genColorTimelineCursor
	colTimelineBeat         = genColorTimelineBeat
	// viz-bg/curve/wave-b carry intentional non-255 alpha; sourced from
	// generated symbols and composed via WithAlpha + named buckets. The
	// previous "stays inline" exemption for WaveTraceDry / EQ zero-line /
	// EQ curve-fill is now resolved (alpha.wave-trace-dry, alpha.eq-zero-line,
	// alpha.eq-curve-fill in DESIGN.md).
	colEQBg                 = WithAlpha(genColorVizBg, genAlphaOverlay)              // viz-bg @ alpha.overlay (220)
	colEQBar                = genColorVizBar
	colEQBarPeak            = genColorVizBarPeak
	colWaveTrace            = genColorVizWaveA
	colWaveTraceDry         = WithAlpha(genColorWaveTraceDry, genAlphaWaveTraceDry)  // wave dry trace @ alpha.wave-trace-dry (100)
	colWaveMid              = WithAlpha(genColorVizWaveB, genAlphaSidebarChip)       // wave-b @ alpha.sidebar-chip (200)
	colEQCurve              = WithAlpha(genColorVizCurve, 230)                       // curve stroke at intentional 230
	colEQCurveFill          = WithAlpha(genColorTimelineView, genAlphaEqCurveFill)   // amber fill @ alpha.eq-curve-fill (20); shares hex with timeline-view by design
	colEQHandle             = colAccent
	colEQHandleActive       = colAccentBright
	colEQHandleBorder       = colAccentDim
	colEQZeroLine           = WithAlpha(genColorEqZeroLine, genAlphaEqZeroLine)      // 0 dB ruler @ alpha.eq-zero-line (80)
	colEQFilterHandle       = genColorVizBar                                         // filter handle reuses viz-bar (cyan, opaque)
	colEQFilterHandleActive = genColorVizBarPeak                                     // active state reuses viz-bar-peak
	colEQFilterLine         = WithAlpha(genColorVizBar, genAlphaSubtle)              // filter line @ alpha.subtle (60)

	// Spectrum frequency-group tinting (warm = bass, neutral = mids, cool
	// = treble). Sourced from DESIGN.md `viz-bass` / `viz-mids` /
	// `viz-treble`. Used by the Spectrum tab's Bass/Mids/Treble bracket
	// strip + group highlight fills.
	colVizBass   = genColorVizBass
	colVizMids   = genColorVizMids
	colVizTreble = genColorVizTreble

	// Graph-pane node / signal / edge — colors + geometry both sourced
	// from DESIGN.md (Phase 1 colors + Phase 4 geometry now extended).
	NodeUI   = NodeStyle{Radius: float32(genGeomNodeRadius), Fill: genColorNodeFill, Border: genColorNodeBorder}
	SignalUI = SignalStyle{Radius: float32(genGeomSignalRadius), Color: colAccent}
	EdgeUI   = EdgeStyle{Color: WithAlpha(genColorEdgeColor, genAlphaEdgeDefault)} // edge @ alpha.edge-default (100)

	// Unified increment/decrement button color (deep-navy + steel-blue border).
	// Same hex as `stepper-*`, but the runtime carries a separate var name
	// for the legacy +/- code path; aliased to the generated stepper tokens.
	colIncDec       = genColorStepperFill
	colIncDecBorder = genColorStepperBorder

	// Transport surface: Level 1 for visual hierarchy
	colTransportSurface = colSurface1
	// Transport border: subtle section divider
	colTransportBorder = colBorderSubtle
	// Transport container border (visible grouping)
	colTransportContainerBorder = colBorderMedium
	// Transport group divider — white@20 = alpha.border-panel
	colTransportDivider = WithAlpha(genColorBorder, genAlphaBorderPanel)

	// Pill container colors for transport button groups
	colTransportGroupBG     = WithAlpha(genColorBorder, genAlphaRowRackZebra)    // pill container fill — white@10 = alpha.row-rack-zebra
	colTransportGroupBorder = WithAlpha(genColorBorder, genAlphaBorderDefault)   // pill container border — white@15 = alpha.border-default

	// Icon tint colors for mobile transport buttons
	colPlayIconTint  = colPlayGreen                    // green for play icon
	colStopIconTint  = colStopRed                      // red for stop icon
	colIncDecIcon    = colTextPrimary                   // primary text for +/- icons (mobile)
	colIncDecIconHi  = genColorIncdecIconHi             // amber for +/- icons (desktop), DESIGN.md incdec-icon-hi
	colVolumeIconOn  = colTextPrimary                   // primary when volume > 0
	colVolumeIconOff = colTextDisabled                  // disabled when muted/zero
	colFollowActive  = colAccentBright                  // primary-bright coral for follow-on (DESIGN.md single-chrome-accent)
	colRecordIdle    = genColorRecordIdle                // red circle for record button (DESIGN.md record-idle, opaque)
	colRecordActive  = genColorRecordActive              // bright red when recording (DESIGN.md record-active, opaque)

	// Mute/Solo active state colors
	colMuteActive    = colMuteRed
	colMuteActiveBdr = genColorMuteActiveBorder       // mute-active-border (DESIGN.md mute-active-border #D87848)
	colSoloActive    = genColorPrimaryBright           // primary-bright coral #FFB498
	colSoloActiveBdr = genColorSoloActiveBorder        // solo-active-border #FFB498

	// Phase M2 (post-refactor cleanup): every var below sources its
	// fill/border from a generated ComponentSpec. The hand-coded literals
	// they used to carry are now in DESIGN.md; this file is the legacy-
	// surface compatibility shim until call sites migrate to NewSpecButton
	// directly. Drift between DESIGN.md and the runtime is caught by
	// TestComponentSpecsDrift; byte-equivalence is asserted there.
	PlayButtonStyle = ButtonStyleFromSpec(ComponentButtonPlayDesktop)
	StopButtonStyle = ButtonStyleFromSpec(ComponentButtonStopDesktop)
	BPMBoxStyle     = TextInputStyleFromSpec(ComponentInputField, color.White)
	// All +/- buttons share the unified blue stepper spec.
	BPMDecStyle         = ButtonStyleFromSpec(ComponentButtonStepper)
	BPMIncStyle         = ButtonStyleFromSpec(ComponentButtonStepper)
	LenDecStyle         = ButtonStyleFromSpec(ComponentButtonStepper)
	LenIncStyle         = ButtonStyleFromSpec(ComponentButtonStepper)
	InstButtonStyle     = ButtonStyleFromSpec(ComponentButtonRowControl)
	UploadBtnStyle      = ButtonStyleFromSpec(ComponentButtonSecondary)
	PopupButtonStyle    = ButtonStyleFromSpec(ComponentButtonSecondary)
	DropdownStyle       = ButtonStyleFromSpec(ComponentButtonDropdown)
	DisabledButtonStyle = ButtonStyleFromSpec(ComponentButtonDisabled)
	// Delete / destructive
	DeleteButtonStyle        = ButtonStyleFromSpec(ComponentButtonDestructive)
	DeleteConfirmButtonStyle = ButtonStyleFromSpec(ComponentButtonDestructiveConfirm)
	// Missing instrument row label
	MissingInstStyle = ButtonStyleFromSpec(ComponentButtonMissingInstrument)
	// Mute/Solo/FX active styles for drum row buttons
	MuteActiveStyle = ButtonStyleFromSpec(ComponentButtonRowControlMuteActive)
	SoloActiveStyle = ButtonStyleFromSpec(ComponentButtonRowControlSoloActive)
	FXActiveStyle   = ButtonStyleFromSpec(ComponentButtonRowControlFxActive)

	// Context menu items: transparent — container provides visual boundary.
	// No spec is byte-equivalent to a fully-transparent fill, so this stays
	// hand-coded; future migration could add a `transparent-button` spec.
	ContextMenuItemStyle = ButtonStyle{Fill: color.RGBA{0, 0, 0, 0}, Border: color.NRGBA{0, 0, 0, 0}}

	// EQ band mute / filter buttons.
	EQMuteButtonStyle         = ButtonStyleFromSpec(ComponentButtonEqMute)
	EQMuteButtonActiveStyle   = ButtonStyleFromSpec(ComponentButtonEqMuteActive)
	EQFilterButtonActiveStyle = ButtonStyleFromSpec(ComponentButtonEqFilterActive)

	// EQ per-band dB text input.
	EQDBBoxStyle = TextInputStyleFromSpec(ComponentInputField, color.White)

	colEQDBLabel      = colTextPrimary
	colEQDBLabelMuted = colTextSecondary

	// ── Transport button styles (all share button-secondary recipe) ──
	TransportPlayStyle     = ButtonStyleFromSpec(ComponentButtonSecondary)
	TransportStopStyle     = ButtonStyleFromSpec(ComponentButtonSecondary)
	TransportIncStyle      = ButtonStyleFromSpec(ComponentButtonSecondary)
	TransportDecStyle      = ButtonStyleFromSpec(ComponentButtonSecondary)
	TransportMiscStyle     = ButtonStyleFromSpec(ComponentButtonSecondary)
	TransportFollowOnStyle = ButtonStyleFromSpec(ComponentButtonTransportFollowOn)
	FABStyle               = ButtonStyleFromSpec(ComponentButtonPrimary)
	MobileRowLabelStyle    = ButtonStyleFromSpec(ComponentButtonRowLabelMobile)

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
	instColors    = buildInstColors()
	customPalette = buildCustomPalette()

	customColors    = map[string]color.Color{}
	nextCustomColor int
)

// buildInstColors lifts genInstrumentDefaults (typed as
// map[string]color.RGBA by the generator) into the historical
// map[string]color.Color shape that drumview.go expects. Done at
// package init via the var initialiser above; cost is one map copy
// at startup.
func buildInstColors() map[string]color.Color {
	out := make(map[string]color.Color, len(genInstrumentDefaults))
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
