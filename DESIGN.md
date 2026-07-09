---
version: "1"
name: Beatmo
description: >
  Beatmo UI design system. Dense graph + timeline + EQ + scope + per-row
  controls coexist in a single canvas without becoming noisy. Cushioned warm-slate
  theme with a single sunset-gold accent; instrument identity is the only multi-color
  signal in the chrome. Built on top of Ebiten (desktop) and a WASM/WebAudio
  runtime (browser).
colors:
  # ── Vice City chromatic ramps (35) — the single source of all hue.
  # Seven hue families × five shades, light→deep (-100..-500). Family order
  # is itself a continuous neon spectrum. hot-pink-400 (#EE00DD) is the
  # GTA Vice City signature. See docs/superpowers/specs/2026-06-13-vice-city-palette-design.md.
  sunset-gold-100: "#FFE98A"
  sunset-gold-200: "#FFD319"
  sunset-gold-300: "#FFB30A"
  sunset-gold-400: "#FF9E1F"
  sunset-gold-500: "#F58A12"
  tangerine-100: "#FFC299"
  tangerine-200: "#FF9E54"
  tangerine-300: "#FF7A3D"
  tangerine-400: "#FF5C2A"
  tangerine-500: "#E84A1F"
  hot-pink-100: "#FFA8E0"
  hot-pink-200: "#FF6AD5"
  hot-pink-300: "#FF2D9E"
  hot-pink-400: "#EE00DD"
  hot-pink-500: "#C400B5"
  orchid-100: "#E0A8FF"
  orchid-200: "#C24FFF"
  orchid-300: "#A030FF"
  orchid-400: "#8C1EFF"
  orchid-500: "#6E12E0"
  electric-blue-100: "#A8C8FF"
  electric-blue-200: "#5E8AFF"
  electric-blue-300: "#3A6AF0"
  electric-blue-400: "#2A5FD6"
  electric-blue-500: "#1E47B5"
  cyan-100: "#9AF0FF"
  cyan-200: "#3FE0E8"
  cyan-300: "#00C8E0"
  cyan-400: "#00A8C9"
  cyan-500: "#0E7391"
  mint-100: "#B3F5C2"
  mint-200: "#6BE89A"
  mint-300: "#3FD67A"
  mint-400: "#2BC85C"
  mint-500: "#1FA84A"
  # ── Neon-night neutrals (violet-tinted; the structural ramp).
  night-bg: "#120A1C"
  night-surface-1: "#1C1230"
  night-surface-2: "#281A40"
  night-surface-3: "#382354"
  night-surface-overlay: "#1F1438"
  night-text-hi: "#FDF2FF"
  night-text-mid: "#C9B8D6"
  night-text-dim: "#8A7A9C"
  # Primary interactive (single sunset-gold accent — warm repaint of the
  # prior cyan/azure accent; used for every active / selected / focused chrome
  # state). See "Single chrome accent" invariant in the prose.
  primary: "#FFB30A"
  primary-bright: "#FFD319"
  primary-dim: "#FF9E1F"

  # Surface hierarchy (4 levels, neon-night violet-black theme — the ladder
  # feels like soft matte plastic against the Vice City horizon, not inky
  # aerospace). Lifts in violet-black steps above the #120A1C night base.
  background: "#120A1C"
  surface-1: "#1C1230"
  surface-2: "#281A40"
  surface-3: "#382354"
  surface-overlay: "#1F1438"

  # On-surface text (faint-magenta near-whites against the violet-black ladder)
  on-surface: "#FDF2FF"
  on-surface-muted: "#C9B8D6"
  on-surface-disabled: "#8A7A9C"
  on-surface-accent: "#FFB30A"

  # Borders (single base color; opacity is applied at draw time — see "Borders" prose)
  border: "#FFFFFF"

  # Semantic state (mint success, vermillion error, deep-magenta mute,
  # ember destructive)
  success: "#3FD67A"
  error: "#FF5C2A"
  mute: "#C400B5"
  destructive: "#E84A1F"
  destructive-border: "#FF5C2A"
  record-idle: "#E84A1F"
  record-active: "#FF5C2A"

  # Visualization tokens (Wave / Spectrum / Meters / Scope / EQ canvases).
  # NOTE on the viz-bar / viz-wave golds: these are *data series* colors,
  # NOT chrome — they share the warm sunset-gold family with the chrome
  # accent but are owned by the data canvases. See the "Single chrome
  # accent" invariant. EQ knobs / handles / curve all use `primary`
  # (sunset-gold) so the chrome carries one accent and the data
  # canvases speak their own visual language.
  viz-bg: "#120A1C"
  viz-bar: "#FFB30A"
  viz-bar-peak: "#FFE98A"
  viz-curve: "#FFB30A"
  viz-wave-a: "#FFD319"
  viz-wave-b: "#8A7A9C"

  # Meters tab — VU-style level meter with golden / orange / red zones plus
  # peak-clip indicator. Distinct hue from the spectrum tokens because the
  # meter row scales by amplitude (not by spectral bin), and the golden-
  # orange-red ramp conveys level warnings in the Vice City palette.
  # viz-meter-low  = sunset-gold-300 (#FFB30A, game primary) — safe/low zone
  # viz-meter-mid  = tangerine-300   (#FF7A3D) — warn/mid zone
  # viz-meter-high = tangerine-500   (#E84A1F) — hot/high zone
  viz-meter-low:  "#FFB30A"
  viz-meter-mid:  "#FF7A3D"
  viz-meter-high: "#E84A1F"
  viz-meter-bg:     "#1C1230"
  viz-meter-clip:   "#E84A1F"

  # Spectrum frequency-group tinting — used by the Spectrum tab to colour
  # the Bass / Mids / Treble bracket strip and group-fill highlights. Warm
  # = bass (low frequencies feel "warm" to listeners), cool = treble (high
  # frequencies feel "cool"), neutral = mids. Kept distinct from the green-
  # amber-red meter ramp so frequency tinting never reads as a level
  # warning.
  viz-bass:   "#FF7A3D"
  viz-mids:   "#C9B8D6"
  viz-treble: "#3A6AF0"

  # Grid-pane glow / ghost overlay — drawn at runtime alpha (faint for
  # ghost, strong for halo). A pale-gold sibling of `primary` so the
  # transient glow stays in the chrome's single-accent family.
  viz-glow: "#FFE98A"

  # Focus ring — the second chrome accent (hot-pink-300). Sits on focused
  # TextInput / selection rings. Distinct from the sunset-gold `primary` /
  # `viz-glow` family so focus reads as its own signal: gold = interactive/
  # selected/active surfaces, hot-pink = focus/selection rings (the
  # two-accent system). Replaces the earlier neutral white, which carried
  # no accent identity.
  focus-ring: "#FF2D9E"

  # Slider thumb (volume / horizontal sliders) — off-white pill with a
  # runtime drop shadow. Off-white (not pure 255) reduces glare against the
  # dark surface ladder.
  slider-thumb-fill:   "#FDF2FF"
  slider-thumb-shadow: "#000000"

  # Slider track background and filled portion. Track is a slightly cooler
  # neutral than surface-3 so the slider chassis doesn't merge with raised
  # button hover surfaces. Fill is `primary-dim` (deep gold) so the slider
  # value range belongs to the single-accent family but reads as muted
  # rather than primary action. This gold fill applies to the GENERIC
  # sliders only — param sliders and the master-volume popup. The PER-ROW
  # volume controls (the in-row speaker level indicator AND the per-row
  # volume popup rail) are a deliberate exception: their fill is the row's
  # instrument color (DrumRow.Color — the same single source of truth the
  # grid nodes and edges draw from), so the volume control reads as the
  # same instrument and tracks any live color edit. See volIconColor
  # (row_rack_zone.go) and SliderPopupConfig.Accent (slider_popup.go).
  slider-track-fill: "#281A40"
  slider-fill:       "#FF9E1F"

  # Sidebar section background — cool muted steel-blue, drawn at runtime alpha
  # ~100/255 to dim the section relative to the panel surface beneath.
  # Not a derived shade of `surface-*` because the sidebar wants visual
  # distance from the elevated controls layer.
  sidebar-section-bg: "#382354"

  # Scope panel — three base trace colors (gold / tangerine / lime),
  # the scope canvas background, and the trigger marker. Each is composited
  # at multiple runtime alphas (line, fill, dimmed-label) via WithAlpha.
  # Trace-a snaps to `primary` so the scope's primary tap belongs to the
  # single-accent family; trace-b is tangerine so the A/B pair stays legible
  # against the gold trace and the gold trigger marker.
  viz-scope-trace-a:    "#FFB30A"
  viz-scope-trace-b:    "#FF7A3D"
  viz-scope-trace-diff: "#3FD67A"
  viz-scope-bg:         "#120A1C"
  viz-scope-trigger:    "#FFD319"

  # Spectrum panel peak marker — pale peach, pinned at the highest recent
  # spectrum value. Distinct from `viz-bar` (the live bar) so it reads as
  # a held memory rather than an instantaneous reading.
  viz-spectrum-peak-marker: "#FDF2FF"

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
  play-desktop-fill:   "#2BC85C"
  play-desktop-border: "#3FD67A"
  stop-desktop-fill:   "#E84A1F"
  stop-desktop-border: "#FF5C2A"

  # Disabled control fill — slightly above on-surface-disabled so the
  # inert button still has a visible body silhouette against surface-1.
  # Used by DisabledButtonStyle.Fill.
  disabled-fill: "#281A40"

  # Destructive confirm state ("X" → "!!") — brighter than the resting
  # destructive fill to telegraph the second-click commitment. Used by
  # DeleteConfirmButtonStyle.{Fill,Border}.
  destructive-confirm-fill:   "#E84A1F"
  destructive-confirm-border: "#FF5C2A"

  # Mute-active border — sibling of `mute` (the fill); the border is
  # ~30% lighter for visible inset against surface-2. Mirrors
  # colMuteActiveBdr.
  mute-active-border: "#FF6AD5"

  # Solo-active fill mirrors `primary-bright` and is reused via
  # `{colors.primary-bright}`. Border is a lighter gold sibling so the
  # active-solo chip stays inside the single-accent family.
  solo-active-border: "#FFD319"

  # FX-active fill — deep desaturated navy that pairs with the sunset-gold
  # accent border to mark a row whose FX panel is open. Mirrors
  # FXActiveStyle.Fill.
  fx-active-fill: "#281A40"

  # EQ filter (HPF/LPF) toggle in active state — sunset-gold pair, snapping
  # to `primary-dim` (fill) and `primary` (border). HP/LP active now
  # belongs to the single-accent family along with every other "selected"
  # state. Mirrors EQFilterButtonActiveStyle.{Fill,Border}.
  eq-filter-active-fill:   "#FF9E1F"
  eq-filter-active-border: "#FFB30A"

  # MissingInstrument fill — slightly darker than the `error` token so
  # the row label reads as "data missing" (a state) rather than "alert"
  # (an event). Pre-existing distinction in the runtime (colError =
  # RGBA{220,60,60,255} vs colStopRed = RGBA{220,70,70,255}).
  error-text: "#FF5C2A"

  # EQ band-mute active fill — slightly darker red than destructive-border
  # so the EQ chip telegraphs "muted band" without competing with the
  # delete-button red. Pre-existing distinction in the runtime
  # (EQMuteButtonActiveStyle.Fill vs colDeleteBorder).
  eq-mute-active-fill: "#E84A1F"

  # Sidebar chrome — cool steel greys for inline chips, badges, and the
  # default-color swatch fallback. These are runtime composites at the
  # sidebar-section / sidebar-chip alpha buckets; the base hex moves
  # here so all sidebar greys are auditable in one place.
  sidebar-chip-fill:        "#382354"
  sidebar-chip-border:      "#8A7A9C"
  sidebar-swatch-fallback:  "#C9B8D6"
  sidebar-badge-bg:         "#1C1230"

  # Row-rack instrument-fallback grey — used when an instrument has no
  # registered color. Slightly lighter than sidebar-swatch-fallback so
  # the row label still reads as "interactive" against the rack surface.
  row-rack-color-fallback: "#FDF2FF"

  # Grid-pane debug overlays — shown only when DEBUG_GEOM=1. Bright pure
  # primaries so the markers pop above any node/edge color.
  viz-debug-edge:     "#FFD319"  # yellow cross at edge endpoint
  viz-debug-node:     "#FF5C2A"  # magenta cross at node center

  # Long-press preview pill chrome (graph deletion confirm). Two greys:
  # warm-slate fill (mirrors `surface-1`) for high-contrast against the
  # canvas, mid-grey border to read as a tappable surface.
  viz-pill-fill:   "#1C1230"
  viz-pill-border: "#8A7A9C"

  # Tap-to-confirm buttons in the long-press popup — desaturated mint/rust
  # pair, deliberately darker than the play/stop button colors so the
  # in-canvas confirm doesn't compete with the transport bar.
  viz-confirm-green: "#3FD67A"
  viz-cancel-red:    "#E84A1F"

  # Splitter (divider line between top/bottom panes) — three desktop
  # shades plus a mobile hairline at panel-border alpha. The shadow/
  # highlight pair gives the divider a subtle 3D edge on desktop.
  divider-base:      "#FDF2FF"
  divider-shadow:    "#000000"
  divider-highlight: "#382354"

  # Drum-row alternating zebra stripes inside the drum cache sprites.
  # Even rows sit slightly above the canvas background; odd rows sit
  # at-or-below it. Both are deliberately close to the surface ladder
  # but distinct from `surface-1` so the zebra reads as cell separation
  # rather than panel elevation. Carry a dark magenta-violet cast (R>G,
  # B>G) so the sequencer belongs to the same Vice City "purple/pink
  # horizon" surface as the graph pane's `grid-horizon`, while staying
  # dark enough that the instrument-colored cells still pop. Pinned by
  # TestDrumViewBackgroundCarriesVioletCast.
  drum-stripe-even: "#1C1230"
  drum-stripe-odd:  "#120A1C"

  # Drum-row hit/glow color — sunset-gold used at runtime alpha for cell hit
  # feedback. Same hex as `primary` so the playhead's transient flash
  # belongs to the single-accent family.
  drum-glow: "#FFB30A"

  # Drum-cell off (resting) fill — slightly above the canvas `background`
  # so the cell silhouette reads against the timeline. Carries the same
  # dark magenta-violet cast as the row stripes so empty cells recede into
  # the Vice City purple horizon. Pinned by
  # TestDrumViewBackgroundCarriesVioletCast.
  drum-cell-off: "#281A40"

  # Drum-cell highlight flash — cream off-white used for the per-beat
  # "hit" indicator. Distinct from `on-surface` (which is also cream but
  # used for chrome) so the flash reads as a transient signal, not a text
  # color. Mirrors the legacy hand-coded colHighlight.
  drum-cell-highlight: "#FDF2FF"

  # EQ readout pill background (the dB number chip behind a band marker).
  eq-readout-bg: "#1C1230"

  # In-canvas long-press popup secondary text.
  popup-text-secondary: "#C9B8D6"

  # Scope panel dimmed label color (legend entries when their trace is
  # hidden — runtime alpha is applied separately).
  scope-label-dim: "#382354"

  # Transport bar cached background — slightly darker than `surface-1`
  # so the timeline's beat ticks read against the bar rather than the
  # rack beneath. Lifted with the surface ladder.
  transport-bar-bg: "#120A1C"

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
  grid-line:           "#1C1230"
  grid-half:           "#1C1230"
  grid-quarter:        "#281A40"
  grid-eighth:         "#281A40"
  grid-sixteenth:      "#382354"
  grid-thirty-second:  "#382354"

  # Grid-pane sunset horizon. The graph pane background is no longer a flat
  # fill of `background`: it grades vertically from `background` (deep, at the
  # top) down to `grid-horizon` (a warm magenta-violet) at the bottom edge,
  # where it meets the splitter — the "horizon line" of the outrun/Vice-City
  # backdrop. Kept dark enough to stay a recessive background beneath the neon
  # instrument nodes; this is a *data-canvas* mood color (like the timeline
  # amber and spectrum golds), not a chrome accent, so it is exempt from the
  # single-accent rule.
  grid-horizon:        "#382354"

  # Graph node body — drawn as a filled circle on the lattice. Fill is a
  # neutral steel that reads against the lifted-slate background; border
  # is one notch brighter so the silhouette is legible regardless of
  # which per-row tint the connecting edges carry. The node is a
  # *chrome* surface (it doesn't carry instrument identity — that lives
  # on the edges and drum cells), so it stays inside the grayscale
  # ladder.
  node-fill:    "#281A40"
  node-border:  "#8A7A9C"

  # Connector / edge base hue. Edges are drawn at runtime alpha
  # (`alpha.edge-default`) and re-tinted per-row to the instrument
  # color; this token is the resting/default-row tint. Slightly
  # warmer than `node-border` so an unconnected node + a default
  # edge read as distinct grayscale steps.
  edge-color:   "#8A7A9C"

  # Splitter handle chrome — the handle at the divider between grid
  # pane and drum pane. Rendered with one unified code path AND one
  # color on every platform: a square handle with a soft glow halo.
  # The accent marks the splitter as a touch affordance; it now
  # snaps to the chrome's primary sunset-gold accent (`primary`) so it belongs
  # to the single-accent family like every other active control.
  splitter-handle:           "#FFB30A"

  # Timeline strip palette — drawn behind the per-row drum cells in
  # the drum pane. Five tokens form the strip's amber accent family. It
  # shares the warm sunset-gold hue with the chrome accent but stays a
  # data-canvas role: a "current beat / view region / cursor" trio that
  # reads as data-canvas chrome rather than primary action. Amber here is
  # to timeline what the viz golds are to spectrum bars: a data-canvas
  # color that escapes the single-accent chrome rule because it speaks the
  # canvas's visual language.
  # timeline-total-bg is darker than `background` to recess the strip.
  # timeline-view is drawn at `alpha.subtle` for the current view-window
  # tint; -hi is the full-opacity bright variant. timeline-cursor is the
  # playhead indicator. timeline-beat is the per-beat tick ruler.
  timeline-total-bg:  "#120A1C"
  timeline-view:      "#FF9E1F"
  timeline-view-hi:   "#FFB30A"
  timeline-cursor:    "#FFD319"
  timeline-beat:      "#8A7A9C"

  # Drum-mute cell pair — when a row is muted, the cells switch from
  # the per-row instrument color to this neutral gray family. Distinct
  # from `mute` (the mute *button* fill, deep rust) so the cell mute
  # signal reads as desaturation, not warning. `drum-mute-cell` is the
  # resting fill; `drum-mute-highlight` is the per-beat flash tint
  # (composited at `alpha.mute-highlight`).
  drum-mute-cell:        "#8A7A9C"
  drum-mute-highlight:   "#FDF2FF"

  # Wave panel "dry" trace — drawn beneath the live trace at low alpha
  # so the comparison between wet/dry signals reads as ghosted overlay.
  # Cool desaturated blue, distinct from `viz-wave-a` (the live trace
  # is sharp gold) so the eye separates them at a glance.
  wave-trace-dry: "#8A7A9C"

  # EQ zero-line ruler — the 0 dB reference line drawn across the EQ
  # canvas. A muted gray composited at `alpha.eq-zero-line` so the
  # ruler is legible without competing with the curve itself.
  eq-zero-line: "#8A7A9C"

  # Context-menu delete-row tint — warning-red at low alpha drawn
  # behind a destructive menu group. Distinct from `error` (text),
  # `mute` (button fill), `destructive` (button fill), and
  # `destructive-border` (button border): this is a *menu container
  # background tint*, not any of those four roles. Resolves the
  # in-code "until token added" TODO in theme.go.
  menu-delete-tint: "#E84A1F"

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
    # rowHeight = spacing.btn-md (36). The instrument row owns the largest
    # share of vertical chrome, so it sets the rhythm for the buttons inside
    # it; using btn-md keeps the row-button cells the same canonical size as
    # the rest of the desktop UI (transport, popup, EQ inputs) and gives the
    # speaker / mute / solo / fx glyphs ~24 px of icon space (rect * 18% pad).
    rowHeight:             36
    grabZone:              5
    minTarget:             0
    minCellWidth:          2
    popupPanelW:           220
    popupBtnW:             18
    popupBtnH:             16
    popupGap:              4
    popupPad:              6
    transportBtnSize:      32
    # rowControlBtnSize floors each row-control cell so a narrow window can
    # not shrink mute/solo/fx/volume below a comfortable mouse target. Set
    # to spacing.btn-sm (28) — half the difference between mobile's 32 px
    # touch floor and the natural cell width — so it only kicks in on very
    # narrow desktops, never crowding wider layouts.
    rowControlBtnSize:     28
    splitterHandleLen:     50
    splitterHandleThk:     8
    headerMinH:            40
    headerMaxH:            40
    timelineBarH:          12
    eqSliderH:             14
    synthHeaderH:          48
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
    closeButtonSize:       22
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
    synthHeaderH:          38
    eqHandleRadius:        26
    eqDBInputH:            20
    sliderTrackH:          6
    sliderThumbH:          24
    sliderThumbW:          14
    nodeMinPx:             12
    nodeMaxPx:             20
    edgeThickMul:          2
    controlPadding:        4
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

