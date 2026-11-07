package ui

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// A note is safe from main-thread stalls only once WebAudio has it, so the lane keeps the next
// AudioHorizonSec committed and RECONCILES on edits: changed fingerprints re-derive rows, and the diff
// cancels or replaces notes. A note within CancelGuard counts as started; mute stops go just in time.

// audioSink is the engine side of the lane (desktop mixer / audio.js).
type audioSink interface {
	Schedule(vid uint64, n laneNote)
	// ATOMIC: the new voice plays only if the old one was really removed. An engine whose clock runs
	// ahead refuses to cancel a started voice, and then the replacement must not play (a double).
	Replace(old uint64, oldVoices int, vid uint64, n laneNote)
	Cancel(vids []uint64)
	StopAt(inst instrument.ID, t float64)
	CancelAllFuture()
}

type laneSource interface {
	Playing() bool
	Now() float64
	TransportGen() uint64
	GlobalKey() uint64 // inputs shared by every row
	Rows() int
	RowKey(row int) uint64 // per-row inputs, including the instrument's voice revision
	VoiceRevOnly(row int) bool
	Steps(from, to float64) (lo, hi int, ok bool) // abs range whose Time ∈ [from, to]
	NoteAt(row, abs int) (laneNote, bool)
	// The switch rule: a note on an unrendered voice plays the previous instrument instead.
	VoiceReady(n laneNote) bool
	Warm(n laneNote)
	Retarget(n laneNote, inst instrument.ID) laneNote
}

type laneConfig struct {
	Horizon          float64
	Guard            float64
	LateTol          float64
	FullReconcile    float64
	VoiceRecommitMin float64
	// 0 (or ≥ Horizon) re-commits the whole window.
	VoiceNear float64
}

type laneStats struct {
	Scheduled, Cancelled, Late, DroppedLate, Reconciles, FullReconciles uint64
}

// A sent stop cannot be retracted; until then an edit can still change it.
const laneStopLeadSec = 0.15

// Past this gap it is a seek or resume, not a stall to account for note by note.
const laneGapResetSec = 2.0

type laneEntry struct {
	vid  uint64 // first voice id of the note's block; 0: no engine voice (stop, or dropped late)
	note laneNote
	sent bool // stops: StopAt dispatched (no longer retractable)
}

type audioLane struct {
	src  laneSource
	sink audioSink
	cfg  laneConfig

	nextVID      uint64
	committed    map[[2]int]laneEntry
	pendingStops [][2]int
	globalKey    uint64
	rowKeys      []uint64
	lastVoiceRec []float64
	pendingVoice []bool
	// Commitments made under an older voice, re-derived as they come within VoiceNear.
	voiceStale []float64
	voiceDone  []float64
	dirtyRows  []bool
	// Until a note's voice on the new instrument is rendered, the row keeps the old instrument, for at
	// most laneSwitchMaxWaitSec; renders are re-requested since a request can be dropped. Reset clears it.
	heard       []instrument.ID
	fallback    []instrument.ID
	switchSince []float64
	switching   []bool
	warmAsked   map[laneWarmKey]float64

	gen       uint64
	genInit   bool
	hiTime    float64
	lastFull  float64
	lastPrune float64
	lastRows  int
	stats     laneStats
	cancels   []uint64
}

func newAudioLane(src laneSource, sink audioSink, cfg laneConfig) *audioLane {
	return &audioLane{src: src, sink: sink, cfg: cfg, committed: map[[2]int]laneEntry{}}
}

func (l *audioLane) Reset() {
	l.sink.CancelAllFuture()
	l.committed = map[[2]int]laneEntry{}
	l.pendingStops = l.pendingStops[:0]
	l.hiTime = 0
	l.rowKeys = nil
	l.heard = nil
	l.fallback = nil
	l.switchSince = nil
	l.switching = nil
	l.warmAsked = nil
}

// Committed reports what the lane committed for (row, abs).
func (l *audioLane) Committed(row, abs int) (laneNote, bool) {
	e, ok := l.committed[[2]int{row, abs}]
	if !ok || (e.note.Kind == laneSound && e.vid == 0) {
		return laneNote{}, false
	}
	return e.note, true
}

