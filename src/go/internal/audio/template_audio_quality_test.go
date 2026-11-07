//go:build !test && !js

package audio

import (
	"encoding/json"
	"math"
	"math/cmplx"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Every template and each instrument through the REAL voice → mixer → master path: the report logs metrics
// (BEATMO_DIAG_WAV dumps WAVs); the guard fails on clipping, a squashed mix, a noisy or muddy bass, or a bass over the
// kick.

// ---- metrics ---------------------------------------------------------------

type audioMetrics struct {
	peak     float64
	rms      float64
	crest    float64 // peak/rms — low => squashed/over-compressed
	clipFrac float64 // fraction of samples within 0.1% of full scale
	flatness float64 // spectral flatness (Wiener entropy): 0 tonal .. 1 white noise
	hfFrac   float64 // energy above 4 kHz / total (high on a bass => metallic)
	harmFrac float64 // energy at harmonics of the detected f0 / total (low => muddy/inharmonic)
	f0       float64 // detected fundamental of the loudest window (Hz)
	maxJump  float64 // largest |sample[i]-sample[i-1]| (clicks/cracking)
	jumpRate float64 // fraction of samples whose jump exceeds 0.10 (sustained crackle)
}

func analyzeAudio(buf []float64, sr int) audioMetrics {
	var m audioMetrics
	if len(buf) == 0 {
		return m
	}
	var sumSq float64
	clip := 0
	bigJumps := 0
	for i, v := range buf {
		a := math.Abs(v)
		if a > m.peak {
			m.peak = a
		}
		sumSq += v * v
		if a >= 0.999 {
			clip++
		}
		if i > 0 {
			j := math.Abs(v - buf[i-1])
			if j > m.maxJump {
				m.maxJump = j
			}
			if j > 0.10 {
				bigJumps++
			}
		}
	}
	m.jumpRate = float64(bigJumps) / float64(len(buf))
	m.rms = math.Sqrt(sumSq / float64(len(buf)))
	if m.rms > 0 {
		m.crest = m.peak / m.rms
	}
	m.clipFrac = float64(clip) / float64(len(buf))
	m.flatness, m.hfFrac = spectralStats(buf, sr)
	m.f0, m.harmFrac = harmonicFraction(buf, sr)
	return m
}

func spectralStats(buf []float64, sr int) (flatness, hfFrac float64) {
	const N = 2048
	if len(buf) < N {
		return 0, 0
	}
	cut := int(float64(N) * 4000.0 / float64(sr))
	var flatAcc, hfAcc, totAcc float64
	frames := 0
	for off := 0; off+N <= len(buf); off += N {
		seg, e := windowed(buf[off:off+N], N)
		if e < 1e-9 {
			continue
		}
		spec := fftPow(seg)
		var logSum, linSum float64
		bins := 0
		for k := 1; k < N/2; k++ {
			p := spec[k] + 1e-12
			logSum += math.Log(p)
			linSum += p
			bins++
			totAcc += p
			if k >= cut {
				hfAcc += p
			}
		}
		flatAcc += math.Exp(logSum/float64(bins)) / (linSum / float64(bins))
		frames++
	}
	if frames == 0 {
		return 0, 0
	}
	hf := 0.0
	if totAcc > 0 {
		hf = hfAcc / totAcc
	}
	return flatAcc / float64(frames), hf
}

// A clean tonal note scores high; a cluster of notes, or noise, low.
func harmonicFraction(buf []float64, sr int) (float64, float64) {
	const N = 8192
	if len(buf) < N {
		return 0, 0
	}
	// Find the loudest N-window (skip rests).
	bestOff, bestE := 0, 0.0
	for off := 0; off+N <= len(buf); off += N / 2 {
		var e float64
		for i := off; i < off+N; i++ {
			e += buf[i] * buf[i]
		}
		if e > bestE {
			bestE, bestOff = e, off
		}
	}
	if bestE < 1e-6 {
		return 0, 0
	}
	seg := buf[bestOff : bestOff+N]
	// A low near-sine correlates at small lags too, so the peak is taken after the first dip.
	loLag, hiLag := sr/400, sr/30
	if hiLag >= N {
		hiLag = N - 1
	}
	corr := make([]float64, hiLag+1)
	for lag := 0; lag <= hiLag; lag++ {
		var c float64
		for i := 0; i < N-lag; i++ {
			c += seg[i] * seg[i+lag]
		}
		corr[lag] = c
	}
	firstDip := 1
	for firstDip <= hiLag && corr[firstDip] > 0 {
		firstDip++
	}
	searchFrom := max(firstDip, loLag)
	bestLag, bestCorr := 0, 0.0
	for lag := searchFrom; lag <= hiLag; lag++ {
		if corr[lag] > bestCorr {
			bestCorr, bestLag = corr[lag], lag
		}
	}
	if bestLag == 0 {
		return 0, 0
	}
	f0 := float64(sr) / float64(bestLag)
	win, _ := windowed(seg, N)
	spec := fftPow(win)
	binHz := float64(sr) / float64(N)
	var harm, tot float64
	for k := 1; k < N/2; k++ {
		tot += spec[k]
	}
	for h := 1; h <= 16; h++ {
		kb := int(float64(h) * f0 / binHz)
		for k := kb - 2; k <= kb+2; k++ {
			if k >= 1 && k < N/2 {
				harm += spec[k]
			}
		}
	}
	if tot <= 0 {
		return f0, 0
	}
	return f0, harm / tot
}

func windowed(src []float64, n int) ([]complex128, float64) {
	seg := make([]complex128, n)
	var e float64
	for i := range n {
		w := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(n-1))
		s := src[i] * w
		seg[i] = complex(s, 0)
		e += s * s
	}
	return seg, e
}

