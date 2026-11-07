// Package log is the leveled logger. INFO: one line per user action or major lifecycle event, never per frame or
// mechanism. WARN degraded, ERROR failed, DEBUG mechanism (never per frame), TRACE firehose. Lines read
// "15:04:05.000 LEVEL [tag] message", the tag lifted from the format's leading [tag].
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

// testLogVerbose reports whether test logging prints live.
func testLogVerbose() bool {
	return os.Getenv("BEATMO_TEST_LOG") == "1" || testVerboseFlag()
}

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
	// Silent by default: lines go to the failure tail at the logger's level, printed only by a failing test that asked
	// (TailOnFailure). BEATMO_TEST_LOG=1 or -test.v shows them live.
	if inTests {
		if testLogVerbose() {
			if lvl := strings.TrimSpace(os.Getenv("BEATMO_TEST_LOG_LEVEL")); lvl != "" {
				level = LevelFromString(lvl)
			} else if level > LevelDebug {
				level = LevelDebug
			}
			out = os.Stdout
		} else {
			out = testTail
		}
	}
	return &Logger{
		logger: log.New(out, "", 0), // No prefix; we format inline so we can extract tags.
		level:  level,
	}
}

// timestampFunc is overridable for deterministic test output.
var timestampFunc = func() string { return time.Now().Format("15:04:05.000") }

// Verbs inside the tag are not interpreted.
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

// Bypasses the in-tests silencing, so a test can capture output deterministically.
func NewForTest(out io.Writer, level Level) *Logger {
	if out == nil {
		out = io.Discard
	}
	return &Logger{
		logger: log.New(out, "", 0),
		level:  level,
	}
}

// Callers MUST defer the restore; pins output for golden comparisons.
func SetTimestampFunc(f func() string) (restore func()) {
	old := timestampFunc
	timestampFunc = f
	return func() { timestampFunc = old }
}
