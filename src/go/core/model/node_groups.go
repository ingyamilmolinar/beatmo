package model

import (
	"errors"
	"fmt"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/enum"
)

// GroupID identifies a node group within a Graph.
type GroupID int

// GroupParam names the node parameter a GroupRule mutates. GroupParamNone
// is no parameter: SetGroupRules rejects a rule naming it.
type GroupParam int

const (
	GroupParamNone GroupParam = iota
	GroupParamPitch
	GroupParamVolume
	GroupParamDuration
)

var groupParamNames = enum.Names[GroupParam]{Kind: "group rule param", Names: []string{"", "pitch", "volume", "duration"}}

func (p GroupParam) String() string                { return enum.String(groupParamNames, p) }
func (p GroupParam) MarshalText() ([]byte, error)  { return enum.Marshal(groupParamNames, p) }
func (p *GroupParam) UnmarshalText(b []byte) error { return enum.Unmarshal(groupParamNames, b, p) }

// ParseGroupParam reads a rule param spelled in data.
func ParseGroupParam(s string) (GroupParam, error) { return enum.Parse(groupParamNames, s) }

// GroupRule: every EveryN rounds, accumulate Delta on Param. The accumulated
// effect (Delta × steps) clamps to [Min, Max]. Zero Min+Max on input means
// "use per-param defaults" (set at store time by SetGroupRules).
//
// AnchorIdx is the absolute subdivision index at which the rule was enabled
// (0 = timeline origin, backward compatible with rules created before this
// field existed). Steps accumulate from AnchorIdx's round, not from the
// timeline origin — see GroupEffect. Negative values normalize to 0 in
// normalizeGroupRules.
type GroupRule struct {
	Param     GroupParam
	Delta     float64
	EveryN    int
	Min       float64
	Max       float64
	AnchorIdx int
}

// NodeGroup is a persistent set of nodes sharing batch edits and rules.
// A node may belong to several groups; rule effects stack (pitch deltas sum,
// volume/duration multipliers multiply).
//
// RulesEnabled is an INDEPENDENT status bit, decoupled from whether Rules is
// non-empty (task-enabled-status brief, Part C): a group can carry configured
// delta rules while RulesEnabled is false (edited but not yet turned on), or
// have RulesEnabled true with an empty Rules slice (a no-op — HasRules is
// still false either way). BuildGroupIndex is the single gate: it only folds
// a group's Rules into the per-node rule index when RulesEnabled is true —
// Member() (ring rendering) is unaffected by the flag. Defaults false (the
// zero value), so every freshly created group — including a marquee
// provisional one — starts Disabled until explicitly enabled and saved.
// Mutated only via SetGroupRulesEnabled; SetGroupRules never touches it.
type NodeGroup struct {
	ID           GroupID
	Name         string
	NodeIDs      []NodeID
	Rules        []GroupRule
	RulesEnabled bool
}

var (
	ErrGroupNotFound    = errors.New("group not found")
	ErrGroupEmptyNodes  = errors.New("group needs at least one node")
	ErrGroupUnknownNode = errors.New("group references unknown node")
	ErrGroupBadRule     = errors.New("invalid group rule")
)

// NodePitchMin / NodePitchMax bound a node's semitone pitch offset: two
// octaves either side of the instrument's base note.
//
// Exported because it is not only the group-rule clamp — the UI's note-matrix
// picker enumerates exactly this range, and the two must not drift. Any other
// consumer that needs "how far can a node's pitch move" reads these.
const (
	NodePitchMin = -24.0
	NodePitchMax = 24.0
)

