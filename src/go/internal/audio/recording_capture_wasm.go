//go:build js && wasm

package audio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"syscall/js"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/async"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// The Blob lives in JS; Go holds only its object URL and filename. Atomic because SaveRecording may run on several
// goroutines after a stop.
type recordingDownloadHandle struct {
	BlobURL  string
	Filename string
	Size     int
}

var pendingDownload atomic.Pointer[recordingDownloadHandle]

// recordingChannelResult is one entry in finalizeResult.Channels.
type recordingChannelResult struct {
	ID             string
	Name           string
	Filename       string
	Samples        int
	DroppedSamples int
	Bytes          int
}

// The Blob crosses as an object URL to keep ownership simple.
type recordingFinalizeResult struct {
	BlobURL     string
	Filename    string
	Size        int
	SampleRate  int
	AutoStopped bool
	AutoStopRsn string
	Channels    []recordingChannelResult
	Stats       recordingWorkerStats
}

type recordingWorkerStats struct {
	Active         bool
	DroppedSamples int64
	BytesUsed      int64
	QueuedBatches  int
	MaxQueueDepth  int
	ElapsedSec     float64
	ActiveChannels int
	AutoStopped    bool
	AutoStopReason string
}

// MUST NOT be called from a JS callback: the event loop is busy running it and cannot dispatch the resolve.
func awaitJSPromise(p js.Value) (js.Value, error) {
	return awaitJSPromiseCtx(context.Background(), p)
}

// For awaits that could hang on a wedged JS peer. The js.Funcs are NOT released on cancel because the Promise may still
// call them: a small leak per cancelled await beats a use-after-release.
func awaitJSPromiseCtx(ctx context.Context, p js.Value) (js.Value, error) {
	if !p.Truthy() {
		return js.Undefined(), errors.New("nil promise")
	}
	type result struct {
		val js.Value
		err error
	}
	ch := make(chan result, 1)
	var resolve, reject js.Func
	resolve = js.FuncOf(func(this js.Value, args []js.Value) any {
		var v js.Value
		if len(args) > 0 {
			v = args[0]
		}
		ch <- result{val: v}
		return nil
	})
	reject = js.FuncOf(func(this js.Value, args []js.Value) any {
		msg := "promise rejected"
		if len(args) > 0 {
			if s := args[0].Get("message"); s.Type() == js.TypeString {
				msg = s.String()
			} else if args[0].Type() == js.TypeString {
				msg = args[0].String()
			}
		}
		ch <- result{err: errors.New(msg)}
		return nil
	})
	p.Call("then", resolve).Call("catch", reject)

	select {
	case r := <-ch:
		resolve.Release()
		reject.Release()
		return r.val, r.err
	case <-ctx.Done():
		// Funcs deliberately not released — see comment above.
		return js.Undefined(), ctx.Err()
	}
}

// Yields to the JS event loop while the Promise is pending.
func platformStartCapture(opts RecordingOptions) error {
	fn := js.Global().Get("startMultiChannelCapture")
	if !fn.Truthy() {
		return errors.New("startMultiChannelCapture is not available")
	}

	ids := js.Global().Get("Array").New(len(opts.Instruments))
	for i, inst := range opts.Instruments {
		ids.SetIndex(i, js.ValueOf(inst.ID.String()))
	}

	jsOpts := map[string]any{
		"format": string(opts.Format),
	}
	if opts.MaxDuration > 0 {
		jsOpts["durationMaxSec"] = opts.MaxDuration.Seconds()
	}
	optsBytes, _ := json.Marshal(jsOpts)

	v, err := awaitJSPromise(fn.Invoke(ids, string(optsBytes)))
	if err != nil {
		return fmt.Errorf("startMultiChannelCapture: %w", err)
	}
	if v.Get("ok").Type() != js.TypeBoolean || !v.Get("ok").Bool() {
		errMsg := "unknown error"
		if e := v.Get("error"); e.Type() == js.TypeString {
			errMsg = e.String()
		}
		return errors.New(errMsg)
	}
	return nil
}

// Belt and braces over the JS timeout (a suspended page, a wedged worker): far above the worst encode at the 30-min
// cap, short enough to free the pool worker for the next session.
const finalizeCaptureTimeout = 60 * time.Second

// Bounded by finalizeCaptureTimeout so a wedged worker cannot hold the lifecycle pool.
func platformFinalizeCapture(meta map[string]any) (*recordingFinalizeResult, error) {
	fn := js.Global().Get("stopMultiChannelCapture")
	if !fn.Truthy() {
		return nil, errors.New("stopMultiChannelCapture is not available")
	}
	metaBytes, _ := json.Marshal(meta)
	ctx, cancel := context.WithTimeout(context.Background(), finalizeCaptureTimeout)
	defer cancel()
	v, err := awaitJSPromiseCtx(ctx, fn.Invoke(string(metaBytes)))
	if err != nil {
		return nil, fmt.Errorf("stopMultiChannelCapture: %w", err)
	}
	if e := v.Get("error"); e.Type() == js.TypeString && e.String() != "" {
		return nil, errors.New(e.String())
	}

	res := &recordingFinalizeResult{
		BlobURL:     v.Get("blobURL").String(),
		Filename:    v.Get("filename").String(),
		Size:        v.Get("size").Int(),
		SampleRate:  v.Get("sampleRate").Int(),
		AutoStopped: v.Get("autoStopped").Bool(),
	}
	if c := v.Get("channels"); c.Truthy() && c.Length() > 0 {
		res.Channels = make([]recordingChannelResult, c.Length())
		for i := 0; i < c.Length(); i++ {
			ch := c.Index(i)
			res.Channels[i] = recordingChannelResult{
				ID:             ch.Get("id").String(),
				Name:           ch.Get("name").String(),
				Filename:       ch.Get("filename").String(),
				Samples:        ch.Get("samples").Int(),
				DroppedSamples: ch.Get("droppedSamples").Int(),
				Bytes:          ch.Get("bytes").Int(),
			}
		}
	}
	if s := v.Get("stats"); s.Truthy() {
		res.Stats = readWorkerStats(s)
	}
	return res, nil
}

