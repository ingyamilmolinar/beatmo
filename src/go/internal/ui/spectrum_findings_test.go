//go:build test

package ui

import (
	"image"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// Whole bands are raised, not single bins: a few hot bins in a cold band average back to the floor.
func synthMaster(floorDB float64, bandDB map[int]float64) *analyzer.ChannelMetrics {
	const nBins = 256
	freq := make([]float64, nBins)
	bins := make([]float64, nBins)
	for i := range freq {
		// Log spread across 20 Hz - 20 kHz so every ISO band holds several bins.
		freq[i] = 20 * math.Pow(1000, float64(i)/float64(nBins-1))
		bins[i] = floorDB
	}
	for b, db := range bandDB {
		lo, hi := isoBands[b][0], isoBands[b][1]
		for i, f := range freq {
			if f >= lo && f < hi {
				bins[i] = db
			}
		}
	}
	return &analyzer.ChannelMetrics{
		ID: channel.Master, Name: "Master", Active: true,
		FFTBins: bins, FreqBins: freq,
	}
}

func kinds(fs []finding) []findingKind {
	out := make([]findingKind, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.Kind)
	}
	return out
}

// A balanced mix produces NOTHING: a tab that always finds a problem is noise.
func TestSpectrumFindings_SilentOnABalancedMix(t *testing.T) {
	m := synthMaster(-30, nil)
	rows := []rowFacts{
		{Name: "Kick", RMSDB: -18, FundHz: 60, Pitched: true},
		{Name: "Hat", RMSDB: -20, Pitched: false},
	}
	if got := spectrumFindings(m, rows); len(got) != 0 {
		t.Fatalf("balanced mix produced findings %v, want none", kinds(got))
	}
}

// Clipping is the one fact that needs no spectrum at all, and it names the row.
func TestSpectrumFindings_ClippingNamesTheRow(t *testing.T) {
	m := synthMaster(-30, nil)
	rows := []rowFacts{
		{Name: "Kick", RMSDB: -18, FundHz: 60, Pitched: true},
		{Name: "Snare", RMSDB: -6, ClipCount: 3, FundHz: 200, Pitched: true},
	}
	got := spectrumFindings(m, rows)
	if len(got) == 0 || got[0].Kind != findingClipping {
		t.Fatalf("got %v, want findingClipping first", kinds(got))
	}
	if len(got[0].Names) != 1 || got[0].Names[0] != "Snare" {
		t.Errorf("clipping named %v, want [Snare]", got[0].Names)
	}
}

// Clipping is audible damage; a pile-up is a balance note.
func TestSpectrumFindings_ClippingOutranksPileUp(t *testing.T) {
	m := synthMaster(-60, map[int]float64{3: -5}) // band 3 = 177-355 Hz
	rows := []rowFacts{
		{Name: "Snare", RMSDB: -6, ClipCount: 1, FundHz: 200, Pitched: true},
	}
	got := spectrumFindings(m, rows)
	if len(got) < 2 {
		t.Fatalf("got %v, want both clipping and pile-up", kinds(got))
	}
	if got[0].Kind != findingClipping {
		t.Errorf("first finding is %v, want findingClipping", got[0].Kind)
	}
}

// Relative to its neighbours, with no target curve, and pointing at the band actually piled up.
func TestSpectrumFindings_PileUpPointsAtTheLoudBand(t *testing.T) {
	m := synthMaster(-60, map[int]float64{3: -5}) // band 3 = 177-355 Hz
	got := spectrumFindings(m, nil)
	var pile *finding
	for i := range got {
		if got[i].Kind == findingPileUp {
			pile = &got[i]
			break
		}
	}
	if pile == nil {
		t.Fatalf("got %v, want a findingPileUp", kinds(got))
	}
	// 250 Hz lands in isoBands[3] = {177, 355}.
	if pile.Band != 3 {
		t.Errorf("pile-up band = %d, want 3 (177-355 Hz)", pile.Band)
	}
}

// "Nothing up top" is a fact about the master spectrum alone.
func TestSpectrumFindings_DeadRangeAboveTheContentStops(t *testing.T) {
	// Every band above 1 kHz at the floor, so the only dead end is the top one.
	m := synthMaster(-78, map[int]float64{0: -26, 1: -20, 2: -22, 3: -24, 4: -28, 5: -30})
	got := spectrumFindings(m, nil)
	var dead *finding
	for i := range got {
		if got[i].Kind == findingDeadRange {
			dead = &got[i]
			break
		}
	}
	if dead == nil {
		t.Fatalf("got %v, want a findingDeadRange", kinds(got))
	}
	if !dead.High {
		t.Errorf("dead range reported High=false, want the MISSING TOP end")
	}
}

