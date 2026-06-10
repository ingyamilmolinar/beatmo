//go:build !js

package audio

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// pipeline is the lock-free, allocation-free recording pipeline that owns
// the per-channel encoder workers and streaming WAV writers.
//
// Hot path (audio thread):
//
//	if p := pipelinePtr.Load(); p != nil {
//	    p.Tap(slotIDs, activeSlots, instBufs, workBuf, blockLen)
//	}
//
// Tap acquires a pre-allocated capture block from a bounded pool, copies
// float64 → float32 into it, then non-blocking sends onto a per-channel
// channel. Workers drain and stream-write to disk via streamWavWriter.
//
// On stop, pipelinePtr is cleared first so no new Taps obtain p, then
// in-flight Taps drain (tracked via inflight counter), then channels are
// closed and writers flushed/closed.
type pipeline struct {
	outputDir  string
	sampleRate int
	format     AudioFormat
	startTime  time.Time
	maxSamples int64
	totalCount atomic.Int64 // total master samples observed
	done       atomic.Bool

	// Shutdown coordination.
	stopping atomic.Bool
	// tapWG is incremented at Tap entry and decremented at exit. Detach
	// uses Wait() for a deterministic, non-polling barrier — replaces the
	// old time.Sleep loop that blocked the UI thread up to 200 ms.
	tapWG sync.WaitGroup

	// Block pool: bounded channel of pre-allocated *captureBlock. Audio
	// thread acquires non-blocking; workers return after writing.
	blockPool chan *captureBlock

	// Per-channel state, indexed by instrument ID. Read-only after start.
	channels map[string]*channelState
	masterCh *channelState

	// Worker shutdown.
	workerWg sync.WaitGroup

	// Counters surfaced via Stats.
	dropsTotal atomic.Int64

	// MIDI events captured during recording.
	midiMu     sync.Mutex
	midiEvents []MIDIEvent
}

// captureBlock holds one block of float32 samples destined for a single
// channel. Capacity is fixed at construction; length varies per use.
type captureBlock struct {
	samples []float32
}

// channelState owns a single channel's queue + writer + worker.
type channelState struct {
	id     string
	name   string
	send   chan *captureBlock
	writer *streamWavWriter
	path   string
	count  atomic.Int64 // samples written
}

// pipelineConfig parameters for newPipeline.
type pipelineConfig struct {
	OutputDir   string
	SampleRate  int
	Format      AudioFormat
	Instruments []InstrumentMeta
	MaxDuration time.Duration

	// Tunables (zero → defaults).
	BlockCapacity int // samples per pooled block (default 4096)
	PoolSize      int // pre-allocated blocks (default 64)
	QueueDepth    int // per-channel queue size (default 256)
}

// pipelinePtr is the global atomic pointer checked on the hot audio path.
// nil = not recording. Mirrors the legacy multiCapturePtr pattern.
var pipelinePtr atomic.Pointer[pipeline]

