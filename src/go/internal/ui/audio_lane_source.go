package ui

import (
	"hash/fnv"
	"math"
	"unsafe"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// gameLaneSource adapts the Game to laneSource. refresh() must run once per
// lane tick, before the lane reads it (laneTick does this).
type gameLaneSource struct {
	s         *playbackSlate
	ctx       laneCtx
	ctxOK     bool
	prevNon   []uint64 // per row: fingerprint without the voice revision
	voiceOnly []bool
}

func (s *playbackSlate) newGameLaneSource() *gameLaneSource { return &gameLaneSource{s: s} }

func (src *gameLaneSource) refresh() { src.ctx, src.ctxOK = src.s.laneContext() }

func (src *gameLaneSource) Playing() bool {
	g := src.s.g
	return g.Playing() && !g.Paused() && src.ctxOK
}
func (src *gameLaneSource) Now() float64         { return audio.Now() }
func (src *gameLaneSource) TransportGen() uint64 { return src.s.g.audioGen.Load() }
func (src *gameLaneSource) Rows() int            { return len(src.s.g.drum.Rows) }

func laneHashU64(h interface{ Write([]byte) (int, error) }, xs ...uint64) {
	var b [8]byte
	for _, x := range xs {
		for i := range b {
			b[i] = byte(x >> (8 * i))
		}
		_, _ = h.Write(b[:])
	}
}

func laneBool(b bool) uint64 {
	if b {
		return 1
	}
	return 0
}

// GlobalKey fingerprints the inputs every row's notes read: the path
// snapshot (pointer: rebuilt on every path change), the predictor epoch
// (node params, rebase, paths), BPM / subdivision, the clock anchor (play,
// seek, BPM re-anchor), solo state, the audio offset and the graph version
// (node / group hooks).
func (src *gameLaneSource) GlobalKey() uint64 {
	g := src.s.g
	h := fnv.New64a()
	bb, as := g.state.AnchorKey()
	laneHashU64(h,
		uint64(uintptr(unsafe.Pointer(src.ctx.snap))),
		g.engine.Predictor.Epoch(),
		uint64(src.ctx.bpm), uint64(src.ctx.div),
		math.Float64bits(bb), math.Float64bits(as),
		laneBool(src.ctx.anySolo),
		math.Float64bits(src.ctx.offset),
		g.laneGraphVersion.Load(),
	)
	return h.Sum64()
}

func (src *gameLaneSource) rowNonVoiceKey(r int) uint64 {
	row := src.s.g.drum.Rows[r]
	h := fnv.New64a()
	_, _ = h.Write([]byte(row.Instrument))
	laneHashU64(h, math.Float64bits(row.Volume), laneBool(row.Muted), laneBool(row.Solo))
	return h.Sum64()
}

// RowKey fingerprints one row's own inputs: instrument, volume, mute, solo
// and the instrument's voice revision (synth / sampler edits).
func (src *gameLaneSource) RowKey(r int) uint64 {
	if len(src.prevNon) != src.Rows() {
		src.prevNon = make([]uint64, src.Rows())
		src.voiceOnly = make([]bool, src.Rows())
	}
	non := src.rowNonVoiceKey(r)
	src.voiceOnly[r] = non == src.prevNon[r]
	src.prevNon[r] = non
	return non*31 + audio.VoiceRevision(src.s.g.drum.Rows[r].Instrument)
}

func (src *gameLaneSource) VoiceRevOnly(r int) bool {
	return r < len(src.voiceOnly) && src.voiceOnly[r]
}

// Steps maps a Time window to abs steps, widened by ONE step each side: a
// groove nudge is at most one subdivision (nodeNudgeSecondsAt), so a nudged
// note can never fall outside the scanned range. The lane filters by Time.
func (src *gameLaneSource) Steps(from, to float64) (int, int, bool) {
	c := src.ctx
	if !src.ctxOK {
		return 0, -1, false
	}
	lo, ok1 := src.s.g.state.StepAtAudioTime(from-c.offset, c.bpm, c.div)
	hi, ok2 := src.s.g.state.StepAtAudioTime(to-c.offset, c.bpm, c.div)
	lo, hi = lo-1, hi+1
	if !ok1 || !ok2 {
		return 0, -1, false
	}
	if lo < 0 {
		lo = 0
	}
	if hi >= lo {
		src.s.g.engine.Predictor.Ensure(hi + 1) // cells must exist before NoteAt reads them
	}
	return lo, hi, true
}

// VoiceReady: every voice of the note (a chord's layers) is rendered.
func (src *gameLaneSource) VoiceReady(n laneNote) bool {
	for k := range n.voiceCount() {
		if p, _ := n.voiceAt(k); !audio.VoiceReady(n.Inst, p) {
			return false
		}
	}
	return true
}

// Warm asks the prewarm pool for the note's voices (never inline: the lane
// runs on the sequencer goroutine).
func (src *gameLaneSource) Warm(n laneNote) {
	pw := src.s.g.voicePrewarmer
	if pw == nil {
		return
	}
	for k := range n.voiceCount() {
		p, _ := n.voiceAt(k)
		pw.warmPitch(n.Inst, p)
	}
}

// Retarget is the same cell played by inst, hashed exactly as NoteAt would.
func (src *gameLaneSource) Retarget(n laneNote, inst instrument.ID) laneNote {
	n.Inst = inst
	n.Hash = laneNoteHash(&n, n.node, audio.VoiceRevision(inst))
	return n
}

func (src *gameLaneSource) NoteAt(row, abs int) (laneNote, bool) {
	if !src.ctxOK {
		return laneNote{}, false
	}
	return src.s.laneNoteAt(&src.ctx, row, abs)
}