// Two loud rows whose fundamentals share a band are named together.
func TestSpectrumFindings_CollisionNamesBothRows(t *testing.T) {
	m := synthMaster(-30, nil)
	rows := []rowFacts{
		{Name: "Dnb Kick", RMSDB: -10, FundHz: 60, Pitched: true},
		{Name: "Organ", RMSDB: -12, FundHz: 65, Pitched: true},
		{Name: "Hat", RMSDB: -14, FundHz: 0, Pitched: false},
	}
	got := spectrumFindings(m, rows)
	var col *finding
	for i := range got {
		if got[i].Kind == findingCollision {
			col = &got[i]
			break
		}
	}
	if col == nil {
		t.Fatalf("got %v, want a findingCollision", kinds(got))
	}
	if len(col.Names) != 2 {
		t.Fatalf("collision named %v, want exactly the two colliding rows", col.Names)
	}
	seen := map[string]bool{col.Names[0]: true, col.Names[1]: true}
	if !seen["Dnb Kick"] || !seen["Organ"] {
		t.Errorf("collision named %v, want Dnb Kick and Organ", col.Names)
	}
}

// There is no fundamental to collide with, and naming one would be a guess.
func TestSpectrumFindings_UnpitchedRowsAreNeverNamedInACollision(t *testing.T) {
	m := synthMaster(-30, nil)
	rows := []rowFacts{
		{Name: "Clap", RMSDB: -8, FundHz: 60, Pitched: false},
		{Name: "Shaker", RMSDB: -8, FundHz: 62, Pitched: false},
	}
	for _, f := range spectrumFindings(m, rows) {
		if f.Kind == findingCollision {
			t.Fatalf("unpitched rows produced a collision naming %v", f.Names)
		}
	}
}

// A row that is not currently audible cannot be fighting anything.
func TestSpectrumFindings_QuietRowsDoNotCollide(t *testing.T) {
	m := synthMaster(-30, nil)
	// Stated, not defaulted: the zero value is 0 dBFS, the loudest possible row.
	rows := []rowFacts{
		{Name: "Kick", RMSDB: -10, PeakDB: -4, FundHz: 60, Pitched: true},
		{Name: "Sub", RMSDB: -70, PeakDB: -72, FundHz: 61, Pitched: true}, // silent on both measures
	}
	for _, f := range spectrumFindings(m, rows) {
		if f.Kind == findingCollision {
			t.Fatalf("a silent row was reported as fighting: %v", f.Names)
		}
	}
}

// A nil / inactive master must not panic and must not invent spectral findings.
func TestSpectrumFindings_NilMasterYieldsNoSpectralFindings(t *testing.T) {
	rows := []rowFacts{{Name: "Snare", RMSDB: -6, ClipCount: 2, FundHz: 200, Pitched: true}}
	got := spectrumFindings(nil, rows)
	for _, f := range got {
		if f.Kind == findingPileUp || f.Kind == findingDeadRange {
			t.Errorf("nil master produced spectral finding %v", f.Kind)
		}
	}
	// Clipping still holds — it never needed the spectrum.
	if len(got) == 0 || got[0].Kind != findingClipping {
		t.Fatalf("got %v, want clipping to survive a nil master", kinds(got))
	}
}

// The wording is tested by shape (names, frequency, no leftover verbs), so copy can change while a missing entry
// or blank sentence still fails.

func TestFindingSentence_NamesTheClippingRow(t *testing.T) {
	s := findingSentence(finding{Kind: findingClipping, Band: -1, Names: []string{"Snare"}})
	if s == "" {
		t.Fatal("clipping rendered an empty sentence")
	}
	if !strings.Contains(s, "Snare") {
		t.Errorf("sentence %q does not name the clipping row", s)
	}
}

func TestFindingSentence_CollisionNamesBothRowsAndTheFrequency(t *testing.T) {
	s := findingSentence(finding{Kind: findingCollision, Band: 1, Names: []string{"Dnb Kick", "Organ"}})
	for _, want := range []string{"Dnb Kick", "Organ"} {
		if !strings.Contains(s, want) {
			t.Errorf("sentence %q does not name %q", s, want)
		}
	}
	// Band 1 is 44-88 Hz; the sentence must locate the problem for the user.
	if !strings.Contains(s, "62") {
		t.Errorf("sentence %q does not say WHERE (expected the 62 Hz band label)", s)
	}
}

