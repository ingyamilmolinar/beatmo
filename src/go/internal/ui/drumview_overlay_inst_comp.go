package ui

import (
	"image"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"
)

// InstrumentOption represents an instrument available for selection.
type InstrumentOption struct {
	ID       string
	Label    string
	Category string
}

// InstrumentMenuProps contains the external state passed to the instrument menu component.
type InstrumentMenuProps struct {
	// AnchorRect is the label button that opens the menu (used for positioning).
	AnchorRect image.Rectangle
	// VertBounds is the vertical bounds for constraining the menu.
	VertBounds image.Rectangle
	// RowIndex is the index of the row being edited.
	RowIndex int
	// CurrentInstrument is the currently selected instrument ID.
	CurrentInstrument string
	// Categories is the list of available categories.
	Categories []string
	// Instruments is the list of all available instruments.
	Instruments []InstrumentOption
	// RowHeight is the height of each row in the menu.
	RowHeight int
	// LabelWidth is the width of the label column.
	LabelWidth int
	// ControlsWidth is the width of the controls column.
	ControlsWidth int
	// ForceCategories starts the menu in categories mode when true and categories exist.
	// Default is false (start in instruments mode).
	ForceCategories bool

	// OnSelect is called when an instrument is selected.
	OnSelect func(instID string)
	// OnClose is called when the menu should close.
	OnClose func()
	// OnRebuild is called after the menu is rebuilt (for legacy state sync).
	OnRebuild func()
}

// InstMenuMode describes the view mode of the instrument menu.
type InstMenuMode string

const (
	InstMenuModeCategories  InstMenuMode = "categories"
	InstMenuModeInstruments InstMenuMode = "instruments"
)

const (
	instMenuMaxVisibleRowsComp = 10
	instMenuScrollBarWidthComp = 10
)

// InstrumentMenuState contains the internal state for the instrument menu component.
type InstrumentMenuState struct {
	open             bool
	hold             bool // Capture flag after menu closes
	mode             InstMenuMode
	activeCat        string
	userScrolled     bool
	lastAdded        string
	cameFromCats     bool
	searchText       string
	filteredInsts    []string
	categoryByID     map[string]string
	displayLabelByID map[string]string
	searchHighlights map[string][]int // Per-ID highlight positions from fuzzy search.
}

// InstrumentMenuComponent is a self-contained dropdown menu for selecting instruments.
type InstrumentMenuComponent struct {
	overlayBase
	props InstrumentMenuProps
	state InstrumentMenuState

	scroll       *ScrollBehavior
	categoryBtns []*Button
	instBtns     []*Button
	searchBox    *TextInput
	backBtn      *Button
	closeBtn     *Button
	fullRect     image.Rectangle
	searchRect   image.Rectangle

	// Deferred tap: position stored on touch begin, fired on touch end if no scroll committed.
	deferredTap DeferredTap
}

// NewInstrumentMenuComponent creates a new instrument menu component.
func NewInstrumentMenuComponent() *InstrumentMenuComponent {
	return &InstrumentMenuComponent{
		overlayBase: newOverlayBase(),
		scroll:      NewScrollBehavior(DropdownScrollbarStyle, 24),
	}
}

// SetProps updates the external props.
func (m *InstrumentMenuComponent) SetProps(p InstrumentMenuProps) {
	// Rebuild category map and display labels if instruments changed
	needsRebuild := len(p.Instruments) != len(m.props.Instruments)
	if !needsRebuild {
		for i, inst := range p.Instruments {
			if inst.ID != m.props.Instruments[i].ID {
				needsRebuild = true
				break
			}
		}
	}
	m.props = p
	if needsRebuild {
		m.rebuildMaps()
	}
}

// Props returns the current props.
func (m *InstrumentMenuComponent) Props() InstrumentMenuProps { return m.props }

