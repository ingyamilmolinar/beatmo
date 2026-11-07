package ui

import (
	"fmt"
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// ─── Sidebar sizing constants ──────────────────────────────────────────────
const (
	sidebarDefaultW = 260
	sidebarMinW     = 200
	sidebarMaxW     = 400
	sidebarBtnH     = 28 // unified button/row height
	sidebarIncBtnW  = 28 // inc/dec button width
	sidebarIncBtnH  = 24 // inc/dec button height
	sidebarGap      = 6
	sidebarPad      = 10
	sidebarHeaderH  = 36
	sidebarSectionH = 30 // collapsible section header height
	sidebarTabW     = 24 // collapsed expand-tab width
	sidebarInnerPad = 4  // inner padding for text within layout rects
	sidebarSwatchSz = 12 // instrument color swatch size in header
	// The nesting cue: an open dropdown reads as a submenu, not sibling rows.
	sidebarOptionIndent = SpaceLG
)

// Per call, so it re-derives with the density like the other tokens.
func sidebarLabelScale() float64 {
	return float64(Profile().DensityValues().SidebarLabelScale) / 1000
}

// Same metrics as DrawTextAtScale.
func sidebarScaledTextWidth(s string, scale float64) int {
	return int(float64(TextWidth(s)) * scale)
}

type NodeSidebar struct {
	game *Game
	open bool
	node *uiNode

	// Geometry
	width     int  // current width (fixed; user-resize removed)
	collapsed bool // fully hidden (show expand tab only)

	// For the non-Button clickables; Buttons get hover from their own input cadence.
	hover hoverFade

	// Zero value = auto placement beside the node. A drag pins the card to the grid pane; Open/Close
	// reset it so a pin never survives a reopen.
	drag PopupDragHandle

	// Sections — individually collapsible
	sectionOpen map[sbSection]bool
	// Section headers toggle directly (not through a Button's held counter), so they need their own
	// press edge or every held frame flips them again.
	secHeldPrev bool
	// A press that began on the open picker owns the gesture to its release, even after the picker
	// closes under it.
	pickerPressOwned bool
	pickerHeldPrev   bool
	pickerSeq        int // DrumView.updateSeq of the last HandleInput call

	// Scroll — uses reusable ScrollBehavior with pixel-level mode (ItemHeight=1)
	scroll      *ScrollBehavior
	deferredTap DeferredTap // two-phase tap: prevents accidental toggles during scroll

	// Controls
	rects map[sbID]image.Rectangle
	btns  map[sbID]*Button
	anim  map[sbID]float64

	closedGuard int // frames to suppress tap-to-create after close

	// Dropdown state
	logicDropdownOpen bool
	// Owned by the sidebar, not the portal: the sidebar renders through GridTree, and a portal
	// picker would paint outside the grid pane.
	noteMatrix              *NoteMatrixComponent
	noteMatrixKeyNav        *MenuKeyNav
	chordMatrix             *ChordMatrixComponent
	chordMatrixAnchor       sbID // the chip the chord matrix hangs from
	chordMatrixKeyNav       *MenuKeyNav
	chordTabHeld            bool // Tab edge state for the chord matrix's page key
	grooveDropdownOpen      bool
	groupAddDropdownOpen    bool
	groupRemoveDropdownOpen bool
}

// NewNodeSidebar creates a new sidebar attached to the game.
func NewNodeSidebar(g *Game) *NodeSidebar {
	return &NodeSidebar{
		game:        g,
		width:       sidebarDefaultW,
		sectionOpen: make(map[sbSection]bool),
		scroll:      NewScrollBehavior(ScrollbarStyleForPlatform(), 1),
		rects:       make(map[sbID]image.Rectangle),
		btns:        make(map[sbID]*Button),
		anim:        make(map[sbID]float64),
	}
}

// Renders through the grid pane's tree, not the global portal, so the card is clipped to the
// grid pane and never paints over the drum view.
func (sb *NodeSidebar) Open(node *uiNode) {
	sb.open = true
	sb.node = node
	sb.collapsed = false
	sb.scroll.VS.First = 0
	sb.scroll.ResetTouch()
	sb.logicDropdownOpen = false
	sb.grooveDropdownOpen = false
	sb.groupAddDropdownOpen = false
	sb.groupRemoveDropdownOpen = false
	sb.closeNoteMatrix()
	sb.closeChordMatrix()
	// Reset all sections to collapsed
	sb.sectionOpen = map[sbSection]bool{}
	sb.deferredTap.Cancel()
	sb.drag.Reset()
	sb.rects = make(map[sbID]image.Rectangle)
	sb.btns = make(map[sbID]*Button)
	sb.anim = make(map[sbID]float64)
	sb.layout()
}

// Close deactivates the sidebar.
func (sb *NodeSidebar) Close() {
	sb.open = false
	sb.node = nil
	sb.closeNoteMatrix()
	sb.closeChordMatrix()
	sb.closedGuard = 2
	sb.drag.Reset()
	sb.btns = make(map[sbID]*Button)
	sb.rects = make(map[sbID]image.Rectangle)
}

func (sb *NodeSidebar) fireGroupChip(gid model.GroupID) {
	g := sb.game
	panel := sb.rects[sbPanel]
	sb.Close()
	g.groupMenu.OpenAt(gid, panel.Min.X+20, panel.Min.Y+20)
}

// A provisional single-member group with no emit: Save in the GroupMenu emits the one creation
// step, and closing without Save discards it. Model errors are a silent no-op.
func (sb *NodeSidebar) createNewGroup() {
	g := sb.game
	node := sb.node
	if node == nil {
		return
	}
	panel := sb.rects[sbPanel]
	// A group holding exactly this node reopens committed instead of minting a duplicate.
	if gid, ok := g.findGroupWithExactNodeSet([]model.NodeID{node.ID}); ok {
		sb.Close()
		g.groupMenu.OpenAt(gid, panel.Min.X+20, panel.Min.Y+20)
		return
	}
	gid, err := g.graph.CreateGroup("", []model.NodeID{node.ID})
	if err != nil {
		g.logger.Debugf("[sidebar] CreateGroup(%d): %v", node.ID, err)
		return
	}
	sb.Close()
	g.groupMenu.OpenAtProvisional(gid, panel.Min.X+20, panel.Min.Y+20)
}

// Model errors (the group vanished mid-frame) are a silent no-op: no emit, no undo step.
func (sb *NodeSidebar) addToGroup(gid model.GroupID) {
	sb.groupAddDropdownOpen = false
	node := sb.node
	if node == nil {
		return
	}
	g := sb.game
	if err := g.graph.AddNodeToGroup(gid, node.ID); err != nil {
		g.logger.Debugf("[sidebar] AddNodeToGroup(%d, %d): %v", gid, node.ID, err)
		return
	}
	emitGroupChanged(gid)
}

// Emits EventGroupDeleted instead when removing the last member auto-deleted the group.
func (sb *NodeSidebar) removeFromGroup(gid model.GroupID) {
	node := sb.node
	if node == nil {
		return
	}
	g := sb.game
	if err := g.graph.RemoveNodeFromGroup(gid, node.ID); err != nil {
		g.logger.Debugf("[sidebar] RemoveNodeFromGroup(%d, %d): %v", gid, node.ID, err)
		return
	}
	if _, ok := g.graph.Group(gid); ok {
		emitGroupChanged(gid)
	} else {
		emitGroupDeleted(gid)
	}
}

// IsOpen returns true if the sidebar is visible.
func (sb *NodeSidebar) IsOpen() bool { return sb.open }

// Node returns the currently-selected node, or nil.
func (sb *NodeSidebar) Node() *uiNode { return sb.node }

// ClosedGuard returns the current closed-guard frame counter.
func (sb *NodeSidebar) ClosedGuard() int { return sb.closedGuard }

// DecrementClosedGuard reduces the guard counter by one (called from game update).
func (sb *NodeSidebar) DecrementClosedGuard() {
	if sb.closedGuard > 0 {
		sb.closedGuard--
	}
}

// Hit reports whether a screen-space point lies within the sidebar panel.
func (sb *NodeSidebar) Hit(x, y int) bool {
	if sb.collapsed {
		r := sb.expandTabRect()
		return image.Pt(x, y).In(r)
	}
	if !sb.open || sb.node == nil {
		return false
	}
	// The picker floats outside the card, so its panel must be in the hit region or the dispatcher
	// never calls HandleInput and every cell is dead.
	if p := sb.openPicker(); p != nil {
		if r := p.InputBounds(); !r.Empty() && image.Pt(x, y).In(r) {
			return true
		}
	}
	if r, ok := sb.rects[sbPanel]; ok {
		return image.Pt(x, y).In(r)
	}
	return false
}

// The ONE list every header consumer reads; a header missing from a hand-kept copy is dead.
var nodeSidebarSections = []sbSection{secVol, secPit, secChord, secDur, secLogic, secGroove, secAud, secGrp}

// ExpandAllSections opens all collapsible sections (for tests).
func (sb *NodeSidebar) ExpandAllSections() {
	for _, s := range nodeSidebarSections {
		sb.sectionOpen[s] = true
	}
}

// expandTabRect returns the rect for the collapsed expand tab.
func (sb *NodeSidebar) expandTabRect() image.Rectangle {
	gridH := sb.game.split.GridH(sb.game.winH)
	return image.Rect(0, 0, sidebarTabW, gridH)
}

// ─── Layout ────────────────────────────────────────────────────────────────

// layout recomputes all rects in screen space using fixed sizes.
func (sb *NodeSidebar) layout() {
	sb.rects = make(map[sbID]image.Rectangle)
	if !sb.open || sb.node == nil {
		return
	}
	g := sb.game
	gridH := g.split.GridH(g.winH)

	// Clamp width
	w := sb.width
	if w < sidebarMinW {
		w = sidebarMinW
	}
	if w > sidebarMaxW {
		w = sidebarMaxW
	}
	if w > g.winW {
		w = g.winW
	}
	sb.width = w

	// Sized to its content and anchored beside the node, below.

	// Determine node type visibility
	nodeType := model.NodeTypeRegular
	if n, ok := g.graph.GetNodeByID(sb.node.ID); ok {
		nodeType = n.Type
	}
	isSilent := nodeType == model.NodeTypeSilent
	isMute := nodeType == model.NodeTypeMute
	showVol := !isSilent && !isMute
	showPitch := !isSilent && !isMute
	showDur := !isSilent && !isMute
	showLogic := !isSilent
	showGroove := !isSilent

	// The title band is snug so the always-visible coordinate line stays cheap; a second line
	// (live fires) only while playing.
	closeSize := sidebarBtnH
	lineH := StyledTextHeight(RoleCaption)
	titleBandH := StyledTextHeight(menuTitleRole) + 4
	coordLines := 1
	if g.Playing() {
		coordLines = 2
	}
	headerH := titleBandH + coordLines*lineH
	headerRect := image.Rect(sidebarPad, sidebarPad, w-sidebarPad, sidebarPad+headerH)
	sb.rects[sbHeader] = headerRect
	sb.rects[sbCoords] = image.Rect(sidebarPad, sidebarPad+titleBandH, w-sidebarPad, sidebarPad+headerH)
	// Desktop only; delete keeps a profile flip from leaving a stale rect.
	if desktopCloseVisible() {
		sb.rects[sbClose] = image.Rect(w-sidebarPad-closeSize, sidebarPad, w-sidebarPad, sidebarPad+closeSize)
	} else {
		delete(sb.rects, sbClose)
	}

	// Content starts below the header band.
	contentTop := headerRect.Max.Y + sidebarGap

	// Layout scrollable content
	y := contentTop - sb.scroll.VS.First
	btnX := w - sidebarPad - 2*sidebarBtnH - sidebarGap

	// Logic kind for parameter display
	kind := model.LogicNone
	if mn, ok := g.graph.GetNodeByID(sb.node.ID); ok {
		kind = mn.Params.LogicKind
	}

	// Section: Volume
	if showVol {
		y = sb.layoutSection(y, w, btnX, secVol, i18n.T(i18n.KeyNodeSecVolume), true)
	}
	// Section: Pitch
	if showPitch {
		y = sb.layoutSection(y, w, btnX, secPit, i18n.T(i18n.KeyCapPitch), true)
	}
	// Section: Chord (chord nodes only — the chord builder)
	if nodeType == model.NodeTypeChord {
		y = sb.layoutChordSection(y, w)
	}
	// Section: Duration
	if showDur {
		y = sb.layoutSection(y, w, btnX, secDur, i18n.T(i18n.KeyNodeSecDuration), true)
	}
	// Section: Logic
	if showLogic {
		y = sb.layoutLogicSection(y, w, btnX, kind)
	}
	// Section: Groove
	if showGroove {
		y = sb.layoutGrooveSection(y, w, btnX)
	}
	// Section: Audible
	{
		sb.rects[sbSectionHeader(secAud)] = image.Rect(sidebarPad, y, w-sidebarPad, y+sidebarSectionH)
		y += sidebarSectionH
		if sb.sectionOpen[secAud] {
			sb.rects[sbAudible] = image.Rect(sidebarPad, y, w-sidebarPad, y+sidebarBtnH)
			y += sidebarBtnH + sidebarGap
		}
	}
	// No Move section: copy/paste and the long-press popup cover it. Groups always shows, so a
	// project with no groups can still create one.
	y = sb.layoutGroupSection(y, w, sb.node.ID)

	// Compute scroll limits via ScrollBehavior
	contentH := y + sb.scroll.VS.First - contentTop
	// Capped to the grid pane; a tall node scrolls the overflow.
	cardH := contentTop + contentH + sidebarPad
	if cardH > gridH {
		cardH = gridH
	}
	viewportH := cardH - contentTop
	sb.rects[sbPanel] = image.Rect(0, 0, w, cardH)
	sb.scroll.VS.Total = contentH
	sb.scroll.VS.Visible = viewportH
	sb.scroll.VS.View = image.Rect(w-sb.scroll.Style.Width, contentTop, w, contentTop+viewportH)
	sb.scroll.VS.Clamp()

	// Clamped to the GRID PANE, not the window, so it never renders over the drum view. Child rects
	// were computed at (0,0) and are translated to the anchor.
	x1, y1, x2, y2 := g.nodeScreenRect(sb.node)
	nodeR := image.Rect(int(x1), int(y1), int(x2), int(y2))
	gridPane := image.Rect(0, 0, g.split.GridW(g.winW), gridH)
	anchored := AnchorPopupRect(gridPane, nodeR, w, cardH, PopupBelow)
	// The whole button cluster: the card's close × would otherwise sit under the higher-z logs
	// button, which swallows the click.
	if avoid := g.gridTopButtonsReservedRect(); !avoid.Empty() {
		anchored = avoidReservedRect(anchored, avoid, gridPane.Dx(), gridPane.Dy())
	}
	// Auto mode returns `anchored` verbatim; once dragged, the pinned origin re-clamped to the
	// current card size.
	placed := sb.drag.Place(anchored, gridPane, g.gridTopButtonsReservedRect())
	origin := placed.Min
	if origin.X != 0 || origin.Y != 0 {
		for k, r := range sb.rects {
			sb.rects[k] = r.Add(origin)
		}
		sb.scroll.VS.View = sb.scroll.VS.View.Add(origin)
	}

	// In final screen space, so draw and hit-test agree.
	sb.rects[sbGrip] = sb.drag.GripRect(sb.rects[sbPanel])

	// Already in screen space, so after the translate and before wireButtons.
	sb.layoutNoteMatrixCells()
	sb.layoutChordMatrixCells()

	// Wire button objects
	sb.wireButtons()
}

// Fits "A#-1" at body size.
const sidebarNoteChipW = 40

func (sb *NodeSidebar) layoutSection(y, w, btnX int, sec sbSection, _ string, hasButtons bool) int {
	sb.rects[sbSectionHeader(sec)] = image.Rect(sidebarPad, y, w-sidebarPad, y+sidebarSectionH)
	y += sidebarSectionH
	if !sb.sectionOpen[sec] {
		return y
	}
	if hasButtons {
		plusX := w - sidebarPad - sidebarIncBtnW
		// pill between buttons
		pillW := Profile().DensityValues().SidebarValuePillW
		pillX := plusX - sidebarGap - pillW
		// −button to the left of pill
		minusX := pillX - sidebarGap - sidebarIncBtnW

		btnY := y + (sidebarBtnH-sidebarIncBtnH)/2 // vertically center 24px buttons in 28px row
		if sec == secPit {
			// Fixed width so the row does not jitter between "C4" and "A#4"; skipped for percussion.
			noteX := minusX - sidebarGap - sidebarNoteChipW
			if sb.nodeIsChord() {
				// A chord name needs the whole left gap: the chip replaces the caption.
				noteX = sidebarPad
			}
			sb.rects[sbPitchNote] = image.Rect(noteX, btnY, minusX-sidebarGap, btnY+sidebarIncBtnH)
		}
		f := sectionField[sec]
		sb.rects[sbMinus(f)] = image.Rect(minusX, btnY, minusX+sidebarIncBtnW, btnY+sidebarIncBtnH)
		sb.rects[sbValue(f)] = image.Rect(pillX, btnY, pillX+pillW, btnY+sidebarIncBtnH)
		sb.rects[sbPlus(f)] = image.Rect(plusX, btnY, plusX+sidebarIncBtnW, btnY+sidebarIncBtnH)
	}
	y += sidebarBtnH + sidebarGap
	return y
}

func (sb *NodeSidebar) layoutChordSection(y, w int) int {
	sb.rects[sbSectionHeader(secChord)] = image.Rect(sidebarPad, y, w-sidebarPad, y+sidebarSectionH)
	y += sidebarSectionH
	if !sb.sectionOpen[secChord] {
		return y
	}
	plusX := w - sidebarPad - sidebarIncBtnW
	pillW := Profile().DensityValues().SidebarValuePillW
	pillX := plusX - sidebarGap - pillW
	minusX := pillX - sidebarGap - sidebarIncBtnW
	// The lead control, so choosing a chord never detours through the Pitch section.
	chipY := y + (sidebarBtnH-sidebarIncBtnH)/2
	sb.rects[sbChordName] = image.Rect(sidebarPad, chipY, w-sidebarPad, chipY+sidebarIncBtnH)
	y += sidebarBtnH + sidebarGap
	for _, f := range []sbField{fieldChordInv, fieldChordSpread} {
		btnY := y + (sidebarBtnH-sidebarIncBtnH)/2
		sb.rects[sbMinus(f)] = image.Rect(minusX, btnY, minusX+sidebarIncBtnW, btnY+sidebarIncBtnH)
		sb.rects[sbValue(f)] = image.Rect(pillX, btnY, pillX+pillW, btnY+sidebarIncBtnH)
		sb.rects[sbPlus(f)] = image.Rect(plusX, btnY, plusX+sidebarIncBtnW, btnY+sidebarIncBtnH)
		y += sidebarBtnH + sidebarGap
	}
	sb.rects[sbChordSingle] = image.Rect(sidebarPad, y, w-sidebarPad, y+sidebarBtnH)
	y += sidebarBtnH + sidebarGap
	return y
}

// Holds the card in place and scrolls by however far id moved, so a type change that adds or
// removes sections never pushes the next tap's target out from under the pointer.
func (sb *NodeSidebar) keepUnderPointer(id sbID, change func()) {
	before := sb.rects[id]
	if panel := sb.rects[sbPanel]; !panel.Empty() {
		sb.drag.Hold(panel.Min)
	}
	change()
	if before.Empty() || !sb.open || sb.node == nil {
		return
	}
	sb.layout()
	after := sb.rects[id]
	if after.Empty() {
		return
	}
	sb.scroll.VS.First += after.Min.Y - before.Min.Y
	sb.scroll.VS.Clamp()
	sb.layout()
	if r := sb.rects[id]; !r.Empty() {
		view := sb.scroll.VS.View
		switch {
		case r.Max.Y > view.Max.Y:
			sb.scroll.VS.First += r.Max.Y - view.Max.Y
		case r.Min.Y < view.Min.Y:
			sb.scroll.VS.First -= view.Min.Y - r.Min.Y
		}
		sb.scroll.VS.Clamp()
		sb.layout()
	}
}

// nodeIsChord reports whether the open node is a chord node.
func (sb *NodeSidebar) nodeIsChord() bool {
	if sb.node == nil {
		return false
	}
	n, ok := sb.game.graph.GetNodeByID(sb.node.ID)
	return ok && n.Type == model.NodeTypeChord
}

// layoutLogicSection lays out the logic section with dropdown and params.
func (sb *NodeSidebar) layoutLogicSection(y, w, btnX int, kind model.LogicKind) int {
	sb.rects[sbSectionHeader(secLogic)] = image.Rect(sidebarPad, y, w-sidebarPad, y+sidebarSectionH)
	y += sidebarSectionH
	if !sb.sectionOpen[secLogic] {
		return y
	}
	// Logic selector button
	sb.rects[sbLogicSelect] = image.Rect(sidebarPad, y, w-sidebarPad, y+sidebarBtnH)
	y += sidebarBtnH + sidebarGap

	if sb.logicDropdownOpen {
		for _, o := range model.LogicKinds() {
			sb.rects[sbLogicOption(o)] = image.Rect(sidebarPad+sidebarOptionIndent, y, w-sidebarPad, y+sidebarBtnH)
			y += sidebarBtnH + 2
		}
		y += sidebarGap - 2
	} else {
		// Parameter adjusters — same [−] [pill] [+] layout as vol/pit/dur
		kindHasN := kind == model.LogicEveryN || kind == model.LogicSkipEveryN
		kindHasP := kind == model.LogicProbability
		if kindHasN {
			plusX := w - sidebarPad - sidebarIncBtnW
			pillW := Profile().DensityValues().SidebarValuePillW
			pillX := plusX - sidebarGap - pillW
			minusX := pillX - sidebarGap - sidebarIncBtnW
			btnY := y + (sidebarBtnH-sidebarIncBtnH)/2
			sb.rects[sbMinus(fieldLogicN)] = image.Rect(minusX, btnY, minusX+sidebarIncBtnW, btnY+sidebarIncBtnH)
			sb.rects[sbValue(fieldLogicN)] = image.Rect(pillX, btnY, pillX+pillW, btnY+sidebarIncBtnH)
			sb.rects[sbPlus(fieldLogicN)] = image.Rect(plusX, btnY, plusX+sidebarIncBtnW, btnY+sidebarIncBtnH)
			y += sidebarBtnH + sidebarGap
		} else if kindHasP {
			plusX := w - sidebarPad - sidebarIncBtnW
			pillW := Profile().DensityValues().SidebarValuePillW
			pillX := plusX - sidebarGap - pillW
			minusX := pillX - sidebarGap - sidebarIncBtnW
			btnY := y + (sidebarBtnH-sidebarIncBtnH)/2
			sb.rects[sbMinus(fieldLogicP)] = image.Rect(minusX, btnY, minusX+sidebarIncBtnW, btnY+sidebarIncBtnH)
			sb.rects[sbValue(fieldLogicP)] = image.Rect(pillX, btnY, pillX+pillW, btnY+sidebarIncBtnH)
			sb.rects[sbPlus(fieldLogicP)] = image.Rect(plusX, btnY, plusX+sidebarIncBtnW, btnY+sidebarIncBtnH)
			y += sidebarBtnH + sidebarGap
		}
	}
	return y
}

// layoutGrooveSection lays out the groove section with dropdown and pct.
func (sb *NodeSidebar) layoutGrooveSection(y, w, btnX int) int {
	sb.rects[sbSectionHeader(secGroove)] = image.Rect(sidebarPad, y, w-sidebarPad, y+sidebarSectionH)
	y += sidebarSectionH
	if !sb.sectionOpen[secGroove] {
		return y
	}
	// Groove selector button
	sb.rects[sbGrooveSelect] = image.Rect(sidebarPad, y, w-sidebarPad, y+sidebarBtnH)
	y += sidebarBtnH + sidebarGap

	if sb.grooveDropdownOpen {
		for _, kind := range []model.GrooveKind{model.GrooveNone, model.GrooveDelay, model.GrooveRush} {
			sb.rects[sbGrooveOption(kind)] = image.Rect(sidebarPad+sidebarOptionIndent, y, w-sidebarPad, y+sidebarBtnH)
			y += sidebarBtnH + 2
		}
		y += sidebarGap - 2
	} else {
		// Pct adjusters — same [−] [pill] [+] layout
		plusX := w - sidebarPad - sidebarIncBtnW
		pillW := Profile().DensityValues().SidebarValuePillW
		pillX := plusX - sidebarGap - pillW
		minusX := pillX - sidebarGap - sidebarIncBtnW
		btnY := y + (sidebarBtnH-sidebarIncBtnH)/2
		sb.rects[sbMinus(fieldGroovePct)] = image.Rect(minusX, btnY, minusX+sidebarIncBtnW, btnY+sidebarIncBtnH)
		sb.rects[sbValue(fieldGroovePct)] = image.Rect(pillX, btnY, pillX+pillW, btnY+sidebarIncBtnH)
		sb.rects[sbPlus(fieldGroovePct)] = image.Rect(plusX, btnY, plusX+sidebarIncBtnW, btnY+sidebarIncBtnH)
		y += sidebarBtnH + sidebarGap
	}
	return y
}

// Always present: Create first (so a project with no groups has a row), then membership chips
// (open-only), then Add (only when a non-member group exists), then Remove (only with a membership).
func (sb *NodeSidebar) layoutGroupSection(y, w int, nodeID model.NodeID) int {
	sb.rects[sbSectionHeader(secGrp)] = image.Rect(sidebarPad, y, w-sidebarPad, y+sidebarSectionH)
	y += sidebarSectionH
	if !sb.sectionOpen[secGrp] {
		return y
	}
	sb.rects[sbGroupNew] = image.Rect(sidebarPad, y, w-sidebarPad, y+sidebarBtnH)
	y += sidebarBtnH + sidebarGap

	g := sb.game
	grpIDs := g.graph.GroupsForNode(nodeID)
	memberSet := make(map[model.GroupID]bool, len(grpIDs))
	for _, gid := range grpIDs {
		memberSet[gid] = true
	}
	for _, gid := range grpIDs {
		sb.rects[sbGroupChip(gid)] = image.Rect(sidebarPad, y, w-sidebarPad, y+sidebarBtnH)
		y += sidebarBtnH + sidebarGap
	}

	hasCandidate := false
	for _, grp := range g.graph.AllGroups() {
		if !memberSet[grp.ID] {
			hasCandidate = true
			break
		}
	}
	if !hasCandidate {
		// No candidate left, so the dropdown cannot be reached.
		sb.groupAddDropdownOpen = false
	} else {
		sb.rects[sbGroupAdd] = image.Rect(sidebarPad, y, w-sidebarPad, y+sidebarBtnH)
		y += sidebarBtnH + sidebarGap
		if sb.groupAddDropdownOpen {
			for _, grp := range g.graph.AllGroups() {
				if memberSet[grp.ID] {
					continue
				}
				sb.rects[sbGroupAddOption(grp.ID)] = image.Rect(sidebarPad+sidebarOptionIndent, y, w-sidebarPad, y+sidebarBtnH)
				y += sidebarBtnH + 2
			}
			y += sidebarGap - 2
		}
	}

	if len(grpIDs) == 0 {
		// No membership left, so the dropdown cannot be reached.
		sb.groupRemoveDropdownOpen = false
	} else {
		sb.rects[sbGroupRemove] = image.Rect(sidebarPad, y, w-sidebarPad, y+sidebarBtnH)
		y += sidebarBtnH + sidebarGap
		if sb.groupRemoveDropdownOpen {
			for _, gid := range grpIDs {
				sb.rects[sbGroupRemoveOption(gid)] = image.Rect(sidebarPad+sidebarOptionIndent, y, w-sidebarPad, y+sidebarBtnH)
				y += sidebarBtnH + 2
			}
			y += sidebarGap - 2
		}
	}
	return y
}

// ─── Button wiring ─────────────────────────────────────────────────────────

func (sb *NodeSidebar) wireButtons() {
	g := sb.game
	node := sb.node
	if node == nil {
		return
	}

	mk := func(id sbID, label string, onClick func()) {
		r, ok := sb.rects[id]
		if !ok || r.Empty() {
			delete(sb.btns, id)
			return
		}
		if _, ok := sb.btns[id]; !ok {
			// The shared menu keycap style, so the node pop-up reads like every other menu.
			sb.btns[id] = NewButton(label, DropdownStyle, func() {
				g.enqueueUI(onClick)
				sb.anim[id] = 1
			})
		} else {
			sb.btns[id].Text = label
			sb.btns[id].OnClick = func() {
				g.enqueueUI(onClick)
				sb.anim[id] = 1
			}
		}
		sb.btns[id].SetRect(r)
		sb.btns[id].ConsumeOnPress = true
		// Styled text, never the bitmap upscale path, which pixelates.
		sb.btns[id].SetTextRole(RoleBody)
		// The icon path gets the shared cushion, glow and chrome.
		if id.el == elMinus {
			sb.btns[id].Icon = "minus"
			sb.btns[id].Text = ""
			sb.btns[id].IconColor = colTextPrimary
		} else if id.el == elPlus {
			sb.btns[id].Icon = "plus"
			sb.btns[id].Text = ""
			sb.btns[id].IconColor = colTextPrimary
		}
	}

	// Volume +/-
	mk(sbMinus(fieldVol), "-", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			p.Volume = stepNodeVolume(p.Volume, false)
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
			emitNodeParamsChanged(node.ID, p)
		}
	})
	mk(sbPlus(fieldVol), "+", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			p.Volume = clampNodeVolume(stepNodeVolume(p.Volume, true))
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
			emitNodeParamsChanged(node.ID, p)
		}
	})
	// drawValuePill paints the readout; the Button only owns the tap.
	for _, f := range []sbField{fieldVol, fieldDur, fieldLogicN, fieldLogicP, fieldGroovePct} {
		mk(sbValue(f), "", func() { g.openNodeValueWheel(f) })
	}
	// Note-matrix cells, when the picker is open.
	sb.wireNoteMatrixCells(mk)
	sb.wireChordMatrixCells(mk)
	// Pitched instruments only; mk() drops it when the rect is absent.
	nodePitch := 0.0
	if mn, ok := g.graph.GetNodeByID(node.ID); ok {
		nodePitch = mn.Params.Pitch
	}
	if note := g.nodeNoteLabel(node.ID, nodePitch); note != "" {
		if sb.nodeIsChord() {
			note = g.nodeChordLabel(node.ID)
		}
		mk(sbPitchNote, note, func() {
			if sb.noteMatrixIsOpen() || sb.chordMatrixIsOpen() {
				sb.closeNoteMatrix()
				sb.closeChordMatrix()
				return
			}
			if sb.nodeIsChord() {
				sb.openChordMatrix(sbPitchNote) // a chord node's chip opens the CHORD matrix
				return
			}
			sb.openNoteMatrix()
		})
	} else {
		delete(sb.btns, sbPitchNote)
		delete(sb.rects, sbPitchNote)
	}
	// Pitch +/-
	mk(sbMinus(fieldPit), "-", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			p.Pitch -= 1
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
			emitNodeParamsChanged(node.ID, p)
		}
	})
	mk(sbPlus(fieldPit), "+", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			p.Pitch += 1
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
			emitNodeParamsChanged(node.ID, p)
		}
	})
	// Duration +/-
	mk(sbMinus(fieldDur), "-", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			p.Duration = clampNodeDuration(clampNodeDuration(p.Duration) - 0.1)
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
			emitNodeParamsChanged(node.ID, p)
		}
	})
	mk(sbPlus(fieldDur), "+", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			p.Duration = clampNodeDuration(clampNodeDuration(p.Duration) + 0.1)
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
			emitNodeParamsChanged(node.ID, p)
		}
	})
	// Logic selector
	mk(sbLogicSelect, "", func() {
		sb.logicDropdownOpen = !sb.logicDropdownOpen
		if sb.logicDropdownOpen {
			sb.grooveDropdownOpen = false
			sb.groupAddDropdownOpen = false
			sb.groupRemoveDropdownOpen = false
		}
	})
	// Logic dropdown items
	if sb.logicDropdownOpen {
		for _, kind := range model.LogicKinds() {
			mk(sbLogicOption(kind), logicKindLabel(kind), func() {
				if mn, ok := g.graph.GetNodeByID(node.ID); ok {
					p := mn.Params
					p.LogicKind = kind
					if kind == model.LogicEveryN || kind == model.LogicSkipEveryN {
						if p.LogicN <= 0 {
							p.LogicN = 2
						}
					}
					if kind == model.LogicProbability {
						if p.LogicP <= 0 {
							p.LogicP = 0.5
						}
					}
					g.graph.SetNodeParams(node.ID, p)
					g.notifyPredictorNode(node.ID)
					emitNodeParamsChanged(node.ID, p)
				}
				sb.logicDropdownOpen = false
				if g.engine != nil && g.engine.Predictor != nil {
					g.engine.Predictor.RebaseAt(g.elapsedBeats)
				}
			})
		}
	}
	// Logic N +/-
	mk(sbMinus(fieldLogicN), "-", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			p.LogicN = clampLogicN(p.LogicN - 1)
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
			emitNodeParamsChanged(node.ID, p)
			if g.engine != nil && g.engine.Predictor != nil {
				g.engine.Predictor.RebaseAt(g.elapsedBeats)
			}
		}
	})
	mk(sbPlus(fieldLogicN), "+", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			p.LogicN = clampLogicN(p.LogicN + 1)
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
			emitNodeParamsChanged(node.ID, p)
			if g.engine != nil && g.engine.Predictor != nil {
				g.engine.Predictor.RebaseAt(g.elapsedBeats)
			}
		}
	})
	// Logic P +/-
	mk(sbMinus(fieldLogicP), "-", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			p.LogicP -= 0.1
			if p.LogicP < 0 {
				p.LogicP = 0
			}
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
			emitNodeParamsChanged(node.ID, p)
			if g.engine != nil && g.engine.Predictor != nil {
				g.engine.Predictor.RebaseAt(g.elapsedBeats)
			}
		}
	})
	mk(sbPlus(fieldLogicP), "+", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			p.LogicP += 0.1
			if p.LogicP > 1 {
				p.LogicP = 1
			}
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
			emitNodeParamsChanged(node.ID, p)
			if g.engine != nil && g.engine.Predictor != nil {
				g.engine.Predictor.RebaseAt(g.elapsedBeats)
			}
		}
	})
	// Groove selector
	mk(sbGrooveSelect, "", func() {
		sb.grooveDropdownOpen = !sb.grooveDropdownOpen
		if sb.grooveDropdownOpen {
			sb.logicDropdownOpen = false
			sb.groupAddDropdownOpen = false
			sb.groupRemoveDropdownOpen = false
		}
	})
	// Groove dropdown items
	if sb.grooveDropdownOpen {
		for _, kind := range []model.GrooveKind{model.GrooveNone, model.GrooveDelay, model.GrooveRush} {
			mk(sbGrooveOption(kind), grooveKindLabel(kind), func() {
				if mn, ok := g.graph.GetNodeByID(node.ID); ok {
					p := mn.Params
					p.GrooveKind = kind
					if p.GroovePct < 0 {
						p.GroovePct = 0
					}
					if p.GroovePct > 1 {
						p.GroovePct = 1
					}
					g.graph.SetNodeParams(node.ID, p)
					g.notifyPredictorNode(node.ID)
					emitNodeParamsChanged(node.ID, p)
				}
				sb.grooveDropdownOpen = false
			})
		}
	}
	// Groove percentage +/-
	mk(sbMinus(fieldGroovePct), "-", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			p.GroovePct -= 0.05
			if p.GroovePct < 0 {
				p.GroovePct = 0
			}
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
			emitNodeParamsChanged(node.ID, p)
		}
	})
	mk(sbPlus(fieldGroovePct), "+", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			p.GroovePct += 0.05
			if p.GroovePct > 1 {
				p.GroovePct = 1
			}
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
			emitNodeParamsChanged(node.ID, p)
		}
	})
	// mk drops the buttons when the section is not laid out.
	mk(sbChordName, g.nodeChordLabel(node.ID), func() {
		if sb.chordMatrixIsOpen() {
			sb.closeChordMatrix()
			return
		}
		sb.openChordMatrix(sbChordName)
	})
	mk(sbMinus(fieldChordInv), "-", func() { g.stepNodeChord(node.ID, fieldChordInv, -1) })
	mk(sbPlus(fieldChordInv), "+", func() { g.stepNodeChord(node.ID, fieldChordInv, +1) })
	mk(sbMinus(fieldChordSpread), "-", func() { g.stepNodeChord(node.ID, fieldChordSpread, -1) })
	mk(sbPlus(fieldChordSpread), "+", func() { g.stepNodeChord(node.ID, fieldChordSpread, +1) })
	mk(sbChordSingle, i18n.T(i18n.KeyChordSingleNote), func() {
		g.setNodeType(node.ID, model.NodeTypeRegular) // the spec is kept
	})
	// Regular → Chord → Silent → Mute; Chord is skipped on an unpitched row.
	mk(sbAudible, i18n.T(i18n.KeyNodeSecAudible), func() {
		sb.keepUnderPointer(sbAudible, func() { g.cycleNodeType(node.ID) })
	})
	mk(sbGroupNew, i18n.T(i18n.KeyGroupCreateNew), func() {
		sb.createNewGroup()
	})
	grpIDs := g.graph.GroupsForNode(node.ID)
	grpIDSet := make(map[model.GroupID]bool, len(grpIDs))
	for _, gid := range grpIDs {
		gid := gid
		grpIDSet[gid] = true
		label := i18n.T(i18n.KeyGroupMenuTitle)
		if grp, ok := g.graph.Group(gid); ok && grp.Name != "" {
			label = grp.Name
		}
		mk(sbGroupChip(gid), label, func() {
			sb.fireGroupChip(gid)
		})
	}
	// mk()'s rect check hides the row when there is no candidate.
	var nonMemberGroups []model.NodeGroup
	for _, grp := range g.graph.AllGroups() {
		if !grpIDSet[grp.ID] {
			nonMemberGroups = append(nonMemberGroups, grp)
		}
	}
	mk(sbGroupAdd, i18n.T(i18n.KeyGroupAddTo), func() {
		sb.groupAddDropdownOpen = !sb.groupAddDropdownOpen
		if sb.groupAddDropdownOpen {
			sb.logicDropdownOpen = false
			sb.grooveDropdownOpen = false
			sb.groupRemoveDropdownOpen = false
		}
	})
	if sb.groupAddDropdownOpen {
		for _, grp := range nonMemberGroups {
			gid := grp.ID
			label := grp.Name
			if label == "" {
				label = i18n.T(i18n.KeyGroupMenuTitle)
			}
			mk(sbGroupAddOption(gid), label, func() {
				sb.addToGroup(gid)
			})
		}
	}
	// mk()'s rect check hides the row without a membership.
	mk(sbGroupRemove, i18n.T(i18n.KeyGroupRemoveFrom), func() {
		sb.groupRemoveDropdownOpen = !sb.groupRemoveDropdownOpen
		if sb.groupRemoveDropdownOpen {
			sb.logicDropdownOpen = false
			sb.grooveDropdownOpen = false
			sb.groupAddDropdownOpen = false
		}
	})
	if sb.groupRemoveDropdownOpen {
		for _, gid := range grpIDs {
			gid := gid
			label := i18n.T(i18n.KeyGroupMenuTitle)
			if grp, ok := g.graph.Group(gid); ok && grp.Name != "" {
				label = grp.Name
			}
			mk(sbGroupRemoveOption(gid), label, func() {
				sb.groupRemoveDropdownOpen = false
				sb.removeFromGroup(gid)
			})
		}
	}
	// The group section has a dynamic count, unlike the fixed ids above.
	for id := range sb.btns {
		gid := model.GroupID(id.arg)
		switch id.el {
		case elGroupChip:
			if !grpIDSet[gid] {
				delete(sb.btns, id)
			}
		case elGroupAddOpt:
			// These list NON-member groups, so grpIDSet must not gate them; prune when closed or vanished.
			if _, exists := sb.game.graph.Group(gid); !sb.groupAddDropdownOpen || !exists {
				delete(sb.btns, id)
			}
		case elGroupRemoveOpt:
			// These list MEMBER groups, so grpIDSet is the right gate here.
			if !sb.groupRemoveDropdownOpen || !grpIDSet[gid] {
				delete(sb.btns, id)
			}
		}
	}
	// No rect on mobile, so drop any stale button from a desktop layout.
	{
		r, ok := sb.rects[sbClose]
		if ok && !r.Empty() {
			if _, exists := sb.btns[sbClose]; !exists {
				sb.btns[sbClose] = NewButton("", DropdownStyle, func() {
					sb.Close()
				})
				sb.btns[sbClose].Icon = "close"
				sb.btns[sbClose].IconColor = closeIconColor()
			} else {
				sb.btns[sbClose].OnClick = func() {
					sb.Close()
				}
			}
			sb.btns[sbClose].SetRect(r)
			sb.btns[sbClose].ConsumeOnPress = true
		} else {
			delete(sb.btns, sbClose)
		}
	}
}

