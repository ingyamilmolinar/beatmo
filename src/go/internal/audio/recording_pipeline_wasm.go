//go:build js && wasm

package audio

// PipelineStats / CurrentPipelineStats expose recording observability
// counters to the perf snapshot path. On WASM the streaming pipeline
// itself runs in a Web Worker, so we proxy the worker's most recent
// stats through here. The shared snapshot is cached by JS and refreshed
// by the stats poller (see startStatsPoller) — this call is non-blocking.

// PipelineStats reports recording pipeline counters for perf snapshots.
type PipelineStats struct {
	Active        bool
	Drops         int64
	MasterQueued  int
	BlockPoolFree int
	Channels      int
	BytesUsed     int64
}

// CurrentPipelineStats returns the latest snapshot from the encoder
// worker. Reads a JS-side cached value that the worker refreshes via
// stats requests every ~500 ms.
func CurrentPipelineStats() PipelineStats {
	s := platformRecordingStatsSnapshot()
	if !s.Active {
		return PipelineStats{}
	}
	return PipelineStats{
		Active:       true,
		Drops:        s.DroppedSamples,
		MasterQueued: s.QueuedBatches,
		// The worker has no concept of a "block pool"; report unused
		// outbox slots instead so the existing recordingPoolFree counter
		// has a sensible value (free = max - currently queued).
		BlockPoolFree: maxOutboxDepth() - s.QueuedBatches,
		Channels:      s.ActiveChannels,
		BytesUsed:     s.BytesUsed,
	}
}

// maxOutboxDepth mirrors the worklet's DEFAULT_MAX_PENDING (8). Surfaced
// here so PipelineStats.BlockPoolFree is meaningful (free = max - in-flight).
func maxOutboxDepth() int { return 8 }

// RecordingDrops returns the current drop count from the worker.
func RecordingDrops() int64 {
	return platformRecordingStatsSnapshot().DroppedSamples
}
