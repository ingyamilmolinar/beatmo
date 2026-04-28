package log

import (
	"bytes"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
)

// These tests override the package-level inTests flag to simulate runtime
// environments and ensure logger wiring behaves as expected.

// fixedTimestamp pins timestampFunc to a deterministic value for tests that
// need to assert the prefix exactly.
func fixedTimestamp(t *testing.T, ts string) {
	t.Helper()
	old := timestampFunc
	timestampFunc = func() string { return ts }
	t.Cleanup(func() { timestampFunc = old })
}

func TestNewDefaultsSilentInTests(t *testing.T) {
	assertDefaultTestState(t)
	oldInTests := inTests
	inTests = true
	defer func() { inTests = oldInTests }()
	withEnvUnset(t, "BEATMO_TEST_LOG")
	withEnvUnset(t, "BEATMO_TEST_LOG_LEVEL")
	oldArgs := os.Args
	os.Args = []string{"cmd.test"}
	defer func() { os.Args = oldArgs }()

	var buf bytes.Buffer
	logger := New(&buf, LevelDebug)
	logger.Debugf("hello")
	logger.Tracef("trace")
	logger.Infof("info")

	if buf.Len() != 0 {
		t.Fatalf("expected no logs when test logging disabled, got %q", buf.String())
	}
	if logger.Level() != LevelNone {
		t.Fatalf("expected level forced to NONE, got %v", logger.Level())
	}
}

func TestNewEnablesLogsWhenOptedIn(t *testing.T) {
	assertDefaultTestState(t)
	oldInTests := inTests
	inTests = true
	defer func() { inTests = oldInTests }()
	withEnv(t, "BEATMO_TEST_LOG", "1")
	withEnv(t, "BEATMO_TEST_LOG_LEVEL", "TRACE")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	oldStdout := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	logger := New(io.Discard, LevelError)
	logger.Tracef("hello %s", "world")
	_ = w.Close()
	out, _ := io.ReadAll(r)
	if !strings.Contains(string(out), "TRACE hello world") {
		t.Fatalf("expected trace log on stdout, got: %q", string(out))
	}
	if logger.Level() != LevelTrace {
		t.Fatalf("expected level forced to TRACE when opt-in, got %v", logger.Level())
	}
}

func TestNewRespectsLevelOutsideTests(t *testing.T) {
	assertDefaultTestState(t)
	oldInTests := inTests
	inTests = false
	defer func() { inTests = oldInTests }()

	var buf bytes.Buffer
	logger := New(&buf, LevelError)

	logger.Debugf("should be silent")
	logger.Errorf("err %d", 1)
	logger.Tracef("trace")

	if buf.Len() == 0 {
		t.Fatalf("expected error log to be written")
	}
	if strings.Contains(buf.String(), "should be silent") {
		t.Fatalf("debug log should not be emitted when level is ERROR; got %q", buf.String())
	}
	if strings.Contains(buf.String(), "trace") {
		t.Fatalf("trace log should not be emitted when level is ERROR; got %q", buf.String())
	}
	if logger.Level() != LevelError {
		t.Fatalf("expected level to remain ERROR outside tests, got %v", logger.Level())
	}
}

func TestNewEnablesLogsWhenTestVFlag(t *testing.T) {
	assertDefaultTestState(t)
	oldInTests := inTests
	inTests = true
	defer func() { inTests = oldInTests }()
	withEnvUnset(t, "BEATMO_TEST_LOG")
	withEnvUnset(t, "BEATMO_TEST_LOG_LEVEL")
	oldArgs := os.Args
	os.Args = []string{"cmd.test", "-test.v"}
	defer func() { os.Args = oldArgs }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	oldStdout := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	logger := New(io.Discard, LevelError)
	logger.Debugf("hello")
	_ = w.Close()
	out, _ := io.ReadAll(r)
	if !bytes.Contains(out, []byte("DEBUG hello")) {
		t.Fatalf("expected debug log with -test.v, got %q", string(out))
	}
}

func TestLevelFromStringUnknownDefaultsDebug(t *testing.T) {
	if got := LevelFromString("bogus"); got != LevelDebug {
		t.Fatalf("expected unknown level to default to DEBUG, got %v", got)
	}
}

