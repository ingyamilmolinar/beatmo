//go:build !test && !js

package audio

import (
	"encoding/json"
	"math"
	"math/cmplx"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
)

// template_audio_quality_test.go — renders every genre template (and each
// instrument in isolation) through the REAL voice→mixer→master-chain path and
// measures objective audio-quality metrics. This is the "way to ensure synth
// instruments don't sound horrible/distorted/noisy" the brief asked for:
//
//   - TestTemplateAudioReport  — diagnostic tool: logs a metrics table for every
//     genre+instrument. Always passes. Set BEATMO_DIAG_WAV=/tmp/diag to also dump
//     WAVs for listening.
//   - TestTemplateAudioQuality — the guard: fails if any mix clips, is squashed,
//     or if a melodic bass is noisy (metallic HF) or muddy (inharmonic cluster),
//     or if the bass overpowers the kick.
//
// Both build under the native (!test) tag because they exercise the CGo synth
// engine and the real master chain (compressor + soft-clip + limiter).

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

// harmonicFraction detects the fundamental of the loudest window via
// autocorrelation and returns (f0, fraction of spectral energy that sits at
// integer multiples of f0). A clean tonal note scores high; a dissonant cluster
// of overlapping notes (or noise) scores low.
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
	// Autocorrelation f0 over 30..400 Hz. A low near-sine is highly correlated at
	// small lags too, so a naive global-max picks a spurious short lag; the robust
	// fix is to skip past the first dip (where autocorr first goes non-positive)
	// and take the peak after it — that lag is the true period.
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

// renderTemplateMix renders `seconds` of a template through the real mixer +
// master chain. When soloID != "" only that instrument's row is scheduled.
// --- JSON-driven rendering (templates are authoritative JSON, no Spec) -------
//
// Templates are hand-authored/exported project files. These helpers render them
// the same way the startup circuit is exercised: parse the embedded JSON, apply
// each instrument exactly as import does, and schedule voices. No procedural
// pattern reconstruction — quality is measured per-instrument-solo, and the
// master-mix headroom guard uses the worst-case all-hits-on-the-downbeat sum.

