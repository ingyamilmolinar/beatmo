//go:build !js

package main

// match.go -- `refs match`: which shipped instrument is closest to a recording
// nobody has catalogued?
//
// `refs compare` / `refs audition` start from an INSTRUMENT and ask how close
// its catalogued reference is. This runs the other way: hand it any WAV (a
// download, a take of your own) and it ranks the whole roster against it. The
// reference is converted exactly like a fetched one (mono / 44.1 kHz /
// 16-bit) and its pitch measured with the three-gate UsableAsPitch rule.
// Every instrument is then rendered AS PLAYED (synthmatch.RenderShippedBPM)
// the way a player would play it to imitate the recording -- at the
// recording's note (or, for an unpitched sound, at whichever pitch within an
// octave sounds closest), held at least as long as the recording sounds -- and
// ranked by its ESSENCE distance (fingerprint.AnalyzeEssence: colour, its
// evolution from the attack into the body, noise texture, how the note dies
// away; the attack time counts lightly). Pitch, note length, level and room
// tone are nuisances removed by construction, not distances. Why the
// previous metrics ranked a real piano closest to the synth double bass:
// docs/superpowers/specs/
// 2026-10-17-wav-match-essence-design.md. The top N are written level-matched
// beside the reference for an ear check.
//
// Nothing here touches catalogue.json: a match is a QUESTION for the listener,
// not a decision. `refs match -listen` asks it and records the answer (with
// free-text notes on the synth) in wav_match_verdicts.jsonl; adopting a WAV as
// an instrument's reference stays `refs adopt`'s job.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"text/tabwriter"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/audio/synthmatch"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// Reference classes, reported (manifest, -validate) but no longer used to
// pick a metric or a pool: the essence distance carries the envelope itself,
// and the pitch plan (matchPitchPlan) decides how each voice is played.
const (
	matchClassSustained  = "sustained"
	matchClassPercussive = "percussive"
)

// matchDecayDB / matchDecaySec: a clip whose level 0.3 s after its loudest
// moment has fallen this far below it is percussive, whatever its pitch.
const (
	matchDecayDB      = -20.0
	matchDecaySec     = 0.3
	matchShortClipSec = 0.4
	matchPeakFrac     = 0.9 // within ~1 dB of the loudest window
)

// matchManifestVersion invalidates every cached entry when the analysis
// changes shape.
const matchManifestVersion = 9

// matchSilenceDB is how far below the clip's peak the leading-silence trim
// cuts: a download often opens with room tone, which would otherwise eat the
// 2 s analysis window and misplace every onset metric. -20 dB, not a noise
// floor like -40: the crowd "ahh" (763910) holds 1.7 s of ambience at -30 dB
// before the voices swell in, and a -40 dB trim kept all of it. A struck
// sound rises through -40..-20 dB in well under a millisecond, inside
// matchPreRollSec, so percussion loses nothing.
const matchSilenceDB = -20.0

// matchPreRollSec keeps a few ms before the detected onset so the attack
// transient itself is never clipped.
const matchPreRollSec = 0.005

// matchFadeSec is the fade applied where the analysis window cuts a still-
// sounding note, so the listener never hears a click the instrument does not
// make.
const matchFadeSec = 0.02

type matchCandidate struct {
	Instrument  instrument.ID `json:"instrument"`
	Score       float64       `json:"score"`
	RenderPitch float64       `json:"render_pitch"`
	// RenderSec is how long the voice was held (HeldSeconds: long enough
	// that the note lasts as long as the recording's).
	RenderSec float64 `json:"render_sec,omitempty"`
	SoundHash string  `json:"sound_hash"`
	Clip      string  `json:"clip,omitempty"`
	// Terms is the raw essence distance per term (what decided Score);
	// Traits says which way the voice differs (brighter, noisier, shorter).
	Terms  *fingerprint.EssenceScore  `json:"terms,omitempty"`
	Traits *fingerprint.EssenceTraits `json:"traits,omitempty"`
}

type matchEntry struct {
	Name        string   `json:"name"`
	Source      string   `json:"source"`
	SHA256      string   `json:"sha256"`
	Duplicates  []string `json:"duplicates,omitempty"`
	Reference   string   `json:"reference"`
	DurationSec float64  `json:"duration_sec"`
	F0Hz        float64  `json:"f0_hz"`
	Pitched     bool     `json:"pitched"`
	Note        string   `json:"note,omitempty"`
	Class       string   `json:"class"`
	// Namesake is the roster instrument the FILENAME names, if any
	// (namesakeInFilename).
	Namesake instrument.ID `json:"namesake,omitempty"`
	// Label is what the recording is meant to be (matchLabel: its catalogue
	// reference's instruments, else Namesake), and LabelBest the closest
	// voice of the label's families. LabelBest's clip is always written, even
	// when it did not rank in the top N, so the listener hears the instrument
	// meant to be this sound next to the ones that measure closest.
	Label     []instrument.ID `json:"label,omitempty"`
	LabelBest instrument.ID   `json:"label_best,omitempty"`
	// Traits is the recording's own EssenceTraits, beside which each
	// candidate's are read.
	Traits     fingerprint.EssenceTraits `json:"traits"`
	RosterHash string                    `json:"roster_hash"`
	Ranking    []matchCandidate          `json:"ranking"`
}

type matchManifest struct {
	Version int          `json:"version"`
	Entries []matchEntry `json:"entries"`
}

type matchOpts struct {
	Glob    string
	OutDir  string
	Top     int
	MaxSec  float64
	Workers int
	Force   bool
}

func matchDir() (string, error) {
	base, ok := wavref.Dir()
	if !ok {
		return "", fmt.Errorf("set %s to a directory for match review audio", wavref.EnvVar)
	}
	return filepath.Join(base, "matches"), nil
}

func matchManifestPath(dir string) string { return filepath.Join(dir, "manifest.json") }

func loadMatchManifest(dir string) (matchManifest, error) {
	b, err := os.ReadFile(matchManifestPath(dir))
	if os.IsNotExist(err) {
		return matchManifest{Version: matchManifestVersion}, nil
	}
	if err != nil {
		return matchManifest{}, err
	}
	var m matchManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return matchManifest{}, fmt.Errorf("refs match: %s: %w", matchManifestPath(dir), err)
	}
	if m.Version != matchManifestVersion {
		return matchManifest{Version: matchManifestVersion}, nil
	}
	return m, nil
}

func writeMatchManifest(dir string, m matchManifest) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(matchManifestPath(dir), append(b, '\n'), 0o644)
}

// matchSlug turns a download's filename into a directory name.
func matchSlug(name string) string {
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	var b strings.Builder
	for _, r := range strings.ToLower(stem) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func sha256File(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:]), nil
}