// Handed to the lane and deliberately never sounded.
func (l *audioLane) DroppedLate(row, abs int) bool {
	e, ok := l.committed[[2]int{row, abs}]
	return ok && e.note.Kind == laneSound && e.vid == 0
}

func (l *audioLane) Stats() laneStats { return l.stats }

func (l *audioLane) Tick() {
	s := l.src
	if !s.Playing() {
		if len(l.committed) > 0 || l.hiTime > 0 {
			l.Reset()
		}
		return
	}
	if g := s.TransportGen(); !l.genInit || g != l.gen {
		if l.genInit {
			l.Reset()
		}
		l.gen, l.genInit = g, true
	}
	now := s.Now()
	rows := s.Rows()
	if len(l.rowKeys) != rows {
		l.rowKeys = make([]uint64, rows)
		l.lastVoiceRec = make([]float64, rows)
		l.pendingVoice = make([]bool, rows)
		l.voiceStale = make([]float64, rows)
		l.voiceDone = make([]float64, rows)
		l.dirtyRows = make([]bool, rows)
		l.heard = make([]instrument.ID, rows)
		l.fallback = make([]instrument.ID, rows)
		l.switchSince = make([]float64, rows)
		l.switching = make([]bool, rows)
		l.globalKey = ^uint64(0) // force a full reconcile
	}
	full, remapped := false, false
	if gk := s.GlobalKey(); gk != l.globalKey {
		l.globalKey = gk
		full, remapped = true, true
	}
	if now-l.lastFull >= l.cfg.FullReconcile {
		full = true // safety net: bounded latency even for a missed fingerprint
	}
	guard := now + l.cfg.Guard
	horizon := now + l.cfg.Horizon

	// 1) Reconcile rows whose inputs moved, over the committed window.
	var lo, hi int
	win := false
	if l.hiTime > guard {
		lo, hi, win = s.Steps(guard, l.hiTime)
	}
	// The near window a voice-only change re-derives at once.
	var nlo, nhi int
	nwin := false
	nearOn := l.cfg.VoiceNear > 0 && l.cfg.VoiceNear < l.cfg.Horizon
	if nearOn && win {
		if nearHi := math.Min(now+l.cfg.VoiceNear, l.hiTime); nearHi > guard {
			nlo, nhi, nwin = s.Steps(guard, nearHi)
		}
	}
	anyDirty := false
	for r := 0; r < rows; r++ {
		l.dirtyRows[r] = false
		dirty := full || l.switching[r]
		l.switching[r] = false // noteAt sets it again while the voice is not ready
		voiceOnly := false
		if k := s.RowKey(r); k != l.rowKeys[r] {
			if !full && s.VoiceRevOnly(r) && now-l.lastVoiceRec[r] < l.cfg.VoiceRecommitMin {
				l.pendingVoice[r] = true // knob drag: rate-limited
			} else {
				voiceOnly = s.VoiceRevOnly(r)
				l.rowKeys[r] = k
				dirty = true
			}
		} else if l.pendingVoice[r] && now-l.lastVoiceRec[r] >= l.cfg.VoiceRecommitMin {
			dirty, voiceOnly = true, true
		} else if full {
			// A voice-only stale row keeps re-deriving its near window instead of replacing the whole horizon.
			voiceOnly = l.voiceStale[r] > 0
		}
		if !dirty {
			l.advanceVoiceStale(r, now, guard)
			continue
		}
		if full {
			l.rowKeys[r] = s.RowKey(r)
		}
		l.pendingVoice[r] = false
		l.lastVoiceRec[r] = now
		l.stats.Reconciles++
		if nearOn && voiceOnly && !remapped {
			// Only the near window now; the rest follows as it comes near. Re-committing the whole horizon per
			// drag tick made thousands of voice starts a second, with near notes queued behind far ones.
			if l.voiceStale[r] < l.hiTime {
				l.voiceStale[r] = l.hiTime
			}
			nearHi := math.Min(now+l.cfg.VoiceNear, l.hiTime)
			if nwin {
				l.reconcileRowUpTo(r, guard, nlo, nhi, nearHi)
			}
			l.voiceDone[r] = nearHi
			continue
		}
		l.voiceStale[r] = 0
		l.dirtyRows[r], anyDirty = true, true
		if win {
			l.reconcileRow(r, guard, lo, hi)
		}
	}
	if anyDirty {
		// A commitment outside the re-walked range can only come from an abs↔time mapping that moved (a
		// seek keeps the generation), so it is stale.
		for key, e := range l.committed {
			if r := key[0]; r >= rows || !l.dirtyRows[r] || (win && key[1] >= lo && key[1] <= hi) {
				continue
			}
			if l.retractable(e, guard) || (e.vid == 0 && e.note.Kind == laneSound && e.note.Time > guard) {
				l.cancel(key, e)
			}
		}
	}
	if full {
		l.lastFull = now
		l.stats.FullReconciles++
	}
	if rows < l.lastRows {
		for key, e := range l.committed {
			if key[0] >= rows && l.retractable(e, guard) {
				l.cancel(key, e)
			}
		}
	}
	l.lastRows = rows

	// 2) Extend the window to the horizon (new cells only).
	from := l.hiTime
	if from == 0 || now-from > laneGapResetSec {
		from = now - l.cfg.LateTol
	}
	if lo, hi, ok := s.Steps(from, horizon); ok {
		for r := 0; r < rows; r++ {
			for abs := lo; abs <= hi; abs++ {
				key := [2]int{r, abs}
				if _, done := l.committed[key]; done {
					continue
				}
				n, ok := l.noteAt(r, abs)
				if !ok || n.Time < from || n.Time > horizon {
					continue
				}
				l.commit(key, n, now, guard, true)
			}
		}
		l.hiTime = horizon
	}

	// 3) Send mute stops that are now due within their lead.
	l.sendDueStops(now)
	l.flushCancels()

	// 4) Forget entries well in the past.
	if now-l.lastPrune >= 0.25 {
		l.lastPrune = now
		for key, e := range l.committed {
			if e.note.Time < now-1.0 {
				delete(l.committed, key)
			}
		}
	}
}

