package log

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
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
	// TUNKUL_TEST_LOG=1 or -test.v. Level can be overridden via env.
	if inTests {
		verbose := os.Getenv("TUNKUL_TEST_LOG") == "1" || testVerboseFlag()
		if verbose {
			if lvl := strings.TrimSpace(os.Getenv("TUNKUL_TEST_LOG_LEVEL")); lvl != "" {
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
		logger: log.New(out, "", 0), // No prefix, handled by format string
		level:  level,
	}
}

func (l *Logger) Tracef(format string, v ...interface{}) {
	if l.level <= LevelTrace {
		l.logger.Printf("TRACE: "+format, v...)
	}
}

func (l *Logger) Debugf(format string, v ...interface{}) {
	if l.level <= LevelDebug {
		l.logger.Printf("DEBUG: "+format, v...)
	}
}

func (l *Logger) Infof(format string, v ...interface{}) {
	if l.level <= LevelInfo {
		l.logger.Printf("INFO: "+format, v...)
	}
}

func (l *Logger) Errorf(format string, v ...interface{}) {
	if l.level <= LevelError {
		l.logger.Printf("ERROR: "+format, v...)
	}
}

func (l *Logger) Warnf(format string, v ...interface{}) {
	if l.level <= LevelInfo { // Warnings are shown at Info level or higher
		l.logger.Printf("WARN: "+format, v...)
	}
}

func (l *Logger) SetLevel(level Level) {
	l.level = level
}

func (l *Logger) Level() Level {
	return l.level
}
