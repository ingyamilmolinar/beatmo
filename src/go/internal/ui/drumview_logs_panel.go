package ui

import "image"

// logsPanelID identifies the Logs Navigation Panel portal entry.
const logsPanelID = "logs"

// Registered as a native input so phones get a real keyboard for the search.
const logsSearchMobileInputID = "logs-search"

// IsLogsPanelOpen reports whether the Logs Navigation Panel portal is open.
func (dv *DrumView) IsLogsPanelOpen() bool {
	return dv.tree != nil && dv.portal().Has(logsPanelID)
}

// The unread badge needs it before the panel first opens.
func (dv *DrumView) ensureLogsComp() *LogsComponent {
	if dv.logsComp == nil {
		dv.logsComp = NewLogsComponent(dv)
	}
	return dv.logsComp
}

func (dv *DrumView) logsUnread() (int, bool) {
	return dv.ensureLogsComp().unreadCount()
}

func (dv *DrumView) pushErrorToast(text string) { dv.logsToast.push(text) }

// logsToastActive reports whether the error toast is currently visible.
func (dv *DrumView) logsToastActive() bool { return dv.logsToast.active() }

func (dv *DrumView) openLogsPanel() {
	if dv.tree == nil {
		return
	}
	p := dv.portal()
	if p == nil || p.Has(logsPanelID) {
		return
	}
	c := dv.ensureLogsComp()
	c.Open()
	c.markSeen() // opening clears the unread badge
	p.Open(menuCardEntry(logsPanelID, &compPortalOverlay{comp: c, tag: logsPanelID, updateFn: c.Update},
		image.Rectangle{}, false, func() { c.Close() }))
}