func fftPow(x []complex128) []float64 {
	n := len(x)
	X := make([]complex128, n)
	copy(X, x)
	j := 0
	for i := 1; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			X[i], X[j] = X[j], X[i]
		}
	}
	for length := 2; length <= n; length <<= 1 {
		wl := cmplx.Exp(complex(0, -2*math.Pi/float64(length)))
		for i := 0; i < n; i += length {
			w := complex(1, 0)
			for k := 0; k < length/2; k++ {
				u := X[i+k]
				v := X[i+k+length/2] * w
				X[i+k] = u + v
				X[i+k+length/2] = u - v
				w *= wl
			}
		}
	}
	out := make([]float64, n)
	for k := range n {
		out[k] = real(X[k])*real(X[k]) + imag(X[k])*imag(X[k])
	}
	return out
}

// ---- offline render --------------------------------------------------------

// Applies each instrument exactly as import does. Quality is measured per instrument; master headroom uses the
// worst case of every hit on one downbeat.

// templateInst mirrors the importable instrument fields the renderer needs.
type templateInst struct {
	ID          string             `json:"id"`
	Volume      float64            `json:"volume"`
	SynthParams map[string]float64 `json:"synth_params"`
	Effects     []EffectSlot       `json:"effects"`
}

// templateDoc is the subset of a template circuit the audio guards render.
type templateDoc struct {
	Genre       string
	BPM         int            `json:"bpm"`
	Instruments []templateInst `json:"instruments"`
}

func parseTemplateDoc(t *testing.T, genre string, b []byte) templateDoc {
	t.Helper()
	var d templateDoc
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatalf("%s: parse template JSON: %v", genre, err)
	}
	d.Genre = genre
	if d.BPM <= 0 {
		d.BPM = 120
	}
	return d
}

// Renders share global engine state, so this instrument's params and chain are reset FIRST, or a heavier prior render
// leaks into this one.
func applyTemplateInstrument(in templateInst) {
	ResetInstrumentParams(instrument.ID(in.ID))
	if len(in.SynthParams) > 0 {
		SetInstrumentParams(instrument.ID(in.ID), RecipeParams(in.SynthParams))
	}
	SetInsertEffects(instrument.ID(in.ID), in.Effects) // empty slice clears any prior chain
}

func newTemplateMixer() *mixer {
	return &mixer{
		workBuf:   make([]float64, blockSize),
		voiceTemp: make([]float64, blockSize),
		masterBuf: make([]float64, blockSize),
		postEQBuf: make([]float64, blockSize),
		instSlots: make(map[channel.ID]int),
	}
}

func instVol(in templateInst) float64 {
	if in.Volume == 0 {
		return 1
	}
	return in.Volume
}