// ─── InputHandler interface ────────────────────────────────────────────────

// InputBounds returns the sidebar rect (or expand tab if collapsed).
func (sb *NodeSidebar) InputBounds() image.Rectangle {
	if sb.collapsed {
		return sb.expandTabRect()
	}
	panel, ok := sb.rects[sbPanel]
	if !ok {
		return image.Rectangle{}
	}
	// The picker floats outside the card and the dispatcher routes on this rect, so union it in.
	if p := sb.openPicker(); p != nil {
		if m := p.InputBounds(); !m.Empty() {
			return panel.Union(m)
		}
	}
	return panel
}

// ZIndex returns the z-order (above splitter and drumview).
func (sb *NodeSidebar) ZIndex() int { return 200 }

// Its touch taps resolve on the release-time grid tap, so the gesture must still be routed there.
func (sb *NodeSidebar) PassesGesturesThrough() bool { return true }

// The deferred-tap + scroll pattern, so touch scrolling never toggles a section.
func (sb *NodeSidebar) HandleInput(x, y int, pressed bool) InputResult {
	if sb.collapsed {
		if pressed {
			sb.collapsed = false
			sb.open = true
		}
		return InputConsumed
	}
	if !sb.open || sb.node == nil {
		return InputIgnored
	}

	// Shift+press starts a connect or marquee: yield the SAME press to handleEditor, or shift+drag
	// from a node under the card is dead.
	if pressed && (isKeyPressed(ebiten.KeyShiftLeft) || isKeyPressed(ebiten.KeyShiftRight)) {
		sb.Close()
		return InputIgnored
	}

	sb.layout()

	// Re-tapping the picked cell closes the picker on the press frame; the held pointer then sits
	// over the card, so the press stays the picker's until release. A gap since the last call resets it.
	if seq := sb.game.drum.updateSeq; sb.pickerSeq != seq && sb.pickerSeq != seq-1 {
		sb.pickerHeldPrev, sb.pickerPressOwned = false, false
	}
	sb.pickerSeq = sb.game.drum.updateSeq
	if pressed && !sb.pickerHeldPrev {
		p := sb.openPicker()
		sb.pickerPressOwned = p != nil && image.Pt(x, y).In(p.InputBounds())
	}
	sb.pickerHeldPrev = pressed
	if sb.pickerPressOwned && sb.openPicker() == nil {
		if !pressed {
			sb.pickerPressOwned = false
		}
		return InputConsumed
	}
	if !pressed {
		sb.pickerPressOwned = false
	}

	// Modal over its own panel; declines only presses on its selectable cells, so those reach the
	// shared Button path. Absorbing all made the cells dead; none let a dimmed cell fire what lies beneath.
	if r := sb.handleNoteMatrixInput(x, y, pressed); r != InputIgnored {
		return r
	}
	if r := sb.handleChordMatrixInput(x, y, pressed); r != InputIgnored {
		return r
	}

	// Captured here, where every call passes: most releases never reach the header branch, and
	// tracking it there would swallow the next real press edge.
	prevSecHeld := sb.secHeldPrev
	sb.secHeldPrev = pressed

	// InputCaptured routes later frames straight here, so the drag survives the cursor leaving the card.
	gridPaneR := image.Rect(0, 0, sb.game.split.GridW(sb.game.winW), sb.game.split.GridH(sb.game.winH))
	if sb.drag.Dragging() {
		panel := sb.rects[sbPanel]
		if pressed {
			sb.drag.MoveTo(image.Pt(x, y), gridPaneR, sb.game.gridTopButtonsReservedRect(),
				image.Pt(panel.Dx(), panel.Dy()))
			sb.layout()
		} else {
			sb.drag.End()
		}
		return InputCaptured
	}
	if pressed {
		if hit := sb.drag.HitRect(sb.rects[sbPanel]); !hit.Empty() && image.Pt(x, y).In(hit) {
			sb.drag.Begin(image.Pt(x, y), sb.rects[sbPanel])
			return InputCaptured
		}
	}

	panelR := sb.rects[sbPanel]

	// 2. Scrollbar drag in-progress
	if sb.scroll.Dragging() {
		if pressed {
			sb.scroll.HandleDragTo(y)
		} else {
			sb.scroll.HandleDragEnd()
		}
		return InputCaptured
	}

	// 3. ScrollingCommitted → handle scroll move, cancel deferred tap
	if sb.scroll.ScrollingCommitted() {
		if pressed {
			sb.scroll.HandleTouchMove(x, y)
		} else {
			sb.deferredTap.Cancel()
			sb.scroll.HandleTouchEnd()
		}
		return InputCaptured
	}

	if sb.scroll.TouchActive() {
		if pressed {
			sb.scroll.HandleTouchMove(x, y)
		} else {
			wasTap := !sb.scroll.ScrollingCommitted()
			sb.scroll.HandleTouchEnd()
			if wasTap {
				sb.deferredTap.End(sb.fireTapAt)
			} else {
				sb.deferredTap.Cancel()
			}
		}
		return InputCaptured
	}

	pt := image.Pt(x, y)

	// Never START a scroll under the open picker: it floats over the card, and a cell over the
	// scrollbar or viewport would be swallowed. A gesture already in progress still finishes.
	overPicker := sb.openPicker() != nil &&
		image.Pt(x, y).In(sb.openPicker().InputBounds())

	// 5. Scrollbar thumb click (before touch begin to avoid intercepting it)
	if !overPicker && sb.scroll.HasScroll() {
		thumbRect := sb.scroll.ThumbRect()
		if pressed && pt.In(thumbRect) {
			sb.scroll.HandleDragStart(y)
			return InputCaptured
		}
	}

	// Excludes the scrollbar track so track clicks reach their own step.
	contentTop := sb.contentTopY()
	viewport := image.Rect(panelR.Min.X, contentTop, panelR.Max.X, panelR.Max.Y)
	onScrollbarTrack := sb.scroll.HasScroll() && pt.In(sb.scroll.BarRect())
	if !overPicker && pressed && pt.In(viewport) && !onScrollbarTrack && !sb.scroll.TouchActive() && !sb.scroll.Dragging() {
		if sb.deferredTap.Begin(x, y) {
			sb.scroll.HandleTouchBegin(x, y)
			return InputCaptured
		}
	}

	// 7. Gate buttons + section headers during touch/deferred tap
	touchSuppressButtons := sb.scroll.TouchActive() || sb.deferredTap.Active()

	// 8. Hit-test buttons in z-order (only if not suppressed)
	if !touchSuppressButtons {
		order := sb.buttonOrder()
		for _, id := range order {
			btn, ok := sb.btns[id]
			if !ok {
				continue
			}
			r, ok := sb.rects[id]
			if !ok || r.Empty() {
				continue
			}
			btn.SetRect(r)
			if btn.HandleInputResult(x, y, pressed) != InputIgnored {
				return InputConsumed
			}
		}
	}

	// Press EDGE only (headers toggle directly, so a held press would keep flipping them), and never
	// while clicks are suppressed: that suppression is what keeps this branch from double-toggling a press
	// the deferred tap already owns.
	if !touchSuppressButtons && !suppressClicksUntilRelease && pressed && !prevSecHeld {
		for _, sec := range nodeSidebarSections {
			r, ok := sb.rects[sbSectionHeader(sec)]
			if !ok || r.Empty() {
				continue
			}
			if pt.In(r) {
				sb.toggleSection(sec)
				return InputConsumed
			}
		}
	}

	// 10. Close logic/groove dropdowns when clicking outside them
	if pressed && !touchSuppressButtons {
		sb.closeDropdownsOutside(x, y)
	}

	// 11. Scrollbar track click (not thumb) — jump scroll
	if pressed && sb.scroll.HasScroll() && pt.In(sb.scroll.BarRect()) {
		bar := sb.scroll.BarRect()
		trackH := bar.Dy()
		if trackH > 0 {
			frac := float64(y-bar.Min.Y) / float64(trackH)
			maxFirst := sb.scroll.VS.Total - sb.scroll.VS.Visible
			if maxFirst < 0 {
				maxFirst = 0
			}
			sb.scroll.VS.First = int(frac * float64(maxFirst))
			sb.scroll.VS.Clamp()
		}
		return InputConsumed
	}

	// 12. Release cleanup
	if !pressed {
		if sb.scroll.Dragging() {
			sb.scroll.HandleDragEnd()
		}
		if sb.scroll.TouchActive() {
			sb.scroll.HandleTouchEnd()
		}
	}

	// Consume any click inside the panel to prevent grid interaction
	return InputConsumed
}

