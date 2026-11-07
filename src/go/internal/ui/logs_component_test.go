//go:build test

package ui

import (
	"fmt"
	"image"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/eventlogger"
)

// Errors land on the ERROR-default filter; one drawn frame configures the MenuScroll so its thumb geometry is
// live. Fails unless the list overflows.
func openLogsPanelWithErrors(t *testing.T, n int) (*Game, *LogsComponent, *ebiten.Image) {
	t.Helper()
	setLogRing(t, n+50)
	g := newGameWithStart(t)
	for i := 0; i < n; i++ {
		g.drum.notifyError(fmt.Sprintf("boom %d", i))
	}
	c := g.drum.ensureLogsComp()
	c.Open()
	dst := ebiten.NewImage(1280, 720)
	c.Draw(dst) // lay out + Configure the scroll
	if !c.scroll.HasScroll() {
		t.Fatalf("need list overflow to exercise the scrollbar; HasScroll=false with %d rows", n)
	}
	return g, c, dst
}

// Input is newest-last; the model sorts newest-first.
func seedInfoRing(t *testing.T, n int) {
	t.Helper()
	ring := eventlogger.NewRingHistory(n + 100)
	prev := eventlogger.History()
	t.Cleanup(func() { eventlogger.SetLogHistory(prev) })
	eventlogger.SetLogHistory(ring)
	base := time.Date(2026, 7, 20, 15, 0, 0, 0, time.Local)
	for i := 0; i < n; i++ {
		ring.Record(eventlogger.LogRecord{
			Seq: int64(i), At: base.Add(time.Duration(i) * time.Second),
			Tag: "node", Msg: fmt.Sprintf("info %d", i),
		})
	}
}

func TestLogsPageCountMatchesRows(t *testing.T) {
	for _, n := range []int{0, 1, 49, 50, 51, 99, 100, 101, 250} {
		seedInfoRing(t, n)
		g := newGameWithStart(t)
		c := g.drum.ensureLogsComp()
		c.Open()
		c.setLevel(levelInfo)
		dst := ebiten.NewImage(1280, 720)
		c.Draw(dst)
		want := (max(n, 1) + c.pageSize - 1) / c.pageSize
		if n == 0 {
			want = 1
		}
		if got := c.pageCountNow(); got != want {
			t.Fatalf("n=%d: pageCount=%d want %d (filtered=%d pageSize=%d)",
				n, got, want, len(c.filtered), c.pageSize)
		}
	}
}

// Guards "m says 5 but the pages are empty".
func TestLogsEveryPageHasContent(t *testing.T) {
	seedInfoRing(t, 220) // 5 pages of 50
	g := newGameWithStart(t)
	c := g.drum.ensureLogsComp()
	c.Open()
	c.setLevel(levelInfo)
	dst := ebiten.NewImage(1280, 720)
	c.Draw(dst)
	m := c.pageCountNow()
	if m < 2 {
		t.Fatalf("expected multiple pages, got %d", m)
	}
	for p := 1; p <= m; p++ {
		c.page = p
		if rows := c.visibleRows(); len(rows) == 0 {
			t.Fatalf("page %d/%d is empty — pagination overstates the page count", p, m)
		}
	}
}

func TestLogsPagesCoverAllRowsNoSkips(t *testing.T) {
	seedInfoRing(t, 213)
	g := newGameWithStart(t)
	c := g.drum.ensureLogsComp()
	c.Open()
	c.setLevel(levelInfo)
	dst := ebiten.NewImage(1280, 720)
	c.Draw(dst)
	m := c.pageCountNow()
	var seen int
	for p := 1; p <= m; p++ {
		c.page = p
		rows := c.visibleRows()
		for i, r := range rows {
			if r.msg != c.filtered[seen].msg {
				t.Fatalf("page %d row %d = %q but filtered[%d] = %q — pages don't tile the set in order",
					p, i, r.msg, seen, c.filtered[seen].msg)
			}
			seen++
		}
	}
	if seen != len(c.filtered) {
		t.Fatalf("pages covered %d rows but filtered has %d — rows are skipped/duplicated", seen, len(c.filtered))
	}
}

