package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

const (
	// Ebiten's debug font uses 6px-wide glyphs in a 16px-tall cell.
	debugCharW = 6  // width of a character drawn by DebugPrintAt
	debugCharH = 16 // height of a character drawn by DebugPrintAt
)

// uiAnimFrame is the shared UI animation frame counter, published once per
// frame by (*DrumView).Draw so button cushion animations (toggle-pulse glow)
// advance without threading a frame argument through every Draw call.
var uiAnimFrame int64

// insetRect returns r shrunk by pad pixels on all sides.
func insetRect(r image.Rectangle, pad int) image.Rectangle {
	if r.Dx() < 2*pad || r.Dy() < 2*pad {
		return image.Rectangle{}
	}
	return image.Rect(r.Min.X+pad, r.Min.Y+pad, r.Max.X-pad, r.Max.Y-pad)
}

// clipTextToWidth trims text so that its rendered width does not exceed maxW.
// It preserves whole runes and appends "..." when truncating. Uses proper font
// metrics when available, falling back to debug font character width.
func clipTextToWidth(text string, maxW int) string {
	return clipTextToWidthMeasure(text, maxW, TextWidth)
}

// clipTextToWidthMeasure is clipTextToWidth with a caller-supplied width
// metric, for text drawn through the styled text-role API (measure with
// StyledTextWidth for the matching role so the clip lands on the real glyphs).
func clipTextToWidthMeasure(text string, maxW int, measure func(string) int) string {
	if maxW <= 0 {
		return ""
	}
	if measure(text) <= maxW {
		return text
	}
	ellipsis := "..."
	rs := []rune(text)
	for keepRunes := len(rs) - 1; keepRunes >= 0; keepRunes-- {
		candidate := string(rs[:keepRunes]) + ellipsis
		if measure(candidate) <= maxW {
			return candidate
		}
	}
	return ellipsis
}

// ButtonVisual is implemented by styles capable of drawing a button.
// pressed indicates the mouse button is currently down; hovered indicates the
// cursor is over the control so styles can provide hover feedback.
type ButtonVisual interface {
	Draw(dst *ebiten.Image, r image.Rectangle, pressed, hovered bool)
}

