package ui

import (
	"fmt"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// sbID names one element of the node sidebar: the key of its rects, buttons
// and press animations. Fixed elements are the sb* values below; the element
// families (a section's header, a field's stepper, a dropdown option, a group
// row, a picker cell) are built by the constructors, so no element is ever
// named by concatenating a string or recovered by parsing one.
type sbID struct {
	el  sbEl
	arg int // the element's operand: section, field, logic/groove kind, group id or packed cell
}

// sbEl is the kind of sidebar element.
type sbEl int

const (
	elNone sbEl = iota
	elPanel
	elHeader
	elCoords
	elClose
	elGrip
	elAudible        // the Audible section's type toggle
	elLogicSelect    // opens the logic rule list
	elGrooveSelect   // opens the groove list
	elPitchNote      // the Pitch row's note chip (opens the note picker)
	elChordName      // the Chord section's chip (opens the chord picker)
	elChordSingle    // collapse a chord node to a single note
	elGroupNew       // create a group from this node
	elGroupAdd       // opens the add-to-group list
	elGroupRemove    // opens the remove-from-group list
	elSection        // arg = sbSection: the section's header
	elMinus          // arg = sbField: the field's − stepper
	elValue          // arg = sbField: the field's value pill (opens the wheel)
	elPlus           // arg = sbField: the field's + stepper
	elLogicOption    // arg = model.LogicKind
	elGrooveOption   // arg = model.GrooveKind
	elGroupChip      // arg = group id: opens that group's menu
	elGroupAddOpt    // arg = group id
	elGroupRemoveOpt // arg = group id
	elNoteCell       // arg = packed col/row of a note-picker cell
	elChordCell      // arg = packed col/row of a chord-picker cell
)

// The fixed elements.
var (
	sbPanel        = sbID{el: elPanel}
	sbHeader       = sbID{el: elHeader}
	sbCoords       = sbID{el: elCoords}
	sbClose        = sbID{el: elClose}
	sbGrip         = sbID{el: elGrip}
	sbAudible      = sbID{el: elAudible}
	sbLogicSelect  = sbID{el: elLogicSelect}
	sbGrooveSelect = sbID{el: elGrooveSelect}
	sbPitchNote    = sbID{el: elPitchNote}
	sbChordName    = sbID{el: elChordName}
	sbChordSingle  = sbID{el: elChordSingle}
	sbGroupNew     = sbID{el: elGroupNew}
	sbGroupAdd     = sbID{el: elGroupAdd}
	sbGroupRemove  = sbID{el: elGroupRemove}
)

// sbSection is one collapsible section of the node sidebar.
type sbSection int

const (
	secVol sbSection = iota
	secPit
	secChord
	secDur
	secLogic
	secGroove
	secAud
	secGrp
)

var sbSectionNames = [...]string{"vol", "pit", "chord", "dur", "logic", "groove", "aud", "grp"}

func (s sbSection) String() string { return sbSectionNames[s] }

// sbField is a numeric field with a [−] [value] [+] stepper row.
type sbField int

const (
	fieldVol sbField = iota
	fieldPit
	fieldDur
	fieldLogicN
	fieldLogicP
	fieldGroovePct
	fieldChordInv
	fieldChordSpread
)

var sbFieldNames = [...]string{"vol", "pit", "dur", "ln", "lp", "gp", "chinv", "chspr"}

func (f sbField) String() string { return sbFieldNames[f] }

// sectionField is the stepped field a plain section (vol / pit / dur) edits.
var sectionField = map[sbSection]sbField{secVol: fieldVol, secPit: fieldPit, secDur: fieldDur}

func sbSectionHeader(s sbSection) sbID           { return sbID{el: elSection, arg: int(s)} }
func sbMinus(f sbField) sbID                     { return sbID{el: elMinus, arg: int(f)} }
func sbValue(f sbField) sbID                     { return sbID{el: elValue, arg: int(f)} }
func sbPlus(f sbField) sbID                      { return sbID{el: elPlus, arg: int(f)} }
func sbLogicOption(k model.LogicKind) sbID       { return sbID{el: elLogicOption, arg: int(k)} }
func sbGrooveOption(k model.GrooveKind) sbID     { return sbID{el: elGrooveOption, arg: int(k)} }
func sbGroupChip(gid model.GroupID) sbID         { return sbID{el: elGroupChip, arg: int(gid)} }
func sbGroupAddOption(gid model.GroupID) sbID    { return sbID{el: elGroupAddOpt, arg: int(gid)} }
func sbGroupRemoveOption(gid model.GroupID) sbID { return sbID{el: elGroupRemoveOpt, arg: int(gid)} }
func sbNoteCell(col, row int) sbID               { return sbID{el: elNoteCell, arg: packCell(col, row)} }
func sbChordCell(col, row int) sbID              { return sbID{el: elChordCell, arg: packCell(col, row)} }

func packCell(col, row int) int { return col<<16 | row&0xffff }

func (id sbID) cell() (col, row int) { return id.arg >> 16, id.arg & 0xffff }

// isPickerCell reports whether id is a cell of the note or chord picker.
func (id sbID) isPickerCell() bool { return id.el == elNoteCell || id.el == elChordCell }

var sbFixedNames = map[sbEl]string{
	elPanel: "panel", elHeader: "header", elCoords: "coords", elClose: "close", elGrip: "grip",
	elAudible: "aud", elLogicSelect: "logic", elGrooveSelect: "grv", elPitchNote: "pitnote",
	elChordName: "chname", elChordSingle: "chsingle", elGroupNew: "grpnew", elGroupAdd: "grpadd",
	elGroupRemove: "grpremove",
}

// String spells the element for logs, hover ids and test messages.
func (id sbID) String() string {
	if s, ok := sbFixedNames[id.el]; ok {
		return s
	}
	switch id.el {
	case elSection:
		return "sec-" + sbSection(id.arg).String()
	case elMinus:
		return sbField(id.arg).String() + "-"
	case elValue:
		return sbField(id.arg).String() + "val"
	case elPlus:
		return sbField(id.arg).String() + "+"
	case elLogicOption:
		return "logic:" + model.LogicKind(id.arg).String()
	case elGrooveOption:
		return "groove:" + model.GrooveKind(id.arg).String()
	case elGroupChip:
		return fmt.Sprintf("grpchip:%d", id.arg)
	case elGroupAddOpt:
		return fmt.Sprintf("grpaddopt:%d", id.arg)
	case elGroupRemoveOpt:
		return fmt.Sprintf("grpremoveopt:%d", id.arg)
	case elNoteCell, elChordCell:
		col, row := id.cell()
		prefix := "note:"
		if id.el == elChordCell {
			prefix = chordMatrixCellPrefix
		}
		return fmt.Sprintf("%s%d:%d", prefix, col, row)
	}
	return ""
}

// parseSBSection reads a section name ("logic") at the JS bridge.
func parseSBSection(name string) (sbSection, bool) {
	for s, n := range sbSectionNames {
		if n == name {
			return sbSection(s), true
		}
	}
	return 0, false
}

// laidOutID finds the laid-out element spelled name (sbID.String), for the JS
// bridge's rect probes — the one place an element arrives as text.
func (sb *NodeSidebar) laidOutID(name string) (sbID, bool) {
	for id := range sb.rects {
		if id.String() == name {
			return id, true
		}
	}
	return sbID{}, false
}
