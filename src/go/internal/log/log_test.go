package log

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// These tests override the package-level inTests flag to simulate runtime
// environments and ensure logger wiring behaves as expected.

func TestNewDefaultsSilentInTests(t *testing.T) {
	assertDefaultTestState(t)
	oldInTests := inTests
	inTests = true
	defer func() { inTests = oldInTests }()
	withEnvUnset(t, "TUNKUL_TEST_LOG")
	withEnvUnset(t, "TUNKUL_TEST_LOG_LEVEL")
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
	withEnv(t, "TUNKUL_TEST_LOG", "1")
	withEnv(t, "TUNKUL_TEST_LOG_LEVEL", "TRACE")

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
	if !strings.Contains(string(out), "TRACE: hello world") {
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
	withEnvUnset(t, "TUNKUL_TEST_LOG")
	withEnvUnset(t, "TUNKUL_TEST_LOG_LEVEL")
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
	if !bytes.Contains(out, []byte("DEBUG: hello")) {
		t.Fatalf("expected debug log with -test.v, got %q", string(out))
	}
}

func TestLevelFromStringUnknownDefaultsDebug(t *testing.T) {
	if got := LevelFromString("bogus"); got != LevelDebug {
		t.Fatalf("expected unknown level to default to DEBUG, got %v", got)
	}
}

func assertDefaultTestState(t *testing.T) {
	t.Helper()
	if !inTests {
		t.Fatalf("inTests=%v want true by default", inTests)
	}
	if _, ok := os.LookupEnv("TUNKUL_TEST_LOG"); ok {
		t.Fatalf("TUNKUL_TEST_LOG should be unset by default for tests")
	}
	if _, ok := os.LookupEnv("TUNKUL_TEST_LOG_LEVEL"); ok {
		t.Fatalf("TUNKUL_TEST_LOG_LEVEL should be unset by default for tests")
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