# Density-decoupled sizing tier. Orthogonal to `profileOverrides:`
# (which carries layout-arrangement decisions like "hide tab pills on
# mobile"). Density controls the absolute size of interactive controls
# — finger-vs-mouse, dense-vs-spacious — without coupling those choices
# to viewport class. Selected at runtime by `Profile().Density()`:
#   ScreenDesktop → DensityComfortable (pixel-perfect with today's
#                    desktopProfile())
#   ScreenMobile  → DensitySpacious (matches today's mobileProfile())
#   tests / future Settings UI can override either independently.
#
# Field set is a strict subset of `profileOverrides:` — the 34 fields
# whose only justification today is "fingers are bigger than mice."
# Compact is below today's desktop baseline (dense screens / power
# users); Comfortable equals today's desktop; Spacious equals today's
# mobile.
densities:
  compact:
    rowHeight:             32
    grabZone:               4
    minTarget:              0
    transportBtnSize:      28
    rowControlBtnSize:     24
    headerMinH:            36
    headerMaxH:            36
    eqSliderH:             12
    eqHandleRadius:        14
    eqDBInputH:            12
    sliderThumbH:          16
    sliderThumbW:          10
    sliderTrackH:           3
    nodeMinPx:              6
    nodeMaxPx:             14
    edgeThickMul:           1
    accentStripeWidth:      2
    accentStripeInsetY:     0
    popupBtnW:             16
    popupBtnH:             14
    popupGap:               3
    popupPad:               4
    popupSectionGap:        3
    closeButtonSize:       20
    splitterHandleLen:     44
    splitterHandleThk:      7
    splitterGrabThreshold:  0
    controlGap:             2
    controlGroupPad:        2
    controlPadding:         3
    fxToggleTrackW:        28
    fxToggleTrackH:        14
    fxToggleThumbD:        12
    timelineBarH:          10
    synthKnobMin:          24
    synthKnobIdeal:        56
    synthKnobCaptionH:     14
    synthConceptVizH:      38
    synthFocusGraphH:      96
    synthRightCardMinH:    34
    synthHeaderButtonH:    24
    synthHeaderThumbW:     96
    synthSectionMinH:      56
    synthChipH:            22
    synthChipMinW:         56
    synthChipStripH:       28
    synthDetailHeaderH:    28
    chainMiniMeterW:        3
    chainMiniMeterGap:      2
    chainTriggerMarkerW:    2
    chainStageColW:        48
    chainStageRowH:        20
    chainLabelScale:      800
    sidebarLabelScale:   1100
    sidebarValuePillW:     44
    audioPillH:            20
    audioPillGap:           2
    audioPillPadX:         12
    audioPillNarrowW:      16
    audioTabMinW:          24
    audioChannelMinW:      60
    chainContentGap:        6
    synthPreviewTargetW:  240
    synthPreviewMinW:     140
    samplerWaveMinH:       30
    samplerMinBodyH:       42
    audioLabelMarginW:     24
    chainReadoutScale:    750
    chainPillScale:       800
    chainBadgeScale:      800
    chainScopeLeftMargin:  28
    chainScopeBottomMargin: 14
    chainLegendStripH:     14
    chainMiniWaveH:         5
    chainAutoFitMinMs:      2
    levelsReadoutWFull:   120
    levelsReadoutWIcons:   28
    levelsReadoutFooterH:  14
    waveCursorStroke:       1
    samplerPlayheadStroke:  3
    spectrumCursorStroke:   1
    spectrumBracketH:       8
    knobStepBadgeW:        38
    knobStepBadgeH:        16
    mobileWheelW:         200
    mobileWheelH:         260
    mobileWheelTickGap:    30
    mobileWheelResStripW:  36
    mobileWheelPillH:      34
  comfortable:
    rowHeight:             36
    grabZone:               5
    minTarget:              0
    transportBtnSize:      32
    rowControlBtnSize:     28
    headerMinH:            40
    headerMaxH:            40
    eqSliderH:             14
    eqHandleRadius:        16
    eqDBInputH:            14
    sliderThumbH:          18
    sliderThumbW:          12
    sliderTrackH:           4
    nodeMinPx:              8
    nodeMaxPx:             16
    edgeThickMul:           1
    accentStripeWidth:      3
    accentStripeInsetY:     0
    popupBtnW:             18
    popupBtnH:             16
    popupGap:               4
    popupPad:               6
    popupSectionGap:        4
    closeButtonSize:       22
    splitterHandleLen:     50
    splitterHandleThk:      8
    splitterGrabThreshold:  0
    controlGap:             2
    controlGroupPad:        3
    controlPadding:         4
    fxToggleTrackW:        32
    fxToggleTrackH:        18
    fxToggleThumbD:        14
    timelineBarH:          12
    synthKnobMin:          28
    synthKnobIdeal:        72
    synthKnobCaptionH:     18
    synthConceptVizH:      50
    synthFocusGraphH:     120
    synthRightCardMinH:    40
    synthHeaderButtonH:    28
    synthHeaderThumbW:    120
    synthSectionMinH:      72
    synthChipH:            26
    synthChipMinW:         64
    synthChipStripH:       34
    synthDetailHeaderH:    34
    chainMiniMeterW:        6
    chainMiniMeterGap:      3
    chainTriggerMarkerW:    3
    chainStageColW:        64
    chainStageRowH:        26
    chainLabelScale:      850
    sidebarLabelScale:   1200
    sidebarValuePillW:     50
    audioPillH:            24
    audioPillGap:           3
    audioPillPadX:         16
    audioPillNarrowW:      18
    audioTabMinW:          28
    audioChannelMinW:      72
    chainContentGap:        8
    synthPreviewTargetW:  280
    synthPreviewMinW:     160
    samplerWaveMinH:       36
    samplerMinBodyH:       48
    audioLabelMarginW:     28
    chainReadoutScale:    800
    chainPillScale:       850
    chainBadgeScale:      900
    chainScopeLeftMargin:  34
    chainScopeBottomMargin: 18
    chainLegendStripH:     16
    chainMiniWaveH:         7
    chainAutoFitMinMs:      2
    levelsReadoutWFull:   180
    levelsReadoutWIcons:   36
    levelsReadoutFooterH:  16
    waveCursorStroke:       1
    samplerPlayheadStroke:  4
    spectrumCursorStroke:   1
    spectrumBracketH:      12
    knobStepBadgeW:        44
    knobStepBadgeH:        18
    mobileWheelW:         220
    mobileWheelH:         300
    mobileWheelTickGap:    34
    mobileWheelResStripW:  40
    mobileWheelPillH:      38
  spacious:
    rowHeight:             44
    grabZone:              16
    minTarget:             44
    transportBtnSize:      44
    rowControlBtnSize:     36
    headerMinH:            56
    headerMaxH:            56
    eqSliderH:             28
    eqHandleRadius:        26
    eqDBInputH:            20
    sliderThumbH:          24
    sliderThumbW:          14
    sliderTrackH:           6
    nodeMinPx:             12
    nodeMaxPx:             20
    edgeThickMul:           2
    accentStripeWidth:      5
    accentStripeInsetY:     2
    popupBtnW:             44
    popupBtnH:             36
    popupGap:               6
    popupPad:              10
    popupSectionGap:        8
    closeButtonSize:       28
    splitterHandleLen:     56
    splitterHandleThk:      6
    splitterGrabThreshold:  8
    controlGap:             4
    controlGroupPad:        4
    controlPadding:         4
    fxToggleTrackW:        40
    fxToggleTrackH:        22
    fxToggleThumbD:        18
    timelineBarH:          14
    synthKnobMin:          40
    synthKnobIdeal:        88
    synthKnobCaptionH:     22
    synthConceptVizH:      58
    synthFocusGraphH:     140
    synthRightCardMinH:    44
    synthHeaderButtonH:    44
    synthHeaderThumbW:     80
    synthSectionMinH:      96
    synthChipH:            32
    synthChipMinW:         72
    synthChipStripH:       40
    synthDetailHeaderH:    44
    chainMiniMeterW:       10
    chainMiniMeterGap:      4
    chainTriggerMarkerW:    4
    chainStageColW:        72
    chainStageRowH:        36
    chainLabelScale:     1100
    sidebarLabelScale:   1300
    sidebarValuePillW:     56
    audioPillH:            30
    audioPillGap:           4
    audioPillPadX:         20
    audioPillNarrowW:      22
    audioTabMinW:          34
    audioChannelMinW:      84
    chainContentGap:       10
    synthPreviewTargetW:  320
    synthPreviewMinW:     180
    samplerWaveMinH:       44
    samplerMinBodyH:       56
    audioLabelMarginW:     32
    chainReadoutScale:   1000
    chainPillScale:      1050
    chainBadgeScale:     1050
    chainScopeLeftMargin:  48
    chainScopeBottomMargin: 26
    chainLegendStripH:     22
    chainMiniWaveH:        12
    chainAutoFitMinMs:      2
    levelsReadoutWFull:   220
    levelsReadoutWIcons:   34
    levelsReadoutFooterH:  20
    waveCursorStroke:       3
    samplerPlayheadStroke:  5
    spectrumCursorStroke:   3
    spectrumBracketH:      16
    knobStepBadgeW:        52
    knobStepBadgeH:        22
    mobileWheelW:         240
    mobileWheelH:         340
    mobileWheelTickGap:    38
    mobileWheelResStripW:  44
    mobileWheelPillH:      44
