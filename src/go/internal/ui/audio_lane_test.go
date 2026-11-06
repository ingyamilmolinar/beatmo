//go:build test

package ui

import (
	"fmt"
	"math"
	"sync"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// fakeSink models an audio engine: Schedule stores a voice; Cancel removes it
// only if it has not started (Time > now) — exactly the engines' contract.
type fakeSink struct {
	mu      sync.Mutex // the kick test drives the lane from the sequencer goroutine
	sched   map[uint64]laneNote
	stopAts []string
	all     int
	now     *float64
	// engineLead: how far the ENGINE's notion of "started" runs ahead of the
	// lane's clock (the desktop mixer renders up to one 10 ms Read chunk
	// ahead). A cancel the lane thinks is in time can still be refused.
	engineLead float64
	refused    int // replacements refused because the old voice had started
}

func newFakeSink(now *float64) *fakeSink { return &fakeSink{sched: map[uint64]laneNote{}, now: now} }

func (f *fakeSink) Schedule(vid uint64, n laneNote) { f.mu.Lock(); f.sched[vid] = n; f.mu.Unlock() }

func (f *fakeSink) Replace(old uint64, _ int, vid uint64, n laneNote) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if o, ok := f.sched[old]; ok {
		if o.Time <= *f.now+f.engineLead {
			f.refused++
			return // the engine already started the old voice: it stays, alone
		}
		delete(f.sched, old)
	}
	f.sched[vid] = n
}

// scheduled is a goroutine-safe snapshot of the voices still scheduled.
func (f *fakeSink) scheduled() []laneNote {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]laneNote, 0, len(f.sched))
	for _, n := range f.sched {
		out = append(out, n)
	}
	return out
}

func (f *fakeSink) Cancel(v []uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, id := range v {
		if n, ok := f.sched[id]; ok && n.Time > *f.now+f.engineLead {
			delete(f.sched, id)
		}
	}
}
func (f *fakeSink) StopAt(inst instrument.ID, t float64) { f.stopAts = append(f.stopAts, string(inst)) }
func (f *fakeSink) CancelAllFuture() {
	f.all++
	for id, n := range f.sched {
		if n.Time > *f.now {
			delete(f.sched, id)
		}
	}
}

// heard is what actually played: scheduled voices whose start has passed.
func (f *fakeSink) heard() map[[2]int]laneNote {
	out := map[[2]int]laneNote{}
	for _, n := range f.sched {
		if n.Time <= *f.now {
			out[[2]int{n.Row, n.Abs}] = n
		}
	}
	return out
}

// gridSource: every row sounds on every step (0.125 s = 120 BPM, div 4);
// rows can be muted, re-leveled, and have their voice revision bumped.
type gridSource struct {
	now       *float64
	playing   bool
	gen       uint64
	step      float64
	rowVol    []float64
	rowMute   []bool
	voiceRev  []uint64
	prevNon   []uint64
	voiceOnly []bool
	layers    [][]model.Layer // per-row voice stack (nil = single voice)
	// The switch rule (audio_lane_switch_test.go): inst overrides a row's
	// instrument (default "i<row>"); notReady instruments have no rendered
	// voice yet; warmed records Warm requests.
	inst     []instrument.ID
	notReady map[instrument.ID]bool
	warmed   []instrument.ID
	ready    func(n laneNote) bool // overrides notReady when set (per-note readiness)
}

func (g *gridSource) rowInst(r int) instrument.ID {
	if r < len(g.inst) && g.inst[r] != "" {
		return g.inst[r]
	}
	return instrument.ID(fmt.Sprintf("i%d", r))
}

func (g *gridSource) VoiceReady(n laneNote) bool {
	if g.ready != nil {
		return g.ready(n)
	}
	return !g.notReady[n.Inst]
}
func (g *gridSource) Warm(n laneNote)            { g.warmed = append(g.warmed, n.Inst) }
func (g *gridSource) Retarget(n laneNote, inst instrument.ID) laneNote {
	n.Inst = inst
	n.Hash = g.noteHash(n)
	return n
}

func (g *gridSource) noteHash(n laneNote) uint64 {
	h := math.Float64bits(n.Vol)*1_000_003 + g.voiceRev[n.Row]*7919 + uint64(n.Abs) + uint64(len(n.Layers))*104_729
	for _, c := range n.Inst {
		h = h*31 + uint64(c)
	}
	return h
}

func b2u(b bool) uint64 {
	if b {
		return 1
	}
	return 0
}

func (g *gridSource) Playing() bool        { return g.playing }
func (g *gridSource) Now() float64         { return *g.now }
func (g *gridSource) TransportGen() uint64 { return g.gen }
func (g *gridSource) GlobalKey() uint64    { return math.Float64bits(g.step) }
func (g *gridSource) Rows() int            { return len(g.rowVol) }
func (g *gridSource) RowKey(r int) uint64 {
	non := math.Float64bits(g.rowVol[r])*3 + b2u(g.rowMute[r])
	for _, c := range g.rowInst(r) {
		non = non*31 + uint64(c)
	}
	g.voiceOnly[r] = non == g.prevNon[r]
	g.prevNon[r] = non
	return non*31 + g.voiceRev[r]
}
func (g *gridSource) VoiceRevOnly(r int) bool { return g.voiceOnly[r] }

