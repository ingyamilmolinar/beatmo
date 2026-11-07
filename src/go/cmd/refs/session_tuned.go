//go:build !js

package main

// The listener must hear exactly what refs tune scored: the recording loaded as refs match loads it and a voice held as
// long as it sounds, at the search pitch. The generic compare render plays each voice at its shipped length and the
// catalogue clip cut hard at its trim: chopped voices against a clicking reference. Every clip is checked first.

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/audio/clone"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// tunedSessionRender is the sessionRenderFn of `refs session -tuned`: the
// family's single candidate T, played as `refs tune` judged it.
func tunedSessionRender(root string, diag io.Writer) sessionRenderFn {
	return func(fc familyCandidates, ref wavref.Ref) (sessionClips, sessionRenderMeta, error) {
		if len(fc.Candidates) != 1 || fc.Candidates[0].Doc == nil {
			return sessionClips{}, sessionRenderMeta{}, fmt.Errorf("refs session -tuned: %s needs exactly one tuned candidate", fc.Base)
		}
		meta, err := loadTuneMeta(filepath.Join(root, string(fc.Base)))
		if err != nil {
			return sessionClips{}, sessionRenderMeta{}, err
		}
		path, ok := wavref.Resolve(ref.ID)
		if !ok {
			return sessionClips{}, sessionRenderMeta{}, fmt.Errorf("refs session -tuned: audio for %s is not available locally; run `refs fetch -id %s` first", ref.ID, ref.ID)
		}
		a, err := renderTunedAudition(fc.Base, *fc.Candidates[0].Doc, path, ref.ID, meta.maxSec(), meta.searchPitch(ref.ID))
		if err != nil {
			return sessionClips{}, sessionRenderMeta{}, err
		}
		fmt.Fprintf(diag, "  essence distance to the reference, as `refs tune` scored it: C %.3f, T %.3f\n", a.CurEssence, a.CandEssence)
		for _, f := range a.Faults {
			fmt.Fprintf(diag, "  WARNING: %s\n", f)
		}
		if a.CandPitch != a.Pitch {
			fmt.Fprintf(diag, "  C plays at %+.2f st, T at %+.2f st: T started from %s's document, which sounds closest at its own pitch\n", a.Pitch, a.CandPitch, meta.From)
		}
		return a.Clips, sessionRenderMeta{Pitch: a.Pitch, Dur: a.Dur}, nil
	}
}

// loadTuneMeta reads a tuned candidate's meta.json.
func loadTuneMeta(dir string) (tuneMeta, error) {
	var m tuneMeta
	b, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err == nil {
		err = json.Unmarshal(b, &m)
	}
	if err != nil {
		return m, fmt.Errorf("refs session: %s/meta.json: %w", dir, err)
	}
	return m, nil
}

// A meta from before it was recorded used the -max-sec default.
func (m tuneMeta) maxSec() float64 {
	if m.MaxSec > 0 {
		return m.MaxSec
	}
	return adoptMaxSecDefault
}

// nil in a run that predates it: the shipped voice's pitch.
func (m tuneMeta) searchPitch(refID string) *float64 {
	for _, r := range m.Records {
		if r.Ref == refID || r.Recording == refID {
			return r.SearchPitch
		}
	}
	return nil
}

// tunedAudition is one tuned family's clip set as written, plus what the
// search measured on it.
type tunedAudition struct {
	Clips sessionClips
	// Pitch is where C plays (the shipped voice's settled pitch), CandPitch
	// where T plays (the search pitch; the same unless the pick started
	// From another instrument).
	Pitch, CandPitch float64
	Dur              float64
	// CurEssence / CandEssence: the raw essence distance of C and T to the
	// reference -- the very number the search minimised for T.
	CurEssence, CandEssence float64
	// Waves are R, C, T exactly as written (level-matched), for the checks.
	Waves  [3]wave.Wave
	Faults []string
}

