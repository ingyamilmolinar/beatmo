package audio

import (
	"math"
	"testing"
)

// public_api_test.go covers the public audio APIs that have no current
// test coverage but represent real contract surfaces:
//
//   - MIDINoteForInstrument: GM percussion mapping consumed by export_audio
//   - passThrough: silent fallback when an effect type is missing from the
//     registry — a real bug if it ever swaps to "silenceProcessor"
//   - InsertEffectCatalog / DefaultParams / NewEffectProcessor: the only
//     way external packages introspect the effect registry
//   - RecordingDrops / IsRecording: status APIs used by perf overlays;
//     tested for safe-when-no-pipeline behavior
//   - clampf: utility used by every effect
//   - applyDefaultFX: instrument-side default FX processing path

// ─── MIDINoteForInstrument ────────────────────────────────────────────────

func TestMIDINoteForInstrumentMappedAndFallback(t *testing.T) {
	// Spot-check the well-known mappings; the contract is that any change
	// to MIDIMapping is a wire-format change that affects MIDI export.
	cases := []struct {
		id   string
		want int
	}{
		{"kick", 36},
		{"snare", 38},
		{"hihat", 42},
		{"clap", 39},
		{"tom", 45},
		{"cowbell", 56},
		{"fm-bell", 80},
		{"fm-lead", 81},
		{"sidestick", 37},
		{"rimshot", 37}, // alias of sidestick — same note
	}
	for _, c := range cases {
		if got := MIDINoteForInstrument(c.id); got != c.want {
			t.Errorf("MIDINoteForInstrument(%q)=%d want %d", c.id, got, c.want)
		}
	}

	// Unmapped IDs default to 38 (Acoustic Snare). Spec'd in the function
	// comment; documented as a fallback contract, not a coincidence.
	for _, unknown := range []string{"", "no-such-instrument", "fm-extra"} {
		if got := MIDINoteForInstrument(unknown); got != 38 {
			t.Errorf("MIDINoteForInstrument(%q)=%d want fallback 38", unknown, got)
		}
	}
}

// ─── passThrough fallback ─────────────────────────────────────────────────

func TestPassThroughIdentity(t *testing.T) {
	p := &passThrough{}

	// ProcessSample is identity over a representative sample range.
	for _, x := range []float64{-1.0, -0.5, 0, 0.25, 0.999999, 1.0} {
		if got := p.ProcessSample(x); got != x {
			t.Errorf("ProcessSample(%v)=%v want %v", x, got, x)
		}
	}

	// Reset and SetParam are no-ops; calling them must not panic.
	p.Reset()
	p.SetParam("anything", 42.0)
	p.SetParam("", 0)

	// ProcessBlockBuf copies in → out verbatim.
	in := []float32{0.1, -0.2, 0.3, -0.4}
	out := make([]float32, len(in))
	p.ProcessBlockBuf(in, out, len(in))
	for i := range in {
		if out[i] != in[i] {
			t.Errorf("ProcessBlockBuf[%d]=%v want %v", i, out[i], in[i])
		}
	}
}

func TestNewEffectProcessorUnknownTypeReturnsPassThrough(t *testing.T) {
	// Unknown EffectType → constructor must hand back a passThrough so
	// the audio chain stays *transparent* (not silent). A regression to
	// silenceProcessor would mute the offending channel.
	p := NewEffectProcessor(EffectSlot{Type: EffectType("__never_registered__")}, 44100)
	if _, ok := p.(*passThrough); !ok {
		t.Fatalf("got %T, want *passThrough", p)
	}
	// Identity check confirms it actually is silent-free.
	if got := p.ProcessSample(0.5); got != 0.5 {
		t.Errorf("unknown effect did not pass through: got %v", got)
	}
}

// ─── Effect registry introspection ────────────────────────────────────────

func TestInsertEffectCatalogIncludesRegisteredEffects(t *testing.T) {
	cat := InsertEffectCatalog()
	if len(cat) == 0 {
		t.Fatalf("InsertEffectCatalog is empty")
	}
	// Every entry must be in the order list and round-trip through
	// EffectTypeOrder. This locks the contract that the catalog and the
	// order list stay in sync.
	order := EffectTypeOrder()
	if len(order) != len(cat) {
		t.Errorf("EffectTypeOrder len=%d but catalog len=%d", len(order), len(cat))
	}
	seen := make(map[EffectType]bool, len(order))
	for _, t := range order {
		seen[t] = true
	}
	for k := range cat {
		if !seen[k] {
			t.Errorf("catalog has %q but EffectTypeOrder doesn't", k)
		}
	}

	// Every entry's Params slice must match what DefaultParams() yields.
	for typ, params := range cat {
		defs := DefaultParams(typ)
		if len(defs) != len(params) {
			t.Errorf("%s: len(DefaultParams)=%d, len(params)=%d", typ, len(defs), len(params))
			continue
		}
		for _, def := range params {
			if got, ok := defs[def.Name]; !ok {
				t.Errorf("%s: param %q missing from DefaultParams", typ, def.Name)
			} else if got != def.Default {
				t.Errorf("%s param %s: default mismatch %v vs %v", typ, def.Name, got, def.Default)
			}
		}
	}
}