// toggleSection toggles a section open/closed and cleans up related dropdowns.
func (sb *NodeSidebar) toggleSection(sec sbSection) {
	sb.sectionOpen[sec] = !sb.sectionOpen[sec]
	if !sb.sectionOpen[secLogic] {
		sb.logicDropdownOpen = false
	}
	if !sb.sectionOpen[secGroove] {
		sb.grooveDropdownOpen = false
	}
	if !sb.sectionOpen[secGrp] {
		sb.groupAddDropdownOpen = false
		sb.groupRemoveDropdownOpen = false
	}
}

// closeDropdownsOutside closes logic/groove dropdowns if click is outside them.
func (sb *NodeSidebar) closeDropdownsOutside(x, y int) {
	pt := image.Pt(x, y)
	if sb.logicDropdownOpen {
		inside := false
		if r, ok := sb.rects[sbLogicSelect]; ok && pt.In(r) {
			inside = true
		}
		for id, r := range sb.rects {
			if id.el == elLogicOption && pt.In(r) {
				inside = true
				break
			}
		}
		if !inside {
			sb.logicDropdownOpen = false
		}
	}
	if sb.grooveDropdownOpen {
		inside := false
		if r, ok := sb.rects[sbGrooveSelect]; ok && pt.In(r) {
			inside = true
		}
		for id, r := range sb.rects {
			if id.el == elGrooveOption && pt.In(r) {
				inside = true
				break
			}
		}
		if !inside {
			sb.grooveDropdownOpen = false
		}
	}
	if sb.groupAddDropdownOpen {
		inside := false
		if r, ok := sb.rects[sbGroupAdd]; ok && pt.In(r) {
			inside = true
		}
		for id, r := range sb.rects {
			if id.el == elGroupAddOpt && pt.In(r) {
				inside = true
				break
			}
		}
		if !inside {
			sb.groupAddDropdownOpen = false
		}
	}
	if sb.groupRemoveDropdownOpen {
		inside := false
		if r, ok := sb.rects[sbGroupRemove]; ok && pt.In(r) {
			inside = true
		}
		for id, r := range sb.rects {
			if id.el == elGroupRemoveOpt && pt.In(r) {
				inside = true
				break
			}
		}
		if !inside {
			sb.groupRemoveDropdownOpen = false
		}
	}
}

