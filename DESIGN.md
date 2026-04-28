---
version: "1"
name: Beatmo
description: >
  Beatmo UI design system. Dense graph + timeline + EQ + scope + per-row
  controls coexist in a single canvas without becoming noisy. Cushioned warm-slate
  theme with a single-coral accent; instrument identity is the only multi-color
  signal in the chrome. Built on top of Ebiten (desktop) and a WASM/WebAudio
  runtime (browser).
colors:
  # Primary interactive (single coral accent — soft pop, used for every
  # active / selected / focused chrome state). See "Single chrome accent"
  # invariant in the prose.
  primary: "#FF8C5A"
  primary-bright: "#FFB498"
  primary-dim: "#C56448"

  # Surface hierarchy (4 levels, lifted warm-slate theme — the ladder feels
  # like soft matte plastic, not inky aerospace). One notch lighter than
  # the prior navy ladder so chrome reads as touchable.
  background: "#10141C"
  surface-1: "#1A1F2C"
  surface-2: "#262C3C"
  surface-3: "#343B4E"
  surface-overlay: "#1F2533"

  # On-surface text (warm cream whites against the lifted slate)
  on-surface: "#FCEFD0"
  on-surface-muted: "#A0B0C8"
  on-surface-disabled: "#5A6470"
  on-surface-accent: "#FF8C5A"

  # Borders (single base color; opacity is applied at draw time — see "Borders" prose)
  border: "#FFFFFF"

  # Semantic state (mint success, warm-coral error, rust mute,
  # deep-brown destructive)
  success: "#50D080"
  error: "#FF5848"
  mute: "#A04830"
  destructive: "#481414"
  destructive-border: "#A03830"
  record-idle: "#E04040"
  record-active: "#FF4040"

  # Visualization tokens (Wave / Spectrum / Meters / Scope / EQ canvases).
  # NOTE on the viz-bar / viz-wave cyans: these are *data series* colors,
  # NOT chrome. They are the only places cyan is allowed in the app — see
  # the "Single chrome accent" invariant. EQ knobs / handles / curve all
  # use `primary` (coral) so the chrome carries one accent and the data
  # canvases speak their own visual language.
  viz-bg: "#13171F"
  viz-bar: "#5AC8E0"
  viz-bar-peak: "#A0E0FF"
  viz-curve: "#FF8C5A"
  viz-wave-a: "#80E0FF"
  viz-wave-b: "#7A8AA8"

  # Meters tab — VU-style level meter with green / yellow / red zones plus
  # peak-clip indicator. Distinct hue from the spectrum tokens because the
  # meter row scales by amplitude (not by spectral bin), and the green-amber-
  # red ramp is a long-standing convention engineers expect.
  viz-meter-green:  "#50D080"
  viz-meter-yellow: "#F0E020"
  viz-meter-red:    "#FF4030"
  viz-meter-bg:     "#1A2438"
  viz-meter-clip:   "#FF3030"

  # Grid-pane glow / ghost overlay — drawn at runtime alpha (faint for
  # ghost, strong for halo). A pale-coral sibling of `primary` so the
  # transient glow stays in the chrome's single-accent family.
  viz-glow: "#FFC8B4"

  # Focus ring — sits on focused TextInput. A slightly lighter coral
  # than `primary` so the focus stroke reads as "lifted off" the
  # surface; same hue family, brighter value.
  focus-ring: "#FFA68C"

  # Slider thumb (volume / horizontal sliders) — off-white pill with a
  # runtime drop shadow. Off-white (not pure 255) reduces glare against the
  # dark surface ladder.
  slider-thumb-fill:   "#FCEFD0"
  slider-thumb-shadow: "#000000"

  # Slider track background and filled portion. Track is a slightly cooler
  # neutral than surface-3 so the slider chassis doesn't merge with raised
  # button hover surfaces. Fill is `primary-dim` (coral) so the slider
  # value range belongs to the single-accent family but reads as muted
  # rather than primary action.
  slider-track-fill: "#2D3848"
  slider-fill:       "#C56448"

  # Sidebar section background — cool muted steel-blue, drawn at runtime alpha
  # ~100/255 to dim the section relative to the panel surface beneath.
  # Not a derived shade of `surface-*` because the sidebar wants visual
  # distance from the elevated controls layer.
  sidebar-section-bg: "#5A6884"

  # Scope panel — three base trace colors (coral / cyan / lime),
  # the scope canvas background, and the trigger marker. Each is composited
  # at multiple runtime alphas (line, fill, dimmed-label) via WithAlpha.
  # Trace-a snaps to `primary` so the scope's primary tap belongs to the
  # single-accent family; trace-b stays cyan as a data-series partner.
  viz-scope-trace-a:    "#FF8C5A"
  viz-scope-trace-b:    "#80E8FF"
  viz-scope-trace-diff: "#A0E080"
  viz-scope-bg:         "#13171F"
  viz-scope-trigger:    "#FFE060"

  # Spectrum panel peak marker — pale peach, pinned at the highest recent
  # spectrum value. Distinct from `viz-bar` (the live bar) so it reads as
  # a held memory rather than an instantaneous reading.
  viz-spectrum-peak-marker: "#FFE8D0"

  # ── Phase 2 PR4: per-component fill/border colors ────────────────────────
  # Each entry below is consumed by exactly one component recipe. They are
  # NOT a parallel palette — every value here was extracted from a
  # hand-coded ButtonStyle/TextInputStyle and named for the component it
  # belongs to so future audits can trace any in-code color back to its
  # owning component.

  # Desktop Play / Stop buttons — slightly richer red/green than the mobile
  # icon-tint counterparts (success / error). The fill shades drift from
  # their tint companions to give desktop buttons a recognizable "candy"
  # look against the dense transport bar. PlayButtonStyle.Fill / .Border,
  # StopButtonStyle.Fill / .Border respectively.
  play-desktop-fill:   "#40A060"
  play-desktop-border: "#60D080"
  stop-desktop-fill:   "#B82C2C"
  stop-desktop-border: "#E04848"

  # Numeric steppers (BPM ±, Length ±) — a deep-navy fill with a steel-blue
  # border, distinct from the primary accent so the +/- chips read
  # as a secondary control group rather than a primary action. Mirrors
  # colIncDec / colIncDecBorder in the runtime.
  stepper-fill:   "#1C3050"
  stepper-border: "#4080C0"

  # Disabled control fill — slightly above on-surface-disabled so the
  # inert button still has a visible body silhouette against surface-1.
  # Used by DisabledButtonStyle.Fill.
  disabled-fill: "#2D3848"

  # Destructive confirm state ("X" → "!!") — brighter than the resting
  # destructive fill to telegraph the second-click commitment. Used by
  # DeleteConfirmButtonStyle.{Fill,Border}.
  destructive-confirm-fill:   "#D03030"
  destructive-confirm-border: "#FF6060"

  # Mute-active border — sibling of `mute` (the fill); the border is
  # ~30% lighter for visible inset against surface-2. Mirrors
  # colMuteActiveBdr.
  mute-active-border: "#D87848"

  # Solo-active fill mirrors `primary-bright` and is reused via
  # `{colors.primary-bright}`. Border is a lighter coral sibling so the
  # active-solo chip stays inside the single-accent family.
  solo-active-border: "#FFB498"

  # FX-active fill — deep desaturated navy that pairs with the amber
  # accent border to mark a row whose FX panel is open. Mirrors
  # FXActiveStyle.Fill.
  fx-active-fill: "#142A50"

  # EQ filter (HPF/LPF) toggle in active state — coral pair, snapping
  # to `primary-dim` (fill) and `primary` (border). HP/LP active now
  # belongs to the single-accent family along with every other "selected"
  # state. Mirrors EQFilterButtonActiveStyle.{Fill,Border}.
  eq-filter-active-fill:   "#C56448"
  eq-filter-active-border: "#FF8C5A"

  # Transport "follow-on" (locked-scroll) — slate fill with a coral
  # border. The border base is its own shade because it sits at custom
  # alpha 80 over surface-1 (see alpha.accent-overlay). Mirrors
  # TransportFollowOnStyle.{Fill,Border base}.
  transport-follow-on-fill:        "#262C3C"
  transport-follow-on-border-base: "#FFA68C"

  # MissingInstrument fill — slightly darker than the `error` token so
  # the row label reads as "data missing" (a state) rather than "alert"
  # (an event). Pre-existing distinction in the runtime (colError =
  # RGBA{220,60,60,255} vs colStopRed = RGBA{220,70,70,255}).
  error-text: "#FF5848"

  # EQ band-mute active fill — slightly darker red than destructive-border
  # so the EQ chip telegraphs "muted band" without competing with the
  # delete-button red. Pre-existing distinction in the runtime
  # (EQMuteButtonActiveStyle.Fill vs colDeleteBorder).
  eq-mute-active-fill: "#B82C2C"

  # Sidebar chrome — cool steel greys for inline chips, badges, and the
  # default-color swatch fallback. These are runtime composites at the
  # sidebar-section / sidebar-chip alpha buckets; the base hex moves
  # here so all sidebar greys are auditable in one place.
  sidebar-chip-fill:        "#384858"
  sidebar-chip-border:      "#7888A0"
  sidebar-swatch-fallback:  "#B4C0D0"
  sidebar-badge-bg:         "#1A2238"

  # Row-rack instrument-fallback grey — used when an instrument has no
  # registered color. Slightly lighter than sidebar-swatch-fallback so
  # the row label still reads as "interactive" against the rack surface.
  row-rack-color-fallback: "#D8E0E8"

  # Grid-pane debug overlays — shown only when DEBUG_GEOM=1. Bright pure
  # primaries so the markers pop above any node/edge color.
  viz-debug-edge:     "#FFFF00"  # yellow cross at edge endpoint
  viz-debug-node:     "#FF4080"  # magenta cross at node center

  # Long-press preview pill chrome (graph deletion confirm). Two greys:
  # warm-slate fill (mirrors `surface-1`) for high-contrast against the
  # canvas, mid-grey border to read as a tappable surface.
  viz-pill-fill:   "#1A1F2C"
  viz-pill-border: "#7888A0"

  # Tap-to-confirm buttons in the long-press popup — desaturated mint/rust
  # pair, deliberately darker than the play/stop button colors so the
  # in-canvas confirm doesn't compete with the transport bar.
  viz-confirm-green: "#40A060"
  viz-cancel-red:    "#8C3030"

  # Splitter (divider line between top/bottom panes) — three desktop
  # shades plus a mobile hairline at panel-border alpha. The shadow/
  # highlight pair gives the divider a subtle 3D edge on desktop.
  divider-base:      "#A0B0C8"
  divider-shadow:    "#10141C"
  divider-highlight: "#343B4E"

  # Drum-row alternating zebra stripes inside the drum cache sprites.
  # Even rows sit slightly above the canvas background; odd rows sit
  # at-or-below it. Both are deliberately close to the surface ladder
  # but distinct from `surface-1` so the zebra reads as cell separation
  # rather than panel elevation. Lifted along with the surface ladder.
  drum-stripe-even: "#181C26"
  drum-stripe-odd:  "#13171F"

  # Drum-row hit/glow color — coral used at runtime alpha for cell hit
  # feedback. Same hex as `primary` so the playhead's transient flash
  # belongs to the single-accent family.
  drum-glow: "#FF8C5A"

  # Drum-cell off (resting) fill — slightly above the canvas `background`
  # so the cell silhouette reads against the timeline. Mirrors
  # the legacy hand-coded colStepOff, lifted with the surface ladder.
  drum-cell-off: "#13171F"

  # Drum-cell highlight flash — cream off-white used for the per-beat
  # "hit" indicator. Distinct from `on-surface` (which is also cream but
  # used for chrome) so the flash reads as a transient signal, not a text
  # color. Mirrors the legacy hand-coded colHighlight.
  drum-cell-highlight: "#FCEFD0"

  # EQ readout pill background (the dB number chip behind a band marker).
  eq-readout-bg: "#1A2438"

  # In-canvas long-press popup secondary text.
  popup-text-secondary: "#B8C8D0"

  # Scope panel dimmed label color (legend entries when their trace is
  # hidden — runtime alpha is applied separately).
  scope-label-dim: "#384450"

  # Transport bar cached background — slightly darker than `surface-1`
  # so the timeline's beat ticks read against the bar rather than the
  # rack beneath. Lifted with the surface ladder.
  transport-bar-bg: "#171B25"

  # Pure black, used at runtime alpha for modal scrims and row-dim
  # overlays. Distinct from any surface token because it's the only
  # opaque base meant to subtract light, not show through.
  dim-black: "#000000"

  # ── Phase 3 PR1: graph-pane visual primitives ────────────────────────────
  # The grid pane (top half of the canvas) was previously themed by
  # six hand-coded RGB literals in theme.go. Each subdivision tier rises
  # one notch above `background` so the lattice reads as a soft depth cue
  # rather than a flat overlay. Pure tonal — no accent hue here; the
  # graph nodes and edges carry whatever color exists in the pane.
  # grid-line is the base lattice (1 cell = subdiv·1); subsequent tiers
  # (half / quarter / eighth / sixteenth / thirty-second) are progressively
  # brighter so the prominent ruler stands out at higher subdivisions.
  grid-line:           "#1C2432"
  grid-half:           "#242C3A"
  grid-quarter:        "#2C3442"
  grid-eighth:         "#343C4A"
  grid-sixteenth:      "#3C4452"
  grid-thirty-second:  "#444C5A"

  # Graph node body — drawn as a filled circle on the lattice. Fill is a
  # neutral steel that reads against the lifted-slate background; border
  # is one notch brighter so the silhouette is legible regardless of
  # which per-row tint the connecting edges carry. The node is a
  # *chrome* surface (it doesn't carry instrument identity — that lives
  # on the edges and drum cells), so it stays inside the grayscale
  # ladder.
  node-fill:    "#282A34"
  node-border:  "#8C8E9B"

  # Connector / edge base hue. Edges are drawn at runtime alpha
  # (`alpha.edge-default`) and re-tinted per-row to the instrument
  # color; this token is the resting/default-row tint. Slightly
  # warmer than `node-border` so an unconnected node + a default
  # edge read as distinct grayscale steps.
  edge-color:   "#787A8C"

  # Splitter handle chrome — desktop pill at the divider between grid
  # pane and drum pane. Five distinct-role tokens form the rest/hover
  # ladder. `splitter-handle-mobile` is the cyan accent variant that
  # marks the splitter as a touch affordance (the only non-coral
  # accent the chrome carries; documented exception for the mobile
  # touch handle, which needs to read at a glance and is too small
  # for the 1-px coral border treatment).
  splitter-handle:           "#C8C8D2"
  splitter-handle-mobile:    "#008CC8"
  splitter-grip-line:        "#64646E"
  splitter-grip-line-hover:  "#B4B4BE"

  # Timeline strip palette — drawn behind the per-row drum cells in
  # the drum pane. Five tokens form the strip's amber accent family,
  # distinct from the chrome coral so a "current beat / view region /
  # cursor" trio reads as data-canvas chrome rather than primary
  # action. Amber here is to timeline what cyan is to spectrum bars:
  # a data-canvas color that escapes the single-coral chrome rule
  # because it speaks the canvas's visual language.
  # timeline-total-bg is darker than `background` to recess the strip.
  # timeline-view is drawn at `alpha.subtle` for the current view-window
  # tint; -hi is the full-opacity bright variant. timeline-cursor is the
  # playhead indicator. timeline-beat is the per-beat tick ruler.
  timeline-total-bg:  "#0C101C"
  timeline-view:      "#FFA040"
  timeline-view-hi:   "#FFB850"
  timeline-cursor:    "#FFC860"
  timeline-beat:      "#50525A"

  # Drum-mute cell pair — when a row is muted, the cells switch from
  # the per-row instrument color to this neutral gray family. Distinct
  # from `mute` (the mute *button* fill, deep rust) so the cell mute
  # signal reads as desaturation, not warning. `drum-mute-cell` is the
  # resting fill; `drum-mute-highlight` is the per-beat flash tint
  # (composited at `alpha.mute-highlight`).
  drum-mute-cell:        "#64646E"
  drum-mute-highlight:   "#B4B6BE"

  # Wave panel "dry" trace — drawn beneath the live trace at low alpha
  # so the comparison between wet/dry signals reads as ghosted overlay.
  # Cool desaturated blue, distinct from `viz-wave-a` (the live trace
  # is sharp cyan) so the eye separates them at a glance.
  wave-trace-dry: "#7890B0"

  # EQ zero-line ruler — the 0 dB reference line drawn across the EQ
  # canvas. A muted gray composited at `alpha.eq-zero-line` so the
  # ruler is legible without competing with the curve itself.
  eq-zero-line: "#787882"

  # Context-menu delete-row tint — warning-red at low alpha drawn
  # behind a destructive menu group. Distinct from `error` (text),
  # `mute` (button fill), `destructive` (button fill), and
  # `destructive-border` (button border): this is a *menu container
  # background tint*, not any of those four roles. Resolves the
  # in-code "until token added" TODO in theme.go.
  menu-delete-tint: "#DC3232"

  # Stepper amber-on-desktop icon tint — used by the +/- icon glyph
  # in BPM/Length steppers on desktop where the glyph rides above a
  # navy `stepper-fill`. Warmer than `timeline-view` so the +/-
  # affordance pops against its container.
  incdec-icon-hi: "#FFB860"