typography:
  # Chrome text renders through the embedded Inter faces (Inter-SemiBold for
  # the bold roles, Inter-Regular for caption). The legacy Ebiten debug font is
  # only the -tags test / font-load-failure fallback. Sizes below are the true
  # render px of the sized text-role API (TextRole in internal/ui/textcache.go);
  # the runtime renders each sprite at this px directly (no GeoM upscale).
  panel-title:
    fontFamily: Inter SemiBold
    fontSize: 21px
    fontWeight: 600
    lineHeight: 1.2
  section-header:
    fontFamily: Inter SemiBold
    fontSize: 16px
    fontWeight: 600
    lineHeight: 1.4
  body:
    fontFamily: Inter SemiBold
    fontSize: 14px
    fontWeight: 600
    lineHeight: 1.4
  button-label:
    fontFamily: Inter SemiBold
    fontSize: 16px
    fontWeight: 600
    lineHeight: 1.0
  caption:
    fontFamily: Inter
    fontSize: 12px
    fontWeight: 400
    lineHeight: 1.4
  tooltip:
    fontFamily: Inter
    fontSize: 12px
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
  xxs: 4px
  xs: 6px
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
  scrollbar-thumb-mobile: 50  # mobile scrollbar thumb (taller, brighter; 6px-wide track shared by all mobile scrollbars — see Mobile vs. Desktop table)
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
  splitter-hover:         240 # splitter handle on hover (just below opaque)
  panel-near-opaque:      250 # floating panel BG (just below opaque)
  beat-group-alt:           4 # drum beat-group alternating tint (white over surface)
  menu-active-tint:        60  # active/selected menu-row fill (primary over panel)
  menu-header-accent:      50  # 1px accent hairline under a menu's slim title row

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
  # Button cushion-depth geometry (see "Elevation & Depth" prose). The
  # interactive-button affordance renders a soft outer accent-glow ring at
  # rest/hover and an inner "pressed-in" shadow on press, plus a springy
  # press/release scale. These four scalars drive ButtonStyle.DrawAnimated
  # in components.go. They are a button-only affordance — panels stay flat.
  # button-inner-shadow-px: inner pressed-in shadow inset thickness (px),
  #   consumed by drawButtonInnerShadow in button_cushion.go.
  # button-press-scale: scale floor while held (1.0 → 0.92, springy).
  # button-release-overshoot: spring overshoot peak on release (→ 1.0).
  button-inner-shadow-px: 2
  button-press-scale: 0.92
  button-release-overshoot: 1.03
  # button-hover-glow-spread: how far (px) the hover bloom expands beyond the
  #   button rect. The hover overlay (hover_glow_overlay.go) strokes a soft
  #   primary-bright bloom at r.Inset(-spread) under the crisp 2-px ring, so
  #   the hover affordance reads clearly (a visible neon lift) rather than a
  #   hairline. Consumed by drawButtonHoverGlow in button_cushion.go.
  button-hover-glow-spread: 3
  # Mechanical-keycap geometry (see "Elevation & Depth" prose). The keycap
  # affordance extrudes a cap above a darker side-wall/socket; pressing
  # travels the cap straight down until it bottoms out.
  # keycap-wall-depth: rest side-wall depth in px == max press travel.
  #   Slim (2px) for the matte retro-analogue restyle (2026-06-17): keys keep
  #   a little physical pop but read flat-matte, not as floating keycaps.
  keycap-wall-depth: 2
  # keycap-active-raise: px a latched/active key sits raised above rest.
  keycap-active-raise: 1
  # keycap-release-overshoot-frac: fraction of wall-depth the cap springs
  #   UP past rest on release before settling (analogue overshoot).
  keycap-release-overshoot-frac: 0.18

  # transport-min-btn-w: WCAG touch-min floor (px) used by safeInsetTransport
  #   so a button never shrinks below a tappable width. Consumed in
  #   transport_zone.go.
  transport-min-btn-w: 48

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
  # Button press spring — after a press/release the scale relaxes back to
  # 1.0 via exp-decay of the (scale-1.0) offset. Drives ButtonStyle press
  # animation in components.go (paired with geometry.button-press-scale /
  # button-release-overshoot). Rate 0.6 gives a snappy, slightly springy
  # settle; threshold 0.01 drops the animation when within ~1% of rest.
  button-press-decay:
    kind: exp-decay
    rate: 0.6
    threshold: 0.01
  # Toggled/active-button glow pulse — the persistent accent-glow ring on a
  # toggled-on button breathes via |sin|. Drives the active-pill / active
  # toggle glow in audio_sticky_bar.go + components.go.
  button-toggle-pulse:
    kind: sin-pulse
    frame-step: 0.08
    amplitude: 0.5
    base: 0.5
    alpha-scale: 110
  # Resting/hover outer-glow alpha ceiling (fade factor, like
  # signal-glow-outer). The static cushion glow drawn behind every
  # interactive button at rest; hover lifts it via the brightness delta.
  button-glow-rest:
    kind: fade
    factor: 0.25
  # Hover/press-glow fade spring — on desktop, when the cursor enters a button
  # its primary-bright gold glow ring eases in from 0 to the button-glow-rest
  # ceiling, and eases back out on leave, so the Vice City neon lift glides
  # rather than popping. Touch has no hover, so on mobile the SAME lift is
  # press-driven (the held control lifts, fades out on release) — the affordance
  # renders on both platforms (visual styling does not diverge by screen class).
  # Drives (*Button).AdvanceHoverAnim in uigrid.go: each
  # frame the 0..1 hover progress relaxes toward its target (1 hovered, 0 not)
  # by multiplying the offset by `rate`, settling when within `threshold`.
  # rate 0.65 → ~0.2s glide @60fps (a touch gentler than button-press-decay's
  # snappy 0.6); intentionally subtle, no ongoing motion once settled.
  button-hover-fade:
    kind: exp-decay
    rate: 0.65
    threshold: 0.01
  # One-shot brightness flash fired when a key latches ON (off→on). Decays
  # to silence so a steadily-on key does zero per-frame work. Drives
  # Button.engageAnim via DecayStep in button_cushion.go.
  button-engage-flash:
    kind: exp-decay
    rate: 0.82
    threshold: 0.02

