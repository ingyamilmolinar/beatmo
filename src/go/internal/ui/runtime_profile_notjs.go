//go:build !js

package ui

import (
	"os"
	"strconv"
	"time"
)

// newRuntimeProfile returns the desktop profile, with optional environment
// variable overrides applied. Phase F bench sweeps use these to flip
// individual knobs without recompiling.
func newRuntimeProfile() *RuntimeProfile {
	p := desktopRuntimeProfile()
	applyEnvProfileOverride(p)
	return p
}

func applyEnvProfileOverride(p *RuntimeProfile) {
	if v, ok := envInt("BEATMO_EDGE_CACHE_PAD"); ok {
		p.EdgeCachePad = v
	}
	if v, ok := envInt("BEATMO_GRID_CACHE_PAD"); ok {
		p.GridCachePad = v
	}
	if v, ok := envBool("BEATMO_DISABLE_NODE_GLOW"); ok {
		p.DisableNodeGlow = v
	}
	if v, ok := envBool("BEATMO_DISABLE_EDGE_ARROWS"); ok {
		p.DisableEdgeArrows = v
	}
	if v, ok := envBool("BEATMO_SIMPLE_DRAW_DEFAULT"); ok {
		p.SimpleDrawDefault = v
	}
	if v, ok := envInt("BEATMO_TIMELINE_INFO_THROTTLE_MS"); ok {
		p.TimelineInfoThrottleMS = v
	}
	if v, ok := envFloat("BEATMO_AUDIO_LOOKAHEAD_SEC"); ok {
		p.AudioLookaheadSec = v
	}
	if v, ok := envInt("BEATMO_AUDIO_BATCH_MAX"); ok {
		p.AudioBatchMax = v
	}
	if v, ok := envInt("BEATMO_SEQUENCER_TICK_MS"); ok {
		p.SequencerTickMS = v
	}
	if v, ok := envBool("BEATMO_ADAPTIVE_PAN_PAD"); ok {
		p.AdaptivePanPad = v
	}
	if v, ok := envBool("BEATMO_FAST_PAN_DETECT"); ok {
		p.FastPanDetect = v
	}
	if v, ok := envBool("BEATMO_PREDICTOR_BACKOFF"); ok {
		p.PredictorBackoffOnSlowDraw = v
	}
	// BEATMO_FORCE_GC_INTERVAL_MS in milliseconds; 0 disables the cooperative
	// forced GC. Desktop default is 0 (off); the GC-tuning sweep uses this to
	// probe whether forced GC helps desktop too.
	if v, ok := envInt("BEATMO_FORCE_GC_INTERVAL_MS"); ok {
		p.ForceGCInterval = time.Duration(v) * time.Millisecond
	}
}

func envInt(key string) (int, bool) {
	s := os.Getenv(key)
	if s == "" {
		return 0, false
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return v, true
}

func envFloat(key string) (float64, bool) {
	s := os.Getenv(key)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func envBool(key string) (bool, bool) {
	s := os.Getenv(key)
	if s == "" {
		return false, false
	}
	switch s {
	case "1", "true", "TRUE", "True", "yes", "on":
		return true, true
	case "0", "false", "FALSE", "False", "no", "off":
		return false, true
	}
	return false, false
}
