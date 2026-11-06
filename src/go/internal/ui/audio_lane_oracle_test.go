//go:build test

package ui

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

type heardNote struct {
	inst       string
	vol, pitch float64 // the cell's first voice (dumps)
	grid       float64
	voices     [][2]float64 // every voice the cell sounded: {pitch, vol}
}

// addVoice records one more voice of the cell (a chord sounds several).
func (h heardNote) addVoice(inst string, vol, pitch, grid float64) heardNote {
	if len(h.voices) == 0 {
		h.inst, h.vol, h.pitch, h.grid = inst, vol, pitch, grid
	}
	h.voices = append(h.voices, [2]float64{pitch, vol})
	return h
}

// sameVoices compares two cells' voice sets, order-independent.
func sameVoices(a, b [][2]float64) bool {
	if len(a) != len(b) {
		return false
	}
	sa := append([][2]float64(nil), a...)
	sb := append([][2]float64(nil), b...)
	less := func(v [][2]float64) func(i, j int) bool {
		return func(i, j int) bool { return v[i][0] < v[j][0] || (v[i][0] == v[j][0] && v[i][1] < v[j][1]) }
	}
	sort.Slice(sa, less(sa))
	sort.Slice(sb, less(sb))
	for i := range sa {
		if math.Abs(sa[i][0]-sb[i][0]) > 1e-9 || math.Abs(sa[i][1]-sb[i][1]) > 1e-9 {
			return false
		}
	}
	return true
}

// TestLaneHeardEqualsLegacyDispatched is the lane's end-to-end oracle: two
// identical games run the same seeded random edit script on one shared fake
// audio clock — A through the LEGACY grid-time dispatch (captured at
// audio.PlayBatch), B through the audio lane (fake sink). Over 20 s the notes
// A dispatched must equal the notes B's sink actually played (committed and
// not cancelled before their start), cell by cell, instrument / volume /
// pitch exact.
//
// Excluded: notes whose grid time lies just before an edit — [edit − 100 ms,
// edit + 20 ms]. There the two are DESIGNED to differ: legacy fixed those
// notes before the edit (they were decided at grid time and sit in its
// lookahead), the lane replaces every not-yet-started note. Everything else
// must match exactly.
func TestLaneHeardEqualsLegacyDispatched(t *testing.T) {
	restore := SetRuntimeProfileForTest(browserRuntimeProfile())
	t.Cleanup(restore)
	// The harness drives a FAKE audio clock (20 s) inside ~1 s of wall time,
	// and Seek / BPM re-anchoring pair time.Now() with audio.Now(): the UI
	// playhead and the timeline then disagree in a way the real app (two
	// clocks advancing together) never sees. The legacy game alone trips
	// parity after a seek with no lane involved. Parity is not this test's
	// subject — dispatched audio is.
	// Non-vacuity is judged over all seeds: random deletes / mutes can thin
	// one seed's circuit down to a few dozen notes.
	total := 0
	for seed := int64(1); seed <= 6; seed++ {
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) { total += runLaneOracle(t, seed) })
	}
	if total < 300 {
		t.Fatalf("oracle compared only %d notes over all seeds — vacuous", total)
	}
}

// awaitAudioLoopDrained blocks until g's audioLoop goroutine has taken every
// queued request off audioCh, then yields so it can finish the batch it holds.
//
// Under `go test` the sequencer runs synchronously inside Update(), but
// audioLoop stays a goroutine on WALL time, while this harness advances a fake
// audio clock ~10x faster than wall time. Unpaced, a loaded machine (the whole
// `./...` suite, or GOMAXPROCS=1) let legacy A's audioLoop fall behind: the
// next edit bumped the row's structural generation under notes still queued,
// audioLoop dropped them as stale ("legacy: nothing"), and Stage C of the
// timing gate panicked on an `overdue/absmax` lateness that exists only in the
// harness. In the real app the two clocks advance together and audioLoop
// drains within a frame, long before the next edit; this restores that.
func awaitAudioLoopDrained(t *testing.T, g *Game) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for len(g.audioCh) > 0 {
		if time.Now().After(deadline) {
			t.Fatalf("audioLoop did not drain %d queued requests within 2 s", len(g.audioCh))
		}
		runtime.Gosched()
	}
	for i := 0; i < 4; i++ {
		runtime.Gosched()
	}
}

// oracleKey identifies a note by row and GRID TIME (0.1 ms buckets), not by
// abs: a seek restarts abs numbering, so the same abs names different
// moments in different transport generations.
func oracleKey(row int, grid float64) [2]int {
	return [2]int{row, int(math.Round(grid * 1e4))}
}

