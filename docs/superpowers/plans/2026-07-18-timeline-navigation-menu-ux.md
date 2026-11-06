# Menu-hosted Timeline Navigation UX — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move precise timeline navigation (jump by N beats/subdivisions, go-to-beat, go-to-time) into a NAVIGATION panel inside a sectioned overflow menu on both platforms, declutter the transport to a single Navigate button, and slim the ribbon into a whole-song progress bar.

**Architecture:** All jumps continue to funnel through the existing `DrumView.RequestSeek*` → `drainSeekRequest` → `Game.SeekSteps` chokepoint (adding a subdivision-delta variant). The NAVIGATION panel is a bounded set of Buttons rendered in a reserved top region of the existing (row-based) overflow menu, reusing `SubdivMenuComponent` (amount picker) and `ParamValueEditor` (go-to fields). The transport loses `rwBtn`/`ffBtn`/`jumpBtn` and gains a `navBtn` (reusing the existing `IconTarget` crosshair). The ribbon drops single-click seek + the drum-window view-rect and renders progress over `[0, timelineBeats]`.

**Tech Stack:** Go 1.26 (`.tools/go/bin/go`), Ebiten v2.9, the `internal/ui` Zone/HitArea/portal tree, `SubdivMenuComponent`, `ParamValueEditor`.

## Global Constraints

- **Go binary:** always `../../.tools/go/bin/go` (from `src/go/`); tests `-tags test -modfile=go.test.mod`.
- **NEVER git add/commit/stash/checkout/reset** (repo rule). Every "Commit" step below means: STOP, leave the change uncommitted for the user. Do not run git state-changing commands. (A prior agent nearly lost work via `git stash` — do not touch git state at all.)
- **Verify test COMPILATION, not just build:** `go build` skips `_test.go`. After each task run `../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run '^$' ./internal/ui/` to confirm the test binary compiles.
- **Do NOT run the full `./internal/ui/` suite** (>15 min under load) — run targeted `-run` patterns only.
- **Concurrent edits:** the user edits this working tree live; transient build failures in files you did NOT touch (e.g. `grid_mode.go`, `synth_focus_graph.go`) are theirs — retry; if a non-your-file won't compile, note it and proceed with what you can verify.
- **Functional tests over isolation:** drive real `Game.Update` where behavior is user-facing.
- **Design tokens:** no new inline `color.RGBA{}` literals; use existing `Token*`/`col*` accessors. Forbidden glyphs (`▶▼▲✕≡⏸■↑↓→←`) — use `IconID`.
- **Jump amount is `{1,2,4,8,16}`** via the `SubdivMenuComponent` idiom (default 4). Go-to-time seeks to the **closest beat at or after** the timestamp: `targetSteps = ceil(seconds*bpm/60)*div`, clamped to `[0, timelineBeats*div]`.
- All jumps go through `SeekSteps` (never a direct seek from an unlocked callback); UI entry points call `dv.RequestSeek*`, drained under `seqMu`.

---

## File Structure

- **Create** `internal/ui/navigation_panel.go` — `NavigationPanel` component (state, layout, draw, input) for the NAVIGATION section.
- **Create** `internal/ui/nav_time.go` — pure helpers: parse `M:SS` → seconds; `timeToSteps(sec, bpm, div, maxSteps)`; `formatBeatTime(steps, div, bpm)` → `"Beat 12 · 0:24"`.
- **Modify** `internal/ui/drumview_seek.go`, `game_seek.go` — add `RequestSeekDeltaSteps` + widen `TakeSeekRequest`/`drainSeekRequest`.
- **Modify** `internal/ui/drumview_context_menu.go` — reserve a top region for the panel; draw + route input to it; add the "NAVIGATION" section header.
- **Modify** `internal/ui/transport_zone.go`, `drumview_ctor.go` — remove `rwBtn`/`ffBtn`/`jumpBtn`; add `navBtn`; wire `OnNavigate`.
- **Modify** `internal/ui/timeline_zone.go` — ribbon: remove single-click seek, keep drag; shrink; progress over `[0, timelineBeats]`; repoint beat counter to open the menu.
- **Tests:** `nav_time_test.go`, `navigation_panel_test.go`, `nav_menu_test.go`, updates to `transport_jump_test.go`/`transport_seek_buttons_test.go`/`timeline_seek_test.go`/`transport_equal_squares_test.go`.

Reference signatures (verbatim from exploration):
- `overflowItem{label,iconID,active,header,search,highlights,trail,onClick}` — `drumview_context_menu.go:898`. Page-0 items built at `:964-1005`; `overflowPopupRect()` height `=len(items)*touchMinTargetPx` at `:745`; `drawOverflowMenu` at `:1109`; bottom-sheet vs desktop via `Profile().UseBottomSheet`.
- `OpenOverflowMenu()` `:1470` → `openOverflowMenuPortal()` (`drumview_portal_open.go:179`), portal ID `"overflow-menu"`, `IsOverflowMenuOpen()` = `portal().Has(...)` (`drumview_notifications.go:143`).
- `SubdivMenuComponent` + `SubdivMenuProps{AnchorRect,Current,Options,OnSelect,OnClose,RowHeight}`; `NewSubdivMenuComponent()`.
- `ParamValueEditor.OpenValue(ValueOpen{Spec,Anchor,Clamp,Get,Set,MobileInputID})`; `positionValueSpec(div,maxSteps)` (`timeline_position_spec.go`).
- `RequestSeekSteps(steps,origin)`, `RequestSeekDeltaBeats(deltaBeats,origin)`, `TakeSeekRequest()`, `drainSeekRequest()` (`drumview_seek.go`, `game_seek.go:121`).
- Transport removal sites (`transport_zone.go`): fields 61-63; callbacks 29-31; construction 211-237; styling 376-386; layout `square()` 940-958 + `nGaps`/`totalUnits` 927-928; mobile empties 879-887; hit-areas `simpleButtons` 1016 (jump) + `repeatButtonHitAdapter` 1055-1056 (rw/ff); dirty-hash 1369-1382; expand 1478-1487; Draw cache 1573-1586 + direct 1639-1652; wiring `drumview_ctor.go:675-676,787-795`.
- Ribbon (`timeline_zone.go`): `drawTimelineBar:365` (view-rect 429-446, cursor 448-478, ghost 483-485), `ribbonWindowBeats:336`, `timelineScrubHitAdapter` (type 1509, OnPress seek 1524-1526, OnDrag 1530, OnRelease 1550), `ribbonStepAt:1471`, `timelineBarH:99`, `SetTimelineBarHeight:176`, beat counter `beatCounterHitAdapter:1283` (`openPositionEditor:1300`).
- `IconTarget IconID="target"` (`icons.go:46`, `drawTargetIcon` `drawing_icons.go:425`) — the crosshair for `navBtn`.

