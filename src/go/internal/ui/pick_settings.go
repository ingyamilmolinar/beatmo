package ui

import (
	"image"
	"maps"
	"slices"
	"strconv"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// pick_settings.go — choosing an instrument never drops or silently changes
// settings (spec 2026-10-31 §2.9; residency spec §5.2's pick table). A row's
// settings are its instrument's audio bundle; the picked instrument brings its
// own (its circuit bundle when another row plays it, else its library one):
//
//	row has none                        → the instrument's
//	instrument has none, or the same    → the row's carry over
//	both set and different, or keeping  → the user is asked; a cancel
//	the row's would change another row    changes nothing

// settingsChoice is what choosing an instrument for a row does to settings.
type settingsChoice struct {
	row, inst audio.AudioBundle
	others    []int // other rows that play the instrument (their sound changes with it)
	ask       bool
}

func (dv *DrumView) settingsChoiceFor(row int, id instrument.ID) settingsChoice {
	c := settingsChoice{row: audio.CurrentAudioBundle(dv.Rows[row].Instrument)}
	for i, r := range dv.Rows {
		if i != row && r != nil && r.Instrument == id {
			c.others = append(c.others, i)
		}
	}
	if len(c.others) > 0 {
		c.inst = audio.CurrentAudioBundle(id)
	} else {
		c.inst, _ = audio.LibraryBundle(id)
	}
	return c.withAsk()
}

// withAsk sets ask: the row has settings that differ from the instrument's,
// and the instrument has its own, or another row plays it.
func (c settingsChoice) withAsk() settingsChoice {
	differ := c.row.Digest() != c.inst.Digest()
	c.ask = !c.row.IsNeutral() && differ && (!c.inst.IsNeutral() || len(c.others) > 0)
	return c
}

// settingsFor is the bundle id plays with after the choice: the row's
// (keep) or the instrument's. Without a question, the row's when it has any.
func (c settingsChoice) settingsFor(keep bool) audio.AudioBundle {
	if keep && !c.row.IsNeutral() {
		return c.row
	}
	return c.inst
}

// keepLabel names the "keep" answer, saying which other rows it changes.
func (c settingsChoice) keepLabel() string {
	if len(c.others) == 0 {
		return i18n.T(i18n.KeyKeepRowSettings)
	}
	return i18n.Tf(i18n.KeyAlsoChangesRowFmt, strconv.Itoa(c.others[0]+1))
}

// pickInstrument switches row to id, asking first when the row's settings
// and the instrument's would conflict (until the answer, the row keeps
// playing what it plays). The pick and the answer are one undo step: both
// land in the same frame.
func (dv *DrumView) pickInstrument(row int, id instrument.ID) {
	if row < 0 || row >= len(dv.Rows) || id == "" || dv.Rows[row].Instrument == id {
		return
	}
	c := dv.settingsChoiceFor(row, id)
	commit := func(keep bool) {
		dv.selRow = row
		dv.SetInstrument(id)
		audio.ApplyAudioBundle(id, c.settingsFor(keep))
	}
	if !c.ask {
		commit(true)
		return
	}
	dv.openQuestion("pick-settings", dv.rowQuestionAnchor(row), []actionMenuItem{
		{Label: c.keepLabel(), OnSelect: func() { commit(true) }},
		{Label: i18n.Tf(i18n.KeyUseInstrumentSettingsFmt, audio.InstrumentDisplayName(id)), OnSelect: func() { commit(false) }},
	}, nil)
}

// applyKitChoosing applies k through the same choice, per kit instrument (one
// channel, one set of settings): the rows moving onto it bring their settings
// — the lowest such row's when they differ, and then the question names whose
// it keeps and which row it changes. When any instrument would need the
// question, ONE question covers the whole swap.
func (dv *DrumView) applyKitChoosing(k audio.Kit) int {
	plan := audio.KitPlan(k, kitRowBinder{dv: dv})
	byTarget := map[instrument.ID][]int{}
	for _, row := range slices.Sorted(maps.Keys(plan)) {
		byTarget[plan[row]] = append(byTarget[plan[row]], row)
	}
	type decision struct{ keep, inst audio.AudioBundle }
	decisions := map[instrument.ID]decision{}
	ask, keepLabel := false, i18n.T(i18n.KeyKeepEachRowSettings)
	for id, rows := range byTarget {
		c := dv.settingsChoiceFor(rows[0], id)
		keep, winner := c.row, rows[0]
		for _, r := range rows[1:] {
			b := audio.CurrentAudioBundle(dv.Rows[r].Instrument)
			switch {
			case keep.IsNeutral():
				keep, winner = b, r
			case !b.IsNeutral() && b.Digest() != keep.Digest():
				ask = true
				keepLabel = i18n.Tf(i18n.KeyKeepRowNSettingsFmt, strconv.Itoa(winner+1), strconv.Itoa(r+1))
			}
		}
		c.row = keep
		c = c.withAsk()
		ask = ask || c.ask
		decisions[id] = decision{keep: c.row, inst: c.inst}
	}
	apply := func(keep bool) int {
		n := audio.ApplyKit(k, kitRowBinder{dv: dv})
		for id, d := range decisions {
			audio.ApplyAudioBundle(id, settingsChoice{row: d.keep, inst: d.inst}.settingsFor(keep))
		}
		return n
	}
	if !ask {
		return apply(true)
	}
	dv.openQuestion("kit-settings", dv.rowQuestionAnchor(0), []actionMenuItem{
		{Label: keepLabel, OnSelect: func() { apply(true) }},
		{Label: i18n.T(i18n.KeyUseKitSettings), OnSelect: func() { apply(false) }},
	}, nil)
	return 0
}

// rowQuestionAnchor anchors a question at the row's label.
func (dv *DrumView) rowQuestionAnchor(row int) image.Rectangle {
	if labels := dv.rowLabels(); row >= 0 && row < len(labels) && labels[row] != nil {
		return labels[row].Rect()
	}
	return image.Rectangle{}
}