// rebuildMaps rebuilds the internal lookup maps from props.
func (m *InstrumentMenuComponent) rebuildMaps() {
	m.state.categoryByID = make(map[string]string)
	m.state.displayLabelByID = make(map[string]string)
	for _, inst := range m.props.Instruments {
		m.state.categoryByID[inst.ID] = inst.Category
		m.state.displayLabelByID[inst.ID] = inst.Label
	}
}

// ensureScroll lazily initializes the scroll behavior.
func (m *InstrumentMenuComponent) ensureScroll() {
	if m.scroll == nil {
		m.scroll = NewScrollBehavior(DropdownScrollbarStyle, 24)
	}
}

// Open opens the menu.
func (m *InstrumentMenuComponent) Open() {
	m.ensureScroll()
	m.state.open = true
	m.state.hold = false
	m.deferredTap.Cancel()
	m.state.userScrolled = false
	m.state.searchText = ""
	m.state.lastAdded = ""
	m.state.cameFromCats = false

	// Default to instruments mode unless ForceCategories is set and categories exist
	if m.props.ForceCategories && len(m.props.Categories) > 0 {
		m.state.mode = InstMenuModeCategories
	} else {
		m.state.mode = InstMenuModeInstruments
	}

	// Default active category to current instrument's category
	m.state.activeCat = ""
	if m.props.CurrentInstrument != "" {
		if cat, ok := m.state.categoryByID[m.props.CurrentInstrument]; ok {
			m.state.activeCat = cat
		}
	}

	m.rebuildMaps()
	m.rebuildMenu()
}

// Close closes the menu.
func (m *InstrumentMenuComponent) Close() {
	m.state.open = false
	m.state.hold = true // Set hold to capture until mouse release
	m.deferredTap.Cancel()
	m.ensureScroll()
	m.scroll.HandleDragEnd()
	m.scroll.ResetTouch()
	if m.props.OnClose != nil {
		m.props.OnClose()
	}
}

// IsOpen returns whether the menu is currently open.
func (m *InstrumentMenuComponent) IsOpen() bool {
	return m.state.open
}

// SetLastAdded sets the instrument ID to auto-bias to when opening.
func (m *InstrumentMenuComponent) SetLastAdded(id string) {
	m.state.lastAdded = id
}

// InstBtns returns the current instrument buttons (for test access and legacy sync).
func (m *InstrumentMenuComponent) InstBtns() []*Button {
	return m.instBtns
}

// VisibleInstIDs returns the instrument IDs for the currently visible instrument buttons.
func (m *InstrumentMenuComponent) VisibleInstIDs() []string {
	if m == nil {
		return nil
	}
	first, _, _ := m.ScrollState()
	var ids []string
	for i := range m.instBtns {
		idx := first + i
		if idx < len(m.state.filteredInsts) {
			ids = append(ids, m.state.filteredInsts[idx])
		}
	}
	return ids
}

// CategoryBtns returns the current category buttons.
func (m *InstrumentMenuComponent) CategoryBtns() []*Button {
	return m.categoryBtns
}

// BackBtn returns the back button (nil if not in instruments mode).
func (m *InstrumentMenuComponent) BackBtn() *Button {
	return m.backBtn
}

// ScrollView returns the scroll view rectangle (for legacy state sync).
func (m *InstrumentMenuComponent) ScrollView() image.Rectangle {
	m.ensureScroll()
	return m.scroll.VS.View
}

// ScrollState returns (First, Visible, Total) for legacy state sync.
func (m *InstrumentMenuComponent) ScrollState() (first, visible, total int) {
	m.ensureScroll()
	return m.scroll.VS.First, m.scroll.VS.Visible, m.scroll.VS.Total
}