// newPipeline opens per-channel WAV writers in cfg.OutputDir and starts
// an encoder worker for each. The pipeline is not yet attached to the
// hot path — caller must Activate it.
func newPipeline(cfg pipelineConfig) (*pipeline, error) {
	if cfg.SampleRate <= 0 {
		return nil, fmt.Errorf("invalid sample rate: %d", cfg.SampleRate)
	}
	if cfg.OutputDir == "" {
		return nil, fmt.Errorf("output dir required")
	}
	if cfg.Format == "" {
		cfg.Format = FormatWAV24
	}
	if cfg.BlockCapacity <= 0 {
		cfg.BlockCapacity = 4096
	}
	if cfg.PoolSize <= 0 {
		cfg.PoolSize = 64
	}
	if cfg.QueueDepth <= 0 {
		cfg.QueueDepth = 256
	}

	encoder, err := NewEncoder(cfg.Format)
	if err != nil {
		return nil, fmt.Errorf("encoder error: %w", err)
	}
	ext := encoder.FileExtension()
	// Streaming writer is only implemented for WAV today; non-WAV formats
	// fall back to in-memory encode at stop time (handled by caller).
	if !isStreamableFormat(cfg.Format) {
		return nil, fmt.Errorf("format %s is not streamable", cfg.Format)
	}

	p := &pipeline{
		outputDir:  cfg.OutputDir,
		sampleRate: cfg.SampleRate,
		format:     cfg.Format,
		startTime:  time.Now(),
		blockPool:  make(chan *captureBlock, cfg.PoolSize),
		channels:   make(map[string]*channelState, len(cfg.Instruments)+1),
	}
	if cfg.MaxDuration > 0 {
		p.maxSamples = int64(cfg.MaxDuration.Seconds() * float64(cfg.SampleRate))
	}

	// Pre-fill the block pool.
	for i := 0; i < cfg.PoolSize; i++ {
		p.blockPool <- &captureBlock{samples: make([]float32, 0, cfg.BlockCapacity)}
	}

	// Open writers + spawn workers for master and each instrument.
	masterPath := filepath.Join(cfg.OutputDir, "master"+ext)
	masterWriter, err := newStreamWavWriter(masterPath, cfg.Format, cfg.SampleRate)
	if err != nil {
		return nil, fmt.Errorf("open master writer: %w", err)
	}
	p.masterCh = &channelState{
		id:     "master",
		name:   "Master",
		send:   make(chan *captureBlock, cfg.QueueDepth),
		writer: masterWriter,
		path:   masterPath,
	}
	p.workerWg.Add(1)
	go p.runChannelWorker(p.masterCh)

	for _, inst := range cfg.Instruments {
		path := filepath.Join(cfg.OutputDir, inst.ID+ext)
		w, err := newStreamWavWriter(path, cfg.Format, cfg.SampleRate)
		if err != nil {
			// Best-effort cleanup of writers we already opened.
			p.shutdownEarly()
			return nil, fmt.Errorf("open writer %s: %w", inst.ID, err)
		}
		ch := &channelState{
			id:     inst.ID,
			name:   inst.Name,
			send:   make(chan *captureBlock, cfg.QueueDepth),
			writer: w,
			path:   path,
		}
		p.channels[inst.ID] = ch
		p.workerWg.Add(1)
		go p.runChannelWorker(ch)
	}

	return p, nil
}

// shutdownEarly is called when newPipeline fails partway through. It
// closes anything that was opened.
func (p *pipeline) shutdownEarly() {
	if p.masterCh != nil {
		close(p.masterCh.send)
	}
	for _, ch := range p.channels {
		close(ch.send)
	}
	p.workerWg.Wait()
}

// Tap is the hot-path entry. It is called from the audio thread (mixer
// processBlock) and MUST NOT block, allocate, or take a mutex on the
// caller's behalf. instBufs[slot] holds float64 samples per active
// instrument; workBuf holds the final mixed master samples.
func (p *pipeline) Tap(slotIDs []string, activeSlots []int, instBufs [][]float64, workBuf []float64, blockLen int) {
	if p.stopping.Load() || p.done.Load() {
		return
	}
	p.tapWG.Add(1)
	defer p.tapWG.Done()
	// Re-check after Add so we don't race with Detach. The Add/Wait
	// happens-before relationship guarantees Detach's Wait sees this.
	if p.stopping.Load() {
		return
	}

	if blockLen <= 0 {
		return
	}

	// Master sample count gates the maxSamples auto-stop.
	if p.maxSamples > 0 {
		if p.totalCount.Load() >= p.maxSamples {
			p.done.Store(true)
			return
		}
	}

	// Master tap.
	if p.masterCh != nil && len(workBuf) >= blockLen {
		if blk := p.acquireBlock(); blk != nil {
			blk.samples = blk.samples[:0]
			for _, s := range workBuf[:blockLen] {
				blk.samples = append(blk.samples, float32(s))
			}
			select {
			case p.masterCh.send <- blk:
			default:
				p.releaseBlock(blk)
				p.dropsTotal.Add(1)
			}
		} else {
			p.dropsTotal.Add(1)
		}
		p.totalCount.Add(int64(blockLen))
	}

	// Per-instrument taps.
	for _, slot := range activeSlots {
		if slot < 0 || slot >= len(slotIDs) || slot >= len(instBufs) {
			continue
		}
		id := slotIDs[slot]
		ch, ok := p.channels[id]
		if !ok {
			// Instrument not registered for recording; drop silently.
			continue
		}
		buf := instBufs[slot]
		if len(buf) < blockLen {
			continue
		}
		blk := p.acquireBlock()
		if blk == nil {
			p.dropsTotal.Add(1)
			continue
		}
		blk.samples = blk.samples[:0]
		for _, s := range buf[:blockLen] {
			blk.samples = append(blk.samples, float32(s))
		}
		select {
		case ch.send <- blk:
		default:
			p.releaseBlock(blk)
			p.dropsTotal.Add(1)
		}
	}
}

