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
	// sidebarOptionIndent is the extra left inset of dropdown OPTION rows
	// (Logic/Groove kinds, Add/Remove-group candidates) below their trigger
	// row — the "level below" nesting cue, so an open dropdown reads as a
	// submenu instead of flat sibling rows.
	sidebarOptionIndent = SpaceLG
)

// sidebarLabelScale is the density-driven text-scale multiplier for sidebar
// labels (was a hardcoded 1.2 const). Comfortable preserves the historical
// 1.2; Compact tightens to 1.1, Spacious opens to 1.3. Read per-call so it
// re-derives every Layout/Draw, matching the other density tokens.
func sidebarLabelScale() float64 {
	return float64(Profile().DensityValues().SidebarLabelScale) / 1000
}

// sidebarScaledTextWidth returns the pixel width of s rendered at the given
// scale, using the same font metrics as DrawTextAtScale.
func sidebarScaledTextWidth(s string, scale float64) int {
	return int(float64(TextWidth(s)) * scale)
}

// NodeSidebar is a left-anchored panel for editing node properties.
// It replaces the floating popup and uses fixed sizes + scrolling.
type NodeSidebar struct {
	game *Game
	open bool
	node *uiNode

	// Geometry
	width     int  // current width (fixed; user-resize removed)
	collapsed bool // fully hidden (show expand tab only)

	// hover eases the lift on the sidebar's non-Button clickables (section
	// headers, action rows, the collapsed expand tab) via the shared
	// hoverFade — Buttons/chips get hover from their own per-frame
	// HandleInputResult cadence.
	hover hoverFade

	// drag is the top-left grip handle. Zero value = auto mode: layout's
	// node-anchored placement passes through untouched. Once the user drags
	// the grip the card is PINNED to the grid pane and stops tracking the
	// node/camera; Open/Close reset it so a pin never survives a reopen.
	drag PopupDragHandle

	// Sections — individually collapsible
	sectionOpen map[sbSection]bool
	// secHeldPrev is the previous frame's press state as seen by the section
	// -header branch of HandleInput, so that branch fires on the press EDGE
	// only. Buttons get this for free from Button.applyPress's held counter
	// (uigrid.go); section headers are toggled directly and so must track it
	// themselves, or every frame the pointer stays down flips them again.
	secHeldPrev bool
	// pickerPressOwned: the current press began on the open note/chord
	// picker, so it owns the gesture to its release even after the picker
	// closes under it (see HandleInput). pickerHeldPrev tracks its press edge.
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
	// noteMatrix is the note/octave picker opened from the Pitch section's
	// note chip. It is owned by the sidebar rather than the DrumView portal
	// because the sidebar renders through GridTree (see Open), so a
	// portal-hosted picker would paint outside the grid pane.
	noteMatrix *NoteMatrixComponent
	// noteMatrixKeyNav is the picker's keyboard cursor — the same MenuKeyNav
	// the instrument, template and context menus use.
	noteMatrixKeyNav *MenuKeyNav
	// chordMatrix is the chord picker opened from a chord node's chip, wired
	// exactly like noteMatrix (game_node_sidebar_chord_matrix.go).
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

// Open activates the sidebar for the given node. The sidebar renders through the
// grid pane's own tree (GridTree, see registerGridTree) — NOT the global portal —
// so its node-anchored card is clipped to the grid pane and can never paint over
// the bottom drum-view panel (a separate component tree). Input flows through the
// grid inputDispatcher.
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

// fireGroupChip closes the sidebar and opens the GroupMenu for gid, anchored
// at the sidebar's panel origin.
func (sb *NodeSidebar) fireGroupChip(gid model.GroupID) {
	g := sb.game
	panel := sb.rects[sbPanel]
	sb.Close()
	g.groupMenu.OpenAt(gid, panel.Min.X+20, panel.Min.Y+20)
}

// createNewGroup turns the sidebar's current node into a brand-new
// PROVISIONAL single-member group and opens its GroupMenu — the "grpnew"
// row's onClick (task-sidebar-create brief). Mirrors handleMarquee's
// creation lifecycle (game_marquee.go): CreateGroup mints the lowest-free
// group id (default name "Group N", RulesEnabled false); NO emit happens
// here — the group stays provisional until the user presses Save in the
// menu (GroupMenu.save() owns the single emitGroupCreated undo step; closing
// without Save silently discards it via discardProvisional). The sidebar
// closes in favor of the GroupMenu, anchored near the sidebar's origin the
// same way fireGroupChip anchors a reopened chip. Model errors (e.g. the
// node vanished mid-frame) are a silent no-op, logged at Debug.
func (sb *NodeSidebar) createNewGroup() {
	g := sb.game
	node := sb.node
	if node == nil {
		return
	}
	panel := sb.rects[sbPanel]
	// Exact-set reuse, mirroring handleMarquee's release path: if a group
	// already contains EXACTLY this node, reopen it (non-provisional — it is
	// committed state) instead of minting a duplicate single-node group.
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

// addToGroup adds the sidebar's current node to group gid via the model API
// and closes the add dropdown (mirrors closing logic/groove dropdowns on
// selection). Model errors (e.g. the group vanished mid-frame) are a silent
// no-op — logged at Debug — no emit, no undo step.
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

// removeFromGroup removes the sidebar's current node from group gid via the
// model API. If the group still exists afterward, emits EventGroupChanged;
// if removing the last member auto-deleted it (model's
// RemoveNodeFromGroup), emits EventGroupDeleted instead. Model errors are a
// silent no-op — logged at Debug — no emit, no undo step.
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
	// The note-matrix picker floats OUTSIDE the sidebar card, so its panel must
	// be part of the sidebar's hit region too. Without this the input
	// dispatcher decides the press is not the sidebar's, never calls
	// HandleInput, and the click falls through to the grid — which is exactly
	// how every matrix cell shipped dead while the component's own unit tests
	// passed.
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

// nodeSidebarSections is every collapsible section, in card order — the ONE
// list the header hit-tests (press path and deferred-tap path), the hover set
// and ExpandAllSections read. A header missing from a hand-kept copy of it is
// dead: the Chord header shipped that way and could never expand the builder.
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

	// Panel rect is set below, after content sizing: a compact card sized to its
	// content (not the full pane) and anchored beside the tapped node.

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

	// Fixed header (NOT scrolled): a snug title band + an always-visible
	// coordinate readout (grid coords + circuit beat/subdivision). The title band
	// is sized to the title glyphs — not the full legacy sidebarHeaderH — so the
	// always-visible line costs only a few px of content height, keeping controls
	// reachable on short panels. A second line (live last/next fire) is reserved
	// only while playing, since when stopped the home line already IS the
	// next-from-0 position.
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
	// Close × is desktop-only (shared desktopCloseVisible gate); on mobile the
	// node card dismisses via tap-away, so no rect is published and the button is
	// never built below. delete keeps a profile flip (tests) from leaving a stale rect.
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
	// No Move section: moving a node is covered by copy/paste (and by the
	// long-press quick-action popup), so the sidebar doesn't duplicate it.
	// Section: Groups — ALWAYS shown (task-sidebar-create), not gated on
	// AllGroups() anymore: even a zero-group project needs the section to
	// host the "Create new group" row. A non-member node still discovers
	// "Add to group" the same way it always did once other groups exist.
	y = sb.layoutGroupSection(y, w, sb.node.ID)

	// Compute scroll limits via ScrollBehavior
	contentH := y + sb.scroll.VS.First - contentTop
	// Compact card: height = content (header + sections), capped to the grid pane
	// so a tall node keeps the card on-screen and scrolls the overflow.
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

	// Anchor the card beside the tapped node, CLAMPED TO THE GRID PANE (not the
	// window) so it stays entirely within the grid pane's own tree and can never
	// render over the bottom drum-view panel — a separate component tree.
	// AnchorPopupRect flips above/below to keep it inside the pane; avoidReservedRect
	// dodges the gear cluster. All child rects (and the scroll viewport) were
	// computed at origin (0,0); translate them to the anchored origin.
	x1, y1, x2, y2 := g.nodeScreenRect(sb.node)
	nodeR := image.Rect(int(x1), int(y1), int(x2), int(y2))
	gridPane := image.Rect(0, 0, g.split.GridW(g.winW), gridH)
	anchored := AnchorPopupRect(gridPane, nodeR, w, cardH, PopupBelow)
	// Avoid the WHOLE top-right button cluster (gear + file + logs): the sidebar's
	// close button sits at the card's top-right and would otherwise collide with
	// the higher-z logs button, which swallows the click. (gridHelpReservedRect
	// covers only gear+file, kept narrower for the group menu's placement spec.)
	if avoid := g.gridTopButtonsReservedRect(); !avoid.Empty() {
		anchored = avoidReservedRect(anchored, avoid, gridPane.Dx(), gridPane.Dy())
	}
	// The grip handle gets the last word: in auto mode Place returns `anchored`
	// verbatim, so default placement is unchanged; once the user has dragged,
	// it returns the pinned origin re-clamped against the CURRENT card size.
	placed := sb.drag.Place(anchored, gridPane, g.gridTopButtonsReservedRect())
	origin := placed.Min
	if origin.X != 0 || origin.Y != 0 {
		for k, r := range sb.rects {
			sb.rects[k] = r.Add(origin)
		}
		sb.scroll.VS.View = sb.scroll.VS.View.Add(origin)
	}

	// Grip rect is published in FINAL screen space — after the translate — so
	// draw and hit-test agree without either re-deriving the origin.
	sb.rects[sbGrip] = sb.drag.GripRect(sb.rects[sbPanel])

	// Note-matrix cell rects are already in SCREEN space (the picker solves
	// against the grid pane), so they are published AFTER the origin translate
	// above and before wireButtons turns them into Buttons.
	sb.layoutNoteMatrixCells()
	sb.layoutChordMatrixCells()

	// Wire button objects
	sb.wireButtons()
}

// layoutSection lays out a simple +/- section (vol, pit, dur).
// Layout: [label] ... [−] [value pill] [+] right-aligned.
// sidebarNoteChipW is the fixed width of the Pitch row's note chip button.
// Wide enough for the longest label ("A#-1") at RoleBody plus padding.
const sidebarNoteChipW = 40

func (sb *NodeSidebar) layoutSection(y, w, btnX int, sec sbSection, _ string, hasButtons bool) int {
	sb.rects[sbSectionHeader(sec)] = image.Rect(sidebarPad, y, w-sidebarPad, y+sidebarSectionH)
	y += sidebarSectionH
	if !sb.sectionOpen[sec] {
		return y
	}
	if hasButtons {
		// Right-align: [−] [pill] [+]
		// +button at the far right
		plusX := w - sidebarPad - sidebarIncBtnW
		// pill between buttons
		pillW := Profile().DensityValues().SidebarValuePillW
		pillX := plusX - sidebarGap - pillW
		// −button to the left of pill
		minusX := pillX - sidebarGap - sidebarIncBtnW

		btnY := y + (sidebarBtnH-sidebarIncBtnH)/2 // vertically center 24px buttons in 28px row
		if sec == secPit {
			// Note chip, left of the -/+ cluster. FIXED width so the row does
			// not jitter as the name changes between "C4" and "A#4"; the draw
			// site skips it entirely for percussion (no note to show).
			noteX := minusX - sidebarGap - sidebarNoteChipW
			if sb.nodeIsChord() {
				// A chord name ("C#m(add9)/E") needs the whole left gap: the
				// chip replaces the row caption.
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

// layoutChordSection lays out the chord builder: the chord chip (opens the
// chord matrix), inversion and spread steppers ([caption] … [−] [pill] [+],
// like layoutSection) and the full-width "Single note" collapse row.
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
	// The chord chip — the section's lead control: names the chord and opens
	// the chord matrix, so choosing a chord never needs a detour through the
	// (separately collapsed) Pitch section.
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

// keepUnderPointer runs a change that re-shapes the card (a type change adds
// or removes whole sections) without moving control id: the card is held at
// its current position instead of re-anchoring beside the node, and the
// content is scrolled by however far id moved. Without it, Regular → Chord
// pushed the Audible button off the bottom of the card, so the next tap of a
// "rotate" hit nothing. When the content cannot scroll far enough (a short
// card), id is still kept inside the viewport.
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

// layoutGroupSection lays out the ALWAYS-PRESENT (task-sidebar-create) Groups
// section, in order (task-remove-selector brief):
//  0. A "Create new group" row ("grpnew"), first — above membership chips —
//     so a zero-group project still gets a usable section (its only row).
//  1. Membership chips ("grpchip:<gid>", opens the GroupMenu) — one per
//     group the node currently belongs to, now full-width open-only buttons
//     (the paired "grpdel:<gid>" remove button is gone; removal moved to the
//     "Remove from group" selector below).
//  2. An "Add to group" row ("grpadd"), shown only when at least one group
//     exists that the node is NOT a member of; clicking it toggles
//     groupAddDropdownOpen, which lists those candidates
//     ("grpaddopt:<gid>").
//  3. A "Remove from group" row ("grpremove"), shown only when the node has
//     at least one membership; clicking it toggles groupRemoveDropdownOpen,
//     which lists those memberships ("grpremoveopt:<gid>"). Picking one
//     removes that membership (NodeSidebar.removeFromGroup).
//
// The section header itself ("sec-grp") is laid out unconditionally by the
// caller (layout() no longer gates this on AllGroups()) — a non-member node,
// or one in a project with zero groups, still needs the section to reach
// "Create new group" / "Add to group".
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
		// No candidate left to add to (e.g. the last one was just added) —
		// the dropdown can't be reachable, so force it closed.
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
		// No membership to remove from — the dropdown can't be reachable,
		// so force it closed (mirrors the grpadd hasCandidate branch above).
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
			// Every node-sidebar control uses the shared menu keycap style
			// (DropdownStyle) so the node pop-up's buttons + dropdown lists read
			// identically to the instrument / context / subdivision menus. The
			// node's own color is layered on via tintBtnAccent (keycap face tint) and, for
			// dropdown lists, drawMenuRow's accent stripe.
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
		// Smooth styled label (embedded Inter) — never the legacy bitmap
		// upscale path, which pixelates at any scale ≠ 1.
		sb.btns[id].SetTextRole(RoleBody)
		// +/− steppers use icon-based drawing (Button.Draw path) instead of the
		// legacy hand-drawn glyph so they get cushion + glow + rounded chrome.
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
	// Value pills open the shared value wheel (no keycap is drawn for them —
	// drawValuePill paints the readout; the Button only owns the tap).
	for _, f := range []sbField{fieldVol, fieldDur, fieldLogicN, fieldLogicP, fieldGroovePct} {
		mk(sbValue(f), "", func() { g.openNodeValueWheel(f) })
	}
	// Note-matrix cells, when the picker is open.
	sb.wireNoteMatrixCells(mk)
	sb.wireChordMatrixCells(mk)
	// Note chip: opens the note/octave matrix picker. Registered before the
	// -/+ buttons so it shares their keycap styling and press animation.
	// Only meaningful for pitched instruments; mk() drops it when the rect is
	// absent, and the pitch draw block omits the rect for percussion.
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
	// Chord builder (chord nodes only; mk drops the buttons when the
	// section is not laid out).
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
	// Type cycle: Regular → Chord → Silent → Mute (Chord skipped on an
	// unpitched row).
	mk(sbAudible, i18n.T(i18n.KeyNodeSecAudible), func() {
		sb.keepUnderPointer(sbAudible, func() { g.cycleNodeType(node.ID) })
	})
	// "Create new group" row — always wired (the section is always
	// present), first, above the membership chips.
	mk(sbGroupNew, i18n.T(i18n.KeyGroupCreateNew), func() {
		sb.createNewGroup()
	})
	// Group chips — one per group this node currently belongs to, now pure
	// open-only full-width buttons (the paired "grpdel:<gid>" remove button
	// is gone; removal moved to the "Remove from group" selector below).
	// Below them, an "Add to group" row + dropdown for groups the node is
	// NOT a member of, then a "Remove from group" row + dropdown for groups
	// the node IS a member of (see layoutGroupSection for the matching
	// geometry).
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
	// "Add to group" row + dropdown of non-member groups. mk()'s own rect
	// check (layoutGroupSection only creates "grpadd" when a candidate
	// exists) handles hiding this row — no extra gating needed here.
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
	// "Remove from group" row + dropdown of MEMBER groups. mk()'s own rect
	// check (layoutGroupSection only creates "grpremove" when the node has
	// >=1 membership) handles hiding this row — no extra gating needed here.
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
	// Prune stale group buttons: dynamic-count section, unlike the fixed
	// vol/pit/dur ids above.
	for id := range sb.btns {
		gid := model.GroupID(id.arg)
		switch id.el {
		case elGroupChip:
			if !grpIDSet[gid] {
				delete(sb.btns, id)
			}
		case elGroupAddOpt:
			// Dropdown option buttons list NON-member groups (grpIDSet holds
			// memberships, so it must not gate them). Prune when the dropdown
			// is closed or the group itself vanished, so rect-less orphans
			// don't accumulate in sb.btns.
			if _, exists := sb.game.graph.Group(gid); !sb.groupAddDropdownOpen || !exists {
				delete(sb.btns, id)
			}
		case elGroupRemoveOpt:
			// Dropdown option buttons list MEMBER groups, so grpIDSet IS the
			// correct gate here (the mirror image of the add options above).
			// Prune when the dropdown is closed or the node is no longer a member.
			if !sb.groupRemoveDropdownOpen || !grpIDSet[gid] {
				delete(sb.btns, id)
			}
		}
	}
	// Close button (desktop only — no rect published on mobile, so skip + drop
	// any stale button from a prior desktop layout).
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
	// The note-matrix picker floats OUTSIDE the sidebar card, so the reported
	// interactive area must cover it too — the input dispatcher routes on this
	// rect, so a press on the picker was otherwise never delivered here and
	// every cell was dead. Union, not replace: the card stays interactive.
	if p := sb.openPicker(); p != nil {
		if m := p.InputBounds(); !m.Empty() {
			return panel.Union(m)
		}
	}
	return panel
}

// ZIndex returns the z-order (above splitter and drumview).
func (sb *NodeSidebar) ZIndex() int { return 200 }

// PassesGesturesThrough opts the sidebar out of the dispatcher's default
// whole-gesture ownership: its touch taps resolve on the release-time grid tap
// (handleTapInGrid's sidebar branch ends the deferred-tap / scroll gesture the
// dispatcher started), so the tap must still be routed there.
func (sb *NodeSidebar) PassesGesturesThrough() bool { return true }

// HandleInput processes mouse/touch input within the sidebar.
// Follows the deferred-tap + scroll-commitment pattern from the instrument
// menu to prevent accidental section toggles during touch scrolling.
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

	// A Shift+press is the start of a connect (link-drag) or marquee gesture
	// on the grid — the sidebar yields: close and report the press as
	// unhandled so the SAME press reaches handleEditor's shift block
	// (which bypasses the closed-guard for shift). Without this, shift+drag
	// FROM the selected node is dead whenever that node renders under this
	// full-height left panel — the exact flow of "click a node, then
	// shift+drag it onto another node to connect them".
	if pressed && (isKeyPressed(ebiten.KeyShiftLeft) || isKeyPressed(ebiten.KeyShiftRight)) {
		sb.Close()
		return InputIgnored
	}

	sb.layout()

	// A press that began on the open picker owns its gesture to the release.
	// Re-tapping the picked cell confirms and CLOSES the picker on the press
	// frame; the still-held pointer then sits over the card beneath, and the
	// next held frame / the release activated whatever control lay under the
	// cell — on a phone the Audible selector, which turned a just-confirmed
	// chord back into a single note. (ConsumeOnPress cannot guard this: the
	// drum view's tree rewrites the global suppress flag every frame.)
	// HandleInput is not called on every frame (the dispatcher routes only
	// some), so a gap since the last call resets the latch rather than
	// trusting a stale held state.
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

	// The note-matrix picker is MODAL over its own panel: it gets first refusal
	// on every press, and declines only for presses on its selectable cells so
	// those reach the shared Button path below (step 8) with the hover and
	// press behaviour every other node-menu control has.
	//
	// Both halves are load-bearing. Absorbing everything here left the cells
	// dead; absorbing nothing let a press on a DIMMED cell fire the sidebar
	// control underneath it.
	if r := sb.handleNoteMatrixInput(x, y, pressed); r != InputIgnored {
		return r
	}
	if r := sb.handleChordMatrixInput(x, y, pressed); r != InputIgnored {
		return r
	}

	// Track the press edge for the section-header branch (step 9) here, at the
	// single point every call passes through — NOT at step 9 itself. Most
	// releases never reach step 9 (the scroll/deferred-tap steps below consume
	// them and return early), so updating it there would leave the flag stuck
	// true after a captured gesture and swallow the next genuine press edge.
	prevSecHeld := sb.secHeldPrev
	sb.secHeldPrev = pressed

	// Grip drag — highest precedence inside the card. Returning InputCaptured
	// makes the dispatcher route every later frame straight here without
	// re-hit-testing (input_dispatch.go), so the drag survives the cursor
	// leaving the card.
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

	// 4. TouchActive but not yet committed (in dead zone) →
	//    process moves; on release fire deferred tap if no scroll committed
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

	// Steps 5 and 6 START a sidebar scroll gesture. They must not run for a
	// press over the OPEN note-matrix picker: the picker floats across the
	// sidebar card, so a cell that overlaps the sidebar's scrollbar (or merely
	// its scroll viewport) had its press swallowed as a thumb-drag or a
	// deferred tap and never reached the cell Button. An active surface owns
	// its own area; the parent must not also act on it.
	//
	// A gesture already IN PROGRESS (steps 2-4) still finishes normally — this
	// only declines to start a new one under the picker.
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

	// 6. Press in scroll viewport → begin deferred tap + touch scroll simultaneously
	//    Exclude scrollbar track area so track clicks reach step 11.
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

	// 9. Section header clicks (only if not suppressed by touch)
	//
	// Fires on the press EDGE only, and never while clicks are globally
	// suppressed. Both guards are load-bearing:
	//
	//   - Edge: this branch toggles sb.sectionOpen directly rather than going
	//     through a Button, so without an edge check every frame the pointer
	//     stayed down flipped the section again — leaving it open or closed on
	//     the parity of how long the user held.
	//   - suppressClicksUntilRelease: Button.ConsumeOnPress raises this so the
	//     same held press cannot activate anything else until release, and
	//     DeferredTap.Begin declines while it is set ("uniform guard behavior
	//     across all menus"). That decline is precisely what lets a press reach
	//     this branch at all — a press inside the scroll viewport is otherwise
	//     captured by the deferred-tap path at step 6 and toggles once on
	//     release. So without this guard the branch fired ONLY in the case
	//     where it was supposed to stay silent, and could double-toggle one
	//     gesture: once here, then again via the deferred tap once the tree
	//     cleared the flag mid-hold.
	//
	// prevSecHeld is captured at the top of HandleInput (see there) so the edge
	// stays accurate even for the gestures that never reach this branch.
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

// fireTapAt replays a deferred tap at stored coordinates — hit-tests buttons
// and section headers in z-order, fires the first match. Called by
// deferredTap.End() on touch release when scroll was NOT committed.
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

// HandleWheel scrolls content. steps comes from wheelScrollSteps() where
// positive = wheel up. ScrollBehavior.HandleWheel has the same convention,
// so we delegate directly.
func (sb *NodeSidebar) HandleWheel(x, y int, steps int) InputResult {
	if !sb.open || sb.node == nil {
		return InputIgnored
	}
	sb.layout() // Ensure scroll limits are up-to-date
	// The note-matrix picker floats over the sidebar, so a wheel event with the
	// pointer over IT must scroll its rows, not the card underneath. The
	// position was previously discarded outright (_, _), which is why the wheel
	// scrolled the sidebar behind an open picker.
	if p := sb.openPicker(); p != nil {
		if r := p.HandleWheel(x, y, steps); r != InputIgnored {
			return r
		}
	}
	sb.scroll.HandleWheel(steps * sidebarBtnH)
	return InputConsumed
}

// UpdateScroll applies per-frame momentum decay. Call from Game.Update()
// every frame (not from HandleInput, which only runs when the cursor is
// over the sidebar).
func (sb *NodeSidebar) UpdateScroll() {
	// Keyboard cursor for the note-matrix picker. Pumped here (not from
	// HandleInput) so the arrows work whether or not the pointer happens to be
	// over the picker — HandleInput only runs when it is.
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
	// The note-matrix cells float above every other control, so they take the
	// highest z. buttonOrder is what fireTapAt walks: a Button missing from
	// this list is DEAD however correctly it is registered and drawn.
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
	// "pitnote" (the note chip that opens the note matrix) is listed BEFORE the
	// steppers: it sits immediately left of "pit-" and must win any overlap.
	// A button absent from this list is DEAD — buttonOrder is what fireTapAt
	// walks, so registering a Button in wireButtons is not enough on its own.
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

// contentTopY is the single source of truth for the Y where scrollable content
// begins — directly below the fixed header band. It derives from the laid-out
// header rect so the input-press, inViewport, and Draw paths stay in lockstep
// with layout(), which grows the header when the coordinate readout adds a second
// (playing) line. It falls back to the legacy fixed header height only before
// layout() has populated the header rect.
func (sb *NodeSidebar) contentTopY() int {
	if hr, ok := sb.rects[sbHeader]; ok && !hr.Empty() {
		return hr.Max.Y + sidebarGap
	}
	return sidebarPad + sidebarHeaderH + sidebarGap
}

// gripHovered reports whether the desktop cursor is over the drag grip. Mobile
// has no hover state, so it always reports false.
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

// inViewport reports whether a rectangle is at least partially visible within
// the scrollable content area (below the fixed header and above the panel bottom).
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

	// Resolve the hovered non-Button clickable (desktop): section headers +
	// full-width action rows get the shared eased hover lift. Buttons, chips
	// and dropdown options carry their own hovered state via the per-frame
	// dispatcher cadence.
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

	// Draw sections. Section headers now self-compute their text baseline via
	// StyledTextHeight; the legacy secTextOffY arg is retained for signature
	// compatibility but ignored by drawSectionHeader.
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

	// All scrolled content draws through a viewport SubImage (below the
	// fixed header, above the panel bottom) so a half-scrolled row can never
	// paint over the title band. The shadowed dst keeps the section-draw
	// code identical; coordinates stay absolute.
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
				// Note chip BUTTON in the row's left gap: the musical note this
				// node sounds, and the opener for the note/octave matrix. It is
				// absent for percussion (nodeNoteLabel returns "" and the
				// button was never registered). Derived live, so it tracks
				// pitch / synth / sampler edits.
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

	// Groups section — ALWAYS shown (task-sidebar-create), not gated on
	// AllGroups() anymore. First row is "Create new group" (grpnew), then
	// full-width open-only membership chips (reopening the GroupMenu on
	// click, fireGroupChip — the paired "grpdel" remove button is gone,
	// task-remove-selector), then an "Add to group" row + dropdown for
	// groups this node is NOT a member of, then a "Remove from group" row +
	// dropdown for groups this node IS a member of.
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
				// Membership chips are navigational (they reopen the
				// GroupMenu), so they render as flat menu rows: the group's
				// identity lives in the leading swatch + hover tint +
				// drill-in chevron — never a color-filled face.
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
							// Group identity = its color everywhere (badges,
							// chips, remove options). Add options list
							// specific groups too, so they wear the same
							// per-group tint, not the node's accent.
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

	// Scrolled content done — back to the unclipped surface for the fixed
	// chrome (header band, separator, close, scrollbar).
	dst = dstFull

	// Fixed header, drawn after the clipped content so it always sits on top.
	sb.drawHeader(dst)
	sepY := sb.rects[sbHeader].Max.Y
	drawRect(dst, image.Rect(sidebarPad, sepY, panel.Max.X-sidebarPad, sepY+1), WithAlpha(genColorSidebarSectionBg, genAlphaSidebarSection), true)

	// Close button — visible neutral chip with the standard hairline border;
	// the muted × glyph matches every other panel's close affordance.
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

	// The note-matrix picker floats above every sidebar control, so it paints
	// last. It is clipped to the grid pane like the rest of the sidebar.
	sb.drawNoteMatrix(dst)
	sb.drawChordMatrix(dst)
}

// drawHeader renders the fixed header with instrument name and color swatch.
// nodeAccent returns the accent color for the node sidebar: the owning row's
// instrument color so the sidebar's header underline, active section stripes,
// and accent text all read as owned by the instrument the node belongs to.
// Falls back to the azure chrome accent when the node has no resolvable row.
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

	// Instrument swatch + name via the shared, unified menu-title treatment so
	// the node pop-up title matches the row context menu 100% (font/size/icon).
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

	// Drag grip: dim by default, brighter on hover, accent while dragging —
	// the standard three-state affordance tint.
	if gr, ok := sb.rects[sbGrip]; ok && !gr.Empty() {
		DrawIcon(dst, IconGrip, gr, popupGripTint(sb.drag.Dragging(), sb.gripHovered()))
	}

	// Coordinate readout: grid coords + circuit beat/subdivision (always). While
	// playing, a second line shows the node's live last/next fire; when stopped
	// the home line already conveys the next-from-0 position, so it is omitted to
	// keep the fixed header short on small panels.
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

// drawSectionHeader draws a collapsible section header using the shared menu
// chrome (icon chevron + item background + active stripe) and an optional
// collapsed-state summary badge.
func (sb *NodeSidebar) drawSectionHeader(dst *ebiten.Image, rectID sbID, label string, sectionID sbSection, _ color.Color, _ int) {
	r, ok := sb.rects[rectID]
	if !ok || r.Empty() || !sb.inViewport(r) {
		return
	}
	expanded := sb.sectionOpen[sectionID]
	// No expand highlight: the chevron direction (down=open, right=closed) already
	// conveys section state, and the node-color fill/stripe + accent text read as a
	// distracting "selected" bar. Headers stay neutral; node color lives on the
	// buttons, header swatch, and dropdown stripes.
	chevR := image.Rect(r.Min.X+sidebarPad, r.Min.Y, r.Min.X+sidebarPad+IconSizeMD, r.Max.Y)
	var chevCol color.Color = colTextSecondary
	var labelCol color.Color = colTextPrimary
	drawMenuChevron(dst, chevR, expanded, chevCol)

	th := StyledTextHeight(RoleSectionHeader)
	labelX := chevR.Max.X + sidebarGap
	labelY := r.Min.Y + (r.Dy()-th)/2
	DrawTextStyled(dst, label, labelX, labelY, RoleSectionHeader, labelCol)

	sb.drawSectionBadge(dst, r, sectionID)

	// Expandable header = clickable: the shared eased hover lift, same as
	// every button (headers stay neutral otherwise — chevron carries state).
	drawButtonHoverGlow(dst, r, RadiusSM, sb.hover.Lift(rectID.String()))
}

// drawSectionBadge renders the collapsed-section summary badge (right-aligned,
// square sharp corners) showing the section's current value (default or not).
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

// sectionBadgeText returns a summary string of a collapsed section's current
// value. Every category surfaces its value — including when it is at the
// default — using the same formatting as updated values, so a glance at the
// collapsed sidebar shows each category's state (e.g. "100%", "+0", "None").
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

// drawBtn draws a single button if it exists and is within the viewport.
// Faces stay neutral (plain DropdownStyle keycaps, like every other menu):
// the node's identity color lives in the header band, title swatch, and
// dropdown accent stripes — never on button fills (2026-07-16 redesign; the
// old tintBtnAccent 40% face tint made the whole panel read as color-filled
// blocks).
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

// sidebarHoverIDs enumerates the sidebar's hover-lift targets that are NOT
// Buttons: the collapsible section headers plus the full-width action rows.
// Resolved against sb.rects each frame in Draw; first hit wins.
var sidebarHoverIDs = []sbID{
	sbSectionHeader(secVol), sbSectionHeader(secPit), sbSectionHeader(secChord), sbSectionHeader(secDur),
	sbSectionHeader(secLogic), sbSectionHeader(secGroove), sbSectionHeader(secAud), sbSectionHeader(secGrp),
	sbAudible, sbChordSingle, sbGroupNew, sbGroupAdd, sbGroupRemove,
}

// drawActionRow renders one full-width sidebar action row: the neutral
// DropdownStyle keycap + a left-aligned styled label + the shared hover
// lift. One helper so every action row (Audible, Move, Create/Add/Remove
// group) shares identical chrome.
// drawChordSection draws the chord builder (layoutChordSection's rows).
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

// drawDropdownItem renders one Logic/Groove/group-dropdown LIST row through
// the shared drawMenuRow primitive (same as every other menu list):
// background accent + keycap chrome + label. accent is caller-supplied so
// each dropdown can carry its own accent language: the node's own color
// (nodeAccent — SSOT DrumRow.Color) for Logic/Groove/"Add to group" so the
// selected row shows a node-color stripe and a hovered row a node-color
// tint; the group's own color (groupColor(gid)) for "Remove from group"
// option rows, matching the membership chips' tinting. The clip guard keeps
// off-viewport rows from drawing.
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

// drawValuePill draws a centered value READOUT between the inc/dec stepper
// keys. In the mechanical-keycap language the steppers are raised keys, so the
// readout reads as a RECESSED display window sunk into the panel: a fill
// darker than the panel + an inner top shadow (the tactile opposite of a
// keycap's top specular), then the border and centered colTextPrimary text.
func (sb *NodeSidebar) drawValuePill(dst *ebiten.Image, key sbID, value string) {
	r, ok := sb.rects[key]
	if !ok || r.Empty() || !sb.inViewport(r) {
		return
	}
	drawRecessedPill(dst, r, value)
}

// drawRecessedPill paints a value READOUT as a recessed display well: a fill
// darker than the panel + an inner top shadow (the tactile opposite of a
// keycap's top specular), the border, then centered colTextPrimary text. Pure
// (no sidebar state) so it is unit-testable and reusable for any readout.
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
