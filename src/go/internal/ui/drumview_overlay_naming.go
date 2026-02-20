package ui

import "image"

// NamingOverlay wraps the WAV instrument naming dialog as an Overlay.
// When active (naming=true), it occupies the full DrumView bounds to
// prevent any click from reaching underlying components.
type NamingOverlay struct {
	dv *DrumView
}

func (o *NamingOverlay) ID() string                   { return "naming" }
func (o *NamingOverlay) IsOpen() bool                 { return o.dv.naming }
func (o *NamingOverlay) ZIndex() int                  { return 240 }
func (o *NamingOverlay) InputBounds() image.Rectangle { return o.dv.Bounds }
func (o *NamingOverlay) Capturing() bool              { return o.dv.naming }

func (o *NamingOverlay) Close() {
	o.dv.closeNaming()
	SuppressClicksUntilMouseUp()
}

func (o *NamingOverlay) HandleInput(x, y int, pressed bool) InputResult {
	if !o.dv.naming {
		return InputIgnored
	}
	// The Update() path handles the actual text/button logic.
	// Consume here so nothing below processes the click.
	return InputConsumed
}

func (o *NamingOverlay) HandleWheel(x, y, steps int) InputResult {
	if !o.dv.naming {
		return InputIgnored
	}
	return InputConsumed // Prevent pass-through
}