func (p *pipeline) acquireBlock() *captureBlock {
	select {
	case b := <-p.blockPool:
		return b
	default:
		return nil
	}
}

func (p *pipeline) releaseBlock(b *captureBlock) {
	if b == nil {
		return
	}
	b.samples = b.samples[:0]
	select {
	case p.blockPool <- b:
	default:
		// Pool full; let GC reclaim.
	}
}

// runChannelWorker drains a channel's queue and writes samples through
// the streamWavWriter. Exits when the channel is closed.
func (p *pipeline) runChannelWorker(ch *channelState) {
	defer p.workerWg.Done()
	for blk := range ch.send {
		if blk == nil {
			continue
		}
		if err := ch.writer.WriteSamplesFloat32(blk.samples); err != nil {
			log.Printf("[REC] %s write: %v", ch.id, err)
		} else {
			ch.count.Add(int64(len(blk.samples)))
		}
		p.releaseBlock(blk)
	}
}

// Detach is the fast first half of Stop: marks the pipeline as stopping
// and waits for any in-flight Tap calls to return. Safe to call from
// the UI thread — completes in microseconds because Tap's body is
// already lock-free and bounded.
//
// Returns nil if successful, or an error if Detach was already called.
// After Detach returns, Tap is a no-op; the caller may safely call
// DrainAndClose on a worker goroutine.
func (p *pipeline) Detach() error {
	if p.stopping.Swap(true) {
		return errors.New("pipeline already stopped")
	}
	// WaitGroup-based barrier replaces the old time.Sleep polling loop.
	// In-flight Taps that observed stopping=false will Done() before
	// Wait returns; new Taps see stopping=true and bail before Add().
	p.tapWG.Wait()
	return nil
}

// DrainWorkers closes the per-channel queues and waits for encoder
// workers to finish writing any buffered blocks. After this returns,
// per-channel `count.Load()` values are final and pipe.Channels()
// reflects actual recorded data. The WAV writers are still open — call
// CloseWriters next to finalize the files.
//
// In steady state this completes in microseconds (workers keep up with
// the audio thread; queues are shallow). Worst case: deep queue × N
// channels × write cost — bounded by the per-channel queue depth so a
// pathological case is still O(seconds), not O(unbounded).
//
// Safe to call from the UI thread: only synchronization, no file I/O.
func (p *pipeline) DrainWorkers() error {
	if !p.stopping.Load() {
		return errors.New("pipeline not detached; call Detach first")
	}
	if p.masterCh != nil {
		close(p.masterCh.send)
	}
	for _, ch := range p.channels {
		close(ch.send)
	}
	p.workerWg.Wait()
	return nil
}

