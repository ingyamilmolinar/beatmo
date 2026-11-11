//go:build test

package ui

import "testing"

func TestNotificationStore_RingBoundAndOrder(t *testing.T) {
	s := newNotificationStore(3)
	for i := 0; i < 5; i++ {
		s.Push(notification{text: string(rune('a' + i)), unixMs: int64(i)})
	}
	h := s.History()
	if len(h) != 3 {
		t.Fatalf("ring not bounded: got %d want 3", len(h))
	}
	// newest-first
	if h[0].text != "e" || h[1].text != "d" || h[2].text != "c" {
		t.Fatalf("order wrong: %q %q %q", h[0].text, h[1].text, h[2].text)
	}
	if got := s.Latest(); got == nil || got.text != "e" {
		t.Fatalf("latest wrong: %+v", got)
	}
	if s.Len() != 3 {
		t.Fatalf("len wrong: got %d want 3", s.Len())
	}
}

func TestNotificationStore_EmptyLatestNil(t *testing.T) {
	s := newNotificationStore(10)
	if l := s.Latest(); l != nil {
		t.Fatalf("empty store Latest should be nil, got %+v", l)
	}
	if s.Len() != 0 {
		t.Fatalf("empty store Len should be 0, got %d", s.Len())
	}
	if s.HasSessionEntry() {
		t.Fatalf("empty store should have no session entry")
	}
}

func TestNotificationStore_PushMarksSessionAndPersists(t *testing.T) {
	s := newNotificationStore(10)
	var got [][]notification
	s.persist = func(snap []notification) { got = append(got, snap) }
	s.Push(notification{text: "hi", isErr: false, unixMs: 7})
	if !s.HasSessionEntry() {
		t.Fatalf("Push should mark a session entry")
	}
	if len(got) != 1 || len(got[0]) != 1 || got[0][0].text != "hi" {
		t.Fatalf("persist sink not called with snapshot: %+v", got)
	}
}

func TestNotificationStore_SeedDoesNotMarkSession(t *testing.T) {
	s := newNotificationStore(10)
	s.seed([]notification{{text: "old1"}, {text: "old2"}})
	if s.HasSessionEntry() {
		t.Fatalf("seed must not count as a session entry")
	}
	if s.Len() != 2 {
		t.Fatalf("seed should populate ring: got %d want 2", s.Len())
	}
	// seed beyond cap keeps the newest
	s2 := newNotificationStore(2)
	s2.seed([]notification{{text: "a"}, {text: "b"}, {text: "c"}})
	h := s2.History()
	if len(h) != 2 || h[0].text != "c" || h[1].text != "b" {
		t.Fatalf("seed cap-trim wrong: %+v", h)
	}
}
