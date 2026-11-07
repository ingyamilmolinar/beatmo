package ui

import (
	"fmt"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Families are built by constructors, so no element is named by concatenating or parsing a string.
type gmID struct {
	el  gmEl
	arg int // the element's operand: the stepper row, the merge group id or the chip row
}

type gmEl int

const (
	gmElNone gmEl = iota
	gmElPanel
	gmElGrip
	gmElTitle
	gmElClose
	gmElSave
	gmElRuleLabel
	gmElRuleOnOff
	gmElMergeLabel
	gmElDelete
	gmElRow       // arg = gmStep: the stepper row's label rect
	gmElMinus     // arg = gmStep
	gmElValue     // arg = gmStep
	gmElPlus      // arg = gmStep
	gmElMergeChip // arg = group id
	gmElMergeRow  // arg = chip row index
)

var (
	gmPanel      = gmID{el: gmElPanel}
	gmGrip       = gmID{el: gmElGrip}
	gmTitle      = gmID{el: gmElTitle}
	gmClose      = gmID{el: gmElClose}
	gmSave       = gmID{el: gmElSave}
	gmRuleLabel  = gmID{el: gmElRuleLabel}
	gmRuleOnOff  = gmID{el: gmElRuleOnOff}
	gmMergeLabel = gmID{el: gmElMergeLabel}
	gmDelete     = gmID{el: gmElDelete}
)

type gmStep int

const (
	gmStepEveryN gmStep = iota
	gmStepPitch
	gmStepVolume
	gmStepDuration
)

var gmStepNames = [...]string{"n", "rdpit", "rdvol", "rddur"}

// gmStepFor is the delta row of a rule param.
func gmStepFor(p model.GroupParam) gmStep {
	switch p {
	case model.GroupParamPitch:
		return gmStepPitch
	case model.GroupParamVolume:
		return gmStepVolume
	}
	return gmStepDuration
}

func gmRow(s gmStep) gmID                { return gmID{el: gmElRow, arg: int(s)} }
func gmMinus(s gmStep) gmID              { return gmID{el: gmElMinus, arg: int(s)} }
func gmValue(s gmStep) gmID              { return gmID{el: gmElValue, arg: int(s)} }
func gmPlus(s gmStep) gmID               { return gmID{el: gmElPlus, arg: int(s)} }
func gmMergeChip(gid model.GroupID) gmID { return gmID{el: gmElMergeChip, arg: int(gid)} }
func gmMergeRow(i int) gmID              { return gmID{el: gmElMergeRow, arg: i} }

var gmFixedNames = map[gmEl]string{
	gmElPanel: "panel", gmElGrip: "grip", gmElTitle: "title", gmElClose: "close", gmElSave: "save",
	gmElRuleLabel: "rulelabel", gmElRuleOnOff: "ruleonoff", gmElMergeLabel: "mergelabel", gmElDelete: "delete",
}

// String spells the element for logs and test messages.
func (id gmID) String() string {
	if s, ok := gmFixedNames[id.el]; ok {
		return s
	}
	switch id.el {
	case gmElRow:
		return gmStepNames[id.arg]
	case gmElMinus:
		return gmStepNames[id.arg] + "-"
	case gmElValue:
		return gmStepNames[id.arg] + "val"
	case gmElPlus:
		return gmStepNames[id.arg] + "+"
	case gmElMergeChip:
		return fmt.Sprintf("mergechip:%d", id.arg)
	case gmElMergeRow:
		return fmt.Sprintf("mergerow%d", id.arg)
	}
	return ""
}

// For tests probing that a retired element is gone.
func (m *GroupMenu) laidOutID(name string) (gmID, bool) {
	for id := range m.rects {
		if id.String() == name {
			return id, true
		}
	}
	return gmID{}, false
}
