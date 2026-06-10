package ui

import (
	"image"
	"slices"
	"strings"

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
	// OnStateChanged, if non-nil, fires after every internal mutation
	// that affects publicly observable state (mode, active category,
	// scroll position, button list). Hosts use this to mirror the
	// component's state into their own bookkeeping fields without
	// reaching through accessor methods on every read. Replaces the
	// retired OnRebuild → syncInstMenuBtnsFromComp pair: now there
	// is exactly one notifier and one canonical sync helper.
	OnStateChanged func()

	// Favorites, if non-nil, drives the per-row star toggle. The menu
	// reads Get to render the icon variant and writes via OnFavorite
	// (or directly on Set if OnFavorite is nil).
	Favorites FavoritesStore
	// OnFavorite, if non-nil, is invoked when the user toggles the
	// star on the i'th row. Callers typically delegate to the
	// FavoritesStore but can intercept for telemetry, logging, etc.
	OnFavorite func(instID string, fav bool)

	// ProjectPins are the per-project pinned instrument ids (tier 0 in
	// the menu's PinSource). Independent from Favorites/UserStars (tier
	// 1). When non-empty, project-pinned items sort above ★ items in
	// every render path. No mutation hook today — the per-row "pin to
	// project" affordance ships in a follow-up; this PR only consumes
	// the set on the read path.
	ProjectPins map[string]struct{}

	// ShowFavoritesCategory makes the menu prepend a virtual "Favorites"
	// category to the category list (when Favorites is also non-nil).
	// Default is false so existing tests that drive the menu without
	// setting this flag retain the pre-PR layout — `Categories[0]`
	// stays at category-button index 0. Production wiring opts in via
	// the props builder in drumview_instrument_helpers.go.
	ShowFavoritesCategory bool

	// Style overrides element dimensions for the menu chrome
	// (breadcrumb / pagination chips / star column / jump-input).
	// Zero-value falls back to design-system defaults via
	// MenuStyle.resolved(). Set fields explicitly to pin a
	// dimension regardless of the active layout profile.
	Style MenuStyle
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
	// pageChipsThreshold pins the boundary between numbered chips
	// and the numeric jump-input. ≤ threshold pages → chips render;
	// > threshold → "Page [N]/M" with a typed input.
	pageChipsThreshold = 7
	// breadcrumbSeparatorIcon is the glyph drawn between breadcrumb
	// segments. The forbidden-glyph guard rejects literal U+2039 /
	// U+203A; this constant is the canonical replacement.
	breadcrumbSeparatorIcon = IconChevronRight
)

// InstrumentMenuState contains the internal state for the instrument menu component.
type InstrumentMenuState struct {
	open             bool
	hold             bool // Capture flag after menu closes
	mode             InstMenuMode
	activeCat        string
	// favoritesView, when true, makes buildInstrumentsMode filter to ★-favorited
	// ids only and skip the activeCat check. Set by clicking the virtual
	// "Favorites" category at the top of buildCategoriesMode; cleared by Back.
	favoritesView    bool
	userScrolled     bool
	lastAdded        string
	cameFromCats     bool
	searchText       string
	filteredInsts    []string
	// fuzzyScores parallels filteredInsts when searchText is non-empty;
	// used by StableTierBreaker to break score ties by tier without losing
	// the score primary key. Empty when no search query is active.
	fuzzyScores      map[string]int
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

	// Breadcrumb segment hit areas (drawn above search row). Rebuilt on
	// every layout pass; clicking segment[i] pops to depth i.
	breadcrumbRects []image.Rectangle
	breadcrumbRect  image.Rectangle

	// Pagination strip hit areas. pageChipRects covers numbered page
	// chips when PageCount() ≤ pageChipsThreshold; jumpInputRect +
	// prevPageRect/nextPageRect cover the >threshold variant.
	pageChipRects  []image.Rectangle
	prevPageRect   image.Rectangle
	nextPageRect   image.Rectangle
	jumpInput      *TextInput
	jumpInputRect  image.Rectangle
	paginationRect image.Rectangle

	// Keyboard navigation state.
	selectedIdx int
	keyEdge     map[ebiten.Key]bool

	// favRects and favIDs together define the per-row star hit areas
	// when props.Favorites is non-nil. favRects[i] is screen-space and
	// favIDs[i] is the corresponding instrument id. They are rebuilt
	// on every rebuildMenu pass and read by HandleInput.
	favRects []image.Rectangle
	favIDs   []string

	// Deferred tap: position stored on touch begin, fired on touch end if no scroll committed.
	deferredTap DeferredTap

	// style is the resolved MenuStyle (props.Style.resolved()) recomputed
	// on every SetProps. Used by element-rect computations so element
	// sizes flow from the design-system layer instead of being
	// hard-coded inline.
	style MenuStyle
}