---

## Task 1: Subdivision-delta seek request (`RequestSeekDeltaSteps`)

**Files:** Modify `internal/ui/drumview_seek.go`, `internal/ui/game_seek.go`; Test `internal/ui/nav_seek_delta_test.go` (create).

**Interfaces:**
- Produces: `func (dv *DrumView) RequestSeekDeltaSteps(deltaSteps int, origin string)`; widened `TakeSeekRequest() (steps, deltaBeats, deltaSteps int, mode seekReqMode, origin string, ok bool)` where `seekReqMode ∈ {seekReqAbs, seekReqDeltaBeats, seekReqDeltaSteps}`.
- Consumes: existing `RequestSeekSteps`/`RequestSeekDeltaBeats`, `g.playheadAbsSubdiv()`, `g.SeekSteps`.

- [ ] **Step 1: Write the failing test**

```go
//go:build test

package ui

import "testing"

func TestRequestSeekDeltaStepsMovesBySubdivisions(t *testing.T) {
	g := newGameWithStart(t)
	div := g.grid.MaxDiv()
	g.SeekSteps(8*div, seekOriginJS) // start at beat 8
	g.drum.RequestSeekDeltaSteps(3, seekOriginCounter)
	g.drainSeekRequest()
	if got, want := g.playheadAbsSubdiv(), 8*div+3; got != want {
		t.Fatalf("playhead=%d want %d (+3 subdivisions, no beat snap)", got, want)
	}
}

func TestRequestSeekDeltaStepsNegativeClampsAtZero(t *testing.T) {
	g := newGameWithStart(t)
	g.SeekSteps(2, seekOriginJS)
	g.drum.RequestSeekDeltaSteps(-10, seekOriginCounter)
	g.drainSeekRequest()
	if got := g.playheadAbsSubdiv(); got != 0 {
		t.Fatalf("playhead=%d want 0 (clamped)", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestRequestSeekDeltaSteps' ./internal/ui/`
Expected: FAIL — `RequestSeekDeltaSteps undefined`.

- [ ] **Step 3: Implement**

In `drumview_seek.go`, replace the `seekRequest` struct + methods with a mode-based version:

```go
type seekReqMode int

const (
	seekReqAbs seekReqMode = iota
	seekReqDeltaBeats
	seekReqDeltaSteps
)

type seekRequest struct {
	value  int // steps (abs) OR delta (beats/steps)
	mode   seekReqMode
	origin string
}

func (dv *DrumView) RequestSeekSteps(steps int, origin string) {
	dv.pendingSeek = &seekRequest{value: steps, mode: seekReqAbs, origin: origin}
}

func (dv *DrumView) RequestSeekDeltaBeats(deltaBeats int, origin string) {
	dv.pendingSeek = &seekRequest{value: deltaBeats, mode: seekReqDeltaBeats, origin: origin}
}

func (dv *DrumView) RequestSeekDeltaSteps(deltaSteps int, origin string) {
	dv.pendingSeek = &seekRequest{value: deltaSteps, mode: seekReqDeltaSteps, origin: origin}
}

func (dv *DrumView) TakeSeekRequest() (value int, mode seekReqMode, origin string, ok bool) {
	r := dv.pendingSeek
	if r == nil {
		return 0, seekReqAbs, "", false
	}
	dv.pendingSeek = nil
	return r.value, r.mode, r.origin, true
}
```

In `game_seek.go`, rewrite `drainSeekRequest`:

```go
func (g *Game) drainSeekRequest() {
	value, mode, origin, ok := g.drum.TakeSeekRequest()
	if !ok {
		return
	}
	g.seqMu.Lock()
	defer g.seqMu.Unlock()
	div := max1(g.grid.MaxDiv())
	var steps int
	switch mode {
	case seekReqDeltaBeats:
		cur := g.playheadAbsSubdiv()
		snapped := int(math.Round(float64(cur)/float64(div))) * div
		steps = snapped + value*div
	case seekReqDeltaSteps:
		steps = g.playheadAbsSubdiv() + value // no beat snap — subdivision granular
	default: // seekReqAbs
		steps = value
	}
	g.SeekSteps(steps, origin)
}
```

- [ ] **Step 4: Update the one other `TakeSeekRequest` caller shape**

`RequestSeekDeltaBeats(-1,...)` callers in `drumview_ctor.go:675-676` are removed in Task 5; no other production caller of `TakeSeekRequest` exists (grep to confirm: `grep -rn TakeSeekRequest internal/ui/ | grep -v _test`). Update `game_seek_request_test.go` if it destructures the old 5-tuple — change to the new `(value, mode, origin, ok)` shape.