func (sb *NodeSidebar) fireTapAt(x, y int) {
	pt := image.Pt(x, y)

	// Close button (highest z-order)
	if btn, ok := sb.btns[sbClose]; ok && btn != nil {
		if r, ok := sb.rects[sbClose]; ok && pt.In(r) && btn.OnClick != nil {
			btn.OnClick()
			return
		}
	}

	// Buttons in z-order
	order := sb.buttonOrder()
	for _, id := range order {
		if id == sbClose {
			continue // already checked
		}
		btn, ok := sb.btns[id]
		if !ok || btn == nil {
			continue
		}
		r, ok := sb.rects[id]
		if !ok || r.Empty() {
			continue
		}
		if pt.In(r) && btn.OnClick != nil {
			btn.OnClick()
			return
		}
	}

	// Section headers
	for _, sec := range nodeSidebarSections {
		r, ok := sb.rects[sbSectionHeader(sec)]
		if !ok || r.Empty() {
			continue
		}
		if pt.In(r) {
			sb.toggleSection(sec)
			return
		}
	}

	// Close dropdowns if tap was outside them
	sb.closeDropdownsOutside(x, y)
}

// Same sign convention as ScrollBehavior.HandleWheel.
func (sb *NodeSidebar) HandleWheel(x, y int, steps int) InputResult {
	if !sb.open || sb.node == nil {
		return InputIgnored
	}
	sb.layout() // Ensure scroll limits are up-to-date
	// Over the picker the wheel scrolls its rows, not the card underneath.
	if p := sb.openPicker(); p != nil {
		if r := p.HandleWheel(x, y, steps); r != InputIgnored {
			return r
		}
	}
	sb.scroll.HandleWheel(steps * sidebarBtnH)
	return InputConsumed
}