components:
  # Primary FAB (mobile "+", primary CTA) — icon-only at runtime; the
  # foreground is the dark background color rendered against the sunset-gold
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
    textColor: "{colors.background}"
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
  #
  # Menus are minimalist-flat: rows sit directly on the single panel surface
  # (no per-row keycap pad/bevel, no group-container boxes). Group breaks use a
  # 1px hairline separator. The owning instrument's color tints the title swatch,
  # the active-row fill + left stripe, and the hover fill; at-rest rows are
  # neutral. Destructive items keep `error` red text.
  #
  # Every menu TITLE is unified (one shared drawMenuTitle): the section-header
  # role (16 px) for the label + a leading instrument swatch (rounded square,
  # menuTitleSwatchSz) when instrument-scoped (row context menu, node pop-up);
  # global titles (overflow "File", color picker) use the same role with no
  # swatch. Title font, size, color, and iconography are 100% consistent.
  #
  # Every menu ROW: a small leading icon (icon-sm desktop / icon-md mobile) at
  # the row's leading edge, label offset to its right (shared menuRowIconRect /
  # menuRowLabelX) so icon and label never overlap on any platform.
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

  # Splitter handle (mobile uses primary-dim sunset-gold)
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

  # Numeric stepper (BPM ±, Length ±) — neutral, identical recipe to
  # button-secondary so the +/- steppers read as ordinary chrome, not a
  # distinct cluster.
  button-stepper:
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

  # Desktop Play / Stop — RETIRED candy-fill styles. The 2026 neutral
  # transport unification renders Play/Stop as button-secondary bodies with
  # the icon carrying the semantics (see layout_profile.go desktopProfile).
  # Tokens + specs retained for legacy reference (PlayButtonStyle in
  # theme.go); do not wire into new chrome without a design decision.
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
    textColor: "{colors.background}"
    border:
      color: "{colors.stop-desktop-border}"
    rounded: "{rounded.md}"
    height: "{spacing.btn-md}"

  # Dropdown control — secondary surface with a primary-tinted border at
  # alpha 50. The sunset-gold tint signals "this opens a list".
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
    textColor: "{colors.background}"
    border:
      color: "{colors.border}"
      alpha: border-default
    rounded: "{rounded.sm}"
    height: "{spacing.btn-md}"

  # EQ filter (HPF / LPF) toggle — active state is sunset-gold; uses inverted
  # background-on-gold text (same pattern as button-primary FAB) so
  # WCAG AA contrast holds against the gold fill. The two-letter "HP"
  # / "LP" labels read crisply as dark glyphs on the gold chiclet.
  button-eq-filter-active:
    category: button
    backgroundColor: "{colors.eq-filter-active-fill}"
    textColor: "{colors.background}"
    border:
      color: "{colors.eq-filter-active-border}"
    rounded: "{rounded.sm}"
    height: "{spacing.btn-md}"

  # Transport "follow-on" — locked-scroll indicator. Neutral surface-3 fill
  # with a standard border so the indicator reads as a filled neutral surface
  # rather than a gold-bordered one.
  button-transport-follow-on:
    category: button
    backgroundColor: "{colors.surface-3}"
    textColor: "{colors.on-surface}"
    iconColor: "{colors.on-surface}"
    border:
      color: "{colors.border}"
      alpha: border-thin
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
    color: "#FF7A3D"
  - id: snare
    color: "#EE00DD"
  - id: hihat
    color: "#FFD319"
  - id: tom
    color: "#8C1EFF"
  - id: clap
    color: "#00C8E0"