# Profile-conditional design overrides — see "Bridge to Go" prose for the
# split rule: dimensional + visual-style fields go here, runtime feature
# toggles (DirectDrawRows, EnableLayoutResize, ShowEscHint, UseBottomSheet,
# etc.) stay in Go (`internal/ui/runtime_profile.go`). The generator emits
# `design_profile.gen.go` containing one `profileValues` struct per profile;
# `layout_profile.go`'s `desktopProfile()` / `mobileProfile()` consume those
# values for the encoded fields, leaving the remaining ones hand-coded
# until the next migration sweep.
profileOverrides:
  desktop:
    rowHeight:             28
    grabZone:              5
    minTarget:             0
    minCellWidth:          2
    popupPanelW:           220
    popupBtnW:             18
    popupBtnH:             16
    popupGap:              4
    popupPad:              6
    transportBtnSize:      32
    rowControlBtnSize:     0
    splitterHandleLen:     50
    splitterHandleThk:     8
    headerMinH:            40
    headerMaxH:            40
    timelineBarH:          12
    eqSliderH:             14
    eqHandleRadius:        16
    eqDBInputH:            14
    sliderTrackH:          4
    sliderThumbH:          18
    sliderThumbW:          12
    nodeMinPx:             8
    nodeMaxPx:             16
    edgeThickMul:          1
    controlPadding:        4
    controlGap:            2
    controlGroupPad:       3
    controlLeftInset:      12
    accentStripeWidth:     3
    accentStripeInsetY:    0
    popupCornerRadius:     12
    closeButtonSize:       16
    defaultTimelineBeats:  0
    splitterGrabThreshold: 0
    popupSectionGap:       4
    fxToggleTrackW:        32
    fxToggleTrackH:        18
    fxToggleThumbD:        14
  mobile:
    rowHeight:             44
    grabZone:              16
    minTarget:             44
    minCellWidth:          2
    popupPanelW:           300
    popupBtnW:             44
    popupBtnH:             36
    popupGap:              6
    popupPad:              10
    transportBtnSize:      44
    rowControlBtnSize:     36
    splitterHandleLen:     56
    splitterHandleThk:     6
    headerMinH:            56
    headerMaxH:            56
    timelineBarH:          14
    eqSliderH:             28
    eqHandleRadius:        26
    eqDBInputH:            20
    sliderTrackH:          6
    sliderThumbH:          24
    sliderThumbW:          14
    nodeMinPx:             12
    nodeMaxPx:             20
    edgeThickMul:          2
    controlPadding:        3
    controlGap:            4
    controlGroupPad:       4
    controlLeftInset:      4
    accentStripeWidth:     5
    accentStripeInsetY:    2
    popupCornerRadius:     16
    closeButtonSize:       28
    defaultTimelineBeats:  8
    splitterGrabThreshold: 8
    popupSectionGap:       8
    fxToggleTrackW:        40
    fxToggleTrackH:        22
    fxToggleThumbD:        18
typography:
  panel-title:
    fontFamily: Ebiten Debug
    fontSize: 21px
    fontWeight: 400
    lineHeight: 1.2
  section-header:
    fontFamily: Ebiten Debug
    fontSize: 16px
    fontWeight: 400
    lineHeight: 1.4
  body:
    fontFamily: Ebiten Debug
    fontSize: 16px
    fontWeight: 400
    lineHeight: 1.4
  button-label:
    fontFamily: Inter SemiBold
    fontSize: 16px
    fontWeight: 600
    lineHeight: 1.0
  caption:
    fontFamily: Ebiten Debug
    fontSize: 16px
    fontWeight: 400
    lineHeight: 1.4
  tooltip:
    fontFamily: Ebiten Debug
    fontSize: 13px
    fontWeight: 400
    lineHeight: 1.2
spacing:
  xs: 3px
  sm: 6px
  md: 10px
  lg: 14px
  xl: 20px
  xxl: 28px
  cluster: 12px
  btn-sm: 28px
  btn-md: 36px
  btn-lg: 44px
  touch-min: 44px
  icon-sm: 16px
  icon-md: 20px
  icon-lg: 24px
rounded:
  sm: 8px
  md: 12px
  lg: 16px
  xl: 20px
  full: 999px
# Icon system — every IconID in the unified icon set draws onto a 24×24
# logical-unit canvas. `stroke` is a fractional unit (Lucide-style 1.75)
# converted to pixels at render time as `stroke * (renderSize / grid)`.
# Padding is the empty band inside the canvas (no ink); radius is the
# default corner radius for rectangular elements (rounded rect bodies,
# pause bars, save chassis).
icon:
  grid: 24
  padding: 2
  radius: 2
  stroke: 1.75