// Button is a basic clickable component with a rectangular bounds and text label.
type Button struct {
	r    image.Rectangle
	Text string
	// TextKey, when non-empty, makes the button resolve its label from the i18n
	// catalog at draw time so a language switch updates it without
	// reconstruction. Overrides Text. Set via NewButtonKey or SetTextKey.
	TextKey i18n.Key
	Style   ButtonVisual
	OnClick func()
	pressed bool
	hovered bool
	Repeat  bool
	held    int
	// pressDepth is the springy press travel: 0 = rest, 1 = fully bottomed
	// out (cap descended by genGeomKeycapWallDepth px), negative = raised
	// (release overshoot). Eased toward pressTarget each frame by
	// AdvancePressAnim via genAnimButtonPressDecay. Stored on the struct so
	// the animation allocates nothing per frame and settles exactly to 0.
	pressDepth  float64
	pressTarget float64
	// engageAnim is a one-shot 1→0 brightness flash fired when the button
	// latches ON (its active/selected off→on transition, detected via
	// wasActive in drawPillTabAt). Decays via genAnimButtonEngageFlash.
	// Drawn over the cap by drawKeycapEngageFlash.
	engageAnim float64
	// wasActive tracks whether this button was drawn in its active/selected
	// state last frame, so a pill renderer can fire the one-shot engage flash
	// exactly on an off→on transition. Set by drawPillTabAt.
	wasActive bool
	// toggled marks a latched/active button. When set, Button.Draw raises the
	// keycap (lit & raised) via keycapCapRect's raised arg. Set via SetToggled.
	// (The old continuous toggle-pulse glow was removed in the keycap redesign;
	// audio-tab pills carry their own lit-cyan active state in drawPillTabAt.)
	toggled bool
	// Disabled marks the button as inert: it renders greyed-out (via
	// DisabledButtonStyle chrome + colTextDisabled label/icon) and never fires
	// OnClick — both the legacy HandleInputResult path and the tree's
	// buttonHitAdapter swallow presses. Set directly (e.g. the subdivision
	// selector is disabled during playback, since validateSubdivisions rejects
	// mid-flight changes).
	Disabled bool
	// ConsumeOnPress: when true, after the first press triggers OnClick, suppress
	// any further button clicks across the UI until the mouse is released. Use
	// this for destructive actions to avoid cascading operations when the layout
	// changes under a held cursor.
	ConsumeOnPress bool
	// TextScale scales the button label text when > 0. A value of 1.5 renders
	// the debug font at 150%. Zero or negative uses the default 1× scale.
	// Ignored when TextRole is set.
	TextScale float64
	// textRole/hasTextRole select the styled text-role path (embedded Inter —
	// crisp at its true pixel size) for the label instead of the legacy
	// bitmap TextSprite + GeoM upscale path, which pixelates at any scale
	// ≠ 1. Set via SetTextRole (RoleBody is the enum zero, so a "set" flag is
	// required — same pattern as SpecID/hasSpecID). New text-bearing chrome
	// buttons should call SetTextRole(RoleBody); icon-only buttons don't
	// need it.
	textRole    TextRole
	hasTextRole bool
	// Optional icon to draw inside the button. When set, the icon is drawn
	// using simple vector primitives so it renders under the default Ebiten
	// debug font (which lacks many Unicode glyphs). Supported values:
	// "play", "pause", "stop", "pencil", "close". IconColor defaults to
	// colButtonBorder when zero.
	Icon      string
	IconColor color.Color
	// specIconSeed is the IconColor SetSpec seeded (nil when the spec has no
	// iconColor). effectiveIconColor re-reads the spec while IconColor still
	// equals the seed, so a runtime theme recolors the glyph without losing a
	// call-site override.
	specIconSeed color.Color
	// Highlights contains rune indices in Text to draw with a colored
	// background, used for fuzzy search match visualization. Only applied
	// when Icon is empty. Indices that fall outside the clipped text are
	// silently ignored.
	Highlights []int
	// TextColor overrides the default white text color when non-nil.
	// Used for semantic coloring (e.g. red text on delete menu items).
	TextColor color.Color
	// SpecID, when non-zero, selects a generated ComponentSpec from the
	// design system as the button's chrome recipe. Phase 4 PR3+ migration:
	// new code should set SpecID instead of Style; Draw() prefers SpecID
	// over Style when both are set, allowing per-zone migration without
	// breaking widgets that haven't been touched yet. Use HasSpecID()
	// rather than testing zero, since ComponentID 0 is a valid spec.
	SpecID    ComponentID
	hasSpecID bool
	// SquareCorners, when true, renders the keycap with square (0-radius)
	// corners instead of the default RadiusSM rounding. Used by the timeline
	// lock/free keycap, which reads as a solid square block filling the left
	// strip of the timeline rather than a rounded chip.
	SquareCorners bool
	// LeadSwatch, when non-nil, paints a leading identity swatch (the
	// instrument menu's rounded square, spacing.swatch-md) at the left of the
	// body and LEFT-aligns the label after it — the "swatch + text" list-row
	// pattern DESIGN.md names as the gold standard. Used by the rack's row
	// labels so each row carries its instrument color beside its name.
	LeadSwatch color.Color
}

// labelArea returns the rect the label is laid out in and whether it is
// left-aligned. A LeadSwatch reserves its leading slot; otherwise the label
// centers inside the body minus the hairline inset.
func (b *Button) labelArea(cap image.Rectangle) (image.Rectangle, bool) {
	if b.LeadSwatch != nil {
		sr := b.leadSwatchRect(cap)
		gap := SpaceSM
		if Profile().IsMobile() {
			gap = SpaceXS // the slim mobile bar needs less air than a square
		}
		return image.Rect(sr.Max.X+gap, cap.Min.Y, cap.Max.X-SpaceXS, cap.Max.Y), true
	}
	// Centered labels keep a 1 px side margin: the label is clipped only when
	// it genuinely cannot fit the body. A wider hairline inset turned a single
	// letter on a narrow chip ("M" on an 18 px mobile toggle) into "...".
	return image.Rect(cap.Min.X+1, cap.Min.Y, cap.Max.X-1, cap.Max.Y), false
}

// leadSwatchRect is the LeadSwatch, vertically centered at the body's leading
// edge: the instrument menu's swatch-md square, or — on a mobile profile,
// where a narrow rack label cannot spare a square's width — a slim identity
// bar of the same color (the profile's accent-stripe width).
func (b *Button) leadSwatchRect(cap image.Rectangle) image.Rectangle {
	sz := SwatchSizeMD
	if sz > cap.Dy() {
		sz = cap.Dy()
	}
	sx := cap.Min.X + SpaceSM
	if Profile().IsMobile() {
		w := Profile().AccentStripeWidth
		if w < 2 {
			w = 2
		}
		h := cap.Dy() - 2*SpaceSM
		if h < sz {
			h = sz
		}
		sx = cap.Min.X + SpaceXS
		sy := cap.Min.Y + (cap.Dy()-h)/2
		return image.Rect(sx, sy, sx+w, sy+h)
	}
	sy := cap.Min.Y + (cap.Dy()-sz)/2
	return image.Rect(sx, sy, sx+sz, sy+sz)
}

