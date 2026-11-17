# Logs Navigation Panel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the in-band notification marquee and its basic history popup with a Logs icon (next to the gear) that opens a searchable, filterable, paginated Logs Navigation Panel over the full `hooks.Bus` event stream merged with user notifications.

**Architecture:** `internal/eventlogger` gains a bounded in-memory ring (it already subscribes to every kind and formats each) exposed via a global `History()` sink mirroring the notification-sink pattern. A new `LogsComponent` UI overlay (modeled on `InstrumentMenuComponent`) merges that ring with the existing `notificationStore` at read time and renders fuzzy name-search, data-driven category chips (from each record's tag), exact/range timestamp filters, and pagination. A new grid-pane Logs icon (mirror of the settings gear) opens it, carries an unread badge, and an error-only transient toast replaces the removed in-band alert.

**Tech Stack:** Go 1.26 (bundled `.tools/go/bin/go`), Ebiten v2 (ebitenstub under `-tags test`), `internal/hooks`, `internal/eventlogger`, `internal/ui`, `internal/i18n`.

## Global Constraints

- **Bundled Go only:** `.tools/go/bin/go` (avoid version drift). `gofmt` via `.tools/go/bin/gofmt`.
- **Fast test command:** from `src/go`: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./<pkg>`.
- **Never stage or commit.** The user commits manually (standing instruction, overrides the skill's commit steps). Each task ends at a **green checkpoint**: run the task's tests + the fast full suite for touched packages, leave all changes **unstaged** for user review. Do NOT run `git add`/`git commit`.
- **DESIGN.md compliance:** tokens only — no inline `color.RGBA`/`color.NRGBA` (respect `token_discipline_test.go`; new UI files with any literal must be added to `allowedLiteralBudget` *with review* — prefer `Token*()` accessors and `col*` constants). Icons via `IconID` + `DrawIcon`; **no forbidden glyphs** (`▶▼▲✕≡⏸■↑↓→←`).
- **i18n:** every user-facing string is an `i18n.Key` present in BOTH `catalog_en.go` and `catalog_es.go`, declared in `keys.go`. `TestI18n*` guards parity.
- **Functional-first tests:** prefer driving real `Game.Update` via the ebitenstub harness over isolated widget pokes (repo convention). Pure logic (parsing, pagination, filtering) gets pure unit tests.
- **Timestamps:** local timezone, `HH:MM` (minute precision) or `HH:MM:SS` (second precision) in filter inputs; row display shows `HH:MM:SS.mmm`.

---

## File Structure

**Create:**
- `src/go/internal/eventlogger/history.go` — `LogRecord`, ring buffer, `LogHistory` sink + `History()`/`SetLogHistory()` accessors.
- `src/go/internal/eventlogger/history_test.go`
- `src/go/internal/ui/logs_model.go` — pure logic: merge records → rows, timestamp parse/match, distinct tags, name/category/time filters, pagination.
- `src/go/internal/ui/logs_model_test.go`
- `src/go/internal/ui/logs_component.go` — `LogsComponent` (implements `compOverlayable`): state, layout, draw.
- `src/go/internal/ui/logs_component_test.go`
- `src/go/internal/ui/drumview_logs_panel.go` — `DrumView` open/close/toggle + portal wiring.
- `src/go/internal/ui/grid_logs_input_handler.go` — grid-pane input handler for the Logs icon (mirror of `grid_help_input_handler.go`).
- `src/go/internal/ui/logs_toast.go` — error-only transient toast.
- `src/go/internal/ui/logs_panel_functional_test.go` — end-to-end functional tests via `Game.Update`.

**Modify:**
- `src/go/internal/eventlogger/logger.go` — record into the ring inside `writeEvent`.
- `src/go/internal/eventlogger/format.go` — add exported `Format(hooks.Event) string`.
- `src/go/cmd/beatmo.go` + `src/go/internal/ui/js_bootstrap_wasm.go` — wire `SetLogHistory` at startup.
- `src/go/internal/ui/icons.go` (+ the icon renderer file, `drawing_icons.go`) — add `IconLogs` + `drawLogsIcon`.
- `src/go/internal/ui/game_struct.go` — `gridLogsBtn`, `gridLogsCapturing`, badge/toast fields.
- `src/go/internal/ui/game_new.go` — construct `gridLogsBtn`.
- `src/go/internal/ui/game_input_shortcuts.go` — `gridLogsButtonRect()`, `toggleLogsPanel()`.
- `src/go/internal/ui/grid_pane_draw.go` — draw the Logs icon + badge next to the gear.
- `src/go/internal/ui/game_layout.go` — refresh the logs rect.
- `src/go/internal/ui/game_update.go` — register `gridLogsInputHandler`; drain error toast.
- `src/go/internal/ui/timeline_zone.go` — remove `drawNotifArea` + notif callbacks + hit-area.
- `src/go/internal/ui/drumview_layout.go` — remove `notifRect` carving.
- `src/go/internal/ui/drumview_ctor.go` — remove notif callback bindings; hook error-toast into `pushNotif`.
- `src/go/internal/ui/keys.go`, `catalog_en.go`, `catalog_es.go` — new keys.
- `src/go/internal/ui/game_settings_gear_zone_test.go` — widen protected zone to both icons.

**Delete:**
- `src/go/internal/ui/notification_marquee.go`
- `src/go/internal/ui/drumview_notif_history.go` (+ its test, if any)

**Keep unchanged:** `notification_store.go`, `notification_history_sink.go`, `notification_legacy.go`, `notifyInfo*/notifyError*` push APIs.

---

## Task 1: Exported `eventlogger.Format`

**Files:**
- Modify: `src/go/internal/eventlogger/format.go`
- Test: `src/go/internal/eventlogger/format_export_test.go` (create)

**Interfaces:**
- Produces: `func Format(e hooks.Event) string` — the narrative message for an event (no bracket tag), `""` when the kind has no formatter.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/eventlogger/format_export_test.go`:
```go
package eventlogger

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

func TestFormatExported(t *testing.T) {
	got := Format(hooks.Event{Kind: hooks.EventPlayStart})
	if got != "play started" {
		t.Fatalf("Format(EventPlayStart) = %q, want %q", got, "play started")
	}
	if got := Format(hooks.Event{Kind: hooks.EventSeek, Payload: hooks.SeekPayload{Beats: 4}}); got != "seek beats=4" {
		t.Fatalf("Format(EventSeek) = %q, want %q", got, "seek beats=4")
	}
	if got := Format(hooks.Event{Kind: hooks.Kind("does.not.exist")}); got != "" {
		t.Fatalf("Format(unknown) = %q, want empty", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/eventlogger/ -run TestFormatExported`
Expected: FAIL — `undefined: Format`.

- [ ] **Step 3: Add the exported function**

Append to `src/go/internal/eventlogger/format.go`:
```go
// Format renders the one-line narrative message for an event (no bracket tag —
// the tag comes from hooks.MetaFor(Kind).Tag). Returns "" when the kind has no
// formatter or the formatter suppresses the line. Exported so the UI logs panel
// can render the same text the INFO narrative uses without duplicating the
// per-kind formatter table.
func Format(e hooks.Event) string {
	f, ok := lookupFormatter(e.Kind)
	if !ok {
		return ""
	}
	return f(e.Payload)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/eventlogger/ -run TestFormatExported`
Expected: PASS.

- [ ] **Step 5: Checkpoint**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/eventlogger/`
Expected: PASS. Leave changes unstaged.

---

## Task 2: eventlogger in-memory ring + `LogHistory` sink

**Files:**
- Create: `src/go/internal/eventlogger/history.go`, `src/go/internal/eventlogger/history_test.go`
- Modify: `src/go/internal/eventlogger/logger.go`

**Interfaces:**
- Produces:
  - `type LogRecord struct { Seq int64; At time.Time; Kind hooks.Kind; Tag string; Scope hooks.ActionScope; Msg string; IsErr bool }`
  - `type LogHistory interface { Record(LogRecord); Snapshot() []LogRecord }`
  - `func History() LogHistory` (never nil), `func SetLogHistory(LogHistory)`, `func NewRingHistory(capacity int) LogHistory`
- Consumes (in logger.go): `Format`, `hooks.MetaFor`.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/eventlogger/history_test.go`:
```go
package eventlogger

import (
	"testing"
	"time"
)

func TestRingHistoryRetainsNewestFirstWithinCap(t *testing.T) {
	h := NewRingHistory(3)
	for i := 1; i <= 5; i++ {
		h.Record(LogRecord{Seq: int64(i), At: time.Unix(int64(i), 0), Msg: "m"})
	}
	snap := h.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("len=%d, want 3 (cap enforced)", len(snap))
	}
	// Newest-first: seq 5,4,3 (1,2 evicted).
	wantSeq := []int64{5, 4, 3}
	for i, w := range wantSeq {
		if snap[i].Seq != w {
			t.Fatalf("snap[%d].Seq=%d, want %d", i, snap[i].Seq, w)
		}
	}
}

func TestSnapshotIsCopy(t *testing.T) {
	h := NewRingHistory(2)
	h.Record(LogRecord{Seq: 1, Msg: "a"})
	snap := h.Snapshot()
	snap[0].Msg = "mutated"
	if got := h.Snapshot()[0].Msg; got != "a" {
		t.Fatalf("Snapshot not copy-safe: got %q after caller mutation", got)
	}
}

func TestGlobalHistoryDefaultNoop(t *testing.T) {
	// Default sink is non-nil and safe.
	if History() == nil {
		t.Fatal("History() returned nil")
	}
	History().Record(LogRecord{Seq: 1}) // must not panic
}

func TestSetLogHistoryRoundTrip(t *testing.T) {
	prev := History()
	t.Cleanup(func() { SetLogHistory(prev) })
	h := NewRingHistory(4)
	SetLogHistory(h)
	History().Record(LogRecord{Seq: 7, Msg: "x"})
	if got := History().Snapshot(); len(got) != 1 || got[0].Seq != 7 {
		t.Fatalf("round trip failed: %+v", got)
	}
	SetLogHistory(nil) // resets to no-op default
	if len(History().Snapshot()) != 0 {
		t.Fatal("nil should reset to no-op default")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/eventlogger/ -run 'TestRingHistory|TestSnapshot|TestGlobalHistory|TestSetLogHistory'`
Expected: FAIL — undefined `NewRingHistory`, `LogRecord`, `History`, `SetLogHistory`.

- [ ] **Step 3: Create the history implementation**

Create `src/go/internal/eventlogger/history.go`:
```go
package eventlogger

import (
	"sync"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// LogRecord is one retained, searchable log line: the event's timestamp, its
// category (Tag, the bracket prefix), the rendered message, and whether it is
// an error. Session-only; never persisted.
type LogRecord struct {
	Seq   int64
	At    time.Time
	Kind  hooks.Kind
	Tag   string
	Scope hooks.ActionScope
	Msg   string
	IsErr bool
}

// LogHistory retains recent LogRecords for the UI logs panel.
type LogHistory interface {
	// Record appends a record (bounded; oldest evicted).
	Record(LogRecord)
	// Snapshot returns retained records NEWEST-FIRST (copy — caller may mutate).
	Snapshot() []LogRecord
}

// ringHistory is a bounded oldest..newest ring behind a mutex.
type ringHistory struct {
	mu      sync.RWMutex
	cap     int
	entries []LogRecord // oldest..newest
}

// NewRingHistory returns a bounded LogHistory. capacity < 1 is clamped to 1.
func NewRingHistory(capacity int) LogHistory {
	if capacity < 1 {
		capacity = 1
	}
	return &ringHistory{cap: capacity}
}

func (r *ringHistory) Record(rec LogRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, rec)
	if len(r.entries) > r.cap {
		r.entries = r.entries[len(r.entries)-r.cap:]
	}
}

func (r *ringHistory) Snapshot() []LogRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]LogRecord, len(r.entries))
	for i, e := range r.entries {
		out[len(r.entries)-1-i] = e // newest-first
	}
	return out
}

// noopHistory is the default: retains nothing. Keeps History() non-nil.
type noopHistory struct{}

func (noopHistory) Record(LogRecord)      {}
func (noopHistory) Snapshot() []LogRecord { return nil }

var (
	histGuard sync.RWMutex
	histSink  LogHistory = noopHistory{}
)

// SetLogHistory registers the global log-history sink. Production wires this
// once at startup with a NewRingHistory(cap); a nil sink resets to the no-op.
func SetLogHistory(h LogHistory) {
	histGuard.Lock()
	defer histGuard.Unlock()
	if h == nil {
		histSink = noopHistory{}
		return
	}
	histSink = h
}

// History returns the registered sink (never nil).
func History() LogHistory {
	histGuard.RLock()
	defer histGuard.RUnlock()
	return histSink
}
```

- [ ] **Step 4: Record from the logger's write path**

In `src/go/internal/eventlogger/logger.go`, add a package-level seq counter and record into the global history inside `writeEvent`. Add near the top (after imports):
```go
// logSeq is a process-wide monotonic sequence for retained LogRecords.
var logSeq atomic.Int64
```
Then in `writeEvent`, immediately after `msg := f(e.Payload)` and the `if msg == ""` guard, before the INFO emit, insert:
```go
	tag := ""
	scope := hooks.ScopeSession
	if meta, ok := hooks.MetaFor(e.Kind); ok {
		tag = meta.Tag
		scope = meta.Scope
	}
	History().Record(LogRecord{
		Seq:   logSeq.Add(1),
		At:    e.At,
		Kind:  e.Kind,
		Tag:   tag,
		Scope: scope,
		Msg:   msg,
		IsErr: false,
	})
```
Then replace the existing `tag := ""` / `if meta, ok := hooks.MetaFor...` block lower in `writeEvent` with a reuse of the `tag` computed above (remove the duplicate declaration so the file still compiles — the INFO emit keeps using `tag`).

> Note: `atomic` is already imported in `logger.go`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/eventlogger/`
Expected: PASS (history tests + existing logger/coverage tests).

- [ ] **Step 6: Checkpoint**

Confirm no goleak regressions: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/eventlogger/ -count=1`. Leave changes unstaged.

---

## Task 3: Wire the log history at startup

**Files:**
- Modify: `src/go/cmd/beatmo.go`, `src/go/internal/ui/js_bootstrap_wasm.go`

**Interfaces:**
- Consumes: `eventlogger.SetLogHistory`, `eventlogger.NewRingHistory`.

- [ ] **Step 1: Add desktop wiring**

In `src/go/cmd/beatmo.go`, near where the `eventlogger.Open(...)` / `SetNotificationHistoryStore(...)` wiring lives (around line 143-191), add before the logger is opened:
```go
	// Retain a bounded in-memory event history for the UI Logs panel.
	eventlogger.SetLogHistory(eventlogger.NewRingHistory(1000))
```
Ensure `eventlogger` is imported (it already is where `eventlogger.Open` is called).

- [ ] **Step 2: Add WASM wiring**

In `src/go/internal/ui/js_bootstrap_wasm.go`, near the `SetNotificationHistoryStore` wiring (around line 28-29), add:
```go
	eventlogger.SetLogHistory(eventlogger.NewRingHistory(1000))
```
Add the `eventlogger` import if not already present.

- [ ] **Step 3: Verify both build**

Run: `cd src/go && ../../.tools/go/bin/go build ./cmd/...`
Expected: success.
Run: `cd src/go && GOOS=js GOARCH=wasm ../../.tools/go/bin/go build -tags test ./internal/ui/` (compile check for the wasm bootstrap path).
Expected: success.

- [ ] **Step 4: Checkpoint**

Leave changes unstaged.

---

## Task 4: New i18n keys

**Files:**
- Modify: `src/go/internal/i18n/keys.go`, `src/go/internal/i18n/catalog_en.go`, `src/go/internal/i18n/catalog_es.go`

**Interfaces:**
- Produces keys: `KeyLogs`, `KeyLogsSearchName`, `KeyLogsTimeAt`, `KeyLogsTimeRange`, `KeyLogsFrom`, `KeyLogsTo`, `KeyLogsAllCategories`, `KeyLogsPage`, `KeyLogsGoToPage`, `KeyNoLogs`.

- [ ] **Step 1: Declare the keys**

In `src/go/internal/i18n/keys.go`, add the constants alongside the existing `Key*` block:
```go
	KeyLogs              Key = "logs.title"
	KeyLogsSearchName    Key = "logs.search_name"
	KeyLogsTimeAt        Key = "logs.time_at"
	KeyLogsTimeRange     Key = "logs.time_range"
	KeyLogsFrom          Key = "logs.from"
	KeyLogsTo            Key = "logs.to"
	KeyLogsAllCategories Key = "logs.all_categories"
	KeyLogsPage          Key = "logs.page" // "Page %d / %d"
	KeyLogsGoToPage      Key = "logs.go_to_page"
	KeyNoLogs            Key = "logs.empty"
```

- [ ] **Step 2: Add English strings**

In `src/go/internal/i18n/catalog_en.go`:
```go
	KeyLogs:              "Logs",
	KeyLogsSearchName:    "Search name or type",
	KeyLogsTimeAt:        "At",
	KeyLogsTimeRange:     "Range",
	KeyLogsFrom:          "From",
	KeyLogsTo:            "To",
	KeyLogsAllCategories: "All",
	KeyLogsPage:          "Page %d / %d",
	KeyLogsGoToPage:      "Go to page",
	KeyNoLogs:            "No logs yet",
```

- [ ] **Step 3: Add Spanish strings**

In `src/go/internal/i18n/catalog_es.go`:
```go
	KeyLogs:              "Registros",
	KeyLogsSearchName:    "Buscar nombre o tipo",
	KeyLogsTimeAt:        "En",
	KeyLogsTimeRange:     "Rango",
	KeyLogsFrom:          "Desde",
	KeyLogsTo:            "Hasta",
	KeyLogsAllCategories: "Todos",
	KeyLogsPage:          "Página %d / %d",
	KeyLogsGoToPage:      "Ir a página",
	KeyNoLogs:            "Sin registros",
```

- [ ] **Step 4: Verify catalog parity**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/i18n/`
Expected: PASS (parity guard sees both catalogs updated).

- [ ] **Step 5: Checkpoint** — leave unstaged.

---

## Task 5: UI pure logic — merge, filter, timestamp, pagination

**Files:**
- Create: `src/go/internal/ui/logs_model.go`, `src/go/internal/ui/logs_model_test.go`

**Interfaces:**
- Produces:
  - `type logRow struct { at time.Time; tag, msg string; isErr bool; highlights []int }`
  - `func mergeLogRows(events []eventlogger.LogRecord, notifs []notifRowInput) []logRow` — newest-first by `at`.
  - `type notifRowInput struct { at time.Time; text string; isErr bool }`
  - `func distinctTags(rows []logRow) []string` — error/info pinned first, then alphabetical.
  - `func parseLogTime(s string) (hh, mm, ss int, prec timePrec, ok bool)` where `type timePrec int` (`precNone`, `precMinute`, `precSecond`).
  - `func matchTimeAt(at time.Time, spec string) bool`
  - `func matchTimeRange(at time.Time, from, to string) bool`
  - `func pageCount(total, pageSize int) int`
  - `func pageSlice[T any](items []T, pageSize, page int) []T` (1-based page, clamped)
  - `func filterRows(rows []logRow, name string, activeTags map[string]bool, tsMode timestampMode, at, from, to string) []logRow`
  - `type timestampMode int` (`tsModeAt`, `tsModeRange`)

- [ ] **Step 1: Write the failing tests**

Create `src/go/internal/ui/logs_model_test.go`:
```go
package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/eventlogger"
)