// Steps widens the range by one step each side, like the Game source's
// ±groove-nudge margin: the lane must filter by Time itself (and honour the
// cancel guard) rather than rely on Steps being exact.
func (g *gridSource) Steps(from, to float64) (int, int, bool) {
	lo := int(math.Ceil(from/g.step-1e-9)) - 1
	hi := int(math.Floor(to/g.step+1e-9)) + 1
	if lo < 0 {
		lo = 0
	}
	return lo, hi, true
}
func (g *gridSource) NoteAt(row, abs int) (laneNote, bool) {
	if row >= len(g.rowVol) || g.rowMute[row] {
		return laneNote{}, false
	}
	t := float64(abs) * g.step
	n := laneNote{Row: row, Abs: abs, Kind: laneSound, Inst: g.rowInst(row), Vol: g.rowVol[row], Pitch: 0, Dur: 1, Time: t, Grid: t}
	if row < len(g.layers) {
		n.Layers = g.layers[row]
	}
	n.Hash = g.noteHash(n)
	return n, true
}

func newGridLane(t *testing.T, rows int) (*audioLane, *gridSource, *fakeSink, *float64) {
	t.Helper()
	now := new(float64)
	*now = 1.06 // between steps: no note sits exactly on the commit boundary
	src := &gridSource{now: now, playing: true, step: 0.125,
		rowVol: make([]float64, rows), rowMute: make([]bool, rows), voiceRev: make([]uint64, rows),
		prevNon: make([]uint64, rows), voiceOnly: make([]bool, rows)}
	for i := range src.rowVol {
		src.rowVol[i] = 1
	}
	sink := newFakeSink(now)
	l := newAudioLane(src, sink, laneConfig{Horizon: 1.0, Guard: 0.008, LateTol: 0.025, FullReconcile: 0.25, VoiceRecommitMin: 0.03})
	return l, src, sink, now
}

func TestLaneCommitsHorizonAhead(t *testing.T) {
	l, _, sink, now := newGridLane(t, 2)
	l.Tick()
	if got := len(sink.sched); got != 2*8 {
		t.Fatalf("want 16 notes committed (steps 1.125…2.0 × 2 rows), got %d", got)
	}
	*now += 0.6 // a 600 ms stall: no ticks at all
	if len(sink.heard()) == 0 {
		t.Fatal("notes inside the stall must already be committed")
	}
}

func TestLaneVolumeEditReachesNextNotStartedNote(t *testing.T) {
	l, src, sink, now := newGridLane(t, 1)
	l.Tick()
	src.rowVol[0] = 0.5
	*now += 0.004
	l.Tick()
	future := 0
	for _, n := range sink.sched {
		if n.Time > *now+0.008 {
			future++
			if n.Vol != 0.5 {
				t.Fatalf("a not-started note kept the old volume: %+v", n)
			}
		}
	}
	if future == 0 {
		t.Fatal("replacements were not scheduled")
	}
}

func TestLaneChangeInsideGuardDoesNotDouble(t *testing.T) {
	l, src, sink, now := newGridLane(t, 1)
	l.Tick()
	*now = 1.125 - 0.004 // abs 9 (1.125) is inside the 8 ms cancel guard
	src.rowVol[0] = 0.5
	l.Tick()
	copies := 0
	for _, n := range sink.sched {
		if n.Abs == 9 {
			copies++
		}
	}
	if copies != 1 {
		t.Fatalf("a note inside the guard must be neither cancelled nor doubled: %d copies", copies)
	}
}

// Replacing a note must be ATOMIC on the engine: when the engine has already
// started the old voice (its clock runs ahead of the lane's), the cancel is
// refused — and the replacement must then NOT play too (a doubled hit / flam).
func TestLaneReplaceRefusedByEngineDoesNotDouble(t *testing.T) {
	l, src, sink, now := newGridLane(t, 1)
	sink.engineLead = 0.015
	l.Tick()
	*now = 1.125 - 0.012 // abs 9: outside the lane's 8 ms guard, inside the engine's lead
	src.rowVol[0] = 0.5
	l.Tick()
	copies := 0
	for _, n := range sink.sched {
		if n.Abs == 9 {
			copies++
		}
	}
	if copies != 1 {
		t.Fatalf("the engine had already started abs 9: the replacement must not play as well (%d copies)", copies)
	}
}

func TestLaneMuteCancelsFutureNotes(t *testing.T) {
	l, src, sink, now := newGridLane(t, 1)
	l.Tick()
	src.rowMute[0] = true
	l.Tick()
	for _, n := range sink.sched {
		if n.Time > *now+0.008 {
			t.Fatalf("muted row still has future note %+v", n)
		}
	}
}

func TestLaneStopCancelsFuture(t *testing.T) {
	l, _, sink, _ := newGridLane(t, 2)
	l.Tick()
	l.Reset()
	if sink.all != 1 || len(sink.sched) != 0 {
		t.Fatalf("Reset must CancelAllFuture: calls=%d left=%d", sink.all, len(sink.sched))
	}
}

