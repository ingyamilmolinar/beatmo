# Instrument Identity Decoupling + Category Lookup — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make renaming an instrument a metadata-only change (the immutable ID never moves), add a canonical category lookup API keyed by ID, and collapse the three-category tangle into one source of truth.

**Architecture:** A process-global display-name override store in `internal/audio` resolves an instrument's label by ID (override → catalog name → pretty-cased id). A new `category.go` derives each instrument's category from its bound recipe + the existing role heuristic, exposing `CategoryOf` / `InstrumentsByCategory` behind a registry seam. Rename collapses to one `SetInstrumentDisplayName` call; the old `RenameInstrument` ID-rewrite cascade is deleted. Duplicate-ID templates are migrated to distinct `-N` instance IDs that resolve to the base recipe via a fallback in `factoryRecipeForInstrument`.

**Tech Stack:** Go 1.23, bundled toolchain at `.tools/go/bin/go`. Audio engine has build-tag splits (`!js` desktop / `js` wasm / `test` stub).

## Global Constraints

- **Bundled Go only:** run everything via `.tools/go/bin/go` (from `src/go`, prefix `../../.tools/go/bin/go`).
- **Fast test command:** `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod <pkg>`.
- **No git commits.** The user forbids `git add`/commit (standing rule). Every task's final step is "run the suite and leave the work uncommitted" — do NOT run `git commit`.
- **No backwards compatibility.** Sole user. Delete dead code outright; no migration shims, no deprecated aliases.
- **Identity = immutable instrument ID.** Display name and category are metadata resolved by lookup keyed by ID. Renaming never changes an ID and never re-keys any map.
- **Custom-category persistence is deferred.** Build the registry seam (`RegisterCategory`/`AssignCategory`) but wire no storage.
- **Format after editing:** `.tools/go/bin/gofmt -w <files>` before running tests.

---

### Task 1: Display-name override store in `internal/audio`

**Files:**
- Create: `src/go/internal/audio/instrument_names.go`
- Create: `src/go/internal/audio/instrument_names_test.go`

**Interfaces:**
- Consumes: existing `CatalogLookup(id string) (SoundMeta, bool)`, `PrettyName(s string) string`.
- Produces:
  - `func SetInstrumentDisplayName(id, name string)`
  - `func InstrumentDisplayName(id string) string`
  - `func ClearInstrumentDisplayName(id string)`

- [ ] **Step 1: Write the failing test**

`src/go/internal/audio/instrument_names_test.go`:

```go
package audio

import "testing"

func TestInstrumentDisplayNameResolution(t *testing.T) {
	ClearInstrumentDisplayName("snare")
	// Falls back to PrettyName(id) when no override and no catalog name.
	if got := InstrumentDisplayName("hi-hat-x"); got != "Hi Hat X" {
		t.Fatalf("pretty fallback: got %q want %q", got, "Hi Hat X")
	}
	// Override wins.
	SetInstrumentDisplayName("hi-hat-x", "My Hat")
	if got := InstrumentDisplayName("hi-hat-x"); got != "My Hat" {
		t.Fatalf("override: got %q want %q", got, "My Hat")
	}
	// Clearing restores the fallback.
	ClearInstrumentDisplayName("hi-hat-x")
	if got := InstrumentDisplayName("hi-hat-x"); got != "Hi Hat X" {
		t.Fatalf("after clear: got %q want %q", got, "Hi Hat X")
	}
	// Empty id yields empty.
	if got := InstrumentDisplayName(""); got != "" {
		t.Fatalf("empty id: got %q want empty", got)
	}
}

func TestInstrumentDisplayNameSettingEmptyClears(t *testing.T) {
	SetInstrumentDisplayName("foo-bar", "Custom")
	SetInstrumentDisplayName("foo-bar", "")
	if got := InstrumentDisplayName("foo-bar"); got != "Foo Bar" {
		t.Fatalf("setting empty should clear override: got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/ -run TestInstrumentDisplayName`
Expected: FAIL — undefined `SetInstrumentDisplayName` / `InstrumentDisplayName` / `ClearInstrumentDisplayName`.

- [ ] **Step 3: Write minimal implementation**

`src/go/internal/audio/instrument_names.go`:

