# INFO-log audit & full user-interaction coverage — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every user action and interaction emit exactly one greppable, lowercase-tagged INFO line through the existing `hooks.Bus → eventlogger` pipeline, with consistent casing, feature-split tags, and full coverage of menus/buttons/scroll/popups/search/text/pan/zoom.

**Architecture:** Two layers over the existing bus. Layer A (domain "what changed") is audited + has gaps filled (language, undo/redo, pan/zoom). Layer B (interaction telemetry "what was touched") adds new event kinds emitted at shared widget chokepoints (`Button.applyPress`, `portal.Open/Close`, scroll release, search, text commit, view switch). The bracket tag becomes a field on the canonical `hooks.ActionRegistry`, so `eventlogger` reads tags from one table and formatters return message-only.

**Tech Stack:** Go 1.23, `internal/hooks` (event bus + registry), `internal/eventlogger` (narrative formatter), `internal/ui` (Ebiten widgets), `internal/log`.

## Global Constraints

- **Bundled Go only:** run tests via `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...` (fast stubbed path). Never the system `go`.
- **DO NOT stage or commit.** The user forbids `git add`/`git commit` — leave all work uncommitted for their review. Every "Checkpoint" step below means *run the verification and stop*, not commit.
- **All bracket tags are lowercase.** No exceptions.
- **One INFO line per discrete action / gesture, never per-frame.** High-rate sites emit only at gesture boundary (release/commit) or are routed through the `eventlogger` coalescer.
- **Every new `hooks.Kind` requires, in the same task:** an `events.go` const, a payload type (if it carries data), a `KindAll` entry, an `ActionRegistry` entry (`Kind`/`Label`/`Tag`/`Scope`), a formatter in `eventlogger/format.go`, a `NumNonVerbose` recompute, and a coalescer entry if high-rate. The completeness tests (`TestEveryKindClassified`, `TestEventLoggerCoversAllNonVerboseKinds`) will fail otherwise.
- **Golden refresh:** after any change to the narrative line shape, regenerate with `UPDATE_GOLDEN=1 ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestEventLoggerGoldenSession ./internal/eventlogger/` and eyeball the diff.

**Running `NumNonVerbose` ledger** (start = 54): Task 1 keeps 54 → Task 6 (+lang) 55 → Task 7 (+undo,+redo) 57 → Task 8 (pan/zoom de-verbose, +2) 59 → Task 9 (+uitap) 60 → Task 10 (+popup_opened,+popup_closed) 62 → Task 11 (+scroll) 63 → Task 12 (+search) 64 → Task 13 (+text) 65 → Task 14 (+view) 66. **Final = 66.** Verbose tail final = 2 (`EventDragProgress`, `EventInstrumentParamChanged`). `KindAll` final = 68.

---

## File map

| File | Responsibility | Tasks |
|---|---|---|
| `internal/hooks/actions.go` | `ActionMeta.Tag` field + tag/label/scope for every Kind | 1, 6, 7, 8–14 |
| `internal/hooks/events.go` | Kind consts, payload types, `KindAll`, `NumNonVerbose`, `IsVerbose` | 6–14 |
| `internal/hooks/actions_test.go` | `TestEveryActionHasLowercaseTag` | 1 |
| `internal/eventlogger/format.go` | formatters return message-only; new formatters | 2, 6–14 |
| `internal/eventlogger/logger.go` | `writeEvent` reads tag from registry | 2 |
| `internal/eventlogger/coalesce.go` | scroll + search coalesce entries | 9, 12 |
| `internal/eventlogger/testdata/golden_session.log` | regenerated golden | 2, 6, 7 |
| `internal/ui/event_helpers.go` | new `emitX` helpers | 6, 7 |
| `internal/ui/interaction_events.go` (new) | Layer-B emit helpers (`emitUITap`, `emitPopup*`, `emitScroll`, `emitSearch`, `emitText`, `emitViewMode`) | 9–14 |
| `internal/ui/uigrid.go` | `Button.applyPress` → `emitUITap` | 9 |
| `internal/ui/portal.go` | `Open`/`Close`/`CloseTop`/`CleanupClosed` → popup events | 10 |
| `internal/ui/scroll_behavior.go` | scroll-end → `emitScroll` | 11 |
| `internal/ui/drumview_overlay_inst_comp.go` | search → `emitSearch` | 12 |
| `internal/ui/textinput.go` | commit → `emitText` | 13 |
| `internal/ui/drumview_layout.go` | `setViewMode` → `emitViewMode` | 14 |
| `internal/ui/game_input_shortcuts.go` | language pick → `emitLanguageChanged` | 6 |
| `internal/ui/undo_seam.go` | undo/redo → `emitUndo`/`emitRedo` | 7 |
| `internal/ui/camera_gesture.go` (new) | pan/zoom gesture-end tracker + emit | 8 |
| `internal/ui/game_update.go` | call the camera gesture tracker around `cam.HandleMouse` | 8 |
| `cmd/*.go`, `internal/bench/hooks_config.go`, `internal/ui/{demo,game_update,game_draw,game_record_bench}.go` | lowercase infra tags | 4 |
| `CLAUDE.md`, `DESIGN.md` | docs | 15 |

---

## Task 1: Add `Tag` to the action registry (single source of truth)

**Files:**
- Modify: `src/go/internal/hooks/actions.go` (struct + every entry)
- Test: `src/go/internal/hooks/actions_test.go`

**Interfaces:**
- Produces: `hooks.ActionMeta.Tag string`; the lowercase tag for every existing Kind. Consumed by `eventlogger.writeEvent` in Task 2.

- [ ] **Step 1: Write the failing test** — append to `actions_test.go`:

```go
// TestEveryActionHasLowercaseTag asserts every registry entry carries a
// non-empty, all-lowercase bracket tag (the single source of truth for the
// INFO tag column).
func TestEveryActionHasLowercaseTag(t *testing.T) {
	for _, a := range ActionRegistry {
		if a.Tag == "" {
			t.Errorf("Kind %q has empty Tag", a.Kind)
			continue
		}
		if a.Tag != strings.ToLower(a.Tag) {
			t.Errorf("Kind %q tag %q is not lowercase", a.Kind, a.Tag)
		}
	}
}
```

Add `import "strings"` and `"testing"` (testing already imported) to the file's import block.

