package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// showLongPressPopup opens the three-button quick-action popup above the given
// node. It's triggered by a long-press gesture on all platforms.
func (g *Game) showLongPressPopup(node *uiNode, touchX, touchY int) {
	// Guard: skip if already in a modal mode.
	if g.modes.Active() != GridModeNone || g.longPressPopup {
		return
	}
	// Close any open node sidebar first.
	if g.sidebar.IsOpen() {
		g.sidebar.Close()
	}

	g.longPressPopup = true
	g.longPressPopupNode = node
	g.longPressPopupHover = ""
	g.longPressPopupLastHover = ""

	// Button dimensions — sourced from density tokens and spacing constants.
	// btnW is widened from the text-only ideal so an icon (IconSizeMD) fits
	// above the label; btnH grows to fit an icon row + label row.
	btnW := 88
	btnH := Profile().DensityValues().PopupBtnH + IconSizeMD + SpaceXS // icon row + label row
	gap := SpaceSM
	pad := SpaceMD
	gridW := g.split.GridW(g.winW)
	// Four keycaps must fit narrow mobile panes: shrink from the ideal 80px
	// down to whatever the grid pane allows.
	if maxBtnW := (gridW - 2*pad - 3*gap) / 4; maxBtnW < btnW {
		btnW = maxBtnW
	}
	panelW := 4*btnW + 3*gap + 2*pad
	panelH := btnH + 2*pad

	// Position: centered above the node by default.
	sx1, sy1, sx2, _ := g.nodeScreenRect(node)
	cx := int((sx1 + sx2) * 0.5)
	px := cx - panelW/2
	py := int(sy1) - panelH - 8

	// If near the top edge, place below the node instead.
	if py < 0 {
		_, _, _, sy2 := g.nodeScreenRect(node)
		py = int(sy2) + 8
	}

	// Clamp within grid pane bounds.
	gridH := g.split.GridH(g.winH)
	if px < 0 {
		px = 0
	}
	if px+panelW > gridW {
		px = gridW - panelW
	}
	if py < 0 {
		py = 0
	}
	if py+panelH > gridH {
		py = gridH - panelH
	}

	// Keep the popup clear of the settings gear's protected top-right zone.
	rect := avoidReservedRect(image.Rect(px, py, px+panelW, py+panelH), g.gridHelpReservedRect(), gridW, gridH)
	g.longPressPopupRect = rect
	px, py = rect.Min.X, rect.Min.Y
	// Buttons inside the panel, with padding.
	bx := px + pad
	by := py + pad
	g.longPressPopupMove = image.Rect(bx, by, bx+btnW, by+btnH)
	g.longPressPopupCopy = image.Rect(bx+(btnW+gap), by, bx+2*btnW+gap, by+btnH)
	g.longPressPopupConn = image.Rect(bx+2*(btnW+gap), by, bx+3*btnW+2*gap, by+btnH)
	g.longPressPopupDel = image.Rect(bx+3*(btnW+gap), by, bx+4*btnW+3*gap, by+btnH)

	emitPopupOpened(longPressPopupID)
}

// updateLongPressPopup handles per-frame input while the popup is visible.
// The user slides their finger to hover over a button and releases to activate.
func (g *Game) updateLongPressPopup(mx, my int, left bool) {
	if !g.longPressPopup {
		return
	}

	pt := image.Pt(mx, my)
	if pt.In(g.longPressPopupMove) {
		g.longPressPopupHover = "move"
	} else if pt.In(g.longPressPopupCopy) {
		g.longPressPopupHover = "copy"
	} else if pt.In(g.longPressPopupConn) {
		g.longPressPopupHover = "connect"
	} else if pt.In(g.longPressPopupDel) {
		g.longPressPopupHover = "delete"
	} else {
		g.longPressPopupHover = ""
	}

	// Track last-known hover while finger is still down.
	// On touch release, coordinates may become stale (0,0) because the
	// touch override deactivates when the touch ends. By remembering the
	// hover from the previous frame we can still activate the correct button.
	if left {
		g.longPressPopupLastHover = g.longPressPopupHover
	}

	// Click-outside contract: a NEW press (down-edge) that lands outside the
	// panel dismisses the popup, matching every other pop-up. The slide-release
	// path below still handles the hold-slide-release gesture.
	pressEdge := left && !g.leftPrev
	if pressEdge && !pt.In(g.longPressPopupRect) {
		g.dismissLongPressPopup("outside-press")
		g.leftPrev = left
		return
	}

	// Detect release: previous frame was pressed, this frame is not.
	if g.leftPrev && !left {
		switch g.longPressPopupLastHover {
		case "move":
			node := g.longPressPopupNode
			emitUITap(i18n.T(i18n.KeyCapMove), "")
			g.dismissLongPressPopup("action")
			g.modes.Enter(GridModeMove, node)
			g.moveSkipRelease = true
		case "copy":
			node := g.longPressPopupNode
			emitUITap(i18n.T(i18n.KeyCapCopy), "")
			g.dismissLongPressPopup("action")
			if g.snapshotNodeToClipboard(node) {
				emitNodeCopied(node.ID, node.I, node.J, g.clipboard.typ)
				g.modes.Enter(GridModeCopy, node)
			}
		case "connect":
			node := g.longPressPopupNode
			emitUITap(i18n.T(i18n.KeyCapConnect), "")
			g.dismissLongPressPopup("action")
			g.enterConnectMode(node)
		case "delete":
			node := g.longPressPopupNode
			emitUITap(i18n.T(i18n.KeyMenuDelete), "")
			g.dismissLongPressPopup("action")
			g.deleteNode(node)
		default:
			g.dismissLongPressPopup("dismiss")
		}
	}
}

