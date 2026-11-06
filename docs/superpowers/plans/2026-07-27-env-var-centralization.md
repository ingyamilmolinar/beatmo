# Environment Variable Centralization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Route every environment variable the Beatmo game reads through a single documented `internal/env` table, make env vars work on WASM, and remove/rename the dead and collision-prone names.

**Architecture:** A new leaf package `internal/env` declares one `Var` per environment variable with name, type, default, scope, and purpose. `Var.Get()` reads an atomic cache resolved on first access, so per-frame readers stay allocation- and syscall-free. WASM support comes from populating `go.env` in the JS bootstrap rather than forking the Go lookup path — `wasm_exec.js` already serializes `this.env` into linear memory for `goenvs_unix`. Drift is caught by four discipline tests in the fast `-tags test` path.

**Tech Stack:** Go 1.26 (`.tools/go/bin/go`), `-tags test -modfile=go.test.mod` fast path, Playwright browser tests, `sync/atomic`.

## Global Constraints

- **Do NOT `git add`, `git stage`, or `git commit` at any point.** This overrides the commit steps the plan template normally carries. The working tree already has 523 modified files from unrelated work; leave staging entirely to the user. Every task ends with a **verification** step instead of a commit.
- **Do not touch unrelated modified files.** The branch `add-core-node-types` has a large in-flight diff. Only edit files this plan names.
- Use `.tools/go/bin/go` for every Go invocation — never system `go`.
- Fast test command: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...`
- `internal/env` imports **only the standard library**. It must not import `internal/log` — `log` imports `env`.
- Boolean truthy set is exactly `1, true, yes, on` (case-insensitive); falsy set is `0, false, no, off`; anything else falls back to the declared default.
- Every `Var` declaration carries a non-empty purpose string. This is enforced by a test.
- No deprecation aliases for renamed or removed variables — clean break, per the approved spec.
- Spec of record: `docs/superpowers/specs/2026-07-27-env-var-centralization-design.md`.

---

## File Structure

**Created:**
- `src/go/internal/env/env.go` — `Var`, typed constructors, resolution, cache, `Reload`, `All`, `SetForTest`.
- `src/go/internal/env/vars.go` — the table: 91 declarations, grouped by scope with section comments.
- `src/go/internal/env/env_test.go` — unit tests for parsing, caching, `Reload`, `SetForTest`.
- `src/go/internal/env/discipline_test.go` — the four drift guards.
- `src/js/wasm_env.browser.test.js` — proves `__beatmoEnv` reaches Go's `os.Getenv`.

**Modified (call-site migration):**
- `src/go/internal/audio/` — `engine_stop.go`, `channels.go`, `export.go`, `engine.go`, `catalog_desktop.go`, `catalog_stub.go`, `recording_paths.go`, `synth_recipe_dispatch.go`
- `src/go/internal/ui/` — `game.go`, `game_new.go`, `game_draw_grid_pane.go`, `camera.go`, `widgets.go`, `perf.go`, `demo.go`, `demo_stub.go`, `layout_profile.go`, `drumview_instruments.go`, `heap_growth_probe.go`, `game_parity_state.go`, `game_parity_timing.go`, `game_graph_update_beat_infos.go`, `game_sequencer_loop.go`, `parity_scan_config.go`, `runtime_profile_js.go`, `runtime_profile_notjs.go`
- `src/go/cmd/` — `beatmo.go`, `profiling.go`, `pyroscope.go`, `beatmo_record_bench.go`
- `src/go/internal/` — `async/runtime.go`, `log/log.go`, `wavref/wavref.go`
- `src/js/index.html:842`
- `CLAUDE.md` — regenerated env tables

**Deleted:**
- `src/go/internal/bench/env.go` (its only export `EnvInt` is superseded)

---

### Task 1: `internal/env` core — Var, parsing, cache, test override

**Files:**
- Create: `src/go/internal/env/env.go`
- Test: `src/go/internal/env/env_test.go`

**Interfaces:**
- Consumes: nothing (leaf package).
- Produces:
  - `type Scope int` with `ScopeRuntime`, `ScopeDebug`, `ScopeTest`
  - `type Var interface { Name() string; Purpose() string; Scope() Scope; DefaultString() string; TypeName() string; IsSet() bool; reload() }`
  - `func Str(name, def string, s Scope, purpose string) *StrVar`
  - `func Bool(name string, def bool, s Scope, purpose string) *BoolVar`
  - `func Int(name string, def int, s Scope, purpose string) *IntVar`
  - `func Float(name string, def float64, s Scope, purpose string) *FloatVar`
  - `func Dur(name string, def time.Duration, s Scope, purpose string) *DurVar`
  - Each `*XVar` has `Get() X` and `SetForTest(v X) func()`
  - `func Reload()` — re-resolve every registered Var
  - `func All() []Var` — every registered Var, sorted by name

- [ ] **Step 1: Write the failing tests**

Create `src/go/internal/env/env_test.go`:

```go
package env

import (
	"os"
	"testing"
	"time"
)

func TestBoolTruthySet(t *testing.T) {
	v := Bool("BEATMO_TEST_BOOL", false, ScopeTest, "unit test var")
	for _, s := range []string{"1", "true", "TRUE", "True", "yes", "on", "ON"} {
		t.Setenv("BEATMO_TEST_BOOL", s)
		Reload()
		if !v.Get() {
			t.Errorf("Get()=false for %q, want true", s)
		}
	}
	for _, s := range []string{"0", "false", "FALSE", "no", "off"} {
		t.Setenv("BEATMO_TEST_BOOL", s)
		Reload()
		if v.Get() {
			t.Errorf("Get()=true for %q, want false", s)
		}
	}
}

func TestBoolGarbageFallsBackToDefault(t *testing.T) {
	v := Bool("BEATMO_TEST_BOOL_DEF", true, ScopeTest, "unit test var")
	t.Setenv("BEATMO_TEST_BOOL_DEF", "banana")
	Reload()
	if !v.Get() {
		t.Errorf("Get()=false for garbage value, want default true")
	}
}

func TestUnsetUsesDefault(t *testing.T) {
	v := Int("BEATMO_TEST_INT_UNSET", 42, ScopeTest, "unit test var")
	_ = os.Unsetenv("BEATMO_TEST_INT_UNSET")
	Reload()
	if got := v.Get(); got != 42 {
		t.Errorf("Get()=%d, want 42", got)
	}
}

func TestIntParseErrorFallsBackToDefault(t *testing.T) {
	v := Int("BEATMO_TEST_INT_BAD", 7, ScopeTest, "unit test var")
	t.Setenv("BEATMO_TEST_INT_BAD", "not-a-number")
	Reload()
	if got := v.Get(); got != 7 {
		t.Errorf("Get()=%d, want default 7", got)
	}
}

func TestFloatAndDur(t *testing.T) {
	f := Float("BEATMO_TEST_FLOAT", 1.5, ScopeTest, "unit test var")
	t.Setenv("BEATMO_TEST_FLOAT", "2.25")
	d := Dur("BEATMO_TEST_DUR_MS", 100*time.Millisecond, ScopeTest, "unit test var; value in milliseconds")
	t.Setenv("BEATMO_TEST_DUR_MS", "250")
	Reload()
	if got := f.Get(); got != 2.25 {
		t.Errorf("Float Get()=%v, want 2.25", got)
	}
	if got := d.Get(); got != 250*time.Millisecond {
		t.Errorf("Dur Get()=%v, want 250ms", got)
	}
}

func TestSetForTestOverridesAndRestores(t *testing.T) {
	v := Bool("BEATMO_TEST_OVERRIDE", false, ScopeTest, "unit test var")
	Reload()
	if v.Get() {
		t.Fatalf("precondition: want false")
	}
	restore := v.SetForTest(true)
	if !v.Get() {
		t.Errorf("Get()=false after SetForTest(true)")
	}
	restore()
	if v.Get() {
		t.Errorf("Get()=true after restore, want false")
	}
}

func TestSetForTestSurvivesReload(t *testing.T) {
	v := Bool("BEATMO_TEST_OVERRIDE_RELOAD", false, ScopeTest, "unit test var")
	Reload()
	restore := v.SetForTest(true)
	defer restore()
	Reload()
	if !v.Get() {
		t.Errorf("Get()=false after Reload(); a test override must survive Reload")
	}
}

func TestIsSetDistinguishesAbsentFromDefaultValued(t *testing.T) {
	v := Int("BEATMO_TEST_ISSET", 5, ScopeTest, "unit test var")

	_ = os.Unsetenv("BEATMO_TEST_ISSET")
	Reload()
	if v.IsSet() {
		t.Errorf("IsSet()=true when unset, want false")
	}

	// A value equal to the default is still explicitly set.
	t.Setenv("BEATMO_TEST_ISSET", "5")
	Reload()
	if !v.IsSet() {
		t.Errorf("IsSet()=false for an explicit value equal to the default, want true")
	}

	// A present-but-unparseable value counts as unset so callers guarding on
	// IsSet do not write a fallback over a builder-supplied value.
	t.Setenv("BEATMO_TEST_ISSET", "banana")
	Reload()
	if v.IsSet() {
		t.Errorf("IsSet()=true for an unparseable value, want false")
	}
	if got := v.Get(); got != 5 {
		t.Errorf("Get()=%d for an unparseable value, want default 5", got)
	}
}