// R, C and T from the tuner's own code (newTuneRec, settlePitch, matchTarget.eval / evalDoc); T at candPitch from
// meta.json. Then level-matched and checked.
func renderTunedAudition(inst instrument.ID, cand instrument.Doc, refPath, refID string, maxSec float64, candPitch *float64) (tunedAudition, error) {
	tmp, err := os.MkdirTemp("", "beatmo-ab-tuned-*")
	if err != nil {
		return tunedAudition{}, err
	}
	defer os.RemoveAll(tmp)
	rec, err := newTuneRec(refPath, refID, refID, tmp, maxSec)
	if err != nil {
		return tunedAudition{}, err
	}
	if err := rec.settlePitch(inst); err != nil {
		return tunedAudition{}, err
	}
	cur, err := rec.t.eval(inst, rec.pitch)
	if err != nil {
		return tunedAudition{}, err
	}
	tp := rec.pitch
	if candPitch != nil {
		tp = *candPitch
	}
	tc, err := rec.t.evalDoc(cand, inst, tp)
	if err != nil {
		return tunedAudition{}, err
	}

	// The listener must not hear a trim as a click: the onset trim cuts a slow rise, and the catalogue clip can end mid-note.
	waves := [3]wave.Wave{
		listeningCopy(wave.Wave{Samples: rec.w.Samples, SampleRate: rec.w.SampleRate, Label: refID}),
		listeningCopy(cur.w), listeningCopy(tc.w),
	}
	var rms, peaks []float64
	for _, w := range waves {
		rms, peaks = append(rms, measureWave(w).RMS), append(peaks, truePeak(w.Samples))
	}
	gains := tunedLevelGains(rms, peaks)
	dir, err := compareDir(inst)
	if err != nil {
		return tunedAudition{}, err
	}
	sha := strings.TrimPrefix(clone.DocSHA(cand), "sha256:")
	names := [3]string{
		fmt.Sprintf("TUNED_AB__R__%s.wav", refID),
		fmt.Sprintf("TUNED_AB__C__%s.wav", inst),
		fmt.Sprintf("TUNED_AB__T__%s-%s.wav", inst, sha),
	}
	keys := [3]string{"R", "C", "T"}
	a := tunedAudition{Pitch: rec.pitch, CandPitch: tp, Dur: rec.w.Duration(), CurEssence: fingerprint.EssenceTotal(cur.ess), CandEssence: fingerprint.EssenceTotal(tc.ess)}
	paths := [3]string{}
	for i, w := range waves {
		w.Samples = scaleSamples(w.Samples, gains[i])
		a.Waves[i] = w
		paths[i] = filepath.Join(dir, names[i])
		if err := writeCompareWAV(paths[i], w.Samples, w.SampleRate); err != nil {
			return tunedAudition{}, err
		}
		paths[i] = absOrSelf(paths[i])
		a.Faults = append(a.Faults, clipFaults(keys[i], w)...)
		if i > 0 {
			if f := holdFault(keys[i], a.Waves[0], w); f != "" {
				a.Faults = append(a.Faults, f)
			}
		}
	}
	if slices.Equal(cur.w.Samples, tc.w.Samples) {
		a.Faults = append(a.Faults, "T sounds IDENTICAL to C here: every change the candidate makes is undone by holding the note as long as the recording (a render.seconds edit is one)")
	}
	if cur.silent || tc.silent {
		a.Faults = append(a.Faults, fmt.Sprintf("a voice renders silent (C at %+.2f st silent=%v, T at %+.2f st silent=%v)", rec.pitch, cur.silent, tp, tc.silent))
	}
	a.Clips = sessionClips{refPath: paths[0], curPath: paths[1], candPaths: map[string]string{"T": paths[2]}, order: []string{"R", "C", "T"}}
	return a, nil
}

// listeningCopy is w as the listener hears it: the analysed clip with its
// trims faded (fadeInLead, fadeOutTail) so neither end clicks.
func listeningCopy(w wave.Wave) wave.Wave {
	s := append([]float64(nil), w.Samples...)
	fadeInLead(s, w.SampleRate)
	fadeOutTail(s, int(matchFadeSec*float64(w.SampleRate)))
	w.Samples = s
	return w
}

// ---- level ------------------------------------------------------------------

// -1 dBTP survives the resampling between file and DAC (PipeWire plays these 44.1 kHz clips at 48 kHz).
var clipHeadroom = math.Pow(10, -1.0/20)

// The VOICES come to the recording's loudness and the recording keeps its own: meeting at the average raised a quiet
// recording with its hiss to a peak the DAC clipped. A breach lowers the whole set together, so the recording may get
// quieter, never louder.
func tunedLevelGains(rms, truePeaks []float64) []float64 {
	g := make([]float64, len(rms))
	for i, r := range rms {
		g[i] = 1
		if i > 0 && r > 0 && rms[0] > 0 {
			g[i] = rms[0] / r
		}
	}
	return clampFamilyGains(g, truePeaks, clipHeadroom)
}

// 4x oversampled through a Hann-windowed sinc (ITU-R BS.1770), so a peak between two samples, which the DAC
// reconstructs, is seen.
func truePeak(s []float64) float64 {
	const taps = 16 // each side
	var peak float64
	for i, v := range s {
		peak = math.Max(peak, math.Abs(v))
		for k := 1; k < 4; k++ {
			frac := float64(k) / 4
			var acc float64
			for j := -taps + 1; j <= taps; j++ {
				n := i + j
				if n < 0 || n >= len(s) {
					continue
				}
				x := float64(j) - frac
				w := 0.5 + 0.5*math.Cos(math.Pi*x/taps)
				acc += s[n] * sinc(x) * w
			}
			peak = math.Max(peak, math.Abs(acc))
		}
	}
	return peak
}

func sinc(x float64) float64 {
	if x == 0 {
		return 1
	}
	return math.Sin(math.Pi*x) / (math.Pi * x)
}

// ---- the listening contract ------------------------------------------------