func TestLogsNextReachesLastPageThenClamps(t *testing.T) {
	seedInfoRing(t, 220)
	g := newGameWithStart(t)
	c := g.drum.ensureLogsComp()
	c.Open()
	c.setLevel(levelInfo)
	dst := ebiten.NewImage(1280, 720)
	c.Draw(dst)
	m := c.pageCountNow()
	for p := 1; p < m; p++ {
		if c.page != p {
			t.Fatalf("expected page %d, at %d", p, c.page)
		}
		nx, ny := rectCenter(c.nextRect)
		c.HandleInput(nx, ny, true)
		c.HandleInput(nx, ny, false)
		c.Draw(dst)
	}
	if c.page != m {
		t.Fatalf("next did not reach last page: at %d want %d", c.page, m)
	}
	// One more next must clamp (button greys at the last page).
	nx, ny := rectCenter(c.nextRect)
	c.HandleInput(nx, ny, true)
	c.HandleInput(nx, ny, false)
	c.Draw(dst)
	if c.page != m {
		t.Fatalf("next past last page must clamp at %d, went to %d", m, c.page)
	}
}

// m comes from the NEW level's rows and the page resets to 1, never the previous level's count.
func TestLogsLevelSwitchUpdatesPageCount(t *testing.T) {
	seedInfoRing(t, 220) // 220 INFO events → 5 pages on INFO
	g := newGameWithStart(t)
	for i := 0; i < 60; i++ { // 60 error notifs → 2 pages on ERROR
		g.drum.notifyError(fmt.Sprintf("boom %d", i))
	}
	c := g.drum.ensureLogsComp()
	c.Open()
	dst := ebiten.NewImage(1280, 720)

	c.setLevel(levelInfo)
	c.Draw(dst)
	if got := c.pageCountNow(); got != 5 {
		t.Fatalf("INFO pageCount=%d want 5 (filtered=%d)", got, len(c.filtered))
	}
	c.page = 4
	c.setLevel(levelError)
	c.Draw(dst)
	if got := c.pageCountNow(); got != 2 {
		t.Fatalf("after switch to ERROR: pageCount=%d want 2 (filtered=%d) — m shows the stale INFO count", got, len(c.filtered))
	}
	if c.page != 1 {
		t.Fatalf("level switch must reset to page 1, at %d", c.page)
	}
}

// The push counter bumps so memoising consumers invalidate, and an EMPTY snapshot is persisted so the clear
// survives a refresh.
func TestNotificationStoreClearPersistsEmpty(t *testing.T) {
	s := newNotificationStore(100)
	var persisted []notification
	persistCalls := 0
	s.persist = func(snap []notification) { persisted = snap; persistCalls++ }
	s.Push(notification{text: "a"})
	s.Push(notification{text: "b"})
	beforeSeq := s.PushSeq()

	s.Clear()

	if s.Len() != 0 {
		t.Fatalf("after Clear, Len=%d want 0", s.Len())
	}
	if s.Latest() != nil {
		t.Fatal("after Clear, Latest must be nil")
	}
	if s.PushSeq() <= beforeSeq {
		t.Fatalf("Clear must advance PushSeq (was %d, now %d) so memo consumers invalidate", beforeSeq, s.PushSeq())
	}
	if persistCalls == 0 {
		t.Fatal("Clear must write through the persist sink (to erase persisted history)")
	}
	if len(persisted) != 0 {
		t.Fatalf("Clear must persist an EMPTY snapshot, got %d entries", len(persisted))
	}
}

func TestLogsClearHistoryEmptiesEverything(t *testing.T) {
	seedInfoRing(t, 120)
	g := newGameWithStart(t)
	for i := 0; i < 30; i++ {
		g.drum.notifyError(fmt.Sprintf("boom %d", i))
	}
	c := g.drum.ensureLogsComp()
	c.Open()
	dst := ebiten.NewImage(1280, 720)
	c.Draw(dst)
	// Precondition: there IS history.
	if n, _ := c.unreadCount(); n == 0 {
		t.Fatal("precondition: expected unread errors before clear")
	}

	c.clearHistory()
	c.Draw(dst)

	if len(c.all) != 0 || len(c.filtered) != 0 {
		t.Fatalf("after clearHistory: all=%d filtered=%d want 0/0", len(c.all), len(c.filtered))
	}
	if c.pageCountNow() != 1 || c.page != 1 {
		t.Fatalf("after clear: page=%d/%d want 1/1", c.page, c.pageCountNow())
	}
	// Every level must be empty.
	for _, lvl := range logLevelOrder {
		c.setLevel(lvl)
		c.Draw(dst)
		if len(c.filtered) != 0 {
			t.Fatalf("after clear, level %d still has %d rows", lvl, len(c.filtered))
		}
	}
	if n, hasErr := c.unreadCount(); n != 0 || hasErr {
		t.Fatalf("after clear, unread badge must be 0/false, got %d/%v", n, hasErr)
	}
	if g.drum.notifStore.Len() != 0 {
		t.Fatalf("after clear, notifStore still has %d entries", g.drum.notifStore.Len())
	}
	if len(eventlogger.History().Snapshot()) != 0 {
		t.Fatal("after clear, event ring must be empty")
	}
	if g.drum.logsToastActive() {
		t.Fatal("after clear, the transient error toast must be dismissed")
	}
}