```go
package audio

import (
	"path"
	"strings"
	"sync"
)

// instNameOverrides holds user-set display-name overrides keyed by the
// immutable instrument ID. Renaming an instrument writes here and nowhere
// else — the ID never moves, so no other map needs re-keying.
var (
	instNameMu        sync.RWMutex
	instNameOverrides = map[string]string{}
)

// SetInstrumentDisplayName records a display-name override for an instrument
// ID. An empty name clears the override (the instrument falls back to its
// catalog/pretty default). The ID is unchanged — this is pure metadata.
func SetInstrumentDisplayName(id, name string) {
	if id == "" {
		return
	}
	instNameMu.Lock()
	if name == "" {
		delete(instNameOverrides, id)
	} else {
		instNameOverrides[id] = name
	}
	instNameMu.Unlock()
}

// ClearInstrumentDisplayName removes any override for the instrument ID.
func ClearInstrumentDisplayName(id string) {
	instNameMu.Lock()
	delete(instNameOverrides, id)
	instNameMu.Unlock()
}

// InstrumentDisplayName resolves the user-facing label for an instrument ID.
// Resolution order: user override → catalog SoundMeta.Name → catalog RelPath
// basename (pretty-cased) → PrettyName(id). This is the single source of
// truth for instrument labels across UI and non-UI callers.
func InstrumentDisplayName(id string) string {
	if id == "" {
		return ""
	}
	instNameMu.RLock()
	override, ok := instNameOverrides[id]
	instNameMu.RUnlock()
	if ok && override != "" {
		return override
	}
	if meta, ok := CatalogLookup(id); ok {
		if meta.Name != "" {
			return meta.Name
		}
		if meta.RelPath != "" {
			base := path.Base(meta.RelPath)
			base = strings.TrimSuffix(base, path.Ext(base))
			if base != "" {
				return PrettyName(base)
			}
		}
	}
	return PrettyName(id)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/ -run TestInstrumentDisplayName -v`
Expected: PASS (both tests).

- [ ] **Step 5: Format and run the audio package suite**

Run: `cd src/go && ../../.tools/go/bin/gofmt -w internal/audio/instrument_names.go internal/audio/instrument_names_test.go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/`
Expected: PASS. Leave uncommitted.

---

### Task 2: Route UI label resolution through the store

**Files:**
- Modify: `src/go/internal/ui/drumview_instrument_helpers.go:53-69` (`computeInstLabel`)
- Test: `src/go/internal/ui/instrument_label_store_test.go` (create)

**Interfaces:**
- Consumes: `audio.InstrumentDisplayName(id)` (Task 1).
- Produces: `computeInstLabel` now delegates to the store; behavior unchanged for un-overridden instruments.

- [ ] **Step 1: Write the failing test**

`src/go/internal/ui/instrument_label_store_test.go`:

```go
package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestComputeInstLabelHonorsOverride(t *testing.T) {
	dv := &DrumView{}
	audio.ClearInstrumentDisplayName("kick")
	if got := dv.computeInstLabel("kick"); got != "Kick" {
		t.Fatalf("default: got %q want %q", got, "Kick")
	}
	audio.SetInstrumentDisplayName("kick", "Thumper")
	defer audio.ClearInstrumentDisplayName("kick")
	if got := dv.computeInstLabel("kick"); got != "Thumper" {
		t.Fatalf("override: got %q want %q", got, "Thumper")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestComputeInstLabelHonorsOverride`
Expected: FAIL — override ignored (returns "Kick").

- [ ] **Step 3: Replace `computeInstLabel` body**

In `src/go/internal/ui/drumview_instrument_helpers.go`, replace the whole `computeInstLabel` function (lines 53-69) with:

```go
// computeInstLabel computes the display label for an instrument ID without
// caching. Delegates to audio.InstrumentDisplayName, the single source of
// truth: user override → catalog name → catalog basename → pretty-cased id.
func (dv *DrumView) computeInstLabel(id string) string {
	return audio.InstrumentDisplayName(id)
}
```

The `path` import in this file is now unused only if no other function uses it — check: `instMenuThumbRect`/`matchInstrumentSearch` don't use `path`. Remove `"path"` from the import block if `gofmt`/build reports it unused.

- [ ] **Step 4: Run test + package build**

Run: `cd src/go && ../../.tools/go/bin/gofmt -w internal/ui/drumview_instrument_helpers.go internal/ui/instrument_label_store_test.go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestComputeInstLabelHonorsOverride -v`
Expected: PASS.

- [ ] **Step 5: Run the ui suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS. Leave uncommitted.

---

### Task 3: Make rename metadata-only; delete `RenameInstrument`

**Files:**
- Modify: `src/go/internal/ui/js_exports_graph_ui.go:1065-1093` (`commitRename`) and `:1112-1134` (`openRenameBox` OnCommit)
- Modify: `src/go/internal/ui/drumview_ctor.go:~808-820` (third rename site)
- Delete: `RenameInstrument` from `src/go/internal/audio/engine_state.go:89-109`, `src/go/internal/audio/stub.go:128-137`, `src/go/internal/audio/engine_wasm.go:186-201`; and `renameInstrumentChannel` from `src/go/internal/audio/channels.go:710` (only caller is RenameInstrument — confirm with grep first)
- Test: `src/go/internal/ui/rename_metadata_only_test.go` (create)

**Interfaces:**
- Consumes: `audio.SetInstrumentDisplayName` (Task 1), existing `dv.invalidateLabelCaches()`, `dv.refreshInstruments()`, `dv.instDisplayLabel(id)`.
- Produces: rename no longer changes `row.Instrument`; no `audio.RenameInstrument` symbol remains.