# Alpha buckets — Stitch only allows opaque #RRGGBB so opacity escapes the
# token system; this maps the escape hatch onto a small named set.
#
# Two parallel ladders, kept distinct because they answer different
# questions and have different end users:
#   - General-purpose buckets (faint/subtle/medium/strong/overlay): used
#     with `WithAlpha(token, AlphaSubtle)` for *fills*, scrims, hover
#     overlays, focus rings — the runtime composes a translucent color
#     from a base token + bucket alpha. Values were chosen for visual
#     contrast at typical UI scale.
#   - Border buckets (border-thin/default/emphasis/panel): used in the
#     `border` field of `components:` entries. Borders sit at very low
#     alpha (8–25) so the white-base outline reads as a hairline
#     against dark surfaces; they do NOT belong on the same ladder as
#     `subtle`/`medium`/`strong` (which are 60/90/180 — visible scrims,
#     not outlines). Naming uses border-* prefix to make the role
#     unambiguous at every call site.
alpha:
  faint:           25
  subtle:          60
  medium:          90
  strong:          180
  overlay:         220
  border-thin:     8   # section dividers, secondary-button outline
  border-default:  15  # standard control border
  border-emphasis: 25  # focused input border
  border-panel:    20  # floating panel chrome
  # Per-component one-offs (Phase 2 PR4). Each is used by exactly one
  # component recipe; consolidating them here keeps every alpha decision
  # in one auditable place.
  accent-tint:      50  # button-dropdown border (primary at 50)
  accent-overlay:   80  # button-transport-follow-on border
  white-decoration: 30  # button-primary (FAB) border, spectrum dB grid
  scrollbar-thumb:        40  # default + dropdown scrollbar thumb
  scrollbar-thumb-mobile: 50  # mobile scrollbar thumb (taller, brighter)
  sidebar-section:        100 # sidebar section/separator background dim
  sidebar-chip:           200 # sidebar inline chip fill/badge bg
  row-rack-zebra:         10  # row-rack zebra stripe overlay
  row-rack-separator:     16  # row-rack separator + label divider
  row-rack-dim:           6   # row-rack label background tint
  transport-separator:    12  # transport bar inter-group divider
  scope-label-dim:        120 # scope legend dim alpha (text on canvas)
  row-active:             153 # row-rack instrument-color overlay when row is active (~60%)
  # ── Phase 3 PR1: graph-pane / timeline / drum-mute alpha buckets ──
  # Per-role buckets for compositions previously expressed as inline
  # alpha integers in theme.go. Same value can appear under multiple
  # role names — the convention (per the existing `accent-overlay`
  # vs `sidebar-section` precedent) is that role-naming matters more
  # than value-deduplication so future audits can trace each
  # composition back to its caller.
  edge-default:           100 # graph-pane connector default alpha
  wave-trace-dry:         100 # Wave panel ghosted reference trace
  eq-zero-line:            80 # EQ canvas 0 dB ruler line
  eq-curve-fill:           20 # EQ curve fill below the stroke
  mute-highlight:         160 # drum-mute per-beat flash composite alpha
  scrim:                  150 # modal/menu backdrop scrim (over `dim-black`)
  splitter-grip-hover:    200 # splitter grip line on hover
  splitter-hover:         240 # splitter handle on hover (just below opaque)
  panel-near-opaque:      250 # floating panel BG (just below opaque)
  beat-group-alt:           4 # drum beat-group alternating tint (white over surface)

# Animation primitives — Phase 3 PR3 (Beatmo extension; not part of the
# canonical Stitch schema).
#
# Each entry names an animation that the chrome runtime previously
# expressed as a magic number scattered across draw call sites. Editing
# this block and re-running `make gen-design-tokens` retunes the
# entire game's animation cadence in one place.
#
# Three closed `kind`s:
#   - `exp-decay`: per-frame retention multiplier (`rate`, 0..1)
#     followed by an exit threshold (`threshold`, 0..1). Used by the
#     node-highlight decay loop in game_highlight_state.go.
#   - `sin-pulse`: oscillator built as `base + amplitude·|sin(frame·step)|`.
#     `alpha-scale` (uint8, 0..255) is the optional ceiling when the
#     output drives an opacity channel; otherwise omit.
#   - `fade`: a single 0..1 multiplier applied via fadeColor() — covers
#     all the `fadeColor(c, X)` call sites that previously hardcoded X.
#
# Adding a new animation: pick one of the three kinds, add an entry,
# add a runtime accessor in animation_helpers.go.
geometry:
  # Signal-pulse outer glow radius as a multiple of the inner core
  # radius. Used by SignalStyle.Draw at components.go:61.
  signal-glow-radius-multiplier: 1.5
  # Edge arrowhead size as a fraction of one beat-step in world space.
  # Used by Grid.EdgeArrowSize() at grid.go:171.
  edge-arrow-step-fraction: 0.15
  # Border stroke widths (pixels) for graph nodes and highlight outlines.
  # node-border is drawn around every node body; highlight-border is the
  # thicker 4-line ring drawn around selected/neighbour nodes.
  node-border-thickness: 1
  highlight-border-thickness: 2
  # Graph node and travelling-signal core radii (pixels). These are the
  # theme-level defaults; profile-dependent min/max for the node radius
  # live under `profileOverrides.node-min-px` / `node-max-px`.
  # Used by NodeUI / SignalUI initialisers in theme.go:155-156.
  node-radius: 16
  signal-radius: 6

animations:
  highlight-decay:
    kind: exp-decay
    rate: 0.8
    threshold: 0.02
  playhead-pulse:
    kind: sin-pulse
    frame-step: 0.05
    amplitude: 0.6
    base: 0.4
    alpha-scale: 120
  signal-glow-outer:
    kind: fade
    factor: 0.3
  node-trigger-glow:
    kind: fade
    factor: 0.35
  playhead-column-fade:
    kind: fade
    factor: 0.85
  edge-faded:
    kind: fade
    factor: 0.6
  highlight-faded:
    kind: fade
    factor: 0.5