- [ ] **Step 2: Run, expect FAIL** (Tag field doesn't exist → compile error):

`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestEveryActionHasLowercaseTag ./internal/hooks/`
Expected: build error `a.Tag undefined`.

- [ ] **Step 3: Add the `Tag` field** to `ActionMeta` in `actions.go`:

```go
type ActionMeta struct {
	Kind           Kind
	Label          string // human label, e.g. "add node"
	Tag            string // INFO bracket tag (lowercase), e.g. "node"
	Scope          ActionScope
	ExcludedReason string // non-empty ⇒ ScopeDocument but deliberately not undoable yet
}
```

- [ ] **Step 4: Populate `Tag` on every `ActionRegistry` entry.** Set the third positional field. Full mapping (this is the `[audio]` feature-split + `uistate`→`panel` retag):

```go
var ActionRegistry = []ActionMeta{
	{EventPlayStart, "start playback", "play", ScopeTransport, ""},
	{EventPlayStop, "stop playback", "play", ScopeTransport, ""},
	{EventPaused, "pause", "play", ScopeTransport, ""},
	{EventResumed, "resume", "play", ScopeTransport, ""},
	{EventSeek, "seek", "seek", ScopeTransport, ""},

	{EventRecordStart, "start recording", "record", ScopeRecording, ""},
	{EventRecordStop, "stop recording", "record", ScopeRecording, ""},
	{EventRecordDropped, "recording dropped", "record", ScopeRecording, ""},

	{EventBPMChange, "change BPM", "bpm", ScopeDocument, ""},
	{EventSubdivChange, "change subdivision", "transport", ScopeDocument, ""},
	{EventLengthChange, "change length", "transport", ScopeSession, ""},

	{EventImport, "import project", "import", ScopeProjectIO, ""},
	{EventExport, "export project", "export", ScopeProjectIO, ""},

	{EventNodeAdded, "add node", "node", ScopeDocument, ""},
	{EventNodeDeleted, "delete node", "node", ScopeDocument, ""},
	{EventNodeMoved, "move node", "node", ScopeDocument, ""},
	{EventNodeTypeChanged, "change node type", "node", ScopeDocument, ""},
	{EventNodeParamsChanged, "edit node", "node", ScopeDocument, ""},
	{EventStartNodeChanged, "set start node", "node", ScopeDocument, ""},
	{EventEdgeAdded, "add edge", "edge", ScopeDocument, ""},
	{EventEdgeDeleted, "delete edge", "edge", ScopeDocument, ""},

	{EventRowAdded, "add row", "row", ScopeDocument, ""},
	{EventRowDeleted, "delete row", "row", ScopeDocument, ""},
	{EventRowInstrumentChange, "change instrument", "row", ScopeDocument, ""},
	{EventRowMute, "toggle mute", "row", ScopeSession, ""},
	{EventRowSolo, "toggle solo", "row", ScopeSession, ""},

	{EventMasterVolumeChange, "set master volume", "master", ScopeDocument, ""},
	{EventEQBandChange, "adjust EQ", "eq", ScopeDocument, ""},
	{EventInsertEffectAdded, "add effect", "fx", ScopeDocument, ""},
	{EventInsertEffectRemoved, "remove effect", "fx", ScopeDocument, ""},
	{EventInsertEffectParam, "adjust effect", "fx", ScopeDocument, ""},
	{EventRowVolume, "set row volume", "row", ScopeDocument, ""},
	{EventRowPan, "set row pan", "row", ScopeDocument, "import-only; no UI commit site yet"},
	{EventInsertEffectMoved, "reorder effect", "fx", ScopeDocument, ""},
	{EventInsertEffectToggled, "toggle effect", "fx", ScopeDocument, ""},
	{EventSendChanged, "adjust send", "send", ScopeDocument, "import-only; no UI commit site yet"},
	{EventInstrumentParamsCommitted, "edit synth", "synth", ScopeDocument, ""},
	{EventInstrumentParamsReset, "reset synth", "synth", ScopeDocument, ""},
	{EventAudioPanelStateChanged, "change audio-panel view", "panel", ScopeSession, ""},

	{EventRowColorChanged, "recolor row", "row", ScopeDocument, ""},
	{EventEQFilterToggled, "toggle EQ filter", "eq", ScopeDocument, ""},
	{EventCustomWAVLoaded, "load WAV", "sampler", ScopeSession, ""},
	{EventInstrumentRenamed, "rename instrument", "synth", ScopeDocument, ""},
	{EventSceneApplied, "apply scene", "scene", ScopeProjectIO, ""},
	{EventUIStateApplied, "apply UI state", "uistate", ScopeSession, ""},
	{EventFavoriteToggled, "toggle favorite", "favorite", ScopeSession, ""},

	{EventRecipeSaved, "save recipe", "recipe", ScopeSession, ""},
	{EventRecipeCreated, "create recipe", "recipe", ScopeSession, ""},
	{EventRecipeDeleted, "delete recipe", "recipe", ScopeSession, ""},
	{EventKitApplied, "apply kit", "kit", ScopeSession, ""},

	{EventSampleSaved, "save sample", "sampler", ScopeSession, ""},
	{EventSampleCreated, "create sample", "sampler", ScopeSession, ""},
	{EventSampleReset, "reset sample", "sampler", ScopeSession, ""},
	{EventSampleEditChanged, "edit sample", "sampler", ScopeDocument, ""},

	{EventCameraPan, "camera pan", "camera", ScopeVerbose, ""},
	{EventCameraZoom, "camera zoom", "camera", ScopeVerbose, ""},
	{EventDragProgress, "drag", "drag", ScopeVerbose, ""},
	{EventInstrumentParamChanged, "edit synth (live)", "synth", ScopeVerbose, ""},
}
```

- [ ] **Step 5: Run, expect PASS** (all three of `TestEveryActionHasLowercaseTag`, `TestEveryKindClassified`, `TestRegistryNoDuplicateKinds`):

`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/hooks/`
Expected: PASS.

- [ ] **Step 6: Checkpoint** — do not commit. Confirm `./internal/hooks/` is green.

---

## Task 2: `eventlogger` reads the tag from the registry; formatters return message-only

**Files:**
- Modify: `src/go/internal/eventlogger/format.go` (type `formatter`, all formatter bodies, `lookupFormatter`)
- Modify: `src/go/internal/eventlogger/logger.go:192-211` (`writeEvent`)
- Modify: `src/go/internal/eventlogger/testdata/golden_session.log` (regenerated)

**Interfaces:**
- Consumes: `hooks.MetaFor(kind).Tag` (Task 1).
- Produces: `type formatter func(payload any) string`; `lookupFormatter(k) (formatter, bool)` unchanged signature shape but value type changes.

- [ ] **Step 1: Change `writeEvent`** in `logger.go` to source the tag from the registry. Replace the body of `writeEvent` (lines ~192-211):

```go
func (l *Logger) writeEvent(e hooks.Event) {
	f, ok := lookupFormatter(e.Kind)
	if !ok {
		return
	}
	msg := f(e.Payload)
	if msg == "" {
		return
	}
	l.written.Add(1)
	srcSuffix := ""
	if s := e.Source.String(); s != "" {
		srcSuffix = "  src=" + s
	}
	tag := ""
	if meta, ok := hooks.MetaFor(e.Kind); ok {
		tag = meta.Tag
	}
	if tag == "" {
		l.log.Infof("%s%s", msg, srcSuffix)
	} else {
		l.log.Infof("[%s] %s%s", tag, msg, srcSuffix)
	}
}
```

- [ ] **Step 2: Change the `formatter` type and the `formatted` struct** in `format.go`. Delete the `formatted` struct; change:

```go
// formatter converts a hook payload into a one-line human message. Returning
// "" suppresses the line. The bracket tag is NOT chosen here — it comes from
// the canonical hooks.ActionRegistry (ActionMeta.Tag), read in writeEvent.
type formatter func(payload any) string
```

- [ ] **Step 3: Rewrite every formatter body** to return the message string only (drop the `tag:` field). Mechanical: `return formatted{tag: "x", msg: "y"}` → `return "y"`. Representative examples (apply the same transform to all ~58):

```go
hooks.EventPlayStart: func(_ any) string { return "play started" },
hooks.EventSeek: func(p any) string {
	s, _ := p.(hooks.SeekPayload)
	return fmt.Sprintf("seek beats=%d", s.Beats)
},
hooks.EventEQBandChange: func(p any) string {
	b, _ := p.(hooks.EQBandPayload)
	return fmt.Sprintf("EQ %s band %d = %+.1f dB", b.Channel, b.Band, b.GainDB)
},
hooks.EventAudioPanelStateChanged: func(p any) string {
	s, _ := p.(hooks.AudioPanelStatePayload)
	return fmt.Sprintf("audio panel %s changed", s.Field)
},
// …apply to all remaining entries identically (drop tag, keep msg).
```

Also update `lookupFormatter`'s return type usage (signature stays `(formatter, bool)`).

- [ ] **Step 4: Build the package, expect compile success and coverage tests pass:**

`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestEventLoggerCovers|TestEventLoggerNoUnknownKinds' ./internal/eventlogger/`
Expected: PASS.

- [ ] **Step 5: Regenerate the golden and inspect the diff:**

`cd src/go && UPDATE_GOLDEN=1 ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestEventLoggerGoldenSession ./internal/eventlogger/`
Then `git diff --no-index /dev/null /dev/null` is N/A — instead inspect: the only changes must be the `panel`/feature-split tag renames already covered by existing fixtures (the golden fixture uses `EventInstrumentRenamed` → now `[synth]`, `EventCustomWAVLoaded` → now `[sampler]`). Verify with `../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestEventLoggerGoldenSession ./internal/eventlogger/` → PASS.

- [ ] **Step 6: Run the whole eventlogger + hooks packages:**

`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/eventlogger/ ./internal/hooks/`
Expected: PASS.

- [ ] **Step 7: Checkpoint** — do not commit.

---

## Task 3: Verify the whole UI build still compiles after the formatter change

**Files:** none new — this is a guard task because `internal/ui` and `internal/eventstream` may reference `eventlogger` internals or the `formatted` type indirectly.

- [ ] **Step 1: Grep for stale `formatted{` references:**

`cd src/go && grep -rn "formatted{" internal/ | grep -v "_test.go"`
Expected: no matches (all removed in Task 2). Fix any stragglers.

- [ ] **Step 2: Full fast suite for the touched packages + their importers:**

`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/eventlogger/ ./internal/hooks/ ./internal/eventstream/ ./internal/ui/`
Expected: PASS (or pre-existing unrelated failures only — note them).

- [ ] **Step 3: Checkpoint** — do not commit.

---

## Task 4: Lowercase the infrastructure direct-call tags

**Files:**
- Modify: `src/go/internal/bench/hooks_config.go:129,137`
- Modify: `src/go/cmd/beatmo.go:32,93,167`, `src/go/cmd/pyroscope.go:34`, `src/go/cmd/beatmo_record_bench.go` (all `[RECORD-BENCH]`, `[HOOKS]`, `[RECORD-BENCH/hook]`)
- Modify: `src/go/internal/ui/demo.go` (all `[DEMO]`), `game_update.go:98,106,116` (`[BENCH]`), `game_draw.go:56,58` (`[SCREENSHOT]`), `game_record_bench.go:42,64,72` (`[BENCH]`)

**Interfaces:** none (string-literal changes only).

- [ ] **Step 1: Write a guard test** — create `src/go/internal/log/tag_case_test.go`:

```go
package log

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNoUppercaseInfoTags scans production Go sources for Infof/Warnf/Errorf
// calls whose leading "[tag]" contains an uppercase letter. All bracket tags
// must be lowercase (the project-wide convention). Test files are skipped.
func TestNoUppercaseInfoTags(t *testing.T) {
	root := filepath.Join("..", "..") // src/go
	tagRE := regexp.MustCompile(`\.(?:Infof|Warnf|Errorf|Tracef|Debugf)\("\[([^\]]+)\]`)
	upper := regexp.MustCompile(`[A-Z]`)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range tagRE.FindAllSubmatch(b, -1) {
			tag := string(m[1])
			if upper.MatchString(tag) {
				t.Errorf("%s: uppercase tag [%s] — tags must be lowercase", path, tag)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run, expect FAIL** listing every uppercase tag:

`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestNoUppercaseInfoTags ./internal/log/`
Expected: FAIL with the `[HOOKS]`, `[BENCH]`, `[DEMO]`, `[RUNTIME]`, `[SCREENSHOT]`, `[PYROSCOPE]`, `[RECORD-BENCH]` sites.

- [ ] **Step 3: Lowercase each tag** at the reported sites. Examples:
  - `hooks_config.go`: `[HOOKS] %s → %s` → `[hooks] %s → %s`; `[HOOKS] subscribed:` → `[hooks] subscribed:`.
  - `beatmo.go`: `[RUNTIME] final snapshot` → `[runtime] final snapshot`, `[RUNTIME] startup snapshot` → `[runtime] startup snapshot` (`[eventlog]` already lowercase).
  - `pyroscope.go`: `[PYROSCOPE] started` → `[pyroscope] started`.
  - `beatmo_record_bench.go`: every `[RECORD-BENCH]` → `[record-bench]`, `[RECORD-BENCH/hook]` → `[record-bench/hook]`, `[HOOKS] dump_pprof` → `[hooks] dump_pprof`.
  - `demo.go`: every `[DEMO]` → `[demo]`.
  - `game_update.go`: `[BENCH] Started/Complete/Parity` → `[bench] …`.
  - `game_draw.go`: `[SCREENSHOT] Error/Saved` → `[screenshot] …`.
  - `game_record_bench.go`: `[BENCH] Recording started/stopped/perf snapshot` → `[bench] …`.

- [ ] **Step 4: Run, expect PASS:**

`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestNoUppercaseInfoTags ./internal/log/`
Expected: PASS.

- [ ] **Step 5: Checkpoint** — do not commit.

---

## Task 5: Scaffolding — `ScopeInteraction` and the Layer-B emit-helper file

**Files:**
- Modify: `src/go/internal/hooks/actions.go` (add scope constant)
- Create: `src/go/internal/ui/interaction_events.go`

**Interfaces:**
- Produces: `hooks.ScopeInteraction ActionScope`; `internal/ui` file holding Layer-B emit helpers (populated in Tasks 9–14).

- [ ] **Step 1: Add the scope constant** to the `ActionScope` block in `actions.go`, after `ScopeVerbose`:

```go
	// ScopeInteraction is pure interaction telemetry (button tap, popup
	// open/close, scroll, search, text commit, view switch). Never undoable;
	// not document or session state — it records what the user touched, not a
	// state change.
	ScopeInteraction
```

- [ ] **Step 2: Create `interaction_events.go`** with the file header and imports (helpers added in later tasks):

```go
package ui

// Layer-B interaction telemetry emit helpers. Each publishes through
// hooks.PublishWithSource with hooks.CaptureSource(1) so the src= column points
// at the widget chokepoint's caller. See
// docs/superpowers/specs/2026-06-27-info-log-audit-and-interaction-coverage-design.md.

import (
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

var _ = hooks.EventPlayStart // placeholder import anchor; removed once helpers land
```

- [ ] **Step 3: Build:**

`cd src/go && ../../.tools/go/bin/go build -tags test ./internal/hooks/ ./internal/ui/`
Expected: success.

- [ ] **Step 4: Confirm `Undoable` is still false for the new scope** (it checks `== ScopeDocument`, so a unit assertion documents intent). Append to `actions_test.go`:

```go
func TestInteractionScopeNotUndoable(t *testing.T) {
	m := ActionMeta{Kind: "x.test", Scope: ScopeInteraction}
	if m.Scope == ScopeDocument {
		t.Fatal("ScopeInteraction must not equal ScopeDocument")
	}
}
```

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestInteractionScopeNotUndoable ./internal/hooks/` → PASS.

- [ ] **Step 5: Checkpoint** — do not commit.

---

## Task 6: Layer-A gap — language change

**Files:**
- Modify: `src/go/internal/hooks/events.go` (const, payload, `KindAll`, `NumNonVerbose`→55)
- Modify: `src/go/internal/hooks/actions.go` (registry entry)
- Modify: `src/go/internal/eventlogger/format.go` (formatter)
- Modify: `src/go/internal/ui/event_helpers.go` (`emitLanguageChanged`)
- Modify: `src/go/internal/ui/game_input_shortcuts.go:299-303` (emit at the settings `onPick`)
- Test: `src/go/internal/ui/language_log_test.go` (new)

**Interfaces:**
- Produces: `hooks.EventLanguageChanged Kind`; `hooks.LanguagePayload{Old, New string}`; `ui.emitLanguageChanged(oldLoc, newLoc string)`.

- [ ] **Step 1: Write the failing test** — `src/go/internal/ui/language_log_test.go`:

```go
package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

func TestLanguageChangeEmitsEvent(t *testing.T) {
	got := make(chan hooks.LanguagePayload, 1)
	unsub := hooks.GlobalBus().Subscribe(hooks.EventLanguageChanged, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.LanguagePayload); ok {
			got <- p
		}
	})
	defer unsub()

	emitLanguageChanged("en", "es")

	select {
	case p := <-got:
		if p.Old != "en" || p.New != "es" {
			t.Fatalf("got %+v", p)
		}
	default:
		// hooks.Bus is async; poll briefly.
		t.Fatal("no EventLanguageChanged delivered")
	}
}
```

(`hooks.GlobalBus()` is confirmed in `internal/hooks/bus.go:220`; package-level `hooks.PublishWithSource` — used by the `emit*` helpers — publishes to that same global bus, so subscribing to `GlobalBus()` reliably receives them. Same pattern in every later interaction test.)

- [ ] **Step 2: Run, expect FAIL** (undefined `EventLanguageChanged`/`LanguagePayload`/`emitLanguageChanged`).

`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestLanguageChangeEmitsEvent ./internal/ui/`

- [ ] **Step 3: Add the Kind + payload** in `events.go`. After the favorite block:

```go
	// EventLanguageChanged fires when the user switches UI language in the
	// settings overlay. ScopeSession (locale persists to userprefs, not the
	// project document).
	EventLanguageChanged Kind = "lang.changed"
```

Add to `KindAll` (in the non-verbose region, e.g. after `EventFavoriteToggled`):

```go
	EventLanguageChanged,
```

Add the payload type near the other payloads:

```go
// LanguagePayload describes a UI-language switch.
type LanguagePayload struct {
	Old string `json:"old"`
	New string `json:"new"`
}
```

Bump `NumNonVerbose` to `55` and add a one-line note to its comment: `// + lang.changed = 55.`

- [ ] **Step 4: Add the registry entry** in `actions.go` (in the round-2/new section):

```go
	{EventLanguageChanged, "change language", "lang", ScopeSession, ""},
```

- [ ] **Step 5: Add the formatter** in `format.go`:

```go
hooks.EventLanguageChanged: func(p any) string {
	l, _ := p.(hooks.LanguagePayload)
	return fmt.Sprintf("language %s → %s", l.Old, l.New)
},
```

- [ ] **Step 6: Add the emit helper** in `event_helpers.go`:

```go
// emitLanguageChanged publishes EventLanguageChanged from the settings overlay
// language pick. old/new are locale strings ("en", "es").
func emitLanguageChanged(oldLoc, newLoc string) {
	hooks.PublishWithSource(hooks.EventLanguageChanged, hooks.LanguagePayload{
		Old: oldLoc, New: newLoc,
	}, hooks.CaptureSource(1))
}
```

- [ ] **Step 7: Wire the emit site** in `game_input_shortcuts.go`. In the settings `onPick` closure (currently `i18n.SetLocale(l); persistLanguage(l)`), capture the previous locale and emit:

```go
Overlay: NewSettingsOverlay(func(l i18n.Locale) {
	old := i18n.CurrentLocale() // capture before switching; use the project's current-locale accessor
	i18n.SetLocale(l)
	persistLanguage(l)
	emitLanguageChanged(old.String(), l.String())
}),
```

(Use the actual current-locale accessor from `internal/i18n`; if none exists, read it via `i18n.Locale` of the active locale before `SetLocale`.)

- [ ] **Step 8: Run the new test + package, expect PASS:**

`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestLanguageChangeEmitsEvent ./internal/ui/ && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/hooks/ ./internal/eventlogger/`
Expected: PASS (registry + coverage completeness now satisfied; `NumNonVerbose` test green).

- [ ] **Step 9: Checkpoint** — do not commit.

---

## Task 7: Layer-A gap — undo / redo

**Files:**
- Modify: `events.go` (2 consts, payload, `KindAll`, `NumNonVerbose`→57)
- Modify: `actions.go` (2 entries)
- Modify: `format.go` (2 formatters)
- Modify: `event_helpers.go` (`emitUndo`, `emitRedo`)
- Modify: `undo_seam.go performUndo` (line ~34) and `performRedo` (line ~56)
- Test: `src/go/internal/ui/undo_log_test.go` (new)

**Interfaces:**
- Produces: `hooks.EventUndo`, `hooks.EventRedo Kind`; `hooks.UndoPayload{Label string}`; `ui.emitUndo(label string)`, `ui.emitRedo(label string)`.

- [ ] **Step 1: Write the failing test** — `undo_log_test.go`:

```go
package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

func TestUndoEmitsEvent(t *testing.T) {
	ch := make(chan string, 2)
	u := hooks.GlobalBus().Subscribe(hooks.EventUndo, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.UndoPayload); ok {
			ch <- "undo:" + p.Label
		}
	})
	defer u()
	r := hooks.GlobalBus().Subscribe(hooks.EventRedo, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.UndoPayload); ok {
			ch <- "redo:" + p.Label
		}
	})
	defer r()

	emitUndo("add node")
	emitRedo("add node")

	deadline := time.After(time.Second)
	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case s := <-ch:
			seen[s] = true
		case <-deadline:
			t.Fatalf("missing events, saw %v", seen)
		}
	}
}
```

- [ ] **Step 2: Run, expect FAIL** (undefined symbols).

- [ ] **Step 3: Add Kinds + payload** in `events.go`:

```go
	// EventUndo / EventRedo fire when the user reverts / re-applies one edit.
	// ScopeSession (the meta-action itself is never journaled).
	EventUndo Kind = "undo"
	EventRedo Kind = "redo"
```

Add both to `KindAll` (non-verbose region). Payload:

```go
// UndoPayload names the action being undone/redone (the registry label of the
// reverted edit, e.g. "adjust EQ").
type UndoPayload struct {
	Label string `json:"label"`
}
```

Bump `NumNonVerbose` → `57`, extend the comment note.

- [ ] **Step 4: Registry entries** in `actions.go`:

```go
	{EventUndo, "undo", "undo", ScopeSession, ""},
	{EventRedo, "redo", "redo", ScopeSession, ""},
```

- [ ] **Step 5: Formatters** in `format.go`:

```go
hooks.EventUndo: func(p any) string {
	u, _ := p.(hooks.UndoPayload)
	return fmt.Sprintf("undo: %s", u.Label)
},
hooks.EventRedo: func(p any) string {
	u, _ := p.(hooks.UndoPayload)
	return fmt.Sprintf("redo: %s", u.Label)
},
```

- [ ] **Step 6: Emit helpers** in `event_helpers.go`:

```go
// emitUndo / emitRedo publish the undo/redo meta-action with the reverted
// edit's label (captured pre-Undo in undo_seam.go).
func emitUndo(label string) {
	hooks.PublishWithSource(hooks.EventUndo, hooks.UndoPayload{Label: label}, hooks.CaptureSource(1))
}
func emitRedo(label string) {
	hooks.PublishWithSource(hooks.EventRedo, hooks.UndoPayload{Label: label}, hooks.CaptureSource(1))
}
```

- [ ] **Step 7: Wire emit sites** in `undo_seam.go`. In `performUndo`, after `label := g.undoManager.UndoLabel()` and once the undo actually proceeds (i.e. after the early-return guard), add `emitUndo(label)`. Place it right after `g.undoManager.Undo()`:

```go
	g.undoManager.Undo()
	emitUndo(label)
	if g.drum == nil {
		return
	}
```

Mirror in `performRedo`:

```go
	g.undoManager.Redo()
	emitRedo(label)
	if g.drum == nil {
		return
	}
```

- [ ] **Step 8: Run, expect PASS:**

`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestUndoEmitsEvent ./internal/ui/ && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/hooks/ ./internal/eventlogger/`

- [ ] **Step 9: Checkpoint** — do not commit.

---

## Task 8: Layer-A gap — pan / zoom one line per gesture + de-verbose

**Files:**
- Modify: `events.go` `IsVerbose` (remove pan+zoom; `NumNonVerbose`→59)
- Modify: `event_helpers.go` (`emitCameraPan`, `emitCameraZoom`)
- Create: `src/go/internal/ui/camera_gesture.go` (gesture tracker)
- Modify: `src/go/internal/ui/game_update.go` (~605-625) to drive the tracker
- Modify: `src/go/internal/eventlogger/golden_session_test.go` + `coalesce.go` are unaffected (pan/zoom stay coalesced; zoom remains in `coalesceKinds`, pan removed so a pan gesture emits exactly once)
- Test: `src/go/internal/ui/camera_gesture_test.go` (new)

**Interfaces:**
- Consumes: `g.cam.HandleMouse(...) bool` (drag flag), `g.cam.Scale`, `g.cam.OffsetX/Y`.
- Produces: `type cameraGesture struct{…}`; `(*cameraGesture).observe(panning bool, offX, offY, scale float64)`; `ui.emitCameraPan(dx, dy float64)`, `ui.emitCameraZoom(factor float64)`.

- [ ] **Step 1: Write the failing test** — `camera_gesture_test.go`:

```go
package ui

import "testing"

// A pan gesture (press-drag-release) emits exactly one pan event with the
// cumulative delta; a release with no movement emits nothing.
func TestCameraGestureEmitsOncePerPan(t *testing.T) {
	var pans []struct{ dx, dy float64 }
	g := &cameraGesture{emitPan: func(dx, dy float64) {
		pans = append(pans, struct{ dx, dy float64 }{dx, dy})
	}, emitZoom: func(float64) {}}

	g.observe(false, 0, 0, 1) // idle
	g.observe(true, 5, 0, 1)  // pan starts (offset jumps)
	g.observe(true, 12, 3, 1) // pan continues
	g.observe(false, 12, 3, 1) // release → one emit, dx=12 dy=3

	if len(pans) != 1 {
		t.Fatalf("want 1 pan emit, got %d: %+v", len(pans), pans)
	}
	if pans[0].dx != 12 || pans[0].dy != 3 {
		t.Fatalf("want dx=12 dy=3, got %+v", pans[0])
	}
}

func TestCameraGestureEmitsZoomOnScaleChange(t *testing.T) {
	var zooms []float64
	g := &cameraGesture{emitPan: func(float64, float64) {}, emitZoom: func(f float64) {
		zooms = append(zooms, f)
	}}
	g.observe(false, 0, 0, 1.0)
	g.observe(false, 0, 0, 1.1) // wheel zoom tick
	if len(zooms) != 1 {
		t.Fatalf("want 1 zoom emit, got %d", len(zooms))
	}
}
```

- [ ] **Step 2: Run, expect FAIL** (undefined `cameraGesture`).

- [ ] **Step 3: Create `camera_gesture.go`:**

```go
package ui

// cameraGesture collapses the per-frame camera mutations from
// Camera.HandleMouse into one INFO line per finished gesture:
//   • pan  → emitted once on press→release, carrying the cumulative offset delta
//   • zoom → emitted each frame Scale changes (the eventlogger coalescer
//            debounce-trails a continuous wheel-zoom burst into one line)
// It holds no Ebiten state; game_update.go feeds it the post-HandleMouse
// snapshot each frame.
type cameraGesture struct {
	panning      bool
	startOffX    float64
	startOffY    float64
	lastScale    float64
	scaleInit    bool
	emitPan      func(dx, dy float64)
	emitZoom     func(factor float64)
}

// observe is called once per frame with the camera's drag flag and current
// offset/scale (already mutated by HandleMouse this frame).
func (g *cameraGesture) observe(panning bool, offX, offY, scale float64) {
	// Zoom: any scale change this frame is a zoom tick.
	if g.scaleInit && scale != g.lastScale && g.emitZoom != nil {
		g.emitZoom(scale / g.lastScale)
	}
	g.lastScale = scale
	g.scaleInit = true

	// Pan: track press→release, emit cumulative delta on release.
	switch {
	case panning && !g.panning: // gesture start
		g.startOffX, g.startOffY = offX, offY
	case !panning && g.panning: // gesture end
		dx, dy := offX-g.startOffX, offY-g.startOffY
		if (dx != 0 || dy != 0) && g.emitPan != nil {
			g.emitPan(dx, dy)
		}
	}
	g.panning = panning
}
```

- [ ] **Step 4: Add emit helpers** in `event_helpers.go`:

```go
// emitCameraPan publishes EventCameraPan once at pan-gesture release with the
// cumulative screen-space delta. No longer verbose — shows in default narrative.
func emitCameraPan(dx, dy float64) {
	hooks.PublishWithSource(hooks.EventCameraPan, hooks.CameraPanPayload{DX: dx, DY: dy}, hooks.CaptureSource(1))
}

// emitCameraZoom publishes EventCameraZoom per zoom tick; the eventlogger
// coalescer collapses a continuous wheel/pinch burst into one trailing line.
func emitCameraZoom(factor float64) {
	hooks.PublishWithSource(hooks.EventCameraZoom, hooks.CameraZoomPayload{Factor: factor}, hooks.CaptureSource(1))
}
```

- [ ] **Step 5: De-verbose pan/zoom** in `events.go` `IsVerbose`:

```go
func IsVerbose(k Kind) bool {
	switch k {
	case EventDragProgress, EventInstrumentParamChanged:
		return true
	}
	return false
}
```

Bump `NumNonVerbose` → `59` and update its comment: `// pan+zoom promoted out of verbose (+2) = 59.` Also update the `events.go` doc-comment on `EventCameraPan`/`EventCameraZoom` to drop "verbose / opt-in" wording (they now show by default).

- [ ] **Step 6: Drive the tracker** in `game_update.go`. Add a `camGesture *cameraGesture` field to `Game` (in `game_struct.go`), lazily initialized, then after the `cam.HandleMouse` calls (the block around lines 605-625), feed it once per frame. Find where `drag` is computed and add, after the final assignment to `drag`:

```go
	if g.camGesture == nil {
		g.camGesture = &cameraGesture{emitPan: func(dx, dy float64) { emitCameraPan(dx, dy) }, emitZoom: func(f float64) { emitCameraZoom(f) }}
	}
	g.camGesture.observe(drag, g.cam.OffsetX, g.cam.OffsetY, g.cam.Scale)
```

(Confirm `drag` is the camera-pan flag in scope at that point; the four `HandleMouse` calls all assign to `drag`.)

- [ ] **Step 7: Run the gesture unit tests + packages, expect PASS:**

`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestCameraGesture ./internal/ui/ && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/hooks/ ./internal/eventlogger/`

- [ ] **Step 8: Update the verbose-filter golden test.** `TestEventLoggerVerboseFiltersByDefault` publishes pan/zoom and asserts they do NOT appear at INFO with `Verbose:false`. That contract is now inverted for pan/zoom. Edit the test: keep `EventDragProgress` as the still-verbose sentinel that must be absent, and add an assertion that `[camera]` IS now present. Replace its publish/assert block:

```go
	pubFixed(bus, hooks.EventCameraPan, hooks.CameraPanPayload{DX: 1, DY: 2})
	pubFixed(bus, hooks.EventDragProgress, hooks.DragProgressPayload{NodeID: 1, I: 0, J: 0})
	pubFixed(bus, hooks.EventPlayStart, nil)

	waitFor(t, func() bool { return logger.Stats().Written >= 1 }, 2*time.Second)
	logger.FlushNow()
	time.Sleep(80 * time.Millisecond)

	out := buf.String()
	if strings.Contains(out, "[drag]") {
		t.Fatalf("drag (still verbose) leaked into INFO:\n%s", out)
	}
	if !strings.Contains(out, "[camera] pan") {
		t.Fatalf("camera pan should now appear at INFO by default:\n%s", out)
	}
```

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestEventLoggerVerbose ./internal/eventlogger/` → PASS.

- [ ] **Step 9: Checkpoint** — do not commit.

---

## Task 9: Layer-B — generic button tap (`EventUITap`)

**Files:**
- Modify: `events.go` (const, payload, `KindAll`, `NumNonVerbose`→60)
- Modify: `actions.go` (entry)
- Modify: `format.go` (formatter)
- Modify: `interaction_events.go` (`emitUITap`; remove the placeholder anchor line)
- Modify: `src/go/internal/ui/uigrid.go:409` (`applyPress`, inside `b.held == 1`)
- Test: `src/go/internal/ui/uitap_log_test.go` (new)

**Interfaces:**
- Produces: `hooks.EventUITap Kind`; `hooks.UITapPayload{Label, Icon string}`; `ui.emitUITap(label, icon string)`.

- [ ] **Step 1: Write the failing test** — `uitap_log_test.go`:

```go
package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// Pressing a Button (press edge) emits exactly one EventUITap with its label.
func TestButtonPressEmitsUITap(t *testing.T) {
	ch := make(chan hooks.UITapPayload, 4)
	unsub := hooks.GlobalBus().Subscribe(hooks.EventUITap, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.UITapPayload); ok {
			ch <- p
		}
	})
	defer unsub()

	b := NewButton("Play", ButtonStyle{}, func() {})
	b.ResetPress()
	b.PressFromTree(true)  // press edge → tap
	b.PressFromTree(false) // release → no second tap

	select {
	case p := <-ch:
		if p.Label != "Play" {
			t.Fatalf("got label %q", p.Label)
		}
	case <-time.After(time.Second):
		t.Fatal("no EventUITap")
	}
	// Ensure release did not emit a second tap.
	select {
	case p := <-ch:
		t.Fatalf("unexpected second tap: %+v", p)
	case <-time.After(150 * time.Millisecond):
	}
}
```

(`NewButton(text string, style ButtonVisual, onClick func()) *Button`; `ButtonStyle{}` is the concrete style used throughout existing tests — see `button_handler_test.go:18`.)

- [ ] **Step 2: Run, expect FAIL** (undefined `EventUITap`).

- [ ] **Step 3: Kind + payload** in `events.go`:

```go
	// EventUITap fires once on a button/pill/toggle/keycap press edge (Layer-B
	// interaction telemetry). The domain effect, if any, is a separate event.
	EventUITap Kind = "ui.tap"