// rosterHash identifies the exact set of sounds a ranking was computed
// against, and the hubness calibration it was corrected with, so a retuned
// instrument or a fresh calibration invalidates the cached rankings.
func rosterHash(ids []instrument.ID) string {
	h := sha256.New()
	for _, id := range ids {
		fmt.Fprintf(h, "%s=%s\n", id, audio.InstrumentSoundHash(id, nil))
	}
	h.Write(matchHubnessJSON)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// loadMatchReference converts src through the same ffmpeg step every
// catalogued reference takes, then trims leading silence and caps it at
// maxSec in memory.
func loadMatchReference(src, tmpDir string, maxSec float64) (wave.Wave, error) {
	conv := filepath.Join(tmpDir, "full.wav")
	cv := wavref.Convert{Channels: 1, Rate: wavref.Rate, Bits: 16}
	if out, err := exec.Command("ffmpeg", ffmpegArgs(src, conv, cv)...).CombinedOutput(); err != nil {
		return wave.Wave{}, fmt.Errorf("ffmpeg %s: %w\n%s", src, err, out)
	}
	defer os.Remove(conv)
	pcm, rate, err := audio.DecodeWAVAtRate(conv, wavref.Rate)
	if err != nil {
		return wave.Wave{}, err
	}
	s := make([]float64, len(pcm))
	for i, v := range pcm {
		s[i] = float64(v)
	}
	return wave.Wave{Samples: trimMatchClip(s, rate, maxSec), SampleRate: rate, Label: filepath.Base(src)}, nil
}

// trimMatchClip drops leading silence (matchSilenceDB below peak, keeping
// matchPreRollSec), caps the clip at maxSec and fades a cut tail.
func trimMatchClip(s []float64, rate int, maxSec float64) []float64 {
	var peak float64
	for _, v := range s {
		peak = math.Max(peak, math.Abs(v))
	}
	if peak == 0 {
		return s
	}
	thr := peak * math.Pow(10, matchSilenceDB/20)
	start := 0
	for i, v := range s {
		if math.Abs(v) >= thr {
			start = i
			break
		}
	}
	start -= int(matchPreRollSec * float64(rate))
	if start < 0 {
		start = 0
	}
	out := append([]float64(nil), s[start:]...)
	if maxSec > 0 {
		if n := int(maxSec * float64(rate)); len(out) > n {
			out = out[:n]
			fadeOutTail(out, int(matchFadeSec*float64(rate)))
		}
	}
	return out
}

// fadeInLead ramps a clip that the onset trim cut mid-rise up from exactly
// 0 over the pre-roll it kept (the samples before the note crosses
// matchSilenceDB), so it starts without a step (a click). A clip whose note
// starts on its first sample -- a struck voice's attack -- is left alone.
func fadeInLead(s []float64, rate int) {
	var peak float64
	for _, v := range s {
		peak = math.Max(peak, math.Abs(v))
	}
	thr := peak * math.Pow(10, matchSilenceDB/20)
	onset := 0
	for onset < len(s) && math.Abs(s[onset]) < thr {
		onset++
	}
	n := min(onset, int(matchPreRollSec*float64(rate)))
	for i := 0; i < n; i++ {
		s[i] *= float64(i) / float64(n)
	}
}

// fadeOutTail ramps the last n samples linearly to exactly 0, so a cut clip
// ends without a step (a click).
func fadeOutTail(s []float64, n int) {
	for i := 0; i < n && i < len(s); i++ {
		s[len(s)-1-i] *= float64(i) / float64(n)
	}
}

// matchRefPitch reports the reference's fundamental when it is usable as a
// pitch (all three UsableAsPitch gates), else 0, corrected for YIN locking
// onto a harmonic (subharmonicCorrection).
func matchRefPitch(w wave.Wave) float64 {
	est := fingerprint.DetectF0Detailed(w)
	ok, _ := fingerprint.UsableAsPitch(w, est)
	if !ok || est.Hz <= 0 {
		return 0
	}
	mag, binHz := matchSpectrum(w)
	f0 := subharmonicCorrection(est.Hz, mag, binHz)
	if pitchExplains(f0, mag, binHz) < matchPitchExplainedMin {
		return 0
	}
	return f0
}

// matchPitchExplainedMin is how much of the strong-partial energy a pitch
// must account for to be THE note: below it the sound has no single note
// (an inharmonic tine or bell cluster), and the reference is matched like an
// unpitched one -- every voice played at whichever pitch within an octave
// sounds closest, as a player imitating it by ear would. The synth kalimba's
// partials at 757, 887, 997, 1122 and 1342 Hz are explained 30-45 % by any
// pitch YIN reads (252, 378, 443, 944 Hz under different nuisances), a
// harmonic tone ~100 %.
const matchPitchExplainedMin = 0.7

// pitchExplains is the share of the strong partials' energy
// (strongPartials) that lies on f0's harmonics 1..matchPartialMaxK -- 0 when
// no partial within matchDominantDB of the strongest is one of harmonics
// 1..matchDominantMaxK. A partial above the 8th harmonic counts as
// unexplained: at 3 % tolerance a low enough "pitch" fits anything up there
// (the real mbira's cluster fits 124 Hz as harmonics 4..7). And a pitch
// carried only by high harmonics is not the note a listener hears: residue
// pitch comes from the low ones (the dominance region), so a LOUD partial
// must be one of them. A loud one, not the loudest: a double reed's formant
// lifts a higher partial over the low ones -- the acclivity oboe's A 440
// peaks at its 6th harmonic (2.66 kHz) with the 2nd and 3rd 2-3 dB under it,
// and requiring the loudest to be harmonic 1..5 read that oboe, and the
// synth cello and bassoon at their own frames, as having no note at all.
func pitchExplains(f0 float64, mag []float64, binHz float64) float64 {
	if f0 <= 0 || binHz <= 0 {
		return 0
	}
	peaks := strongPartials(mag, binHz) // strongest first
	var on, all, loudest float64
	dominant := false
	for i, p := range peaks {
		a := mag[min(len(mag)-1, int(math.Round(p/binHz)))]
		if i == 0 {
			loudest = a
		}
		e := a * a
		all += e
		n := math.Round(p / f0)
		if n < 1 || n > matchPartialMaxK || math.Abs(p-n*f0) > matchPartialTol*p {
			continue
		}
		on += e
		if n <= matchDominantMaxK && a >= loudest*math.Pow(10, matchDominantDB/20) {
			dominant = true
		}
	}
	if !dominant {
		return 0
	}
	if all == 0 {
		return 1
	}
	return on / all
}

// A pitched sound has a partial within matchDominantDB of its strongest
// among its first matchDominantMaxK harmonics (cello-C2's strongest is the
// 3rd, the IOWA oboe's the 3rd, the acclivity oboe's 2nd and 3rd sit 2-3 dB
// under its 6th); the real mbira's cluster, read as 124 Hz, has none (its
// loud partials are 862 Hz alone, the 7th).
const (
	matchDominantMaxK = 5
	matchDominantDB   = -6.0
)

// The partials a pitch must explain are the spectral peaks within
// matchPartialDB of the strongest (at most matchPartialPeaks of them,
// matchPartialMinHz..matchPartialMaxHz), each within matchPartialTol of a
// harmonic.
const (
	matchPartialDB    = -20.0
	matchPartialPeaks = 8
	matchPartialMinHz = 30.0
	matchPartialMaxHz = 6000.0
	matchPartialTol   = 0.03
	// matchPartialMaxK caps the harmonic number a partial may be explained
	// as: at 3 % tolerance, harmonics 9..12 of a low pitch cover a third of
	// the spectrum between them, so a dense inharmonic cluster "fits" a
	// spurious low fundamental.
	matchPartialMaxK = 8
)

// strongPartials is the local maxima of mag within matchPartialDB of the
// strongest, matchPartialMinHz..matchPartialMaxHz, strongest first, at most
// matchPartialPeaks, one per matchPartialTol neighbourhood, each refined to
// its parabolic-interpolated frequency.
func strongPartials(mag []float64, binHz float64) []float64 {
	lo, hi := int(matchPartialMinHz/binHz), min(len(mag)-2, int(matchPartialMaxHz/binHz))
	var peak float64
	for i := lo; i <= hi; i++ {
		peak = math.Max(peak, mag[i])
	}
	if peak == 0 {
		return nil
	}
	thr := peak * math.Pow(10, matchPartialDB/20)
	type pk struct{ hz, amp float64 }
	var all []pk
	for i := max(lo, 1); i <= hi; i++ {
		if mag[i] < thr || mag[i] < mag[i-1] || mag[i] < mag[i+1] {
			continue
		}
		a, b, c := mag[i-1], mag[i], mag[i+1]
		d := 0.0
		if den := a - 2*b + c; den != 0 {
			d = 0.5 * (a - c) / den
		}
		all = append(all, pk{(float64(i) + d) * binHz, b})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].amp > all[j].amp })
	var out []float64
	for _, p := range all {
		near := false
		for _, q := range out {
			if math.Abs(p.hz-q) <= matchPartialTol*q {
				near = true
				break
			}
		}
		if !near {
			out = append(out, p.hz)
		}
		if len(out) == matchPartialPeaks {
			break
		}
	}
	return out
}

