//go:build !test && !js

package audio

import (
	"math"
	"sync"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

type testMode struct{ f, alpha, amp, phase float64 }

// modalDoc builds a minimal document whose sound is exactly modes (Hz mode,
// decay law a=1 p=0 b=0 so a row's decay IS its alpha, strike off).
func modalDoc(t testing.TB, modes []testMode) instrument.Doc {
	t.Helper()
	reg := BuiltinModuleRegistry()
	d := instrument.Doc{ID: "modal-test", Name: "modal-test", Render: instrument.Render{Seconds: 1, Amplitude: 1},
		Stages: map[string]instrument.Stage{}}
	for _, typ := range []string{"core", "finish"} {
		m, err := instrument.NewModule(reg, typ, true)
		if err != nil {
			t.Fatal(err)
		}
		d.Stages[typ] = instrument.Stage{V: m.V, On: true, Values: m.Values}
	}
	m, err := instrument.NewModule(reg, "modal", true)
	if err != nil {
		t.Fatal(err)
	}
	m.Values["modal_freq_mode"] = 1
	d.Stages["modal"] = instrument.Stage{V: m.V, On: true, Values: m.Values}
	tb := instrument.Table{Cols: []string{"freq", "decay", "amp", "phase", "attack"}}
	for _, md := range modes {
		tb.Rows = append(tb.Rows, []float64{md.f, md.alpha, md.amp, md.phase, 0})
	}
	d, err = instrument.SetTable(d, reg, "modal", tb)
	if err != nil {
		t.Fatal(err)
	}
	if err := instrument.Validate(d, reg); err != nil {
		t.Fatal(err)
	}
	return d
}

func renderDocRaw(d instrument.Doc, sr, n int) []float32 {
	buf := make([]float32, n)
	renderModularBlock(buf, sr, n, modularBlock(instrument.Flatten(d, BuiltinModuleRegistry())))
	return buf
}

// TestModalBankMatchesTheModeModel: the C bank renders Σ A e^{-αt} cos(2πft+φ)
// — the clone analysis' Mode convention — to float precision.
func TestModalBankMatchesTheModeModel(t *testing.T) {
	modes := []testMode{{440, 2, 0.4, 0.3}, {1210, 4.5, 0.2, -1.1}, {2380, 9, 0.12, 2}, {3950, 16, 0.06, 0.7}}
	const sr, n = 48000, 48000
	got := renderDocRaw(modalDoc(t, modes), sr, n)
	var worst float64
	for j := 0; j < n; j++ {
		tt := float64(j) / sr
		var want float64
		for _, m := range modes {
			want += m.amp * math.Exp(-m.alpha*tt) * math.Cos(2*math.Pi*m.f*tt+m.phase)
		}
		worst = math.Max(worst, math.Abs(float64(got[j])-want))
	}
	t.Logf("worst deviation from the mode model: %.3g", worst)
	if worst > 1e-5 {
		t.Fatalf("bank deviates from the mode model by %.3g", worst)
	}
}

func TestModalStagesAreExactBypassesWhenOff(t *testing.T) {
	d := modalDoc(t, []testMode{{440, 2, 0.4, 0}})
	off := d.Clone()
	st := off.Stages["modal"]
	st.On = false
	off.Stages["modal"] = st
	for _, v := range renderDocRaw(off, 48000, 4800) {
		if v != 0 {
			t.Fatal("a disabled modal stage made sound")
		}
	}
}

// TestStrikeAtIdentityIsBitIdentical: strike ON with tc=0, hardness 0, click 0
// leaves the bank untouched (the identity rows are stored against).
func TestStrikeAtIdentityIsBitIdentical(t *testing.T) {
	reg := BuiltinModuleRegistry()
	d := modalDoc(t, []testMode{{440, 2, 0.4, 0.3}, {2000, 8, 0.1, 1}})
	m, err := instrument.NewModule(reg, "strike", true)
	if err != nil {
		t.Fatal(err)
	}
	with := d.Clone()
	with.Stages["strike"] = instrument.Stage{V: m.V, On: true, Values: m.Values}
	a, b := renderDocRaw(d, 48000, 9600), renderDocRaw(with, 48000, 9600)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("identity strike moved sample %d", i)
		}
	}
	with.Stages["strike"].Values["strike_tc"] = 0.002
	c := renderDocRaw(with, 48000, 9600)
	same := true
	for i := range a {
		if a[i] != c[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("a 2 ms contact changed nothing")
	}
}