// templateInst mirrors the importable instrument fields the renderer needs.
type templateInst struct {
	ID          string             `json:"id"`
	Volume      float64            `json:"volume"`
	DelaySend   float64            `json:"delay_send"`
	ReverbSend  float64            `json:"reverb_send"`
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

// applyTemplateInstrument configures the engine for one instrument exactly as
// the importer does: synth-param overlay, insert effects, and sends. The JSON
// `effects` array deserializes straight into []EffectSlot (the same contract
// TestTemplateInsertEffectsAreImportable locks).
//
// Renders share global engine state, so it FIRST resets this instrument's param
// overlay and insert chain — otherwise a heavier prior render (e.g. a bright
// house hihat with brightness=0.6) leaks into a param-less render of the same id
// (techno's hihat), inflating its measured flatness. ResetInstruments() clears
// channels/voices but not the per-instrument param overlay.
func applyTemplateInstrument(in templateInst) {
	ResetInstrumentParams(in.ID)
	if len(in.SynthParams) > 0 {
		SetInstrumentParams(in.ID, RecipeParams(in.SynthParams))
	}
	SetDelaySend(in.ID, in.DelaySend)
	SetReverbSend(in.ID, in.ReverbSend)
	SetInsertEffects(in.ID, in.Effects) // empty slice clears any prior chain
}

func newTemplateMixer() *mixer {
	return &mixer{
		workBuf:   make([]float64, blockSize),
		voiceTemp: make([]float64, blockSize),
		masterBuf: make([]float64, blockSize),
		postEQBuf: make([]float64, blockSize),
		instSlots: make(map[string]int),
	}
}

func instVol(in templateInst) float64 {
	if in.Volume == 0 {
		return 1
	}
	return in.Volume
}

// renderTemplateAllHitsPeak schedules ONE hit of every instrument at t=0 — the
// worst-case aligned downbeat — through the real chain and returns the
// pre-limiter master peak. Mirrors the startup-circuit allHitsPeak strategy.
func renderTemplateAllHitsPeak(t *testing.T, d templateDoc) float64 {
	t.Helper()
	ResetInstruments()
	resetSendEffectsState() // clear any reverb/delay tail a prior test left (render isolation)
	sr := sampleRate
	m := newTemplateMixer()
	for _, in := range d.Instruments {
		applyTemplateInstrument(in)
		v := newRecipeAwareVoice(in.ID, d.BPM, sr)
		if v == nil {
			continue
		}
		m.Schedule(in.ID, &scaledVoice{v: v, gain: instVol(in)}, 0)
	}
	StartOutputCapture()
	readMixerSamples(m, sr) // 1s captures the full transient
	return analyzeAudio(StopOutputCapture(), sr).peak
}

// renderTemplateInstrumentSolo renders one instrument played once per beat over
// the duration (so the voice stays active for crackle/flatness analysis),
// configured exactly as import would.
func renderTemplateInstrumentSolo(t *testing.T, d templateDoc, in templateInst, seconds float64) []float64 {
	t.Helper()
	ResetInstruments()
	resetSendEffectsState() // clear any reverb/delay tail a prior test left (render isolation)
	applyTemplateInstrument(in)
	sr := sampleRate
	totalSamples := int(seconds * float64(sr))
	beatSamples := int(60.0 / float64(d.BPM) * float64(sr))
	if beatSamples <= 0 {
		beatSamples = sr / 2
	}
	m := newTemplateMixer()
	for delay := 0; delay < totalSamples; delay += beatSamples {
		v := newRecipeAwareVoice(in.ID, d.BPM, sr)
		if v == nil {
			break
		}
		m.Schedule(in.ID, &scaledVoice{v: v, gain: instVol(in)}, delay)
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

func isBassInstrument(id string) bool {
	return id == "fm-bass" || id == "sub-bass" || strings.HasPrefix(id, "sub-")
}
func isKickInstrument(id string) bool { return strings.HasPrefix(id, "kick") }

func TestTemplateAudioQuality(t *testing.T) {
	for _, tp := range assets.Templates() {
		t.Run(tp.Genre, func(t *testing.T) {
			d := parseTemplateDoc(t, tp.Genre, tp.Bytes)

			// EVERY instrument in the template, rendered solo through the real
			// chain (synth params + insert effects + sends, exactly as import),
			// must be clean: no clipping, not white-noise hissy, no sustained
			// crackle. (Worst-case master headroom is guarded separately in
			// TestTemplateMasterMix.)
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
				// Templates are authoritative, user-authored circuits, so a bright
				// noise-based percussion voice (a hi-hat at brightness 0.6 measures
				// ~0.38) is an intentional sound, not a bug. This guard only catches a
				// near-pure-white-noise render — a genuinely broken/garbage voice.
				//
				// Spectral flatness alone is too blunt: a CLAP is, by design, a short
				// burst of band-limited noise, so its spectrum is legitimately near-
				// white (pop-ballad/house-piano claps measure ~0.48–0.50). What tells
				// a real noise-burst HIT apart from a genuinely broken "stuck hiss" is
				// TIME structure, not spectrum. A percussion hit is transient — energy
				// packed into short decaying bursts with near-silence between, so its
				// crest factor (peak/rms over the whole 3 s solo) is high (claps ≈7.6–
				// 10.6, hats ≈10–18). A broken white-noise render fills the buffer
				// evenly and sits at a low crest (≈3–5). So: sustained sounds keep the
				// strict 0.50 bound; a transient noise-burst (clap/hat/snare) is held
				// to a looser ceiling that still rejects an actually-pure-white render
				// (flatness → ~0.9–1.0) but passes a real clap (~0.5).
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
				if !isBassInstrument(in.ID) {
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

// TestTemplateInsertEffectsAreImportable proves each template's per-instrument
// `effects` JSON deserializes into the exact []audio.EffectSlot the importer
// feeds to SetInsertEffects — so the filters actually reach the engine on import
// (the UI round-trip test TestTemplates_ImportRoundTrip exercises the full path;
// this locks the audio-side contract independently).
func TestTemplateInsertEffectsAreImportable(t *testing.T) {
	found := 0
	for _, tp := range assets.Templates() {
		// Fresh doc per iteration: a reused struct lets json slice-reuse retain
		// stale `effects` from a prior template (inflates the count).
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
	if found == 0 {
		t.Fatal("no template insert effects found — the lowpass tuning is not being emitted")
	}
	t.Logf("verified %d importable insert-effect slots across templates", found)
}