// subharmonicCorrection divides a detected f0 that is really a harmonic of
// the note: YIN locks onto the 2nd or 3rd harmonic of a tone whose
// fundamental is weaker than it. A divisor d is taken only when the partials
// of f0/d that are NOT multiples of f0 all carry energy -- the in-between
// harmonics only a lower fundamental explains. A clean tone at f0 has nothing
// but leakage (< -40 dB) at any of them, so a correct pitch is never divided.
//
//   - d = 2 (the acclivity oboe "A 440" measures 886 Hz, yet holds 443 Hz at
//     -9 dB and the odd harmonic 1330 Hz at -1 dB; only a 443 Hz fundamental
//     explains 1330): f0/2 within 20 dB of the peak and 3·f0/2 within 25.
//     AnalyzeF0Support at f0/2 misses the weaker fundamental, so the test is
//     made directly.
//   - d = 3 (cello-C2 measures 199 Hz, G3: its 66 Hz fundamental sits at
//     -25 dB under partials at 133 (-5), 266 (-15) and 332 Hz (-12), and the
//     f0/2 test sees nothing at 100 Hz): partials 1, 2, 4 and 5 of f0/3 all
//     within 30 dB.
//
// Without this every instrument is rendered in the wrong register and the
// ranking judges a different note.
func subharmonicCorrection(f0 float64, mag []float64, binHz float64) float64 {
	if binHz <= 0 {
		return f0
	}
	present := func(hz, db float64) bool { return bandLevelDB(mag, binHz, hz) >= db }
	for i := 0; i < 2; i++ {
		half, third := f0/2, f0/3
		switch {
		case half >= matchSubOctaveMinHz && present(half, matchSubOctaveDB) && present(3*half, matchSubOctaveOddDB):
			f0 = half
		case third >= matchSubOctaveMinHz && present(third, matchSubTwelfthDB) && present(2*third, matchSubTwelfthDB) &&
			present(4*third, matchSubTwelfthDB) && present(5*third, matchSubTwelfthDB):
			f0 = third
		default:
			return f0
		}
	}
	return f0
}

// Sub-harmonic evidence thresholds, relative to the strongest spectral peak.
// The oboe's real fundamental sits at -9 dB and its odd harmonic at -1 dB;
// cello-C2's in-between partials at -5..-25 dB; a tone detected correctly has
// nothing but leakage (< -40 dB) at any of them.
const (
	matchSubOctaveDB    = -20.0
	matchSubOctaveOddDB = -25.0
	matchSubTwelfthDB   = -30.0
	matchSubOctaveMinHz = 25.0
)

// matchSpectrum is the magnitude spectrum of the steady part of the clip
// (from 100 ms after the trimmed onset), skipping the attack whose broadband
// transient would put energy everywhere. 32768 points (~0.74 s at 44.1 kHz)
// so the segment fills the window: MagnitudeSpectrum windows the WHOLE
// buffer, zero padding included.
func matchSpectrum(w wave.Wave) ([]float64, float64) {
	seg := w.Slice(100, 1100)
	if len(seg.Samples) < 2048 {
		seg = w
	}
	return wave.MagnitudeSpectrum(seg, 32768, wave.WindowHann)
}

// bandLevelDB is the strongest bin within ±3 % of hz, in dB relative to the
// strongest bin of the whole spectrum.
func bandLevelDB(mag []float64, binHz, hz float64) float64 {
	var peak, band float64
	lo, hi := int(hz*0.97/binHz), int(math.Ceil(hz*1.03/binHz))
	for i, m := range mag {
		peak = math.Max(peak, m)
		if i >= lo && i <= hi {
			band = math.Max(band, m)
		}
	}
	if peak == 0 || band == 0 {
		return math.Inf(-1)
	}
	return 20 * math.Log10(band/peak)
}

// matchClassOf classifies a reference (see matchClassSustained).
//
// The class is the ENVELOPE's: a sung crowd "ahh" has no single pitch and is
// still a sustained sound, which the percussion traces judge badly (it
// matched a supersaw lead).
func matchClassOf(w wave.Wave) string {
	if w.Duration() < matchShortClipSec {
		return matchClassPercussive
	}
	const win = 0.02
	n := int(win * float64(w.SampleRate))
	if n <= 0 {
		return matchClassPercussive
	}
	var levels []float64
	for i := 0; i+n <= len(w.Samples); i += n {
		var sum float64
		for _, v := range w.Samples[i : i+n] {
			sum += v * v
		}
		levels = append(levels, math.Sqrt(sum/float64(n)))
	}
	var peak float64
	for _, l := range levels {
		peak = math.Max(peak, l)
	}
	if peak == 0 {
		return matchClassPercussive
	}
	// The ONSET of the loud part, not its arg-max: a steady tone's loudest
	// window can sit anywhere, including the last 0.3 s, where there is no
	// "later" left to look at.
	peakAt := 0
	for i, l := range levels {
		if l >= peak*matchPeakFrac {
			peakAt = i
			break
		}
	}
	// A swell that only peaks near the end of the window shows no decay at
	// all: judge it on the last window rather than calling it struck.
	later := min(peakAt+int(matchDecaySec/win), len(levels)-1)
	if 20*math.Log10(levels[later]/peak+1e-12) < matchDecayDB {
		return matchClassPercussive
	}
	return matchClassSustained
}