func TestDefaultParamsUnknownTypeReturnsNil(t *testing.T) {
	if got := DefaultParams(EffectType("nope")); got != nil {
		t.Errorf("unknown effect type DefaultParams=%v want nil", got)
	}
}

func TestRegisterEffectUpdatesExistingNew(t *testing.T) {
	// RegisterEffect with the same Type twice must update the New
	// constructor without duplicating the catalog entry. This is how
	// platform-specific overrides (CGo vs WASM) replace the constructor
	// at init time.
	const sentinel = EffectType("__test_register__")
	t.Cleanup(func() {
		// Tests share package state; remove the sentinel so a re-run
		// starts from a clean registry.
		registryMu.Lock()
		delete(registryMap, sentinel)
		// Trim from the order slice if present.
		filtered := registryOrder[:0]
		for _, k := range registryOrder {
			if k != sentinel {
				filtered = append(filtered, k)
			}
		}
		registryOrder = filtered
		registryMu.Unlock()
	})

	calls := []string{}
	RegisterEffect(EffectRegistration{
		Type:        sentinel,
		DisplayName: "First",
		Params:      []EffectParamDef{{Name: "p", Default: 1}},
		New: func(sr int, params map[string]float64) InsertEffect {
			calls = append(calls, "first")
			return &passThrough{}
		},
	})
	RegisterEffect(EffectRegistration{
		Type:        sentinel,
		DisplayName: "ignored on second register",
		Params:      []EffectParamDef{{Name: "p", Default: 999}}, // ignored
		New: func(sr int, params map[string]float64) InsertEffect {
			calls = append(calls, "second")
			return &passThrough{}
		},
	})

	// The "second" New must be the live constructor; "first" no longer
	// exists.
	_ = NewEffectProcessor(EffectSlot{Type: sentinel}, 44100)
	if len(calls) != 1 || calls[0] != "second" {
		t.Errorf("calls=%v want [\"second\"]", calls)
	}

	// DisplayName/Params on the registry entry came from the *first*
	// registration (the override only swaps New).
	regs := EffectRegistrations()
	if reg, ok := regs[sentinel]; !ok || reg.DisplayName != "First" {
		t.Errorf("registry entry: %+v", reg)
	}
}

// ─── mergeDefaults ────────────────────────────────────────────────────────

func TestMergeDefaultsUserOverridesDefault(t *testing.T) {
	// Use any registered effect — we don't actually run it, just inspect
	// the param merge.
	order := EffectTypeOrder()
	if len(order) == 0 {
		t.Skip("no effects registered")
	}
	typ := order[0]
	defs := DefaultParams(typ)
	if len(defs) == 0 {
		t.Skip("first effect has no params")
	}
	// Pick one param key, override.
	var k string
	for k = range defs {
		break
	}
	user := map[string]float64{k: 999}
	merged := mergeDefaults(typ, user)
	if merged[k] != 999 {
		t.Errorf("user value did not override default: %v", merged[k])
	}
	// Other keys still come from defaults.
	for dk, dv := range defs {
		if dk == k {
			continue
		}
		if merged[dk] != dv {
			t.Errorf("default %q lost: %v want %v", dk, merged[dk], dv)
		}
	}
}

func TestMergeDefaultsUnknownEffect(t *testing.T) {
	// Unknown type → no defaults to merge. User values pass through.
	user := map[string]float64{"k": 1}
	got := mergeDefaults(EffectType("none"), user)
	if got["k"] != 1 {
		t.Errorf("user value lost: %v", got)
	}
}

// ─── applyDefaultFX (instrument-side default chain) ───────────────────────

func TestApplyDefaultFXNoOpsOnEmptySlots(t *testing.T) {
	buf := []float32{1, 2, 3, 4}
	want := []float32{1, 2, 3, 4}
	applyDefaultFX(buf, 44100, nil)
	for i := range buf {
		if buf[i] != want[i] {
			t.Errorf("buf[%d]=%v changed by no-op call", i, buf[i])
		}
	}
}

func TestApplyDefaultFXSkipsDisabledSlots(t *testing.T) {
	// When every slot is Enabled=false, the buffer is untouched.
	buf := []float32{0.1, 0.2, 0.3}
	want := append([]float32{}, buf...)
	applyDefaultFX(buf, 44100, []EffectSlot{
		{Type: EffectDistortion, Enabled: false},
		{Type: EffectDelay, Enabled: false},
	})
	for i := range buf {
		if buf[i] != want[i] {
			t.Errorf("buf[%d]=%v want %v (disabled slots changed buffer)", i, buf[i], want[i])
		}
	}
}

