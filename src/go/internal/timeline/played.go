package timeline

import "github.com/ingyamilmolinar/beatmo/core/model"

// Played-cell tier: the immutable record of what the grid SHOWED for each cell
// at the moment the playhead passed it. It is the single source the DrumView
// renders past cells from, so nothing that happens afterwards — a node edit,
// a solo, a dropped or late highlight, a predictor rebuild — can reach a cell
// that has already played.
//
// Commits (ring → immutables → archive) record provenance for the scheduler
// and parity; they are written by several paths and some kinds are mutable by
// design. The played tier is deliberately separate and much simpler: one byte
// per cell, append-mostly, first write wins, never demoted.
//
// Storage is chunked so a sparse or far-seeked row costs only the chunks it
// touches: 1 byte per cell per row (≈1 MB per million subdivisions per row),
// kept for the whole session (full-history retention; cleared on Stop,
// truncated by a backward seek).

const playedChunkBits = 12
const playedChunkSize = 1 << playedChunkBits // 4096 cells per chunk

// Cell byte layout: bit 0 = present, bit 1 = step on, bits 2..7 = node type.
const (
	playedPresent = 1 << 0
	playedOn      = 1 << 1
	playedTypeSh  = 2
)

// PlayedHistory is one row's played cells, detached from any row index so it
// can follow the row through a document restore (see TakePlayed/PutPlayed).
type PlayedHistory = playedLog

type playedLog struct {
	chunks map[int][]uint8
	max    int // highest frozen abs, -1 when empty
}

func newPlayedLog() *playedLog {
	return &playedLog{chunks: make(map[int][]uint8), max: -1}
}

func encodePlayed(val bool, typ model.NodeType) uint8 {
	b := uint8(playedPresent) | uint8(typ)<<playedTypeSh
	if val {
		b |= playedOn
	}
	return b
}

func (p *playedLog) get(abs int) (bool, model.NodeType, bool) {
	if p == nil || abs < 0 {
		return false, model.NodeTypeInvisible, false
	}
	c := p.chunks[abs>>playedChunkBits]
	if c == nil {
		return false, model.NodeTypeInvisible, false
	}
	b := c[abs&(playedChunkSize-1)]
	if b&playedPresent == 0 {
		return false, model.NodeTypeInvisible, false
	}
	return b&playedOn != 0, model.NodeType(b >> playedTypeSh), true
}

// freeze stores the cell unless abs is already frozen. Reports whether it wrote.
func (p *playedLog) freeze(abs int, val bool, typ model.NodeType) bool {
	if abs < 0 {
		return false
	}
	ci := abs >> playedChunkBits
	c := p.chunks[ci]
	if c == nil {
		c = make([]uint8, playedChunkSize)
		p.chunks[ci] = c
	}
	i := abs & (playedChunkSize - 1)
	if c[i]&playedPresent != 0 {
		return false
	}
	c[i] = encodePlayed(val, typ)
	if abs > p.max {
		p.max = abs
	}
	return true
}

// truncateFrom forgets every cell at or after abs (backward seek: the rewound
// region is re-simulated and re-recorded on the next pass).
func (p *playedLog) truncateFrom(abs int) {
	if p == nil || abs > p.max {
		return
	}
	if abs < 0 {
		abs = 0
	}
	first := abs >> playedChunkBits
	for ci, c := range p.chunks {
		switch {
		case ci > first:
			delete(p.chunks, ci)
		case ci == first:
			clear(c[abs&(playedChunkSize-1):])
		}
	}
	p.max = -1
	for ci, c := range p.chunks {
		for i := len(c) - 1; i >= 0; i-- {
			if c[i]&playedPresent != 0 {
				if a := ci<<playedChunkBits | i; a > p.max {
					p.max = a
				}
				break
			}
		}
	}
}

// FreezePlayed records what the grid rendered for (row, abs) as the playhead
// passed it. First write wins: a cell that has played is never rewritten.
// Reports whether this call stored the cell.
func (s *Service) FreezePlayed(row, abs int, val bool, typ model.NodeType) bool {
	if row < 0 || abs < 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.played == nil {
		s.played = make(map[int]*playedLog)
	}
	p := s.played[row]
	if p == nil {
		p = newPlayedLog()
		s.played[row] = p
	}
	return p.freeze(abs, val, typ)
}

// Played returns the frozen played cell for (row, abs), if any.
func (s *Service) Played(row, abs int) (bool, model.NodeType, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.played[row].get(abs)
}

// PlayedMax returns the highest frozen abs for row, or -1.
func (s *Service) PlayedMax(row int) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if p := s.played[row]; p != nil {
		return p.max
	}
	return -1
}

// TakePlayed detaches and returns row's played history (nil when empty).
func (s *Service) TakePlayed(row int) *PlayedHistory {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.played[row]
	delete(s.played, row)
	return p
}

// PutPlayed installs h as row's played history, replacing whatever the row
// had. A nil h clears it.
func (s *Service) PutPlayed(row int, h *PlayedHistory) {
	if row < 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if h == nil {
		delete(s.played, row)
		return
	}
	if s.played == nil {
		s.played = make(map[int]*playedLog)
	}
	s.played[row] = h
}

// RemoveRow deletes row's history and shifts every later row's history down
// one index, across every tier. Rows are addressed by index, so without this
// the row that slides into the vacated slot would read its neighbour's past.
// The removed row's played history is returned so an undo can bring it back.
func (s *Service) RemoveRow(row int) *PlayedHistory {
	if row < 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := s.played[row]
	if row < len(s.rows) {
		s.rows = append(s.rows[:row], s.rows[row+1:]...)
	}
	s.immutables = shiftRowMap(s.immutables, row)
	s.archives = shiftRowMap(s.archives, row)
	s.superseded = shiftRowMap(s.superseded, row)
	s.played = shiftRowMap(s.played, row)
	return removed
}

func shiftRowMap[V any](m map[int]V, removed int) map[int]V {
	if len(m) == 0 {
		return m
	}
	out := make(map[int]V, len(m))
	for r, v := range m {
		switch {
		case r < removed:
			out[r] = v
		case r > removed:
			out[r-1] = v
		}
	}
	return out
}
