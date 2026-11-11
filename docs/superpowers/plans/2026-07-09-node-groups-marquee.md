# Node Groups (Marquee Multi-Select + Group Logic) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Shift+drag marquee selection over grid nodes creates a persistent node group; a GroupMenu applies batch edits and one ongoing "change param by delta every N rounds" rule, evaluated non-destructively at trigger time.

**Architecture:** First-class `NodeGroup` storage + CRUD API on `model.Graph`; a pure `GroupEffect` evaluator over a precomputed immutable `GroupIndex`, applied inside the single param chokepoint `evalNodeParamsOnly`; UI reuses the dragLink gesture shape and the NodeSidebar panel pattern. Spec: `docs/superpowers/specs/2026-07-09-node-group-selection-design.md`.

**Tech Stack:** Go 1.23 (bundled `.tools/go/bin/go`), Ebiten, existing hooks/undo/eventlogger/i18n infra.

## Global Constraints

- **NEVER `git add` or `git commit`** — house rule. Leave everything uncommitted. Plan steps therefore end at "tests pass", never at "commit".
- Always use the bundled Go: `/home/ymolinar/Repos/beatmo/.tools/go/bin/go`.
- Fast test command shape: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./<pkg>/ -run '<Regex>'`.
- Functional tests over isolation: drive real `Game.Update` via `SetInputForTest`/`click`/`advanceFrames` (see `internal/ui/testutil_test.go` header — read it before writing any UI test; call `assertDefaultParityState(t)` first, `g.updateBeatInfos()` after circuit changes).
- No new `color.RGBA{}` literals in `internal/ui/` (token ratchet — use `theme_tokens.go` accessors + `WithAlpha`). No forbidden glyphs (`▶✕≡…`) — use `IconID`/text labels.
- All new log/bracket tags lowercase. All user-visible strings through `internal/i18n` (add keys to `keys.go`, `catalog_en.go`, `catalog_es.go`).
- Group rules must NOT change whether a node fires — only vol/pitch/dur. Do not touch `shouldTriggerNode`/`seqShouldTriggerNode`/`evalAudible`.
- Round math: `round = (idx - loopStart) / loopLen` for `idx > loopStart`, else 0; non-loop rows always round 0.
- Multi-group stacking: pitch deltas sum; volume/duration multipliers multiply; final clamp pitch ∈ [-24,24], vol ∈ [0,4], dur ∈ [0.1,4] — applied only when the node has ≥1 group rule (zero behavior change for ungrouped nodes).

---

### Task 1: Model — NodeGroup types + Graph storage + CRUD API

**Files:**
- Create: `src/go/core/model/node_groups.go`
- Create: `src/go/core/model/node_groups_test.go`
- Modify: `src/go/core/model/graph.go` (struct fields lines 5-14, `NewGraph` lines 16-26, `RemoveNode` lines 46-58)

**Interfaces:**
- Consumes: existing `NodeID`, `Graph.Nodes`, `Graph.onNodeChanged` pattern.
- Produces (later tasks call these exactly):
  - `type GroupID int`, `type GroupParam string` (`GroupParamPitch/Volume/Duration` consts = `"pitch"/"volume"/"duration"`)
  - `type GroupRule struct { Param GroupParam; Delta float64; EveryN int; Min, Max float64 }`
  - `type NodeGroup struct { ID GroupID; Name string; NodeIDs []NodeID; Rules []GroupRule }`
  - `(g *Graph) CreateGroup(name string, nodeIDs []NodeID) (GroupID, error)`
  - `(g *Graph) DeleteGroup(id GroupID) error`
  - `(g *Graph) RenameGroup(id GroupID, name string) error`
  - `(g *Graph) SetGroupNodes(id GroupID, nodeIDs []NodeID) error`
  - `(g *Graph) AddNodeToGroup(id GroupID, n NodeID) error`
  - `(g *Graph) RemoveNodeFromGroup(id GroupID, n NodeID) error`
  - `(g *Graph) SetGroupRules(id GroupID, rules []GroupRule) error`
  - `(g *Graph) ClearGroupRules(id GroupID) error`
  - `(g *Graph) Group(id GroupID) (NodeGroup, bool)` (deep copy)
  - `(g *Graph) AllGroups() []NodeGroup` (sorted by ID, deep copies)
  - `(g *Graph) GroupsForNode(n NodeID) []GroupID` (sorted)
  - `(g *Graph) SetGroupChangedHook(fn func(GroupID))`
  - Errors: `ErrGroupNotFound`, `ErrGroupEmptyNodes`, `ErrGroupUnknownNode`, `ErrGroupBadRule`

- [ ] **Step 1: Write the failing tests**

Create `src/go/core/model/node_groups_test.go`:

```go
package model

import (
	"errors"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func newGroupTestGraph(t *testing.T) (*Graph, NodeID, NodeID, NodeID) {
	t.Helper()
	g := NewGraph(game_log.New(game_log.LevelError))
	a := g.AddNode(0, 0, NodeTypeRegular)
	b := g.AddNode(1, 0, NodeTypeRegular)
	c := g.AddNode(2, 0, NodeTypeRegular)
	return g, a, b, c
}

func TestCreateGroupAndRead(t *testing.T) {
	g, a, b, _ := newGroupTestGraph(t)
	id, err := g.CreateGroup("", []NodeID{a, b, a}) // dup a: deduped
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	grp, ok := g.Group(id)
	if !ok {
		t.Fatalf("Group(%d) not found", id)
	}
	if grp.Name == "" {
		t.Fatalf("expected default name, got empty")
	}
	if len(grp.NodeIDs) != 2 {
		t.Fatalf("want 2 members (deduped), got %v", grp.NodeIDs)
	}
	// Read is a copy: mutating it must not affect the graph.
	grp.NodeIDs[0] = 999
	grp2, _ := g.Group(id)
	if grp2.NodeIDs[0] == 999 {
		t.Fatalf("Group() must return a deep copy")
	}
}

func TestCreateGroupValidation(t *testing.T) {
	g, a, _, _ := newGroupTestGraph(t)
	if _, err := g.CreateGroup("x", nil); !errors.Is(err, ErrGroupEmptyNodes) {
		t.Fatalf("want ErrGroupEmptyNodes, got %v", err)
	}
	if _, err := g.CreateGroup("x", []NodeID{a, 12345}); !errors.Is(err, ErrGroupUnknownNode) {
		t.Fatalf("want ErrGroupUnknownNode, got %v", err)
	}
}

func TestGroupsForNodeAndMultiMembership(t *testing.T) {
	g, a, b, c := newGroupTestGraph(t)
	id1, _ := g.CreateGroup("", []NodeID{a, b})
	id2, _ := g.CreateGroup("", []NodeID{a, c})
	got := g.GroupsForNode(a)
	if len(got) != 2 || got[0] != id1 || got[1] != id2 {
		t.Fatalf("GroupsForNode(a) = %v, want [%d %d]", got, id1, id2)
	}
	if got := g.GroupsForNode(c); len(got) != 1 || got[0] != id2 {
		t.Fatalf("GroupsForNode(c) = %v", got)
	}
}

func TestSetGroupRulesValidationAndDefaults(t *testing.T) {
	g, a, _, _ := newGroupTestGraph(t)
	id, _ := g.CreateGroup("", []NodeID{a})
	if err := g.SetGroupRules(id, []GroupRule{{Param: "nope", Delta: 1, EveryN: 1}}); !errors.Is(err, ErrGroupBadRule) {
		t.Fatalf("bad param: want ErrGroupBadRule, got %v", err)
	}
	if err := g.SetGroupRules(id, []GroupRule{{Param: GroupParamPitch, Delta: 1, EveryN: 0}}); !errors.Is(err, ErrGroupBadRule) {
		t.Fatalf("EveryN<1: want ErrGroupBadRule, got %v", err)
	}
	if err := g.SetGroupRules(id, []GroupRule{{Param: GroupParamPitch, Delta: 1, EveryN: 1, Min: 5, Max: 2}}); !errors.Is(err, ErrGroupBadRule) {
		t.Fatalf("Min>Max: want ErrGroupBadRule, got %v", err)
	}
	// Zero Min/Max → per-param defaults applied on store.
	if err := g.SetGroupRules(id, []GroupRule{{Param: GroupParamPitch, Delta: 2, EveryN: 4}}); err != nil {
		t.Fatalf("SetGroupRules: %v", err)
	}
	grp, _ := g.Group(id)
	if grp.Rules[0].Min != -24 || grp.Rules[0].Max != 24 {
		t.Fatalf("pitch defaults: got Min=%v Max=%v, want -24/24", grp.Rules[0].Min, grp.Rules[0].Max)
	}
	if err := g.SetGroupRules(id, []GroupRule{{Param: GroupParamVolume, Delta: -0.1, EveryN: 2}}); err != nil {
		t.Fatalf("SetGroupRules vol: %v", err)
	}
	grp, _ = g.Group(id)
	if grp.Rules[0].Min != 0 || grp.Rules[0].Max != 4 {
		t.Fatalf("vol defaults: got %v/%v, want 0/4", grp.Rules[0].Min, grp.Rules[0].Max)
	}
}

func TestMembershipMutations(t *testing.T) {
	g, a, b, c := newGroupTestGraph(t)
	id, _ := g.CreateGroup("", []NodeID{a})
	if err := g.AddNodeToGroup(id, b); err != nil {
		t.Fatalf("AddNodeToGroup: %v", err)
	}
	if err := g.AddNodeToGroup(id, b); err != nil { // idempotent
		t.Fatalf("AddNodeToGroup dup: %v", err)
	}
	if err := g.SetGroupNodes(id, []NodeID{b, c}); err != nil {
		t.Fatalf("SetGroupNodes: %v", err)
	}
	if err := g.SetGroupNodes(id, nil); !errors.Is(err, ErrGroupEmptyNodes) {
		t.Fatalf("empty SetGroupNodes: want ErrGroupEmptyNodes, got %v", err)
	}
	if got := g.GroupsForNode(a); len(got) != 0 {
		t.Fatalf("a should have left the group, got %v", got)
	}
	// Removing the last member auto-deletes the group.
	if err := g.RemoveNodeFromGroup(id, b); err != nil {
		t.Fatalf("RemoveNodeFromGroup: %v", err)
	}
	if err := g.RemoveNodeFromGroup(id, c); err != nil {
		t.Fatalf("RemoveNodeFromGroup last: %v", err)
	}
	if _, ok := g.Group(id); ok {
		t.Fatalf("group should be auto-deleted when emptied")
	}
}

func TestRemoveNodeCleansGroups(t *testing.T) {
	g, a, b, _ := newGroupTestGraph(t)
	id, _ := g.CreateGroup("", []NodeID{a, b})
	g.RemoveNode(a)
	grp, ok := g.Group(id)
	if !ok || len(grp.NodeIDs) != 1 || grp.NodeIDs[0] != b {
		t.Fatalf("after RemoveNode(a): %v ok=%v", grp.NodeIDs, ok)
	}
	g.RemoveNode(b)
	if _, ok := g.Group(id); ok {
		t.Fatalf("group must auto-delete when last member node is removed")
	}
}

func TestMoveNodePreservesMembership(t *testing.T) {
	g, a, _, _ := newGroupTestGraph(t)
	id, _ := g.CreateGroup("", []NodeID{a})
	if !g.MoveNode(a, 5, 7) {
		t.Fatalf("MoveNode failed")
	}
	if got := g.GroupsForNode(a); len(got) != 1 || got[0] != id {
		t.Fatalf("membership must survive MoveNode, got %v", got)
	}
}

func TestGroupChangedHookFires(t *testing.T) {
	g, a, b, _ := newGroupTestGraph(t)
	var fired []GroupID
	g.SetGroupChangedHook(func(id GroupID) { fired = append(fired, id) })
	id, _ := g.CreateGroup("", []NodeID{a, b})
	_ = g.SetGroupRules(id, []GroupRule{{Param: GroupParamPitch, Delta: 1, EveryN: 1}})
	_ = g.RenameGroup(id, "melody")
	_ = g.DeleteGroup(id)
	if len(fired) != 4 {
		t.Fatalf("hook should fire once per mutation (create/rules/rename/delete), got %d: %v", len(fired), fired)
	}
	if err := g.DeleteGroup(id); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("double delete: want ErrGroupNotFound, got %v", err)
	}
}
```

Note: check `internal/log` for the real logger constructor used in existing `core/model` tests (e.g. `graph_traversal` tests) and match it — if `game_log.New(game_log.LevelError)` isn't the actual signature, use whatever those tests use.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./core/model/ -run 'TestCreateGroup|TestGroupsForNode|TestSetGroupRules|TestMembership|TestRemoveNodeCleans|TestMoveNodePreserves|TestGroupChangedHook'`
Expected: compile FAIL (`undefined: GroupID` etc.)

