//go:build test

package ui

import (
	"image"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

type fakeNotifSink struct {
	loaded []notification
	saved  [][]notification
}

func (f *fakeNotifSink) Load() []notification { return f.loaded }
func (f *fakeNotifSink) Save(s []notification) {
	f.saved = append(f.saved, append([]notification(nil), s...))
}

func TestDrumViewNotif_SeedsFromSinkWithoutMarkingSession(t *testing.T) {
	assertDefaultParityState(t)
	prev := NotificationHistory()
	t.Cleanup(func() { SetNotificationHistoryStore(prev) })
	fake := &fakeNotifSink{loaded: []notification{{text: "old1"}, {text: "old2"}}}
	SetNotificationHistoryStore(fake)

	g := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, g)

	if dv.notifStore.Len() != 2 {
		t.Fatalf("expected ring seeded from sink, got len=%d", dv.notifStore.Len())
	}
	if dv.notifStore.HasSessionEntry() {
		t.Fatalf("seeded history must NOT count as a session entry (band starts idle)")
	}
}

func TestDrumViewNotif_PushPersistsThroughSink(t *testing.T) {
	assertDefaultParityState(t)
	prev := NotificationHistory()
	t.Cleanup(func() { SetNotificationHistoryStore(prev) })
	fake := &fakeNotifSink{loaded: []notification{{text: "old1"}}}
	SetNotificationHistoryStore(fake)

	g := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, g)
	dv.notifyError("boom")

	if !dv.notifStore.HasSessionEntry() {
		t.Fatalf("push should mark a session entry")
	}
	if len(fake.saved) == 0 {
		t.Fatalf("push should write through the persist sink")
	}
	last := fake.saved[len(fake.saved)-1]
	if len(last) != 2 || last[1].text != "boom" || !last[1].isErr {
		t.Fatalf("persisted payload wrong: %+v", last)
	}
}