// matchSearchSemis is how far an unpitched sound (or an unpitched voice) is
// pitch-searched either side of the voice's natural pitch, and the coarse
// grid step before the search refines to 1 semitone around the best. A
// voice resampled further than an octave is not that instrument any more;
// no player would play it there to imitate a sound.
const (
	matchSearchSemis = 12.0
	matchSearchStep  = 3.0
)

// matchVoiceMarginSec is how much longer than the reference's sounding region
// a voice is held, so its release starts after the compared region ends.
const matchVoiceMarginSec = 0.15

// matchPlan is how one instrument is played against a reference.
type matchPlan struct {
	// skip: the instrument cannot sound this note as itself (a non-melodic
	// voice that would need resampling past matchSearchSemis).
	skip bool
	// pitches are the node pitches to try when !search; the closest is kept.
	pitches []float64
	// search: try pitches around 0 and keep the closest.
	search bool
}

// matchPitchPlan decides how inst is played against a reference at f0
// (0 = unpitched). The pitch is the player's choice, so it is never what the
// ranking judges:
//
//   - pitched reference, pitched voice: rendered AT the reference's note --
//     melodic voices render there; a non-melodic voice is resampled there,
//     but only within an octave of its natural pitch (a snare slowed to D2
//     once scored closer to a cello than any string). A non-melodic voice
//     also tries the octave either side (still within that octave of its
//     natural pitch): a drum membrane's modes sit near 1 : 1.5, so the
//     pitch of a drum recording reads as the struck note or the residue an
//     octave below it depending on how the modes decay -- the shipped tom
//     re-played 5 semitones up read 86 Hz instead of 171 and was resampled
//     an octave away from itself (#14, refs match -nuisance). Melodic
//     voices keep the one note: trying octaves for them graded as noise.
//   - anything unpitched: the voice is tried across +/-matchSearchSemis and
//     the pitch that sounds closest is kept (drums are pitched by resampling
//     in the engine, exactly as here).
func matchPitchPlan(inst instrument.ID, f0 float64) matchPlan {
	if f0 > 0 {
		if _, pitched := audio.InstrumentPitchInfo(inst); pitched {
			p := matchRenderPitch(inst, f0)
			if audio.IsMelodicInstrument(inst) {
				return matchPlan{pitches: []float64{p}}
			}
			var ps []float64
			for _, c := range []float64{p, p - 12, p + 12} {
				if math.Abs(c) <= matchSearchSemis {
					ps = append(ps, c)
				}
			}
			if len(ps) == 0 {
				return matchPlan{skip: true}
			}
			return matchPlan{pitches: ps}
		}
	}
	return matchPlan{search: true}
}

// matchRenderPitch is the node pitch at which inst sounds f0 AS THE MATCHER
// HEARS IT: the voice's own pitch at node pitch 0, measured by the very
// pipeline that measured the reference (matchVoiceFrame), so whatever that
// pipeline reads into a timbre -- a virtual pitch, a strong sub-octave --
// is read the same way on both sides. A voice the pipeline cannot pitch
// falls back to its declared frame (InstrumentPitchInfo). An unpitched
// reference, or an unpitched instrument, renders at its natural pitch.
func matchRenderPitch(inst instrument.ID, f0 float64) float64 {
	if f0 <= 0 {
		return 0
	}
	off, pitched := audio.InstrumentPitchInfo(inst)
	if !pitched {
		return 0
	}
	if hz, ok := matchVoiceFrame(inst); ok {
		return 12 * math.Log2(f0/hz)
	}
	return 12*math.Log2(f0/a3Hz) - off
}

// matchVoiceFrame is the pitch matchRefPitch measures on inst played at
// node pitch 0, the way a reference is measured (onset-trimmed, at most
// adoptMaxSecDefault); ok=false when the voice has no usable pitch or it
// sits more than an octave (and a half step) from the declared frame. An
// octave is a reading, not a failure -- a drawbar organ's 16' IS its lowest
// pitch -- but further off than that the detector has failed. Cached per
// sound.
//
// The declared frame alone is not enough: the curated melodic voices are
// built so pitch 0 is A3, yet the drawbar organ measures 110 Hz there (its
// 16' stop) -- so a recording of that very voice, read at its 16' pitch, was
// matched against the voice an octave away.
func matchVoiceFrame(inst instrument.ID) (float64, bool) {
	key := string(inst) + "=" + audio.InstrumentSoundHash(inst, nil)
	if v, ok := matchFrames.Load(key); ok {
		hz := v.(float64)
		return hz, hz > 0
	}
	hz := 0.0
	if w, err := synthmatch.RenderHeld(inst, 0, wavref.Rate, 0, 0, nil); err == nil {
		ref := wave.Wave{Samples: trimMatchClip(w.Samples, w.SampleRate, adoptMaxSecDefault), SampleRate: w.SampleRate}
		hz = matchRefPitch(ref)
	}
	if off, _ := audio.InstrumentPitchInfo(inst); hz > 0 && math.Abs(12*math.Log2(hz/a3Hz)-off) > matchSearchSemis+0.5 {
		hz = 0
	}
	matchFrames.Store(key, hz)
	matchFramesMeasured.Store(true)
	return hz, hz > 0
}

var matchFrames sync.Map // instrument=soundhash -> measured Hz at node pitch 0 (0 = none)

// matchFramesMeasured: this process measured a frame persistMatchFrames has
// not seen.
var matchFramesMeasured atomic.Bool

// matchFramesFile carries matchVoiceFrame across runs of the SAME refs
// binary: measuring every voice's frame is 150 renders and pitch detections
// (~1.8 s, close to half of matching one recording). It is keyed by the
// binary's own hash, so a rebuilt matcher -- any change to the renderer or
// the pitch pipeline -- measures afresh; each entry's sound hash covers a
// retuned voice.
type matchFramesFile struct {
	Build  string             `json:"build"`
	Frames map[string]float64 `json:"frames"`
}

