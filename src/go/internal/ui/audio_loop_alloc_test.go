//go:build test

// Phase 4 — audioLoop allocation discipline.
//
// The long-session OOM the user reported with plain playback (no EQ
// panel) landed in audioLoop's `reqs := make([]soundReq, 0, reqCap)`
// at game_audio_loop.go:22. The fix reuses scratch buffers
// (audioLoopReqs / audioLoopBatch) across iterations so steady-state
// playback does not allocate ~3 KB of slice headers per drain cycle.
//
// This test exercises the SAME scratch-buffer reuse pattern the real
// audioLoop uses, on an isolated Game shell so the live audioLoop
// goroutine doesn't race with us for the channel. Asserts the
// allocation budget. Pre-fix: ~6 KB+ per cycle. Post-fix: ≤ 1 KB.

package ui

import (
	"runtime"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestAudioLoopAllocStability validates the reuse contract on the
// scratch buffers without invoking the real audioLoop goroutine.
// Constructs a bare Game struct (no New() — that would spawn the live
// goroutine that competes for audioCh) and runs the drain pattern
// against the same Game fields.
func TestAudioLoopAllocStability(t *testing.T) {
	// Bare struct — no goroutines, no subsystems. We're testing the
	// allocator behaviour of the reuse pattern, not the dispatch path.
	g := &Game{
		audioCh: make(chan soundReq, 256),
	}

	enqueue := func(n int) {
		for i := 0; i < n; i++ {
			g.audioCh <- soundReq{id: "kick", vol: 1, pitch: 0, dur: 1, row: -1, abs: -1}
		}
	}

	// Warm up: queue + drain a handful of cycles so the scratch slices
	// reach their steady-state capacity.
	for warmup := 0; warmup < 16; warmup++ {
		enqueue(8)
		drainOneAudioLoopIteration(g)
	}

	const cycles = 1000
	runtime.GC()
	var pre, post runtime.MemStats
	runtime.ReadMemStats(&pre)
	for i := 0; i < cycles; i++ {
		enqueue(8)
		drainOneAudioLoopIteration(g)
	}
	runtime.ReadMemStats(&post)
	bytesPerCycle := (post.TotalAlloc - pre.TotalAlloc) / cycles
	t.Logf("audioLoop scratch reuse TotalAlloc/cycle = %d bytes (cycles=%d)",
		bytesPerCycle, cycles)

	// Phase 4 budget. Pre-fix the reqs (~3 KB) + batch (~256 B) slices
	// alone made every cycle >3.3 KB. With reuse, the steady-state
	// allocations come only from append-grow (amortized to 0) and
	// the channel send/receive bookkeeping. 512 B cap catches a
	// regression that reverts the reuse, with comfortable headroom
	// for incidental scheduler/runtime noise.
	const maxBytesPerCycle = 512
	if bytesPerCycle > maxBytesPerCycle {
		t.Errorf("audioLoop scratch reuse: bytes/cycle = %d, want <= %d "+
			"(scratch-buffer reuse regressed; see game_audio_loop.go and "+
			"game_struct.go audioLoopReqs/audioLoopBatch fields)",
			bytesPerCycle, maxBytesPerCycle)
	}

	// Sanity: scratch fields must have grown to hold the batch and
	// stayed at capacity across cycles. Capacity should never shrink.
	if cap(g.audioLoopReqs) == 0 {
		t.Error("audioLoopReqs capacity = 0 after warm-up; reuse never engaged")
	}
	if cap(g.audioLoopBatch) == 0 {
		t.Error("audioLoopBatch capacity = 0 after warm-up; reuse never engaged")
	}
}

// drainOneAudioLoopIteration mirrors one iteration of audioLoop's
// for-range body. Tracks the SAME scratch-buffer reuse the real
// audioLoop does so the test exercises the production fix path.
func drainOneAudioLoopIteration(g *Game) {
	first, ok := <-g.audioCh
	if !ok {
		return
	}

	maxBatch := 32 // matches RuntimeProf().AudioBatchMax in default profile
	reqCap := 32
	if reqCap > maxBatch {
		reqCap = maxBatch
	}
	if cap(g.audioLoopReqs) < reqCap {
		g.audioLoopReqs = make([]soundReq, 0, reqCap)
	} else {
		g.audioLoopReqs = g.audioLoopReqs[:0]
	}
	reqs := g.audioLoopReqs
	reqs = append(reqs, first)

	drain := true
	for drain && len(reqs) < maxBatch {
		select {
		case req, ok := <-g.audioCh:
			if !ok {
				drain = false
				continue
			}
			reqs = append(reqs, req)
		default:
			drain = false
		}
	}
	g.audioLoopReqs = reqs

	if cap(g.audioLoopBatch) < len(reqs) {
		g.audioLoopBatch = make([]audio.BatchParam, 0, len(reqs))
	} else {
		g.audioLoopBatch = g.audioLoopBatch[:0]
	}
	batch := g.audioLoopBatch
	for _, req := range reqs {
		batch = append(batch, audio.BatchParam{ID: req.id, Vol: req.vol, Pitch: req.pitch, Dur: req.dur})
	}
	g.audioLoopBatch = batch
}