// longPressPopupID is the popup-lifecycle telemetry surface id for the node
// long-press quick-action popup (see hooks.PopupPayload).
const longPressPopupID = "node-longpress"

// dismissLongPressPopup clears all popup state, emitting popup.closed with
// reason (only when the popup was actually open — repeated/no-op dismiss
// calls, e.g. from closeAllPopups when nothing is showing, must not double-emit).
func (g *Game) dismissLongPressPopup(reason string) {
	if !g.longPressPopup {
		return
	}
	emitPopupClosed(longPressPopupID, reason)
	g.longPressPopup = false
	g.longPressPopupNode = nil
	g.longPressPopupRect = image.Rectangle{}
	g.longPressPopupMove = image.Rectangle{}
	g.longPressPopupCopy = image.Rectangle{}
	g.longPressPopupConn = image.Rectangle{}
	g.longPressPopupDel = image.Rectangle{}
	g.longPressPopupHover = ""
	g.longPressPopupLastHover = ""
}

// longPressConnectHoverColor returns the azure accent color used for the
// Connect button hover text. Extracted as a function so tests can assert the
// single-chrome-accent invariant (cyan is reserved for viz data only).
func longPressConnectHoverColor() color.Color { return colAccent }

// drawLongPressPopup renders the three-button quick-action popup.
// longPressPopupAccent returns the accent color for the node long-press popup:
// the owning row's instrument color so the popup's hover highlights match the
// instrument the node belongs to. Falls back to the azure chrome accent when
// the node has no resolvable row.
func (g *Game) longPressPopupAccent() color.Color {
	if g.longPressPopupNode != nil {
		if row := g.rowIndexForNode(g.longPressPopupNode.ID); row >= 0 && row < len(g.drum.Rows) {
			if c := g.drum.Rows[row].Color; c != nil {
				return c
			}
		}
	}
	return colAccent
}

// longPressDeleteTextColor is the Delete keycap's label tint: the filled
// destructive (neon) body takes a dark label per the DESIGN.md dark-on-neon
// contrast rule and the `button-destructive.textColor` component token. A
// near-white label on the saturated ember fill read as a glossy highlight
// and broke the filled-control pattern.
func longPressDeleteTextColor() color.Color { return genColorBackground }

// drawPopupActionCap renders one quick-action keycap: shared keycap chrome,
// a leading/top icon, and a centered label. tint colors BOTH icon and label
// (instrument accent on hover); delete overrides via its own fill/label.
func drawPopupActionCap(dst *ebiten.Image, rect image.Rectangle, icon IconID, label string, fill, border, tint color.Color, hovered bool) {
	capRect := drawKeycapPanelButton(dst, rect, fill, border, hovered)
	iconSz := IconSizeMD
	ix := capRect.Min.X + (capRect.Dx()-iconSz)/2
	iy := capRect.Min.Y + SpaceXS
	drawIconByID(dst, icon, image.Rect(ix, iy, ix+iconSz, iy+iconSz), tint)
	lw := StyledTextWidth(label, RoleBody)
	lx := capRect.Min.X + (capRect.Dx()-lw)/2
	ly := iy + iconSz + SpaceXS
	DrawTextStyled(dst, label, lx, ly, RoleBody, tint)
}

func (g *Game) drawLongPressPopup(dst *ebiten.Image) {
	if !g.longPressPopup {
		return
	}
	accent := g.longPressPopupAccent()
	drawScrim(dst)
	drawPanel(dst, g.longPressPopupRect)

	tint := func(hover bool) color.Color {
		if hover {
			return accent
		}
		return color.Color(genColorPopupTextSecondary)
	}
	drawPopupActionCap(dst, g.longPressPopupMove, IconMove, i18n.T(i18n.KeyCapMove),
		colDropdown, colTransportBorder, tint(g.longPressPopupHover == "move"), g.longPressPopupHover == "move")
	drawPopupActionCap(dst, g.longPressPopupCopy, IconCopy, i18n.T(i18n.KeyCapCopy),
		colDropdown, colTransportBorder, tint(g.longPressPopupHover == "copy"), g.longPressPopupHover == "copy")
	drawPopupActionCap(dst, g.longPressPopupConn, IconConnect, i18n.T(i18n.KeyCapConnect),
		colDropdown, colTransportBorder, tint(g.longPressPopupHover == "connect"), g.longPressPopupHover == "connect")
	// Delete: destructive fill + dark-on-neon icon+label.
	drawPopupActionCap(dst, g.longPressPopupDel, IconTrash, i18n.T(i18n.KeyMenuDelete),
		colDeleteFill, colDeleteBorder, longPressDeleteTextColor(), g.longPressPopupHover == "delete")
}