// rebuildMenu rebuilds the menu buttons and layout.
func (m *InstrumentMenuComponent) rebuildMenu() {
	m.ensureScroll()
	m.categoryBtns = m.categoryBtns[:0]
	m.instBtns = m.instBtns[:0]

	if m.props.AnchorRect.Empty() {
		m.scroll.VS.View = image.Rectangle{}
		return
	}

	base := m.props.AnchorRect
	vertBounds := m.props.VertBounds
	if vertBounds.Empty() {
		vertBounds = base
	}

	rowH := m.props.RowHeight
	if rowH < 1 {
		rowH = 24
	}
	m.scroll.ItemHeight = rowH

	// On mobile, use full width and always open upward (bottom sheet).
	if Profile().UseBottomSheet {
		base = image.Rect(vertBounds.Min.X, base.Min.Y, vertBounds.Max.X, base.Max.Y)
	}

	// Widen popup to fit long labels
	minMenuW := m.props.LabelWidth + m.props.ControlsWidth/2
	if minMenuW < 260 {
		minMenuW = 260
	}
	if Profile().UseBottomSheet {
		// Use full width on mobile.
		minMenuW = vertBounds.Dx()
	}
	if minMenuW > vertBounds.Dx() {
		minMenuW = vertBounds.Dx()
	}
	if base.Dx() < minMenuW {
		base = image.Rect(base.Min.X, base.Min.Y, base.Min.X+minMenuW, base.Max.Y)
	}

	// Decide direction based on available space
	spaceDown := vertBounds.Max.Y - base.Max.Y
	spaceUp := base.Min.Y - vertBounds.Min.Y
	openUp := spaceDown < spaceUp
	if Profile().UseBottomSheet {
		openUp = true // always bottom sheet on mobile
	}

	if m.state.mode == InstMenuModeCategories {
		m.buildCategoriesMode(base, vertBounds, rowH, openUp)
	} else {
		m.buildInstrumentsMode(base, vertBounds, rowH, openUp)
	}
	// Notify for legacy state sync
	if m.props.OnRebuild != nil {
		m.props.OnRebuild()
	}
}

// buildCategoriesMode builds the menu in categories mode.
func (m *InstrumentMenuComponent) buildCategoriesMode(base, vertBounds image.Rectangle, rowH int, openUp bool) {
	catCount := len(m.props.Categories)
	vis := instMenuMaxVisibleRowsComp
	if Profile().UseBottomSheet {
		// On mobile, show more rows to fill the screen.
		mobileVis := (vertBounds.Dy() - rowH*2) / rowH
		if mobileVis > vis {
			vis = mobileVis
		}
	}
	if vis > catCount {
		vis = catCount
	}
	if vis < 1 {
		vis = 1
	}

	maxVisHost := vertBounds.Dy() / rowH
	if maxVisHost < 1 {
		maxVisHost = 1
	}
	if vis > maxVisHost {
		vis = maxVisHost
	}

	totalH := vis * rowH
	startY := base.Max.Y
	if openUp {
		startY = base.Min.Y - totalH
	}
	if startY < vertBounds.Min.Y {
		startY = vertBounds.Min.Y
	}
	if startY+totalH > vertBounds.Max.Y {
		startY = vertBounds.Max.Y - totalH
	}

	m.scroll.VS.Total = catCount
	m.scroll.VS.Visible = vis
	m.scroll.VS.View = image.Rect(base.Min.X, startY, base.Max.X, startY+totalH)
	m.fullRect = m.scroll.VS.View

	// Bias to active category
	if !m.state.userScrolled && m.state.activeCat != "" {
		if idx := slices.Index(m.props.Categories, m.state.activeCat); idx >= 0 {
			first := idx - vis + 1
			if first < 0 {
				first = 0
			}
			m.scroll.VS.First = first
		}
	}
	m.scroll.VS.Clamp()

	// Build category buttons
	start := m.scroll.VS.First
	for i := 0; i < vis && start+i < len(m.props.Categories); i++ {
		cat := m.props.Categories[start+i]
		r := image.Rect(base.Min.X, startY+i*rowH, base.Max.X, startY+(i+1)*rowH)
		btnCat := cat
		btn := NewButton(btnCat, DropdownStyle, func() {
			m.state.activeCat = btnCat
			m.state.mode = InstMenuModeInstruments
			m.scroll.VS.First = 0
			m.state.cameFromCats = true
			m.state.userScrolled = false
			m.rebuildMenu()
		})
		if btnCat == m.state.activeCat {
			btn.Style = PopupButtonStyle
		}
		btn.SetRect(insetRect(r, buttonPad))
		m.categoryBtns = append(m.categoryBtns, btn)
	}

	m.buildCloseBtn()
	m.SetBounds(m.fullRect)
}

