package ui

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// audio_lane.go — the reconciling scheduling horizon
// (docs/superpowers/specs/2026-10-02-audio-lane-scheduling-horizon-design.md).
//
// On WASM, Go runs on the page's single main thread: a note is safe from UI
// stalls only once it has been handed to WebAudio. The lane therefore keeps
// every note of the next AudioHorizonSec COMMITTED to the engine, and keeps
// that commitment correct under real-time edits by RECONCILING rather than
// invalidating: each tick it fingerprints what decides the sound (per row and
// globally); for any row whose fingerprint moved it recomputes the window from
// current state (laneSource.NoteAt) and diffs it against what is committed —
// unchanged notes stay, changed ones are cancelled and replaced, vanished ones
// cancelled. An edit that bypasses every invalidation funnel still cannot
// leave a stale note: if it changes the sound, it changes a fingerprint.
//
// A note within CancelGuard of its start is treated as started and never
// touched (edit semantics (a): an edit reaches the next note that has not
// started). Mute stops cannot be retracted once sent, so they are committed
// just in time (laneStopLeadSec ahead), not a horizon ahead.

// audioSink is the engine side of the lane (desktop mixer / audio.js).
type audioSink interface {
	Schedule(vid uint64, n laneNote)
	// Replace swaps voice old for vid ATOMICALLY on the engine: the new voice
	// is scheduled only if the old one was really removed. An engine whose
	// clock runs ahead of the lane's (the desktop mixer renders a Read chunk
	// ahead; WebAudio may already be playing it) refuses the cancel of a voice
	// it has started — and then the replacement must not play too (a double).
	// old is the first of the oldVoices ids the replaced note owns.
	Replace(old uint64, oldVoices int, vid uint64, n laneNote)
	Cancel(vids []uint64)
	StopAt(inst instrument.ID, t float64)
	CancelAllFuture()
}

// laneSource is everything the lane reads. The Game implements it
// (audio_lane_source.go); tests use a scripted grid.
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
	// The switch rule (noteAt): whether a note's voice is rendered, a request
	// to render it, and the same note on another instrument.
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
	// VoiceNear: a voice-only edit (a knob drag) re-commits only the notes
	// starting within this window; the rest is re-derived as it enters it.
	// 0 (or ≥ Horizon) re-commits the whole committed window.
	VoiceNear float64
}

type laneStats struct {
	Scheduled, Cancelled, Late, DroppedLate, Reconciles, FullReconciles uint64
}

// laneStopLeadSec: how far ahead a mute stop is sent (it cannot be cancelled
// once sent; until then an edit can still retract it).
const laneStopLeadSec = 0.15

// laneGapResetSec: after a gap longer than this the extend window restarts at
// now (a seek / resume, not a stall to account for note by note).
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
	// voiceStale[r] > 0: row r's commitments up to that time were made under
	// an older voice revision and are re-derived as they enter VoiceNear;
	// voiceDone[r] is how far that re-derivation has reached.
	voiceStale []float64
	voiceDone  []float64
	dirtyRows  []bool
	// The switch rule (noteAt). heard[r] is the instrument the lane last
	// committed a voice of on row r; fallback[r] the instrument the row moved
	// off (since switchSince[r]), which a note plays until its own voice on
	// the new instrument is rendered — at most laneSwitchMaxWaitSec;
	// switching[r] re-derives the row every tick meanwhile; warmAsked
	// re-requests a render every laneSwitchRewarmSec (a request can be
	// dropped). Reset clears it all: the next play starts on the row's own.
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

// Reset cancels every not-yet-started lane voice and forgets all commitments
// (stop, pause, a new transport generation).
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

// DroppedLate reports whether the lane reached (row, abs) too late to start it
// on time and dropped it by its late policy (counted in laneStats.DroppedLate):
// the cell was handed to the lane, and deliberately never sounds.
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
			// The periodic safety pass: a row stale only in its voice keeps
			// re-deriving from the near window (every note still is before it
			// starts), instead of replacing its whole horizon again.
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
			// A knob edit: re-commit the notes about to play now; the rest of
			// the horizon follows as it comes near (advanceVoiceStale). A
			// whole-horizon re-commit per drag tick was ~3000 WebAudio voice
			// starts/s, and near-due replacements queued behind far ones.
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
		// A commitment OUTSIDE the re-walked abs range can only come from an
		// abs↔time mapping that has since moved (a seek or re-anchor keeps the
		// transport generation): under the current mapping no retractable note
		// lies there, so it is stale — cancel it.
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

// advanceVoiceStale re-derives, for a row whose commitments are stale in their
// voice only, the notes that have just come within VoiceNear — only those:
// the near window itself is re-derived by the (rate-limited) edit path —
// until everything committed under the old voice (voiceStale[r]) is reached.
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

// reconcileRow re-derives row r over the committed window, whose abs range
// [lo, hi] (Steps(guard, hiTime)) the caller computed once for every row.
func (l *audioLane) reconcileRow(r int, guard float64, lo, hi int) {
	l.reconcileRowUpTo(r, guard, lo, hi, l.hiTime)
}

// reconcileRowUpTo re-derives only the cells of [lo, hi] whose committed or
// current note starts by upTo — Steps' one-step margin must not reach (and
// replace, or cancel) a note past a partial window. upTo is hiTime for the
// whole committed window.
func (l *audioLane) reconcileRowUpTo(r int, guard float64, lo, hi int, upTo float64) {
	for abs := lo; abs <= hi; abs++ {
		key := [2]int{r, abs}
		e, had := l.committed[key]
		n, want := l.noteAt(r, abs)
		if (!had || e.note.Time > upTo) && (!want || n.Time > upTo) {
			continue
		}
		if had && !l.retractable(e, guard) && !(e.vid == 0 && e.note.Kind == laneSound) {
			// Started / sent: leave it — unless the abs↔time mapping moved
			// (a backward seek; every undo/redo while playing seeks to the
			// beat start) and this cell now lies in the FUTURE. Then the entry
			// is history from the old mapping, not this cell's note: forget it
			// (no cancel — it already played) and evaluate the cell afresh.
			// LateTol keeps a just-started note that a BPM re-anchor nudged by
			// a few ms from playing twice.
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

// commit hands a note to the engine. extend=true for first-time commits from
// the extend pass, where a note already due is subject to the late policy.
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

// laneSwitchMaxWaitSec bounds how long a row keeps its previous instrument
// for notes whose new voice is not rendered: a render that fails or never
// lands must not leave the user's choice silently ignored.
// laneSwitchRewarmSec is how often an unrendered voice is requested again.
const (
	laneSwitchMaxWaitSec = 2.0
	laneSwitchRewarmSec  = 0.25
)

type laneWarmKey struct {
	inst  instrument.ID
	pitch float64
}

// noteAt is the source's note under the switch rule (spec 2026-10-31 §1.3):
// after a row's instrument changes, a note whose own voice on the new
// instrument is not rendered yet keeps the instrument the row moved off —
// never a note on a voice that is not there (it would start late, or not at
// all). Readiness is per note: a melodic instrument renders each pitch apart.
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
		// Give up waiting: the move is done; re-derive the row once so the
		// notes still committed on the old instrument follow.
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

// mint reserves a contiguous block of voice ids for a note (one per voice)
// and returns its first id. For a single voice this is nextVID++ exactly.
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
