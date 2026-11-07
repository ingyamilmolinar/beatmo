//go:build !js

package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The diagnostic must REDISCOVER, untold, what the listener said about the annotated instruments (ear_verdicts.jsonl,
// read-only). Against a FROZEN baseline of real measurements taken before tuning, not live audio: a fixed instrument
// would fail a live gate for the right reason. The expected column comes from the listener's words.

// verdictCase is one row of the mapping doc's "Verdict -> axis table".
type verdictCase struct {
	instrument instrument.ID
	axis       string
	wantDir    string // "high" or "low" -- the sign the listener's own words imply for the synth
	verdict    string // one line of context: what the listener actually said
}

// The headline gate ("N/10 listener complaints independently reproduced").
var primaryVerdicts = []verdictCase{
	{roster.Oboe, "noise fraction", "high",
		`"extremely breathy, remove most of it, overpowering"`},
	// Not "noise too high": the synth has MORE noise but the WRONG KIND (flatness 0.599 → 0.204). Band-limited noise reads
	// as hiss, broadband as snare wires: "breathy, fuzzy, hissy".
	{roster.Snare, "spectral flatness", "low",
		`"ghost sound, breathy, fuzzy, hissy"`},
	{roster.Clap, "noise fraction", "high",
		`"too noisy throughout, loud hissing, very breathy"`},
	{roster.Cello, "noise fraction", "low",
		`"more like a flute or oboe than a cello string"`},
	{roster.Harp, "ring T60", "low",
		`"needs much longer decay, let it ring"`},
	{roster.Hihat, "ring T60", "low",
		`"bright and short; real is vibrational, resonant, shh"`},
	{roster.TubularBells, "ring T60", "low",
		`"simple clean tone vs resonant wavy metallic ring that prolongs"`},
	{roster.Kalimba, "inharmonicity", "low",
		`"sounds like a plucked string; needs the wooden marimba feel"`},
	// "RATTLY" is a noise word: noise fraction 0.770 → 0.155 and flatness 0.317 → 0.021, the rattle component missing
	// (centroid mismatches as a consequence).
	{roster.Rimshot, "noise fraction", "low",
		`"sounds like a marimba/wooden snare; real is full, deep, lower, rattly, cracky"`},
	{roster.Bassoon, "centroid", "low",
		`"needs more nasal, a bit too dark and mellow"`},
}

// Extra axes on instruments already covered; they do not count toward the headline.
var bonusVerdicts = []verdictCase{
	{roster.TubularBells, "inharmonicity", "low",
		"also: inharmonicity too low"},
	{roster.Clap, "ring T60", "high",
		"also: noise decay too sustained (should be a burst)"},
	// Ring T60 2.4x too long and release far too slow: "a ghost sound" lingers.
	{roster.Snare, "ring T60", "high",
		"also: ring/release far too sustained (\"sounds like a ghost sound\")"},
}

// Captured before any tuning round touched these instruments.
const baselineFile = "testdata/diagnose_baseline_2026-08-22.json"

// loadBaseline reads and decodes baselineFile into instrument -> diagnosis.
func loadBaseline(t *testing.T) map[string]instrumentDiagnosis {
	t.Helper()
	raw, err := os.ReadFile(baselineFile)
	if err != nil {
		t.Fatalf("read %s: %v (this file must be committed -- it is the frozen acceptance-gate fixture, not regenerated per-run)", baselineFile, err)
	}
	var m map[string]instrumentDiagnosis
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode %s: %v", baselineFile, err)
	}
	return m
}

// runVerdictCase resolves the instrument's FROZEN diagnosis and asserts the
// named axis is flagged (verdict != ok) with the expected direction.
func runVerdictCase(t *testing.T, baseline map[string]instrumentDiagnosis, c verdictCase) (passed bool) {
	t.Helper()
	d, ok := baseline[string(c.instrument)]
	if !ok {
		t.Fatalf("%s: no frozen baseline entry (baseline file is missing this instrument)", c.instrument)
		return false
	}
	a, ok := d.Axis(c.axis)
	if !ok {
		t.Errorf("%s: axis %q was not computed at all (listener said: %s)", c.instrument, c.axis, c.verdict)
		return false
	}
	if a.Verdict == verdictOK {
		t.Errorf("%s: axis %q NOT flagged (ref=%.4f synth=%.4f, delta=%.4f) -- listener said: %s",
			c.instrument, c.axis, a.Ref, a.Synth, a.Synth-a.Ref, c.verdict)
		return false
	}
	if a.Direction != c.wantDir {
		t.Errorf("%s: axis %q flagged direction=%s, want %s (ref=%.4f synth=%.4f) -- listener said: %s",
			c.instrument, c.axis, a.Direction, c.wantDir, a.Ref, a.Synth, c.verdict)
		return false
	}
	t.Logf("%s: axis %q correctly flagged %s (%s) (ref=%.4f synth=%.4f verdict=%s)",
		c.instrument, c.axis, a.Direction, a.Note, a.Ref, a.Synth, a.Verdict)
	return true
}

