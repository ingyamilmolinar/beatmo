//go:build js && wasm

package audio

// The pipeline runs in a Web Worker; its stats arrive through a JS cache the poller refreshes, so reads never block.

// PipelineStats reports recording pipeline counters for perf snapshots.
type PipelineStats struct {
	Active        bool
	Drops         int64
	MasterQueued  int
	BlockPoolFree int
	Channels      int
	BytesUsed     int64
}

func CurrentPipelineStats() PipelineStats {
	s := platformRecordingStatsSnapshot()
	if !s.Active {
		return PipelineStats{}
	}
	return PipelineStats{
		Active:       true,
		Drops:        s.DroppedSamples,
		MasterQueued: s.QueuedBatches,
		// The worker has no block pool; free outbox slots fill the same counter.
		BlockPoolFree: maxOutboxDepth() - s.QueuedBatches,
		Channels:      s.ActiveChannels,
		BytesUsed:     s.BytesUsed,
	}
}

// Mirrors the worklet's DEFAULT_MAX_PENDING.
func maxOutboxDepth() int { return 8 }

// RecordingDrops returns the current drop count from the worker.
func RecordingDrops() int64 {
	return platformRecordingStatsSnapshot().DroppedSamples
}