// buildInstrumentsMode builds the menu in instruments mode.
func (m *InstrumentMenuComponent) buildInstrumentsMode(base, vertBounds image.Rectangle, rowH int, openUp bool) {
	// Filter instruments: category filter first, then fuzzy search.
	var catItems []MenuSearchItem
	for _, inst := range m.props.Instruments {
		if m.state.activeCat != "" && m.state.categoryByID[inst.ID] != m.state.activeCat {
			continue
		}
		label := m.state.displayLabelByID[inst.ID]
		if label == "" {
			label = inst.ID
		}
		catItems = append(catItems, MenuSearchItem{Key: inst.ID, Label: label})
	}
	var searcher MenuSearcher
	searchResults := searcher.Search(m.state.searchText, catItems)
	m.state.filteredInsts = m.state.filteredInsts[:0]
	m.state.searchHighlights = make(map[string][]int, len(searchResults))
	for _, r := range searchResults {
		m.state.filteredInsts = append(m.state.filteredInsts, r.Key)
		if len(r.Highlights) > 0 {
			m.state.searchHighlights[r.Key] = r.Highlights
		}
	}

	showBack := len(m.props.Categories) > 0
	extraRows := 1 // search row
	if showBack {
		extraRows++ // back row
	}

	vis := instMenuMaxVisibleRowsComp
	if Profile().UseBottomSheet {
		// On mobile, fill the screen with instrument rows.
		mobileVis := (vertBounds.Dy() - rowH*2) / rowH
		if mobileVis > vis {
			vis = mobileVis
		}
	}
	maxVisHost := vertBounds.Dy()/rowH - extraRows
	if maxVisHost < 1 {
		maxVisHost = 1
	}
	wantMinVis := 2
	if vis > maxVisHost {
		vis = maxVisHost
	}
	if vis > len(m.state.filteredInsts) {
		vis = len(m.state.filteredInsts)
	}
	if vis < wantMinVis && maxVisHost >= wantMinVis && len(m.state.filteredInsts) >= wantMinVis {
		vis = wantMinVis
	}
	maxVis := instMenuMaxVisibleRowsComp
	if Profile().UseBottomSheet {
		maxVis = maxVisHost
	}
	if vis > maxVis {
		vis = maxVis
	}
	if vis < 1 {
		vis = 1
	}

	totalH := (vis + extraRows) * rowH
	startY := base.Max.Y
	if openUp {
		startY = base.Min.Y - totalH
	}
	if startY < vertBounds.Min.Y {
		startY = vertBounds.Min.Y
	}
	if startY+totalH > vertBounds.Max.Y {
		startY = vertBounds.Max.Y - totalH
	}

	// Search box position
	searchY := startY
	if showBack {
		searchY += rowH
	}
	m.searchRect = image.Rect(base.Min.X, searchY, base.Max.X, searchY+rowH)

	// Initialize search box if needed
	if m.searchBox == nil {
		m.searchBox = NewTextInput(image.Rectangle{}, BPMBoxStyle)
		m.searchBox.MaxLen = 40
	}
	m.searchBox.SetText(m.state.searchText)
	m.searchBox.Rect = insetRect(m.searchRect, buttonPad)

	listStartY := searchY + rowH

	m.scroll.VS.Total = len(m.state.filteredInsts)
	m.scroll.VS.Visible = vis

	if len(m.state.filteredInsts) == 0 {
		emptyView := image.Rect(base.Min.X, listStartY, base.Max.X, listStartY+rowH)
		m.scroll.VS.View = emptyView
		m.fullRect = image.Rect(base.Min.X, startY, base.Max.X, startY+rowH*2)
		placeholder := NewButton("No matches", DisabledButtonStyle, nil)
		placeholder.SetRect(insetRect(emptyView, buttonPad))
		m.instBtns = append(m.instBtns, placeholder)
		m.SetBounds(m.fullRect)
		return
	}

	m.scroll.VS.View = image.Rect(base.Min.X, listStartY, base.Max.X, listStartY+vis*rowH)
	m.fullRect = image.Rect(base.Min.X, startY, base.Max.X, listStartY+vis*rowH)

	// Clamp and shift if needed
	if !vertBounds.Empty() {
		shiftY := 0
		if m.fullRect.Min.Y < vertBounds.Min.Y {
			shiftY = vertBounds.Min.Y - m.fullRect.Min.Y
		} else if m.fullRect.Max.Y > vertBounds.Max.Y {
			shiftY = vertBounds.Max.Y - m.fullRect.Max.Y
		}
		if shiftY != 0 {
			m.fullRect = m.fullRect.Add(image.Pt(0, shiftY))
			m.scroll.VS.View = m.scroll.VS.View.Add(image.Pt(0, shiftY))
			m.searchRect = m.searchRect.Add(image.Pt(0, shiftY))
		}
	}

	// Bias to last added instrument
	if !m.state.userScrolled && m.state.lastAdded != "" {
		if idx := slices.Index(m.state.filteredInsts, m.state.lastAdded); idx >= 0 {
			first := idx - vis + 1
			if first < 0 {
				first = 0
			}
			m.scroll.VS.First = first
		}
	}

	// Bias to current instrument
	if !m.state.userScrolled && m.state.lastAdded == "" && m.props.CurrentInstrument != "" {
		if idx := slices.Index(m.state.filteredInsts, m.props.CurrentInstrument); idx >= 0 {
			first := idx - vis + 1
			if first < 0 {
				first = 0
			}
			m.scroll.VS.First = first
		}
	}

	m.state.lastAdded = ""
	m.scroll.VS.Clamp()

	hasScroll := m.scroll.HasScroll()
	buttonMaxX := base.Max.X
	if hasScroll {
		buttonMaxX -= instMenuScrollBarWidthComp
		if buttonMaxX <= base.Min.X {
			buttonMaxX = base.Min.X + 1
		}
	}

	// Back button
	if showBack {
		backRect := image.Rect(base.Min.X, startY, base.Max.X, startY+rowH)
		m.backBtn = NewButton("Back", DropdownStyle, func() {
			m.state.mode = InstMenuModeCategories
			m.scroll.VS.First = 0
			m.state.cameFromCats = false
			m.state.userScrolled = false
			m.rebuildMenu()
		})
		m.backBtn.SetRect(insetRect(backRect, buttonPad))
	} else {
		m.backBtn = nil
	}

	// Instrument buttons
	instStartY := listStartY
	for i := 0; i < vis && m.scroll.VS.First+i < len(m.state.filteredInsts); i++ {
		id := m.state.filteredInsts[m.scroll.VS.First+i]
		r := image.Rect(base.Min.X, instStartY+i*rowH, buttonMaxX, instStartY+(i+1)*rowH)
		if r.Min.Y < vertBounds.Min.Y {
			r = image.Rect(r.Min.X, vertBounds.Min.Y, r.Max.X, vertBounds.Min.Y+rowH)
		}
		if r.Max.Y > vertBounds.Max.Y {
			r = image.Rect(r.Min.X, vertBounds.Max.Y-rowH, r.Max.X, vertBounds.Max.Y)
		}

		optID := id
		label := m.state.displayLabelByID[id]
		if label == "" {
			label = id
		}
		btn := NewButton(label, DropdownStyle, func() {
			if m.props.OnSelect != nil {
				m.props.OnSelect(optID)
			}
			m.Close()
		})
		btn.Highlights = m.state.searchHighlights[optID]
		btn.SetRect(insetRect(r, buttonPad))
		m.instBtns = append(m.instBtns, btn)
	}

	m.buildCloseBtn()
	m.SetBounds(m.fullRect)
}

