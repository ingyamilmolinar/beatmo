package bench

import "testing"

func TestKnownActionsList(t *testing.T) {
	got := KnownActions()
	want := []HookAction{ActionLog, ActionExit, ActionDumpPProf}
	if len(got) != len(want) {
		t.Fatalf("KnownActions length=%d want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("KnownActions[%d]=%q want %q", i, got[i], want[i])
		}
	}
}

func TestIsKnownActionRecognized(t *testing.T) {
	for _, a := range KnownActions() {
		if !IsKnownAction(string(a)) {
			t.Errorf("IsKnownAction(%q)=false, want true", a)
		}
	}
}

func TestIsKnownActionUnknown(t *testing.T) {
	for _, name := range []string{"", "logg", "EXIT", "noop", "dump_PPROF"} {
		if IsKnownAction(name) {
			t.Errorf("IsKnownAction(%q)=true, want false (typos must be rejected)", name)
		}
	}
}
