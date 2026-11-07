package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

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

	// Wide and tall enough for an icon above the label.
	btnW := 88
	btnH := Profile().DensityValues().PopupBtnH + IconSizeMD + SpaceXS // icon row + label row
	gap := SpaceSM
	pad := SpaceMD
	gridW := g.split.GridW(g.winW)
	// Four keycaps must fit a narrow mobile pane.
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
	rect := avoidReservedRect(image.Rect(px, py, px+panelW, py+panelH), g.gridTopButtonsReservedRect(), gridW, gridH)
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

// Slide to a button and release to activate.
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

	// On release the touch override is gone and coordinates may read (0,0), so remember the hover.
	if left {
		g.longPressPopupLastHover = g.longPressPopupHover
	}

	// A new press outside dismisses it, like every pop-up.
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

const longPressPopupID = "node-longpress"

// Emits only when the popup was open, so repeated dismisses never double-emit.
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

// The chrome accent: cyan is reserved for data.
func longPressConnectHoverColor() color.Color { return colAccent }

// The row's instrument color, so the hover matches the node's instrument.
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

// A dark label on the filled destructive body (dark-on-neon contrast rule).
func longPressDeleteTextColor() color.Color { return genColorBackground }

// tint colors both icon and label; border is unused (flat tonal: no border at rest).
func drawPopupActionCap(dst *ebiten.Image, rect image.Rectangle, icon IconID, label string, fill, border, tint color.Color, hovered bool) {
	_ = border
	capRect := drawFlatPanelButton(dst, rect, fill, hovered)
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
