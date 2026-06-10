//go:build !test && !js

package audio

// Cross-platform golden baseline for the insert-effect C DSP (insert_fx.c).
//
// The same C source is compiled twice: natively into build/libdrums.a (this
// package's cgo wrappers — the desktop audio path) and via Emscripten into
// src/js/drums.single.js (the browser AudioWorklet path,
// src/js/insert_fx_worklet.js). Until this test there was NO check that the
// two builds produce the same audio for the same effect + params — per-effect
// functional tests existed on both sides, but a drift (param-mapping bug,
// compiler flag change, libm divergence) would ship silently.
//
// This test renders a deterministic input through every registered effect at
// two param sets (catalog defaults + a "hot" aggressive set) and locks the
// native output in testdata/ifx_xplat_golden.json — full PCM, not just a
// hash, so the browser side (src/js/xplat_insert_fx_parity.browser.test.js)
// can diff per-sample against the SAME file and report bit-exact vs
// tolerance-grade per effect (native cc may contract a*b+c into FMA on
// x86 and links glibc libm; wasm has no FMA and uses musl-derived libm, so
// some effects are close-but-not-bit-identical by construction — the same
// reality the modular xplat suite documents as "correlation-grade").
//
// Processing is chunked at 128 samples to match the AudioWorklet quantum so
// both platforms exercise identical block boundaries.
//
// Regeneration protocol (intentional DSP changes only):
//
//	cd src/go && IFX_GOLDEN_UPDATE=1 xvfb-run -a ../../.tools/go/bin/go \
//	  test ./internal/audio/ -run TestInsertFXXplatGolden -v
//
// then re-run without the env var and commit the JSON in the same change.

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"sort"
	"testing"
)

const (
	ifxGoldenSR      = 48000
	ifxGoldenSamples = 8192 // ~170ms — long enough for delay (hot: 50ms), reverb early reflections, LFO movement
	ifxGoldenChunk   = 128  // AudioWorklet render quantum
	ifxGoldenPath    = "testdata/ifx_xplat_golden.json"
)

// ifxGoldenInput generates the deterministic test signal: a full-scale
// integer-LCG noise burst. Mirrored EXACTLY (same integer arithmetic, same
// float64 divide, same float32 rounding) by makeInput() in
// src/js/xplat_insert_fx_parity.browser.test.js — change one, change both.
func ifxGoldenInput(n int) []float32 {
	out := make([]float32, n)
	state := uint32(12345)
	for i := range out {
		state = (1103515245*state + 12345) & 0x7fffffff
		out[i] = float32(float64(state)/2147483647.0*2.0 - 1.0)
	}
	return out
}

// ifxGoldenHotParams holds the aggressive per-effect settings (borrowed from
// the worklet functional suite) so the golden exercises the wet path — at
// catalog defaults several effects are mostly-dry or sub-threshold. Delay
// time is shortened to 50ms so the first echoes land inside the window.
var ifxGoldenHotParams = map[EffectType]map[string]float64{
	EffectDistortion: {"drive": 15, "tone": 4000, "mix": 1},
	EffectDelay:      {"time": 50, "feedback": 0.6, "mix": 0.8},
	EffectReverb:     {"room": 0.8, "damping": 0.5, "mix": 0.8},
	EffectChorus:     {"rate": 3, "depth": 10, "mix": 0.8},
	EffectBitcrusher: {"bits": 4, "rate": 0.2, "mix": 1},
	EffectFilter:     {"mode": 0, "cutoff": 200, "q": 2, "mix": 1},
	EffectPhaser:     {"stages": 8, "rate": 2, "depth": 1, "feedback": 0.8, "mix": 1},
	EffectFlanger:    {"rate": 2, "depth": 5, "feedback": 0.8, "mix": 1},
	EffectTremolo:    {"rate": 8, "depth": 1, "shape": 0, "mix": 1},
	EffectGate:       {"threshold": -10, "attack": 1, "release": 10, "range": -90},
	EffectLimiter:    {"threshold": -20, "release": 50, "ceiling": -6},
	EffectRingMod:    {"frequency": 300, "shape": 0, "mix": 1},
	EffectWaveshaper: {"curve": 2, "drive": 10, "mix": 1},
	EffectAutoWah:    {"sensitivity": 1, "rate": 5, "depth": 1, "mix": 1},
	EffectCompressor: {"threshold": -30, "ratio": 20, "attack": 0.1, "release": 50, "makeup": 12, "mix": 1},
	EffectTransient:  {"attack": 200, "sustain": 30, "speed": 5},
	EffectTape:       {"drive": 8, "warmth": 0.8, "wow": 0.5, "flutter": 0.5, "mix": 1},
	EffectPitchShift: {"pitch": 12, "mix": 1, "window": 50},
}

type ifxGoldenCase struct {
	Name   string             `json:"name"`
	Params map[string]float64 `json:"params"`
	SHA256 string             `json:"sha256"`
	PCM    string             `json:"pcm_f32le_b64"`
}