// labelOrigin places a label of size (w,h) in the label area: centered, or
// flush left when the button carries a LeadSwatch.
func (b *Button) labelOrigin(cap image.Rectangle, w, h int) (int, int) {
	area, left := b.labelArea(cap)
	x := area.Min.X + (area.Dx()-w)/2
	if left {
		x = area.Min.X
	}
	return x, cap.Min.Y + (cap.Dy()-h)/2
}

// SetSpec assigns a generated ComponentSpec ID to this button. Subsequent
// Draw() calls render via Spec(SpecID) → Render(spec, state) instead of
// the legacy Style.Draw path. Pass ComponentID(0) only via this setter,
// not by directly assigning the field, so the "set" flag is honored.
//
// If the spec declares an iconColor: in DESIGN.md, b.IconColor is seeded
// from spec.IconColor at SetSpec time. The call site can subsequently
// override b.IconColor for state-dependent variations (record-pulse,
// follow-on highlight, etc.); the spec value is the resting default.
func (b *Button) SetSpec(id ComponentID) {
	b.SpecID = id
	b.hasSpecID = true
	if spec := Spec(id); spec.HasIconColor {
		b.IconColor = spec.IconColor
		b.specIconSeed = spec.IconColor
	}
}

// effectiveIconColor returns the icon tint: a call-site override wins; an
// icon color seeded from the spec is re-read live so a runtime theme
// (ApplyTheme rebuilds the spec table) recolors existing buttons.
func (b *Button) effectiveIconColor() color.Color {
	if b.hasSpecID && b.specIconSeed != nil && b.IconColor == b.specIconSeed {
		if spec := Spec(b.SpecID); spec.HasIconColor {
			return spec.IconColor
		}
	}
	return liveCaptured(b.IconColor)
}

// effectiveTextColor is the label tint with a captured token resolved live
// (see liveCaptured); nil when no explicit color was set.
func (b *Button) effectiveTextColor() color.Color { return liveCaptured(b.TextColor) }

// HasSpecID reports whether SetSpec has been called on this button.
func (b *Button) HasSpecID() bool { return b.hasSpecID }

// Global guard to prevent multiple buttons from firing while a mouse press is
// held after a non-repeat click caused the UI to reflow under the cursor.
var suppressClicksUntilRelease bool

// SuppressClicksUntilMouseUp enables the global guard; the next mouse release
// clears it inside Handle.
func SuppressClicksUntilMouseUp() { suppressClicksUntilRelease = true }

// NewButton constructs a button with the given label, style, and optional click handler.
func NewButton(text string, style ButtonVisual, onClick func()) *Button {
	return &Button{Text: text, Style: style, OnClick: onClick}
}

// NewSpecButton constructs a button whose chrome is rendered from a
// generated ComponentSpec. Phase 4 PR3+ path: prefer this over NewButton
// for new code.
func NewSpecButton(text string, id ComponentID, onClick func()) *Button {
	b := &Button{Text: text, OnClick: onClick}
	b.SetSpec(id)
	return b
}

// NewButtonKey constructs a spec-styled button whose label is resolved live
// from the i18n catalog (resolve-at-draw). Prefer this for any localized label.
func NewButtonKey(key i18n.Key, id ComponentID, onClick func()) *Button {
	b := &Button{TextKey: key, OnClick: onClick}
	b.SetSpec(id)
	return b
}

// SetTextKey switches the button to live i18n resolution.
func (b *Button) SetTextKey(key i18n.Key) { b.TextKey = key }

// displayText returns the label to render: the live-resolved catalog string
// when TextKey is set, else the static Text.
func (b *Button) displayText() string {
	if b.TextKey != "" {
		return i18n.T(b.TextKey)
	}
	return b.Text
}

// Rect returns the button's bounds.
func (b *Button) Rect() image.Rectangle { return b.r }

// SetRect sets the button's bounds.
func (b *Button) SetRect(r image.Rectangle) { b.r = r }

