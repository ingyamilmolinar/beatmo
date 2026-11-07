//go:build !test && !js

package main

// Real build: render the shipped engine, write the session's clips and judge the FILES (the -tags test stub has no
// note length to get wrong). Each recording is the instrument's own voice in a room, staged under a temp $BEATMO_WAV_REFS.

import (
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// A generated recording at the instrument's first catalogue path, and a candidate moved half-way across every searched
// value's trust box (as a refs tune result moves) from the pick's start document, with the search pitch runTune records.
func stageTuned(t *testing.T, root string, pick tunePick) (instrument.Doc, wavref.Ref) {
	t.Helper()
	cat, err := loadCatalogue()
	if err != nil {
		t.Fatal(err)
	}
	var ref wavref.Ref
	for _, r := range cat.ForInstrument(pick.Inst) {
		if r.File != "" && r.Measured != nil {
			ref = r
			break
		}
	}
	if ref.ID == "" {
		t.Fatalf("%s has no catalogued reference to stand in for", pick.Inst)
	}
	rec, err := nuisanceRecording(pick.Inst, matchNuisance{Name: "ab-tuned", Room: true, WetDB: -12})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(wavref.Root(), ref.File)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeCompareWAV(path, rec.Samples, rec.SampleRate); err != nil {
		t.Fatal(err)
	}
	start, defs, err := tuneStart(pick)
	if err != nil {
		t.Fatal(err)
	}
	d0, dims, err := tuneSpace(start, defs, pick)
	if err != nil {
		t.Fatal(err)
	}
	y := make([]float64, len(dims))
	for i := range y {
		y[i] = 0.5
	}
	cand, err := applyTune(d0, dims, y)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, string(pick.Inst))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := instrument.MarshalFile(cand)
	if err != nil {
		t.Fatal(err)
	}
	meta := tuneMeta{Inst: pick.Inst, Ref: ref.ID, SamePitch: true, MaxSec: adoptMaxSecDefault, Why: pick.Why, From: string(pick.From)}
	if pick.From != "" {
		rec, err := newTuneRec(path, ref.ID, ref.ID, t.TempDir(), adoptMaxSecDefault)
		if err != nil {
			t.Fatal(err)
		}
		if err := rec.settlePitchDoc(pick.Inst, &start); err != nil {
			t.Fatal(err)
		}
		sp := rec.pitch
		meta.Records = []tuneRecReport{{Recording: ref.ID, Ref: ref.ID, SearchPitch: &sp}}
	}
	m, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(dir, "candidate.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), m, 0o644); err != nil {
		t.Fatal(err)
	}
	return cand, ref
}

func needFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("SKIPPED: ffmpeg converts every recording `refs tune` and `refs session -tuned` load; it is not installed")
	}
}

// The bound is the WAV writer's own error (truncation to 24 bits, two steps, ~-132 dBFS). A gain within 1e-5 of unity
// is unity: truncation biases the fit, and the recording at its own level must be proved exactly.
func sameUpToLevel(t *testing.T, file wave.Wave, want []float64) float64 {
	t.Helper()
	if len(file.Samples) != len(want) {
		t.Fatalf("%s: %d samples, the scored clip %d", file.Label, len(file.Samples), len(want))
		return 0
	}
	var fw, ww float64
	for i, v := range want {
		fw += file.Samples[i] * v
		ww += v * v
	}
	g := fw / ww
	if math.Abs(g-1) < 1e-5 {
		g = 1
	}
	var worst float64
	for i, v := range want {
		worst = math.Max(worst, math.Abs(file.Samples[i]-g*v))
	}
	if worst > 2.0/(1<<23) {
		t.Errorf("%s is not the scored clip times one gain: off by up to %.2g (%.1f 24-bit steps) at gain %.6f", file.Label, worst, worst*(1<<23), g)
	}
	return g
}

func decodeClip(t *testing.T, path string) wave.Wave {
	t.Helper()
	pcm, rate, err := audio.DecodeWAVAtRate(path, wavref.Rate)
	if err != nil {
		t.Fatal(err)
	}
	s := make([]float64, len(pcm))
	for i, v := range pcm {
		s[i] = float64(v)
	}
	return wave.Wave{Samples: s, SampleRate: rate, Label: filepath.Base(path)}
}