- [ ] **Step 3: Implement**

Create `src/go/core/model/node_groups.go`:

```go
package model

import (
	"errors"
	"fmt"
	"sort"
)

// GroupID identifies a node group within a Graph.
type GroupID int

// GroupParam names the node parameter a GroupRule mutates.
type GroupParam string

const (
	GroupParamPitch    GroupParam = "pitch"
	GroupParamVolume   GroupParam = "volume"
	GroupParamDuration GroupParam = "duration"
)

// GroupRule: every EveryN rounds, accumulate Delta on Param. The accumulated
// effect (Delta × steps) clamps to [Min, Max]. Zero Min+Max on input means
// "use per-param defaults" (set at store time by SetGroupRules).
type GroupRule struct {
	Param  GroupParam
	Delta  float64
	EveryN int
	Min    float64
	Max    float64
}

// NodeGroup is a persistent set of nodes sharing batch edits and rules.
// A node may belong to several groups; rule effects stack (pitch deltas sum,
// volume/duration multipliers multiply).
type NodeGroup struct {
	ID      GroupID
	Name    string
	NodeIDs []NodeID
	Rules   []GroupRule
}

var (
	ErrGroupNotFound    = errors.New("group not found")
	ErrGroupEmptyNodes  = errors.New("group needs at least one node")
	ErrGroupUnknownNode = errors.New("group references unknown node")
	ErrGroupBadRule     = errors.New("invalid group rule")
)

// groupRuleDefaultClamp returns the default accumulated-effect clamp per param.
func groupRuleDefaultClamp(p GroupParam) (float64, float64) {
	switch p {
	case GroupParamPitch:
		return -24, 24
	case GroupParamVolume:
		return 0, 4
	default: // duration
		return 0.1, 4
	}
}

func (g *Graph) fireGroupChanged(id GroupID) {
	if g.onGroupChanged != nil {
		g.onGroupChanged(id)
	}
}

// SetGroupChangedHook registers a callback invoked after every group mutation
// (create, delete, rename, membership, rules). Sibling of SetNodeChangedHook.
func (g *Graph) SetGroupChangedHook(fn func(GroupID)) { g.onGroupChanged = fn }

// dedupValidateNodeIDs returns nodeIDs deduped in first-seen order, erroring
// on empty sets and IDs not present in the graph.
func (g *Graph) dedupValidateNodeIDs(nodeIDs []NodeID) ([]NodeID, error) {
	seen := map[NodeID]bool{}
	out := make([]NodeID, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		if seen[id] {
			continue
		}
		if _, ok := g.Nodes[id]; !ok {
			return nil, fmt.Errorf("%w: %d", ErrGroupUnknownNode, id)
		}
		seen[id] = true
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, ErrGroupEmptyNodes
	}
	return out, nil
}

// CreateGroup creates a group over nodeIDs (deduped). Empty name gets
// "Group N". Returns the new GroupID.
func (g *Graph) CreateGroup(name string, nodeIDs []NodeID) (GroupID, error) {
	ids, err := g.dedupValidateNodeIDs(nodeIDs)
	if err != nil {
		return 0, err
	}
	if g.Groups == nil {
		g.Groups = map[GroupID]*NodeGroup{}
	}
	g.nextGroupID++
	id := g.nextGroupID
	if name == "" {
		name = fmt.Sprintf("Group %d", id)
	}
	g.Groups[id] = &NodeGroup{ID: id, Name: name, NodeIDs: ids}
	g.fireGroupChanged(id)
	return id, nil
}

func (g *Graph) DeleteGroup(id GroupID) error {
	if _, ok := g.Groups[id]; !ok {
		return ErrGroupNotFound
	}
	delete(g.Groups, id)
	g.fireGroupChanged(id)
	return nil
}

func (g *Graph) RenameGroup(id GroupID, name string) error {
	grp, ok := g.Groups[id]
	if !ok {
		return ErrGroupNotFound
	}
	grp.Name = name
	g.fireGroupChanged(id)
	return nil
}

// SetGroupNodes replaces membership. Empty set is an error (use DeleteGroup).
func (g *Graph) SetGroupNodes(id GroupID, nodeIDs []NodeID) error {
	grp, ok := g.Groups[id]
	if !ok {
		return ErrGroupNotFound
	}
	ids, err := g.dedupValidateNodeIDs(nodeIDs)
	if err != nil {
		return err
	}
	grp.NodeIDs = ids
	g.fireGroupChanged(id)
	return nil
}

// AddNodeToGroup is idempotent for existing members.
func (g *Graph) AddNodeToGroup(id GroupID, n NodeID) error {
	grp, ok := g.Groups[id]
	if !ok {
		return ErrGroupNotFound
	}
	if _, nok := g.Nodes[n]; !nok {
		return fmt.Errorf("%w: %d", ErrGroupUnknownNode, n)
	}
	for _, m := range grp.NodeIDs {
		if m == n {
			return nil
		}
	}
	grp.NodeIDs = append(grp.NodeIDs, n)
	g.fireGroupChanged(id)
	return nil
}

// RemoveNodeFromGroup removes n; a group that empties is auto-deleted.
func (g *Graph) RemoveNodeFromGroup(id GroupID, n NodeID) error {
	grp, ok := g.Groups[id]
	if !ok {
		return ErrGroupNotFound
	}
	kept := grp.NodeIDs[:0]
	for _, m := range grp.NodeIDs {
		if m != n {
			kept = append(kept, m)
		}
	}
	grp.NodeIDs = kept
	if len(grp.NodeIDs) == 0 {
		delete(g.Groups, id)
	}
	g.fireGroupChanged(id)
	return nil
}

// normalizeGroupRules validates and applies default clamps in place.
func normalizeGroupRules(rules []GroupRule) ([]GroupRule, error) {
	out := make([]GroupRule, 0, len(rules))
	for _, r := range rules {
		switch r.Param {
		case GroupParamPitch, GroupParamVolume, GroupParamDuration:
		default:
			return nil, fmt.Errorf("%w: unknown param %q", ErrGroupBadRule, r.Param)
		}
		if r.EveryN < 1 {
			return nil, fmt.Errorf("%w: every_n %d < 1", ErrGroupBadRule, r.EveryN)
		}
		if r.Min == 0 && r.Max == 0 {
			r.Min, r.Max = groupRuleDefaultClamp(r.Param)
		}
		if r.Min > r.Max {
			return nil, fmt.Errorf("%w: min %v > max %v", ErrGroupBadRule, r.Min, r.Max)
		}
		out = append(out, r)
	}
	return out, nil
}

// SetGroupRules replaces the group's rules (v1 UI passes 0 or 1).
func (g *Graph) SetGroupRules(id GroupID, rules []GroupRule) error {
	grp, ok := g.Groups[id]
	if !ok {
		return ErrGroupNotFound
	}
	norm, err := normalizeGroupRules(rules)
	if err != nil {
		return err
	}
	grp.Rules = norm
	g.fireGroupChanged(id)
	return nil
}

func (g *Graph) ClearGroupRules(id GroupID) error { return g.SetGroupRules(id, nil) }

// Group returns a deep copy (callers must never alias internal state).
func (g *Graph) Group(id GroupID) (NodeGroup, bool) {
	grp, ok := g.Groups[id]
	if !ok {
		return NodeGroup{}, false
	}
	return copyGroup(grp), true
}

func copyGroup(grp *NodeGroup) NodeGroup {
	return NodeGroup{
		ID:      grp.ID,
		Name:    grp.Name,
		NodeIDs: append([]NodeID(nil), grp.NodeIDs...),
		Rules:   append([]GroupRule(nil), grp.Rules...),
	}
}

// AllGroups returns deep copies sorted by ID.
func (g *Graph) AllGroups() []NodeGroup {
	out := make([]NodeGroup, 0, len(g.Groups))
	for _, grp := range g.Groups {
		out = append(out, copyGroup(grp))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// GroupsForNode returns the sorted IDs of every group containing n.
func (g *Graph) GroupsForNode(n NodeID) []GroupID {
	var out []GroupID
	for id, grp := range g.Groups {
		for _, m := range grp.NodeIDs {
			if m == n {
				out = append(out, id)
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// removeNodeFromAllGroups strips n from every group (auto-deleting emptied
// groups). Called by Graph.RemoveNode so the "no dangling NodeIDs" invariant
// holds regardless of which code path deletes a node.
func (g *Graph) removeNodeFromAllGroups(n NodeID) {
	for id, grp := range g.Groups {
		for _, m := range grp.NodeIDs {
			if m == n {
				_ = g.RemoveNodeFromGroup(id, n)
				break
			}
		}
	}
}
```

Modify `src/go/core/model/graph.go`:
1. Add fields to the `Graph` struct (after `onNodeChanged func(NodeID)`):

```go
	Groups         map[GroupID]*NodeGroup
	nextGroupID    GroupID
	onGroupChanged func(GroupID)
```

2. In `NewGraph`, add `Groups: map[GroupID]*NodeGroup{},` to the literal.
3. In `RemoveNode`, after the edge-deletion loop and before the final `onNodeChanged` call, add:

```go
	g.removeNodeFromAllGroups(id)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: same command as Step 2. Expected: PASS.
Also run the whole model package to catch regressions: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./core/model/`

---

### Task 2: Model — GroupIndex + GroupEffect evaluator

**Files:**
- Create: `src/go/core/model/group_effect.go`
- Create: `src/go/core/model/group_effect_test.go`

**Interfaces:**
- Consumes: `NodeGroup`, `GroupRule`, `GroupID`, `NodeID` from Task 1.
- Produces:
  - `type GroupIndex struct { rulesByNode map[NodeID][]GroupRule }`
  - `BuildGroupIndex(groups map[GroupID]*NodeGroup) *GroupIndex` — nil-safe, deterministic (iterates groups sorted by ID)
  - `(idx *GroupIndex) HasRules(n NodeID) bool`
  - `(idx *GroupIndex) Member(n NodeID) bool`
  - `GroupEffect(idx *GroupIndex, n NodeID, round int) (pitchDelta, volMul, durMul float64)` — pure; identity `(0, 1, 1)` when idx nil / node unruled / round ≤ 0 with no steps
  - `GroupRound(idx, loopStart, loopLen int) int` — the shared round math

- [ ] **Step 1: Write the failing tests**

Create `src/go/core/model/group_effect_test.go`:

