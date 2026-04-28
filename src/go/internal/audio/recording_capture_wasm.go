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
)

// recordingDownloadHandle is what we hand to SaveRecording so it can
// trigger the browser download. The Blob lives entirely in JS; on the Go
// side we hold only the object URL (string) and the filename so the
// triggerZipDownload helper can build an <a download> element. Stored in
// an atomic pointer because StopRecording (writer) and SaveRecording
// (reader) are decoupled — multiple goroutines may call SaveRecording
// after a stop.
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

// recordingFinalizeResult is what the JS encoder Worker hands back when
// finalize completes. The Blob ref is encoded as a Object URL (string)
// to keep cross-thread ownership simple from Go.
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

// recordingWorkerStats is the snapshot the worker publishes for perf
// observability. Read by CurrentPipelineStats() on demand.
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

// awaitJSPromise blocks the calling goroutine until the JS Promise
// resolves or rejects. The Go-WASM scheduler yields to the JS event loop
// while we wait on the channel, so the UI does not freeze.
//
// Note: this MUST NOT be called from inside a JS callback (e.g., from
// js.FuncOf), because the JS event loop is synchronously executing that
// callback and cannot dispatch the resolve. Always call from an ordinary
// goroutine.
func awaitJSPromise(p js.Value) (js.Value, error) {
	return awaitJSPromiseCtx(context.Background(), p)
}

// awaitJSPromiseCtx is the context-aware variant of awaitJSPromise. If
// ctx is cancelled (e.g., a timeout fires) before the Promise settles,
// returns ctx.Err(). The js.Funcs are intentionally NOT released on
// ctx-cancel because the JS Promise may still call them later — releasing
// would cause a use-after-release. We accept a small one-time memory
// cost per cancelled await (≈two js.Funcs + a buffered channel slot;
// well under 1 KB) in exchange for memory safety. Cancellation is rare
// (only on a wedged JS peer) and bounded by the number of recording
// sessions per process lifetime.
//
// Use this for any await that could legitimately hang on a wedged JS
// peer (e.g., the encoder Worker on finalize). The pure awaitJSPromise
// is fine for awaits that are guaranteed to settle quickly (e.g.,
// startMultiChannelCapture readiness handshake).
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

// platformStartCapture invokes window.startMultiChannelCapture and waits
// for the JS pipeline (capture worklet + encoder worker) to be wired
// before returning. Blocks the calling goroutine but yields to the JS
// event loop while the Promise is pending.
func platformStartCapture(opts RecordingOptions) error {
	fn := js.Global().Get("startMultiChannelCapture")
	if !fn.Truthy() {
		return errors.New("startMultiChannelCapture is not available")
	}

	ids := js.Global().Get("Array").New(len(opts.Instruments))
	for i, inst := range opts.Instruments {
		ids.SetIndex(i, js.ValueOf(inst.ID))
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

// finalizeCaptureTimeout caps how long the Go side will wait for the
// encoder Worker to hand back the final Blob. The JS side has its own
// shorter timeout (audio.js stopMultiChannelCapture); this is a
// belt-and-suspenders upper bound that prevents the lifecycle pool
// worker from being held hostage indefinitely if the JS-side timeout
// itself fails to fire (page suspended, worker wedged, etc). 60s is
// well above the worst-case encode time at the 30-min hard cap (~10s)
// while still being short enough that a hung worker frees the pool
// worker for the next session.
const finalizeCaptureTimeout = 60 * time.Second

// platformFinalizeCapture invokes window.stopMultiChannelCapture and
// waits for the encoder worker to emit the final Blob. The result holds
// a Blob object URL the caller can hand to triggerZipDownload. Bounded
// by finalizeCaptureTimeout so a wedged worker cannot leak the
// lifecycle-pool worker.
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

// platformRecordingStatsSnapshot polls the worker's most recent stats.
// Non-blocking — uses the cache the JS layer maintains via periodic
// stats requests. Called from CurrentPipelineStats() in the perf snapshot
// path; cheap enough to call every frame.
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

// platformRecordingRequestStats nudges the worker to publish a fresh
// stats snapshot. The Go side calls this every second or so via the
// background scheduler so the cached snapshot stays current. The reply
// arrives asynchronously and updates the JS-side cache.
func platformRecordingRequestStats() {
	fn := js.Global().Get("recordingRequestStats")
	if fn.Truthy() {
		fn.Invoke()
	}
}

// statsPollInterval is the cadence at which the Go side nudges the
// worker for a fresh stats snapshot. Reads are cheap (one js.Invoke);
// the worker reply runs on the worker thread and updates the JS-side
// cache asynchronously.
const statsPollInterval = 500 * time.Millisecond

// statsPoller drives the periodic platformRecordingRequestStats nudge
// via async.Scheduler so we don't hold a long-lived ticker goroutine.
// One Scheduler instance owns one timer goroutine regardless of how
// many entries are pending; each fire re-arms the next deadline. This
// matches the canonical async API documented in CLAUDE.md ("Use instead
// of go func() { time.Sleep(d); fn() } whenever pending count could
// grow with workload").
type statsPoller struct {
	sched  *async.Scheduler
	mu     sync.Mutex // guards cancel; closed is atomic for fast-path checks
	cancel func()
	closed atomic.Bool
}

// startStatsPoller boots a Scheduler-backed poller that nudges the
// encoder Worker every statsPollInterval. Backed by lifecyclePool();
// per-session there is at most one in-flight nudge plus one finalize,
// so the single worker handles both with no contention. The poller is
// torn down by stop().
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
		// Raced with stop(): cancel the entry we just scheduled so it
		// doesn't fire. The job is also closed-guarded, but cancelling
		// avoids one wasted pool dispatch.
		p.mu.Unlock()
		cancel()
		return
	}
	p.cancel = cancel
	p.mu.Unlock()
}

// stop halts the poller. Idempotent: safe to call multiple times.
// After stop returns, no new nudges will fire (the in-flight job, if
// any, is closed-guarded inside its callback).
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

// init wires the legacy platformRecordingStop variable so any existing
// callers (none in production WASM, but defensive against test stubs)
// receive empty results. The new path uses platformStartCapture and
// platformFinalizeCapture directly.
func init() {
	platformRecordingStart = func(_ []InstrumentMeta) {
		// Intentional no-op: the new platformStartCapture is invoked
		// directly from StartRecording with full options. Kept here only
		// so the package-level variable's signature stays stable for
		// non-WASM callers that may set their own stub.
	}
	platformRecordingStop = func() (map[string][]float64, []float64, int) {
		// Intentional no-op: the new platformFinalizeCapture path replaces
		// this. Returning empty preserves the old contract.
		return nil, nil, 0
	}
}