func TestModalSkipsModesAboveTheLimit(t *testing.T) {
	d := modalDoc(t, []testMode{{22000, 2, 0.4, 0}}) // above 0.45·48k
	for _, v := range renderDocRaw(d, 48000, 4800) {
		if v != 0 {
			t.Fatal("a mode above 0.45·sr was rendered (would alias)")
		}
	}
}

// TestModalTranspositionFollowsTheDecayLaw: in ratio mode a higher note puts
// every mode higher AND, through the law, makes it ring shorter.
func TestModalTranspositionFollowsTheDecayLaw(t *testing.T) {
	d := modalDoc(t, []testMode{{1, 1, 0.5, 0}})
	vals := d.Stages["modal"].Values
	vals["modal_freq_mode"] = 0
	vals["modal_law_a"] = 2
	vals["modal_law_p"] = 1
	energyAfter := func(pitch float64) float64 {
		b := instrument.Flatten(d, BuiltinModuleRegistry())
		b[pitchFieldIndex()] = pitch
		buf := make([]float32, 48000)
		renderModularBlock(buf, 48000, 48000, modularBlock(b))
		var e float64
		for _, v := range buf[24000:] {
			e += float64(v) * float64(v)
		}
		return e
	}
	if !(energyAfter(12) < energyAfter(0)) {
		t.Fatal("an octave up must decay faster under a rising law")
	}
}