// Every frame from Game.Update: HandleInput only runs while the cursor is over the sidebar.
func (sb *NodeSidebar) UpdateScroll() {
	// Pumped here so the arrows work wherever the pointer is.
	sb.updateNoteMatrixKeyNav()
	sb.updateChordMatrixKeyNav()

	if !sb.open {
		return
	}
	sb.scroll.UpdateMomentum()
}

// Capturing reports if the sidebar is in an active drag.
func (sb *NodeSidebar) Capturing() bool {
	return sb.drag.Dragging() || sb.scroll.TouchActive() || sb.scroll.Dragging()
}

// buttonOrder returns button ids in z-order (highest first).
func (sb *NodeSidebar) buttonOrder() []sbID {
	// The cells float above everything, so highest z. fireTapAt walks this list: a Button missing
	// from it is dead however it is registered and drawn.
	order := append([]sbID{}, sb.noteMatrixCellIDs()...)
	order = append(order, sb.chordMatrixCellIDs()...)
	order = append(order, sbClose)
	if sb.groupAddDropdownOpen {
		for id := range sb.rects {
			if id.el == elGroupAddOpt {
				order = append(order, id)
			}
		}
	}
	if sb.groupRemoveDropdownOpen {
		for id := range sb.rects {
			if id.el == elGroupRemoveOpt {
				order = append(order, id)
			}
		}
	}
	if sb.grooveDropdownOpen {
		for id := range sb.rects {
			if id.el == elGrooveOption {
				order = append(order, id)
			}
		}
	}
	if sb.logicDropdownOpen {
		for id := range sb.rects {
			if id.el == elLogicOption {
				order = append(order, id)
			}
		}
	}
	order = append(order, sbMinus(fieldLogicN), sbPlus(fieldLogicN), sbMinus(fieldLogicP), sbPlus(fieldLogicP), sbMinus(fieldGroovePct), sbPlus(fieldGroovePct))
	order = append(order, sbLogicSelect, sbGrooveSelect, sbGroupAdd, sbGroupRemove, sbGroupNew)
	// Before the steppers: it sits just left of "pit-" and must win an overlap.
	order = append(order, sbPitchNote)
	order = append(order, sbMinus(fieldVol), sbPlus(fieldVol), sbMinus(fieldPit), sbPlus(fieldPit), sbMinus(fieldDur), sbPlus(fieldDur), sbAudible)
	order = append(order, sbChordName, sbMinus(fieldChordInv), sbPlus(fieldChordInv), sbMinus(fieldChordSpread), sbPlus(fieldChordSpread), sbChordSingle)
	order = append(order, sbValue(fieldVol), sbValue(fieldDur), sbValue(fieldLogicN), sbValue(fieldLogicP), sbValue(fieldGroovePct))
	for id := range sb.rects {
		if id.el == elGroupChip {
			order = append(order, id)
		}
	}
	return order
}

