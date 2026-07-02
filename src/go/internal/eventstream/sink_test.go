package eventstream

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

func newTestBus(t *testing.T) *hooks.Bus {
	t.Helper()
	// Large pool queue so the fan-out path doesn't drop events under
	// burst publishes — that would conflate "bus fan-out drops" with
	// "sink-queue drops" in the saturation test.
	return hooks.NewBus(context.Background(), hooks.Options{
		PoolWorkers: 2, PoolQueue: 4096, Name: "eventstream-test",
	})
}

func TestSink_RoundTripCapturesAllEvents(t *testing.T) {
	bus := newTestBus(t)
	defer bus.Close()
	path := filepath.Join(t.TempDir(), "events.jsonl")

	sink, err := Open(bus, path, Options{FlushInterval: 50 * time.Millisecond})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	bus.PublishKind(hooks.EventPlayStart, nil)
	bus.PublishKind(hooks.EventBPMChange, 144)
	bus.PublishKind(hooks.EventNodeAdded, hooks.NodeEdit{ID: 1, I: 4, J: -3, Type: "regular"})
	bus.PublishKind(hooks.EventPlayStop, nil)

	// Allow time for fan-out + flush.
	time.Sleep(200 * time.Millisecond)
	if err := sink.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	records := readJSONL(t, path)
	if len(records) != 4 {
		t.Fatalf("got %d records, want 4: %+v", len(records), records)
	}
	kinds := make(map[string]bool)
	for _, r := range records {
		kinds[r.Kind] = true
	}
	for _, want := range []string{
		string(hooks.EventPlayStart),
		string(hooks.EventBPMChange),
		string(hooks.EventNodeAdded),
		string(hooks.EventPlayStop),
	} {
		if !kinds[want] {
			t.Errorf("missing kind %q in trace", want)
		}
	}
}

func TestSink_PreservesSequenceOrdering(t *testing.T) {
	bus := newTestBus(t)
	defer bus.Close()
	path := filepath.Join(t.TempDir(), "events.jsonl")
	sink, _ := Open(bus, path, Options{FlushInterval: 50 * time.Millisecond})

	const N = 100
	for i := 0; i < N; i++ {
		bus.PublishKind(hooks.EventBPMChange, i)
	}
	time.Sleep(300 * time.Millisecond)
	sink.Close()

	records := readJSONL(t, path)
	if len(records) != N {
		t.Fatalf("got %d, want %d", len(records), N)
	}
	for i, r := range records {
		if int(r.Seq) != i+1 {
			t.Fatalf("record %d Seq = %d, want %d", i, r.Seq, i+1)
		}
	}
}

func TestSink_FiltersVerboseByDefault(t *testing.T) {
	bus := newTestBus(t)
	defer bus.Close()
	path := filepath.Join(t.TempDir(), "events.jsonl")
	sink, _ := Open(bus, path, Options{FlushInterval: 50 * time.Millisecond})

	bus.PublishKind(hooks.EventDragProgress, hooks.DragProgressPayload{NodeID: 1, I: 0, J: 0})
	bus.PublishKind(hooks.EventNodeAdded, hooks.NodeEdit{ID: 1})

	time.Sleep(200 * time.Millisecond)
	sink.Close()

	records := readJSONL(t, path)
	if len(records) != 1 {
		t.Fatalf("got %d, want 1 (verbose should be filtered)", len(records))
	}
	if records[0].Kind != string(hooks.EventNodeAdded) {
		t.Fatalf("got kind %s, want %s", records[0].Kind, hooks.EventNodeAdded)
	}
}

func TestSink_VerboseModeIncludesAll(t *testing.T) {
	bus := newTestBus(t)
	defer bus.Close()
	path := filepath.Join(t.TempDir(), "events.jsonl")
	sink, _ := Open(bus, path, Options{
		FlushInterval: 50 * time.Millisecond,
		Verbose:       true,
	})

	bus.PublishKind(hooks.EventDragProgress, hooks.DragProgressPayload{NodeID: 1, I: 0, J: 0})
	bus.PublishKind(hooks.EventNodeAdded, hooks.NodeEdit{ID: 1})

	time.Sleep(200 * time.Millisecond)
	sink.Close()

	records := readJSONL(t, path)
	if len(records) != 2 {
		t.Fatalf("got %d, want 2 (verbose mode)", len(records))
	}
}