// TestNoisebandRowLevelIsItsRMS: one row with no decay renders at its level.
func TestNoisebandRowLevelIsItsRMS(t *testing.T) {
	reg := BuiltinModuleRegistry()
	d := modalDoc(t, nil)
	m, err := instrument.NewModule(reg, "noiseband", true)
	if err != nil {
		t.Fatal(err)
	}
	d.Stages["noiseband"] = instrument.Stage{V: m.V, On: true, Values: m.Values}
	d, err = instrument.SetTable(d, reg, "noiseband", instrument.Table{
		Cols: []string{"center", "q", "level", "attack", "fast_rate", "fast_mix", "tail_rate"},
		Rows: [][]float64{{2000, 4.318, -20, 0, 0, 0, 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	buf := renderDocRaw(d, 48000, 96000)
	var s float64
	for _, v := range buf[4800:] {
		s += float64(v) * float64(v)
	}
	rmsDB := 10 * math.Log10(s/float64(len(buf)-4800))
	t.Logf("row at −20 dB renders at %.3f dB RMS", rmsDB)
	if math.Abs(rmsDB-(-20)) > 0.75 {
		t.Fatalf("row at −20 dB renders at %.2f dB RMS", rmsDB)
	}
}

func TestModalRendersAreDeterministicAndConcurrent(t *testing.T) {
	d := modalDoc(t, []testMode{{440, 2, 0.4, 0.3}, {1210, 4.5, 0.2, -1.1}})
	reg := BuiltinModuleRegistry()
	nb, _ := instrument.NewModule(reg, "noiseband", true)
	d.Stages["noiseband"] = instrument.Stage{V: nb.V, On: true, Values: nb.Values}
	d, err := instrument.SetTable(d, reg, "noiseband", instrument.Table{
		Cols: []string{"center", "q", "level", "attack", "fast_rate", "fast_mix", "tail_rate"},
		Rows: [][]float64{{3000, 4, -30, 0.002, 60, 0.7, 6}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := renderDocRaw(d, 48000, 24000)
	var wg sync.WaitGroup
	errs := make(chan string, 8)
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := renderDocRaw(d, 48000, 24000)
			for i := range got {
				if got[i] != want[i] {
					errs <- "concurrent render differs"
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
}

// TestModalStagesAreLive: every declared value and every table column of the
// three stages moves the render in a context where its stage plays (the
// shipped-instrument liveness test cannot see them yet — no clone ships).
func TestModalStagesAreLive(t *testing.T) {
	reg := BuiltinModuleRegistry()
	base := modalDoc(t, []testMode{{440, 2, 0.4, 0.3}, {1210, 4.5, 0.2, -1.1}, {2380, 9, 0.12, 2}})
	sk, _ := instrument.NewModule(reg, "strike", true)
	base.Stages["strike"] = instrument.Stage{V: sk.V, On: true, Values: sk.Values}
	base.Stages["strike"].Values["strike_tc"] = 0.001
	nb, _ := instrument.NewModule(reg, "noiseband", true)
	base.Stages["noiseband"] = instrument.Stage{V: nb.V, On: true, Values: nb.Values}
	base, err := instrument.SetTable(base, reg, "noiseband", instrument.Table{
		Cols: []string{"center", "q", "level", "attack", "fast_rate", "fast_mix", "tail_rate"},
		Rows: [][]float64{{3000, 4, -30, 0.002, 60, 0.7, 6}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ref := renderDocRaw(base, 48000, 24000)
	moved := func(d instrument.Doc) bool {
		got := renderDocRaw(d, 48000, 24000)
		for i := range got {
			if got[i] != ref[i] {
				return true
			}
		}
		return false
	}
	alt := map[string]float64{
		"modal_tune": 50, "modal_stretch": 0.05, "modal_decay_x": 2, "modal_bright": 3, "modal_keep": 1,
		"modal_law_a": 2, "modal_law_p": 1, "modal_law_b": 50, "modal_freq_mode": 0,
		"strike_tc": 0.003, "strike_hardness": 3, "strike_click": 0.5, "noiseband_level": 6,
	}
	for _, typ := range []string{"modal", "strike", "noiseband"} {
		decl, _ := reg.Type(typ)
		for _, v := range decl.Values {
			x, ok := alt[v.Name]
			if !ok {
				t.Fatalf("no perturbation for %s.%s", typ, v.Name)
			}
			d := base.Clone()
			d.Stages[typ].Values[v.Name] = x
			if !moved(d) {
				t.Errorf("%s.%s = %v changes nothing", typ, v.Name, x)
			}
		}
		if decl.Table == nil {
			continue
		}
		for c, col := range decl.Table.Columns {
			d := base.Clone()
			row := d.Stages[typ].Table.Rows[0]
			nv := row[c]*1.1 + 0.001
			if nv > col.Max {
				nv = row[c] * 0.9
			}
			row[c] = nv
			if !moved(d) {
				t.Errorf("%s.table.%s changes nothing", typ, col.Name)
			}
		}
	}
}

func BenchmarkModalBank128x3s(b *testing.B) {
	var modes []testMode
	for i := 0; i < 128; i++ {
		modes = append(modes, testMode{f: 100 + 120*float64(i), alpha: 1 + 0.3*float64(i), amp: 0.01, phase: math.Mod(float64(i), 6)})
	}
	d := modalDoc(b, modes)
	block := modularBlock(instrument.Flatten(d, BuiltinModuleRegistry()))
	buf := make([]float32, 3*48000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		renderModularBlock(buf, 48000, len(buf), block)
	}
}

// TestModalModesMatchTheEngine: ModalModes (the Go mirror the Synth tab's mode
// graph and the WAV-clone tooling read) sums to what the C bank renders, with
// every macro off its identity: ratio rows, stretch, tune, Decay ×,
// Brightness, a decay law and the strike.
func TestModalModesMatchTheEngine(t *testing.T) {
	d := modalDoc(t, []testMode{{1, 1.2, 0.4, 0.3}, {2.76, 0.9, 0.2, -1.1}, {5.4, 1.1, 0.12, 2}, {8.93, 1, 0.06, 0.7}})
	mv := d.Stages["modal"].Values
	mv["modal_freq_mode"], mv["modal_tune"], mv["modal_stretch"], mv["modal_decay_x"], mv["modal_bright"] = 0, 13, 0.02, 1.3, -1.5
	mv["modal_law_a"], mv["modal_law_p"], mv["modal_law_b"] = 2, 0.7, 30
	reg := BuiltinModuleRegistry()
	var err error
	if d, err = instrument.AddStage(d, reg, "strike", true); err != nil {
		t.Fatal(err)
	}
	sv := d.Stages["strike"].Values
	sv["strike_tc"], sv["strike_hardness"] = 0.0009, -2
	const sr, n = 48000, 48000
	got := renderDocRaw(d, sr, n)
	modes := ModalModes(FlatFieldMap(d), 0, false, sr)
	if len(modes) != 4 {
		t.Fatalf("%d modes", len(modes))
	}
	var worst float64
	for j := 0; j < n; j++ {
		tt := float64(j) / sr
		var want float64
		for _, m := range modes {
			want += m.Amp * math.Exp(-m.Alpha*tt) * math.Cos(2*math.Pi*m.Freq*tt+m.Phase)
		}
		worst = math.Max(worst, math.Abs(float64(got[j])-want))
	}
	if worst > 1e-5 {
		t.Fatalf("ModalModes deviates from the C render by %.3g", worst)
	}
}