components:
  # Primary FAB (mobile "+", primary CTA) — icon-only at runtime; the
  # foreground is the dark background color rendered against the magenta
  # accent for ~12:1 contrast (matches the solo-active button pattern).
  button-primary:
    category: button
    backgroundColor: "{colors.primary}"
    textColor: "{colors.background}"
    border:
      color: "{colors.border}"
      alpha: white-decoration
    rounded: "{rounded.md}"
    height: "{spacing.btn-lg}"

  # Secondary transport (Play/Stop/Record/BPM stepper/follow/upload/import)
  button-secondary:
    category: button
    backgroundColor: "{colors.surface-2}"
    textColor: "{colors.on-surface}"
    iconColor: "{colors.on-surface-muted}"
    border:
      color: "{colors.border}"
      alpha: border-thin
    rounded: "{rounded.md}"
    height: "{spacing.btn-md}"
    topEdgeHighlight: true
    interaction:
      hover: { fillDelta: 12, borderDelta: 20 }
      press: { fillDelta: -20, highlightDelta: 10 }
  button-secondary-active:
    category: button
    backgroundColor: "{colors.surface-3}"

  # Row-rack control (M / S / FX / O / X — text-label chips)
  button-row-control:
    category: button
    backgroundColor: "{colors.surface-2}"
    textColor: "{colors.on-surface}"
    border:
      color: "{colors.border}"
      alpha: border-thin
    rounded: "{rounded.sm}"
    height: "{spacing.btn-md}"
    typography: "{typography.button-label}"
    interaction:
      hover: { fillDelta: 12, borderDelta: 20 }
      press: { fillDelta: -20, highlightDelta: 10 }
  # Active variants are full recolors with opaque colored borders (no
  # alpha bucket); the border is its own RGB token, not a white-base
  # composite. Omitting `border.alpha` selects opaque (255).
  button-row-control-mute-active:
    category: button
    backgroundColor: "{colors.mute}"
    textColor: "{colors.on-surface}"
    border:
      color: "{colors.mute-active-border}"
    rounded: "{rounded.sm}"
    height: "{spacing.btn-md}"
  button-row-control-solo-active:
    category: button
    backgroundColor: "{colors.primary-bright}"
    textColor: "{colors.background}"
    border:
      color: "{colors.solo-active-border}"
    rounded: "{rounded.sm}"
    height: "{spacing.btn-md}"
  button-row-control-fx-active:
    category: button
    backgroundColor: "{colors.fx-active-fill}"
    textColor: "{colors.primary}"
    border:
      color: "{colors.primary}"
    rounded: "{rounded.sm}"
    height: "{spacing.btn-md}"

  # Destructive (delete button) — runtime renders a destructive-fill body
  # with a destructive-border outline; text is on-surface (default body
  # text), giving ~8:1 contrast. The "confirm" visual on second click is
  # a brightened-fill hover effect produced at draw time, not a separate
  # token.
  button-destructive:
    category: button
    backgroundColor: "{colors.destructive}"
    textColor: "{colors.on-surface}"
    border:
      color: "{colors.destructive-border}"
    rounded: "{rounded.sm}"
    height: "{spacing.btn-md}"
  # Confirm visual on second click — brighter fill + bright border.
  button-destructive-confirm:
    category: button
    backgroundColor: "{colors.destructive-confirm-fill}"
    textColor: "{colors.on-surface}"
    border:
      color: "{colors.destructive-confirm-border}"
    rounded: "{rounded.sm}"
    height: "{spacing.btn-md}"
  # `colors.destructive-border` is consumed by `button-destructive`'s
  # extended `border: { color }` block. Stitch's narrow component schema
  # doesn't recognize the extension as a token use, so the lint snapshot
  # carries `colors.destructive-border` as an explicit orphan allowance.

  # Disabled — fill is `disabled-fill` (slightly above on-surface-disabled
  # so the button still has a body silhouette); border is the standard
  # white@thin hairline so it doesn't telegraph "interactive".
  button-disabled:
    category: button
    backgroundColor: "{colors.disabled-fill}"
    textColor: "{colors.on-surface-muted}"
    border:
      color: "{colors.border}"
      alpha: border-thin
    rounded: "{rounded.sm}"

  # Floating panel (inspector / FX / context menu container)
  panel:
    backgroundColor: "{colors.surface-overlay}"
    textColor: "{colors.on-surface}"
    rounded: "{rounded.md}"
  bottom-sheet:
    backgroundColor: "{colors.surface-overlay}"
    textColor: "{colors.on-surface}"
    rounded: "{rounded.xl}"

  # Context menu items — destructive items share the panel background and
  # only color the text. This is intentional: the menu is a single
  # surface-overlay, and the destructive intent is signalled by red text,
  # not a red row.
  context-menu-item:
    backgroundColor: "{colors.surface-overlay}"
    textColor: "{colors.on-surface}"
    height: "{spacing.btn-md}"
  context-menu-item-destructive:
    backgroundColor: "{colors.surface-overlay}"
    textColor: "{colors.error}"

  # Record button (icon-only; semantic state colors)
  button-record-idle:
    backgroundColor: "{colors.surface-2}"
    textColor: "{colors.record-idle}"
    rounded: "{rounded.md}"
    height: "{spacing.btn-md}"
  button-record-active:
    backgroundColor: "{colors.surface-2}"
    textColor: "{colors.record-active}"
    rounded: "{rounded.md}"
    height: "{spacing.btn-md}"

  # Play button while playing (mobile uses success-tinted icon)
  button-play-active:
    backgroundColor: "{colors.surface-2}"
    textColor: "{colors.success}"
    rounded: "{rounded.md}"
    height: "{spacing.btn-md}"

  # Section header text (uses on-surface-accent)
  section-header-active:
    textColor: "{colors.on-surface-accent}"
    typography: "{typography.section-header}"

  # Splitter handle (mobile uses primary-dim magenta)
  splitter-handle-mobile:
    backgroundColor: "{colors.primary-dim}"
    rounded: "{rounded.sm}"

  # Row rack container (surface-1 is the row-list backdrop)
  row-rack:
    backgroundColor: "{colors.surface-1}"

  # Visualization canvas (Wave / Spectrum / Meters / Scope / EQ)
  viz-canvas:
    backgroundColor: "{colors.viz-bg}"
    rounded: "{rounded.sm}"
  viz-spectrum-bar:
    backgroundColor: "{colors.viz-bar}"
  viz-spectrum-bar-peak:
    backgroundColor: "{colors.viz-bar-peak}"
  viz-eq-curve:
    textColor: "{colors.viz-curve}"
  viz-wave-trace-a:
    textColor: "{colors.viz-wave-a}"
  viz-wave-trace-b:
    textColor: "{colors.viz-wave-b}"

  # Inputs (BPM box, dB box) — reference border base; alpha applied at draw time
  input-field:
    category: input
    backgroundColor: "{colors.surface-2}"
    textColor: "{colors.on-surface}"
    border:
      color: "{colors.border}"
      alpha: border-default
    rounded: "{rounded.sm}"
    interaction:
      focus: { fillDelta: 30, borderDelta: 80 }
  input-field-focused:
    category: input
    backgroundColor: "{colors.surface-2}"
    textColor: "{colors.on-surface}"
    border:
      color: "{colors.border}"
      alpha: border-emphasis

  # ── Phase 2 PR4: brand-new components onboarding remaining hand-coded styles ──

  # Numeric stepper (BPM ±, Length ±) — distinct navy fill so the stepper
  # group reads as a secondary control cluster, not a primary action.
  button-stepper:
    category: button
    backgroundColor: "{colors.stepper-fill}"
    textColor: "{colors.on-surface}"
    border:
      color: "{colors.stepper-border}"
    rounded: "{rounded.md}"
    height: "{spacing.btn-md}"

  # Desktop Play / Stop — slightly richer than the mobile success/error
  # tints; these get full button bodies (fill + border) on desktop.
  button-play-desktop:
    category: button
    backgroundColor: "{colors.play-desktop-fill}"
    textColor: "{colors.background}"
    border:
      color: "{colors.play-desktop-border}"
    rounded: "{rounded.md}"
    height: "{spacing.btn-md}"
  button-stop-desktop:
    category: button
    backgroundColor: "{colors.stop-desktop-fill}"
    textColor: "{colors.on-surface}"
    border:
      color: "{colors.stop-desktop-border}"
    rounded: "{rounded.md}"
    height: "{spacing.btn-md}"

  # Dropdown control — secondary surface with a primary-tinted border at
  # alpha 50. The magenta tint signals "this opens a list".
  button-dropdown:
    category: button
    backgroundColor: "{colors.surface-2}"
    textColor: "{colors.on-surface}"
    border:
      color: "{colors.primary}"
      alpha: accent-tint
    rounded: "{rounded.md}"
    height: "{spacing.btn-md}"

  # Missing-instrument row label — error-fill body to telegraph "this row
  # references an instrument id that is not loaded".
  button-missing-instrument:
    category: button
    backgroundColor: "{colors.error-text}"
    textColor: "{colors.on-surface}"
    border:
      color: "{colors.border}"
      alpha: border-default
    rounded: "{rounded.sm}"

  # EQ band-mute chip (resting + active). Resting uses surface-3 to sit
  # one level above the EQ panel; active swaps to a saturated red but
  # keeps the same border so the chip silhouette is stable.
  button-eq-mute:
    category: button
    backgroundColor: "{colors.surface-3}"
    textColor: "{colors.on-surface}"
    border:
      color: "{colors.border}"
      alpha: border-default
    rounded: "{rounded.sm}"
    height: "{spacing.btn-md}"
  button-eq-mute-active:
    category: button
    backgroundColor: "{colors.eq-mute-active-fill}"
    textColor: "{colors.on-surface}"
    border:
      color: "{colors.border}"
      alpha: border-default
    rounded: "{rounded.sm}"
    height: "{spacing.btn-md}"

  # EQ filter (HPF / LPF) toggle — active state is coral; uses inverted
  # background-on-coral text (same pattern as button-primary FAB) so
  # WCAG AA contrast holds against the coral fill. The two-letter "HP"
  # / "LP" labels read crisply as dark glyphs on the coral chiclet.
  button-eq-filter-active:
    category: button
    backgroundColor: "{colors.eq-filter-active-fill}"
    textColor: "{colors.background}"
    border:
      color: "{colors.eq-filter-active-border}"
    rounded: "{rounded.sm}"
    height: "{spacing.btn-md}"

  # Transport "follow-on" — locked-scroll indicator. Custom blue fill +
  # primary-tinted border at alpha 80 (visible but not loud).
  button-transport-follow-on:
    category: button
    backgroundColor: "{colors.transport-follow-on-fill}"
    textColor: "{colors.on-surface}"
    border:
      color: "{colors.transport-follow-on-border-base}"
      alpha: accent-overlay
    rounded: "{rounded.md}"
    height: "{spacing.btn-md}"

  # Mobile row label — surface-1 body so the label sits visually below the
  # rack controls; thin hairline border.
  button-row-label-mobile:
    category: button
    backgroundColor: "{colors.surface-1}"
    textColor: "{colors.on-surface}"
    border:
      color: "{colors.border}"
      alpha: border-thin
    rounded: "{rounded.sm}"
    height: "{spacing.btn-md}"

  # `colors.border` is consumed by every component's extended
  # `border: { color }` block (rendered at runtime as the base white
  # composited under named alpha buckets). Stitch's narrow component
  # schema doesn't see the extension as a use, so the lint snapshot
  # carries `colors.border` as an explicit orphan allowance.
# Per-built-in-instrument default colors. Unlike `instrumentSwatches:`
# (a *recommendation* surface for the picker), this block IS consumed by
# the chrome runtime: when a row references one of these instrument IDs
# and the user hasn't picked a custom color, the row inherits this hex
# as its drum-cell tint, edge tint, and rack swatch. Editing this list
# changes every default-coloured row in the app at the next build.
#
# Distinct from `colors:` because these are not chrome accents — they
# are *data identity*: each instrument ID gets one stable hex. New
# built-in instruments must add an entry here; otherwise the row
# falls back to `instrumentFallbackPalette:` cycling.
instrumentDefaults:
  - id: kick
    color: "#32AA64"
  - id: snare
    color: "#C34850"
  - id: hihat
    color: "#C8A82D"
  - id: tom
    color: "#4464C3"
  - id: clap
    color: "#B944A8"

# Fallback palette for any instrument ID not in `instrumentDefaults:`.
# Cycled in source order, with state preserved at runtime so two unknown
# instruments don't collapse onto the same color on the same canvas.
# Five entries chosen to extend the `instrumentDefaults` family without
# overlapping hues; if a sixth or seventh instrument lands, add an
# entry here rather than minting a brand-new chrome color token.
instrumentFallbackPalette:
  - color: "#44A8A8"
  - color: "#AA6E44"
  - color: "#6E44A8"
  - color: "#AA446E"
  - color: "#44A86E"

# Curated suggestion swatches for the per-row instrument color picker.
# Twelve harmonized pastel-leaning hues designed to read against the
# lifted slate ladder without bleed-through.
#
# IMPORTANT: this block is a *recommendation surface*, not a constraint.
# The color-picker (`drumview_color_menu.go`) renders these as suggested
# swatches above the free hue wheel; users can still pick arbitrary hex.
# The chrome runtime ignores this block entirely. Instrument identity is
# the ONLY multi-color signal in the app — see the "Instrument identity
# is the only multi-color signal" invariant. This is where the "fun"
# lives; the chrome stays restrained.
#
# `coral` shares the hex of `colors.primary` and acts as the sentinel
# default when no instrument color is set.
#
# Stitch lint flags this as an orphan top-level key (it isn't part of
# the canonical Stitch schema); the warning is absorbed in
# `testdata/design_md_lint.expected.json` with rationale.
instrumentSwatches:
  - name: coral
    hex: "#FF8C5A"
  - name: peach
    hex: "#FFB484"
  - name: marigold
    hex: "#FFD56A"
  - name: lime
    hex: "#A8E07A"
  - name: mint
    hex: "#7AE0B8"
  - name: aqua
    hex: "#7ADCE0"
  - name: sky
    hex: "#84B8FF"
  - name: lavender
    hex: "#A89AFF"
  - name: orchid
    hex: "#D89AFF"
  - name: rose
    hex: "#FF9AC8"
  - name: sand
    hex: "#D4C29A"
  - name: slate
    hex: "#9AB0C8"
---

# Beatmo UI Design System

> Authoritative design reference for the Beatmo UI. Any model or
> engineer touching UI code must follow these rules. The YAML front matter
> above is normative; the prose below explains *why* values are what they are
> and how to apply them. Deviations require updating this document first.
>
> The Go side (`src/go/internal/ui/theme.go`, `theme_tokens.go`,
> `ui_style.go`, `touch_sizes.go`) is the runtime expression of these tokens.
> Inline `color.RGBA{…}` / numeric literals in `internal/ui/` that bypass the
> token system are bugs.

## Overview

Beatmo is a graph-driven grid sequencer. The UI is dense by nature — a node
graph on top, a multi-row drum timeline in the middle, an EQ/scope/spectrum
panel at the bottom, and a per-instrument FX rack overlaid on demand. The
design philosophy is **minimalism backed by semantic affordances**, in this
priority order:

1. **Prefer color, typography, and spatial cues over icons.** The instrument
   selector menu (`drumview_instrument_menu.go`) is the canonical example:
   each instrument is a colored swatch + a text label. No glyph. The color
   carries identity, the text carries the name, the spatial grouping carries
   the category. This is the gold standard for in-panel lists.
2. **Use icons only when the action is glyph-recognizable** at the rendered
   size. Recognizable in this codebase: Play, Pause, Stop, Record, Trash,
   Close, Plus, Minus, Mute, Solo, Pencil, Save, chevrons (Up/Down/Right),
   Speaker on/off, Track on/off, Note, Circle (color swatch proxy), Audio
   (EQ bars), Rows. When in doubt, prefer a one-letter text label or a
   colored chip — they survive scaling and theming better than vector glyphs.
3. **Minimum render sizes.** Vector icons via `drawIconByID` are
   pixel-stroke primitives without sub-pixel AA; below ~28 px they look
   jagged. Hard floor: `spacing.btn-sm` (28 px) desktop, `spacing.touch-min`
   (44 px) mobile. Below those, render a text label or a color-coded chip —
   never a smaller icon.