// Re-seeding from the persisted snapshot is what a page refresh does.
func TestLogsClearHistorySurvivesRefresh(t *testing.T) {
	setLogRing(t, 200)
	g := newGameWithStart(t)
	// Wire a fake persistence backend to capture what a refresh would reload.
	var persisted []notification
	g.drum.notifStore.persist = func(snap []notification) {
		persisted = append(persisted[:0], snap...)
	}
	for i := 0; i < 20; i++ {
		g.drum.notifyError(fmt.Sprintf("boom %d", i))
	}
	c := g.drum.ensureLogsComp()
	c.Open()

	c.clearHistory()

	// Simulate a refresh: a fresh store seeded from persistence.
	fresh := newNotificationStore(notifHistoryCap)
	fresh.seed(persisted)
	if fresh.Len() != 0 {
		t.Fatalf("after refresh, reseeded store has %d entries — clear did not survive persistence", fresh.Len())
	}
}

func TestLogsClearButtonClickClears(t *testing.T) {
	seedInfoRing(t, 80)
	g := newGameWithStart(t)
	for i := 0; i < 20; i++ {
		g.drum.notifyError(fmt.Sprintf("boom %d", i))
	}
	c := g.drum.ensureLogsComp()
	c.Open()
	dst := ebiten.NewImage(1280, 720)
	c.Draw(dst) // lays out clearRect
	if c.clearRect.Empty() {
		t.Fatal("clear button rect not laid out")
	}
	cx, cy := rectCenter(c.clearRect)
	c.HandleInput(cx, cy, true)
	c.HandleInput(cx, cy, false)
	c.Draw(dst)
	if len(c.filtered) != 0 || g.drum.notifStore.Len() != 0 || len(eventlogger.History().Snapshot()) != 0 {
		t.Fatalf("clear button did not clear history: filtered=%d notif=%d ring=%d",
			len(c.filtered), g.drum.notifStore.Len(), len(eventlogger.History().Snapshot()))
	}
}

// After a level chip, a search edit and clearing the search, the page count, page and scroll are in sync with
// the results in the same frame: a filter that shrinks the set shrinks m too.
func TestLogsPaginationRefreshesInRealTime(t *testing.T) {
	seedInfoRing(t, 220) // 5 pages on INFO
	g := newGameWithStart(t)
	for i := 0; i < 60; i++ { // 2 pages on ERROR
		g.drum.notifyError(fmt.Sprintf("boom %d", i))
	}
	c := g.drum.ensureLogsComp()
	c.Open()
	dst := ebiten.NewImage(1280, 720)

	// Start on INFO, deep in the list and scrolled.
	c.setLevel(levelInfo)
	c.page = 4
	c.scroll.ScrollBehavior().VS.First = 5
	c.Draw(dst)
	if c.pageCountNow() != 5 {
		t.Fatalf("setup: INFO pageCount=%d want 5", c.pageCountNow())
	}

	ex, ey := rectCenter(c.levelBtns[levelError].Rect())
	c.HandleInput(ex, ey, true)
	c.HandleInput(ex, ey, false)
	c.Draw(dst)
	if c.activeLevel != levelError {
		t.Fatalf("ERROR chip click did not switch level (activeLevel=%d)", c.activeLevel)
	}
	if got := c.pageCountNow(); got != 2 {
		t.Fatalf("after ERROR chip: pageCount=%d want 2 (stale m — pagination did not refresh)", got)
	}
	if c.page != 1 {
		t.Fatalf("filter change must reset to page 1, at %d", c.page)
	}
	if got := c.scroll.ScrollBehavior().VS.First; got != 0 {
		t.Fatalf("filter change must reset scroll to top, VS.First=%d", got)
	}

	// 2) Search narrows the ERROR set to a single match — m must become 1.
	c.page = 2
	c.scroll.ScrollBehavior().VS.First = 1
	c.searchBox.SetText("boom 7")
	c.Update() // applyNameQuery fires from the changed search value
	c.Draw(dst)
	if got := c.pageCountNow(); got != 1 {
		t.Fatalf("after search 'boom 7': pageCount=%d want 1 (filtered=%d)", got, len(c.filtered))
	}
	if c.page != 1 || c.scroll.ScrollBehavior().VS.First != 0 {
		t.Fatalf("search must reset page+scroll (page=%d first=%d)", c.page, c.scroll.ScrollBehavior().VS.First)
	}

	// 3) Clearing the search restores the full ERROR set — m back to 2.
	c.clearSearch()
	c.Draw(dst)
	if got := c.pageCountNow(); got != 2 {
		t.Fatalf("after clearing search: pageCount=%d want 2", got)
	}
}