// For each annotated instrument the FROZEN pre-tuning diagnosis must flag the axis the listener named, with the right sign.
func TestDiagnose_ReproducesListenerVerdicts(t *testing.T) {
	baseline := loadBaseline(t)

	passed := 0
	for _, c := range primaryVerdicts {
		c := c
		t.Run(string(c.instrument), func(t *testing.T) {
			if runVerdictCase(t, baseline, c) {
				passed++
			}
		})
	}
	t.Logf("ACCEPTANCE GATE: %d/%d primary listener complaints independently reproduced (against frozen pre-tuning baseline %s)",
		passed, len(primaryVerdicts), baselineFile)
}

// Bonus lines the battery does NOT reproduce, named with the reason: aspirations belong in a ratchet, not a red build.
var openBonusVerdicts = map[string]string{
	"tubular-bells/sustain modulation": "GENUINELY NOT REPRODUCED. The listener heard 'waves in time'; " +
		"EnvelopeSegments.SustainVariation measures std/mean over the sustain region and reads " +
		"0.0578 ref vs 0.0360 synth -- a real difference, but well inside KindFraction's 0.05 drift " +
		"threshold. Modulation MAGNITUDE cannot separate a slow swell from no swell at all; the " +
		"modulation-SPECTRUM descriptors (Repr=ReprModulation, still unregistered) are what would.",

	"hihat/noise fraction": "SUPERSEDED, not failing. The frozen baseline records ref=0.9187 " +
		"synth=0.9995 -- flagged 'high', i.e. the synth has MORE noise -- while the listener said " +
		"'too tonal'. That axis was measuring nothing: NoiseRatio notches +/-1 bin around multiples " +
		"of f0, and DetectF0 returns 56.5 Hz for a cymbal. noise.fraction is now gated on " +
		"fingerprint.F0ReliableConfidence and reports an honest 'no reliable fundamental' for this " +
		"signal instead of a confidently wrong direction. The frozen baseline predates that gate " +
		"and so cannot express the new outcome; spectral flatness carries noise character here.",
}

// A RATCHET: every bonusVerdicts case must reproduce, openBonusVerdicts names the ones that do not; moving a case is a
// reviewed edit.
func TestDiagnose_ReproducesBonusVerdicts(t *testing.T) {
	baseline := loadBaseline(t)

	passed := 0
	for _, c := range bonusVerdicts {
		c := c
		t.Run(string(c.instrument)+"_"+c.axis, func(t *testing.T) {
			if runVerdictCase(t, baseline, c) {
				passed++
			}
		})
	}
	t.Logf("BONUS GATE: %d/%d reproduced, %d open", passed, len(bonusVerdicts), len(openBonusVerdicts))
	for k, why := range openBonusVerdicts {
		t.Logf("  OPEN %s: %s", k, why)
	}
}

// A case that starts reproducing fails here, so it is promoted rather than rotting as an excuse.
func TestDiagnose_OpenBonusVerdictsStayOpen(t *testing.T) {
	baseline := loadBaseline(t)
	cases := map[string]verdictCase{
		"tubular-bells/sustain modulation": {roster.TubularBells, "sustain modulation", "low", ""},
		"hihat/noise fraction":             {roster.Hihat, "noise fraction", "low", ""},
	}
	for key, c := range cases {
		if _, listed := openBonusVerdicts[key]; !listed {
			continue
		}
		d, ok := baseline[string(c.instrument)]
		if !ok {
			continue
		}
		a, ok := d.Axis(c.axis)
		if !ok {
			continue
		}
		if a.Verdict != verdictOK && a.Direction == c.wantDir {
			t.Errorf("%s now reproduces (verdict=%s direction=%s) -- move it from openBonusVerdicts "+
				"into bonusVerdicts and delete the excuse", key, a.Verdict, a.Direction)
		}
	}
}