4. **No raw Unicode glyphs in chrome.** Every meaningful glyph in chrome must
   be either an `IconID` or one of the explicit single-character
   text-label exceptions enumerated under "Do's and Don'ts → Permitted
   Text-Glyph Exceptions". A raw `▶`, `▼`, `▲`, `✕`, `≡`, etc. anywhere in
   `internal/ui/` is a bug.
5. **Stateful indicators may use single-character text** (e.g., `||` for
   freeze, `>` for resume) where the icon set has no entry and minting one
   would add visual noise to a small, non-primary control. Such uses must be
   enumerated in the exceptions table. Color the glyph with `{colors.on-surface-muted}`
   (inactive) → `{colors.primary}` (active) so the state is legible without
   inventing a new icon.

The two surfaces are a **Grid Pane** (top — node graph + edges + arrows,
camera pan/zoom) and a **Drum Pane** (bottom — timeline editor with rows,
transport, BPM/length/subdiv controls, per-row instrument selector, mute /
solo / FX, volume, color picker).

## Colors

A four-level lifted-warm-slate surface hierarchy with a single coral
accent and three distinct red roles. Every color is a token; never use a
hex literal in code.

### Surface hierarchy

The base palette nests in order of elevation. Surface-1 contains surface-2
buttons; never place a lower-level surface inside a higher-level container.
The ladder is lifted ~10% above pure-dark navy so chrome reads as
touchable matte plastic, not aerospace black.

- **`background` (`#10141C`)** — Grid pane, timeline canvas (deepest level).
- **`surface-1` (`#1A1F2C`)** — Row rack, transport bar background.
- **`surface-2` (`#262C3C`)** — Cards, buttons, input fields (elevated).
- **`surface-3` (`#343B4E`)** — Hover states, active surfaces (raised).
- **`surface-overlay` (`#1F2533`)** — Overlay panels, popups, bottom sheets.
  Drawn at 250/255 alpha against the scrim — opacity is applied in code by
  `drawPanel(...)`, not encoded in the token (the spec only allows opaque
  colors).

### Accent (coral — single chrome accent)

The chrome carries exactly one accent. Every "active / selected / focused
/ hover-emphasis" state in chrome resolves to one of these three values.
No second accent (no parallel cyan, no panel-state-dependent recolor) is
allowed in chrome — see "Single chrome accent" invariant below. Cyan is
reserved for `viz-bar` / `viz-wave-*` *data series*, never chrome.

- **`primary` (`#FF8C5A`)** — Primary interactive / active state.
- **`primary-bright` (`#FFB498`)** — Hover / focus ring; never decorative.
- **`primary-dim` (`#C56448`)** — Splitter handle, slider fill, HP/LP
  active fill, secondary emphasis.
- A "subtle" variant of `primary` (8% alpha) is used for backgrounds and
  fills — applied at draw time as `withAlpha(TokenAccent(), 20)`. There is
  no separate token for it.

### On-surface text

- **`on-surface` (`#FCEFD0`)** — Main labels, button text.
- **`on-surface-muted` (`#A0B0C8`)** — Captions, secondary labels, inactive
  icons.
- **`on-surface-disabled` (`#5A6470`)** — Grayed-out, non-interactive.
- **`on-surface-accent` (`#FF8C5A`)** — Active section headers, links. Same
  hex as `primary`; the role is distinct (text vs. interactive surface).

### Borders

A single base color (`border: "#FFFFFF"`) is used at three opacity levels.
Opacity is applied in code, not in tokens (Stitch spec only allows
`"#RRGGBB"`):

- **subtle** — white at 8/255 alpha. Section dividers.
- **medium** — white at 15/255 alpha. Default control borders.
- **strong** — white at 25/255 alpha. Focused input borders.
- **panel border** — white at 20/255 alpha. Overlay panel chrome.

Go bindings: `colBorderSubtle / colBorderMedium / colBorderStrong /
colPanelBorder` in `theme.go`.

### Semantic state — three reds, three distinct roles

This is the most error-prone part of the palette. Memorize:

- **`error` (`#FF5848`)** — Stop / error **text** only. Stop button icon
  tint, error toasts, parity warnings. Never a button fill.
- **`mute` (`#A04830`)** — Mute button **fill** only. Slightly darker so it
  reads as a state badge, not an alert.
- **`destructive` (`#481414`)** — Destructive button **fill** only (delete
  row, "Delete" context menu item). Paired with **`destructive-border`
  (`#A03830`)**.

Never reuse a red variant for a different semantic role. Do not introduce a
fourth red.

The two greens (`success` for play state, plus a slightly richer
`colPlayButton = #40A060`  used as the desktop play-button fill) follow the
same logic: one for icon tint, one for fill.

### Single chrome accent — three load-bearing invariants

These three invariants travel together. A change that violates one tends
to violate the others. Memorize them; the visual coherence of the whole
app collapses if any of them slips.

1. **Single chrome accent.** Every "active / selected / focused /
   hover-emphasis" state in chrome resolves to `primary` (`#FF8C5A`),
   `primary-bright` (`#FFB498`), or `primary-dim` (`#C56448`). No second
   accent is allowed in chrome — no parallel cyan, no parallel amber, no
   panel-state-dependent recolor. The cyan tokens (`viz-bar`,
   `viz-bar-peak`, `viz-wave-a`, `viz-wave-b`) belong **only** to data
   visualizations (spectrum bars, wave traces). If a token outside the
   `viz-*` family uses cyan, that is a bug. If a chrome control's
   selected state uses anything other than the three coral tokens above,
   that is a bug.

2. **Panel state never recolors chrome.** Opening the FX panel must not
   change the EQ panel's accent. Opening the inspector sidebar must not
   change the row-rack chip color. If a token's color depends on which
   panel is open, it is the wrong token — the panel-open code path
   should toggle a *border alpha* or a *fill brightness*, never swap
   the underlying hex.

3. **Instrument identity is the only multi-color signal.** Drum cells,
   graph nodes, edge tints, and the per-row instrument color swatch
   carry the row's identity (free-hex per instrument; the
   `instrumentSwatches` block names a curated suggestion set). No other
   UI surface uses the instrument color set. Chrome accents (coral),
   visualization tokens (cyan/lime/peak markers), and semantic state
   (red/green) are each their own closed family; instrument color sits
   outside all of them.

### Record states

- **`record-idle` (`#E04040`)** — Record button at rest.
- **`record-active` (`#FF4040`)** — Record button while recording. Both
  drawn at 80% alpha in code for a subtle pulse.

### Visualization tokens

The Wave / Spectrum / Meters / Scope / EQ canvases share these:

- **`viz-bg` (`#13171F`)** — Canvas background. Sits on top of
  `surface-overlay`.
- **`viz-curve` (`#FF8C5A`)** — EQ curve stroke (= `primary`); plus 8% fill
  below to a derived `viz-curve-fill` applied in code.
- **`viz-bar` (`#5AC8E0`)**, **`viz-bar-peak` (`#A0E0FF`)** — Spectrum bars
  rise from canvas baseline; peak hold in pale cyan.
- **`viz-wave-a` (`#80E0FF`)** — Scope tap A trace, Wave primary trace.
- **`viz-wave-b` (`#7A8AA8`)** — Scope tap B trace; split / diff secondary.

When adding a new visualization, prefer reusing the existing tokens.
Introduce a new token only if the role is genuinely distinct.

## Typography

All chrome text uses the **bundled Ebiten debug font** (6×16 px cell per
glyph) scaled via `TextScale`. Row-rack control labels (M / S / FX / O / X)
use **Inter SemiBold** rendered through `text/v2` because the debug font
looks jagged at single-character chip sizes.

Six typography roles, each a token:

- **`panel-title`** (1.3× of body, 21 px) — Inspector / FX panel header.
- **`section-header`** (1.0×, 16 px) — Collapsible section labels.
- **`body`** (1.0×, 16 px) — Default labels.
- **`button-label`** (1.0×, 16 px, Inter SemiBold) — Row controls, transport
  text labels.
- **`caption`** (1.0×, 16 px) — BPM number, percentage values.
- **`tooltip`** (0.8×, 13 px) — Cursor labels, coordinates (desktop only).

Derive all sizes from `TextHeight()` / `TextWidth()` so the font can be
swapped later without hunting hardcoded constants.

## Layout

### Two-pane composition

The canvas is vertically split:

- **Grid Pane (top)** — Node graph visualization, edges with arrows, camera
  pan/zoom. Layer 0 (`background`).
- **Drum Pane (bottom)** — Timeline editor + transport + EQ panel. Layer 1
  (`surface-1`).

Resizable via a horizontal splitter. Mobile collapses the pane to one of the
two via tabs.

### Spacing scale

Use these tokens — never magic numbers. Defined in
`src/go/internal/ui/touch_sizes.go`. The whole scale is bumped ~25% over
the prior tactical-density baseline so cramped clusters get room to
breathe; the new `cluster` token names the inter-cluster gap explicitly
so the transport bar's Play/Stop/Record vs. BPM steppers vs. ÷n vs.
follow-on groups can be space-separated by a single token reach.

| Token | Value | Usage |
|---|---|---|
| `spacing.xs` | 3 px | Hairline insets |
| `spacing.sm` | 6 px | Tight gaps (button padding, icon inset) |
| `spacing.md` | 10 px | Normal gaps (section spacing) |
| `spacing.lg` | 14 px | Loose gaps (panel padding) |
| `spacing.xl` | 20 px | Extra space (between panels) |
| `spacing.xxl` | 28 px | Section breathing room |
| `spacing.cluster` | 12 px | Inter-cluster gap on the transport bar (between Play/Stop/Record group, BPM steppers, ÷n, follow-on); replaces hardcoded magic numbers between control groups |

### Button heights

| Token | Value | Usage |
|---|---|---|
| `spacing.btn-sm` | 28 px | Small buttons (close, stepper) |
| `spacing.btn-md` | 36 px | Medium buttons (row controls, transport) |
| `spacing.btn-lg` | 44 px | Large buttons (FAB) |
| `spacing.touch-min` | 44 px | Minimum hit target on mobile |

### Icon sizes

| Token | Value | Usage |
|---|---|---|
| `spacing.icon-sm` | 16 px | In-row leading icons (context menu) |
| `spacing.icon-md` | 20 px | Standard icon button glyph |
| `spacing.icon-lg` | 24 px | Transport icons |

Note: the Stitch spec puts `spacing` and `rounded` in separate top-level
groups. Button heights and icon sizes are dimensional spacing per spec, so
they live under `spacing` with `btn-` and `icon-` prefixes.

## Elevation & Depth

Beatmo uses **cushioned tonal layers + thin borders + a single accent-glow
on hover (desktop)**. No drop shadows on solid surfaces; panel overlays
use a subtle shadow + panel-border. The mental model is "soft physical
pads on a warm slate desk", not "stack of frosted-glass cards".

### Surface elevation cues

The four-level surface hierarchy in "Colors" *is* the elevation system. A
button on `surface-1` rises by sitting on `surface-2`; hovering lifts it
to `surface-3`. There is no shadow between surface levels — elevation is
purely tonal.

### Cushioned elevation (desktop chrome)

Desktop chrome reads as soft physical pads through three stacked cues
applied per-control. Mobile chrome stays flat (no top-edge highlight, no
hover glow); it gets generous rounding instead.