// HandleWheel arms a per-notch cooldown released only by TickStep each frame; a scroll not registered with
// tickMenuScrollCooldowns advances one item and locks.
func TestLogsWheelScrollsRepeatedlyAfterTick(t *testing.T) {
	g, c, dst := openLogsPanelWithErrors(t, 40)

	// First notch scrolls one item.
	c.HandleWheel(0, 0, -1)
	c.Draw(dst)
	if got := c.scroll.ScrollBehavior().VS.First; got != 1 {
		t.Fatalf("first wheel notch: VS.First=%d want 1", got)
	}

	// Real Game.Update frames, the path that ticks the cooldown in production.
	for i := 0; i < controlGridScrollCooldownFrames+1; i++ {
		g.Update()
	}

	// Second notch must scroll again.
	c.HandleWheel(0, 0, -1)
	c.Draw(dst)
	if got := c.scroll.ScrollBehavior().VS.First; got != 2 {
		t.Fatalf("second wheel notch after cooldown: VS.First=%d want 2 "+
			"(cooldown never released — logs scroll not ticked in tickMenuScrollCooldowns)", got)
	}
}

// A thumb press must CAPTURE, or the tree never routes the drag to the panel.
func TestLogsScrollbarThumbDragCapturesAndMoves(t *testing.T) {
	_, c, dst := openLogsPanelWithErrors(t, 40)

	thumb := c.scroll.ScrollBehavior().ThumbRect()
	if thumb.Empty() {
		t.Fatal("thumb rect empty despite HasScroll")
	}
	tx, ty := rectCenter(thumb)

	// Press on the thumb must CAPTURE.
	if res := c.HandleInput(tx, ty, true); res != InputCaptured {
		t.Fatalf("press on scrollbar thumb: got %v want InputCaptured", res)
	}
	if !c.Capturing() {
		t.Fatal("Capturing() must be true while the thumb is held")
	}

	// Drag to the bottom of the list viewport — the list must advance.
	c.HandleInput(tx, c.scrollView.Max.Y-1, true)
	c.Draw(dst)
	if got := c.scroll.ScrollBehavior().VS.First; got == 0 {
		t.Fatalf("thumb drag to the bottom did not advance the list (VS.First=%d)", got)
	}

	// Release ends the capture.
	c.HandleInput(tx, c.scrollView.Max.Y-1, false)
	if c.Capturing() {
		t.Fatal("release must end the thumb-drag capture")
	}
}

func TestLogsScrollNoOpWhenContentFits(t *testing.T) {
	setLogRing(t, 50)
	g := newGameWithStart(t)
	g.drum.notifyError("only one")
	c := g.drum.ensureLogsComp()
	c.Open()
	dst := ebiten.NewImage(1280, 720)
	c.Draw(dst)
	if c.scroll.HasScroll() {
		t.Fatal("a single row must not produce a scrollbar")
	}
	c.HandleWheel(0, 0, -1)
	c.Draw(dst)
	if got := c.scroll.ScrollBehavior().VS.First; got != 0 {
		t.Fatalf("wheel with no overflow must be inert, VS.First=%d", got)
	}
	if c.Capturing() {
		t.Fatal("Capturing() must be false when nothing is scrolling")
	}
}