// Only the notes just entering VoiceNear (the near window itself goes through the edit path).
func (l *audioLane) advanceVoiceStale(r int, now, guard float64) {
	if l.voiceStale[r] == 0 {
		return
	}
	from := math.Max(guard, l.voiceDone[r])
	to := math.Min(now+l.cfg.VoiceNear, l.hiTime)
	if to > from {
		if lo, hi, ok := l.src.Steps(from, to); ok {
			l.reconcileRowUpTo(r, guard, lo, hi, to)
		}
		l.voiceDone[r] = to
	}
	if l.voiceDone[r] >= l.voiceStale[r] {
		l.voiceStale[r] = 0
	}
}

// retractable: the engine has not started (or been handed) this entry.
func (l *audioLane) retractable(e laneEntry, guard float64) bool {
	if e.note.Kind == laneStop {
		return !e.sent
	}
	return e.vid != 0 && e.note.Time > guard
}

func (l *audioLane) reconcileRow(r int, guard float64, lo, hi int) {
	l.reconcileRowUpTo(r, guard, lo, hi, l.hiTime)
}

// Steps' one-step margin must not reach past a partial window.
func (l *audioLane) reconcileRowUpTo(r int, guard float64, lo, hi int, upTo float64) {
	for abs := lo; abs <= hi; abs++ {
		key := [2]int{r, abs}
		e, had := l.committed[key]
		n, want := l.noteAt(r, abs)
		if (!had || e.note.Time > upTo) && (!want || n.Time > upTo) {
			continue
		}
		if had && !l.retractable(e, guard) && !(e.vid == 0 && e.note.Kind == laneSound) {
			// Started or sent: leave it, unless a backward seek put this cell in the FUTURE. Then the entry is
			// history: forget it (no cancel) and evaluate afresh; LateTol stops a nudged note playing twice.
			if !(want && n.Time > guard && math.Abs(n.Time-e.note.Time) > l.cfg.LateTol) {
				continue
			}
			delete(l.committed, key)
			had = false
		}
		if want && (n.Time <= guard || n.Time > l.hiTime) {
			want = false
		}
		switch {
		case had && want && n.Hash == e.note.Hash:
			// unchanged
		case had && want && e.vid != 0 && e.note.Kind == laneSound && n.Kind == laneSound:
			vid := l.mint(&n)
			l.sink.Replace(e.vid, e.note.voiceCount(), vid, n)
			l.committed[key] = laneEntry{vid: vid, note: n, sent: true}
			l.heard[r] = n.Inst
			l.stats.Scheduled++
			l.stats.Cancelled++
		case had && want:
			l.cancel(key, e)
			l.commit(key, n, 0, guard, false)
		case had:
			l.cancel(key, e)
		case want:
			l.commit(key, n, 0, guard, false)
		}
	}
}

