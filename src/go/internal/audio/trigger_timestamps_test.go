package audio

import (
	"testing"
	"time"
)

func TestLastTriggerAt_UnknownIDIsZero(t *testing.T) {
	ResetTriggerTimestamps()
	t.Cleanup(ResetTriggerTimestamps)
	got := LastTriggerAt("does-not-exist")
	if !got.IsZero() {
		t.Errorf("LastTriggerAt(unknown) = %v; want zero time", got)
	}
}

func TestSinceLastTrigger_UnknownIDIsFar(t *testing.T) {
	ResetTriggerTimestamps()
	t.Cleanup(ResetTriggerTimestamps)
	got := SinceLastTrigger("does-not-exist")
	if got < 10*time.Hour {
		t.Errorf("SinceLastTrigger(unknown) = %v; want >= 10h", got)
	}
}

func TestRecordVoiceTriggerThenReads(t *testing.T) {
	ResetTriggerTimestamps()
	t.Cleanup(ResetTriggerTimestamps)

	before := time.Now()
	RecordVoiceTrigger("kick")
	after := time.Now()

	got := LastTriggerAt("kick")
	if got.IsZero() {
		t.Fatalf("LastTriggerAt(kick) = zero after RecordVoiceTrigger")
	}
	if got.Before(before) || got.After(after) {
		t.Errorf("LastTriggerAt(kick) = %v; want in [%v, %v]", got, before, after)
	}
	since := SinceLastTrigger("kick")
	if since > time.Second {
		t.Errorf("SinceLastTrigger(kick) = %v; want < 1s", since)
	}
}

func TestResetTriggerTimestamps_ClearsAll(t *testing.T) {
	ResetTriggerTimestamps()
	t.Cleanup(ResetTriggerTimestamps)

	RecordVoiceTrigger("a")
	RecordVoiceTrigger("b")
	if LastTriggerAt("a").IsZero() || LastTriggerAt("b").IsZero() {
		t.Fatalf("preconditions failed: triggers not recorded")
	}
	ResetTriggerTimestamps()
	if !LastTriggerAt("a").IsZero() {
		t.Errorf("after reset, LastTriggerAt(a) = %v; want zero", LastTriggerAt("a"))
	}
	if !LastTriggerAt("b").IsZero() {
		t.Errorf("after reset, LastTriggerAt(b) = %v; want zero", LastTriggerAt("b"))
	}
	if SinceLastTrigger("a") < 10*time.Hour {
		t.Errorf("after reset, SinceLastTrigger(a) = %v; want >= 10h", SinceLastTrigger("a"))
	}
}
