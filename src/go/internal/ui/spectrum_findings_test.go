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

// synthMaster builds a master ChannelMetrics whose FFT sits at a flat `floorDB`
// everywhere, with every bin inside the named ISO bands raised to that band's
// dB. Overriding whole BANDS rather than individual bins matters: real piled-up
// energy fills a band, and a handful of hot bins in an otherwise cold band
// averages back down to nearly the floor — which would make a pile-up test pass
// or fail on bin spacing rather than on the behaviour under test.
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

// A balanced spectrum with no clipping and no shared fundamentals must produce
// NOTHING. This is the most important case: a tab that always finds a problem
// is noise, and "only checkable facts" means silence is a valid answer.
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

// Ranking: a clipping row outranks a spectral observation. Clipping is audible
// damage; a pile-up is only a balance note.
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

// A band far louder than its NEIGHBOURS is self-relative — no target curve, no
// taste. The finding must point at the band that is actually piled up.
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
	// Energy across the bottom bands; every band above 1 kHz sits at the floor,
	// so the ONLY dead end is the top one.
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

// The headline feature: two loud rows whose fundamentals land in one band are
// fighting, and the finding names both.
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

// Unpitched instruments have no fundamental to collide WITH. Naming one would
// be a guess, and the design forbids guesses.
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
	// PeakDB must be stated, not defaulted: the zero value is 0 dBFS, i.e.
	// FULL SCALE, so an unset peak reads as the loudest possible row.
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

// ── Sentence layer ───────────────────────────────────────────────────────
//
// The banner is the whole point of the redesign, so the wording is tested, not
// just the detection. These assert SHAPE (names appear, frequency appears, no
// leftover format verbs) rather than exact prose, so copy can be reworded in
// the catalog without breaking the suite — but a missing catalog entry, an
// unsubstituted %s, or a finding that silently renders blank still fails.

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

// Every kind must render. A new findingKind with no catalog entry is a blank
// banner in production, which is exactly the "shipped unbaked" failure the
// project's UI rules warn about.
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

// An octave error here silently mislabels which band an instrument occupies,
// and this codebase has repeatedly been bitten by exactly that (YIN reading a
// reference an octave low, harpsichord measuring 440 against its true 220). Pin
// the conversion against known anchors.
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

// The bridge must report a row's SOUNDING fundamental, i.e. node pitch 0 folded
// with the instrument's own offset — not a bare A3 for every row.
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

// Unpitched instruments must come back Pitched=false so the engine can refuse
// to name them, rather than arriving with a bogus fundamental.
func TestSpectrumRowFacts_UnpitchedRowsCarryNoFundamental(t *testing.T) {
	g := setupSpectrumGame(t)
	for _, f := range g.drum.spectrumRowFacts(testAnalyzerStateOverride) {
		if !f.Pitched && f.FundHz != 0 {
			t.Errorf("unpitched row %q carries FundHz=%v, want 0", f.Name, f.FundHz)
		}
	}
}

// ── Banner geometry ──────────────────────────────────────────────────────

// The banner must claim its strip from the TOP of the panel and hand the rest
// to the existing renderers untouched. If the two rects overlapped, the
// sentence would be stamped over the bars (the exact collision the
// bracket/Hz-row split was introduced to fix elsewhere in this file).
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

// A panel too short to afford a banner must give the whole rect to the plot
// rather than squeezing the bars into nothing. Mobile panels get very short.
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

// ── Stability ────────────────────────────────────────────────────────────
//
// A percussive row's rolling RMS swings enormously between hits — measured on
// the real analyzer, Dnb Kick reported -25.0 dB on one frame and -38.8 dB three
// frames later. Judging "is this row audible" from RMS alone therefore dropped
// rows below the floor between beats, and the headline sentence flipped between
// findings several times a second. Peak over the same window is far steadier,
// so a row counts as audible on EITHER measure.

func TestSpectrumFindings_RowAudibleByPeakBetweenHits(t *testing.T) {
	m := synthMaster(-30, nil)
	// Both rows are mid-decay: RMS has collapsed below the floor, but each
	// still peaked loudly inside the analyzer window.
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

// The whole point: a different finding must persist before it replaces the
// sentence on screen, so the banner is readable rather than strobing.
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

// A candidate that keeps changing never wins — otherwise a flapping finding
// would still reach the screen via whichever value happened to be last.
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

// Returning to the line already on screen must cancel any pending candidate,
// not sit half-way through a switch.
func TestBannerHold_ReturningToTheShownLineCancelsThePendingSwitch(t *testing.T) {
	var h bannerHold
	t0 := time.Unix(300, 0)
	h.update(spectrumBanner{Text: "collision"}, t0)
	h.update(spectrumBanner{Text: "dead range"}, t0.Add(100*time.Millisecond))
	h.update(spectrumBanner{Text: "collision"}, t0.Add(200*time.Millisecond))
	// The earlier candidate is stale now; a single late sighting must restart
	// its dwell rather than adopt instantly.
	if got := h.update(spectrumBanner{Text: "dead range"}, t0.Add(200*time.Millisecond+spectrumBannerDwell/2)); got.Text != "collision" {
		t.Errorf("stale candidate adopted immediately as %q", got.Text)
	}
}
