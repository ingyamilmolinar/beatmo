package ui

import (
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
)

// keyboard_shortcuts.go — component-owned keyboard shortcut routing.
//
// Keyboard shortcuts are not spatial, so they cannot be arbitrated by the
// HitIndex. Instead, each surface (grid pane, transport, audio panel, undo)
// owns a shortcut NODE that declares the keys it consumes and a HandleShortcut
// that acts on them. The router dispatches nodes in descending z.
//
// Two tiers:
//   - UNGATED (undo/redo): dispatched unconditionally every frame, so Ctrl/Cmd+Z
//     keeps working over popups, during drags, and while a text field is focused
//     (its prior behavior — see game_update.go). dispatchUngated is called from
//     handleUndoRedoKeys.
//   - GATED (Space, pan, zoom, reset, BPM±, tabs, "?"): dispatched only when no
//     surface claims the keyboard (DrumView.KeyboardClaimed()). dispatchGated is
//     called from handleGlobalShortcuts after the Esc + KeyboardClaimed gate.
//
// The current nodes own DISJOINT key sets (TestShortcutKeysAreDisjointAndComplete),
// so the router runs EVERY node each frame — preserving today's behavior where,
// e.g., Space + an arrow pan both fire in one frame. The HandleShortcut() bool
// return is the first-claim-wins contract hook for any future overlapping key;
// it is not used for flow control while keys stay disjoint.

// KeyboardShortcutHandler is a keyboard-shortcut consumer. HandleShortcut reads
// the keys this node owns for the current frame and returns whether it acted.
type KeyboardShortcutHandler interface {
	HandleShortcut() bool
}

// shortcutFunc adapts a plain func to KeyboardShortcutHandler.
type shortcutFunc func() bool

func (f shortcutFunc) HandleShortcut() bool { return f() }

// Z-priority for shortcut nodes (descending dispatch). Disjoint keys make the
// exact order behaviorally moot today; the ordering mirrors the pointer-pane
// intuition (panel over grid) so a future overlapping key resolves sensibly.
const (
	ZShortcutGrid       = 10
	ZShortcutTransport  = 20
	ZShortcutSeek       = 25 // above transport (20), below audio panel (30)
	ZShortcutAudioPanel = 30
	ZShortcutUndo       = 100 // ungated tier; z only orders within the tier
)

type shortcutChild struct {
	h     KeyboardShortcutHandler
	z     int
	name  string
	keys  []ebiten.Key
	gated bool
}

// keyboardShortcutRouter holds the shortcut nodes and dispatches them in two
// tiers. Owned by Game.
type keyboardShortcutRouter struct {
	children []shortcutChild
}

// add registers a node, keeping children sorted by descending z (stable).
func (r *keyboardShortcutRouter) add(name string, z int, gated bool, keys []ebiten.Key, h KeyboardShortcutHandler) {
	r.children = append(r.children, shortcutChild{h: h, z: z, name: name, keys: keys, gated: gated})
	sort.SliceStable(r.children, func(i, j int) bool { return r.children[i].z > r.children[j].z })
}

// dispatchUngated runs the ungated tier (undo/redo) and returns whether any
// ungated node claimed this frame. Called unconditionally.
func (r *keyboardShortcutRouter) dispatchUngated() bool {
	claimed := false
	for _, c := range r.children {
		if c.gated {
			continue
		}
		if c.h.HandleShortcut() {
			claimed = true
		}
	}
	return claimed
}

// dispatchGated runs the gated tier. Called only when no surface owns the WHOLE
// keyboard. Runs every gated node (keys are disjoint — see file header).
//
// blocked holds the keys a partial claimant already consumed this frame (an open
// value popup owns the arrows; see keyboard_key_claim.go). They are filtered out
// for the duration of the dispatch — PER KEY, not per node, so a popup that owns
// only the arrows still leaves the grid node's [ ] zoom and 0 reset working.
func (r *keyboardShortcutRouter) dispatchGated(blocked map[ebiten.Key]bool) {
	if len(blocked) > 0 {
		defer suppressKeys(blocked)()
	}
	for _, c := range r.children {
		if !c.gated {
			continue
		}
		c.h.HandleShortcut()
	}
}

// --- test/contract introspection helpers ---

func (r *keyboardShortcutRouter) nodeNames() []string {
	out := make([]string, 0, len(r.children))
	for _, c := range r.children {
		out = append(out, c.name)
	}
	return out
}

func (r *keyboardShortcutRouter) keysOf(name string) []ebiten.Key {
	for _, c := range r.children {
		if c.name == name {
			return c.keys
		}
	}
	return nil
}

func (r *keyboardShortcutRouter) isUngated(name string) bool {
	for _, c := range r.children {
		if c.name == name {
			return !c.gated
		}
	}
	return false
}

// newKeyboardShortcutRouter builds the router with the standing nodes,
// each delegating to a Game method that owns its keys.
func newKeyboardShortcutRouter(g *Game) *keyboardShortcutRouter {
	r := &keyboardShortcutRouter{}
	r.add("undo", ZShortcutUndo, false /*ungated*/, []ebiten.Key{ebiten.KeyZ, ebiten.KeyY, ebiten.KeyR}, shortcutFunc(g.undoShortcut))
	r.add("clipboard", ZShortcutUndo, false /*ungated*/, []ebiten.Key{ebiten.KeyC, ebiten.KeyX, ebiten.KeyV}, shortcutFunc(g.clipboardShortcut))
	r.add("grid", ZShortcutGrid, true /*gated*/, []ebiten.Key{
		ebiten.KeyArrowLeft, ebiten.KeyArrowRight, ebiten.KeyArrowUp, ebiten.KeyArrowDown,
		ebiten.KeyBracketLeft, ebiten.KeyBracketRight, ebiten.Key0,
	}, shortcutFunc(g.gridShortcut))
	r.add("transport", ZShortcutTransport, true, []ebiten.Key{
		ebiten.KeySpace, ebiten.KeyEqual, ebiten.KeyNumpadAdd, ebiten.KeyMinus, ebiten.KeyNumpadSubtract,
	}, shortcutFunc(g.transportShortcut))
	r.add("seek", ZShortcutSeek, true, []ebiten.Key{
		ebiten.KeyPeriod, ebiten.KeyComma, ebiten.KeyHome,
	}, shortcutFunc(g.seekShortcut))
	r.add("audiopanel", ZShortcutAudioPanel, true, []ebiten.Key{
		ebiten.Key1, ebiten.Key2, ebiten.Key3, ebiten.Key4, ebiten.Key5, ebiten.Key6, ebiten.Key7,
		ebiten.KeySlash,
	}, shortcutFunc(g.audioPanelShortcut))
	return r
}