// persistMatchFrames seeds matchFrames from path when this binary wrote it
// and returns the function that writes them back if any were measured. Only
// the CLI calls it, so tests never touch the reference directory.
func persistMatchFrames(path string) (save func()) {
	exe, err := os.Executable()
	if err != nil {
		return func() {}
	}
	build, err := sha256File(exe)
	if err != nil {
		return func() {}
	}
	var f matchFramesFile
	if b, err := os.ReadFile(path); err == nil && json.Unmarshal(b, &f) == nil && f.Build == build {
		for k, hz := range f.Frames {
			matchFrames.Store(k, hz)
		}
	}
	return func() {
		if !matchFramesMeasured.Load() {
			return
		}
		out := matchFramesFile{Build: build, Frames: map[string]float64{}}
		matchFrames.Range(func(k, v any) bool {
			out.Frames[k.(string)] = v.(float64)
			return true
		})
		b, err := json.Marshal(out)
		if err != nil || os.MkdirAll(filepath.Dir(path), 0o755) != nil {
			return
		}
		tmp, err := os.CreateTemp(filepath.Dir(path), ".frames-*.json")
		if err != nil {
			return
		}
		_, werr := tmp.Write(b)
		if cerr := tmp.Close(); werr == nil && cerr == nil {
			werr = os.Rename(tmp.Name(), path) // concurrent runs: last writer wins, never a torn file
		}
		if werr != nil {
			_ = os.Remove(tmp.Name())
		}
	}
}

type matchRender struct {
	cand   matchCandidate
	w      wave.Wave
	ess    fingerprint.EssenceScore
	traits fingerprint.EssenceTraits
	// rawEssence is the essence distance before the hubness correction
	// (match_hubness.go); cand.Score is the corrected one.
	rawEssence float64
	silent     bool
}

// renderMatchVoice renders inst at node pitch p as a player would hold it
// against a reference whose sounding region is needSec long: held that long,
// trimmed of leading silence exactly like the reference (so both onsets sit
// at 0), clipSec long.
func renderMatchVoice(inst instrument.ID, p float64, sr int, needSec, clipSec float64) (wave.Wave, float64, error) {
	held := synthmatch.HeldSeconds(inst, p, needSec+matchVoiceMarginSec)
	w, err := synthmatch.RenderHeld(inst, p, sr, 0, held, nil)
	if err != nil {
		return wave.Wave{}, held, err
	}
	return fitMatchClip(w, string(inst), sr, clipSec), held, nil
}

// renderMatchDoc is renderMatchVoice for a document that need not be
// registered (a `refs tune` candidate): the same hold, trim and length, so
// a candidate is played against a recording exactly as the roster is.
func renderMatchDoc(d instrument.Doc, label string, p float64, sr int, needSec, clipSec float64) (wave.Wave, float64, error) {
	held := synthmatch.HeldSecondsDoc(d, p, needSec+matchVoiceMarginSec)
	w, err := synthmatch.RenderHeldDoc(d, p, sr, 0, held)
	if err != nil {
		return wave.Wave{}, held, err
	}
	return fitMatchClip(w, label, sr, clipSec), held, nil
}

// fitMatchClip trims a rendition's leading silence exactly like the
// reference's and pads it to clipSec.
func fitMatchClip(w wave.Wave, label string, sr int, clipSec float64) wave.Wave {
	s := trimMatchClip(w.Samples, sr, clipSec)
	if n := int(clipSec * float64(sr)); len(s) < n {
		s = append(s, make([]float64, n-len(s))...)
	}
	return wave.Wave{Samples: s, SampleRate: sr, Label: label}
}

// matchTarget is what every rendition is judged against: the reference's
// essence, its pitch and its length.
type matchTarget struct {
	f0      float64
	refE    fingerprint.Essence
	sr      int
	clipSec float64
}

// eval renders inst at node pitch p against the target and measures it.
func (t matchTarget) eval(inst instrument.ID, p float64) (matchRender, error) {
	w, held, err := renderMatchVoice(inst, p, t.sr, t.refE.WindowSec, t.clipSec)
	if err != nil {
		return matchRender{}, fmt.Errorf("render %s at %+.2f: %w", inst, p, err)
	}
	return t.measure(inst, p, w, held), nil
}

// evalDoc is eval for an unregistered document played as inst would be.
func (t matchTarget) evalDoc(d instrument.Doc, inst instrument.ID, p float64) (matchRender, error) {
	w, held, err := renderMatchDoc(d, string(inst), p, t.sr, t.refE.WindowSec, t.clipSec)
	if err != nil {
		return matchRender{}, fmt.Errorf("render %s candidate at %+.2f: %w", inst, p, err)
	}
	return t.measure(inst, p, w, held), nil
}

// measure scores one rendition against the target.
func (t matchTarget) measure(inst instrument.ID, p float64, w wave.Wave, held float64) matchRender {
	e := fingerprint.AnalyzeEssence(w, fingerprint.EssenceOpts{WindowSec: t.refE.WindowSec})
	return matchRender{
		cand:   matchCandidate{Instrument: inst, RenderPitch: p, RenderSec: held},
		w:      w,
		ess:    fingerprint.EssenceTerms(t.refE, e),
		traits: e.Traits(),
		silent: e.Silent || isNearSilent(w.Samples),
	}
}

// matchSearch is one instrument's pitch-plan search against a target: the
// distance at every pitch tried and the closest rendition so far.
type matchSearch struct {
	inst instrument.ID
	// doc, when set, is played in place of inst's shipped document (still
	// with inst's pitch plan): a `refs tune` candidate.
	doc   *instrument.Doc
	plan  matchPlan
	tried map[float64]float64
	best  matchRender
	obj   float64
	have  bool
}

func newMatchSearch(inst instrument.ID, f0 float64) *matchSearch {
	return &matchSearch{inst: inst, plan: matchPitchPlan(inst, f0), tried: map[float64]float64{}}
}

// consider tries pitch p (once) and keeps it if it is strictly closer: ties
// keep the earlier pitch, as the plan orders them.
func (s *matchSearch) consider(t matchTarget, p float64) error {
	if _, ok := s.tried[p]; ok {
		return nil
	}
	var r matchRender
	var err error
	if s.doc != nil {
		r, err = t.evalDoc(*s.doc, s.inst, p)
	} else {
		r, err = t.eval(s.inst, p)
	}
	if err != nil {
		return err
	}
	o := math.Inf(1)
	if !r.silent {
		o = fingerprint.EssenceTotal(r.ess)
	}
	s.tried[p] = o
	if !s.have || o < s.obj {
		s.best, s.obj, s.have = r, o, true
	}
	return nil
}

// scout is the first, cheap look at the instrument: every pitch of a fixed
// plan, or, for a search, every matchScoutStep semitones (a subset of the
// search grid, so settling later re-renders nothing). A fixed plan is never
// cut short: its octave alternatives answer a reading an octave off, which
// the primary pitch cannot see -- the tom re-played +5 st in a room reads its
// 85 Hz residue, and scouted at that pitch alone it ranked itself #31
// (TestEssenceSelfIdentifiesUnderNuisance).
func (s *matchSearch) scout(t matchTarget) error {
	switch {
	case s.plan.skip:
		return nil
	case s.plan.search:
		for p := -matchSearchSemis; p <= matchSearchSemis; p += matchScoutStep {
			if err := s.consider(t, p); err != nil {
				return err
			}
		}
		return nil
	}
	for _, p := range s.plan.pitches {
		if err := s.consider(t, p); err != nil {
			return err
		}
	}
	return nil
}

