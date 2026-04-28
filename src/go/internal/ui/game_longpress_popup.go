package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
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

	// Button dimensions.
	btnW := 80
	btnH := 44
	gap := 8
	panelW := 3*btnW + 2*gap + 16 // 16 for horizontal padding (8 each side)
	panelH := btnH + 16           // 16 for vertical padding (8 each side)

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
	bx := px + 8
	by := py + 8
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

// drawLongPressPopup renders the three-button quick-action popup.
func (g *Game) drawLongPressPopup(dst *ebiten.Image) {
	if !g.longPressPopup {
		return
	}

	// Panel background.
	drawScrim(dst)
	drawPanel(dst, g.longPressPopupRect)

	// Move button — unified dark style, green text when hovered.
	moveHover := g.longPressPopupHover == "move"
	moveFill, moveBorder := color.Color(colDropdown), color.Color(colTransportBorder)
	if moveHover {
		moveFill = adjustColor(moveFill, 12)
		moveBorder = adjustColor(moveBorder, 20)
	}
	drawRoundedButton(dst, g.longPressPopupMove, moveFill, moveBorder, popupButtonRadius(), false)
	moveTxtCol := color.Color(genColorPopupTextSecondary)
	if moveHover {
		moveTxtCol = colPlayIconTint // green
	}
	mtx := g.longPressPopupMove.Min.X + (g.longPressPopupMove.Dx()-TextWidth("Move"))/2
	mty := g.longPressPopupMove.Min.Y + (g.longPressPopupMove.Dy()-TextHeight())/2
	DrawTextColorAt(dst, "Move", mtx, mty, moveTxtCol)

	// Connect button — unified dark style, cyan text when hovered.
	connHover := g.longPressPopupHover == "connect"
	connFill, connBorder := color.Color(colDropdown), color.Color(colTransportBorder)
	if connHover {
		connFill = adjustColor(connFill, 12)
		connBorder = adjustColor(connBorder, 20)
	}
	drawRoundedButton(dst, g.longPressPopupConn, connFill, connBorder, popupButtonRadius(), false)
	connTxtCol := color.Color(genColorPopupTextSecondary)
	if connHover {
		connTxtCol = colStep // cyan
	}
	ctx := g.longPressPopupConn.Min.X + (g.longPressPopupConn.Dx()-TextWidth("Connect"))/2
	cty := g.longPressPopupConn.Min.Y + (g.longPressPopupConn.Dy()-TextHeight())/2
	DrawTextColorAt(dst, "Connect", ctx, cty, connTxtCol)

	// Delete button — destructive red style.
	delHover := g.longPressPopupHover == "delete"
	delFill, delBorder := color.Color(colDeleteFill), color.Color(colDeleteBorder)
	if delHover {
		delFill = adjustColor(delFill, 12)
		delBorder = adjustColor(delBorder, 20)
	}
	drawRoundedButton(dst, g.longPressPopupDel, delFill, delBorder, popupButtonRadius(), false)
	dtx := g.longPressPopupDel.Min.X + (g.longPressPopupDel.Dx()-TextWidth("Delete"))/2
	dty := g.longPressPopupDel.Min.Y + (g.longPressPopupDel.Dy()-TextHeight())/2
	DrawTextAt(dst, "Delete", dtx, dty)
}