// CloseWriters is the slow tail: bufio.Flush + Seek + Write to patch
// header sizes + file Close per channel. This is what we want OFF the
// UI thread on slow disks — 7 channels × ~5 syscalls each can spike.
//
// Returns the per-channel ChannelMeta with finalized sample counts and
// any errors from the close cycle joined together.
func (p *pipeline) CloseWriters() ([]ChannelMeta, error) {

	// Close writers (flushes bufio + patches WAV header sizes). Only emit
	// metadata for channels that actually received samples — empty writers
	// remain on disk as silent stubs but are not advertised in the result.
	var errs []error
	var metas []ChannelMeta
	if p.masterCh != nil {
		if err := p.masterCh.writer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close master: %w", err))
		}
		if cnt := p.masterCh.count.Load(); cnt > 0 {
			metas = append(metas, ChannelMeta{
				ID:       p.masterCh.id,
				Name:     p.masterCh.name,
				Filename: filepath.Base(p.masterCh.path),
				Samples:  int(cnt),
			})
		}
	}
	for _, ch := range p.channels {
		if err := ch.writer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close %s: %w", ch.id, err))
			continue
		}
		if cnt := ch.count.Load(); cnt > 0 {
			metas = append(metas, ChannelMeta{
				ID:       ch.id,
				Name:     ch.name,
				Filename: filepath.Base(ch.path),
				Samples:  int(cnt),
			})
		}
	}
	return metas, errors.Join(errs...)
}

// Stop is the convenience wrapper: Detach + DrainWorkers + CloseWriters.
// Retained for callers (and tests) that want the simple synchronous
// flow. Production code paths should call the three-phase form so
// CloseWriters runs on a background worker.
func (p *pipeline) Stop() ([]ChannelMeta, error) {
	if err := p.Detach(); err != nil {
		return nil, err
	}
	if err := p.DrainWorkers(); err != nil {
		return nil, err
	}
	return p.CloseWriters()
}

// Drops returns the total number of dropped blocks across all channels.
func (p *pipeline) Drops() int64 {
	return p.dropsTotal.Load()
}

// MetaSnapshot returns ChannelMeta entries for all channels that have
// received samples, using the current atomic counters. Safe to call any
// time after DrainWorkers (which guarantees counts are stable). Used to
// populate result.Metadata.Channels synchronously while the WAV writer
// Close is still pending on the lifecycle pool.
func (p *pipeline) MetaSnapshot() []ChannelMeta {
	out := make([]ChannelMeta, 0, len(p.channels)+1)
	if p.masterCh != nil {
		if cnt := p.masterCh.count.Load(); cnt > 0 {
			out = append(out, ChannelMeta{
				ID:       p.masterCh.id,
				Name:     p.masterCh.name,
				Filename: filepath.Base(p.masterCh.path),
				Samples:  int(cnt),
			})
		}
	}
	for _, ch := range p.channels {
		if cnt := ch.count.Load(); cnt > 0 {
			out = append(out, ChannelMeta{
				ID:       ch.id,
				Name:     ch.name,
				Filename: filepath.Base(ch.path),
				Samples:  int(cnt),
			})
		}
	}
	return out
}

// Channels returns the per-channel writer paths and sample counts. Used
// for building EncodedChannel results without re-reading file contents.
func (p *pipeline) Channels() []EncodedChannel {
	out := make([]EncodedChannel, 0, len(p.channels)+1)
	if p.masterCh != nil && p.masterCh.count.Load() > 0 {
		out = append(out, EncodedChannel{
			ID:       p.masterCh.id,
			Name:     p.masterCh.name,
			Filename: filepath.Base(p.masterCh.path),
			Path:     p.masterCh.path,
		})
	}
	for _, ch := range p.channels {
		if ch.count.Load() == 0 {
			continue
		}
		out = append(out, EncodedChannel{
			ID:       ch.id,
			Name:     ch.name,
			Filename: filepath.Base(ch.path),
			Path:     ch.path,
		})
	}
	return out
}