const (
	// faultFloorDB: below the clip's peak by this much a sample is not sound.
	faultFloorDB = -40.0
	// faultLateSec: sound must start within this of the clip's start (every
	// clip is onset-trimmed, so both notes start together).
	faultLateSec = 0.05
	// faultDropoutSec: a run of digital silence this long INSIDE the sound is
	// a dropout -- a chopped clip.
	faultDropoutSec = 0.01
	// faultHeldDB: a sound "holds" while it is within this of its loudest
	// faultEnvSec window.
	faultHeldDB = -20.0
	// A gate, not a decay: a dry snare loses tens of dB in 200 ms and keeps ringing below that.
	faultSilentDB = -120.0
	faultCutSec   = 0.2
	faultEnvSec   = 0.05
)

// clipFaults lists everything about one clip a listener would hear as broken
// playback rather than as the instrument: silence, clipping, a late start, a
// dropout inside the sound, and an end on a step (a click).
func clipFaults(key string, w wave.Wave) []string {
	s, sr := w.Samples, float64(w.SampleRate)
	var peak float64
	clipped := 0
	for _, v := range s {
		peak = math.Max(peak, math.Abs(v))
		if math.Abs(v) >= 0.999 {
			clipped++
		}
	}
	if peak == 0 {
		return []string{key + " is silent"}
	}
	var out []string
	if clipped > 0 {
		out = append(out, fmt.Sprintf("%s clips (%d samples at full scale)", key, clipped))
	} else if tp := truePeak(s); tp > clipHeadroom {
		out = append(out, fmt.Sprintf("%s true peak %.2f is past the -1 dBTP headroom: the DAC clips it between the samples", key, tp))
	}
	thr := peak * math.Pow(10, faultFloorDB/20)
	first, last := -1, -1
	for i, v := range s {
		if math.Abs(v) >= thr {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if onset := firstAbove(s, peak*math.Pow(10, matchSilenceDB/20)); math.Abs(s[0]) >= thr && onset > 0 {
		out = append(out, fmt.Sprintf("%s starts on a step (first sample %.0f dB re its peak, before its note): a click", key, 20*math.Log10(math.Abs(s[0])/peak)))
	}
	if t := float64(first) / sr; t > faultLateSec {
		out = append(out, fmt.Sprintf("%s starts %.0f ms late", key, t*1000))
	}
	run, need := 0, int(faultDropoutSec*sr)
	for i := first; i <= last; i++ {
		if math.Abs(s[i]) < 1e-9 {
			run++
			if run == need {
				out = append(out, fmt.Sprintf("%s drops out at %.2f s", key, float64(i-run+1)/sr))
			}
		} else {
			run = 0
		}
	}
	if end := math.Abs(s[len(s)-1]); end > thr {
		out = append(out, fmt.Sprintf("%s ends on a step to silence (last sample %.0f dB re its peak): a click", key, 20*math.Log10(end/peak)))
	}
	return out
}

func firstAbove(s []float64, thr float64) int {
	for i, v := range s {
		if math.Abs(v) >= thr {
			return i
		}
	}
	return len(s)
}

// A voice CUT while the recording still holds its note (falls to digital silence within faultCutSec). A voice that
// merely DECAYS faster is timbre for the listener to judge, not a broken clip.
func holdFault(key string, ref, voice wave.Wave) string {
	re, ve := envelopeDB(ref), envelopeDB(voice)
	span := int(math.Round(faultCutSec / faultEnvSec))
	for i := 0; i+span < len(ve) && i+span < len(re); i++ {
		if ve[i] < faultHeldDB || ve[i+span] > faultSilentDB || re[i] < faultHeldDB || re[i+span] < faultHeldDB {
			continue
		}
		return fmt.Sprintf("%s is cut at %.2f s while the recording still holds its note (falls %.0f dB in %.0f ms; sounds until %.2f s, the recording until %.2f s)",
			key, float64(i)*faultEnvSec, ve[i]-ve[i+span], faultCutSec*1000, lastSoundingSec(voice), lastSoundingSec(ref))
	}
	return ""
}

// envelopeDB is the faultEnvSec RMS envelope, in dB re its loudest window.
func envelopeDB(w wave.Wave) []float64 {
	hop := max(1, int(faultEnvSec*float64(w.SampleRate)))
	var env []float64
	top := 0.0
	for i := 0; i < len(w.Samples); i += hop {
		var sum float64
		n := 0
		for _, v := range w.Samples[i:min(i+hop, len(w.Samples))] {
			sum += v * v
			n++
		}
		r := math.Sqrt(sum / float64(n))
		env = append(env, r)
		top = math.Max(top, r)
	}
	for i, r := range env {
		env[i] = 20 * math.Log10(math.Max(r, 1e-12)/math.Max(top, 1e-12))
	}
	return env
}

// lastSoundingSec is where w last rises above faultFloorDB of its peak.
func lastSoundingSec(w wave.Wave) float64 {
	var peak float64
	for _, v := range w.Samples {
		peak = math.Max(peak, math.Abs(v))
	}
	thr := peak * math.Pow(10, faultFloorDB/20)
	for i := len(w.Samples) - 1; i >= 0; i-- {
		if peak > 0 && math.Abs(w.Samples[i]) >= thr {
			return float64(i+1) / float64(w.SampleRate)
		}
	}
	return 0
}
