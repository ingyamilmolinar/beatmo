// Package log is the project's leveled logger.
//
// Level contract (enforced by tests; see internal/log/forbidden_at_info_test.go
// and internal/eventlogger/golden_session_test.go):
//
//	INFO   One line per user action or major component lifecycle event. Never
//	       per-frame, never "ignored X because Y", never internal-mechanism
//	       breakdowns. Most user-action narrative flows through internal/eventlogger
//	       which subscribes to hooks.Bus and emits through Infof.
//	WARN   Degraded-but-functional. Slow paths, parity mismatches when non-
//	       fatal, dropped frames, fallbacks taken.
//	ERROR  Failure: an operation could not complete.
//	DEBUG  Mechanism detail useful for diagnosing a specific subsystem. May
//	       fire per event or per pool job, but never per frame.
//	TRACE  Firehose. Per-frame, per-cell, per-tick.
//
// Output format: "15:04:05.000 LEVEL [tag] message" where [tag] is the first
// bracketed token of the format string (extracted automatically). When no tag
// is present the bracketed section is omitted.
package log

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var inTests = func() bool {
	// Heuristic: test binaries are named *.test and receive -test.* flags.
	base := filepath.Base(os.Args[0])
	if strings.HasSuffix(base, ".test") {
		return true
	}
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "-test.") {
			return true
		}
	}
	return false
}()

func testVerboseFlag() bool {
	for _, arg := range os.Args[1:] {
		if arg == "-test.v" || strings.HasPrefix(arg, "-test.v=") {
			return true
		}
	}
	return false
}

type Level int

const (
	LevelTrace Level = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
	LevelNone
)

func (l Level) String() string {
	switch l {
	case LevelTrace:
		return "TRACE"
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelNone:
		return "NONE"
	default:
		return "UNKNOWN"
	}
}

func LevelFromString(s string) Level {
	switch strings.ToUpper(s) {
	case "TRACE":
		return LevelTrace
	case "DEBUG":
		return LevelDebug
	case "INFO":
		return LevelInfo
	case "WARN":
		return LevelWarn
	case "ERROR":
		return LevelError
	case "NONE":
		return LevelNone
	default:
		return LevelDebug // Default to DEBUG
	}
}

type Logger struct {
	logger *log.Logger
	level  Level
}

func New(out io.Writer, level Level) *Logger {
	// Default to discard when not provided to avoid nil panics in tests.
	if out == nil {
		out = io.Discard
	}
	// In tests, default to silent to keep suites quiet. Opt-in via
	// BEATMO_TEST_LOG=1 or -test.v. Level can be overridden via env.
	if inTests {
		verbose := os.Getenv("BEATMO_TEST_LOG") == "1" || testVerboseFlag()
		if verbose {
			if lvl := strings.TrimSpace(os.Getenv("BEATMO_TEST_LOG_LEVEL")); lvl != "" {
				level = LevelFromString(lvl)
			} else if level > LevelDebug {
				level = LevelDebug
			}
			out = os.Stdout
		} else {
			level = LevelNone
			out = io.Discard
		}
	}
	return &Logger{
		logger: log.New(out, "", 0), // No prefix; we format inline so we can extract tags.
		level:  level,
	}
}

// timestampFunc is overridable for deterministic test output.
var timestampFunc = func() string { return time.Now().Format("15:04:05.000") }

// extractTag pulls the leading "[tag]" out of format if present and returns
// (tag, rest) so callers can render the tag in its own column. Verbs inside
// the tag are not interpreted (callers don't put printf verbs in tags).
func extractTag(format string) (tag, rest string) {
	if len(format) > 0 && format[0] == '[' {
		if i := strings.IndexByte(format, ']'); i > 0 {
			return format[1:i], strings.TrimLeft(format[i+1:], " ")
		}
	}
	return "", format
}

// emit composes the final format string with timestamp + level + optional tag
// column, then delegates to the underlying log.Logger.
func (l *Logger) emit(level string, format string, v ...interface{}) {
	tag, rest := extractTag(format)
	ts := timestampFunc()
	// Pad the level field to 5 chars so columns line up across levels.
	const lvlWidth = 5
	pad := ""
	if n := lvlWidth - len(level); n > 0 {
		pad = strings.Repeat(" ", n)
	}
	if tag != "" {
		l.logger.Printf(ts+" "+level+pad+" ["+tag+"] "+rest, v...)
	} else {
		l.logger.Printf(ts+" "+level+pad+" "+rest, v...)
	}
}

func (l *Logger) Tracef(format string, v ...interface{}) {
	if l.level <= LevelTrace {
		l.emit("TRACE", format, v...)
	}
}

func (l *Logger) Debugf(format string, v ...interface{}) {
	if l.level <= LevelDebug {
		l.emit("DEBUG", format, v...)
	}
}

func (l *Logger) Infof(format string, v ...interface{}) {
	if l.level <= LevelInfo {
		l.emit("INFO", format, v...)
	}
}

func (l *Logger) Warnf(format string, v ...interface{}) {
	if l.level <= LevelWarn {
		l.emit("WARN", format, v...)
	}
}

func (l *Logger) Errorf(format string, v ...interface{}) {
	if l.level <= LevelError {
		l.emit("ERROR", format, v...)
	}
}

func (l *Logger) SetLevel(level Level) {
	l.level = level
}

func (l *Logger) Level() Level {
	return l.level
}

// NewForTest returns a Logger that ignores the in-tests silencing, writing
// every level >= level to out. Use this in tests that need to capture log
// output deterministically (the regular New routes test output to stdout
// or io.Discard depending on BEATMO_TEST_LOG, which makes capture awkward).
func NewForTest(out io.Writer, level Level) *Logger {
	if out == nil {
		out = io.Discard
	}
	return &Logger{
		logger: log.New(out, "", 0),
		level:  level,
	}
}

// SetTimestampFunc overrides the package-level timestamp formatter. Returns
// a restore closure callers MUST defer. Tests use this to pin output to a
// known string so golden-file comparisons are deterministic.
func SetTimestampFunc(f func() string) (restore func()) {
	old := timestampFunc
	timestampFunc = f
	return func() { timestampFunc = old }
}
