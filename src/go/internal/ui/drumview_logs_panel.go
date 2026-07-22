package ui

// logsPanelID identifies the Logs Navigation Panel portal entry.
const logsPanelID = "logs"

// logsSearchMobileInputID is the mobile native-input bridge id for the logs
// panel's name-search box (registered in native_gesture.go, read back in
// LogsComponent.Update), so mobile users get a real keyboard for search.
const logsSearchMobileInputID = "logs-search"

// IsLogsPanelOpen reports whether the Logs Navigation Panel portal is open.
func (dv *DrumView) IsLogsPanelOpen() bool {
	return dv.tree != nil && dv.portal().Has(logsPanelID)
}

// ensureLogsComp lazily creates the logs component (also used by the unread
// badge, which needs the component before the panel is first opened).
func (dv *DrumView) ensureLogsComp() *LogsComponent {
	if dv.logsComp == nil {
		dv.logsComp = NewLogsComponent(dv)
	}
	return dv.logsComp
}

// logsUnread returns the unread event/notification count + whether any unread
// is an error (drives the grid Logs-icon badge).
func (dv *DrumView) logsUnread() (int, bool) {
	return dv.ensureLogsComp().unreadCount()
}

// pushErrorToast raises the transient error toast (called from the notification
// push path for error notifications only).
func (dv *DrumView) pushErrorToast(text string) { dv.logsToast.push(text) }

// logsToastActive reports whether the error toast is currently visible.
func (dv *DrumView) logsToastActive() bool { return dv.logsToast.active() }

// openLogsPanel opens the Logs Navigation Panel as a modal overlay. The panel
// paints inside a centered rect; the portal supplies the dimming scrim. Closing
// is via the panel's X button, Esc, or a second tap on the logs icon.
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
	p.Open(PortalEntry{
		ID:      logsPanelID,
		Overlay: &compPortalOverlay{comp: c, tag: logsPanelID, updateFn: c.Update},
		Modal:   true,
		Scrim:   true,
		OnClose: func() { c.Close() },
	})
}