func TestFindingSentence_DeadRangeDistinguishesTopFromBottom(t *testing.T) {
	high := findingSentence(finding{Kind: findingDeadRange, Band: 7, High: true})
	low := findingSentence(finding{Kind: findingDeadRange, Band: 2, High: false})
	if high == "" || low == "" {
		t.Fatalf("dead-range rendered empty: high=%q low=%q", high, low)
	}
	if high == low {
		t.Errorf("missing-top and missing-bottom produced the same sentence %q", high)
	}
}

// A kind with no catalogue entry is a blank banner in production.
func TestFindingSentence_EveryKindRenders(t *testing.T) {
	for _, f := range []finding{
		{Kind: findingClipping, Band: -1, Names: []string{"Kick"}},
		{Kind: findingCollision, Band: 3, Names: []string{"Kick", "Bass"}},
		{Kind: findingPileUp, Band: 3},
		{Kind: findingDeadRange, Band: 7, High: true},
		{Kind: findingDeadRange, Band: 1, High: false},
	} {
		s := findingSentence(f)
		if s == "" {
			t.Errorf("%v rendered an empty sentence", f.Kind)
		}
		if strings.Contains(s, "%!") || strings.Contains(s, "%s") || strings.Contains(s, "%d") {
			t.Errorf("%v sentence has an unsubstituted verb: %q", f.Kind, s)
		}
	}
}

// ── Live bridge ──────────────────────────────────────────────────────────

// An octave error silently mislabels a band, so the conversion is pinned to known anchors.
func TestMidiToHz_KnownAnchors(t *testing.T) {
	for _, tc := range []struct {
		midi int
		hz   float64
	}{
		{69, 440},  // A4, the tuning reference
		{57, 220},  // A3 — node pitch 0 by this project's convention
		{45, 110},  // A2
		{21, 27.5}, // A0, bottom of the piano
	} {
		got := midiToHz(tc.midi)
		if got < tc.hz*0.999 || got > tc.hz*1.001 {
			t.Errorf("midiToHz(%d) = %.3f, want %.3f", tc.midi, got, tc.hz)
		}
	}
}

// The sounding fundamental, pitch 0 folded with the instrument's offset, not A3 for every row.
func TestSpectrumRowFacts_ReportsPerRowFundamentals(t *testing.T) {
	g := setupSpectrumGame(t)
	facts := g.drum.spectrumRowFacts(testAnalyzerStateOverride)
	if len(facts) == 0 {
		t.Fatal("bridge produced no row facts for a scene with rows")
	}
	named := 0
	for _, f := range facts {
		if f.Name == "" {
			t.Error("a row fact carries no name — the banner would name nobody")
		}
		if f.Pitched {
			named++
			if f.FundHz <= 0 {
				t.Errorf("row %q is pitched but reports FundHz=%v", f.Name, f.FundHz)
			}
		}
	}
	if named == 0 {
		t.Error("no row resolved a fundamental; collisions could never be detected")
	}
}

// So the engine can refuse to name them.
func TestSpectrumRowFacts_UnpitchedRowsCarryNoFundamental(t *testing.T) {
	g := setupSpectrumGame(t)
	for _, f := range g.drum.spectrumRowFacts(testAnalyzerStateOverride) {
		if !f.Pitched && f.FundHz != 0 {
			t.Errorf("unpitched row %q carries FundHz=%v, want 0", f.Name, f.FundHz)
		}
	}
}

// ── Banner geometry ──────────────────────────────────────────────────────

// From the top of the panel, never overlapping the plot below it.
func TestSpectrumBannerSplit_ReservesATopStripWithoutOverlap(t *testing.T) {
	rect := image.Rect(10, 20, 810, 320)
	banner, rest := spectrumBannerSplit(rect)

	if banner.Empty() {
		t.Fatal("banner strip is empty — the sentence would have nowhere to render")
	}
	if banner.Min.Y != rect.Min.Y {
		t.Errorf("banner starts at y=%d, want the panel top y=%d", banner.Min.Y, rect.Min.Y)
	}
	if rest.Min.Y < banner.Max.Y {
		t.Errorf("plot area starts at y=%d, overlapping the banner which ends at y=%d",
			rest.Min.Y, banner.Max.Y)
	}
	if rest.Max.Y != rect.Max.Y {
		t.Errorf("plot area ends at y=%d, want the panel bottom y=%d", rest.Max.Y, rect.Max.Y)
	}
	if banner.Min.X != rect.Min.X || banner.Max.X != rect.Max.X {
		t.Error("banner does not span the full panel width")
	}
}

