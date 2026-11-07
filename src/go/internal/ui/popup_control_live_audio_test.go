//go:build test

package ui

// Turning a control changes the next trigger with no Enter, click-away or Save; only the undo step waits for the
// commit. Every test reads the AUDIO layer's state, never the UI's copy.

import (
	"image"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// SampleEditFor is what the voice dispatch renders through.
func TestSamplerWheelArrowIsAudibleWithoutCommit(t *testing.T) {
	g, _ := newMobileSamplerWheelGame(t)
	dv := g.drum
	capID := dv.sampler.captureID
	audio.ClearSampleEdit(capID)

	dv.openSamplerKnobWheelPopup(samplerKnobTranspose)
	if !dv.samplerWheelPopup.IsOpen() {
		t.Fatal("sampler wheel popup did not open")
	}

	pressKeyThroughShortcuts(g, ebiten.KeyArrowUp)

	if dv.sampler.transposeSemis == 0 {
		t.Fatal("precondition: ArrowUp did not move the UI-side transpose")
	}
	got, ok := audio.SampleEditFor(capID)
	if !ok {
		t.Fatalf("no sample edit reached the audio layer — the change is inaudible until commit")
	}
	if got.TransposeSemis != dv.sampler.transposeSemis {
		t.Fatalf("audio layer has stale transpose: audio=%v ui=%v", got.TransposeSemis, dv.sampler.transposeSemis)
	}
}

// The chop moves in audio during the drag, not on release.
func TestSamplerTrimHandleDragIsAudibleWhileDragging(t *testing.T) {
	g, _ := newMobileSamplerWheelGame(t)
	dv := g.drum
	capID := dv.sampler.captureID
	audio.ClearSampleEdit(capID)

	// Drag the START handle inward, with NO release.
	wave := dv.sampler.waveformRect
	if wave.Empty() {
		t.Skip("sampler wave rect not laid out")
	}
	dv.samplerHandleDrag(0, wave.Min.X+wave.Dx()/4)

	if dv.sampler.startFrac == 0 {
		t.Fatal("precondition: the drag did not move the UI-side start handle")
	}
	got, ok := audio.SampleEditFor(capID)
	if !ok {
		t.Fatalf("no sample edit reached the audio layer mid-drag — the trim is inaudible until release")
	}
	if got.StartFrac != dv.sampler.startFrac {
		t.Fatalf("audio layer has stale start: audio=%v ui=%v", got.StartFrac, dv.sampler.startFrac)
	}
}

// Single-click toggles, where live and commit coincide.
func TestSamplerToggleIsAudibleImmediately(t *testing.T) {
	g, _ := newMobileSamplerWheelGame(t)
	dv := g.drum
	capID := dv.sampler.captureID
	audio.ClearSampleEdit(capID)

	dv.sampler.normalize = true
	dv.commitSamplerEdit()

	got, ok := audio.SampleEditFor(capID)
	if !ok || !got.Normalize {
		t.Fatalf("normalize toggle did not reach the audio layer: ok=%v got=%+v", ok, got)
	}
}

// Into the channel's biquad chain on the keypress.
func TestEQWheelArrowIsAudibleWithoutCommit(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	dv := g.drum
	dv.eqPanelZone.SetActiveTab(TabEQ)

	ch := dv.activeEQChannel()
	dv.openEQKnobWheelPopup(0)
	if !dv.eqWheelPopup.IsOpen() {
		t.Fatal("EQ wheel popup did not open")
	}

	pressKeyThroughShortcuts(g, ebiten.KeyArrowUp)

	want := dv.eqPanelZone.bandGainsDB[0]
	if want == 0 {
		t.Fatal("precondition: ArrowUp did not move the UI-side band gain")
	}
	snap := audio.ChannelEQSnapshot(ch)
	if len(snap.Bands) == 0 {
		t.Fatalf("no EQ reached the audio layer for channel %q — the change is inaudible until commit", ch)
	}
	if snap.Bands[0].GainDB != want {
		t.Fatalf("audio layer has stale band gain: audio=%v ui=%v", snap.Bands[0].GainDB, want)
	}
}

// The release commit is only the undo step.
func TestMasterVolumeArrowIsAudibleWithoutCommit(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	dv := g.drum
	dv.Bounds = image.Rect(0, 400, 1280, 720)

	dv.openMasterVolumePopup()
	if !dv.masterVolPopup.IsOpen() {
		t.Fatal("master volume popup did not open")
	}
	before := audio.MainVolume()

	pressKeyThroughShortcuts(g, ebiten.KeyArrowDown)

	if audio.MainVolume() == before {
		t.Fatalf("master volume did not reach the audio layer: still %v", before)
	}
}

// DrumRow.Volume is what the sequencer reads for the next hit.
func TestRowVolumeArrowIsAudibleWithoutCommit(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	dv := g.drum
	dv.Bounds = image.Rect(0, 400, 1280, 720)

	dv.openVolumePopup(0)
	if !dv.volPopup.IsOpen() {
		t.Fatal("row volume popup did not open")
	}
	before := dv.Rows[0].Volume

	pressKeyThroughShortcuts(g, ebiten.KeyArrowDown)

	if dv.Rows[0].Volume >= before {
		t.Fatalf("row volume did not move on the keypress: %v -> %v", before, dv.Rows[0].Volume)
	}
}

// The debounced wheelNudger is only the undo step.
func TestSynthArmedArrowIsAudibleWithoutCommit(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	layoutSynthTab(t, g)

	inst := dv.synthTabActiveInstrument()
	idx := dv.synthSelectedSection().knobIdxs[0]
	name := dv.instEditorBindings[idx].def.Name
	dv.armSynthControl(inst, idx)
	before := audio.GetInstrumentParams(inst)[name]

	pressKeyThroughShortcuts(g, ebiten.KeyArrowUp)
	// Drain the kinetic impulse; onEdit fires per crossed detent.
	for i := 0; i < 120 && dv.instEditorArmedEditor != nil && dv.instEditorArmedEditor.Tick(); i++ {
	}

	if got := audio.GetInstrumentParams(inst)[name]; got == before {
		t.Fatalf("armed synth param %q did not reach the audio layer: still %v", name, got)
	}
}

// Live per detent, announced once per gesture: three presses, then the end, give exactly one event.
func TestSamplerLiveApplyStillAnnouncesOncePerGesture(t *testing.T) {
	var count int32
	unsub := hooks.Subscribe(hooks.EventSampleEditChanged, func(e hooks.Event) { atomic.AddInt32(&count, 1) })
	t.Cleanup(unsub)

	g, _ := newMobileSamplerWheelGame(t)
	dv := g.drum
	audio.ClearSampleEdit(dv.sampler.captureID)
	time.Sleep(40 * time.Millisecond) // let the Clear's own event drain
	atomic.StoreInt32(&count, 0)

	dv.openSamplerKnobWheelPopup(samplerKnobTranspose)
	for i := 0; i < 3; i++ {
		pressKeyThroughShortcuts(g, ebiten.KeyArrowUp)
	}
	time.Sleep(40 * time.Millisecond)
	if c := atomic.LoadInt32(&count); c != 0 {
		t.Fatalf("live per-detent applies announced %d times; the log contract is one line per GESTURE", c)
	}

	dv.samplerWheelPopup.Accept() // gesture ends
	waitForRelease(t, func() bool { return atomic.LoadInt32(&count) == 1 })
	time.Sleep(40 * time.Millisecond)
	if c := atomic.LoadInt32(&count); c != 1 {
		t.Fatalf("gesture end announced %d times, want exactly 1", c)
	}
}