- [ ] **Step 5: Run tests + compile-check**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestRequestSeekDelta|TestSeekRequest' ./internal/ui/`
Expected: PASS.
Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run '^$' ./internal/ui/` — ok (test binary compiles).

- [ ] **Step 6: Commit** (STOP — leave uncommitted)
```
feat(ui): subdivision-delta seek request (RequestSeekDeltaSteps)
```

---

## Task 2: Time helpers (`M:SS` ↔ steps, position label)

**Files:** Create `internal/ui/nav_time.go`, `internal/ui/nav_time_test.go`.

**Interfaces:**
- Produces: `func parseClock(s string) (seconds float64, ok bool)`; `func timeToSteps(seconds float64, bpm, div, maxSteps int) int`; `func formatBeatTime(steps, div, bpm int) string`.

- [ ] **Step 1: Write the failing test**

```go
//go:build test

package ui

import "testing"

func TestParseClock(t *testing.T) {
	cases := []struct{ s string; sec float64; ok bool }{
		{"0:30", 30, true}, {"1:05", 65, true}, {"12", 12, true}, {"2:00", 120, true},
		{"", 0, false}, {"a:b", 0, false}, {"1:99", 0, false},
	}
	for _, c := range cases {
		sec, ok := parseClock(c.s)
		if ok != c.ok || (ok && sec != c.sec) {
			t.Fatalf("parseClock(%q)=(%v,%v) want (%v,%v)", c.s, sec, ok, c.sec, c.ok)
		}
	}
}

func TestTimeToStepsClosestBeatAtOrAfter(t *testing.T) {
	div, bpm := 8, 120 // 2 beats/sec → 30s = beat 60
	maxSteps := 1000 * div
	// 30s → exactly beat 60 → 60*div.
	if got := timeToSteps(30, bpm, div, maxSteps); got != 60*div {
		t.Fatalf("timeToSteps(30)=%d want %d", got, 60*div)
	}
	// 30.1s → 60.2 beats → ceil to beat 61.
	if got := timeToSteps(30.1, bpm, div, maxSteps); got != 61*div {
		t.Fatalf("timeToSteps(30.1)=%d want %d (ceil to next beat)", got, 61*div)
	}
	// clamp above song length.
	if got := timeToSteps(1e9, bpm, div, maxSteps); got != maxSteps {
		t.Fatalf("timeToSteps(huge)=%d want %d (clamped)", got, maxSteps)
	}
}

