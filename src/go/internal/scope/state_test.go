package scope

import "testing"

func TestStageLabel(t *testing.T) {
	cases := []struct {
		stage Stage
		want  string
	}{
		{StageSynth, "Synth"},
		{StageAntiPop, "AntiPop"},
		{StageInsertFX, "FX"},
		{StageEQ, "EQ"},
		{StageSends, "Bus"},
		{StageMaster, "Master"},
	}
	for _, c := range cases {
		if got := StageLabel(c.stage); got != c.want {
			t.Errorf("StageLabel(%d)=%q, want %q", c.stage, got, c.want)
		}
	}
}

func TestStageDescription(t *testing.T) {
	// Each stage must have a non-empty description for the Chain tab tooltip.
	for _, s := range AllStages() {
		if got := StageDescription(s); got == "" {
			t.Errorf("StageDescription(%d) empty for stage %s", s, StageLabel(s))
		}
	}
	// Unknown stages return empty.
	if got := StageDescription(Stage(-1)); got != "" {
		t.Errorf("StageDescription(-1)=%q, want empty", got)
	}
}

func TestStageLabelUnknownReturnsEmpty(t *testing.T) {
	for _, s := range []Stage{-1, Stage(stageCount), Stage(stageCount + 5)} {
		if got := StageLabel(s); got != "" {
			t.Errorf("StageLabel(%d)=%q, want empty string", s, got)
		}
	}
}

func TestAllStagesOrderAndLength(t *testing.T) {
	got := AllStages()
	want := []Stage{StageSynth, StageAntiPop, StageInsertFX, StageEQ, StageSends, StageMaster}
	if len(got) != len(want) {
		t.Fatalf("AllStages length=%d want %d (got %v)", len(got), len(want), got)
	}
	if len(got) != int(stageCount) {
		t.Errorf("AllStages length=%d should equal stageCount=%d", len(got), stageCount)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("AllStages[%d]=%d (%s), want %d (%s)",
				i, got[i], StageLabel(got[i]), want[i], StageLabel(want[i]))
		}
	}
}