// Only an active scroll gesture captures.
func TestLogsCapturingFalseAtRestAfterTap(t *testing.T) {
	_, c, _ := openLogsPanelWithErrors(t, 40)
	// Tap the INFO level chip (a header button), press then release.
	if b := c.levelBtns[levelInfo]; b != nil {
		bx, by := rectCenter(b.Rect())
		c.HandleInput(bx, by, true)
		c.HandleInput(bx, by, false)
	}
	if c.Capturing() {
		t.Fatal("Capturing() must be false after a plain button tap")
	}
}

// A new page never opens scrolled halfway down.
func TestLogsPageSwitchResetsScroll(t *testing.T) {
	// Two pages of 50, and page 1 overflows so the scroll can be non-zero.
	_, c, dst := openLogsPanelWithErrors(t, 80)
	if c.pageCountNow() < 2 {
		t.Fatalf("need >1 page to switch; got %d", c.pageCountNow())
	}

	// Scroll partway down page 1.
	c.scroll.ScrollBehavior().VS.First = 5
	c.Draw(dst)
	if c.scroll.ScrollBehavior().VS.First == 0 {
		t.Fatal("precondition: scroll should be non-zero before the page switch")
	}

	// Next page must open at the top.
	c.pageNext()
	c.Draw(dst)
	if got := c.scroll.ScrollBehavior().VS.First; got != 0 {
		t.Fatalf("next-page must reset scroll to top, VS.First=%d want 0", got)
	}

	// Scroll partway down page 2, then go back — prev must also reset to the top.
	c.scroll.ScrollBehavior().VS.First = 3
	c.Draw(dst)
	c.pagePrev()
	c.Draw(dst)
	if got := c.scroll.ScrollBehavior().VS.First; got != 0 {
		t.Fatalf("prev-page must reset scroll to top, VS.First=%d want 0", got)
	}
}

func TestLogsWheelClampsAtEnds(t *testing.T) {
	g, c, dst := openLogsPanelWithErrors(t, 40)

	// Wheel up at the top is a no-op.
	c.HandleWheel(0, 0, 1)
	c.Draw(dst)
	if got := c.scroll.ScrollBehavior().VS.First; got != 0 {
		t.Fatalf("wheel-up at top must stay at 0, got %d", got)
	}

	// Scroll to the bottom, one notch per cooldown window, then one extra past it.
	sb := c.scroll.ScrollBehavior()
	maxFirst := sb.VS.Total - sb.VS.Visible
	for i := 0; i < maxFirst+3; i++ {
		c.HandleWheel(0, 0, -1)
		c.Draw(dst)
		for j := 0; j < controlGridScrollCooldownFrames+1; j++ {
			g.Update()
		}
	}
	if got := c.scroll.ScrollBehavior().VS.First; got != maxFirst {
		t.Fatalf("wheel-down must clamp at maxFirst=%d, got %d", maxFirst, got)
	}
}

func rectCenter(r image.Rectangle) (int, int) {
	return r.Min.X + r.Dx()/2, r.Min.Y + r.Dy()/2
}

func setLogRing(t *testing.T, cap int) eventlogger.LogHistory {
	t.Helper()
	prev := eventlogger.History()
	t.Cleanup(func() { eventlogger.SetLogHistory(prev) })
	ring := eventlogger.NewRingHistory(cap)
	eventlogger.SetLogHistory(ring)
	return ring
}

func TestLogsComponentRebuildPaginates(t *testing.T) {
	ring := setLogRing(t, 100)
	base := time.Date(2026, 7, 19, 12, 0, 0, 0, time.Local)
	for i := 0; i < 25; i++ {
		ring.Record(eventlogger.LogRecord{Seq: int64(i), At: base.Add(time.Duration(i) * time.Second), Tag: "node", Msg: "add node"})
	}
	g := newGameWithStart(t)
	c := NewLogsComponent(g.drum)
	c.activeLevel = levelInfo // events are INFO-level (default filter is ERROR)
	c.pageSize = 10           // deterministic for the test
	c.rebuild()
	if got := c.pageCountNow(); got != 3 {
		t.Fatalf("pageCount=%d want 3", got)
	}
	if got := len(c.visibleRows()); got != 10 {
		t.Fatalf("page 1 rows=%d want 10", got)
	}
	c.page = 3
	if got := len(c.visibleRows()); got != 5 {
		t.Fatalf("page 3 rows=%d want 5", got)
	}
}

