package bench

import "testing"

type stubCounters struct {
	drops  int64
	active bool
}

func (s stubCounters) RecordingDrops() int64 { return s.drops }
func (s stubCounters) IsRecording() bool     { return s.active }

func TestRuntimeStatsSnapshotKeys(t *testing.T) {
	stats := RuntimeStatsSnapshot(stubCounters{drops: 7, active: true})
	for _, key := range []string{
		"goroutines", "heap_alloc_kb", "heap_sys_kb", "heap_objects",
		"recording_drops", "recording_active",
	} {
		if _, ok := stats[key]; !ok {
			t.Errorf("snapshot missing key %q (got %#v)", key, stats)
		}
	}
}

func TestRuntimeStatsSnapshotAudioPassThrough(t *testing.T) {
	stats := RuntimeStatsSnapshot(stubCounters{drops: 42, active: true})
	if got, want := stats["recording_drops"], int64(42); got != want {
		t.Errorf("recording_drops=%v, want %v", got, want)
	}
	if got, want := stats["recording_active"], true; got != want {
		t.Errorf("recording_active=%v, want %v", got, want)
	}
}

func TestRuntimeStatsSnapshotNilAudio(t *testing.T) {
	stats := RuntimeStatsSnapshot(nil)
	if _, ok := stats["recording_drops"]; ok {
		t.Errorf("nil audio should omit recording_drops, got %#v", stats)
	}
	if _, ok := stats["recording_active"]; ok {
		t.Errorf("nil audio should omit recording_active, got %#v", stats)
	}
	// Non-audio keys still present.
	if _, ok := stats["goroutines"]; !ok {
		t.Errorf("nil audio should still emit runtime keys, got %#v", stats)
	}
}

func TestRuntimeStatsSnapshotMonotonic(t *testing.T) {
	stats := RuntimeStatsSnapshot(stubCounters{})
	if g, ok := stats["goroutines"].(int); !ok || g <= 0 {
		t.Errorf("goroutines=%v should be a positive int", stats["goroutines"])
	}
	if h, ok := stats["heap_alloc_kb"].(uint64); !ok {
		t.Errorf("heap_alloc_kb should be uint64, got %T", stats["heap_alloc_kb"])
	} else if h > 1<<40 {
		t.Errorf("heap_alloc_kb=%d implausibly large", h)
	}
}
