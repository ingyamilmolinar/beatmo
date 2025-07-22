package log

import (
	"io"
	"log"
	"strings"
)

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelError
	LevelNone
)

func (l Level) String() string {
	switch l {
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
	return &Logger{
		logger: log.New(out, "", 0), // No prefix, handled by format string
		level:  level,
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