// buildCloseBtn creates the close button at the top-right of the menu.
func (m *InstrumentMenuComponent) buildCloseBtn() {
	if m.fullRect.Empty() {
		m.closeBtn = nil
		return
	}
	r := closeButtonRect(m.fullRect, buttonPad)
	m.closeBtn = NewButton("", PopupButtonStyle, func() { m.Close() })
	m.closeBtn.Icon = "close"
	m.closeBtn.IconColor = colButtonBorder
	m.closeBtn.SetRect(r)
	m.closeBtn.ConsumeOnPress = true
}

// CloseBtn returns the close button (for testing).
func (m *InstrumentMenuComponent) CloseBtn() *Button { return m.closeBtn }

// fireTapAt finds the button at (x, y) and calls its OnClick directly,
// bypassing Button.Handle's press-to-fire mechanism. This is used for
// deferred taps where the touch has ended and we know it was a tap.
func (m *InstrumentMenuComponent) fireTapAt(x, y int) {
	pt := image.Pt(x, y)
	// Close button has highest z-order — check first.
	if m.closeBtn != nil && pt.In(m.closeBtn.Rect()) && m.closeBtn.OnClick != nil {
		m.closeBtn.OnClick()
		return
	}
	if m.state.mode == InstMenuModeInstruments && m.backBtn != nil {
		if pt.In(m.backBtn.Rect()) && m.backBtn.OnClick != nil {
			m.backBtn.OnClick()
			return
		}
	}
	if m.state.mode == InstMenuModeCategories {
		for _, btn := range m.categoryBtns {
			if pt.In(btn.Rect()) && btn.OnClick != nil {
				btn.OnClick()
				return
			}
		}
	}
	if m.state.mode == InstMenuModeInstruments {
		for _, btn := range m.instBtns {
			if pt.In(btn.Rect()) && btn.OnClick != nil {
				btn.OnClick()
				return
			}
		}
	}
}