```go
package model

import "testing"

func idxFor(groups ...*NodeGroup) *GroupIndex {
	m := map[GroupID]*NodeGroup{}
	for _, g := range groups {
		m[g.ID] = g
	}
	return BuildGroupIndex(m)
}

func mustRule(t *testing.T, r GroupRule) GroupRule {
	t.Helper()
	norm, err := normalizeGroupRules([]GroupRule{r})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	return norm[0]
}

func TestGroupRoundMath(t *testing.T) {
	cases := []struct{ idx, start, length, want int }{
		{0, 0, 4, 0}, {3, 0, 4, 0}, {4, 0, 4, 1}, {11, 0, 4, 2},
		{2, 3, 4, 0},  // before loop start → round 0
		{7, 3, 4, 1},  // (7-3)/4 = 1
		{5, 0, 0, 0},  // no loop (len 0) → 0
		{5, 0, -1, 0}, // defensive
	}
	for _, c := range cases {
		if got := GroupRound(c.idx, c.start, c.length); got != c.want {
			t.Errorf("GroupRound(%d,%d,%d) = %d, want %d", c.idx, c.start, c.length, got, c.want)
		}
	}
}

func TestGroupEffectSingleRule(t *testing.T) {
	r := mustRule(t, GroupRule{Param: GroupParamPitch, Delta: 2, EveryN: 4})
	idx := idxFor(&NodeGroup{ID: 1, NodeIDs: []NodeID{7}, Rules: []GroupRule{r}})
	for _, c := range []struct {
		round int
		want  float64
	}{{0, 0}, {3, 0}, {4, 2}, {8, 4}, {100, 24} /* clamped to default max 24 */} {
		pd, vm, dm := GroupEffect(idx, 7, c.round)
		if pd != c.want || vm != 1 || dm != 1 {
			t.Errorf("round %d: got (%v,%v,%v), want (%v,1,1)", c.round, pd, vm, dm, c.want)
		}
	}
	// Unruled node and nil index are identity.
	if pd, vm, dm := GroupEffect(idx, 99, 8); pd != 0 || vm != 1 || dm != 1 {
		t.Errorf("unruled node: got (%v,%v,%v)", pd, vm, dm)
	}
	if pd, vm, dm := GroupEffect(nil, 7, 8); pd != 0 || vm != 1 || dm != 1 {
		t.Errorf("nil index: got (%v,%v,%v)", pd, vm, dm)
	}
}

func TestGroupEffectVolumeDurationRamp(t *testing.T) {
	rv := mustRule(t, GroupRule{Param: GroupParamVolume, Delta: -0.25, EveryN: 1})
	rd := mustRule(t, GroupRule{Param: GroupParamDuration, Delta: 0.5, EveryN: 2})
	idx := idxFor(&NodeGroup{ID: 1, NodeIDs: []NodeID{3}, Rules: []GroupRule{rv, rd}})
	pd, vm, dm := GroupEffect(idx, 3, 2)
	if pd != 0 {
		t.Errorf("pitch: got %v", pd)
	}
	if vm != 0.5 { // 1 + (-0.25*2)
		t.Errorf("volMul: got %v, want 0.5", vm)
	}
	if dm != 1.5 { // 1 + (0.5*1)
		t.Errorf("durMul: got %v, want 1.5", dm)
	}
	// Volume ramp clamps at its default Min=0, never negative.
	_, vm, _ = GroupEffect(idx, 3, 100)
	if vm != 0 {
		t.Errorf("volMul at round 100: got %v, want 0 (clamped)", vm)
	}
}

func TestGroupEffectStacking(t *testing.T) {
	r1 := mustRule(t, GroupRule{Param: GroupParamPitch, Delta: 1, EveryN: 1})
	r2 := mustRule(t, GroupRule{Param: GroupParamPitch, Delta: 2, EveryN: 2})
	rv1 := mustRule(t, GroupRule{Param: GroupParamVolume, Delta: -0.1, EveryN: 1})
	rv2 := mustRule(t, GroupRule{Param: GroupParamVolume, Delta: -0.1, EveryN: 1})
	idx := idxFor(
		&NodeGroup{ID: 1, NodeIDs: []NodeID{5}, Rules: []GroupRule{r1, rv1}},
		&NodeGroup{ID: 2, NodeIDs: []NodeID{5}, Rules: []GroupRule{r2, rv2}},
	)
	pd, vm, _ := GroupEffect(idx, 5, 4)
	if pd != 8 { // 1*4 + 2*2 — pitch deltas SUM across groups
		t.Errorf("stacked pitch: got %v, want 8", pd)
	}
	want := 0.6 * 0.6 // multipliers MULTIPLY across groups
	if diff := vm - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("stacked volume: got %v, want %v", vm, want)
	}
}

func TestBuildGroupIndexDeterministic(t *testing.T) {
	r1 := mustRule(t, GroupRule{Param: GroupParamPitch, Delta: 1, EveryN: 1})
	r2 := mustRule(t, GroupRule{Param: GroupParamPitch, Delta: 2, EveryN: 1})
	g1 := &NodeGroup{ID: 1, NodeIDs: []NodeID{5}, Rules: []GroupRule{r1}}
	g2 := &NodeGroup{ID: 2, NodeIDs: []NodeID{5}, Rules: []GroupRule{r2}}
	for i := 0; i < 20; i++ {
		idx := idxFor(g1, g2)
		pd, _, _ := GroupEffect(idx, 5, 1)
		if pd != 3 {
			t.Fatalf("iteration %d: got %v, want 3", i, pd)
		}
	}
	if !idxFor(g1).Member(5) || idxFor(g1).Member(6) {
		t.Fatalf("Member() wrong")
	}
	if !idxFor(g1).HasRules(5) {
		t.Fatalf("HasRules() wrong")
	}
	// Group with members but no rules: Member true, HasRules false.
	g3 := &NodeGroup{ID: 3, NodeIDs: []NodeID{9}}
	idx := idxFor(g3)
	if !idx.Member(9) || idx.HasRules(9) {
		t.Fatalf("no-rule group: Member/HasRules wrong")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./core/model/ -run 'TestGroupRound|TestGroupEffect|TestBuildGroupIndex'`
Expected: compile FAIL (`undefined: BuildGroupIndex`)

- [ ] **Step 3: Implement**

Create `src/go/core/model/group_effect.go`:

```go
package model

import "sort"

// GroupIndex is an immutable per-node view over group rules + membership,
// rebuilt on group mutation and read at trigger time. Readers must treat it
// as frozen; mutations swap in a freshly built index.
type GroupIndex struct {
	rulesByNode  map[NodeID][]GroupRule
	memberByNode map[NodeID]bool
}

// BuildGroupIndex flattens groups into a per-node rule list. Deterministic:
// groups are folded in ascending GroupID order (stacking is order-independent
// mathematically, but keep iteration stable anyway). Nil-safe.
func BuildGroupIndex(groups map[GroupID]*NodeGroup) *GroupIndex {
	idx := &GroupIndex{
		rulesByNode:  map[NodeID][]GroupRule{},
		memberByNode: map[NodeID]bool{},
	}
	ids := make([]GroupID, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		grp := groups[id]
		for _, n := range grp.NodeIDs {
			idx.memberByNode[n] = true
			if len(grp.Rules) > 0 {
				idx.rulesByNode[n] = append(idx.rulesByNode[n], grp.Rules...)
			}
		}
	}
	return idx
}

// Member reports whether n belongs to any group (for rendering rings).
func (idx *GroupIndex) Member(n NodeID) bool {
	if idx == nil {
		return false
	}
	return idx.memberByNode[n]
}

// HasRules reports whether any group rule affects n.
func (idx *GroupIndex) HasRules(n NodeID) bool {
	if idx == nil {
		return false
	}
	return len(idx.rulesByNode[n]) > 0
}

// GroupRound maps an absolute subdivision index to a loop round. Indices at
// or before loopStart, and non-loop rows (loopLen <= 0), are round 0. This is
// the same cycle math the mute-node every_n_triggers logic uses.
func GroupRound(idx, loopStart, loopLen int) int {
	if loopLen <= 0 || idx <= loopStart {
		return 0
	}
	return (idx - loopStart) / loopLen
}

func clampGroupF(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// GroupEffect returns the stacked rule effect for n at round: pitchDelta in
// semitones (sums across rules), volMul and durMul as multipliers (multiply
// across rules; each rule contributes clamp(1 + Delta×steps, Min, Max)).
// Pure. Identity is (0, 1, 1).
func GroupEffect(idx *GroupIndex, n NodeID, round int) (float64, float64, float64) {
	pitchDelta, volMul, durMul := 0.0, 1.0, 1.0
	if idx == nil {
		return pitchDelta, volMul, durMul
	}
	for _, r := range idx.rulesByNode[n] {
		steps := 0
		if round > 0 && r.EveryN > 0 {
			steps = round / r.EveryN
		}
		switch r.Param {
		case GroupParamPitch:
			pitchDelta += clampGroupF(r.Delta*float64(steps), r.Min, r.Max)
		case GroupParamVolume:
			volMul *= clampGroupF(1+r.Delta*float64(steps), r.Min, r.Max)
		case GroupParamDuration:
			durMul *= clampGroupF(1+r.Delta*float64(steps), r.Min, r.Max)
		}
	}
	return pitchDelta, volMul, durMul
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: same as Step 2, then the whole package: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./core/model/`
Expected: PASS.

---

### Task 3: Hooks kinds, registry, eventlogger, emit helpers

**Files:**
- Modify: `src/go/internal/hooks/events.go` (Kind consts near line 36-41; payloads near line 327; `KindAll` near line 191; `NumNonVerbose` const at line 236)
- Modify: `src/go/internal/hooks/actions.go` (registry rows near line 74)
- Modify: `src/go/internal/eventlogger/format.go` (formatter map near line 94)
- Modify: `src/go/internal/ui/event_helpers.go` (new emit helpers after `emitNodeParamsChanged`, line ~78)

**Interfaces:**
- Consumes: `hooks.PublishWithSource`, `hooks.CaptureSource`, `recordUndo` (existing).
- Produces (used by Tasks 5-8):
  - `hooks.EventGroupCreated Kind = "group.created"`, `hooks.EventGroupChanged Kind = "group.changed"`, `hooks.EventGroupDeleted Kind = "group.deleted"`
  - `hooks.GroupPayload struct { ID int; Name string; Nodes []int }` (json tags `id`, `name,omitempty`, `nodes,omitempty`)
  - `emitGroupCreated(id model.GroupID, name string, nodeIDs []model.NodeID)`, `emitGroupChanged(id model.GroupID)`, `emitGroupDeleted(id model.GroupID)` in `internal/ui`

- [ ] **Step 1: Make the discipline tests the failing tests.** No new test files — the registry is guarded by existing tests (`TestEveryKindClassified`, `TestEveryActionHasLowercaseTag`, `TestHooks_NumNonVerboseUpdated` in `internal/hooks`, `coverage_test.go` in `internal/eventlogger`). First add ONLY the three Kind consts + `KindAll` entries in `events.go`:

```go
	// Node groups (marquee multi-select). All three are document-scope,
	// undoable, non-verbose.
	EventGroupCreated Kind = "group.created"
	EventGroupChanged Kind = "group.changed"
	EventGroupDeleted Kind = "group.deleted"
```

Add the three to `KindAll` in the NON-verbose region (next to the other node/edge kinds near line 191 — read the surrounding comment: verbose kinds sit at the tail after index `NumNonVerbose`; these must go before that boundary).

- [ ] **Step 2: Run discipline tests to verify they fail**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/hooks/ ./internal/eventlogger/`
Expected: FAIL — `TestEveryKindClassified` (no registry entry), `TestHooks_NumNonVerboseUpdated` (66 ≠ 69), eventlogger coverage (no formatter).

- [ ] **Step 3: Complete the wiring**

1. `events.go`: bump `const NumNonVerbose = 66` → `69`. Add near `NodeParamsPayload` (line ~329):

```go
// GroupPayload describes a node-group mutation.
type GroupPayload struct {
	ID    int    `json:"id"`
	Name  string `json:"name,omitempty"`
	Nodes []int  `json:"nodes,omitempty"`
}
```

2. `actions.go`: add three rows next to the node/edge entries (line ~74), matching `ActionMeta{Kind, Label, Tag, Scope, ExcludedReason}` row shape:

```go
	{EventGroupCreated, "create group", "group", ScopeDocument, ""},
	{EventGroupChanged, "edit group", "group", ScopeDocument, ""},
	{EventGroupDeleted, "delete group", "group", ScopeDocument, ""},
