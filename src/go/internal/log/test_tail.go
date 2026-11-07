package log

import (
	"strconv"
	"strings"
	"sync"
)

// A silenced Logger under go test writes into one bounded process-wide ring: a passing test prints nothing, a failing
// one that registered TailOnFailure prints the latest lines, so suites stay quiet without losing a failure's context.

// testTailCap is how many lines the ring keeps; testTailShown is how many of
// the latest a failing test prints.
const (
	testTailCap   = 1024
	testTailShown = 40
)

type tailRing struct {
	mu    sync.Mutex
	lines [testTailCap]string
	next  uint64 // sequence number of the next line written
}

var testTail = &tailRing{}

// Write stores one line per newline-terminated record (log.Logger writes each
// record in a single call).
func (r *tailRing) Write(p []byte) (int, error) {
	s := strings.TrimRight(string(p), "\n")
	r.mu.Lock()
	for _, line := range strings.Split(s, "\n") {
		r.lines[r.next%testTailCap] = line
		r.next++
	}
	r.mu.Unlock()
	return len(p), nil
}

func (r *tailRing) seq() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.next
}

// since returns at most n of the latest lines written at or after seq start,
// plus how many lines since start are not returned (shown-limit or overwritten).
func (r *tailRing) since(start uint64, n int) (lines []string, omitted uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	from := start
	if r.next > testTailCap && from < r.next-testTailCap {
		from = r.next - testTailCap
	}
	if r.next-from > uint64(n) {
		from = r.next - uint64(n)
	}
	for s := from; s < r.next; s++ {
		lines = append(lines, r.lines[s%testTailCap])
	}
	return lines, from - start
}

// This package ships in the binaries, so it must not import testing.
type FailureReporter interface {
	Helper()
	Cleanup(func())
	Failed() bool
	Logf(format string, args ...any)
}

var tailRegistered sync.Map // FailureReporter → struct{}

// The ring is process-wide, so a parallel test's lines can appear. Idempotent per tb, so helpers call it freely; a no-op
// when test logging already prints.
func TailOnFailure(tb FailureReporter) {
	tb.Helper()
	if !inTests || testLogVerbose() {
		return
	}
	if _, dup := tailRegistered.LoadOrStore(tb, struct{}{}); dup {
		return
	}
	start := testTail.seq()
	tb.Cleanup(func() {
		tailRegistered.Delete(tb)
		if !tb.Failed() {
			return
		}
		lines, omitted := testTail.since(start, testTailShown)
		if len(lines) == 0 {
			return
		}
		var b strings.Builder
		if omitted > 0 {
			b.WriteString("latest log lines during this test (")
			b.WriteString(strconv.FormatUint(omitted, 10))
			b.WriteString(" earlier omitted):\n")
		} else {
			b.WriteString("log lines during this test:\n")
		}
		for _, l := range lines {
			b.WriteString("    ")
			b.WriteString(l)
			b.WriteByte('\n')
		}
		tb.Logf("%s", b.String())
	})
}