// The one source for where scrolled content begins, derived from the laid-out header so input,
// viewport and Draw agree when the header grows a second line.
func (sb *NodeSidebar) contentTopY() int {
	if hr, ok := sb.rects[sbHeader]; ok && !hr.Empty() {
		return hr.Max.Y + sidebarGap
	}
	return sidebarPad + sidebarHeaderH + sidebarGap
}

// Mobile has no hover.
func (sb *NodeSidebar) gripHovered() bool {
	if Profile().IsMobile() {
		return false
	}
	gr, ok := sb.rects[sbGrip]
	if !ok || gr.Empty() {
		return false
	}
	mx, my := cursorPosition()
	return image.Pt(mx, my).In(gr)
}

func (sb *NodeSidebar) inViewport(r image.Rectangle) bool {
	contentTop := sb.contentTopY()
	bottom := sb.game.split.GridH(sb.game.winH)
	if p, ok := sb.rects[sbPanel]; ok && !p.Empty() {
		bottom = p.Max.Y // compact card bottom (panel is content-sized + anchored)
	}
	return r.Max.Y > contentTop && r.Min.Y < bottom
}

// ─── Draw ──────────────────────────────────────────────────────────────────

// Draw renders the sidebar onto the screen.
func (sb *NodeSidebar) Draw(dst *ebiten.Image) {
	if sb.collapsed {
		sb.drawExpandTab(dst)
		return
	}
	if !sb.open || sb.node == nil {
		return
	}

	sb.layout()
	g := sb.game
	panel := sb.rects[sbPanel]

	// Buttons and chips carry their own hover; this covers headers and action rows.
	hoverID := ""
	if !Profile().IsMobile() {
		mx, my := cursorPosition()
		pt := image.Pt(mx, my)
		for _, id := range sidebarHoverIDs {
			if r, ok := sb.rects[id]; ok && !r.Empty() && sb.inViewport(r) && pt.In(r) {
				hoverID = id.String()
				break
			}
		}
	}
	sb.hover.Advance(hoverID)

	// Panel background with stronger shadow and elevated surface
	drawPanelShadow(dst, panel, 6)
	drawRoundedRect(dst, panel, colPanelBG, RadiusContainer, true)
	drawRoundedRect(dst, panel, WithAlpha(genColorSidebarSectionBg, genAlphaSidebarSection), RadiusContainer, false)

	// Get node data
	mn, haveNode := g.graph.GetNodeByID(sb.node.ID)
	nodeType := model.NodeTypeRegular
	volVal := 1.0
	pitVal := 0.0
	durVal := 1.0
	if haveNode {
		nodeType = mn.Type
		volVal = mn.Params.Volume
		pitVal = mn.Params.Pitch
		durVal = mn.Params.Duration
	}

	contentTop := sb.contentTopY()

	sepColor := WithAlpha(genColorSidebarSectionBg, genAlphaSidebarSection)
	const secTextOffY = 0

	isSilent := nodeType == model.NodeTypeSilent
	isMute := nodeType == model.NodeTypeMute
	showVol := !isSilent && !isMute
	showPitch := !isSilent && !isMute
	showDur := !isSilent && !isMute
	showLogic := !isSilent
	showGroove := !isSilent

	_ = panel // used by layout; drawing uses sb.rects

	// Through a viewport SubImage, so a half-scrolled row can never paint over the title band.
	dstFull := dst
	dst = dstFull.SubImage(image.Rect(panel.Min.X, contentTop, panel.Max.X, panel.Max.Y)).(*ebiten.Image)

	// Volume section
	if showVol {
		sb.drawSectionHeader(dst, sbSectionHeader(secVol), i18n.T(i18n.KeyNodeSecVolume), secVol, sepColor, secTextOffY)
		if sb.sectionOpen[secVol] {
			if r := sb.rects[sbMinus(fieldVol)]; !r.Empty() && sb.inViewport(r) {
				y := r.Min.Y + (sidebarBtnH-StyledTextHeight(RoleCaption))/2
				DrawTextStyled(dst, i18n.T(i18n.KeyCapVol), sidebarPad+2, y, RoleCaption, colTextSecondary)
				sb.drawBtn(dst, sbMinus(fieldVol))
				sb.drawValuePill(dst, sbValue(fieldVol), formatNodeVolumeDb(volVal))
				sb.drawBtn(dst, sbPlus(fieldVol))
			}
		}
	}
	// Pitch section
	if showPitch {
		sb.drawSectionHeader(dst, sbSectionHeader(secPit), i18n.T(i18n.KeyCapPitch), secPit, sepColor, secTextOffY)
		if sb.sectionOpen[secPit] {
			if r := sb.rects[sbMinus(fieldPit)]; !r.Empty() && sb.inViewport(r) {
				y := r.Min.Y + (sidebarBtnH-StyledTextHeight(RoleCaption))/2
				if nodeType != model.NodeTypeChord { // the wide chord chip owns the gap
					DrawTextStyled(dst, i18n.T(i18n.KeyCapPitch), sidebarPad+2, y, RoleCaption, colTextSecondary)
				}
				sb.drawBtn(dst, sbMinus(fieldPit))
				sb.drawValuePill(dst, sbValue(fieldPit), fmt.Sprintf("%+d", int(pitVal)))
				sb.drawBtn(dst, sbPlus(fieldPit))
				// Derived live, so it follows pitch, synth and sampler edits; absent for percussion.
				sb.drawBtn(dst, sbPitchNote)
			}
		}
	}
	// Chord section (the chord builder)
	if nodeType == model.NodeTypeChord {
		sb.drawChordSection(dst, mn.Params.Chord, sepColor)
	}
	// Duration section
	if showDur {
		sb.drawSectionHeader(dst, sbSectionHeader(secDur), i18n.T(i18n.KeyNodeSecDuration), secDur, sepColor, secTextOffY)
		if sb.sectionOpen[secDur] {
			if r := sb.rects[sbMinus(fieldDur)]; !r.Empty() && sb.inViewport(r) {
				y := r.Min.Y + (sidebarBtnH-StyledTextHeight(RoleCaption))/2
				DrawTextStyled(dst, i18n.T(i18n.KeyCapDur), sidebarPad+2, y, RoleCaption, colTextSecondary)
				sb.drawBtn(dst, sbMinus(fieldDur))
				sb.drawValuePill(dst, sbValue(fieldDur), fmt.Sprintf("%.2fx", durVal))
				sb.drawBtn(dst, sbPlus(fieldDur))
			}
		}
	}
	// Logic section
	if showLogic {
		sb.drawSectionHeader(dst, sbSectionHeader(secLogic), i18n.T(i18n.KeyNodeSecLogic), secLogic, sepColor, secTextOffY)
		if sb.sectionOpen[secLogic] {
			if logicRect := sb.rects[sbLogicSelect]; !logicRect.Empty() && sb.inViewport(logicRect) {
				sb.drawBtn(dst, sbLogicSelect)
				yLogic := logicRect.Min.Y + (sidebarBtnH-StyledTextHeight(RoleBody))/2
				DrawTextStyled(dst, i18n.T(i18n.KeyNodeLogic), logicRect.Min.X+sidebarInnerPad, yLogic, RoleBody, colTextPrimary)
				// Current value
				cur := logicKindShortLabel(model.LogicNone)
				if haveNode {
					cur = logicKindShortLabel(mn.Params.LogicKind)
				}
				curW := StyledTextWidth(cur, RoleBody)
				curX := logicRect.Max.X - sidebarInnerPad - curW
				DrawTextStyled(dst, cur, curX, yLogic, RoleBody, colTextPrimary)
			}
			// Dropdown items
			if sb.logicDropdownOpen {
				var selID sbID
				if haveNode {
					selID = sbLogicOption(mn.Params.LogicKind)
				}
				for id := range sb.rects {
					if id.el == elLogicOption {
						sb.drawDropdownItem(dst, id, id == selID, sb.nodeAccent())
					}
				}
			} else if haveNode {
				// Parameter controls — styled inc/dec with value pill
				switch mn.Params.LogicKind {
				case model.LogicEveryN, model.LogicSkipEveryN:
					if lnRect := sb.rects[sbMinus(fieldLogicN)]; !lnRect.Empty() && sb.inViewport(lnRect) {
						DrawTextStyled(dst, "N", sidebarPad+2, lnRect.Min.Y+(sidebarBtnH-StyledTextHeight(RoleCaption))/2, RoleCaption, colTextSecondary)
						sb.drawBtn(dst, sbMinus(fieldLogicN))
						sb.drawValuePill(dst, sbValue(fieldLogicN), fmt.Sprintf("%d", mn.Params.LogicN))
						sb.drawBtn(dst, sbPlus(fieldLogicN))
					}
				case model.LogicProbability:
					if lpRect := sb.rects[sbMinus(fieldLogicP)]; !lpRect.Empty() && sb.inViewport(lpRect) {
						DrawTextStyled(dst, "P", sidebarPad+2, lpRect.Min.Y+(sidebarBtnH-StyledTextHeight(RoleCaption))/2, RoleCaption, colTextSecondary)
						sb.drawBtn(dst, sbMinus(fieldLogicP))
						sb.drawValuePill(dst, sbValue(fieldLogicP), fmt.Sprintf("%.0f%%", mn.Params.LogicP*100))
						sb.drawBtn(dst, sbPlus(fieldLogicP))
					}
				}
			}
		}
	}
	// Groove section
	if showGroove {
		sb.drawSectionHeader(dst, sbSectionHeader(secGroove), i18n.T(i18n.KeyNodeSecGroove), secGroove, sepColor, secTextOffY)
		if sb.sectionOpen[secGroove] {
			if grvRect := sb.rects[sbGrooveSelect]; !grvRect.Empty() && sb.inViewport(grvRect) {
				sb.drawBtn(dst, sbGrooveSelect)
				yGroove := grvRect.Min.Y + (sidebarBtnH-StyledTextHeight(RoleBody))/2
				DrawTextStyled(dst, i18n.T(i18n.KeyNodeGroove), grvRect.Min.X+sidebarInnerPad, yGroove, RoleBody, colTextPrimary)
				if haveNode {
					curGroove := grooveKindLabel(mn.Params.GrooveKind)
					grvW := StyledTextWidth(curGroove, RoleBody)
					grvX := grvRect.Max.X - sidebarInnerPad - grvW
					DrawTextStyled(dst, curGroove, grvX, yGroove, RoleBody, colTextPrimary)
				}
			}
			if sb.grooveDropdownOpen {
				var selID sbID
				if haveNode {
					selID = sbGrooveOption(mn.Params.GrooveKind)
				}
				for id := range sb.rects {
					if id.el == elGrooveOption {
						sb.drawDropdownItem(dst, id, id == selID, sb.nodeAccent())
					}
				}
			} else {
				if haveNode {
					if gpRect := sb.rects[sbMinus(fieldGroovePct)]; !gpRect.Empty() && sb.inViewport(gpRect) {
						DrawTextStyled(dst, i18n.T(i18n.KeyCapPct), sidebarPad+2, gpRect.Min.Y+(sidebarBtnH-StyledTextHeight(RoleCaption))/2, RoleCaption, colTextSecondary)
						sb.drawBtn(dst, sbMinus(fieldGroovePct))
						sb.drawValuePill(dst, sbValue(fieldGroovePct), fmt.Sprintf("%.0f%%", mn.Params.GroovePct*100))
						sb.drawBtn(dst, sbPlus(fieldGroovePct))
					}
				}
			}
		}
	}
	// Audible section
	{
		sb.drawSectionHeader(dst, sbSectionHeader(secAud), i18n.T(i18n.KeyNodeSecAudible), secAud, sepColor, secTextOffY)
		if sb.sectionOpen[secAud] {
			label := i18n.T(i18n.KeyNodeSecAudible)
			switch nodeType {
			case model.NodeTypeChord:
				label = i18n.T(i18n.KeyAudChord)
			case model.NodeTypeSilent:
				label = i18n.T(i18n.KeyAudSilent)
			case model.NodeTypeMute:
				label = i18n.T(i18n.KeyAudMuted)
			}
			sb.drawActionRow(dst, sbAudible, label)
		}
	}

	{
		sb.drawSectionHeader(dst, sbSectionHeader(secGrp), i18n.T(i18n.KeyGroupMenuTitle), secGrp, sepColor, secTextOffY)
		if sb.sectionOpen[secGrp] {
			sb.drawActionRow(dst, sbGroupNew, i18n.T(i18n.KeyGroupCreateNew))
			grpIDs := g.graph.GroupsForNode(sb.node.ID)
			for _, gid := range grpIDs {
				chipID := sbGroupChip(gid)
				r := sb.rects[chipID]
				if r.Empty() || !sb.inViewport(r) {
					continue
				}
				label := i18n.T(i18n.KeyGroupMenuTitle)
				if grp, ok := g.graph.Group(gid); ok && grp.Name != "" {
					label = grp.Name
				}
				// Navigational, so flat menu rows: identity in the swatch and hover tint, never a filled face.
				if btn := sb.btns[chipID]; btn != nil {
					state := menuItemRest
					if btn.hovered || btn.pressed {
						state = menuItemHover
					}
					drawMenuRow(dst, btn, MenuRowSpec{
						Accent:      g.groupColor(gid),
						State:       state,
						Swatch:      g.groupColor(gid),
						Label:       label,
						TrailIconID: IconChevronRight,
					})
				}
			}
			if addRect, ok := sb.rects[sbGroupAdd]; ok && !addRect.Empty() && sb.inViewport(addRect) {
				sb.drawActionRow(dst, sbGroupAdd, i18n.T(i18n.KeyGroupAddTo))
				if sb.groupAddDropdownOpen {
					for id := range sb.rects {
						if id.el == elGroupAddOpt {
							// A group wears its own color everywhere, not the node's accent.
							sb.drawDropdownItem(dst, id, false, g.groupColor(model.GroupID(id.arg)))
						}
					}
				}
			}
			if removeRect, ok := sb.rects[sbGroupRemove]; ok && !removeRect.Empty() && sb.inViewport(removeRect) {
				sb.drawActionRow(dst, sbGroupRemove, i18n.T(i18n.KeyGroupRemoveFrom))
				if sb.groupRemoveDropdownOpen {
					for id := range sb.rects {
						if id.el == elGroupRemoveOpt {
							sb.drawDropdownItem(dst, id, false, g.groupColor(model.GroupID(id.arg)))
						}
					}
				}
			}
		}
	}

	dst = dstFull

	// Fixed header, drawn after the clipped content so it always sits on top.
	sb.drawHeader(dst)
	sepY := sb.rects[sbHeader].Max.Y
	drawRect(dst, image.Rect(sidebarPad, sepY, panel.Max.X-sidebarPad, sepY+1), WithAlpha(genColorSidebarSectionBg, genAlphaSidebarSection), true)

	if btn, ok := sb.btns[sbClose]; ok && btn != nil {
		r := btn.Rect()
		if !r.Empty() {
			drawRect(dst, r, WithAlpha(genColorSidebarChipFill, genAlphaSidebarChip), true)
			drawRect(dst, r, colBorderMedium, false)
			btn.Draw(dst)
		}
	}

	// Scrollbar via ScrollBehavior
	sb.scroll.Draw(dst)

	// Decay animations
	for k, v := range sb.anim {
		if v > 0 {
			v *= 0.85
			if v < 0.02 {
				v = 0
			}
			sb.anim[k] = v
		}
	}

	// Last, above every control; clipped to the grid pane like the rest.
	sb.drawNoteMatrix(dst)
	sb.drawChordMatrix(dst)
}

