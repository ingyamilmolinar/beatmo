package ui

import (
	"image"
	"image/color"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/eventlogger"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// LogsComponent is the Logs Navigation Panel: a large modal overlay that lists
// the merged event stream + notifications with fuzzy name search, data-driven
// category chips, exact/range timestamp filters, and pagination. It implements
// compOverlayable and is hosted in the portal by DrumView.openLogsPanel.
//
// This file carries the component's state + lifecycle + panel geometry; the
// row/pagination rendering, search box, category chips, and timestamp controls
// are layered on in sibling methods (see logs_component_*.go).
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

	// scrollable list within the current page (reuses MenuScroll — the same
	// clicky scrollbar the template/instrument menus use). scrollView is the
	// list viewport; bodyRect its outer rect.
	scroll     *MenuScroll
	scrollView image.Rectangle

	// unread-badge watermarks: newest event Seq + newest notification timestamp
	// seen when the panel was last opened.
	lastSeenSeq     int64
	lastSeenNotifMs int64

	// unread-badge memoization: the badge is queried every frame, but the
	// underlying rings change rarely. Cache the (count, hasErr) result keyed by
	// the rings' cheap latest-Seq / latest-notif-ms, recomputing (the expensive
	// full-ring scan) only when one of those advances. markSeen invalidates it.
	unreadValid    bool
	unreadCnt      int
	unreadErr      bool
	unreadKeySeq   int64
	unreadKeyNtPsh int64

	// footer page-nav: real keycap Buttons. The rects mirror the buttons' bounds
	// (kept for hit-test helpers/tests). There is NO close (×) button — the logs
	// grid button toggles this panel closed.
	prevBtn, nextBtn *Button
	prevRect         image.Rectangle
	nextRect         image.Rectangle

	// header "Clear history" action button (empties the log ring + notifications,
	// incl. persistence). Its rect mirrors the button bounds for hit-test tests.
	clearBtn  *Button
	card      *MenuCard // shared frame (panel, title band, Clear history action, close ×)
	clearRect image.Rectangle

	// name search + level chips (fixed DEBUG/INFO/WARN/ERROR radio pill Buttons).
	searchBox  *TextInput
	searchRect image.Rectangle
	levelBtns  [len(logLevelOrder)]*Button

	// timestamp filter controls: At|Range toggle (pill buttons) + value fields
	// (keycap buttons that open the date/time picker). The picker is a stacked
	// portal owned by the component.
	atModeBtn, rangeModeBtn              *Button
	atFieldBtn, fromFieldBtn, toFieldBtn *Button
	picker                               *logsDateTimePicker
}

// NewLogsComponent constructs a closed logs panel bound to dv.
// logsPageSize is the fixed rows-per-page. Pages bound the total; a page taller
// than the body scrolls within itself via MenuScroll.
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

// Open marks the panel open and rebuilds its data snapshot. Because the panel is
// reached from an error-focused notification icon, the level filter defaults to
// ERROR (the ERROR chip pre-selected); other level chips are one click away.
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

// markSeen records the newest event Seq + notification timestamp as read,
// clearing the unread badge.
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

