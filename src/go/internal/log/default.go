package log

import (
	"os"
	"sync/atomic"
)

// The process-wide default logger, for packages that have no Logger injected
// (internal/audio's engine, recorder and library code). The binary points it
// at its own -log logger at startup (SetDefault), so those lines share the
// format, level and destination of every other line; under `go test` it is a
// silenced DEBUG logger like any other, so its lines go to the failure tail
// (TailOnFailure) instead of the console. Never import the standard library's
// "log" in production code — it bypasses both.
var defaultLogger atomic.Pointer[Logger]

func init() {
	level := LevelInfo
	if inTests {
		level = LevelDebug
	}
	defaultLogger.Store(New(os.Stderr, level))
}

// Default returns the process-wide default logger.
func Default() *Logger { return defaultLogger.Load() }

// SetDefault replaces the process-wide default logger (nil is ignored) and
// returns a restore closure.
func SetDefault(l *Logger) (restore func()) {
	if l == nil {
		return func() {}
	}
	prev := defaultLogger.Swap(l)
	return func() { defaultLogger.Store(prev) }
}

// Tracef logs through the default logger.
func Tracef(format string, v ...interface{}) { Default().Tracef(format, v...) }

// Debugf logs through the default logger.
func Debugf(format string, v ...interface{}) { Default().Debugf(format, v...) }

// Infof logs through the default logger.
func Infof(format string, v ...interface{}) { Default().Infof(format, v...) }

// Warnf logs through the default logger.
func Warnf(format string, v ...interface{}) { Default().Warnf(format, v...) }

// Errorf logs through the default logger.
func Errorf(format string, v ...interface{}) { Default().Errorf(format, v...) }