```

Add to `KindAll`. Payload:

```go
// UITapPayload identifies a tapped button by its resolved label (and icon id
// when the button is icon-only).
type UITapPayload struct {
	Label string `json:"label"`
	Icon  string `json:"icon,omitempty"`
}
```

Bump `NumNonVerbose` → `60`.

- [ ] **Step 4: Registry entry** in `actions.go`:

```go
	{EventUITap, "tap", "ui", ScopeInteraction, ""},
```

- [ ] **Step 5: Formatter** in `format.go`:

```go
hooks.EventUITap: func(p any) string {
	u, _ := p.(hooks.UITapPayload)
	label := u.Label
	if label == "" {
		label = u.Icon
	}
	if label == "" {
		return "" // unlabeled control: suppress
	}
	return fmt.Sprintf("tap %q", label)
},
```

- [ ] **Step 6: Emit helper** in `interaction_events.go` (delete the `var _ = …` anchor):

```go
// emitUITap publishes EventUITap from the shared Button press core. Suppressed
// (no publish) when both label and icon are empty.
func emitUITap(label, icon string) {
	if label == "" && icon == "" {
		return
	}
	hooks.PublishWithSource(hooks.EventUITap, hooks.UITapPayload{Label: label, Icon: icon}, hooks.CaptureSource(1))
}
```

- [ ] **Step 7: Wire the chokepoint** in `uigrid.go` `applyPress`, inside `if b.held == 1 {` BEFORE `b.OnClick()` (so the tap is logged even if OnClick panics/reorders), and ONLY on the press edge (not the repeat branch):

```go
		if b.held == 1 {
			emitUITap(b.displayText(), b.Icon)
			b.pressTarget = 1
			if b.OnClick != nil {
				b.OnClick()
			}
			...
		}
```

- [ ] **Step 8: Run, expect PASS:**

`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestButtonPressEmitsUITap ./internal/ui/ && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/hooks/ ./internal/eventlogger/`

- [ ] **Step 9: Guard against per-frame regression — run the broader UI suite** to ensure the new emit doesn't trip goleak/parity or flood any test that drives many button presses:

`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS (note any pre-existing unrelated failures).

- [ ] **Step 10: Checkpoint** — do not commit.

---

## Task 10: Layer-B — popup open / close (portal chokepoint)

**Files:**
- Modify: `events.go` (2 consts, payload, `KindAll`, `NumNonVerbose`→62)
- Modify: `actions.go` (2 entries)
- Modify: `format.go` (2 formatters)
- Modify: `interaction_events.go` (`emitPopupOpened`, `emitPopupClosed`)
- Modify: `src/go/internal/ui/portal.go` (`Open`:53, `Close`:76, `CloseTop`:83, `CleanupClosed`:212)
- Test: `src/go/internal/ui/popup_log_test.go` (new)

**Interfaces:**
- Produces: `hooks.EventPopupOpened`, `hooks.EventPopupClosed Kind`; `hooks.PopupPayload{ID, Reason string}`; `ui.emitPopupOpened(id string)`, `ui.emitPopupClosed(id, reason string)`.

- [ ] **Step 1: Write the failing test** — `popup_log_test.go`:

```go
package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

func TestPopupOpenCloseEmit(t *testing.T) {
	opened := make(chan string, 2)
	closed := make(chan string, 2)
	uo := hooks.GlobalBus().Subscribe(hooks.EventPopupOpened, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.PopupPayload); ok {
			opened <- p.ID
		}
	})
	defer uo()
	uc := hooks.GlobalBus().Subscribe(hooks.EventPopupClosed, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.PopupPayload); ok {
			closed <- p.ID
		}
	})
	defer uc()

	emitPopupOpened("inst-menu")
	emitPopupClosed("inst-menu", "explicit")

	select {
	case id := <-opened:
		if id != "inst-menu" {
			t.Fatalf("opened id %q", id)
		}
	case <-time.After(time.Second):
		t.Fatal("no open event")
	}
	select {
	case id := <-closed:
		if id != "inst-menu" {
			t.Fatalf("closed id %q", id)
		}
	case <-time.After(time.Second):
		t.Fatal("no close event")
	}
}
```

- [ ] **Step 2: Run, expect FAIL.**

- [ ] **Step 3: Kinds + payload** in `events.go`:

```go
	// EventPopupOpened / EventPopupClosed fire from the shared overlay portal
	// for every menu/dialog/picker/slider-popup. Reason on close is
	// "explicit", "top", or "dismiss" (Esc/click-outside self-close).
	EventPopupOpened Kind = "popup.opened"
	EventPopupClosed Kind = "popup.closed"
```

Add both to `KindAll`. Payload:

```go
// PopupPayload identifies a portal overlay by its stable ID (e.g. "inst-menu",
// "color-wheel", "settings"). Reason is set on close only.
type PopupPayload struct {
	ID     string `json:"id"`
	Reason string `json:"reason,omitempty"`
}
```

Bump `NumNonVerbose` → `62`.

- [ ] **Step 4: Registry entries** in `actions.go`:

```go
	{EventPopupOpened, "open popup", "popup", ScopeInteraction, ""},
	{EventPopupClosed, "close popup", "popup", ScopeInteraction, ""},
```

- [ ] **Step 5: Formatters** in `format.go`:

```go
hooks.EventPopupOpened: func(p any) string {
	v, _ := p.(hooks.PopupPayload)
	return fmt.Sprintf("opened %s", v.ID)
},
hooks.EventPopupClosed: func(p any) string {
	v, _ := p.(hooks.PopupPayload)
	if v.Reason != "" {
		return fmt.Sprintf("closed %s (%s)", v.ID, v.Reason)
	}
	return fmt.Sprintf("closed %s", v.ID)
},
```

- [ ] **Step 6: Emit helpers** in `interaction_events.go`:

```go
func emitPopupOpened(id string) {
	if id == "" {
		return
	}
	hooks.PublishWithSource(hooks.EventPopupOpened, hooks.PopupPayload{ID: id}, hooks.CaptureSource(1))
}
func emitPopupClosed(id, reason string) {
	if id == "" {
		return
	}
	hooks.PublishWithSource(hooks.EventPopupClosed, hooks.PopupPayload{ID: id, Reason: reason}, hooks.CaptureSource(1))
}
```

- [ ] **Step 7: Wire the portal chokepoints** in `portal.go`:
  - In `Open(entry PortalEntry)`, after appending to the stack: `emitPopupOpened(entry.ID)`.
  - In `Close(id string)`, after `p.closeByID(id)`: `emitPopupClosed(id, "explicit")`.
  - In `CloseTop()`, capture the top id before closing and emit `emitPopupClosed(id, "top")`.
  - In `CleanupClosed()`, for each entry removed because `ShouldClose()`==true, emit `emitPopupClosed(entry.ID, "dismiss")`.

  Guard against double-emit: `CloseTop` and `CleanupClosed` ultimately call `closeByID`/`RemovePortal`, not `Close(id)`, so they will not double-fire. Verify by reading `closeByID` — if `CloseTop` calls `Close(id)` internally, emit only in `Close` and have `CloseTop`/`CleanupClosed` pass the reason through a private `closeWithReason(id, reason)` helper. (Implementer: pick the single path so each close emits once.)

- [ ] **Step 8: Run, expect PASS:**

`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestPopupOpenCloseEmit ./internal/ui/ && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/hooks/ ./internal/eventlogger/ ./internal/ui/`

- [ ] **Step 9: Checkpoint** — do not commit.

---

## Task 11: Layer-B — scroll gesture end (coalesced)

**Files:**
- Modify: `events.go` (const, payload, `KindAll`, `NumNonVerbose`→63)
- Modify: `actions.go` (entry)
- Modify: `format.go` (formatter)
- Modify: `coalesce.go` (add `EventScroll`)
- Modify: `interaction_events.go` (`emitScroll`)
- Modify: `scroll_behavior.go` (`HandleTouchEnd`:199, `HandleDragEnd`:72, `EndStepDrag`)
- Test: `src/go/internal/ui/scroll_log_test.go` (new) + `internal/eventlogger/coalesce_test.go` addition

**Interfaces:**
- Produces: `hooks.EventScroll Kind`; `hooks.ScrollPayload{Surface string}`; `ui.emitScroll(surface string)`.

- [ ] **Step 1: Write the failing test** — `scroll_log_test.go`:

```go
package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

func TestScrollGestureEndEmits(t *testing.T) {
	ch := make(chan string, 4)
	u := hooks.GlobalBus().Subscribe(hooks.EventScroll, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.ScrollPayload); ok {
			ch <- p.Surface
		}
	})
	defer u()

	emitScroll("inst-menu")

	select {
	case s := <-ch:
		if s != "inst-menu" {
			t.Fatalf("surface %q", s)
		}
	case <-time.After(time.Second):
		t.Fatal("no scroll event")
	}
}
```

- [ ] **Step 2: Run, expect FAIL.**

- [ ] **Step 3: Kind + payload** in `events.go`:

```go
	// EventScroll fires at scroll-gesture end (touch lift / scrollbar drag
	// release / stepped-grid drag end). Coalesced so a flick + momentum is one
	// line. Surface names the scrolled region.
	EventScroll Kind = "ui.scroll"
```

Add to `KindAll`. Payload:

```go
// ScrollPayload names the scrolled surface (e.g. "inst-menu", "row-rack",
// "synth-grid").
type ScrollPayload struct {
	Surface string `json:"surface"`
}
```

Bump `NumNonVerbose` → `63`.

- [ ] **Step 4: Registry entry** in `actions.go`:

```go
	{EventScroll, "scroll", "scroll", ScopeInteraction, ""},
```

- [ ] **Step 5: Formatter** in `format.go`:

```go
hooks.EventScroll: func(p any) string {
	s, _ := p.(hooks.ScrollPayload)
	if s.Surface == "" {
		return "scrolled"
	}
	return fmt.Sprintf("scrolled %s", s.Surface)
},
```

- [ ] **Step 6: Coalesce it** — add to `coalesceKinds` in `coalesce.go`:

```go
	hooks.EventScroll: {}, // flick + momentum → one trailing line
```

- [ ] **Step 7: Emit helper** in `interaction_events.go`:

```go
func emitScroll(surface string) {
	hooks.PublishWithSource(hooks.EventScroll, hooks.ScrollPayload{Surface: surface}, hooks.CaptureSource(1))
}
```

- [ ] **Step 8: Wire the chokepoints.** `ScrollBehavior` needs to know its surface name. Add a `Surface string` field to `ScrollBehavior` (set by each owner when constructing — default "" is acceptable, formatter renders "scrolled"). Then:
  - In `HandleTouchEnd()`: after `sb.TS.End()`, if a real scroll committed (`sb.TS.ScrollingCommitted()`), `emitScroll(sb.Surface)`.
  - In `HandleDragEnd()`: after `sb.VS.EndDrag()`, `emitScroll(sb.Surface)`.
  - In `EndStepDrag()`: after the existing body, `emitScroll(sb.Surface)`.

  Set `Surface` at the known owners (inst-menu, row-rack, synth grid) where `ScrollBehavior`/`TouchScroller` is created. Where the owner is not easily reachable, leave "" (still logs "scrolled").

- [ ] **Step 9: Add the coalesce unit assertion** in `internal/eventlogger/coalesce_test.go` (mirror the existing BPM-coalesce test): publish 10 `EventScroll` with the same surface in a burst, assert exactly one line. (Copy the structure of `TestEventLoggerCoalescesRapidBPM`, swapping the kind/payload and the line regexp `\[scroll\] scrolled inst-menu`.)

- [ ] **Step 10: Run, expect PASS:**

`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestScrollGestureEndEmits' ./internal/ui/ && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/hooks/ ./internal/eventlogger/`

- [ ] **Step 11: Checkpoint** — do not commit.

---

## Task 12: Layer-B — search query (coalesced)

**Files:**
- Modify: `events.go` (const, payload, `KindAll`, `NumNonVerbose`→64)
- Modify: `actions.go` (entry)
- Modify: `format.go` (formatter)
- Modify: `coalesce.go` (add `EventSearchChanged`)
- Modify: `interaction_events.go` (`emitSearchChanged`)
- Modify: `src/go/internal/ui/drumview_overlay_inst_comp.go:1393-1398` (after `m.rebuildMenu()`)
- Test: `src/go/internal/ui/search_log_test.go` (new)

**Interfaces:**
- Produces: `hooks.EventSearchChanged Kind`; `hooks.SearchPayload{Surface, Query string}`; `ui.emitSearchChanged(surface, query string)`.

- [ ] **Step 1: Write the failing test** — `search_log_test.go`:

```go
package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

func TestSearchEmits(t *testing.T) {
	ch := make(chan hooks.SearchPayload, 4)
	u := hooks.GlobalBus().Subscribe(hooks.EventSearchChanged, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.SearchPayload); ok {
			ch <- p
		}
	})
	defer u()

	emitSearchChanged("inst-menu", "kick")

	select {
	case p := <-ch:
		if p.Query != "kick" || p.Surface != "inst-menu" {
			t.Fatalf("got %+v", p)
		}
	case <-time.After(time.Second):
		t.Fatal("no search event")
	}
}
```

- [ ] **Step 2: Run, expect FAIL.**

- [ ] **Step 3: Kind + payload** in `events.go`:

```go
	// EventSearchChanged fires when a search field's query changes. Emitted
	// per-keystroke at the source but coalesced so rapid typing → one trailing
	// line with the settled query.
	EventSearchChanged Kind = "ui.search"
```

Add to `KindAll`. Payload:

```go
// SearchPayload carries the search surface + the (settled) query string.
type SearchPayload struct {
	Surface string `json:"surface"`
	Query   string `json:"query"`
}
```

Bump `NumNonVerbose` → `64`.

- [ ] **Step 4: Registry entry:** `{EventSearchChanged, "search", "search", ScopeInteraction, ""},`

- [ ] **Step 5: Formatter:**

```go
hooks.EventSearchChanged: func(p any) string {
	s, _ := p.(hooks.SearchPayload)
	if s.Query == "" {
		return fmt.Sprintf("search %s cleared", s.Surface)
	}
	return fmt.Sprintf("search %s = %q", s.Surface, s.Query)
},
```

- [ ] **Step 6: Coalesce it** — add `hooks.EventSearchChanged: {},` to `coalesceKinds`.

- [ ] **Step 7: Emit helper:**

```go
func emitSearchChanged(surface, query string) {
	hooks.PublishWithSource(hooks.EventSearchChanged, hooks.SearchPayload{Surface: surface, Query: query}, hooks.CaptureSource(1))
}
```

- [ ] **Step 8: Wire the chokepoint** in `drumview_overlay_inst_comp.go` where the search text changes:

```go
	if newText != m.state.searchText {
		m.state.searchText = newText
		m.rebuildMenu()
		emitSearchChanged("inst-menu", newText)
	}
```

- [ ] **Step 9: Run, expect PASS:**

`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestSearchEmits ./internal/ui/ && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/hooks/ ./internal/eventlogger/`

- [ ] **Step 10: Checkpoint** — do not commit.

---

## Task 13: Layer-B — generic text-input commit

**Files:**
- Modify: `events.go` (const, payload, `KindAll`, `NumNonVerbose`→65)
- Modify: `actions.go` (entry)
- Modify: `format.go` (formatter)
- Modify: `interaction_events.go` (`emitTextCommitted`)
- Modify: `src/go/internal/ui/textinput.go` (commit path: Enter at ~219, `OnFocusLost` at ~145)
- Test: `src/go/internal/ui/textcommit_log_test.go` (new)

**Interfaces:**
- Produces: `hooks.EventTextCommitted Kind`; `hooks.TextPayload{Field, Value string}`; `ui.emitTextCommitted(field, value string)`.

Note: domain text commits that already emit a specific event (instrument rename → `EventInstrumentRenamed`, BPM entry → `EventBPMChange`) MUST NOT also call `emitTextCommitted`. This helper is only wired into the generic `TextInput` commit path for inputs whose `Field` tag is set; rename/BPM editors set no field tag, so they are skipped (see Step 8 guard).

- [ ] **Step 1: Write the failing test** — `textcommit_log_test.go`:

```go
package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

func TestTextCommitEmits(t *testing.T) {
	ch := make(chan hooks.TextPayload, 2)
	u := hooks.GlobalBus().Subscribe(hooks.EventTextCommitted, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.TextPayload); ok {
			ch <- p
		}
	})
	defer u()

	emitTextCommitted("project-name", "my song")

	select {
	case p := <-ch:
		if p.Field != "project-name" || p.Value != "my song" {
			t.Fatalf("got %+v", p)
		}
	case <-time.After(time.Second):
		t.Fatal("no text-commit event")
	}
}
```

- [ ] **Step 2: Run, expect FAIL.**

- [ ] **Step 3: Kind + payload** in `events.go`:

```go
	// EventTextCommitted fires when a generic text field commits (Enter/blur).
	// Domain-specific text commits (instrument rename, BPM entry) use their own
	// events and do NOT emit this.
	EventTextCommitted Kind = "ui.text"
```

Add to `KindAll`. Payload:

```go
// TextPayload carries the field tag + the committed value.
type TextPayload struct {
	Field string `json:"field"`
	Value string `json:"value"`
}
```

Bump `NumNonVerbose` → `65`.

- [ ] **Step 4: Registry entry:** `{EventTextCommitted, "edit text", "input", ScopeInteraction, ""},`

- [ ] **Step 5: Formatter:**

```go
hooks.EventTextCommitted: func(p any) string {
	t, _ := p.(hooks.TextPayload)
	return fmt.Sprintf("%s = %q", t.Field, t.Value)
},
```

- [ ] **Step 6: Emit helper:**

```go
func emitTextCommitted(field, value string) {
	if field == "" {
		return // no field tag ⇒ a domain editor owns this commit; skip generic log
	}
	hooks.PublishWithSource(hooks.EventTextCommitted, hooks.TextPayload{Field: field, Value: value}, hooks.CaptureSource(1))
}
```

- [ ] **Step 7: Add a `LogField string` to `TextInput`** in `textinput.go` (default ""), so only fields explicitly tagged emit the generic line.

- [ ] **Step 8: Wire the commit path.** In `textinput.go`, at the Enter-commit (`t.focused = false; return true`) and in the `OnFocusLost` path, add:

```go
	emitTextCommitted(t.LogField, t.Value())
```

(`emitTextCommitted` no-ops when `LogField==""`, so rename/BPM/search inputs — which leave it unset — never double-log.)

- [ ] **Step 9: Run, expect PASS:**

`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestTextCommitEmits ./internal/ui/ && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/hooks/ ./internal/eventlogger/`

- [ ] **Step 10: Checkpoint** — do not commit.

---

## Task 14: Layer-B — mobile view-mode switch + audio-panel tab switch

**Files:**
- Modify: `events.go` (const, payload, `KindAll`, `NumNonVerbose`→66)
- Modify: `actions.go` (entry)
- Modify: `format.go` (formatter)
- Modify: `interaction_events.go` (`emitViewMode`)
- Modify: `src/go/internal/ui/drumview_layout.go` (`setViewMode`)
- Modify: `src/go/internal/ui/audio_sticky_bar.go` (`SetActiveTab` user-driven path) OR the `onTab` closure — emit `EventAudioPanelStateChanged{Field:"tab:<name>"}`
- Test: `src/go/internal/ui/viewmode_log_test.go` (new)

**Interfaces:**
- Produces: `hooks.EventViewModeChanged Kind`; `hooks.ViewModePayload{Mode string}`; `ui.emitViewMode(mode string)`.

- [ ] **Step 1: Write the failing test** — `viewmode_log_test.go`:

```go
package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

func TestViewModeEmits(t *testing.T) {
	ch := make(chan string, 2)
	u := hooks.GlobalBus().Subscribe(hooks.EventViewModeChanged, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.ViewModePayload); ok {
			ch <- p.Mode
		}
	})
	defer u()

	emitViewMode("eq")

	select {
	case m := <-ch:
		if m != "eq" {
			t.Fatalf("mode %q", m)
		}
	case <-time.After(time.Second):
		t.Fatal("no view-mode event")
	}
}
```

- [ ] **Step 2: Run, expect FAIL.**

- [ ] **Step 3: Kind + payload** in `events.go`:

```go
	// EventViewModeChanged fires when the mobile bottom-nav switches the
	// visible view (pads/eq/wave/spectrum/levels/chain/synth).
	EventViewModeChanged Kind = "ui.view"
```

Add to `KindAll`. Payload:

```go
// ViewModePayload names the newly-selected mobile view mode.
type ViewModePayload struct {
	Mode string `json:"mode"`
}
```

Bump `NumNonVerbose` → `66` (final). Update the comment with the final total.

- [ ] **Step 4: Registry entry:** `{EventViewModeChanged, "switch view", "view", ScopeInteraction, ""},`

- [ ] **Step 5: Formatter:**

```go
hooks.EventViewModeChanged: func(p any) string {
	v, _ := p.(hooks.ViewModePayload)
	return fmt.Sprintf("view → %s", v.Mode)
},
```

- [ ] **Step 6: Emit helper:**

```go
func emitViewMode(mode string) {
	hooks.PublishWithSource(hooks.EventViewModeChanged, hooks.ViewModePayload{Mode: mode}, hooks.CaptureSource(1))
}
```

- [ ] **Step 7: Wire `setViewMode`** in `drumview_layout.go` — emit only on an actual change (dedupe):

```go
func (dv *DrumView) setViewMode(m viewMode) {
	if dv.currentViewMode == m {
		return
	}
	dv.currentViewMode = m
	emitViewMode(viewModeName(m)) // viewModeName maps the enum to a lowercase label
	... // existing body
}
```

If a `viewModeName` helper does not exist, add a small `func viewModeName(m viewMode) string` switch returning `"pads"`/`"eq"`/`"wave"`/`"spectrum"`/`"levels"`/`"chain"`/`"synth"`. Confirm the existing field name (`currentViewMode`) by reading `drumview_layout.go`.

- [ ] **Step 8: Wire audio-panel tab switch** (reuses the existing `EventAudioPanelStateChanged`). In the user-driven `onTab` closure (the sticky-bar tab-pill click handler in `eq_panel_zone.go initButtons` / `audio_sticky_bar.go`), after the tab changes, call the existing helper with a `tab:` field:

```go
	emitAudioPanelStateChanged("tab:" + panelTabName(tab))
```

Add a `panelTabName(PanelTab) string` helper if absent (lowercase: `wave`/`spectrum`/`levels`/`eq`/`chain`/`synth`). Do NOT add this to the programmatic `SetActiveTab` paths used by scene/init — only the user click path, to avoid logging on startup. (This emits under the `panel` tag set in Task 1; no new Kind.)

- [ ] **Step 9: Run, expect PASS:**

`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestViewModeEmits ./internal/ui/ && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/hooks/ ./internal/eventlogger/ ./internal/ui/`

- [ ] **Step 10: Checkpoint** — do not commit.

---

## Task 15: Documentation

**Files:**
- Modify: `CLAUDE.md` ("Event Notification (Two-Tier)" + "Log levels" sections)
- Modify: `DESIGN.md` (only if it references log tags — verify first)

- [ ] **Step 1: Verify DESIGN.md references:** `grep -n "\[audio\]\|\[camera\]\|log tag\|INFO tag" DESIGN.md` — edit only if hits exist.

- [ ] **Step 2: Update CLAUDE.md** — in the "Event Notification" section, document: (a) the bracket tag is now a field on `hooks.ActionRegistry` (`ActionMeta.Tag`) — the single source of truth, read by `eventlogger`; (b) the full lowercase tag taxonomy table from the spec; (c) Layer-B interaction telemetry (`ui`/`popup`/`scroll`/`search`/`input`/`view` tags, emitted at `Button.applyPress`, `portal.Open/Close`, scroll release, search, text commit, `setViewMode`); (d) the anti-pollution rules (one-per-gesture, coalescer keyed by Kind so interaction telemetry uses per-surface kinds, accepted tap+effect duplication); (e) all bracket tags are lowercase (guard: `internal/log/tag_case_test.go::TestNoUppercaseInfoTags`). Add a note that adding a new `hooks.Kind` requires a `Tag` in the registry (guard: `TestEveryActionHasLowercaseTag`).

- [ ] **Step 3: Full fast suite (final gate):**

`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...`
Expected: PASS (note any pre-existing, unrelated failures and confirm they fail identically on the base branch).

- [ ] **Step 4: Browser bridge smoke** (no new JS exports, but confirm the catalogue drift guard is unaffected):

`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestJSExportsCatalogueDrift ./internal/ui/`
Expected: PASS.

- [ ] **Step 5: Checkpoint** — do not commit. Report completion to the user with a summary of the new tags + event kinds for their review.

---

## Self-review notes

- **Spec coverage:** Casing (Tasks 1, 4) ✓; `[audio]` split (Task 1) ✓; pan/zoom one-per-gesture + de-verbose (Task 8) ✓; language/undo/redo gaps (Tasks 6, 7) ✓; blanket button tap (Task 9) ✓; portal open/close (Task 10) ✓; scroll (Task 11) ✓; search (Task 12) ✓; text commit (Task 13) ✓; view + tab switch (Task 14) ✓; registry-as-source-of-truth + lowercase test (Tasks 1, 2) ✓; docs (Task 15) ✓; `NumNonVerbose` ledger reconciled to 66 ✓.
- **Density** intentionally excluded (no commit site) — matches spec "Out of scope".
- **Type consistency:** payload type names (`LanguagePayload`, `UndoPayload`, `UITapPayload`, `PopupPayload`, `ScrollPayload`, `SearchPayload`, `TextPayload`, `ViewModePayload`) and helper names (`emitLanguageChanged`, `emitUndo`/`emitRedo`, `emitUITap`, `emitPopupOpened`/`emitPopupClosed`, `emitScroll`, `emitSearchChanged`, `emitTextCommitted`, `emitViewMode`) are used consistently across their defining task and the file map.
- **Verification done:** `hooks.GlobalBus()` (bus.go:220), package-level `hooks.PublishWithSource`/`Subscribe`/`CaptureSource`, and `NewButton(text, ButtonVisual, func())` with concrete `ButtonStyle{}` are all confirmed against the codebase. The one symbol still to confirm at implementation time is `viewMode`/`currentViewMode` field naming in `drumview_layout.go` (Task 14 Step 7) and whether a `viewModeName`/`panelTabName` helper already exists.