// Draw renders the button and its label.
func (b *Button) Draw(dst *ebiten.Image) {
	// Advance the springy press scale once per visible frame. Draw is the
	// per-frame tick site for buttons; a settled button no-ops here.
	b.AdvancePressAnim()
	// Every interactive control shares one radius regardless of size — see
	// DESIGN.md "Shapes". This is the chokepoint that gives the whole app its
	// button silhouette; a per-size radius here is what made the toolbar read
	// as a pile of unrelated widgets.
	rad := fitRadius(RadiusControl, b.r)
	if b.SquareCorners {
		rad = RadiusNone
	}
	// Flat-tonal body: ONE filled rounded rect at the full button rect. The
	// control's silhouette is the tone step against its container — no socket
	// wall, no contact shadow, no bevel, no border. See DESIGN.md
	// "Elevation & Depth". `cap` is retained as the content rect name so the
	// label/icon centering below is unchanged; it is now simply b.r.
	cap := b.r
	switch {
	case b.Disabled:
		// Greyed-out chrome; never reflect press/hover/active state.
		DisabledButtonStyle.Draw(dst, cap, false, false)
	case b.hasSpecID:
		Render(dst, cap, Spec(b.SpecID), ComponentState{Pressed: b.pressed, Hovered: b.hovered, Active: b.toggled})
	case b.Style != nil:
		b.Style.Draw(dst, cap, b.pressed, b.hovered)
	}
	if !b.Disabled && b.toggled {
		// Latched: an accent TINT over the body plus a 2px accent bar. Never a
		// flood — the accent belongs to values, not chrome, so a selected
		// control reads clearly without shouting over its neighbours.
		drawRoundedRect(dst, cap, WithAlpha(colAccent, AlphaSubtle), rad, true)
		drawFlatSelectionBar(dst, cap, colAccent, selectionEdgeBottom, 1)
	}
	if b.LeadSwatch != nil {
		sr := b.leadSwatchRect(cap)
		drawRoundedRect(dst, sr, b.LeadSwatch, fitRadius(RadiusControl, sr), true)
	}
	// Skip text rendering when an icon is set — the icon is the visual
	// representation and the debug font can't render Unicode icon glyphs
	// (they appear as small white rectangles).
	if b.Icon == "" && b.hasTextRole {
		b.drawStyledLabel(dst, cap)
	} else if b.Icon == "" {
		scale := b.TextScale
		if scale <= 0 {
			scale = 1.0
		}
		// Clip text to fit within the label area.
		area, _ := b.labelArea(cap)
		clipped := clipTextToWidth(b.displayText(), int(float64(area.Dx())/scale))
		spr := TextSprite(clipped)
		// Center (or, with a LeadSwatch, left-align) the scaled text.
		w := int(float64(TextWidth(clipped)) * scale)
		h := int(float64(TextHeight()) * scale)
		x, y := b.labelOrigin(cap, w, h)

		// Draw fuzzy-match highlight rectangles behind matched chars.
		if len(b.Highlights) > 0 {
			drawTextHighlights(dst, clipped, b.Highlights, x, y, scale, TextHeight(), TextWidth)
		}

		var op ebiten.DrawImageOptions
		op.GeoM.Scale(scale, scale)
		op.GeoM.Translate(float64(x), float64(y))
		textCol := b.effectiveTextColor()
		if b.Disabled {
			textCol = colTextDisabled
		} else if b.toggled && textCol == nil {
			// Bright legend on the accent-TINTED latched body. (The old dark
			// legend existed for the solid-amber flood, which the flat-tonal
			// restyle removed — on a tint, dark text is unreadable.)
			textCol = colTextPrimary
		}
		if textCol != nil {
			r, g, bb, a := textCol.RGBA()
			if a > 0 {
				op.ColorScale.Scale(float32(r)/float32(a), float32(g)/float32(a), float32(bb)/float32(a), float32(a)/0xffff)
			}
		}
		dst.DrawImage(spr, &op)
	}
	// Icon overlay (font-independent)
	if b.Icon != "" {
		col := b.effectiveIconColor()
		if col == nil {
			col = colButtonBorder
		}
		if b.Disabled {
			col = colTextDisabled
		} else if b.toggled && b.IconColor == nil {
			// Accent glyph on the tinted latched body — the icon carries the
			// "on" signal alongside the selection bar.
			col = colAccent
		}
		// Proportional margin: 18% of min(w,h), min 2px.
		dim := cap.Dx()
		if cap.Dy() < dim {
			dim = cap.Dy()
		}
		pad := dim * 18 / 100
		if pad < 2 {
			pad = 2
		}
		box := image.Rect(cap.Min.X+pad, cap.Min.Y+pad, cap.Max.X-pad, cap.Max.Y-pad)
		// Sprite cache path: 1 DrawImage blit instead of many drawRect calls.
		if spr := iconSprite(b.Icon, box.Dx(), box.Dy()); spr != nil {
			var op ebiten.DrawImageOptions
			op.GeoM.Translate(float64(box.Min.X), float64(box.Min.Y))
			cr, cg, cb, ca := col.RGBA()
			if ca > 0 {
				op.ColorScale.Scale(float32(cr)/float32(ca), float32(cg)/float32(ca), float32(cb)/float32(ca), float32(ca)/0xffff)
			}
			dst.DrawImage(spr, &op)
			return
		}
		// Fallback: call the icon's draw function directly. Used by test builds
		// (sprite cache disabled) and any non-cached draw path.
		drawIconByID(dst, IconID(b.Icon), box, col)
	}
}