- [ ] **Step 1: Write the failing test**

`src/go/internal/ui/rename_metadata_only_test.go`. Use the existing DrumView test constructor pattern (see `testutil_test.go` / neighbouring `drumview_test.go` for `newTestDrumView`-style setup; if a helper named differently exists, use it). The test renames row 0 and asserts the instrument ID is unchanged while the display name follows:

```go
package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestRenameIsMetadataOnly(t *testing.T) {
	dv := newDrumViewForTest(t) // existing test helper; row 0 seeded with an instrument
	row := 0
	origID := dv.Rows[row].Instrument
	if origID == "" {
		t.Fatalf("row 0 has no instrument")
	}
	defer audio.ClearInstrumentDisplayName(origID)

	dv.renameInstrumentTo(row, "My Custom Name")

	if dv.Rows[row].Instrument != origID {
		t.Fatalf("rename changed ID: got %q want %q (rename must be metadata-only)", dv.Rows[row].Instrument, origID)
	}
	if dv.instDisplayLabel(origID) != "My Custom Name" {
		t.Fatalf("display label not updated: got %q", dv.instDisplayLabel(origID))
	}
	if audio.InstrumentDisplayName(origID) != "My Custom Name" {
		t.Fatalf("store not updated: got %q", audio.InstrumentDisplayName(origID))
	}
}
```

Note: if no `newDrumViewForTest` helper exists, reuse whichever constructor the existing rename/label tests in `internal/ui` use (grep `func newDrumView` / `func newTestDrumView` in `*_test.go`). Do NOT invent a new harness.

- [ ] **Step 2: Add the shared rename helper (so all three sites and the test share one code path)**

In `src/go/internal/ui/drumview_instruments.go`, add a method that is the single rename implementation:

```go
// renameInstrumentTo sets a metadata-only display-name override for the
// instrument in the given row. The instrument ID is never changed — this is
// the entire rename operation. Empty/whitespace names are ignored.
func (dv *DrumView) renameInstrumentTo(row int, name string) {
	name = strings.TrimSpace(name)
	if name == "" || row < 0 || row >= len(dv.Rows) {
		return
	}
	id := dv.Rows[row].Instrument
	if id == "" {
		return
	}
	audio.SetInstrumentDisplayName(id, name)
	dv.Rows[row].Name = dv.computeInstLabel(id)
	if row < len(dv.rowLabels()) {
		dv.rowLabels()[row].Text = dv.Rows[row].Name
	}
	dv.invalidateLabelCaches()
	if dv.IsInstMenuOpen() {
		dv.refreshInstMenuComponent()
	}
	dv.markRowControlsDirty()
	dv.bgDirty = true
}
```