func TestLevelString(t *testing.T) {
	cases := []struct {
		l    Level
		want string
	}{
		{LevelTrace, "TRACE"},
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{LevelNone, "NONE"},
		{Level(99), "UNKNOWN"},
	}
	for _, tc := range cases {
		if got := tc.l.String(); got != tc.want {
			t.Errorf("Level(%d).String() = %q, want %q", tc.l, got, tc.want)
		}
	}
}

func TestLevelFromStringEdgeCases(t *testing.T) {
	cases := []struct {
		in   string
		want Level
	}{
		{"TRACE", LevelTrace},
		{"trace", LevelTrace},
		{"Trace", LevelTrace},
		{"DEBUG", LevelDebug},
		{"debug", LevelDebug},
		{"INFO", LevelInfo},
		{"info", LevelInfo},
		{"WARN", LevelWarn},
		{"warn", LevelWarn},
		{"ERROR", LevelError},
		{"NONE", LevelNone},
		{"", LevelDebug},
		{"garbage", LevelDebug},
	}
	for _, tc := range cases {
		if got := LevelFromString(tc.in); got != tc.want {
			t.Errorf("LevelFromString(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// withInTestsFalse flips the package inTests flag so New() honors the
// caller-provided io.Writer and Level instead of routing to stdout/discard.
func withInTestsFalse(t *testing.T) {
	t.Helper()
	old := inTests
	inTests = false
	t.Cleanup(func() { inTests = old })
}

func TestSetLevel(t *testing.T) {
	assertDefaultTestState(t)
	withInTestsFalse(t)

	var buf bytes.Buffer
	logger := New(&buf, LevelError)
	logger.Tracef("muted")
	if buf.Len() != 0 {
		t.Fatalf("expected no output before SetLevel, got %q", buf.String())
	}
	logger.SetLevel(LevelTrace)
	if logger.Level() != LevelTrace {
		t.Fatalf("Level() = %v, want LevelTrace", logger.Level())
	}
	logger.Tracef("now visible %d", 7)
	if !strings.Contains(buf.String(), "TRACE now visible 7") {
		t.Fatalf("expected trace output after SetLevel, got %q", buf.String())
	}
}

// TestWarnHasOwnThreshold verifies that Warnf has its own LevelWarn threshold,
// distinct from LevelInfo. Previously Warnf shared the LevelInfo gate, so
// callers couldn't silence warnings without also silencing info.
func TestWarnHasOwnThreshold(t *testing.T) {
	assertDefaultTestState(t)
	withInTestsFalse(t)

	cases := []struct {
		level     Level
		wantWrite bool
	}{
		{LevelTrace, true},
		{LevelDebug, true},
		{LevelInfo, true},
		{LevelWarn, true},
		{LevelError, false},
		{LevelNone, false},
	}
	for _, tc := range cases {
		var buf bytes.Buffer
		logger := New(&buf, tc.level)
		logger.Warnf("careful")
		got := strings.Contains(buf.String(), "WARN  careful")
		if got != tc.wantWrite {
			t.Errorf("Warnf at %v: wrote=%v want=%v (buf=%q)", tc.level, got, tc.wantWrite, buf.String())
		}
	}
}

func TestNewNilWriterDoesNotPanic(t *testing.T) {
	assertDefaultTestState(t)
	withInTestsFalse(t)
	logger := New(nil, LevelTrace)
	logger.Tracef("hello") // routes to io.Discard, must not panic
}

func TestInfofThresholds(t *testing.T) {
	assertDefaultTestState(t)
	withInTestsFalse(t)

	for _, tc := range []struct {
		level     Level
		wantWrite bool
	}{
		{LevelTrace, true},
		{LevelDebug, true},
		{LevelInfo, true},
		{LevelWarn, false},
		{LevelError, false},
		{LevelNone, false},
	} {
		var buf bytes.Buffer
		logger := New(&buf, tc.level)
		logger.Infof("hi")
		got := strings.Contains(buf.String(), "INFO  hi")
		if got != tc.wantWrite {
			t.Errorf("Infof at %v: wrote=%v want=%v (buf=%q)", tc.level, got, tc.wantWrite, buf.String())
		}
	}
}

// TestLogFormatNoTag asserts the bare "TIMESTAMP LEVEL message" shape.
// The level field is right-padded to 5 chars so columns line up.
func TestLogFormatNoTag(t *testing.T) {
	assertDefaultTestState(t)
	withInTestsFalse(t)
	fixedTimestamp(t, "12:34:56.789")

	var buf bytes.Buffer
	logger := New(&buf, LevelTrace)
	logger.Infof("hello %s", "world")
	logger.Debugf("debug line")
	logger.Warnf("warn line")
	logger.Errorf("err line")
	logger.Tracef("trace line")

	out := buf.String()
	wantLines := []string{
		"12:34:56.789 INFO  hello world\n",
		"12:34:56.789 DEBUG debug line\n",
		"12:34:56.789 WARN  warn line\n",
		"12:34:56.789 ERROR err line\n",
		"12:34:56.789 TRACE trace line\n",
	}
	for _, want := range wantLines {
		if !strings.Contains(out, want) {
			t.Errorf("missing line %q in output:\n%s", want, out)
		}
	}
}

// TestLogFormatWithTag asserts that a leading "[tag]" in the format string is
// extracted into its own column, leaving the message body without the tag.
func TestLogFormatWithTag(t *testing.T) {
	assertDefaultTestState(t)
	withInTestsFalse(t)
	fixedTimestamp(t, "12:34:56.789")

	var buf bytes.Buffer
	logger := New(&buf, LevelTrace)
	logger.Infof("[zoom] wheel dz=%v", 1.5)
	logger.Debugf("[refresh] horizon→%d", 48)

	out := buf.String()
	wants := []string{
		"12:34:56.789 INFO  [zoom] wheel dz=1.5\n",
		"12:34:56.789 DEBUG [refresh] horizon→48\n",
	}
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Errorf("missing line %q in output:\n%s", want, out)
		}
	}
}

// TestLogTimestampShape sanity-checks the live timestamp formatter (not pinned)
// to ensure it uses HH:MM:SS.mmm shape — guards against accidental format drift.
func TestLogTimestampShape(t *testing.T) {
	assertDefaultTestState(t)
	withInTestsFalse(t)

	var buf bytes.Buffer
	logger := New(&buf, LevelInfo)
	logger.Infof("ping")
	re := regexp.MustCompile(`^\d{2}:\d{2}:\d{2}\.\d{3} INFO {2}ping$`)
	line := strings.TrimRight(buf.String(), "\n")
	if !re.MatchString(line) {
		t.Fatalf("timestamp shape mismatch: %q", line)
	}
}

func assertDefaultTestState(t *testing.T) {
	t.Helper()
	if !inTests {
		t.Fatalf("inTests=%v want true by default", inTests)
	}
	if _, ok := os.LookupEnv("BEATMO_TEST_LOG"); ok {
		t.Fatalf("BEATMO_TEST_LOG should be unset by default for tests")
	}
	if _, ok := os.LookupEnv("BEATMO_TEST_LOG_LEVEL"); ok {
		t.Fatalf("BEATMO_TEST_LOG_LEVEL should be unset by default for tests")
	}
}

func withEnv(t *testing.T, key, val string) {
	t.Helper()
	if _, ok := os.LookupEnv(key); ok {
		t.Fatalf("expected %s to be unset before override", key)
	}
	old, ok := os.LookupEnv(key)
	if err := os.Setenv(key, val); err != nil {
		t.Fatalf("setenv %s: %v", key, err)
	}
	t.Cleanup(func() {
		if ok {
			_ = os.Setenv(key, old)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

func withEnvUnset(t *testing.T, key string) {
	t.Helper()
	if _, ok := os.LookupEnv(key); ok {
		t.Fatalf("expected %s to be unset before override", key)
	}
	old, ok := os.LookupEnv(key)
	_ = os.Unsetenv(key)
	t.Cleanup(func() {
		if ok {
			_ = os.Setenv(key, old)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}
