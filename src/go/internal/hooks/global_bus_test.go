package hooks

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// Direct tests of the package-level forwarders: calls from other packages attribute coverage to their call sites.

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
		PublishKind(EventSeek, SeekPayload{Beats: 120})
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

	Publish(Event{Kind: EventSeek, Payload: SeekPayload{Beats: 60}})
	PublishKind(EventSeek, SeekPayload{Beats: 90})

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
	// A private bus, so the count is deterministic (the global one is shared by the whole binary).
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
	// Only drag-progress and instrument-param are verbose (camera kinds fire once per gesture); pinned so the filtered set
	// never grows quietly.
	verbose := []Kind{EventDragProgress, EventInstrumentParamChanged}
	for _, k := range verbose {
		if !IsVerbose(k) {
			t.Errorf("%s should be verbose", k)
		}
	}

	nonVerbose := []Kind{
		EventPlayStart, EventPlayStop, EventPaused, EventResumed,
		EventSeek, EventNodeAdded, EventNodeDeleted,
		EventEdgeAdded, EventRowAdded, EventMasterVolumeChange,
		EventCameraPan, EventCameraZoom, // promoted out of verbose
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