func TestAllIsSortedAndContainsRegistered(t *testing.T) {
	Str("BEATMO_TEST_ZZZ", "", ScopeTest, "unit test var")
	Str("BEATMO_TEST_AAA", "", ScopeTest, "unit test var")
	all := All()
	var prev string
	found := map[string]bool{}
	for _, v := range all {
		if prev != "" && v.Name() < prev {
			t.Fatalf("All() not sorted: %q after %q", v.Name(), prev)
		}
		prev = v.Name()
		found[v.Name()] = true
	}
	for _, want := range []string{"BEATMO_TEST_AAA", "BEATMO_TEST_ZZZ"} {
		if !found[want] {
			t.Errorf("All() missing %q", want)
		}
	}
}

func TestDuplicateNamePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Errorf("registering a duplicate name did not panic")
		}
	}()
	Str("BEATMO_TEST_DUP", "", ScopeTest, "unit test var")
	Str("BEATMO_TEST_DUP", "", ScopeTest, "unit test var")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/env/`
Expected: FAIL — build error, `undefined: Bool`, `undefined: Reload`, etc.

- [ ] **Step 3: Write the implementation**

Create `src/go/internal/env/env.go`:

```go
// Package env is the single source of truth for every environment variable
// the Beatmo game reads.
//
// Every variable is declared exactly once in vars.go with a name, type,
// default, scope and purpose. Nothing outside this package calls os.Getenv —
// TestNoDirectGetenv in discipline_test.go enforces that.
//
// # Why a registry
//
// Before this package the tree held 99 os.Getenv call sites across 46 files
// reading 95 distinct names, with five duplicated parse helpers and four
// mutually inconsistent boolean dialects. Nothing knew the full set, so a
// hand-maintained reset list in internal/ui had rotted in both directions —
// two names with no reader, eighteen live names missing.
//
// # Caching
//
// Get() reads an atomically-held value resolved on first access rather than
// calling os.Getenv per read. Several variables (NO_PIXEL_SNAP, RENDER_SAFE,
// the audio bypasses) are read inside draw and mixing loops, and were
// previously cached in package-level vars for exactly that reason.
//
// Reload() re-resolves every variable. Production calls it once at startup;
// tests call it after os.Setenv. A SetForTest override deliberately survives
// Reload so a test can force a value without touching the environment.
//
// # WASM
//
// os.Getenv works in the browser because the JS bootstrap populates go.env
// before go.run() (src/js/index.html). wasm_exec.js serializes that object
// into linear memory for goenvs_unix, so this package needs no build-tagged
// lookup path and behaves identically on both platforms.
package env

import (
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Scope classifies why a variable exists. It drives the generated
// documentation tables and the naming guard.
type Scope int

const (
	// ScopeRuntime is user-facing configuration: asset paths, sample rate,
	// output directories.
	ScopeRuntime Scope = iota
	// ScopeDebug is a developer diagnostic. Shipped, off by default.
	ScopeDebug
	// ScopeTest is read only from _test.go files and never by the shipping
	// binary.
	ScopeTest
)

func (s Scope) String() string {
	switch s {
	case ScopeRuntime:
		return "runtime"
	case ScopeDebug:
		return "debug"
	case ScopeTest:
		return "test"
	}
	return "unknown"
}

// Var is the read-only view of a registered variable, used by All() and the
// discipline tests.
type Var interface {
	Name() string
	Purpose() string
	Scope() Scope
	DefaultString() string
	TypeName() string
	// IsSet reports whether the variable was present and non-empty in the
	// environment. Callers that must not clobber an existing value when the
	// variable is absent guard on this — applyEnvProfileOverride is the
	// canonical case, replacing the old (value, ok) helpers.
	IsSet() bool
	reload()
}

var (
	registryMu sync.Mutex
	registry   = map[string]Var{}
)

func register(v Var) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := registry[v.Name()]; dup {
		panic("env: duplicate variable name " + v.Name())
	}
	registry[v.Name()] = v
	v.reload()
}

