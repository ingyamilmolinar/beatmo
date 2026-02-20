package ui

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
)

const desktopTopOffset = 40

// gridTopOffset returns the transport-bar offset for the grid pane.
// On mobile (small screen), the transport bar lives in the drum pane,
// so no offset is needed. On desktop, reserves 40px at the top.
func gridTopOffset() int {
	if isSmallScreen() {
		return 0
	}
	return desktopTopOffset
}

const ebitenTPS = 60 // Ticks per second for Ebiten (stubbed for tests)

var enableDefaultStart = true

type PerfMode struct {
	fastPath     atomic.Bool
	forceRefresh atomic.Bool
}

func (p *PerfMode) FastPathEnabled() bool { return p.fastPath.Load() }
func (p *PerfMode) SetFastPath(enabled bool) {
	p.fastPath.Store(enabled)
	if enabled {
		// Ensure the first frame after enabling fast path publishes a fresh window.
		p.forceRefresh.Store(true)
	}
}
func (p *PerfMode) ForceRefresh() { p.forceRefresh.Store(true) }
func (p *PerfMode) ConsumeForceRefresh() bool {
	return p.forceRefresh.Swap(false)
}

const perfFastPathRefreshModulo = 8

var defaultPerfFastPath bool

func init() {
	if os.Getenv("PERF_BROWSER_UPDATE_MAX_MS") != "" || os.Getenv("PERF_FAST_PATH") == "1" || runtime.GOOS == "js" {
		defaultPerfFastPath = true
	}
}

var timelineTrace = os.Getenv("TIMELINE_TRACE") == "1"
var timelineTraceRow = func() int {
	if !timelineTrace {
		return 0
	}
	if v := os.Getenv("TIMELINE_TRACE_ROW"); v != "" {
		if idx, err := strconv.Atoi(v); err == nil && idx >= 0 {
			return idx
		}
	}
	return 0
}()

func SetDefaultStartForTest(enable bool) { enableDefaultStart = enable }

func (g *Game) SetPerfFastPath(enabled bool) {
	if g == nil {
		return
	}
	g.perfMode.SetFastPath(enabled)
}

var parityFatalEnabled atomic.Bool

func init() {
	parityFatalEnabled.Store(true)
	if v := strings.TrimSpace(os.Getenv("PARITY_FATAL")); v != "" {
		if v == "0" || strings.ToLower(v) == "false" {
			parityFatalEnabled.Store(false)
		}
	}
}

func SetParityFatal(enabled bool) { parityFatalEnabled.Store(enabled) }

type parityWatchMode int

const (
	parityWatchOff parityWatchMode = iota
	parityWatchLog
	parityWatchPanic
)

var parityWatchDefault parityWatchMode

func init() {
	parityWatchDefault = parityWatchOff
	if v := strings.TrimSpace(os.Getenv("PARITY_WATCH")); v != "" {
		switch strings.ToLower(v) {
		case "1", "true", "log":
			parityWatchDefault = parityWatchLog
		case "panic", "fatal":
			parityWatchDefault = parityWatchPanic
			// Ensure legacy parity fatal path also fires.
			parityFatalEnabled.Store(true)
		}
	}
}

// On WASM/Playwright runs we prefer logging over panicking so a transient
// parity mismatch does not terminate the Go runtime and break browser tests.
// Opt back into fatal behaviour with PARITY_WASM_FATAL=1|true|panic.
func init() {
	if runtime.GOOS != "js" {
		return
	}
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("PARITY_WASM_FATAL"))); v == "1" || v == "true" || v == "panic" {
		return
	}
	parityFatalEnabled.Store(false)
	// Keep logs for diagnostics but avoid exits.
	parityWatchDefault = parityWatchLog
}
