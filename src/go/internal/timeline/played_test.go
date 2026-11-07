package timeline

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

func TestFreezePlayedIsFirstWriteWins(t *testing.T) {
	s := NewService()
	if !s.FreezePlayed(0, 10, true, model.NodeTypeRegular) {
		t.Fatalf("first freeze reported not frozen")
	}
	if s.FreezePlayed(0, 10, false, model.NodeTypeInvisible) {
		t.Fatalf("second freeze of the same abs overwrote the played cell")
	}
	val, typ, ok := s.Played(0, 10)
	if !ok || !val || typ != model.NodeTypeRegular {
		t.Fatalf("Played(0,10) = (%v,%v,%v), want (true,Regular,true)", val, typ, ok)
	}
}

func TestPlayedRoundTripsEveryNodeTypeAndIsSparse(t *testing.T) {
	s := NewService()
	types := []model.NodeType{model.NodeTypeRegular, model.NodeTypeInvisible, model.NodeTypeSilent, model.NodeTypeMute}
	for i, typ := range types {
		for _, val := range []bool{false, true} {
			abs := 100000*i + 7
			if val {
				abs++
			}
			s.FreezePlayed(2, abs, val, typ)
			gv, gt, ok := s.Played(2, abs)
			if !ok || gv != val || gt != typ {
				t.Fatalf("abs=%d got (%v,%v,%v) want (%v,%v,true)", abs, gv, gt, ok, val, typ)
			}
		}
	}
	if _, _, ok := s.Played(2, 9); ok {
		t.Fatalf("unfrozen abs reported as played")
	}
	if _, _, ok := s.Played(5, 7); ok {
		t.Fatalf("untouched row reported a played cell")
	}
	if got := s.PlayedMax(2); got != 300008 {
		t.Fatalf("PlayedMax=%d want 300008", got)
	}
	if got := s.PlayedMax(5); got != -1 {
		t.Fatalf("PlayedMax(empty)=%d want -1", got)
	}
}

func TestRewindToTruncatesPlayedAtAndAfterAbs(t *testing.T) {
	s := NewService()
	for abs := 0; abs < 20; abs++ {
		s.RecordCommitKind(0, abs, true, model.NodeTypeRegular, CommitKindPlayback, 8, 8)
		s.FreezePlayed(0, abs, true, model.NodeTypeRegular)
	}
	s.RewindTo(0, 12)
	for abs := 0; abs < 20; abs++ {
		_, _, ok := s.Played(0, abs)
		if want := abs < 12; ok != want {
			t.Fatalf("after RewindTo(12): Played(%d) ok=%v want %v", abs, ok, want)
		}
	}
	// The rewound region re-records on the next pass.
	if !s.FreezePlayed(0, 15, false, model.NodeTypeSilent) {
		t.Fatalf("rewound abs could not be re-frozen")
	}
}

func TestClearRowDropsPlayed(t *testing.T) {
	s := NewService()
	s.RecordCommitKind(1, 3, true, model.NodeTypeRegular, CommitKindPlayback, 8, 8)
	s.FreezePlayed(1, 3, true, model.NodeTypeRegular)
	s.ClearRow(1)
	if _, _, ok := s.Played(1, 3); ok {
		t.Fatalf("ClearRow kept a played cell")
	}
}

