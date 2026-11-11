# Notification System Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax. **NOTE: this branch's owner asked NOT to stage or commit — replace every "commit" step with "run the full package test suite and leave files unstaged."**

**Goal:** Replace the floating top-right toast with a dedicated, persistent, marquee notification area in the header band (left of which sits the unified track button + beat counter), plus a clickable history popup.

**Architecture:** A `notificationStore` (single source of truth) holds a bounded history ring + the live entry; `notifyInfo/notifyError` feed it. `TimelineZone` draws the in-band notification area with a slow left marquee and opens a portal history popup on click. History persists across restarts via a new `NotificationHistoryStore` in `userprefs` mirroring `AudioPanelStore`.

**Tech Stack:** Go 1.23, Ebiten (stubbed under `-tags test`), `internal/ui` zones + portal, `internal/userprefs` (js/notjs split).

**Test command (fast path):**
`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/userprefs/`

---

## File Structure

- `internal/ui/drumview_notifications.go` (modify) — `notification` struct (drop `ttl`, add `unixMs`); `notifyInfo/notifyError` push into the store.
- `internal/ui/notification_store.go` (create) — `notificationStore`: ring + live entry + `Push`, `History`, `Latest`; `UseNotificationsHistory` gate var; persistence load/save hooks.
- `internal/ui/notification_marquee.go` (create) — pure `notifMarqueeOffsetX(frame, textW, areaW int) int`.
- `internal/userprefs/store.go` (modify) — `NotificationRecord`, `NotificationHistoryStore` interface, `prefsDoc.Notifications`, `marshalPrefsV5`, `parsePrefsV4` (returns notifications).
- `internal/userprefs/store_notjs.go` / `store_js.go` (modify) — `LoadNotifications`/`SaveNotifications`, snapshot field, ensureLoaded wiring.
- `internal/ui/drumview_layout.go` (modify) — unify track button to a left chip both platforms; compute `dv.notifRect`; left-anchor the counter.
- `internal/ui/drumview.go` (modify) — add `notifRect` field + `notifStore`; trackBtn layout fields.
- `internal/ui/timeline_zone.go` (modify) — draw notif area + marquee; register notif hit area; `TimelineCallbacks` gains `NotifRect`, `NotifLatest`, `OnNotifClick`.
- `internal/ui/drumview_notif_history.go` (create) — portal popup: open/close, `notifHistoryRect`, `drawNotifHistory`.
- `internal/ui/drumview_ctor.go` (modify) — fill new callbacks; load persisted history; remove `newNotificationsLayer` registration.
- `internal/ui/layer_notifications.go` (delete).

---

### Task 1: notification model + store

**Files:**
- Modify: `internal/ui/drumview_notifications.go`
- Create: `internal/ui/notification_store.go`
- Modify: `internal/ui/drumview.go` (add `notifStore *notificationStore`)
- Test: `internal/ui/notification_store_test.go` (create)

- [ ] **Step 1: failing test** — `notification_store_test.go`:
```go
package ui

import "testing"

func TestNotificationStore_RingBoundAndOrder(t *testing.T) {
	s := newNotificationStore(3)
	for i := 0; i < 5; i++ {
		s.Push(notification{text: string(rune('a' + i)), unixMs: int64(i)})
	}
	h := s.History()
	if len(h) != 3 {
		t.Fatalf("ring not bounded: got %d want 3", len(h))
	}
	// newest-first
	if h[0].text != "e" || h[2].text != "c" {
		t.Fatalf("order wrong: %v", []string{h[0].text, h[1].text, h[2].text})
	}
	if got := s.Latest(); got == nil || got.text != "e" {
		t.Fatalf("latest wrong: %+v", got)
	}
}

func TestNotificationStore_EmptyLatestNil(t *testing.T) {
	if l := newNotificationStore(10).Latest(); l != nil {
		t.Fatalf("empty store Latest should be nil, got %+v", l)
	}
}
```

- [ ] **Step 2: run, expect FAIL** (undefined `newNotificationStore`).