# Fallback palette for any instrument ID not in `instrumentDefaults:`.
# Cycled in source order, with state preserved at runtime so two unknown
# instruments don't collapse onto the same color on the same canvas.
# Five entries chosen to extend the `instrumentDefaults` family without
# overlapping hues; if a sixth or seventh instrument lands, add an
# entry here rather than minting a brand-new chrome color token.
instrumentFallbackPalette:
  - color: "#3FD67A"
  - color: "#FF6AD5"
  - color: "#2A5FD6"
  - color: "#C24FFF"
  - color: "#FFB30A"

# Curated suggestion swatches for the per-row instrument color picker.
# The 35 Vice City ramp members (seven families x five steps) designed to
# read against the lifted slate ladder without bleed-through.
#
# IMPORTANT: this block is a *recommendation surface*, not a constraint.
# The color-picker (`drumview_color_menu.go`) renders these as suggested
# swatches above the free hue wheel; users can still pick arbitrary hex.
# The chrome runtime ignores this block entirely. Instrument identity is
# the ONLY multi-color signal in the app — see the "Instrument identity
# is the only multi-color signal" invariant. This is where the "fun"
# lives; the chrome stays restrained.
#
#
# Stitch lint flags this as an orphan top-level key (it isn't part of
# the canonical Stitch schema); the warning is absorbed in
# `testdata/design_md_lint.expected.json` with rationale.
instrumentSwatches:
  - name: sunset-gold-100
    hex: "#FFE98A"
  - name: sunset-gold-200
    hex: "#FFD319"
  - name: sunset-gold-300
    hex: "#FFB30A"
  - name: sunset-gold-400
    hex: "#FF9E1F"
  - name: sunset-gold-500
    hex: "#F58A12"
  - name: tangerine-100
    hex: "#FFC299"
  - name: tangerine-200
    hex: "#FF9E54"
  - name: tangerine-300
    hex: "#FF7A3D"
  - name: tangerine-400
    hex: "#FF5C2A"
  - name: tangerine-500
    hex: "#E84A1F"
  - name: hot-pink-100
    hex: "#FFA8E0"
  - name: hot-pink-200
    hex: "#FF6AD5"
  - name: hot-pink-300
    hex: "#FF2D9E"
  - name: hot-pink-400
    hex: "#EE00DD"
  - name: hot-pink-500
    hex: "#C400B5"
  - name: orchid-100
    hex: "#E0A8FF"
  - name: orchid-200
    hex: "#C24FFF"
  - name: orchid-300
    hex: "#A030FF"
  - name: orchid-400
    hex: "#8C1EFF"
  - name: orchid-500
    hex: "#6E12E0"
  - name: electric-blue-100
    hex: "#A8C8FF"
  - name: electric-blue-200
    hex: "#5E8AFF"
  - name: electric-blue-300
    hex: "#3A6AF0"
  - name: electric-blue-400
    hex: "#2A5FD6"
  - name: electric-blue-500
    hex: "#1E47B5"
  - name: cyan-100
    hex: "#9AF0FF"
  - name: cyan-200
    hex: "#3FE0E8"
  - name: cyan-300
    hex: "#00C8E0"
  - name: cyan-400
    hex: "#00A8C9"
  - name: cyan-500
    hex: "#0E7391"
  - name: mint-100
    hex: "#B3F5C2"
  - name: mint-200
    hex: "#6BE89A"
  - name: mint-300
    hex: "#3FD67A"
  - name: mint-400
    hex: "#2BC85C"
  - name: mint-500
    hex: "#1FA84A"

# The canonical instrument color SEQUENCE — the single, predictable, finite
# series from which every circuit's instrument colors (and therefore node +
# edge colors) are derived. Unlike `instrumentSwatches:` (an unordered
# recommendation surface) this block defines a deterministic ORDER: row N of
# any demo circuit or genre template takes `instrumentSequence[N % len]`.
#
# Each entry is the NAME of a swatch declared in `instrumentSwatches:` above —
# so the hexes stay single-sourced and the whole series is provably on-palette.
# The ordering is deliberately warm↔cool so adjacent rows stay legibly
# distinct. The generator resolves these names and emits:
#   - `genInstrumentSequence []color.RGBA`   (internal/ui, drives row colors)
#   - `templates.InstrumentSequence []string` (internal/templates, baked JSON)
# Both consumers walk the SAME series; there are no hand-maintained hex lists.
#
# Stitch lint flags this as an orphan top-level key; the warning is absorbed in
# `testdata/design_md_lint.expected.json` with rationale (same as the swatches).
instrumentSequence:
  - hot-pink-400
  - cyan-300
  - sunset-gold-200
  - orchid-300
  - mint-300
  - electric-blue-300
  - tangerine-300
  - hot-pink-200
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

A four-level neon-night surface hierarchy with a single sunset-gold chrome
accent and three distinct red roles. Every color is a token; never use a hex
literal in code.

### Surface hierarchy

The base palette nests in order of elevation. Surface-1 contains surface-2
buttons; never place a lower-level surface inside a higher-level container.
The ladder lifts in violet-black steps above the `#120A1C` night base so
chrome reads as touchable matte plastic against the Vice City horizon.

- **`background` (`#120A1C`)** — Grid pane, timeline canvas (deepest level).
- **`surface-1` (`#1C1230`)** — Row rack, transport bar background.
- **`surface-2` (`#281A40`)** — Cards, buttons, input fields (elevated).
- **`surface-3` (`#382354`)** — Hover states, active surfaces (raised).
- **`surface-overlay` (`#1F1438`)** — Overlay panels, popups, bottom sheets.
  Drawn at 250/255 alpha against the scrim — opacity is applied in code by
  `drawPanel(...)`, not encoded in the token (the spec only allows opaque
  colors).

### Accent (two-accent system: sunset-gold + hot-pink)

The chrome carries two accents: sunset-gold (`primary`, `#FFB30A`) for every
"active / selected / active-surface / hover-emphasis" state, and hot-pink
(`focus-ring`, `#FF2D9E`) for focus/selection rings. No *third* chrome
accent (no parallel hue, no panel-state-dependent recolor) is allowed —
see the chrome-accent invariants below. The sunset-gold chrome accent is the
warm repaint of the prior cyan/azure accent — yellow pops harder against the
violet-black night surfaces; the hot-pink focus accent is the second member
of the system. The remaining hue ramps belong to instrument
identity, semantic state, and data-visualization, never to a competing
chrome accent.

- **`primary` (`#FFB30A`)** — Primary interactive / active state.
- **`focus-ring` (`#FF2D9E`)** — Focus / selection ring; the second accent.
- **`primary-bright` (`#FFD319`)** — Hover ring; never decorative.
- **`primary-dim` (`#FF9E1F`)** — Splitter handle, slider fill, HP/LP
  active fill, secondary emphasis.