// The node's row instrument color, so the sidebar reads as owned by that instrument.
func (sb *NodeSidebar) nodeAccent() color.Color {
	if sb.node != nil && sb.game != nil {
		if row, ok := sb.game.nodeRows[sb.node.ID]; ok && row >= 0 && row < len(sb.game.drum.Rows) {
			if c := sb.game.drum.Rows[row].Color; c != nil {
				return c
			}
		}
	}
	return colAccent
}

func (sb *NodeSidebar) drawHeader(dst *ebiten.Image) {
	g := sb.game
	headerRect := sb.rects[sbHeader]
	if headerRect.Empty() {
		return
	}

	// Shared "Neon Horizon" header band, tinted to the node's instrument color.
	drawMenuHeaderBandAccent(dst, headerRect, sb.nodeAccent())

	// The shared menu-title treatment, so it matches the row context menu exactly.
	swatchCol := color.Color(genColorSidebarSwatchFallback)
	label := i18n.T(i18n.KeyNodeTitle)
	if row, ok := g.nodeRows[sb.node.ID]; ok && row >= 0 && row < len(g.drum.Rows) {
		dr := g.drum.Rows[row]
		swatchCol = dr.Color
		label = dr.Name
	}
	coordR := sb.rects[sbCoords]
	titleBottom := headerRect.Min.Y + sidebarHeaderH
	if !coordR.Empty() {
		titleBottom = coordR.Min.Y
	}
	// Title starts clear of the grip glyph so the two never overlap.
	titleX := headerRect.Min.X + 2
	if gr, ok := sb.rects[sbGrip]; ok && !gr.Empty() {
		if x := gr.Max.X + sidebarGap; x > titleX {
			titleX = x
		}
	}
	titleRect := image.Rect(titleX, headerRect.Min.Y, headerRect.Max.X, titleBottom)
	drawMenuTitle(dst, titleRect, label, swatchCol)

	if gr, ok := sb.rects[sbGrip]; ok && !gr.Empty() {
		DrawIcon(dst, IconGrip, gr, popupGripTint(sb.drag.Dragging(), sb.gripHovered()))
	}

	// The live line only while playing; stopped, the home line already says where it fires next.
	if !coordR.Empty() && sb.node != nil {
		tm := g.nodeLiveTiming(sb.node.ID)
		x := coordR.Min.X + 2
		DrawTextStyled(dst, g.formatNodeCoordHome(sb.node.I, sb.node.J, tm), x, coordR.Min.Y, RoleCaption, colTextSecondary)
		if g.Playing() {
			if line2 := g.formatNodeFireLine(tm, true); line2 != "" {
				DrawTextStyled(dst, line2, x, coordR.Min.Y+StyledTextHeight(RoleCaption), RoleCaption, colTextSecondary)
			}
		}
	}
}