// maxMIDIEventsPerSession bounds the per-session MIDI events buffer. Recording
// is already 1800 s capped via pipelineConfig.MaxDuration; ~110 ev/s is well
// above any human-realistic drumming density. The cap exists to guard against
// runaway accumulation if the session somehow generates events at an
// unbounded rate.
const maxMIDIEventsPerSession = 200_000

// midiDropChunk amortizes the cost of dropping over-cap events.
const midiDropChunk = 1024

// AppendMIDIEvent records a MIDI event captured during the session. Safe
// to call from any goroutine. Drops the oldest midiDropChunk events if the
// hard cap is reached so callers never block and the buffer never grows
// without bound.
func (p *pipeline) AppendMIDIEvent(evt MIDIEvent) {
	p.midiMu.Lock()
	if len(p.midiEvents) >= maxMIDIEventsPerSession {
		// Drop oldest in chunks to amortize the slice copy.
		drop := midiDropChunk
		if drop > len(p.midiEvents) {
			drop = len(p.midiEvents)
		}
		p.midiEvents = append(p.midiEvents[:0], p.midiEvents[drop:]...)
	}
	p.midiEvents = append(p.midiEvents, evt)
	p.midiMu.Unlock()
}

// MIDIEvents returns a copy of all captured MIDI events.
func (p *pipeline) MIDIEvents() []MIDIEvent {
	p.midiMu.Lock()
	out := make([]MIDIEvent, len(p.midiEvents))
	copy(out, p.midiEvents)
	p.midiMu.Unlock()
	return out
}

// IsAutoStopped reports whether the pipeline reached its max-duration cap.
func (p *pipeline) IsAutoStopped() bool {
	return p.done.Load()
}

// Elapsed returns the time since the pipeline started.
func (p *pipeline) Elapsed() time.Duration {
	return time.Since(p.startTime)
}

// PipelineStats reports recording pipeline counters for perf snapshots.
type PipelineStats struct {
	Active        bool
	Drops         int64
	MasterQueued  int
	BlockPoolFree int
	Channels      int
	// BytesUsed is the total encoded payload across all channels. On
	// desktop this is updated by the streaming WAV writers; on WASM it
	// is reported by the off-thread encoder Worker. Useful as a hard cap
	// signal for very long sessions.
	BytesUsed int64
}

// CurrentPipelineStats returns a snapshot of the active pipeline, if any.
func CurrentPipelineStats() PipelineStats {
	p := pipelinePtr.Load()
	if p == nil {
		return PipelineStats{}
	}
	stats := PipelineStats{
		Active:        true,
		Drops:         p.dropsTotal.Load(),
		BlockPoolFree: len(p.blockPool),
		Channels:      len(p.channels),
	}
	if p.masterCh != nil {
		stats.MasterQueued = len(p.masterCh.send)
	}
	// Approximate bytes used = samples × bytesPerSample. Cheap to derive
	// here without adding another atomic counter to the hot path.
	for _, ch := range p.channels {
		bps := 0
		if ch.writer != nil {
			bps = ch.writer.bps / 8
		}
		stats.BytesUsed += ch.count.Load() * int64(bps)
	}
	if p.masterCh != nil {
		bps := 0
		if p.masterCh.writer != nil {
			bps = p.masterCh.writer.bps / 8
		}
		stats.BytesUsed += p.masterCh.count.Load() * int64(bps)
	}
	return stats
}

// RecordingDrops returns the cumulative number of capture blocks dropped
// due to backpressure or pool exhaustion since the current session began.
// Returns 0 when not recording.
func RecordingDrops() int64 {
	p := pipelinePtr.Load()
	if p == nil {
		return 0
	}
	return p.dropsTotal.Load()
}

func isStreamableFormat(f AudioFormat) bool {
	switch f {
	case FormatWAV16, FormatWAV24, FormatWAV32:
		return true
	}
	return false
}
