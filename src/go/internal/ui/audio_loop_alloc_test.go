//go:build test

// audioLoop reuses scratch buffers so steady playback does not allocate slice headers per drain; this runs the
// same pattern on a bare Game, away from the live goroutine.

package ui

import (
	"runtime"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// A bare Game: New() would spawn the live audioLoop competing for audioCh.
func TestAudioLoopAllocStability(t *testing.T) {
	g := &Game{
		audioCh: make(chan soundReq, 256),
	}
	// Struct literals bypassing New() must set the back-reference.
	g.playbackSlate.g = g

	enqueue := func(n int) {
		for i := 0; i < n; i++ {
			g.audioCh <- soundReq{id: roster.Kick, vol: 1, pitch: 0, dur: 1, row: -1, abs: -1}
		}
	}

	// Until the scratch slices reach steady capacity.
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

	// Steady state allocates only channel bookkeeping; 512 B catches a reverted reuse (> 3 KB a cycle) with headroom
	// for runtime noise.
	const maxBytesPerCycle = 512
	if bytesPerCycle > maxBytesPerCycle {
		t.Errorf("audioLoop scratch reuse: bytes/cycle = %d, want <= %d "+
			"(scratch-buffer reuse regressed; see game_audio_loop.go and "+
			"game_struct.go audioLoopReqs/audioLoopBatch fields)",
			bytesPerCycle, maxBytesPerCycle)
	}

	// Grown, and never shrinking.
	if cap(g.audioLoopReqs) == 0 {
		t.Error("audioLoopReqs capacity = 0 after warm-up; reuse never engaged")
	}
	if cap(g.audioLoopBatch) == 0 {
		t.Error("audioLoopBatch capacity = 0 after warm-up; reuse never engaged")
	}
}

// One iteration of audioLoop's body, with the same scratch reuse.
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
