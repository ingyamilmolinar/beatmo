package ui

import (
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

type laneOpKind uint8

const (
	laneOpSchedule laneOpKind = iota + 1
	laneOpCancel
	laneOpStopAt
	laneOpCancelAll
	laneOpReplace // vid replaces old, only if the engine removed old
)

type laneOp struct {
	kind laneOpKind
	vid  uint64
	old  uint64 // laneOpReplace: first id of the replaced note's block
	oldN int    // laneOpReplace: how many voices the replaced note owns
	note laneNote
	vids []uint64
	inst instrument.ID
	t    float64
}

// outboxSink is the production audioSink: the lane (ticking under seqMu)
// only RECORDS ops, in order; audioLoop applies them through flushLaneOps.
// So the engine calls run on the audio dispatch goroutine, never under seqMu
// — a cold desktop voice render must not block Update.
type outboxSink struct {
	mu   sync.Mutex
	ops  []laneOp
	wake chan struct{}
}

func (o *outboxSink) push(op laneOp) {
	o.mu.Lock()
	o.ops = append(o.ops, op)
	o.mu.Unlock()
	select {
	case o.wake <- struct{}{}:
	default:
	}
}

func (o *outboxSink) Schedule(vid uint64, n laneNote) {
	o.push(laneOp{kind: laneOpSchedule, vid: vid, note: n})
}
func (o *outboxSink) Replace(old uint64, oldVoices int, vid uint64, n laneNote) {
	o.push(laneOp{kind: laneOpReplace, old: old, oldN: oldVoices, vid: vid, note: n})
}
func (o *outboxSink) Cancel(v []uint64) {
	o.push(laneOp{kind: laneOpCancel, vids: append([]uint64(nil), v...)})
}
func (o *outboxSink) StopAt(inst instrument.ID, t float64) {
	o.push(laneOp{kind: laneOpStopAt, inst: inst, t: t})
}
func (o *outboxSink) CancelAllFuture() { o.push(laneOp{kind: laneOpCancelAll}) }

func (o *outboxSink) take() []laneOp {
	o.mu.Lock()
	ops := o.ops
	o.ops = nil
	o.mu.Unlock()
	return ops
}

// flushLaneOps applies queued lane ops in order (audioLoop goroutine).
// Consecutive schedules are batched into one audio.PlayBatch — one Go→JS
// crossing on WASM.
func (g *Game) flushLaneOps() {
	if g.laneSink == nil {
		return
	}
	// One flusher at a time: audioLoop and Close both flush, and two
	// interleaved flushes would share laneBatchScratch and could apply a taken
	// Schedule batch after Close's cancel-all.
	g.laneFlushMu.Lock()
	defer g.laneFlushMu.Unlock()
	ops := g.laneSink.take()
	if len(ops) == 0 {
		return
	}
	batch := g.laneBatchScratch[:0]
	audioNow := audio.Now()
	flush := func() {
		if len(batch) > 0 {
			audio.PlayBatch(batch)
			batch = batch[:0]
		}
	}
	add := func(vid uint64, n laneNote) {
		// Stage C on the lane: lead = when − now at commit (≈ the horizon
		// in steady state; negative = committed late). One observation and
		// one timing trace per CELL, however many voices it sounds.
		if audioNow > 0 && !g.parityInGrace() {
			g.schedMetrics.Observe(audioNow, n.Time)
		}
		g.openTimingTrace(n.Row, n.Abs, n.Time, g.audioGen.Load(), g.parityGen.Load())
		for k := 0; k < n.voiceCount(); k++ {
			pitch, vol := n.voiceAt(k)
			if g.voicePrewarmer != nil {
				g.voicePrewarmer.warmPitch(n.Inst, pitch)
			}
			batch = append(batch, audio.BatchParam{ID: n.Inst, Vol: vol, Pitch: pitch, Dur: n.Dur,
				When: n.Time, HasWhen: true, Row: n.Row, Abs: n.Abs, Ideal: n.Time, Grid: n.Grid, VoiceID: vid + uint64(k)})
		}
	}
	for i := 0; i < len(ops); i++ {
		op := ops[i]
		switch op.kind {
		case laneOpSchedule:
			add(op.vid, op.note)
		case laneOpReplace:
			// A run of replacements: cancel every old voice in ONE engine call
			// (one bridge crossing on WASM), THEN schedule the replacements
			// whose old voices the engine actually removed. A chord is
			// replaced all-or-nothing: if ANY of its old voices had started,
			// none of the new ones play (never the old and new chord together).
			j := i
			olds := g.laneOldScratch[:0]
			for ; j < len(ops) && ops[j].kind == laneOpReplace; j++ {
				for k := 0; k < max(1, ops[j].oldN); k++ {
					olds = append(olds, ops[j].old+uint64(k))
				}
			}
			g.laneOldScratch = olds
			flush()
			started := audio.CancelVoices(olds)
			for _, r := range ops[i:j] {
				if !anyStarted(started, r.old, r.oldN) {
					add(r.vid, r.note)
				}
			}
			i = j - 1
		case laneOpCancel:
			flush()
			audio.CancelVoices(op.vids)
		case laneOpStopAt:
			flush()
			audio.StopLaneAt(op.inst, op.t)
		case laneOpCancelAll:
			flush()
			audio.CancelAllLaneFuture()
		}
	}
	flush()
	g.laneBatchScratch = batch
	g.drainEmissionReports()
}

// anyStarted reports whether the engine refused any id of a block.
func anyStarted(started []uint64, base uint64, n int) bool {
	for k := 0; k < max(1, n); k++ {
		if containsVID(started, base+uint64(k)) {
			return true
		}
	}
	return false
}

func containsVID(vids []uint64, v uint64) bool {
	for _, x := range vids {
		if x == v {
			return true
		}
	}
	return false
}