// NewInstrumentMenuComponent creates a new instrument menu component.
func NewInstrumentMenuComponent() *InstrumentMenuComponent {
	return &InstrumentMenuComponent{
		overlayBase: newOverlayBase(),
		scroll:      NewScrollBehavior(DropdownScrollbarStyle, 24),
		keyEdge:     map[ebiten.Key]bool{},
		style:       DefaultMenuStyle(),
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
	m.style = p.Style.resolved()
	if needsRebuild {
		m.rebuildMaps()
	}
}

// Style returns the resolved MenuStyle currently in use. Useful for
// tests and callers that need to align surrounding chrome (e.g. the
// portal close-button position) with the menu's element sizes.
func (m *InstrumentMenuComponent) Style() MenuStyle { return m.style }

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

// Refresh re-runs rebuildMenu so callers can flush new prop values
// (instrument catalog updates, search-filter changes) without
// re-opening the menu. Cheap when the menu is closed (early returns
// inside rebuildMenu via empty AnchorRect / IsOpen guards).
func (m *InstrumentMenuComponent) Refresh() {
	if m == nil {
		return
	}
	m.rebuildMenu()
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
	if m.props.OnStateChanged != nil {
		m.props.OnStateChanged()
	}
}

// favoritesCategoryEnabled reports whether the virtual "Favorites" category
// should be injected at the top of the category list. Both flags must be
// set: ShowFavoritesCategory (caller opt-in) AND a non-nil Favorites store.
// Tests that don't opt in keep the legacy layout where Categories[0] is the
// first rendered category button.
func (m *InstrumentMenuComponent) favoritesCategoryEnabled() bool {
	return m != nil && m.props.ShowFavoritesCategory && m.props.Favorites != nil
}

// favoritesIDs returns the union of ★ keys (tier 1) for the favoritesView
// filter. Project pins are NOT folded in here — they appear in their natural
// categories (and sort to the top of every list via PinSource), but they are
// not necessarily favorited and shouldn't be force-included in the
// "Favorites" view if the user hasn't ★'d them.
func (m *InstrumentMenuComponent) favoritesIDs() map[string]struct{} {
	out := map[string]struct{}{}
	if m == nil || m.props.Favorites == nil {
		return out
	}
	for _, k := range m.props.Favorites.Keys() {
		out[k] = struct{}{}
	}
	return out
}

// pinSource builds a render-time PinSource snapshot from props. Cheap; the
// underlying maps come straight from props.ProjectPins and Favorites().Keys().
func (m *InstrumentMenuComponent) pinSource() PinSource {
	src := PinSource{ProjectPins: m.props.ProjectPins, UserStars: m.favoritesIDs()}
	return src
}

// buildCategoriesMode builds the menu in categories mode.
func (m *InstrumentMenuComponent) buildCategoriesMode(base, vertBounds image.Rectangle, rowH int, openUp bool) {
	// Render list of categories: virtual "Favorites" prepended when a
	// FavoritesStore is wired, then the caller-supplied categories.
	categories := m.props.Categories
	favPrefix := m.favoritesCategoryEnabled()
	catCount := len(categories)
	if favPrefix {
		catCount++
	}
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

	// Bias to active category. The Favorites virtual category is index 0 when
	// favPrefix is true; the caller-supplied categories start at offset 1.
	if !m.state.userScrolled && m.state.activeCat != "" {
		if idx := slices.Index(categories, m.state.activeCat); idx >= 0 {
			absIdx := idx
			if favPrefix {
				absIdx++
			}
			first := absIdx - vis + 1
			if first < 0 {
				first = 0
			}
			m.scroll.VS.First = first
		}
	}
	m.scroll.VS.Clamp()

	// Build category buttons. When favPrefix is true, the row at offset 0 is
	// the virtual "Favorites" entry and the caller-supplied categories shift
	// down by one. Selecting "Favorites" enters instruments mode with the
	// favoritesView filter set; selecting any other category clears it.
	start := m.scroll.VS.First
	for i := 0; i < vis && start+i < catCount; i++ {
		absIdx := start + i
		r := image.Rect(base.Min.X, startY+i*rowH, base.Max.X, startY+(i+1)*rowH)

		if favPrefix && absIdx == 0 {
			favCount := len(m.favoritesIDs())
			label := "Favorites"
			if favCount > 0 {
				label = "Favorites (" + pageChipLabel(favCount) + ")"
			}
			btn := NewButton(label, DropdownStyle, func() {
				m.state.favoritesView = true
				m.state.activeCat = ""
				m.state.mode = InstMenuModeInstruments
				m.scroll.VS.First = 0
				m.state.cameFromCats = true
				m.state.userScrolled = false
				m.rebuildMenu()
			})
			if m.state.favoritesView {
				btn.Style = PopupButtonStyle
			}
			btn.SetRect(insetRect(r, SpaceXS))
			m.categoryBtns = append(m.categoryBtns, btn)
			continue
		}

		// Caller-supplied categories. catIdx accounts for the virtual prefix.
		catIdx := absIdx
		if favPrefix {
			catIdx--
		}
		if catIdx < 0 || catIdx >= len(categories) {
			continue
		}
		cat := categories[catIdx]
		btnCat := cat
		btn := NewButton(btnCat, DropdownStyle, func() {
			m.state.favoritesView = false
			m.state.activeCat = btnCat
			m.state.mode = InstMenuModeInstruments
			m.scroll.VS.First = 0
			m.state.cameFromCats = true
			m.state.userScrolled = false
			m.rebuildMenu()
		})
		if !m.state.favoritesView && btnCat == m.state.activeCat {
			btn.Style = PopupButtonStyle
		}
		btn.SetRect(insetRect(r, SpaceXS))
		m.categoryBtns = append(m.categoryBtns, btn)
	}

	m.buildCloseBtn()
	m.SetBounds(m.fullRect)
}

// buildInstrumentsMode builds the menu in instruments mode.
func (m *InstrumentMenuComponent) buildInstrumentsMode(base, vertBounds image.Rectangle, rowH int, openUp bool) {
	// Filter instruments: favorites view OR category filter, then fuzzy
	// search. favoritesView short-circuits the category filter so the user
	// sees their full ★ set regardless of which category each item lives in.
	var favIDs map[string]struct{}
	if m.state.favoritesView {
		favIDs = m.favoritesIDs()
	}
	var catItems []MenuSearchItem
	for _, inst := range m.props.Instruments {
		if m.state.favoritesView {
			if _, ok := favIDs[inst.ID]; !ok {
				continue
			}
		} else if m.state.activeCat != "" && m.state.categoryByID[inst.ID] != m.state.activeCat {
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
	queryActive := strings.TrimSpace(m.state.searchText) != ""
	if queryActive {
		m.state.fuzzyScores = make(map[string]int, len(searchResults))
	} else {
		m.state.fuzzyScores = nil
	}
	for _, r := range searchResults {
		m.state.filteredInsts = append(m.state.filteredInsts, r.Key)
		if len(r.Highlights) > 0 {
			m.state.searchHighlights[r.Key] = r.Highlights
		}
		if queryActive {
			m.state.fuzzyScores[r.Key] = r.Score
		}
	}

	// Tier sort. Favorites view is already filtered to the ★ set, so a tier
	// sort there would just rearrange the alpha order — skip it (everything
	// is favorited; sort alphabetically by label only).
	if !m.state.favoritesView {
		labelFor := func(id string) string {
			lbl := m.state.displayLabelByID[id]
			if lbl == "" {
				lbl = id
			}
			return lbl
		}
		src := m.pinSource()
		if queryActive {
			src.StableTierBreaker(m.state.filteredInsts,
				func(id string) int { return m.state.fuzzyScores[id] },
				labelFor)
		} else {
			src.SortByTierAlpha(m.state.filteredInsts, labelFor)
		}
	} else if !queryActive {
		// Favorites view, no query → alphabetical by label.
		labelFor := func(id string) string {
			lbl := m.state.displayLabelByID[id]
			if lbl == "" {
				lbl = id
			}
			return lbl
		}
		PinSource{}.SortByTierAlpha(m.state.filteredInsts, labelFor)
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
	m.searchBox.Rect = insetRect(m.searchRect, SpaceSM)

	listStartY := searchY + rowH

	m.scroll.VS.Total = len(m.state.filteredInsts)
	m.scroll.VS.Visible = vis

	if len(m.state.filteredInsts) == 0 {
		// Even with an empty result set we still render the Back button (when
		// the menu has categories) so the user can always escape — without
		// this, a Favorites view with no ★s and a query that produces no
		// matches both leave the user stuck. Back rect lives at the original
		// position above the search row.
		extraTop := rowH                // search row
		if showBack {
			extraTop += rowH            // back row
		}
		emptyView := image.Rect(base.Min.X, listStartY, base.Max.X, listStartY+rowH)
		m.scroll.VS.View = emptyView
		m.fullRect = image.Rect(base.Min.X, startY, base.Max.X, startY+extraTop+rowH)
		emptyText := "No matches"
		if m.state.favoritesView && strings.TrimSpace(m.state.searchText) == "" {
			emptyText = "No favorites yet — tap the star on any instrument"
		}
		if showBack {
			bcH := m.style.BreadcrumbStripH
			if bcH <= 0 || bcH > rowH {
				bcH = rowH
			}
			backRect := image.Rect(base.Min.X, startY, base.Max.X, startY+bcH)
			m.backBtn = NewButton("Back", DropdownStyle, func() {
				m.state.mode = InstMenuModeCategories
				m.state.favoritesView = false
				m.scroll.VS.First = 0
				m.state.cameFromCats = false
				m.state.userScrolled = false
				m.rebuildMenu()
			})
			m.backBtn.SetRect(insetRect(backRect, SpaceXS))
			m.computeBreadcrumbStrip(backRect)
		} else {
			m.backBtn = nil
		}
		placeholder := NewButton(emptyText, DisabledButtonStyle, nil)
		placeholder.SetRect(insetRect(emptyView, SpaceXS))
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

	// Pagination strip layout: bottom row of the menu, full width.
	// Height comes from MenuStyle.PaginationStripH so the design
	// system can shrink the strip independently of the row height.
	pagH := m.style.PaginationStripH
	if pagH <= 0 || pagH > rowH {
		pagH = rowH
	}
	pagY := listStartY + vis*rowH
	pagRect := image.Rect(base.Min.X, pagY, base.Max.X, pagY+pagH)
	m.computePaginationStrip(pagRect)

	hasScroll := m.scroll.HasScroll()
	buttonMaxX := base.Max.X
	if hasScroll {
		buttonMaxX -= instMenuScrollBarWidthComp
		if buttonMaxX <= base.Min.X {
			buttonMaxX = base.Min.X + 1
		}
	}
	// Star toggle column reserves a small fixed slot at the right edge
	// when favorites are wired. Width comes from MenuStyle so the
	// design system controls the column dimension.
	favColWidth := m.style.StarColW
	starsActive := m.props.Favorites != nil
	if starsActive {
		buttonMaxX -= favColWidth
		if buttonMaxX <= base.Min.X {
			buttonMaxX = base.Min.X + 1
		}
	}
	m.favRects = m.favRects[:0]
	m.favIDs = m.favIDs[:0]

	// Breadcrumb strip + back-button compat. The breadcrumb segments
	// "Categories › <activeCat>" are clickable hit areas drawn at the
	// top of the menu; clicking "Categories" pops back to the category
	// list. The legacy back button is kept (rendered invisibly behind
	// the breadcrumb strip) so existing tests that trigger BackBtn()
	// continue to work.
	if showBack {
		// Breadcrumb height defaults to row height for visual alignment
		// with surrounding rows, but MenuStyle.BreadcrumbStripH may
		// shrink it independently.
		bcH := m.style.BreadcrumbStripH
		if bcH <= 0 || bcH > rowH {
			bcH = rowH
		}
		backRect := image.Rect(base.Min.X, startY, base.Max.X, startY+bcH)
		m.backBtn = NewButton("Back", DropdownStyle, func() {
			m.state.mode = InstMenuModeCategories
			m.state.favoritesView = false
			m.scroll.VS.First = 0
			m.state.cameFromCats = false
			m.state.userScrolled = false
			m.rebuildMenu()
		})
		m.backBtn.SetRect(insetRect(backRect, SpaceXS))
		m.computeBreadcrumbStrip(backRect)
	} else {
		m.backBtn = nil
		m.breadcrumbRects = m.breadcrumbRects[:0]
		m.breadcrumbRect = image.Rectangle{}
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
			// Menu intentionally stays open after selection so the user can
			// audition multiple instruments. Dismissal is explicit: X button,
			// Escape, click-outside, or opening another row's menu.
		})
		btn.Highlights = m.state.searchHighlights[optID]
		btn.SetRect(insetRect(r, SpaceXS))
		m.instBtns = append(m.instBtns, btn)
		if starsActive {
			// Star hit area is the small rectangle to the right of the
			// row button. Drawn in Draw, hit-tested in HandleInput.
			starRect := image.Rect(buttonMaxX, r.Min.Y, buttonMaxX+favColWidth, r.Max.Y)
			m.favRects = append(m.favRects, starRect)
			m.favIDs = append(m.favIDs, optID)
		}
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
	r := closeButtonRect(m.fullRect, SpaceXS)
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
		// Star hit area takes priority over the row button so a touch on
		// the star toggles the favorite instead of selecting the row.
		for i, rect := range m.favRects {
			if pt.In(rect) {
				m.toggleFavoriteAt(i)
				return
			}
		}
		for _, btn := range m.instBtns {
			if pt.In(btn.Rect()) && btn.OnClick != nil {
				btn.OnClick()
				return
			}
		}
	}
}

// toggleFavoriteAt flips the star for favIDs[i] and notifies the
// caller. Bounds-safe so callers can pass hit-test results.
func (m *InstrumentMenuComponent) toggleFavoriteAt(i int) {
	if m.props.Favorites == nil {
		return
	}
	if i < 0 || i >= len(m.favIDs) {
		return
	}
	id := m.favIDs[i]
	now := !m.props.Favorites.Get(id)
	m.props.Favorites.Set(id, now)
	emitFavoriteToggled(id, now)
	if m.props.OnFavorite != nil {
		m.props.OnFavorite(id, now)
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
	if m.closeBtn != nil && m.closeBtn.HandleInputResult(x, y, pressed) != InputIgnored {
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
		if m.backBtn.HandleInputResult(x, y, pressed) != InputIgnored {
			return InputConsumed
		}
	}

	// Handle category buttons (suppress during touch scroll)
	if m.state.mode == InstMenuModeCategories && !touchSuppressButtons {
		for _, btn := range m.categoryBtns {
			if btn.HandleInputResult(x, y, pressed) != InputIgnored {
				return InputConsumed
			}
		}
	}

	// Breadcrumb segment click — pop to that depth. Pressed edge only.
	if pressed && m.state.mode == InstMenuModeInstruments && !touchSuppressButtons && len(m.breadcrumbRects) > 0 {
		if d := m.breadcrumbHitAt(x, y); d >= 0 && d < len(m.BreadcrumbPath())-1 {
			m.popBreadcrumbTo(d)
			return InputConsumed
		}
	}

	// Pagination strip clicks: prev/next chevrons, numbered chips, and
	// the jump-input field. Routed before row buttons so a click in the
	// strip never selects an instrument.
	if pressed && m.state.mode == InstMenuModeInstruments && !touchSuppressButtons {
		if pt.In(m.prevPageRect) {
			m.kbPageUp()
			return InputConsumed
		}
		if pt.In(m.nextPageRect) {
			m.kbPageDown()
			return InputConsumed
		}
		for i, chip := range m.pageChipRects {
			if pt.In(chip) {
				m.jumpToPage(i + 1)
				return InputConsumed
			}
		}
		if pt.In(m.jumpInputRect) && m.jumpInput != nil {
			m.jumpInput.SetFocus(true)
			return InputConsumed
		}
	}

	// Handle star (favorite) toggles ahead of the row buttons so a click
	// on the star never selects the instrument. Only fires on the press
	// edge to match Button.Handle semantics.
	if pressed && m.state.mode == InstMenuModeInstruments && !m.scroll.Dragging() && !touchSuppressButtons && len(m.favRects) > 0 {
		for i, rect := range m.favRects {
			if pt.In(rect) {
				m.toggleFavoriteAt(i)
				return InputConsumed
			}
		}
	}

	// Handle instrument buttons (suppress during drag or touch scroll)
	if m.state.mode == InstMenuModeInstruments && !m.scroll.Dragging() && !touchSuppressButtons {
		for _, btn := range m.instBtns {
			if btn.HandleInputResult(x, y, pressed) != InputIgnored {
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

// Update processes per-frame updates (search box polling + momentum decay
// + keyboard navigation).
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

	// Keyboard navigation polled on rising edges. Mobile (bottom sheet)
	// skips keyboard handlers — touch is the input model there.
	if !Profile().IsMobile() {
		m.handleKeyboardNav()
	}

	if m.scroll.HasMomentum() {
		if m.scroll.UpdateMomentum() {
			m.state.userScrolled = true
			m.rebuildMenu()
		}
	}
}

// handleKeyboardNav dispatches arrow keys, Enter, PgUp/PgDn, Home/End,
// "/", and Esc to the appropriate scroll/selection action. Rising-edge
// detection prevents key-repeat spam.
func (m *InstrumentMenuComponent) handleKeyboardNav() {
	m.handleKeyEdge(ebiten.KeyDown, m.kbMoveDown)
	m.handleKeyEdge(ebiten.KeyUp, m.kbMoveUp)
	m.handleKeyEdge(ebiten.KeyEnter, m.kbConfirm)
	m.handleKeyEdge(ebiten.KeyPageDown, m.kbPageDown)
	m.handleKeyEdge(ebiten.KeyPageUp, m.kbPageUp)
	m.handleKeyEdge(ebiten.KeyHome, m.kbHome)
	m.handleKeyEdge(ebiten.KeyEnd, m.kbEnd)
	m.handleKeyEdge(ebiten.KeySlash, m.kbFocusSearch)
}

func (m *InstrumentMenuComponent) handleKeyEdge(k ebiten.Key, fn func()) {
	now := isKeyPressed(k)
	if now && !m.keyEdge[k] {
		fn()
	}
	m.keyEdge[k] = now
}

func (m *InstrumentMenuComponent) kbMoveDown() {
	if m.searchBox != nil && m.searchBox.Focused() {
		return
	}
	if m.state.mode != InstMenuModeInstruments {
		return
	}
	if len(m.state.filteredInsts) == 0 {
		return
	}
	if m.selectedIdx < len(m.state.filteredInsts)-1 {
		m.selectedIdx++
	}
	m.scrollToSelection()
}

func (m *InstrumentMenuComponent) kbMoveUp() {
	if m.searchBox != nil && m.searchBox.Focused() {
		return
	}
	if m.state.mode != InstMenuModeInstruments {
		return
	}
	if m.selectedIdx > 0 {
		m.selectedIdx--
	}
	m.scrollToSelection()
}

func (m *InstrumentMenuComponent) kbConfirm() {
	if m.searchBox != nil && m.searchBox.Focused() {
		return
	}
	if m.state.mode != InstMenuModeInstruments {
		return
	}
	if m.selectedIdx < 0 || m.selectedIdx >= len(m.state.filteredInsts) {
		return
	}
	id := m.state.filteredInsts[m.selectedIdx]
	if m.props.OnSelect != nil {
		m.props.OnSelect(id)
	}
	m.Close()
}

func (m *InstrumentMenuComponent) kbPageDown() {
	if m.scroll.VS.Visible <= 0 {
		return
	}
	m.scroll.VS.First += m.scroll.VS.Visible
	m.scroll.VS.Clamp()
	m.state.userScrolled = true
	m.rebuildMenu()
}

func (m *InstrumentMenuComponent) kbPageUp() {
	if m.scroll.VS.Visible <= 0 {
		return
	}
	m.scroll.VS.First -= m.scroll.VS.Visible
	m.scroll.VS.Clamp()
	m.state.userScrolled = true
	m.rebuildMenu()
}

func (m *InstrumentMenuComponent) kbHome() {
	m.scroll.VS.First = 0
	m.scroll.VS.Clamp()
	m.selectedIdx = 0
	m.state.userScrolled = true
	m.rebuildMenu()
}

func (m *InstrumentMenuComponent) kbEnd() {
	if m.scroll.VS.Total > 0 && m.scroll.VS.Visible > 0 {
		m.scroll.VS.First = m.scroll.VS.Total - m.scroll.VS.Visible
	}
	m.scroll.VS.Clamp()
	if len(m.state.filteredInsts) > 0 {
		m.selectedIdx = len(m.state.filteredInsts) - 1
	}
	m.state.userScrolled = true
	m.rebuildMenu()
}

func (m *InstrumentMenuComponent) kbFocusSearch() {
	if m.searchBox == nil {
		return
	}
	m.searchBox.SetFocus(true)
}

// scrollToSelection ensures the selected row is within the visible
// page, advancing m.scroll.VS.First as needed.
func (m *InstrumentMenuComponent) scrollToSelection() {
	if m.scroll.VS.Visible <= 0 {
		return
	}
	if m.selectedIdx < m.scroll.VS.First {
		m.scroll.VS.First = m.selectedIdx
	} else if m.selectedIdx >= m.scroll.VS.First+m.scroll.VS.Visible {
		m.scroll.VS.First = m.selectedIdx - m.scroll.VS.Visible + 1
	}
	m.scroll.VS.Clamp()
	m.state.userScrolled = true
	m.rebuildMenu()
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
	if m.fullRect.Empty() {
		return
	}

	drawScrim(dst)
	if Profile().UseBottomSheet {
		drawBottomSheetPanel(dst, m.fullRect)
	} else {
		drawPanel(dst, m.fullRect)
	}
	if Profile().IsMobile() {
		m.drawBottomSheetHandle(dst)
	}

	if m.state.mode == InstMenuModeCategories {
		for _, btn := range m.categoryBtns {
			btn.Draw(dst)
		}
		m.drawScrollbar(dst)
	} else {
		// Breadcrumb strip replaces the back button visually; the
		// back button remains for legacy click compat.
		m.drawBreadcrumbStrip(dst)
		if m.searchBox != nil {
			m.drawSearchRow(dst)
		}
		for _, btn := range m.instBtns {
			btn.Draw(dst)
		}
		m.drawFavoriteStars(dst)
		m.drawPaginationStrip(dst)
		m.drawScrollbar(dst)
	}
	if m.closeBtn != nil {
		m.closeBtn.Draw(dst)
	}
}

// computeBreadcrumbStrip lays out the breadcrumb segments inside r.
// At depth 0 (categories mode) the strip is a single root segment;
// at depth 1 (instruments mode in a category) it's two segments
// separated by IconChevronRight. Hit areas land in m.breadcrumbRects
// in left-to-right order.
func (m *InstrumentMenuComponent) computeBreadcrumbStrip(r image.Rectangle) {
	m.breadcrumbRect = r
	m.breadcrumbRects = m.breadcrumbRects[:0]
	labels := m.BreadcrumbPath()
	if len(labels) == 0 {
		return
	}
	// Equal-width slots with separator gutters between them. Separator
	// gutter width is the icon size plus segment padding so the chevron
	// has visual breathing room without overlapping segment text.
	sepW := IconSizeMD + m.style.SegmentPaddingX
	totalW := r.Dx()
	if len(labels) == 1 {
		m.breadcrumbRects = append(m.breadcrumbRects, r)
		return
	}
	gaps := len(labels) - 1
	segW := (totalW - gaps*sepW) / len(labels)
	if segW < 1 {
		segW = 1
	}
	x := r.Min.X
	for range labels {
		seg := image.Rect(x, r.Min.Y, x+segW, r.Max.Y)
		m.breadcrumbRects = append(m.breadcrumbRects, seg)
		x = seg.Max.X + sepW
	}
}

// drawBreadcrumbStrip renders the segment labels with chevron-icon
// separators. The active (last) segment uses the on-surface accent
// color; previous segments use the secondary text color so the user
// reads them as clickable.
func (m *InstrumentMenuComponent) drawBreadcrumbStrip(dst *ebiten.Image) {
	labels := m.BreadcrumbPath()
	if len(labels) == 0 || len(m.breadcrumbRects) == 0 {
		return
	}
	for i, seg := range m.breadcrumbRects {
		if i >= len(labels) {
			break
		}
		col := colTextSecondary
		if i == len(labels)-1 {
			col = colTextAccent
		}
		DrawTextColorAt(dst, labels[i], seg.Min.X+m.style.SegmentPaddingX, seg.Min.Y+seg.Dy()/2, col)
		// Separator chevron between segments.
		if i < len(labels)-1 && i < len(m.breadcrumbRects)-1 {
			next := m.breadcrumbRects[i+1]
			sepRect := image.Rect(seg.Max.X, seg.Min.Y, next.Min.X, seg.Max.Y)
			DrawIcon(dst, breadcrumbSeparatorIcon, insetRect(sepRect, SpaceXS), colTextSecondary)
		}
	}
}

// jumpToPage navigates the underlying scroll state to the (1-based)
// page n, clamped to [1, PageCount()].
func (m *InstrumentMenuComponent) jumpToPage(n int) {
	pc := m.PageCount()
	if pc <= 0 {
		return
	}
	if n < 1 {
		n = 1
	}
	if n > pc {
		n = pc
	}
	m.scroll.VS.First = (n - 1) * m.scroll.VS.Visible
	m.scroll.VS.Clamp()
	m.state.userScrolled = true
	m.rebuildMenu()
}

// computePaginationStrip lays out the pagination chips or jump-input
// inside r. ≤ pageChipsThreshold pages → numbered chips; otherwise a
// numeric input plus prev/next chevrons. Hit areas land in
// m.pageChipRects, m.prevPageRect, m.nextPageRect, m.jumpInputRect.
func (m *InstrumentMenuComponent) computePaginationStrip(r image.Rectangle) {
	m.paginationRect = r
	m.pageChipRects = m.pageChipRects[:0]
	m.prevPageRect = image.Rectangle{}
	m.nextPageRect = image.Rectangle{}
	m.jumpInputRect = image.Rectangle{}
	pc := m.PageCount()
	if pc <= 1 {
		return
	}
	chevW := IconSizeLG
	prev := image.Rect(r.Min.X, r.Min.Y, r.Min.X+chevW, r.Max.Y)
	next := image.Rect(r.Max.X-chevW, r.Min.Y, r.Max.X, r.Max.Y)
	m.prevPageRect = prev
	m.nextPageRect = next
	innerLeft := prev.Max.X + SpaceXS
	innerRight := next.Min.X - SpaceXS
	innerW := innerRight - innerLeft
	if innerW <= 0 {
		return
	}
	if pc <= pageChipsThreshold {
		// Numbered chips, one per page. Width is innerW/pc but at
		// least style.ChipMinW so chips remain tappable.
		chipW := innerW / pc
		if chipW < m.style.ChipMinW {
			chipW = m.style.ChipMinW
		}
		x := innerLeft
		for i := 0; i < pc; i++ {
			chip := image.Rect(x, r.Min.Y, x+chipW, r.Max.Y)
			m.pageChipRects = append(m.pageChipRects, chip)
			x = chip.Max.X
		}
		return
	}
	// Jump-input variant: "Page [N]/M" with a numeric TextInput.
	m.jumpInputRect = image.Rect(innerLeft, r.Min.Y, innerRight, r.Max.Y)
	if m.jumpInput == nil {
		m.jumpInput = NewTextInput(image.Rectangle{}, BPMBoxStyle)
		m.jumpInput.MaxLen = 4
		m.jumpInput.Accept = func(rn rune) bool { return rn >= '0' && rn <= '9' }
		m.jumpInput.InputMode = "numeric"
	}
	m.jumpInput.Rect = insetRect(m.jumpInputRect, SpaceXS)
}

// drawPaginationStrip renders the pagination row at the bottom of the
// menu. No-op when there's only one page.
func (m *InstrumentMenuComponent) drawPaginationStrip(dst *ebiten.Image) {
	pc := m.PageCount()
	if pc <= 1 || m.paginationRect.Empty() {
		return
	}
	if !m.prevPageRect.Empty() {
		DrawIcon(dst, IconChevronLeft, insetRect(m.prevPageRect, SpaceXS), colTextSecondary)
	}
	if !m.nextPageRect.Empty() {
		DrawIcon(dst, IconChevronRight, insetRect(m.nextPageRect, SpaceXS), colTextSecondary)
	}
	cur := m.Page()
	if pc <= pageChipsThreshold {
		for i, chip := range m.pageChipRects {
			label := pageChipLabel(i + 1)
			col := colTextSecondary
			if i+1 == cur {
				col = colTextAccent
			}
			DrawTextColorAt(dst, label, chip.Min.X+chip.Dx()/2, chip.Min.Y+chip.Dy()/2, col)
		}
		return
	}
	// Jump-input variant.
	if m.jumpInput != nil {
		m.jumpInput.Draw(dst)
	}
	DrawTextColorAt(dst, pageOfMLabel(cur, pc), m.jumpInputRect.Min.X+SpaceSM, m.jumpInputRect.Min.Y+m.jumpInputRect.Dy()/2, colTextSecondary)
}

// pageChipLabel formats a 1-based page number for chip display.
func pageChipLabel(n int) string {
	const digits = "0123456789"
	if n <= 0 {
		return "1"
	}
	if n < 10 {
		return string(digits[n])
	}
	out := ""
	for n > 0 {
		out = string(digits[n%10]) + out
		n /= 10
	}
	return out
}

// pageOfMLabel formats the "N / M" label for the jump-input variant.
func pageOfMLabel(n, m int) string {
	return pageChipLabel(n) + " / " + pageChipLabel(m)
}

// breadcrumbHitAt returns the depth (0-based) of the segment under
// (x, y), or -1 if no breadcrumb segment is hit.
func (m *InstrumentMenuComponent) breadcrumbHitAt(x, y int) int {
	pt := image.Pt(x, y)
	for idx, seg := range m.breadcrumbRects {
		if pt.In(seg) {
			return idx
		}
	}
	return -1
}

// popBreadcrumbTo navigates to the breadcrumb segment at depth d.
// d == 0 returns to categories mode; deeper depths are no-ops in the
// current 2-level model (room for future deeper hierarchies).
func (m *InstrumentMenuComponent) popBreadcrumbTo(d int) {
	if d <= 0 {
		m.state.mode = InstMenuModeCategories
		m.state.activeCat = ""
		m.state.favoritesView = false
		m.scroll.VS.First = 0
		m.state.cameFromCats = false
		m.state.userScrolled = false
		m.rebuildMenu()
		return
	}
	// Future-proof: deeper levels become no-ops today.
}

// drawFavoriteStars renders the per-row star icons. Empty star (outline)
// for unfavorited items; filled star for favorited. The hit areas live
// in m.favRects with matching ids in m.favIDs.
func (m *InstrumentMenuComponent) drawFavoriteStars(dst *ebiten.Image) {
	if m.props.Favorites == nil {
		return
	}
	for i, rect := range m.favRects {
		id := m.favIDs[i]
		fav := m.props.Favorites.Get(id)
		icon := IconStar
		col := colTextSecondary
		if fav {
			icon = IconStarFilled
			col = genColorPrimary
		}
		// Inset slightly so the icon doesn't touch the row border.
		DrawIcon(dst, icon, insetRect(rect, SpaceXS), col)
	}
}

// BreadcrumbPath returns the current path as visible-segment labels.
// Returns []string{} when closed; ["Categories"] at root; ["Categories",
// <activeCat>] when drilled into a category. Used by the new
// instMenuBreadcrumbPath JS export and by tests asserting on the
// breadcrumb state without coupling to the legacy mode enum.
func (m *InstrumentMenuComponent) BreadcrumbPath() []string {
	if !m.state.open {
		return []string{}
	}
	out := []string{"Categories"}
	if m.state.mode == InstMenuModeInstruments {
		if m.state.favoritesView {
			out = append(out, "Favorites")
		} else if m.state.activeCat != "" {
			out = append(out, m.state.activeCat)
		}
	}
	return out
}

// Page returns the 1-based current page index, derived from the
// underlying scroll offset and visible row count. Empty list returns 1.
func (m *InstrumentMenuComponent) Page() int {
	m.ensureScroll()
	if m.scroll.VS.Visible <= 0 || m.scroll.VS.Total == 0 {
		return 1
	}
	return (m.scroll.VS.First / m.scroll.VS.Visible) + 1
}

// PageCount returns the total page count (1 when fewer items than the
// visible window).
func (m *InstrumentMenuComponent) PageCount() int {
	m.ensureScroll()
	total := m.scroll.VS.Total
	vis := m.scroll.VS.Visible
	if total == 0 || vis == 0 {
		return 0
	}
	if total <= vis {
		return 1
	}
	return (total + vis - 1) / vis
}

// PageSize returns the number of rows visible per page (== scroll.VS.Visible).
func (m *InstrumentMenuComponent) PageSize() int {
	m.ensureScroll()
	if m.scroll.VS.Visible < 1 {
		return 1
	}
	return m.scroll.VS.Visible
}

// drawBottomSheetHandle paints a small rounded pill at the top-center of the
// bottom sheet to telegraph it as a draggable surface. Mirrors the recipe in
// drumview_context_menu.go.
func (m *InstrumentMenuComponent) drawBottomSheetHandle(dst *ebiten.Image) {
	handleW := 36
	handleH := 4
	hx := m.fullRect.Min.X + m.fullRect.Dx()/2 - handleW/2
	hy := m.fullRect.Min.Y + 8
	drawRoundedRect(dst, image.Rect(hx, hy, hx+handleW, hy+handleH),
		WithAlpha(genColorBorder, genAlphaScrollbarThumb), handleH/2, true)
}

// drawSearchRow renders the search input plus a muted "Search" hint when the
// box is empty and unfocused. Hint placement matches TextInput.Draw's text
// origin (4 px x-inset, vertically centered).
func (m *InstrumentMenuComponent) drawSearchRow(dst *ebiten.Image) {
	m.searchBox.Draw(dst)
	if m.state.searchText == "" && !m.searchBox.Focused() {
		r := m.searchBox.Rect
		th := TextHeight()
		ty := r.Min.Y + (r.Dy()-th)/2
		DrawTextColorAt(dst, "Search", r.Min.X+4, ty, colTextSecondary)
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