// settle completes a search: the matchSearchStep grid, then 2 and 1
// semitones either side of the closest (a fixed plan is complete once
// scouted). A scouted instrument settles exactly where an unscouted one
// would: the scouted pitches are grid points, and ties keep the earlier
// pitch.
func (s *matchSearch) settle(t matchTarget) error {
	if s.plan.skip || !s.plan.search {
		return nil
	}
	for p := -matchSearchSemis; p <= matchSearchSemis; p += matchSearchStep {
		if err := s.consider(t, p); err != nil {
			return err
		}
	}
	for step := matchSearchStep - 1; step >= 1; step-- {
		c := s.best.cand.RenderPitch
		for _, p := range []float64{c - step, c + step} {
			if math.Abs(p) <= matchSearchSemis {
				if err := s.consider(t, p); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// matchScoutStep is the scouting grid of a pitch search (see matchSearch.scout),
// and matchContenders how many of the closest scouted instruments settle.
// The rest keep their scouted distance: they are out of the running, and
// settling all of them cost most of every ranking (refs match -validate grades
// the same either way; spec 2026-10-17, "where the time goes").
const (
	matchScoutStep  = 6.0
	matchContenders = 30
)

// matchSettleAll settles every instrument: the exact, exhaustive ranking a
// calibration is measured on.
func matchSettleAll(instrument.ID) bool { return true }

// forEachParallel runs fn(i) for i in [0, n) on workers goroutines.
func forEachParallel(n, workers int, fn func(i int)) {
	jobs := make(chan int)
	var wg sync.WaitGroup
	for k := 0; k < max(1, workers); k++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				fn(i)
			}
		}()
	}
	for i := 0; i < n; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
}

// rankRoster plays every id against the reference and ranks it by the
// hubness-corrected essence distance, best first. Every instrument is
// SCOUTED (matchSearch.scout); the matchContenders closest by the corrected
// essence distance -- plus every id settle accepts (nil: none; the ones a
// caller reports on by name, or matchSettleAll for a calibration) -- then
// SETTLE their full pitch plan. Renders run in parallel: each is an independent offline C render
// (lazy C tables initialise through beatmo_once, so cold concurrent renders
// are safe -- TestColdConcurrentRendersMatchSerial).
func rankRoster(ref wave.Wave, f0 float64, ids []instrument.ID, workers int, settle func(id instrument.ID) bool) ([]matchRender, error) {
	t := matchTarget{f0: f0, refE: fingerprint.AnalyzeEssence(ref, fingerprint.EssenceOpts{}), sr: wavref.Rate, clipSec: ref.Duration()}
	searches := make([]*matchSearch, len(ids))
	errs := make([]error, len(ids))
	forEachParallel(len(ids), workers, func(i int) {
		searches[i] = newMatchSearch(ids[i], f0)
		errs[i] = searches[i].scout(t)
	})
	var hubs map[instrument.ID]float64
	if h, err := committedHubness(); err == nil {
		hubs, _ = h.hubValues(ids)
	}
	var open []int
	for i, s := range searches {
		if errs[i] == nil && s.have {
			open = append(open, i)
		}
	}
	sort.SliceStable(open, func(a, b int) bool {
		sa, sb := searches[open[a]], searches[open[b]]
		return hubCorrected(sa.obj, hubs[sa.inst]) < hubCorrected(sb.obj, hubs[sb.inst])
	})
	var settling []int
	for k, i := range open {
		if k < matchContenders || settle != nil && settle(ids[i]) {
			settling = append(settling, i)
		}
	}
	forEachParallel(len(settling), workers, func(k int) {
		i := settling[k]
		errs[i] = searches[i].settle(t)
	})

	var kept []matchRender
	var firstErr error
	for i, s := range searches {
		if errs[i] != nil {
			if firstErr == nil {
				firstErr = errs[i]
			}
			continue // one broken voice must not sink the whole ranking
		}
		if s.have {
			kept = append(kept, s.best)
		}
	}
	if len(kept) == 0 {
		if firstErr != nil {
			return nil, firstErr
		}
		return nil, fmt.Errorf("refs match: no instrument can be played against this reference")
	}
	setEssenceScores(kept)
	applyHubs(kept, hubs)
	sort.SliceStable(kept, func(a, b int) bool { return kept[a].cand.Score < kept[b].cand.Score })
	return kept, nil
}

// setEssenceScores sets every render's raw essence distance (silent = +Inf).
func setEssenceScores(rs []matchRender) {
	terms := make([]fingerprint.EssenceScore, len(rs))
	silent := make([]bool, len(rs))
	for i, r := range rs {
		terms[i], silent[i] = r.ess, r.silent
	}
	tot := fingerprint.EssenceTotalsOf(terms, silent)
	for i := range rs {
		rs[i].rawEssence = tot[i]
	}
}

// writeMatchClips level-matches the reference and the top renders as one set
// (familyLevelGains, the same plan `refs audition` uses) and writes them.
func writeMatchClips(dir string, ref wave.Wave, top []matchRender) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	rms := []float64{measureWave(ref).RMS}
	peaks := []float64{measureWave(ref).Peak}
	for _, r := range top {
		m := measureWave(r.w)
		rms, peaks = append(rms, m.RMS), append(peaks, m.Peak)
	}
	gains, err := familyLevelGains(matchRMS, rms, peaks)
	if err != nil {
		return "", err
	}
	refPath := filepath.Join(dir, "0__REFERENCE.wav")
	if err := writeCompareWAV(refPath, scaleSamples(ref.Samples, gains[0]), ref.SampleRate); err != nil {
		return "", err
	}
	for i := range top {
		p := filepath.Join(dir, fmt.Sprintf("%d__%s.wav", i+1, top[i].cand.Instrument))
		if err := writeCompareWAV(p, scaleSamples(top[i].w.Samples, gains[i+1]), top[i].w.SampleRate); err != nil {
			return "", err
		}
		top[i].cand.Clip = absOrSelf(p)
	}
	return absOrSelf(refPath), nil
}