- A "subtle" variant of `primary` (8% alpha) is used for backgrounds and
  fills — applied at draw time as `withAlpha(TokenAccent(), 20)`. There is
  no separate token for it.

### On-surface text

- **`on-surface` (`#FDF2FF`)** — Main labels, button text.
- **`on-surface-muted` (`#C9B8D6`)** — Captions, secondary labels, inactive
  icons.
- **`on-surface-disabled` (`#8A7A9C`)** — Grayed-out, non-interactive.
- **`on-surface-accent` (`#FFB30A`)** — Active section headers, links. Same
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

- **`error` (`#FF5C2A`)** — Vermillion. Stop / error **text** only. Stop
  button icon tint, error toasts, parity warnings. Never a button fill.
- **`mute` (`#C400B5`)** — Deep magenta. Legacy mute-state badge fill,
  retained as a semantic token (still wired to
  `ComponentButtonRowControlMuteActive`). **Note:** the per-row mute/solo/FX
  controls in the instrument rack no longer use a fixed red — they derive from
  the row's **instrument color** (see "Per-instrument control shading" below).
  Do not repurpose this token for a different role.
- **`destructive` (`#E84A1F`)** — Ember. Destructive button **fill** only
  (delete row, "Delete" context menu item). Paired with
  **`destructive-border`**.

Never reuse a red variant for a different semantic role. Do not introduce a
fourth red.

The green family follows the same logic: `success` (`#3FD67A`) is the play
icon tint (mobile) and play-active state; `play-desktop-fill` (`#2BC85C`) is
a retained legacy token for the retired candy-fill desktop play button (see
"Desktop Play / Stop" under Components — the shipped transport is neutral).

### Chrome accents — three load-bearing invariants

These three invariants travel together. A change that violates one tends
to violate the others. Memorize them; the visual coherence of the whole
app collapses if any of them slips.