func TestFormatBeatTime(t *testing.T) {
	// beat 60 at 120bpm, div 8 → "Beat 60 · 0:30".
	if got := formatBeatTime(60*8, 8, 120); got != "Beat 60 · 0:30" {
		t.Fatalf("formatBeatTime=%q want %q", got, "Beat 60 · 0:30")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestParseClock|TestTimeToSteps|TestFormatBeatTime' ./internal/ui/`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement `nav_time.go`**

(No build tag — this is a production file compiled in every build, like `timeline_position_spec.go`.)

```go
package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// parseClock parses "M:SS" or a bare seconds count into seconds. Seconds field
// must be 0..59.
func parseClock(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if i := strings.IndexByte(s, ':'); i >= 0 {
		m, err1 := strconv.Atoi(strings.TrimSpace(s[:i]))
		sec, err2 := strconv.Atoi(strings.TrimSpace(s[i+1:]))
		if err1 != nil || err2 != nil || m < 0 || sec < 0 || sec > 59 {
			return 0, false
		}
		return float64(m*60 + sec), true
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return 0, false
	}
	return float64(v), true
}

// timeToSteps returns the absolute subdivision index of the closest beat AT OR
// AFTER `seconds`, clamped to [0, maxSteps].
func timeToSteps(seconds float64, bpm, div, maxSteps int) int {
	if bpm <= 0 {
		bpm = 120
	}
	if div <= 0 {
		div = 1
	}
	beat := int(math.Ceil(seconds * float64(bpm) / 60.0))
	steps := beat * div
	if steps < 0 {
		steps = 0
	}
	if maxSteps > 0 && steps > maxSteps {
		steps = maxSteps
	}
	return steps
}

// formatBeatTime renders "Beat <b> · <m:ss>" for a position readout.
func formatBeatTime(steps, div, bpm int) string {
	if div <= 0 {
		div = 1
	}
	if bpm <= 0 {
		bpm = 120
	}
	beat := steps / div
	sec := int(math.Round(float64(beat) * 60.0 / float64(bpm)))
	return fmt.Sprintf("Beat %d · %d:%02d", beat, sec/60, sec%60)
}
```

(Production file — no build tag, matching `timeline_position_spec.go`.)

- [ ] **Step 4: Run tests + compile-check**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestParseClock|TestTimeToSteps|TestFormatBeatTime' ./internal/ui/`
Expected: PASS.

- [ ] **Step 5: Commit** (STOP — leave uncommitted)
```
feat(ui): nav time helpers (M:SS parse, closest-beat-after, position label)
```

---

## Task 3: NavigationPanel component

**Files:** Create `internal/ui/navigation_panel.go`, `internal/ui/navigation_panel_test.go`.

**Interfaces:**
- Produces: `NavigationPanel` with `SetCallbacks(NavPanelCallbacks)`, `Layout(rect image.Rectangle)`, `PanelHeight() int`, `Draw(dst *ebiten.Image)`, `HandleInput(x, y int, pressed bool) InputResult`, `Buttons() []*Button` (for hit registration).
- `NavPanelCallbacks{PlayheadStep func() int; BPM func() int; Div func() int; TimelineBeats func() int; JumpAmount func() int; JumpUnitBeats func() bool; OnPrev func(); OnNext func(); OnPickAmount func(); OnToggleUnit func(); OnGotoBeat func(); OnGotoTime func()}`.
- Consumes: `Button`, `drawMenuRow`/`DrawTextAt`, `IconRewind`/`IconFastForward`.

**Design:** A compact panel, laid out top-to-bottom inside `rect`:
- Line 1 (readout, `touchMinTargetPx` tall): `formatBeatTime(...)` via `DrawTextAt` at role Body.
- Line 2 (jump row): `[◀ Prev]`  `[amount ×N]`  `[Next ▶]`  `[unit: Beats|Subdiv]` — four Buttons laid out horizontally, equal-ish widths, `touchMinTargetPx` tall.
- Line 3 (go-to beat): a full-width `[Go to beat…]` Button.
- Line 4 (go-to time): a full-width `[Go to time…]` Button.
`PanelHeight()` returns `4 * touchMinTargetPx`. The six Buttons' `OnClick` call the matching callback. Prev/Next carry `IconRewind`/`IconFastForward`; amount button's `Text` is `"×"+JumpAmount()`; unit button's `Text` is `"Beats"` or `"Subdiv"`.

- [ ] **Step 1: Write the failing test**

```go
//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestNavigationPanelWiresControls(t *testing.T) {
	var prev, next, pick, unit, gotoBeat, gotoTime int
	p := &NavigationPanel{}
	p.SetCallbacks(NavPanelCallbacks{
		PlayheadStep:  func() int { return 96 },
		BPM:           func() int { return 120 },
		Div:           func() int { return 8 },
		TimelineBeats: func() int { return 64 },
		JumpAmount:    func() int { return 4 },
		JumpUnitBeats: func() bool { return true },
		OnPrev:        func() { prev++ },
		OnNext:        func() { next++ },
		OnPickAmount:  func() { pick++ },
		OnToggleUnit:  func() { unit++ },
		OnGotoBeat:    func() { gotoBeat++ },
		OnGotoTime:    func() { gotoTime++ },
	})
	p.Layout(image.Rect(0, 0, 320, 4*touchMinTargetPx))
	if p.PanelHeight() != 4*touchMinTargetPx {
		t.Fatalf("PanelHeight=%d want %d", p.PanelHeight(), 4*touchMinTargetPx)
	}
	// Drawing must not panic and must render (pixel presence).
	dst := ebiten.NewImage(320, 4*touchMinTargetPx)
	p.Draw(dst)
	// Each control button's OnClick fires its callback.
	fired := map[string]*int{"prev": &prev, "next": &next, "pick": &pick, "unit": &unit, "gotoBeat": &gotoBeat, "gotoTime": &gotoTime}
	for _, b := range p.Buttons() {
		if b.OnClick != nil {
			b.OnClick()
		}
	}
	for name, c := range fired {
		if *c == 0 {
			t.Fatalf("control %q OnClick never fired a callback", name)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestNavigationPanel' ./internal/ui/`
Expected: FAIL — `NavigationPanel undefined`.

- [ ] **Step 3: Implement `navigation_panel.go`** (production file — no build tag)

```go
package ui

import (
	"image"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
)

type NavPanelCallbacks struct {
	PlayheadStep  func() int
	BPM           func() int
	Div           func() int
	TimelineBeats func() int
	JumpAmount    func() int
	JumpUnitBeats func() bool
	OnPrev        func()
	OnNext        func()
	OnPickAmount  func()
	OnToggleUnit  func()
	OnGotoBeat    func()
	OnGotoTime    func()
}

type NavigationPanel struct {
	cb        NavPanelCallbacks
	rect      image.Rectangle
	readout   image.Rectangle
	prevBtn   *Button
	amountBtn *Button
	nextBtn   *Button
	unitBtn   *Button
	gotoBeat  *Button
	gotoTime  *Button
}

func (p *NavigationPanel) SetCallbacks(cb NavPanelCallbacks) {
	p.cb = cb
	if p.prevBtn == nil {
		p.prevBtn = NewButton("", nil, func() { p.call(p.cb.OnPrev) })
		p.prevBtn.Icon = string(IconRewind)
		p.amountBtn = NewButton("", nil, func() { p.call(p.cb.OnPickAmount) })
		p.nextBtn = NewButton("", nil, func() { p.call(p.cb.OnNext) })
		p.nextBtn.Icon = string(IconFastForward)
		p.unitBtn = NewButton("", nil, func() { p.call(p.cb.OnToggleUnit) })
		p.gotoBeat = NewButton("Go to beat…", nil, func() { p.call(p.cb.OnGotoBeat) })
		p.gotoTime = NewButton("Go to time…", nil, func() { p.call(p.cb.OnGotoTime) })
	}
}

func (p *NavigationPanel) call(f func()) {
	if f != nil {
		f()
	}
}

func (p *NavigationPanel) PanelHeight() int { return 4 * touchMinTargetPx }

func (p *NavigationPanel) Layout(rect image.Rectangle) {
	p.rect = rect
	rowH := touchMinTargetPx
	x0, w := rect.Min.X, rect.Dx()
	y := rect.Min.Y
	p.readout = image.Rect(x0, y, x0+w, y+rowH)
	y += rowH
	// jump row: prev | amount | next | unit  (4 equal cells)
	cw := w / 4
	p.prevBtn.SetRect(image.Rect(x0, y, x0+cw, y+rowH))
	p.amountBtn.SetRect(image.Rect(x0+cw, y, x0+2*cw, y+rowH))
	p.nextBtn.SetRect(image.Rect(x0+2*cw, y, x0+3*cw, y+rowH))
	p.unitBtn.SetRect(image.Rect(x0+3*cw, y, x0+w, y+rowH))
	y += rowH
	p.gotoBeat.SetRect(image.Rect(x0, y, x0+w, y+rowH))
	y += rowH
	p.gotoTime.SetRect(image.Rect(x0, y, x0+w, y+rowH))
	// live labels
	if p.cb.JumpAmount != nil {
		p.amountBtn.Text = "×" + strconv.Itoa(p.cb.JumpAmount())
	}
	if p.cb.JumpUnitBeats != nil {
		if p.cb.JumpUnitBeats() {
			p.unitBtn.Text = "Beats"
		} else {
			p.unitBtn.Text = "Subdiv"
		}
	}
}

func (p *NavigationPanel) Buttons() []*Button {
	return []*Button{p.prevBtn, p.amountBtn, p.nextBtn, p.unitBtn, p.gotoBeat, p.gotoTime}
}

func (p *NavigationPanel) Draw(dst *ebiten.Image) {
	// position readout
	if p.cb.PlayheadStep != nil && p.cb.Div != nil && p.cb.BPM != nil {
		txt := formatBeatTime(p.cb.PlayheadStep(), p.cb.Div(), p.cb.BPM())
		DrawTextAt(dst, txt, p.readout.Min.X+SpaceMD, p.readout.Min.Y+p.readout.Dy()/2-RoleBodyLineHalf(), colTextPrimary)
	}
	for _, b := range p.Buttons() {
		b.Draw(dst)
	}
}

func (p *NavigationPanel) HandleInput(x, y int, pressed bool) InputResult {
	for _, b := range p.Buttons() {
		if pt(x, y, b.Rect()) {
			if pressed && b.OnClick != nil {
				b.OnClick()
			}
			return InputConsumed
		}
	}
	return InputIgnored
}
```

Verify against reality: `DrawTextAt` signature (`textcache.go`), `pt(x,y,rect)` helper, `RoleBodyLineHalf()` (if no such helper, center text with the existing role-height accessor used by `drawMenuRow`; grep `RoleBody` usage in `menu_chrome.go` and mirror). `NewButton(text, style, onClick)` sets `.Text`/`.Icon`. If `colTextPrimary` isn't the right token, use the one `drawMenuRow` uses for labels.

- [ ] **Step 4: Run test + compile-check**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestNavigationPanel' ./internal/ui/`
Expected: PASS.

- [ ] **Step 5: Commit** (STOP — leave uncommitted)
```
feat(ui): NavigationPanel component (readout + jump/unit + go-to controls)
```

---

## Task 4: Embed NavigationPanel in the sectioned overflow menu

**Files:** Modify `internal/ui/drumview.go` (fields), `internal/ui/drumview_ctor.go` (construct + callbacks), `internal/ui/drumview_context_menu.go` (reserve region, header item, draw, input), `internal/ui/drumview_open_dialogs.go` (amount picker + go-to editors); Test `internal/ui/nav_menu_test.go`.

**Interfaces:**
- Consumes: `NavigationPanel` (Task 3), `RequestSeekDeltaBeats`/`RequestSeekDeltaSteps`/`RequestSeekSteps` (Task 1), `parseClock`/`timeToSteps` (Task 2), `positionValueSpec`, `SubdivMenuComponent`, `ParamValueEditor`.
- Produces: `DrumView.navPanel *NavigationPanel`, `DrumView.jumpBeats int` (retained), `DrumView.jumpUnitBeats bool` (new), `func (dv *DrumView) OpenNavigation()` (opens the overflow menu at page 0 where NAVIGATION lives).

- [ ] **Step 1: Write the failing test**

```go
//go:build test

package ui

import "testing"

func TestOverflowMenuHasNavigationSection(t *testing.T) {
	g := newGameWithStart(t)
	items := g.drum.overflowItems()
	var navHdr, fileHdr bool
	for _, it := range items {
		if it.header && it.label == "NAVIGATION" {
			navHdr = true
		}
		if it.header && it.label == KeyMenuFile {
			fileHdr = true
		}
	}
	if !navHdr {
		t.Fatal("overflow page-0 must contain a NAVIGATION section header")
	}
	if !fileHdr {
		t.Fatal("overflow page-0 must still contain the FILE section header")
	}
}

func TestNavPrevNextJumpByAmountAndUnit(t *testing.T) {
	g := newGameWithStart(t)
	div := g.grid.MaxDiv()
	g.SeekSteps(20*div, seekOriginJS)
	g.drum.jumpBeats = 4
	g.drum.jumpUnitBeats = true
	g.drum.navPanel.cb.OnNext() // +4 beats
	g.drainSeekRequest()
	if got := g.playheadAbsSubdiv(); got != 24*div {
		t.Fatalf("next(beats)=%d want %d", got, 24*div)
	}
	g.drum.jumpUnitBeats = false
	g.drum.navPanel.cb.OnPrev() // -4 subdivisions
	g.drainSeekRequest()
	if got := g.playheadAbsSubdiv(); got != 24*div-4 {
		t.Fatalf("prev(subdiv)=%d want %d", got, 24*div-4)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestOverflowMenuHasNavigationSection|TestNavPrevNext' ./internal/ui/`
Expected: FAIL — no NAVIGATION header / `navPanel` nil.

- [ ] **Step 3: Add fields + construct + wire callbacks**

`drumview.go`: add `navPanel *NavigationPanel` and `jumpUnitBeats bool` near `jumpBeats`.

`drumview_ctor.go`: after `jumpMenuComp` construction (~line 214), add:
```go
dv.jumpUnitBeats = true
dv.navPanel = &NavigationPanel{}
dv.navPanel.SetCallbacks(NavPanelCallbacks{
	PlayheadStep:  func() int { if dv.game != nil { return dv.game.playheadAbsSubdiv() }; return 0 },
	BPM:           func() int { return dv.BPM() },
	Div:           func() int { return dv.timelineUnitsPerBeat },
	TimelineBeats: func() int { return dv.timelineBeats },
	JumpAmount:    func() int { return dv.jumpBeats },
	JumpUnitBeats: func() bool { return dv.jumpUnitBeats },
	OnPrev: func() {
		if dv.jumpUnitBeats {
			dv.RequestSeekDeltaBeats(-dv.jumpBeats, seekOriginCounter)
		} else {
			dv.RequestSeekDeltaSteps(-dv.jumpBeats, seekOriginCounter)
		}
	},
	OnNext: func() {
		if dv.jumpUnitBeats {
			dv.RequestSeekDeltaBeats(dv.jumpBeats, seekOriginCounter)
		} else {
			dv.RequestSeekDeltaSteps(dv.jumpBeats, seekOriginCounter)
		}
	},
	OnPickAmount: func() { dv.OpenJumpMenu() },
	OnToggleUnit: func() { dv.jumpUnitBeats = !dv.jumpUnitBeats },
	OnGotoBeat:   func() { dv.openGotoBeatEditor() },
	OnGotoTime:   func() { dv.openGotoTimeEditor() },
})
```
(Use the real BPM accessor — `dv.BPM()` if present, else the transport's; grep. `dv.timelineUnitsPerBeat`/`dv.timelineBeats` are existing fields.)

- [ ] **Step 4: Add the go-to editors + amount OnSelect (drumview_open_dialogs.go)**

`OpenJumpMenu` already exists (amount picker); ensure its `OnSelect` also refreshes the panel amount label (it reads `dv.jumpBeats` live, so just `dv.jumpBeats = value` is enough — no transport pill anymore; remove the `dv.transportZone.SetJumpBeats` call in `OpenJumpMenu` since the pill is gone in Task 5). Add:

```go
func (dv *DrumView) openGotoBeatEditor() {
	div := max1(dv.timelineUnitsPerBeat)
	maxSteps := dv.timelineBeats * div
	dv.ensurePosEditor().OpenValue(ValueOpen{
		Spec:   positionValueSpec(div, maxSteps),
		Anchor: dv.overflowPopupRect(),
		Clamp:  dv.Bounds,
		Get:    func() float64 { if dv.game != nil { return float64(dv.game.playheadAbsSubdiv()) }; return 0 },
		Set:    func(v float64) { dv.RequestSeekSteps(int(v), seekOriginCounter) },
	})
}

func (dv *DrumView) openGotoTimeEditor() {
	div := max1(dv.timelineUnitsPerBeat)
	bpm := dv.BPM()
	maxSteps := dv.timelineBeats * div
	dv.ensurePosEditor().OpenValue(ValueOpen{
		Spec: ValueSpec{
			MinW: 72, MaxLen: 6,
			Format: func(v float64) string { return formatClock(int(v)) },          // v = current seconds
			Parse:  func(s string) (float64, bool) { sec, ok := parseClock(s); return sec, ok },
		},
		Anchor: dv.overflowPopupRect(),
		Clamp:  dv.Bounds,
		Get: func() float64 {
			if dv.game == nil { return 0 }
			steps := dv.game.playheadAbsSubdiv()
			return float64((steps / div) * 60 / max1(bpm))
		},
		Set: func(v float64) { dv.RequestSeekSteps(timeToSteps(v, bpm, div, maxSteps), seekOriginCounter) },
	})
}
```
Add `formatClock(sec int) string { return fmt.Sprintf("%d:%02d", sec/60, sec%60) }` to `nav_time.go` (+ test a case). `ensurePosEditor()` returns a shared `*ParamValueEditor` on DrumView — if none exists, add one (`dv.posEditor` mirroring the TimelineZone one, driven by DrumView Update/Draw; grep how the overlay editor is driven and mirror). Reuse the TimelineZone `posEditor` if reachable.

- [ ] **Step 5: Add the NAVIGATION section + panel region to the overflow menu (drumview_context_menu.go)**

In `overflowItems()` page-0 slice (`:964`), PREPEND the NAVIGATION header before the FILE header:
```go
items = append(items, overflowItem{label: "NAVIGATION", header: true})
// (the panel itself is drawn in the reserved region, not as items)
items = append(items, overflowItem{label: KeyMenuFile, header: true})
// … existing Upload/Import/Export/Load template …
```
Reserve a panel region at the top of the popup: in `overflowPopupRect()` (`:745`) add `navPanelH := 0; if dv.overflowPage == 0 { navPanelH = dv.navPanel.PanelHeight() }` to the height, and expose it so draw/input can offset. In `drawOverflowMenu` (`:1109`), on page 0 lay out + draw the panel in the reserved top band BEFORE the item rows (offset the item band down by `navPanelH`): `dv.navPanel.Layout(image.Rect(popupRect.Min.X, bandTop, popupRect.Max.X, bandTop+navPanelH)); dv.navPanel.Draw(dst)`. In `handleOverflowMenuInput` (`:592`), on page 0 route a hit inside the panel band to `dv.navPanel.HandleInput(mx, my, left)` before the item hit-test. Register the panel's Buttons for hover if needed (mirror `overflowBtns`).

Keep the FILE rows and paging (page 1 templates) exactly as-is; NAVIGATION + panel only render on page 0.

- [ ] **Step 6: Add `OpenNavigation()` (shared entry point)**

In `drumview_open_dialogs.go`: `func (dv *DrumView) OpenNavigation() { dv.overflowPage = 0; dv.OpenOverflowMenu() }`. Both the Navigate button (Task 5) and the beat counter (Task 6) call this. (Task 5's Step 4 references it; it is defined here.)

- [ ] **Step 7: Run tests + targeted menu tests + compile-check**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestOverflowMenu|TestNavPrevNext|Overflow' ./internal/ui/`
Expected: PASS (existing overflow tests still green — FILE section unchanged).
Run: `-run '^$' ./internal/ui/` — compiles.

- [ ] **Step 8: Commit** (STOP — leave uncommitted)
```
feat(ui): NAVIGATION section + panel in the sectioned overflow menu
```

---

## Task 5: Transport — remove RW/FF/×N, add Navigate button

**Files:** Modify `internal/ui/transport_zone.go`, `internal/ui/drumview_ctor.go`; Test update `internal/ui/transport_jump_test.go`, `internal/ui/transport_seek_buttons_test.go`, `internal/ui/transport_equal_squares_test.go`; new `internal/ui/transport_nav_button_test.go`.

**Interfaces:**
- Produces: `TransportCallbacks.OnNavigate func()`; `navBtn *Button` (IconTarget); removes `rwBtn`/`ffBtn`/`jumpBtn` + `OnRewind`/`OnFastForward`/`OnJumpSizeClick`.
- Consumes: `dv.OpenNavigation()` (Task 4 — add a thin `OpenNavigation()` = ensure page 0 + `OpenOverflowMenu()`).

- [ ] **Step 1: Write the failing test**

```go
//go:build test

package ui

import "testing"

func TestTransportHasNavButtonNotShuttle(t *testing.T) {
	g := newGameWithStart(t)
	g.Layout(1280, 720)
	advanceFrames(g, 3)
	z := g.drum.transportZone
	if z.navBtn == nil || z.navBtn.Rect().Empty() {
		t.Fatal("desktop transport must show the Navigate button")
	}
	// rw/ff/jump fields removed → referencing them must not compile; assert via
	// the callback surface instead: OnNavigate wired, OnRewind/etc gone.
	if z.callbacks.OnNavigate == nil {
		t.Fatal("OnNavigate must be wired")
	}
}

func TestNavButtonOpensOverflowMenu(t *testing.T) {
	g := newGameWithStart(t)
	g.Layout(1280, 720)
	advanceFrames(g, 3)
	g.drum.transportZone.callbacks.OnNavigate()
	if !g.drum.IsOverflowMenuOpen() {
		t.Fatal("Navigate button must open the overflow (Options) menu")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestTransportHasNavButton|TestNavButtonOpens' ./internal/ui/`
Expected: FAIL — `navBtn`/`OnNavigate` undefined.

- [ ] **Step 3: Remove rw/ff/jump; add navBtn**

In `transport_zone.go`, delete every rw/ff/jump site enumerated in File Structure: fields (61-63), callbacks (29-31), construction (211-237), styling (376-386), `SetJumpBeats`/`JumpBtnRect` (685-698), layout `square()` (940-958), mobile empties (879-887), hit-areas (`simpleButtons` 1016, `repeatButtonHitAdapter` 1055-1056), dirty-hash (1369-1382 rw/ff/jump lines), expand (1478-1487), Draw cache (1573-1586) + direct (1639-1652). Update the layout run constants (927-928): removing 3 and adding 1 nav = net −2 → `const nGaps = 10`; `totalUnits := 9*1.0 + transportBPMBoxUnits + transportBPMStepUnits`. Update the running-order doc comment.

Add `navBtn` mirroring `stopBtn` at each site (field, callback `OnNavigate`, construction with `z.navBtn.Icon = string(IconTarget)`, styling `p.StopBtnStyle`/`colTextPrimary`, one `square(z.navBtn)` in the desktop run placed after `subdivBtn` — position it where the crowded controls were, `{z.navBtn,"transport-nav",true}` in `simpleButtons`, dirty-hash mix, expand, Draw both paths). On mobile: LAY IT OUT (do NOT empty its rect) — the Navigate button must be reachable on mobile (that's the whole point). Place it in the mobile transport run next to the kebab.

- [ ] **Step 4: Add `OpenNavigation` + wire OnNavigate**

`drumview_open_dialogs.go`: `func (dv *DrumView) OpenNavigation() { dv.overflowPage = 0; dv.OpenOverflowMenu() }`.
`drumview_ctor.go`: remove the `OnRewind`/`OnFastForward` (675-676) and `OnJumpSizeClick` (787-795) wiring; add `OnNavigate: func() { dv.OpenNavigation() }`.

- [ ] **Step 5: Update the broken tests**

`transport_jump_test.go` and `transport_seek_buttons_test.go` referenced `OnRewind`/`OnFastForward`/`OnJumpSizeClick` and the removed buttons — rewrite them to drive navigation via `dv.navPanel.cb.OnNext/OnPrev` (behavior now lives there) or delete the ones fully superseded, citing the panel tests. Update `transport_equal_squares_test.go`'s `orderedTransportControls` to drop rw/ff/jump and include `nav`, and the count in any button-count assertion — with a one-line rationale.

- [ ] **Step 6: Run tests + guards + compile-check**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'Transport|Nav|Desktop|Mobile|Forbidden' ./internal/ui/`
Expected: PASS (equal-squares/no-overlap updated for the new run).
Run: `-run '^$' ./internal/ui/` — compiles.

- [ ] **Step 7: Commit** (STOP — leave uncommitted)
```
feat(ui): transport declutter — replace RW/FF/×N with a Navigate button
```

---

## Task 6: Ribbon — drag-only, slim progress bar, repoint beat counter

**Files:** Modify `internal/ui/timeline_zone.go`; Test update `internal/ui/timeline_seek_test.go`; new `internal/ui/ribbon_progress_test.go`.

**Interfaces:**
- Consumes: `TimelineCallbacks` (add `OnOpenNavigation func()`), `dv.OpenNavigation()`.
- Produces: ribbon that spans `[0, TimelineBeats()]`, drag-to-scrub only, reduced height; beat counter opens navigation.

- [ ] **Step 1: Write the failing test**

```go
//go:build test

package ui

import "testing"

func TestRibbonSingleClickDoesNotSeek(t *testing.T) {
	g := newGameWithTimeline(t) // harness that lays out the ribbon (see timeline_seek_test.go)
	z := g.drum.timelineZone
	bar := z.timelineBarRect
	adapter := &timelineScrubHitAdapter{zone: z}
	before := g.playheadAbsSubdiv()
	// a pure tap (press+release at same x, no drag) must NOT seek.
	x := bar.Min.X + bar.Dx()/2
	y := bar.Min.Y + bar.Dy()/2
	adapter.OnPress(x, y)
	adapter.OnRelease(x, y)
	g.drainSeekRequest()
	if g.playheadAbsSubdiv() != before {
		t.Fatalf("single click must not seek (before=%d after=%d)", before, g.playheadAbsSubdiv())
	}
}

func TestRibbonDragStillSeeks(t *testing.T) {
	g := newGameWithTimeline(t)
	z := g.drum.timelineZone
	bar := z.timelineBarRect
	adapter := &timelineScrubHitAdapter{zone: z}
	adapter.OnPress(bar.Min.X+5, bar.Min.Y+bar.Dy()/2)
	adapter.OnDrag(bar.Min.X+bar.Dx()*3/5, bar.Min.Y+bar.Dy()/2) // move → different beat
	adapter.OnRelease(bar.Min.X+bar.Dx()*3/5, bar.Min.Y+bar.Dy()/2)
	g.drainSeekRequest()
	if g.playheadAbsSubdiv() == 0 {
		t.Fatal("a drag across the ribbon must still commit a seek")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestRibbonSingleClick|TestRibbonDrag' ./internal/ui/`
Expected: FAIL — single-click currently seeks on press.

- [ ] **Step 3: Remove single-click seek, keep drag**

In `timelineScrubHitAdapter.OnPress` (`:1513`), remove the immediate `OnSeekToStep(..., seekOriginRibbon)` call (`:1524-1526`) — press now only arms scrubbing + records `scrubPressBeat = ribbonStepAt(x)` and sets `scrubGhostBeat`. `OnDrag` unchanged (updates ghost). `OnRelease` (`:1550`): commit `OnSeekToStep(ribbonStepAt(x), seekOriginRibbon)` only if `ribbonStepAt(x) != scrubPressBeat` (a real drag) — a pure tap (release==press beat) commits nothing. Keep `drawScrubGhost` (drag feedback).

- [ ] **Step 4: Slim + reposition the progress scale**

In `drawTimelineBar` (`:365`), replace the sliding `ribbonWindowBeats` window with a whole-song span: cursor `frac = float64(playheadBeats) / float64(max1(totalBeats))` where `totalBeats = z.callbacks.TimelineBeats()`; draw the cursor at `barRect.Min.X + int(frac*barRect.Dx())`. Remove the pattern-window view-rect draw (`:429-446`). Keep ticks optional (they can stay; they now span the whole bar). Reduce the bar height: lower the `TimelineBarH` design token in `DESIGN.md` (density source) and regenerate (`make gen-design-tokens`) OR, if a token change is out of scope, clamp `z.timelineBarH` to a smaller value in `SetTimelineBarHeight`. Prefer the token path; if the generated-token round-trip is heavy, clamp in `SetTimelineBarHeight` with a comment and a follow-up note.

- [ ] **Step 5: Repoint the beat counter to open navigation**

`beatCounterHitAdapter.OnPress` (`:1285`): replace `h.zone.openPositionEditor()` with `if h.zone.callbacks.OnOpenNavigation != nil { h.zone.callbacks.OnOpenNavigation() }`. Add `OnOpenNavigation func()` to `TimelineCallbacks` and wire it in `drumview_ctor.go` to `func() { dv.OpenNavigation() }`. `openPositionEditor`/`posEditor` on TimelineZone may now be unused — remove if no other caller (grep), or keep if the go-to-beat editor reuses it.

- [ ] **Step 6: Run tests + timeline guards + compile-check**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestRibbon|Timeline|Scrub' ./internal/ui/`
Expected: PASS (repoint the timeline_seek_test.go assertions that expected single-click seek → assert drag seek / nav-open).
Run: `-run '^$' ./internal/ui/` — compiles.

- [ ] **Step 7: Commit** (STOP — leave uncommitted)
```
feat(ui): ribbon = slim whole-song progress bar, drag-only, opens nav
```

---

## Task 7: Visual verification (screenshots)

**Files:** Modify `internal/ui/scene_catalog.go` (nav-menu scene if needed); no production code.

- [ ] **Step 1: Add/confirm a scene that opens the Options menu at NAVIGATION**

`scene_catalog.go`: ensure `mobile_overflow_open` and `overflow_menu_open` render the new NAVIGATION section (they call `OpenOverflowMenu` — page 0 now shows it). Optionally add `transport_navigate_open` mirroring them via `g.drum.OpenNavigation()`.

- [ ] **Step 2: Build the desktop screenshot binary**

Run: `cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo_screenshot ./cmd`
Expected: builds.

- [ ] **Step 3: Capture desktop + mobile**

Run (desktop): `xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot -log ERROR -scene overflow_menu_open -screenshot /tmp/nav_desktop.png`
Run (mobile): `xvfb-run -a -s "-screen 0 500x950x24" /tmp/beatmo_screenshot -log ERROR -mobile -win-w 390 -win-h 844 -scene mobile_overflow_open -scene-pass mobile -screenshot /tmp/nav_mobile.png`
Expected: PNGs written.

- [ ] **Step 4: Inspect** — the controller (not a subagent) opens the PNGs and confirms: NAVIGATION section with position readout + Prev/×N/Next/Unit row + Go-to-beat + Go-to-time, FILE section below, the transport shows the Navigate (target) button not RW/FF/×N, and the ribbon is slimmer. Iterate if visually off.

- [ ] **Step 5: Commit** (STOP — leave uncommitted)
```
test(ui): nav-menu screenshot scenes
```

---

## Self-review notes for the implementer

- **`DrawTextAt`/`pt`/role-height accessors**: verify exact names against `textcache.go`/`menu_chrome.go` before Task 3 — the panel draw must match how `drawMenuRow` centers labels.
- **`ParamValueEditor` reuse**: the go-to-beat and go-to-time editors reuse one `*ParamValueEditor`; confirm only one is open at a time (opening one closes the other — `OpenValue` replaces).
- **Portal stacking**: the amount dropdown (`OpenJumpMenu`) and the go-to editors open OVER the open overflow sheet. Verify the portal layer renders them above the sheet and routes input to them first (they are separate portal entries). If they render *under* the sheet, raise their z or open them as the sheet's nested overlay — flag as a risk if the portal system doesn't stack.
- **`seqMu` discipline**: every nav entry point calls `dv.RequestSeek*` (never `SeekSteps` directly); the drain under `seqMu` is unchanged.
- **`OpenNavigation`** is the single entry the Navigate button and beat counter share.
