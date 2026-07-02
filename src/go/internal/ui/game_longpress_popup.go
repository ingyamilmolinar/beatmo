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
	if g.moveMode || g.connectMode || g.longPressPopup {
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
	btnW := 80
	btnH := Profile().DensityValues().PopupBtnH
	gap := SpaceSM
	pad := SpaceMD
	panelW := 3*btnW + 2*gap + 2*pad
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
	gridW := g.split.GridW(g.winW)
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

	g.longPressPopupRect = image.Rect(px, py, px+panelW, py+panelH)
	// Buttons inside the panel, with padding.
	bx := px + pad
	by := py + pad
	g.longPressPopupMove = image.Rect(bx, by, bx+btnW, by+btnH)
	g.longPressPopupConn = image.Rect(bx+btnW+gap, by, bx+2*btnW+gap, by+btnH)
	g.longPressPopupDel = image.Rect(bx+2*(btnW+gap), by, bx+3*btnW+2*gap, by+btnH)
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

	// Detect release: previous frame was pressed, this frame is not.
	if g.leftPrev && !left {
		switch g.longPressPopupLastHover {
		case "move":
			node := g.longPressPopupNode
			g.dismissLongPressPopup()
			g.moveMode = true
			g.movingNode = node
			g.moveSkipRelease = true
		case "connect":
			node := g.longPressPopupNode
			g.dismissLongPressPopup()
			g.enterConnectMode(node)
		case "delete":
			node := g.longPressPopupNode
			g.dismissLongPressPopup()
			g.deleteNode(node)
		default:
			g.dismissLongPressPopup()
		}
	}
}

// dismissLongPressPopup clears all popup state.
func (g *Game) dismissLongPressPopup() {
	g.longPressPopup = false
	g.longPressPopupNode = nil
	g.longPressPopupRect = image.Rectangle{}
	g.longPressPopupMove = image.Rectangle{}
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

func (g *Game) drawLongPressPopup(dst *ebiten.Image) {
	if !g.longPressPopup {
		return
	}
	accent := g.longPressPopupAccent()

	// Panel background.
	drawScrim(dst)
	drawPanel(dst, g.longPressPopupRect)

	// Move button — raised keycap, neutral color, instrument-accent text on hover.
	moveHover := g.longPressPopupHover == "move"
	moveCap := drawKeycapPanelButton(dst, g.longPressPopupMove, colDropdown, colTransportBorder, moveHover)
	moveTxtCol := color.Color(genColorPopupTextSecondary)
	if moveHover {
		moveTxtCol = accent // instrument-color text when hovered
	}
	mtx := moveCap.Min.X + (moveCap.Dx()-StyledTextWidth(i18n.T(i18n.KeyCapMove), RoleBody))/2
	mty := moveCap.Min.Y + (moveCap.Dy()-StyledTextHeight(RoleBody))/2
	DrawTextStyled(dst, i18n.T(i18n.KeyCapMove), mtx, mty, RoleBody, moveTxtCol)

	// Connect button — raised keycap, neutral color, instrument-accent text on hover.
	connHover := g.longPressPopupHover == "connect"
	connCap := drawKeycapPanelButton(dst, g.longPressPopupConn, colDropdown, colTransportBorder, connHover)
	connTxtCol := color.Color(genColorPopupTextSecondary)
	if connHover {
		connTxtCol = accent // instrument-color text when hovered
	}
	ctx := connCap.Min.X + (connCap.Dx()-StyledTextWidth(i18n.T(i18n.KeyCapConnect), RoleBody))/2
	cty := connCap.Min.Y + (connCap.Dy()-StyledTextHeight(RoleBody))/2
	DrawTextStyled(dst, i18n.T(i18n.KeyCapConnect), ctx, cty, RoleBody, connTxtCol)

	// Delete button — raised keycap, destructive rust color, primary text (no accent on hover).
	delHover := g.longPressPopupHover == "delete"
	delCap := drawKeycapPanelButton(dst, g.longPressPopupDel, colDeleteFill, colDeleteBorder, delHover)
	dtx := delCap.Min.X + (delCap.Dx()-StyledTextWidth(i18n.T(i18n.KeyMenuDelete), RoleBody))/2
	dty := delCap.Min.Y + (delCap.Dy()-StyledTextHeight(RoleBody))/2
	DrawTextStyled(dst, i18n.T(i18n.KeyMenuDelete), dtx, dty, RoleBody, colTextPrimary)
}
