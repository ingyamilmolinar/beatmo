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
// # Concurrency
//
// Get() and IsSet() are each individually lock-free and safe to call from
// any goroutine at any time. But every typed reload() (and SetForTest) writes
// isSet and val as two INDEPENDENT atomic stores, not one atomic transaction.
// A goroutine calling IsSet() then Get() back-to-back can observe isSet==true
// paired with the value from before the reload — the isSet store lands first
// and a concurrent reader can slip in between the two stores. Reload() and
// SetForTest() are therefore not safe to call concurrently with each other or
// with themselves on the same Var, and an IsSet()+Get() pair is only
// guaranteed coherent when no Reload()/SetForTest() call is in flight for
// that Var. Today's call pattern (one Reload() at startup, serial test
// reloads) makes this inert; it matters once code (e.g. a profile-override
// path gating a write on IsSet()) starts calling Get()/IsSet() from a draw or
// audio loop while a test concurrently calls Reload() or SetForTest().
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

// unregister removes name from the registry. It exists solely so tests can
// register scratch variables (via the registering constructors) without
// leaking them into All() for the rest of the process — the registry is
// process-global and otherwise permanent. Production code has no reason to
// call this: real variables are declared once at package init and live for
// the process lifetime by design.
func unregister(name string) {
	registryMu.Lock()
	defer registryMu.Unlock()
	delete(registry, name)
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
//
// Reload is not safe to call concurrently with itself or with SetForTest on
// the same Var: each typed reload() stores isSet and val as two separate
// atomic operations, so a Get()/IsSet() pair observed mid-Reload from another
// goroutine is not guaranteed coherent (see the package doc's "Concurrency"
// section). Get() and IsSet() remain individually safe to call from any
// goroutine at any time.
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
//
// Restores must be strictly LIFO per Var: call `defer restore()` immediately
// after SetForTest, and if you stack multiple overrides on the same Var,
// restore them in the exact reverse order you applied them. Each restore
// closure only knows the single (overridden, isSet) snapshot it captured at
// its own SetForTest call; restoring out of order re-stores a stale snapshot
// over the state belonging to a still-active later override, corrupting it.
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
//
// Restores must be strictly LIFO per Var: call `defer restore()` immediately
// after SetForTest, and if you stack multiple overrides on the same Var,
// restore them in the exact reverse order you applied them. Each restore
// closure only knows the single (overridden, isSet) snapshot it captured at
// its own SetForTest call; restoring out of order re-stores a stale snapshot
// over the state belonging to a still-active later override, corrupting it.
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
//
// Restores must be strictly LIFO per Var: call `defer restore()` immediately
// after SetForTest, and if you stack multiple overrides on the same Var,
// restore them in the exact reverse order you applied them. Each restore
// closure only knows the single (overridden, isSet) snapshot it captured at
// its own SetForTest call; restoring out of order re-stores a stale snapshot
// over the state belonging to a still-active later override, corrupting it.
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
//
// Restores must be strictly LIFO per Var: call `defer restore()` immediately
// after SetForTest, and if you stack multiple overrides on the same Var,
// restore them in the exact reverse order you applied them. Each restore
// closure only knows the single (overridden, isSet) snapshot it captured at
// its own SetForTest call; restoring out of order re-stores a stale snapshot
// over the state belonging to a still-active later override, corrupting it.
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
//
// Restores must be strictly LIFO per Var: call `defer restore()` immediately
// after SetForTest, and if you stack multiple overrides on the same Var,
// restore them in the exact reverse order you applied them. Each restore
// closure only knows the single (overridden, isSet) snapshot it captured at
// its own SetForTest call; restoring out of order re-stores a stale snapshot
// over the state belonging to a still-active later override, corrupting it.
func (v *DurVar) SetForTest(d time.Duration) func() {
	prev, prevOverridden, prevSet := v.val.Load(), v.overridden.Swap(true), v.isSet.Swap(true)
	v.val.Store(int64(d))
	return func() {
		v.val.Store(prev)
		v.overridden.Store(prevOverridden)
		v.isSet.Store(prevSet)
	}
}