func TestSink_DropsOnSaturation(t *testing.T) {
	// Test the drop contract directly: with a queue of capacity 2 and no
	// writer goroutine draining it, the 3rd+ enqueue must drop. Going
	// through the bus + writer races a fast in-memory consumer against
	// producers — on most machines the writer drains the queue between
	// sends and no drops occur, so that path can't deterministically
	// exercise saturation. The drop semantics live in enqueue; test them.
	s := &Sink{queue: make(chan Record, 2)}
	const N = 1000
	for i := 0; i < N; i++ {
		s.enqueue(hooks.Event{Kind: hooks.EventBPMChange, Payload: i})
	}
	stats := s.Stats()
	if stats.Dropped == 0 {
		t.Fatalf("expected drops under saturation; stats=%+v", stats)
	}
	if want := int64(N - cap(s.queue)); stats.Dropped != want {
		t.Fatalf("dropped = %d, want %d (N - queue cap); stats=%+v", stats.Dropped, want, stats)
	}
}

func TestSink_OpenWithEmptyPathIsNoOp(t *testing.T) {
	bus := newTestBus(t)
	defer bus.Close()
	sink, err := Open(bus, "", Options{})
	if err != nil {
		t.Fatalf("Open(empty): %v", err)
	}
	bus.PublishKind(hooks.EventPlayStart, nil) // must not panic
	if err := sink.Close(); err != nil {
		t.Fatalf("Close no-op: %v", err)
	}
	if stats := sink.Stats(); stats.Written != 0 || stats.Active {
		t.Fatalf("no-op stats unexpected: %+v", stats)
	}
}

func TestSink_NilBusErrors(t *testing.T) {
	_, err := Open(nil, "/tmp/foo.jsonl", Options{})
	if err == nil {
		t.Fatal("expected error for nil bus")
	}
}

func TestSink_RotatesAtMaxBytes(t *testing.T) {
	bus := newTestBus(t)
	defer bus.Close()
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	sink, _ := Open(bus, path, Options{
		FlushInterval: 30 * time.Millisecond,
		MaxBytes:      256, // tiny so we rotate quickly
	})

	for i := 0; i < 50; i++ {
		bus.PublishKind(hooks.EventNodeAdded, hooks.NodeEdit{
			ID: i, I: i, J: i, Type: "regular",
		})
	}
	time.Sleep(300 * time.Millisecond)
	sink.Close()

	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("expected rotation file %s.1 to exist: %v", path, err)
	}
}

func TestSink_FlushNowDeterministicallyFlushes(t *testing.T) {
	bus := newTestBus(t)
	defer bus.Close()
	path := filepath.Join(t.TempDir(), "events.jsonl")
	sink, err := Open(bus, path, Options{FlushInterval: 5 * time.Millisecond})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer sink.Close()

	for i := range 5 {
		bus.PublishKind(hooks.EventBPMChange, i)
	}
	sink.FlushNow()

	records := readJSONL(t, path)
	if len(records) < 5 {
		t.Fatalf("FlushNow did not flush all records: got %d want >=5", len(records))
	}
}

func TestSink_FlushNowOnClosedSinkSkipsSleep(t *testing.T) {
	bus := newTestBus(t)
	defer bus.Close()
	path := filepath.Join(t.TempDir(), "events.jsonl")
	sink, _ := Open(bus, path, Options{FlushInterval: time.Hour})
	bus.PublishKind(hooks.EventPlayStart, nil)
	if err := sink.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	start := time.Now()
	sink.FlushNow() // closed → must not sleep an hour
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("FlushNow on closed sink took %v; expected near-instant", elapsed)
	}
}