// Draw populates the prev/next hit rects that HandleInput then tests.
func TestLogsComponentDrawThenClickHitRects(t *testing.T) {
	ring := setLogRing(t, 500)
	base := time.Date(2026, 7, 19, 12, 0, 0, 0, time.Local)
	for i := 0; i < 200; i++ {
		ring.Record(eventlogger.LogRecord{Seq: int64(i), At: base.Add(time.Duration(i) * time.Second), Tag: "node", Msg: "add node"})
	}
	g := newGameWithStart(t)
	c := g.drum.ensureLogsComp()
	c.Open()
	c.activeLevel = levelInfo // events are INFO-level (Open defaulted to ERROR)
	dst := ebiten.NewImage(1280, 720)
	c.Draw(dst) // rebuilds + populates close/prev/next hit rects

	if c.pageCountNow() < 2 {
		t.Fatalf("need >1 page to test next; got %d (pageSize=%d)", c.pageCountNow(), c.pageSize)
	}
	// Next page.
	nx, ny := rectCenter(c.nextRect)
	c.HandleInput(nx, ny, true)
	if c.page != 2 {
		t.Fatalf("next click: page=%d want 2", c.page)
	}
	// Prev page back to 1.
	px, py := rectCenter(c.prevRect)
	c.HandleInput(px, py, true)
	if c.page != 1 {
		t.Fatalf("prev click: page=%d want 1", c.page)
	}
	// The panel has no ×; the grid logs button toggles it.
}

func TestLogsSearchNameFiltersList(t *testing.T) {
	g := newGameWithStart(t)
	c := NewLogsComponent(g.drum)
	c.activeLevel = levelInfo
	c.all = []logRow{
		{level: levelInfo, tag: "node", msg: "add node", at: logTm(1, 0, 0)},
		{level: levelInfo, tag: "play", msg: "play started", at: logTm(1, 0, 1)},
	}
	c.applyNameQuery("play")
	if len(c.filtered) != 1 || c.filtered[0].tag != "play" {
		t.Fatalf("search failed: %+v", c.filtered)
	}
}

func TestLogsLevelChipClickSelects(t *testing.T) {
	setLogRing(t, 50)
	g := newGameWithStart(t)
	c := g.drum.ensureLogsComp()
	c.Open() // defaults to ERROR
	if c.activeLevel != levelError {
		t.Fatalf("default level = %v want ERROR", c.activeLevel)
	}
	dst := ebiten.NewImage(1280, 720)
	c.Draw(dst) // lays out the level chips
	// Click the INFO chip.
	var infoBtn *Button
	for i, lvl := range logLevelOrder {
		if lvl == levelInfo {
			infoBtn = c.levelBtns[i]
		}
	}
	r := infoBtn.Rect()
	if r.Empty() {
		t.Fatal("INFO chip has no rect")
	}
	c.HandleInput(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2, true)
	if c.activeLevel != levelInfo {
		t.Fatalf("clicking INFO chip: activeLevel=%v want INFO", c.activeLevel)
	}
}

func TestLogsLevelFilter(t *testing.T) {
	g := newGameWithStart(t)
	c := NewLogsComponent(g.drum)
	c.all = []logRow{
		{level: levelInfo, msg: "a", at: logTm(1, 0, 0)},
		{level: levelError, msg: "b", at: logTm(1, 0, 1)},
		{level: levelInfo, msg: "c", at: logTm(1, 0, 2)},
	}
	c.setLevel(levelError)
	if len(c.filtered) != 1 || c.filtered[0].msg != "b" {
		t.Fatalf("ERROR level filter: %+v", c.filtered)
	}
	c.setLevel(levelInfo)
	if len(c.filtered) != 2 {
		t.Fatalf("INFO level filter: %+v", c.filtered)
	}
	c.setLevel(levelWarn) // no WARN rows → empty
	if len(c.filtered) != 0 {
		t.Fatalf("WARN level filter should be empty: %+v", c.filtered)
	}
}

