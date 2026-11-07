package ui

import (
	"image"
	"image/color"
	"slices"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Shared by the layout (registers the field so a tap raises the phone keyboard) and Update
// (reads the typed value back).
const instSearchMobileInputID = "inst-search"

// InstrumentOption represents an instrument available for selection.
type InstrumentOption struct {
	ID       instrument.ID
	Label    string
	Category audio.Category
	// The live row color, so the swatch matches the grid node; nil when no row plays it.
	Color color.Color
}

type menuCatMeta struct {
	favRow bool
	count  int
	active bool // the current category / Favorites view
}

// InstrumentMenuProps contains the external state passed to the instrument menu component.
type InstrumentMenuProps struct {
	// nil in isolated component tests.
	TextFields *textFieldRegistry
	// AnchorRect is the label button that opens the menu (used for positioning).
	AnchorRect image.Rectangle
	// VertBounds is the vertical bounds for constraining the menu.
	VertBounds image.Rectangle
	// RowIndex is the index of the row being edited.
	RowIndex int
	// CurrentInstrument is the currently selected instrument ID.
	CurrentInstrument instrument.ID
	// Categories is the list of available categories.
	Categories []audio.Category
	// Instruments is the list of all available instruments.
	Instruments []InstrumentOption
	// RowHeight is the height of each row in the menu.
	RowHeight int
	// LabelWidth is the width of the label column.
	LabelWidth int
	// ControlsWidth is the width of the controls column.
	ControlsWidth   int
	ForceCategories bool

	// OnSelect is called when an instrument is selected.
	OnSelect func(instID instrument.ID)
	// A selection can be deferred (a settings question opens first and may be cancelled), so read
	// what the row plays rather than assume a click applied it.
	RowInstrument func() instrument.ID
	// OnClose is called when the menu should close.
	OnClose func()
	// Fires after every mutation of observable state (mode, category, scroll, buttons), so the host
	// mirrors it through one notifier.
	OnStateChanged func()

	// Writes go through OnFavorite, or Set when it is nil.
	Favorites  FavoritesStore
	OnFavorite func(instID instrument.ID, fav bool)

	// Tier 0 (above ★ favorites) in every list; read-only here.
	ProjectPins map[instrument.ID]struct{}

	// Off by default so tests that index Categories[0] keep their layout; production opts in.
	ShowFavoritesCategory bool

	// Zero fields fall back to the design-system defaults.
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
	// The literal chevrons are forbidden glyphs.
	breadcrumbSeparatorIcon = IconChevronRight
)

// InstrumentMenuState contains the internal state for the instrument menu component.
type InstrumentMenuState struct {
	open      bool
	hold      bool // Capture flag after menu closes
	mode      InstMenuMode
	activeCat audio.Category
	// Shows only ★ ids across every category; set by the virtual Favorites row, cleared by Back.
	favoritesView bool
	userScrolled  bool
	// Updated on every in-menu pick, so the active stripe follows an audition while the menu is open.
	selectedID    instrument.ID
	lastAdded     instrument.ID
	cameFromCats  bool
	searchText    string
	filteredInsts []instrument.ID
	// Lets StableTierBreaker break score ties by tier; empty without a query.
	fuzzyScores      map[instrument.ID]int
	categoryByID     map[instrument.ID]audio.Category
	displayLabelByID map[instrument.ID]string
	// Rebuilt on every SetProps so a node recolor flows through.
	colorByID        map[instrument.ID]color.Color
	searchHighlights map[instrument.ID][]int // Per-ID highlight positions from fuzzy search.
}

// InstrumentMenuComponent is a self-contained dropdown menu for selecting instruments.
type InstrumentMenuComponent struct {
	overlayBase
	props InstrumentMenuProps
	state InstrumentMenuState

	menuScroll   *MenuScroll     // shared scroll component (owns the ScrollBehavior)
	scroll       *ScrollBehavior // == menuScroll.ScrollBehavior(); kept for the existing call sites
	categoryBtns []*Button
	categoryMeta []menuCatMeta
	instBtns     []*Button
	searchBox    *TextInput
	backBtn      *Button
	closeBtn     *Button
	fullRect     image.Rectangle
	searchRect   image.Rectangle

	breadcrumbRects []image.Rectangle
	breadcrumbRect  image.Rectangle

	nav     *MenuKeyNav
	keyEdge map[ebiten.Key]bool

	instIDs []instrument.ID

	instTrails []string

	// Row-indexed with instBtns/instIDs, so a star can never draw for a row that is not on screen:
	// every append goes through addInstRow and rebuildMenu truncates all four slices together.
	favRects []image.Rectangle

	// Deferred tap: position stored on touch begin, fired on touch end if no scroll committed.
	deferredTap DeferredTap

	style MenuStyle
}

// NewInstrumentMenuComponent creates a new instrument menu component.
func NewInstrumentMenuComponent() *InstrumentMenuComponent {
	return &InstrumentMenuComponent{
		overlayBase: newOverlayBase(),
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
	// Not gated by needsRebuild: a recolor keeps the id list but changes the color.
	m.rebuildColorMap()
	if needsRebuild {
		m.rebuildMaps()
	}
}

func (m *InstrumentMenuComponent) rebuildColorMap() {
	m.state.colorByID = make(map[instrument.ID]color.Color, len(m.props.Instruments))
	for _, inst := range m.props.Instruments {
		if inst.Color != nil {
			m.state.colorByID[inst.ID] = inst.Color
		}
	}
}

// The one seam for swatches, stripes, stars and accent, so every surface matches the grid node.
func (m *InstrumentMenuComponent) instColorFor(id instrument.ID) color.Color {
	if m.state.colorByID != nil {
		if c, ok := m.state.colorByID[id]; ok && c != nil {
			return c
		}
	}
	return instColor(id)
}

func (m *InstrumentMenuComponent) Style() MenuStyle { return m.style }

// Props returns the current props.
func (m *InstrumentMenuComponent) Props() InstrumentMenuProps { return m.props }

// rebuildMaps rebuilds the internal lookup maps from props.
func (m *InstrumentMenuComponent) rebuildMaps() {
	m.state.categoryByID = make(map[instrument.ID]audio.Category)
	m.state.displayLabelByID = make(map[instrument.ID]string)
	for _, inst := range m.props.Instruments {
		m.state.categoryByID[inst.ID] = inst.Category
		m.state.displayLabelByID[inst.ID] = inst.Label
	}
}

// ensureScroll lazily initializes the scroll behavior.
func (m *InstrumentMenuComponent) ensureScroll() {
	if m.menuScroll == nil {
		m.menuScroll = NewMenuScroll(dropdownScrollbarStyle(), 24)
		m.scroll = m.menuScroll.ScrollBehavior()
	}
	if m.scroll == nil {
		m.scroll = m.menuScroll.ScrollBehavior()
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
	m.state.selectedID = m.props.CurrentInstrument

	// Default to instruments mode unless ForceCategories is set and categories exist
	if m.props.ForceCategories && len(m.props.Categories) > 0 {
		m.state.mode = InstMenuModeCategories
	} else {
		m.state.mode = InstMenuModeInstruments
	}

	// Default active category to current instrument's category
	m.state.activeCat = audio.CatNone
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

// While open, the search box consumes typed and caret keys.
func (m *InstrumentMenuComponent) ClaimsKeyboard() bool {
	return m != nil && m.IsOpen()
}

// SetLastAdded sets the instrument ID to auto-bias to when opening.
func (m *InstrumentMenuComponent) SetLastAdded(id instrument.ID) {
	m.state.lastAdded = id
}

// Cheap when closed.
func (m *InstrumentMenuComponent) Refresh() {
	if m == nil {
		return
	}
	m.rebuildMenu()
}

// false on an empty query, so the universal Esc handler closes the menu.
func (m *InstrumentMenuComponent) HandleEscape() bool {
	if strings.TrimSpace(m.state.searchText) == "" {
		return false
	}
	m.clearSearch()
	return true
}

// Portal input handlers only run on press edges, so hover needs its own per-frame pass.
func (m *InstrumentMenuComponent) HoverAt(mx, my int) {
	for _, b := range m.instBtns {
		b.UpdateHover(mx, my)
	}
	for _, b := range m.categoryBtns {
		b.UpdateHover(mx, my)
	}
	m.backBtn.UpdateHover(mx, my)
	m.closeBtn.UpdateHover(mx, my)
}

func (m *InstrumentMenuComponent) clearSearch() {
	m.state.searchText = ""
	if m.searchBox != nil {
		m.searchBox.SetText("")
	}
	m.rebuildMenu() // reuse the existing rebuild so the filtered list refreshes
}

func (m *InstrumentMenuComponent) searchClearHit(pt image.Point) bool {
	return m.searchBox != nil && m.searchBox.Value() != "" &&
		menuSearchAdornmentsActive(m.searchBox) &&
		pt.In(menuSearchClearRect(m.searchBox))
}

// InstBtns returns the current instrument buttons (for test access and legacy sync).
func (m *InstrumentMenuComponent) InstBtns() []*Button {
	return m.instBtns
}

// VisibleInstIDs returns the instrument IDs for the currently visible instrument buttons.
func (m *InstrumentMenuComponent) VisibleInstIDs() []instrument.ID {
	if m == nil {
		return nil
	}
	first, _, _ := m.ScrollState()
	var ids []instrument.ID
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

// Empty outside global search rather than absent, so it stays index-aligned with InstBtns.
func (m *InstrumentMenuComponent) GlobalResultTrails() []string {
	return m.instTrails
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

// The ONLY place instBtns grows, so the four row-indexed slices cannot drift apart.
func (m *InstrumentMenuComponent) addInstRow(btn *Button, id instrument.ID, trail string, star image.Rectangle) {
	m.instBtns = append(m.instBtns, btn)
	m.instIDs = append(m.instIDs, id)
	m.instTrails = append(m.instTrails, trail)
	m.favRects = append(m.favRects, star)
}

// rebuildMenu rebuilds the menu buttons and layout.
func (m *InstrumentMenuComponent) rebuildMenu() {
	m.ensureScroll()
	m.categoryBtns = m.categoryBtns[:0]
	m.categoryMeta = m.categoryMeta[:0]
	// Before any early return below, or an empty view keeps the previous listing's stars.
	m.instBtns = m.instBtns[:0]
	m.instIDs = m.instIDs[:0]
	m.instTrails = m.instTrails[:0]
	m.favRects = m.favRects[:0]

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

// Needs both the opt-in and a Favorites store.
func (m *InstrumentMenuComponent) favoritesCategoryEnabled() bool {
	return m != nil && m.props.ShowFavoritesCategory && m.props.Favorites != nil
}

// Project pins are not folded in: a pin is not necessarily a favorite.
func (m *InstrumentMenuComponent) favoritesIDs() map[instrument.ID]struct{} {
	out := map[instrument.ID]struct{}{}
	if m == nil || m.props.Favorites == nil {
		return out
	}
	for _, k := range m.props.Favorites.Keys() {
		out[k] = struct{}{}
	}
	return out
}

func (m *InstrumentMenuComponent) pinSource() PinSource {
	src := PinSource{ProjectPins: m.props.ProjectPins, UserStars: m.favoritesIDs()}
	return src
}

// Synced only when the box diverges from the state: after typing they match, and SetText would
// jump the caret to the end.
func (m *InstrumentMenuComponent) ensureSearchBoxSynced() {
	m.searchBox = ensureMenuSearchBox(m.searchBox, m.searchRect, m.props.TextFields,
		TextFieldSpec{ID: instSearchMobileInputID, Owner: "inst-menu"})
	if m.searchBox.Value() != m.state.searchText {
		m.searchBox.SetText(m.state.searchText)
	}
}

func (m *InstrumentMenuComponent) globalQueryActive() bool {
	return m.state.mode == InstMenuModeCategories &&
		strings.TrimSpace(m.state.searchText) != ""
}

func (m *InstrumentMenuComponent) buildCategoriesMode(base, vertBounds image.Rectangle, rowH int, openUp bool) {
	if m.globalQueryActive() {
		m.buildGlobalSearchMode(base, vertBounds, rowH, openUp)
		return
	}
	categories := m.props.Categories
	favPrefix := m.favoritesCategoryEnabled()
	// Instruments-per-category counts feed the trailing count captions.
	catCounts := make(map[audio.Category]int, len(categories))
	for _, opt := range m.props.Instruments {
		catCounts[opt.Category]++
	}
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

	// The title band also hosts the close ×, so it never sits on the first row.
	headerH := m.style.BreadcrumbStripH
	if headerH <= 0 || headerH > rowH {
		headerH = rowH
	}

	maxVisHost := (vertBounds.Dy() - headerH - rowH) / rowH
	if maxVisHost < 1 {
		maxVisHost = 1
	}
	if vis > maxVisHost {
		vis = maxVisHost
	}

	totalH := headerH + rowH + vis*rowH
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

	// Global search row directly below the title band.
	searchY := startY + headerH
	m.searchRect = image.Rect(base.Min.X, searchY, base.Max.X, searchY+rowH)
	m.ensureSearchBoxSynced()
	listStartY := searchY + rowH

	m.scroll.VS.Total = catCount
	m.scroll.VS.Visible = vis
	m.scroll.VS.View = image.Rect(base.Min.X, listStartY, base.Max.X, listStartY+vis*rowH)
	m.fullRect = image.Rect(base.Min.X, startY, base.Max.X, listStartY+vis*rowH)

	closeBtnRect := closeButtonRect(m.fullRect, SpaceXS)
	bcRight := base.Max.X
	if !closeBtnRect.Empty() && startY < closeBtnRect.Max.Y && startY+headerH > closeBtnRect.Min.Y {
		if r := closeBtnRect.Min.X - SpaceMD; r < bcRight {
			bcRight = r
		}
	}
	m.computeBreadcrumbStrip(image.Rect(base.Min.X, startY, bcRight, startY+headerH))
	m.backBtn = nil

	// The virtual Favorites row is index 0 when favPrefix is set.
	if !m.state.userScrolled && m.state.activeCat != audio.CatNone {
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

	// With favPrefix, row 0 is the virtual Favorites entry (sets favoritesView); every other
	// category clears it.
	start := m.scroll.VS.First
	for i := 0; i < vis && start+i < catCount; i++ {
		absIdx := start + i
		r := image.Rect(base.Min.X, listStartY+i*rowH, base.Max.X, listStartY+(i+1)*rowH)

		if favPrefix && absIdx == 0 {
			favCount := len(m.favoritesIDs())
			label := i18n.T(i18n.KeyMenuFavorites)
			btn := NewButton(label, DropdownStyle, func() {
				m.state.favoritesView = true
				m.state.activeCat = audio.CatNone
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
			m.categoryMeta = append(m.categoryMeta, menuCatMeta{favRow: true, count: favCount, active: m.state.favoritesView})
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
		btn := NewButton(btnCat.Label(), DropdownStyle, func() {
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
		m.categoryMeta = append(m.categoryMeta, menuCatMeta{count: catCounts[btnCat], active: !m.state.favoritesView && btnCat == m.state.activeCat})
	}

	m.buildCloseBtn()
	m.SetBounds(m.fullRect)
}

type globalSearchRow struct {
	isCategory bool
	isFavCat   bool           // the virtual "Favorites" category row
	inst       instrument.ID  // instrument rows
	category   audio.Category // category rows (CatNone for Favorites)
	label      string
	trail      string // instrument rows: the category the instrument lives in
	score      int
	highlights []int
	tier       int
}

// Pinned and ★ instruments first, then matching categories, then the rest; each instrument row
// names its category so the user sees where a hit lives.
func (m *InstrumentMenuComponent) buildGlobalSearchMode(base, vertBounds image.Rectangle, rowH int, openUp bool) {
	query := strings.TrimSpace(m.state.searchText)

	// Instrument matches across the whole catalog.
	items := make([]MenuSearchItem, 0, len(m.props.Instruments))
	for _, inst := range m.props.Instruments {
		label := m.state.displayLabelByID[inst.ID]
		if label == "" {
			label = string(inst.ID)
		}
		items = append(items, MenuSearchItem{Key: string(inst.ID), Label: label})
	}
	var searcher MenuSearcher
	res := searcher.Search(query, items)
	src := m.pinSource()

	var favRows, catRows, instRows []globalSearchRow
	for _, r := range res {
		id := instrument.ID(r.Key)
		row := globalSearchRow{
			inst: id, label: r.Label, trail: m.state.categoryByID[id].Label(),
			score: r.Score, highlights: r.Highlights, tier: src.Tier(id),
		}
		if row.tier <= 1 {
			favRows = append(favRows, row)
		} else {
			instRows = append(instRows, row)
		}
	}
	if m.favoritesCategoryEnabled() {
		favLabel := i18n.T(i18n.KeyMenuFavorites)
		if fr := FuzzyMatch(query, favLabel); fr.Score >= 0 {
			catRows = append(catRows, globalSearchRow{
				isCategory: true, isFavCat: true, label: favLabel,
				score: fr.Score, highlights: fr.Positions,
			})
		}
	}
	for _, cat := range m.props.Categories {
		if fr := FuzzyMatch(query, cat.Label()); fr.Score >= 0 {
			catRows = append(catRows, globalSearchRow{
				isCategory: true, category: cat, label: cat.Label(),
				score: fr.Score, highlights: fr.Positions,
			})
		}
	}
	sortRows := func(rows []globalSearchRow, tierFirst bool) {
		slices.SortStableFunc(rows, func(a, b globalSearchRow) int {
			if tierFirst && a.tier != b.tier {
				return a.tier - b.tier
			}
			if a.score != b.score {
				return b.score - a.score
			}
			return strings.Compare(strings.ToLower(a.label), strings.ToLower(b.label))
		})
	}
	sortRows(favRows, true)
	sortRows(catRows, false)
	sortRows(instRows, false)
	rows := make([]globalSearchRow, 0, len(favRows)+len(catRows)+len(instRows))
	rows = append(rows, favRows...)
	rows = append(rows, catRows...)
	rows = append(rows, instRows...)

	headerH := m.style.BreadcrumbStripH
	if headerH <= 0 || headerH > rowH {
		headerH = rowH
	}
	vis := instMenuMaxVisibleRowsComp
	if Profile().UseBottomSheet {
		mobileVis := (vertBounds.Dy() - rowH*2) / rowH
		if mobileVis > vis {
			vis = mobileVis
		}
	}
	maxVisHost := (vertBounds.Dy() - headerH - rowH) / rowH
	if maxVisHost < 1 {
		maxVisHost = 1
	}
	if vis > maxVisHost {
		vis = maxVisHost
	}
	if vis > len(rows) {
		vis = len(rows)
	}
	if vis < 1 {
		vis = 1
	}

	totalH := headerH + rowH + vis*rowH
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
	searchY := startY + headerH
	m.searchRect = image.Rect(base.Min.X, searchY, base.Max.X, searchY+rowH)
	m.ensureSearchBoxSynced()
	listStartY := searchY + rowH

	m.scroll.VS.Total = len(rows)
	m.scroll.VS.Visible = vis
	m.scroll.VS.View = image.Rect(base.Min.X, listStartY, base.Max.X, listStartY+vis*rowH)
	m.fullRect = image.Rect(base.Min.X, startY, base.Max.X, listStartY+vis*rowH)
	m.scroll.VS.Clamp()

	closeBtnRect := closeButtonRect(m.fullRect, SpaceXS)
	bcRight := base.Max.X
	if !closeBtnRect.Empty() && startY < closeBtnRect.Max.Y && startY+headerH > closeBtnRect.Min.Y {
		if r := closeBtnRect.Min.X - SpaceMD; r < bcRight {
			bcRight = r
		}
	}
	m.computeBreadcrumbStrip(image.Rect(base.Min.X, startY, bcRight, startY+headerH))
	m.backBtn = nil

	// A category row drills in and clears the query, so the category opens unfiltered.
	start := m.scroll.VS.First
	for i := 0; i < vis && start+i < len(rows); i++ {
		row := rows[start+i]
		r := image.Rect(base.Min.X, listStartY+i*rowH, base.Max.X, listStartY+(i+1)*rowH)
		if row.isCategory {
			isFav := row.isFavCat
			cat := row.category
			btn := NewButton(row.label, DropdownStyle, func() {
				m.state.searchText = ""
				if m.searchBox != nil {
					m.searchBox.SetText("")
				}
				m.state.favoritesView = isFav
				m.state.activeCat = cat
				m.state.mode = InstMenuModeInstruments
				m.scroll.VS.First = 0
				m.state.cameFromCats = true
				m.state.userScrolled = false
				m.rebuildMenu()
			})
			btn.Highlights = row.highlights
			btn.SetRect(insetRect(r, SpaceXS))
			m.categoryBtns = append(m.categoryBtns, btn)
			continue
		}
		optID := row.inst
		btn := NewButton(row.label, DropdownStyle, func() {
			// Same preview/confirm semantics as an instruments-mode row.
			m.applyInstrument(optID)
		})
		btn.Highlights = row.highlights
		btn.SetRect(insetRect(r, SpaceXS))
		// No star: these rows share the list with category rows and have no gutter.
		m.addInstRow(btn, optID, row.trail, image.Rectangle{})
	}

	m.buildCloseBtn()
	m.SetBounds(m.fullRect)
}

// buildInstrumentsMode builds the menu in instruments mode.
func (m *InstrumentMenuComponent) buildInstrumentsMode(base, vertBounds image.Rectangle, rowH int, openUp bool) {
	// favoritesView skips the category filter, so the whole ★ set shows.
	var favIDs map[instrument.ID]struct{}
	if m.state.favoritesView {
		favIDs = m.favoritesIDs()
	}
	var catItems []MenuSearchItem
	for _, inst := range m.props.Instruments {
		if m.state.favoritesView {
			if _, ok := favIDs[inst.ID]; !ok {
				continue
			}
		} else if m.state.activeCat != audio.CatNone && m.state.categoryByID[inst.ID] != m.state.activeCat {
			continue
		}
		label := m.state.displayLabelByID[inst.ID]
		if label == "" {
			label = string(inst.ID)
		}
		catItems = append(catItems, MenuSearchItem{Key: string(inst.ID), Label: label})
	}
	var searcher MenuSearcher
	searchResults := searcher.Search(m.state.searchText, catItems)
	m.state.filteredInsts = m.state.filteredInsts[:0]
	m.state.searchHighlights = make(map[instrument.ID][]int, len(searchResults))
	queryActive := strings.TrimSpace(m.state.searchText) != ""
	if queryActive {
		m.state.fuzzyScores = make(map[instrument.ID]int, len(searchResults))
	} else {
		m.state.fuzzyScores = nil
	}
	for _, r := range searchResults {
		id := instrument.ID(r.Key)
		m.state.filteredInsts = append(m.state.filteredInsts, id)
		if len(r.Highlights) > 0 {
			m.state.searchHighlights[id] = r.Highlights
		}
		if queryActive {
			m.state.fuzzyScores[id] = r.Score
		}
	}

	// Everything in the Favorites view is ★, so sort by label only.
	if !m.state.favoritesView {
		labelFor := func(id instrument.ID) string {
			lbl := m.state.displayLabelByID[id]
			if lbl == "" {
				lbl = string(id)
			}
			return lbl
		}
		src := m.pinSource()
		if queryActive {
			// Favorites always on top: tier is the primary key, score orders within a tier.
			src.SortByTierScore(m.state.filteredInsts,
				func(id instrument.ID) int { return m.state.fuzzyScores[id] },
				labelFor)
		} else {
			src.SortByTierAlpha(m.state.filteredInsts, labelFor)
		}
	} else if !queryActive {
		// Favorites view, no query → alphabetical by label.
		labelFor := func(id instrument.ID) string {
			lbl := m.state.displayLabelByID[id]
			if lbl == "" {
				lbl = string(id)
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

	m.ensureSearchBoxSynced()

	listStartY := searchY + rowH

	m.scroll.VS.Total = len(m.state.filteredInsts)
	m.scroll.VS.Visible = vis

	if len(m.state.filteredInsts) == 0 {
		// Back renders even with no results, or an empty view strands the user.
		extraTop := rowH // search row
		if showBack {
			extraTop += rowH // back row
		}
		emptyView := image.Rect(base.Min.X, listStartY, base.Max.X, listStartY+rowH)
		m.scroll.VS.View = emptyView
		m.fullRect = image.Rect(base.Min.X, startY, base.Max.X, startY+extraTop+rowH)
		emptyText := i18n.T(i18n.KeyMenuNoMatches)
		if m.state.favoritesView && strings.TrimSpace(m.state.searchText) == "" {
			emptyText = i18n.T(i18n.KeyMenuNoFavorites) + " — " + i18n.T(i18n.KeyMenuNoFavoritesHint)
		}
		if showBack {
			bcH := m.style.BreadcrumbStripH
			if bcH <= 0 || bcH > rowH {
				bcH = rowH
			}
			backRect := image.Rect(base.Min.X, startY, base.Max.X, startY+bcH)
			m.backBtn = NewButton(i18n.T(i18n.KeyMenuBack), DropdownStyle, func() {
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
		// The placeholder is not an instrument: no id, no trail, no star.
		m.addInstRow(placeholder, "", "", image.Rectangle{})
		m.SetBounds(m.fullRect)
		return
	}

	m.scroll.VS.View = image.Rect(base.Min.X, listStartY, base.Max.X, listStartY+vis*rowH)
	m.fullRect = image.Rect(base.Min.X, startY, base.Max.X, listStartY+vis*rowH)

	// A pure translation in both axes, never ClampRectInto: shrinking the fixed-size rect would
	// detach the rows from the panel.
	if !vertBounds.Empty() {
		delta := image.Point{}
		if m.fullRect.Min.X < vertBounds.Min.X {
			delta.X = vertBounds.Min.X - m.fullRect.Min.X
		} else if m.fullRect.Max.X > vertBounds.Max.X {
			delta.X = vertBounds.Max.X - m.fullRect.Max.X
		}
		if m.fullRect.Min.Y < vertBounds.Min.Y {
			delta.Y = vertBounds.Min.Y - m.fullRect.Min.Y
		} else if m.fullRect.Max.Y > vertBounds.Max.Y {
			delta.Y = vertBounds.Max.Y - m.fullRect.Max.Y
		}
		if delta.X != 0 || delta.Y != 0 {
			m.fullRect = m.fullRect.Add(delta)
			m.scroll.VS.View = m.scroll.VS.View.Add(delta)
			m.searchRect = m.searchRect.Add(delta)
			startY += delta.Y
			listStartY += delta.Y
			base = base.Add(delta)
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

	// Computed first so the breadcrumb and the star column can clear it.
	closeBtnRect := closeButtonRect(m.fullRect, SpaceXS)

	hasScroll := m.scroll.HasScroll()
	buttonMaxX := base.Max.X
	if hasScroll {
		buttonMaxX -= dropdownScrollbarWidth()
		if buttonMaxX <= base.Min.X {
			buttonMaxX = base.Min.X + 1
		}
	}
	favColWidth := m.style.StarColW
	starsActive := m.props.Favorites != nil
	if starsActive {
		buttonMaxX -= favColWidth
		if buttonMaxX <= base.Min.X {
			buttonMaxX = base.Min.X + 1
		}
	}

	// The back button stays, invisible behind the breadcrumb, for callers of BackBtn().
	if showBack {
		bcH := m.style.BreadcrumbStripH
		if bcH <= 0 || bcH > rowH {
			bcH = rowH
		}
		bcRight := base.Max.X
		if !closeBtnRect.Empty() && startY < closeBtnRect.Max.Y && startY+bcH > closeBtnRect.Min.Y {
			if r := closeBtnRect.Min.X - SpaceMD; r < bcRight {
				bcRight = r
			}
		}
		backRect := image.Rect(base.Min.X, startY, bcRight, startY+bcH)
		m.backBtn = NewButton(i18n.T(i18n.KeyMenuBack), DropdownStyle, func() {
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
			label = string(id)
		}
		btn := NewButton(label, DropdownStyle, func() {
			// Every selection path goes through applyInstrument, so pointer and keyboard cannot drift.
			m.applyInstrument(optID)
		})
		btn.Highlights = m.state.searchHighlights[optID]
		btn.SetRect(insetRect(r, SpaceXS))
		var starRect image.Rectangle
		if starsActive {
			starRect = image.Rect(buttonMaxX, r.Min.Y, buttonMaxX+favColWidth, r.Max.Y)
			// A star in the close button's band slides left so the two never collide.
			if !closeBtnRect.Empty() && starRect.Min.Y < closeBtnRect.Max.Y && starRect.Max.Y > closeBtnRect.Min.Y {
				if shift := starRect.Max.X - (closeBtnRect.Min.X - SpaceMD); shift > 0 {
					starRect = starRect.Sub(image.Pt(shift, 0))
				}
			}
		}
		m.addInstRow(btn, optID, "", starRect)
	}

	m.buildCloseBtn()
	m.SetBounds(m.fullRect)
}

// Desktop only (nil on mobile, where an outside tap closes); every site guards nil.
func (m *InstrumentMenuComponent) buildCloseBtn() {
	if m.fullRect.Empty() {
		m.closeBtn = nil
		return
	}
	m.closeBtn = newPopupCloseButton(func() { m.Close() })
	if m.closeBtn != nil {
		m.closeBtn.SetRect(closeButtonRect(m.fullRect, SpaceXS))
	}
}

// CloseBtn returns the close button (for testing).
func (m *InstrumentMenuComponent) CloseBtn() *Button { return m.closeBtn }

// Fires OnClick directly: the touch already ended and is known to be a tap.
func (m *InstrumentMenuComponent) fireTapAt(x, y int) {
	pt := image.Pt(x, y)
	// Close button has highest z-order — check first.
	if m.closeBtn != nil && pt.In(m.closeBtn.Rect()) && m.closeBtn.OnClick != nil {
		m.closeBtn.OnClick()
		return
	}
	// Search clear-× (drawn adornment; inactive while the native <input> overlays).
	if m.searchClearHit(pt) {
		m.clearSearch()
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
	if m.state.mode == InstMenuModeInstruments || m.globalQueryActive() {
		// The star wins over the row, so touching it toggles instead of selecting.
		if i := m.starRowAt(pt); i >= 0 {
			m.toggleFavoriteAt(i)
			return
		}
		// Through the same press core as the mouse (PressFromTree), so a touched row eases down and stays
		// pressed exactly like a clicked one.
		for _, btn := range m.instBtns {
			btn.ResetPress()
		}
		for _, btn := range m.instBtns {
			if pt.In(btn.Rect()) {
				btn.PressFromTree(true)
				return
			}
		}
	}
}

// Zero star rects never hit, and cannot swallow the origin.
func (m *InstrumentMenuComponent) starRowAt(pt image.Point) int {
	for i, rect := range m.favRects {
		if rect.Empty() {
			continue
		}
		if pt.In(rect) {
			return i
		}
	}
	return -1
}

// The id comes from the row's own instIDs entry, so a star only toggles the instrument beside it.
func (m *InstrumentMenuComponent) toggleFavoriteAt(i int) {
	if m.props.Favorites == nil {
		return
	}
	if i < 0 || i >= len(m.instIDs) || i >= len(m.favRects) {
		return
	}
	if m.favRects[i].Empty() {
		return
	}
	id := m.instIDs[i]
	if id == "" {
		return
	}
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

	// The deferred-tap machine is a mobile affordance; on desktop it made clicks fire on release
	// and dropped any drift past the dead zone as a scroll.
	mobile := Profile().IsMobile()

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
	if mobile && m.scroll.ScrollingCommitted() {
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

	if mobile && m.scroll.TouchActive() {
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

	// Before touch begin: the thumb sits inside VS.View and would read as a touch scroll.
	if m.scroll.HasScroll() {
		thumbRect := m.scroll.ThumbRect()
		if pressed && pt.In(thumbRect) {
			m.scroll.HandleDragStart(y)
			return InputCaptured
		}
	}

	// Suppress every button on contact; the deferred tap fires on release if no scroll committed.
	// The close × defers too, so mobile taps fire on release everywhere.
	inDeferrable := pt.In(m.scroll.VS.View) ||
		(m.closeBtn != nil && pt.In(m.closeBtn.Rect()))
	if mobile && pressed && inDeferrable && !m.scroll.TouchActive() && !m.scroll.Dragging() {
		if m.deferredTap.Begin(x, y) {
			m.scroll.HandleTouchBegin(x, y)
			return InputCaptured
		}
	}

	// Close button (highest z-order, outside scroll area)
	if m.closeBtn != nil && m.closeBtn.HandleInputResult(x, y, pressed) != InputIgnored {
		return InputConsumed
	}

	// Consumed so clicks cannot fall through; typing is handled in Update.
	if m.searchBox != nil && !m.searchRect.Empty() && pt.In(m.searchRect) {
		if pressed && m.searchClearHit(pt) {
			m.clearSearch()
		}
		return InputConsumed
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

	// Before the rows, so a click on the star never selects the instrument.
	if pressed && m.state.mode == InstMenuModeInstruments && !m.scroll.Dragging() && !touchSuppressButtons {
		if i := m.starRowAt(pt); i >= 0 {
			m.toggleFavoriteAt(i)
			return InputConsumed
		}
	}

	// The menu stays open after a pick and returns InputConsumed, so no release ever reaches these
	// buttons; clear every row's latched press first or a re-click on the selected row is swallowed.
	if (m.state.mode == InstMenuModeInstruments || m.globalQueryActive()) &&
		!m.scroll.Dragging() && !touchSuppressButtons {
		if pressed {
			for _, btn := range m.instBtns {
				btn.ResetPress()
			}
		}
		for _, btn := range m.instBtns {
			if btn.HandleInputResult(x, y, pressed) != InputIgnored {
				return InputConsumed
			}
		}
	}

	// Not while clicks are suppressed: geometry changes between modes.
	if pressed && !suppressClicksUntilRelease && !pt.In(m.fullRect) && !pt.In(m.props.AnchorRect) {
		m.Close()
		return InputConsumed
	}

	// If within bounds, consume to prevent click-through
	if pt.In(m.fullRect) {
		return InputConsumed
	}

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

func (m *InstrumentMenuComponent) Update() {
	if !m.state.open {
		return
	}
	m.ensureScroll()

	// Every frame: HandleInput only fires on pointer events, so typed characters would be lost.
	if m.searchBox != nil {
		// On mobile the text is typed into a native <input>; poll its value so the list filters live.
		if Profile().IsMobile() {
			if v, ok := mobileInputGetValue(instSearchMobileInputID); ok && v != m.state.searchText {
				m.state.searchText = v
				m.searchBox.SetText(v)
				m.rebuildMenu()
			}
		}
		m.searchBox.Update()
		newText := m.searchBox.Value()
		if newText != m.state.searchText {
			m.state.searchText = newText
			m.rebuildMenu()
			emitSearchChanged("inst-menu", newText)
		}
	}

	if !Profile().IsMobile() {
		// reveal() scrolls, but rows only regenerate in rebuildMenu; without a rebuild the list stays
		// frozen while the highlight walks off the page.
		beforeFirst := m.scroll.VS.First
		m.handleKeyboardNav()
		if m.scroll.VS.First != beforeFirst {
			// A keyboard scroll is a user scroll, or rebuildMenu re-biases the list back onto the current
			// instrument.
			m.state.userScrolled = true
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

// The shared MenuKeyNav plus this menu's two extra bindings. Up/Down walk the filtered results
// even while typing; the caret keeps Left/Right.
func (m *InstrumentMenuComponent) handleKeyboardNav() {
	if m.state.mode == InstMenuModeInstruments {
		m.keyNav().Update(m)
	}
	m.handleKeyEdge(ebiten.KeySlash, m.kbFocusSearch)
}

// keyNav lazily creates this menu's keyboard cursor.
func (m *InstrumentMenuComponent) keyNav() *MenuKeyNav {
	if m.nav == nil {
		m.nav = NewMenuKeyNav()
	}
	return m.nav
}

// Rows are the FILTERED list, so MenuKeyNav is Reset whenever it changes shape.

// MenuItemCount implements MenuKeyNavHost.
func (m *InstrumentMenuComponent) MenuItemCount() int { return len(m.state.filteredInsts) }

// MenuItemSelectable implements MenuKeyNavHost: every filtered row is a choice.
func (m *InstrumentMenuComponent) MenuItemSelectable(i int) bool {
	return i >= 0 && i < len(m.state.filteredInsts)
}

// Identical to clicking the row: Enter previews, a second Enter confirms.
func (m *InstrumentMenuComponent) MenuActivate(i int) {
	if i < 0 || i >= len(m.state.filteredInsts) {
		return
	}
	m.applyInstrument(m.state.filteredInsts[i])
}

// The one mutation for every selection path. A new instrument applies and keeps the menu open
// for auditioning; the applied one confirms and closes. Positional, not timed.
func (m *InstrumentMenuComponent) applyInstrument(id instrument.ID) {
	if id == "" {
		return
	}
	if m.appliedID() == id {
		// Already applied — this is the confirm.
		m.Close()
		return
	}
	// props.CurrentInstrument is a snapshot from open and never refreshed.
	m.state.selectedID = id
	if m.props.OnSelect != nil {
		m.props.OnSelect(id)
	}
	m.state.selectedID = m.appliedID() // the selection may be waiting on a question
	// So after a click the next Enter confirms that row.
	m.syncCursorToSelected()
}

func (m *InstrumentMenuComponent) appliedID() instrument.ID {
	if m.props.RowInstrument != nil {
		return m.props.RowInstrument()
	}
	return m.state.selectedID
}

// The cursor indexes the full filtered list, so the lookup runs over filteredInsts.
func (m *InstrumentMenuComponent) syncCursorToSelected() {
	id := m.state.selectedID
	if id == "" {
		return
	}
	nav := m.keyNav()
	nav.BindHost(m)
	for i, cand := range m.state.filteredInsts {
		if cand == id {
			nav.SetSelected(i)
			return
		}
	}
}

// MenuScrollBehavior implements MenuKeyNavHost.
func (m *InstrumentMenuComponent) MenuScrollBehavior() *ScrollBehavior { return m.scroll }

// The cursor index is absolute while favRects is the scrolled viewport, so rebase it first.
func (m *InstrumentMenuComponent) MenuSecondary(i int) {
	if i < 0 || i >= len(m.state.filteredInsts) {
		return
	}
	row := i - m.scroll.VS.First
	if row < 0 || row >= len(m.instIDs) {
		// Cursor moved without scrolling: no star rect to toggle.
		return
	}
	// The list moved under us; starring would hit the wrong instrument.
	if m.instIDs[row] != m.state.filteredInsts[i] {
		return
	}
	m.toggleFavoriteAt(row)
}

func (m *InstrumentMenuComponent) keyCursorViewportRow() int {
	if !m.state.open || m.nav == nil {
		return -1
	}
	sel := m.nav.Selected()
	if sel < 0 || sel >= len(m.state.filteredInsts) {
		return -1
	}
	row := sel - m.scroll.VS.First
	if row < 0 || row >= len(m.instBtns) {
		return -1
	}
	return row
}

// A ring, not a fill, so it stays distinct from the applied row's highlight; both show at once
// while arrowing.
func (m *InstrumentMenuComponent) drawKeyCursorRing(dst *ebiten.Image) {
	row := m.keyCursorViewportRow()
	if row < 0 {
		return
	}
	r := m.instBtns[row].Rect()
	if r.Empty() {
		return
	}
	drawRoundedRect(dst, r, genColorFocusRing, fitRadius(RadiusControl, r), false)
}

// The full list MenuKeyNav addresses, not the scrolled viewport VisibleInstIDs returns.
func (m *InstrumentMenuComponent) FilteredInstIDsForTest() []instrument.ID {
	return append([]instrument.ID(nil), m.state.filteredInsts...)
}

// MenuKeyNavForTest exposes the keyboard cursor for tests.
func (m *InstrumentMenuComponent) MenuKeyNavForTest() *MenuKeyNav { return m.keyNav() }

func (m *InstrumentMenuComponent) kbFocusSearch() {
	if m.searchBox == nil {
		return
	}
	m.searchBox.SetFocus(true)
}

func (m *InstrumentMenuComponent) handleKeyEdge(k ebiten.Key, fn func()) {
	now := isKeyPressed(k)
	if now && !m.keyEdge[k] {
		fn()
	}
	m.keyEdge[k] = now
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

	if Profile().UseBottomSheet {
		drawBottomSheetPanel(dst, m.fullRect)
	} else {
		drawPanel(dst, m.fullRect)
	}
	if Profile().IsMobile() {
		m.drawBottomSheetHandle(dst)
	}

	if m.state.mode == InstMenuModeCategories {
		m.drawBreadcrumbStrip(dst)
		// Global search row below the title, same chrome as instruments mode.
		if !m.searchRect.Empty() {
			drawMenuHeaderBandAccent(dst, m.searchRect, m.menuAccent())
		}
		if m.searchBox != nil {
			m.drawSearchRow(dst)
		}
		if m.globalQueryActive() {
			for i, btn := range m.instBtns {
				var id instrument.ID
				trail := ""
				if i < len(m.instIDs) {
					id = m.instIDs[i]
				}
				if i < len(m.instTrails) {
					trail = m.instTrails[i]
				}
				m.drawInstRowTrail(dst, btn, id, trail)
			}
			for _, btn := range m.categoryBtns {
				m.drawGlobalCatRow(dst, btn)
			}
		} else {
			for i, btn := range m.categoryBtns {
				m.drawCategoryRow(dst, btn, i)
			}
		}
		m.drawScrollbar(dst)
	} else {
		m.drawBreadcrumbStrip(dst)
		if !m.searchRect.Empty() {
			drawMenuHeaderBandAccent(dst, m.searchRect, m.menuAccent())
		}
		if m.searchBox != nil {
			m.drawSearchRow(dst)
		}
		for i, btn := range m.instBtns {
			var id instrument.ID
			if i < len(m.instIDs) {
				id = m.instIDs[i]
			}
			m.drawInstRow(dst, btn, id)
		}
		m.drawFavoriteStars(dst)
		// Above rows and stars so it is never half-covered, below the scrollbar.
		m.drawKeyCursorRing(dst)
		m.drawScrollbar(dst)
	}
	if m.closeBtn != nil {
		m.closeBtn.Draw(dst)
	}
}

func (m *InstrumentMenuComponent) computeBreadcrumbStrip(r image.Rectangle) {
	m.breadcrumbRect = r
	m.breadcrumbRects = m.breadcrumbRects[:0]
	labels := m.BreadcrumbPath()
	if len(labels) == 0 {
		return
	}
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

// The owning instrument's color for the chrome; instrument rows keep their own colors.
func (m *InstrumentMenuComponent) menuAccent() color.Color {
	if cur := m.ActiveInstrumentID(); cur != "" {
		if c := m.instColorFor(cur); c != nil {
			return c
		}
	}
	return colAccent
}

func (m *InstrumentMenuComponent) drawBreadcrumbStrip(dst *ebiten.Image) {
	labels := m.BreadcrumbPath()
	if len(labels) == 0 || len(m.breadcrumbRects) == 0 {
		return
	}
	for i, seg := range m.breadcrumbRects {
		if i >= len(labels) {
			break
		}
		var col color.Color = colTextSecondary
		if i == len(labels)-1 {
			col = m.menuAccent()
		}
		th := StyledTextHeight(RoleBody)
		ty := seg.Min.Y + (seg.Dy()-th)/2
		DrawTextStyled(dst, labels[i], seg.Min.X+m.style.SegmentPaddingX, ty, RoleBody, col)
		// Separator chevron between segments.
		if i < len(labels)-1 && i < len(m.breadcrumbRects)-1 {
			next := m.breadcrumbRects[i+1]
			sepRect := image.Rect(seg.Max.X, seg.Min.Y, next.Min.X, seg.Max.Y)
			DrawIcon(dst, breadcrumbSeparatorIcon, insetRect(sepRect, SpaceXS), colTextSecondary)
		}
	}
}

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

func (m *InstrumentMenuComponent) breadcrumbHitAt(x, y int) int {
	pt := image.Pt(x, y)
	for idx, seg := range m.breadcrumbRects {
		if pt.In(seg) {
			return idx
		}
	}
	return -1
}

// Only depth 0 does anything in the two-level model.
func (m *InstrumentMenuComponent) popBreadcrumbTo(d int) {
	if d <= 0 {
		m.state.mode = InstMenuModeCategories
		m.state.activeCat = audio.CatNone
		m.state.favoritesView = false
		m.scroll.VS.First = 0
		m.state.cameFromCats = false
		m.state.userScrolled = false
		m.rebuildMenu()
		return
	}
	// Future-proof: deeper levels become no-ops today.
}

// Color is the instrument's identity, so the chip must read at a glance.
const instMenuSwatchSz = SwatchSizeMD

func (m *InstrumentMenuComponent) drawInstRow(dst *ebiten.Image, btn *Button, id instrument.ID) {
	m.drawInstRowTrail(dst, btn, id, "")
}

func (m *InstrumentMenuComponent) drawInstRowTrail(dst *ebiten.Image, btn *Button, id instrument.ID, trail string) {
	if btn.Rect().Empty() {
		return
	}
	// The empty state is a centered muted line, not a fake row.
	if id == "" && btn.OnClick == nil {
		drawMenuRow(dst, btn, MenuRowSpec{
			State:       menuItemRest,
			Label:       btn.Text,
			LabelColor:  colTextSecondary,
			CenterLabel: true,
		})
		return
	}
	// The live selection, so exactly one row reads active during an audition.
	state := menuItemRest
	if id != "" && id == m.ActiveInstrumentID() {
		state = menuItemActive
	} else if btn.hovered || btn.pressed {
		state = menuItemHover
	}
	accent := m.instColorFor(id)
	drawMenuRow(dst, btn, MenuRowSpec{
		Accent:     accent,
		State:      state,
		Swatch:     accent, // instrument swatch == its accent color
		Highlights: btn.Highlights,
		Label:      btn.Text,
		TrailLabel: trail,
	})
}

func (m *InstrumentMenuComponent) drawGlobalCatRow(dst *ebiten.Image, btn *Button) {
	if btn.Rect().Empty() {
		return
	}
	state := menuItemRest
	if btn.hovered || btn.pressed {
		state = menuItemHover
	}
	drawMenuRow(dst, btn, MenuRowSpec{
		Accent:     m.menuAccent(),
		State:      state,
		IconID:     IconChevronRight,
		Highlights: btn.Highlights,
		Label:      btn.Text,
	})
}

func (m *InstrumentMenuComponent) drawCategoryRow(dst *ebiten.Image, btn *Button, idx int) {
	if btn.Rect().Empty() {
		return
	}
	state := menuItemRest
	// Recorded in meta, never inferred from style identity: ApplyTheme rebuilds the styles, so the
	// comparison fails after a theme change.
	if idx >= 0 && idx < len(m.categoryMeta) && m.categoryMeta[idx].active {
		state = menuItemActive
	} else if btn.hovered || btn.pressed {
		state = menuItemHover
	}
	spec := MenuRowSpec{
		Accent:      m.menuAccent(),
		State:       state,
		Label:       btn.Text,
		TrailIconID: IconChevronRight,
	}
	if idx >= 0 && idx < len(m.categoryMeta) {
		meta := m.categoryMeta[idx]
		if meta.count > 0 {
			spec.TrailLabel = compactCountLabel(meta.count)
		}
		if meta.favRow {
			spec.IconID = IconStarFilled
			spec.IconTint = colAccent
		}
	}
	drawMenuRow(dst, btn, spec)
}

// Exact below 1000, "7.9k" above.
func compactCountLabel(n int) string {
	if n < 1000 {
		return pageChipLabel(n)
	}
	whole := n / 1000
	tenth := (n % 1000) / 100
	if tenth == 0 {
		return pageChipLabel(whole) + "k"
	}
	return pageChipLabel(whole) + "." + pageChipLabel(tenth) + "k"
}

// Row-indexed: only rows in this frame's layout draw a star.
func (m *InstrumentMenuComponent) drawFavoriteStars(dst *ebiten.Image) {
	if m.props.Favorites == nil {
		return
	}
	for i, rect := range m.favRects {
		if rect.Empty() || i >= len(m.instIDs) {
			continue
		}
		id := m.instIDs[i]
		if id == "" {
			continue
		}
		fav := m.props.Favorites.Get(id)
		icon := IconStar
		var col color.Color = colTextSecondary
		if fav {
			icon = IconStarFilled
			// Filled star carries that instrument's own color.
			col = m.instColorFor(id)
		}
		// Inset slightly so the icon doesn't touch the row border.
		DrawIcon(dst, icon, insetRect(rect, SpaceXS), col)
	}
}

func (m *InstrumentMenuComponent) BreadcrumbPath() []string {
	if !m.state.open {
		return []string{}
	}
	out := []string{i18n.T(i18n.KeyMenuCategories)}
	if m.state.mode == InstMenuModeInstruments {
		if m.state.favoritesView {
			out = append(out, i18n.T(i18n.KeyMenuFavorites))
		} else if m.state.activeCat != audio.CatNone {
			out = append(out, m.state.activeCat.Label())
		}
	}
	return out
}

func (m *InstrumentMenuComponent) Page() int {
	m.ensureScroll()
	if m.scroll.VS.Visible <= 0 || m.scroll.VS.Total == 0 {
		return 1
	}
	return (m.scroll.VS.First / m.scroll.VS.Visible) + 1
}

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

func (m *InstrumentMenuComponent) drawBottomSheetHandle(dst *ebiten.Image) {
	drawSheetDragHandle(dst, m.fullRect)
}

func (m *InstrumentMenuComponent) drawSearchRow(dst *ebiten.Image) {
	drawMenuSearchBox(dst, m.searchBox)
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
func (m *InstrumentMenuComponent) ActiveCategory() audio.Category {
	return m.state.activeCat
}

// The live selection; the draw path and the test seam both read it so they cannot disagree.
func (m *InstrumentMenuComponent) ActiveInstrumentID() instrument.ID {
	if m.props.RowInstrument != nil {
		return m.props.RowInstrument()
	}
	if m.state.selectedID != "" {
		return m.state.selectedID
	}
	return m.props.CurrentInstrument
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

// DrumView mirrors this into dv.instSearchRect, or the phone keyboard never opens for the field.
func (m *InstrumentMenuComponent) SearchRect() image.Rectangle {
	if m == nil {
		return image.Rectangle{}
	}
	return m.searchRect
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
