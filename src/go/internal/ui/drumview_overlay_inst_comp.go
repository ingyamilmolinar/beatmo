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

// instSearchMobileInputID is the mobile native-input bridge id for the
// instrument-menu search field. Shared by drumview_layout.go (which registers
// the field's rect so a tap raises the native keyboard) and the component's
// Update() (which reads the live typed value back to drive real-time filtering).
const instSearchMobileInputID = "inst-search"

// InstrumentOption represents an instrument available for selection.
type InstrumentOption struct {
	ID       instrument.ID
	Label    string
	Category audio.Category
	// Color is the instrument's effective display color — the live DrumRow.Color
	// of the row this instrument is bound to, so the picker swatch / active
	// stripe / accent matches the grid node exactly. Nil for instruments not on
	// any row; the menu then falls back to the instColor(id) registered default.
	Color color.Color
}

// menuCatMeta is the per-category-row display metadata rendered by
// drawCategoryRow: the trailing instrument count and whether the row is the
// virtual Favorites entry (gold filled-star leading icon).
type menuCatMeta struct {
	favRow bool
	count  int
	active bool // the current category / Favorites view
}

// InstrumentMenuProps contains the external state passed to the instrument menu component.
type InstrumentMenuProps struct {
	// TextFields is the owning DrumView's text-field registry (native keyboard
	// wiring for the search box); nil in isolated component tests.
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
	ControlsWidth int
	// ForceCategories starts the menu in categories mode when true and categories exist.
	// Default is false (start in instruments mode).
	ForceCategories bool

	// OnSelect is called when an instrument is selected.
	OnSelect func(instID instrument.ID)
	// RowInstrument, if non-nil, reports what the row actually plays now. A
	// selection can be deferred (a settings question opens first, and may be
	// cancelled), so the menu reads the applied instrument from here rather
	// than assuming a click applied it.
	RowInstrument func() instrument.ID
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
	OnFavorite func(instID instrument.ID, fav bool)

	// ProjectPins are the per-project pinned instrument ids (tier 0 in
	// the menu's PinSource). Independent from Favorites/UserStars (tier
	// 1). When non-empty, project-pinned items sort above ★ items in
	// every render path. No mutation hook today — the per-row "pin to
	// project" affordance ships in a follow-up; this PR only consumes
	// the set on the read path.
	ProjectPins map[instrument.ID]struct{}

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
	activeCat audio.Category
	// favoritesView, when true, makes buildInstrumentsMode filter to ★-favorited
	// ids only and skip the activeCat check. Set by clicking the virtual
	// "Favorites" category at the top of buildCategoriesMode; cleared by Back.
	favoritesView bool
	userScrolled  bool
	// selectedID is the live "currently-selected" instrument the menu
	// highlights as active. Seeded from props.CurrentInstrument at Open and
	// updated on every in-menu selection so the active stripe follows the user
	// while the menu stays open (audition). Empty falls back to the prop.
	selectedID    instrument.ID
	lastAdded     instrument.ID
	cameFromCats  bool
	searchText    string
	filteredInsts []instrument.ID
	// fuzzyScores parallels filteredInsts when searchText is non-empty;
	// used by StableTierBreaker to break score ties by tier without losing
	// the score primary key. Empty when no search query is active.
	fuzzyScores      map[instrument.ID]int
	categoryByID     map[instrument.ID]audio.Category
	displayLabelByID map[instrument.ID]string
	// colorByID maps an instrument id to its effective display color (the live
	// owning-row color from props.InstrumentOption.Color). Rebuilt on every
	// SetProps so a node recolor flows through; absent ids fall back to
	// instColor(id) via instColorFor. See TestInstMenuInstrumentRowMatchesNodeColor.
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
	// categoryMeta is parallel to categoryBtns (categories mode only): the
	// per-row trailing count + the virtual-Favorites marker consumed by
	// drawCategoryRow. Global-search category rows carry zero-value meta.
	categoryMeta []menuCatMeta
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
	// nav is the shared keyboard cursor (menu_keynav.go). It replaced the
	// bespoke selectedIdx + seven kb* methods this menu used to carry alone.
	nav     *MenuKeyNav
	keyEdge map[ebiten.Key]bool

	// instIDs parallels instBtns: instIDs[i] is the instrument id rendered
	// by instBtns[i]. Needed by Draw to look up the per-instrument swatch
	// color and to determine the menuItemActive stripe for the currently
	// selected instrument. Rebuilt on every rebuildMenu pass.
	instIDs []instrument.ID

	// instTrails parallels instBtns in the categories-mode global search
	// results: instTrails[i] is the category label rendered as the trailing
	// caption on instBtns[i]. Empty string outside global-search results.
	// Rebuilt on every rebuildMenu pass.
	instTrails []string

	// favRects parallels instBtns: favRects[i] is the screen-space star hit
	// area for instBtns[i], and the id it toggles is instIDs[i]. The zero
	// rect means "this row has no star" (no FavoritesStore wired, or the row
	// is the empty-state placeholder). Keeping the star in a row-indexed slot
	// — rather than in its own list with its own id list — is what makes a
	// star impossible to draw for a row that isn't on screen: every append
	// goes through addInstRow, and rebuildMenu truncates all four slices
	// together before any build path runs.
	favRects []image.Rectangle

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
	m.state.colorByID = make(map[instrument.ID]color.Color, len(m.props.Instruments))
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
func (m *InstrumentMenuComponent) instColorFor(id instrument.ID) color.Color {
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

// ClaimsKeyboard reports whether the instrument menu currently owns the
// keyboard. While open, the search box (and pagination jump field) consume
// typed/caret keys, so the grid must not act on them. Part of the
// keyboard-ownership contract (keyboard_focus.go).
func (m *InstrumentMenuComponent) ClaimsKeyboard() bool {
	return m != nil && m.IsOpen()
}

// SetLastAdded sets the instrument ID to auto-bias to when opening.
func (m *InstrumentMenuComponent) SetLastAdded(id instrument.ID) {
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
	m.clearSearch()
	return true
}

// HoverAt implements portalHoverer: refreshes every row/button hovered flag
// from the plain cursor position once per frame, so menu rows show their
// hover tint on desktop (portal input handlers only run on press edges).
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

// clearSearch wipes the query and refreshes the filtered list — shared by
// Esc and the search field's trailing clear-× button.
func (m *InstrumentMenuComponent) clearSearch() {
	m.state.searchText = ""
	if m.searchBox != nil {
		m.searchBox.SetText("")
	}
	m.rebuildMenu() // reuse the existing rebuild so the filtered list refreshes
}

// searchClearHit reports whether pt lands on the search field's clear-×
// (only live while a query is present and the drawn adornments are active).
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

// GlobalResultTrails returns, for each instrument row of the categories-mode
// global search results (parallel to InstBtns), the category the instrument
// lives in — rendered as the row's trailing caption. Rows built outside
// global search carry an empty trail rather than being absent, so the slice
// stays index-aligned with InstBtns in every mode.
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

// addInstRow appends one instrument row and everything that hangs off it in a
// single step, so the four row-indexed slices can never drift out of sync.
// star is the zero rect for rows that carry no ★ (favorites unwired, or the
// empty-state placeholder). This is the ONLY place that grows instBtns.
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
	// Truncate every row-indexed slice up front, BEFORE any early return
	// below (empty anchor) or inside a build path (categories mode, empty
	// result set) can skip the per-row layout. The stars used to be cleared
	// deep inside buildInstrumentsMode, past its empty-list early return, so
	// an empty Favorites view kept the previous listing's stars and painted
	// them down the screen over unrelated UI.
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
	// NewTextField (via ensureMenuSearchBox) wires the keyboard: the desktop
	// soft-keyboard proxy (lossless under slow frames) and, on mobile, the
	// native <input> overlay (instSearchMobileInputID).
	m.searchBox = ensureMenuSearchBox(m.searchBox, m.searchRect, m.props.TextFields,
		TextFieldSpec{ID: instSearchMobileInputID, Owner: "inst-menu"})
	if m.searchBox.Value() != m.state.searchText {
		m.searchBox.SetText(m.state.searchText)
	}
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

// globalSearchRow is one row of the categories-mode global search results:
// either a category (drill-in) or an instrument (select), pre-sorted for
// display.
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
		// Global-search results carry no star column (the rows share the
		// list with category drill-in rows and there is no reserved gutter).
		m.addInstRow(btn, optID, row.trail, image.Rectangle{})
	}

	m.buildCloseBtn()
	m.SetBounds(m.fullRect)
}

