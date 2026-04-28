package hooks

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// global_bus_test.go covers the package-level Publish/PublishKind/Subscribe
// forwarders against the lazily-initialized process-wide GlobalBus(), plus
// IsVerbose and Stats(). Round 1 hit these helpers transitively from
// other packages, but coverage attributes those calls to the call sites
// — these direct tests make the contract explicit.

func awaitEvent(t *testing.T, k Kind, do func()) Event {
	t.Helper()
	got := make(chan Event, 1)
	unsub := Subscribe(k, func(e Event) {
		select {
		case got <- e:
		default:
		}
	})
	t.Cleanup(unsub)
	do()
	select {
	case e := <-got:
		return e
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for %s", k)
		return Event{}
	}
}

func TestGlobalBusPublishKindRoundTrip(t *testing.T) {
	// Round-trip via PublishKind.
	e := awaitEvent(t, EventSeek, func() {
		PublishKind(EventSeek, SeekPayload{Beats:120})
	})
	if e.Kind != EventSeek {
		t.Errorf("Kind=%v want %v", e.Kind, EventSeek)
	}
	if p, ok := e.Payload.(SeekPayload); !ok || p.Beats != 120 {
		t.Errorf("payload: %#v", e.Payload)
	}
	// At should be populated by Publish (non-zero time).
	if e.At.IsZero() {
		t.Errorf("At is zero — Publish must stamp time")
	}
}

func TestGlobalBusPublishMatchesPublishKind(t *testing.T) {
	// Publish and PublishKind must produce equivalent events. Subscribe
	// once, fire both, observe two arrivals.
	got := make(chan Event, 2)
	unsub := Subscribe(EventSeek, func(e Event) {
		select {
		case got <- e:
		default:
		}
	})
	t.Cleanup(unsub)

	Publish(Event{Kind: EventSeek, Payload: SeekPayload{Beats:60}})
	PublishKind(EventSeek, SeekPayload{Beats:90})

	collected := make([]Event, 0, 2)
	deadline := time.After(500 * time.Millisecond)
	for len(collected) < 2 {
		select {
		case e := <-got:
			collected = append(collected, e)
		case <-deadline:
			t.Fatalf("only got %d/2 events", len(collected))
		}
	}
	beats := map[int]bool{}
	for _, e := range collected {
		p := e.Payload.(SeekPayload)
		beats[p.Beats] = true
	}
	if !beats[60] || !beats[90] {
		t.Errorf("missing one of 60/90: got %v", beats)
	}
}

func TestStatsCountsPublished(t *testing.T) {
	// Use a private bus so the count is deterministic (the global bus
	// is shared across the whole test binary).
	b := NewBus(context.TODO(), Options{Name: "test.stats"})
	t.Cleanup(func() { _ = b.Close() })

	var received atomic.Int32
	b.Subscribe(EventSeek, func(Event) { received.Add(1) })

	const N = 25
	for i := range N {
		b.PublishKind(EventSeek, SeekPayload{Beats: i})
	}

	// Stats() reflects the publish count immediately even if delivery is
	// still draining via the pool.
	st := b.Stats()
	if st.Published != N {
		t.Errorf("Stats().Published = %d, want %d", st.Published, N)
	}

	// Drain by closing — Close blocks until the pool finishes.
	if err := b.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if got := int(received.Load()); got != N {
		t.Errorf("received %d events, want %d", got, N)
	}
}

func TestIsVerbose(t *testing.T) {
	// Spec lives in events.go: only camera pan/zoom and drag-progress
	// are verbose. Lock the contract to prevent quiet additions that
	// would expand sink filtering.
	verbose := []Kind{EventCameraPan, EventCameraZoom, EventDragProgress}
	for _, k := range verbose {
		if !IsVerbose(k) {
			t.Errorf("%s should be verbose", k)
		}
	}

	nonVerbose := []Kind{
		EventPlayStart, EventPlayStop, EventPaused, EventResumed,
		EventSeek, EventNodeAdded, EventNodeDeleted,
		EventEdgeAdded, EventRowAdded, EventMasterVolumeChange,
	}
	for _, k := range nonVerbose {
		if IsVerbose(k) {
			t.Errorf("%s should NOT be verbose", k)
		}
	}

	// Spot-check that KindAll contains every verbose kind exactly once
	// (no duplicates, no quietly missing).
	verboseSeen := map[Kind]int{}
	for _, k := range KindAll {
		if IsVerbose(k) {
			verboseSeen[k]++
		}
	}
	for _, k := range verbose {
		if verboseSeen[k] != 1 {
			t.Errorf("verbose kind %s appears %d times in KindAll, want 1",
				k, verboseSeen[k])
		}
	}
}
