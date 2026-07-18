package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

type recObserver struct{ calls []string }

func (r *recObserver) OnGridModeChanged(prev, next GridModeKind, subject *uiNode) {
	r.calls = append(r.calls, prev.String()+"->"+next.String())
}

func TestGridModeManager_TransitionMatrix(t *testing.T) {
	n := &uiNode{ID: 1, I: 2, J: 3}
	var m gridModeManager
	obs := &recObserver{}
	m.Register(obs)

	// None → Move
	m.Enter(GridModeMove, n)
	if !m.Is(GridModeMove) || m.Subject() != n {
		t.Fatalf("expected move armed with subject, got %v %v", m.Active(), m.Subject())
	}
	// Enter with nil subject or None kind is a no-op.
	m.Enter(GridModeCopy, nil)
	m.Enter(GridModeNone, n)
	if !m.Is(GridModeMove) {
		t.Fatalf("no-op Enters must not change state, got %v", m.Active())
	}
	// Move armed while Connect requested: implicit cancel ("switched") then
	// re-arm — observers see move->none followed by none->connect.
	m.Enter(GridModeConnect, n)
	if !m.Is(GridModeConnect) {
		t.Fatalf("expected connect after switch, got %v", m.Active())
	}
	// Cancel clears.
	m.Cancel("esc")
	if m.Active() != GridModeNone || m.Subject() != nil {
		t.Fatalf("cancel must clear state")
	}
	// Cancel when None is a no-op (no observer call, no panic).
	before := len(obs.calls)
	m.Cancel("esc")
	if len(obs.calls) != before {
		t.Fatalf("cancel-when-none must not notify")
	}
	// Commit clears.
	m.Enter(GridModeCopy, n)
	m.Commit()
	if m.Active() != GridModeNone {
		t.Fatalf("commit must clear state")
	}
	// Commit when None is a no-op.
	m.Commit()

	want := []string{"none->move", "move->none", "none->connect", "connect->none", "none->copy", "copy->none"}
	if len(obs.calls) != len(want) {
		t.Fatalf("observer calls = %v, want %v", obs.calls, want)
	}
	for i := range want {
		if obs.calls[i] != want[i] {
			t.Fatalf("observer call %d = %q, want %q", i, obs.calls[i], want[i])
		}
	}
}

func TestGridModeManager_EmitsArmedAndCanceled(t *testing.T) {
	armed := make(chan hooks.GridModePayload, 4)
	canceled := make(chan hooks.GridModePayload, 4)
	u1 := hooks.GlobalBus().Subscribe(hooks.EventGridModeArmed, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.GridModePayload); ok {
			armed <- p
		}
	})
	defer u1()
	u2 := hooks.GlobalBus().Subscribe(hooks.EventGridModeCanceled, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.GridModePayload); ok {
			canceled <- p
		}
	})
	defer u2()

	n := &uiNode{ID: 7, I: 4, J: 5}
	var m gridModeManager
	m.Enter(GridModeMove, n)
	select {
	case p := <-armed:
		if p.Mode != "move" || p.ID != 7 || p.I != 4 || p.J != 5 {
			t.Fatalf("armed payload = %+v", p)
		}
	case <-time.After(time.Second):
		t.Fatal("no EventGridModeArmed")
	}
	m.Cancel("invalid-target")
	select {
	case p := <-canceled:
		if p.Mode != "move" || p.Reason != "invalid-target" {
			t.Fatalf("canceled payload = %+v", p)
		}
	case <-time.After(time.Second):
		t.Fatal("no EventGridModeCanceled")
	}
	// Commit emits neither armed nor canceled.
	m.Enter(GridModeCopy, n)
	<-armed
	m.Commit()
	select {
	case p := <-canceled:
		t.Fatalf("commit must not emit canceled: %+v", p)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestGridModeManager_BlinkPhase(t *testing.T) {
	n := &uiNode{ID: 1}
	var m gridModeManager
	m.Enter(GridModeMove, n)
	if !m.blinkOn() {
		t.Fatal("blink must start ON at phase 0")
	}
	for i := 0; i < gridModeBlinkHalfPeriod; i++ {
		m.Tick()
	}
	if m.blinkOn() {
		t.Fatal("blink must be OFF after one half period")
	}
	for i := 0; i < gridModeBlinkHalfPeriod; i++ {
		m.Tick()
	}
	if !m.blinkOn() {
		t.Fatal("blink must be ON again after a full period")
	}
	// Re-arming resets the phase.
	m.Cancel("test")
	m.Enter(GridModeMove, n)
	if !m.blinkOn() {
		t.Fatal("phase must reset on Enter")
	}
}