type ifxGoldenDoc struct {
	SampleRate int                        `json:"sample_rate"`
	Samples    int                        `json:"samples"`
	Chunk      int                        `json:"chunk"`
	Effects    map[string][]ifxGoldenCase `json:"effects"`
}

type ifxBlockProcessor interface {
	ProcessBlockBuf(in, out []float32, samples int)
}

func renderIFXGolden(t *testing.T, typ EffectType, params map[string]float64) []float32 {
	t.Helper()
	eff := NewEffectProcessor(EffectSlot{Type: typ, Enabled: true, Params: params}, ifxGoldenSR)
	if _, isPass := eff.(*passThrough); isPass {
		t.Fatalf("effect %q resolved to passThrough — registry missing native constructor", typ)
	}
	in := ifxGoldenInput(ifxGoldenSamples)
	out := make([]float32, ifxGoldenSamples)
	bp, ok := eff.(ifxBlockProcessor)
	if !ok {
		t.Fatalf("effect %q does not implement ProcessBlockBuf", typ)
	}
	for off := 0; off < ifxGoldenSamples; off += ifxGoldenChunk {
		end := off + ifxGoldenChunk
		bp.ProcessBlockBuf(in[off:end], out[off:end], ifxGoldenChunk)
	}
	return out
}

func ifxPCMBytes(buf []float32) []byte {
	b := make([]byte, len(buf)*4)
	for i, v := range buf {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(v))
	}
	return b
}

func TestInsertFXXplatGolden(t *testing.T) {
	regs := EffectRegistrations()
	types := make([]string, 0, len(regs))
	for typ := range regs {
		types = append(types, string(typ))
	}
	sort.Strings(types)

	doc := ifxGoldenDoc{
		SampleRate: ifxGoldenSR,
		Samples:    ifxGoldenSamples,
		Chunk:      ifxGoldenChunk,
		Effects:    map[string][]ifxGoldenCase{},
	}
	for _, name := range types {
		typ := EffectType(name)
		cases := []ifxGoldenCase{
			{Name: "defaults", Params: DefaultParams(typ)},
		}
		if hot, ok := ifxGoldenHotParams[typ]; ok {
			// Store the MERGED param map (hot over catalog defaults), not the
			// raw hot map: the browser test must receive every init argument
			// explicitly rather than relying on the worklet-mirror's `??`
			// fallbacks staying in sync with the Go catalog (the same drift
			// class the chain-spec parity work fixed by emitting merged
			// params).
			cases = append(cases, ifxGoldenCase{Name: "hot", Params: mergeDefaults(typ, hot)})
		} else {
			t.Errorf("effect %q has no hot-params entry — add one to ifxGoldenHotParams", typ)
		}
		for i := range cases {
			out := renderIFXGolden(t, typ, cases[i].Params)
			pcm := ifxPCMBytes(out)
			sum := sha256.Sum256(pcm)
			cases[i].SHA256 = hex.EncodeToString(sum[:])
			cases[i].PCM = base64.StdEncoding.EncodeToString(pcm)
		}
		doc.Effects[name] = cases
	}

	if os.Getenv("IFX_GOLDEN_UPDATE") == "1" {
		data, err := json.MarshalIndent(doc, "", " ")
		if err != nil {
			t.Fatalf("marshal golden: %v", err)
		}
		if err := os.WriteFile(ifxGoldenPath, data, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("wrote %s (%d effects × 2 cases, %d samples each)", ifxGoldenPath, len(types), ifxGoldenSamples)
		return
	}

	raw, err := os.ReadFile(ifxGoldenPath)
	if err != nil {
		t.Fatalf("read golden (regenerate with IFX_GOLDEN_UPDATE=1): %v", err)
	}
	var want ifxGoldenDoc
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatalf("parse golden: %v", err)
	}
	if want.SampleRate != doc.SampleRate || want.Samples != doc.Samples || want.Chunk != doc.Chunk {
		t.Fatalf("golden config mismatch: file (sr=%d n=%d chunk=%d) vs test (sr=%d n=%d chunk=%d) — regenerate",
			want.SampleRate, want.Samples, want.Chunk, doc.SampleRate, doc.Samples, doc.Chunk)
	}
	for _, name := range types {
		got, gotOK := doc.Effects[name]
		wantCases, wantOK := want.Effects[name]
		if !wantOK {
			t.Errorf("effect %q missing from golden file — regenerate with IFX_GOLDEN_UPDATE=1", name)
			continue
		}
		_ = gotOK
		if len(got) != len(wantCases) {
			t.Errorf("effect %q: case count drifted (got %d, golden %d)", name, len(got), len(wantCases))
			continue
		}
		for i := range got {
			if got[i].SHA256 != wantCases[i].SHA256 {
				t.Errorf("effect %q case %q: native render drifted from golden (got sha %s, want %s) — "+
					"if the DSP change is intentional, regenerate with IFX_GOLDEN_UPDATE=1 in the same change",
					name, got[i].Name, got[i].SHA256, wantCases[i].SHA256)
			}
		}
	}
}