func (sb *NodeSidebar) drawSectionHeader(dst *ebiten.Image, rectID sbID, label string, sectionID sbSection, _ color.Color, _ int) {
	r, ok := sb.rects[rectID]
	if !ok || r.Empty() || !sb.inViewport(r) {
		return
	}
	expanded := sb.sectionOpen[sectionID]
	// Neutral: the chevron already shows the state, and a colored bar read as "selected".
	chevR := image.Rect(r.Min.X+sidebarPad, r.Min.Y, r.Min.X+sidebarPad+IconSizeMD, r.Max.Y)
	var chevCol color.Color = colTextSecondary
	var labelCol color.Color = colTextPrimary
	drawMenuChevron(dst, chevR, expanded, chevCol)

	th := StyledTextHeight(RoleSectionHeader)
	labelX := chevR.Max.X + sidebarGap
	labelY := r.Min.Y + (r.Dy()-th)/2
	DrawTextStyled(dst, label, labelX, labelY, RoleSectionHeader, labelCol)

	sb.drawSectionBadge(dst, r, sectionID)

	drawButtonHoverGlow(dst, r, RadiusSM, sb.hover.Lift(rectID.String()))
}

func (sb *NodeSidebar) drawSectionBadge(dst *ebiten.Image, r image.Rectangle, sectionID sbSection) {
	if sb.sectionOpen[sectionID] {
		return
	}
	badge := sb.sectionBadgeText(sectionID)
	if badge == "" {
		return
	}
	badgePadX := 6
	badgePadY := 2
	bw := StyledTextWidth(badge, RoleCaption) + 2*badgePadX
	bh := StyledTextHeight(RoleCaption) + 2*badgePadY
	bx := r.Max.X - bw - 2
	by := r.Min.Y + (sidebarSectionH-bh)/2
	pillR := image.Rect(bx, by, bx+bw, by+bh)
	drawRect(dst, pillR, colSurface2, true)
	DrawTextStyled(dst, badge, bx+badgePadX, by+badgePadY, RoleCaption, colTextSecondary)
}

// Every section shows its value even at the default, so a glance shows each state.
func (sb *NodeSidebar) sectionBadgeText(sectionID sbSection) string {
	if sb.node == nil {
		return ""
	}
	g := sb.game
	mn, ok := g.graph.GetNodeByID(sb.node.ID)
	if !ok {
		return ""
	}
	switch sectionID {
	case secVol:
		return formatNodeVolumeDb(mn.Params.Volume)
	case secPit:
		if mn.Type == model.NodeTypeChord {
			return g.nodeChordLabel(sb.node.ID)
		}
		if note := g.nodeNoteLabel(sb.node.ID, mn.Params.Pitch); note != "" {
			return note
		}
		return fmt.Sprintf("%+d", int(mn.Params.Pitch))
	case secDur:
		return fmt.Sprintf("%.2fx", mn.Params.Duration)
	case secLogic:
		switch mn.Params.LogicKind {
		case model.LogicEveryN:
			return fmt.Sprintf("Every %d", mn.Params.LogicN)
		case model.LogicSkipEveryN:
			return fmt.Sprintf("Skip %d", mn.Params.LogicN)
		case model.LogicProbability:
			return fmt.Sprintf("P %.0f%%", mn.Params.LogicP*100)
		case model.LogicIfPrevSkipped:
			return i18n.T(i18n.KeyLogicShortPrevSkip)
		case model.LogicIfPrevTriggered:
			return i18n.T(i18n.KeyLogicShortPrevTrig)
		}
		return i18n.T(i18n.KeyLogicNone)
	case secGroove:
		if k := mn.Params.GrooveKind; k != model.GrooveNone {
			return fmt.Sprintf("%s %.0f%%", grooveKindLabel(k), mn.Params.GroovePct*100)
		}
		return i18n.T(i18n.KeyLogicNone)
	case secChord:
		return g.nodeChordLabel(sb.node.ID)
	case secAud:
		switch mn.Type {
		case model.NodeTypeChord:
			return i18n.T(i18n.KeyAudChord)
		case model.NodeTypeSilent:
			return i18n.T(i18n.KeyAudSilent)
		case model.NodeTypeMute:
			return i18n.T(i18n.KeyAudMuted)
		}
		return i18n.T(i18n.KeyNodeSecAudible)
	}
	return ""
}

// drawExpandTab draws the collapsed sidebar expand tab.
func (sb *NodeSidebar) drawExpandTab(dst *ebiten.Image) {
	r := sb.expandTabRect()
	drawRoundedRect(dst, r, WithAlpha(genColorSidebarBadgeBg, genAlphaSidebarChip), RadiusXXS, true)
	if !Profile().IsMobile() {
		id := ""
		if mx, my := cursorPosition(); image.Pt(mx, my).In(r) {
			id = "expand-tab"
		}
		drawButtonHoverGlow(dst, r, RadiusXXS, sb.hover.Advance(id))
	}
	// Chevron-right at the unified icon-system size.
	dim := IconSizeMD
	cx := r.Min.X + r.Dx()/2
	cy := r.Min.Y + r.Dy()/2
	iconR := image.Rect(cx-dim/2, cy-dim/2, cx+dim/2, cy+dim/2)
	DrawIcon(dst, IconChevronRight, iconR, colTextSecondary)
}

// Faces stay neutral: the node's color lives in the header, swatch and dropdown stripes, never
// on button fills.
func (sb *NodeSidebar) drawBtn(dst *ebiten.Image, id sbID) {
	b := sb.btns[id]
	if b == nil {
		return
	}
	if r, ok := sb.rects[id]; ok && !sb.inViewport(r) {
		return
	}
	b.Draw(dst)
}

// First hit wins.
var sidebarHoverIDs = []sbID{
	sbSectionHeader(secVol), sbSectionHeader(secPit), sbSectionHeader(secChord), sbSectionHeader(secDur),
	sbSectionHeader(secLogic), sbSectionHeader(secGroove), sbSectionHeader(secAud), sbSectionHeader(secGrp),
	sbAudible, sbChordSingle, sbGroupNew, sbGroupAdd, sbGroupRemove,
}

func (sb *NodeSidebar) drawChordSection(dst *ebiten.Image, c model.ChordSpec, sepColor color.Color) {
	sb.drawSectionHeader(dst, sbSectionHeader(secChord), i18n.T(i18n.KeyNodeSecChord), secChord, sepColor, 0)
	if !sb.sectionOpen[secChord] {
		return
	}
	sb.drawBtn(dst, sbChordName)
	c = model.NormalizeChord(c)
	rows := []struct {
		field          sbField
		caption, value string
	}{
		{fieldChordInv, i18n.T(i18n.KeyChordInversion), chordInversionLabel(c.Inversion)},
		{fieldChordSpread, i18n.T(i18n.KeyChordSpread), chordSpreadLabel(c.Spread)},
	}
	for _, r := range rows {
		rect := sb.rects[sbMinus(r.field)]
		if rect.Empty() || !sb.inViewport(rect) {
			continue
		}
		y := rect.Min.Y + (sidebarBtnH-StyledTextHeight(RoleCaption))/2
		// Screen space: the card is anchored beside the node, not at x=0.
		x := sb.rects[sbPanel].Min.X + sidebarPad + 2
		DrawTextStyled(dst, r.caption, x, y, RoleCaption, colTextSecondary)
		sb.drawBtn(dst, sbMinus(r.field))
		sb.drawValuePill(dst, sbValue(r.field), r.value)
		sb.drawBtn(dst, sbPlus(r.field))
	}
	sb.drawActionRow(dst, sbChordSingle, i18n.T(i18n.KeyChordSingleNote))
}

func (sb *NodeSidebar) drawActionRow(dst *ebiten.Image, id sbID, label string) {
	r, ok := sb.rects[id]
	if !ok || r.Empty() || !sb.inViewport(r) {
		return
	}
	DropdownStyle.DrawAnimated(dst, r, false, sb.anim[id])
	DrawTextStyled(dst, label, r.Min.X+sidebarInnerPad,
		r.Min.Y+(sidebarBtnH-StyledTextHeight(RoleBody))/2, RoleBody, colTextPrimary)
	drawButtonHoverGlow(dst, r, RadiusSM, sb.hover.Lift(id.String()))
}

// Logic, Groove and Add-to-group rows take the node's color; Remove options take each group's
// own color, like the membership chips.
func (sb *NodeSidebar) drawDropdownItem(dst *ebiten.Image, id sbID, selected bool, accent color.Color) {
	b := sb.btns[id]
	if b == nil {
		return
	}
	r, ok := sb.rects[id]
	if !ok || r.Empty() || !sb.inViewport(r) {
		return
	}
	state := menuItemRest
	if selected {
		state = menuItemActive
	} else if b.hovered || b.pressed {
		state = menuItemHover
	}
	drawMenuRow(dst, b, MenuRowSpec{
		Accent: accent,
		State:  state,
		Label:  b.Text,
	})
}

func (sb *NodeSidebar) drawValuePill(dst *ebiten.Image, key sbID, value string) {
	r, ok := sb.rects[key]
	if !ok || r.Empty() || !sb.inViewport(r) {
		return
	}
	drawRecessedPill(dst, r, value)
}

// A recessed well, the tactile opposite of a keycap; pure so it can be reused and tested.
func drawRecessedPill(dst *ebiten.Image, r image.Rectangle, value string) {
	drawRoundedRect(dst, r, adjustColor(colSurface2, -22), fitRadius(RadiusControl, r), true)
	drawRect(dst, image.Rect(r.Min.X+2, r.Min.Y+1, r.Max.X-2, r.Min.Y+1+genGeomButtonInnerShadowPx),
		WithAlphaFromColor(color.Black, 56), true)
	drawRoundedRect(dst, r, colBorderMedium, fitRadius(RadiusControl, r), false)

	tw := StyledTextWidth(value, RoleBody)
	th := StyledTextHeight(RoleBody)
	tx := r.Min.X + (r.Dx()-tw)/2
	ty := r.Min.Y + (r.Dy()-th)/2
	DrawTextStyled(dst, value, tx, ty, RoleBody, colTextPrimary)
}
