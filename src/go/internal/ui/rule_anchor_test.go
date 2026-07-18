package ui

import (
	"sort"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// collectParityPitches reads the pitch the REAL scheduler computed at each
// captured trigger for row 0, keyed by absolute subdivision index. Ported
// from the controller's zz_evidence_rule_playback_test.go instrumentation
// (collectParityPitches over g.parityAudio under g.parityMu).
func collectParityPitches(g *Game) map[int]float64 {
	out := map[int]float64{}
	g.parityMu.Lock()
	defer g.parityMu.Unlock()
	for _, ev := range g.parityAudio {
		if ev.Row == 0 {
			out[ev.Abs] = ev.Pitch
		}
	}
	return out
}

// sortedIdxs returns the sorted keys of a pitch-by-abs map.
func sortedIdxs(pitches map[int]float64) []int {
	idxs := make([]int, 0, len(pitches))
	for i := range pitches {
		idxs = append(idxs, i)
	}
	sort.Ints(idxs)
	return idxs
}

// roundPitchesInOrder collapses a sorted slice of absolute trigger indices
// into one pitch value per DISTINCT round (round computed via the same
// loopStart/loopLen math GroupRound uses), in ascending round order. The
// 2-node test loop (buildEdge) fires twice per round (once per node), 32
// subdivisions apart, so consecutive raw indices can share the same round —
// this collapses that duplication so callers can assert "pitch increases by
// exactly Delta every ROUND" without hardcoding the fixture's node count.
func roundPitchesInOrder(idxs []int, pitches map[int]float64, loopStart, loopLen int) []float64 {
	var out []float64
	lastRound := -1
	haveLast := false
	for _, i := range idxs {
		round := 0
		if loopLen > 0 && i > loopStart {
			round = (i - loopStart) / loopLen
		}
		if haveLast && round == lastRound {
			continue
		}
		out = append(out, pitches[i])
		lastRound, haveLast = round, true
	}
	return out
}

// maxIdx returns the largest key in a pitch-by-abs map, or -1 if empty. Used
// as a watermark to split "before"/"after" triggers — a plain length
// comparison doesn't work because g.parityAudio is a bounded ring buffer:
// once it's full, adding new entries evicts old ones, so the total count can
// stay constant across a boundary even though the SET of captured indices has
// moved forward.
func maxIdx(pitches map[int]float64) int {
	m := -1
	for i := range pitches {
		if i > m {
			m = i
		}
	}
	return m
}

// TestRulePitchStepsPerRoundFromStart is the permanent regression of the
// working case (root-cause brief, Part C #2): a rule set BEFORE play starts
// (AnchorIdx 0, the timeline origin) must produce pitches equal to the round
// number exactly at each round's triggers. This must hold both before and
// after the anchored-rounds fix.
func TestRulePitchStepsPerRoundFromStart(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a, b := buildEdge(g)
	gid, err := g.graph.CreateGroup("", []model.NodeID{a.ID, b.ID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if err := g.graph.SetGroupRules(gid, []model.GroupRule{
		{Param: model.GroupParamPitch, Delta: 1, EveryN: 1},
	}); err != nil {
		t.Fatalf("SetGroupRules: %v", err)
	}
	// task-enabled-status brief Part C: RulesEnabled is independent of Rules
	// and defaults false — this test exercises rule APPLICATION, so it must
	// explicitly enable.
	if err := g.graph.SetGroupRulesEnabled(gid, true); err != nil {
		t.Fatalf("SetGroupRulesEnabled: %v", err)
	}

	loopStart := g.loopStartByRow[0]
	loopLen := len(g.beatInfosByRow[0]) - loopStart

	g.playFn = func(id string, vol float64, when ...float64) {}
	g.SetPlaying(true)
	setPlayStartForAbs(g, 0)
	_ = g.Update()
	advancePlaybackByAbs(g, 6*loopLen+2)
	g.SetPlaying(false)

	pitches := collectParityPitches(g)
	if len(pitches) == 0 {
		t.Fatalf("no parity audio captured — direct-play path not taken?")
	}
	for _, i := range sortedIdxs(pitches) {
		round := 0
		if loopLen > 0 && i > loopStart {
			round = (i - loopStart) / loopLen
		}
		want := float64(round)
		if got := pitches[i]; got != want {
			t.Fatalf("idx=%d round=%d pitch=%v, want %v (rule set from start must step exactly per round)", i, round, got, want)
		}
	}
}

// TestRuleEnabledMidPlaybackAnchorsAtZero is the root-cause repro (brief Part
// C #1). Playing ~10 rounds with NO rule, then enabling a pitch +1-every-1
// rule via the MENU path (the paths that must stamp anchors: setRuleEnabled/
// writeRule) must anchor the rule at the moment it's enabled: the first
// post-enable round's pitch is 0, then +1, then +2 — NOT +10/+11/+12 (the
// pre-fix bug, where GroupEffect used the absolute round number regardless of
// when the rule was set).
func TestRuleEnabledMidPlaybackAnchorsAtZero(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a, b := buildEdge(g)
	gid, err := g.graph.CreateGroup("", []model.NodeID{a.ID, b.ID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	loopStart := g.loopStartByRow[0]
	loopLen := len(g.beatInfosByRow[0]) - loopStart

	g.playFn = func(id string, vol float64, when ...float64) {}
	g.SetPlaying(true)
	setPlayStartForAbs(g, 0)
	_ = g.Update()
	// Play ~10 rounds with NO rule.
	advancePlaybackByAbs(g, 10*loopLen)
	// Snapshot the trigger watermark BEFORE enabling — the split point
	// between "before" and "post-enable" triggers (see maxIdx's doc comment
	// on why this must be a watermark, not a count).
	beforeMax := maxIdx(collectParityPitches(g))

	// Configure + enable the rule mid-playback via the MENU path
	// (stepRuleDelta writes the rule via writeRules — the anchor-stamping
	// path under test; setRuleEnabled flips the independent RulesEnabled bit,
	// task-enabled-status brief Part C — deltas alone never apply). One
	// rdpit+ step composes exactly "pitch +1 every 1 round" (scratch EveryN
	// defaults to 1).
	g.groupMenu.OpenAt(gid, 100, 100)
	g.groupMenu.stepRuleDelta(model.GroupParamPitch, 1)
	g.groupMenu.setRuleEnabled(true)
	advanceFrames(g, 1)
	advancePlaybackByAbs(g, 3*loopLen)
	g.SetPlaying(false)

	pitches := collectParityPitches(g)
	var post []int
	for _, i := range sortedIdxs(pitches) {
		if i > beforeMax {
			post = append(post, i)
		}
	}
	if len(post) < 3 {
		t.Fatalf("not enough post-enable triggers captured: %v (beforeMax=%d)", post, beforeMax)
	}
	// The pre-fix bug used the ABSOLUTE round number regardless of when the
	// rule was set: enabling ~10 rounds in produced pitches racing up from
	// ~+10 toward the ±24 clamp. The fix anchors the rule at the enable
	// moment, so the first post-enable ROUND's pitch must be small (nowhere
	// near the ~10-11 the bug produced), and every subsequent round must step
	// by exactly Delta (1 semitone) — linear accumulation from the anchor,
	// not from the timeline origin. roundPitchesInOrder collapses the
	// fixture's 2-triggers-per-round shape (buildEdge's a/b nodes) into one
	// pitch per round.
	byRound := roundPitchesInOrder(post, pitches, loopStart, loopLen)
	if len(byRound) < 3 {
		t.Fatalf("not enough distinct post-enable rounds: %v", byRound)
	}
	first := byRound[0]
	if first >= 3 {
		t.Fatalf("first post-enable round pitch=%v, want < 3 (anchor must reset near 0, not continue the pre-fix ~10/11 pattern)", first)
	}
	for k := 1; k < 3; k++ {
		want := first + float64(k)
		if byRound[k] != want {
			t.Fatalf("post-enable round #%d pitch=%v, want %v (must step by exactly Delta=1 per round from the anchor): %v", k, byRound[k], want, byRound)
		}
	}
}

// TestRuleToggleOffOnGatesWithoutReanchor is the task-enabled-status brief
// Part C port of the old "Off/On re-anchors" test: under the new INDEPENDENT
// RulesEnabled status bit, toggling Off/On (setRuleEnabled) is a pure gate —
// it NEVER touches Rules or AnchorIdx (that's writeRules' job, only invoked
// by delta/EveryN edits). A rule active from the start (anchor 0) accumulates
// for 5 rounds; disabling it makes pitch read flat identity (0) for as long
// as it stays disabled (BuildGroupIndex skips the group's rules entirely);
// re-enabling resumes the ramp at whatever the ORIGINAL anchor's formula
// produces for the CURRENT round — a "jump" reflecting the rounds that
// elapsed while disabled, not a reset back near 0 (that reset behavior
// belonged to the old "Off clears rules" design, removed by this task).
func TestRuleToggleOffOnGatesWithoutReanchor(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a, b := buildEdge(g)
	gid, err := g.graph.CreateGroup("", []model.NodeID{a.ID, b.ID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if err := g.graph.SetGroupRules(gid, []model.GroupRule{
		{Param: model.GroupParamPitch, Delta: 1, EveryN: 1},
	}); err != nil {
		t.Fatalf("SetGroupRules: %v", err)
	}
	if err := g.graph.SetGroupRulesEnabled(gid, true); err != nil {
		t.Fatalf("SetGroupRulesEnabled: %v", err)
	}

	loopStart := g.loopStartByRow[0]
	loopLen := len(g.beatInfosByRow[0]) - loopStart

	g.playFn = func(id string, vol float64, when ...float64) {}
	g.SetPlaying(true)
	setPlayStartForAbs(g, 0)
	_ = g.Update()
	advancePlaybackByAbs(g, 5*loopLen)
	preDisableMax := maxIdx(collectParityPitches(g))

	// Disable via the menu path — a pure gate, no rule/anchor mutation.
	g.groupMenu.OpenAt(gid, 100, 100)
	g.groupMenu.setRuleEnabled(false)
	advanceFrames(g, 1)
	if grp, ok := g.graph.Group(gid); !ok || len(grp.Rules) != 1 || grp.Rules[0].AnchorIdx != 0 {
		t.Fatalf("disabling must not touch Rules/AnchorIdx, got %+v ok=%v", grp, ok)
	}

	advancePlaybackByAbs(g, 3*loopLen)
	disabledPitches := collectParityPitches(g)
	for i, p := range disabledPitches {
		if i > preDisableMax && p != 0 {
			t.Fatalf("while disabled, pitch must read flat identity 0, got idx=%d pitch=%v", i, p)
		}
	}
	preReenableMax := maxIdx(disabledPitches)

	// Re-enable — still a pure gate.
	g.groupMenu.setRuleEnabled(true)
	advanceFrames(g, 1)
	if grp, ok := g.graph.Group(gid); !ok || len(grp.Rules) != 1 || grp.Rules[0].AnchorIdx != 0 {
		t.Fatalf("re-enabling must not touch Rules/AnchorIdx either, got %+v ok=%v", grp, ok)
	}

	advancePlaybackByAbs(g, 3*loopLen)
	g.SetPlaying(false)

	pitches := collectParityPitches(g)
	var post []int
	for _, i := range sortedIdxs(pitches) {
		if i > preReenableMax {
			post = append(post, i)
		}
	}
	if len(post) == 0 {
		t.Fatalf("no post-reenable triggers captured (preReenableMax=%d)", preReenableMax)
	}
	// The anchor never moved off 0, so GroupEffect's formula (steps == round,
	// since AnchorIdx==0 and EveryN==1) applies at every captured post-
	// reenable index — including ones with round >= 8, well past the pre-fix
	// design's "reset near 0" expectation.
	for _, i := range post {
		round := model.GroupRound(i, loopStart, loopLen)
		want := float64(round)
		if got := pitches[i]; got != want {
			t.Fatalf("idx=%d round=%d pitch=%v, want %v (re-enable must resume the original anchor's formula, not reset)", i, round, got, want)
		}
	}
}

// TestDisabledRulesAreInert is task-enabled-status brief test C4: a committed
// group with configured deltas but RulesEnabled false must produce FLAT
// (identity) pitch throughout playback — the rule is inert purely because
// BuildGroupIndex skips a disabled group's Rules (core/model/group_effect.go)
// — and enabling it via SetGroupRulesEnabled mid-session must make the ramp
// resume immediately (the session's existing AnchorIdx applies — "session
// anchor rules apply", matching how a live toggle behaves elsewhere in this
// suite, e.g. TestRuleToggleOffOnGatesWithoutReanchor).
func TestDisabledRulesAreInert(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a, b := buildEdge(g)
	gid, err := g.graph.CreateGroup("", []model.NodeID{a.ID, b.ID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if err := g.graph.SetGroupRules(gid, []model.GroupRule{
		{Param: model.GroupParamPitch, Delta: 1, EveryN: 1},
	}); err != nil {
		t.Fatalf("SetGroupRules: %v", err)
	}
	// Deliberately left Disabled (the model default) — pins that configuring
	// deltas alone never makes them audible.
	if grp, ok := g.graph.Group(gid); !ok || grp.RulesEnabled {
		t.Fatalf("setup: group must start Disabled, got %+v ok=%v", grp, ok)
	}

	loopStart := g.loopStartByRow[0]
	loopLen := len(g.beatInfosByRow[0]) - loopStart

	g.playFn = func(id string, vol float64, when ...float64) {}
	g.SetPlaying(true)
	setPlayStartForAbs(g, 0)
	_ = g.Update()
	advancePlaybackByAbs(g, 5*loopLen)

	flatPitches := collectParityPitches(g)
	if len(flatPitches) == 0 {
		t.Fatalf("no parity audio captured while disabled")
	}
	for i, p := range flatPitches {
		if p != 0 {
			t.Fatalf("disabled group must produce flat pitch, got idx=%d pitch=%v", i, p)
		}
	}
	preEnableMax := maxIdx(flatPitches)

	if err := g.graph.SetGroupRulesEnabled(gid, true); err != nil {
		t.Fatalf("SetGroupRulesEnabled: %v", err)
	}
	advanceFrames(g, 1)
	advancePlaybackByAbs(g, 3*loopLen)
	g.SetPlaying(false)

	pitches := collectParityPitches(g)
	sawRamp := false
	for i, p := range pitches {
		if i > preEnableMax {
			round := model.GroupRound(i, loopStart, loopLen)
			want := float64(round) // AnchorIdx==0, EveryN==1: steps == round
			if p != want {
				t.Fatalf("post-enable idx=%d round=%d pitch=%v, want %v (session anchor rule must resume)", i, round, p, want)
			}
			if p != 0 {
				sawRamp = true
			}
		}
	}
	if !sawRamp {
		t.Fatalf("expected at least one nonzero post-enable pitch (ramp must resume), got all zero: %v", pitches)
	}
}

// TestRuleAnchorRoundTripsThroughExportImport is brief Part C #4: a rule with
// a nonzero AnchorIdx must survive export -> Import byte-faithfully (the
// exported document, and the re-exported document after import, must be
// identical), and the imported rule's AnchorIdx must match exactly.
func TestRuleAnchorRoundTripsThroughExportImport(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if err := g.graph.SetGroupRules(gid, []model.GroupRule{
		{Param: model.GroupParamPitch, Delta: 2, EveryN: 3, AnchorIdx: 17},
	}); err != nil {
		t.Fatalf("SetGroupRules: %v", err)
	}

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(800, 600)
	if err := g2.Import(data); err != nil {
		t.Fatalf("Import: %v", err)
	}

	groups := g2.graph.AllGroups()
	if len(groups) != 1 || len(groups[0].Rules) != 1 {
		t.Fatalf("imported groups = %+v, want exactly 1 group with 1 rule", groups)
	}
	if got := groups[0].Rules[0].AnchorIdx; got != 17 {
		t.Fatalf("imported AnchorIdx = %d, want 17", got)
	}

	data2, err := g2.drum.exportBytes()
	if err != nil {
		t.Fatalf("re-Export: %v", err)
	}
	if string(data) != string(data2) {
		t.Fatalf("export -> Import -> export is not byte-faithful:\nfirst:  %s\nsecond: %s", data, data2)
	}
}

// ─── Part 2: rule-edit continuity (task-rule-continuity brief) ────────────
//
// Current (pre-fix) behavior: writeRules stamps a FRESH AnchorIdx on EVERY
// write, so an in-place delta/EveryN edit mid-play resets the ramp to 0 —
// audio jumps back to base and restarts. The required behavior: an in-place
// edit (the group already has a rule for that param) must PRESERVE the
// existing AnchorIdx so the step count continues; only a param gaining a
// rule for the first time (or Off->On) gets a fresh anchor.

// TestRuleDeltaEditMidPlaybackContinuesRamp is brief Part 2 test #1 (MUST
// FAIL against current code, which re-anchors on every write): a pitch rule
// active from the start (AnchorIdx 0) accumulates for 5 rounds; editing its
// delta mid-play (1 -> 2, an IN-PLACE edit — the rule already exists) must
// NOT re-anchor. Every round's expected pitch is derived directly from
// GroupEffect's own formula (steps == round, since the anchor never moves
// off 0): rounds up to and including the edit round keep the OLD delta,
// rounds after use the NEW delta — continuing the ramp, not resetting it.
func TestRuleDeltaEditMidPlaybackContinuesRamp(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a, b := buildEdge(g)
	gid, err := g.graph.CreateGroup("", []model.NodeID{a.ID, b.ID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	const initialDelta = 1.0
	if err := g.graph.SetGroupRules(gid, []model.GroupRule{
		{Param: model.GroupParamPitch, Delta: initialDelta, EveryN: 1},
	}); err != nil {
		t.Fatalf("SetGroupRules: %v", err)
	}
	if err := g.graph.SetGroupRulesEnabled(gid, true); err != nil {
		t.Fatalf("SetGroupRulesEnabled: %v", err)
	}

	loopStart := g.loopStartByRow[0]
	loopLen := len(g.beatInfosByRow[0]) - loopStart

	g.playFn = func(id string, vol float64, when ...float64) {}
	g.SetPlaying(true)
	setPlayStartForAbs(g, 0)
	_ = g.Update()

	const preEditRounds = 5
	advancePlaybackByAbs(g, preEditRounds*loopLen+2)

	// In-place delta edit via the menu path (stepRuleDelta -> writeRules) —
	// the group already has a pitch rule, so this must preserve its anchor.
	g.groupMenu.OpenAt(gid, 100, 100)
	g.groupMenu.stepRuleDelta(model.GroupParamPitch, 1) // delta 1 -> 2
	advanceFrames(g, 1)

	grp, ok := g.graph.Group(gid)
	if !ok || len(grp.Rules) != 1 {
		t.Fatalf("setup: want 1 rule after edit, got %+v", grp.Rules)
	}
	newDelta := grp.Rules[0].Delta
	if newDelta != initialDelta+1 {
		t.Fatalf("setup: delta after one stepRuleDelta = %v, want %v", newDelta, initialDelta+1)
	}
	if anchor := grp.Rules[0].AnchorIdx; anchor != 0 {
		t.Fatalf("in-place delta edit must preserve AnchorIdx=0, got %d (re-anchored instead of continuing the ramp)", anchor)
	}

	const postEditRounds = 2
	advancePlaybackByAbs(g, postEditRounds*loopLen)
	g.SetPlaying(false)

	// NOTE: onGroupsChanged (game_groups.go) wipes g.parityAudio whenever a
	// group rule changes while playing — it drops in-flight captures so the
	// new rule takes effect cleanly from the next re-predicted trigger. So
	// only captures recorded AFTER the edit are visible here; derive the
	// expected pitch per-capture from GroupEffect's own formula (round via
	// model.GroupRound, anchor preserved at 0 so steps == round) rather than
	// assuming a fixed pre/post round index list.
	after := collectParityPitches(g)
	if len(after) == 0 {
		t.Fatalf("no parity audio captured after the delta edit")
	}
	minRound := -1
	for abs, pitch := range after {
		round := model.GroupRound(abs, loopStart, loopLen)
		want := newDelta * float64(round)
		if pitch != want {
			t.Fatalf("abs=%d round=%d pitch=%v, want %v (anchor preserved at 0, new delta=%v, steps=round)", abs, round, pitch, want, newDelta)
		}
		if minRound == -1 || round < minRound {
			minRound = round
		}
	}
	// Anti-regression guard: the pre-fix bug re-anchored on every write, so
	// steps (and therefore round, and therefore pitch) would restart near 0
	// instead of continuing from where playback had already reached
	// (preEditRounds, since the edit landed right after that round fired).
	if minRound < preEditRounds {
		t.Fatalf("earliest post-edit captured round = %d, want >= %d (ramp must continue, not reset)", minRound, preEditRounds)
	}
}

// TestRuleEveryNEditMidPlaybackKeepsCycleCount is brief Part 2 test #2 (MUST
// FAIL against current code): a pitch rule active from the start accumulates
// for 4 rounds (+4); editing EveryN mid-play (1 -> 2, an IN-PLACE edit) must
// preserve the anchor so the cycle COUNT continues — the step formula keeps
// using the SAME anchor (0), just with the new EveryN divisor, rather than
// resetting to a fresh anchor near "now".
func TestRuleEveryNEditMidPlaybackKeepsCycleCount(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a, b := buildEdge(g)
	gid, err := g.graph.CreateGroup("", []model.NodeID{a.ID, b.ID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	const delta = 1.0
	if err := g.graph.SetGroupRules(gid, []model.GroupRule{
		{Param: model.GroupParamPitch, Delta: delta, EveryN: 1},
	}); err != nil {
		t.Fatalf("SetGroupRules: %v", err)
	}
	if err := g.graph.SetGroupRulesEnabled(gid, true); err != nil {
		t.Fatalf("SetGroupRulesEnabled: %v", err)
	}

	loopStart := g.loopStartByRow[0]
	loopLen := len(g.beatInfosByRow[0]) - loopStart

	g.playFn = func(id string, vol float64, when ...float64) {}
	g.SetPlaying(true)
	setPlayStartForAbs(g, 0)
	_ = g.Update()

	const preEditRounds = 4
	advancePlaybackByAbs(g, preEditRounds*loopLen+2)

	// In-place EveryN edit via the menu path — must PRESERVE the rule's
	// existing AnchorIdx (0), so the step count continues from the same
	// anchor with the new EveryN divisor.
	g.groupMenu.OpenAt(gid, 100, 100)
	g.groupMenu.stepRuleEveryN(1) // EveryN 1 -> 2
	advanceFrames(g, 1)

	grp, ok := g.graph.Group(gid)
	if !ok || len(grp.Rules) != 1 {
		t.Fatalf("setup: want 1 rule after edit, got %+v", grp.Rules)
	}
	newEveryN := grp.Rules[0].EveryN
	if newEveryN != 2 {
		t.Fatalf("setup: EveryN after one stepRuleEveryN = %d, want 2", newEveryN)
	}
	if anchor := grp.Rules[0].AnchorIdx; anchor != 0 {
		t.Fatalf("in-place EveryN edit must preserve AnchorIdx=0, got %d (cycle count reset instead of continuing)", anchor)
	}

	const postEditRounds = 3
	advancePlaybackByAbs(g, postEditRounds*loopLen)
	g.SetPlaying(false)

	// NOTE: onGroupsChanged (game_groups.go) wipes g.parityAudio whenever a
	// group rule changes while playing, so only captures recorded AFTER the
	// edit are visible here. Derive the expected pitch per-capture from
	// GroupEffect's own formula: steps = (round - anchorRound) / EveryN,
	// anchorRound == 0 throughout (asserted above, preserved not reset), so
	// steps = round / newEveryN using integer division exactly like
	// GroupEffect.
	after := collectParityPitches(g)
	if len(after) == 0 {
		t.Fatalf("no parity audio captured after the EveryN edit")
	}
	minRound := -1
	for abs, pitch := range after {
		round := model.GroupRound(abs, loopStart, loopLen)
		steps := round / newEveryN
		want := delta * float64(steps)
		if pitch != want {
			t.Fatalf("abs=%d round=%d pitch=%v, want %v (anchor preserved at 0, everyN=%d, steps=%d)", abs, round, pitch, want, newEveryN, steps)
		}
		if minRound == -1 || round < minRound {
			minRound = round
		}
	}
	// Anti-regression guard: the pre-fix bug re-anchored on every write, so
	// the cycle count (and therefore round) would restart near 0 instead of
	// continuing from where playback had already reached (preEditRounds,
	// since the edit landed right after that round fired).
	if minRound < preEditRounds {
		t.Fatalf("earliest post-edit captured round = %d, want >= %d (cycle count must continue, not reset)", minRound, preEditRounds)
	}
}

// TestRuleNewParamMidPlaybackAnchorsFresh is brief Part 2 test #3: pitch has
// a rule from the start; adding a brand-new DURATION rule mid-play (a param
// the group had no rule for) must stamp a FRESH anchor for duration only —
// the pre-existing pitch rule's anchor must stay untouched (per-param
// anchors diverge correctly), and pitch's own ramp must keep continuing
// exactly as if the duration rule had never been added.
func TestRuleNewParamMidPlaybackAnchorsFresh(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a, b := buildEdge(g)
	gid, err := g.graph.CreateGroup("", []model.NodeID{a.ID, b.ID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if err := g.graph.SetGroupRules(gid, []model.GroupRule{
		{Param: model.GroupParamPitch, Delta: 1, EveryN: 1},
	}); err != nil {
		t.Fatalf("SetGroupRules: %v", err)
	}

	if err := g.graph.SetGroupRulesEnabled(gid, true); err != nil {
		t.Fatalf("SetGroupRulesEnabled: %v", err)
	}
	loopStart := g.loopStartByRow[0]
	loopLen := len(g.beatInfosByRow[0]) - loopStart

	g.playFn = func(id string, vol float64, when ...float64) {}
	g.SetPlaying(true)
	setPlayStartForAbs(g, 0)
	_ = g.Update()
	advancePlaybackByAbs(g, 5*loopLen+2)

	// Add a brand-new DURATION rule mid-play via the menu (rddur+) — the
	// group didn't have a duration rule before. Pitch's existing rule must
	// keep its anchor (0); duration must get a FRESH (nonzero) anchor.
	g.groupMenu.OpenAt(gid, 100, 100)
	g.groupMenu.stepRuleDelta(model.GroupParamDuration, 1)
	advanceFrames(g, 1)

	grp, ok := g.graph.Group(gid)
	if !ok || len(grp.Rules) != 2 {
		t.Fatalf("setup: want 2 rules (pitch + duration), got %+v", grp.Rules)
	}
	pitchRule := findRule(grp.Rules, model.GroupParamPitch)
	durRule := findRule(grp.Rules, model.GroupParamDuration)
	if pitchRule == nil || pitchRule.AnchorIdx != 0 {
		t.Fatalf("existing pitch rule's anchor must be preserved at 0, got %+v", pitchRule)
	}
	if durRule == nil {
		t.Fatalf("duration rule must exist, got %+v", grp.Rules)
	}
	if durRule.AnchorIdx == 0 {
		t.Fatalf("new duration rule must get a FRESH nonzero anchor (playback is well underway), got AnchorIdx=0")
	}

	const postAddRounds = 2
	advancePlaybackByAbs(g, postAddRounds*loopLen)
	g.SetPlaying(false)

	// Pitch continues its pre-existing ramp (anchor 0, steps == round),
	// undisturbed by the sibling duration rule being added. NOTE:
	// onGroupsChanged wipes g.parityAudio whenever a group rule changes
	// while playing, so only captures recorded AFTER the add are visible
	// here — derive the expected pitch per-capture from GroupEffect's own
	// formula rather than assuming a fixed round index list (mirrors tests
	// #1/#2 above).
	after := collectParityPitches(g)
	if len(after) == 0 {
		t.Fatalf("no parity audio captured after adding the duration rule")
	}
	const preAddRounds = 5
	minRound := -1
	for abs, pitch := range after {
		round := model.GroupRound(abs, loopStart, loopLen)
		want := 1.0 * float64(round) // pitch rule: delta=1, everyN=1, anchor=0
		if pitch != want {
			t.Fatalf("abs=%d round=%d pitch=%v, want %v (pitch rule's own anchor must be undisturbed by the new duration rule)", abs, round, pitch, want)
		}
		if minRound == -1 || round < minRound {
			minRound = round
		}
	}
	if minRound < preAddRounds {
		t.Fatalf("earliest post-add captured round = %d, want >= %d (pitch ramp must continue, not reset)", minRound, preAddRounds)
	}
}

// ─── Part 3: replay after stop (task-rule-replay brief) ───────────────────
//
// Root-cause hypothesis: ruleAnchorIdxNow() stamps AnchorIdx at the live
// playhead when rules are written during playback (e.g. abs 640). On
// stop->play, the playhead restarts at abs 0; GroupEffect's
// anchor=min(anchor,absIdx) collapse makes steps=0 for the entire region
// below the stored anchor — the ramp is silent until the playhead re-passes
// wherever it was saved. The fix: every FRESH playback start (stop->play,
// not resume-from-pause) re-anchors ALL group rules to the session's
// starting absolute index (model.Graph.ReanchorAllGroupRules, called from
// handlePlaybackTransition's curr&&!prevPaused branch — the single chokepoint
// shared by the transport play button and the JS startPlay export, since both
// funnel through dv.playPressed -> Game.Update's PlayPressed() handling ->
// handlePlaybackTransition at the end of the same Update() call).
//
// These tests drive the REAL production play/stop seam (dv.TriggerPlayPause/
// dv.TriggerStop + g.Update(), mirroring the on-screen buttons per their doc
// comments) rather than the direct g.SetPlaying(true) calls the Part 1/2
// tests above use — a direct SetPlaying(true) call sets the state BEFORE
// Update() runs, so Update()'s prevPlaying (captured at its own top) already
// matches curr and handlePlaybackTransition never detects a transition. Only
// the flag-and-consume path (TriggerPlayPause/TriggerStop) reproduces the
// false->true edge within a single Update() call, exactly like a real button
// press or the JS startPlay export.

// TestSavedRulesApplyOnReplayAfterStop is brief test #1 (MUST FAIL against
// pre-fix code): play, enable a pitch +1-every-1 rule mid-play (anchoring it
// well after abs 0), confirm the ramp is running, stop (production StopPressed
// path), play again from abs 0, and assert the first few replayed rounds
// step 0, +1, +2, +3 — NOT a wall of 0s (the pre-fix bug: the stale non-zero
// anchor collapses every round below it to steps=0 until the playhead
// re-passes that point).
func TestSavedRulesApplyOnReplayAfterStop(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a, b := buildEdge(g)
	gid, err := g.graph.CreateGroup("", []model.NodeID{a.ID, b.ID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	loopStart := g.loopStartByRow[0]
	loopLen := len(g.beatInfosByRow[0]) - loopStart

	g.playFn = func(id string, vol float64, when ...float64) {}

	// Real play-start seam (transport button path).
	g.drum.TriggerPlayPause()
	_ = g.Update()
	if !g.Playing() {
		t.Fatalf("setup: not playing after TriggerPlayPause")
	}
	setPlayStartForAbs(g, 5)
	_ = g.Update()

	// Configure + enable the rule ~round 5 in via the menu path (Save/commit
	// flow — stepRuleDelta -> writeRules anchors the rule well after abs 0;
	// setRuleEnabled flips the independent RulesEnabled bit — task-enabled-
	// status brief Part C, deltas alone never apply).
	advancePlaybackByAbs(g, 5*loopLen)
	g.groupMenu.OpenAt(gid, 100, 100)
	g.groupMenu.stepRuleDelta(model.GroupParamPitch, 1)
	g.groupMenu.setRuleEnabled(true)
	advanceFrames(g, 1)

	grp, ok := g.graph.Group(gid)
	if !ok || len(grp.Rules) != 1 {
		t.Fatalf("setup: want 1 rule after enabling, got %+v", grp.Rules)
	}
	savedAnchor := grp.Rules[0].AnchorIdx
	if savedAnchor == 0 {
		t.Fatalf("setup: rule anchored at 0, want a nonzero mid-play anchor (playback is well underway) — the repro requires a stale nonzero anchor")
	}

	// Confirm the ramp is actually running before stopping (sanity, not the
	// bug under test).
	advancePlaybackByAbs(g, 3*loopLen)
	running := collectParityPitches(g)
	sawNonZero := false
	for _, p := range running {
		if p != 0 {
			sawNonZero = true
			break
		}
	}
	if !sawNonZero {
		t.Fatalf("rule does not appear to be ramping before stop: %v", running)
	}

	// Real stop path (production-closest — game_update.go's StopPressed()
	// block via dv.TriggerStop, not the lighter stopPlaybackForTest helper).
	g.drum.TriggerStop()
	_ = g.Update()
	if g.Playing() {
		t.Fatalf("setup: still playing after TriggerStop")
	}

	// Play again — the real production play-start seam, starting fresh from
	// abs 0 (a genuine stop->play transition: prevPaused must be false).
	g.drum.TriggerPlayPause()
	_ = g.Update()

	grp2, ok := g.graph.Group(gid)
	if !ok || len(grp2.Rules) != 1 {
		t.Fatalf("setup: rule must survive stop, got %+v", grp2.Rules)
	}
	replayAnchor := grp2.Rules[0].AnchorIdx
	// The pre-fix bug: nothing re-anchors on replay, so the anchor stays at
	// its stale pre-stop value (savedAnchor, well after abs 0). The fix
	// re-anchors to (near) abs 0 on every fresh play start.
	if replayAnchor >= savedAnchor {
		t.Fatalf("replay anchor=%d, want far less than the stale pre-stop anchor=%d (must re-anchor near abs 0 on a fresh play start, not carry the old anchor across stop->play)", replayAnchor, savedAnchor)
	}

	setPlayStartForAbs(g, 0)
	_ = g.Update()
	advancePlaybackByAbs(g, 4*loopLen)

	pitches := collectParityPitches(g)
	if len(pitches) == 0 {
		t.Fatalf("no parity audio captured on replay")
	}
	// Derive the expected pitch per capture from GroupEffect's own formula
	// using the REAL replay anchor (rather than assuming round 0 is the
	// first captured round — the scheduler may skip a couple of early
	// rounds depending on pulse pre-spawn timing, which is orthogonal to the
	// anchor bug under test here). The pre-fix bug means replayAnchor is
	// still the stale savedAnchor, so this formula would predict pitch=0 for
	// every captured round below that (round < anchorRound clamps steps to
	// 0) — exactly the RED failure mode ("[0 0 0]") — while the assertion
	// above already catches the anchor itself not moving.
	anchorRound := model.GroupRound(replayAnchor, loopStart, loopLen)
	minRound := -1
	for abs, pitch := range pitches {
		round := model.GroupRound(abs, loopStart, loopLen)
		steps := round - anchorRound
		if steps < 0 {
			steps = 0
		}
		want := float64(steps)
		if pitch != want {
			t.Fatalf("abs=%d round=%d pitch=%v, want %v (replayAnchor=%d anchorRound=%d, savedAnchor=%d) — ramp must step from the fresh replay anchor, not the stale pre-stop one", abs, round, pitch, want, replayAnchor, anchorRound, savedAnchor)
		}
		if minRound == -1 || round < minRound {
			minRound = round
		}
	}
	// Anti-regression: the earliest captured round's pitch must be small
	// (the ramp restarted near round 0), not still deep into the pre-stop
	// scale (would be ~8+ if the stale anchor persisted).
	if minRound > anchorRound+2 {
		t.Fatalf("earliest captured round=%d is suspiciously far from the fresh anchor round=%d; replay may not have actually restarted the ramp", minRound, anchorRound)
	}
}

// TestRuleSavedWhileStoppedStillRampsOnPlay is brief test #2 (control, passes
// now): a rule saved while STOPPED (anchor 0, the timeline origin) must keep
// ramping correctly on play — guards against the fix regressing the already-
// working case (anchor-0 math is untouched by ReanchorAllGroupRules, since
// re-anchoring to abs 0 on a fresh play start is a no-op for a rule already
// anchored at 0).
func TestRuleSavedWhileStoppedStillRampsOnPlay(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a, b := buildEdge(g)
	gid, err := g.graph.CreateGroup("", []model.NodeID{a.ID, b.ID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	// Configure + enable the rule via the menu path WHILE STOPPED (matches
	// the brief's "rules written while STOPPED (anchor 0) replay fine"
	// control case; setRuleEnabled flips the independent RulesEnabled bit —
	// task-enabled-status brief Part C, deltas alone never apply).
	g.groupMenu.OpenAt(gid, 100, 100)
	g.groupMenu.stepRuleDelta(model.GroupParamPitch, 1)
	g.groupMenu.setRuleEnabled(true)
	advanceFrames(g, 1)
	grp, ok := g.graph.Group(gid)
	if !ok || len(grp.Rules) != 1 || grp.Rules[0].AnchorIdx != 0 {
		t.Fatalf("setup: want 1 rule anchored at 0 (set while stopped), got %+v", grp.Rules)
	}

	loopStart := g.loopStartByRow[0]
	loopLen := len(g.beatInfosByRow[0]) - loopStart

	g.playFn = func(id string, vol float64, when ...float64) {}
	g.drum.TriggerPlayPause()
	_ = g.Update()
	setPlayStartForAbs(g, 0)
	_ = g.Update()
	advancePlaybackByAbs(g, 4*loopLen)

	pitches := collectParityPitches(g)
	if len(pitches) == 0 {
		t.Fatalf("no parity audio captured")
	}
	// Anchor is 0 (asserted above), so GroupEffect's formula reduces to
	// pitch == round exactly at every captured trigger — derived per-capture
	// rather than assuming round 0 is literally the first captured round
	// (the scheduler may skip a couple of early rounds depending on pulse
	// pre-spawn timing under the real TriggerPlayPause seam; orthogonal to
	// the anchor behavior this test guards).
	for abs, pitch := range pitches {
		round := model.GroupRound(abs, loopStart, loopLen)
		want := float64(round)
		if pitch != want {
			t.Fatalf("abs=%d round=%d pitch=%v, want %v (rule anchored at 0, steps==round)", abs, round, pitch, want)
		}
	}
}

// TestMidPlayEditContinuityStillHolds is brief test #3: re-anchoring only
// fires at a fresh play START (handlePlaybackTransition's curr&&!prevPaused
// branch), never mid-session, so the task-rule-continuity in-place-edit
// continuity guarantee (TestRuleDeltaEditMidPlaybackContinuesRamp and
// TestRuleEveryNEditMidPlaybackKeepsCycleCount, both defined above in this
// file) must remain green under the replay fix. This is a smoke re-assertion
// of that same invariant using the REAL production play-start seam (rather
// than direct SetPlaying) to prove the fix doesn't reset an in-place edit's
// anchor mid-session the way it correctly resets it across a stop->play.
func TestMidPlayEditContinuityStillHolds(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a, b := buildEdge(g)
	gid, err := g.graph.CreateGroup("", []model.NodeID{a.ID, b.ID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	const initialDelta = 1.0
	if err := g.graph.SetGroupRules(gid, []model.GroupRule{
		{Param: model.GroupParamPitch, Delta: initialDelta, EveryN: 1},
	}); err != nil {
		t.Fatalf("SetGroupRules: %v", err)
	}
	if err := g.graph.SetGroupRulesEnabled(gid, true); err != nil {
		t.Fatalf("SetGroupRulesEnabled: %v", err)
	}

	loopStart := g.loopStartByRow[0]
	loopLen := len(g.beatInfosByRow[0]) - loopStart

	g.playFn = func(id string, vol float64, when ...float64) {}
	g.drum.TriggerPlayPause()
	_ = g.Update()
	setPlayStartForAbs(g, 0)
	_ = g.Update()

	const preEditRounds = 5
	advancePlaybackByAbs(g, preEditRounds*loopLen+2)

	// In-place delta edit mid-SESSION (not a play-start transition) — must
	// preserve the anchor (0), continuing the ramp, exactly like
	// TestRuleDeltaEditMidPlaybackContinuesRamp above.
	g.groupMenu.OpenAt(gid, 100, 100)
	g.groupMenu.stepRuleDelta(model.GroupParamPitch, 1) // delta 1 -> 2
	advanceFrames(g, 1)

	grp, ok := g.graph.Group(gid)
	if !ok || len(grp.Rules) != 1 {
		t.Fatalf("setup: want 1 rule after edit, got %+v", grp.Rules)
	}
	newDelta := grp.Rules[0].Delta
	if anchor := grp.Rules[0].AnchorIdx; anchor != 0 {
		t.Fatalf("in-place mid-session edit must preserve AnchorIdx=0, got %d (replay fix must not re-anchor mid-session)", anchor)
	}

	const postEditRounds = 2
	advancePlaybackByAbs(g, postEditRounds*loopLen)

	after := collectParityPitches(g)
	if len(after) == 0 {
		t.Fatalf("no parity audio captured after the delta edit")
	}
	minRound := -1
	for abs, pitch := range after {
		round := model.GroupRound(abs, loopStart, loopLen)
		want := newDelta * float64(round)
		if pitch != want {
			t.Fatalf("abs=%d round=%d pitch=%v, want %v (anchor preserved at 0, new delta=%v, steps=round)", abs, round, pitch, want, newDelta)
		}
		if minRound == -1 || round < minRound {
			minRound = round
		}
	}
	if minRound < preEditRounds {
		t.Fatalf("earliest post-edit captured round = %d, want >= %d (ramp must continue, not reset)", minRound, preEditRounds)
	}
}

// TestSeekPlayStartsRampAtSeek is brief test #4: seeking to a position several
// rounds in BEFORE ever playing, then starting playback from that seek
// position (real production seam), must re-anchor the rule AT the seek
// position (not abs 0) — "the session's starting absolute index" is the seek
// position when playback starts from a seek, per the brief's design note.
func TestSeekPlayStartsRampAtSeek(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a, b := buildEdge(g)
	gid, err := g.graph.CreateGroup("", []model.NodeID{a.ID, b.ID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if err := g.graph.SetGroupRules(gid, []model.GroupRule{
		{Param: model.GroupParamPitch, Delta: 1, EveryN: 1},
	}); err != nil {
		t.Fatalf("SetGroupRules: %v", err)
	}
	if err := g.graph.SetGroupRulesEnabled(gid, true); err != nil {
		t.Fatalf("SetGroupRulesEnabled: %v", err)
	}

	loopStart := g.loopStartByRow[0]
	loopLen := len(g.beatInfosByRow[0]) - loopStart
	div := g.grid.MaxDiv()
	if div <= 0 {
		div = 1
	}

	// Seek several rounds in, BEFORE ever playing.
	const seekRounds = 4
	seekAbs := loopStart + seekRounds*loopLen
	seekBeats := seekAbs / div
	g.playFn = func(id string, vol float64, when ...float64) {}
	g.Seek(seekBeats)

	// Real production play-start seam, starting FROM the seek position.
	g.drum.TriggerPlayPause()
	_ = g.Update()

	grp, ok := g.graph.Group(gid)
	if !ok || len(grp.Rules) != 1 {
		t.Fatalf("setup: want 1 rule, got %+v", grp.Rules)
	}
	anchor := grp.Rules[0].AnchorIdx
	// The re-anchor must land at (or within 1 subdivision of, from the
	// beats<->subdivision rounding) the seek position, NOT 0.
	if d := anchor - seekAbs; d < -1 || d > 1 {
		t.Fatalf("anchor after seek+play = %d, want within 1 of seek position %d (must start the ramp AT the seek, not at 0)", anchor, seekAbs)
	}

	setPlayStartForAbs(g, seekAbs)
	_ = g.Update()
	advancePlaybackByAbs(g, 3*loopLen)

	pitches := collectParityPitches(g)
	if len(pitches) == 0 {
		t.Fatalf("no parity audio captured after seek+play")
	}
	anchorRound := model.GroupRound(anchor, loopStart, loopLen)
	minRound := -1
	for abs, pitch := range pitches {
		round := model.GroupRound(abs, loopStart, loopLen)
		steps := round - anchorRound
		if steps < 0 {
			steps = 0
		}
		want := float64(steps)
		if pitch != want {
			t.Fatalf("abs=%d round=%d pitch=%v, want %v (anchorRound=%d) — ramp must start at the seek position's round, not 0", abs, round, pitch, want, anchorRound)
		}
		if minRound == -1 || round < minRound {
			minRound = round
		}
	}
	if minRound < anchorRound {
		t.Fatalf("earliest captured round = %d, want >= anchor round %d", minRound, anchorRound)
	}
}