// buildInstrumentsMode builds the menu in instruments mode.
func (m *InstrumentMenuComponent) buildInstrumentsMode(base, vertBounds image.Rectangle, rowH int, openUp bool) {
	// Filter instruments: favorites view OR category filter, then fuzzy
	// search. favoritesView short-circuits the category filter so the user
	// sees their full ★ set regardless of which category each item lives in.
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

	// Tier sort. Favorites view is already filtered to the ★ set, so a tier
	// sort there would just rearrange the alpha order — skip it (everything
	// is favorited; sort alphabetically by label only).
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
			// Favorites always on top of search results: tier is the PRIMARY
			// key (pins, then ★-favorites, then the rest), fuzzy score orders
			// rows within each tier.
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
			label = string(id)
		}
		btn := NewButton(label, DropdownStyle, func() {
			// Every selection path (desktop press, mobile fireTapAt, keyboard
			// Enter) routes through applyInstrument, so the pointer and the
			// keyboard cannot drift apart.
			m.applyInstrument(optID)
		})
		btn.Highlights = m.state.searchHighlights[optID]
		btn.SetRect(insetRect(r, SpaceXS))
		// Star hit area is the small rectangle to the right of the row
		// button — zero rect when favorites aren't wired. Drawn in Draw,
		// hit-tested in HandleInput; both key off this row's index.
		var starRect image.Rectangle
		if starsActive {
			starRect = image.Rect(buttonMaxX, r.Min.Y, buttonMaxX+favColWidth, r.Max.Y)
			// Give the close × clearance: if this row shares the close
			// button's Y-band, slide the star column left so the star and the
			// × never collide (SpaceMD gap).
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

// buildCloseBtn creates the close button at the top-right of the menu — desktop
// only, via the shared newPopupCloseButton helper (nil on mobile, where the
// bottom sheet closes by tapping away). Every draw/hit site guards `closeBtn != nil`.
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
		// Star hit area takes priority over the row button so a touch on
		// the star toggles the favorite instead of selecting the row.
		if i := m.starRowAt(pt); i >= 0 {
			m.toggleFavoriteAt(i)
			return
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

// starRowAt returns the row index whose star contains pt, or -1. Rows with a
// zero star rect (favorites unwired, placeholder row, global-search result)
// are never hit: image.Rectangle.Empty() rejects them before the containment
// test, which also means a zero rect can't swallow the origin.
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

// toggleFavoriteAt flips the star for the instrument on row i and notifies
// the caller. The id comes from instIDs — the row's own id — so a star can
// only ever toggle the instrument it is drawn next to. Bounds-safe so callers
// can pass hit-test results.
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
	// A press on the trailing clear-× wipes the query instead.
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

	// Handle star (favorite) toggles ahead of the row buttons so a click
	// on the star never selects the instrument. Only fires on the press
	// edge to match Button.Handle semantics.
	if pressed && m.state.mode == InstMenuModeInstruments && !m.scroll.Dragging() && !touchSuppressButtons {
		if i := m.starRowAt(pt); i >= 0 {
			m.toggleFavoriteAt(i)
			return InputConsumed
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
		// MenuKeyNav.reveal scrolls the list so the cursor stays on the visible
		// page, but the rows themselves (instBtns/instIDs/instTrails/favRects)
		// only regenerate in rebuildMenu. Without this, End/PageDown moved
		// scroll.VS.First while the drawn list stayed frozen on the first page:
		// the highlight walked off the page, the labels were stale, and
		// Shift+Enter would have starred whatever sat at that viewport slot.
		//
		// VisibleInstIDs hides this — it DERIVES ids from filteredInsts plus
		// scroll.First rather than reading the built rows — so assert on
		// instIDs when guarding this.
		beforeFirst := m.scroll.VS.First
		m.handleKeyboardNav()
		if m.scroll.VS.First != beforeFirst {
			// A keyboard scroll is a USER scroll. Without this flag rebuildMenu
			// re-biases VS.First back onto CurrentInstrument (the "bias to
			// current instrument" block), so the list snaps straight back to
			// where it opened and End/PageDown appear not to work at all.
			// Mirrors the momentum-scroll path below.
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

// handleKeyboardNav runs one frame of the SHARED menu keyboard navigation
// (menu_keynav.go), plus the two bindings unique to this menu.
//
// The arrow/Enter/Home/End/PageUp/PageDown behaviour used to live here as
// seven bespoke kb* methods — this menu was the only one that had any keyboard
// support at all. They now drive MenuKeyNav so every list menu shares one
// implementation instead of six copies drifting apart.
//
// Unlike before, the search box no longer swallows the arrows: while typing a
// filter, Up/Down walk the filtered results and Enter picks the highlighted
// one. The caret keeps Left/Right and text entry.
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

// --- MenuKeyNavHost ---
//
// Rows are the FILTERED instrument list, so indices follow the current search
// query; MenuKeyNav is Reset whenever that list changes shape (see rebuildMenu).

// MenuItemCount implements MenuKeyNavHost.
func (m *InstrumentMenuComponent) MenuItemCount() int { return len(m.state.filteredInsts) }

// MenuItemSelectable implements MenuKeyNavHost: every filtered row is a choice.
func (m *InstrumentMenuComponent) MenuItemSelectable(i int) bool {
	return i >= 0 && i < len(m.state.filteredInsts)
}

// MenuActivate implements MenuKeyNavHost: identical to clicking the row.
//
// It used to fire OnSelect and then Close unconditionally, so Enter and a click
// meant different things — a click previewed, Enter selected-and-slammed-shut.
// Both now go through applyInstrument, so Enter previews and a second Enter
// (the "double Enter" confirm) closes.
func (m *InstrumentMenuComponent) MenuActivate(i int) {
	if i < 0 || i >= len(m.state.filteredInsts) {
		return
	}
	m.applyInstrument(m.state.filteredInsts[i])
}

// applyInstrument is the ONE mutation every selection path goes through.
//
// SELECTING PREVIEWS, RE-SELECTING CONFIRMS — the same positional rule the
// colour picker (applySwatch) and the note matrix (applyNoteMatrixPitch) use.
// Choosing a different instrument applies it and leaves the menu OPEN so
// several can be auditioned; choosing the one already applied is the confirm
// and closes. Positional, not time-based: no double-click timer to tune, and
// the pointer and the keyboard behave identically because they share this path.
func (m *InstrumentMenuComponent) applyInstrument(id instrument.ID) {
	if id == "" {
		return
	}
	if m.appliedID() == id {
		// Already applied — this is the confirm.
		m.Close()
		return
	}
	// Track the live selection so the active-row highlight follows it.
	// props.CurrentInstrument is only a snapshot from menu-open and is never
	// refreshed while the menu stays open, so without this the open-time
	// instrument would stay highlighted forever (the "buttons remain toggled"
	// bug).
	m.state.selectedID = id
	if m.props.OnSelect != nil {
		m.props.OnSelect(id)
	}
	m.state.selectedID = m.appliedID() // the selection may be waiting on a question
	// Move the keyboard cursor onto what was just applied, so after a CLICK the
	// next Enter confirms that row rather than re-applying whatever the cursor
	// was left pointing at.
	m.syncCursorToSelected()
}

// appliedID is the instrument the row plays now (RowInstrument), else the
// last selection.
func (m *InstrumentMenuComponent) appliedID() instrument.ID {
	if m.props.RowInstrument != nil {
		return m.props.RowInstrument()
	}
	return m.state.selectedID
}

// syncCursorToSelected places the keyboard cursor on the currently-applied
// instrument, keeping pointer and keyboard coherent.
//
// The cursor indexes filteredInsts (the FULL filtered list), not the scrolled
// viewport, so the lookup must run over filteredInsts too.
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

// MenuSecondary implements MenuKeyNavSecondary: Shift+Enter toggles the ★ on
// the cursor row, the keyboard twin of clicking the star column.
//
// The cursor index is ABSOLUTE (into filteredInsts) while favRects/instIDs are
// the scrolled VIEWPORT, so it has to be rebased through keyCursorViewportRow.
// Using the cursor index directly stars whatever instrument happens to occupy
// that viewport slot — correct only while the list is unscrolled.
func (m *InstrumentMenuComponent) MenuSecondary(i int) {
	if i < 0 || i >= len(m.state.filteredInsts) {
		return
	}
	row := i - m.scroll.VS.First
	if row < 0 || row >= len(m.instIDs) {
		// The cursor is outside the drawn viewport; there is no star rect to
		// toggle. reveal() normally prevents this, but a caller that moved the
		// cursor without scrolling can still land here.
		return
	}
	// Belt and braces: the rebased row must still be the instrument the cursor
	// points at, or the list moved under us and starring it would be wrong.
	if m.instIDs[row] != m.state.filteredInsts[i] {
		return
	}
	m.toggleFavoriteAt(row)
}

// keyCursorViewportRow maps the keyboard cursor onto the VIEWPORT row index
// used by instBtns/instIDs/favRects, or -1 when nothing is highlighted or the
// cursor is scrolled out of view.
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

// drawKeyCursorRing paints the border highlight for the keyboard cursor.
//
// Without it the arrows moved a selection with no on-screen representation —
// the user was steering something invisible. Deliberately a RING rather than a
// fill so it reads as distinct from the active-instrument row highlight; both
// can be visible at once (the cursor parked on a row that is not the applied
// instrument is the normal state while arrowing).
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

// FilteredInstIDsForTest exposes the FULL filtered row list — the index space
// MenuKeyNav addresses. Distinct from VisibleInstIDs, which is the scrolled
// VIEWPORT slice; conflating the two is how a keyboard action lands on the
// wrong instrument once the list is scrolled.
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
			var id instrument.ID
			if i < len(m.instIDs) {
				id = m.instIDs[i]
			}
			m.drawInstRow(dst, btn, id)
		}
		m.drawFavoriteStars(dst)
		// Above the rows and stars so the ring is never half-covered, below the
		// scrollbar so it cannot paint over it.
		m.drawKeyCursorRing(dst)
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

// instMenuSwatchSz is the side length (in px) of the colored instrument
// swatch drawn to the left of each instrument label (spacing.swatch-md —
// the list-row step of the swatch scale; color IS the instrument's
// identity, so the chip must read at a glance).
const instMenuSwatchSz = SwatchSizeMD

// drawInstRow renders one instrument-list row through drawMenuRow: background
// accent tinted to the instrument's own color, keycap chrome, a colored swatch,
// fuzzy-match highlights, and the label. Pixel-identical to the previous
// hand-rolled body; routing through drawMenuRow unifies styling with every
// other overlay menu.
func (m *InstrumentMenuComponent) drawInstRow(dst *ebiten.Image, btn *Button, id instrument.ID) {
	m.drawInstRowTrail(dst, btn, id, "")
}

// drawInstRowTrail is drawInstRow with an optional right-aligned muted trail
// caption — the categories-mode global search uses it to show which category
// each matched instrument lives in.
func (m *InstrumentMenuComponent) drawInstRowTrail(dst *ebiten.Image, btn *Button, id instrument.ID, trail string) {
	if btn.Rect().Empty() {
		return
	}
	// Empty-state placeholder (no id, no action): a centered muted line per
	// the DESIGN.md Menu-anatomy empty-state pattern, not a fake list row.
	if id == "" && btn.OnClick == nil {
		drawMenuRow(dst, btn, MenuRowSpec{
			State:       menuItemRest,
			Label:       btn.Text,
			LabelColor:  colTextSecondary,
			CenterLabel: true,
		})
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
// accent, the label at SpaceMD, a trailing instrument-count caption, and a
// drill-in chevron so categories read as folders. The virtual Favorites row
// leads with a gold filled star. idx indexes m.categoryMeta (parallel to
// m.categoryBtns).
func (m *InstrumentMenuComponent) drawCategoryRow(dst *ebiten.Image, btn *Button, idx int) {
	if btn.Rect().Empty() {
		return
	}
	state := menuItemRest
	// "Active" is recorded in the row's meta, never inferred from style
	// identity: `btn.Style == PopupButtonStyle` broke on every runtime theme
	// change (ApplyTheme rebuilds PopupButtonStyle with new colors, so the
	// captured copy no longer compared equal and the active row lost its
	// highlight).
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

// compactCountLabel renders a list-size caption that stays glanceable for
// sample-bank-sized categories: exact below 1000, "7.9k" above.
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

// drawFavoriteStars renders the per-row star icons. Empty star (outline)
// for unfavorited items; filled star for favorited. favRects is row-indexed
// (see addInstRow): a star is drawn only for a row that exists in this
// frame's layout, and its id is that row's instIDs entry. Rows with a zero
// star rect — the empty-state placeholder, or every row when favorites are
// unwired — draw nothing.
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
		} else if m.state.activeCat != audio.CatNone {
			out = append(out, m.state.activeCat.Label())
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

// ActiveInstrumentID returns the instrument id the menu renders as the
// currently-selected ("active") row. The menu stays open after a selection so
// the user can audition multiple instruments; the active highlight must follow
// the LIVE selection so exactly one row ever reads as selected. drawInstRow and
// menuAccent both consult this so the test seam and the render path can never
// disagree.
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
