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

// Picking an instrument never drops or silently changes settings: the row's carry over when the instrument has
// none (or the same), the instrument's apply when the row has none, and otherwise the user is asked.

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

func (c settingsChoice) withAsk() settingsChoice {
	differ := c.row.Digest() != c.inst.Digest()
	c.ask = !c.row.IsNeutral() && differ && (!c.inst.IsNeutral() || len(c.others) > 0)
	return c
}

// Without a question, the row's when it has any.
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

// Until the answer the row keeps playing what it plays; the pick and the answer land in one frame, one undo step.
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

// Per kit instrument (one channel, one set of settings): rows moving onto it bring theirs, the lowest row's
// when they differ. ONE question covers the whole swap.
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