// colFuzzyHighlight is the background color for fuzzy-match highlighted chars.
var colFuzzyHighlight color.NRGBA // accent, semi-transparent; derived in rebuildThemeAliases (theme.go)

// drawTextHighlights draws small colored rectangles behind characters at the
// given rune indices. Used to visualize fuzzy search matches. measure and
// cellH MUST be the same metric the text is rendered with (TextWidth/
// TextHeight for body-font buttons, StyledTextWidth/StyledTextHeight for
// styled menu labels): cell i spans [measure(text[:i]), measure(text[:i+1])]
// so the cells tile exactly with the drawn glyphs. Summing per-rune widths
// instead is wrong for proportional fonts — an isolated rune measures its ink
// bounds (a space measures 0, side bearings vanish, kerning is lost), so the
// cells drift left of the glyphs they mark.
func drawTextHighlights(dst *ebiten.Image, text string, highlights []int, baseX, baseY int, scale float64, cellH int, measure func(string) int) {
	rs := []rune(text)
	hlSet := make(map[int]bool, len(highlights))
	maxIdx := -1
	for _, idx := range highlights {
		if idx >= 0 && idx < len(rs) {
			hlSet[idx] = true
			if idx > maxIdx {
				maxIdx = idx
			}
		}
	}
	if maxIdx < 0 {
		return
	}
	prevW := 0
	for i := 0; i <= maxIdx; i++ {
		w := measure(string(rs[:i+1]))
		if w < prevW {
			w = prevW // measure is nondecreasing; guard trailing-space quirks
		}
		if hlSet[i] && w > prevW {
			sx := baseX + int(float64(prevW)*scale)
			sw := int(float64(w-prevW) * scale)
			sh := int(float64(cellH) * scale)
			drawRect(dst, image.Rect(sx, baseY, sx+sw, baseY+sh), colFuzzyHighlight, true)
		}
		prevW = w
	}
}

// UpdateHover refreshes the button's hover flag from the plain cursor
// position. For buttons hosted in portal overlays, whose input handlers only
// run on press/drag/release edges — plain mouse-over never reaches
// HandleInputResult there, so hover feedback needs this per-frame seam
// (portalHoverer). Desktop only (the caller guards).
func (b *Button) UpdateHover(mx, my int) {
	if b == nil {
		return
	}
	b.hovered = !b.Disabled && image.Pt(mx, my).In(b.Rect())
}

// SetTextRole routes the button's label through the styled text-role API
// (see textRole/hasTextRole). RoleBody is the enum zero, hence a setter with
// a "set" flag rather than a bare field.
func (b *Button) SetTextRole(role TextRole) {
	b.textRole = role
	b.hasTextRole = true
}

// StyledTextRole reports the styled label role and whether one is set.
func (b *Button) StyledTextRole() (TextRole, bool) { return b.textRole, b.hasTextRole }

// drawStyledLabel renders the button label via the styled text-role API —
// the smooth (embedded-Inter) sibling of the legacy sprite path, selected by
// setting Button.TextRole. Centering, clipping, fuzzy highlights, and the
// disabled/latched text colors match the legacy path's behavior.
func (b *Button) drawStyledLabel(dst *ebiten.Image, cap image.Rectangle) {
	role := b.textRole
	measure := func(s string) int { return StyledTextWidth(s, role) }
	area, _ := b.labelArea(cap)
	clipped := clipTextToWidthMeasure(b.displayText(), area.Dx(), measure)
	w := measure(clipped)
	h := StyledTextHeight(role)
	x, y := b.labelOrigin(cap, w, h)
	if len(b.Highlights) > 0 {
		drawTextHighlights(dst, clipped, b.Highlights, x, y, 1.0, h, measure)
	}
	textCol := b.effectiveTextColor()
	if b.Disabled {
		textCol = colTextDisabled
	} else if b.toggled && textCol == nil {
		textCol = genColorBackground
	}
	if textCol == nil {
		textCol = colTextPrimary
	}
	DrawTextStyled(dst, clipped, x, y, role, textCol)
}

