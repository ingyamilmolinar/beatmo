package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// startBenchRecording is invoked from the bench lifecycle once playback
// has actually started. It assembles a RecordingOptions from the current
// drum rows and starts a streaming session whose WAV files land in
// g.benchOutDir.
func (g *Game) startBenchRecording() {
	if g.drum == nil {
		return
	}
	instruments := make([]audio.InstrumentMeta, 0, len(g.drum.Rows))
	seen := map[string]bool{}
	for _, row := range g.drum.Rows {
		if row.Instrument == "" || seen[row.Instrument] {
			continue
		}
		seen[row.Instrument] = true
		instruments = append(instruments, audio.InstrumentMeta{
			ID:   row.Instrument,
			Name: row.Name,
		})
	}
	opts := audio.RecordingOptions{
		Format:      audio.FormatWAV24,
		Instruments: instruments,
		BPM:         g.benchBPM,
		OutputDir:   g.benchOutDir,
	}
	if err := audio.StartRecording(opts); err != nil {
		g.logger.Errorf("[BENCH] StartRecording failed: %v", err)
		return
	}
	g.logger.Infof("[BENCH] Recording started -> %s", g.benchOutDir)
}

// finishBenchRecording stops the recording, writes a perfStats snapshot
// next to the WAVs, and logs the per-channel drop counter so the run is
// self-documenting.
func (g *Game) finishBenchRecording(stats PerfStats) {
	if !audio.IsRecording() {
		return
	}
	result, err := audio.StopRecording()
	if err != nil {
		g.logger.Errorf("[BENCH] StopRecording failed: %v", err)
		return
	}
	dir := result.SessionDir
	if dir == "" {
		dir = g.benchOutDir
	}
	if _, err := audio.SaveRecording(result); err != nil {
		g.logger.Errorf("[BENCH] SaveRecording failed: %v", err)
	}
	g.logger.Infof("[BENCH] Recording stopped: drops=%d channels=%d dir=%s",
		stats.RecordingDrops, len(result.Channels), dir)

	if statsBytes, err := json.MarshalIndent(stats, "", "  "); err == nil {
		statsPath := filepath.Join(dir, "perf_stats.json")
		if err := os.WriteFile(statsPath, statsBytes, 0o644); err != nil {
			g.logger.Errorf("[BENCH] write perf_stats.json: %v", err)
		} else {
			g.logger.Infof("[BENCH] perf snapshot -> %s", statsPath)
		}
	}

	if stats.RecordingDrops > 0 {
		dropsPath := filepath.Join(dir, "audio_drops.txt")
		_ = os.WriteFile(dropsPath, []byte(fmt.Sprintf("%d\n", stats.RecordingDrops)), 0o644)
	}
}
