//go:build !js

package main

import (
	"encoding/json"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// matchTestTone renders a steady tone with the given per-harmonic amplitudes
// (amps[0] = fundamental).
func matchTestTone(f0 float64, amps []float64, durSec float64) wave.Wave {
	const sr = 44100
	n := int(durSec * sr)
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / sr
		for k, a := range amps {
			s[i] += a * math.Sin(2*math.Pi*f0*float64(k+1)*t)
		}
		s[i] *= 0.2
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

// A double-reed tone whose fundamental is weaker than its second harmonic is
// the oboe case: whatever the detector locks onto, the pitch reported must be
// the fundamental, because rendering the roster an octave above the recording
// judges every instrument in the wrong register.
func TestMatchRefPitchFindsAWeakFundamental(t *testing.T) {
	w := matchTestTone(220, []float64{0.35, 1, 1, 0.25, 0.6, 0.4}, 2)
	if got := matchRefPitch(w); math.Abs(got-220) > 3 {
		t.Fatalf("matchRefPitch = %.1f Hz, want ~220 (the weak fundamental)", got)
	}
}

// The sub-octave correction must never halve a pitch that was right: a clean
// harmonic tone has no energy below its fundamental.
func TestMatchRefPitchKeepsACorrectPitch(t *testing.T) {
	for _, f0 := range []float64{110, 440, 880} {
		w := matchTestTone(f0, []float64{1, 0.5, 0.3, 0.2}, 2)
		if got := matchRefPitch(w); math.Abs(got-f0)/f0 > 0.02 {
			t.Errorf("f0 %.0f: matchRefPitch = %.1f Hz", f0, got)
		}
	}
}

// YIN can lock onto the THIRD harmonic too (cello-C2 read as G3): the
// in-between partials of f0/3 are what only the real fundamental explains.
func TestSubharmonicCorrectionFindsAThirdHarmonicLock(t *testing.T) {
	const binHz = 1.0
	spec := func(peaks map[float64]float64) []float64 {
		mag := make([]float64, 4000)
		for i := range mag {
			mag[i] = 1e-5 // -100 dB floor
		}
		for hz, db := range peaks {
			mag[int(hz)] = math.Pow(10, db/20)
		}
		return mag
	}
	// cello-C2 as measured: 66 Hz at -25, 133 at -5, 199 at 0, 266 at -15, 332 at -12.
	cello := spec(map[float64]float64{66: -25, 133: -5, 199: 0, 266: -15, 332: -12, 399: -20})
	if got := subharmonicCorrection(199, cello, binHz); math.Abs(got-199.0/3) > 0.01 {
		t.Errorf("third-harmonic lock: got %.1f Hz, want %.1f", got, 199.0/3)
	}
	// A clean harmonic tone at 199 Hz: nothing below it, nothing in between.
	clean := spec(map[float64]float64{199: 0, 398: -6, 597: -10, 796: -14})
	if got := subharmonicCorrection(199, clean, binHz); got != 199 {
		t.Errorf("a correct pitch was divided: got %.1f Hz", got)
	}
	// The oboe octave error keeps working: 443 at -9, 886 at 0, 1329 at -1.
	oboe := spec(map[float64]float64{443: -9, 886: 0, 1329: -1, 1772: -8})
	if got := subharmonicCorrection(886, oboe, binHz); math.Abs(got-443) > 0.01 {
		t.Errorf("octave lock: got %.1f Hz, want 443", got)
	}
}

func TestMatchClassOf(t *testing.T) {
	sustained := matchTestTone(220, []float64{1, 0.5}, 2)
	if c := matchClassOf(sustained); c != matchClassSustained {
		t.Errorf("steady 2 s tone = %s, want sustained", c)
	}
	struck := matchTestTone(220, []float64{1, 0.5}, 2)
	for i := range struck.Samples {
		struck.Samples[i] *= math.Exp(-float64(i) / 44100 / 0.05)
	}
	if c := matchClassOf(struck); c != matchClassPercussive {
		t.Errorf("50 ms-decay tone = %s, want percussive", c)
	}
	// A swell that peaks in the last 0.3 s of the window is still sustained:
	// nothing in it decays.
	swell := matchTestTone(220, []float64{1, 0.5}, 2)
	for i := range swell.Samples {
		swell.Samples[i] *= math.Min(1, float64(i)/44100/1.9)
	}
	if c := matchClassOf(swell); c != matchClassSustained {
		t.Errorf("late-peaking swell = %s, want sustained", c)
	}
	short := matchTestTone(220, []float64{1}, 0.3)
	if c := matchClassOf(short); c != matchClassPercussive {
		t.Errorf("0.3 s clip = %s, want percussive", c)
	}
}

// Pitch is the player's choice, never what the ranking judges: a melodic
// voice is rendered AT the reference's note, a drum is resampled there only
// within an octave of its natural pitch, and anything unpitched is searched.
func TestMatchPitchPlan(t *testing.T) {
	cello := matchPitchPlan(roster.Cello, 73.4)
	if cello.skip || cello.search || len(cello.pitches) != 1 || cello.pitches[0] != matchRenderPitch(roster.Cello, 73.4) {
		t.Errorf("melodic voice vs a pitched reference = %+v, want rendered at the note only", cello)
	}
	// A drum read an octave low (the residue under its 1 : 1.5 modes) is
	// also tried an octave up -- within an octave of its natural pitch.
	tom := matchPitchPlan(roster.Tom, 0.5*matchTestFrame(t, roster.Tom))
	if tom.skip || tom.search || len(tom.pitches) != 2 || math.Abs(tom.pitches[0]+12) > 1e-6 || math.Abs(tom.pitches[1]) > 1e-6 {
		t.Errorf("tom vs its own pitch an octave low = %+v, want [-12, 0]", tom)
	}
	if p := matchPitchPlan(roster.Kick, 880); !p.skip {
		t.Errorf("a kick resampled to A5 is not a kick any more, want skipped, got %+v", p)
	}
	if p := matchPitchPlan(roster.Hihat, 440); !p.search {
		t.Errorf("an unpitched voice vs a pitched reference = %+v, want pitch-searched", p)
	}
	if p := matchPitchPlan(roster.Cello, 0); !p.search {
		t.Errorf("anything vs an unpitched reference = %+v, want pitch-searched", p)
	}
}

// matchTestFrame is the Hz a voice's node pitch 0 sounds at to the matcher.
func matchTestFrame(t *testing.T, inst instrument.ID) float64 {
	t.Helper()
	if hz, ok := matchVoiceFrame(inst); ok {
		return hz
	}
	off, _ := audio.InstrumentPitchInfo(inst)
	return a3Hz * math.Pow(2, off/12)
}

func TestEveryRosterInstrumentHasABroadClass(t *testing.T) {
	seen := map[string]string{}
	for class, ids := range matchBroadClasses {
		for _, id := range ids {
			if prev, dup := seen[string(id)]; dup {
				t.Errorf("%s is in both %s and %s", id, prev, class)
			}
			seen[string(id)] = class
		}
	}
	roster := map[string]bool{}
	for _, id := range rosterIDs() {
		roster[string(id)] = true
		if broadClass(id) == "" {
			t.Errorf("roster instrument %s has no broad class in matchBroadClasses", id)
		}
	}
	for id := range seen {
		if !roster[id] {
			t.Errorf("matchBroadClasses names %s, which is not a roster instrument", id)
		}
	}
}

func TestTrimMatchClipDropsLeadingSilenceAndFadesTheCut(t *testing.T) {
	const sr = 44100
	s := make([]float64, sr*3)
	for i := sr / 2; i < len(s); i++ {
		s[i] = 0.5
	}
	out := trimMatchClip(s, sr, 1)
	if len(out) != sr {
		t.Fatalf("len = %d, want %d (capped at max-sec)", len(out), sr)
	}
	rate := sr
	pre := int(matchPreRollSec * float64(rate))
	if out[pre] != 0.5 || out[pre-1] != 0 {
		t.Errorf("onset not at the pre-roll: out[%d]=%v out[%d]=%v", pre-1, out[pre-1], pre, out[pre])
	}
	if out[len(out)-1] != 0 {
		t.Errorf("cut tail not faded: last sample %v", out[len(out)-1])
	}
}

func TestMatchSlugAndNoteName(t *testing.T) {
	if got := matchSlug("68448__pinkyfinger__piano-g (1).WAV"); got != "68448__pinkyfinger__piano-g--1" {
		t.Errorf("slug = %q", got)
	}
	for hz, want := range map[float64]string{440: "A4", 220: "A3", 73.4: "D2", 49: "G1"} {
		if got := noteNameForHz(hz); got != want {
			t.Errorf("noteNameForHz(%v) = %q, want %q", hz, got, want)
		}
	}
}

func TestRecordMatchVerdictSupersedesTheSameRecording(t *testing.T) {
	p := filepath.Join(t.TempDir(), "v.jsonl")
	for _, v := range []matchVerdict{
		{Wav: "a.wav", SHA256: "aa", Match: roster.Oboe, Verdict: "n"},
		{Wav: "b.wav", SHA256: "bb", Match: roster.Kick, Verdict: "y"},
		{Wav: "a.wav", SHA256: "aa", Match: roster.Oboe, Verdict: "p", Comment: "reedier"},
	} {
		if err := recordMatchVerdict(p, v); err != nil {
			t.Fatal(err)
		}
	}
	all, err := loadMatchVerdicts(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[1].SHA256 != "aa" || all[1].Verdict != "p" || all[1].Comment != "reedier" {
		t.Fatalf("verdicts = %+v, want b then the re-judged a", all)
	}
}

// The listen loop end to end: approve one recording with a note, reject the
// next and name a runner-up as closer, then a resumed run asks nothing.
func TestRunMatchListenRecordsVerdictsAndRunnerUps(t *testing.T) {
	dir := t.TempDir()
	vp := filepath.Join(dir, "v.jsonl")
	entry := func(name, sha string, insts ...instrument.ID) matchEntry {
		e := matchEntry{Name: name, SHA256: sha, Reference: name + ".ref", DurationSec: 1}
		for i, in := range insts {
			e.Ranking = append(e.Ranking, matchCandidate{Instrument: in, Score: float64(i), Clip: name + "." + string(in)})
		}
		return e
	}
	man := matchManifest{Version: matchManifestVersion, Entries: []matchEntry{
		entry("kick.wav", "k1", roster.Kick, roster.KickDeep, roster.Tom),
		entry("cello.wav", "c1", roster.Viola, roster.Cello, roster.DoubleBass),
	}}
	var played []string
	newPlayer := func(float64) player {
		return funcPlayer(func(p string) error { played = append(played, p); return nil })
	}
	in := strings.NewReader("m\ny, nice body\nn, too bright\n2: closer but dull\n")
	var out strings.Builder
	if err := runMatchListen(&out, in, man, vp, true, "", 3, newPlayer); err != nil {
		t.Fatal(err)
	}
	wantPlays := []string{
		"kick.wav.ref", "kick.wav.kick", "kick.wav.kick", // auto-play, then "m"
		"cello.wav.ref", "cello.wav.viola", "cello.wav.cello", "cello.wav.double-bass", // reject -> runners-up
	}
	if strings.Join(played, " ") != strings.Join(wantPlays, " ") {
		t.Errorf("played %v\nwant   %v", played, wantPlays)
	}
	all, err := loadMatchVerdicts(vp)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("got %d verdicts, want 2:\n%s", len(all), out.String())
	}
	if v := all[0]; v.Wav != "kick.wav" || v.Match != roster.Kick || v.Verdict != "y" || v.Comment != "nice body" || v.Better != "" {
		t.Errorf("kick verdict = %+v", v)
	}
	if v := all[1]; v.Match != roster.Viola || v.Verdict != "n" || instrument.ID(v.Better) != roster.Cello || v.Comment != "too bright; closer but dull" {
		t.Errorf("cello verdict = %+v", v)
	}

	played = nil
	out.Reset()
	if err := runMatchListen(&out, strings.NewReader(""), man, vp, true, "", 3, newPlayer); err != nil {
		t.Fatal(err)
	}
	if len(played) != 0 || !strings.Contains(out.String(), "2 recording(s) already have a verdict") {
		t.Errorf("resumed run played %v / said %q", played, out.String())
	}

	// A filename filter that matches nothing says so -- it must not pose as
	// "already answered" (the Makefile's ab-notes default ONLY=n,p once
	// leaked into ab-match and emptied the queue with exactly that message).
	out.Reset()
	if err := runMatchListen(&out, strings.NewReader(""), man, vp, false, "n,p", 3, newPlayer); err != nil {
		t.Fatal(err)
	}
	if len(played) != 0 || !strings.Contains(out.String(), `no recording's filename contains "n,p"`) {
		t.Errorf("filtered run played %v / said %q", played, out.String())
	}
}

// A note typed on a skip is not saved, and the listener is told so.
func TestRunMatchListenSkipKeepsNothing(t *testing.T) {
	vp := filepath.Join(t.TempDir(), "v.jsonl")
	man := matchManifest{Entries: []matchEntry{{Name: "a.wav", SHA256: "a", Reference: "r",
		Ranking: []matchCandidate{{Instrument: roster.Oboe, Clip: "m"}}}}}
	var out strings.Builder
	err := runMatchListen(&out, strings.NewReader("s, later\n"), man, vp, true, "", 3,
		func(float64) player { return funcPlayer(func(string) error { return nil }) })
	if err != nil {
		t.Fatal(err)
	}
	if all, _ := loadMatchVerdicts(vp); len(all) != 0 {
		t.Errorf("skip recorded %+v", all)
	}
	if !strings.Contains(out.String(), "NOT saved") {
		t.Errorf("discarded note not reported: %q", out.String())
	}
}

func TestNamesakeInFilename(t *testing.T) {
	ids := []instrument.ID{roster.Oboe, roster.Sax, "sax-baritone", roster.Kick, roster.Cello, "piano", roster.PianoGrand, roster.Harp}
	for name, want := range map[string]string{
		"22686__acclivity__oboe_a_440.wav":                 "oboe",
		"360251__mtg__sax-baritone-c3.wav":                 "sax-baritone", // the longest id wins
		"36010__sandyrb__dnb-kick-003 (1).wav":             "kick",
		"68448__pinkyfinger__piano-g.wav":                  "piano",
		"127160__daphne_in_wonderland__celtic_harp_g2.wav": "harp",
		"682231__henkonen__fhorn-18.wav":                   "", // no word is an id
		"thrl-hat_A_minor.wav":                             "",
	} {
		if got := namesakeInFilename(name, ids); got != instrument.ID(want) {
			t.Errorf("namesakeInFilename(%q) = %q, want %q", name, got, want)
		}
	}
}

// What the recording is labelled as is offered after a rejection even when
// the analysis ranked it far down, marked as such.
func TestRunMatchListenOffersTheLabel(t *testing.T) {
	vp := filepath.Join(t.TempDir(), "v.jsonl")
	e := matchEntry{Name: "oboe_a_440.wav", SHA256: "o", Reference: "ref", Namesake: roster.Oboe, Label: []instrument.ID{roster.Oboe}, LabelBest: roster.Oboe}
	for i, in := range []instrument.ID{roster.Trumpet, roster.Viola, roster.Flute, roster.Sax, roster.Oboe} {
		c := matchCandidate{Instrument: in, Score: float64(i)}
		if i < 3 || in == roster.Oboe {
			c.Clip = string("clip." + in)
		}
		e.Ranking = append(e.Ranking, c)
	}
	var played []string
	var out strings.Builder
	err := runMatchListen(&out, strings.NewReader("n, too brassy\n4\n"), matchManifest{Entries: []matchEntry{e}}, vp, true, "", 3,
		func(float64) player {
			return funcPlayer(func(p string) error { played = append(played, p); return nil })
		})
	if err != nil {
		t.Fatal(err)
	}
	if want := "ref clip.trumpet clip.viola clip.flute clip.oboe"; strings.Join(played, " ") != want {
		t.Errorf("played %v, want %s", played, want)
	}
	if !strings.Contains(out.String(), "labelled as") || strings.Contains(out.String(), " sax ") {
		t.Errorf("listing should mark the label and skip unranked clips:\n%s", out.String())
	}
	all, _ := loadMatchVerdicts(vp)
	if len(all) != 1 || instrument.ID(all[0].Better) != roster.Oboe || all[0].Comment != "too brassy" {
		t.Errorf("verdicts = %+v", all)
	}
}

// A voice's hub is how close it TYPICALLY comes to real recordings: the mean
// of its k closest distances; leaving a reference out excludes it.
func TestComputeHubsIsTheMeanOfTheClosestRecordings(t *testing.T) {
	rows := []map[instrument.ID]float64{
		{"hub": 0.2, "other": 0.9},
		{"hub": 0.3, "other": 0.5},
		{"hub": 0.4, "other": math.Inf(1)},
	}
	got := computeHubs(rows, 2, -1)
	if math.Abs(got["hub"]-0.25) > 1e-12 || math.Abs(got["other"]-0.7) > 1e-12 {
		t.Errorf("hubs = %v, want hub 0.25 (0.2,0.3), other 0.7 (0.5,0.9; +Inf ignored)", got)
	}
	if loo := computeHubs(rows, 2, 0); math.Abs(loo["hub"]-0.35) > 1e-12 {
		t.Errorf("leave-one-out hub = %v, want 0.35 (0.3,0.4)", loo["hub"])
	}
}

// The correction is what stops a voice that sits near EVERY recording from
// topping this one: a hub that is merely as close as it always is loses to a
// voice that is unusually close to THIS recording.
func TestApplyHubsPenalisesAHub(t *testing.T) {
	rs := []matchRender{
		{cand: matchCandidate{Instrument: roster.Harmonium}, rawEssence: 0.80},
		{cand: matchCandidate{Instrument: roster.Flute}, rawEssence: 0.85},
		{cand: matchCandidate{Instrument: "silent"}, rawEssence: math.Inf(1)},
	}
	applyHubs(rs, map[instrument.ID]float64{roster.Harmonium: 0.70, roster.Flute: 1.10})
	if !(rs[1].cand.Score < rs[0].cand.Score) {
		t.Errorf("corrected: flute %.2f, harmonium %.2f -- the hub must lose", rs[1].cand.Score, rs[0].cand.Score)
	}
	if !math.IsInf(rs[2].cand.Score, 1) {
		t.Error("a silent render must stay last")
	}
}

// A voice retuned since the calibration (or new) is ranked with the median
// hub -- neutral -- never with a hub measured on a sound it no longer makes.
func TestHubValuesFallBackToTheMedianWhenStale(t *testing.T) {
	h := matchHubness{Version: matchHubnessVersion, K: matchHubnessK, Instruments: map[instrument.ID]matchHubEntry{
		roster.Kick:  {SoundHash: audio.InstrumentSoundHash(roster.Kick, nil), Hub: 1},
		roster.Snare: {SoundHash: audio.InstrumentSoundHash(roster.Snare, nil), Hub: 3},
		roster.Tom:   {SoundHash: "a-sound-it-no-longer-makes", Hub: 100},
	}}
	hubs, stale := h.hubValues([]instrument.ID{roster.Kick, roster.Snare, roster.Tom, roster.Hihat})
	if hubs[roster.Kick] != 1 || hubs[roster.Snare] != 3 || hubs[roster.Tom] != 2 || hubs[roster.Hihat] != 2 {
		t.Errorf("hubs = %v, want kick 1, snare 3, and the median 2 for the retuned tom and the uncalibrated hihat", hubs)
	}
	if len(stale) != 2 {
		t.Errorf("stale = %v, want tom and hihat", stale)
	}
}

func TestCommittedHubnessParses(t *testing.T) {
	h, err := committedHubness()
	if err != nil {
		t.Fatal(err)
	}
	if h.Version != matchHubnessVersion || h.K != matchHubnessK {
		t.Errorf("%s is version %d / k %d, the matcher is %d / %d -- run `make wav-match-calibrate`", matchHubnessRel, h.Version, h.K, matchHubnessVersion, matchHubnessK)
	}
}

// partialsTone sums sines at the given frequencies (Hz -> amplitude).
func partialsTone(partials map[float64]float64, durSec float64) wave.Wave {
	const sr = 44100
	s := make([]float64, int(durSec*sr))
	for hz, a := range partials {
		for i := range s {
			s[i] += 0.2 * a * math.Sin(2*math.Pi*hz*float64(i)/sr)
		}
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

// A dense inharmonic cluster -- the synth kalimba's own partials -- has no
// single note: no pitch YIN reads accounts for its partials, so the matcher
// must treat it as unpitched (search the pitch per voice) instead of playing
// every voice on whichever divisor YIN locked onto under the nuisance of
// the day.
func TestMatchRefPitchDeclinesAnInharmonicCluster(t *testing.T) {
	kalimba := map[float64]float64{220: 0.009, 756.8: 0.97, 886.6: 1, 996.6: 0.6, 1122: 0.35, 1342: 0.2}
	w := partialsTone(kalimba, 1.5)
	mag, binHz := matchSpectrum(w)
	for _, f0 := range []float64{124.2, 252, 378, 443, 944} {
		if e := pitchExplains(f0, mag, binHz); e >= matchPitchExplainedMin {
			t.Errorf("%.0f Hz claims %.0f %% of an inharmonic cluster", f0, 100*e)
		}
	}
	if got := matchRefPitch(w); got != 0 {
		t.Errorf("matchRefPitch = %.1f Hz, want 0 (no single note)", got)
	}
	// A harmonic tone is explained in full.
	saw := partialsTone(map[float64]float64{220: 1, 440: 0.5, 660: 0.33, 880: 0.25}, 1.5)
	mag, binHz = matchSpectrum(saw)
	if e := pitchExplains(220, mag, binHz); e < 0.99 {
		t.Errorf("a saw at its own pitch is explained %.0f %%", 100*e)
	}
}

// A double reed's formant lifts a high partial over the low ones: the
// acclivity oboe's A 440 peaks at its 6th harmonic (2.66 kHz), with the 2nd
// and 3rd 2-3 dB under it. It is still one clear note -- requiring the
// LOUDEST partial to be harmonic 1..5 matched it as unpitched (bassoon
// first, the oboe #11).
func TestMatchRefPitchHearsAFormantPeakedReed(t *testing.T) {
	db := func(d float64) float64 { return math.Pow(10, d/20) }
	oboe := partialsTone(map[float64]float64{
		443: db(-10), 886: db(-2), 1329: db(-3), 2215: db(-5), 2658: 1, 3101: db(-2), 3544: db(-7), 4430: db(-8),
	}, 1.5)
	if got := matchRefPitch(oboe); math.Abs(got-443) > 3 {
		t.Fatalf("matchRefPitch = %.1f Hz, want ~443", got)
	}
	// The real mbira's cluster has no loud partial among the first five
	// harmonics of the 124 Hz YIN reads (its loud partial, 862 Hz, would be
	// the 7th): no note.
	mbira := partialsTone(map[float64]float64{
		862: 1, 499: db(-8), 562: db(-12), 426: db(-16), 795: db(-17), 633: db(-18), 762: db(-19),
	}, 1.5)
	mag, binHz := matchSpectrum(mbira)
	if e := pitchExplains(124, mag, binHz); e != 0 {
		t.Errorf("124 Hz explains %.0f %% of the mbira cluster, want 0 (no loud low harmonic)", 100*e)
	}
}

// matchTestBurst is a decaying noise burst: an unpitched reference.
func matchTestBurst(durSec float64) wave.Wave {
	const sr = 44100
	r := rand.New(rand.NewSource(17))
	s := make([]float64, int(durSec*sr))
	for i := range s {
		s[i] = math.Exp(-8*float64(i)/sr) * (2*r.Float64() - 1)
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

func matchTestTarget(ref wave.Wave, f0 float64) matchTarget {
	return matchTarget{f0: f0, refE: fingerprint.AnalyzeEssence(ref, fingerprint.EssenceOpts{}), sr: wavref.Rate, clipSec: ref.Duration()}
}

// Scouting first changes nothing about where an instrument settles: the
// scouted pitches are grid points of the full search, so a settled search
// renders exactly the pitches a full search renders and keeps the same one.
func TestScoutedSearchSettlesWhereAFullSearchDoes(t *testing.T) {
	cases := []struct {
		ref   wave.Wave
		f0    float64
		insts []instrument.ID
	}{
		{matchTestBurst(0.6), 0, []instrument.ID{roster.Hihat, roster.Snare, roster.Cello, roster.Kalimba}},            // pitch searches
		{matchTestTone(110, []float64{1, 0.5, 0.3}, 0.8), 110, []instrument.ID{roster.Cello, roster.Tom, roster.Kick}}, // fixed plans, octave alternatives
	}
	octaves := false // some fixed plan carried octave alternatives
	for _, c := range cases {
		tg := matchTestTarget(c.ref, c.f0)
		for _, inst := range c.insts {
			scouted, full := newMatchSearch(inst, c.f0), newMatchSearch(inst, c.f0)
			if err := scouted.scout(tg); err != nil {
				t.Fatal(err)
			}
			// A reading an octave off is the reference's, so the scout
			// already tries every pitch a fixed plan names.
			if p := scouted.plan; !p.skip && !p.search {
				octaves = octaves || len(p.pitches) > 1
				if len(scouted.tried) != len(p.pitches) {
					t.Errorf("%s vs f0 %.0f: the scout rendered %d of the plan's %d pitches", inst, c.f0, len(scouted.tried), len(p.pitches))
				}
			}
			if err := scouted.settle(tg); err != nil {
				t.Fatal(err)
			}
			// The unpruned search: a fixed plan's pitches in order, or (on
			// a fresh search) the grid and the refinement around its best.
			if full.plan.search {
				if err := full.settle(tg); err != nil {
					t.Fatal(err)
				}
			} else {
				for _, p := range full.plan.pitches {
					if err := full.consider(tg, p); err != nil {
						t.Fatal(err)
					}
				}
			}
			if scouted.have != full.have || scouted.best.cand.RenderPitch != full.best.cand.RenderPitch ||
				math.Float64bits(scouted.obj) != math.Float64bits(full.obj) {
				t.Errorf("%s vs f0 %.0f: scouted settles at %+.0f (%v), a full search at %+.0f (%v)", inst, c.f0,
					scouted.best.cand.RenderPitch, scouted.obj, full.best.cand.RenderPitch, full.obj)
			}
			if len(scouted.tried) != len(full.tried) {
				t.Errorf("%s vs f0 %.0f: scouting then settling rendered %d pitches, a full search %d", inst, c.f0, len(scouted.tried), len(full.tried))
			}
		}
	}
	if !octaves {
		t.Fatal("no case had a fixed plan with octave alternatives: the scout's octave check is vacuous")
	}
}

// Pruning only spares renders that cannot win: the top 3 is the exhaustive
// ranking's, no instrument scores closer than its exhaustive score, and an
// instrument the caller names (the map's label, -explain's list) settles
// exactly even when it is out of the running.
func TestRankRosterSettlesTheContenders(t *testing.T) {
	ids := rosterIDs()[:matchContenders+12]
	// A tone ranked as if unpitched, so every voice pitch-searches: its note
	// sits 3 semitones up, between the scouting grid's points.
	ref := matchTestTone(220*math.Pow(2, 3.0/12), []float64{1, 0.5, 0.33, 0.25}, 0.6)
	pruned, err := rankRoster(ref, 0, ids, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	exact, err := rankRoster(ref, 0, ids, 4, matchSettleAll)
	if err != nil {
		t.Fatal(err)
	}
	exactOf := map[string]float64{}
	for _, r := range exact {
		exactOf[string(r.cand.Instrument)] = r.cand.Score
	}
	for i := 0; i < 3; i++ {
		if pruned[i].cand.Instrument != exact[i].cand.Instrument || pruned[i].cand.Score != exact[i].cand.Score {
			t.Errorf("#%d: pruned %s %.4f, exhaustive %s %.4f", i+1, pruned[i].cand.Instrument, pruned[i].cand.Score,
				exact[i].cand.Instrument, exact[i].cand.Score)
		}
	}
	for _, r := range pruned {
		if r.cand.Score < exactOf[string(r.cand.Instrument)] {
			t.Errorf("%s scouted closer (%.4f) than its full search (%.4f)", r.cand.Instrument, r.cand.Score, exactOf[string(r.cand.Instrument)])
		}
	}
	// The instrument scouting misjudged most (only an unsettled one can be).
	var miss string
	var gap float64
	for _, r := range pruned {
		if g := r.cand.Score - exactOf[string(r.cand.Instrument)]; g > gap {
			miss, gap = string(r.cand.Instrument), g
		}
	}
	if miss == "" {
		t.Fatal("every instrument scouted at its full-search distance: this reference cannot show settling by name")
	}
	named, err := rankRoster(ref, 0, ids, 4, func(id instrument.ID) bool { return id == instrument.ID(miss) })
	if err != nil {
		t.Fatal(err)
	}
	if i := slices.IndexFunc(named, func(r matchRender) bool { return r.cand.Instrument == instrument.ID(miss) }); named[i].cand.Score != exactOf[miss] {
		t.Errorf("named %s settled at %.4f, its full search %.4f", miss, named[i].cand.Score, exactOf[miss])
	}
}

// The scouting grid must stay inside the search grid, or a contender would
// settle somewhere a full search never looks.
func TestScoutGridIsPartOfTheSearchGrid(t *testing.T) {
	if math.Mod(matchScoutStep, matchSearchStep) != 0 || math.Mod(2*matchSearchSemis, matchScoutStep) != 0 {
		t.Fatalf("scout step %v must be a multiple of the search step %v and divide the %v-semitone range",
			matchScoutStep, matchSearchStep, 2*matchSearchSemis)
	}
}

// A voice's measured frame survives to the next run of the same binary,
// exactly; a file another build wrote is ignored (its renderer or pitch
// pipeline may differ).
func TestMatchFramesPersistPerBinary(t *testing.T) {
	reset := func() {
		matchFrames.Clear()
		matchFramesMeasured.Store(false)
	}
	reset()
	t.Cleanup(reset)
	path := filepath.Join(t.TempDir(), "frames.json")
	save := persistMatchFrames(path)
	matchFrames.Store("cello=abc", 65.40639132514966)
	matchFramesMeasured.Store(true)
	save()
	reset()
	persistMatchFrames(path)
	if v, ok := matchFrames.Load("cello=abc"); !ok || v.(float64) != 65.40639132514966 {
		t.Fatalf("frame after reload = %v (%v), want 65.40639132514966 exactly", v, ok)
	}
	b, err := json.Marshal(matchFramesFile{Build: "another build", Frames: map[string]float64{"cello=abc": 99}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	reset()
	persistMatchFrames(path)
	if v, ok := matchFrames.Load("cello=abc"); ok {
		t.Fatalf("loaded %v from another build's frames", v)
	}
}