// readWorkerStats marshals the JS stats object into recordingWorkerStats.
func readWorkerStats(s js.Value) recordingWorkerStats {
	out := recordingWorkerStats{Active: true}
	if v := s.Get("droppedSamples"); v.Truthy() {
		out.DroppedSamples = int64(v.Int())
	}
	if v := s.Get("bytesUsed"); v.Truthy() {
		out.BytesUsed = int64(v.Int())
	}
	if v := s.Get("queuedBatches"); v.Truthy() {
		out.QueuedBatches = v.Int()
	}
	if v := s.Get("maxQueueDepth"); v.Truthy() {
		out.MaxQueueDepth = v.Int()
	}
	if v := s.Get("elapsedSec"); v.Truthy() {
		out.ElapsedSec = v.Float()
	}
	if v := s.Get("activeChannels"); v.Truthy() {
		out.ActiveChannels = v.Int()
	}
	return out
}

// Non-blocking: reads the cache JS maintains, cheap enough every frame.
func platformRecordingStatsSnapshot() recordingWorkerStats {
	fn := js.Global().Get("recordingStatsSnapshot")
	if !fn.Truthy() {
		return recordingWorkerStats{}
	}
	v := fn.Invoke()
	if !v.Truthy() {
		return recordingWorkerStats{}
	}
	out := recordingWorkerStats{}
	if v.Get("active").Bool() {
		out.Active = true
	}
	if x := v.Get("droppedSamples"); x.Truthy() {
		out.DroppedSamples = int64(x.Int())
	}
	if x := v.Get("bytesUsed"); x.Truthy() {
		out.BytesUsed = int64(x.Int())
	}
	if x := v.Get("queuedBatches"); x.Truthy() {
		out.QueuedBatches = x.Int()
	}
	if x := v.Get("maxQueueDepth"); x.Truthy() {
		out.MaxQueueDepth = x.Int()
	}
	if x := v.Get("elapsedSec"); x.Truthy() {
		out.ElapsedSec = x.Float()
	}
	if x := v.Get("activeChannels"); x.Truthy() {
		out.ActiveChannels = x.Int()
	}
	if x := v.Get("autoStopped"); x.Truthy() {
		out.AutoStopped = x.Bool()
	}
	if x := v.Get("autoStopReason"); x.Truthy() && x.Type() == js.TypeString {
		out.AutoStopReason = x.String()
	}
	return out
}

// The reply arrives asynchronously and updates the JS-side cache.
func platformRecordingRequestStats() {
	fn := js.Global().Get("recordingRequestStats")
	if fn.Truthy() {
		fn.Invoke()
	}
}

const statsPollInterval = 500 * time.Millisecond

// An async.Scheduler entry rather than a ticker goroutine.
type statsPoller struct {
	sched  *async.Scheduler
	mu     sync.Mutex // guards cancel; closed is atomic for fast-path checks
	cancel func()
	closed atomic.Bool
}

// At most one nudge and one finalize per session, so the single lifecycle worker handles both.
func startStatsPoller() *statsPoller {
	p := &statsPoller{sched: async.NewScheduler(lifecyclePool())}
	p.arm()
	return p
}

func (p *statsPoller) arm() {
	if p.closed.Load() {
		return
	}
	cancel, err := p.sched.Schedule(time.Now().Add(statsPollInterval), func(_ context.Context) {
		if p.closed.Load() {
			return
		}
		platformRecordingRequestStats()
		p.arm()
	})
	if err != nil {
		return
	}
	p.mu.Lock()
	if p.closed.Load() {
		// Raced with stop(): the job is closed-guarded anyway, but cancelling saves a dispatch.
		p.mu.Unlock()
		cancel()
		return
	}
	p.cancel = cancel
	p.mu.Unlock()
}

// Idempotent; an in-flight job is closed-guarded.
func (p *statsPoller) stop() {
	if !p.closed.CompareAndSwap(false, true) {
		return
	}
	p.mu.Lock()
	cancel := p.cancel
	p.cancel = nil
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	_ = p.sched.Close()
}

// The browser path calls platformStartCapture / platformFinalizeCapture directly; these keep the variables' contract
// for other callers.
func init() {
	platformRecordingStart = func(_ []InstrumentMeta) {
	}
	platformRecordingStop = func() (map[channel.ID][]float64, []float64, int) {
		return nil, nil, 0
	}
}