// extend=true: a note already due falls under the late policy.
func (l *audioLane) commit(key [2]int, n laneNote, now, guard float64, extend bool) {
	if n.Kind == laneStop {
		l.committed[key] = laneEntry{note: n}
		l.pendingStops = append(l.pendingStops, key)
		return
	}
	if n.Time <= guard {
		if extend && n.Time >= now-l.cfg.LateTol {
			n.Time = guard // play now: late, counted
			l.stats.Late++
		} else {
			l.stats.DroppedLate++
			l.committed[key] = laneEntry{note: n} // remembered: never rushed later
			return
		}
	}
	vid := l.mint(&n)
	l.sink.Schedule(vid, n)
	l.committed[key] = laneEntry{vid: vid, note: n, sent: true}
	l.stats.Scheduled++
	l.heard[key[0]] = n.Inst
}

// A render that never lands must not leave the user's choice silently ignored.
const (
	laneSwitchMaxWaitSec = 2.0
	laneSwitchRewarmSec  = 0.25
)

type laneWarmKey struct {
	inst  instrument.ID
	pitch float64
}

// Never a note on a voice that is not rendered (it would start late or not at all); readiness is
// per note, since melodic instruments render each pitch apart.
func (l *audioLane) noteAt(r, abs int) (laneNote, bool) {
	n, ok := l.src.NoteAt(r, abs)
	if !ok || n.Kind != laneSound {
		return n, ok
	}
	now := l.src.Now()
	if h := l.heard[r]; h != "" && h != n.Inst && h != l.fallback[r] {
		l.fallback[r], l.switchSince[r] = h, now // the row moved off h
	}
	fb := l.fallback[r]
	if fb == "" || fb == n.Inst {
		return n, true
	}
	if now-l.switchSince[r] > laneSwitchMaxWaitSec {
		// Stop waiting: re-derive once so notes still on the old instrument follow.
		l.fallback[r], l.heard[r], l.switching[r] = "", n.Inst, true
		return n, true
	}
	if l.src.VoiceReady(n) {
		return n, true
	}
	l.switching[r] = true
	if k := (laneWarmKey{n.Inst, n.Pitch}); now-l.warmAsked[k] >= laneSwitchRewarmSec || l.warmAsked[k] == 0 {
		if l.warmAsked == nil {
			l.warmAsked = map[laneWarmKey]float64{}
		}
		l.warmAsked[k] = now
		l.src.Warm(n)
	}
	return l.src.Retarget(n, fb), true
}

// One id per voice; for a single voice exactly nextVID++.
func (l *audioLane) mint(n *laneNote) uint64 {
	base := l.nextVID + 1
	l.nextVID += uint64(n.voiceCount())
	return base
}

func (l *audioLane) cancel(key [2]int, e laneEntry) {
	if e.vid != 0 {
		l.cancels = append(l.cancels, laneVoiceIDs(e.vid, &e.note)...)
		l.stats.Cancelled++
	}
	delete(l.committed, key)
}

func (l *audioLane) flushCancels() {
	if len(l.cancels) == 0 {
		return
	}
	l.sink.Cancel(l.cancels)
	l.cancels = l.cancels[:0]
}

func (l *audioLane) sendDueStops(now float64) {
	if len(l.pendingStops) == 0 {
		return
	}
	keep := l.pendingStops[:0]
	for _, key := range l.pendingStops {
		e, ok := l.committed[key]
		if !ok || e.note.Kind != laneStop || e.sent {
			continue // retracted or replaced
		}
		if e.note.Time > now+laneStopLeadSec {
			keep = append(keep, key)
			continue
		}
		l.sink.StopAt(e.note.Inst, e.note.Time)
		e.sent = true
		l.committed[key] = e
	}
	l.pendingStops = keep
}