// namesakeInFilename returns the roster id a recording's filename names,
// "" when none: an id whose words all appear, in order and contiguous, among
// the filename's words ("22686__acclivity__oboe_a_440" names oboe,
// "360251__mtg__piano-grand-c3" piano-grand). The longest such id wins.
//
// It is a HINT for the listener, never the match: instrumentFromFilename is
// the strict resolver adoption needs, where a wrong guess silently repoints
// an instrument. Here a namesake only adds one more clip to hear.
func namesakeInFilename(name string, ids []instrument.ID) instrument.ID {
	words := strings.FieldsFunc(strings.ToLower(strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
	var best instrument.ID
	for _, id := range ids {
		parts := strings.Split(string(id), "-")
		for i := 0; i+len(parts) <= len(words); i++ {
			ok := true
			for k, p := range parts {
				if words[i+k] != p {
					ok = false
					break
				}
			}
			if ok && len(id) > len(best) {
				best = id
			}
		}
	}
	return best
}

// noteNameForHz names the nearest equal-tempered note (A4 = 440 Hz).
func noteNameForHz(hz float64) string {
	if hz <= 0 {
		return ""
	}
	midi := int(math.Round(69 + 12*math.Log2(hz/440)))
	names := [...]string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}
	return fmt.Sprintf("%s%d", names[((midi%12)+12)%12], midi/12-1)
}

// matchCatalogueGlob is the -wavs word for every catalogued INSTRUMENT
// reference present on this machine (the reference-audio root also holds
// whole songs and scratch files no instrument is).
const matchCatalogueGlob = "catalogue"

// expandMatchGlob expands a comma-separated list of globs ("~/" = home;
// matchCatalogueGlob = the catalogued instrument references) into the
// sorted, de-duplicated files they name.
func expandMatchGlob(globs string) ([]string, error) {
	seen := map[string]bool{}
	var paths []string
	for _, glob := range strings.Split(globs, ",") {
		glob = strings.TrimSpace(glob)
		if glob == "" {
			continue
		}
		if glob == matchCatalogueGlob {
			c, err := loadCatalogue()
			if err != nil {
				return nil, err
			}
			for _, r := range c.Refs {
				if r.Status != "" || r.Tier != "instrument" {
					continue
				}
				if p, ok := wavref.Resolve(r.ID); ok && !seen[p] {
					seen[p] = true
					paths = append(paths, p)
				}
			}
			continue
		}
		if strings.HasPrefix(glob, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}
			glob = filepath.Join(home, glob[2:])
		}
		ps, err := filepath.Glob(glob)
		if err != nil {
			return nil, err
		}
		for _, p := range ps {
			if !seen[p] {
				seen[p] = true
				paths = append(paths, p)
			}
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// runMatch ranks the roster against every WAV the glob names. Entries whose
// file bytes, metric and roster sounds are unchanged are reused from the
// manifest, so re-running after a few new downloads only analyses those.
func runMatch(w, diag io.Writer, o matchOpts) (matchManifest, error) {
	paths, err := expandMatchGlob(o.Glob)
	if err != nil {
		return matchManifest{}, err
	}
	if len(paths) == 0 {
		return matchManifest{}, fmt.Errorf("refs match: no files match %q", o.Glob)
	}
	if err := os.MkdirAll(o.OutDir, 0o755); err != nil {
		return matchManifest{}, err
	}
	prev, err := loadMatchManifest(o.OutDir)
	if err != nil {
		return matchManifest{}, err
	}
	cached := map[string]matchEntry{}
	for _, e := range prev.Entries {
		cached[e.SHA256] = e
	}
	ids := rosterIDs()
	rh := rosterHash(ids)
	warnStaleHubs(diag, ids)
	// What a recording is labelled as (catalogue or filename) and its family
	// always settle, so the map's rank and losing term for it are exact.
	// (No catalogue or families only means nothing extra settles.)
	cat, _ := loadCatalogue()
	famOf, _ := matchFamilies(cat)

	man := matchManifest{Version: matchManifestVersion}
	bySHA := map[string]int{}
	for _, p := range paths {
		sum, err := sha256File(p)
		if err != nil {
			return matchManifest{}, err
		}
		name := filepath.Base(p)
		if i, dup := bySHA[sum]; dup {
			man.Entries[i].Duplicates = append(man.Entries[i].Duplicates, name)
			continue
		}
		if e, ok := cached[sum]; ok && !o.Force && e.RosterHash == rh && len(e.Ranking) >= o.Top && clipsExist(e, o.Top) {
			e.Name, e.Source, e.Duplicates = name, p, nil
			bySHA[sum] = len(man.Entries)
			man.Entries = append(man.Entries, e)
			fmt.Fprintf(diag, "cached   %s\n", name)
			continue
		}
		fmt.Fprintf(diag, "analyse  %s ... ", name)
		dir := filepath.Join(o.OutDir, matchSlug(name))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return matchManifest{}, err
		}
		ref, err := loadMatchReference(p, dir, o.MaxSec)
		if err != nil {
			fmt.Fprintf(diag, "FAIL %v\n", err)
			continue
		}
		f0 := matchRefPitch(ref)
		class := matchClassOf(ref)
		namesake := namesakeInFilename(name, ids)
		label := matchLabel(p, namesake, cat)
		want := matchWanted(label, famOf)
		ranked, err := rankRoster(ref, f0, ids, o.Workers, func(id instrument.ID) bool { return want[id] })
		if err != nil {
			fmt.Fprintf(diag, "FAIL %v\n", err)
			continue
		}
		top := append([]matchRender(nil), ranked...)
		if len(top) > o.Top {
			top = top[:o.Top]
		}
		var best instrument.ID
		bestAt := 0
		for i, r := range ranked {
			if want[r.cand.Instrument] {
				best, bestAt = r.cand.Instrument, i
				break
			}
		}
		if best != "" && bestAt >= len(top) {
			top = append(top, ranked[bestAt])
		}
		// Clips from a previous run with a different top-N would linger and
		// confuse a listener browsing the directory.
		old, _ := filepath.Glob(filepath.Join(dir, "*__*.wav"))
		for _, f := range old {
			_ = os.Remove(f)
		}
		refPath, err := writeMatchClips(dir, ref, top)
		if err != nil {
			return matchManifest{}, err
		}
		e := matchEntry{
			Name: name, Source: p, SHA256: sum, Reference: refPath,
			DurationSec: ref.Duration(), F0Hz: f0, Pitched: f0 > 0, Note: noteNameForHz(f0),
			Class: class, RosterHash: rh, Namesake: namesake, Label: label, LabelBest: best,
			Traits: fingerprint.AnalyzeEssence(ref, fingerprint.EssenceOpts{}).Traits(),
		}
		clipOf := map[string]string{}
		for _, r := range top {
			clipOf[string(r.cand.Instrument)] = r.cand.Clip
		}
		for _, r := range ranked {
			c := r.cand
			c.Clip = clipOf[string(c.Instrument)]
			c.SoundHash = audio.InstrumentSoundHash(c.Instrument, nil)
			terms, traits := r.ess, r.traits
			c.Terms, c.Traits = &terms, &traits
			e.Ranking = append(e.Ranking, c)
		}
		bySHA[sum] = len(man.Entries)
		man.Entries = append(man.Entries, e)
		fmt.Fprintf(diag, "%s (%.3f)\n", e.Ranking[0].Instrument, e.Ranking[0].Score)
	}
	if err := writeMatchManifest(o.OutDir, man); err != nil {
		return matchManifest{}, err
	}
	printMatchTable(w, man, o.Top)
	fmt.Fprintf(w, "\nclips + manifest: %s\nlisten and approve/reject: make ab-match\n", o.OutDir)
	return man, nil
}

func clipsExist(e matchEntry, top int) bool {
	if _, err := os.Stat(e.Reference); err != nil {
		return false
	}
	for i, c := range e.Ranking {
		if i >= top && c.Instrument != e.LabelBest {
			continue
		}
		if _, err := os.Stat(c.Clip); err != nil {
			return false
		}
	}
	return true
}

// printMatchTable shows every recording's closest voices beside what it is
// labelled as (LABELLED: the label, the rank of its closest family member,
// and that member when it is not the label itself).
func printMatchTable(w io.Writer, man matchManifest, top int) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	hdr := "WAV\tPITCH\tLABELLED"
	for i := 1; i <= top; i++ {
		hdr += fmt.Sprintf("\t#%d", i)
	}
	fmt.Fprintln(tw, hdr)
	for _, e := range man.Entries {
		pitch := "unpitched"
		if e.Pitched {
			pitch = fmt.Sprintf("%s %.1fHz", e.Note, e.F0Hz)
		}
		row := fmt.Sprintf("%s\t%s\t%s", matchShortName(e.Name), pitch, labelCell(e))
		for i := 0; i < top && i < len(e.Ranking); i++ {
			row += fmt.Sprintf("\t%s %.3f", e.Ranking[i].Instrument, e.Ranking[i].Score)
		}
		fmt.Fprintln(tw, row)
	}
	tw.Flush()
}

// labelCell is the LABELLED column: "-" for an unlabelled recording, else
// "harp #2", or "kick #1 kick-808" when the closest family member is not the
// labelled instrument itself.
func labelCell(e matchEntry) string {
	if len(e.Label) == 0 {
		return "-"
	}
	if e.LabelBest == "" {
		return string(e.Label[0]) + " (not ranked)"
	}
	cell := fmt.Sprintf("%s #%d", e.Label[0], rankOf(e, e.LabelBest))
	if !slices.Contains(e.Label, e.LabelBest) {
		cell += " " + string(e.LabelBest)
	}
	return cell
}

// rankOf is inst's 1-based position in e's ranking (0 = not ranked).
func rankOf(e matchEntry, inst instrument.ID) int {
	for i, c := range e.Ranking {
		if c.Instrument == inst {
			return i + 1
		}
	}
	return 0
}

// matchShortName keeps a table readable: freesound names run to 100 chars.
func matchShortName(name string) string {
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	if len(stem) > 48 {
		return stem[:45] + "..."
	}
	return stem
}

func cmdMatch(args []string) error {
	fs := flag.NewFlagSet("match", flag.ExitOnError)
	glob := fs.String("wavs", "~/Downloads/*.wav", "glob(s) of recordings to match, comma-separated (quote them so the shell does not expand them); \""+matchCatalogueGlob+"\" = every catalogued instrument reference")
	out := fs.String("out", "", "review directory (default $BEATMO_WAV_REFS/matches)")
	top := fs.Int("top", 3, "how many closest instruments to keep clips for")
	maxSec := fs.Float64("max-sec", adoptMaxSecDefault, "analysis window after the leading-silence trim")
	workers := fs.Int("workers", defaultMatchWorkers(), "parallel renders")
	force := fs.Bool("force", false, "re-analyse every file, ignoring the manifest cache")
	listen := fs.Bool("listen", false, "interactive: play each reference against its match and record approve/reject + notes")
	verdicts := fs.String("verdicts", "", "verdicts jsonl (default internal/audio/testdata/wav_match_verdicts.jsonl)")
	resume := fs.Bool("resume", true, "-listen: skip recordings that already have a verdict")
	only := fs.String("only", "", "-listen: only recordings whose filename contains this text")
	validate := fs.Bool("validate", false, "rank every catalogued reference and report how often its own instrument comes first")
	calibrate := fs.Bool("calibrate", false, "measure every instrument's hubness over the catalogued references and write "+matchHubnessRel)
	nuisance := fs.Bool("nuisance", false, "re-play shipped voices with one nuisance each (pitch, note length, room, reverb, microphone) and report whether the matcher still finds the voice")
	mapOut := fs.Bool("map", false, "after ranking, write MAP.md beside the clips: every recording's closest voices against what it is labelled as, and every instrument's coverage (matched / fidelity gap / stand-in / uncovered)")
	explain := fs.String("explain", "", "comma-separated instruments: for every -wavs recording, print each essence term of the top -top candidates and of these (\"-\" = only the top)")
	cases := fs.String("cases", "", "-nuisance: comma-separated instruments to re-play (default a fixed set spanning every kind; all = the whole roster)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("refs match: unexpected argument %q (quote -wavs globs)", fs.Arg(0))
	}
	// Beside the references every mode reads (Root, not matchDir: -validate
	// and -nuisance run without $BEATMO_WAV_REFS set).
	defer persistMatchFrames(filepath.Join(wavref.Root(), "matches", "frames.json"))()
	if *calibrate {
		return runMatchCalibrate(os.Stdout, os.Stderr, *workers)
	}
	if *explain != "" {
		var insts []instrument.ID
		if *explain != "-" {
			var err error
			if insts, err = instrument.ParseIDs(strings.Split(*explain, ",")); err != nil {
				return fmt.Errorf("-explain: %w", err)
			}
		}
		return runMatchExplain(os.Stdout, os.Stderr, *glob, *maxSec, insts, *top, *workers)
	}
	if *nuisance {
		cs := matchNuisanceCases
		switch v := strings.TrimSpace(*cases); v {
		case "":
		case "all":
			cs = rosterIDs()
		default:
			var err error
			if cs, err = instrument.ParseIDs(strings.Split(v, ",")); err != nil {
				return fmt.Errorf("-cases: %w", err)
			}
		}
		return runMatchNuisance(os.Stdout, os.Stderr, cs, matchNuisances, *workers)
	}
	if *validate {
		return runMatchValidate(os.Stdout, os.Stderr, *workers)
	}
	dir := *out
	if dir == "" {
		d, err := matchDir()
		if err != nil {
			return err
		}
		dir = d
	}
	o := matchOpts{Glob: *glob, OutDir: dir, Top: *top, MaxSec: *maxSec, Workers: *workers, Force: *force}
	man, err := runMatch(os.Stdout, os.Stderr, o)
	if err != nil {
		return err
	}
	if *mapOut {
		if err := writeMatchMap(os.Stdout, dir, man); err != nil {
			return err
		}
	}
	if !*listen {
		return nil
	}
	vp := *verdicts
	if vp == "" {
		root, ok := wavref.ModuleRoot()
		if !ok {
			return fmt.Errorf("refs match: cannot locate the module root (no go.mod above cwd)")
		}
		vp = filepath.Join(root, matchVerdictsRel)
	}
	return runMatchListen(os.Stdout, os.Stdin, man, vp, *resume, *only, o.Top, defaultPlayer)
}

func defaultMatchWorkers() int { return max(1, runtime.NumCPU()-2) }