// All returns every registered variable, sorted by name.
func All() []Var {
	registryMu.Lock()
	out := make([]Var, 0, len(registry))
	for _, v := range registry {
		out = append(out, v)
	}
	registryMu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// Reload re-resolves every registered variable from the environment.
// Variables under a SetForTest override keep the overridden value.
func Reload() {
	for _, v := range All() {
		v.reload()
	}
}

// meta is the shared header every typed Var embeds.
type meta struct {
	name     string
	purpose  string
	scope    Scope
	typeName string
	defStr   string
	// overridden is set by SetForTest and makes reload() a no-op.
	overridden atomic.Bool
	// isSet records whether the last resolve found a present, non-empty value.
	isSet atomic.Bool
}

func (m *meta) Name() string          { return m.name }
func (m *meta) Purpose() string       { return m.purpose }
func (m *meta) Scope() Scope          { return m.scope }
func (m *meta) DefaultString() string { return m.defStr }
func (m *meta) TypeName() string      { return m.typeName }

// IsSet reports whether the variable was present and non-empty in the
// environment. SetForTest marks the variable set, so a forced value behaves
// like an explicitly-provided one.
func (m *meta) IsSet() bool { return m.isSet.Load() }

func newMeta(name, purpose string, s Scope, typeName, defStr string) meta {
	if name == "" {
		panic("env: variable declared with an empty name")
	}
	if purpose == "" {
		panic("env: variable " + name + " declared with an empty purpose")
	}
	return meta{name: name, purpose: purpose, scope: s, typeName: typeName, defStr: defStr}
}

// raw returns the trimmed environment value and whether it was present and
// non-empty. An empty value is treated as unset throughout, matching the
// behavior of every call site this package replaced.
func raw(name string) (string, bool) {
	s := strings.TrimSpace(os.Getenv(name))
	return s, s != ""
}

// ---- string ---------------------------------------------------------------

// StrVar is a string-valued variable.
type StrVar struct {
	meta
	def string
	val atomic.Value // string
}

// Str declares a string variable.
func Str(name, def string, s Scope, purpose string) *StrVar {
	v := &StrVar{meta: newMeta(name, purpose, s, "string", def), def: def}
	register(v)
	return v
}

// Get returns the resolved value.
func (v *StrVar) Get() string {
	s, _ := v.val.Load().(string)
	return s
}

func (v *StrVar) reload() {
	if v.overridden.Load() {
		return
	}
	s, ok := raw(v.name)
	v.isSet.Store(ok)
	if ok {
		v.val.Store(s)
		return
	}
	v.val.Store(v.def)
}

// SetForTest forces a value until the returned func is called. The override
// survives Reload and marks the variable IsSet.
func (v *StrVar) SetForTest(s string) func() {
	prev, prevOverridden, prevSet := v.Get(), v.overridden.Swap(true), v.isSet.Swap(true)
	v.val.Store(s)
	return func() {
		v.val.Store(prev)
		v.overridden.Store(prevOverridden)
		v.isSet.Store(prevSet)
	}
}

// ---- bool -----------------------------------------------------------------

// BoolVar is a boolean-valued variable.
type BoolVar struct {
	meta
	def bool
	val atomic.Bool
}

// Bool declares a boolean variable. Truthy values are 1, true, yes, on;
// falsy are 0, false, no, off; anything else falls back to def.
func Bool(name string, def bool, s Scope, purpose string) *BoolVar {
	v := &BoolVar{meta: newMeta(name, purpose, s, "bool", strconv.FormatBool(def)), def: def}
	register(v)
	return v
}

// Get returns the resolved value.
func (v *BoolVar) Get() bool { return v.val.Load() }

func (v *BoolVar) reload() {
	if v.overridden.Load() {
		return
	}
	s, ok := raw(v.name)
	v.isSet.Store(ok)
	v.val.Store(parseBoolValue(s, ok, v.def))
}

// parseBoolValue applies the one truthy/falsy dialect used everywhere. A
// present-but-unrecognized value falls back to def; before centralization
// four mutually inconsistent dialects were in use.
func parseBoolValue(s string, present, def bool) bool {
	if !present {
		return def
	}
	switch strings.ToLower(s) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

// SetForTest forces a value until the returned func is called. The override
// survives Reload and marks the variable IsSet.
func (v *BoolVar) SetForTest(b bool) func() {
	prev, prevOverridden, prevSet := v.val.Load(), v.overridden.Swap(true), v.isSet.Swap(true)
	v.val.Store(b)
	return func() {
		v.val.Store(prev)
		v.overridden.Store(prevOverridden)
		v.isSet.Store(prevSet)
	}
}

// ---- int ------------------------------------------------------------------

// IntVar is an integer-valued variable.
type IntVar struct {
	meta
	def int
	val atomic.Int64
}

// Int declares an integer variable. A parse error falls back to def.
func Int(name string, def int, s Scope, purpose string) *IntVar {
	v := &IntVar{meta: newMeta(name, purpose, s, "int", strconv.Itoa(def)), def: def}
	register(v)
	return v
}

// Get returns the resolved value.
func (v *IntVar) Get() int { return int(v.val.Load()) }

func (v *IntVar) reload() {
	if v.overridden.Load() {
		return
	}
	if s, ok := raw(v.name); ok {
		if n, err := strconv.Atoi(s); err == nil {
			v.isSet.Store(true)
			v.val.Store(int64(n))
			return
		}
	}
	// A present-but-unparseable value counts as unset: callers guarding on
	// IsSet must not act on garbage.
	v.isSet.Store(false)
	v.val.Store(int64(v.def))
}

// SetForTest forces a value until the returned func is called. The override
// survives Reload and marks the variable IsSet.
func (v *IntVar) SetForTest(n int) func() {
	prev, prevOverridden, prevSet := v.val.Load(), v.overridden.Swap(true), v.isSet.Swap(true)
	v.val.Store(int64(n))
	return func() {
		v.val.Store(prev)
		v.overridden.Store(prevOverridden)
		v.isSet.Store(prevSet)
	}
}

// ---- float ----------------------------------------------------------------

// FloatVar is a float-valued variable.
type FloatVar struct {
	meta
	def float64
	val atomic.Value // float64
}

// Float declares a float variable. A parse error falls back to def.
func Float(name string, def float64, s Scope, purpose string) *FloatVar {
	v := &FloatVar{
		meta: newMeta(name, purpose, s, "float", strconv.FormatFloat(def, 'g', -1, 64)),
		def:  def,
	}
	register(v)
	return v
}

// Get returns the resolved value.
func (v *FloatVar) Get() float64 {
	f, _ := v.val.Load().(float64)
	return f
}

func (v *FloatVar) reload() {
	if v.overridden.Load() {
		return
	}
	if s, ok := raw(v.name); ok {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			v.isSet.Store(true)
			v.val.Store(f)
			return
		}
	}
	v.isSet.Store(false)
	v.val.Store(v.def)
}

// SetForTest forces a value until the returned func is called. The override
// survives Reload and marks the variable IsSet.
func (v *FloatVar) SetForTest(f float64) func() {
	prev, prevOverridden, prevSet := v.Get(), v.overridden.Swap(true), v.isSet.Swap(true)
	v.val.Store(f)
	return func() {
		v.val.Store(prev)
		v.overridden.Store(prevOverridden)
		v.isSet.Store(prevSet)
	}
}

// ---- duration -------------------------------------------------------------

// DurVar is a duration-valued variable. The environment value is read as an
// integer count of milliseconds, matching every existing *_MS variable.
type DurVar struct {
	meta
	def time.Duration
	val atomic.Int64
}

// Dur declares a duration variable whose environment value is an integer
// count of milliseconds.
func Dur(name string, def time.Duration, s Scope, purpose string) *DurVar {
	v := &DurVar{
		meta: newMeta(name, purpose, s, "duration-ms", strconv.FormatInt(def.Milliseconds(), 10)),
		def:  def,
	}
	register(v)
	return v
}

// Get returns the resolved value.
func (v *DurVar) Get() time.Duration { return time.Duration(v.val.Load()) }

func (v *DurVar) reload() {
	if v.overridden.Load() {
		return
	}
	if s, ok := raw(v.name); ok {
		if n, err := strconv.Atoi(s); err == nil {
			v.isSet.Store(true)
			v.val.Store(int64(time.Duration(n) * time.Millisecond))
			return
		}
	}
	v.isSet.Store(false)
	v.val.Store(int64(v.def))
}

// SetForTest forces a value until the returned func is called. The override
// survives Reload and marks the variable IsSet.
func (v *DurVar) SetForTest(d time.Duration) func() {
	prev, prevOverridden, prevSet := v.val.Load(), v.overridden.Swap(true), v.isSet.Swap(true)
	v.val.Store(int64(d))
	return func() {
		v.val.Store(prev)
		v.overridden.Store(prevOverridden)
		v.isSet.Store(prevSet)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/env/ -v`
Expected: PASS, all 9 tests.

- [ ] **Step 5: Verify (no commit)**

Run: `cd src/go && ../../.tools/go/bin/go vet -tags test -modfile=go.test.mod ./internal/env/`
Expected: clean. Do NOT `git add` or `git commit`.

---

### Task 2: WASM env plumbing

**Files:**
- Modify: `src/js/index.html:842`
- Create: `src/js/wasm_env.browser.test.js`

**Interfaces:**
- Consumes: `internal/env` from Task 1 (indirectly — this task proves `os.Getenv` works in the browser).
- Produces: the `window.__beatmoEnv` contract — a flat object of string→string set before the WASM module runs.

- [ ] **Step 1: Write the failing browser test**

Create `src/js/wasm_env.browser.test.js`. Model the harness setup on an existing minimal suite such as `webaudio_smoke.browser.test.js`; the assertion body is:

```js
// Proves window.__beatmoEnv reaches Go's os.Getenv in the browser.
//
// Before this, src/js/wasm_exec.js constructed the bridge with `this.env = {}`
// and nothing ever assigned to it, so every os.Getenv in the WASM build
// returned "". That silently disabled ~95 knobs, including PARITY_WASM_FATAL
// and PARITY_TIMING_FATAL, which exist only to be set on WASM.
await page.addInitScript(() => {
  window.__beatmoEnv = { BEATMO_WASM_ENV_PROBE: "hello-from-js" };
});

// ... standard WASM boot + wait for exports ...

const got = await page.evaluate(() => window.envProbe("BEATMO_WASM_ENV_PROBE"));
if (got !== "hello-from-js") {
  throw new Error(`envProbe returned ${JSON.stringify(got)}, want "hello-from-js"`);
}

const missing = await page.evaluate(() => window.envProbe("BEATMO_WASM_ENV_ABSENT"));
if (missing !== "") {
  throw new Error(`absent var returned ${JSON.stringify(missing)}, want ""`);
}
```

The test needs a Go-side probe export. Add to the WASM bridge (alongside the
other debug exports registered in `internal/ui/js_exports_*.go`):

```go
// envProbe returns os.Getenv(name). Debug-only export used by
// wasm_env.browser.test.js to prove window.__beatmoEnv reaches Go.
js.Global().Set("envProbe", js.FuncOf(func(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return ""
	}
	return os.Getenv(args[0].String())
}))
```

Register `envProbe` in the catalogue in `src/js/wasm_bridge_smoke.browser.test.js`
and in `internal/ui/js_exports_catalogue_drift_test.go`, per the JS Test Charter.

Note: this probe is the one sanctioned `os.Getenv` outside `internal/env`.
Add it to the exemption list in `TestNoDirectGetenv` (Task 8).

- [ ] **Step 2: Run the test to verify it fails**

Run: `GO=$(pwd)/.tools/go/bin/go node src/js/wasm_env.browser.test.js`
Expected: FAIL — `envProbe returned "" , want "hello-from-js"`, because `go.env` is still empty.

- [ ] **Step 3: Populate `go.env` in the bootstrap**

In `src/js/index.html`, at line 842:

```js
    const go = new Go(); // from wasm_exec.js
    // Env vars for the Go side. wasm_exec.js serializes this object into
    // linear memory as KEY=value pairs at run(), which is exactly what Go's
    // goenvs_unix reads — so os.Getenv works in the browser once this is
    // populated. Without it the object stays {} and every os.Getenv returns "".
    go.env = { ...(globalThis.__beatmoEnv || {}) };
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `GO=$(pwd)/.tools/go/bin/go node src/js/wasm_env.browser.test.js`
Expected: PASS.

- [ ] **Step 5: Verify boot is unaffected (no commit)**

Run:
```bash
GO=$(pwd)/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js
GO=$(pwd)/.tools/go/bin/go node src/js/wasm_load_startup.browser.test.js
```
Expected: both PASS. The startup test guards the boot waterfall and transfer-size budget; `go.env` adds a handful of bytes to linear memory, not to the wire. Do NOT commit.

---

### Task 3: Declare the table

**Files:**
- Create: `src/go/internal/env/vars.go`
- Test: `src/go/internal/env/vars_test.go`

**Interfaces:**
- Consumes: constructors from Task 1.
- Produces: 91 exported `*XVar` values. Later tasks reference them by the Go
  identifiers listed in the mapping table below.

The full name→identifier→type→default mapping is the inventory in
`docs/superpowers/specs/2026-07-27-env-var-centralization-design.md`
§Complete inventory. Transcribe every row whose Action is `keep` or `rename`,
using the **new** name for renamed rows, plus the new `BEATMO_HEAP_DIAG`.
Skip every row whose Action is `remove`.

Go identifiers follow the variable name with the `BEATMO_` prefix dropped and
the remainder in UpperCamelCase: `BEATMO_ASSETS` → `AssetsRoot` (as named in
the spec), `BEATMO_BYPASS_EQ` → `BypassEQ`, `PARITY_SCAN_STRIDE` →
`ParityScanStride`, `NO_PIXEL_SNAP` → `NoPixelSnap`.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/env/vars_test.go`:

```go
package env

import "testing"

// TestTableSize pins the registry size so an accidental deletion or a
// duplicate-by-copy-paste is caught. Update deliberately when adding a var.
func TestTableSize(t *testing.T) {
	const want = 91
	if got := len(All()); got != want {
		t.Errorf("len(All())=%d, want %d — update this constant deliberately when adding or removing a variable", got, want)
	}
}

// TestRemovedNamesAreGone guards the six removals in the approved spec.
// A reintroduction would resurrect a name the spec deliberately retired.
func TestRemovedNamesAreGone(t *testing.T) {
	removed := []string{
		"DRAW_FRAMEBUFFER",
		"PREVIEW_DEBUG",
		"BEATMO_CONFIG",
		"SEQ_TICK_MS",
		"PERF_FAST_PATH",
		"BEATMO_HEAP_PROBE",
		"BEATMO_HEAP_WATCH",
	}
	live := map[string]bool{}
	for _, v := range All() {
		live[v.Name()] = true
	}
	for _, name := range removed {
		if live[name] {
			t.Errorf("%q is registered but the spec removed it", name)
		}
	}
}

// TestRenamedNamesUseNewSpelling guards the twelve renames.
func TestRenamedNamesUseNewSpelling(t *testing.T) {
	oldToNew := map[string]string{
		"SWEEP":                  "BEATMO_KICK_SWEEP",
		"WRITE_WAV":              "BEATMO_WRITE_WAV",
		"TEST_TONE":              "BEATMO_TEST_TONE",
		"TEST_VOICE":             "BEATMO_TEST_VOICE",
		"TEST_RAW_VOICE":         "BEATMO_TEST_RAW_VOICE",
		"TEST_RAW_VOICE_FULL":    "BEATMO_TEST_RAW_VOICE_FULL",
		"TEST_RAW_VOICE_NOSCALE": "BEATMO_TEST_RAW_VOICE_NOSCALE",
		"SINGLE_VOICE":           "BEATMO_SINGLE_VOICE",
		"AUDIO_SAMPLE_RATE":      "BEATMO_AUDIO_SAMPLE_RATE",
		"PPROF":                  "BEATMO_PPROF",
		"UPDATE_GOLDEN":          "BEATMO_UPDATE_GOLDEN",
		"RUN_SOAK_REAL":          "BEATMO_RUN_SOAK_REAL",
	}
	live := map[string]bool{}
	for _, v := range All() {
		live[v.Name()] = true
	}
	for old, new := range oldToNew {
		if live[old] {
			t.Errorf("%q is still registered; the spec renamed it to %q", old, new)
		}
		if !live[new] {
			t.Errorf("%q is not registered; the spec renamed %q to it", new, old)
		}
	}
}

// TestHeapDiagLevels pins the three-state replacement for the two retired
// heap booleans.
func TestHeapDiagLevels(t *testing.T) {
	for _, tc := range []struct{ set string; wantWatch, wantProbe bool }{
		{"", false, false},
		{"watch", true, false},
		{"probe", true, true},
		{"garbage", false, false},
	} {
		t.Setenv("BEATMO_HEAP_DIAG", tc.set)
		Reload()
		if got := HeapWatchEnabled(); got != tc.wantWatch {
			t.Errorf("BEATMO_HEAP_DIAG=%q HeapWatchEnabled()=%v, want %v", tc.set, got, tc.wantWatch)
		}
		if got := HeapProbeEnabled(); got != tc.wantProbe {
			t.Errorf("BEATMO_HEAP_DIAG=%q HeapProbeEnabled()=%v, want %v", tc.set, got, tc.wantProbe)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/env/ -run 'TestTableSize|TestRemoved|TestRenamed|TestHeapDiag'`
Expected: FAIL — `len(All())=0, want 91` and `undefined: HeapWatchEnabled`.

- [ ] **Step 3: Write `vars.go`**

Transcribe the spec inventory. The file is grouped with section comments
matching the spec's section headings. Opening excerpt showing the required
shape — every remaining declaration follows it exactly:

```go
package env

import "time"

// ---- Runtime configuration -------------------------------------------------

var (
	// AssetsRoot overrides where instrument samples and templates load from.
	AssetsRoot = Str("BEATMO_ASSETS", "", ScopeRuntime,
		"Override the default assets root directory.")

	DemoConfig = Str("BEATMO_DEMO_CONFIG", "", ScopeRuntime,
		"Path to a JSON circuit loaded as the initial demo.")

	RecordingsDir = Str("BEATMO_RECORDINGS_DIR", "", ScopeRuntime,
		"Base directory for recording output; defaults to $CWD/recordings.")

	AudioSampleRate = Int("BEATMO_AUDIO_SAMPLE_RATE", 44100, ScopeRuntime,
		"Output sample rate in Hz. Was AUDIO_SAMPLE_RATE.")
)

// ---- Audio debug: bypasses -------------------------------------------------

var (
	TestTone = Bool("BEATMO_TEST_TONE", false, ScopeDebug,
		"Output a pure 440 Hz sine, bypassing all synth. Tests the Oto/driver path. Was TEST_TONE.")

	TestVoice = Bool("BEATMO_TEST_VOICE", false, ScopeDebug,
		"Replace the C synth with simple 220 Hz sine voices. Tests the voice/mixer path. Was TEST_VOICE.")

	BypassEQ = Bool("BEATMO_BYPASS_EQ", false, ScopeDebug,
		"Skip all EQ/processor processing. Isolates mixer vs channel bugs.")
)

// ---- Heap diagnostics ------------------------------------------------------

// HeapDiag replaces the retired BEATMO_HEAP_PROBE + BEATMO_HEAP_WATCH pair.
// The two booleans encoded one three-state level: PROBE implied WATCH.
var HeapDiag = Str("BEATMO_HEAP_DIAG", "", ScopeDebug,
	"Heap diagnostics level: 'watch' enables the 30 s heap-growth rollup WARN; "+
		"'probe' additionally enables the per-second allocation-rate probe with "+
		"top contributors by tag. Replaces BEATMO_HEAP_PROBE and BEATMO_HEAP_WATCH.")

// HeapWatchEnabled reports whether the 30 s heap-growth rollup is on.
func HeapWatchEnabled() bool {
	switch HeapDiag.Get() {
	case "watch", "probe":
		return true
	}
	return false
}

// HeapProbeEnabled reports whether the per-second allocation probe is on.
// Probe implies watch, matching the retired BEATMO_HEAP_PROBE semantics.
func HeapProbeEnabled() bool { return HeapDiag.Get() == "probe" }
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/env/ -v`
Expected: PASS. If `TestTableSize` reports a count other than 91, reconcile
against the spec inventory rather than editing the constant — a mismatch means
a row was skipped or duplicated.

- [ ] **Step 5: Verify (no commit)**

Run: `cd src/go && ../../.tools/go/bin/go vet -tags test -modfile=go.test.mod ./internal/env/`
Expected: clean. Do NOT commit.

---

### Task 4: RuntimeProfile unification — retire `__beatmoProfileOverride`

Do this before the `internal/ui` migration: Task 6 routes the removed
`SEQ_TICK_MS` through `RuntimeProf().SequencerTickMS`, which this task makes
available on both platforms.

**Files:**
- Modify: `src/go/internal/ui/runtime_profile_notjs.go`
- Modify: `src/go/internal/ui/runtime_profile_js.go`
- Create: `src/go/internal/ui/runtime_profile_env.go` (no build tags)
- Modify: `src/js/webaudio_bench_startup.browser.test.js`, `src/js/webaudio_perf_e2e.browser.test.js`, `src/js/webaudio_fx_chain_stress.browser.test.js`
- Test: `src/go/internal/ui/runtime_profile_env_test.go`

**Interfaces:**
- Consumes: the 13 RuntimeProfile `*XVar`s from Task 3.
- Produces: `func applyEnvProfileOverride(p *RuntimeProfile)` in
  `runtime_profile_env.go`, callable from both build-tag variants.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/runtime_profile_env_test.go`:

```go
package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/env"
)

func TestApplyEnvProfileOverrideAppliesEveryField(t *testing.T) {
	restores := []func(){
		env.EdgeCachePad.SetForTest(11),
		env.GridCachePad.SetForTest(12),
		env.DisableNodeGlow.SetForTest(true),
		env.DisableEdgeArrows.SetForTest(true),
		env.SimpleDrawDefault.SetForTest(true),
		env.TimelineInfoThrottleMS.SetForTest(13),
		env.AudioLookaheadSec.SetForTest(0.25),
		env.AudioBatchMax.SetForTest(14),
		env.SequencerTickMS.SetForTest(15),
		env.AdaptivePanPad.SetForTest(true),
		env.FastPanDetect.SetForTest(true),
		env.PredictorBackoff.SetForTest(true),
		env.ForceGCInterval.SetForTest(16 * time.Millisecond),
	}
	for _, r := range restores {
		defer r()
	}

	p := &RuntimeProfile{}
	applyEnvProfileOverride(p)

	if p.EdgeCachePad != 11 {
		t.Errorf("EdgeCachePad=%d, want 11", p.EdgeCachePad)
	}
	if p.GridCachePad != 12 {
		t.Errorf("GridCachePad=%d, want 12", p.GridCachePad)
	}
	if !p.DisableNodeGlow {
		t.Errorf("DisableNodeGlow=false, want true")
	}
	if !p.DisableEdgeArrows {
		t.Errorf("DisableEdgeArrows=false, want true")
	}
	if !p.SimpleDrawDefault {
		t.Errorf("SimpleDrawDefault=false, want true")
	}
	if p.TimelineInfoThrottleMS != 13 {
		t.Errorf("TimelineInfoThrottleMS=%d, want 13", p.TimelineInfoThrottleMS)
	}
	if p.AudioLookaheadSec != 0.25 {
		t.Errorf("AudioLookaheadSec=%v, want 0.25", p.AudioLookaheadSec)
	}
	if p.AudioBatchMax != 14 {
		t.Errorf("AudioBatchMax=%d, want 14", p.AudioBatchMax)
	}
	if p.SequencerTickMS != 15 {
		t.Errorf("SequencerTickMS=%d, want 15", p.SequencerTickMS)
	}
	if !p.AdaptivePanPad {
		t.Errorf("AdaptivePanPad=false, want true")
	}
	if !p.FastPanDetect {
		t.Errorf("FastPanDetect=false, want true")
	}
	if !p.PredictorBackoffOnSlowDraw {
		t.Errorf("PredictorBackoffOnSlowDraw=false, want true")
	}
	if p.ForceGCInterval != 16*time.Millisecond {
		t.Errorf("ForceGCInterval=%v, want 16ms", p.ForceGCInterval)
	}
}

// TestApplyEnvProfileOverrideLeavesUnsetFieldsAlone pins the "override only
// when explicitly set" contract. A var left at its default must not clobber
// the builder's value.
func TestApplyEnvProfileOverrideLeavesUnsetFieldsAlone(t *testing.T) {
	p := &RuntimeProfile{EdgeCachePad: 99, DisableNodeGlow: true}
	applyEnvProfileOverride(p)
	if p.EdgeCachePad != 99 {
		t.Errorf("EdgeCachePad=%d, want 99 — an unset env var must not overwrite", p.EdgeCachePad)
	}
	if !p.DisableNodeGlow {
		t.Errorf("DisableNodeGlow=false, want true — an unset env var must not overwrite")
	}
}
```

The "leave unset alone" contract matters: the old helpers returned
`(value, ok)` and skipped assignment when `ok` was false. `Var.IsSet()` from
Task 1 preserves it — `applyEnvProfileOverride` guards every assignment on it,
so an absent variable never writes a fallback default over the builder's
value. No change to Task 1 is needed; `IsSet` is already part of its interface.

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestApplyEnvProfileOverride`
Expected: FAIL — `undefined: applyEnvProfileOverride` in a non-tagged file.

- [ ] **Step 3: Move `applyEnvProfileOverride` into an untagged file**

Create `src/go/internal/ui/runtime_profile_env.go`:

```go
package ui

import "github.com/ingyamilmolinar/beatmo/internal/env"

// applyEnvProfileOverride applies BEATMO_* environment overrides on top of a
// builder-produced profile. Phase F bench sweeps use these to flip individual
// knobs without recompiling.
//
// This file carries no build tags: env vars now work on WASM too (the JS
// bootstrap populates go.env), so the browser and desktop profiles share one
// override path. The bespoke window.__beatmoProfileOverride reader that
// runtime_profile_js.go used to carry is retired in favor of __beatmoEnv.
//
// Each assignment is guarded on IsSet so an unset variable leaves the
// builder's value untouched — matching the (value, ok) helpers this replaced.
func applyEnvProfileOverride(p *RuntimeProfile) {
	if env.EdgeCachePad.IsSet() {
		p.EdgeCachePad = env.EdgeCachePad.Get()
	}
	if env.GridCachePad.IsSet() {
		p.GridCachePad = env.GridCachePad.Get()
	}
	if env.DisableNodeGlow.IsSet() {
		p.DisableNodeGlow = env.DisableNodeGlow.Get()
	}
	if env.DisableEdgeArrows.IsSet() {
		p.DisableEdgeArrows = env.DisableEdgeArrows.Get()
	}
	if env.SimpleDrawDefault.IsSet() {
		p.SimpleDrawDefault = env.SimpleDrawDefault.Get()
	}
	if env.TimelineInfoThrottleMS.IsSet() {
		p.TimelineInfoThrottleMS = env.TimelineInfoThrottleMS.Get()
	}
	if env.AudioLookaheadSec.IsSet() {
		p.AudioLookaheadSec = env.AudioLookaheadSec.Get()
	}
	if env.AudioBatchMax.IsSet() {
		p.AudioBatchMax = env.AudioBatchMax.Get()
	}
	if env.SequencerTickMS.IsSet() {
		p.SequencerTickMS = env.SequencerTickMS.Get()
	}
	if env.AdaptivePanPad.IsSet() {
		p.AdaptivePanPad = env.AdaptivePanPad.Get()
	}
	if env.FastPanDetect.IsSet() {
		p.FastPanDetect = env.FastPanDetect.Get()
	}
	if env.PredictorBackoff.IsSet() {
		p.PredictorBackoffOnSlowDraw = env.PredictorBackoff.Get()
	}
	// BEATMO_FORCE_GC_INTERVAL_MS in milliseconds; 0 disables the cooperative
	// forced GC. Desktop default is 0 (off); the GC-tuning sweep uses this to
	// probe whether forced GC helps desktop too.
	if env.ForceGCInterval.IsSet() {
		p.ForceGCInterval = env.ForceGCInterval.Get()
	}
}
```

Remove `applyEnvProfileOverride`, `envInt`, `envFloat`, `envBool` from
`runtime_profile_notjs.go`, leaving only:

```go
//go:build !js

package ui

// newRuntimeProfile returns the desktop profile with env overrides applied.
func newRuntimeProfile() *RuntimeProfile {
	p := desktopRuntimeProfile()
	applyEnvProfileOverride(p)
	return p
}
```

Replace the `__beatmoProfileOverride` reader in `runtime_profile_js.go` with:

```go
//go:build js

package ui

// newRuntimeProfile returns the browser profile with env overrides applied.
// Overrides arrive through window.__beatmoEnv (populated into go.env by the
// JS bootstrap), so this is the same path desktop uses.
func newRuntimeProfile() *RuntimeProfile {
	p := browserRuntimeProfile()
	applyEnvProfileOverride(p)
	return p
}
```

- [ ] **Step 4: Run to verify it passes**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestApplyEnvProfileOverride -v
GOOS=js GOARCH=wasm ../../.tools/go/bin/go build -tags test ./internal/ui/
```
Expected: tests PASS; the js build compiles.

- [ ] **Step 5: Migrate the three browser tests and verify (no commit)**

In `webaudio_bench_startup.browser.test.js:102`,
`webaudio_perf_e2e.browser.test.js:66`, and
`webaudio_fx_chain_stress.browser.test.js:255`, replace

```js
window.__beatmoProfileOverride = cfg;
```

with

```js
// Env-var overrides reach Go via go.env (see index.html). Keys are the
// BEATMO_* names from internal/env/vars.go; values must be strings.
window.__beatmoEnv = Object.fromEntries(
  Object.entries(cfg).map(([k, v]) => [k, String(v)]),
);
```

and change each `cfg` literal's keys from RuntimeProfile field names to the
corresponding `BEATMO_*` variable names (e.g. `drawMinIntervalMS: 0` becomes
`BEATMO_TIMELINE_INFO_THROTTLE_MS: 0` where that is the field being swept —
check each against `runtime_profile.go`'s field list). Update the
`BEATMO_PROFILE_OVERRIDE` env plumbing in those harnesses to pass
`BEATMO_ENV_OVERRIDE` JSON instead.

Run:
```bash
GO=$(pwd)/.tools/go/bin/go node src/js/webaudio_perf_e2e.browser.test.js
GO=$(pwd)/.tools/go/bin/go node src/js/webaudio_bench_startup.browser.test.js
```
Expected: PASS. Do NOT commit.

---

### Task 5: Migrate `internal/audio`

**Files:**
- Modify: `src/go/internal/audio/engine_stop.go`, `channels.go`, `export.go`, `engine.go`, `catalog_desktop.go`, `catalog_stub.go`, `recording_paths.go`, `synth_recipe_dispatch.go`

**Interfaces:**
- Consumes: `env.TestTone`, `env.TestVoice`, `env.TestRawVoice`, `env.TestRawVoiceFull`, `env.TestRawVoiceNoScale`, `env.SingleVoice`, `env.BypassChannelProc`, `env.BypassHeadroom`, `env.BypassEQ`, `env.BypassNormalize`, `env.AudioClipDebug`, `env.DebugMixer`, `env.DebugChannel`, `env.AssetsRoot`, `env.RecordingsDir`, `env.AudioSampleRate`, `env.ScopeExport`, `env.ScopeExportPath`, `env.ScopeExportInterval`, `env.SynthDispatchDebug`, `env.PrewarmDebug` from Task 3.
- Produces: nothing new.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/audio/env_migration_test.go`:

```go
package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/env"
)

// TestBypassEQHonorsEnvVar proves the bypass reads through internal/env
// rather than an init-time os.Getenv cache — the cache made the flag
// untestable without a subprocess.
func TestBypassEQHonorsEnvVar(t *testing.T) {
	restore := env.BypassEQ.SetForTest(true)
	defer restore()
	if !env.BypassEQ.Get() {
		t.Fatalf("precondition: BypassEQ.Get()=false after SetForTest(true)")
	}
	// processChannel must short-circuit. Assert via the observable effect:
	// with EQ bypassed a non-flat EQ curve leaves the signal unchanged.
	SetChannelEQ("main", 12)
	in := []float32{0.5, -0.5, 0.25, -0.25}
	got := append([]float32(nil), in...)
	processChannelForTest("main", got)
	for i := range in {
		if got[i] != in[i] {
			t.Errorf("sample %d = %v, want %v unchanged with BypassEQ set", i, got[i], in[i])
		}
	}
}
```

If `processChannelForTest` does not exist, add a thin test-only export in
`channels.go` wrapping the existing unexported channel-processing entry point;
do not restructure the processing code itself.

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/ -run TestBypassEQHonorsEnvVar`
Expected: FAIL — the value is still read from the init-time `bypassEQ` cache, so `SetForTest` has no effect.

- [ ] **Step 3: Convert the call sites**

In `engine_stop.go`, delete the nine package-level `var`s at lines 50-86 and
replace each usage. For example `if bypassHeadroom {` becomes
`if env.BypassHeadroom.Get() {`. Keep the non-env vars (`debugMixerCount`,
`testTonePhase`) exactly as they are. Update the `init()` log lines to the new
names:

```go
func init() {
	if env.TestTone.Get() {
		log.Println("[audio debug] BEATMO_TEST_TONE - Outputting 440Hz test tone (bypassing all synth)")
	}
	// ... same for the other modes ...
}
```

In `channels.go` delete `bypassEQ` and `debugChannelProc`; in `export.go`
delete `bypassNormalize`; in `synth_recipe_dispatch.go` delete
`debugSynthDispatch` and `prewarmDebug`. Replace every read with the
corresponding `env.X.Get()`.

In `engine.go:22`, replace the sample-rate block with
`sampleRate := env.AudioSampleRate.Get()`. In `engine.go:170-180` replace the
three `SCOPE_EXPORT*` reads with `env.ScopeExport.Get()`,
`env.ScopeExportInterval.Get()`, `env.ScopeExportPath.Get()`.

In `catalog_desktop.go:39` and `catalog_stub.go:129` replace
`os.Getenv("BEATMO_ASSETS")` with `env.AssetsRoot.Get()`. In
`recording_paths.go:14` replace with `env.RecordingsDir.Get()`.

Remove the now-unused `"os"` imports.

- [ ] **Step 4: Run to verify it passes**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/...
```
Expected: PASS.

- [ ] **Step 5: Verify native audio too (no commit)**

Run: `cd src/go && xvfb-run -a ../../.tools/go/bin/go test ./internal/audio/... -run "Mixer|Channel|Biquad"`
Expected: PASS. Do NOT commit.

---

### Task 6: Migrate `internal/ui`

The largest task. 41 test sites across 9 files assign the cached vars directly
(`envRenderSafe = true`); those become `env.RenderSafe.SetForTest(true)`.

**Files:**
- Modify: `game.go`, `game_new.go`, `game_draw_grid_pane.go`, `camera.go`, `widgets.go`, `perf.go`, `demo.go`, `demo_stub.go`, `layout_profile.go`, `drumview_instruments.go`, `heap_growth_probe.go`, `game_parity_state.go`, `game_parity_timing.go`, `game_graph_update_beat_infos.go`, `game_sequencer_loop.go`, `parity_scan_config.go`
- Modify (test overrides): `trace_sprite_test.go`, `synth_component_sprite_test.go`, `render_verify_test.go`, `screen_edges_arrows_test.go`, and the 5 other files found by the grep in Step 3
- Modify: `game_test_registry_test.go:121-150`

**Interfaces:**
- Consumes: the render-debug, parity, timeline, and demo `*XVar`s from Task 3; `applyEnvProfileOverride` from Task 4.
- Produces: nothing new.

- [ ] **Step 1: Write the failing test**

Add to `src/go/internal/ui/env_migration_test.go` (new file):

```go
package ui

import (
	"os"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/env"
)

// TestResetUnsetsEveryRegisteredVar proves the test reset hook derives from
// the registry rather than a hand-maintained list. The old list had rotted:
// it named DRAW_FRAMEBUFFER and PREVIEW_DEBUG (no reader anywhere) and omitted
// eighteen live keys, so a test setting one leaked it into every later test.
func TestResetUnsetsEveryRegisteredVar(t *testing.T) {
	const probe = "BEATMO_DEBUG_LAYOUT" // was missing from the old hand list
	t.Setenv(probe, "1")
	env.Reload()
	if !env.DebugLayout.Get() {
		t.Fatalf("precondition: DebugLayout.Get()=false with %s=1", probe)
	}
	resetTestEnvAndGlobals()
	if v := os.Getenv(probe); v != "" {
		t.Errorf("%s=%q after reset, want unset", probe, v)
	}
	if env.DebugLayout.Get() {
		t.Errorf("DebugLayout.Get()=true after reset, want false")
	}
}

// TestSeqTickRoutesThroughRuntimeProfile pins the SEQ_TICK_MS removal: the
// sequencer loop must read RuntimeProf().SequencerTickMS, the documented
// mechanism for browser/desktop divergence, not a second env var.
func TestSeqTickRoutesThroughRuntimeProfile(t *testing.T) {
	restore := SetRuntimeProfileForTest(desktopRuntimeProfile())
	defer restore()
	RuntimeProf().SequencerTickMS = 9
	if got := sequencerTickInterval(); got.Milliseconds() != 9 {
		t.Errorf("sequencerTickInterval()=%v, want 9ms from RuntimeProf().SequencerTickMS", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestResetUnsetsEveryRegisteredVar|TestSeqTickRoutesThroughRuntimeProfile'`
Expected: FAIL — reset uses the stale hand list; `sequencerTickInterval` undefined.

- [ ] **Step 3: Convert the production call sites**

Delete the cached `var` block in `game_draw_grid_pane.go:15-23` entirely and
replace each read: `if envNoPixelSnap {` → `if env.NoPixelSnap.Get() {`, and
so on for all seven.

Delete `debugGeom` in `widgets.go:57` and `perfLogEnabled` in `perf.go:14`;
replace reads with `env.DebugGeom.Get()` / `env.PerfLog.Get()`. Same for
`camera.go:80`.

In `game.go`: replace `timelineTrace`/`timelineTraceRow` init with
`env.TimelineTrace.Get()` / `env.TimelineTraceRow.Get()`; replace the
`PARITY_FATAL` / `PARITY_WATCH` / `PARITY_WASM_FATAL` blocks with
`env.ParityFatal`, `env.ParityWatch`, `env.ParityWasmFatal`. Delete the
`PERF_FAST_PATH` disjunct at line 49 per the spec removal, leaving:

```go
if env.PerfBrowserUpdateMaxMS.IsSet() || RuntimeProf().DefaultPerfFastPath {
```

In `demo.go:64-67` and `demo_stub.go:41-44`, collapse the two-name lookup to
one — `BEATMO_CONFIG` is removed:

```go
cfg := env.DemoConfig.Get()
```

In `game_sequencer_loop.go:19`, replace the `SEQ_TICK_MS` block with a helper
that reads the profile:

```go
// sequencerTickInterval returns the sequencer tick period. Browser/desktop
// divergence lives in RuntimeProfile (BEATMO_SEQUENCER_TICK_MS), which is the
// documented mechanism; the old SEQ_TICK_MS was a second, duplicate knob.
func sequencerTickInterval() time.Duration {
	ms := RuntimeProf().SequencerTickMS
	if ms < 1 {
		ms = 1
	}
	return time.Duration(ms) * time.Millisecond
}
```

In `heap_growth_probe.go:26,33` replace both booleans with
`env.HeapProbeEnabled()` / `env.HeapWatchEnabled()`.

In `parity_scan_config.go`, delete `envIntDefault` and read
`env.ParityScanEvery.Get()` / `env.ParityScanStride.Get()`.

Remaining one-liners: `game_new.go:125,258,265`, `layout_profile.go:14`,
`drumview_instruments.go:163`, `game_parity_state.go:387`,
`game_parity_timing.go:56,274`, `game_graph_update_beat_infos.go:286,403`.

- [ ] **Step 4: Convert the 41 test overrides and the reset hook**

Find them:

```bash
cd src/go && grep -rn "envRenderSafe = \|envScreenEdges = \|envNoGridDraw = \|envNoGridTileCache = \|envNoPixelSnap = \|envNoEdgeCache = \|envNoSpriteNodes = \|timelineTrace = \|timelineTraceRow = \|debugGeom = " --include="*_test.go" .
```

Convert each pair. Before:

```go
envRenderSafe = true
t.Cleanup(func() { envRenderSafe = false })
```

After:

```go
t.Cleanup(env.RenderSafe.SetForTest(true))
```

Note the semantic improvement: the old form restored to `false` rather than
the prior value, so a nested override lost state. `SetForTest` restores what
was actually there.

In `game_test_registry_test.go`, replace the whole `envKeys` slice and its
loop with:

```go
	// Derived from the registry, so it cannot drift. The hand-maintained list
	// this replaced named two variables with no reader (DRAW_FRAMEBUFFER,
	// PREVIEW_DEBUG) and omitted eighteen live ones.
	for _, v := range env.All() {
		_ = os.Unsetenv(v.Name())
	}
	env.Reload()
```

and delete the now-redundant manual resets below it (`envRenderSafe = false`,
`timelineTrace = false`, `timelineTraceRow = 0`, `debugGeom = false`,
`parityScanEveryFrames = ...`, `parityScanStride = ...`).

- [ ] **Step 5: Run to verify it passes and verify (no commit)**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/
```
Expected: PASS. `internal/ui` is the largest suite; if it times out, raise with
`-timeout 1500s` — per prior findings that is contention, not a hang.

Then: `cd src/go && xvfb-run -a ../../.tools/go/bin/go test ./internal/ui/ -run 'ScreenEdges|RenderVerify|TraceSprite'`
Expected: PASS. Do NOT commit.

---

### Task 7: Migrate `cmd`, `async`, `bench`, `log`, `wavref`, `deploy`, `eventlogger`

**Files:**
- Modify: `src/go/cmd/beatmo.go`, `profiling.go`, `pyroscope.go`, `beatmo_record_bench.go`
- Modify: `src/go/internal/async/runtime.go`, `src/go/internal/log/log.go`, `src/go/internal/wavref/wavref.go`
- Modify: `src/go/internal/deploy/asset_size_test.go`, `src/go/internal/eventlogger/golden_session_test.go`, `src/go/internal/audio/legacy_oracle_golden_test.go`, `src/go/internal/audio/ifx_xplat_golden_test.go`, `src/go/internal/audio/render_rtf_test.go`, `src/go/internal/audio/template_audio_quality_test.go`, `src/go/internal/audio/kick_refs_tune_test.go`, `src/go/internal/audio/zgump_kick_tune_test.go`, `src/go/internal/ui/soak_heap_bound_real_test.go`, `src/go/internal/ui/fontcache_visibility_test.go`, `src/go/internal/deploy/headers_verify_test.go`
- Delete: `src/go/internal/bench/env.go`

**Interfaces:**
- Consumes: the logging, profiling, runtime-tuning, and test-only `*XVar`s from Task 3.
- Produces: nothing new.

- [ ] **Step 1: Write the failing test**

Add to `src/go/internal/env/vars_test.go`:

```go
// TestStartupReloadIsIdempotent pins the contract that cmd/beatmo calls
// Reload() once at startup and that doing so twice changes nothing.
func TestStartupReloadIsIdempotent(t *testing.T) {
	t.Setenv("BEATMO_PPROF", "1")
	Reload()
	first := PProf.Get()
	Reload()
	if second := PProf.Get(); second != first {
		t.Errorf("PProf.Get()=%v after second Reload, want %v", second, first)
	}
	if !first {
		t.Errorf("PProf.Get()=false with BEATMO_PPROF=1")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/env/ -run TestStartupReloadIsIdempotent`
Expected: FAIL — `undefined: PProf` until Task 3's table includes it, or PASS trivially if already declared; in that case proceed.

- [ ] **Step 3: Convert the call sites**

`cmd/beatmo.go`: add `env.Reload()` as the first statement of `main()` after
`flag.Parse()`, with the comment:

```go
	// Resolve every registered env var once, up front. Everything downstream
	// reads the cached values through internal/env.
	env.Reload()
```

Then replace lines 143, 145, 171, 182 with `env.EventLog.Get()`,
`env.EventLogVerbose.Get()`, `env.InfoLog.Get() != "off"`,
`env.ScopeExport.Get()`. Preserve the existing flag-wins precedence exactly —
the flag is checked first at each site today; keep that ordering.

`cmd/profiling.go:16`: `if !env.PProf.Get() { return }`.
`cmd/pyroscope.go:14,18`: `env.PyroscopeURL.Get()`, `env.PyroscopeApp.Get()`.
`cmd/beatmo_record_bench.go:62,63,90,94`: `env.BlockProfile.Get()`,
`env.MutexProfile.Get()`, `env.RecordBenchHeapSnapshots.Get()`,
`env.RecordBenchSynthChurnHz.Get()`.

`internal/async/runtime.go:69-81`: replace the five `envInt` calls with
`env.MemoryLimitMB.Get()`, `env.GCPercent.Get()`, `env.GOMAXPROCS.Get()`,
`env.BlockProfile.Get()`, `env.MutexProfile.Get()`. Delete the local `envInt`
helper at line 134 and its test in `runtime_test.go`.

`internal/log/log.go:116-118`: `env.TestLog.Get()`, `env.TestLogLevel.Get()`.
Confirm no import cycle — `env` imports only stdlib.

`internal/wavref/wavref.go:24`: `env.WavRefs.Get()`. Keep the exported
`EnvVar` constant but redefine it as `const EnvVar = "BEATMO_WAV_REFS"` and
have `Dir()` read through `env`.

Test files: swap each `os.Getenv("X")` for the corresponding `env.X.Get()`,
using the renamed spellings for `SWEEP`, `WRITE_WAV`, `UPDATE_GOLDEN`,
`RUN_SOAK_REAL`.

Delete `src/go/internal/bench/env.go`. If `internal/bench` has other files,
keep the package; if `env.go` was its only file, delete the directory and
remove the import from `cmd/beatmo_record_bench.go`.

- [ ] **Step 4: Run to verify it passes**

Run:
```bash
cd src/go && ../../.tools/go/bin/go build ./... && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...
```
Expected: PASS.

- [ ] **Step 5: Verify (no commit)**

Run: `cd src/go && GOOS=js GOARCH=wasm ../../.tools/go/bin/go build ./...`
Expected: clean. Do NOT commit.

---

### Task 8: Discipline guards

**Files:**
- Create: `src/go/internal/env/discipline_test.go`

**Interfaces:**
- Consumes: `env.All()` from Task 1.
- Produces: nothing.

- [ ] **Step 1: Write the failing tests**

```go
package env

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// exemptFromGetenvBan lists the only files allowed to call os.Getenv outside
// this package. Keep it as short as possible; each entry needs a reason.
var exemptFromGetenvBan = map[string]string{
	// The WASM env probe deliberately reads an arbitrary caller-supplied name
	// to prove window.__beatmoEnv reaches Go. See wasm_env.browser.test.js.
	"internal/ui/js_exports_debug.go": "envProbe debug export",
}

func TestNoDirectGetenv(t *testing.T) {
	root := repoGoRoot(t)
	fset := token.NewFileSet()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "internal/env/") {
			return nil
		}
		if _, ok := exemptFromGetenvBan[rel]; ok {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return nil
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "os" {
				return true
			}
			switch sel.Sel.Name {
			case "Getenv", "LookupEnv", "Environ":
				t.Errorf("%s:%d: os.%s outside internal/env — declare the variable in internal/env/vars.go and read it with env.X.Get()",
					rel, fset.Position(sel.Pos()).Line, sel.Sel.Name)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}

func TestEveryVarDocumented(t *testing.T) {
	for _, v := range All() {
		if strings.TrimSpace(v.Purpose()) == "" {
			t.Errorf("%s has an empty purpose", v.Name())
		}
	}
}

// legacyUnprefixed is the frozen allowlist of names that predate the
// BEATMO_ convention. It may only SHRINK. A new variable must be
// BEATMO_-prefixed.
//
// A prefix-set guard would not work here: AUDIO_CLIP_DEBUG, BPM_TIMING_TEST,
// FONTCACHE_SUBPROCESS, IFX_GOLDEN_UPDATE, RENDER_SAFE, SCREEN_EDGES,
// TIMELINE_TRACE and TIMELINE_TRACE_ROW share no common prefix.
var legacyUnprefixed = map[string]bool{
	"AUDIO_CLIP_DEBUG": true, "BPM_TIMING_TEST": true,
	"BYPASS_CHANNEL_PROC": true, "BYPASS_EQ": true,
	"BYPASS_HEADROOM": true, "BYPASS_NORMALIZE": true,
	"DEBUG_CHANNEL": true, "DEBUG_DRAW_NODES": true,
	"DEBUG_GEOM": true, "DEBUG_HISTORY_SEED": true,
	"DEBUG_MIXER": true, "FONTCACHE_SUBPROCESS": true,
	"IFX_GOLDEN_UPDATE": true, "NO_EDGE_CACHE": true,
	"NO_GRID_DRAW": true, "NO_GRID_TILE_CACHE": true,
	"NO_PIXEL_SNAP": true, "NO_SPRITE_NODES": true,
	"PARITY_DUMP_STDERR": true, "PARITY_FATAL": true,
	"PARITY_SCAN_EVERY": true, "PARITY_SCAN_STRIDE": true,
	"PARITY_TIMING": true, "PARITY_TIMING_FATAL": true,
	"PARITY_WASM_FATAL": true, "PARITY_WATCH": true,
	"PERF_BROWSER_UPDATE_MAX_MS": true, "PERF_LOG": true,
	"PYROSCOPE_APP": true, "PYROSCOPE_URL": true,
	"RENDER_SAFE": true, "SCOPE_EXPORT": true,
	"SCOPE_EXPORT_INTERVAL": true, "SCOPE_EXPORT_PATH": true,
	"SCREEN_EDGES": true, "TIMELINE_TRACE": true,
	"TIMELINE_TRACE_ROW": true,
}

func TestEnvNamingConvention(t *testing.T) {
	const wantLegacy = 37
	if len(legacyUnprefixed) != wantLegacy {
		t.Errorf("legacyUnprefixed has %d entries, want %d — the list may only shrink; update this constant when you prefix one",
			len(legacyUnprefixed), wantLegacy)
	}
	seen := map[string]bool{}
	for _, v := range All() {
		name := v.Name()
		seen[name] = true
		if strings.HasPrefix(name, "BEATMO_") {
			continue
		}
		if legacyUnprefixed[name] {
			continue
		}
		t.Errorf("%s is not BEATMO_-prefixed and is not on the frozen legacy allowlist; new variables must be BEATMO_-prefixed", name)
	}
	for name := range legacyUnprefixed {
		if !seen[name] {
			t.Errorf("legacyUnprefixed names %q but it is not registered — remove it from the list (the list may only shrink)", name)
		}
	}
}
```

Add a `repoGoRoot(t *testing.T) string` helper that walks up from the test's
working directory to the directory containing `go.mod`.

- [ ] **Step 2: Run to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/env/ -run 'TestNoDirectGetenv|TestEnvNamingConvention'`
Expected: FAIL only if a call site was missed in Tasks 5-7. If they pass immediately, that confirms the migration is complete — record the pass and continue.

- [ ] **Step 3: Fix whatever the guards report**

Convert any remaining `os.Getenv` the AST walk found. If a genuinely
unconvertible site exists, add it to `exemptFromGetenvBan` **with a reason**,
not silently.

- [ ] **Step 4: Run to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/env/ -v`
Expected: PASS.

- [ ] **Step 5: Verify (no commit)**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...`
Expected: PASS. Do NOT commit.

---

### Task 9: `-list-env` flag and CLAUDE.md regeneration

**Files:**
- Modify: `src/go/cmd/beatmo.go`
- Modify: `CLAUDE.md`
- Create: `src/go/internal/env/doc_drift_test.go`

**Interfaces:**
- Consumes: `env.All()`.
- Produces: `func MarkdownTable(s Scope) string` in `internal/env`, used by both the flag and the drift test.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/env/doc_drift_test.go`:

```go
package env

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCLAUDEMDEnvTablesMatch regenerates the environment tables from the
// registry and diffs them against CLAUDE.md. Mirrors design_md_drift_test.go.
//
// The tables live between the marker comments:
//   <!-- BEGIN GENERATED ENV TABLE -->
//   <!-- END GENERATED ENV TABLE -->
func TestCLAUDEMDEnvTablesMatch(t *testing.T) {
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	const begin = "<!-- BEGIN GENERATED ENV TABLE -->"
	const end = "<!-- END GENERATED ENV TABLE -->"
	doc := string(raw)
	i := strings.Index(doc, begin)
	j := strings.Index(doc, end)
	if i < 0 || j < 0 || j < i {
		t.Fatalf("CLAUDE.md is missing the generated-env-table markers %q / %q", begin, end)
	}
	got := strings.TrimSpace(doc[i+len(begin) : j])
	want := strings.TrimSpace(GeneratedDoc())
	if got != want {
		t.Errorf("CLAUDE.md env tables are stale.\nRegenerate with:\n  ./beatmo -list-env -markdown\nand paste between the markers.\n\n--- got ---\n%s\n\n--- want ---\n%s", got, want)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/env/ -run TestCLAUDEMDEnvTablesMatch`
Expected: FAIL — `undefined: GeneratedDoc`, then missing markers.

- [ ] **Step 3: Implement `GeneratedDoc` and the flag**

Add to `src/go/internal/env/doc.go`:

```go
package env

import (
	"fmt"
	"strings"
)

// GeneratedDoc renders every registered variable as Markdown tables grouped
// by scope. It is the source for the generated section of CLAUDE.md and for
// `beatmo -list-env -markdown`.
func GeneratedDoc() string {
	var b strings.Builder
	for _, s := range []Scope{ScopeRuntime, ScopeDebug, ScopeTest} {
		rows := make([]Var, 0)
		for _, v := range All() {
			if v.Scope() == s {
				rows = append(rows, v)
			}
		}
		if len(rows) == 0 {
			continue
		}
		fmt.Fprintf(&b, "#### %s (%d)\n\n", strings.Title(s.String()), len(rows))
		b.WriteString("| Variable | Type | Default | Purpose |\n|---|---|---|---|\n")
		for _, v := range rows {
			fmt.Fprintf(&b, "| `%s` | %s | `%s` | %s |\n",
				v.Name(), v.TypeName(), v.DefaultString(), v.Purpose())
		}
		b.WriteString("\n")
	}
	return b.String()
}
```

In `cmd/beatmo.go`, add alongside the other list flags:

```go
	listEnv := flag.Bool("list-env", false, "print every environment variable the game reads and exit")
	listEnvMD := flag.Bool("markdown", false, "with -list-env, emit the Markdown table for CLAUDE.md")
```

and after `flag.Parse()` + `env.Reload()`:

```go
	if *listEnv {
		if *listEnvMD {
			fmt.Print(env.GeneratedDoc())
		} else {
			for _, v := range env.All() {
				fmt.Printf("%s\t%s\t%s\t%s\t%s\n",
					v.Name(), v.TypeName(), v.DefaultString(), v.Scope(), v.Purpose())
			}
		}
		return
	}
```

- [ ] **Step 4: Regenerate CLAUDE.md and verify**

Replace the four hand-written env-var tables in CLAUDE.md — the "Environment
Variables" table under Debugging, and the "Runtime Configuration",
"Profiling", "Audio Debugging", "UI Rendering & Geometry", "Timeline & Parity"
and "Test Infrastructure (Go)" tables under the "Environment Variables"
heading — with:

```markdown
<!-- BEGIN GENERATED ENV TABLE -->
<!-- END GENERATED ENV TABLE -->
```

then fill it:

```bash
CGO_ENABLED=1 .tools/go/bin/go build -o /tmp/beatmo_listenv ./src/go/cmd
/tmp/beatmo_listenv -list-env -markdown
```

Paste the output between the markers. Leave the Node/Playwright/Makefile
tables ("Test Infrastructure (Browser / JS)", "Browser Performance
Thresholds", "Benchmarking", "LLM Recording & Agent", "Deployment (GCP)",
"External Services") untouched and add a one-line note above them:

```markdown
> The tables below are **harness** variables read by Node/Playwright test
> runners and Makefile targets, not by the game. They are hand-maintained;
> the generated table above covers everything `internal/env` owns.
```

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/env/ -run TestCLAUDEMDEnvTablesMatch`
Expected: PASS.

- [ ] **Step 5: Full verification (no commit)**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...
cd src/go && ../../.tools/go/bin/go build ./... && GOOS=js GOARCH=wasm ../../.tools/go/bin/go build ./...
```
Then from the repo root:
```bash
GO=$(pwd)/.tools/go/bin/go node src/js/wasm_env.browser.test.js
GO=$(pwd)/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js
GO=$(pwd)/.tools/go/bin/go node src/js/xplat_parity.browser.test.js
```
Expected: all PASS. Do NOT commit — report results and leave staging to the user.

---

## Self-Review

**Spec coverage.** Every spec section maps to a task: §Design.1 `internal/env`
→ Tasks 1+3; §Design.2 WASM → Task 2; §Design.3 call-site migration → Tasks
5-7; §Design.4 guards → Task 8; §Design.5 documentation → Task 9; §Removals →
Tasks 3 (registry guard), 6 (`SEQ_TICK_MS`, `PERF_FAST_PATH`, `BEATMO_CONFIG`,
heap pair); §Renames → Task 3 + the frozen allowlist in Task 8;
§Behavior changes → covered by the browser tests in Tasks 2 and 4.

**Two additions beyond the spec**, both forced by reading the code:

1. **`SetForTest`** — 41 test sites across 9 files assign the cached package
   vars directly (`envRenderSafe = true`). Without a test override those tests
   have no way to force a value once the `var` is deleted. Added to Task 1's
   interface and exercised in Task 6.
2. **`IsSet()`** — the 13 RuntimeProfile overrides used `(value, ok)` helpers
   and skipped assignment when absent. A plain `Get()` returning a zero default
   would clobber the builder's value. Added in Task 4, and `PERF_FAST_PATH`'s
   removal in Task 6 depends on it too (`env.PerfBrowserUpdateMaxMS.IsSet()`).

Both are fully specified in Task 1's code — `isSet atomic.Bool` on `meta`,
`IsSet()` on the `Var` interface, set by every `reload()` and by every
`SetForTest`. No task asks the implementer to retrofit an earlier task.

Note the deliberate `IsSet` semantics for numeric types: a present-but-
unparseable value (`BEATMO_AUDIO_BATCH_MAX=banana`) reports `IsSet() == false`,
so `applyEnvProfileOverride` leaves the builder's value alone rather than
writing the fallback default over it. The `Str` and `Bool` types report
`IsSet() == true` for any present non-empty value, since neither can fail to
parse.

**Ordering.** Task 4 precedes Task 6 because `SEQ_TICK_MS`'s removal routes
through `RuntimeProf().SequencerTickMS`. Task 8's guards run last so they
report only genuinely-missed sites rather than work-in-progress.

**Risk.** Task 6 is the largest and touches the most-contended test suite.
If `internal/ui` fails after the reset-hook change, suspect a test that
depended on the old list's *incompleteness* — one that set a variable the old
reset never cleared and relied on it persisting. That is the bug being fixed;
fix the test, not the hook.