1. **Two chrome accents, no more.** Every "active / selected /
   hover-emphasis" state in chrome resolves to the sunset-gold accent — `primary`
   (`#FFB30A`), `primary-bright` (`#FFD319`), or `primary-dim`
   (`#FF9E1F`) — while focus/selection rings use the hot-pink accent
   `focus-ring` (`#FF2D9E`). No *third* chrome accent is allowed — no
   parallel hue, no panel-state-dependent recolor. The visualization
   tokens (`viz-bar`, `viz-bar-peak`, `viz-wave-a`, `viz-wave-b`) belong
   **only** to data visualizations (spectrum bars, wave traces). If a
   chrome control's selected state uses anything other than the sunset-gold
   accent tokens (or a focus ring using anything other than `focus-ring`),
   that is a bug. **Exception:** controls and menus that belong to a single
   instrument resolve to that instrument's color, not the sunset-gold accent — this
   is instrument *identity*, not chrome (see invariant 3, "Per-instrument
   control shading").

2. **Panel state never recolors chrome.** Opening the FX panel must not
   change the EQ panel's accent. Opening the inspector sidebar must not
   change the row-rack chip color. If a token's color depends on which
   panel is open, it is the wrong token — the panel-open code path
   should toggle a *border alpha* or a *fill brightness*, never swap
   the underlying hex.

3. **Instrument identity is the only multi-color signal.** Drum cells,
   graph nodes, edge tints, the per-row instrument color swatch, **and the
   per-row controls that belong to a single instrument** carry the row's
   identity (free-hex per instrument; the `instrumentSwatches` block names a
   curated suggestion set). The instrument-identity surfaces are:
   - the rack's per-row **mute / solo / FX** toggles (shades of the
     instrument color — see "Per-instrument control shading" below) and the
     per-row **ellipsis/kebab** chip;
   - the **FX-panel** for that row: parameter slider rails **and** the
     per-effect enable-toggle pill tint to the instrument color (the *Remove*
     button keeps its destructive red);
   - the per-instrument **menus**, whose owning instrument drives every
     state-highlight (header underline, active-item stripe, hover/selected
     accent, color dot/swatch) while at-rest button fills stay neutral:
     - the **kebab context menu** (owner = the row's instrument);
     - the **instrument picker** (owner = `CurrentInstrument`; each instrument
       *row* additionally shows its own per-id color, and the filled favorite
       star uses that instrument's color);
     - the **node pop-up** — desktop sidebar (header band, section stripes,
       expanded chevron/label, ± steppers) and mobile long-press (Move/Connect
       hover) — owned by the node's row via `nodeRows` (*Delete* stays red).

   Destructive **Delete / Remove** buttons keep the semantic red safety signal
   even inside instrument-colored menus; the instrument color drives
   highlights/accents, never the destructive role.

   No *chrome* surface uses the instrument color set. The chrome accents
   (sunset-gold + hot-pink focus ring), visualization tokens (viz golds / lime /
   peak markers), and semantic state (reds / greens) are each their own
   closed family; instrument color sits outside all of them.

   **Per-instrument control shading.** Mute = the instrument color
   *darkened* (deeper shade); solo = the instrument color *brightened*
   (lighter shade); FX-on = the full instrument hue (with a brightened-shade
   badge); any *off* state = a dim low-alpha tint of the hue plus a thin hued
   border. The M / S / FX letters carry the semantic meaning; lightness
   separates the active states. All shades derive from the one instrument
   color via the `adjustColor` / `WithAlphaFromColor` helpers
   (`row_instrument_shades.go`) — no per-state hex literals.

### Color usage (Vice City palette)

Every color in the UI comes from the Vice City palette declared in the
`colors:` front matter — seven neon hue ramps (sunset-gold, tangerine,
hot-pink, orchid, electric-blue, cyan, mint; five shades each) plus the
neon-night neutral ramp (violet-black surfaces, faint-magenta near-white
text) and the two achromatic anchors (`#FFFFFF` / `#000000`, used only via
alpha). The `TestPaletteMembership` guard fails the build on any off-palette
token.

**Two-accent system.** Chrome uses two accents: sunset-gold (`primary`,
`#FFB30A`) for interactive/selected/active surfaces, and hot-pink
(`focus-ring`, `#FF2D9E`) for focus/selection rings. The sunset-gold accent
covers every interactive / selected / active chrome surface (with
`primary-bright` `#FFD319` for hover and `primary-dim` `#FF9E1F` for muted
fills); it *repaints* the prior cyan accent onto the Vice City sunset-gold
ramp — yellow pops harder against the violet-black night surfaces, role
unchanged. The hot-pink `focus-ring` is the second
accent — it sits on the focused-`TextInput` / selection ring so focus
reads as its own signal, distinct from (and never competing with) the
sunset-gold interactive accent.

**Three reds, repainted.** `error` = vermillion (`#FF5C2A`, stop/error text
only), `destructive` = ember (`#E84A1F`, delete fills), `mute` = deep
magenta (`#C400B5`, mute fill). Distinct roles, all on-palette — see
"Semantic state" above for the role boundaries.

**Contrast — two safe patterns only.** (1) near-white / near-black text on
a dark surface, or (2) near-black `#120A1C` label on a neon fill. Never
mid-neon text on mid-neon. Targets: text & icons ≥ 4.5:1; large text &
graphical / control strokes ≥ 3:1. Enforced by `TestColorContrast` and
`TestDesignMDContrast`.

**Controls.** Filled controls = neon fill + `#120A1C` label/icon
(dark-on-neon — the `button-primary` FAB uses `textColor: {colors.background}`
which resolves to `#120A1C`). Secondary controls = ghost outline (neon
stroke + neon label on a dark surface). Icons default to `on-surface`,
flipping to `#120A1C` when sitting on an active neon fill.

**Swatches.** Pale instrument / picker swatches always carry a 1px low-alpha
`#FFFFFF` border so light shades don't vanish on the dark background.

**Glow is decoration, never contrast.** Neon bloom (glow ring / drop
shadow) is aesthetic only; legibility must hold with glow removed.

### Record states

- **`record-idle` (`#E84A1F`)** — Record button at rest.
- **`record-active` (`#FF5C2A`)** — Record button while recording. Both
  drawn at 80% alpha in code for a subtle pulse.

### Visualization tokens

The Wave / Spectrum / Meters / Scope / EQ canvases share these:

- **`viz-bg` (`#120A1C`)** — Canvas background; the night base.
- **`viz-curve` (`#FFB30A`)** — EQ curve stroke (= `primary`); plus 8% fill
  below to a derived `viz-curve-fill` applied in code.
- **`viz-bar` (`#FFB30A`)**, **`viz-bar-peak` (`#FFE98A`)** — Spectrum bars
  rise from canvas baseline; peak hold in pale gold.
- **`viz-wave-a` (`#FFD319`)** — Scope tap A trace, Wave primary trace.
- **`viz-wave-b` (`#8A7A9C`)** — Scope tap B trace; split / diff secondary.

When adding a new visualization, prefer reusing the existing tokens.
Introduce a new token only if the role is genuinely distinct.

## Typography

All chrome text renders through the **embedded Inter faces** — Inter-SemiBold
for the bold roles, Inter-Regular for caption — drawn via the sized text-role
API (`TextRole` / `DrawTextStyled` in `internal/ui/textcache.go`), which
rasterizes each sprite at its true target px (no GeoM upscale, so glyphs stay
crisp). The bundled Ebiten debug font is only the `-tags test` /
font-load-failure fallback. (Row-rack controls render vector `IconID`
glyphs, not text — see Components → "Row control".)

Six typography roles, each a token. The four chrome roles below map 1:1 to the
`TextRole` enum (`RolePanelTitle` / `RoleSectionHeader` / `RoleBody` /
`RoleCaption`); read them via `DrawTextStyled(dst, s, x, y, role, col)` and size
layout with `StyledTextWidth` / `StyledTextHeight`:

- **`panel-title`** (21 px, Inter SemiBold) — Inspector / FX large panel header.
- **`section-header`** (16 px, Inter SemiBold) — Collapsible section labels **and
  every menu title** (row context menu, node pop-up, overflow "File", color
  picker). All menu titles share this one role + a leading instrument swatch
  (rounded square, `menuTitleSwatchSz`) when scoped to an instrument — so title
  font, size, color, and iconography are identical across menus. See the
  "Menus are minimalist-flat" note under `components`.
- **`body`** (14 px, Inter SemiBold) — Default labels, menu items, values.
- **`button-label`** (16 px, Inter SemiBold) — Row controls, transport labels.
- **`caption`** (12 px, Inter Regular) — Sub-labels, percentages, BPM number.
- **`tooltip`** (12 px, Inter Regular) — Cursor labels, coordinates (desktop).

Prefer the role API (`DrawTextStyled` + `StyledText{Width,Height}`) over the
legacy `DrawTextAtScale` GeoM-upscale path for any new chrome text.

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

Beatmo uses **cushioned tonal layers + thin borders + a matte
hardware-bevel press affordance on interactive buttons (all platforms)**.
No *Material-style offset drop shadows* on solid surfaces; panel overlays
use a subtle shadow + panel-border. Interactive **buttons** carry a slim
matte keycap (flat fill framed by a faint light top edge + a soft dark
bottom lip; cap travels down on press, springs back with overshoot; an
active/latched key looks pressed-IN and lit amber) — non-interactive
surfaces (panels, cards, rows) stay flat. The mental model is **matte
hardware on a vintage synth/mixing-console panel**: depth comes from edge
contrast, NOT from gloss, sheen, specular shine, or neon glow (retro-
analogue restyle, 2026-06-17). Earlier revisions layered a sheen band, a
crisp specular line, a cyan hover-glow ring, a `|sin|` toggle pulse, and a
cyan engage flash; those are all removed.

### Surface elevation cues

The four-level surface hierarchy in "Colors" *is* the elevation system. A
button on `surface-1` rises by sitting on `surface-2`; hovering lifts it
to `surface-3`. There is no shadow between surface levels — elevation is
purely tonal.

### Keycap elevation (all platforms)

Every discrete button (`Button.Draw`) renders as a slim matte keycap: a
dark **socket/side-wall** (`drawKeycapShell`, depth = `keycap-wall-depth`,
slim at 2px) extruded behind a flat **cap face** (`keycapCapRect`) framed
by a **matte bevel** (`drawKeycapBevel`): a faint 1-px light top edge + a
soft dark bottom-inner-shadow lip — no sheen band, no specular shine. At
rest the cap sits slightly raised above the socket. On press the cap
**travels straight down** (eased via `pressDepth`/`AdvancePressAnim`) until
it bottoms out against the socket floor. On release it **springs back**
past rest by a small overshoot fraction (`keycap-release-overshoot-frac`)
before settling — an analogue bounce, not a digital snap. A **contact
shadow** (`drawKeycapContactShadow`) is drawn beneath the cap when it is
raised, disappearing as the cap bottoms out. This geometry replaces the old
press-scale shrink; the scale tokens (`button-press-scale`,
`button-release-overshoot`) are retained in the YAML for legacy reference
but no longer drive the visual.

**Knobs** carry the same matte light model: a **knurled volumetric cap**
(`knurledCapSprite`, cached per diameter) with a ridged rim, matte concentric
dome (no specular highlight), side wall, and contact shadow sits behind the
existing value arc and pointer. **Slider thumbs** render a matte cap
(contact shadow + flat body + a neutral edge rim that strengthens while
dragging); the old top specular highlight, the cyan drag rim, and the rail
outer-glow stroke are all gone.

Desktop chrome stacks one additional cue per-control. Mobile chrome stays
flat (no hover lift); it gets generous rounding instead.

1. **Tonal lift** — surface-2 → surface-3 on hover (already covered
   above).
2. **Hover lift** — a flat, uniform ~8% white brighten
   (`keycapHoverLiftAlpha`) over the hovered clickable control (buttons,
   text inputs like the BPM box, the volume icon/slider, and similar) that
   **eases in** when the cursor enters and **fades out** on leave, so the
   lift glides rather than popping. NOT a rim, specular catch, or bloom —
   just a quiet "this is reachable" brighten. It is a single per-frame
   **overlay** (`hover_glow_overlay.go`, drawn by `DrumViewTree.Draw` on
   top of all zones, beneath portals): each frame it asks the live
   `HitIndex` which clickable control is under the cursor (resolving a hit
   via the `glowTarget` interface, which yields the control's rect and a
   suppress flag — implemented by the button, text-input, volume-icon, and
   slider-group hit adapters; drag-only surfaces like knobs, the timeline
   scrub, and grid drag deliberately opt out), eases a 0..1 progress toward
   "over a control?" using the `button-hover-fade` token (exp-decay), and
   paints the lift via `drawButtonHoverGlow`. A focused (being-edited) text
   input is suppressed so it shows only its own focus ring. An overlay —
   not `Button.Draw` — is required because (a) the input dispatcher only
   runs on press/drag/release, so a button never learns the cursor is
   merely *over* it, and (b) zones render their buttons into state-hashed
   sprite caches, so `Button.Draw` isn't called per frame; the overlay
   sidesteps both by drawing outside every cache. Desktop-only: the overlay
   is a no-op on a mobile profile, so a tap never flashes. The surface-2 →
   surface-3 tonal lift ships separately via the `interaction.hover`
   brightness delta on `ComponentSpec`.

### Active "latched amber" state (recessed)

A latched/active button (e.g. a selected audio-tab pill, a toggled
transport key) renders as a **recessed lamp-amber keycap** — the cap sits
flush (raised=false) with an **inverted bevel** (`drawKeycapActiveInset`:
top inner-shadow + faint bottom light edge) so it reads as **pressed-IN**,
filled with a solid warm amber (`keycapActiveFill`, the on-palette
sunset-gold `#FFB30A`) and a dark legend (`{colors.background}`) for
contrast. The amber is a SOLID fill, never a glow. There is **no engage
flash and no toggle pulse**: the static recessed-amber cap is the only
active signal, so a latched key does **zero per-frame work**. The
`engageAnim` field + `button-engage-flash` / `button-toggle-pulse` tokens
and the `drawKeycapEngageFlash` no-op are retained for the press-spring
contract/tests but paint nothing.

Amber-as-active-fill now shares its hue with the chrome accent: the
`primary` / `on-surface-accent` accent was repainted from cyan to this same
sunset-gold `#FFB30A`, so a latched keycap and an active chrome surface read
as one coherent warm-gold "this is on" signal (hot-pink `focus-ring` stays
the second, focus-only accent). It is not a free hue — it reuses an existing
sunset-gold palette member, so `TestPaletteMembership` stays green. Semantic-colored toggles (mute / solo /
record) keep their own accent colors but gain the same recessed keycap
volume.

The hover overlay skips active buttons entirely (no double-lift). Both idle
latched keys and settled pressed keys perform zero ongoing animation work
once their transients have decayed.

### WASM idle budget rule

All static volume — cap body, socket, contact shadow, amber latched fill —
bakes into per-state **sprite caches** so `Button.Draw` only blits a
pre-rendered texture when nothing is changing. All **motion** (press travel,
overshoot spring, hover fade) is a cheap **blit-offset + alpha overlay**
that writes nothing to the cache. A control that is idle (not pressed, not
being hovered) or steadily latched (recessed amber, settled) performs **zero
per-frame allocation and zero per-frame draw work** — the matte restyle
removed the engage flash and toggle pulse, so a latched key now does
literally nothing per frame. This is a hard discipline enforced by the
alloc-rate + settle-to-silence tests in `button_keycap_alloc_test.go`.

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
- Both platforms: `DrawTopEdgeHighlight = true` (subtle 3D depth). The depth
  highlight does not diverge by screen class — only layout & density may.
- Go styles: `TransportPlayStyle`, `TransportStopStyle`, `TransportIncStyle`,
  `TransportDecStyle`, `TransportMiscStyle`, `TransportFollowOnStyle`,
  `UploadBtnStyle`, `PopupButtonStyle`.

### Row control (inline)

`{components.button-row-control}` — `surface-2` fill, subtle border,
**vector `IconID` glyphs** at 16–24 px.

- **Icons:** `IconMute` (speaker + slash) / `IconSolo` (headphones) /
  `IconFx` (sparkle) / `IconTarget` (Origin crosshair) / `IconClose`
  (Delete). The antialiased vector renderer (1.75-unit stroke, round caps —
  see "Icon renderer") keeps these legible at row-control size; the
  pre-overhaul scanline icons were jagged, which is why this section
  historically mandated text labels. The Icon Map appendix's "Row controls"
  table is the authoritative glyph list.
- Per-instrument shading: inactive/active fills derive from the row's
  instrument color (see "Per-instrument control shading" under Colors), not
  the fixed mute/solo component tokens; those tokens remain wired for the
  legacy `ComponentButtonRowControl*` specs.
- Go style: `InstButtonStyle` (inactive) → `MuteActiveStyle` /
  `SoloActiveStyle` / `FXActiveStyle` (active).

### Destructive (delete / confirm)

`{components.button-destructive}` — `destructive` fill, `destructive-border`
border.

- **Use:** Delete row, "Delete" context menu item, confirm dialogs.
- **Label:** `"X"` text; confirm state uses `"!!"` with
  `{components.button-destructive-confirm}`.
- **Text color:** `background` (dark-on-neon — matches
  `button-destructive.textColor` and the filled-control contrast rule).
  Red text belongs only to destructive *menu items* on a neutral surface
  (`context-menu-item-destructive`), never to labels on a red fill.
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
| Play | `Profile().PlayIconColor` (desktop: `on-surface`, mobile: `success`) | playing: `primary` accent (pause glyph) |
| Stop | `Profile().StopIconColor` (desktop: `on-surface`, mobile: `error`) | — |
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
- **Section toggles:** the shared `drawMenuChevron` helper
  (`IconChevronRight` collapsed / `IconChevronDown` expanded) — the same
  chevron renderer menus use, never `>` / `v` text.
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

On **mobile**, the right-side view tabs are not rendered inside the panel
— the audio-view tabs live in the bottom-navigation segmented strip
(`Pads / EQ / Wave / Spec / Mtr / Scope`) so a single navigation control
selects between rows and every audio sub-view. The left-side filter
chips (`Master / HP / LP / Freeze`) remain inside the panel because they
operate *within* the active audio view. Desktop continues to render both
strips inside the panel (it has the screen real estate and the user
expects the in-panel affordance).

**Inspector section toggles.** Use the shared `drawMenuChevron` helper
(`IconChevronRight` / `IconChevronDown`) already wired in
`game_node_sidebar.go` — one chevron renderer across sidebar sections and
menus. Do not use raw text `>` / `v` / `▶` / `▼` for collapsibles.

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
| `±` | Step-size prefix on knob step badges + wheel step chips (`±100 Hz`) | `knob_step.go` `formatStepValue` | Same rationale as `÷`: a math operator marking the chip as a step size so it never reads as a second value readout. Unit-less steps use the ASCII `x` multiplier prefix instead. |
| `!!` | Row-rack delete-confirm warning | `row_rack_zone.go:1161` | Stateful warning glyph distinct from the resting `IconClose` icon — confirms a destructive action with explicit text rather than a second icon. |
| `OVR` / `SPL` / `DIF` / `AG` / `FIT` | Chain (scope) panel mode / auto-gain / auto-fit pills | `chain_panel_zone.go` | Two/three-letter mode codes are descriptive labels, not glyphs. `AG` is the ONE label for the auto-gain concept everywhere — the Wave tab's pill and its corner badge (`AG x4.2`, `render_waveform_scale.go`) reuse it; never introduce a synonym (the old Wave `AUTO` label is retired). |
| `HP` / `LP` | EQ filter toggles | `eq_panel_zone.go:172–182` | Same rationale as Chain mode buttons. |
| `K20` / `CLR` | Levels tab K-20 view toggle / clear-clips | `audio_tab_controls.go` | Same rationale — K-20 is the metering standard's own name. |
| `RST` / `LOG` / `LIN` | Spectrum reset-hold / freq-scale toggle | `audio_tab_controls.go` | Same rationale. All analyzer pill codes are UPPERCASE; single bare letters are reserved for data labels (the wave panel's `L`/`R` channel tags), never action pills. |

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
| Sidebar section toggles | `icon-md` chevron | **Yes** (chevrons) | no | no | `drawMenuChevron` → `IconChevronRight/Down`, shared with menus. |
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

The icon set has **30 glyphs** defined in `icons.go`. Every icon has
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
| Template card | `IconTemplate` | "Load template" entry + template rows in the overflow menu — never reuse `IconRows` (that glyph means "switch to row view") |
| Pencil | `IconPencil` | Rename / edit |

## Mobile vs. Desktop — Intentional Differences

These differences are **intentional** and must be preserved:

| Aspect | Desktop | Mobile |
|---|---|---|
| Row height | 36 px (`profileOverrides.desktop.rowHeight` = `spacing.btn-md`, fixed) | 44 px (`profileOverrides.mobile.rowHeight`) × `dv.rowZoom` ∈ [0.7, 1.6] |
| Context menu style | Floating panel | Full-width bottom sheet |
| Row controls visible | Vol icon · Mute · Solo · FX · ⋯ kebab (Rename/Color/Origin/Delete in the kebab context menu — floating panel) | Same control set inline; the kebab context menu opens as a full-width bottom sheet. |
| Audio-view tabs | In-panel right-side strip (`EQ / Wave / Spectrum / Levels / Chain / Synth / Sampler`) | Bottom-navigation segmented strip (`Pads / EQ / Wave / Spec / Lvl / Chn / Syn / Smpl`); the EQ panel only carries the left filter chips (`Master / HP / LP / Freeze`). The strip's highlight derives from the panel's `PanelTabState` (single source of truth — programmatic tab switches sync it via `OnTabChange`). |
| Track / follow toggle | Vertical strip left of the timeline | Square chip on the right edge of the timeline ruler header |
| Row zoom | N/A (fixed `desktopRowHeightPx`) | ⊕ / ⊖ chip top-right of the rows zone + 2-finger pinch over `dv.rowsRect()` (in-memory `dv.rowZoom`) |
| Accent stripe | 3 px, flush | 5 px, 2 px vertical inset |
| Scrollbar width | 6 px hairline (`DefaultScrollbarStyle.Width`); popup/menu scrollers 10 px (`DropdownScrollbarStyle.Width`) | 6 px (`mobileScrollbarWidth`) — **every** mobile scrollbar shares this one width (content scrollers via `MobileScrollbarStyle`, popup/menu scrollers via `dropdownScrollbarStyle`), so none reads thicker than another. Touch grab comes from the ≥ 44 px-tall thumb (`MinThumbH`), not the width. |
| Popup text scale | 1.0× | 1.3–1.7× |
| Timeline default | No cap (full circuit) | 8 beats |

All other visual differences are **accidental** and should be unified. The
above are **layout** (arrangement) and **density** (sizing) divergences — the
only two axes allowed to differ by screen class. **Visual styling and animation
never diverge by platform.** Buttons, knobs, sliders, pills, and toggles render
identically on both; toggle buttons (Reverse / Normalize / Fade, EQ mutes, tab
pills, …) show the same latched amber-keycap state everywhere. The button depth
highlight, the hover/press lift (hover-driven on desktop, press-driven on touch
since touch has no hover), the transport-cluster pill, and the recording-armed
ring all render on both platforms.

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

**Scene families** (see `scene_catalog.go` for the authoritative list —
`./beatmo -list-scenes` prints it; names are kebab-case and stable):

**Transport / baseline:** `transport_idle`, `transport_playing`,
`transport_high_bpm`, `transport_recording`, `shortcuts_overlay`.

**Audio-panel tabs:** `eq_tab_eq`, `eq_tab_wave`, `eq_tab_spectrum`,
`eq_tab_levels`, `eq_tab_chain`, `eq_tab_synth`, `eq_with_band_adjusted`,
`eq_hpf_active`, `eq_lpf_active`, `eq_band_muted`,
`eq_chain_custom_settings`, plus the `crop_synth_*` / `crop_sampler_*` /
`crop_chain_*` / `crop_eq_*` subject-cropped scenes.

**Per-row menus / popups:** `context_menu_open`, `instrument_menu_open`,
`instrument_menu_categories`, `color_wheel_open`, `subdiv_menu_open`,
`master_vol_popup`, `desktop_per_row_vol_popup`.

**FX panel:** `fx_panel_open_empty`, `fx_panel_with_3_effects`,
`fx_panel_distortion_only`, `fx_panel_reverb_only`, `fx_panel_delay_only`,
`fx_panel_knob_drawer_open`.

**Graph / sidebar:** `node_added`, `edge_built`, `node_sidebar_open`,
`node_longpress_menu`, `graph_complex_3_nodes_4_edges`,
`node_sidebar_logic_expanded`, `node_sidebar_all_expanded`,
`node_sidebar_groove_expanded`, `node_sidebar_audio_expanded`,
`node_sidebar_node_muted`, `node_sidebar_node_silent`,
`node_sidebar_node_invisible`.

**Multi-row state:** `multi_row_full_grid`, `row_muted_soloed`.

**Playback overlays** (menus + popups during playback):
`playback_context_menu_open`, `playback_instrument_menu_open`,
`playback_color_wheel_open`, `playback_subdiv_menu_open`,
`playback_fx_panel_with_3_effects`, `playback_eq_tab_levels`,
`playback_eq_tab_chain`, `playback_eq_tab_spectrum`,
`playback_eq_tab_synth`.

**Recording overlays:** `recording_with_context_menu`,
`recording_during_playback_overlay`.

**Mobile:** `mobile_default`, `mobile_overflow_open`,
`mobile_overflow_menu_redesigned`, `mobile_view_audio`,
`mobile_per_row_vol_popup`, `mobile_row_inline_controls`,
`mobile_transport_bottom_bar`, the `mobile_bottom_nav_*` tab family
(pads / eq / wave / spectrum / levels / chain / synth / sampler),
`mobile_track_chip_following` / `_free`, `mobile_length_max` / `_min`,
`crop_mobile_synth_wheel`.

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
| `topEdgeHighlight` | `bool` | Enables the 1-px top-edge depth highlight for the secondary button family (both platforms). |
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
