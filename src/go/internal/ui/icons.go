package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// IconID names a glyph in the unified icon set. Using the typed constant
// avoids typos at call sites ("ploy" silently rendered nothing).
type IconID string

const (
	IconPlay         IconID = "play"
	IconPause        IconID = "pause"
	IconStop         IconID = "stop"
	IconRecord       IconID = "record"
	IconPencil       IconID = "pencil"
	IconSave         IconID = "save"
	IconClose        IconID = "close"
	IconOverflow     IconID = "overflow"
	IconPlus         IconID = "plus"
	IconMinus        IconID = "minus"
	IconRows         IconID = "rows"
	IconAudio        IconID = "audio"
	IconChevronUp    IconID = "chevron-up"
	IconChevronDown  IconID = "chevron-down"
	IconChevronRight IconID = "chevron-right"
	IconChevronLeft  IconID = "chevron-left"
	IconStar         IconID = "star"
	IconStarFilled   IconID = "star-filled"
	IconTrack        IconID = "track"
	IconTrackOff     IconID = "track-off"
	IconUpload       IconID = "upload"
	IconImport       IconID = "import"
	IconExport       IconID = "export"
	IconUndo         IconID = "undo"
	IconRedo         IconID = "redo"
	IconSpeaker      IconID = "speaker"
	IconSpeakerOff   IconID = "speaker-off"
	IconNote         IconID = "note"
	IconMute         IconID = "mute"
	IconSolo         IconID = "solo"
	IconFx           IconID = "fx"
	IconTarget       IconID = "target"
	IconSettings     IconID = "settings"
	IconTrash        IconID = "trash"
	IconCircle       IconID = "circle"

	IconTriggerMarker IconID = "trigger-marker"
	IconHeadroom      IconID = "headroom"
	IconClipCount     IconID = "clip-count"
	IconLoudest       IconID = "loudest"
)

// drawIconByID dispatches to the glyph's draw function. Returns true if the
// icon is known. Kept as a switch (rather than a map[IconID]func) so that
// test overrides of individual drawXxxIcon vars still take effect — a map
// populated at init-time would snapshot the original function values.
func drawIconByID(dst *ebiten.Image, id IconID, r image.Rectangle, col color.Color) bool {
	switch id {
	case IconPlay:
		drawPlayIcon(dst, r, col)
	case IconPause:
		drawPauseIcon(dst, r, col)
	case IconStop:
		drawStopIcon(dst, r, col)
	case IconRecord:
		drawRecordIcon(dst, r, col)
	case IconPencil:
		drawPencilIcon(dst, r, col)
	case IconSave:
		drawSaveIcon(dst, r, col)
	case IconClose:
		drawCloseIcon(dst, r, col)
	case IconOverflow:
		drawOverflowIcon(dst, r, col)
	case IconPlus:
		drawPlusIcon(dst, r, col)
	case IconMinus:
		drawMinusIcon(dst, r, col)
	case IconRows:
		drawRowsIcon(dst, r, col)
	case IconAudio:
		drawAudioIcon(dst, r, col)
	case IconChevronUp:
		drawChevronUpIcon(dst, r, col)
	case IconChevronDown:
		drawChevronDownIcon(dst, r, col)
	case IconChevronRight:
		drawChevronRightIcon(dst, r, col)
	case IconChevronLeft:
		drawChevronLeftIcon(dst, r, col)
	case IconStar:
		drawStarIcon(dst, r, col)
	case IconStarFilled:
		drawStarFilledIcon(dst, r, col)
	case IconTrack:
		drawTrackIcon(dst, r, col)
	case IconTrackOff:
		drawTrackOffIcon(dst, r, col)
	case IconUpload:
		drawUploadIcon(dst, r, col)
	case IconImport:
		drawImportIcon(dst, r, col)
	case IconExport:
		drawExportIcon(dst, r, col)
	case IconUndo:
		drawUndoIcon(dst, r, col)
	case IconRedo:
		drawRedoIcon(dst, r, col)
	case IconSpeaker:
		drawSpeakerIcon(dst, r, col)
	case IconSpeakerOff:
		drawSpeakerOffIcon(dst, r, col)
	case IconNote:
		drawNoteIcon(dst, r, col)
	case IconMute:
		drawMuteIcon(dst, r, col)
	case IconSolo:
		drawSoloIcon(dst, r, col)
	case IconFx:
		drawFxIcon(dst, r, col)
	case IconTarget:
		drawTargetIcon(dst, r, col)
	case IconSettings:
		drawSettingsIcon(dst, r, col)
	case IconTrash:
		drawTrashIcon(dst, r, col)
	case IconCircle:
		drawCircleIcon(dst, r, col)
	case IconTriggerMarker:
		drawTriggerMarkerIcon(dst, r, col)
	case IconHeadroom:
		drawHeadroomIcon(dst, r, col)
	case IconClipCount:
		drawClipCountIcon(dst, r, col)
	case IconLoudest:
		drawLoudestIcon(dst, r, col)
	default:
		return false
	}
	return true
}

// DrawIcon renders the named glyph centered in r using col. Callers that
// already have a glyph rectangle (e.g., buttons inset by padding) pass it
// directly; there is no extra inset here.
func DrawIcon(dst *ebiten.Image, id IconID, r image.Rectangle, col color.Color) bool {
	return drawIconByID(dst, id, r, col)
}