func runLaneOracle(t *testing.T, seed int64) int {
	a := newLaneCircuitGame(t)
	b := newLaneCircuitGame(t)
	a.parityWatch, b.parityWatch = parityWatchLog, parityWatchLog // see TestLaneHeardEqualsLegacyDispatched
	// A custom-logic node on both: its decision (gain + a position-dependent
	// transpose) reaches the lane only through the predictor's recorded
	// DecisionAt — the oracle must see it match legacy. Keyed on
	// AbsoluteIndex, not TriggerCount: legacy keeps its OWN trigger counter
	// (nodeLogicTriggerCountsByRow, never reset by a seek) while the lane
	// follows the predictor's walk — the documented source of truth. No
	// production writer sets Params.Logic (no UI / JSON / JS path).
	for _, g := range []*Game{a, b} {
		n := g.nodeAt(6, 0)
		if n == nil {
			t.Fatal("setup: node 6 missing")
		}
		g.graph.SetNodeParams(n.ID, model.NodeParams{Volume: 1, Duration: 1, Logic: func(c model.NodeContext) model.NodeDecision {
			return model.NodeDecision{VolumeMul: 0.8, PitchDelta: float64(c.AbsoluteIndex%3 + 1)}
		}})
		g.updateBeatInfos()
	}
	// One clock for both: audio.SetNowForTest is process-global (b's won).
	clock := laneTestNow(b)
	laneClocks.Store(a, clock)
	a.state.StartClock(time.Now(), *clock)
	noop := func(int, int) {}
	a.scheduleHook, b.scheduleHook = noop, noop // same "instrument available" rule on both

	var mu sync.Mutex
	legacy := map[[2]int]heardNote{}
	restoreHook := audio.SetPlayBatchHookForTest(func(batch []audio.BatchParam) {
		mu.Lock()
		defer mu.Unlock()
		for _, p := range batch {
			if p.Row >= 0 {
				k := oracleKey(p.Row, p.Grid)
				legacy[k] = legacy[k].addVoice(string(p.ID), p.Vol, p.Pitch, p.Grid)
			}
		}
	})
	defer restoreHook()

	sink := newFakeSink(clock)
	enableAudioLaneForTest(t, b, sink)

	rngA := rand.New(rand.NewSource(seed))
	rngB := rand.New(rand.NewSource(seed))
	var edits []float64
	var editNames []string
	const step, dur = 0.004, 20.0
	start := *clock
	nextEdit := start + 0.5 // edits from 0.5 s in, every 0.4–0.8 s, none in the last 2 s
	for *clock < start+dur {
		advanceLaneClock(clock, step)
		if *clock >= nextEdit && *clock < start+dur-2 {
			applyRandomLaneEdit(t, a, rngA)
			nameA := lastLaneEditName
			applyRandomLaneEdit(t, b, rngB)
			if lastLaneEditName != nameA {
				t.Fatalf("edit scripts diverged: %q vs %q", nameA, lastLaneEditName)
			}
			edits = append(edits, *clock)
			editNames = append(editNames, nameA)
			nextEdit += 0.4 + rngA.Float64()*0.4
			rngB.Float64() // keep the two streams aligned
		}
		if err := a.Update(); err != nil {
			t.Fatal(err)
		}
		awaitAudioLoopDrained(t, a)
		if err := b.Update(); err != nil {
			t.Fatal(err)
		}
	}
	end := *clock
	// Let audioLoop drain A's last batches.
	time.Sleep(200 * time.Millisecond)

	excluded := func(grid float64) bool {
		if grid < 1.3 || grid > end-0.3 {
			return true
		}
		for _, e := range edits {
			if grid >= e-0.1 && grid <= e+0.02 {
				return true
			}
		}
		return false
	}
	lane := map[[2]int]heardNote{}
	for _, n := range sink.sched { // not heard(): it keys by abs, which a seek reuses
		if n.Kind == laneSound && n.Time <= end {
			k := oracleKey(n.Row, n.Grid)
			for v := 0; v < n.voiceCount(); v++ {
				pitch, vol := n.voiceAt(v)
				lane[k] = lane[k].addVoice(string(n.Inst), vol, pitch, n.Grid)
			}
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if w := os.Getenv("ORACLE_WINDOW"); w != "" {
		var lo, hi float64
		fmt.Sscanf(w, "%f,%f", &lo, &hi)
		var lines []string
		for _, l := range legacy {
			if l.grid >= lo && l.grid <= hi {
				lines = append(lines, fmt.Sprintf("%.4f L %s v%.2f p%.0f", l.grid, l.inst, l.vol, l.pitch))
			}
		}
		for _, n := range lane {
			if n.grid >= lo && n.grid <= hi {
				lines = append(lines, fmt.Sprintf("%.4f   N %s v%.2f p%.0f", n.grid, n.inst, n.vol, n.pitch))
			}
		}
		for i, e := range edits {
			if e >= lo && e <= hi {
				lines = append(lines, fmt.Sprintf("%.4f EDIT %s", e, editNames[i]))
			}
		}
		sort.Strings(lines)
		for _, l := range lines {
			t.Log(l)
		}
	}
	var diffs []string
	compared := 0
	for k, l := range legacy {
		if excluded(l.grid) {
			continue
		}
		compared++
		n, ok := lane[k]
		if !ok {
			diffs = append(diffs, fmt.Sprintf("%v legacy %+v, lane: nothing", k, l))
		} else if n.inst != l.inst || !sameVoices(n.voices, l.voices) {
			diffs = append(diffs, fmt.Sprintf("%v legacy %+v, lane %+v", k, l, n))
		}
	}
	for k, n := range lane {
		if excluded(n.grid) {
			continue
		}
		if _, ok := legacy[k]; !ok {
			diffs = append(diffs, fmt.Sprintf("%v lane %+v, legacy: nothing", k, n))
		}
	}
	after := func(grid float64) string {
		for i := len(edits) - 1; i >= 0; i-- {
			if edits[i] <= grid {
				return fmt.Sprintf("after %s@%.3f +%.3f", editNames[i], edits[i], grid-edits[i])
			}
		}
		return "before any edit"
	}
	for i, d := range diffs {
		var g float64
		if _, err := fmt.Sscanf(d[strings.Index(d, "grid:")+5:], "%f", &g); err == nil {
			diffs[i] = fmt.Sprintf("%08.3f %s  [%s]", g, d, after(g))
		}
	}
	sort.Strings(diffs)
	if os.Getenv("ORACLE_DUMP") != "" {
		for _, d := range diffs {
			t.Log(d)
		}
	}
	t.Logf("%d notes compared over %d edits", compared, len(edits))
	if len(diffs) > 0 {
		t.Fatalf("%d mismatches over %d compared notes (%d edits); first: %s", len(diffs), compared, len(edits), diffs[0])
	}
	return compared
}
