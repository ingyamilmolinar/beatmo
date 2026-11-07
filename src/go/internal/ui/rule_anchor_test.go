package ui

import (
	"sort"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// The pitch the REAL scheduler computed at each trigger, keyed by abs.
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

// The 2-node loop fires twice per round, so raw indices are collapsed to one pitch per round (GroupRound's math):
// callers assert "+Delta every round" without hardcoding the node count.
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

// A watermark, not a count: parityAudio is a bounded ring, so its size can stay constant while the captured set
// moves forward.
func maxIdx(pitches map[int]float64) int {
	m := -1
	for i := range pitches {
		if i > m {
			m = i
		}
	}
	return m
}

// A rule set before play (anchor 0) gives pitch == round at each round's triggers.
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
	// RulesEnabled is independent of Rules and defaults false.
	if err := g.graph.SetGroupRulesEnabled(gid, true); err != nil {
		t.Fatalf("SetGroupRulesEnabled: %v", err)
	}

	loopStart := g.loopStartByRow[0]
	loopLen := len(g.beatInfosByRow[0]) - loopStart

	g.playFn = func(id instrument.ID, vol float64, when ...float64) {}
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

// A rule enabled mid-play through the menu anchors at that moment: the first round after is 0, then +1, +2, never
// the absolute round number.
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

	g.playFn = func(id instrument.ID, vol float64, when ...float64) {}
	g.SetPlaying(true)
	setPlayStartForAbs(g, 0)
	_ = g.Update()
	// Play ~10 rounds with NO rule.
	advancePlaybackByAbs(g, 10*loopLen)
	// The split between before and after enabling.
	beforeMax := maxIdx(collectParityPitches(g))

	// stepRuleDelta writes the rule (stamping the anchor) and setRuleEnabled flips the independent bit; one rdpit+
	// step is "pitch +1 every 1 round".
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
	// The first round after enabling is small and each later one steps by exactly Delta: accumulation from the anchor,
	// not from the timeline origin.
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

// Off/On is a pure gate that never touches Rules or AnchorIdx: disabled reads flat 0, and re-enabling resumes the
// original anchor's ramp at the CURRENT round, a jump over the disabled rounds rather than a reset.
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

	g.playFn = func(id instrument.ID, vol float64, when ...float64) {}
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
	// The anchor never left 0, so steps == round at every capture after re-enabling.
	for _, i := range post {
		round := model.GroupRound(i, loopStart, loopLen)
		want := float64(round)
		if got := pitches[i]; got != want {
			t.Fatalf("idx=%d round=%d pitch=%v, want %v (re-enable must resume the original anchor's formula, not reset)", i, round, got, want)
		}
	}
}

// Configured deltas with RulesEnabled false are inert (BuildGroupIndex skips them); enabling mid-session resumes
// the ramp at once from the existing anchor.
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
	// Configuring deltas alone never makes them audible.
	if grp, ok := g.graph.Group(gid); !ok || grp.RulesEnabled {
		t.Fatalf("setup: group must start Disabled, got %+v ok=%v", grp, ok)
	}

	loopStart := g.loopStartByRow[0]
	loopLen := len(g.beatInfosByRow[0]) - loopStart

	g.playFn = func(id instrument.ID, vol float64, when ...float64) {}
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

// Byte-faithful through export → import, and the imported AnchorIdx matches exactly.
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

// An in-place edit (the param already has a rule) keeps the existing AnchorIdx so the ramp continues; only a param
// gaining its first rule, or Off→On, gets a fresh anchor.

// Rounds up to the edit keep the old delta, later ones use the new one: the ramp continues, it never resets.
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

	g.playFn = func(id instrument.ID, vol float64, when ...float64) {}
	g.SetPlaying(true)
	setPlayStartForAbs(g, 0)
	_ = g.Update()

	const preEditRounds = 5
	advancePlaybackByAbs(g, preEditRounds*loopLen+2)

	// The group already has a pitch rule, so the anchor must be kept.
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

	// onGroupsChanged wipes parityAudio when a rule changes while playing, so only captures after the edit exist;
	// each one's expected pitch comes from GroupEffect's own formula.
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
	// A re-anchor would restart the rounds near 0 instead of continuing from where playback was.
	if minRound < preEditRounds {
		t.Fatalf("earliest post-edit captured round = %d, want >= %d (ramp must continue, not reset)", minRound, preEditRounds)
	}
}

