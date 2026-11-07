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

// Delta × steps, every EveryN rounds, clamped to [Min, Max] (zero Min+Max = per-param defaults). AnchorIdx is where
// the rule was enabled (0 = origin): steps count from its round. Negative normalises to 0.
type GroupRule struct {
	Param     GroupParam
	Delta     float64
	EveryN    int
	Min       float64
	Max       float64
	AnchorIdx int
}

// A node may be in several groups; effects stack (pitch sums, vol/dur multiply). RulesEnabled is an INDEPENDENT bit:
// BuildGroupIndex folds Rules only while it is set, Member() ignores it, and it defaults false so every new group starts
// disabled. Only SetGroupRulesEnabled changes it.
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

// Two octaves either side of the instrument's base note. Exported: the note-matrix picker enumerates exactly this range.
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

// There is no counter to reset: lowestFreeGroupID mints ids fresh, so re-importing a document gives the same ids
// every time, which undo/redo's byte-for-byte round trip depends on.
func (g *Graph) ResetGroups() {
	g.Groups = map[GroupID]*NodeGroup{}
}

// A freed number is reusable at once, whatever freed it; a stale UI handle on a reused id is safe because consumers close
// or refresh on the group-changed hook.
func (g *Graph) lowestFreeGroupID() GroupID {
	id := GroupID(1)
	for {
		if _, ok := g.Groups[id]; !ok {
			return id
		}
		id++
	}
}

// The lowest free id, not a growing counter.
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

// Never touches Rules. The hook makes the toggle live: the index is rebuilt and, while playing, in-flight audio drops so
// the next trigger reflects it.
func (g *Graph) SetGroupRulesEnabled(id GroupID, on bool) error {
	grp, ok := g.Groups[id]
	if !ok {
		return ErrGroupNotFound
	}
	grp.RulesEnabled = on
	g.fireGroupChanged(id)
	return nil
}

// Runtime ramp state, NOT a document edit: never undo/emit it. Each fresh play (not resume) re-anchors every rule to the
// session's start, or a rule anchored late in one session collapses to steps 0 below that anchor in the next. Fires the
// hook once per group touched; returns the count.
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

// Called by Graph.RemoveNode, so no path that deletes a node leaves a dangling NodeID.
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