// Every curated pick through refs session -tuned with a recording player: R, C, T in order with gaps; every clip passes
// the listening contract and C, T hold where the recording holds; R at its own level or quieter; T's essence distance to
// R, measured on the FILES, is the search's own objective term.
func TestAbTunedPlaysEveryPickAsItWasTuned(t *testing.T) {
	needFFmpeg(t)
	t.Setenv(wavref.EnvVar, t.TempDir())
	picks, err := loadTunePicks()
	if err != nil {
		t.Fatal(err)
	}
	candPath, err := defaultCandidatesPath()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, pick := range picks {
		t.Run(string(pick.Inst), func(t *testing.T) {
			cand, ref := stageTuned(t, root, pick)
			var log []string
			p := gappedPlayer{p: funcPlayer(func(path string) error { log = append(log, path); return nil }),
				gap: clipGap, sleep: func(time.Duration) { log = append(log, "gap") }}
			var out strings.Builder
			if err := runSession(&out, strings.NewReader("q\n"), p, []instrument.ID{pick.Inst}, candPath, filepath.Join(t.TempDir(), "s.jsonl"), false, false, true, "", root); err != nil {
				t.Fatalf("%v\n%s", err, out.String())
			}
			if strings.Contains(out.String(), "WARNING") {
				t.Errorf("the session warned about its own clips:\n%s", out.String())
			}
			if len(log) != 6 || log[1] != "gap" || log[3] != "gap" || log[5] != "gap" ||
				!strings.Contains(log[0], "TUNED_AB__R__") || !strings.Contains(log[2], "TUNED_AB__C__") || !strings.Contains(log[4], "TUNED_AB__T__") {
				t.Fatalf("played %v", log)
			}
			r, c, tc := decodeClip(t, log[0]), decodeClip(t, log[2]), decodeClip(t, log[4])
			for k, w := range map[string]wave.Wave{"R": r, "C": c, "T": tc} {
				for _, f := range clipFaults(k, w) {
					t.Errorf("%s: %s", w.Label, f)
				}
			}
			for k, w := range map[string]wave.Wave{"C": c, "T": tc} {
				if f := holdFault(k, r, w); f != "" {
					t.Errorf("%s", f)
				}
			}

			// What was played IS what the search scored, equal up to the level match and 24-bit rounding.
			path, _ := wavref.Resolve(ref.ID)
			rec, err := newTuneRec(path, ref.ID, ref.ID, t.TempDir(), adoptMaxSecDefault)
			if err != nil {
				t.Fatal(err)
			}
			if err := rec.settlePitch(pick.Inst); err != nil {
				t.Fatal(err)
			}
			cur, err := rec.t.eval(pick.Inst, rec.pitch)
			if err != nil {
				t.Fatal(err)
			}
			meta, err := loadTuneMeta(filepath.Join(root, string(pick.Inst)))
			if err != nil {
				t.Fatal(err)
			}
			tp := rec.pitch
			if sp := meta.searchPitch(ref.ID); sp != nil {
				tp = *sp
			} else if pick.From != "" {
				t.Fatal("a pick From another instrument must record its search pitch")
			}
			scored, err := rec.t.evalDoc(cand, pick.Inst, tp)
			if err != nil {
				t.Fatal(err)
			}
			refS := listeningCopy(rec.w).Samples
			// Never raised: raising it raised its hiss into the DAC's clip.
			if g := sameUpToLevel(t, r, refS); g > 1+1e-6 {
				t.Errorf("the reference is played %.1f dB LOUDER than the recording (gain %.3f)", 20*math.Log10(g), g)
			}
			sameUpToLevel(t, c, listeningCopy(cur.w).Samples)
			sameUpToLevel(t, tc, listeningCopy(scored.w).Samples)
			refE := fingerprint.AnalyzeEssence(r, fingerprint.EssenceOpts{})
			heard := fingerprint.EssenceTotal(fingerprint.EssenceTerms(refE, fingerprint.AnalyzeEssence(tc, fingerprint.EssenceOpts{WindowSec: refE.WindowSec})))
			t.Logf("essence T↔R: scored %.4f, on the 24-bit files %.4f", fingerprint.EssenceTotal(scored.ess), heard)
		})
	}
}

// The generic compare render (every voice at its shipped length) must FAIL the contract on a held wind, or the
// contract proves nothing.
func TestCompareRenderBreaksTheHoldContract(t *testing.T) {
	t.Setenv(wavref.EnvVar, t.TempDir())
	root := t.TempDir()
	cand, ref := stageTuned(t, root, tunePick{Inst: roster.Oboe, Only: []string{"filter_cutoff"}})
	fc := familyCandidates{Reference: ref.ID, Base: roster.Oboe, Candidates: []candidateSpec{{ID: "t", Doc: &cand, SamePitch: true}}}
	clips, _, err := renderSessionClips(fc, ref)
	if err != nil {
		t.Fatal(err)
	}
	r, c := decodeClip(t, clips.refPath), decodeClip(t, clips.curPath)
	if f := holdFault("C", r, c); f == "" {
		t.Fatalf("the 120 BPM compare render passed the hold contract: C sounds until %.2f s of a %.2f s recording", lastSoundingSec(c), lastSoundingSec(r))
	} else {
		t.Logf("caught: %s", f)
	}
	if got := slices.Index(clips.order, "T"); got != 2 {
		t.Fatalf("order %v", clips.order)
	}
}
