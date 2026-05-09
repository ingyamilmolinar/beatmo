package hooks

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestCaptureSourceReturnsCallerNotHelper verifies skip=1 returns the
// caller's caller (not CaptureSource itself, not the immediate caller).
// This is the contract every emit helper relies on.
func TestCaptureSourceReturnsCallerNotHelper(t *testing.T) {
	// emit is the "helper" — we want CaptureSource(1) inside it to point at
	// the test, not at emit.go.
	emit := func() Source { return CaptureSource(1) }

	_, wantFile, wantLine, _ := runtime.Caller(0)
	src := emit() // expected line: the line above + 1
	wantLine++    // src() call sits one line below the runtime.Caller() above

	if src.IsZero() {
		t.Fatalf("CaptureSource returned zero value")
	}
	if !strings.HasSuffix(wantFile, src.File) {
		t.Errorf("captured file = %q; want suffix-match against %q", src.File, wantFile)
	}
	if src.Line != wantLine {
		t.Errorf("captured line = %d; want %d (one past the runtime.Caller probe)", src.Line, wantLine)
	}
	if !strings.HasPrefix(src.Pkg, "internal/hooks") {
		t.Errorf("captured pkg = %q; want internal/hooks prefix", src.Pkg)
	}
}

// TestCaptureSourceSkipZero returns the immediate caller — useful for
// callers that DON'T want to be wrapped (raw users of the API).
func TestCaptureSourceSkipZero(t *testing.T) {
	_, wantFile, wantLine, _ := runtime.Caller(0)
	src := CaptureSource(0) // line below the probe
	wantLine++

	if !strings.HasSuffix(wantFile, src.File) || src.Line != wantLine {
		t.Errorf("CaptureSource(0): got %s:%d, want %s:%d", src.File, src.Line, wantFile, wantLine)
	}
}

// TestPublishWithSourcePropagatesToSubscriber verifies the Source survives
// the bus dispatch (which runs subscribers on a separate worker pool).
func TestPublishWithSourcePropagatesToSubscriber(t *testing.T) {
	bus := NewBus(context.Background(), Options{PoolWorkers: 1, Name: "src-test"})
	defer bus.Close()

	var (
		mu     sync.Mutex
		gotSrc Source
		fired  = make(chan struct{}, 1)
	)
	unsub := bus.Subscribe(EventPlayStart, func(e Event) {
		mu.Lock()
		gotSrc = e.Source
		mu.Unlock()
		select {
		case fired <- struct{}{}:
		default:
		}
	})
	defer unsub()

	want := Source{Pkg: "internal/foo", File: "bar.go", Line: 42}
	bus.PublishWithSource(EventPlayStart, nil, want)

	select {
	case <-fired:
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber never received event")
	}

	mu.Lock()
	defer mu.Unlock()
	if gotSrc != want {
		t.Errorf("source on subscriber = %+v; want %+v", gotSrc, want)
	}
}

// TestPublishKindHasZeroSource confirms the legacy publish path still works
// and produces a zero-value Source so renderers can omit the src= column.
func TestPublishKindHasZeroSource(t *testing.T) {
	bus := NewBus(context.Background(), Options{PoolWorkers: 1, Name: "src-zero"})
	defer bus.Close()

	var (
		mu     sync.Mutex
		gotSrc Source
		fired  = make(chan struct{}, 1)
	)
	unsub := bus.Subscribe(EventPlayStop, func(e Event) {
		mu.Lock()
		gotSrc = e.Source
		mu.Unlock()
		select {
		case fired <- struct{}{}:
		default:
		}
	})
	defer unsub()

	bus.PublishKind(EventPlayStop, nil)

	select {
	case <-fired:
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber never received event")
	}

	mu.Lock()
	defer mu.Unlock()
	if !gotSrc.IsZero() {
		t.Errorf("PublishKind should produce zero Source; got %+v", gotSrc)
	}
}

// TestSourceStringFormat exercises the human-readable rendering used by
// eventlogger. Empty Pkg drops the slash; zero value renders empty.
func TestSourceStringFormat(t *testing.T) {
	cases := []struct {
		s    Source
		want string
	}{
		{Source{}, ""},
		{Source{Pkg: "internal/ui", File: "foo.go", Line: 12}, "internal/ui/foo.go:12"},
		{Source{File: "bar.go", Line: 1}, "bar.go:1"},
		{Source{Pkg: "core/engine", File: "predictor.go", Line: 200}, "core/engine/predictor.go:200"},
	}
	for _, tc := range cases {
		if got := tc.s.String(); got != tc.want {
			t.Errorf("Source(%+v).String() = %q; want %q", tc.s, got, tc.want)
		}
	}
}

// TestSplitPkgFileTrimToProjectRoot verifies splitPkgFile finds the known
// project roots (internal, core, cmd) and returns paths anchored there,
// not absolute machine-specific paths.
func TestSplitPkgFileTrimToProjectRoot(t *testing.T) {
	cases := []struct {
		in      string
		wantPkg string
		wantBase string
	}{
		{"/home/u/Repos/beatmo/src/go/internal/ui/game_graph_nodes.go", "internal/ui", "game_graph_nodes.go"},
		{"/home/u/Repos/beatmo/src/go/core/engine/predictor.go", "core/engine", "predictor.go"},
		{"/home/u/Repos/beatmo/src/go/cmd/beatmo.go", "cmd", "beatmo.go"},
		{"/tmp/something/else/x.go", "else", "x.go"}, // fallback: last dir segment
	}
	for _, tc := range cases {
		p, b := splitPkgFile(tc.in)
		if p != tc.wantPkg || b != tc.wantBase {
			t.Errorf("splitPkgFile(%q) = (%q, %q); want (%q, %q)", tc.in, p, b, tc.wantPkg, tc.wantBase)
		}
	}
}