func TestLogsAtFilterViaSetTimeSel(t *testing.T) {
	g := newGameWithStart(t)
	c := NewLogsComponent(g.drum)
	c.activeLevel = levelDebug // rows below carry the zero-value level; filter to match
	c.all = []logRow{
		{tag: "a", msg: "1", at: logTm(12, 0, 0)},
		{tag: "a", msg: "2", at: logTm(12, 5, 0)},
		{tag: "a", msg: "3", at: logTm(12, 10, 0)},
	}
	// At the 12:05 minute selects only "2".
	c.setTimeMode(tsModeAt)
	c.setTimeSel(pickAt, logsTimeSel{set: true, t: logTm(12, 5, 0)})
	if len(c.filtered) != 1 || c.filtered[0].msg != "2" {
		t.Fatalf("At filter failed: %+v", c.filtered)
	}
	// Range 12:00–12:05 selects two.
	c.setTimeMode(tsModeRange)
	c.setTimeSel(pickFrom, logsTimeSel{set: true, t: logTm(12, 0, 0)})
	c.setTimeSel(pickTo, logsTimeSel{set: true, t: logTm(12, 5, 0)})
	if len(c.filtered) != 2 {
		t.Fatalf("Range filter failed: %+v", c.filtered)
	}
	// Clearing At reverts to no filter.
	c.setTimeMode(tsModeAt)
	c.setTimeSel(pickAt, logsTimeSel{})
	if len(c.filtered) != 3 {
		t.Fatalf("cleared At should show all: %+v", c.filtered)
	}
}

func TestLogsRangeFilterSpansTwoDays(t *testing.T) {
	g := newGameWithStart(t)
	c := NewLogsComponent(g.drum)
	c.activeLevel = levelDebug // rows below carry the zero-value level; filter to match
	d := func(day, h int) time.Time { return time.Date(2026, 7, day, h, 0, 0, 0, time.Local) }
	c.all = []logRow{
		{tag: "a", msg: "jul18-08", at: d(18, 8)},
		{tag: "a", msg: "jul18-20", at: d(18, 20)},
		{tag: "a", msg: "jul19-10", at: d(19, 10)},
		{tag: "a", msg: "jul20-06", at: d(20, 6)},
	}
	// The same clock times on other days are excluded: the range is DATE-aware.
	c.setTimeMode(tsModeRange)
	c.setTimeSel(pickFrom, logsTimeSel{set: true, t: d(18, 9)})
	c.setTimeSel(pickTo, logsTimeSel{set: true, t: d(19, 17)})
	if len(c.filtered) != 2 {
		t.Fatalf("range across days: got %d rows, want 2: %+v", len(c.filtered), c.filtered)
	}
	msgs := map[string]bool{}
	for _, r := range c.filtered {
		msgs[r.msg] = true
	}
	if !msgs["jul18-20"] || !msgs["jul19-10"] {
		t.Fatalf("wrong rows in range: %+v", c.filtered)
	}
}

func TestLogsFieldButtonOpensPicker(t *testing.T) {
	setLogRing(t, 10)
	g := newGameWithStart(t)
	c := g.drum.ensureLogsComp()
	c.Open()
	dst := ebiten.NewImage(1280, 720)
	c.Draw(dst) // lays out the At field button
	r := c.atFieldBtn.Rect()
	if r.Empty() {
		t.Fatal("At field button not laid out")
	}
	c.HandleInput(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2, true)
	if !c.IsDatePickerOpen() {
		t.Fatal("clicking the At field button should open the date/time picker portal")
	}
	// Drive the picker to commit a moment; the field + filter update.
	c.picker.selectDay(logTm(12, 0, 0))
	c.picker.setHour(9)
	c.picker.setMinute(30)
	c.picker.commitSet()
	if !c.atSel.set || c.atSel.t.Hour() != 9 || c.atSel.t.Minute() != 30 {
		t.Fatalf("picker Set should update atSel: %+v", c.atSel)
	}
}