func TestSink_NilReceiverIsSafe(t *testing.T) {
	var s *Sink
	if got := s.Stats(); got != (Stats{}) {
		t.Fatalf("nil Stats = %+v, want zero", got)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("nil Close = %v", err)
	}
	s.FlushNow() // must not panic
}

func TestSink_DoubleCloseIsSafe(t *testing.T) {
	bus := newTestBus(t)
	defer bus.Close()
	path := filepath.Join(t.TempDir(), "events.jsonl")
	sink, _ := Open(bus, path, Options{FlushInterval: 50 * time.Millisecond})
	if err := sink.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := sink.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestSink_EnqueueAfterCloseDropped(t *testing.T) {
	bus := newTestBus(t)
	defer bus.Close()
	path := filepath.Join(t.TempDir(), "events.jsonl")
	sink, _ := Open(bus, path, Options{FlushInterval: 50 * time.Millisecond})
	if err := sink.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// Direct enqueue exercises the "closed.Load() == true" guard.
	sink.enqueue(hooks.Event{Kind: hooks.EventPlayStart})
	if got := sink.Stats().Written; got != 0 {
		t.Fatalf("Written = %d after enqueue-on-closed, want 0", got)
	}
}

func TestSink_EnqueueZeroTimeStampsNow(t *testing.T) {
	bus := newTestBus(t)
	defer bus.Close()
	path := filepath.Join(t.TempDir(), "events.jsonl")
	sink, _ := Open(bus, path, Options{FlushInterval: 5 * time.Millisecond})
	defer sink.Close()

	// Hand-crafted event with At zero exercises the "rec.At.IsZero()" branch.
	sink.enqueue(hooks.Event{Kind: hooks.EventPlayStart})
	sink.FlushNow()
	records := readJSONL(t, path)
	if len(records) == 0 {
		t.Fatal("no records written")
	}
	if records[0].At.IsZero() {
		t.Fatal("expected non-zero At after enqueue stamping")
	}
}

func TestSink_WriteOneEncoderErrorSkipsRecord(t *testing.T) {
	bus := newTestBus(t)
	defer bus.Close()
	path := filepath.Join(t.TempDir(), "events.jsonl")
	sink, _ := Open(bus, path, Options{FlushInterval: 5 * time.Millisecond})
	defer sink.Close()

	// Channels are not JSON-encodable. The record is consumed but never
	// written; subsequent encodable records still succeed.
	sink.enqueue(hooks.Event{Kind: hooks.EventPlayStart, Payload: make(chan int)})
	sink.enqueue(hooks.Event{Kind: hooks.EventPlayStop})
	sink.FlushNow()

	records := readJSONL(t, path)
	for _, r := range records {
		if r.Kind == string(hooks.EventPlayStart) {
			t.Fatalf("unencodable record should have been skipped: %+v", r)
		}
	}
}

func TestSink_OpenAppliesDefaults(t *testing.T) {
	bus := newTestBus(t)
	defer bus.Close()
	path := filepath.Join(t.TempDir(), "events.jsonl")
	// Zero FlushInterval and QueueSize → defaults applied (250ms, 1024).
	sink, err := Open(bus, path, Options{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer sink.Close()
	if sink.opts.FlushInterval == 0 {
		t.Error("FlushInterval default not applied")
	}
	if sink.opts.QueueSize == 0 {
		t.Error("QueueSize default not applied")
	}
}

func TestSink_OpenInvalidPathErrors(t *testing.T) {
	bus := newTestBus(t)
	defer bus.Close()
	// A path under a non-existent directory cannot be created.
	_, err := Open(bus, "/this/dir/does/not/exist/file.jsonl", Options{})
	if err == nil {
		t.Fatal("expected error for unwritable path")
	}
}

// readJSONL parses a JSONL file into a slice of Record.
func readJSONL(t *testing.T, path string) []Record {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open trace: %v", err)
	}
	defer f.Close()
	var out []Record
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		var r Record
		if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
			t.Fatalf("unmarshal %q: %v", scanner.Text(), err)
		}
		out = append(out, r)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner: %v", err)
	}
	return out
}