// ─── clampf ───────────────────────────────────────────────────────────────

func TestClampf(t *testing.T) {
	cases := []struct {
		v, lo, hi, want float64
	}{
		{0, -1, 1, 0},
		{-2, -1, 1, -1},
		{2, -1, 1, 1},
		{-1, -1, 1, -1}, // boundary
		{1, -1, 1, 1},   // boundary
		{0.5, 0, 1, 0.5},
	}
	for _, c := range cases {
		if got := clampf(c.v, c.lo, c.hi); got != c.want {
			t.Errorf("clampf(%v, %v, %v)=%v want %v", c.v, c.lo, c.hi, got, c.want)
		}
	}
}

// ─── RecordingDrops / IsRecording when no pipeline is active ──────────────

func TestRecordingStatusWhenIdle(t *testing.T) {
	// With no active pipeline, both APIs must return safe defaults
	// rather than panicking on a nil deref. This is what UI code calls
	// every frame.
	if IsRecording() {
		t.Errorf("IsRecording should be false when idle")
	}
	if got := RecordingDrops(); got != 0 {
		t.Errorf("RecordingDrops idle=%d want 0", got)
	}
	stats := CurrentPipelineStats()
	if stats.Active {
		t.Errorf("CurrentPipelineStats.Active should be false when idle")
	}
}

// ─── Channel public API ──────────────────────────────────────────────────

// withSavedMainVolume restores MainVolume and the platform notify callback
// after the test, so other tests in the package don't see leakage.
func withSavedMainVolume(t *testing.T) {
	t.Helper()
	prevVol := MainVolume()
	prevCB := platformChannelVolumeChanged
	t.Cleanup(func() {
		platformChannelVolumeChanged = prevCB
		SetMainVolume(prevVol)
	})
}

func TestChannelIDRoundTrip(t *testing.T) {
	// New (non-instrument) channels keep the id passed at construction.
	ch := newChannel("TestChannelIDRoundTrip", nil)
	if got := ch.ID(); got != "TestChannelIDRoundTrip" {
		t.Errorf("Channel.ID()=%q want %q", got, "TestChannelIDRoundTrip")
	}
}

func TestSetMainVolumeRoundTrip(t *testing.T) {
	withSavedMainVolume(t)

	for _, v := range []float64{0, 0.25, 0.5, 0.75, 1.0} {
		SetMainVolume(v)
		if got := MainVolume(); got != v {
			t.Errorf("MainVolume after SetMainVolume(%v)=%v want %v", v, got, v)
		}
	}
}

func TestSetMainVolumeClampsNegative(t *testing.T) {
	withSavedMainVolume(t)

	SetMainVolume(-0.5)
	if got := MainVolume(); got != 0 {
		t.Errorf("MainVolume after SetMainVolume(-0.5)=%v want 0 (negative clamps to 0)", got)
	}
}

func TestSetMainVolumeNaNAndInfFallback(t *testing.T) {
	withSavedMainVolume(t)

	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		SetMainVolume(v)
		if got := MainVolume(); got != 1 {
			t.Errorf("MainVolume after SetMainVolume(%v)=%v want 1 (NaN/Inf falls back)", v, got)
		}
	}
}

func TestSetMainVolumeFiresPlatformCallback(t *testing.T) {
	withSavedMainVolume(t)

	type call struct {
		id  string
		vol float64
	}
	var calls []call
	platformChannelVolumeChanged = func(id string, vol float64) {
		calls = append(calls, call{id, vol})
	}

	SetMainVolume(0.42)
	if len(calls) == 0 {
		t.Fatalf("SetMainVolume did not invoke platformChannelVolumeChanged")
	}
	last := calls[len(calls)-1]
	if last.id != mainChannelID {
		t.Errorf("callback id=%q want %q", last.id, mainChannelID)
	}
	if last.vol != 0.42 {
		t.Errorf("callback vol=%v want 0.42", last.vol)
	}
}

// ─── SetupMasterCompressor ───────────────────────────────────────────────

func TestSetupMasterCompressorAttachesToMaster(t *testing.T) {
	prev := masterCompressor
	t.Cleanup(func() { masterCompressor = prev })
	masterCompressor = nil

	SetupMasterCompressor(44100)

	if masterCompressor == nil {
		t.Fatalf("SetupMasterCompressor left masterCompressor nil")
	}

	master := chanMgr.ensureChannel(mainChannelID)
	master.mu.RLock()
	procs := append([]Processor(nil), master.processors...)
	master.mu.RUnlock()

	found := false
	for _, p := range procs {
		if p == masterCompressor {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("master channel processors do not contain masterCompressor (got %d procs)", len(procs))
	}
}