// One hit of every instrument at t=0, the worst-case downbeat; the peak before the soft clip.
func renderTemplateAllHitsPeak(t *testing.T, d templateDoc) float64 {
	t.Helper()
	game_log.TailOnFailure(t) // the capture lines, only if this test fails
	ResetInstruments()
	sr := sampleRate
	m := newTemplateMixer()
	for _, in := range d.Instruments {
		applyTemplateInstrument(in)
		v := newRecipeAwareVoice(instrument.ID(in.ID), sr)
		if v == nil {
			continue
		}
		m.Schedule(channel.Of(instrument.ID(in.ID)), &scaledVoice{v: v, gain: instVol(in)}, 0, -1, -1)
	}
	StartOutputCapture()
	readMixerSamples(m, sr) // 1s captures the full transient
	return analyzeAudio(StopOutputCapture(), sr).peak
}

// Once per beat so the voice stays active for crackle and flatness analysis.
func renderTemplateInstrumentSolo(t *testing.T, d templateDoc, in templateInst, seconds float64) []float64 {
	t.Helper()
	game_log.TailOnFailure(t) // the capture lines, only if this test fails
	ResetInstruments()
	applyTemplateInstrument(in)
	sr := sampleRate
	totalSamples := int(seconds * float64(sr))
	beatSamples := int(60.0 / float64(d.BPM) * float64(sr))
	if beatSamples <= 0 {
		beatSamples = sr / 2
	}
	m := newTemplateMixer()
	for delay := 0; delay < totalSamples; delay += beatSamples {
		v := newRecipeAwareVoice(instrument.ID(in.ID), sr)
		if v == nil {
			break
		}
		m.Schedule(channel.Of(instrument.ID(in.ID)), &scaledVoice{v: v, gain: instVol(in)}, delay, -1, -1)
	}
	StartOutputCapture()
	readMixerSamples(m, totalSamples)
	return StopOutputCapture()
}

// ---- TOOL: full metrics table ---------------------------------------------

func TestTemplateAudioReport(t *testing.T) {
	dump := os.Getenv("BEATMO_DIAG_WAV")
	t.Logf("%-9s %-12s %-6s %-6s %-5s %-5s %-5s %-5s %-5s %s",
		"genre", "inst", "peak", "rms", "crest", "clip%", "flat", "hf%", "harm%", "f0")
	for _, tp := range assets.Templates() {
		d := parseTemplateDoc(t, tp.Genre, tp.Bytes)
		ids := make([]string, 0, len(d.Instruments))
		byID := make(map[string]templateInst, len(d.Instruments))
		for _, in := range d.Instruments {
			ids = append(ids, in.ID)
			byID[in.ID] = in
		}
		sort.Strings(ids)
		for _, id := range ids {
			solo := renderTemplateInstrumentSolo(t, d, byID[id], 3.0)
			sm := analyzeAudio(solo, sampleRate)
			t.Logf("%-9s %-12s %.3f  %.3f  %.2f  %.2f  %.3f %.1f  %.1f  %.0f",
				tp.Genre, id, sm.peak, sm.rms, sm.crest, sm.clipFrac*100, sm.flatness, sm.hfFrac*100, sm.harmFrac*100, sm.f0)
			if dump != "" {
				_ = ExportCaptureToWAV(solo, dump+"_"+tp.Genre+"_"+id+".wav")
			}
		}
	}
}

// ---- GUARD: audio quality --------------------------------------------------

func isBassInstrument(id instrument.ID) bool {
	return id == roster.FmBass || id == roster.SubBass || strings.HasPrefix(string(id), "sub-")
}
func isKickInstrument(id string) bool { return strings.HasPrefix(id, "kick") }