// HandleInput processes mouse input for the instrument menu.
func (m *InstrumentMenuComponent) HandleInput(x, y int, pressed bool) InputResult {
	if !m.state.open {
		// Handle hold state after close
		if m.state.hold {
			if !pressed {
				m.state.hold = false
			}
			return InputCaptured
		}
		return InputIgnored
	}

	m.ensureScroll()

	// ESC closes
	if isKeyPressed(ebiten.KeyEscape) {
		m.Close()
		return InputConsumed
	}

	// Handle scrollbar drag
	if m.scroll.Dragging() {
		if m.scroll.HandleDragTo(y) {
			m.state.userScrolled = true
			m.rebuildMenu()
		}
		if !pressed {
			m.scroll.HandleDragEnd()
		}
		return InputCaptured
	}

	// Handle touch scroll (suppresses button taps while scrolling)
	if m.scroll.ScrollingCommitted() {
		if pressed {
			m.scroll.HandleTouchMove(x, y)
		} else {
			// Scroll was committed — this is not a tap.
			m.deferredTap.Cancel()
			m.scroll.HandleTouchEnd()
		}
		if m.scroll.Dirty() {
			m.state.userScrolled = true
			m.scroll.ClearDirty()
			m.rebuildMenu()
		}
		return InputCaptured
	}

	// While a touch is active but not yet committed (in dead zone),
	// process moves. On release, check if it was a tap.
	if m.scroll.TouchActive() {
		if pressed {
			if m.scroll.HandleTouchMove(x, y) {
				m.state.userScrolled = true
				m.rebuildMenu()
			}
		} else {
			wasTap := !m.scroll.ScrollingCommitted()
			m.scroll.HandleTouchEnd()
			if wasTap {
				m.deferredTap.End(m.fireTapAt)
			} else {
				m.deferredTap.Cancel()
			}
		}
		return InputCaptured
	}

	pt := image.Pt(x, y)

	// Handle scrollbar click BEFORE touch begin — the scrollbar thumb is
	// inside VS.View so we must check it first to avoid intercepting it
	// as a touch scroll.
	if m.scroll.HasScroll() {
		thumbRect := m.scroll.ThumbRect()
		if pressed && pt.In(thumbRect) {
			m.scroll.HandleDragStart(y)
			return InputCaptured
		}
	}

	// Touch begin in scroll area: suppress ALL buttons on initial contact.
	// The deferred tap will fire on release if no scroll was committed.
	if pressed && pt.In(m.scroll.VS.View) && !m.scroll.TouchActive() && !m.scroll.Dragging() {
		if m.deferredTap.Begin(x, y) {
			m.scroll.HandleTouchBegin(x, y)
			return InputCaptured
		}
	}

	// Close button (highest z-order, outside scroll area)
	if m.closeBtn != nil && m.closeBtn.Handle(x, y, pressed) {
		return InputConsumed
	}

	// Consume clicks within the search rect to prevent click-through.
	// Keyboard/char input is handled by Update() every frame.
	if m.state.mode == InstMenuModeInstruments && m.searchBox != nil {
		if pt.In(m.searchRect) {
			return InputConsumed
		}
	}

	// Gate button handlers: suppress while touch is active or deferred tap pending.
	touchSuppressButtons := m.scroll.TouchActive() || m.deferredTap.Active()

	// Handle back button (suppress during touch scroll)
	if m.state.mode == InstMenuModeInstruments && m.backBtn != nil && !touchSuppressButtons {
		if m.backBtn.Handle(x, y, pressed) {
			return InputConsumed
		}
	}

	// Handle category buttons (suppress during touch scroll)
	if m.state.mode == InstMenuModeCategories && !touchSuppressButtons {
		for _, btn := range m.categoryBtns {
			if btn.Handle(x, y, pressed) {
				return InputConsumed
			}
		}
	}

	// Handle instrument buttons (suppress during drag or touch scroll)
	if m.state.mode == InstMenuModeInstruments && !m.scroll.Dragging() && !touchSuppressButtons {
		for _, btn := range m.instBtns {
			if btn.Handle(x, y, pressed) {
				return InputConsumed
			}
		}
	}

	// If pressed outside menu bounds, close it
	// Skip this check while suppressClicksUntilRelease is active to prevent
	// closing on category switch (geometry changes between modes)
	if pressed && !suppressClicksUntilRelease && !pt.In(m.fullRect) && !pt.In(m.props.AnchorRect) {
		m.Close()
		return InputConsumed
	}

	// If within bounds, consume to prevent click-through
	if pt.In(m.fullRect) {
		return InputConsumed
	}

	// Consume any press while click suppression is active to prevent
	// fall-through during geometry transitions.
	if pressed && suppressClicksUntilRelease {
		return InputConsumed
	}

	return InputIgnored
}