// Too short for a banner, the plot keeps the whole rect.
func TestSpectrumBannerSplit_YieldsEverythingOnAShortPanel(t *testing.T) {
	rect := image.Rect(0, 0, 400, 24)
	banner, rest := spectrumBannerSplit(rect)
	if !banner.Empty() {
		t.Errorf("short panel reserved a banner %v, leaving %v for the plot", banner, rest)
	}
	if rest != rect {
		t.Errorf("short panel plot rect = %v, want the whole rect %v", rest, rect)
	}
}

// A percussive row's RMS swings ~14 dB between hits while its peak holds, so a row is audible on EITHER measure,
// or the sentence flips several times a second.

func TestSpectrumFindings_RowAudibleByPeakBetweenHits(t *testing.T) {
	m := synthMaster(-30, nil)
	// Mid-decay: RMS below the floor, peak still loud.
	rows := []rowFacts{
		{Name: "Dnb Kick", RMSDB: -52, PeakDB: -8, FundHz: 49, Pitched: true},
		{Name: "Sub Bass", RMSDB: -50, PeakDB: -11, FundHz: 46, Pitched: true},
	}
	found := false
	for _, f := range spectrumFindings(m, rows) {
		if f.Kind == findingCollision {
			found = true
		}
	}
	if !found {
		t.Error("rows that peaked loudly this window were treated as silent — the banner will flicker between beats")
	}
}

func TestSpectrumFindings_GenuinelySilentRowStaysSilentOnBothMeasures(t *testing.T) {
	m := synthMaster(-30, nil)
	rows := []rowFacts{
		{Name: "Kick", RMSDB: -10, PeakDB: -4, FundHz: 49, Pitched: true},
		{Name: "Muted", RMSDB: -90, PeakDB: -88, FundHz: 46, Pitched: true},
	}
	for _, f := range spectrumFindings(m, rows) {
		if f.Kind == findingCollision {
			t.Errorf("a row silent on BOTH measures was reported as fighting: %v", f.Names)
		}
	}
}

// ── Banner dwell ─────────────────────────────────────────────────────────

func TestBannerHold_AdoptsTheFirstLineImmediately(t *testing.T) {
	var h bannerHold
	t0 := time.Unix(0, 0)
	got := h.update(spectrumBanner{Text: "first"}, t0)
	if got.Text != "first" {
		t.Errorf("first line = %q, want it shown immediately", got.Text)
	}
}

// A new finding must persist before it replaces the shown line, so the banner never strobes.
func TestBannerHold_HoldsTheCurrentLineUntilTheNewOnePersists(t *testing.T) {
	var h bannerHold
	t0 := time.Unix(100, 0)
	h.update(spectrumBanner{Text: "collision"}, t0)

	if got := h.update(spectrumBanner{Text: "dead range"}, t0.Add(100*time.Millisecond)); got.Text != "collision" {
		t.Errorf("banner switched after 100ms to %q; want it to hold %q", got.Text, "collision")
	}
	// The candidate first appeared at t0+100ms, so the dwell runs from there.
	if got := h.update(spectrumBanner{Text: "dead range"}, t0.Add(100*time.Millisecond+spectrumBannerDwell+time.Millisecond)); got.Text != "dead range" {
		t.Errorf("banner did not adopt %q after the dwell elapsed, got %q", "dead range", got.Text)
	}
}

// Or a flapping finding reaches the screen as whichever value came last.
func TestBannerHold_FlappingCandidateNeverReplacesTheShownLine(t *testing.T) {
	var h bannerHold
	t0 := time.Unix(200, 0)
	h.update(spectrumBanner{Text: "collision"}, t0)
	for i := 0; i < 40; i++ {
		alt := "pile-up"
		if i%2 == 0 {
			alt = "dead range"
		}
		at := t0.Add(time.Duration(i+1) * 100 * time.Millisecond)
		if got := h.update(spectrumBanner{Text: alt}, at); got.Text != "collision" {
			t.Fatalf("step %d: flapping candidates replaced the line with %q", i, got.Text)
		}
	}
}

func TestBannerHold_ReturningToTheShownLineCancelsThePendingSwitch(t *testing.T) {
	var h bannerHold
	t0 := time.Unix(300, 0)
	h.update(spectrumBanner{Text: "collision"}, t0)
	h.update(spectrumBanner{Text: "dead range"}, t0.Add(100*time.Millisecond))
	h.update(spectrumBanner{Text: "collision"}, t0.Add(200*time.Millisecond))
	// A stale candidate restarts its dwell.
	if got := h.update(spectrumBanner{Text: "dead range"}, t0.Add(200*time.Millisecond+spectrumBannerDwell/2)); got.Text != "collision" {
		t.Errorf("stale candidate adopted immediately as %q", got.Text)
	}
}