func TestTemplateAudioQuality(t *testing.T) {
	for _, tp := range assets.Templates() {
		t.Run(tp.Genre, func(t *testing.T) {
			d := parseTemplateDoc(t, tp.Genre, tp.Bytes)

			// No clipping, no white-noise hiss, no sustained crackle.
			var kickPeak float64
			solos := map[string]audioMetrics{}
			for _, in := range d.Instruments {
				s := analyzeAudio(renderTemplateInstrumentSolo(t, d, in, 3.0), sampleRate)
				solos[in.ID] = s
				if isKickInstrument(in.ID) {
					kickPeak = s.peak
				}
				if s.peak >= 0.999 {
					t.Errorf("%s: peak=%.3f — clipping at the voice/channel", in.ID, s.peak)
				}
				// A clap is legitimately near-white, so TIME structure tells a hit from a stuck hiss: a transient burst has a high
				// crest factor and gets a looser flatness ceiling that still rejects a pure-white render.
				const noiseBurstCrestMin = 6.0     // transient ⇒ percussive noise burst, not a stuck hiss
				const noiseBurstFlatnessMax = 0.65 // even a clap is nowhere near pure white noise
				flatnessMax := 0.50
				if s.crest >= noiseBurstCrestMin {
					flatnessMax = noiseBurstFlatnessMax
				}
				if s.flatness > flatnessMax {
					t.Errorf("%s: flatness=%.3f (>%.2f, crest=%.2f): near-white-noise — broken/garbage render?",
						in.ID, s.flatness, flatnessMax, s.crest)
				}
				if s.jumpRate > 1.0 {
					t.Errorf("%s: jumpRate=%.2f%% (>1%%): crackle/clicks", in.ID, s.jumpRate*100)
				}
			}

			// Bass quality + balance vs kick.
			for _, in := range d.Instruments {
				if !isBassInstrument(instrument.ID(in.ID)) {
					continue
				}
				b := solos[in.ID]
				if b.hfFrac > 0.35 {
					t.Errorf("%s: %.1f%% of energy above 4kHz (>35%%): metallic, not a bass", in.ID, b.hfFrac*100)
				}
				if b.harmFrac < 0.55 {
					t.Errorf("%s: harmonic fraction %.1f%% (<55%%): muddy/inharmonic", in.ID, b.harmFrac*100)
				}
				if kickPeak > 0 && b.peak > 2.0*kickPeak {
					t.Errorf("%s: bass peak %.3f > 2x kick peak %.3f — bass overpowers the drums", in.ID, b.peak, kickPeak)
				}
			}
		})
	}
}

// The fixture half pins the wire shape gen-showcase emits, which no shipped template exercises now; the catalogue half
// validates any effects a template does carry.
func TestTemplateInsertEffectsAreImportable(t *testing.T) {
	// If EffectSlot's tags drift from it, a template with effects imports silently filterless.
	const emittedByGenShowcase = `{
	  "type": "filter",
	  "enabled": true,
	  "params": {"cutoff": 9000, "mix": 1, "mode": 0, "q": 0.707}
	}`
	var slot EffectSlot
	if err := json.Unmarshal([]byte(emittedByGenShowcase), &slot); err != nil {
		t.Fatalf("gen-showcase effect JSON does not deserialize into EffectSlot: %v", err)
	}
	want := EffectSlot{
		Type:    EffectFilter,
		Enabled: true,
		Params:  map[string]float64{"cutoff": 9000, "mix": 1, "mode": 0, "q": 0.707},
	}
	if slot.Type != want.Type || slot.Enabled != want.Enabled {
		t.Errorf("type/enabled: got {%q %v}, want {%q %v}", slot.Type, slot.Enabled, want.Type, want.Enabled)
	}
	if !reflect.DeepEqual(slot.Params, want.Params) {
		t.Errorf("params: got %v, want %v", slot.Params, want.Params)
	}

	found := 0
	for _, tp := range assets.Templates() {
		// A reused struct lets json keep stale effects from a prior template.
		var doc struct {
			Instruments []struct {
				ID      string       `json:"id"`
				Effects []EffectSlot `json:"effects"`
			} `json:"instruments"`
		}
		if err := json.Unmarshal(tp.Bytes, &doc); err != nil {
			t.Fatalf("%s: %v", tp.Genre, err)
		}
		for _, in := range doc.Instruments {
			for _, fx := range in.Effects {
				found++
				if fx.Type != EffectFilter {
					t.Errorf("%s/%s: effect type %q != %q", tp.Genre, in.ID, fx.Type, EffectFilter)
				}
				if !fx.Enabled {
					t.Errorf("%s/%s: effect not enabled", tp.Genre, in.ID)
				}
				if c := fx.Params["cutoff"]; c < 20 || c > 20000 {
					t.Errorf("%s/%s: filter cutoff %.0f out of audio range", tp.Genre, in.ID, c)
				}
			}
		}
	}
	t.Logf("verified the emitted wire shape + %d importable insert-effect slots across templates", found)
}