```

3. `eventlogger/format.go`: add formatters beside `EventNodeParamsChanged` (line ~94):

```go
	hooks.EventGroupCreated: func(p any) string {
		g, _ := p.(hooks.GroupPayload)
		return fmt.Sprintf("group %d %q created (%d nodes)", g.ID, g.Name, len(g.Nodes))
	},
	hooks.EventGroupChanged: func(p any) string {
		g, _ := p.(hooks.GroupPayload)
		return fmt.Sprintf("group %d updated", g.ID)
	},
	hooks.EventGroupDeleted: func(p any) string {
		g, _ := p.(hooks.GroupPayload)
		return fmt.Sprintf("group %d deleted", g.ID)
	},
```

4. `internal/ui/event_helpers.go`: after `emitNodeParamsChanged` add:

```go
// emitGroupCreated publishes EventGroupCreated and records the undo step.
func emitGroupCreated(id model.GroupID, name string, nodeIDs []model.NodeID) {
	nodes := make([]int, 0, len(nodeIDs))
	for _, n := range nodeIDs {
		nodes = append(nodes, int(n))
	}
	hooks.PublishWithSource(hooks.EventGroupCreated, hooks.GroupPayload{
		ID: int(id), Name: name, Nodes: nodes,
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventGroupCreated)
}

// emitGroupChanged publishes EventGroupChanged (rename/membership/rules).
func emitGroupChanged(id model.GroupID) {
	hooks.PublishWithSource(hooks.EventGroupChanged, hooks.GroupPayload{ID: int(id)}, hooks.CaptureSource(1))
	recordUndo(hooks.EventGroupChanged)
}

// emitGroupDeleted publishes EventGroupDeleted.
func emitGroupDeleted(id model.GroupID) {
	hooks.PublishWithSource(hooks.EventGroupDeleted, hooks.GroupPayload{ID: int(id)}, hooks.CaptureSource(1))
	recordUndo(hooks.EventGroupDeleted)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/hooks/ ./internal/eventlogger/ ./internal/log/`
Expected: PASS (incl. `TestNoUppercaseInfoTags`). If `internal/ui` fails compile on unused emit helpers, that's fine — they're used from Task 5 on; Go doesn't error on unused package-level funcs.

---

### Task 4: UI plumbing — group index snapshot, round helper, evalNodeParamsOnly

**Files:**
- Create: `src/go/internal/ui/game_groups.go`
- Create: `src/go/internal/ui/game_groups_test.go`
- Modify: `src/go/internal/ui/game_trigger_logic.go` (`evalNodeParamsOnly`, lines 127-145)
- Modify: `src/go/internal/ui/game_new.go` (hook wiring after `SetNodeChangedHook` block, line ~299)

**Interfaces:**
- Consumes: Task 1 API, Task 2 `BuildGroupIndex`/`GroupEffect`/`GroupRound`; `Game` fields `isLoopByRow`, `loopStartByRow`, `beatInfosByRow`; node-param-change playing-branch in `game_new.go:309-330`.
- Produces:
  - `(g *Game) refreshGroupIndex()`, `(g *Game) groupIndexSnapshot() *model.GroupIndex`, `(g *Game) roundAtRowIdx(row, idx int) int`, `(g *Game) onGroupsChanged()`
  - New `Game` fields: `groupIdxMu sync.RWMutex`, `groupIdx *model.GroupIndex` — declare them in `game_groups.go`? No — Go fields must live on the struct: add to the `Game` struct where `nodeCacheMu` lives (find `nodeCacheMu` declaration via grep; add the two fields beside it).

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/game_groups_test.go`. Follow the harness conventions in `testutil_test.go` (read its header first). The test builds a 2-node loop circuit, groups both nodes with a pitch rule, and asserts `evalNodeParamsOnly` returns base pitch in round 0 and stepped pitch in later rounds:

```go
package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// buildGroupLoopCircuit creates a 2-node loop (a → b → a) on row 0 and
// returns the game plus node IDs. Mirrors the circuit-building pattern used
// by the parity/logic tests in this package (see testutil_test.go header).
func buildGroupLoopCircuit(t *testing.T) (*Game, model.NodeID, model.NodeID) {
	t.Helper()
	assertDefaultParityState(t)
	g := newTestGame(t) // use this package's canonical ctor — grep existing
	// *_test.go for the constructor used by functional tests (e.g. the one
	// undo_seam_test.go newTestGameForUndo wraps) and match it exactly.
	na := g.tryAddNode(0, 0, model.NodeTypeRegular)
	nb := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(na, nb)
	g.addEdge(nb, na)
	g.graph.StartNodeID = na.ID
	g.updateBeatInfos()
	return g, na.ID, nb.ID
}

func TestGroupRulePitchAppliesByRound(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if err := g.graph.SetGroupRules(gid, []model.GroupRule{
		{Param: model.GroupParamPitch, Delta: 2, EveryN: 1},
	}); err != nil {
		t.Fatalf("SetGroupRules: %v", err)
	}

	loopStart := g.loopStartByRow[0]
	loopLen := len(g.beatInfosByRow[0]) - loopStart
	if !g.isLoopByRow[0] || loopLen <= 0 {
		t.Fatalf("expected loop row, isLoop=%v loopLen=%d", g.isLoopByRow[0], loopLen)
	}

	infoAt := func(idx int) model.BeatInfo { return g.beatInfoAtRow(0, idx) }

	// Round 0: base pitch (0).
	_, p0, _ := g.evalNodeParamsOnly(0, loopStart, infoAt(loopStart))
	if p0 != 0 {
		t.Fatalf("round 0 pitch = %v, want 0", p0)
	}
	// Round 2: +4 semitones (delta 2 × 2 rounds).
	idx2 := loopStart + 2*loopLen
	_, p2, _ := g.evalNodeParamsOnly(0, idx2, infoAt(idx2))
	if p2 != 4 {
		t.Fatalf("round 2 pitch = %v, want 4", p2)
	}
	// Step boundary: pitch changes exactly AT the round's loopStart, and the
	// last index of the previous round still has the previous value.
	_, pPrev, _ := g.evalNodeParamsOnly(0, idx2-1, infoAt(idx2-1))
	if pPrev != 2 {
		t.Fatalf("last idx of round 1 pitch = %v, want 2", pPrev)
	}
	// Clamp: far future rounds clamp at +24.
	idxFar := loopStart + 200*loopLen
	_, pFar, _ := g.evalNodeParamsOnly(0, idxFar, infoAt(idxFar))
	if pFar != 24 {
		t.Fatalf("far round pitch = %v, want clamped 24", pFar)
	}
}

func TestGroupRuleInertOnUngroupedNodeAndNonLoop(t *testing.T) {
	g, aID, _ := buildGroupLoopCircuit(t)
	// No group at all → identity behavior (existing semantics untouched).
	info := g.beatInfoAtRow(0, 0)
	_, p, _ := g.evalNodeParamsOnly(0, 100, info)
	if p != 0 {
		t.Fatalf("ungrouped pitch = %v, want 0", p)
	}
	// Group with rule on a but round always 0 when row isn't a loop:
	// covered at model level by TestGroupRoundMath; here assert a group
	// WITHOUT rules changes nothing even for members.
	gid, _ := g.graph.CreateGroup("", []model.NodeID{aID})
	_ = gid
	_, p, _ = g.evalNodeParamsOnly(0, 100, info)
	pitchAtRound2 := p
	if pitchAtRound2 != 0 {
		t.Fatalf("rule-less group must be inert, pitch = %v", p)
	}
}
```

Adapt the constructor/helpers to the package's real ones (`newTestGame` name shown; use whatever functional tests in `internal/ui` actually use — check `undo_seam_test.go:42` and neighbors). Keep assertions identical.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestGroupRule'`
Expected: FAIL — pitch stays 0 in round 2 (no integration yet). If the constructor name is wrong it fails compile first; fix per Step 1 note.

- [ ] **Step 3: Implement**

1. Add fields to the `Game` struct beside `nodeCacheMu` (grep `nodeCacheMu sync.RWMutex` for the declaring file):

```go
	// Node-group rule evaluation: immutable index snapshot, swapped whole on
	// every group mutation. Read from the sequencer goroutine and UI thread.
	groupIdxMu sync.RWMutex
	groupIdx   *model.GroupIndex
```

2. Create `src/go/internal/ui/game_groups.go`:

```go
package ui

import "github.com/ingyamilmolinar/beatmo/core/model"

// refreshGroupIndex rebuilds the immutable group-rule index from the graph.
// Called from the graph's group-changed hook (UI thread) and after import.
func (g *Game) refreshGroupIndex() {
	idx := model.BuildGroupIndex(g.graph.Groups)
	g.groupIdxMu.Lock()
	g.groupIdx = idx
	g.groupIdxMu.Unlock()
}

// groupIndexSnapshot returns the current immutable index (may be nil).
func (g *Game) groupIndexSnapshot() *model.GroupIndex {
	g.groupIdxMu.RLock()
	defer g.groupIdxMu.RUnlock()
	return g.groupIdx
}

// roundAtRowIdx maps an absolute subdivision index to the row's loop round.
// Non-loop rows are always round 0. Mirrors game_graph_beat_index.go's loop
// math; callers hold seqMu (sequencer schedule / Update), matching how
// isLoopByRow is read elsewhere.
func (g *Game) roundAtRowIdx(row, idx int) int {
	if row < 0 || row >= len(g.isLoopByRow) || !g.isLoopByRow[row] {
		return 0
	}
	if row >= len(g.loopStartByRow) || row >= len(g.beatInfosByRow) {
		return 0
	}
	start := g.loopStartByRow[row]
	loopLen := len(g.beatInfosByRow[row]) - start
	return model.GroupRound(idx, start, loopLen)
}

// onGroupsChanged reacts to any group mutation: rebuild the index and, while
// playing, drop in-flight audio + advance the parity generation so the new
// rule takes effect from the next re-predicted trigger (same mechanism as the
// node-param-change hook in game_new.go).
func (g *Game) onGroupsChanged() {
	g.refreshGroupIndex()
	if g.importing {
		return
	}
	if g.Playing() {
		g.audioGen.Add(1)
		g.bumpParityGen("group-rule-change", structuralMutationOptions{
			SkipPathsDirty:     true,
			SkipPathChangeMark: true,
			SkipBufferClear:    true,
		})
		g.parityMu.Lock()
		g.parityAudio = nil
		g.parityAudioMaxIdx = nil
		g.paritySeqDecisions = make(map[int]map[int]paritySeqDecision)
		g.parityMu.Unlock()
		g.ClearParityMismatches()
	}
}
```

3. In `game_new.go`, immediately after the `g.graph.SetNodeChangedHook(...)` registration block (line ~299), add:

```go
	g.graph.SetGroupChangedHook(func(model.GroupID) { g.onGroupsChanged() })
	g.refreshGroupIndex()
```

4. In `game_trigger_logic.go`, extend `evalNodeParamsOnly` (keep existing body; add after the `nodeSnapshot` block, before `return`):

```go
	if gi := g.groupIndexSnapshot(); gi != nil && gi.HasRules(info.NodeID) {
		pd, vm, dm := model.GroupEffect(gi, info.NodeID, g.roundAtRowIdx(row, idx))
		pitch += pd
		vol *= vm
		dur *= dm
		// Final legal-range clamp — only for ruled nodes so ungrouped
		// behavior is byte-identical to before this feature.
		if pitch < -24 {
			pitch = -24
		} else if pitch > 24 {
			pitch = 24
		}
		if vol < 0 {
			vol = 0
		} else if vol > 4 {
			vol = 4
		}
		if dur < 0.1 {
			dur = 0.1
		} else if dur > 4 {
			dur = 4
		}
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestGroupRule'` → PASS.
Then the parity/logic neighborhoods: `... -run 'Parity|NodeLogic|Audible' ./internal/ui/ ./core/engine/` → PASS (no behavior change for ungrouped nodes).

---

### Task 5: Export / import round-trip

**Files:**
- Modify: `src/go/internal/ui/export.go` (`exportFile` struct line ~21; write site in the export function after the `nodes` loop, line ~426)
- Modify: `src/go/internal/ui/import.go` (`importFile` struct line ~25; apply site after the edge-creation loop)
- Create: `src/go/internal/ui/group_import_export_test.go`

**Interfaces:**
- Consumes: Task 1 API. Import's `idToNode map[int]*uiNode` (JSON node id → new `*uiNode`) — **group node_ids MUST be remapped through it**; imported node IDs are reissued.
- Produces: JSON shape (also consumed by Task 9's JS dump):

```json
"groups": [{"id": 1, "name": "Group 1", "node_ids": [3,5], "rules": [{"param": "pitch", "delta": 2, "every_n": 4, "min": -24, "max": 24}]}]
```

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/group_import_export_test.go`:

```go
package ui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

func TestGroupExportImportRoundTrip(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, _ := g.graph.CreateGroup("melody", []model.NodeID{aID, bID})
	_ = g.graph.SetGroupRules(gid, []model.GroupRule{
		{Param: model.GroupParamPitch, Delta: 2, EveryN: 4},
	})
	data, err := g.Export()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !strings.Contains(string(data), "\"groups\"") {
		t.Fatalf("export missing groups array:\n%s", data)
	}
	if err := g.Import(data); err != nil {
		t.Fatalf("Import: %v", err)
	}
	groups := g.graph.AllGroups()
	if len(groups) != 1 {
		t.Fatalf("want 1 group after re-import, got %d", len(groups))
	}
	grp := groups[0]
	if grp.Name != "melody" || len(grp.NodeIDs) != 2 || len(grp.Rules) != 1 {
		t.Fatalf("round-trip mismatch: %+v", grp)
	}
	r := grp.Rules[0]
	if r.Param != model.GroupParamPitch || r.Delta != 2 || r.EveryN != 4 || r.Min != -24 || r.Max != 24 {
		t.Fatalf("rule mismatch: %+v", r)
	}
	// Node-ID remap: members must reference nodes that exist post-import.
	for _, n := range grp.NodeIDs {
		if _, ok := g.graph.GetNodeByID(n); !ok {
			t.Fatalf("group references missing node %d after import", n)
		}
	}
}

func TestGroupImportValidation(t *testing.T) {
	g, aID, _ := buildGroupLoopCircuit(t)
	data, _ := g.Export()
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	doc["groups"] = []any{
		// unknown node id 999 dropped; a survives
		map[string]any{"id": 1, "name": "ok", "node_ids": []any{float64(aID), float64(999)},
			"rules": []any{map[string]any{"param": "pitch", "delta": 2, "every_n": 0}}}, // every_n floored to 1
		// all-unknown members → group dropped entirely
		map[string]any{"id": 2, "name": "gone", "node_ids": []any{float64(777)}},
		// bad rule param → rule dropped, group kept
		map[string]any{"id": 3, "name": "norule", "node_ids": []any{float64(aID)},
			"rules": []any{map[string]any{"param": "sparkle", "delta": 1, "every_n": 1}}},
	}
	mutated, _ := json.Marshal(doc)
	if err := g.Import(mutated); err != nil {
		t.Fatalf("Import: %v", err)
	}
	groups := g.graph.AllGroups()
	if len(groups) != 2 {
		t.Fatalf("want 2 surviving groups, got %d: %+v", len(groups), groups)
	}
	if len(groups[0].NodeIDs) != 1 {
		t.Fatalf("unknown member should be dropped: %+v", groups[0])
	}
	if len(groups[0].Rules) != 1 || groups[0].Rules[0].EveryN != 1 {
		t.Fatalf("every_n must floor to 1: %+v", groups[0].Rules)
	}
	if len(groups[1].Rules) != 0 {
		t.Fatalf("bad rule must be dropped per-rule: %+v", groups[1])
	}
}

func TestLegacyFileWithoutGroupsImportsClean(t *testing.T) {
	g, _, _ := buildGroupLoopCircuit(t)
	gid, _ := g.graph.CreateGroup("", []model.NodeID{g.beatInfoAtRow(0, 0).NodeID})
	_ = gid
	data, _ := g.Export()
	var doc map[string]any
	_ = json.Unmarshal(data, &doc)
	delete(doc, "groups")
	legacy, _ := json.Marshal(doc)
	if err := g.Import(legacy); err != nil {
		t.Fatalf("Import legacy: %v", err)
	}
	// Import is replace-not-merge: no groups after importing a group-less file.
	if n := len(g.graph.AllGroups()); n != 0 {
		t.Fatalf("legacy import must clear groups, got %d", n)
	}
}
```

(If `g.Export()` has a different name/signature — grep `func (g *Game) Export` — adapt the call, keep assertions.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestGroupExportImport|TestGroupImportValidation|TestLegacyFileWithoutGroups'`
Expected: FAIL (`export missing groups array`).

- [ ] **Step 3: Implement**

1. `export.go` — add types + field:

```go
type exportGroupRule struct {
	Param  string  `json:"param"`
	Delta  float64 `json:"delta"`
	EveryN int     `json:"every_n"`
	Min    float64 `json:"min,omitempty"`
	Max    float64 `json:"max,omitempty"`
}

type exportGroup struct {
	ID      int               `json:"id"`
	Name    string            `json:"name,omitempty"`
	NodeIDs []int             `json:"node_ids"`
	Rules   []exportGroupRule `json:"rules,omitempty"`
}
```

Add `Groups []exportGroup \`json:"groups,omitempty"\`` to BOTH `exportFile` (export.go:21) and `importFile` (import.go:25).

2. In the export function, after the `nodes` slice is built (line ~426), serialize groups. Invisible nodes are not exported, so filter members to exported ids:

```go
	exportedID := make(map[int]bool, len(nodes))
	for _, en := range nodes {
		exportedID[en.ID] = true
	}
	groups := make([]exportGroup, 0)
	for _, grp := range g.Graph.AllGroups() { // adapt receiver: this code runs where `g := dv.Graph` — use the same graph handle
		eg := exportGroup{ID: int(grp.ID), Name: grp.Name}
		for _, n := range grp.NodeIDs {
			if exportedID[int(n)] {
				eg.NodeIDs = append(eg.NodeIDs, int(n))
			}
		}
		if len(eg.NodeIDs) == 0 {
			continue
		}
		for _, r := range grp.Rules {
			eg.Rules = append(eg.Rules, exportGroupRule{
				Param: string(r.Param), Delta: r.Delta, EveryN: r.EveryN, Min: r.Min, Max: r.Max,
			})
		}
		groups = append(groups, eg)
	}
```

and set the field on the `exportFile` literal alongside `Nodes`.

3. `import.go` — after the edge-creation loop (where `idToNode` is complete), apply groups. Import is replace-not-merge; the graph was rebuilt from scratch so it starts group-less — no explicit clear needed (verify the import path really does rebuild `g.graph`; if it reuses the graph, clear `g.graph.Groups` first):

```go
	for _, eg := range f.Groups {
		ids := make([]model.NodeID, 0, len(eg.NodeIDs))
		for _, jsonID := range eg.NodeIDs {
			if un := idToNode[jsonID]; un != nil {
				ids = append(ids, un.ID)
			}
		}
		if len(ids) == 0 {
			continue // all members unknown → drop group
		}
		gid, err := g.graph.CreateGroup(eg.Name, ids)
		if err != nil {
			g.logger.Debugf("[import] dropping group %d: %v", eg.ID, err)
			continue
		}
		rules := make([]model.GroupRule, 0, len(eg.Rules))
		for _, r := range eg.Rules {
			n := r.EveryN
			if n < 1 {
				n = 1
			}
			rules = append(rules, model.GroupRule{
				Param:  model.GroupParam(strings.ToLower(strings.TrimSpace(r.Param))),
				Delta:  clampF64(r.Delta, -24, 24),
				EveryN: n,
				Min:    r.Min,
				Max:    r.Max,
			})
		}
		// Per-rule rejection: try all rules; on error drop only bad rules.
		if err := g.graph.SetGroupRules(gid, rules); err != nil {
			kept := rules[:0]
			for _, r := range rules {
				if e := g.graph.SetGroupRules(gid, append(append([]model.GroupRule{}, kept...), r)); e == nil {
					kept = append(kept, r)
				}
			}
			_ = g.graph.SetGroupRules(gid, kept)
		}
	}
	g.refreshGroupIndex()
```

- [ ] **Step 4: Run tests to verify they pass**

Run: Step 2 command → PASS. Then broader: `... ./internal/ui/ -run 'Import|Export'` → PASS (existing import/export suites unaffected).

---

### Task 6: GroupMenu panel + i18n

**Files:**
- Create: `src/go/internal/ui/game_group_menu.go`
- Create: `src/go/internal/ui/game_group_menu_test.go`
- Modify: `src/go/internal/i18n/keys.go`, `catalog_en.go`, `catalog_es.go` (new keys)
- Modify: `src/go/internal/ui/grid_tree.go` (add `GZGroupMenu = 62` to the GZ const block, line ~14-27)
- Modify: `src/go/internal/ui/grid_pane_layers.go` (`registerGridTree`, register draw layer)
- Modify: `src/go/internal/ui/game_update.go` (dispatcher rebuild block, line ~345-366)
- Modify: `src/go/internal/ui/game_new.go` (construct `g.groupMenu`)

**Interfaces:**
- Consumes: Task 1 API, Task 3 emit helpers, `beginUndoGroup`/`endUndoGroup`, `Button`/`NewButton`/`DropdownStyle` (`uigrid.go:164`), draw prims `drawPanelShadow`/`drawRoundedRect`/`drawRect`/`WithAlpha`/`colPanelBG`/`RadiusLG` (as used in `game_node_sidebar.go:1013+`), `SetNodeParams`+`emitNodeParamsChanged` apply pattern (`game_node_sidebar.go:445-452`), `g.enqueueUI`.
- Produces (Task 7/8 call these):
  - `type GroupMenu struct` with `(m *GroupMenu) OpenAt(id model.GroupID, x, y int)`, `Close()`, `IsOpen() bool`, `GroupID() model.GroupID`, `Hit(x, y int) bool`, `Draw(dst *ebiten.Image)`, and `InputHandler` impl (`InputBounds() image.Rectangle`, `ZIndex() int` = 210, `HandleInput(x, y int, pressed bool) InputResult`, plus whatever else the `InputHandler` interface in `input_handler.go` requires — match `NodeSidebar`'s method set).
  - `Game` field `groupMenu *GroupMenu`.

i18n keys to add (EN / ES):
| Key const | id | EN | ES |
|---|---|---|---|
| `KeyGroupMenuTitle` | `group.menu.title` | `Group` | `Grupo` |
| `KeyGroupMembers` | `group.menu.members` | `%d nodes` | `%d nodos` |
| `KeyGroupBatch` | `group.menu.batch` | `Edit all` | `Editar todos` |
| `KeyGroupRule` | `group.menu.rule` | `Rule` | `Regla` |
| `KeyGroupRuleOn` | `group.menu.rule.on` | `On` | `Sí` |
| `KeyGroupRuleOff` | `group.menu.rule.off` | `Off` | `No` |
| `KeyGroupEveryN` | `group.menu.everyn` | `Every %d rounds` | `Cada %d vueltas` |
| `KeyGroupDelete` | `group.menu.delete` | `Delete group` | `Borrar grupo` |
| `KeyGroupParamPitch` | `group.menu.param.pitch` | `Pitch` | `Tono` |
| `KeyGroupParamVolume` | `group.menu.param.volume` | `Volume` | `Volumen` |
| `KeyGroupParamDuration` | `group.menu.param.duration` | `Length` | `Duración` |

(Follow the exact declaration style of `keys.go:314` and the catalog maps at `catalog_en.go:297` / `catalog_es.go:297`; run the i18n package tests — they enforce EN/ES completeness.)

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/game_group_menu_test.go`:

```go
package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

func TestGroupMenuBatchPitchAppliesToAllMembersAsOneUndoStep(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, _ := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	g.groupMenu.OpenAt(gid, 100, 100)
	if !g.groupMenu.IsOpen() {
		t.Fatalf("menu should be open")
	}
	undoBefore := g.undoManager.Len() // adapt to the real depth accessor —
	// grep undo.go/undo tests for how existing tests count steps.

	g.groupMenu.applyBatch("pit+")
	advanceFrames(g, 2) // enqueueUI actions run inside Update

	for _, id := range []model.NodeID{aID, bID} {
		n, _ := g.graph.GetNodeByID(id)
		if n.Params.Pitch != 1 {
			t.Fatalf("node %d pitch = %v, want 1", id, n.Params.Pitch)
		}
	}
	if got := g.undoManager.Len() - undoBefore; got != 1 {
		t.Fatalf("batch edit must be exactly 1 undo step, got %d", got)
	}
}

func TestGroupMenuRuleEditing(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, _ := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	g.groupMenu.OpenAt(gid, 100, 100)

	g.groupMenu.setRuleEnabled(true)
	g.groupMenu.stepRuleDelta(+1)
	g.groupMenu.stepRuleEveryN(+1) // 1 → 2
	advanceFrames(g, 2)

	grp, _ := g.graph.Group(gid)
	if len(grp.Rules) != 1 {
		t.Fatalf("want 1 rule, got %+v", grp.Rules)
	}
	if grp.Rules[0].EveryN != 2 {
		t.Fatalf("EveryN = %d, want 2", grp.Rules[0].EveryN)
	}
	g.groupMenu.setRuleEnabled(false)
	advanceFrames(g, 2)
	grp, _ = g.graph.Group(gid)
	if len(grp.Rules) != 0 {
		t.Fatalf("rule off must clear rules, got %+v", grp.Rules)
	}
}

func TestGroupMenuDeleteGroupClosesMenu(t *testing.T) {
	g, aID, _ := buildGroupLoopCircuit(t)
	gid, _ := g.graph.CreateGroup("", []model.NodeID{aID})
	g.groupMenu.OpenAt(gid, 100, 100)
	g.groupMenu.deleteGroup()
	advanceFrames(g, 2)
	if g.groupMenu.IsOpen() {
		t.Fatalf("menu must close when its group is deleted")
	}
	if _, ok := g.graph.Group(gid); ok {
		t.Fatalf("group must be deleted")
	}
}

func TestGroupMenuClosesWhenGroupEmptiedByNodeDeletion(t *testing.T) {
	g, aID, _ := buildGroupLoopCircuit(t)
	gid, _ := g.graph.CreateGroup("", []model.NodeID{aID})
	g.groupMenu.OpenAt(gid, 100, 100)
	g.deleteNode(g.nodesByID[aID])
	advanceFrames(g, 2)
	if g.groupMenu.IsOpen() {
		t.Fatalf("menu must close when its group empties/auto-deletes")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestGroupMenu'`
Expected: compile FAIL (`g.groupMenu undefined`).

- [ ] **Step 3: Implement**

Create `src/go/internal/ui/game_group_menu.go`. Model it as a compact fixed-layout panel (NOT a full sidebar clone — no scroll/collapse). Core skeleton (fill in layout numbers from density tokens; use `Profile().DensityValues()` button heights like the sidebar does):

```go
package ui

import (
	"fmt"
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// GroupMenu is the anchored panel opened after a marquee selection (or from
// a sidebar group chip). It edits ONE group: batch edits fan out over all
// members via SetNodeParams inside one undo group; the rule editor writes
// Rules[0] through the model API.
type GroupMenu struct {
	game    *Game
	open    bool
	groupID model.GroupID
	panel   image.Rectangle
	rects   map[string]image.Rectangle
	btns    map[string]*Button
	// rule scratch state mirrors Rules[0] (param + delta + everyN).
	ruleParam model.GroupParam
}

func NewGroupMenu(g *Game) *GroupMenu {
	return &GroupMenu{
		game:      g,
		rects:     map[string]image.Rectangle{},
		btns:      map[string]*Button{},
		ruleParam: model.GroupParamPitch,
	}
}

// OpenAt opens the menu for group id anchored near (x, y), clamped on-screen.
func (m *GroupMenu) OpenAt(id model.GroupID, x, y int) {
	if _, ok := m.game.graph.Group(id); !ok {
		return
	}
	m.open = true
	m.groupID = id
	if grp, ok := m.game.graph.Group(id); ok && len(grp.Rules) > 0 {
		m.ruleParam = grp.Rules[0].Param
	}
	m.layoutAt(x, y)
	m.game.dispatcherDirty = true
}

func (m *GroupMenu) Close() {
	m.open = false
	m.btns = map[string]*Button{}
	m.rects = map[string]image.Rectangle{}
	m.game.dispatcherDirty = true
}

func (m *GroupMenu) IsOpen() bool             { return m.open }
func (m *GroupMenu) GroupID() model.GroupID   { return m.groupID }
func (m *GroupMenu) Hit(x, y int) bool        { return m.open && image.Pt(x, y).In(m.panel) }
func (m *GroupMenu) InputBounds() image.Rectangle { return m.panel }
func (m *GroupMenu) ZIndex() int              { return 210 }

// group returns the live group; closes the menu if it vanished (auto-delete).
func (m *GroupMenu) group() (model.NodeGroup, bool) {
	grp, ok := m.game.graph.Group(m.groupID)
	if !ok && m.open {
		m.Close()
	}
	return grp, ok
}

// applyBatch applies a one-shot edit to every member as ONE undo step.
// action ∈ {"vol-","vol+","pit-","pit+","dur-","dur+"}.
func (m *GroupMenu) applyBatch(action string) {
	g := m.game
	grp, ok := m.group()
	if !ok {
		return
	}
	g.enqueueUI(func() {
		beginUndoGroup("group edit")
		defer endUndoGroup()
		for _, id := range grp.NodeIDs {
			mn, ok := g.graph.GetNodeByID(id)
			if !ok {
				continue
			}
			p := mn.Params
			switch action {
			case "vol-":
				p.Volume = clampF64(p.Volume-0.1, 0, 2)
			case "vol+":
				p.Volume = clampF64(p.Volume+0.1, 0, 2)
			case "pit-":
				p.Pitch = clampF64(p.Pitch-1, -24, 24)
			case "pit+":
				p.Pitch = clampF64(p.Pitch+1, -24, 24)
			case "dur-":
				p.Duration = clampF64(p.Duration-0.1, 0.1, 4)
			case "dur+":
				p.Duration = clampF64(p.Duration+0.1, 0.1, 4)
			}
			g.graph.SetNodeParams(id, p)
			g.notifyPredictorNode(id)
			emitNodeParamsChanged(id, p)
		}
		emitGroupChanged(grp.ID)
	})
}

// currentRule returns Rules[0] or a default-shaped disabled rule.
func (m *GroupMenu) currentRule() (model.GroupRule, bool) {
	grp, ok := m.group()
	if !ok || len(grp.Rules) == 0 {
		return model.GroupRule{Param: m.ruleParam, Delta: 1, EveryN: 1}, false
	}
	return grp.Rules[0], true
}

func (m *GroupMenu) writeRule(r model.GroupRule, enabled bool) {
	g := m.game
	id := m.groupID
	g.enqueueUI(func() {
		if !enabled {
			if err := g.graph.ClearGroupRules(id); err == nil {
				emitGroupChanged(id)
			}
			return
		}
		if err := g.graph.SetGroupRules(id, []model.GroupRule{r}); err == nil {
			emitGroupChanged(id)
		}
	})
}

func (m *GroupMenu) setRuleEnabled(on bool) {
	r, _ := m.currentRule()
	m.writeRule(r, on)
}

func (m *GroupMenu) stepRuleDelta(dir int) {
	r, enabled := m.currentRule()
	step := 1.0
	if r.Param != model.GroupParamPitch {
		step = 0.05 // multiplier-offset step for vol/dur
	}
	r.Delta += float64(dir) * step
	m.writeRule(r, enabled || true) // stepping delta implicitly enables
}

func (m *GroupMenu) stepRuleEveryN(dir int) {
	r, enabled := m.currentRule()
	r.EveryN += dir
	if r.EveryN < 1 {
		r.EveryN = 1
	}
	m.writeRule(r, enabled || true)
}

func (m *GroupMenu) cycleRuleParam() {
	r, enabled := m.currentRule()
	switch r.Param {
	case model.GroupParamPitch:
		r.Param = model.GroupParamVolume
	case model.GroupParamVolume:
		r.Param = model.GroupParamDuration
	default:
		r.Param = model.GroupParamPitch
	}
	// Reset clamps so normalizeGroupRules re-derives per-param defaults.
	r.Min, r.Max = 0, 0
	m.ruleParam = r.Param
	m.writeRule(r, enabled)
}

func (m *GroupMenu) deleteGroup() {
	g := m.game
	id := m.groupID
	g.enqueueUI(func() {
		if err := g.graph.DeleteGroup(id); err == nil {
			emitGroupDeleted(id)
		}
		m.Close()
	})
}
```

Then add, in the same file: `layoutAt(x, y int)` (fixed-size panel, clamp to `g.split.GridW/GridH`, fill `m.rects` for each control id: `title`, `close`, `vol-`, `vol+`, `pit-`, `pit+`, `dur-`, `dur+`, `param`, `delta-`, `delta+`, `n-`, `n+`, `ruleonoff`, `delete`), `ensureButtons()` (mirror the sidebar's `mk` builder at `game_node_sidebar.go:386-412`: `NewButton(label, DropdownStyle, func(){ g.enqueueUI(onClick) })`, `SetRect`, `ConsumeOnPress = true`; bind each id to the corresponding method above), `HandleInput(x, y int, pressed bool) InputResult` (press inside panel → find the button rect containing the point, fire its OnClick, return `InputConsumed`; press inside panel but on no control → `InputConsumed` (opaque); press outside → `Close()` + `InputIgnored` — match how the sidebar treats outside-clicks, see its `HandleInput`), and `Draw(dst *ebiten.Image)` (mirror sidebar Draw at `game_node_sidebar.go:1013`: `drawPanelShadow`, `drawRoundedRect(dst, panel, colPanelBG, RadiusLG, true)`, title via the sidebar's text call, then each button's draw call — copy the exact button-draw invocation the sidebar uses; labels via `i18n.T(...)` keys from the table above; rule readouts via `fmt.Sprintf(i18n.T(i18n.KeyGroupEveryN), r.EveryN)`).

Also implement any other methods the `InputHandler` interface (`input_handler.go:15`) requires — copy `NodeSidebar`'s stubs (`HandleWheel` → ignore, `Capturing()` → false).

Wiring:
1. `game_new.go`: `g.groupMenu = NewGroupMenu(g)` next to `g.sidebar = NewNodeSidebar(g)` (grep for it).
2. `grid_tree.go`: add `GZGroupMenu = 62` between `GZSidebar` and `GZCursorLabel`.
3. `grid_pane_layers.go` `registerGridTree`: add after the sidebar layer:

```go
	g.gridTree.RegisterLayer(gridLayer{id: "grid-group-menu", z: GZGroupMenu, clip: false, vis: func() bool { return g.groupMenu.IsOpen() }, draw: func(dst *ebiten.Image) { g.groupMenu.Draw(dst) }})
```

4. `game_update.go` dispatcher rebuild block (line ~345): extend the dirty condition and registration:

```go
	sidebarOpen := g.sidebar.IsOpen()
	groupMenuOpen := g.groupMenu.IsOpen()
	if g.dispatcherDirty || sidebarOpen != g.lastDispatcherSidebarOpen || groupMenuOpen != g.lastDispatcherGroupMenuOpen {
		...
		g.lastDispatcherGroupMenuOpen = groupMenuOpen
		...
		if groupMenuOpen {
			g.inputDispatcher.Register(g.groupMenu)
		}
```

Add the `lastDispatcherGroupMenuOpen bool` field beside `lastDispatcherSidebarOpen` (grep its declaration).
5. Menu-close on group-vanish: in `onGroupsChanged()` (Task 4 file), add before the Playing branch:

```go
	if g.groupMenu != nil && g.groupMenu.IsOpen() {
		if _, ok := g.graph.Group(g.groupMenu.GroupID()); !ok {
			g.groupMenu.Close()
		}
	}
```

6. `menuHit` (`game_input_drag.go:70`): include the group menu so pan doesn't fight it: `return g.sidebar.Hit(x, y) || g.groupMenu.Hit(x, y) || image.Pt(x, y).In(g.gridHelpButtonRect())`.
7. i18n: add the 11 keys per the table (keys.go + both catalogs).

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestGroupMenu'` → PASS.
Then i18n completeness: `... ./internal/i18n/` → PASS.
Then token/glyph discipline: `... ./internal/ui/ -run 'TestTokenDiscipline|TestDesignMDDrift'` → PASS (no new literals).

---

### Task 7: Marquee gesture — Shift+drag from empty space

**Files:**
- Create: `src/go/internal/ui/game_marquee.go`
- Create: `src/go/internal/ui/game_marquee_test.go`
- Modify: `src/go/internal/ui/game_input_editor.go` (shift block, lines 137-161)
- Modify: `src/go/internal/ui/game_update.go` (panOK, line ~585)
- Modify: `src/go/internal/ui/grid_tree.go` (`GZMarquee = 46`)
- Modify: `src/go/internal/ui/grid_pane_layers.go` (marquee draw layer)
- Modify: `src/go/internal/ui/game_ui_types.go` (marquee struct beside `dragLink`, line ~30)

**Interfaces:**
- Consumes: `nodeScreenRect(n) (x1,y1,x2,y2 float64)` (`game_node_screen_rect.go:8`), `g.nodes []*uiNode`, `g.nodeAtScreen`, Task 1 `CreateGroup`, Task 3 `emitGroupCreated`, Task 6 `groupMenu.OpenAt`.
- Produces: `marqueeDrag` struct + `Game.marquee` field; `(g *Game) handleMarquee(left bool, x, y int) bool` (returns true when it consumed the frame); `(g *Game) marqueeRect() image.Rectangle`; `(g *Game) drawGridMarquee(dst *ebiten.Image)`.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/game_marquee_test.go`. Drive the REAL input path with `SetInputForTest` (pattern from `testutil_test.go`'s `click` helper — shift key via the key-func argument):

```go
package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// shiftDrag simulates press at (x1,y1), drag to (x2,y2), release — all with
// Shift held. Frames advance through the real Update loop.
func shiftDrag(g *Game, x1, y1, x2, y2 int) {
	pos := [2]int{x1, y1}
	down := true
	restore := SetInputForTest(
		func() (int, int) { return pos[0], pos[1] },
		func(b ebiten.MouseButton) bool { return down && b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return k == ebiten.KeyShiftLeft },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
	)
	defer restore()
	advanceFrames(g, 2) // press
	pos = [2]int{x2, y2}
	advanceFrames(g, 3) // drag
	down = false
	advanceFrames(g, 2) // release
}

func TestMarqueeCreatesGroupAndOpensMenu(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	// Compute a screen rect that encloses both nodes with margin.
	ax1, ay1, _, _ := g.nodeScreenRect(g.nodesByID[aID])
	_, _, bx2, by2 := g.nodeScreenRect(g.nodesByID[bID])
	shiftDrag(g, int(ax1)-20, int(ay1)-20, int(bx2)+20, int(by2)+20)

	groups := g.graph.AllGroups()
	if len(groups) != 1 {
		t.Fatalf("want 1 group after marquee, got %d", len(groups))
	}
	if len(groups[0].NodeIDs) != 2 {
		t.Fatalf("want both nodes in group, got %v", groups[0].NodeIDs)
	}
	if !g.groupMenu.IsOpen() || g.groupMenu.GroupID() != groups[0].ID {
		t.Fatalf("group menu must open for the new group")
	}
	// No node was created/selected by the release (marquee ≠ click).
	if len(g.nodes) != 2 {
		t.Fatalf("marquee must not add nodes, have %d", len(g.nodes))
	}
}

func TestMarqueeOverEmptySpaceIsNoOp(t *testing.T) {
	g, _, _ := buildGroupLoopCircuit(t)
	// Far corner away from both nodes.
	shiftDrag(g, 600, 400, 700, 480)
	if n := len(g.graph.AllGroups()); n != 0 {
		t.Fatalf("empty marquee must create no group, got %d", n)
	}
	if g.groupMenu.IsOpen() {
		t.Fatalf("menu must not open on empty marquee")
	}
}

func TestShiftClickWithoutDragIsNoOp(t *testing.T) {
	g, _, _ := buildGroupLoopCircuit(t)
	shiftDrag(g, 600, 400, 601, 401) // < 4px movement
	if n := len(g.graph.AllGroups()); n != 0 {
		t.Fatalf("tiny shift-click must be a no-op, got %d groups", n)
	}
}

func TestShiftDragFromNodeStillCreatesEdge(t *testing.T) {
	// Regression guard: link-drag (shift+drag STARTING ON a node) untouched.
	g, aID, bID := buildGroupLoopCircuit(t)
	g.deleteEdge(g.nodesByID[aID], g.nodesByID[bID]) // remove a→b, re-create via gesture
	ax1, ay1, ax2, ay2 := g.nodeScreenRect(g.nodesByID[aID])
	bx1, by1, bx2, by2 := g.nodeScreenRect(g.nodesByID[bID])
	shiftDrag(g, int((ax1+ax2)/2), int((ay1+ay2)/2), int((bx1+bx2)/2), int((by1+by2)/2))
	if _, ok := g.graph.Edges[[2]model.NodeID{aID, bID}]; !ok {
		t.Fatalf("link drag from node must still create the edge")
	}
	if n := len(g.graph.AllGroups()); n != 0 {
		t.Fatalf("link drag must not create a group, got %d", n)
	}
}
```

(Verify `deleteEdge`'s exact name/signature via grep; keep the assertion. If the test constructor spawns nodes at screen positions outside the default window, adjust node grid coords so both `nodeScreenRect`s are on-screen — print them with `t.Logf` if the first run misses.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestMarquee|TestShiftClick|TestShiftDragFromNode'`
Expected: `TestShiftDragFromNode` PASSES (existing behavior); the marquee tests FAIL (no group created).

- [ ] **Step 3: Implement**

1. `game_ui_types.go` — beside `dragLink` (line ~30):

```go
type marqueeDrag struct {
	active         bool
	startX, startY int // screen px at press
	curX, curY     int // screen px, updated each frame while held
}
```

Add `marquee marqueeDrag` to the `Game` struct beside the `linkDrag` field (grep `linkDrag dragLink` for the declaring file).

2. Create `src/go/internal/ui/game_marquee.go`:

```go
package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

const marqueeMinDragPx = 4

// marqueeRect returns the normalized screen-space selection rectangle.
func (g *Game) marqueeRect() image.Rectangle {
	return image.Rect(g.marquee.startX, g.marquee.startY, g.marquee.curX, g.marquee.curY).Canon()
}

// handleMarquee owns the Shift+drag-from-empty-space gesture. Called from
// handleEditor's shift branch AFTER link-drag had its chance (link-drag claims
// Shift+press ON a node; marquee claims Shift+press on empty space). Returns
// true while the marquee owns the gesture.
func (g *Game) handleMarquee(left bool, x, y int) bool {
	if !g.marquee.active {
		if left && !g.leftPrev && g.nodeAtScreen(x, y) == nil {
			g.marquee = marqueeDrag{active: true, startX: x, startY: y, curX: x, curY: y}
			return true
		}
		return false
	}
	if left {
		g.marquee.curX, g.marquee.curY = x, y
		return true
	}
	// Release: select + create group.
	r := g.marqueeRect()
	g.marquee = marqueeDrag{}
	if r.Dx() < marqueeMinDragPx && r.Dy() < marqueeMinDragPx {
		return true // shift+click on empty space stays a no-op
	}
	ids := g.nodesInScreenRect(r)
	if len(ids) == 0 {
		return true
	}
	gid, err := g.graph.CreateGroup("", ids)
	if err != nil {
		g.logger.Debugf("[group] marquee create failed: %v", err)
		return true
	}
	grp, _ := g.graph.Group(gid)
	emitGroupCreated(gid, grp.Name, grp.NodeIDs)
	g.groupMenu.OpenAt(gid, x, y)
	g.logger.Debugf("[group] marquee created group=%d nodes=%d", gid, len(ids))
	return true
}

// nodesInScreenRect returns IDs of visible nodes whose screen rect intersects r.
func (g *Game) nodesInScreenRect(r image.Rectangle) []model.NodeID {
	var out []model.NodeID
	for _, n := range g.nodes {
		if mn, ok := g.graph.GetNodeByID(n.ID); !ok || mn.Type == model.NodeTypeInvisible {
			continue
		}
		x1, y1, x2, y2 := g.nodeScreenRect(n)
		nr := image.Rect(int(x1), int(y1), int(x2), int(y2))
		if nr.Overlaps(r) {
			out = append(out, n.ID)
		}
	}
	return out
}

// drawGridMarquee renders the live selection rectangle (GZMarquee layer).
func (g *Game) drawGridMarquee(dst *ebiten.Image) {
	if !g.marquee.active {
		return
	}
	r := g.marqueeRect()
	if r.Empty() {
		return
	}
	// Translucent accent fill + 1px border, via theme tokens only (token
	// ratchet: no inline color literals — use the accent token + WithAlpha,
	// matching how other grid overlays pick their colors).
	fill := WithAlpha(TokenPrimary(), AlphaFaint)
	border := WithAlpha(TokenPrimary(), AlphaStrong)
	drawRect(dst, r, fill, true)
	drawRect(dst, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1), border, true)
	drawRect(dst, image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y), border, true)
	drawRect(dst, image.Rect(r.Min.X, r.Min.Y, r.Min.X+1, r.Max.Y), border, true)
	drawRect(dst, image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y), border, true)
}
```

(`TokenPrimary()` is illustrative — use the real accent accessor from `theme_tokens.go`; grep for the accessor the selected-node ring uses and take the same one. `drawRect(dst, rect, col, fill bool)` signature verified at `game_node_sidebar.go:1036`.)

3. `game_input_editor.go` — extend the shift branch (lines 137-161). Replace the block header logic so marquee participates:

```go
	// ---------------- link drag / marquee (shift held or in progress) ----
	if g.linkDrag.active || g.marquee.active || shift {
		// Link drag claims Shift+press ON a node (existing behavior, first).
		if left && !g.linkDrag.active && !g.marquee.active && shift {
			if n := g.nodeAtScreen(x, y); n != nil {
				g.linkDrag = dragLink{from: n, active: true}
			}
		}
		if g.linkDrag.active {
			if !left {
				// ... existing release/commit body unchanged ...
			} else {
				g.linkDrag.toX, g.linkDrag.toY = gx, gy
			}
			g.leftPrev = left
			return
		}
		// Marquee claims Shift+press on EMPTY space.
		g.handleMarquee(left, x, y)
		g.leftPrev = left
		return
	}
```

Preserve the existing link-drag release body verbatim inside the restructured branch, and note the original block did NOT update `g.leftPrev` before `return` — it relied on the fall-through... **check**: the original `return`s inside the shift block skip the trailing `g.leftPrev = left` at line 257, which means `leftPrev` was stale during link drags. Mirror the original control flow EXACTLY for the link-drag path (don't fix/alter its `leftPrev` handling) and set `g.leftPrev = left` only on the marquee path (the marquee needs a correct press edge). If tests show the press edge misfires, drive the marquee arm off its own `prev` field inside `marqueeDrag` instead of `g.leftPrev`.

4. `game_update.go:585` — add to `panOK`: `&& !g.marquee.active` (belt-and-suspenders; shift already blocks pan).

5. `grid_tree.go`: add `GZMarquee = 46` between `GZConnectMode` (44) and `GZLongPress` (50). `grid_pane_layers.go`:

```go
	g.gridTree.RegisterLayer(gridLayer{id: "grid-marquee", z: GZMarquee, clip: true, vis: func() bool { return g.marquee.active }, draw: g.drawGridMarquee})
```

- [ ] **Step 4: Run tests to verify they pass**

Run: Step 2 command → all PASS.
Then the full grid-input neighborhood: `... ./internal/ui/ -run 'Grid|LinkDrag|Editor|Pan|Camera'` and the z-order guard `... -run 'TestGridLayerZOrder'` (add the new layers to its expected table if it pins the full list — read `grid_layer_zorder_test.go` and extend its want-list with `grid-marquee` (46) and `grid-group-menu` (62)).

---

### Task 8: Member rings + sidebar group chips

**Files:**
- Modify: `src/go/internal/ui/grid_pane_draw.go` (`drawGridNodes` selected-ring seam, lines ~701 and ~1035)
- Modify: `src/go/internal/ui/game_node_sidebar.go` (new "Groups" section)
- Create: `src/go/internal/ui/game_group_visuals_test.go`

**Interfaces:**
- Consumes: `groupIndexSnapshot().Member(id)` (Task 2/4), `groupMenu.OpenAt`/`IsOpen`/`GroupID` (Task 6), `graph.GroupsForNode` (Task 1), sidebar section machinery (`layoutSection`, `mk` — `game_node_sidebar.go:221`, `386`).
- Produces: nothing new for later tasks.

- [ ] **Step 1: Write the failing test**

```go
package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

func TestSidebarGroupChipOpensGroupMenu(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, _ := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	g.sidebar.Open(g.nodesByID[aID])
	advanceFrames(g, 1)
	// Fire the chip's action directly (button plumbing is covered by the
	// sidebar's own tests); the chip id is "grpchip:<gid>".
	g.sidebar.fireGroupChip(gid)
	advanceFrames(g, 2)
	if g.sidebar.IsOpen() {
		t.Fatalf("sidebar must close when a group chip opens the group menu")
	}
	if !g.groupMenu.IsOpen() || g.groupMenu.GroupID() != gid {
		t.Fatalf("group menu must open for chip's group")
	}
}

func TestGroupMembershipQueryForRendering(t *testing.T) {
	g, aID, _ := buildGroupLoopCircuit(t)
	if g.groupIndexSnapshot().Member(aID) {
		t.Fatalf("no groups yet: Member must be false")
	}
	_, _ = g.graph.CreateGroup("", []model.NodeID{aID})
	if !g.groupIndexSnapshot().Member(aID) {
		t.Fatalf("Member(a) must be true after grouping (index refreshed via hook)")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `... ./internal/ui/ -run 'TestSidebarGroupChip|TestGroupMembershipQuery'`
Expected: compile FAIL (`fireGroupChip` undefined). `TestGroupMembershipQuery` may already pass — fine.

- [ ] **Step 3: Implement**

1. **Rings** in `drawGridNodes` (`grid_pane_draw.go`): at the selected-ring decision seam (~line 701 / 1035), add a group ring for member nodes. Pattern — read how `isSelected` draws its ring, then:

```go
	gi := g.groupIndexSnapshot()
	isGroupMember := gi.Member(n.ID)
	menuGroupMember := false
	if g.groupMenu.IsOpen() {
		if grp, ok := g.graph.Group(g.groupMenu.GroupID()); ok {
			for _, m := range grp.NodeIDs {
				if m == n.ID {
					menuGroupMember = true
					break
				}
			}
		}
	}
```

Draw: faint ring (`WithAlpha(<accent token>, AlphaSubtle)`) for `isGroupMember`, strong ring (`AlphaStrong`) for `menuGroupMember` — reuse the exact ring-draw call the selected state uses, one radius step larger so both can show. **Perf note**: `drawGridNodes` is per-frame; `Member()` is a map hit — fine; but hoist the open-menu member set OUT of the per-node loop (compute `menuMembers map[model.NodeID]bool` once before the loop).

2. **Sidebar chips** in `game_node_sidebar.go`: in the section-layout run (`layout`, near line 221) add, after the existing sections, a `grp` section shown only when `len(g.graph.GroupsForNode(node.ID)) > 0`; title `i18n.T(i18n.KeyGroupMenuTitle)`. For each group id, lay a rect+button `fmt.Sprintf("grpchip:%d", gid)` labeled with the group's Name via the existing `mk` builder, `onClick` → `sb.fireGroupChip(gid)`. Add:

```go
// fireGroupChip closes the sidebar and opens the GroupMenu for gid, anchored
// at the sidebar's panel origin.
func (sb *NodeSidebar) fireGroupChip(gid model.GroupID) {
	g := sb.game
	panel := sb.rects["panel"]
	sb.Close()
	g.groupMenu.OpenAt(gid, panel.Min.X+20, panel.Min.Y+20)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: Step 2 command → PASS. Then sidebar + draw suites: `... ./internal/ui/ -run 'Sidebar|NodeSidebar|DrawNodes'` and the alloc/discipline guards: `... -run 'TestTokenDiscipline|Alloc'` → PASS (hoisted member-set keeps per-frame allocs flat; if an alloc-budget test trips, move the `menuMembers` map to a reused `Game` scratch field).

---

### Task 9: JS exports + bridge catalogue

**Files:**
- Modify: `src/go/internal/ui/js_exports_graph_ui.go` (new exports beside `setNodeLogicGrid`, line ~115)
- Modify: `src/js/wasm_bridge_smoke.browser.test.js` (catalogue entries)
- Check: `src/go/internal/ui/js_exports_catalogue_drift_test.go` (drift guard will fail until the JS catalogue lists the new exports)

**Interfaces:**
- Consumes: Task 1 API, Task 3 emit helpers.
- Produces JS globals: `createNodeGroup(id1, id2, ...) → groupId|-1`, `setGroupRule(groupId, param, delta, everyN) → bool`, `deleteGroup(groupId) → bool`, `dumpGroups() → JSON string of AllGroups()`.

- [ ] **Step 1: Add the exports** (mirror the `jsFn(func(args jsArgs) any {...})` idiom at `js_exports_graph_ui.go:115-145`; read two neighboring exports first for the args-accessor methods):

```go
	js.Global().Set("createNodeGroup", jsFn(func(args jsArgs) any {
		ids := make([]model.NodeID, 0, args.Len())
		for k := 0; k < args.Len(); k++ {
			ids = append(ids, model.NodeID(args.Int(k)))
		}
		gid, err := g.graph.CreateGroup("", ids)
		if err != nil {
			return -1
		}
		grp, _ := g.graph.Group(gid)
		emitGroupCreated(gid, grp.Name, grp.NodeIDs)
		return int(gid)
	}))
	js.Global().Set("setGroupRule", jsFn(func(args jsArgs) any {
		gid := model.GroupID(args.Int(0))
		rule := model.GroupRule{
			Param:  model.GroupParam(args.String(1)),
			Delta:  args.Float(2),
			EveryN: args.Int(3),
		}
		if err := g.graph.SetGroupRules(gid, []model.GroupRule{rule}); err != nil {
			return false
		}
		emitGroupChanged(gid)
		return true
	}))
	js.Global().Set("deleteGroup", jsFn(func(args jsArgs) any {
		gid := model.GroupID(args.Int(0))
		if err := g.graph.DeleteGroup(gid); err != nil {
			return false
		}
		emitGroupDeleted(gid)
		return true
	}))
	js.Global().Set("dumpGroups", jsFn(func(args jsArgs) any {
		b, err := json.Marshal(g.graph.AllGroups())
		if err != nil {
			return "[]"
		}
		return string(b)
	}))
```

(Adapt `args.Int/Float/String/Len` to the real `jsArgs` accessor names — grep `type jsArgs` first.)

- [ ] **Step 2: Update the JS catalogue.** In `src/js/wasm_bridge_smoke.browser.test.js`, add entries for the four exports following the file's existing entry shape (existence + callable with safe args: `createNodeGroup()` with no args safely returns -1, `dumpGroups()` returns a JSON string, `setGroupRule(9999, 'pitch', 1, 1)` returns false, `deleteGroup(9999)` returns false).

- [ ] **Step 3: Verify**

Run drift guard: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Catalogue'` → PASS.
Compile the js build: `cd /home/ymolinar/Repos/beatmo/src/go && GOOS=js GOARCH=wasm ../../.tools/go/bin/go build -tags test -modfile=go.test.mod ./...` → OK.
(Optional, if a WASM toolchain run is cheap here: `GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js`.)

---

### Task 10: Verification sweep

**Files:** none new (fixes only if failures appear).

- [ ] **Step 1: Full fast suite**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...`
Expected: PASS. Known pre-existing failures on this branch are catalogued in memory `project_add_core_node_types_preexisting_failures` — compare against a pre-change baseline (`git stash` NOT allowed; instead run the failing test names and check they fail identically before/after by inspecting whether they touch group code).

- [ ] **Step 2: Race pass on touched concurrency**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -race -tags test -modfile=go.test.mod ./core/model/ ./internal/ui/ -run 'Group'`
Expected: PASS (index snapshot swap under `groupIdxMu`; reads under seqMu).

- [ ] **Step 3: Parity + logic suites**

Run: `... -run 'Parity|Audible|NodeLogic' ./internal/ui/ ./core/engine/`
Expected: PASS — group rules don't alter trigger decisions.

- [ ] **Step 4: Verify end-to-end behavior (superpowers:verification-before-completion + verify skill)**

Desktop smoke: `make run` manually is NOT required (no manual test steps — house rule); instead run the functional round-boundary test one more time with `-count=2` for flake check, and `make screenshot` if the marquee/menu should be visually spot-checked via the existing screenshot harness (optional scene addition is out of scope).

- [ ] **Step 5: Spec conformance check** — re-read `docs/superpowers/specs/2026-07-09-node-group-selection-design.md` §§1-9 and confirm each requirement maps to shipped code/tests; note any deliberate deviation at the bottom of the spec.

---

## Self-review notes (already applied)

- **Spec coverage**: §1 → Tasks 1-2; §2 (API) → Task 1; §3 (evaluator) → Tasks 2+4 (single chokepoint `evalNodeParamsOnly` — spec corrected); §4 (UI) → Tasks 6-8; §5 (persistence) → Task 5; §6 (hooks/undo) → Task 3; §7 (JS) → Task 9; §8 (testing) → embedded per task + Task 10; §9 (errors) → Tasks 1, 5, 6, 7.
- **Known adaptation points** (executor must resolve by reading the named file, NOT by guessing): test-game constructor name (Task 4/6/7 tests), `undoManager.Len()` depth accessor (Task 6), accent-token accessor for marquee/rings (Task 7/8), `jsArgs` accessor names (Task 9), exact button-draw invocation (Task 6). Each is annotated inline where it appears.
- **Type consistency**: `GroupID`/`GroupParam`/`GroupRule`/`NodeGroup`/`GroupIndex` names and signatures are identical across Tasks 1-9.