// groupRuleDefaultClamp returns the default accumulated-effect clamp per param.
func groupRuleDefaultClamp(p GroupParam) (float64, float64) {
	switch p {
	case GroupParamPitch:
		return NodePitchMin, NodePitchMax
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

// ResetGroups clears every group, unlike assigning an empty map to Groups
// directly (kept as its own method for symmetry with the rest of the
// document-replace reset helpers, and so call sites don't need to know
// Groups' zero value). Import's replace-not-merge reset uses this.
//
// Group numbering is allocated fresh by lowestFreeGroupID on every
// CreateGroup (see its doc comment) — there is no counter to reset here:
// re-importing the SAME document twice mints the same GroupIDs each time
// (file order -> IDs 1..n), keeping exported bytes deterministic/byte-stable
// (undo/redo's byte-for-byte round-trip depends on this).
func (g *Graph) ResetGroups() {
	g.Groups = map[GroupID]*NodeGroup{}
}

// lowestFreeGroupID returns the smallest positive GroupID not currently
// present in g.Groups (linear scan — group counts are tiny). This subsumes
// the old tail-rollback allocator (nextGroupID + ReleaseGroupID, removed):
// a group's number becomes reusable the instant it's deleted, whether that's
// a discarded provisional (GroupMenu.discardProvisional), an explicit delete
// (the menu's Delete button), or a node-deletion auto-delete
// (removeNodeFromAllGroups) — no separate release call needed, and no
// aliasing risk (a stale UI handle onto a deleted-then-reused id is safe by
// construction: consumers close/refresh on vanish via the group-changed
// hook — see GroupMenu.group()/onGroupsChanged).
func (g *Graph) lowestFreeGroupID() GroupID {
	id := GroupID(1)
	for {
		if _, ok := g.Groups[id]; !ok {
			return id
		}
		id++
	}
}

// CreateGroup creates a group over nodeIDs (deduped). Empty name gets
// "Group N". Returns the new GroupID — the lowest free id (see
// lowestFreeGroupID), not a monotonically growing counter.
func (g *Graph) CreateGroup(name string, nodeIDs []NodeID) (GroupID, error) {
	ids, err := g.dedupValidateNodeIDs(nodeIDs)
	if err != nil {
		return 0, err
	}
	if g.Groups == nil {
		g.Groups = map[GroupID]*NodeGroup{}
	}
	id := g.lowestFreeGroupID()
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
		if r.AnchorIdx < 0 {
			r.AnchorIdx = 0
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

// SetGroupRulesEnabled sets the group's independent RulesEnabled status bit
// (task-enabled-status brief, Part C) — it NEVER touches Rules. Firing the
// group-changed hook here is what makes toggling live: onGroupsChanged
// rebuilds the GroupIndex (BuildGroupIndex re-reads RulesEnabled) and, while
// playing, drops in-flight audio so the very next re-predicted trigger
// reflects the new gate — enabling a group with nonzero deltas is
// immediately audible, disabling silences the ramp back to identity.
func (g *Graph) SetGroupRulesEnabled(id GroupID, on bool) error {
	grp, ok := g.Groups[id]
	if !ok {
		return ErrGroupNotFound
	}
	grp.RulesEnabled = on
	g.fireGroupChanged(id)
	return nil
}

// ReanchorAllGroupRules re-anchors EVERY rule of EVERY rule-bearing group to
// anchor (clamped to >= 0), firing fireGroupChanged exactly ONCE PER GROUP
// touched (not once per rule), and returns the count of groups touched.
// Groups with no rules are skipped entirely (no-op, no hook fire).
//
// This is runtime ramp state, like a trigger counter — NOT a document edit:
// callers must NEVER wrap this in an undo/emit (see
// internal/ui/game_seek.go's handlePlaybackTransition, the single production
// call site). Semantics: AnchorIdx is ramp state for the CURRENT play
// session, so every fresh playback start (stop→play, not resume-from-pause)
// re-anchors all rules to the session's starting absolute subdivision index —
// otherwise a rule saved mid-session N (anchored at, say, abs 640) collapses
// to steps=0 for the entire region below that anchor once the playhead
// restarts at abs 0 in session N+1, per GroupEffect's
// anchor=min(anchor,absIdx) collapse (see GroupEffect's doc comment for the
// root-cause bug this fixes). Mid-play edit continuity (writeRules preserving
// a param's existing anchor on an in-place edit) is untouched — that applies
// WITHIN a session; this applies BETWEEN sessions.
func (g *Graph) ReanchorAllGroupRules(anchor int) int {
	if anchor < 0 {
		anchor = 0
	}
	touched := 0
	for _, grp := range g.Groups {
		if len(grp.Rules) == 0 {
			continue
		}
		for i := range grp.Rules {
			grp.Rules[i].AnchorIdx = anchor
		}
		touched++
		g.fireGroupChanged(grp.ID)
	}
	return touched
}

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
		ID:           grp.ID,
		Name:         grp.Name,
		NodeIDs:      append([]NodeID(nil), grp.NodeIDs...),
		Rules:        append([]GroupRule(nil), grp.Rules...),
		RulesEnabled: grp.RulesEnabled,
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
