package ui

// GridModeKind identifies an armed grid interaction mode entered from the node
// quick-action popup (or the sidebar's Move button). While a mode is armed the
// user can pan/zoom freely; the next grid tap places (move/copy), connects
// (connect), or cancels the mode.
type GridModeKind int

const (
	GridModeNone GridModeKind = iota
	GridModeMove
	GridModeCopy
	GridModeConnect
)

func (k GridModeKind) String() string {
	switch k {
	case GridModeMove:
		return "move"
	case GridModeCopy:
		return "copy"
	case GridModeConnect:
		return "connect"
	}
	return "none"
}

// GridModeObserver is the opt-in API for components that must react to mode
// transitions (e.g. the node sidebar closes when a mode arms). Observers run
// synchronously on the UI Update goroutine — they must not re-acquire seqMu,
// and they must not call Enter/Cancel/Commit back into the manager from
// OnGridModeChanged: a re-entrant transition would hand later observers in the
// same notification a stale next/subject that no longer matches Active().
type GridModeObserver interface {
	OnGridModeChanged(prev, next GridModeKind, subject *uiNode)
}

// gridModeBlinkHalfPeriod is the blink half-period in Update ticks (~250 ms at
// 60 TPS): the armed subject node's body is highlighted for one half period,
// plain for the next.
const gridModeBlinkHalfPeriod = 15

// gridModeManager is the single owner of armed grid-mode state. Enter, Cancel
// and Commit are the ONLY mutation paths (grid_mode_discipline_test.go guards
// this structurally): they emit the armed/canceled hooks events centrally and
// notify observers synchronously, so call sites cannot forget either. Armed
// modes mutate nothing until the committed operation runs, so Cancel is always
// a safe rollback: clear state, notify, done.
type gridModeManager struct {
	kind      GridModeKind
	subject   *uiNode
	phase     int // blink phase, advanced by Tick() once per Update while armed
	observers []GridModeObserver
}

// Register adds an observer. Registration is permanent (startup wiring only).
func (m *gridModeManager) Register(o GridModeObserver) {
	m.observers = append(m.observers, o)
}

// Enter arms kind on subject. Entering with kind==GridModeNone or a nil
// subject is a no-op (use Cancel to disarm). Entering while another mode is
// armed cancels it first (reason "switched") and then arms the new mode.
func (m *gridModeManager) Enter(kind GridModeKind, subject *uiNode) {
	if kind == GridModeNone || subject == nil {
		return
	}
	if m.kind != GridModeNone {
		m.Cancel("switched")
	}
	m.kind = kind
	m.subject = subject
	m.phase = 0
	emitGridModeArmed(kind.String(), subject.ID, subject.I, subject.J)
	m.notify(GridModeNone, kind, subject)
}

// Cancel rolls back the armed mode without touching the document. No-op when
// nothing is armed.
func (m *gridModeManager) Cancel(reason string) {
	if m.kind == GridModeNone {
		return
	}
	prev, subject := m.kind, m.subject
	m.kind = GridModeNone
	m.subject = nil
	m.phase = 0
	emitGridModeCanceled(prev.String(), subject.ID, subject.I, subject.J, reason)
	m.notify(prev, GridModeNone, subject)
}

// Commit clears the armed mode after its operation ran. It emits no event of
// its own — the committed operation (moveNode / pasteNodeFrom / addEdge)
// publishes the domain event. No-op when nothing is armed.
func (m *gridModeManager) Commit() {
	if m.kind == GridModeNone {
		return
	}
	prev, subject := m.kind, m.subject
	m.kind = GridModeNone
	m.subject = nil
	m.phase = 0
	m.notify(prev, GridModeNone, subject)
}

func (m *gridModeManager) notify(prev, next GridModeKind, subject *uiNode) {
	// Iterate a snapshot so a Register call from inside an observer cannot
	// grow the slice mid-loop. Re-entrant Enter/Cancel/Commit from observers
	// stays forbidden by the GridModeObserver contract.
	obs := make([]GridModeObserver, len(m.observers))
	copy(obs, m.observers)
	for _, o := range obs {
		o.OnGridModeChanged(prev, next, subject)
	}
}

func (m *gridModeManager) Active() GridModeKind   { return m.kind }
func (m *gridModeManager) Is(k GridModeKind) bool { return m.kind == k }
func (m *gridModeManager) Subject() *uiNode       { return m.subject }

// Tick advances the blink phase; called once per Game.Update while armed.
func (m *gridModeManager) Tick() {
	if m.kind != GridModeNone {
		m.phase++
	}
}

// blinkOn reports whether the armed subject's blink highlight is in its ON
// half period this tick.
func (m *gridModeManager) blinkOn() bool {
	return (m.phase/gridModeBlinkHalfPeriod)%2 == 0
}

// sidebarModeObserver closes the node sidebar whenever a grid mode arms — the
// armed-mode overlay owns the grid until placement. Registered once at Game
// construction; the first proof of the opt-in observer API.
type sidebarModeObserver struct{ g *Game }

func (o sidebarModeObserver) OnGridModeChanged(prev, next GridModeKind, _ *uiNode) {
	if next != GridModeNone && o.g.sidebar != nil && o.g.sidebar.IsOpen() {
		o.g.sidebar.Close()
	}
}
