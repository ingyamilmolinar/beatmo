package log

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// fakeTB records what TailOnFailure prints; failed decides the verdict.
type fakeTB struct {
	failed   bool
	cleanups []func()
	logged   []string
}

func (f *fakeTB) Helper()                 {}
func (f *fakeTB) Cleanup(fn func())       { f.cleanups = append(f.cleanups, fn) }
func (f *fakeTB) Failed() bool            { return f.failed }
func (f *fakeTB) Logf(s string, a ...any) { f.logged = append(f.logged, fmt.Sprintf(s, a...)) }
func (f *fakeTB) finish() {
	for i := len(f.cleanups) - 1; i >= 0; i-- {
		f.cleanups[i]()
	}
}

func silencedTestLogger(t *testing.T) *Logger {
	t.Helper()
	oldInTests := inTests
	inTests = true
	t.Cleanup(func() { inTests = oldInTests })
	withEnvUnset(t, "BEATMO_TEST_LOG")
	withEnvUnset(t, "BEATMO_TEST_LOG_LEVEL")
	oldArgs := os.Args
	os.Args = []string{"cmd.test"}
	t.Cleanup(func() { os.Args = oldArgs })
	return New(os.Stdout, LevelDebug)
}

func TestTailOnFailurePrintsNothingForAPassingTest(t *testing.T) {
	l := silencedTestLogger(t)
	tb := &fakeTB{}
	TailOnFailure(tb)
	l.Debugf("[audio] captured")
	tb.finish()
	if len(tb.logged) != 0 {
		t.Fatalf("a passing test printed %q", tb.logged)
	}
}

func TestTailOnFailurePrintsOnlyTheLatestLinesOfThisTest(t *testing.T) {
	l := silencedTestLogger(t)
	l.Debugf("[audio] before the test")
	tb := &fakeTB{failed: true}
	TailOnFailure(tb)
	TailOnFailure(tb) // helpers may register repeatedly: one dump
	for i := 0; i < testTailShown+5; i++ {
		l.Debugf("[audio] line %d", i)
	}
	l.Tracef("[audio] below the logger's level")
	tb.finish()
	if len(tb.logged) != 1 {
		t.Fatalf("want one dump, got %d", len(tb.logged))
	}
	out := tb.logged[0]
	if strings.Contains(out, "before the test") || strings.Contains(out, "below the logger") {
		t.Fatalf("dump holds lines it should not:\n%s", out)
	}
	if strings.Contains(out, "line 4\n") || !strings.Contains(out, "line 5\n") || !strings.Contains(out, fmt.Sprintf("line %d\n", testTailShown+4)) {
		t.Fatalf("dump is not the latest %d lines:\n%s", testTailShown, out)
	}
	if !strings.Contains(out, "5 earlier omitted") {
		t.Fatalf("dump does not say how many lines it left out:\n%s", out)
	}
}

func TestTailOnFailureSurvivesRingWraparound(t *testing.T) {
	l := silencedTestLogger(t)
	tb := &fakeTB{failed: true}
	TailOnFailure(tb)
	for i := 0; i < 3*testTailCap; i++ {
		l.Debugf("[audio] line %d", i)
	}
	tb.finish()
	out := tb.logged[0]
	if got := strings.Count(out, "[audio] line"); got != testTailShown {
		t.Fatalf("want %d lines, got %d", testTailShown, got)
	}
	if !strings.Contains(out, fmt.Sprintf("line %d\n", 3*testTailCap-1)) {
		t.Fatalf("latest line missing:\n%s", out)
	}
}

func TestPackageFuncsUseTheDefaultLogger(t *testing.T) {
	var b strings.Builder
	restore := SetDefault(NewForTest(&b, LevelWarn))
	Debugf("[audio] hidden")
	Warnf("[audio] shown %d", 1)
	restore()
	if got := b.String(); strings.Contains(got, "hidden") || !strings.Contains(got, "WARN  [audio] shown 1") {
		t.Fatalf("default logger output = %q", got)
	}
}
