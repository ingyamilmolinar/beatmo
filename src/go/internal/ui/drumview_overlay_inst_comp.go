package ui

import (
	"image"
	"image/color"
	"slices"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// instSearchMobileInputID is the mobile native-input bridge id for the
// instrument-menu search field. Shared by drumview_layout.go (which registers
// the field's rect so a tap raises the native keyboard) and the component's
// Update() (which reads the live typed value back to drive real-time filtering).
const instSearchMobileInputID = "inst-search"

// InstrumentOption represents an instrument available for selection.
type InstrumentOption struct {
	ID       string
	Label    string
	Category string
	// Color is the instrument's effective display color — the live DrumRow.Color
	// of the row this instrument is bound to, so the picker swatch / active
	// stripe / accent matches the grid node exactly. Nil for instruments not on
	// any row; the menu then falls back to the instColor(id) registered default.
	Color color.Color
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
	// (breadcrumb / star column).
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
	// breadcrumbSeparatorIcon is the glyph drawn between breadcrumb
	// segments. The forbidden-glyph guard rejects literal U+2039 /
	// U+203A; this constant is the canonical replacement.
	breadcrumbSeparatorIcon = IconChevronRight
)

// InstrumentMenuState contains the internal state for the instrument menu component.
type InstrumentMenuState struct {
	open      bool
	hold      bool // Capture flag after menu closes
	mode      InstMenuMode
	activeCat string
	// favoritesView, when true, makes buildInstrumentsMode filter to ★-favorited
	// ids only and skip the activeCat check. Set by clicking the virtual
	// "Favorites" category at the top of buildCategoriesMode; cleared by Back.
	favoritesView bool
	userScrolled  bool
	// selectedID is the live "currently-selected" instrument the menu
	// highlights as active. Seeded from props.CurrentInstrument at Open and
	// updated on every in-menu selection so the active stripe follows the user
	// while the menu stays open (audition). Empty falls back to the prop.
	selectedID   string
	lastAdded    string
	cameFromCats  bool
	searchText    string
	filteredInsts []string
	// fuzzyScores parallels filteredInsts when searchText is non-empty;
	// used by StableTierBreaker to break score ties by tier without losing
	// the score primary key. Empty when no search query is active.
	fuzzyScores      map[string]int
	categoryByID     map[string]string
	displayLabelByID map[string]string
	// colorByID maps an instrument id to its effective display color (the live
	// owning-row color from props.InstrumentOption.Color). Rebuilt on every
	// SetProps so a node recolor flows through; absent ids fall back to
	// instColor(id) via instColorFor. See TestInstMenuInstrumentRowMatchesNodeColor.
	colorByID        map[string]color.Color
	searchHighlights map[string][]int // Per-ID highlight positions from fuzzy search.
}

// InstrumentMenuComponent is a self-contained dropdown menu for selecting instruments.
type InstrumentMenuComponent struct {
	overlayBase
	props InstrumentMenuProps
	state InstrumentMenuState

	menuScroll   *MenuScroll     // shared scroll component (owns the ScrollBehavior)
	scroll       *ScrollBehavior // == menuScroll.ScrollBehavior(); kept for the existing call sites
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

	// Keyboard navigation state.
	selectedIdx int
	keyEdge     map[ebiten.Key]bool

	// instIDs parallels instBtns: instIDs[i] is the instrument id rendered
	// by instBtns[i]. Needed by Draw to look up the per-instrument swatch
	// color and to determine the menuItemActive stripe for the currently
	// selected instrument. Rebuilt on every rebuildMenu pass.
	instIDs []string

	// instTrails parallels instBtns in the categories-mode global search
	// results: instTrails[i] is the category label rendered as the trailing
	// caption on instBtns[i]. Empty (len 0) outside global-search results.
	// Rebuilt on every rebuildMenu pass.
	instTrails []string

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
	// Rebuild the per-instrument color overrides every SetProps (not gated by
	// needsRebuild): a node recolor leaves the id list identical but changes the
	// effective color, and the menu must pick it up.
	m.rebuildColorMap()
	if needsRebuild {
		m.rebuildMaps()
	}
}

// rebuildColorMap refreshes state.colorByID from props.Instruments, recording
// only entries that carry an explicit color (an instrument bound to a row).
func (m *InstrumentMenuComponent) rebuildColorMap() {
	m.state.colorByID = make(map[string]color.Color, len(m.props.Instruments))
	for _, inst := range m.props.Instruments {
		if inst.Color != nil {
			m.state.colorByID[inst.ID] = inst.Color
		}
	}
}

// instColorFor resolves an instrument's effective display color: the live owning
// row color when the instrument is on a row (props.InstrumentOption.Color),
// otherwise the registered instColor(id) default. This is the single seam the
// menu uses for swatches, active stripes, favorite stars, and chrome accent so
// every surface matches the grid node exactly.
func (m *InstrumentMenuComponent) instColorFor(id string) color.Color {
	if m.state.colorByID != nil {
		if c, ok := m.state.colorByID[id]; ok && c != nil {
			return c
		}
	}
	return instColor(id)
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
	// Seed the live selection from the row's current instrument; subsequent
	// in-menu picks update it so the active highlight follows the user.
	m.state.selectedID = m.props.CurrentInstrument

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

// ClaimsKeyboard reports whether the instrument menu currently owns the
// keyboard. While open, the search box (and pagination jump field) consume
// typed/caret keys, so the grid must not act on them. Part of the
// keyboard-ownership contract (keyboard_focus.go).
func (m *InstrumentMenuComponent) ClaimsKeyboard() bool {
	return m != nil && m.IsOpen()
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

// HandleEscape clears a non-empty search query (keeping the menu open). With an
// empty query it returns false so the universal Esc handler closes the menu.
func (m *InstrumentMenuComponent) HandleEscape() bool {
	if strings.TrimSpace(m.state.searchText) == "" {
		return false
	}
	m.state.searchText = ""
	if m.searchBox != nil {
		m.searchBox.SetText("")
	}
	m.rebuildMenu() // reuse the existing rebuild so the filtered list refreshes
	return true
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

// GlobalResultTrails returns, for each instrument row of the categories-mode
// global search results (parallel to InstBtns), the category the instrument
// lives in — rendered as the row's trailing caption.
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

// rebuildMenu rebuilds the menu buttons and layout.
func (m *InstrumentMenuComponent) rebuildMenu() {
	m.ensureScroll()
	m.categoryBtns = m.categoryBtns[:0]
	m.instBtns = m.instBtns[:0]
	m.instIDs = m.instIDs[:0]
	m.instTrails = m.instTrails[:0]

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

// ensureSearchBoxSynced lazily creates the shared search TextInput and keeps
// it in sync with state.searchText + the current m.searchRect. The box is
// synced only when it actually diverges from the state (programmatic set,
// clear, mobile native input). When the rebuild was triggered BY typing, box
// and state already match — calling SetText then would reset the caret to
// the end, teleporting it away from a mid-text edit.
func (m *InstrumentMenuComponent) ensureSearchBoxSynced() {
	if m.searchBox == nil {
		m.searchBox = NewTextInput(image.Rectangle{}, BPMBoxStyle)
		m.searchBox.MaxLen = 40
	}
	if m.searchBox.Value() != m.state.searchText {
		m.searchBox.SetText(m.state.searchText)
	}
	m.searchBox.Rect = insetRect(m.searchRect, SpaceSM)
}

// globalQueryActive reports whether the categories view currently shows
// global (cross-category) search results instead of the category list.
func (m *InstrumentMenuComponent) globalQueryActive() bool {
	return m.state.mode == InstMenuModeCategories &&
		strings.TrimSpace(m.state.searchText) != ""
}

// buildCategoriesMode builds the menu in categories mode: the "Categories"
// title strip, the GLOBAL search row right below it, then either the category
// list (empty query) or the mixed global search results (query active).
func (m *InstrumentMenuComponent) buildCategoriesMode(base, vertBounds image.Rectangle, rowH int, openUp bool) {
	if m.globalQueryActive() {
		m.buildGlobalSearchMode(base, vertBounds, rowH, openUp)
		return
	}
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

	// Title band: the "Categories" breadcrumb root renders above the rows,
	// mirroring instruments mode. It also hosts the close × so the button
	// never sits on top of the first row. The global search row sits between
	// the title band and the first category row.
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

	// Close button + title strip share the top band; the strip's right edge
	// clears the × by SpaceMD (mirrors the instruments-mode breadcrumb).
	closeBtnRect := closeButtonRect(m.fullRect, SpaceXS)
	bcRight := base.Max.X
	if !closeBtnRect.Empty() && startY < closeBtnRect.Max.Y && startY+headerH > closeBtnRect.Min.Y {
		if r := closeBtnRect.Min.X - SpaceMD; r < bcRight {
			bcRight = r
		}
	}
	m.computeBreadcrumbStrip(image.Rect(base.Min.X, startY, bcRight, startY+headerH))
	m.backBtn = nil

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
		r := image.Rect(base.Min.X, listStartY+i*rowH, base.Max.X, listStartY+(i+1)*rowH)

		if favPrefix && absIdx == 0 {
			favCount := len(m.favoritesIDs())
			label := i18n.T(i18n.KeyMenuFavorites)
			if favCount > 0 {
				label = label + " (" + pageChipLabel(favCount) + ")"
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

// globalSearchRow is one row of the categories-mode global search results:
// either a category (drill-in) or an instrument (select), pre-sorted for
// display.
type globalSearchRow struct {
	isCategory bool
	isFavCat   bool   // the virtual "Favorites" category row
	id         string // instrument id, or category name ("" for Favorites)
	label      string
	trail      string // instrument rows: the category the instrument lives in
	score      int
	highlights []int
	tier       int
}

// buildGlobalSearchMode renders the categories view with an active query: a
// mixed, real-time result list over EVERY instrument and category. Order:
// pinned/★ instruments first ("favorites always on top"), then matching
// categories (drill-in rows), then the remaining matching instruments. Each
// instrument row carries its category as a trailing caption so the user sees
// where a hit lives.
func (m *InstrumentMenuComponent) buildGlobalSearchMode(base, vertBounds image.Rectangle, rowH int, openUp bool) {
	query := strings.TrimSpace(m.state.searchText)

	// Instrument matches across the whole catalog.
	items := make([]MenuSearchItem, 0, len(m.props.Instruments))
	for _, inst := range m.props.Instruments {
		label := m.state.displayLabelByID[inst.ID]
		if label == "" {
			label = inst.ID
		}
		items = append(items, MenuSearchItem{Key: inst.ID, Label: label})
	}
	var searcher MenuSearcher
	res := searcher.Search(query, items)
	src := m.pinSource()

	var favRows, catRows, instRows []globalSearchRow
	for _, r := range res {
		row := globalSearchRow{
			id: r.Key, label: r.Label, trail: m.state.categoryByID[r.Key],
			score: r.Score, highlights: r.Highlights, tier: src.Tier(r.Key),
		}
		if row.tier <= 1 {
			favRows = append(favRows, row)
		} else {
			instRows = append(instRows, row)
		}
	}
	// Category matches: the virtual Favorites entry first (when enabled),
	// then the caller-supplied categories.
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
		if fr := FuzzyMatch(query, cat); fr.Score >= 0 {
			catRows = append(catRows, globalSearchRow{
				isCategory: true, id: cat, label: cat,
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

	// Layout: title band + search row + windowed result rows — the same
	// chrome as the empty-query categories view.
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

	// Windowed rows → buttons. Instrument rows land in instBtns/instIDs/
	// instTrails (tap = select, audition semantics); category rows land in
	// categoryBtns (tap = drill in, clearing the query so the category opens
	// unfiltered).
	start := m.scroll.VS.First
	for i := 0; i < vis && start+i < len(rows); i++ {
		row := rows[start+i]
		r := image.Rect(base.Min.X, listStartY+i*rowH, base.Max.X, listStartY+(i+1)*rowH)
		if row.isCategory {
			isFav := row.isFavCat
			cat := row.id
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
		optID := row.id
		btn := NewButton(row.label, DropdownStyle, func() {
			// Same audition semantics as an instruments-mode row: track the
			// live selection, keep the menu open.
			m.state.selectedID = optID
			if m.props.OnSelect != nil {
				m.props.OnSelect(optID)
			}
		})
		btn.Highlights = row.highlights
		btn.SetRect(insetRect(r, SpaceXS))
		m.instBtns = append(m.instBtns, btn)
		m.instIDs = append(m.instIDs, optID)
		m.instTrails = append(m.instTrails, row.trail)
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
			// Favorites always on top of search results: tier is the PRIMARY
			// key (pins, then ★-favorites, then the rest), fuzzy score orders
			// rows within each tier.
			src.SortByTierScore(m.state.filteredInsts,
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

	m.ensureSearchBoxSynced()

	listStartY := searchY + rowH

	m.scroll.VS.Total = len(m.state.filteredInsts)
	m.scroll.VS.Visible = vis

	if len(m.state.filteredInsts) == 0 {
		// Even with an empty result set we still render the Back button (when
		// the menu has categories) so the user can always escape — without
		// this, a Favorites view with no ★s and a query that produces no
		// matches both leave the user stuck. Back rect lives at the original
		// position above the search row.
		extraTop := rowH // search row
		if showBack {
			extraTop += rowH // back row
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
		m.instBtns = append(m.instBtns, placeholder)
		m.SetBounds(m.fullRect)
		return
	}

	m.scroll.VS.View = image.Rect(base.Min.X, listStartY, base.Max.X, listStartY+vis*rowH)
	m.fullRect = image.Rect(base.Min.X, startY, base.Max.X, listStartY+vis*rowH)

	// Translate the whole picker so it sits inside vertBounds, then shift every
	// derived coordinate (breadcrumb, search row, list, per-row buttons) by the
	// same delta so they stay aligned to the surface. This is a pure
	// translation in BOTH axes — we must NOT shrink the rect (the menu's
	// content has a fixed height/width), so we compute the translation delta
	// directly rather than via ClampRectInto, which would shrink an
	// oversized rect and detach the rows from the panel.
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

	// Close button rect (top-right). Computed here so the breadcrumb strip and
	// the per-row star column can give it SpaceMD clearance and never collide.
	closeBtnRect := closeButtonRect(m.fullRect, SpaceXS)

	hasScroll := m.scroll.HasScroll()
	buttonMaxX := base.Max.X
	if hasScroll {
		buttonMaxX -= dropdownScrollbarWidth()
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
		// Inset the breadcrumb's right edge so it clears the close × at the
		// top-right (the breadcrumb shares the top band with the close button).
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
			label = id
		}
		btn := NewButton(label, DropdownStyle, func() {
			// Track the live selection so the active-row highlight follows it.
			// props.CurrentInstrument is only a snapshot from menu-open and is
			// never refreshed while the menu stays open, so without this the
			// open-time instrument would stay highlighted forever (the "buttons
			// remain toggled" bug). Set on every selection path (desktop press,
			// mobile fireTapAt, keyboard) since they all route through OnClick.
			m.state.selectedID = optID
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
		m.instIDs = append(m.instIDs, optID)
		if starsActive {
			// Star hit area is the small rectangle to the right of the
			// row button. Drawn in Draw, hit-tested in HandleInput.
			starRect := image.Rect(buttonMaxX, r.Min.Y, buttonMaxX+favColWidth, r.Max.Y)
			// Give the close × clearance: if this row shares the close
			// button's Y-band, slide the star column left so the star and the
			// × never collide (SpaceMD gap).
			if !closeBtnRect.Empty() && starRect.Min.Y < closeBtnRect.Max.Y && starRect.Max.Y > closeBtnRect.Min.Y {
				if shift := starRect.Max.X - (closeBtnRect.Min.X - SpaceMD); shift > 0 {
					starRect = starRect.Sub(image.Pt(shift, 0))
				}
			}
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
	m.closeBtn.IconColor = closeIconColor()
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
	if m.state.mode == InstMenuModeInstruments || m.globalQueryActive() {
		// Star hit area takes priority over the row button so a touch on
		// the star toggles the favorite instead of selecting the row.
		for i, rect := range m.favRects {
			if pt.In(rect) {
				m.toggleFavoriteAt(i)
				return
			}
		}
		// Mirror the desktop dispatch (HandleInput, instruments-mode block):
		// clear any stale press from the previously-selected row, then drive the
		// tapped row through the SAME Button press core the mouse path uses
		// (PressFromTree → applyPress). This fires OnClick AND eases the keycap
		// down + leaves the row pressed-IN, so the selected-row "toggle pressed
		// down" feedback is identical on touch and under the cursor. The deferred
		// tap is the only reason instrument rows aren't already on the shared
		// press path; firing OnClick directly here skipped the press lifecycle so
		// the mobile selection stayed visually flat. The menu stays open (one-shot
		// InputConsumed, no release), so the pressed state persists until the next
		// selection's ResetPress — matching desktop.
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

	// The touch / deferred-tap state machine (dead-zone scroll, fire-on-release)
	// is a MOBILE affordance. On desktop, route input straight to the per-control
	// hit handlers below, which fire immediately on the press edge via
	// Button.HandleInputResult — the same split the context menu uses
	// (drumview_context_menu.go handleContextMenuInput). Without this, desktop
	// clicks went through the touch path: they only registered on release and
	// any pointer drift past the 10px dead zone (tapMaxMovePx) was swallowed as a
	// scroll, so the menu "did not respond to clicks well". Scrollbar thumb drag
	// stays available on both platforms (handled below, outside the touch blocks).
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

	// While a touch is active but not yet committed (in dead zone),
	// process moves. On release, check if it was a tap.
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
	// The close × joins the deferral (fireTapAt checks it first on
	// release) so mobile taps fire on release everywhere — before the
	// categories title band existed the × sat inside the scroll view and
	// got this behavior implicitly.
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

	// Consume clicks within the search rect to prevent click-through.
	// Keyboard/char input is handled by Update() every frame. The search row
	// exists in BOTH modes now (instruments filter + categories global search).
	if m.searchBox != nil && !m.searchRect.Empty() && pt.In(m.searchRect) {
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

	// Handle instrument buttons (suppress during drag or touch scroll).
	//
	// The menu stays open after a selection (audition) and returns
	// InputConsumed, so the input tree never delivers a release to these
	// buttons — Button.held stays latched at 1 across selections. Clear the
	// latched press state of every row on the press edge so a fresh click on a
	// previously-selected row is never swallowed by a stale held counter (the
	// "instrument menu is very flaky" bug). HandleInput reaches this loop once
	// per physical press on desktop (one-shot, not captured), so reset-then-fire
	// fires exactly once.
	// Instrument rows exist in instruments mode AND in the categories-mode
	// global search results.
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
	// Runs in BOTH modes: instruments mode filters within the category,
	// categories mode drives the global search results.
	if m.searchBox != nil {
		// Mobile: the search text is typed into a native HTML <input> overlay,
		// not the ebiten keyboard the searchBox reads. Pull its live value
		// (non-consuming) every frame so the list filters/highlights in real
		// time as the user types. mobileInputGetValue returns ok=false when no
		// native input is active for this field, leaving the query untouched.
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

	// Backdrop scrim painted by the OverlayPortal (PortalEntry.Scrim) so the
	// whole stack dims once; this overlay only draws its own surface.
	if Profile().UseBottomSheet {
		drawBottomSheetPanel(dst, m.fullRect)
	} else {
		drawPanel(dst, m.fullRect)
	}
	if Profile().IsMobile() {
		m.drawBottomSheetHandle(dst)
	}

	if m.state.mode == InstMenuModeCategories {
		// Title band: "Categories" breadcrumb root, same strip renderer as
		// instruments mode so title typography stays identical across modes.
		m.drawBreadcrumbStrip(dst)
		// Global search row below the title, same chrome as instruments mode.
		if !m.searchRect.Empty() {
			drawMenuHeaderBandAccent(dst, m.searchRect, m.menuAccent())
		}
		if m.searchBox != nil {
			m.drawSearchRow(dst)
		}
		if m.globalQueryActive() {
			// Mixed global results: instrument rows (with category trail
			// captions) + category drill-in rows.
			for i, btn := range m.instBtns {
				id, trail := "", ""
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
			for _, btn := range m.categoryBtns {
				m.drawCategoryRow(dst, btn)
			}
		}
		m.drawScrollbar(dst)
	} else {
		// Breadcrumb strip replaces the back button visually; the
		// back button remains for legacy click compat.
		m.drawBreadcrumbStrip(dst)
		// Header band over the search row (Neon Horizon treatment), tinted to
		// the row's current instrument color.
		if !m.searchRect.Empty() {
			drawMenuHeaderBandAccent(dst, m.searchRect, m.menuAccent())
		}
		if m.searchBox != nil {
			m.drawSearchRow(dst)
		}
		for i, btn := range m.instBtns {
			id := ""
			if i < len(m.instIDs) {
				id = m.instIDs[i]
			}
			m.drawInstRow(dst, btn, id)
		}
		m.drawFavoriteStars(dst)
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
// menuAccent returns the picker's owning-instrument accent — the color of the
// row's currently-selected instrument — so the menu chrome (active category
// rows, pagination chip, breadcrumb, header underline) reads as owned by that
// instrument. Instrument ROWS keep their own per-id color (drawInstRow). Falls
// back to the azure chrome accent when there is no current instrument.
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

// instMenuSwatchSz is the side length (in px) of the colored instrument
// swatch drawn to the left of each instrument label. Matches the row-rack
// accent-dot convention (small, clearly visible, not overwhelming).
const instMenuSwatchSz = 8

// drawInstRow renders one instrument-list row through drawMenuRow: background
// accent tinted to the instrument's own color, keycap chrome, a colored swatch,
// fuzzy-match highlights, and the label. Pixel-identical to the previous
// hand-rolled body; routing through drawMenuRow unifies styling with every
// other overlay menu.
func (m *InstrumentMenuComponent) drawInstRow(dst *ebiten.Image, btn *Button, id string) {
	m.drawInstRowTrail(dst, btn, id, "")
}

// drawInstRowTrail is drawInstRow with an optional right-aligned muted trail
// caption — the categories-mode global search uses it to show which category
// each matched instrument lives in.
func (m *InstrumentMenuComponent) drawInstRowTrail(dst *ebiten.Image, btn *Button, id, trail string) {
	if btn.Rect().Empty() {
		return
	}
	// LIVE selection (ActiveInstrumentID) → exactly one row reads active; the
	// highlight follows audition picks while the menu stays open.
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

// drawGlobalCatRow renders a category row inside the global search results: a
// leading chevron icon marks it as a drill-in container, and the fuzzy-match
// highlights land on the category name.
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

// drawCategoryRow renders one category row through drawMenuRow: background
// accent, keycap chrome, and the label at SpaceMD — identical to the previous
// hand-rolled body, unified with every other overlay menu via drawMenuRow.
func (m *InstrumentMenuComponent) drawCategoryRow(dst *ebiten.Image, btn *Button) {
	if btn.Rect().Empty() {
		return
	}
	state := menuItemRest
	if btn.Style == PopupButtonStyle {
		// PopupButtonStyle is set on the active category/Favorites row.
		state = menuItemActive
	} else if btn.hovered || btn.pressed {
		state = menuItemHover
	}
	drawMenuRow(dst, btn, MenuRowSpec{
		Accent: m.menuAccent(),
		State:  state,
		Label:  btn.Text,
	})
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

// BreadcrumbPath returns the current path as visible-segment labels,
// localized in the active locale. Returns []string{} when closed; the
// localized "Categories" root at root level (English default shown here);
// [<Categories>, <activeCat>] when drilled into a category. Used by the
// instMenuBreadcrumbPath JS export and by tests asserting on the
// breadcrumb state without coupling to the legacy mode enum.
func (m *InstrumentMenuComponent) BreadcrumbPath() []string {
	if !m.state.open {
		return []string{}
	}
	out := []string{i18n.T(i18n.KeyMenuCategories)}
	if m.state.mode == InstMenuModeInstruments {
		if m.state.favoritesView {
			out = append(out, i18n.T(i18n.KeyMenuFavorites))
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

// drawBottomSheetHandle paints the shared drag-handle pill at the top-center
// of the bottom sheet to telegraph it as a draggable surface.
func (m *InstrumentMenuComponent) drawBottomSheetHandle(dst *ebiten.Image) {
	drawSheetDragHandle(dst, m.fullRect)
}

// drawSearchRow renders the search input plus a muted "Search" hint when the
// box is empty and unfocused. Hint placement uses StyledTextHeight(RoleBody)
// for vertical centering.
func (m *InstrumentMenuComponent) drawSearchRow(dst *ebiten.Image) {
	m.searchBox.Draw(dst)
	if m.state.searchText == "" && !m.searchBox.Focused() {
		r := m.searchBox.Rect
		th := StyledTextHeight(RoleBody)
		ty := r.Min.Y + (r.Dy()-th)/2
		DrawTextStyled(dst, i18n.T(i18n.KeyCapSearch), r.Min.X+SpaceXS, ty, RoleBody, colTextSecondary)
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

// ActiveInstrumentID returns the instrument id the menu renders as the
// currently-selected ("active") row. The menu stays open after a selection so
// the user can audition multiple instruments; the active highlight must follow
// the LIVE selection so exactly one row ever reads as selected. drawInstRow and
// menuAccent both consult this so the test seam and the render path can never
// disagree.
func (m *InstrumentMenuComponent) ActiveInstrumentID() string {
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

// SearchRect returns the current search-field rect (both modes host a search
// row: instruments mode filters within the category, categories mode drives
// the global search). DrumView mirrors this into dv.instSearchRect so the
// soft-keyboard / mobile native-input registrations in drumview_layout.go can
// locate the field — without the mirror, the field is never registered and
// the mobile native keyboard never opens.
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