// textRect returns the rectangle occupied by the button's text when drawn,
// clamped to the button bounds. Matches Draw(): clips text to button width
// and applies TextScale (or the styled-role metrics when TextRole is set).
//
//nolint:unused // used by test files with test build tag
func (b *Button) textRect() image.Rectangle {
	if b.hasTextRole {
		measure := func(s string) int { return StyledTextWidth(s, b.textRole) }
		area, _ := b.labelArea(b.r)
		clipped := clipTextToWidthMeasure(b.displayText(), area.Dx(), measure)
		w := measure(clipped)
		h := StyledTextHeight(b.textRole)
		x, y := b.labelOrigin(b.r, w, h)
		return image.Rect(x, y, x+w, y+h).Intersect(b.r)
	}
	scale := b.TextScale
	if scale <= 0 {
		scale = 1.0
	}
	area, _ := b.labelArea(b.r)
	clipped := clipTextToWidth(b.displayText(), int(float64(area.Dx())/scale))
	w := int(float64(TextWidth(clipped)) * scale)
	h := int(float64(TextHeight()) * scale)
	x, y := b.labelOrigin(b.r, w, h)
	return image.Rect(x, y, x+w, y+h).Intersect(b.r)
}

// HandleInputResult processes a mouse click at (mx,my) and returns an
// InputResult instead of a bool.  It triggers OnClick when pressed inside.
// On touch devices, the hit area is expanded to meet minimum touch target size.
// Returns InputConsumed on click, InputIgnored otherwise.
func (b *Button) HandleInputResult(mx, my int, pressed bool) InputResult {
	if b.Disabled {
		b.pressed = false
		b.hovered = false
		b.held = 0
		return InputIgnored
	}
	if suppressClicksUntilRelease {
		if !pressed {
			suppressClicksUntilRelease = false
		}
		b.pressed = false
		b.held = 0
		return InputIgnored
	}
	inside := image.Pt(mx, my).In(b.HitRect())
	return b.applyPress(inside, pressed)
}

// HitRect is the rect a press must land in to hit the button: the drawn rect,
// grown to TouchMinTarget on touch profiles. It is THE hit geometry for the
// button — an input handler that routes presses to it must route on this rect,
// not on Rect(), or a press inside the touch target is never delivered and
// falls through to whatever lies beneath (the mobile top-right-button bug).
// A zero-area rect (e.g. a hidden mobile column) stays empty: not interactable.
func (b *Button) HitRect() image.Rectangle {
	hitRect := b.r
	if minTarget := TouchMinTarget(); minTarget > 0 && !hitRect.Empty() {
		if hitRect.Dx() < minTarget {
			expand := (minTarget - hitRect.Dx()) / 2
			hitRect.Min.X -= expand
			hitRect.Max.X += expand
		}
		if hitRect.Dy() < minTarget {
			expand := (minTarget - hitRect.Dy()) / 2
			hitRect.Min.Y -= expand
			hitRect.Max.Y += expand
		}
	}
	return hitRect
}

// applyPress is the shared press/fire/repeat/release core for one pointer
// event, factored out of HandleInputResult so the tree's buttonHitAdapter can
// reuse the exact same edge-firing, held-counter, repeat, and release-settle
// logic without re-running geometry. `inside` is whether the pointer is over
// the (touch-expanded) button; callers that trust an upstream hit decision
// (the input tree) pass inside=true. Returns InputConsumed when the press is
// consumed, InputIgnored otherwise.
func (b *Button) applyPress(inside, pressed bool) InputResult {
	b.hovered = inside
	if pressed && inside {
		b.held++
		if b.held == 1 {
			// Press edge: ease the cap down toward full travel.
			emitUITap(b.displayText(), b.Icon)
			b.pressTarget = 1
			if b.OnClick != nil {
				b.OnClick()
			}
			if b.ConsumeOnPress {
				suppressClicksUntilRelease = true
			}
		} else if b.Repeat && b.repeatTick() {
			if b.OnClick != nil {
				b.OnClick()
			}
		}
		b.pressed = true
		return InputConsumed
	}
	if b.pressed {
		// Release edge: settle target to rest, kick the cap UP for the
		// analogue overshoot before it springs back down to 0.
		b.pressTarget = 0
		if b.pressDepth > 0 {
			b.pressDepth = -genGeomKeycapReleaseOvershootFrac
		}
	}
	b.pressed = false
	b.held = 0
	return InputIgnored
}