func TestLaneTransportGenChangeResets(t *testing.T) {
	l, src, sink, _ := newGridLane(t, 1)
	l.Tick()
	src.gen++
	l.Tick()
	if sink.all != 1 {
		t.Fatalf("a new transport generation must CancelAllFuture once, got %d", sink.all)
	}
}

func TestLaneLatePolicy(t *testing.T) {
	l, _, sink, now := newGridLane(t, 1)
	l.Tick()    // committed through 2.06
	*now += 1.5 // stall longer than H: notes in (2.06, 2.535) were never committed
	l.Tick()
	st := l.Stats()
	if st.DroppedLate == 0 {
		t.Fatal("notes due more than LateTol ago must be dropped and counted, not rushed")
	}
	for _, n := range sink.sched {
		if n.Abs > 16 && n.Time < *now-0.025-1e-9 {
			t.Fatalf("rushed a stale note %+v", n)
		}
	}
}

func TestLaneVoiceRecommitRateLimited(t *testing.T) {
	l, src, _, now := newGridLane(t, 1)
	l.Tick()
	before := l.Stats().Reconciles
	for i := 0; i < 10; i++ { // knob drag: a new voice revision every 4 ms tick
		src.voiceRev[0]++
		*now += 0.004
		l.Tick()
	}
	if got := l.Stats().Reconciles - before; got > 2 {
		t.Fatalf("voice-only changes must be rate-limited to ≤ 1 per 30 ms: %d reconciles in 40 ms", got)
	}
	mid := l.Stats().Reconciles
	*now += 0.04
	l.Tick() // the drag settled: the final revision must still be reconciled
	if l.Stats().Reconciles == mid {
		t.Fatal("the final voice revision was never reconciled")
	}
}

// A knob drag (voice-only edits) re-commits only the notes within VoiceNear
// at once; the rest of the horizon — committed under the previous voice — is
// re-derived as it enters that window. Every note still starts on the voice
// current when it was re-derived, for a fraction of the churn: re-committing
// the whole second on every drag tick was ~3000 WebAudio voice starts/s in
// webaudio_all_knob_bpm_stress, and near-due replacements queued behind the
// far ones and started late.
func TestLaneVoiceDragRecommitsNearWindowFirst(t *testing.T) {
	drag := func(near float64) (cancelled uint64, stale int) {
		l, src, sink, now := newGridLane(t, 1)
		l.cfg.VoiceNear = near
		l.Tick()
		base := l.Stats().Cancelled
		dragEnd := *now + 1.0
		for *now < dragEnd { // a new voice revision every 16 ms, ticks every 4 ms
			src.voiceRev[0]++
			for k := 0; k < 4; k++ {
				*now += 0.004
				l.Tick()
			}
		}
		cancelled = l.Stats().Cancelled - base
		settle := *now
		for k := 0; k < 400; k++ { // the drag settled: let the old horizon play out
			*now += 0.004
			l.Tick()
		}
		for _, n := range sink.heard() {
			want, _ := src.NoteAt(n.Row, n.Abs)
			if n.Time > settle+0.008 && n.Hash != want.Hash {
				stale++
			}
		}
		return cancelled, stale
	}
	whole, wholeStale := drag(0)
	nearCancelled, nearStale := drag(0.25)
	if wholeStale != 0 || nearStale != 0 {
		t.Fatalf("notes after the drag must start on its final voice: %d stale (whole window), %d (near window)", wholeStale, nearStale)
	}
	if nearCancelled*2 > whole {
		t.Fatalf("near-window recommits must at least halve the voice churn of a drag: %d replacements vs %d for the whole window", nearCancelled, whole)
	}
}

// A voice-only edit still reaches the next note that has not started on the
// first tick the recommit rate limit allows.
func TestLaneVoiceEditReachesNextNoteAtOnce(t *testing.T) {
	l, src, sink, now := newGridLane(t, 1)
	l.cfg.VoiceNear = 0.25
	l.Tick()
	*now += 0.04 // past VoiceRecommitMin since the initial commit
	l.Tick()
	src.voiceRev[0]++
	*now += 0.004
	l.Tick()
	next := laneNote{Time: math.Inf(1)}
	for _, n := range sink.sched {
		if n.Time > *now+0.008 && n.Time < next.Time {
			next = n
		}
	}
	if want, _ := src.NoteAt(next.Row, next.Abs); next.Hash != want.Hash {
		t.Fatalf("the next not-started note (t=%.3f) kept the old voice", next.Time)
	}
}

func TestLaneRowRemovedCancelsItsNotes(t *testing.T) {
	l, src, sink, now := newGridLane(t, 2)
	l.Tick()
	src.rowVol, src.rowMute, src.voiceRev = src.rowVol[:1], src.rowMute[:1], src.voiceRev[:1]
	src.prevNon, src.voiceOnly = src.prevNon[:1], src.voiceOnly[:1]
	l.Tick()
	for _, n := range sink.sched {
		if n.Row == 1 && n.Time > *now+0.008 {
			t.Fatalf("a removed row kept a future note %+v", n)
		}
	}
}
