//go:build !test && !js

package audio

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Dump every built-in's render before a change and after: each must be BIT-IDENTICAL unless the step allows a bounded
// change (BEATMO_RENDER_PARITY_MIN_SNR_DB). Skips unless BEATMO_RENDER_PARITY_DIR is set; dumps never live in the repo.

func renderParityDir(t *testing.T) string {
	d := os.Getenv("BEATMO_RENDER_PARITY_DIR")
	if d == "" {
		t.Skip("BEATMO_RENDER_PARITY_DIR unset")
	}
	return d
}

func TestRenderParityDump(t *testing.T) {
	dir := renderParityDir(t)
	label := os.Getenv("BEATMO_RENDER_PARITY_LABEL")
	if label == "" {
		t.Fatal("BEATMO_RENDER_PARITY_LABEL unset")
	}
	out := filepath.Join(dir, label)
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	// Rounding to float32 before the double block reproduces a float block's delivery, proving the type switch alone is
	// bit-exact.
	if os.Getenv("BEATMO_RENDER_PARITY_ROUND_F32") != "" {
		renderModularRoundParamsToFloat32 = true
		defer func() { renderModularRoundParamsToFloat32 = false }()
	}
	Reset()
	ResetInstruments()
	ids := append([]instrument.ID(nil), roster.All...)
	slices.Sort(ids)
	for _, id := range ids {
		buf, sr := RenderInstrumentOneShotRaw(id)
		b := make([]byte, 4+4*len(buf))
		binary.LittleEndian.PutUint32(b, uint32(sr))
		for i, v := range buf {
			binary.LittleEndian.PutUint32(b[4+4*i:], math.Float32bits(v))
		}
		if err := os.WriteFile(filepath.Join(out, string(id)+".f32"), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cases := map[string]map[string]string{}
	nCases := 0
	for _, id := range ids {
		cases[string(id)] = renderParityCases(id)
		nCases += len(cases[string(id)])
	}
	b, err := json.MarshalIndent(cases, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "cases.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("dumped %d instruments + %d knob/pitch cases to %s", len(ids), nCases, out)
}

// As float32, what the engine emits.
func parityHash(x []float64) string {
	h := sha256.New()
	var b [4]byte
	for _, v := range x {
		binary.LittleEndian.PutUint32(b[:], math.Float32bits(float32(v)))
		h.Write(b[:])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Melodic voices at −12/−5/+7/+12 and every visible knob at its extremes.
func renderParityCases(id instrument.ID) map[string]string {
	out := map[string]string{}
	if docMelodic(id) {
		sr := SampleRate()
		for _, semi := range []int{-12, -5, 7, 12} {
			v, ok := tryRecipeVoicePitched(id, sr, float64(semi))
			if !ok || v == nil {
				continue
			}
			var x []float64
			for i := 0; i < sr; i++ {
				smp, done := v.Sample()
				x = append(x, smp)
				if done {
					break
				}
			}
			out[fmt.Sprintf("pitch%+d", semi)] = parityHash(x)
		}
	}
	for k, v := range engineParityCases(id) {
		out[k] = v
	}
	return out
}

// The path every note takes, 250 ms. ok=false when the document's modules cannot take the field.
func engineCaseRender(id, field string, v float64) ([]float32, bool) {
	d, ok := EnsureInstrumentDoc(instrument.ID(id))
	if !ok {
		return nil, false
	}
	reg := BuiltinModuleRegistry()
	d2, err := instrument.SetField(d, reg, field, v)
	if err != nil {
		return nil, false
	}
	return renderBlockForTest(instrument.Flatten(d2, reg), SampleRate()/4), true
}

// Min, Max and 0 when it lies in range, even when the document's value is 0: exactly where a "0 means the C default"
// fallback hides.
func engineParityCases(id instrument.ID) map[string]string {
	out := map[string]string{}
	defs := InstrumentParamDefs(id)
	if len(defs) == 0 {
		return out
	}
	merged := MergedInstrumentParamsRO(id)
	ident := modularFieldInits()
	hash := func(buf []float32) string {
		f := make([]float64, len(buf))
		for i, s := range buf {
			f[i] = float64(s)
		}
		return parityHash(f)
	}
	put := func(key, field string, v float64) {
		if buf, ok := engineCaseRender(string(id), field, v); ok {
			out[key] = hash(buf)
		}
	}
	for _, d := range defs {
		if d.Name == "noise_seed" {
			continue
		}
		if d.Group == SynthHiddenGroup && merged[d.Name] == ident[d.Name] {
			continue
		}
		put("engine:"+d.Name+"@min", d.Name, d.Min)
		put("engine:"+d.Name+"@max", d.Name, d.Max)
		if d.Min <= 0 && 0 <= d.Max {
			put("engine:"+d.Name+"@zero", d.Name, 0)
		}
	}
	return out
}

// parityGlobs reads a comma list of path.Match globs from env.
func parityGlobs(env string) []string {
	var g []string
	for _, x := range strings.Split(os.Getenv(env), ",") {
		if x = strings.TrimSpace(x); x != "" {
			g = append(g, x)
		}
	}
	return g
}

func matchesAny(globs []string, s string) bool {
	for _, g := range globs {
		if ok, _ := path.Match(g, s); ok {
			return true
		}
	}
	return false
}

// "engine:gen1_x@min" → "gen1_x"; "pitch+7" → "".
func caseParam(c string) string {
	c = strings.TrimPrefix(c, "engine:")
	if i := strings.LastIndex(c, "@"); i > 0 {
		return c[:i]
	}
	return ""
}

func readParity(t *testing.T, path string) (int, []float32) {
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sr := int(binary.LittleEndian.Uint32(b))
	n := (len(b) - 4) / 4
	out := make([]float32, n)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[4+4*i:]))
	}
	return sr, out
}

func TestRenderParityCompare(t *testing.T) {
	dir := renderParityDir(t)
	a, b := os.Getenv("BEATMO_RENDER_PARITY_A"), os.Getenv("BEATMO_RENDER_PARITY_B")
	if a == "" || b == "" {
		t.Fatal("BEATMO_RENDER_PARITY_A / _B unset")
	}
	minSNR := math.Inf(1)
	if v := os.Getenv("BEATMO_RENDER_PARITY_MIN_SNR_DB"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			t.Fatal(err)
		}
		minSNR = f
	}
	files, _ := filepath.Glob(filepath.Join(dir, a, "*.f32"))
	if len(files) == 0 {
		t.Fatalf("no dumps under %s", filepath.Join(dir, a))
	}
	identical, changed := 0, 0
	for _, fa := range files {
		id := filepath.Base(fa[:len(fa)-4])
		fb := filepath.Join(dir, b, id+".f32")
		if _, err := os.Stat(fb); err != nil {
			t.Errorf("%s: missing from %s (instrument lost)", id, b)
			continue
		}
		srA, xa := readParity(t, fa)
		srB, xb := readParity(t, fb)
		if srA != srB || len(xa) != len(xb) {
			t.Errorf("%s: shape changed sr %d→%d len %d→%d", id, srA, srB, len(xa), len(xb))
			continue
		}
		var sig, noise float64
		same := true
		for i := range xa {
			if math.Float32bits(xa[i]) != math.Float32bits(xb[i]) {
				same = false
			}
			d := float64(xa[i]) - float64(xb[i])
			sig += float64(xa[i]) * float64(xa[i])
			noise += d * d
		}
		if same {
			identical++
			continue
		}
		changed++
		snr := 10 * math.Log10(sig/(noise+1e-300))
		if snr < minSNR {
			t.Errorf("%s: SNR %.1f dB < %.1f dB (not bit-identical)", id, snr, minSNR)
		} else {
			t.Logf("%s: changed, SNR %.1f dB", id, snr)
		}
	}
	t.Logf("%d bit-identical, %d changed", identical, changed)
	if onlyB, _ := filepath.Glob(filepath.Join(dir, b, "*.f32")); len(onlyB) > len(files) {
		t.Logf("%d instruments exist only in %s (new instruments, not compared)", len(onlyB)-len(files), b)
	}
	compareParityCases(t, filepath.Join(dir, a, "cases.json"), filepath.Join(dir, b, "cases.json"), !math.IsInf(minSNR, 1))
}

// A changed case is an error unless bounded changes are allowed; a case missing from B is always one (coverage lost).
// _REKEYED names instruments whose knob cases key on different names; _DROPPED globs params removed on purpose;
// _ALLOW globs "<id> <case>" a step means to change.
func compareParityCases(t *testing.T, pa, pb string, allowChanges bool) {
	dropped, allow := parityGlobs("BEATMO_RENDER_PARITY_DROPPED"), parityGlobs("BEATMO_RENDER_PARITY_ALLOW")
	nDropped, nAllowed := 0, 0
	rekeyed := map[string]bool{}
	for _, id := range strings.Split(os.Getenv("BEATMO_RENDER_PARITY_REKEYED"), ",") {
		if id = strings.TrimSpace(id); id != "" {
			rekeyed[id] = true
		}
	}
	load := func(p string) map[string]map[string]string {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		var m map[string]map[string]string
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("parse %s: %v", p, err)
		}
		return m
	}
	A, B := load(pa), load(pb)
	same, changed, lost, added := 0, 0, 0, 0
	ids := make([]string, 0, len(A))
	for id := range A {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		cb, ok := B[id]
		if rekeyed[id] {
			t.Logf("cases: %s re-keyed (recipe changed): %d cases in A, %d in B", id, len(A[id]), len(cb))
			continue
		}
		if !ok {
			t.Errorf("cases: %s missing from B", id)
			continue
		}
		names := make([]string, 0, len(A[id]))
		for c := range A[id] {
			names = append(names, c)
		}
		sort.Strings(names)
		for _, c := range names {
			hb, ok := cb[c]
			switch {
			case !ok && matchesAny(dropped, caseParam(c)):
				nDropped++
			case !ok:
				lost++
				t.Errorf("cases: %s %s missing from B (coverage lost)", id, c)
			case hb == A[id][c]:
				same++
			case matchesAny(allow, id+" "+c):
				nAllowed++
			default:
				changed++
				if !allowChanges {
					t.Errorf("cases: %s %s changed (not bit-identical)", id, c)
				}
			}
		}
		for c := range cb {
			if _, ok := A[id][c]; !ok {
				added++
			}
		}
	}
	t.Logf("cases: %d bit-identical, %d changed, %d allowed-changed, %d lost, %d dropped, %d new", same, changed, nAllowed, lost, nDropped, added)
}

// Through the C engine, at Min, Max and 0.
func TestEngineParityCasesCoverSeedAndVisibleParams(t *testing.T) {
	cases := engineParityCases(roster.Hihat)
	for _, want := range []string{
		"engine:gen1_cym_rattle@zero", // visible modal knob, 0 in range
		"engine:gen1_cym_rattle@max",
		"engine:post_decay_rate@min", // the seed sets it (20)
	} {
		if _, ok := cases[want]; !ok {
			t.Errorf("engine case %q missing (have %d cases)", want, len(cases))
		}
	}
	again := engineParityCases(roster.Hihat)
	for k, h := range cases {
		if again[k] != h {
			t.Errorf("%s not deterministic", k)
		}
	}
}
