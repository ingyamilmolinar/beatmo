package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// Typed, so a typo ("ploy") is a compile error instead of a blank icon.
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
	IconMove         IconID = "move"
	IconCopy         IconID = "copy"
	IconConnect      IconID = "connect"
	IconSettings     IconID = "settings"
	IconTrash        IconID = "trash"
	IconCircle       IconID = "circle"
	IconTemplate     IconID = "template"
	IconSearch       IconID = "search"
	IconFolder       IconID = "folder"
	IconLogs         IconID = "logs"
	IconPalette      IconID = "palette"
	IconChord        IconID = "chord"
	IconCalendar     IconID = "calendar"
	IconGrip         IconID = "grip"

	IconRewind      IconID = "rewind"
	IconFastForward IconID = "fast-forward"

	IconHeadroom  IconID = "headroom"
	IconClipCount IconID = "clip-count"
	IconLoudest   IconID = "loudest"
)

// A switch, not a map built at init, so test overrides of the drawXxxIcon vars still take effect.
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
	case IconGrip:
		drawGripIcon(dst, r, col)
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
	case IconMove:
		drawMoveIcon(dst, r, col)
	case IconCopy:
		drawCopyIcon(dst, r, col)
	case IconConnect:
		drawConnectIcon(dst, r, col)
	case IconSettings:
		drawSettingsIcon(dst, r, col)
	case IconTrash:
		drawTrashIcon(dst, r, col)
	case IconCircle:
		drawCircleIcon(dst, r, col)
	case IconTemplate:
		drawTemplateIcon(dst, r, col)
	case IconSearch:
		drawSearchIcon(dst, r, col)
	case IconFolder:
		drawFolderIcon(dst, r, col)
	case IconLogs:
		drawLogsIcon(dst, r, col)
	case IconPalette:
		drawPaletteIcon(dst, r, col)
	case IconChord:
		drawChordIcon(dst, r, col)
	case IconCalendar:
		drawCalendarIcon(dst, r, col)
	case IconHeadroom:
		drawHeadroomIcon(dst, r, col)
	case IconClipCount:
		drawClipCountIcon(dst, r, col)
	case IconLoudest:
		drawLoudestIcon(dst, r, col)
	case IconRewind:
		drawRewindIcon(dst, r, col)
	case IconFastForward:
		drawFastForwardIcon(dst, r, col)
	default:
		return false
	}
	return true
}

// No extra inset: callers pass the glyph rect.
func DrawIcon(dst *ebiten.Image, id IconID, r image.Rectangle, col color.Color) bool {
	return drawIconByID(dst, id, r, col)
}