// PressFromTree applies a pointer event whose hit was already decided by the
// input tree (so no geometry re-check). Disabled buttons swallow the press
// without firing. This is the single seam the shared buttonHitAdapter uses, so
// every tree-routed button reuses HandleInputResult's exact press lifecycle
// (edge fire, held counter, repeat, ConsumeOnPress, release settle).
func (b *Button) PressFromTree(pressed bool) InputResult {
	if b.Disabled {
		b.pressed = false
		b.hovered = false
		b.held = 0
		return InputConsumed
	}
	return b.applyPress(true, pressed)
}

// ResetPress clears the transient press/hover state so the NEXT pressed-inside
// frame is treated as a fresh press edge (Button.held back to 0 → OnClick fires
// on held==1). Stay-open menus that reuse the same *Button across selections
// (the instrument menu) must call this before re-dispatching a press: those
// menus return InputConsumed (one-shot, not captured), so the input tree never
// delivers a release to the button, leaving held latched at 1 — a second click
// on the same row would otherwise increment held to 2 and be silently swallowed.
// Mirrors the release-edge settle in HandleInputResult.
func (b *Button) ResetPress() {
	if b.pressed {
		b.pressTarget = 0
		if b.pressDepth > 0 {
			b.pressDepth = -genGeomKeycapReleaseOvershootFrac
		}
	}
	b.pressed = false
	b.hovered = false
	b.held = 0
}

// ReleaseStuckPress clears a latched press (pressed/held) and springs the cap
// back to rest, WITHOUT disturbing hover. It heals a button whose release edge
// was swallowed by a menu/scrim that opened on its press — the input path never
// delivered the release, so the cap stayed stuck down and held latched at 1 (a
// later tap would then increment held past the press edge and be swallowed,
// making the button un-clickable). Unlike ResetPress it preserves hover, so a
// caller may invoke it every frame the pointer is up without flickering the
// desktop hover glow. Safe with the pointer up: no pointer down means no active
// hold to cancel, so it never aborts a real press or double-fires OnClick.
func (b *Button) ReleaseStuckPress() {
	if !b.pressed && b.held == 0 {
		return
	}
	if b.pressed && b.pressDepth > 0 {
		b.pressDepth = -genGeomKeycapReleaseOvershootFrac
	}
	b.pressTarget = 0
	b.pressed = false
	b.held = 0
}

// AdvancePressAnim eases the press depth toward its target (analogue settle,
// both directions) and decays the one-shot engage flash. Call once per frame
// from Draw. Allocation-free; a no-op once depth==target and flash==0.
func (b *Button) AdvancePressAnim() {
	if b.pressDepth != b.pressTarget {
		r := genAnimButtonPressDecay.Rate
		b.pressDepth = b.pressTarget + (b.pressDepth-b.pressTarget)*r
		if math.Abs(b.pressDepth-b.pressTarget) < genAnimButtonPressDecay.Threshold {
			b.pressDepth = b.pressTarget
		}
	}
	if b.engageAnim > 0 {
		if v, alive := DecayStep(b.engageAnim, genAnimButtonEngageFlash); alive {
			b.engageAnim = v
		} else {
			b.engageAnim = 0
		}
	}
}

// fillColor resolves the cap's base fill for the shell/wall derivation: the
// spec's Fill when spec-driven, else a neutral surface fallback.
func (b *Button) fillColor() color.Color {
	if b.hasSpecID {
		return Spec(b.SpecID).Fill
	}
	return TokenSurface2()
}

// Pressed reports whether the button is currently in the pressed (held-down)
// visual state. Driven by the shared input lifecycle (HandleInputResult /
// buttonHitAdapter); used by tests and any caller that needs press feedback.
func (b *Button) Pressed() bool { return b.pressed }

// SetToggled marks the button as latched/active so its cushion glow becomes a
// persistent pulsing accent ring. Idempotent.
func (b *Button) SetToggled(on bool) { b.toggled = on }

// Toggled reports whether the button is in the latched/active state.
func (b *Button) Toggled() bool { return b.toggled }

// repeatTick reports whether this held button fires an auto-repeat this frame.
//
// The cadence now comes from the shared buttonRepeat curve (key_repeat.go) so
// MenuKeyNav can reuse the same acceleration ramp instead of growing a second
// copy. tickLegacyModulo preserves this button's exact historical firing rule,
// double-fire at acceleration steps included — see key_repeat.go for why menus
// use dueSince instead.
func (b *Button) repeatTick() bool {
	return buttonRepeat.tickLegacyModulo(b.held)
}

// GridLayout splits a rectangle into rows and columns using fractional weights.
type GridLayout struct {
	bounds     image.Rectangle
	colWeights []float64
	rowWeights []float64
	colPos     []int
	rowPos     []int
}