// HandleWheel processes mouse wheel scrolling.
func (m *InstrumentMenuComponent) HandleWheel(x, y, steps int) InputResult {
	if !m.state.open {
		return InputIgnored
	}
	if !image.Pt(x, y).In(m.fullRect) {
		return InputIgnored
	}
	m.ensureScroll()
	if m.scroll.HandleWheel(steps) {
		m.state.userScrolled = true
		m.rebuildMenu()
	}
	return InputConsumed // Always consume when menu is open and cursor is over it
}

// Update processes per-frame updates (search box polling + momentum decay).
// Touch move/end is handled entirely in HandleInput to avoid
// interfering with deferred tap detection.
func (m *InstrumentMenuComponent) Update() {
	if !m.state.open {
		return
	}
	m.ensureScroll()

	// Poll search box every frame so keyboard input is processed even
	// when no pointer events are active. HandleInput() only fires on
	// pointer events (via compHitHandler), so without this, typed
	// characters are silently lost after the initial click-to-focus.
	if m.state.mode == InstMenuModeInstruments && m.searchBox != nil {
		m.searchBox.Update()
		newText := m.searchBox.Value()
		if newText != m.state.searchText {
			m.state.searchText = newText
			m.rebuildMenu()
		}
	}

	if m.scroll.HasMomentum() {
		if m.scroll.UpdateMomentum() {
			m.state.userScrolled = true
			m.rebuildMenu()
		}
	}
}

