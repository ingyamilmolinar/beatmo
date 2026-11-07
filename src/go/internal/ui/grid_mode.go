package ui

// While armed the user can pan and zoom freely; the next grid tap places, connects or cancels.
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

// Synchronous on the UI goroutine: must not take seqMu, and must not call Enter/Cancel/Commit from
// OnGridModeChanged, which would hand later observers a stale next/subject.
type GridModeObserver interface {
	OnGridModeChanged(prev, next GridModeKind, subject *uiNode)
}

// ~250 ms at 60 TPS.
const gridModeBlinkHalfPeriod = 15

// Enter, Cancel and Commit are the ONLY mutation paths, so events and observers can never be
// forgotten. An armed mode mutates nothing until it commits, so Cancel is always a safe rollback.
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

// None or a nil subject is a no-op; entering over another mode cancels it ("switched") first.
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

// Never touches the document.
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

// Emits nothing: the committed operation publishes the domain event.
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
	// A snapshot, so a Register from inside an observer cannot grow the slice mid-loop.
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

func (m *gridModeManager) blinkOn() bool {
	return (m.phase/gridModeBlinkHalfPeriod)%2 == 0
}

// The armed mode owns the grid until placement.
type sidebarModeObserver struct{ g *Game }

func (o sidebarModeObserver) OnGridModeChanged(prev, next GridModeKind, _ *uiNode) {
	if next != GridModeNone && o.g.sidebar != nil && o.g.sidebar.IsOpen() {
		o.g.sidebar.Close()
	}
}