Add `"strings"` to the import block if not already present (it is used elsewhere in the package but check this file's imports).

- [ ] **Step 3: Run the new test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/gofmt -w internal/ui/drumview_instruments.go internal/ui/rename_metadata_only_test.go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestRenameIsMetadataOnly -v`
Expected: PASS.

- [ ] **Step 4: Replace the three rename call-sites to use the helper**

In `src/go/internal/ui/js_exports_graph_ui.go`, replace the body of the `commitRename` success branch (lines 1071-1090) with:

```go
		if name != "" && g.drum.renameRow >= 0 && g.drum.renameRow < len(g.drum.Rows) {
			g.drum.renameInstrumentTo(g.drum.renameRow, name)
		}
```

In the same file, replace the `openRenameBox` `OnCommit` success branch (lines 1114-1133) with:

```go
					if name != "" && g.drum.renameRow >= 0 && g.drum.renameRow < len(g.drum.Rows) {
						g.drum.renameInstrumentTo(g.drum.renameRow, name)
						g.drum.notifyInfoKey(i18n.KeyNotifRenamedInstrument, name)
					}
					g.drum.renameRow = -1
```

In `src/go/internal/ui/drumview_ctor.go` (around line 808-820), replace the equivalent `oldID := …; newID := strings.ToLower(name); audio.RenameInstrument(...)` block with a single `dv.renameInstrumentTo(<row>, name)` call (read the surrounding code to get the correct row variable and trailing bookkeeping; keep any `notifyInfoKey` line, drop the ID-rewrite cascade).

- [ ] **Step 5: Delete `RenameInstrument` and `renameInstrumentChannel`**

First confirm no remaining callers:

Run: `cd src/go && grep -rn "RenameInstrument\b\|renameInstrumentChannel" internal/ --include=*.go | grep -v _test`
Expected: only the three definition sites (`engine_state.go`, `stub.go`, `engine_wasm.go`) and the `channels.go` definition remain — no call sites.

Delete:
- `RenameInstrument` func in `engine_state.go` (lines 89-109)
- `RenameInstrument` func in `stub.go` (lines 128-137)
- `RenameInstrument` func in `engine_wasm.go` (lines 186-201)
- `renameInstrumentChannel` func in `channels.go` (line 710 + body)

If `renameInstrumentChannel` has any other caller, leave it and only delete the three `RenameInstrument` funcs.

- [ ] **Step 6: Run audio + ui suites**

Run: `cd src/go && ../../.tools/go/bin/gofmt -w internal/ui/js_exports_graph_ui.go internal/ui/drumview_ctor.go internal/audio/engine_state.go internal/audio/stub.go internal/audio/engine_wasm.go internal/audio/channels.go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/ ./internal/ui/`
Expected: PASS. Rewrite or delete any test that asserted the old "rename changes the ID" behavior (search `RenameInstrument` in `*_test.go` and update to assert ID-stability). Leave uncommitted.

---

### Task 4: Import/export use the name store

**Files:**
- Modify: `src/go/internal/ui/import.go:436` (`row.Name = inst.Name`)
- Modify: `src/go/internal/ui/export.go` (the `exportInstrument` build loop, ~line 428-453, `Name:` field)
- Test: add a case to the existing import/export round-trip test (`src/go/internal/ui/import_export_logic_test.go`)

**Interfaces:**
- Consumes: `audio.SetInstrumentDisplayName`, `audio.InstrumentDisplayName`, `dv.computeInstLabel`.
- Produces: project JSON `name` field is the display-name override; round-trips through the store.

- [ ] **Step 1: Write the failing test**

Add to `src/go/internal/ui/import_export_logic_test.go` (match the file's existing harness style — reuse its export/import helpers):

```go
func TestImportSetsDisplayNameOverride(t *testing.T) {
	audio.ClearInstrumentDisplayName("kick")
	defer audio.ClearInstrumentDisplayName("kick")
	// Build a minimal project JSON with one instrument id "kick" named "Boomer".
	js := `{"version":1,"subdiv":8,"bpm":120,` +
		`"instruments":[{"name":"Boomer","id":"kick","kind":"builtin","volume":1,"origin":0,"color":"#C87850FF"}],` +
		`"nodes":[{"id":0,"i":0,"j":0,"type":"regular","outputs":[]}]}`
	g := newGameForImportTest(t) // existing helper used by other tests in this file
	if err := g.importProjectJSON([]byte(js)); err != nil { // use the file's actual import entrypoint
		t.Fatalf("import: %v", err)
	}
	if got := audio.InstrumentDisplayName("kick"); got != "Boomer" {
		t.Fatalf("override not set on import: got %q", got)
	}
}
```

Adjust `newGameForImportTest` / `importProjectJSON` to the real helper + entrypoint names used elsewhere in `import_export_logic_test.go` (grep the file first).

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestImportSetsDisplayNameOverride`
Expected: FAIL — `InstrumentDisplayName("kick")` returns "Kick", not "Boomer".

- [ ] **Step 3: Update import**

In `src/go/internal/ui/import.go`, replace line 436 (`row.Name = inst.Name`) with:

```go
		if inst.Name != "" {
			audio.SetInstrumentDisplayName(inst.ID, inst.Name)
		} else {
			audio.ClearInstrumentDisplayName(inst.ID)
		}
		row.Name = g.drum.computeInstLabel(inst.ID)
```

- [ ] **Step 4: Update export**

In `src/go/internal/ui/export.go`, in the `exportInstrument` build loop, set the `Name` field from the resolved display name instead of `r.Name`:

```go
		Name: audio.InstrumentDisplayName(r.Instrument),
```

(Find the existing `Name: r.Name,` assignment in `exportInstrument{...}` and replace it.)

- [ ] **Step 5: Run test + ui suite**

Run: `cd src/go && ../../.tools/go/bin/gofmt -w internal/ui/import.go internal/ui/export.go internal/ui/import_export_logic_test.go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestImportSetsDisplayNameOverride|Import|Export'`
Expected: PASS. Then run the full ui suite. Leave uncommitted.

---

### Task 5: Canonical category taxonomy + lookup API

**Files:**
- Create: `src/go/internal/audio/category.go`
- Create: `src/go/internal/audio/category_test.go`

**Interfaces:**
- Consumes: `factoryRecipeForInstrument(id)` (with Task 7's base-ID fallback, but works standalone now), `roleHeuristic(id)`, `Instruments()`, `Catalog()`.
- Produces:
  - types `CategoryID string`, `Category struct { ID CategoryID; Name string }`
  - category consts `CatKick … CatOther`
  - `func Categories() []Category`
  - `func CategoryOf(id string) CategoryID`
  - `func InstrumentsByCategory(cat CategoryID) []string`
  - registry seam `func RegisterCategory(c Category)`, `func AssignCategory(instID string, cat CategoryID)`

- [ ] **Step 1: Write the failing test**

`src/go/internal/audio/category_test.go`:

```go
package audio

import (
	"slices"
	"testing"
)

func TestCategoryOfBuiltins(t *testing.T) {
	cases := map[string]CategoryID{
		"kick":           CatKick,
		"snare":          CatSnare,
		"hihat":          CatHiHat,
		"violin":         CatStrings,
		"guitar-electric": CatGuitar,
		"organ":          CatKeys,
		"trumpet":        CatBrass,
		"flute":          CatWoodwind,
		"totally-unknown-xyz": CatOther,
	}
	for id, want := range cases {
		if got := CategoryOf(id); got != want {
			t.Errorf("CategoryOf(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestInstrumentsByCategoryContainsKick(t *testing.T) {
	got := InstrumentsByCategory(CatKick)
	if !slices.Contains(got, "kick") {
		t.Fatalf("InstrumentsByCategory(CatKick) missing 'kick': %v", got)
	}
}

func TestAssignCategoryOverride(t *testing.T) {
	AssignCategory("kick", CatPercussion)
	defer AssignCategory("kick", CatKick) // restore
	if got := CategoryOf("kick"); got != CatPercussion {
		t.Fatalf("AssignCategory override: got %q want %q", got, CatPercussion)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/ -run 'TestCategory|TestInstrumentsByCategory|TestAssignCategory'`
Expected: FAIL — undefined symbols.

- [ ] **Step 3: Implement**

`src/go/internal/audio/category.go`:

```go
package audio

import (
	"strings"
	"sync"
)

// CategoryID is the stable identifier for an instrument category. The
// built-in taxonomy is hardcoded; custom categories may be added at runtime
// via RegisterCategory (persistence is deferred).
type CategoryID string

// Category is a displayable taxonomy entry.
type Category struct {
	ID   CategoryID
	Name string
}

// Built-in category taxonomy. Names are display labels.
const (
	CatKick       CategoryID = "kick"
	CatSnare      CategoryID = "snare"
	CatHiHat      CategoryID = "hihat"
	CatCymbal     CategoryID = "cymbal"
	CatTom        CategoryID = "tom"
	CatPercussion CategoryID = "percussion"
	CatBass       CategoryID = "bass"
	CatKeys       CategoryID = "keys"
	CatGuitar     CategoryID = "guitar"
	CatStrings    CategoryID = "strings"
	CatWoodwind   CategoryID = "woodwind"
	CatBrass      CategoryID = "brass"
	CatLead       CategoryID = "lead"
	CatOther      CategoryID = "other"
)

var (
	categoryMu sync.RWMutex
	// builtinCategoryOrder is the canonical display order.
	builtinCategoryOrder = []Category{
		{CatKick, "Kick"}, {CatSnare, "Snare"}, {CatHiHat, "Hi-Hat"},
		{CatCymbal, "Cymbal"}, {CatTom, "Tom"}, {CatPercussion, "Percussion"},
		{CatBass, "Bass"}, {CatKeys, "Keys"}, {CatGuitar, "Guitar"},
		{CatStrings, "Strings"}, {CatWoodwind, "Woodwind"}, {CatBrass, "Brass"},
		{CatLead, "Lead/Synth"}, {CatOther, "Other"},
	}
	// extraCategories holds custom categories registered at runtime.
	extraCategories []Category
	// categoryAssignments holds explicit per-instrument overrides (custom or
	// corrective) that take precedence over the derived category.
	categoryAssignments = map[string]CategoryID{}
)

// Categories returns the taxonomy in display order (built-ins, then customs).
func Categories() []Category {
	categoryMu.RLock()
	defer categoryMu.RUnlock()
	out := make([]Category, 0, len(builtinCategoryOrder)+len(extraCategories))
	out = append(out, builtinCategoryOrder...)
	out = append(out, extraCategories...)
	return out
}

// RegisterCategory adds a custom category to the taxonomy (idempotent by ID).
// Persistence is deferred; this only mutates the in-memory registry.
func RegisterCategory(c Category) {
	if c.ID == "" {
		return
	}
	categoryMu.Lock()
	defer categoryMu.Unlock()
	for _, e := range builtinCategoryOrder {
		if e.ID == c.ID {
			return
		}
	}
	for i, e := range extraCategories {
		if e.ID == c.ID {
			extraCategories[i] = c
			return
		}
	}
	extraCategories = append(extraCategories, c)
}

// AssignCategory pins an instrument ID to a category, overriding derivation.
func AssignCategory(instID string, cat CategoryID) {
	if instID == "" {
		return
	}
	categoryMu.Lock()
	categoryAssignments[instID] = cat
	categoryMu.Unlock()
}

// CategoryOf returns the category for an instrument ID. Order: explicit
// assignment → recipe-derived → role-heuristic-derived → CatOther.
func CategoryOf(id string) CategoryID {
	if id == "" {
		return CatOther
	}
	categoryMu.RLock()
	assigned, ok := categoryAssignments[id]
	categoryMu.RUnlock()
	if ok {
		return assigned
	}
	if c, ok := categoryByRecipe(factoryRecipeForInstrument(id)); ok {
		return c
	}
	if c, ok := categoryByRole(roleHeuristic(id)); ok {
		return c
	}
	// Last resort: try the id text itself (covers WAV-only catalog ids with
	// no recipe binding, e.g. "kick-acoustic").
	if c, ok := categoryByText(id); ok {
		return c
	}
	return CatOther
}

// InstrumentsByCategory returns every known instrument ID in a category, in
// registration/catalog order.
func InstrumentsByCategory(cat CategoryID) []string {
	seen := map[string]bool{}
	var out []string
	add := func(id string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		if CategoryOf(id) == cat {
			out = append(out, id)
		}
	}
	for _, id := range Instruments() {
		add(id)
	}
	for _, m := range Catalog() {
		add(m.ID)
	}
	return out
}

// categoryByRecipe maps a SynthRecipe id to a category via substring rules.
func categoryByRecipe(recipe string) (CategoryID, bool) {
	if recipe == "" {
		return "", false
	}
	return categoryByText(recipe)
}

// categoryByText classifies an id/recipe string by substring. Ordered most-
// specific first so "open-hihat" hits HiHat before any generic rule.
func categoryByText(s string) (CategoryID, bool) {
	lc := strings.ToLower(s)
	switch {
	case strings.Contains(lc, "hihat"), strings.Contains(lc, "hi-hat"), strings.Contains(lc, "hat"):
		return CatHiHat, true
	case strings.Contains(lc, "ride"), strings.Contains(lc, "crash"), strings.Contains(lc, "cymbal"):
		return CatCymbal, true
	case strings.Contains(lc, "kick"):
		return CatKick, true
	case strings.Contains(lc, "snare"), strings.Contains(lc, "rimshot"), strings.Contains(lc, "sidestick"):
		return CatSnare, true
	case strings.Contains(lc, "tom"):
		return CatTom, true
	case strings.Contains(lc, "clap"), strings.Contains(lc, "cowbell"), strings.Contains(lc, "shaker"), strings.Contains(lc, "conga"), strings.Contains(lc, "perc"):
		return CatPercussion, true
	case strings.Contains(lc, "bass"), strings.Contains(lc, "808"), strings.Contains(lc, "sub"):
		return CatBass, true
	case strings.Contains(lc, "guitar"):
		return CatGuitar, true
	case strings.Contains(lc, "violin"), strings.Contains(lc, "cello"), strings.Contains(lc, "harp"), strings.Contains(lc, "string"):
		return CatStrings, true
	case strings.Contains(lc, "piano"), strings.Contains(lc, "organ"), strings.Contains(lc, "epiano"), strings.Contains(lc, "keys"):
		return CatKeys, true
	case strings.Contains(lc, "flute"), strings.Contains(lc, "oboe"), strings.Contains(lc, "sax"), strings.Contains(lc, "clarinet"):
		return CatWoodwind, true
	case strings.Contains(lc, "trumpet"), strings.Contains(lc, "horn"), strings.Contains(lc, "brass"), strings.Contains(lc, "trombone"):
		return CatBrass, true
	case strings.Contains(lc, "lead"), strings.Contains(lc, "pluck"), strings.Contains(lc, "bell"):
		return CatLead, true
	}
	return "", false
}

// categoryByRole maps a kit role (from roleHeuristic) to a category.
func categoryByRole(role string) (CategoryID, bool) {
	switch role {
	case "kick":
		return CatKick, true
	case "snare":
		return CatSnare, true
	case "hat":
		return CatHiHat, true
	case "tom":
		return CatTom, true
	case "ride", "crash":
		return CatCymbal, true
	case "clap", "cowbell", "shaker":
		return CatPercussion, true
	case "bass":
		return CatBass, true
	}
	return "", false
}
```

- [ ] **Step 4: Run the category tests**

Run: `cd src/go && ../../.tools/go/bin/gofmt -w internal/audio/category.go internal/audio/category_test.go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/ -run 'TestCategory|TestInstrumentsByCategory|TestAssignCategory' -v`
Expected: PASS. If a builtin id maps to the wrong category, adjust the substring ordering in `categoryByText` (do not special-case individual ids unless unavoidable).

- [ ] **Step 5: Run the audio suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/`
Expected: PASS. Leave uncommitted.

---

### Task 6: UI picker reads the category API (retire folder-derived categories)

**Files:**
- Modify: `src/go/internal/ui/drumview_instruments.go:24-77` (the `instCatByID` / `instCategories` construction in `refreshInstruments`)
- Modify: `src/go/internal/ui/drumview.go:29` (`fallbackInstCategory` — remove if now unused)
- Test: `src/go/internal/ui/instrument_category_picker_test.go` (create)

**Interfaces:**
- Consumes: `audio.CategoryOf(id)`, `audio.Categories()`.
- Produces: `dv.instCatByID[id]` is the canonical category display name; `dv.instCategories` is the canonical taxonomy (only categories that have ≥1 instrument).

- [ ] **Step 1: Write the failing test**

`src/go/internal/ui/instrument_category_picker_test.go`:

```go
package ui

import "testing"

func TestPickerCategoriesFromCanonicalTaxonomy(t *testing.T) {
	dv := newDrumViewForTest(t)
	dv.instRefreshDirty = true
	dv.refreshInstruments()
	// kick must be classified under the canonical "Kick" category, not a
	// folder-derived or "Registered" fallback bucket.
	if got := dv.instCatByID["kick"]; got != "Kick" {
		t.Fatalf("kick category = %q, want %q", got, "Kick")
	}
	// The fallback "Registered" bucket must no longer appear.
	for _, c := range dv.instCategories {
		if c == "Registered" {
			t.Fatalf("stale fallback category 'Registered' still present: %v", dv.instCategories)
		}
	}
}
```

Use the same `newDrumViewForTest` helper resolved in Task 3.

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestPickerCategoriesFromCanonicalTaxonomy`
Expected: FAIL — `kick` maps to folder/“Registered”, not "Kick".

- [ ] **Step 3: Rewrite the category construction in `refreshInstruments`**

In `src/go/internal/ui/drumview_instruments.go`, replace the block that builds `dv.instCatByID` / `cats` / fallback (lines 24-77, from `meta := audio.Catalog()` through the `slices.Sort(cats)` / `dv.instCategories = cats` assignment) with:

```go
	meta := audio.Catalog()
	dv.instMeta = map[string]audio.SoundMeta{}
	for _, m := range meta {
		dv.instMeta[m.ID] = m
	}

	seen := map[string]bool{}
	opts := []string{}
	for _, id := range audio.Instruments() {
		if !seen[id] {
			opts = append(opts, id)
			seen[id] = true
		}
	}
	for _, m := range meta {
		if !seen[m.ID] {
			opts = append(opts, m.ID)
			seen[m.ID] = true
		}
	}

	// Canonical category per instrument, plus the ordered taxonomy filtered to
	// categories that actually have at least one instrument.
	dv.instCatByID = map[string]string{}
	catNameByID := map[audio.CategoryID]string{}
	for _, c := range audio.Categories() {
		catNameByID[c.ID] = c.Name
	}
	usedCats := map[audio.CategoryID]bool{}
	for _, id := range opts {
		cid := audio.CategoryOf(id)
		dv.instCatByID[id] = catNameByID[cid]
		usedCats[cid] = true
	}
	cats := []string{}
	for _, c := range audio.Categories() {
		if usedCats[c.ID] {
			cats = append(cats, c.Name)
		}
	}
	dv.instCategories = cats
	if len(dv.instCategories) > 0 && dv.instMenuActiveCat == "" {
		dv.instMenuActiveCat = dv.instCategories[0]
	}
```

This drops the `CatalogCategories()` call, the `fallbackCat` logic, and the `slices.Sort(cats)` (taxonomy order is canonical, not alphabetical). If `slices` becomes unused in the file after this change, leave it — it is still used later in `refreshInstruments` for `slices.Sort(missing)` and `slices.Equal`.

- [ ] **Step 4: Remove the now-dead fallback constant**

In `src/go/internal/ui/drumview.go`, remove the `fallbackInstCategory = "Registered"` declaration (line 29) if grep shows no remaining references:

Run: `cd src/go && grep -rn "fallbackInstCategory" internal/ui/`
Expected after edit: no matches → safe to delete the const.

- [ ] **Step 5: Run picker test + ui suite**

Run: `cd src/go && ../../.tools/go/bin/gofmt -w internal/ui/drumview_instruments.go internal/ui/drumview.go internal/ui/instrument_category_picker_test.go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestPickerCategoriesFromCanonicalTaxonomy -v`
Expected: PASS. Then run the full `./internal/ui/` suite; fix or update any category-related test that asserted the old folder/“Registered” buckets. Leave uncommitted.

---

### Task 7: Duplicate-ID template migration + base-ID recipe fallback

**Files:**
- Modify: `src/go/internal/audio/synth_recipe.go:593` (`factoryRecipeForInstrument` — add base-ID fallback)
- Modify: `src/go/cmd/gen-showcase/main.go:179-183` (auto-suffix duplicate IDs in the instruments loop)
- Regenerate: `src/go/internal/assets/templates/*.json` via the gen-showcase build
- Test: `src/go/internal/audio/recipe_base_id_fallback_test.go` (create); update `src/go/cmd/gen-showcase/drift_test.go` expectations if it pins instrument IDs

**Interfaces:**
- Consumes: `baseInstrumentID(id)` (existing, strips `-1`/`-2`), `builtinInstrumentRecipeBindings`.
- Produces: `factoryRecipeForInstrument("organ-2") == factoryRecipeForInstrument("organ")`; gen-showcase emits distinct `-N` IDs for repeated spec IDs.

- [ ] **Step 1: Write the failing test (recipe fallback)**

`src/go/internal/audio/recipe_base_id_fallback_test.go`:

```go
package audio

import "testing"

func TestFactoryRecipeBaseIDFallback(t *testing.T) {
	base := factoryRecipeForInstrument("organ")
	if base == "" {
		t.Fatalf("precondition: 'organ' has no factory recipe")
	}
	if got := factoryRecipeForInstrument("organ-2"); got != base {
		t.Fatalf("organ-2 recipe = %q, want base %q", got, base)
	}
	// A genuinely unbound id with no base still yields empty.
	if got := factoryRecipeForInstrument("nonsense-xyz"); got != "" {
		t.Fatalf("nonsense-xyz recipe = %q, want empty", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/ -run TestFactoryRecipeBaseIDFallback`
Expected: FAIL — `factoryRecipeForInstrument("organ-2")` returns "".

- [ ] **Step 3: Add the base-ID fallback**

In `src/go/internal/audio/synth_recipe.go`, replace `factoryRecipeForInstrument` (line 593-595) with:

```go
func factoryRecipeForInstrument(instID string) string {
	if r, ok := builtinInstrumentRecipeBindings[instID]; ok {
		return r
	}
	// Instance variants ("organ-2") resolve to their base instrument's recipe
	// so a duplicated voice renders identically without its own binding entry.
	if base := baseInstrumentID(instID); base != instID {
		return builtinInstrumentRecipeBindings[base]
	}
	return ""
}
```

Note `baseInstrumentID` lives in `synth_recipe_dispatch.go` (same package) — no import needed. It strips `-1`/`-2`; the six duplicate templates each repeat an ID exactly twice, so `-2` is the only suffix produced.

- [ ] **Step 4: Run the fallback test**

Run: `cd src/go && ../../.tools/go/bin/gofmt -w internal/audio/synth_recipe.go internal/audio/recipe_base_id_fallback_test.go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/ -run TestFactoryRecipeBaseIDFallback -v`
Expected: PASS.

- [ ] **Step 5: Auto-suffix duplicate IDs in gen-showcase**

In `src/go/cmd/gen-showcase/main.go`, inside `build`, before the per-row loop that appends instruments, track seen IDs and suffix repeats. Add a `map[string]int` declared just before the `for ri := …` row loop (find the loop that sets `ins := s.Insts[ri]`):

```go
	idSeen := map[string]int{}
```

Then, immediately after `ins := s.Insts[ri]` (line 158), compute the instance ID:

```go
		instID := ins.ID
		idSeen[ins.ID]++
		if n := idSeen[ins.ID]; n > 1 {
			instID = fmt.Sprintf("%s-%d", ins.ID, n)
		}
```

In the `d.Instruments = append(...)` literal (line 179-183), change `ID: ins.ID` to `ID: instID`. Leave `seededModularInstruments[ins.ID]` keyed by the base `ins.ID` (the seed-trigger decision is per base instrument). Add `"fmt"` to imports if not present (it is already used elsewhere in main.go — verify).

- [ ] **Step 6: Regenerate templates and verify duplicates are gone**

Run: `cd src/go && ../../.tools/go/bin/go run ./cmd/gen-showcase`
Then confirm the six files now carry distinct IDs:

Run: `cd src/go && for f in bach-toccata blackbox-ride-on-time asturias eagles-hotel-california puente-oye-como-va wonder-superstition; do echo "== $f =="; ../../.tools/go/bin/go run ./cmd/gen-showcase >/dev/null; grep '"id"' internal/assets/templates/$f.json | sort | uniq -d; done`
Expected: no duplicate `"id"` lines printed for any file (the `uniq -d` output is empty). The second voice now shows e.g. `"organ-2"`.

- [ ] **Step 7: Run gen-showcase drift + fidelity + audio + ui suites**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./cmd/gen-showcase/ ./internal/audio/ ./internal/ui/`
Expected: PASS. If `drift_test.go` or `musical_fidelity_test.go` pins the old duplicate IDs, update the expectations to the new `-N` instance IDs (the audio output is unchanged — only the identity string differs). Leave uncommitted.

---

### Task 8: Full-suite verification

**Files:** none (verification only).

- [ ] **Step 1: Run the entire fast Go suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...`
Expected: PASS across all packages.

- [ ] **Step 2: Build the WASM target to catch js-tag breakage from the `RenameInstrument` deletion**

Run: `cd /home/ymolinar/Repos/beatmo && make wasm`
Expected: builds cleanly (the `js` build no longer references the deleted `RenameInstrument`).

- [ ] **Step 3: Bridge smoke (rename + instrument exports still callable)**

Run: `cd /home/ymolinar/Repos/beatmo && GO=$(pwd)/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js`
Expected: PASS (or SKIP if no GPU — note the env limitation from project memory; do not treat a GPU-less crash as a code failure).

- [ ] **Step 4: Report**

Summarize: tests green, WASM builds, rename is metadata-only, category API live, duplicate-IDs migrated. Leave everything uncommitted for user review.
```