func tm(h, m, s int) time.Time { return time.Date(2026, 7, 19, h, m, s, 0, time.Local) }

func TestMergeLogRowsNewestFirst(t *testing.T) {
	ev := []eventlogger.LogRecord{{At: tm(12, 0, 1), Tag: "node", Msg: "add node"}}
	nt := []notifRowInput{{at: tm(12, 0, 5), text: "Imported", isErr: false}, {at: tm(12, 0, 3), text: "boom", isErr: true}}
	got := mergeLogRows(ev, nt)
	if len(got) != 3 {
		t.Fatalf("len=%d want 3", len(got))
	}
	if !got[0].at.Equal(tm(12, 0, 5)) || got[2].tag != "node" {
		t.Fatalf("not newest-first: %+v", got)
	}
	if got[1].tag != "error" || !got[1].isErr {
		t.Fatalf("error notif tag/flag wrong: %+v", got[1])
	}
}

func TestDistinctTagsPinsErrorInfoFirst(t *testing.T) {
	rows := []logRow{{tag: "row"}, {tag: "node"}, {tag: "error"}, {tag: "info"}, {tag: "node"}}
	got := distinctTags(rows)
	want := []string{"error", "info", "node", "row"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestParseAndMatchTime(t *testing.T) {
	if !matchTimeAt(tm(12, 4, 33), "12:04") {
		t.Fatal("minute-precision At should match same minute")
	}
	if matchTimeAt(tm(12, 5, 0), "12:04") {
		t.Fatal("different minute should not match")
	}
	if !matchTimeAt(tm(12, 4, 33), "12:04:33") {
		t.Fatal("exact second At should match")
	}
	if matchTimeAt(tm(12, 4, 34), "12:04:33") {
		t.Fatal("off-by-one second should not match")
	}
	if matchTimeAt(tm(12, 4, 33), "") {
		// empty spec = no filter → treated as match
		t.Skip()
	}
}

func TestMatchTimeRange(t *testing.T) {
	if !matchTimeRange(tm(12, 4, 0), "12:00", "12:10") {
		t.Fatal("in-range should match")
	}
	if matchTimeRange(tm(12, 20, 0), "12:00", "12:10") {
		t.Fatal("out-of-range should not match")
	}
	if !matchTimeRange(tm(12, 4, 0), "12:00", "") {
		t.Fatal("open-ended To (since) should match")
	}
	if !matchTimeRange(tm(12, 4, 0), "", "12:10") {
		t.Fatal("open-ended From (until) should match")
	}
	if !matchTimeRange(tm(9, 9, 9), "", "") {
		t.Fatal("empty range = no filter")
	}
}

func TestPagination(t *testing.T) {
	if pageCount(0, 10) != 1 {
		t.Fatal("empty → 1 page")
	}
	if pageCount(25, 10) != 3 {
		t.Fatal("25/10 → 3 pages")
	}
	items := []int{1, 2, 3, 4, 5}
	if got := pageSlice(items, 2, 2); len(got) != 2 || got[0] != 3 {
		t.Fatalf("page 2 of size 2 = %v want [3 4]", got)
	}
	if got := pageSlice(items, 2, 99); len(got) != 1 || got[0] != 5 {
		t.Fatalf("clamp high page to last: got %v", got)
	}
	if got := pageSlice(items, 2, 0); got[0] != 1 {
		t.Fatalf("clamp low page to 1: got %v", got)
	}
}

func TestFilterRowsByNameIncludesTag(t *testing.T) {
	rows := []logRow{{tag: "node", msg: "add node"}, {tag: "play", msg: "play started"}}
	got := filterRows(rows, "node", nil, tsModeAt, "", "", "")
	if len(got) != 1 || got[0].tag != "node" {
		t.Fatalf("name search over tag failed: %+v", got)
	}
}

func TestFilterRowsByCategory(t *testing.T) {
	rows := []logRow{{tag: "node", msg: "a", at: tm(1, 0, 0)}, {tag: "play", msg: "b", at: tm(1, 0, 1)}}
	got := filterRows(rows, "", map[string]bool{"play": true}, tsModeAt, "", "", "")
	if len(got) != 1 || got[0].tag != "play" {
		t.Fatalf("category filter failed: %+v", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestMergeLogRows|TestDistinctTags|TestParseAndMatchTime|TestMatchTimeRange|TestPagination|TestFilterRows'`
Expected: FAIL — undefined identifiers.

- [ ] **Step 3: Implement the pure model**

Create `src/go/internal/ui/logs_model.go`:
```go
package ui

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/eventlogger"
)

type timestampMode int

const (
	tsModeAt timestampMode = iota
	tsModeRange
)

type timePrec int

const (
	precNone timePrec = iota
	precMinute
	precSecond
)

// logRow is one rendered/merged/filtered row in the logs panel.
type logRow struct {
	at         time.Time
	tag        string
	msg        string
	isErr      bool
	highlights []int // fuzzy match positions in msg (set by name filter)
}

// notifRowInput is the read-time projection of a UI notification.
type notifRowInput struct {
	at    time.Time
	text  string
	isErr bool
}

// mergeLogRows combines event records and notifications into one newest-first
// list. Notifications get tag "error" or "info".
func mergeLogRows(events []eventlogger.LogRecord, notifs []notifRowInput) []logRow {
	out := make([]logRow, 0, len(events)+len(notifs))
	for _, e := range events {
		out = append(out, logRow{at: e.At, tag: e.Tag, msg: e.Msg, isErr: e.IsErr})
	}
	for _, n := range notifs {
		tag := "info"
		if n.isErr {
			tag = "error"
		}
		out = append(out, logRow{at: n.at, tag: tag, msg: n.text, isErr: n.isErr})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].at.After(out[j].at) })
	return out
}

// distinctTags returns the unique tags present, error/info pinned first, then
// alphabetical. New tags therefore appear automatically.
func distinctTags(rows []logRow) []string {
	seen := map[string]bool{}
	for _, r := range rows {
		if r.tag != "" {
			seen[r.tag] = true
		}
	}
	var rest []string
	for t := range seen {
		if t != "error" && t != "info" {
			rest = append(rest, t)
		}
	}
	sort.Strings(rest)
	var out []string
	if seen["error"] {
		out = append(out, "error")
	}
	if seen["info"] {
		out = append(out, "info")
	}
	return append(out, rest...)
}

// parseLogTime parses "H:MM" / "HH:MM" (minute) or "HH:MM:SS" (second).
func parseLogTime(s string) (hh, mm, ss int, prec timePrec, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, 0, precNone, false
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, 0, 0, precNone, false
	}
	h, e1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	m, e2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if e1 != nil || e2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, 0, precNone, false
	}
	if len(parts) == 2 {
		return h, m, 0, precMinute, true
	}
	sec, e3 := strconv.Atoi(strings.TrimSpace(parts[2]))
	if e3 != nil || sec < 0 || sec > 59 {
		return 0, 0, 0, precNone, false
	}
	return h, m, sec, precSecond, true
}

// atMinutes/atSeconds convert a time to seconds-of-day for comparison.
func atSecondsOfDay(t time.Time) int { return t.Hour()*3600 + t.Minute()*60 + t.Second() }

// matchTimeAt reports whether at matches spec at the spec's precision. Empty or
// unparseable spec = no filter (returns true).
func matchTimeAt(at time.Time, spec string) bool {
	h, m, s, prec, ok := parseLogTime(spec)
	if !ok {
		return true
	}
	if prec == precMinute {
		return at.Hour() == h && at.Minute() == m
	}
	return atSecondsOfDay(at) == h*3600+m*60+s
}

// matchTimeRange reports whether at ∈ [from, to]. Empty bound = open-ended.
func matchTimeRange(at time.Time, from, to string) bool {
	cur := atSecondsOfDay(at)
	if h, m, s, prec, ok := parseLogTime(from); ok {
		lo := h*3600 + m*60 + s
		if prec == precMinute {
			lo = h*3600 + m*60
		}
		if cur < lo {
			return false
		}
	}
	if h, m, s, prec, ok := parseLogTime(to); ok {
		hi := h*3600 + m*60 + s
		if prec == precMinute {
			hi = h*3600 + m*60 + 59 // inclusive whole minute
		}
		if cur > hi {
			return false
		}
	}
	return true
}

// pageCount returns the number of pages (>=1) for total items at pageSize.
func pageCount(total, pageSize int) int {
	if pageSize < 1 {
		pageSize = 1
	}
	if total <= 0 {
		return 1
	}
	return (total + pageSize - 1) / pageSize
}

// pageSlice returns the 1-based page window, clamping page into [1, pageCount].
func pageSlice[T any](items []T, pageSize, page int) []T {
	if pageSize < 1 {
		pageSize = 1
	}
	pc := pageCount(len(items), pageSize)
	if page < 1 {
		page = 1
	}
	if page > pc {
		page = pc
	}
	start := (page - 1) * pageSize
	if start >= len(items) {
		return nil
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

// filterRows applies name (fuzzy over tag+msg), category, and timestamp filters.
// Name matches set logRow.highlights (positions within msg) for rendering.
func filterRows(rows []logRow, name string, activeTags map[string]bool, tsMode timestampMode, at, from, to string) []logRow {
	name = strings.TrimSpace(name)
	out := make([]logRow, 0, len(rows))
	for _, r := range rows {
		if len(activeTags) > 0 && !activeTags[r.tag] {
			continue
		}
		switch tsMode {
		case tsModeRange:
			if !matchTimeRange(r.at, from, to) {
				continue
			}
		default:
			if !matchTimeAt(r.at, at) {
				continue
			}
		}
		if name != "" {
			hay := r.tag + " " + r.msg
			fr := FuzzyMatch(name, hay)
			if fr.Score < 0 {
				continue
			}
			// Shift highlight positions that fall inside msg back to msg-space.
			prefix := len([]rune(r.tag + " "))
			var hl []int
			for _, p := range fr.Positions {
				if p >= prefix {
					hl = append(hl, p-prefix)
				}
			}
			r.highlights = hl
		}
		out = append(out, r)
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestMergeLogRows|TestDistinctTags|TestParseAndMatchTime|TestMatchTimeRange|TestPagination|TestFilterRows'`
Expected: PASS.

- [ ] **Step 5: Checkpoint** — leave unstaged.

---

## Task 6: `IconLogs` + grid Logs icon (opens empty overlay)

**Files:**
- Modify: `src/go/internal/ui/icons.go`, the icon renderer (`drawing_icons.go`), `game_struct.go`, `game_new.go`, `game_input_shortcuts.go`, `grid_pane_draw.go`, `game_layout.go`, `game_update.go`
- Create: `src/go/internal/ui/grid_logs_input_handler.go`, `src/go/internal/ui/drumview_logs_panel.go`
- Test: `src/go/internal/ui/logs_panel_functional_test.go` (create — opens/closes)

**Interfaces:**
- Produces: `IconLogs IconID`, `(*Game).gridLogsButtonRect() image.Rectangle`, `(*Game).toggleLogsPanel()`, `(*DrumView).openLogsPanel()`, `(*DrumView).IsLogsPanelOpen() bool`, `type gridLogsInputHandler struct{ g *Game }`.
- Consumes: existing gear pattern (`gridHelpBtn`, `gridHelpButtonRect`, `gridHelpInputHandler`, `toggleSettingsOverlay`), `LogsComponent` (Task 7 provides the real body; here a minimal stub component is fine — but to avoid a placeholder, implement `LogsComponent` minimally now and flesh it in Task 7).

- [ ] **Step 1: Write the failing functional test**

Create `src/go/internal/ui/logs_panel_functional_test.go`:
```go
package ui

import "testing"

func TestLogsIconTogglesPanel(t *testing.T) {
	g := newTestGame(t) // standard ebitenstub game constructor used across ui tests
	if g.dv.IsLogsPanelOpen() {
		t.Fatal("panel should start closed")
	}
	// Click the logs icon center.
	r := g.gridLogsButtonRect()
	clickAt(t, g, r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2) // helper: press+release+Update
	if !g.dv.IsLogsPanelOpen() {
		t.Fatal("panel should open after clicking logs icon")
	}
	// Toggle closes it.
	g.toggleLogsPanel()
	updateFrames(t, g, 2)
	if g.dv.IsLogsPanelOpen() {
		t.Fatal("panel should close on second toggle")
	}
}
```
> If `newTestGame`/`clickAt`/`updateFrames`/`g.dv` helper names differ in this package, use the equivalents already used by `game_settings_gear_zone_test.go` / other `game_*_test.go` (read one first). The assertions and flow stay identical.

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestLogsIconTogglesPanel`
Expected: FAIL — undefined `IsLogsPanelOpen`, `gridLogsButtonRect`, `toggleLogsPanel`.

- [ ] **Step 3: Add the icon**

In `src/go/internal/ui/icons.go`, add to the `IconID` const block (after `IconSettings`):
```go
	IconLogs IconID = "logs"
```
In the `DrawIcon` dispatch switch (same file, near `case IconSettings:`), add:
```go
	case IconLogs:
		drawLogsIcon(dst, r, col)
```
In the icon renderer file (`drawing_icons.go`, alongside `drawSettingsIcon`), add a document-with-lines glyph using existing primitives (mirror the stroke helpers used by neighbors):
```go
// drawLogsIcon renders a stacked-lines "log list" glyph (three rows of text).
func drawLogsIcon(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	pad := r.Dx() / 5
	x0 := r.Min.X + pad
	x1 := r.Max.X - pad
	th := maxInt(1, r.Dy()/12)
	for i := 0; i < 3; i++ {
		y := r.Min.Y + pad + i*(r.Dy()-2*pad)/2
		drawLineThick(dst, x0, y, x1, y, th, col) // reuse the existing stroke helper used by sibling icons
	}
}
```
> Use whatever thick-line/stroke helper the neighboring icon renderers use (read `drawSettingsIcon` for the exact helper name — e.g. `drawHLine`, `strokeLine`). Keep it token-colored via the passed `col`.

- [ ] **Step 4: Add fields, construction, rect, handler, toggle, draw, registration**

`game_struct.go` — add fields near `gridHelpBtn`:
```go
	gridLogsBtn       *Button // logs icon, left of the settings gear
	gridLogsCapturing bool
```

`game_new.go` — after the `gridHelpBtn` construction (~line 150):
```go
	g.gridLogsBtn = NewSpecButton("", ComponentButtonSecondary, g.toggleLogsPanel)
	g.gridLogsBtn.Icon = string(IconLogs)
```

`game_input_shortcuts.go` — add next to `gridHelpButtonRect()`:
```go
// gridLogsButtonRect returns the logs icon rect, one slot left of the gear.
func (g *Game) gridLogsButtonRect() image.Rectangle {
	gear := g.gridHelpButtonRect()
	const gap = 8
	return image.Rect(gear.Min.X-gap-gear.Dx(), gear.Min.Y, gear.Min.X-gap, gear.Max.Y)
}

// toggleLogsPanel opens/closes the logs panel (mirror of toggleSettingsOverlay).
func (g *Game) toggleLogsPanel() {
	if g.dv == nil {
		return
	}
	if g.dv.IsLogsPanelOpen() {
		g.dv.portal().Close("logs")
		return
	}
	g.dv.openLogsPanel()
}
```

`grid_pane_draw.go` — in `drawGridHelpButton` (~line 1333), after drawing the gear, draw the logs icon:
```go
	g.gridLogsBtn.SetRect(g.gridLogsButtonRect())
	g.gridLogsBtn.Draw(dst)
```

`game_layout.go` — where `gridHelpButtonRect()` is refreshed (~line 161), also `g.gridLogsBtn.SetRect(g.gridLogsButtonRect())` (guard nil).

Create `src/go/internal/ui/grid_logs_input_handler.go` (mirror `grid_help_input_handler.go`):
```go
package ui

import "image"

const gridLogsInputZ = 300

type gridLogsInputHandler struct{ g *Game }

func (h gridLogsInputHandler) InputBounds() image.Rectangle { return h.g.gridLogsButtonRect() }
func (h gridLogsInputHandler) ZIndex() int                  { return gridLogsInputZ }

func (h gridLogsInputHandler) HandleInput(x, y int, pressed bool) InputResult {
	if h.g.gridLogsBtn == nil {
		return InputIgnored
	}
	h.g.gridLogsBtn.SetRect(h.g.gridLogsButtonRect())
	res := h.g.gridLogsBtn.HandleInputResult(x, y, pressed)
	if res == InputConsumed || res == InputCaptured {
		h.g.gridLogsCapturing = true
	}
	return res
}
```

`game_update.go` — where `gridHelpInputHandler` is registered (~line 419):
```go
	if g.gridLogsBtn != nil {
		g.inputDispatcher.Register(gridLogsInputHandler{g: g})
	}
```

- [ ] **Step 5: Add the panel open/close (portal) with a minimal LogsComponent**

Create `src/go/internal/ui/drumview_logs_panel.go`:
```go
package ui

import "image"

// IsLogsPanelOpen reports whether the logs panel portal is open.
func (dv *DrumView) IsLogsPanelOpen() bool {
	return dv.tree != nil && dv.portal().Has("logs")
}

// openLogsPanel opens the Logs Navigation Panel as a modal overlay.
func (dv *DrumView) openLogsPanel() {
	if dv.tree == nil {
		return
	}
	if dv.logsComp == nil {
		dv.logsComp = NewLogsComponent(dv)
	}
	dv.logsComp.Open()
	dv.logsComp.markSeen() // clears the unread badge
	dv.portal().Open(PortalEntry{
		ID:      "logs",
		Modal:   true,
		Overlay: &compPortalOverlay{comp: dv.logsComp, tag: "logs", updateFn: dv.logsComp.Update},
		OnClose: func() { dv.logsComp.Close() },
	})
}
```
Add `logsComp *LogsComponent` to the `DrumView` struct (in `drumview.go` near `notifStore`).

`NewLogsComponent`, `Open`, `Close`, `markSeen`, `Update`, and the full `compOverlayable` methods are provided in Task 7. For this task, create a **minimal** `logs_component.go` so it compiles and the toggle test passes — Task 7 replaces the body:
```go
package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

type LogsComponent struct {
	overlayBase
	dv   *DrumView
	open bool
}

func NewLogsComponent(dv *DrumView) *LogsComponent { return &LogsComponent{dv: dv} }
func (c *LogsComponent) Open()                      { c.open = true }
func (c *LogsComponent) Close()                     { c.open = false }
func (c *LogsComponent) IsOpen() bool               { return c.open }
func (c *LogsComponent) markSeen()                  {}
func (c *LogsComponent) Update()                    {}
func (c *LogsComponent) InputBounds() image.Rectangle {
	return c.dv.Bounds // full-screen modal for now
}
func (c *LogsComponent) HandleInput(x, y int, pressed bool) InputResult { return InputConsumed }
func (c *LogsComponent) HandleWheel(x, y, steps int) InputResult        { return InputConsumed }
func (c *LogsComponent) Capturing() bool                                { return false }
func (c *LogsComponent) Draw(dst *ebiten.Image)                         {}
```

- [ ] **Step 6: Run the functional test**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestLogsIconTogglesPanel`
Expected: PASS.

- [ ] **Step 7: Checkpoint**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Icon|Gear|Grid'` to catch icon/grid regressions. Leave unstaged.

---

## Task 7: `LogsComponent` — layout, rows, pagination footer

**Files:**
- Modify: `src/go/internal/ui/logs_component.go` (replace the minimal stub)
- Create: `src/go/internal/ui/logs_component_test.go`

**Interfaces:**
- Consumes: `logRow`, `mergeLogRows`, `distinctTags`, `filterRows`, `pageCount`, `pageSlice`, `eventlogger.History`, `dv.notifStore`, `drawMenuRow`, `MenuRowSpec`, `drawRoundedRect`, tokens.
- Produces: `(*LogsComponent).rebuild()`, `(*LogsComponent).visibleRows() []logRow`, `(*LogsComponent).pageCountNow() int`, plus real `Draw`.

- [ ] **Step 1: Write the failing test (rebuild + pagination math)**

Create `src/go/internal/ui/logs_component_test.go`:
```go
package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/eventlogger"
)

func TestLogsComponentRebuildPaginates(t *testing.T) {
	prev := eventlogger.History()
	t.Cleanup(func() { eventlogger.SetLogHistory(prev) })
	ring := eventlogger.NewRingHistory(100)
	eventlogger.SetLogHistory(ring)
	base := time.Date(2026, 7, 19, 12, 0, 0, 0, time.Local)
	for i := 0; i < 25; i++ {
		ring.Record(eventlogger.LogRecord{Seq: int64(i), At: base.Add(time.Duration(i) * time.Second), Tag: "node", Msg: "add node"})
	}
	dv := newTestDrumView(t) // standard ui-test DrumView constructor
	c := NewLogsComponent(dv)
	c.pageSize = 10 // deterministic for the test
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
```
> Use the DrumView test constructor already used by other `*_test.go` in this package (read one, e.g. how `logs_panel_functional_test.go`'s game exposes `g.dv`). If only a `Game` constructor exists, get `dv` from it.

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestLogsComponentRebuildPaginates`
Expected: FAIL — undefined `pageSize`, `rebuild`, `pageCountNow`, `visibleRows`.

- [ ] **Step 3: Implement the component body**

Replace `src/go/internal/ui/logs_component.go` with the full implementation (state, rebuild, layout, draw, pagination footer). Key structure:
```go
package ui

import (
	"fmt"
	"image"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/eventlogger"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

type LogsComponent struct {
	overlayBase
	dv   *DrumView
	open bool

	// state
	nameQuery  string
	tsMode     timestampMode
	tsAt       string
	tsFrom     string
	tsTo       string
	activeTags map[string]bool
	page       int
	pageSize   int

	// derived (rebuild)
	all       []logRow
	filtered  []logRow
	tags      []string
	pageCountCache int

	// unread tracking
	lastSeenSeq int64

	// widgets (Task 8/9 populate)
	prevBtn *Button
	nextBtn *Button
}

func NewLogsComponent(dv *DrumView) *LogsComponent {
	return &LogsComponent{dv: dv, activeTags: map[string]bool{}, page: 1, pageSize: 12}
}

func (c *LogsComponent) Open()        { c.open = true; c.rebuild() }
func (c *LogsComponent) Close()       { c.open = false }
func (c *LogsComponent) IsOpen() bool { return c.open }
func (c *LogsComponent) markSeen() {
	snap := eventlogger.History().Snapshot()
	if len(snap) > 0 {
		c.lastSeenSeq = snap[0].Seq
	}
}
func (c *LogsComponent) Update() {}

// notifInputs projects the notification store into read-time rows.
func (c *LogsComponent) notifInputs() []notifRowInput {
	if c.dv == nil || c.dv.notifStore == nil {
		return nil
	}
	var out []notifRowInput
	for _, n := range c.dv.notifStore.History() { // newest-first
		out = append(out, notifRowInput{at: time.UnixMilli(n.unixMs), text: n.display(), isErr: n.isErr})
	}
	return out
}

func (c *LogsComponent) rebuild() {
	c.all = mergeLogRows(eventlogger.History().Snapshot(), c.notifInputs())
	c.tags = distinctTags(c.all)
	c.filtered = filterRows(c.all, c.nameQuery, c.activeTags, c.tsMode, c.tsAt, c.tsFrom, c.tsTo)
	c.pageCountCache = pageCount(len(c.filtered), c.pageSize)
	if c.page > c.pageCountCache {
		c.page = c.pageCountCache
	}
	if c.page < 1 {
		c.page = 1
	}
}

func (c *LogsComponent) pageCountNow() int  { return c.pageCountCache }
func (c *LogsComponent) visibleRows() []logRow {
	return pageSlice(c.filtered, c.pageSize, c.page)
}

func (c *LogsComponent) InputBounds() image.Rectangle { return c.panelRect() }

// panelRect centers a large panel in the drum bounds (like SettingsOverlay).
func (c *LogsComponent) panelRect() image.Rectangle {
	b := c.dv.Bounds
	w := b.Dx() * 3 / 4
	h := b.Dy() * 3 / 4
	cx, cy := b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2
	return image.Rect(cx-w/2, cy-h/2, cx+w/2, cy+h/2)
}

func (c *LogsComponent) Capturing() bool { return false }

func (c *LogsComponent) Draw(dst *ebiten.Image) {
	r := c.panelRect()
	drawRoundedRect(dst, r, colSurface2, RadiusLG, true)
	drawRoundedRect(dst, r, colBorderSubtle, RadiusLG, false)
	// Header title.
	DrawTextColorAt(dst, i18n.T(i18n.KeyLogs), r.Min.X+SpaceMD, r.Min.Y+SpaceSM, colTextPrimary)

	// Body: recompute page size from available body height each frame.
	rowH := TextHeight() + SpaceSM
	bodyTop := r.Min.Y + headerHeightLogs
	bodyBot := r.Max.Y - footerHeightLogs
	c.pageSize = maxInt(1, (bodyBot-bodyTop)/rowH)
	c.rebuild()

	y := bodyTop
	rows := c.visibleRows()
	if len(rows) == 0 {
		DrawTextColorAt(dst, i18n.T(i18n.KeyNoLogs), r.Min.X+SpaceMD, y, colTextSecondary)
	}
	for _, row := range rows {
		col := colTextPrimary
		if row.isErr {
			col = colError
		}
		ts := row.at.Format("15:04:05.000")
		// Tag + message + right-aligned timestamp.
		line := "[" + row.tag + "] " + row.msg
		DrawTextColorAt(dst, line, r.Min.X+SpaceMD, y, col)
		DrawTextColorAt(dst, ts, r.Max.X-SpaceMD-TextWidth(ts), y, colTextSecondary)
		y += rowH
	}

	// Footer: page controls.
	c.drawFooter(dst, r)
}

const (
	headerHeightLogs = 84 // title + search + timestamp + chips (Tasks 8/9 fill)
	footerHeightLogs = 40
)

func (c *LogsComponent) drawFooter(dst *ebiten.Image, r image.Rectangle) {
	label := fmt.Sprintf(i18n.T(i18n.KeyLogsPage), c.page, c.pageCountNow())
	fy := r.Max.Y - footerHeightLogs + SpaceSM
	cx := r.Min.X + r.Dx()/2
	DrawTextColorAt(dst, label, cx-TextWidth(label)/2, fy, colTextSecondary)
	// Prev/next chevrons.
	chSz := IconSizeSM
	prev := image.Rect(cx-TextWidth(label)/2-SpaceLG-chSz, fy, cx-TextWidth(label)/2-SpaceLG, fy+chSz)
	next := image.Rect(cx+TextWidth(label)/2+SpaceLG, fy, cx+TextWidth(label)/2+SpaceLG+chSz, fy+chSz)
	DrawIcon(dst, IconChevronLeft, prev, colTextSecondary)
	DrawIcon(dst, IconChevronRight, next, colTextSecondary)
	c.prevRect, c.nextRect = prev, next
}
```
Add `prevRect, nextRect image.Rectangle` fields. Implement `HandleInput` to: page-nav on prev/next rects (decrement/increment `c.page`, clamp, `rebuild`), else consume. Implement `HandleWheel` to page (wheel up = prev page, down = next). `HandleInput`:
```go
func (c *LogsComponent) HandleInput(x, y int, pressed bool) InputResult {
	if !pressed {
		return InputConsumed
	}
	p := image.Pt(x, y)
	if p.In(c.prevRect) {
		c.page--
		c.rebuild()
		return InputConsumed
	}
	if p.In(c.nextRect) {
		c.page++
		c.rebuild()
		return InputConsumed
	}
	// Click outside the panel closes (modal): let the portal handle click-outside.
	if !p.In(c.panelRect()) {
		c.Close()
		return InputIgnored
	}
	return InputConsumed
}

func (c *LogsComponent) HandleWheel(x, y, steps int) InputResult {
	if steps > 0 {
		c.page--
	} else if steps < 0 {
		c.page++
	}
	c.rebuild()
	return InputConsumed
}
```
> `TextWidth`, `TextHeight`, `maxInt`, `DrawTextColorAt`, `DrawIcon`, `drawRoundedRect`, `SpaceMD/LG/SM`, `IconSizeSM`, `RadiusLG`, `colSurface2`, `colBorderSubtle`, `colTextPrimary/Secondary`, `colError` all exist in this package. If `TextWidth` differs (e.g. `MeasureText`), use the package's existing measurer (read `render_meters.go` for the name).

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestLogsComponentRebuildPaginates`
Expected: PASS.

- [ ] **Step 5: Checkpoint** — token discipline: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestTokenDiscipline|TestDesignMDDrift'`. If the new files carry any inline literal, prefer a token; only touch `allowedLiteralBudget` with review. Leave unstaged.

---

## Task 8: Search-by-name + data-driven category chips

**Files:**
- Modify: `src/go/internal/ui/logs_component.go`
- Modify: `src/go/internal/ui/logs_component_test.go` (add cases)

**Interfaces:**
- Consumes: `ensureMenuSearchBox`, `drawMenuSearchBox`, `menuSearchClearRect`, `TextInput`, `MenuScroll` (for the chip strip), tokens.

- [ ] **Step 1: Write failing tests (search + chip filter drive rebuild)**

Add to `src/go/internal/ui/logs_component_test.go`:
```go
func TestLogsSearchNameFiltersList(t *testing.T) {
	dv := newTestDrumView(t)
	c := NewLogsComponent(dv)
	c.all = []logRow{{tag: "node", msg: "add node", at: tm(1, 0, 0)}, {tag: "play", msg: "play started", at: tm(1, 0, 1)}}
	c.applyNameQuery("play")
	if len(c.filtered) != 1 || c.filtered[0].tag != "play" {
		t.Fatalf("search failed: %+v", c.filtered)
	}
}

func TestLogsCategoryToggleFiltersList(t *testing.T) {
	dv := newTestDrumView(t)
	c := NewLogsComponent(dv)
	c.all = []logRow{{tag: "node", msg: "a", at: tm(1, 0, 0)}, {tag: "eq", msg: "b", at: tm(1, 0, 1)}}
	c.toggleTag("eq")
	c.reapply()
	if len(c.filtered) != 1 || c.filtered[0].tag != "eq" {
		t.Fatalf("category toggle failed: %+v", c.filtered)
	}
	c.toggleTag("eq") // off again
	c.reapply()
	if len(c.filtered) != 2 {
		t.Fatalf("clearing category failed: %+v", c.filtered)
	}
}
```

- [ ] **Step 2: Run to verify fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestLogsSearchNameFiltersList|TestLogsCategoryToggleFiltersList'`
Expected: FAIL — undefined `applyNameQuery`, `toggleTag`, `reapply`.

- [ ] **Step 3: Implement search + chips**

Add to `logs_component.go`:
```go
// reapply recomputes only the filtered set + page count (cheaper than rebuild;
// used after a filter change that doesn't need a fresh snapshot).
func (c *LogsComponent) reapply() {
	c.filtered = filterRows(c.all, c.nameQuery, c.activeTags, c.tsMode, c.tsAt, c.tsFrom, c.tsTo)
	c.pageCountCache = pageCount(len(c.filtered), c.pageSize)
	c.page = 1
}

func (c *LogsComponent) applyNameQuery(q string) {
	c.nameQuery = q
	c.reapply()
}

func (c *LogsComponent) toggleTag(tag string) {
	if c.activeTags[tag] {
		delete(c.activeTags, tag)
	} else {
		c.activeTags[tag] = true
	}
}
```
Add a `searchBox *TextInput` field. In `Draw`, above the body, lay out and draw the search box + chip strip:
```go
	// Search box (row 1 of header).
	searchRect := image.Rect(r.Min.X+SpaceMD, r.Min.Y+SpaceLG+TextHeight(), r.Min.X+r.Dx()/2, r.Min.Y+SpaceLG+TextHeight()+ControlHeight())
	c.searchBox = ensureMenuSearchBox(c.searchBox, searchRect)
	drawMenuSearchBox(dst, c.searchBox)
	if v := c.searchBox.Value(); v != c.nameQuery {
		c.applyNameQuery(v)
	}
	// Chip strip (row 2 of header): one toggle per distinct tag + "All".
	c.drawChips(dst, r)
```
Implement `drawChips` iterating `append([]string{allChipKey}, c.tags...)`, drawing each as a small rounded pill (active = accent fill `colPrimary`, inactive = `colSurface3`), recording hit rects into `c.chipRects map[string]image.Rectangle`. Wire chip hits in `HandleInput` before the page-nav checks:
```go
	for tag, cr := range c.chipRects {
		if p.In(cr) {
			if tag == allChipKey {
				c.activeTags = map[string]bool{}
			} else {
				c.toggleTag(tag)
			}
			c.reapply()
			return InputConsumed
		}
	}
```
Add `const allChipKey = ""` handling (the "All" pill clears `activeTags`). Also feed the search box `Update()` from the component `Update()`:
```go
func (c *LogsComponent) Update() {
	if c.searchBox != nil {
		c.searchBox.Update()
		if v := c.searchBox.Value(); v != c.nameQuery {
			c.applyNameQuery(v)
		}
	}
}
```
Route search-box clicks/focus through `HandleInput` (hit-test `c.searchBox.Rect` → forward press to focus; also handle the clear-× via `menuSearchClearRect`). `ClaimsKeyboard()` returns `c.searchBox != nil && c.searchBox.Focused()` (add the method — `compPortalOverlay` forwards it).

- [ ] **Step 4: Run to verify pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestLogsSearchNameFiltersList|TestLogsCategoryToggleFiltersList|TestLogsComponentRebuildPaginates'`
Expected: PASS.

- [ ] **Step 5: Add a functional new-tag-auto-appears test**

Add to `logs_panel_functional_test.go`:
```go
func TestLogsCategoriesIncludeNewTagsAutomatically(t *testing.T) {
	dv := newTestDrumView(t)
	c := NewLogsComponent(dv)
	c.all = []logRow{{tag: "brandnewkind", msg: "x", at: tm(1, 0, 0)}}
	c.tags = distinctTags(c.all)
	found := false
	for _, tg := range c.tags {
		if tg == "brandnewkind" {
			found = true
		}
	}
	if !found {
		t.Fatal("a never-before-seen tag must appear as a category automatically")
	}
}
```
Run it: expect PASS.

- [ ] **Step 6: Checkpoint** — leave unstaged.

---

## Task 9: Timestamp At/Range controls

**Files:**
- Modify: `src/go/internal/ui/logs_component.go`, `src/go/internal/ui/logs_component_test.go`

**Interfaces:**
- Consumes: `TextInput` (BPMBoxStyle), `Button` for the `At|Range` toggle, `matchTimeAt`, `matchTimeRange`.

- [ ] **Step 1: Write failing test**

Add to `logs_component_test.go`:
```go
func TestLogsTimestampAtAndRangeFilter(t *testing.T) {
	dv := newTestDrumView(t)
	c := NewLogsComponent(dv)
	c.all = []logRow{
		{tag: "a", msg: "1", at: tm(12, 0, 0)},
		{tag: "a", msg: "2", at: tm(12, 5, 0)},
		{tag: "a", msg: "3", at: tm(12, 10, 0)},
	}
	// At (minute precision) selects only 12:05.
	c.tsMode = tsModeAt
	c.setTimeAt("12:05")
	if len(c.filtered) != 1 || c.filtered[0].msg != "2" {
		t.Fatalf("At filter failed: %+v", c.filtered)
	}
	// Range 12:00–12:05 selects two.
	c.tsMode = tsModeRange
	c.setTimeRange("12:00", "12:05")
	if len(c.filtered) != 2 {
		t.Fatalf("Range filter failed: %+v", c.filtered)
	}
}
```

- [ ] **Step 2: Run to verify fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestLogsTimestampAtAndRangeFilter`
Expected: FAIL — undefined `setTimeAt`, `setTimeRange`.

- [ ] **Step 3: Implement setters + draw the controls**

Add:
```go
func (c *LogsComponent) setTimeAt(s string)         { c.tsAt = s; c.reapply() }
func (c *LogsComponent) setTimeRange(from, to string) { c.tsFrom, c.tsTo = from, to; c.reapply() }
func (c *LogsComponent) setTimeMode(m timestampMode)  { c.tsMode = m; c.reapply() }
```
Add `atBox, fromBox, toBox *TextInput` and an `At|Range` segmented toggle (two `Button`s). Lay them out in the header (right of the search box). In `Draw`, draw the mode toggle; when `tsMode == tsModeAt` draw `atBox` (one field), else draw `fromBox` + `toBox`. In `Update`, sync each box's `Value()` into `tsAt`/`tsFrom`/`tsTo` (calling `reapply` on change). Wire the toggle buttons' `OnClick` to `setTimeMode`. Time inputs use `AcceptTextRune` restricted to digits and `:` (add an `Accept` filter). Active toggle pill uses `colPrimary`, inactive `colSurface3`.

- [ ] **Step 4: Run to verify pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestLogsTimestampAtAndRangeFilter`
Expected: PASS.

- [ ] **Step 5: Checkpoint** — leave unstaged.

---

## Task 10: Unread badge + error-only toast

**Files:**
- Modify: `src/go/internal/ui/logs_component.go` (unread count), `grid_pane_draw.go` (badge), `game_struct.go`, `game_update.go`, `drumview_ctor.go`
- Create: `src/go/internal/ui/logs_toast.go`, add tests to `logs_panel_functional_test.go`

**Interfaces:**
- Produces: `(*LogsComponent).unreadCount() (n int, hasErr bool)`, `(*Game).drawLogsBadge(dst)`, `type logsToast struct{...}`, `(*DrumView).pushErrorToast(text string)`.

- [ ] **Step 1: Write failing tests**

Add to `logs_panel_functional_test.go`:
```go
func TestUnreadBadgeCountsAndClearsOnOpen(t *testing.T) {
	prev := eventlogger.History()
	t.Cleanup(func() { eventlogger.SetLogHistory(prev) })
	ring := eventlogger.NewRingHistory(50)
	eventlogger.SetLogHistory(ring)
	dv := newTestDrumView(t)
	c := NewLogsComponent(dv)
	base := time.Date(2026, 7, 19, 12, 0, 0, 0, time.Local)
	for i := 0; i < 3; i++ {
		ring.Record(eventlogger.LogRecord{Seq: int64(i + 1), At: base, Tag: "node", Msg: "x"})
	}
	if n, _ := c.unreadCount(); n != 3 {
		t.Fatalf("unread=%d want 3", n)
	}
	ring.Record(eventlogger.LogRecord{Seq: 99, At: base, Tag: "error", Msg: "boom", IsErr: true})
	if _, hasErr := c.unreadCount(); !hasErr {
		t.Fatal("hasErr should be true when an unread record is an error")
	}
	c.markSeen()
	if n, _ := c.unreadCount(); n != 0 {
		t.Fatalf("after markSeen unread=%d want 0", n)
	}
}

func TestErrorToastFiresOnErrorNotOnInfo(t *testing.T) {
	dv := newTestDrumView(t)
	dv.notifyInfo("hello")
	if dv.logsToastActive() {
		t.Fatal("info must NOT raise a toast")
	}
	dv.notifyError("boom")
	if !dv.logsToastActive() {
		t.Fatal("error must raise a toast")
	}
}
```

- [ ] **Step 2: Run to verify fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestUnreadBadge|TestErrorToast'`
Expected: FAIL — undefined `unreadCount`, `logsToastActive`.

- [ ] **Step 3: Implement unread count**

In `logs_component.go`:
```go
// unreadCount returns how many event records have Seq > lastSeenSeq, and
// whether any of them is an error.
func (c *LogsComponent) unreadCount() (int, bool) {
	n := 0
	hasErr := false
	for _, e := range eventlogger.History().Snapshot() {
		if e.Seq > c.lastSeenSeq {
			n++
			if e.IsErr {
				hasErr = true
			}
		}
	}
	return n, hasErr
}
```

- [ ] **Step 4: Implement the error-only toast**

Create `src/go/internal/ui/logs_toast.go`:
```go
package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// logsToastTTLFrames is how long an error toast stays fully visible before
// fading (≈4s at 60fps).
const logsToastTTLFrames = 240

type logsToast struct {
	text     string
	frames   int // remaining frames; 0 = inactive
}

func (t *logsToast) active() bool { return t.frames > 0 }
func (t *logsToast) push(text string) { t.text = text; t.frames = logsToastTTLFrames }
func (t *logsToast) tick() {
	if t.frames > 0 {
		t.frames--
	}
}

// draw renders the toast near the top of the grid pane in error red.
func (t *logsToast) draw(dst *ebiten.Image, gridRect image.Rectangle) {
	if !t.active() {
		return
	}
	w := TextWidth(t.text) + 2*SpaceMD
	h := TextHeight() + 2*SpaceSM
	cx := gridRect.Min.X + gridRect.Dx()/2
	r := image.Rect(cx-w/2, gridRect.Min.Y+SpaceMD, cx+w/2, gridRect.Min.Y+SpaceMD+h)
	drawRoundedRect(dst, r, colSurface3, RadiusMD, true)
	drawRoundedRect(dst, r, colError, RadiusMD, false)
	DrawTextColorAt(dst, t.text, r.Min.X+SpaceMD, r.Min.Y+SpaceSM, colError)
	_ = i18n.KeyLogs // toast text is the notification message itself
}
```
Add `logsToast logsToast` to `DrumView` (in `drumview.go`) and helpers in `drumview_logs_panel.go`:
```go
func (dv *DrumView) pushErrorToast(text string) { dv.logsToast.push(text) }
func (dv *DrumView) logsToastActive() bool      { return dv.logsToast.active() }
```
In `drumview_notifications.go` `pushNotif`/`pushNotifKey`, after the `notifStore.Push`, add:
```go
	if isErr {
		dv.pushErrorToast(msg) // for keyed variant, use i18n.Tf(key, ...) rendered text
	}
```
For the keyed path, render the text via the same `notification{...}.display()` you just pushed. Tick + draw the toast: in `game_update.go` call `dv.logsToast.tick()` once per frame; in `grid_pane_draw.go` (end of `drawGridHelpButton` or a sibling) call `dv.logsToast.draw(dst, gridRect)`.

- [ ] **Step 5: Draw the badge**

Add `(*Game).drawLogsBadge(dst *ebiten.Image)` called from `grid_pane_draw.go` after drawing the logs icon:
```go
func (g *Game) drawLogsBadge(dst *ebiten.Image) {
	if g.dv == nil || g.dv.logsComp == nil {
		return
	}
	n, hasErr := g.dv.logsComp.unreadCount()
	if n <= 0 {
		return
	}
	r := g.gridLogsButtonRect()
	bs := 12
	br := image.Rect(r.Max.X-bs, r.Min.Y-bs/2, r.Max.X+bs/2, r.Min.Y+bs/2)
	fill := colPrimary
	if hasErr {
		fill = colError
	}
	drawRoundedRect(dst, br, fill, RadiusSM, true)
	// count text centered (single digit / "9+")
	label := "9+"
	if n < 10 {
		label = strconv.Itoa(n)
	}
	DrawTextColorAt(dst, label, br.Min.X+(br.Dx()-TextWidth(label))/2, br.Min.Y, colOnPrimary)
}
```
> Ensure `g.dv.logsComp` is created eagerly (in `NewLogsComponent` at DrumView ctor) so the badge shows before the panel is first opened. In `drumview_ctor.go`, set `dv.logsComp = nil` initially and lazily create it in a new `dv.ensureLogsComp()` the badge calls; simplest: create it in the ctor after `notifStore`. Use `colOnPrimary`/`colOnSurfaceAccent` per DESIGN.md for text on the accent fill.

- [ ] **Step 6: Run to verify pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestUnreadBadge|TestErrorToast'`
Expected: PASS.

- [ ] **Step 7: Checkpoint** — token discipline + i18n: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestTokenDiscipline|TestDesignMDDrift'`. Leave unstaged.

---

## Task 11: Remove the in-band notification component + old popup; final discipline/functional sweep

**Files:**
- Modify: `timeline_zone.go`, `drumview_layout.go`, `drumview_ctor.go`, `game_settings_gear_zone_test.go`
- Delete: `notification_marquee.go`, `drumview_notif_history.go` (+ any `*_test.go` referencing them)

**Interfaces:**
- Removes: `TimelineZone` `NotifRect/NotifLatest/OnNotifClick` callbacks, `drawNotifArea`, `notifClickAdapter`, `dv.notifRect`, `openNotifHistoryPortal`, `computeNotifHistoryRect`, `drawNotifHistory`, `IsNotifHistoryOpen`, `notifMarqueeOffsetX`.

- [ ] **Step 1: Write the failing regression test**

Add to `logs_panel_functional_test.go`:
```go
func TestInBandNotificationAreaRemoved(t *testing.T) {
	g := newTestGame(t)
	// notifRect must no longer be carved.
	if g.dv.notifRectForTest() != (image.Rectangle{}) {
		t.Fatal("in-band notifRect should be gone (empty)")
	}
	// The old notif-history popup portal must not exist.
	g.dv.notifyInfo("hello")
	updateFrames(t, g, 2)
	if g.dv.portal().Has("notif-history") {
		t.Fatal("old notif-history popup should be removed")
	}
}
```
Add a tiny test accessor in `drumview_notifications.go`:
```go
func (dv *DrumView) notifRectForTest() image.Rectangle { return dv.notifRect }
```
(Keep the `notifRect` field but never assign it → always empty. Or delete the field and have the accessor return `image.Rectangle{}` — pick deletion; then this accessor returns the zero value directly.)

- [ ] **Step 2: Run to verify fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestInBandNotificationAreaRemoved`
Expected: FAIL (popup still registered / notifRect still carved) or compile error (guides the deletions).

- [ ] **Step 3: Remove the in-band draw + callbacks**

- `timeline_zone.go`: delete `drawNotifArea`, its call at line ~271, the `NotifRect/NotifLatest/OnNotifClick` fields (lines ~50-52), the `"timeline-notif"` hit-area block (~1202-1215), and `notifClickAdapter` (~1239-1251).
- `drumview_layout.go`: delete the `dv.notifRect = ...` carving (lines ~648-685); let the beat counter/readout use the reclaimed width (the `bandRight` math stays; just stop reserving the notif slot).
- `drumview_ctor.go`: delete the `NotifRect/NotifLatest/OnNotifClick` bindings (~1080-1091).
- Delete `notification_marquee.go`.

- [ ] **Step 4: Remove the old popup**

- Delete `drumview_notif_history.go`.
- Remove any references to `openNotifHistoryPortal`, `IsNotifHistoryOpen`, `computeNotifHistoryRect`, `drawNotifHistory`, `notifHistoryRect` across the package (grep and delete). `formatNotifTime` is unused after this — delete it too.

Run: `cd src/go && ../../.tools/go/bin/go build -tags test ./internal/ui/` and fix each undefined reference until it compiles.

- [ ] **Step 5: Widen the gear protected-zone test**

In `game_settings_gear_zone_test.go`, extend the "topmost / avoided by menus" assertions to also cover `g.gridLogsButtonRect()` (both icons are protected, non-overlapping, both at z≈300). Add:
```go
func TestLogsIconInProtectedZone(t *testing.T) {
	g := newTestGame(t)
	gear := g.gridHelpButtonRect()
	logs := g.gridLogsButtonRect()
	if logs.Overlaps(gear) {
		t.Fatal("logs icon must not overlap the gear")
	}
	if logs.Max.X > gear.Min.X {
		t.Fatal("logs icon must sit left of the gear")
	}
}
```

- [ ] **Step 6: Run the full package suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS. Fix any test that referenced the removed notif area (e.g. update or delete `drumview_notif_history_test.go` if present, and any timeline-zone test asserting the notif slot).

- [ ] **Step 7: Cross-package + discipline sweep**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod \
  ./internal/ui ./internal/eventlogger ./internal/i18n ./internal/hooks
```
Then design guards:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod \
  -run 'TestDesignMDDrift|TestTokenDiscipline|TestDesignMDLintSnapshot' ./internal/ui/
```
Expected: all PASS.

- [ ] **Step 8: Checkpoint** — leave all changes unstaged for user review.

---

## Self-Review

**Spec coverage:**
- §1 data model & history → Tasks 1, 2, 3. ✓
- §1 category derivation (data-driven) → Task 5 `distinctTags` + Task 8 chips + auto-new-tag test. ✓
- §1 notifications merge at read time → Task 5 `mergeLogRows` + Task 7 `notifInputs`. ✓
- §2 panel form/plumbing (compOverlayable, portal) → Task 6 + 7. ✓
- §2 search-by-name (fuzzy, tag included) → Task 5 `filterRows` + Task 8. ✓
- §2 timestamp exact + range → Task 5 `matchTimeAt`/`matchTimeRange` + Task 9. ✓
- §2 category chips → Task 8. ✓
- §2 rows via drawMenuRow/timestamp trailing → Task 7 (note: Task 7 draws rows inline with tag+msg+timestamp; drawMenuRow upgrade is optional polish — the spec's intent (tag label + msg + right-aligned timestamp + error color + fuzzy highlight) is met. If strict `drawMenuRow` reuse is required, swap the inline row draw for `drawMenuRow(MenuRowSpec{Label:msg, Highlights:row.highlights, TrailLabel:ts, ...})` — same data). ✓
- §2 pagination → Task 5 `pageSlice`/`pageCount` + Task 7 footer. ✓
- §3 logs icon next to gear → Task 6. ✓
- §3 unread badge → Task 10. ✓
- §3 error-only toast → Task 10. ✓
- §4 removal of in-band area + old popup → Task 11. ✓
- §5 DESIGN.md compliance → checkpoints in Tasks 7, 10, 11. ✓
- §6 testing → each task is TDD; functional tests in Tasks 6, 8, 10, 11. ✓

**Placeholder scan:** No "TBD"/"handle edge cases"/"similar to Task N". Two annotated flexibilities remain (test-helper constructor names, exact stroke-helper name) — both instruct the implementer to read one named neighbor file for the exact symbol, with the behavior fully specified. These are unavoidable because the harness helper names aren't visible from the spec; they are not placeholders for logic.

**Type consistency:** `LogRecord`/`LogHistory`/`History`/`NewRingHistory` consistent across Tasks 2, 3, 5, 7, 10. `logRow`/`filterRows`/`mergeLogRows`/`pageSlice`/`pageCount`/`distinctTags`/`matchTimeAt`/`matchTimeRange`/`timestampMode` consistent across Tasks 5, 7, 8, 9. `LogsComponent` methods (`rebuild`/`reapply`/`visibleRows`/`pageCountNow`/`unreadCount`/`markSeen`/`toggleTag`/`applyNameQuery`/`setTimeAt`/`setTimeRange`) consistent across Tasks 6–10. `gridLogsButtonRect`/`toggleLogsPanel`/`gridLogsInputHandler`/`IsLogsPanelOpen`/`openLogsPanel` consistent across Tasks 6, 10, 11.

**Follow-up folded in:** Task 10 Step 5 notes the badge needs `logsComp` created eagerly — reconciled by creating it in the DrumView ctor (Task 6 adds the field; Task 10 ensures eager creation).