// NewGridLayout creates a layout for the given bounds.
func NewGridLayout(b image.Rectangle, cols, rows []float64) *GridLayout {
	g := &GridLayout{bounds: b, colWeights: cols, rowWeights: rows}
	g.recalc()
	return g
}

func (g *GridLayout) recalc() {
	totalW := 0.0
	for _, w := range g.colWeights {
		totalW += w
	}
	totalH := 0.0
	for _, h := range g.rowWeights {
		totalH += h
	}
	// Compute positions as rounded fractions of the total to distribute
	// rounding error evenly (±1px per cell) instead of accumulating it
	// in the last cell.
	g.colPos = make([]int, len(g.colWeights)+1)
	cumW := 0.0
	for i, w := range g.colWeights {
		g.colPos[i] = g.bounds.Min.X + int(cumW/totalW*float64(g.bounds.Dx()))
		cumW += w
	}
	g.colPos[len(g.colWeights)] = g.bounds.Max.X

	g.rowPos = make([]int, len(g.rowWeights)+1)
	cumH := 0.0
	for i, h := range g.rowWeights {
		g.rowPos[i] = g.bounds.Min.Y + int(cumH/totalH*float64(g.bounds.Dy()))
		cumH += h
	}
	g.rowPos[len(g.rowWeights)] = g.bounds.Max.Y
}

// Cell returns the rectangle for the specified cell.
func (g *GridLayout) Cell(col, row int) image.Rectangle {
	return image.Rect(g.colPos[col], g.rowPos[row], g.colPos[col+1], g.rowPos[row+1])
}

// SubGrid creates a new GridLayout within the specified cell, allowing nested
// grids with different column/row structures.
func (g *GridLayout) SubGrid(col, row int, cols, rows []float64) *GridLayout {
	return NewGridLayout(g.Cell(col, row), cols, rows)
}

// LayoutGroup is a named, hierarchical GridLayout that tracks parent-child
// relationships. Moving or resizing the parent automatically repositions
// all children. This enables relocating entire UI sections (e.g., transport
// controls, row controls) by changing a single parent cell assignment.
type LayoutGroup struct {
	ID       string
	grid     *GridLayout
	parent   *LayoutGroup
	pCol     int // parent cell column
	pRow     int // parent cell row
	children []*LayoutGroup
	visible  bool
}

// NewLayoutGroup creates a top-level layout group.
func NewLayoutGroup(id string, bounds image.Rectangle, cols, rows []float64) *LayoutGroup {
	return &LayoutGroup{
		ID:      id,
		grid:    NewGridLayout(bounds, cols, rows),
		visible: true,
	}
}

// AddChild creates a child group anchored to the parent's cell at (col, row).
func (g *LayoutGroup) AddChild(id string, col, row int, cols, rows []float64) *LayoutGroup {
	child := &LayoutGroup{
		ID:      id,
		grid:    NewGridLayout(g.grid.Cell(col, row), cols, rows),
		parent:  g,
		pCol:    col,
		pRow:    row,
		visible: true,
	}
	g.children = append(g.children, child)
	return child
}

// Bounds returns the absolute rectangle of this group.
func (g *LayoutGroup) Bounds() image.Rectangle {
	if !g.visible {
		return image.Rectangle{}
	}
	return g.grid.bounds
}

// Cell returns the absolute cell rectangle within this group.
func (g *LayoutGroup) Cell(col, row int) image.Rectangle {
	if !g.visible {
		return image.Rectangle{}
	}
	return g.grid.Cell(col, row)
}

// SetBounds updates the bounds and cascades to all children.
func (g *LayoutGroup) SetBounds(b image.Rectangle) {
	g.grid.bounds = b
	g.grid.recalc()
	for _, c := range g.children {
		c.SetBounds(g.grid.Cell(c.pCol, c.pRow))
	}
}

// SetVisible hides or shows this group and all its children.
func (g *LayoutGroup) SetVisible(v bool) {
	g.visible = v
	for _, c := range g.children {
		c.SetVisible(v)
	}
}

// Relayout recalculates this group's bounds from its parent cell and cascades.
func (g *LayoutGroup) Relayout() {
	if g.parent != nil {
		g.SetBounds(g.parent.grid.Cell(g.pCol, g.pRow))
	} else {
		g.grid.recalc()
		for _, c := range g.children {
			c.SetBounds(g.grid.Cell(c.pCol, c.pRow))
		}
	}
}

// Find looks up a child group by ID in the subtree (depth-first).
func (g *LayoutGroup) Find(id string) *LayoutGroup {
	if g.ID == id {
		return g
	}
	for _, c := range g.children {
		if found := c.Find(id); found != nil {
			return found
		}
	}
	return nil
}