// Otherwise the row sliding into the deleted slot reads its neighbour's past.
func TestRemoveRowShiftsEveryTier(t *testing.T) {
	s := NewService()
	s.SetArchiveMaxEntries(0)
	for row := 0; row < 3; row++ {
		typ := model.NodeType(row) // distinct per row
		for abs := 0; abs < 5; abs++ {
			s.RecordCommitKind(row, abs, row%2 == 0, typ, CommitKindPlayback, 4, 4)
			s.FreezePlayed(row, abs, row%2 == 0, typ)
		}
	}
	s.RemoveRow(1)
	check := func(row int, wantVal bool, wantTyp model.NodeType) {
		t.Helper()
		v, typ, kind, ok := s.CommittedKind(row, 2)
		if !ok || v != wantVal || typ != wantTyp || kind != CommitKindPlayback {
			t.Fatalf("row %d commit = (%v,%v,%v,%v) want (%v,%v,Playback,true)", row, v, typ, kind, ok, wantVal, wantTyp)
		}
		pv, pt, pok := s.Played(row, 2)
		if !pok || pv != wantVal || pt != wantTyp {
			t.Fatalf("row %d played = (%v,%v,%v) want (%v,%v,true)", row, pv, pt, pok, wantVal, wantTyp)
		}
	}
	check(0, true, model.NodeType(0))
	check(1, true, model.NodeType(2)) // old row 2 moved into slot 1
	if _, _, ok := s.Played(2, 2); ok {
		t.Fatalf("stale played history left in the vacated last slot")
	}
	if _, _, _, ok := s.CommittedKind(2, 2); ok {
		t.Fatalf("stale commit left in the vacated last slot")
	}
}

func TestRemoveRowShiftsArchivedHistory(t *testing.T) {
	s := NewService()
	s.SetArchiveMaxEntries(0)
	// Enough playback on row 2 to push early entries into the cold archive.
	for abs := 0; abs < immutablesPerRowMax+64; abs++ {
		s.RecordCommitKind(2, abs, true, model.NodeTypeMute, CommitKindPlayback, 4, 4)
	}
	if s.ArchiveLenForTest(2) == 0 {
		t.Fatalf("setup: expected archived entries on row 2")
	}
	s.RemoveRow(0)
	v, typ, kind, ok := s.CommittedKind(1, 0)
	if !ok || !v || typ != model.NodeTypeMute || kind != CommitKindPlayback {
		t.Fatalf("archived abs=0 of old row 2 not found on row 1: (%v,%v,%v,%v)", v, typ, kind, ok)
	}
}

// What played is history, not document: it must move out of a row and back in across undo/redo.
func TestTakeAndPutPlayedMovesARowsHistory(t *testing.T) {
	s := NewService()
	for abs := 0; abs < 10; abs++ {
		s.FreezePlayed(1, abs, abs%3 == 0, model.NodeTypeSilent)
	}
	h := s.TakePlayed(1)
	if h == nil {
		t.Fatalf("TakePlayed returned nil for a row with history")
	}
	if _, _, ok := s.Played(1, 3); ok {
		t.Fatalf("TakePlayed left the history in place")
	}
	s2 := NewService()
	s2.FreezePlayed(4, 3, false, model.NodeTypeRegular) // replaced, not merged
	s2.PutPlayed(4, h)
	for abs := 0; abs < 10; abs++ {
		v, typ, ok := s2.Played(4, abs)
		if !ok || v != (abs%3 == 0) || typ != model.NodeTypeSilent {
			t.Fatalf("abs=%d after PutPlayed = (%v,%v,%v)", abs, v, typ, ok)
		}
	}
	if got := s2.PlayedMax(4); got != 9 {
		t.Fatalf("PlayedMax after PutPlayed = %d want 9", got)
	}
	if s.TakePlayed(7) != nil {
		t.Fatalf("TakePlayed of an empty row returned history")
	}
}

func TestRemoveRowReturnsTheRemovedRowsPlayedHistory(t *testing.T) {
	s := NewService()
	s.FreezePlayed(0, 1, true, model.NodeTypeRegular)
	s.FreezePlayed(1, 1, true, model.NodeTypeMute)
	h := s.RemoveRow(1)
	if h == nil {
		t.Fatalf("RemoveRow returned no history for the removed row")
	}
	s.PutPlayed(5, h)
	if v, typ, ok := s.Played(5, 1); !ok || !v || typ != model.NodeTypeMute {
		t.Fatalf("returned history = (%v,%v,%v)", v, typ, ok)
	}
}