- [ ] **Step 3: implement** — `notification_store.go`:
```go
package ui

// UseNotificationsHistory gates persistence of the notification history.
// Tests flip it false to exercise legacy / no-persistence handling.
var UseNotificationsHistory = true

// notificationStore is the single source of truth for user notifications:
// a bounded newest-last ring plus accessors. The live band shows Latest();
// the history popup shows History() (newest-first).
type notificationStore struct {
	cap     int
	entries []notification // oldest..newest
	// persist is an optional sink invoked after every Push (set by DrumView
	// to write through to userprefs). nil in pure-unit tests.
	persist func([]notification)
}

func newNotificationStore(capacity int) *notificationStore {
	if capacity < 1 {
		capacity = 1
	}
	return &notificationStore{cap: capacity}
}

func (s *notificationStore) Push(n notification) {
	s.entries = append(s.entries, n)
	if len(s.entries) > s.cap {
		s.entries = s.entries[len(s.entries)-s.cap:]
	}
	if s.persist != nil {
		s.persist(s.snapshot())
	}
}

// History returns entries newest-first (copy).
func (s *notificationStore) History() []notification {
	out := make([]notification, len(s.entries))
	for i, n := range s.entries {
		out[len(s.entries)-1-i] = n
	}
	return out
}

// Latest returns the newest entry or nil when empty.
func (s *notificationStore) Latest() *notification {
	if len(s.entries) == 0 {
		return nil
	}
	n := s.entries[len(s.entries)-1]
	return &n
}

// seed replaces the ring with persisted entries (oldest..newest).
func (s *notificationStore) seed(entries []notification) {
	if len(entries) > s.cap {
		entries = entries[len(entries)-s.cap:]
	}
	s.entries = append(s.entries[:0], entries...)
}

func (s *notificationStore) snapshot() []notification {
	out := make([]notification, len(s.entries))
	copy(out, s.entries)
	return out
}
```

- [ ] **Step 4: modify `drumview_notifications.go`** — change struct + funnel:
```go
type notification struct {
	text   string
	isErr  bool
	unixMs int64 // unix millis when raised; 0 in pure-unit tests
}

func (dv *DrumView) notifyInfo(msg string)  { dv.pushNotif(msg, false) }
func (dv *DrumView) notifyError(msg string) { dv.pushNotif(msg, true) }
func (dv *DrumView) pushNotif(msg string, isErr bool) {
	if dv.notifStore == nil {
		dv.notifStore = newNotificationStore(notifHistoryCap)
	}
	dv.notifStore.Push(notification{text: msg, isErr: isErr, unixMs: nowUnixMilli()})
}
```
Add `const notifHistoryCap = 100` to `notification_store.go`. Add a `nowUnixMilli()` helper (in `notification_store.go`): `func nowUnixMilli() int64 { return time.Now().UnixMilli() }` (import `time`).

- [ ] **Step 5: modify `drumview.go`** — replace `notifs []notification` (line ~720) with `notifStore *notificationStore`. Grep for `dv.notifs`/`.notifs` and remove stale references (only `layer_notifications.go`, deleted in Task 8, and any tests).

- [ ] **Step 6: run** `go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestNotificationStore` → PASS. Then leave unstaged (no commit).

---

### Task 2: userprefs persistence — `NotificationHistoryStore`

**Files:**
- Modify: `internal/userprefs/store.go`, `store_notjs.go`, `store_js.go`
- Test: `internal/userprefs/notification_store_test.go` (create)

- [ ] **Step 1: failing test** mirroring `audio_panel_state_test.go`:
```go
package userprefs

import (
	"path/filepath"
	"testing"
)

func TestNotificationHistory_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prefs.json")
	name := "userprefs.test." + t.Name()
	st, err := NewBackingStore(Options{Path: path, PoolName: name})
	if err != nil { t.Fatal(err) }
	ns := st.(NotificationHistoryStore)
	recs := []NotificationRecord{{Text: "hi", IsErr: false, UnixMs: 1}, {Text: "boom", IsErr: true, UnixMs: 2}}
	if err := ns.SaveNotifications(recs); err != nil { t.Fatal(err) }
	st.(interface{ FlushForTest() })  // see Step 3 note on flush
	// reopen
	st2, err := NewBackingStore(Options{Path: path, PoolName: name + ".2"})
	if err != nil { t.Fatal(err) }
	got, err := st2.(NotificationHistoryStore).LoadNotifications()
	if err != nil { t.Fatal(err) }
	if len(got) != 2 || got[0].Text != "hi" || got[1].IsErr != true {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}
```
(Adjust flush to match the existing pattern used by `audio_panel_state_test.go` — read that test first and copy its synchronous-flush approach.)