1. **Tonal lift** — surface-2 → surface-3 on hover (already covered
   above).
2. **Top-edge highlight** — a 1-px highlight at
   `WithAlpha(on-surface, AlphaSubtle)` sits across the top of every
   secondary button (`topEdgeHighlight: true` in the component spec).
   Reads as a soft specular catch on a curved pad.
3. **Hover glow** *(naming reserved; runtime work is a follow-up)* — a
   2-px outer glow at `WithAlpha(primary, AlphaSubtle)` rendered as an
   inset radial around the hovered control; press inverts it to an
   inner shadow. The glow uses **only** `primary` so the hover signal
   stays inside the single-accent family. Implementation lives inside
   the existing `interaction.hover` delta on `ComponentSpec`; no new
   spec field.

### Hover / press brightness deltas

`ButtonStyle.Draw` automatically:

- Hover: `+12` brightness via `adjustColor` (in addition to the glow
  named above).
- Press: `−20` brightness.

No additional code required at call sites.

### Panel chrome

Floating panels (inspector, FX, context menu, color wheel) draw via
`drawPanel(dst, r)` which paints:

1. `surface-overlay` fill (with 250/255 alpha).
2. Rounded corners (`rounded.md` desktop, `rounded.xl` mobile bottom-sheet).
3. A 1 px **panel border** (`border` at 20/255 alpha).
4. A subtle drop shadow.

Backdrop scrims (full-canvas dim behind a panel) use `colScrim = NRGBA{0,
0, 0, 150}` — applied in code, not tokenized (the spec doesn't accept
alpha-bearing colors).

## Shapes

A five-step rounding scale. Use these — never numeric literals. The
scale is bumped one notch over a "professional/tactical" baseline so
the whole UI reads as cushioned/touchable rather than wireframe-strict.

| Token | Value | Usage |
|---|---|---|
| `rounded.sm` | 8 px | Compact buttons, context menu groups, row chips |
| `rounded.md` | 12 px | Standard buttons, popups, panel chrome |
| `rounded.lg` | 16 px | Desktop bottom sheets |
| `rounded.xl` | 20 px | Mobile bottom sheets |
| `rounded.full` | 999 px | Pill buttons (transport group bg, EQ/Scope tabs) |

`rounded.full` replaces the in-code `min(w, h) / 2` pill convention so
every pill in the app shares one token-driven story. When a runtime
caller renders a pill, it should reach for `rounded.full` rather than
computing a half-height.

## Components

Every button falls into one of five categories. Each maps to one of the
component-token entries in the YAML front matter.

### Primary (action)

`{components.button-primary}` — `colors.primary` fill, white border.

- **Use:** Floating action button (mobile "+" to add a row), primary CTA.
- **Icon:** mandatory.
- Go style: `FABStyle` in `theme.go`.

### Secondary (neutral transport / control)

`{components.button-secondary}` — `surface-2` fill, subtle border.

- **Use:** Play, Stop, Record, BPM ±, subdiv, view-switch, follow-on,
  upload/import/export, overflow.
- **Icon:** mandatory; text only if icon is genuinely insufficient.
- Desktop: `DrawTopEdgeHighlight = true` (subtle 3D depth).
- Mobile: flat (`DrawTopEdgeHighlight = false`).
- Go styles: `TransportPlayStyle`, `TransportStopStyle`, `TransportIncStyle`,
  `TransportDecStyle`, `TransportMiscStyle`, `TransportFollowOnStyle`,
  `UploadBtnStyle`, `PopupButtonStyle`.

### Row control (inline)

`{components.button-row-control}` — `surface-2` fill, subtle border, short
**text labels** rendered in Inter SemiBold.

- **Labels:** `"M"` (Mute) / `"S"` (Solo) / `"FX"` / `"O"` (Origin) / `"X"`
  (Delete). Font glyphs are anti-aliased at design time and look clean at
  16–24 px. The programmatic vector-icon system (`drawIconByID`) is **not
  used here** — it lacks sub-pixel AA and looks jagged at small sizes.
- Active states are separate component tokens
  (`button-row-control-mute-active`, `…-solo-active`, `…-fx-active`).
- Go style: `InstButtonStyle` (inactive) → `MuteActiveStyle` /
  `SoloActiveStyle` / `FXActiveStyle` (active).

### Destructive (delete / confirm)

`{components.button-destructive}` — `destructive` fill, `destructive-border`
border.

- **Use:** Delete row, "Delete" context menu item, confirm dialogs.
- **Label:** `"X"` text; confirm state uses `"!!"` with
  `{components.button-destructive-confirm}`.
- **Text color:** `error`.
- Go styles: `DeleteButtonStyle`, `DeleteConfirmButtonStyle`.

### Disabled

`{components.button-disabled}` — `on-surface-disabled` fill, subtle border.

- **Use:** Any button that cannot activate in the current state.
- Go style: `DisabledButtonStyle`.

### Constructors

`ui_style.go` provides one-call constructors. Use these instead of inline
`ButtonStyle{Fill:…, Border:…}` literals scattered across the codebase.

```go
// Row control (icon-only, surface-2 + subtle border):
btn := RowControlButton(IconMute)
btn.OnClick = onMute

// Update active state (call each frame or on state change):
ActiveRowControl(btn, isMuted)

// Destructive:
del := DestructiveButton()

// Close button for panel header:
cls := CloseButton()

// Add / plus button:
add := AddButton()

// Generic icon-only:
btn := IconOnlyButton(IconChevronDown, InstButtonStyle)

// Panel geometry helpers:
spec := PanelSpec{Title: "FX: Kick-deep", Width: 260, MinHeight: 80}
hdrR  := spec.HeaderRect(anchor)
bodyR := spec.ContentRect(anchor, bodyH)

// Menu height:
m := MenuSpec{Items: []MenuItemSpec{
    {Label: "Mute",   Icon: IconMute},
    {Label: "Delete", Icon: IconTrash, Destructive: true},
}}
totalH := m.TotalHeight()
itemH  := m.ItemHeight()
```

### Semantic IconColor mapping

The icon color (`Button.IconColor`) carries semantic meaning. Set it
explicitly at button construction — never rely on the default. The
`Profile()` may shadow some on mobile to honor flat-vs-depth differences
(see "Mobile vs. Desktop"); that is the only sanctioned override.

| Button | Default IconColor | Active variant |
|---|---|---|
| Play | `Profile().PlayIconColor` (desktop: `on-surface`, mobile: `success`) | — |
| Stop | `Profile().StopIconColor` (desktop: `error`, mobile: `on-surface`) | — |
| Record (idle) | `record-idle` | swap to `record-active` while recording |
| BPM +/− | `Profile().BPMIconColor` | — |
| Track on/off | `on-surface-muted` | accent border in `TransportFollowOnStyle` when locked |
| Upload / Import / Export | `on-surface-muted` | — |
| Mute (row) | text `"M"` (no icon) | `MuteActiveStyle` fill when muted |
| Solo (row) | text `"S"` (no icon) | `SoloActiveStyle` fill when soloed |
| FX (row) | text `"FX"` (no icon) | `FXActiveStyle` border when panel open |
| Close (any panel) | `on-surface-muted` | — |
| Trash / Destructive | `error` | confirm: `button-destructive-confirm` |
| Chevron (FX expand / reorder, BPM stepper) | `on-surface-muted` | — |

### Panel & overlay components

**Floating panel (popup / inspector)** — `{components.panel}`.

- **Header:** `surface-overlay` background, title at `typography.panel-title`.
- **Close button:** top-right via `closeButtonRect(panelRect, pad)`, icon
  `IconClose`.
- **Section toggles:** filled triangle chevron
  (`drawFilledTriangleDown` / `…Right` in sidebar) — never `>` / `v` text.
- Collapsed sections show a badge pill (value summary, right-aligned).

**Bottom sheet (mobile)** — `{components.bottom-sheet}`. Top-only rounded
corners (`rounded.xl`), drag handle pill at top, title row with
`IconClose` top-right.

**Context menu** — `{components.context-menu-item}`.

- **Desktop:** floating panel, ~180 px wide, items stacked vertically.
- **Mobile:** full-width bottom sheet.
- Item height: 40 px desktop, 48 px mobile.
- Leading icon (20×20 px): `on-surface-muted` tint.
- Label: `on-surface`, single line, no truncation.
- Destructive item: `{components.context-menu-item-destructive}` —
  `destructive` bg, `error` text, `IconTrash` leading icon.
- Group separator: 1 px `border` (subtle alpha).

**EQ panel tabs.** Both the left filter tabs (`Master` / `HP` / `LP`) and
the right view tabs (`EQ` / `Wave` / `Spectrum` / `Meters` / `Scope`) must
use the **same visual affordance**: pill-shaped buttons with
`{components.button-row-control}`, active state `primary` border.

**Inspector section toggles.** Use the filled triangle chevron already in
`game_node_sidebar.go`. Do not use raw text `>` / `v` / `▶` / `▼` for
collapsibles.

## Do's and Don'ts

### Tokens, not literals

- **Do** route every color through a `Token*()` accessor in `theme_tokens.go`
  or, where the accessor is missing, through a `colXxx` constant from
  `theme.go`.
- **Don't** write `color.RGBA{…}` or `color.NRGBA{…}` literals in
  `internal/ui/` outside `theme.go`, `theme_tokens.go`, `drawing.go`, and
  `icons.go`.
- **Don't** reuse a red variant for a different semantic role (see
  "Colors → Semantic state"). Three reds, three roles.

### Icons & glyphs

1. **Never use raw Unicode** (`▶`, `▼`, `✕`, `✓`, `≡`, `+`) where an
   `IconID` exists.
2. Icons are drawn white-on-transparent via `iconSprite` and tinted at
   call-site via `IconColor`.
3. Icon inset: ~20% (`dim/5`) — built into all `draw*Icon` functions.
4. Stroke weight: `iconStroke(r) = dim/8`, floor 2 px — shared across the
   set.
5. Minimum icon button: `spacing.btn-sm` (28 px) desktop,
   `spacing.touch-min` (44 px) mobile.

### Permitted text-glyph exceptions

A single Unicode character is allowed in chrome **only** in the contexts
listed below. Anything else routes through `IconID`. Each exception names
the file:line that owns it so future audits can verify nothing has crept
in.

| Glyph | Context | Owner | Why text instead of an icon |
|---|---|---|---|
| `\|\|` (frozen) / `>` (resume) | Freeze toggle in any analyzer/scope panel header (Scope, EQ analyzer, …) | `scope_panel_zone.go:123`, `eq_panel_zone.go:149` | One rule covers every freeze toggle. Stateful indicator inside a 12–14 px chip; no `IconID` for "freeze" exists, and the glyph IS the affordance. Color transitions `on-surface-muted` (inactive) → `primary` (frozen). Adding a new analyzer panel that needs freeze reuses this row — do not add per-panel rows. |
| `÷n` | Subdivision indicator on transport bar | `transport_zone.go:191` | The `÷` is a math operator, not a UI glyph; `n` is data, and the operator is the most compact label. |
| `!!` | Row-rack delete-confirm warning | `row_rack_zone.go:1161` | Stateful warning glyph distinct from the resting `IconClose` icon — confirms a destructive action with explicit text rather than a second icon. |
| `OVR` / `SPL` / `DIF` / `AG` | Scope panel mode/auto-gain buttons | `scope_panel_zone.go:106–122` | Three-letter mode codes are descriptive labels, not glyphs. |
| `HP` / `LP` | EQ filter toggles | `eq_panel_zone.go:172–182` | Same rationale as Scope mode buttons. |