func TestLogsToggleClickSwitchesMode(t *testing.T) {
	setLogRing(t, 10)
	g := newGameWithStart(t)
	c := g.drum.ensureLogsComp()
	c.Open()
	dst := ebiten.NewImage(1280, 720)
	c.Draw(dst)
	rr := c.rangeModeBtn.Rect()
	if rr.Empty() {
		t.Fatal("range toggle button not laid out")
	}
	c.HandleInput(rr.Min.X+rr.Dx()/2, rr.Min.Y+rr.Dy()/2, true)
	if c.tsMode != tsModeRange {
		t.Fatalf("range toggle click: tsMode=%d want Range", c.tsMode)
	}
	ar := c.atModeBtn.Rect()
	c.HandleInput(ar.Min.X+ar.Dx()/2, ar.Min.Y+ar.Dy()/2, true)
	if c.tsMode != tsModeAt {
		t.Fatalf("at toggle click: tsMode=%d want At", c.tsMode)
	}
}

// Routine events and info notifications don't badge; opening the panel marks them seen.
func TestUnreadBadgeCountsErrorsOnlyAndClears(t *testing.T) {
	ring := setLogRing(t, 50)
	g := newGameWithStart(t)
	dv := g.drum
	c := dv.ensureLogsComp()
	c.markSeen() // baseline: ignore anything produced during game setup
	base := time.Date(2026, 7, 19, 12, 0, 0, 0, time.Local)
	// Routine events do NOT badge.
	for i := 1; i <= 3; i++ {
		ring.Record(eventlogger.LogRecord{Seq: int64(i), At: base, Tag: "node", Msg: "x"})
	}
	// An info notification does NOT badge either.
	dv.notifyInfo("loaded demo.json")
	if n, hasErr := c.unreadCount(); n != 0 || hasErr {
		t.Fatalf("events/info must not badge: n=%d hasErr=%v want 0,false", n, hasErr)
	}
	// An error notification badges (red).
	dv.notifyError("boom")
	if n, hasErr := c.unreadCount(); n != 1 || !hasErr {
		t.Fatalf("an unread error must badge: n=%d hasErr=%v want 1,true", n, hasErr)
	}
	// Seeing them (markSeen / panel open) clears the badge.
	c.markSeen()
	if n, _ := c.unreadCount(); n != 0 {
		t.Fatalf("after markSeen: n=%d want 0", n)
	}
}

func TestLogsOpenDefaultsToErrorLevel(t *testing.T) {
	setLogRing(t, 20)
	g := newGameWithStart(t)
	dv := g.drum
	c := dv.ensureLogsComp()
	dv.notifyInfo("loaded")
	dv.notifyError("boom")
	c.Open()
	if c.activeLevel != levelError {
		t.Fatalf("Open should default to ERROR level; got %v", c.activeLevel)
	}
	for _, r := range c.filtered {
		if r.level != levelError {
			t.Fatalf("ERROR-filtered view showed a non-error row: %+v", r)
		}
	}
}

// The panel has no ×: the grid logs button toggles it closed, also after a reopen.
func TestLogsPanelTogglesViaLogsButton(t *testing.T) {
	setLogRing(t, 10)
	g := newGameWithStart(t)
	toggle := func() { g.toggleLogsPanel(); advanceFrames(g, 2) }

	toggle() // open
	if !g.drum.IsLogsPanelOpen() {
		t.Fatal("logs button should open the panel")
	}
	toggle() // close
	if g.drum.IsLogsPanelOpen() {
		t.Fatal("second logs-button tap must close the panel")
	}
	toggle() // reopen
	if !g.drum.IsLogsPanelOpen() {
		t.Fatal("logs button should reopen the panel")
	}
	toggle() // close after reopen
	if g.drum.IsLogsPanelOpen() {
		t.Fatal("close after reopen must also work")
	}
}

func TestErrorToastFiresOnErrorNotOnInfo(t *testing.T) {
	g := newGameWithStart(t)
	dv := g.drum
	dv.notifyInfo("hello")
	if dv.logsToastActive() {
		t.Fatal("info notification must NOT raise a toast")
	}
	dv.notifyError("boom")
	if !dv.logsToastActive() {
		t.Fatal("error notification must raise a toast")
	}
}

func TestLogsComponentMergesNotifications(t *testing.T) {
	setLogRing(t, 100)
	g := newGameWithStart(t)
	dv := g.drum
	dv.notifyError("boom happened")
	c := NewLogsComponent(dv)
	c.pageSize = 50
	c.rebuild()
	found := false
	for _, r := range c.filtered {
		if r.tag == "error" && r.isErr {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a merged error notification row, got %+v", c.filtered)
	}
}
