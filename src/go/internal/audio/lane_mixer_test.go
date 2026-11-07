//go:build !test && !js

package audio

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// Never attached to a device: tests pull samples through Read.
func laneTestMixer() *mixer {
	return &mixer{
		workBuf:   make([]float64, blockSize),
		voiceTemp: make([]float64, blockSize),
		masterBuf: make([]float64, blockSize),
		postFXBuf: make([]float64, blockSize),
		postEQBuf: make([]float64, blockSize),
		instSlots: make(map[channel.ID]int),
	}
}

func (m *mixer) renderForTest(samples int) []byte {
	p := make([]byte, samples*2)
	_, _ = m.Read(p)
	return p
}

func (m *mixer) hasVoiceForTest(vid uint64) bool {
	m.pendingMu.Lock()
	defer m.pendingMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, vs := range m.pendingAdd {
		if vs.vid == vid {
			return true
		}
	}
	for _, vs := range m.voices {
		if vs.vid == vid {
			return true
		}
	}
	return false
}

func peakOf(p []byte) int {
	pk := 0
	for i := 0; i+1 < len(p); i += 2 {
		v := int(int16(uint16(p[i]) | uint16(p[i+1])<<8))
		if v < 0 {
			v = -v
		}
		if v > pk {
			pk = v
		}
	}
	return pk
}

// Removed outright, not RequestStop'd, which leaves a faded ghost when its start arrives.
func TestMixerCancelBeforeStartIsSilent(t *testing.T) {
	m := laneTestMixer()
	m.ScheduleLane(channel.Of("lanetest"), newTestSineVoice(), 4096, -1, -1, 7)
	m.cancelVoices([]uint64{7})
	if pk := peakOf(m.renderForTest(16384)); pk != 0 {
		t.Fatalf("cancelled voice rendered (peak %d)", pk)
	}
}

// Edit semantics (a): a voice that already started is never cut by a cancel.
func TestMixerCancelAfterStartDoesNothing(t *testing.T) {
	m := laneTestMixer()
	m.ScheduleLane(channel.Of("lanetest"), newTestSineVoice(), 0, -1, -1, 9)
	m.renderForTest(1024)
	m.cancelVoices([]uint64{9})
	if pk := peakOf(m.renderForTest(1024)); pk == 0 {
		t.Fatal("cancel must not touch a voice that already started")
	}
}

// Previews / UI sounds carry vid 0 and survive CancelAllLaneFuture.
func TestMixerCancelAllFutureSparesPreviews(t *testing.T) {
	m := laneTestMixer()
	m.ScheduleLane(channel.Of("lanetest"), newTestSineVoice(), 2048, -1, -1, 11) // lane
	m.Schedule(channel.Of("lanetest"), newTestSineVoice(), 2048, -1, -1)         // preview (vid 0)
	m.cancelAllLaneFuture()
	if m.hasVoiceForTest(11) {
		t.Fatal("the future lane voice must be removed")
	}
	if pk := peakOf(m.renderForTest(8192)); pk == 0 {
		t.Fatal("the preview (vid 0) must survive CancelAllLaneFuture")
	}
}

// A mute node must not kill the notes after it.
func TestMixerStopLaneAtSparesLaterVoices(t *testing.T) {
	m := laneTestMixer()
	m.ScheduleLane(channel.Of("lanetest"), newTestSineVoice(), 0, -1, -1, 21)    // sounding before t
	m.ScheduleLane(channel.Of("lanetest"), newTestSineVoice(), 8192, -1, -1, 22) // starts after t
	m.stopLaneAtSample(channel.Of("lanetest"), 4096)
	m.renderForTest(4096 + 2048) // past t + the anti-pop fade
	if !m.hasVoiceForTest(22) {
		t.Fatal("a voice starting after t must survive StopLaneAt")
	}
	if m.hasVoiceForTest(21) {
		t.Fatal("a voice started before t must be faded out and removed")
	}
}

// A lane voice is committed up to the horizon early; a trigger at schedule time would run meters and scope ahead of
// the sound.
func TestMixerLaneTriggerNotifiesAtFirstRender(t *testing.T) {
	m := laneTestMixer()
	got := make(chan string, 4)
	// Filtered by its own id: earlier tests' triggers reach the async loop and can arrive after this hook is installed.
	restore := setLaneTriggerHookForTest(func(id string) {
		if id == "lanetrigger" {
			got <- id
		}
	})
	t.Cleanup(restore)
	m.ScheduleLane(channel.Of("lanetrigger"), newTestSineVoice(), 4096, -1, -1, 31)
	m.renderForTest(2048) // before the voice's start
	select {
	case id := <-got:
		t.Fatalf("trigger for %q fired before the voice started", id)
	case <-time.After(50 * time.Millisecond):
	}
	m.renderForTest(4096) // crosses the start
	select {
	case id := <-got:
		if id != "lanetrigger" {
			t.Fatalf("trigger for %q, want lanetrigger", id)
		}
	case <-time.After(time.Second):
		t.Fatal("lane voice trigger never fired after it started rendering")
	}
}

// The mixer clock runs up to a Read chunk ahead of the lane's; a refused cancel is reported so the replacement is
// dropped instead of doubling.
func TestMixerCancelReportsVoicesThatAlreadyStarted(t *testing.T) {
	m := laneTestMixer()
	m.ScheduleLane(channel.Of("lanecancel"), newTestSineVoice(), 0, -1, -1, 41)    // starts now
	m.ScheduleLane(channel.Of("lanecancel"), newTestSineVoice(), 8192, -1, -1, 42) // still ahead
	m.renderForTest(1024)
	started := m.cancelVoices([]uint64{41, 42})
	if len(started) != 1 || started[0] != 41 {
		t.Fatalf("cancelVoices reported started=%v, want [41] (42 had not started and was removed)", started)
	}
	if m.hasVoiceForTest(42) {
		t.Fatal("the not-yet-started voice 42 was not removed")
	}
}