### Icon-vs-text decision matrix

When deciding whether a control wants an icon, a text label, or a colored
chip, walk this matrix from left to right and stop at the first row that
fits.

| Context | Render size | Icon? | Text? | Color-only? | Notes |
|---|---|---|---|---|---|
| Transport (Play/Stop/Record/Track/Upload/Import/Export/Overflow) | ≥ 32 px | **Yes** | no | no | |
| BPM ± steppers | ≥ 28 px | **Yes** (chevrons) | no | no | |
| Subdivision label | ≥ 28 px | no | **Yes** (`÷n`) | no | Permitted exception. |
| Row-rack control buttons (mute/solo/fx/origin/delete) | 16–24 px | **Yes** (`IconMute`, `IconSolo`, `IconFx`, `IconTarget`, `IconClose`) | no | no | Vector renderer makes 16 px legible. |
| Sidebar expand tab (collapsed sidebar) | 20 px | **Yes** (`IconChevronRight`) | no | no | |
| Row instrument selector | row label | no | **Yes** + color swatch | no | Gold standard. |
| Row volume slider thumb | 14–18 px | no | no | **Yes** | Colored circle; no glyph. |
| Row color swatch button | 12 px | no | no | **Yes** | Per-instrument color is the affordance. |
| Context-menu items | 20×20 icon + label | **Yes** + label | label always | no | |
| Instrument list rows | row label + 12 px swatch | no | **Yes** + color | no | |
| FX panel: expand / collapse / reorder / remove | 24 px | **Yes** (chevrons + close) | no | no | |
| FX panel: enable toggle | pill 32×18 / 40×22 | no | no | **Yes** (green/grey) | Custom switch widget. |
| EQ / Scope panel tabs | pill | no | **Yes** | no | |
| EQ / Scope freeze | 12–14 px | no | **Yes** (`\|\|`/`>`) | no | Permitted exception. |
| EQ HP / LP / band-mute | small chip | no | **Yes** (2 letters / `M`) | no | Permitted exception. |
| Sidebar section toggles | 10–12 px triangle | no | no | **Yes** + filled triangle | `drawFilledTriangleDown/Right`, *not* an `IconID`. |
| Master volume icon | 20 px | **Yes** | no | no | `IconSpeaker` / `IconSpeakerOff`. |
| Anything below 28 px desktop / 44 px mobile that doesn't fit the rows above | — | **no** | yes / colored chip | maybe | Default to text or color. |

### Adding new UI elements — checklist

Before adding any new button, panel, icon, or menu item:

- [ ] Does an `IconID` already exist for the semantic action? Use it.
- [ ] Walk the decision matrix above. Is icon, text, or color-only correct
      here?
- [ ] If text, does it belong in the permitted-glyph list above? If new, add
      the row first.
- [ ] Which button category does this fall into? Use the matching component
      token / Go `ButtonStyle`.
- [ ] Does a `ui_style.go` constructor already cover this? Use it instead of
      inline literals.
- [ ] Is the IconColor semantic? Cross-check the IconColor mapping table.
- [ ] Is this desktop-only or mobile-only? Document the intentional
      difference under "Mobile vs. Desktop".
- [ ] Does the new color (if any) map to an existing token? If not, add a
      token to the YAML front matter first.
- [ ] Add at least one scene in `scene_catalog.go` (see "Scene Coverage")
      covering the new state.
- [ ] Run `make screenshots-all` (and `MOBILE=1` if mobile-affecting) and
      compare to baseline before merging.

### Icon renderer (vector)

Every `IconID` renders onto a square logical canvas of `IconGrid` (24)
units. Bodies live in `drawing_icons.go`; the kernel — canvas mapping,
stroke-width math, path helpers — is in `icon_renderer.go`. Geometry
goes through Ebiten's `vector` package (antialiased path rasterization
via `vector.Path` + `DrawTriangles`), so 16-px icons read cleanly and
the whole set shares a single visual language:

- **Grid:** 24×24 logical units (`IconGrid`). Every icon stays in this
  space; the renderer scales to whatever bounding rect the call site
  passes in.
- **Padding:** 2-unit empty band (`IconPadding`). Live area is 20×20.
- **Stroke:** 1.75 logical units (`IconStrokeWeight`), float32 so the
  fraction survives at small render sizes. Pixel width is
  `weight × (renderSize / grid)`, floored at 1 px.
- **Caps & joins:** round (every helper sets `LineCapRound`,
  `LineJoinRound`).
- **Corner radius:** 2 logical units (`IconCornerRadius`) for any
  rectangular elements (pause bars, save chassis, stop square).
- **Antialias:** always on.

Adding a new icon is a 5-to-10-line body in `drawing_icons.go`. The
public surface — `IconID`, `DrawIcon(dst, id, r, col)` — is unchanged
from the pre-vector renderer; call sites do not need to migrate.

---

# Appendices

The following sections fall outside the canonical Stitch section list but
are preserved here as project-specific design references. The Stitch lint
treats unknown headings as informational; do not rename these to canonical
names.

## Icon Map

The icon set has **29 glyphs** defined in `icons.go`. Every icon has
**exactly one** semantic role. Use `DrawIcon(dst, id, r, col)` or set
`Button.Icon = string(IconID)`.

### Transport

| Icon | `IconID` | Usage |
|---|---|---|
| Play triangle | `IconPlay` | Play button |
| Pause bars | `IconPause` | Pause state |
| Stop square | `IconStop` | Stop button |
| Record dot+ring | `IconRecord` | Record button |

### Row controls

Row-rack buttons use the unified `IconID` glyphs at 16–24 px. The vector
renderer (1.75-unit stroke, round caps, antialiased) keeps small icons
legible — earlier scanline-stamped icons looked jagged at this size,
which is why the controls used text labels prior to the icon-system
overhaul.

| Button | `IconID` | Inactive style | Active style |
|---|---|---|---|
| Mute | `IconMute` (speaker + slash) | `InstButtonStyle` | `MuteActiveStyle` |
| Solo | `IconSolo` (headphones) | `InstButtonStyle` | `SoloActiveStyle` |
| FX | `IconFx` (sparkle) | `InstButtonStyle` | `FXActiveStyle` |
| Origin | `IconTarget` (crosshair) | `InstButtonStyle` | — |
| Delete | `IconClose` (× diagonals) | `DeleteButtonStyle` | `DeleteConfirmButtonStyle` (text `"!!"`) |

The delete-confirm state stays as text `"!!"` because it is a stateful
warning glyph, distinct in role from the resting close-icon — see the
permitted-text-glyph table.

### Navigation / control

| Icon | `IconID` | Usage |
|---|---|---|
| 3 dots vertical | `IconOverflow` | Context menu trigger |
| + cross | `IconPlus` | Add row, Add Effect — **never raw `+` character** |
| − bar | `IconMinus` | Remove item |
| Chevron up | `IconChevronUp` | Increment, reorder up — **never `^` or `▲`** |
| Chevron down | `IconChevronDown` | Decrement, expand-when-collapsed (mobile FX row), reorder down — **never `v` or `▼`** |
| Chevron right | `IconChevronRight` | Collapsed-section indicator (mobile FX row) — **never `>` or `▶`** |
| × diagonals | `IconClose` | Close any panel/popup, remove an effect slot — **never `X`, `✕`, or `×`** |

### File operations

| Icon | `IconID` | Usage |
|---|---|---|
| Up-arrow + dashed bar | `IconUpload` | Upload / share |
| Down-arrow + tray | `IconImport` | Import JSON |
| Up-arrow + tray | `IconExport` | Export JSON |
| Floppy disk | `IconSave` | Save |

### View / state

| Icon | `IconID` | Usage |
|---|---|---|
| 3 horizontal lines | `IconRows` | Switch to row view |
| EQ bars | `IconAudio` | Switch to EQ/audio view |
| Closed padlock | `IconTrack` | Follow-on (locked scroll) |
| Open padlock | `IconTrackOff` | Follow-off (free scroll) |
| Speaker+waves | `IconSpeaker` | Volume on / audible |
| Speaker body only | `IconSpeakerOff` | Volume zero / muted |
| Musical note | `IconNote` | Pitch / note indicator |
| Filled circle | `IconCircle` | Color swatch proxy in menus |
| Pencil | `IconPencil` | Rename / edit |

## Mobile vs. Desktop — Intentional Differences

These differences are **intentional** and must be preserved:

| Aspect | Desktop | Mobile |
|---|---|---|
| Row height | 40 px (`desktopRowHeightPx`) | 56 px (`touchRowHeightPx`) |
| Button depth | `DrawTopEdgeHighlight = true` | `DrawTopEdgeHighlight = false` (flat) |
| Splitter handle color | `colSplitterHandle` (gray pill) | `colSplitterHandleMobile` (magenta) |
| Splitter grip marks | Yes (3 tick marks) | No |
| Context menu style | Floating panel | Full-width bottom sheet |
| Row controls visible | M/S/FX/O/X | Overflow button only |
| Accent stripe | 3 px, flush | 5 px, 2 px vertical inset |
| Popup text scale | 1.0× | 1.3–1.7× |
| Timeline default | No cap (full circuit) | 8 beats |

All other visual differences are **accidental** and should be unified.

## Screenshot Infrastructure

```bash
# Single shot: desktop native + browser desktop + browser mobile
make screenshot               # → screenshots/{desktop,browser_desktop,browser_mobile}.png

# All UI scenes
make screenshots-all          # → screenshots/all/{desktop,mobile}/<scene>.png
make screenshots-all SCENES=transport_idle,context_menu_open
make screenshots-all MOBILE=1 # adds mobile viewport pass

# Direct CLI flags on the binary
./beatmo -screenshot /tmp/out.png           # capture and exit
./beatmo -scene transport_idle -screenshot /tmp/scene.png
./beatmo -list-scenes                       # list all registered scene names
```

## Scene Coverage

Every UI surface must have at least one entry in
`src/go/internal/ui/scene_catalog.go`. Adding a new panel, overlay,
sub-menu, or significant state combination requires adding a scene in the
same PR. The CI gate is: `make screenshots-all` succeeds and
`manifest.json` contains every name in the canonical list below.

**Canonical scenes** (current as of `add-core-node-types` branch). Names
are kebab-case and stable.

**Transport / baseline:** `transport_idle`, `transport_playing`,
`transport_high_bpm`, `transport_recording`.

**EQ tabs:** `eq_tab_eq`, `eq_tab_wave`, `eq_tab_spectrum`, `eq_tab_meters`,
`eq_tab_scope`, `eq_with_band_adjusted`, `eq_hpf_active`, `eq_lpf_active`,
`eq_band_muted`, `eq_scope_custom_settings`.

