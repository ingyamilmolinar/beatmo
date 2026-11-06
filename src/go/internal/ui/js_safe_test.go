package ui

import (
	"reflect"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

type jsSafeTestEnum int

// TestJSSafeConvertsNamedTypes: syscall/js.ValueOf panics on a named type, so
// every one that can reach it through an export's result — on its own or
// nested in []any / map[string]any — must arrive as the plain type.
func TestJSSafeConvertsNamedTypes(t *testing.T) {
	in := map[string]any{
		"id":    roster.Oboe,
		"plain": "x",
		"rows":  []any{roster.Kick, 3, map[string]any{"n": jsSafeTestEnum(2)}},
	}
	got := jsSafe(in)
	want := map[string]any{
		"id":    "oboe",
		"plain": "x",
		"rows":  []any{"kick", 3, map[string]any{"n": int64(2)}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("jsSafe = %#v, want %#v", got, want)
	}
	if in["id"] != roster.Oboe {
		t.Fatalf("jsSafe mutated its input: %#v", in["id"])
	}
	if s := jsSafe(roster.Tuba); s != "tuba" {
		t.Fatalf("jsSafe(ID) = %#v, want plain \"tuba\"", s)
	}
	if s := jsSafe([]any{channel.Master, channel.Of(roster.Kick)}); !reflect.DeepEqual(s, []any{"main", "kick"}) {
		t.Fatalf("jsSafe(channel.ID) = %#v, want the wire names", s)
	}
}

// TestJSSafeLeavesPlainValuesAlone: nothing named → the very same value back
// (no copy on the bridge's hot path).
func TestJSSafeLeavesPlainValuesAlone(t *testing.T) {
	arr := []any{"a", 1, 2.5, true, nil, int8(3)}
	m := map[string]any{"arr": arr, "s": "x"}
	if got := jsSafe(arr).([]any); &got[0] != &arr[0] {
		t.Fatalf("jsSafe copied a slice with nothing to convert")
	}
	if got := jsSafe(m).(map[string]any); reflect.ValueOf(got).Pointer() != reflect.ValueOf(m).Pointer() {
		t.Fatalf("jsSafe copied a map with nothing to convert")
	}
	type opaque struct{ x int }
	o := opaque{1}
	if got := jsSafe(o); got != o {
		t.Fatalf("jsSafe changed a non-basic value: %#v", got)
	}
}
