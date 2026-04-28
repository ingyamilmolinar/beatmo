package eventlogger

import (
	"sync"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// TestCoalescerEmitsTrailingValue verifies that a burst of N events for the
// same coalesced Kind produces exactly one emission carrying the LAST event's
// payload (debounce-trailing semantics).
func TestCoalescerEmitsTrailingValue(t *testing.T) {
	var (
		mu       sync.Mutex
		received []hooks.Event
	)
	c := newCoalescer(50*time.Millisecond, func(e hooks.Event) {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
	})

	for i := 1; i <= 20; i++ {
		c.Submit(hooks.Event{Kind: hooks.EventBPMChange, Payload: float64(100 + i)})
	}
	// Wait past the debounce window.
	time.Sleep(120 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if got := len(received); got != 1 {
		t.Fatalf("expected exactly 1 emission after burst, got %d (events=%v)", got, received)
	}
	if v, _ := received[0].Payload.(float64); v != 120.0 {
		t.Fatalf("expected trailing payload 120.0, got %v", received[0].Payload)
	}
}

// TestCoalescerSeparatesKinds verifies that bursts of different Kinds are
// independent — coalescing BPM doesn't suppress an EQ change.
func TestCoalescerSeparatesKinds(t *testing.T) {
	var (
		mu       sync.Mutex
		received []hooks.Kind
	)
	c := newCoalescer(40*time.Millisecond, func(e hooks.Event) {
		mu.Lock()
		received = append(received, e.Kind)
		mu.Unlock()
	})

	for i := 0; i < 5; i++ {
		c.Submit(hooks.Event{Kind: hooks.EventBPMChange, Payload: float64(120)})
		c.Submit(hooks.Event{Kind: hooks.EventMasterVolumeChange, Payload: hooks.MasterVolumePayload{Volume: 0.7}})
	}
	time.Sleep(120 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("expected one emission per Kind (2 total), got %d (kinds=%v)", len(received), received)
	}
	seen := map[hooks.Kind]bool{received[0]: true, received[1]: true}
	if !seen[hooks.EventBPMChange] || !seen[hooks.EventMasterVolumeChange] {
		t.Fatalf("expected both BPM and master-volume to emit, got %v", received)
	}
}

// TestCoalescerFlushAllForcesEmission verifies FlushAll synchronously emits
// every pending entry, used at shutdown and by deterministic tests.
func TestCoalescerFlushAllForcesEmission(t *testing.T) {
	var (
		mu       sync.Mutex
		received []hooks.Kind
	)
	c := newCoalescer(10*time.Second, func(e hooks.Event) {
		mu.Lock()
		received = append(received, e.Kind)
		mu.Unlock()
	})
	c.Submit(hooks.Event{Kind: hooks.EventBPMChange, Payload: float64(120)})
	c.Submit(hooks.Event{Kind: hooks.EventEQBandChange, Payload: hooks.EQBandPayload{Channel: "kick", Band: 3, GainDB: -2}})

	c.FlushAll()
	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("FlushAll should emit both pending entries, got %d (%v)", len(received), received)
	}
}

// TestCoalescerSkipsNonCoalescedKinds verifies IsCoalesced returns false for
// kinds not in the coalesceKinds set. The Logger uses this to decide whether
// to bypass the coalescer entirely.
func TestCoalescerSkipsNonCoalescedKinds(t *testing.T) {
	c := newCoalescer(10*time.Millisecond, func(_ hooks.Event) {
		t.Fatal("coalescer should not emit kinds it doesn't handle")
	})
	if c.IsCoalesced(hooks.EventPlayStart) {
		t.Fatal("PlayStart should not be coalesced")
	}
	if !c.IsCoalesced(hooks.EventBPMChange) {
		t.Fatal("BPMChange should be coalesced")
	}
}