**Per-row menus / popups:** `context_menu_open`, `instrument_menu_open`,
`color_wheel_open`, `subdiv_menu_open`, `master_vol_popup`,
`desktop_per_row_vol_popup`.

**FX panel:** `fx_panel_open_empty`, `fx_panel_with_3_effects`,
`fx_panel_distortion_only`, `fx_panel_reverb_only`, `fx_panel_delay_only`,
`fx_panel_knob_drawer_open`.

**Graph / sidebar:** `node_added`, `edge_built`, `node_sidebar_open`,
`graph_complex_3_nodes_4_edges`, `node_sidebar_logic_expanded`,
`node_sidebar_groove_expanded`, `node_sidebar_audio_expanded`,
`node_sidebar_node_muted`, `node_sidebar_node_silent`,
`node_sidebar_node_invisible`.

**Multi-row state:** `multi_row_full_grid`, `row_muted_soloed`.

**Playback overlays** (menus + popups during playback): `playback_context_menu_open`,
`playback_instrument_menu_open`, `playback_color_wheel_open`,
`playback_subdiv_menu_open`, `playback_fx_panel_with_3_effects`,
`playback_eq_tab_meters`, `playback_eq_tab_scope`.

**Recording overlays:** `recording_with_context_menu`,
`recording_during_playback_overlay`.

**Mobile-only:** `mobile_default`, `mobile_overflow_open`,
`mobile_view_audio`, `mobile_per_row_vol_popup`.

When adding a scene: pick a name in this taxonomy (transport / eq / fx /
graph / playback / mobile / recording), set `Mobile: true` if the state is
mobile-only or mobile-meaningful, and reuse existing setup helpers
(`ensureRow`, `sceneSetBPM`, `g.SetPlaying(true)`,
`g.drum.OpenContextMenu(idx)`, etc.) — do not invent parallel openers.

## Bridge to Go

The YAML front matter is normative. The Go runtime expresses these tokens
via:

**Generated** (do not hand-edit; regenerate with `make gen-design-tokens`):
- `src/go/internal/ui/design_tokens.gen.go` — emits, in this order:
  - `genColor*` (one per `colors:` entry)
  - `genSpacing*`, `genRounded*`, `genIcon*` (one per `spacing:` / `rounded:` / `icon:` entry)
  - `genAlpha*` (one per `alpha:` entry)
  - `genInstrumentSwatch*` + `genInstrumentSwatches` (curated picker swatches)
  - `genInstrumentDefault*` + `genInstrumentDefaults` (built-in instrument default colors, Phase 3 PR2)
  - `genInstrumentFallback*` + `genInstrumentFallbackPalette` (cycle palette for unknown ids, Phase 3 PR2)
  - `ExpDecayAnim` / `SinPulseAnim` / `FadeFactor` types + `genAnim*` constants (Phase 3 PR3)
  - `genGeom*` constants (Phase 3 PR4)
- `src/go/internal/ui/design_components.gen.go` — `ComponentSpec` values + `ComponentID` enum + `Spec(id)` accessor, one per `components:` entry.
- `src/go/internal/ui/design_profile.gen.go` — `genDesktopProfile`, `genMobileProfile` `profileValues` structs from `profileOverrides:`.

**Hand-written runtime** (consumes the generated symbols):
- `src/go/internal/ui/theme.go` — `colXxx` aliases pointing at `genColorXxx`. Pre-existing semantic names preserved for legacy call sites. Also hosts `buildInstColors` / `buildCustomPalette` which lift the generated instrument-defaults map / fallback slice into the historical `map[string]color.Color` / `[]color.Color` shapes consumed by `drumview.go`.
- `src/go/internal/ui/animation_helpers.go` (Phase 3 PR3) — `DecayStep`, `SinPulse`, `SinPulseAlpha` consumers for `genAnim*`. Per-frame call sites read these — never inline the magic numbers.
- `src/go/internal/ui/theme_tokens.go` — `Token*()` accessors + named alpha bucket constants (aliased from generated buckets) + `WithAlpha(token, bucket)` helper.
- `src/go/internal/ui/touch_sizes.go` — `Space*`, `Btn*`, `Radius*`, `IconSize*` aliased from generated symbols.
- `src/go/internal/ui/ui_style.go` — `IconOnlyButton`, `RowControlButton`, `ActiveRowControl`, `DestructiveButton`, `AddButton`, `CloseButton`, `PanelSpec`, `MenuSpec` constructors.
- `src/go/internal/ui/render.go` — `Render(dst, rect, spec, state)` is the single chrome rendering primitive. Pure function; consumes `ComponentSpec` + `ComponentState`. `renderLegacy(...)` is the lower-level entry used by the `*Style.Draw` adapters.
- `src/go/internal/ui/design_types.go` — runtime types `ComponentSpec`, `BorderRef`, `InteractionDelta`, `Interaction`, `ComponentState`, `ComponentID`, `ComponentCategory`.
- `src/go/internal/ui/layout_profile.go` — `LayoutProfile` builder reads `profileValues` for dimensional fields; remaining fields hand-coded.
- `src/go/internal/ui/runtime_profile.go` — `RuntimeFlags` + `Runtime()` accessor for behavior toggles (DirectDrawRows, EnableLayoutResize, ShowEscHint, UseBottomSheet, etc.). Documents the design-data vs runtime-flag boundary.

Validation:

- `npx @google/design.md lint DESIGN.md` — should return zero errors.
  Warnings (orphan tokens, extension fields below) are absorbed into
  `src/go/internal/ui/testdata/design_md_lint.expected.json` with
  per-warning rationale.
- `cmd/gen_design_tokens` (run via `make gen-design-tokens`) reads the
  YAML and emits `internal/ui/design_tokens.gen.go` (primitives) and
  `internal/ui/design_components.gen.go` (component specs). Generated
  files are committed.
- `design_tokens_freshness_test.go` re-runs the generator into a tempdir
  and diffs against the committed file — fails on "you forgot to run
  `make gen-design-tokens`".
- `design_md_drift_test.go` independently parses DESIGN.md primitives
  and asserts each runtime constant matches — catches generator bugs
  the freshness test can't (where committed and regenerated agree but
  both are wrong).
- A pre-commit hook installed via `make install-hooks` regenerates and
  rejects commits with stale `*.gen.go`.

### Extension fields (Phase 2 codegen)

`components:` entries accept fields beyond Stitch's narrow set. The
generator parses them strictly (unknown keys fail with file:line);
Stitch's lint flags them as warnings, absorbed into the snapshot.

| Field | Type | Purpose |
|---|---|---|
| `category` | `button \| input \| panel \| menu \| container \| viz \| widget \| decoration` | Closed enum for grouping + sanity checks. |
| `border` | `{color: "{colors.X}", alpha: <bucket>}` | Border color + named alpha bucket. Rejects unknown buckets. |
| `iconColor` | `"{colors.X}"` | Promotes the IconColor mapping table (Bridge-to-Go §IconColor) into the design system. |
| `topEdgeHighlight` | `bool` | Enables the 1-px desktop top-edge highlight for the secondary button family. |
| `interaction.{hover,press,focus}.{fillDelta,borderDelta,highlightDelta}` | `int` (signed, ±127) | Per-state brightness deltas. Per-component values preserved verbatim from `ButtonStyle.Draw` / `TextInputStyle.DrawAnimated` etc. — no normalization. |
| `variants.<name>` | mapping (Phase 2 PR3) | Full token rebindings keyed by `mobile`, `active`, `disabled`, etc. Reserved field; parser accepts but emitter ignores until PR3. |
| `dynamic` / `animation` | `{kind, anchor, target}` | Reserved escape hatches for runtime-bound color/alpha (e.g. ColorSwatchStyle.Color closure, record-pulse alpha). Parser accepts; emitter wires registry binding in PR3. |

### Top-level extension blocks (Phase 3 PR1–4)

The `colors:` / `spacing:` / `rounded:` / `alpha:` / `components:` /
`profileOverrides:` set has been extended with five additional
top-level YAML keys. All five are normative and consumed by the
chrome runtime; editing any of them and re-running
`make gen-design-tokens` propagates the change end-to-end.

| Block | Phase | Schema kind | Generator output | Runtime consumer |
|---|---|---|---|---|
| `instrumentDefaults:` | 3 PR2 | `[{id, color}]` | `genInstrumentDefaults map[string]color.RGBA` + per-id `genInstrumentDefault*` constants | `theme.go` `buildInstColors()` → `instColors` (drum cell tint, edge tint, rack swatch) |
| `instrumentFallbackPalette:` | 3 PR2 | `[{color}]` | `genInstrumentFallbackPalette []color.RGBA` + indexed `genInstrumentFallback*` constants | `theme.go` `buildCustomPalette()` → `customPalette` (cycle for unknown instrument ids) |
| `animations:` | 3 PR3 | `{name: {kind: exp-decay\|sin-pulse\|fade, …}}` | `ExpDecayAnim` / `SinPulseAnim` / `FadeFactor` types + `genAnim*` constants | `animation_helpers.go` `DecayStep` / `SinPulse` / `SinPulseAlpha`; call sites read `genAnim*` directly for `fadeColor` factors |
| `geometry:` | 3 PR4 | `{name: number}` | `genGeom*` float64 constants | Read directly at call sites: `components.go` (`SignalStyle.Draw` glow multiplier, `NodeStyle.Draw` border thickness), `grid.go` (`EdgeArrowSize`), `game_draw_grid_pane.go` (highlight border thickness) |

### Animation kinds (Phase 3 PR3)

The `animations:` block uses a closed `kind` enum to dispatch to typed
runtime helpers. Schema validation enforces per-kind required fields.

| `kind` | Required fields | Optional fields | Generated type | Runtime helper |
|---|---|---|---|---|
| `exp-decay` | `rate`, `threshold` | — | `ExpDecayAnim{Rate, Threshold}` | `DecayStep(v, anim) (float64, bool)` |
| `sin-pulse` | `frame-step`, `amplitude`, `base` | `alpha-scale` (uint8) | `SinPulseAnim{FrameStep, Amplitude, Base, AlphaScale}` | `SinPulse(frame, anim) float64`, `SinPulseAlpha(frame, anim) uint8` |
| `fade` | `factor` | — | `FadeFactor` (named float64) | Read directly via `float64(genAnimXxx)` at `fadeColor()` call sites |

Adding a new animation: pick a kind, add an entry under `animations:`,
run the generator. Adding a fourth kind requires adding the kind to
the schema enum, the generator's per-kind validator, the template,
and a new helper in `animation_helpers.go`.

### Editing-DESIGN.md → re-render workflow

1. Edit `DESIGN.md`'s YAML front matter — colors, instrument defaults,
   animation cadence, or geometry.
2. Run `make gen-design-tokens` (or commit; the pre-commit hook does
   it automatically).
3. Run `make run` (desktop) or `make wasm && open browser` to see the
   change end-to-end.
4. Verify guard tests pass:
   `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestDesignMDDrift|TestTokenDiscipline|TestDesignMDLintSnapshot' ./internal/ui/`

A change in DESIGN.md that does not show up at runtime is a missed
call-site — there is a hand-coded literal somewhere bypassing the
token system. Add a token (or a runtime helper) to bring that call
site under DESIGN.md control.
