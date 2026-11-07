package ui

import (
	"image"
	"image/color"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/eventlogger"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// A large modal of the merged event stream + notifications with search, category chips, time filters and pages;
// rendering lives in the logs_component_*.go siblings.
type LogsComponent struct {
	overlayBase
	dv   *DrumView
	open bool

	// filter/search state
	nameQuery   string
	tsMode      timestampMode
	atSel       logsTimeSel
	fromSel     logsTimeSel
	toSel       logsTimeSel
	activeLevel logLevel // single-select level filter (mutually exclusive)
	page        int
	pageSize    int

	// derived each rebuild
	all            []logRow
	filtered       []logRow
	pageCountCache int

	// The same clicky MenuScroll as the template and instrument menus.
	scroll     *MenuScroll
	scrollView image.Rectangle

	// The newest event Seq and notification time seen at the last open.
	lastSeenSeq     int64
	lastSeenNotifMs int64

	// The badge asks every frame; the full ring scan reruns only when a ring's latest Seq or time advances.
	unreadValid    bool
	unreadCnt      int
	unreadErr      bool
	unreadKeySeq   int64
	unreadKeyNtPsh int64

	// No close ×: the logs grid button toggles the panel. The rects mirror the buttons for hit-test helpers.
	prevBtn, nextBtn *Button
	prevRect         image.Rectangle
	nextRect         image.Rectangle

	// Empties the log ring and the notifications, persistence included.
	clearBtn  *Button
	card      *MenuCard // shared frame (panel, title band, Clear history action, close ×)
	clearRect image.Rectangle

	// name search + level chips (fixed DEBUG/INFO/WARN/ERROR radio pill Buttons).
	searchBox  *TextInput
	searchRect image.Rectangle
	levelBtns  [len(logLevelOrder)]*Button

	// The picker is a stacked portal owned by the component.
	atModeBtn, rangeModeBtn              *Button
	atFieldBtn, fromFieldBtn, toFieldBtn *Button
	picker                               *logsDateTimePicker
}

// Pages bound the total; a page taller than the body scrolls within itself.
const logsPageSize = 50

func NewLogsComponent(dv *DrumView) *LogsComponent {
	return &LogsComponent{
		overlayBase: newOverlayBase(),
		dv:          dv,
		activeLevel: levelError,
		page:        1,
		pageSize:    logsPageSize,
		scroll:      NewMenuScroll(dropdownScrollbarStyle(), logsRowH()),
	}
}

// Reached from an error-focused icon, so the level filter starts on ERROR.
func (c *LogsComponent) Open() {
	c.open = true
	c.page = 1
	c.activeLevel = levelError
	c.rebuild()
	c.reapply()
}

// Close marks the panel closed.
func (c *LogsComponent) Close() { c.open = false }

// IsOpen reports whether the panel is open.
func (c *LogsComponent) IsOpen() bool { return c.open }

func (c *LogsComponent) markSeen() {
	if snap := eventlogger.History().Snapshot(); len(snap) > 0 {
		c.lastSeenSeq = snap[0].Seq // newest-first
	}
	if c.dv != nil && c.dv.notifStore != nil {
		if latest := c.dv.notifStore.Latest(); latest != nil {
			c.lastSeenNotifMs = latest.unixMs
		}
	}
	c.unreadValid = false // watermarks moved → recompute on next query
}

// Unread ERRORS only: the icon is an error indicator, so routine events never count. Memoized, since the badge
// asks every frame.
func (c *LogsComponent) unreadCount() (int, bool) {
	curSeq := eventlogger.History().LatestSeq()
	// The push counter is monotonic and survives eviction, so ANY new notification invalidates the cache.
	curPush := c.notifPushSeq()
	if c.unreadValid && curSeq == c.unreadKeySeq && curPush == c.unreadKeyNtPsh {
		return c.unreadCnt, c.unreadErr
	}

	n := 0
	if c.dv != nil && c.dv.notifStore != nil {
		for _, notif := range c.dv.notifStore.History() {
			if notif.isErr && notif.unixMs > c.lastSeenNotifMs {
				n++
			}
		}
	}
	hasErr := n > 0

	c.unreadValid, c.unreadCnt, c.unreadErr = true, n, hasErr
	c.unreadKeySeq, c.unreadKeyNtPsh = curSeq, curPush
	return n, hasErr
}

// 0 when absent; the eviction-proof change signal.
func (c *LogsComponent) notifPushSeq() int64 {
	if c.dv == nil || c.dv.notifStore == nil {
		return 0
	}
	return c.dv.notifStore.PushSeq()
}

func (c *LogsComponent) Update() {
	// rebuild recomputes the windowed rows for the advancing offset.
	if c.scroll != nil {
		c.scroll.Tick(c.rebuild)
	}
	if c.searchBox != nil {
		// The mobile native <input> writes out of band; pull its value first.
		if v, ok := mobileInputGetValue(logsSearchMobileInputID); ok && v != c.searchBox.Value() {
			c.searchBox.SetText(v)
		}
		c.searchBox.Update()
		if v := c.searchBox.Value(); v != c.nameQuery {
			c.applyNameQuery(v)
		}
	}
}

// The single chokepoint after ANY filter change: back to page 1 at the top, so the page count and viewport never
// go stale. Does not re-pull the snapshot; Draw's per-frame rebuild keeps the stream live.
func (c *LogsComponent) reapply() {
	c.filtered = filterRows(c.all, c.nameQuery, c.activeLevel, c.tsMode, c.atSel, c.fromSel, c.toSel)
	c.pageCountCache = pageCount(len(c.filtered), c.pageSize)
	c.page = 1
	c.scrollToTop()
}

// So new content never opens scrolled halfway down.
func (c *LogsComponent) scrollToTop() {
	if c.scroll != nil {
		c.scroll.ScrollBehavior().VS.First = 0
	}
}

// setLevel selects a single level (mutually exclusive) and re-applies filters.
func (c *LogsComponent) setLevel(l logLevel) {
	c.activeLevel = l
	c.reapply()
}

// applyNameQuery sets the fuzzy name/tag query and re-applies filters.
func (c *LogsComponent) applyNameQuery(q string) {
	c.nameQuery = q
	c.reapply()
}

// clearSearch wipes the query (Esc / trailing clear-×) and re-applies.
func (c *LogsComponent) clearSearch() {
	c.nameQuery = ""
	if c.searchBox != nil {
		c.searchBox.SetText("")
	}
	c.reapply()
}

// So the grid never acts on keys typed into the search box.
func (c *LogsComponent) ClaimsKeyboard() bool { return c.IsOpen() }

// An empty query returns false so the universal handler closes the panel.
func (c *LogsComponent) HandleEscape() bool {
	if c.nameQuery == "" {
		return false
	}
	c.clearSearch()
	return true
}

// notifInputs projects the notification store into read-time rows.
func (c *LogsComponent) notifInputs() []notifRowInput {
	if c.dv == nil || c.dv.notifStore == nil {
		return nil
	}
	out := make([]notifRowInput, 0, c.dv.notifStore.Len())
	for _, n := range c.dv.notifStore.History() { // newest-first
		out = append(out, notifRowInput{at: time.UnixMilli(n.unixMs), text: n.display(), isErr: n.isErr})
	}
	return out
}

func (c *LogsComponent) rebuild() {
	c.all = mergeLogRows(eventlogger.History().Snapshot(), c.notifInputs())
	c.filtered = filterRows(c.all, c.nameQuery, c.activeLevel, c.tsMode, c.atSel, c.fromSel, c.toSel)
	c.pageCountCache = pageCount(len(c.filtered), c.pageSize)
	if c.page > c.pageCountCache {
		c.page = c.pageCountCache
	}
	if c.page < 1 {
		c.page = 1
	}
}

// pageCountNow returns the cached page count from the last rebuild.
func (c *LogsComponent) pageCountNow() int { return c.pageCountCache }

// visibleRows returns the current page's rows.
func (c *LogsComponent) visibleRows() []logRow {
	return pageSlice(c.filtered, c.pageSize, c.page)
}

// Mobile, or no owner, falls back to the centred form.
func (c *LogsComponent) panelRect() image.Rectangle {
	b := c.dv.Bounds
	w := b.Dx() * 3 / 4
	h := b.Dy() * 3 / 4
	// Anchored to the grid-pane button on every screen size: centring in the drum pane put menus inside the drum view
	// on small screens.
	if g := c.dv.game; g != nil {
		if anchor := g.gridLogsButtonRect(); !anchor.Empty() {
			return g.gridAnchoredPopupRect(anchor, w, h, GridAnchorOpts{})
		}
	}
	cx := b.Min.X + b.Dx()/2
	cy := b.Min.Y + b.Dy()/2
	return image.Rect(cx-w/2, cy-h/2, cx+w/2, cy+h/2)
}

// InputBounds is the modal panel rect (the hit region while open).
func (c *LogsComponent) InputBounds() image.Rectangle { return c.panelRect() }

// While true the tree keeps routing OnDrag here and the portal stays open.
func (c *LogsComponent) Capturing() bool { return c.scrollCapturing() }

func (c *LogsComponent) scrollCapturing() bool {
	if c.scroll == nil {
		return false
	}
	sb := c.scroll.ScrollBehavior()
	return sb.Dragging() || sb.ScrollingCommitted() || c.scroll.TapActive()
}

// InputCaptured while the scroll owns a gesture, so the tree keeps sending OnDrag.
func (c *LogsComponent) routeScroll(p image.Point, pressed bool) InputResult {
	if c.scroll == nil {
		return InputIgnored
	}
	handled := c.scroll.HandleInput(MenuScrollInput{
		Pt:        p,
		Pressed:   pressed,
		PopupRect: c.panelRect(),
		ItemView:  c.scrollView,
		// Rows are not tappable widgets; a committed mobile tap fires nothing.
		FireTapAt: func(int, int) {},
	})
	if c.scrollCapturing() {
		return InputCaptured
	}
	if handled {
		return InputConsumed
	}
	return InputIgnored
}

// Page changes are the footer's job.
func (c *LogsComponent) HandleWheel(x, y, steps int) InputResult {
	if c.scroll != nil {
		c.scroll.HandleWheel(steps)
	}
	return InputConsumed
}

// logsRowH is the per-row height in the body.
func logsRowH() int { return TextHeight() + SpaceSM }

// Shared with the FX add-menu and the template menu.
func logsFooterH() int { return menuPagerFooterH() }

func logsBtnSize() int { return menuPagerBtnSize() }

// The new page opens at its first row.
func (c *LogsComponent) pagePrev() { c.page--; c.rebuild(); c.scrollToTop() }
func (c *LogsComponent) pageNext() { c.page++; c.rebuild(); c.scrollToTop() }

// Clears every source, writing an empty snapshot through persistence so a refresh stays cleared, and drops the
// unread memo so the grid icon reads 0.
func (c *LogsComponent) clearHistory() {
	eventlogger.History().Clear()
	if c.dv != nil && c.dv.notifStore != nil {
		c.dv.notifStore.Clear()
	}
	if c.dv != nil {
		c.dv.logsToast.dismiss() // drop any lingering error toast too
	}
	c.unreadValid = false // watermark memo must recompute against the empty rings
	c.rebuild()           // re-pull the now-empty sources into all/filtered
	c.reapply()           // reset page + scroll, recompute the (now 1) page count
}

// ensurePanelButtons lazily builds the close + page-nav + clear keycap buttons.
func (c *LogsComponent) ensurePanelButtons() {
	if c.prevBtn != nil {
		return
	}
	c.card = NewMenuCard(c.Close)
	c.prevBtn = NewSpecButton("", ComponentButtonSecondary, c.pagePrev)
	c.prevBtn.Icon = string(IconChevronLeft)
	c.nextBtn = NewSpecButton("", ComponentButtonSecondary, c.pageNext)
	c.nextBtn.Icon = string(IconChevronRight)
	// Text button; drawn with a locale-resolved label each frame (see Draw).
	c.clearBtn = NewSpecButton("", ComponentButtonSecondary, c.clearHistory)
}

// HoverAt refreshes the panel buttons' hover state (desktop hover tint).
func (c *LogsComponent) HoverAt(mx, my int) {
	if c.card != nil {
		c.card.HoverAt(mx, my)
	}
	for _, b := range []*Button{c.prevBtn, c.nextBtn, c.atModeBtn, c.rangeModeBtn, c.atFieldBtn, c.fromFieldBtn, c.toFieldBtn} {
		if b != nil {
			b.UpdateHover(mx, my)
		}
	}
	for _, b := range c.levelBtns {
		if b != nil {
			b.UpdateHover(mx, my)
		}
	}
}

// ensureLevelButtons lazily builds the fixed DEBUG/INFO/WARN/ERROR pill Buttons.
func (c *LogsComponent) ensureLevelButtons() {
	if c.levelBtns[0] != nil {
		return
	}
	for i, lvl := range logLevelOrder {
		l := lvl
		c.levelBtns[i] = NewSpecButton(lvl.label(), ComponentButtonSecondary, func() { c.setLevel(l) })
	}
}

// Everything is consumed, so a press never leaks beneath the modal.
func (c *LogsComponent) HandleInput(x, y int, pressed bool) InputResult {
	// Frame controls first (close ×, "Clear history" header action).
	if c.card != nil && c.IsOpen() {
		if r := c.card.HandleInput(x, y, pressed); r != InputIgnored {
			return InputConsumed
		}
	}
	p := image.Pt(x, y)

	// A scroll gesture owns every event FIRST, so a drag over a header button never re-triggers it.
	if c.scrollCapturing() {
		return c.routeScroll(p, pressed)
	}

	if pressed && c.searchBox != nil && !c.searchRect.Empty() && p.In(c.searchRect) {
		if c.searchBox.Value() != "" && menuSearchAdornmentsActive(c.searchBox) && p.In(menuSearchClearRect(c.searchBox)) {
			c.clearSearch()
		}
		return InputConsumed
	}
	// Press AND release reach every button so each one's press lifecycle resets; otherwise a button fires once
	// and goes dead.
	c.handleTimeInput(x, y, pressed)
	for _, b := range c.levelBtns {
		if b != nil {
			b.HandleInputResult(x, y, pressed)
		}
	}
	for _, b := range []*Button{c.prevBtn, c.nextBtn} {
		if b != nil {
			b.HandleInputResult(x, y, pressed)
		}
	}
	// A thumb press captures so the tree routes the drag back here; the catch-all consumes other in-panel taps.
	if res := c.routeScroll(p, pressed); res != InputIgnored {
		return res
	}
	return InputConsumed
}

func (c *LogsComponent) Draw(dst *ebiten.Image) {
	r := c.panelRect()
	c.ensurePanelButtons()
	// The label is set before Layout so the action button fits the active locale.
	c.clearBtn.Text = i18n.T(i18n.KeyLogsClearHistory)
	c.card.Layout(r, MenuCardSpec{Title: i18n.T(i18n.KeyLogs), IconID: IconLogs, Actions: []*Button{c.clearBtn}})
	c.card.Draw(dst)
	c.clearRect = c.clearBtn.Rect()

	searchH := TextHeight() + 2*SpaceSM
	searchY := c.card.TitleRect().Max.Y + SpaceSM
	c.searchRect = image.Rect(r.Min.X+SpaceMD, searchY, r.Min.X+r.Dx()/2, searchY+searchH)

	chipH := TextHeight() + 2*SpaceXS
	chipsY := searchY + searchH + SpaceSM

	rowH := logsRowH()
	bodyTop := chipsY + chipH + SpaceSM
	bodyBot := r.Max.Y - logsFooterH()

	// Rebuilt every frame: the stream is live.
	c.rebuild()

	// Search box (self-focusing TextInput; Update reads the keyboard).
	var reg *textFieldRegistry
	if c.dv != nil {
		reg = c.dv.textFields
	}
	c.searchBox = ensureMenuSearchBox(c.searchBox, c.searchRect, reg,
		TextFieldSpec{ID: logsSearchMobileInputID, Owner: "logs"})
	c.searchBox.MobileInputID = logsSearchMobileInputID
	drawMenuSearchBox(dst, c.searchBox)

	// Timestamp filter (At | Range toggle + time inputs) in the right half.
	c.drawTimeControls(dst, r, searchY, searchH)

	// Level chips (DEBUG/INFO/WARN/ERROR, single-select radio).
	c.drawLevelChips(dst, r, chipsY, chipH)

	// Timestamp, level, message left to right, so a long message clips at the gutter and never covers the time.
	c.scrollView = image.Rect(r.Min.X+SpaceMD, bodyTop, r.Max.X-SpaceSM, bodyBot)
	rows := c.visibleRows()
	visible := max(1, (bodyBot-bodyTop)/rowH)
	c.scroll.Configure(c.scrollView, len(rows), visible)
	first := c.scroll.ScrollBehavior().VS.First
	gutter := 0
	if c.scroll.HasScroll() {
		gutter = instMenuScrollBarWidth
	}
	textRight := r.Max.X - SpaceMD - gutter

	if len(rows) == 0 {
		DrawTextColorAt(dst, i18n.T(i18n.KeyNoLogs), r.Min.X+SpaceMD, bodyTop, colTextSecondary)
	}
	tsW := TextWidth("00:00:00.000") + SpaceMD
	lvlW := TextWidth("DEBUG") + SpaceMD
	for i := 0; i < visible && first+i < len(rows); i++ {
		row := rows[first+i]
		y := bodyTop + i*rowH
		x := r.Min.X + SpaceMD
		// 1. timestamp (fixed-left, muted).
		DrawTextColorAt(dst, row.at.Format("15:04:05.000"), x, y, colTextSecondary)
		// 2. level (colored by severity).
		DrawTextColorAt(dst, row.level.label(), x+tsW, y, logLevelColor(row.level))
		// 3. message (flows right; clipped to the scrollbar gutter).
		msg := clipTextToWidth(row.msg, textRight-(x+tsW+lvlW))
		DrawTextColorAt(dst, msg, x+tsW+lvlW, y, colTextPrimary)
	}
	c.scroll.Draw(dst)

	c.drawFooter(dst, r)
}

// logLevelColor maps a level to its row text color.
func logLevelColor(l logLevel) color.Color {
	switch l {
	case levelError:
		return colError
	case levelWarn:
		return colAccent
	case levelDebug:
		return colTextSecondary
	default:
		return colTextPrimary
	}
}

// drawLevelChips renders the fixed DEBUG/INFO/WARN/ERROR radio pills.
func (c *LogsComponent) drawLevelChips(dst *ebiten.Image, r image.Rectangle, y, h int) {
	c.ensureLevelButtons()
	x := r.Min.X + SpaceMD
	for i, lvl := range logLevelOrder {
		btn := c.levelBtns[i]
		w := TextWidth(lvl.label()) + 2*SpaceSM
		btn.SetRect(image.Rect(x, y, x+w, y+h))
		drawPillTabAt(dst, btn, c.activeLevel == lvl)
		x += w + SpaceXS
	}
}

func (c *LogsComponent) drawFooter(dst *ebiten.Image, r image.Rectangle) {
	label := i18n.Tf(i18n.KeyLogsPage, c.page, c.pageCountNow())
	c.prevRect, c.nextRect = drawMenuPagerFooter(dst, r, label, c.prevBtn, c.nextBtn)
}