// unreadCount returns how many unread ERRORS the notification icon should badge,
// and whether any is an error (always true when n>0). The logs icon is an
// error-focused notification indicator: routine events/info are NOT counted —
// only error notifications newer than the last-seen watermark. Opening the panel
// (markSeen) clears it.
//
// Called every frame by the grid badge, so the result is memoized: the notif
// ring scan runs only when the rings' cheap latest-Seq / latest-notif-ms advance
// (or markSeen invalidates the cache).
func (c *LogsComponent) unreadCount() (int, bool) {
	curSeq := eventlogger.History().LatestSeq()
	// The notif push-counter is monotonic and survives ring eviction, so ANY new
	// notification (even two in the same millisecond, even after the ring fills)
	// changes the key and invalidates the cache.
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

// notifPushSeq returns the notification store's monotonic push counter (0 when
// absent) — the cheap, eviction-proof change signal for the unread memoization.
func (c *LogsComponent) notifPushSeq() int64 {
	if c.dv == nil || c.dv.notifStore == nil {
		return 0
	}
	return c.dv.notifStore.PushSeq()
}

// Update drives the search box (caret/typing) and re-applies filters when the
// query changes. The TextInput self-focuses on click and reads the keyboard.
func (c *LogsComponent) Update() {
	// Touch-fling momentum — the inertial half of the shared instrument-menu
	// feel. rebuild recomputes the windowed rows for the advancing offset.
	if c.scroll != nil {
		c.scroll.Tick(c.rebuild)
	}
	if c.searchBox != nil {
		// Mobile native <input> writes the query out-of-band (not the ebiten
		// keyboard the box reads); pull its live value first (mirrors the
		// instrument menu). No-op on desktop / when no native value is pending.
		if v, ok := mobileInputGetValue(logsSearchMobileInputID); ok && v != c.searchBox.Value() {
			c.searchBox.SetText(v)
		}
		c.searchBox.Update()
		if v := c.searchBox.Value(); v != c.nameQuery {
			c.applyNameQuery(v)
		}
	}
}

// reapply recomputes the filtered set + page count after ANY filter change
// (level, search, time) and snaps the view back to the start: page 1, scroll at
// the top. This is the single chokepoint that keeps the results — and the
// pagination "n / m" — in sync with the current filters in real time. Every
// filter mutator routes through here, so a new selection never leaves a stale
// page count or a scrolled-down viewport. Does NOT re-pull the snapshot (cheaper
// than rebuild; Draw's per-frame rebuild keeps the live stream current).
func (c *LogsComponent) reapply() {
	c.filtered = filterRows(c.all, c.nameQuery, c.activeLevel, c.tsMode, c.atSel, c.fromSel, c.toSel)
	c.pageCountCache = pageCount(len(c.filtered), c.pageSize)
	c.page = 1
	c.scrollToTop()
}

// scrollToTop snaps the list back to its first row. Called whenever the visible
// set changes wholesale (filter change via reapply, page change) so the new
// content never opens scrolled halfway down.
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

// ClaimsKeyboard makes the panel own the keyboard while open, so the grid never
// acts on keys typed into the search box.
func (c *LogsComponent) ClaimsKeyboard() bool { return c.IsOpen() }

// HandleEscape clears a non-empty query (keeping the panel open); with an empty
// query it returns false so the universal handler closes the panel.
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

// rebuild recomputes the merged snapshot, the distinct-tag set, the filtered
// rows, and the page count. Clamps page into range.
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

// panelRect returns the logs panel rect. Same size as before (3/4 of the drum
// pane); on desktop it now drops from the grid-pane logs button via the shared
// helper instead of centering in the drum pane. Mobile (UseBottomSheet) or a
// missing owner falls back to the centered form.
func (c *LogsComponent) panelRect() image.Rectangle {
	b := c.dv.Bounds
	w := b.Dx() * 3 / 4
	h := b.Dy() * 3 / 4
	// Anchor to the grid-pane logs button on EVERY screen size (grid-pane-clamped)
	// so it never falls back to centering in dv.Bounds (the drum pane) and
	// appearing in the drum-view panel — the small-screen "menus in the drum view"
	// bug. Logs lays its rows out within the returned rect, so a shorter grid pane
	// simply shows fewer rows + paginates.
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

// Capturing reports whether the scrollbar owns an in-flight gesture (thumb drag
// on desktop, touch-scroll on mobile). The tree keeps routing OnDrag to the panel
// while this is true, and the portal stays open. Mirrors InstrumentMenuComponent.
func (c *LogsComponent) Capturing() bool { return c.scrollCapturing() }

// scrollCapturing is true while the MenuScroll is mid-gesture: a desktop thumb
// drag, a committed mobile touch-scroll, or an in-flight deferred tap.
func (c *LogsComponent) scrollCapturing() bool {
	if c.scroll == nil {
		return false
	}
	sb := c.scroll.ScrollBehavior()
	return sb.Dragging() || sb.ScrollingCommitted() || c.scroll.TapActive()
}

// routeScroll forwards one event to the MenuScroll and returns the InputResult:
// InputCaptured while the scroll owns a gesture (so the tree keeps sending
// OnDrag), else InputConsumed when it merely handled/consumed the event, else
// InputIgnored when the scroll wanted nothing to do with it.
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

// HandleWheel scrolls the list within the current page (one item per step, the
// clicky MenuScroll behavior). Page changes are via the footer prev/next.
func (c *LogsComponent) HandleWheel(x, y, steps int) InputResult {
	if c.scroll != nil {
		c.scroll.HandleWheel(steps)
	}
	return InputConsumed
}

// logsRowH is the per-row height in the body.
func logsRowH() int { return TextHeight() + SpaceSM }

// logsFooterH reserves the pagination band at the bottom of the panel.
// Delegates to menuPagerFooterH (menu_pager.go) so logs, the FX add-menu, and
// the template menu share one single-source footer height.
func logsFooterH() int { return menuPagerFooterH() }

// pagePrev/pageNext change the page and snap the scroll back to the top so the
// new page always opens at its first row.
func (c *LogsComponent) pagePrev() { c.page--; c.rebuild(); c.scrollToTop() }
func (c *LogsComponent) pageNext() { c.page++; c.rebuild(); c.scrollToTop() }

// clearHistory empties EVERY log source behind the panel: the in-memory event
// ring (eventlogger) and the notification store — the latter also writing an
// empty snapshot through its persist sink so the cleared state survives a
// refresh. It then refreshes the view (empty on every level, page 1, scroll top)
// and invalidates the unread-badge memo so the grid Logs icon drops to 0.
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

// HandleInput handles clicks inside the modal panel: close button, page nav,
// else consume (so a press never leaks to the background beneath the modal).
func (c *LogsComponent) HandleInput(x, y int, pressed bool) InputResult {
	// Frame controls first (close ×, "Clear history" header action).
	if c.card != nil && c.IsOpen() {
		if r := c.card.HandleInput(x, y, pressed); r != InputIgnored {
			return InputConsumed
		}
	}
	p := image.Pt(x, y)

	// An in-flight scroll gesture (thumb drag / touch-scroll) owns every event
	// FIRST — checked before the buttons so a drag passing over a header button
	// can never re-trigger it. While capturing, the tree routes OnDrag straight
	// here (pressed=true), so this branch continues and ends the gesture.
	if c.scrollCapturing() {
		return c.routeScroll(p, pressed)
	}

	// Search field: on press only; a click on the trailing clear-× wipes the
	// query. (TextInput self-focuses on click via Update.)
	if pressed && c.searchBox != nil && !c.searchRect.Empty() && p.In(c.searchRect) {
		if c.searchBox.Value() != "" && menuSearchAdornmentsActive(c.searchBox) && p.In(menuSearchClearRect(c.searchBox)) {
			c.clearSearch()
		}
		return InputConsumed
	}
	// Route BOTH press and release to every button so their press-lifecycle
	// (held counter / pressed flag) resets on release — otherwise a button fires
	// its OnClick only once and then goes dead (the close-button regression).
	// OnClick fires on the press edge of the button under the cursor; buttons
	// elsewhere just reset. No early return, so all buttons see every event.
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
	// Scrollbar thumb press / mobile touch-begin (reuses MenuScroll — the same
	// clicky scrollbar the template/instrument menus use). A press on the thumb
	// starts a drag and returns InputCaptured so the tree keeps routing the drag
	// back here; the trailing popup catch-all consumes any other in-panel tap.
	if res := c.routeScroll(p, pressed); res != InputIgnored {
		return res
	}
	return InputConsumed
}

// Draw paints the modal panel: surface, header (title, close, search box, level
// chips), the scrollable current page of rows, and the pagination footer.
func (c *LogsComponent) Draw(dst *ebiten.Image) {
	r := c.panelRect()
	// Shared panel chrome (shadow + surface-overlay fill + panel border +
	// popupCornerRadius) — identical to File and Settings.
	// --- Header layout (card title band, search row, chip row) ---
	c.ensurePanelButtons()
	// Shared MenuCard frame: panel, title band (IconLogs + "Logs"), the
	// "Clear history" header action, close ×. Label set before Layout so the
	// action button is sized for the active locale.
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

	// Refresh the merged snapshot + filtered rows each frame (live stream). The
	// page size is fixed (logsPageSize); a page taller than the body scrolls.
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

	// Body: scrollable list of the current page's rows. Reserve a scrollbar
	// gutter on the right. Rows: TIMESTAMP · LEVEL · message (left→right, so a
	// long message flows right and clips at the gutter — never overlaps the
	// timestamp on its left).
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

// drawFooter paints the shared "Page X / M" navigator (extracted to
// drawMenuPagerFooter so logs, the FX add-menu, and the template menu share one
// width-adaptive pager). Records prev/next hit rects.
func (c *LogsComponent) drawFooter(dst *ebiten.Image, r image.Rectangle) {
	label := i18n.Tf(i18n.KeyLogsPage, c.page, c.pageCountNow())
	c.prevRect, c.nextRect = drawMenuPagerFooter(dst, r, label, c.prevBtn, c.nextBtn)
}