// drawScrollbar renders the scrollbar if scrolling is needed.
func (m *InstrumentMenuComponent) drawScrollbar(dst *ebiten.Image) {
	m.ensureScroll()
	m.scroll.Draw(dst)
}

// Draw renders the instrument menu.
func (m *InstrumentMenuComponent) Draw(dst *ebiten.Image) {
	if !m.state.open {
		return
	}

	if m.state.mode == InstMenuModeCategories {
		for _, btn := range m.categoryBtns {
			btn.Draw(dst)
		}
		m.drawScrollbar(dst)
	} else {
		if m.backBtn != nil {
			m.backBtn.Draw(dst)
		}
		if m.searchBox != nil {
			m.searchBox.Draw(dst)
		}
		for _, btn := range m.instBtns {
			btn.Draw(dst)
		}
		m.drawScrollbar(dst)
	}
	// Close button on top
	if m.closeBtn != nil {
		m.closeBtn.Draw(dst)
	}
}

// Capturing returns whether the component is capturing input.
func (m *InstrumentMenuComponent) Capturing() bool {
	m.ensureScroll()
	return m.scroll.Dragging() || m.scroll.ScrollingCommitted() || m.state.hold
}

// InputBounds returns the menu bounds for overlay compatibility.
func (m *InstrumentMenuComponent) InputBounds() image.Rectangle {
	if !m.state.open {
		return image.Rectangle{}
	}
	return m.fullRect
}

// Mode returns the current menu mode.
func (m *InstrumentMenuComponent) Mode() InstMenuMode {
	return m.state.mode
}

// ActiveCategory returns the currently active category.
func (m *InstrumentMenuComponent) ActiveCategory() string {
	return m.state.activeCat
}

// Scroll returns the scroll state for testing.
func (m *InstrumentMenuComponent) Scroll() VerticalScroller {
	m.ensureScroll()
	return m.scroll.VS
}

// ScrollBehaviorRef returns the scroll behavior for testing.
func (m *InstrumentMenuComponent) ScrollBehaviorRef() *ScrollBehavior {
	m.ensureScroll()
	return m.scroll
}

// SearchBox returns the search text input for testing/legacy sync.
func (m *InstrumentMenuComponent) SearchBox() *TextInput {
	return m.searchBox
}

// SetSearchText sets the search text and rebuilds the menu.
func (m *InstrumentMenuComponent) SetSearchText(text string) {
	m.state.searchText = text
	if m.searchBox != nil {
		m.searchBox.SetText(text)
	}
	m.state.userScrolled = false
	m.rebuildMenu()
}

// SetScrollFirst sets the scroll position and rebuilds the menu.
// This is used to sync scroll state from legacy code.
func (m *InstrumentMenuComponent) SetScrollFirst(first int) {
	if !m.state.open {
		return
	}
	m.ensureScroll()
	m.scroll.VS.First = first
	m.scroll.VS.Clamp()
	m.state.userScrolled = true
	m.rebuildMenu()
}