- [ ] **Step 2: run, expect FAIL** (undefined `NotificationRecord` / `NotificationHistoryStore`).

- [ ] **Step 3: implement in `store.go`:**
```go
// NotificationRecord is one persisted user notification.
type NotificationRecord struct {
	Text  string `json:"text"`
	IsErr bool   `json:"is_err,omitempty"`
	UnixMs int64 `json:"unix_ms,omitempty"`
}

// NotificationHistoryStore persists the bounded notification history across
// sessions. Expose via type assertion on the Store returned by NewBackingStore.
type NotificationHistoryStore interface {
	LoadNotifications() ([]NotificationRecord, error)
	SaveNotifications(recs []NotificationRecord) error
}
```
Add `Notifications []NotificationRecord` to `prefsDoc` (json:"notifications,omitempty"`). Add `marshalPrefsV5(... , notifications []NotificationRecord)` extending `marshalPrefsV4`, and a `parsePrefsV4` that returns notifications too (or augment `parsePrefsV3` callers). **Important:** wire the new field through the LOAD path (`ensureLoadedLocked`) — do not repeat the apparent sampleEdits load omission; the round-trip test guards this.

- [ ] **Step 4: implement store methods** in `store_notjs.go` and `store_js.go` mirroring `LoadAudioPanelState`/`SaveAudioPanelState`: a `notifications []NotificationRecord` field on the concrete struct + snapshot field; `SaveNotifications` bounds to 100 (drop-oldest) then `queueWriteLocked`; update `marshalPrefsV4`→`marshalPrefsV5` call sites (notjs:467, js:275) and `ensureLoadedLocked` parse (notjs:118, js:68).

- [ ] **Step 5: run** `./internal/userprefs/ -run TestNotificationHistory` → PASS. Run the whole `./internal/userprefs/` suite to confirm no regression. Leave unstaged.

---

### Task 3: wire store ↔ persistence in DrumView

**Files:**
- Modify: `internal/ui/notification_store.go` (conversion helpers), `internal/ui/drumview_ctor.go`
- Test: `internal/ui/notification_persist_test.go` (create)

- [ ] **Step 1: failing test** — construct a DrumView test harness (use the existing `newTestGame`/`testDrumView` helper — grep `func newTestDrumView` / `assertDefaultParityState`), push a notification, assert the injected fake persist sink received it; and that seeding from records populates History. Keep it deterministic (set `UseNotificationsHistory=true`, fake sink).

- [ ] **Step 2: run, expect FAIL.**

- [ ] **Step 3: implement** — in `drumview_ctor.go`, after the store/prefs handle is available, set:
```go
dv.notifStore = newNotificationStore(notifHistoryCap)
if ns, ok := dv.prefsStore.(userprefs.NotificationHistoryStore); ok && UseNotificationsHistory {
	if recs, err := ns.LoadNotifications(); err == nil {
		dv.notifStore.seed(recordsToNotifs(recs))
	}
	dv.notifStore.persist = func(snap []notification) {
		_ = ns.SaveNotifications(notifsToRecords(snap))
	}
}
```
(Find the existing prefs-store handle name via grep `userprefs.` in `drumview_ctor.go` / `drumview.go`; reuse it — do NOT open a second store.) Add `recordsToNotifs`/`notifsToRecords` in `notification_store.go`. **Per the design, do NOT seed the live band from history** — `seed` fills the ring for the popup, but `Latest()` is only shown in the band once a *new* session notification arrives; gate the band render on a `sessionDirty` bool set on first `Push`. Add `func (s *notificationStore) HasSessionEntry() bool`.

- [ ] **Step 4: run** the new test → PASS. Leave unstaged.

---

### Task 4: marquee offset math (pure)

**Files:**
- Create: `internal/ui/notification_marquee.go`
- Test: `internal/ui/notification_marquee_test.go`

- [ ] **Step 1: failing test:**
```go
package ui
import "testing"
func TestMarquee_FitsIsStatic(t *testing.T) {
	if off := notifMarqueeOffsetX(999, 40, 100, 8); off != 0 {
		t.Fatalf("fitting text must not scroll, got %d", off)
	}
}
func TestMarquee_OverflowScrollsLeftThenPausesAndLoops(t *testing.T) {
	textW, areaW, gap := 200, 100, 8
	// frame 0 -> offset 0 (start)
	if off := notifMarqueeOffsetX(0, textW, areaW, gap); off != 0 {
		t.Fatalf("frame0 want 0 got %d", off)
	}
	// monotonic non-positive while scrolling
	prev := 0
	sawNeg := false
	for f := 1; f < 400; f++ {
		off := notifMarqueeOffsetX(f, textW, areaW, gap)
		if off > 0 { t.Fatalf("offset must be <=0, got %d at f=%d", off, f) }
		if off < 0 { sawNeg = true }
		// never scroll past where the tail+gap clears the area
		if off < -(textW-areaW+gap) { t.Fatalf("scrolled too far: %d", off) }
		prev = off
	}
	_ = prev
	if !sawNeg { t.Fatal("expected leftward scroll") }
}
```

- [ ] **Step 2: run, expect FAIL.**

- [ ] **Step 3: implement** — slow constant rate (1px per `notifMarqueePxPeriod` frames), hold at start `notifMarqueeHoldFrames`, scroll to `-(textW-areaW+gap)`, hold, loop:
```go
package ui

const (
	notifMarqueeHoldFrames = 90 // ~1.5s pause at each end (60fps)
	notifMarqueePxPeriod   = 3  // advance 1px every N frames (slow)
)

// notifMarqueeOffsetX returns the x pixel offset (<=0) to translate the
// notification text so a too-wide message scrolls slowly leftward, pausing
// at both ends, then looping. Returns 0 when the text fits.
func notifMarqueeOffsetX(frame, textW, areaW, gap int) int {
	travel := textW - areaW + gap
	if travel <= 0 {
		return 0
	}
	scrollFrames := travel * notifMarqueePxPeriod
	period := notifMarqueeHoldFrames + scrollFrames + notifMarqueeHoldFrames
	p := ((frame % period) + period) % period
	switch {
	case p < notifMarqueeHoldFrames:
		return 0
	case p < notifMarqueeHoldFrames+scrollFrames:
		return -((p - notifMarqueeHoldFrames) / notifMarqueePxPeriod)
	default:
		return -travel
	}
}
```

- [ ] **Step 4: run** → PASS. Leave unstaged.

---

### Task 5: layout — unify track button, add `notifRect`, left-anchor counter

**Files:**
- Modify: `internal/ui/drumview.go` (add `notifRect image.Rectangle`)
- Modify: `internal/ui/drumview_layout.go` (track-btn block ~584-621; beatCounterRect ~473-476)
- Test: `internal/ui/notification_layout_test.go`

- [ ] **Step 1: failing test** (both profiles):
```go
func TestNotifBandLayout_BothProfiles(t *testing.T) {
	for _, mobile := range []bool{false, true} {
		restore := setProfileForTest(mobile) // grep existing helper name
		g := newLaidOutTestGame(t)           // grep existing helper; ensure calcLayout ran
		dv := g.drum
		band := dv.beatCounterRect
		notif := dv.notifRect
		track := dv.trackBtn().Rect()
		if notif.Empty() { t.Fatalf("mobile=%v: notifRect empty", mobile) }
		if track.Empty() { t.Fatalf("mobile=%v: track chip empty", mobile) }
		// order: track chip left of counter; counter left of notif
		if !(track.Max.X <= band.Min.X) { t.Fatalf("mobile=%v: track not left of counter", mobile) }
		if !(band.Max.X <= notif.Min.X) { t.Fatalf("mobile=%v: counter not left of notif", mobile) }
		if notif.Max.X > dv.timelineRect.Max.X+1 { t.Fatalf("mobile=%v: notif overflows band right", mobile) }
		restore()
	}
}
```
(Read existing layout tests e.g. `TestTrackChip_VisibleOnMobile`, `responsive_layout_test.go` to reuse the real harness/helpers and profile setters.)

- [ ] **Step 2: run, expect FAIL.**

- [ ] **Step 3: implement** — In `drumview_layout.go`: replace the desktop-vs-mobile track-button branch with one unified left chip placed at the band's left edge (`tlWidget.Min.X`), square `TouchMinTarget()` on mobile / band-height on desktop. Set `beatCounterRect.Min.X = trackChip.Max.X + SpaceSM` and shrink the pill slot to the cached info width (`TextWidth(timelineInfoCached)` + pad), then `dv.notifRect = image.Rect(beatCounterRect.Max.X+SpaceSM, bcTop, rightLimit, bcTop+infoH)`. Preserve the existing chip-strip right clamp (lines ~560-567) by applying it to `notifRect.Max.X` too. Keep `timelineRect` left reservation consistent with the chip width.

- [ ] **Step 4: run** → PASS, plus the full `./internal/ui/` layout tests (`-run 'Layout|TrackChip|Track'`) to catch regressions in the track-button contract tests; update those tests only where the unification intentionally changes geometry, with a one-line rationale. Leave unstaged.

---

### Task 6: TimelineZone — draw notif area + marquee + click-to-open

**Files:**
- Modify: `internal/ui/timeline_zone.go` (Draw ~220-235; `rebuildHitAreas` ~1020), `TimelineCallbacks` (~24-62)
- Modify: `internal/ui/drumview_ctor.go` (fill callbacks)
- Test: `internal/ui/notification_band_render_test.go`

- [ ] **Step 1: failing test** — assert the zone registers a `"timeline-notif"` hit area when `notifRect` non-empty, and that invoking its handler opens the `notif-history` portal:
```go
func TestNotifArea_HitAreaOpensHistory(t *testing.T) {
	g := newLaidOutTestGame(t)
	dv := g.drum
	dv.notifyInfo("hello")
	g.layoutAndDraw(t) // grep real helper that runs Update/Draw once
	var h *HitArea
	for i := range dv.timelineZone.hitAreas {
		if dv.timelineZone.hitAreas[i].Tag == "timeline-notif" { h = &dv.timelineZone.hitAreas[i] }
	}
	if h == nil { t.Fatal("notif hit area missing") }
	h.Handler.OnPress(dv.notifRect.Min.X+1, dv.notifRect.Min.Y+1)
	if !dv.tree.Portal().Has("notif-history") { t.Fatal("history popup did not open") }
}
```

- [ ] **Step 2: run, expect FAIL.**

- [ ] **Step 3: implement** — Add to `TimelineCallbacks`: `NotifRect func() image.Rectangle`, `NotifLatest func() (text string, isErr bool, show bool)`, `OnNotifClick func()`. In `drumview_ctor.go` fill them (`NotifRect: func() image.Rectangle { return dv.notifRect }`, `NotifLatest` reads `dv.notifStore.Latest()` + `HasSessionEntry()`, `OnNotifClick: dv.openNotifHistoryPortal`). In `TimelineZone.Draw` add `z.drawNotifArea(screen)` next to `drawBeatCounter`. Implement `drawNotifArea` (new method in `timeline_zone.go` or a `timeline_zone_notif.go`): draw the surface chip (`colSurface1`/`colBorderSubtle`), a severity accent stripe, then the text clipped to `notifRect` translated by `notifMarqueeOffsetX(int(z.callbacks.Frame()), TextWidth(text), notifRect.Dx()-2*pad, gap)`; color `colTextPrimary` info / `colError` error. In `rebuildHitAreas` append a `buttonless` hit area:
```go
if r := z.callbacks.NotifRect(); r != nil && !r.Empty() {
	z.hitAreas = append(z.hitAreas, HitArea{
		Rect: r, ZIndex: 112, Handler: &notifClickAdapter{zone: z},
		Tag: "timeline-notif", Touch: true, ClipRect: z.rect,
	})
}
```
Add `notifClickAdapter` (OnPress → `z.callbacks.OnNotifClick()`; return `InputConsumed`; other methods no-op; OnWheel `InputIgnored`).

- [ ] **Step 4: run** → PASS. Leave unstaged.

---

### Task 7: history popup (portal)

**Files:**
- Create: `internal/ui/drumview_notif_history.go`
- Modify: `internal/ui/drumview_notifications.go` (add `IsNotifHistoryOpen`)
- Test: `internal/ui/notification_history_popup_test.go`

- [ ] **Step 1: failing test** — open via `dv.openNotifHistoryPortal()`, assert `IsNotifHistoryOpen()` true and `notifHistoryRect` non-empty and fits in `dv.Bounds`; simulate an outside-click close (mirror an existing portal close test).

- [ ] **Step 2: run, expect FAIL.**

- [ ] **Step 3: implement** — `drumview_notif_history.go`:
```go
func (dv *DrumView) IsNotifHistoryOpen() bool {
	return dv.tree != nil && dv.tree.Portal().Has("notif-history")
}

func (dv *DrumView) openNotifHistoryPortal() {
	if dv.tree == nil { return }
	dv.computeNotifHistoryRect()
	dv.tree.Portal().Open(PortalEntry{
		ID: "notif-history",
		Overlay: &dvOverlayPortal{
			id:       "notif-history",
			isOpenFn: dv.IsNotifHistoryOpen,
			rectFn:   func() image.Rectangle { return dv.notifHistoryRect },
			inputFn:  func(x, y int, pressed bool) InputResult { return InputConsumed },
			wheelFn:  func(x, y, steps int) InputResult { return InputConsumed },
			drawFn:   dv.drawNotifHistory,
		},
		Scrim: true,
	})
}
```
`computeNotifHistoryRect`: anchor below `dv.notifRect`, width = max(notifRect.Dx(), min readable width), height = clamp(rows*lineH+pad, …) within `dv.Bounds`. `drawNotifHistory`: surface panel + each `dv.notifStore.History()` row as `timeWord + " " + text`, errors `colError` / info `colTextPrimary`; newest first; clip to rect. (Scroll optional v1 — if history exceeds height, show the most recent N that fit; `log()` a TODO comment, do not silently imply full coverage.)

- [ ] **Step 4: run** → PASS. Leave unstaged.

---

### Task 8: remove floating toast layer

**Files:**
- Delete: `internal/ui/layer_notifications.go`
- Modify: `internal/ui/drumview_ctor.go` (remove `dv.tree.RegisterLayer(newNotificationsLayer(dv))` ~line 939)
- Modify: `internal/ui/drumview_tree.go` (repurpose/retire `ZNotifications` comment)

- [ ] **Step 1:** delete the file and the registration line. Grep `newNotificationsLayer`, `NotificationsLayer`, `\.notifs\b`, `\.ttl\b`, `\.msg\b` (on notification) across `internal/ui` and fix every reference (tests included).
- [ ] **Step 2: run** `go build -tags test ./internal/ui/` then the full `./internal/ui/` suite → PASS. Leave unstaged.

---

### Task 9: pixel coverage for the notif area

**Files:**
- Test: `internal/ui/notification_band_pixels_test.go`

- [ ] **Step 1:** mirror `audio_panel_render_pixels_test.go`'s drawRect/text-color interception: push an error notification, draw, assert the notif-area region contains `colError`-tinted text pixels (and `colTextPrimary` for an info notification). Use `RENDER_SAFE=1` style or the existing draw interceptor helper.
- [ ] **Step 2: run** → PASS. Leave unstaged.

---

## Final verification (run before reporting done)

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/userprefs/
```
- Token discipline: if any new `color.RGBA` literal was introduced, refactor to a `Token*()` accessor (the `token_discipline_test.go` ratchet will fail otherwise).
- Confirm `git status` shows the changes UNSTAGED and nothing committed.

## Self-review notes
- Spec coverage: band layout (T5), marquee (T4/T6), persistence across restarts (T2/T3), history popup (T7), info+error styling (T6/T7/T9), remove floating toast (T8). ✓
- Type consistency: `notification{text,isErr,unixMs}` used uniformly; `notificationStore.{Push,History,Latest,seed,HasSessionEntry}`; `NotificationRecord{Text,IsErr,UnixMs}`; callbacks `NotifRect/NotifLatest/OnNotifClick`. ✓
- Discovery items flagged inline (grep real helper names: profile setter, laid-out test-game builder, prefs-store handle, flush-for-test pattern).
