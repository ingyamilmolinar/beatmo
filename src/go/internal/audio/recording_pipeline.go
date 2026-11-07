//go:build !js

package audio

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// Lock-free and allocation-free on the audio thread: Tap copies into a pooled block and sends without blocking; workers
// stream to disk. Stop clears pipelinePtr first, then waits out in-flight Taps, then closes channels and writers.
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
	// A deterministic, non-polling barrier for Detach.
	tapWG sync.WaitGroup

	// The audio thread acquires without blocking; workers return blocks after writing.
	blockPool chan *captureBlock

	// Per-channel state, indexed by instrument ID. Read-only after start.
	channels map[channel.ID]*channelState
	masterCh *channelState

	// Worker shutdown.
	workerWg sync.WaitGroup

	// Counters surfaced via Stats.
	dropsTotal atomic.Int64

	// MIDI events captured during recording.
	midiMu     sync.Mutex
	midiEvents []MIDIEvent
}

// Fixed capacity; length varies per use.
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

// nil = not recording.
var pipelinePtr atomic.Pointer[pipeline]

// Not yet on the hot path: the caller must Activate it.
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
	// Only WAV streams; other formats encode in memory at stop.
	if !isStreamableFormat(cfg.Format) {
		return nil, fmt.Errorf("format %s is not streamable", cfg.Format)
	}

	p := &pipeline{
		outputDir:  cfg.OutputDir,
		sampleRate: cfg.SampleRate,
		format:     cfg.Format,
		startTime:  time.Now(),
		blockPool:  make(chan *captureBlock, cfg.PoolSize),
		channels:   make(map[channel.ID]*channelState, len(cfg.Instruments)+1),
	}
	if cfg.MaxDuration > 0 {
		p.maxSamples = int64(cfg.MaxDuration.Seconds() * float64(cfg.SampleRate))
	}

	// Pre-fill the block pool.
	for i := 0; i < cfg.PoolSize; i++ {
		p.blockPool <- &captureBlock{samples: make([]float32, 0, cfg.BlockCapacity)}
	}

	// Open writers + spawn workers for master and each instrument.
	masterPath := filepath.Join(cfg.OutputDir, RecordingMasterID+ext)
	masterWriter, err := newStreamWavWriter(masterPath, cfg.Format, cfg.SampleRate)
	if err != nil {
		return nil, fmt.Errorf("open master writer: %w", err)
	}
	p.masterCh = &channelState{
		id:     RecordingMasterID,
		name:   "Master",
		send:   make(chan *captureBlock, cfg.QueueDepth),
		writer: masterWriter,
		path:   masterPath,
	}
	p.workerWg.Add(1)
	go p.runChannelWorker(p.masterCh)

	for _, inst := range cfg.Instruments {
		path := filepath.Join(cfg.OutputDir, inst.ID.String()+ext)
		w, err := newStreamWavWriter(path, cfg.Format, cfg.SampleRate)
		if err != nil {
			// Best-effort cleanup of writers we already opened.
			p.shutdownEarly()
			return nil, fmt.Errorf("open writer %s: %w", inst.ID, err)
		}
		ch := &channelState{
			id:     inst.ID.String(),
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

func (p *pipeline) shutdownEarly() {
	if p.masterCh != nil {
		close(p.masterCh.send)
	}
	for _, ch := range p.channels {
		close(ch.send)
	}
	p.workerWg.Wait()
}

// On the audio thread: MUST NOT block, allocate or take a mutex.
func (p *pipeline) Tap(slotIDs []channel.ID, activeSlots []int, instBufs [][]float64, workBuf []float64, blockLen int) {
	if p.stopping.Load() || p.done.Load() {
		return
	}
	p.tapWG.Add(1)
	defer p.tapWG.Done()
	// Re-checked after Add so Detach's Wait sees this Tap (Add/Wait happens-before).
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

func (p *pipeline) runChannelWorker(ch *channelState) {
	defer p.workerWg.Done()
	for blk := range ch.send {
		if blk == nil {
			continue
		}
		if err := ch.writer.WriteSamplesFloat32(blk.samples); err != nil {
			log.Errorf("[record] %s write: %v", ch.id, err)
		} else {
			ch.count.Add(int64(len(blk.samples)))
		}
		p.releaseBlock(blk)
	}
}

// Safe on the UI thread: microseconds, since Tap is lock-free and bounded. Afterwards Tap is a no-op.
func (p *pipeline) Detach() error {
	if p.stopping.Swap(true) {
		return errors.New("pipeline already stopped")
	}
	// Taps that saw stopping=false finish before Wait returns; new ones bail before Add.
	p.tapWG.Wait()
	return nil
}

// After this, sample counts are final; the writers are still open. Only synchronization, no file I/O, so safe on the
// UI thread; bounded by the per-channel queue depth.
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

// The slow tail (flush, header patch, Close per channel), kept OFF the UI thread for slow disks.
func (p *pipeline) CloseWriters() ([]ChannelMeta, error) {

	// Empty writers stay on disk as silent stubs but are not advertised.
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

// Synchronous convenience; production runs the three phases so CloseWriters lands on a background worker.
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

// Stable after DrainWorkers, so the result can be filled while Close is still pending.
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

// Without re-reading file contents.
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

// Recording is already capped at 1800 s; ~110 events/s is beyond any drumming, so this only stops runaway accumulation.
const maxMIDIEventsPerSession = 200_000

// midiDropChunk amortizes the cost of dropping over-cap events.
const midiDropChunk = 1024

// Drops the oldest midiDropChunk events at the cap, so callers never block.
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
	// The streaming writers on desktop, the encoder Worker on WASM: a hard-cap signal for long sessions.
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
	// Samples × bytes per sample: no extra hot-path counter.
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

// Backpressure and pool exhaustion this session; 0 when not recording.
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
