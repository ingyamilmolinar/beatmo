package eventlogger

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

func TestFormatExported(t *testing.T) {
	got := Format(hooks.Event{Kind: hooks.EventPlayStart})
	if got != "play started" {
		t.Fatalf("Format(EventPlayStart) = %q, want %q", got, "play started")
	}
	if got := Format(hooks.Event{Kind: hooks.EventSeek, Payload: hooks.SeekPayload{Beats: 4}}); got != "seek beats=4" {
		t.Fatalf("Format(EventSeek) = %q, want %q", got, "seek beats=4")
	}
	if got := Format(hooks.Event{Kind: hooks.Kind("does.not.exist")}); got != "" {
		t.Fatalf("Format(unknown) = %q, want empty", got)
	}
}
