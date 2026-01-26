package ui

import (
	"image"
	"testing"
)

func TestTransportComponent_Creation(t *testing.T) {
	tc := NewTransportComponent("transport")

	if tc.ID() != "transport" {
		t.Errorf("ID() = %q, want %q", tc.ID(), "transport")
	}

	// Verify all widgets are initialized
	if tc.playBtn == nil {
		t.Error("playBtn should be initialized")
	}
	if tc.stopBtn == nil {
		t.Error("stopBtn should be initialized")
	}
	if tc.bpmBox == nil {
		t.Error("bpmBox should be initialized")
	}
	if tc.mainVolSlider == nil {
		t.Error("mainVolSlider should be initialized")
	}
}

func TestTransportComponent_SetProps(t *testing.T) {
	tc := NewTransportComponent("transport")

	props := TransportProps{
		BPM:        140,
		IsPlaying:  true,
		Follow:     true,
		MainVolume: 0.8,
	}
	tc.SetProps(props)

	got := tc.Props()
	if got.BPM != 140 {
		t.Errorf("Props().BPM = %d, want 140", got.BPM)
	}
	if !got.IsPlaying {
		t.Error("Props().IsPlaying should be true")
	}
	if got.MainVolume != 0.8 {
		t.Errorf("Props().MainVolume = %f, want 0.8", got.MainVolume)
	}

	// BPM box should be updated
	if tc.BPMText() != "140" {
		t.Errorf("BPMText() = %q, want %q", tc.BPMText(), "140")
	}
}

func TestTransportComponent_Callbacks(t *testing.T) {
	tc := NewTransportComponent("transport")
	tc.SetBounds(image.Rect(0, 0, 600, 100))
	tc.Mount(MountContext{Bounds: tc.Bounds()})

	playCalled := false
	stopCalled := false
	bpmChangeDelta := 0

	tc.SetProps(TransportProps{
		BPM: 120,
		OnPlay: func() {
			playCalled = true
		},
		OnStop: func() {
			stopCalled = true
		},
		OnBPMChange: func(delta int) {
			bpmChangeDelta = delta
		},
	})

	// Trigger layout
	tc.recalcLayout()

	// Simulate play button click
	tc.playBtn.OnClick()
	if !playCalled {
		t.Error("OnPlay callback should be called")
	}

	// Simulate stop button click
	tc.stopBtn.OnClick()
	if !stopCalled {
		t.Error("OnStop callback should be called")
	}

	// Simulate BPM increment
	tc.bpmIncBtn.OnClick()
	if bpmChangeDelta != 1 {
		t.Errorf("OnBPMChange delta = %d, want 1", bpmChangeDelta)
	}

	// Simulate BPM decrement
	tc.bpmDecBtn.OnClick()
	if bpmChangeDelta != -1 {
		t.Errorf("OnBPMChange delta = %d, want -1", bpmChangeDelta)
	}
}

func TestTransportComponent_Layout(t *testing.T) {
	tc := NewTransportComponent("transport")
	bounds := image.Rect(0, 0, 600, 100)
	tc.SetBounds(bounds)
	tc.Mount(MountContext{Bounds: bounds})

	// Force layout calculation
	tc.recalcLayout()

	// Verify buttons have non-empty rects
	if tc.playBtn.Rect().Empty() {
		t.Error("playBtn should have non-empty rect after layout")
	}
	if tc.stopBtn.Rect().Empty() {
		t.Error("stopBtn should have non-empty rect after layout")
	}
	if tc.bpmBox.Rect.Empty() {
		t.Error("bpmBox should have non-empty rect after layout")
	}

	// Verify buttons are within bounds
	if !tc.playBtn.Rect().In(bounds) {
		t.Error("playBtn should be within bounds")
	}
	if !tc.stopBtn.Rect().In(bounds) {
		t.Error("stopBtn should be within bounds")
	}
}

func TestTransportComponent_Animations(t *testing.T) {
	tc := NewTransportComponent("transport")

	// Trigger animation
	tc.state.playAnim = 1.0

	// Decay should reduce animation value
	tc.decayAnims()
	if tc.state.playAnim >= 1.0 {
		t.Error("Animation should decay")
	}

	// After many decays, should approach zero
	for i := 0; i < 100; i++ {
		tc.decayAnims()
	}
	if tc.state.playAnim > 0.01 {
		t.Errorf("Animation should be near zero, got %f", tc.state.playAnim)
	}
}

func TestTransportComponent_BPMError(t *testing.T) {
	tc := NewTransportComponent("transport")

	if tc.state.bpmErrorAnim != 0 {
		t.Error("bpmErrorAnim should start at 0")
	}

	tc.ShowBPMError()
	if tc.state.bpmErrorAnim != 1 {
		t.Errorf("bpmErrorAnim = %f, want 1", tc.state.bpmErrorAnim)
	}
}
