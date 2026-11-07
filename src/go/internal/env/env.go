// Package env declares every environment variable the game reads, once (vars.go), and nothing else calls os.Getenv.
// Get reads a value cached on first access (some are read in draw and mix loops); Reload re-resolves after os.Setenv;
// on WASM the JS bootstrap fills go.env before go.run, so there is no build-tagged path.
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
	// Callers that must not clobber an existing value when the variable is absent guard on this.
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

// Test-only: the registry is process-global and permanent, so a test's scratch variable would stay in All() for the run.
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

// SetForTest overrides survive. Not safe concurrently with itself or SetForTest on one Var: isSet and val are two
// separate atomic stores, so an IsSet()+Get() pair mid-Reload may mix old and new. Get and IsSet alone are always safe.
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

// SetForTest marks the variable set, so a forced value behaves like a provided one.
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

// An empty value is unset everywhere.
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

// SetForTest forces a value (marked IsSet, surviving Reload) until the returned restore runs. Restores must be LIFO per
// Var: each knows only the snapshot it captured, so out of order it clobbers a still-active later override.
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

// The one truthy/falsy dialect; a present but unrecognised value falls back to def.
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

// SetForTest forces a value (marked IsSet, surviving Reload) until the returned restore runs. Restores must be LIFO per
// Var: each knows only the snapshot it captured, so out of order it clobbers a still-active later override.
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

// SetForTest forces a value (marked IsSet, surviving Reload) until the returned restore runs. Restores must be LIFO per
// Var: each knows only the snapshot it captured, so out of order it clobbers a still-active later override.
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

// SetForTest forces a value (marked IsSet, surviving Reload) until the returned restore runs. Restores must be LIFO per
// Var: each knows only the snapshot it captured, so out of order it clobbers a still-active later override.
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

// SetForTest forces a value (marked IsSet, surviving Reload) until the returned restore runs. Restores must be LIFO per
// Var: each knows only the snapshot it captured, so out of order it clobbers a still-active later override.
func (v *DurVar) SetForTest(d time.Duration) func() {
	prev, prevOverridden, prevSet := v.val.Load(), v.overridden.Swap(true), v.isSet.Swap(true)
	v.val.Store(int64(d))
	return func() {
		v.val.Store(prev)
		v.overridden.Store(prevOverridden)
		v.isSet.Store(prevSet)
	}
}