// The cycle COUNT continues: the same anchor (0) with the new EveryN divisor.
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

	g.playFn = func(id instrument.ID, vol float64, when ...float64) {}
	g.SetPlaying(true)
	setPlayStartForAbs(g, 0)
	_ = g.Update()

	const preEditRounds = 4
	advancePlaybackByAbs(g, preEditRounds*loopLen+2)

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

	// Only captures after the edit exist (onGroupsChanged wipes parityAudio); steps = round / newEveryN, integer
	// division as in GroupEffect.
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
	// A re-anchor would restart the rounds near 0 instead of continuing from where playback was.
	if minRound < preEditRounds {
		t.Fatalf("earliest post-edit captured round = %d, want >= %d (cycle count must continue, not reset)", minRound, preEditRounds)
	}
}

// Anchors are per param: the new duration rule gets a fresh one, and pitch keeps its anchor and its ramp.
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

	g.playFn = func(id instrument.ID, vol float64, when ...float64) {}
	g.SetPlaying(true)
	setPlayStartForAbs(g, 0)
	_ = g.Update()
	advancePlaybackByAbs(g, 5*loopLen+2)

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

	// Only captures after the add exist (onGroupsChanged wipes parityAudio); expected pitch from GroupEffect.
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

// An anchor stamped mid-play sits far past abs 0, and on stop → play every round below it would read steps=0.
// Every FRESH start (not a resume) re-anchors all rules to the starting index in handlePlaybackTransition.
// These drive the real TriggerPlayPause / TriggerStop seam: SetPlaying(true) before Update never shows a transition.

// A rule anchored well after abs 0, then stop and play: the replayed rounds step 0, +1, +2, not a wall of 0s.
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

	g.playFn = func(id instrument.ID, vol float64, when ...float64) {}

	// Real play-start seam (transport button path).
	g.drum.TriggerPlayPause()
	_ = g.Update()
	if !g.Playing() {
		t.Fatalf("setup: not playing after TriggerPlayPause")
	}
	setPlayStartForAbs(g, 5)
	_ = g.Update()

	// Enabled about five rounds in, so its anchor lands well after abs 0.
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

	// The production StopPressed path, not stopPlaybackForTest.
	g.drum.TriggerStop()
	_ = g.Update()
	if g.Playing() {
		t.Fatalf("setup: still playing after TriggerStop")
	}

	// A genuine stop → play transition from abs 0.
	g.drum.TriggerPlayPause()
	_ = g.Update()

	grp2, ok := g.graph.Group(gid)
	if !ok || len(grp2.Rules) != 1 {
		t.Fatalf("setup: rule must survive stop, got %+v", grp2.Rules)
	}
	replayAnchor := grp2.Rules[0].AnchorIdx
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
	// Expected pitch from GroupEffect with the REAL replay anchor: the scheduler may skip early rounds depending on
	// pulse pre-spawn timing, which has nothing to do with the anchor.
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
	// The ramp restarted near round 0, not deep into the pre-stop scale.
	if minRound > anchorRound+2 {
		t.Fatalf("earliest captured round=%d is suspiciously far from the fresh anchor round=%d; replay may not have actually restarted the ramp", minRound, anchorRound)
	}
}

// Re-anchoring to abs 0 is a no-op for a rule already anchored at 0.
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

	g.playFn = func(id instrument.ID, vol float64, when ...float64) {}
	g.drum.TriggerPlayPause()
	_ = g.Update()
	setPlayStartForAbs(g, 0)
	_ = g.Update()
	advancePlaybackByAbs(g, 4*loopLen)

	pitches := collectParityPitches(g)
	if len(pitches) == 0 {
		t.Fatalf("no parity audio captured")
	}
	// pitch == round at every capture; early rounds may be skipped by pulse pre-spawn timing.
	for abs, pitch := range pitches {
		round := model.GroupRound(abs, loopStart, loopLen)
		want := float64(round)
		if pitch != want {
			t.Fatalf("abs=%d round=%d pitch=%v, want %v (rule anchored at 0, steps==round)", abs, round, pitch, want)
		}
	}
}

// Re-anchoring happens only at a fresh play START, never mid-session, so an in-place edit's continuity survives
// the replay rule.
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

	g.playFn = func(id instrument.ID, vol float64, when ...float64) {}
	g.drum.TriggerPlayPause()
	_ = g.Update()
	setPlayStartForAbs(g, 0)
	_ = g.Update()

	const preEditRounds = 5
	advancePlaybackByAbs(g, preEditRounds*loopLen+2)

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

// Playback started from a seek starts the session at the seek position, so the rule anchors there, not at 0.
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
	g.playFn = func(id instrument.ID, vol float64, when ...float64) {}
	g.Seek(seekBeats)

	// Real production play-start seam, starting FROM the seek position.
	g.drum.TriggerPlayPause()
	_ = g.Update()

	grp, ok := g.graph.Group(gid)
	if !ok || len(grp.Rules) != 1 {
		t.Fatalf("setup: want 1 rule, got %+v", grp.Rules)
	}
	anchor := grp.Rules[0].AnchorIdx
	// Within one subdivision, from the beats↔subdivision rounding.
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
