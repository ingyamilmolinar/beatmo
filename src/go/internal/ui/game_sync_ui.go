package ui

import (
	"fmt"
	"math"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

// syncUIToTime aligns UI pulses and counters to the current engine timeline
// so that the visual state matches playback even after heavy UI work.
func (g *Game) syncUIToTime() {
	if !g.Playing() {
		return
	}
	div := g.grid.MaxDiv()
	if div <= 0 {
		return
	}
	// If playback was enabled programmatically (tests) without going through
	// the Play button path, initialize the timebase so visuals can advance.
	if g.state.PlayStart().IsZero() {
		if runningUnderGoTest() {
			return
		}
		g.state.SetPlayStart(time.Now())
		if n := audio.Now(); n > 0 {
			g.state.SetAudioStart(n)
		}
	}
	// Absolute position in subdivisions (float) from wall-clock timeline.
	bpm := g.AppliedBPM()
	if bpm <= 0 {
		bpm = g.bpm
	}
	if bpm <= 0 {
		return
	}
	var dtSec float64
	audioStart := g.state.AudioStart()
	if n := audio.Now(); n > 0 && audioStart > 0 {
		dtSec = n - audioStart
	} else if playStart := g.state.PlayStart(); !playStart.IsZero() {
		dtSec = time.Since(playStart).Seconds()
	} else {
		return
	}
	absBeats := g.state.BeatBase() + dtSec*float64(bpm)/60.0
	absDivF := absBeats * float64(div)
	target := int(math.Floor(absDivF))
	// Clamp negative just in case.
	if target < 0 {
		target = 0
	}
	// Ensure arrays sized to row count.
	if len(g.nextBeatIdxs) != len(g.drum.Rows) {
		g.nextBeatIdxs = make([]int, len(g.drum.Rows))
	}
	for row := range g.drum.Rows {
		// Ensure path exists.
		if row >= len(g.beatInfosByRow) || len(g.beatInfosByRow[row]) == 0 {
			continue
		}
		p := g.pulseForRow(row)
		if p == nil {
			// Create a pulse aligned at target without scheduling audio.
			infoFrom := g.beatInfoAtRow(row, target)
			infoTo := g.beatInfoAtRow(row, target+1)
			unit := g.grid.Unit()
			x1 := float64(infoFrom.I) * unit
			y1 := float64(infoFrom.J) * unit
			x2 := float64(infoTo.I) * unit
			y2 := float64(infoTo.J) * unit
			dist := hypot(x2-x1, y2-y1)
			seg := dist / g.grid.Step
			if seg <= 0 {
				seg = 1
			}
			p = &pulse{
				x1: x1, y1: y1, x2: x2, y2: y2,
				fromBeatInfo: infoFrom, toBeatInfo: infoTo,
				pathIdx: g.wrapBeatIndexRow(row, target+1),
				lastIdx: target,
				from:    g.nodeByID(infoFrom.NodeID), to: g.nodeByID(infoTo.NodeID),
				path: g.beatInfosByRow[row], row: row, segBeats: seg,
			}
			g.activePulses = append(g.activePulses, p)
			if row == 0 && g.activePulse == nil {
				g.activePulse = p
			}
		} else if p.lastIdx != target {
			// Re-anchor to target if we fell behind.
			infoFrom := g.beatInfoAtRow(row, target)
			infoTo := g.beatInfoAtRow(row, target+1)
			unit := g.grid.Unit()
			p.x1 = float64(infoFrom.I) * unit
			p.y1 = float64(infoFrom.J) * unit
			p.x2 = float64(infoTo.I) * unit
			p.y2 = float64(infoTo.J) * unit
			p.fromBeatInfo = infoFrom
			p.toBeatInfo = infoTo
			p.pathIdx = g.wrapBeatIndexRow(row, target+1)
			p.lastIdx = target
			dist := hypot(p.x2-p.x1, p.y2-p.y1)
			seg := dist / g.grid.Step
			if seg <= 0 {
				seg = 1
			}
			p.segBeats = seg
		}
		// Freeze newly passed indices so past never changes.
		if len(g.frozenUpToByRow) != len(g.drum.Rows) {
			g.frozenUpToByRow = make([]int, len(g.drum.Rows))
			for i := range g.frozenUpToByRow {
				g.frozenUpToByRow[i] = -1
			}
		}
		upTo := g.frozenUpToByRow[row]
		// Freeze everything up to and including 'target'.
		freezeTarget := target
		if row < len(g.nextBeatIdxs) {
			next := g.nextBeatIdxs[row]
			if next > 0 && freezeTarget >= next {
				freezeTarget = next - 1
			}
		}
		if freezeTarget > upTo {
			need := target + 1
			// Window the predictor actually has values for. The sliding window
			// only ever advances (Ensure does not rewind), so after a backward
			// seek/replay the playhead can land BELOW windowStart: indices there
			// have been evicted and AudibleAt/VisibleAt/TriggeredAt return false
			// meaning "not computed", NOT "no hit". Freezing that false as an
			// immutable playback commit permanently masks real hits — most
			// visibly the start node at abs=0 — which then trips the
			// scheduler-vs-DrumView parity watchdog on the next playback. We stop
			// freezing at the first index outside the window and let a later
			// frame freeze it once refreshDrumRow has re-anchored the window over
			// the playhead. Regression: TestSwitchInstrumentAfterStopReplayParity.
			predWinStart, predWinEnd := 0, need
			if g.engine != nil && g.engine.Predictor != nil {
				g.engine.Predictor.Ensure(need)
				predWinStart = g.engine.Predictor.WindowStart()
				predWinEnd = g.engine.Predictor.Horizon()
			}
			frozeUpTo := upTo
			for j := upTo + 1; j <= freezeTarget; j++ {
				if j < predWinStart || j >= predWinEnd {
					break
				}
				bi := g.beatInfoAtRow(row, j)
				on := false
				if g.engine != nil && g.engine.Predictor != nil {
					switch bi.NodeType {
					case model.NodeTypeMute:
						on = g.engine.Predictor.TriggeredAt(row, j)
					default:
						on = g.engine.Predictor.VisibleAt(row, j)
					}
				}
				// Once an index has been passed by the playhead it should be
				// immutable in the UI, regardless of whether the engine has
				// produced an audible sample yet. Treat all frozen entries as
				// playback commits so later edits cannot mutate the past.
				g.recordTimelineCommitKind(row, j, on, bi.NodeType, timeline.CommitKindPlayback)
				frozeUpTo = j
			}
			g.frozenUpToByRow[row] = frozeUpTo
		}
		// Set animation progress precisely to current fraction within segment.
		// absDivF and lastIdx are in subdivisions; convert to beats first.
		prevT := p.t
		delta := (absDivF - float64(p.lastIdx)) / float64(div)
		if delta < 0 {
			delta = 0
		}
		if p.segBeats <= 0 {
			p.segBeats = 1
		}
		t := delta / p.segBeats
		if t < 0 {
			t = 0
		} else if t > 0.999 {
			t = 0.999
		}
		// Clamp to keep animation monotonic within a segment unless we wrapped
		// very near the end (t ~1 -> 0).
		if prevT > 0 && t < prevT && !(prevT > 0.9 && t < 0.2) {
			t = prevT
		}
		p.t = t
		// Update counters and visual highlight state without queuing audio.
		g.nextBeatIdxs[row] = target + 1
		if row == 0 {
			g.setPrimaryStep(target)
			whole := float64(target) / float64(div)
			fracBeat := absBeats - math.Floor(absBeats)
			if fracBeat < 0 {
				fracBeat = 0
			}
			g.state.SetLastBeat(whole)
			g.state.SetLastProg(fracBeat)
			g.state.SetLastDisplayBeat(absBeats)
		}
		beatDuration := int64(60.0 / float64(max1(g.AppliedBPM())) * ebitenTPS)
		g.highlightVisual(row, target, p.fromBeatInfo, beatDuration)
	}
}

func (g *Game) advancePulse(p *pulse) bool {
	beatDuration := int64(60.0 / float64(g.bpm) * ebitenTPS)

	// The pulse has arrived at p.toBeatInfo. Highlight it.
	arrivalBeatInfo := p.toBeatInfo
	arrivalPathIdx := p.pathIdx

	g.logger.Tracef("[pulse/advance] arrived beat=%d info=%+v", arrivalPathIdx, arrivalBeatInfo)
	if p.row < len(g.drum.Rows) {
		origin := g.drum.Rows[p.row].Origin
		if origin != model.InvalidNodeID && arrivalBeatInfo.NodeID == origin &&
			p.row < len(g.nextOriginIdxByRow) && p.row < len(g.originIdxsByRow) {
			positions := g.originIdxsByRow[p.row]
			if len(positions) > 0 {
				seq := g.nextOriginIdxByRow[p.row]
				expectedIdx := positions[seq%len(positions)]
				if arrivalPathIdx != expectedIdx {
					g.logger.Errorf("pulse jumped to origin out of order: row=%d idx=%d expected=%d", p.row, arrivalPathIdx, expectedIdx)
					panic(fmt.Sprintf("pulse jumped to origin out of order: row=%d idx=%d expected=%d", p.row, arrivalPathIdx, expectedIdx))
				}
				seq++
				if seq >= len(positions) {
					seq = 0
					if p.row >= len(g.loopCountByRow) {
						g.loopCountByRow = make([]int, len(g.drum.Rows))
					}
					g.loopCountByRow[p.row]++
				}
				g.nextOriginIdxByRow[p.row] = seq
			}
		}
	}

	if len(g.nextBeatIdxs) != len(g.drum.Rows) {
		g.nextBeatIdxs = make([]int, len(g.drum.Rows))
	}
	idx := g.nextBeatIdxs[p.row]
	// When beat paths are not initialized (some unit tests construct pulses
	// directly), avoid calling highlightBeat which depends on prediction
	// buffers and graph paths. Still advance counters/pulse state.
	if p.row < len(g.beatInfosByRow) && len(g.beatInfosByRow[p.row]) > 0 {
		g.setLastTriggered(p.row, arrivalBeatInfo.NodeID, true)
		g.highlightBeat(p.row, idx, arrivalBeatInfo, beatDuration)
	}
	p.lastIdx = idx
	if p.row == 0 {
		g.setPrimaryStep(idx)
	}
	g.nextBeatIdxs[p.row] = idx + 1

	// Advance pathIdx for the *next* pulse segment
	p.pathIdx++

	// If the end of the path is reached, check for a loop.
	path := p.path
	if p.pathIdx >= len(path) {
		if g.isLoopByRow[p.row] {
			p.pathIdx = g.loopStartByRow[p.row]
		} else {
			return false
		}
	}

	// Set up the next segment of the pulse's journey.
	prevIdx := p.pathIdx - 1
	if prevIdx < 0 {
		if g.isLoopByRow[p.row] {
			prevIdx = len(path) - 1
		} else {
			return false
		}
	}
	if g.isLoopByRow[p.row] && p.pathIdx == g.loopStartByRow[p.row] {
		prevIdx = len(path) - 1
	}
	p.fromBeatInfo = path[prevIdx]
	p.toBeatInfo = path[p.pathIdx]
	// Seam fix: if wrapping to loop start lands on an invisible marker,
	// jump to the first regular node inside the loop.
	if g.isLoopByRow[p.row] && p.pathIdx == g.loopStartByRow[p.row] && p.toBeatInfo.NodeType == model.NodeTypeInvisible {
		j := p.pathIdx
		for j < len(path) && path[j].NodeType == model.NodeTypeInvisible {
			j++
		}
		if j < len(path) {
			p.pathIdx = j
			p.toBeatInfo = path[p.pathIdx]
		}
	}
	p.from = g.nodeByID(p.fromBeatInfo.NodeID)
	p.to = g.nodeByID(p.toBeatInfo.NodeID)

	// Set pulse start and end coordinates for animation using beat info
	unit := g.grid.Unit()
	p.x1 = float64(p.fromBeatInfo.I) * unit
	p.y1 = float64(p.fromBeatInfo.J) * unit
	p.x2 = float64(p.toBeatInfo.I) * unit
	p.y2 = float64(p.toBeatInfo.J) * unit
	dist := hypot(p.x2-p.x1, p.y2-p.y1)
	beats := dist / g.grid.Step
	if beats <= 0 {
		beats = 1
	}
	p.segBeats = beats
	p.t = 0 // Reset animation progress

	return true
}
