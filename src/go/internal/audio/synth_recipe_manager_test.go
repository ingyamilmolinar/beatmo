package audio

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

const testInstID = "test-mgr-snare"

// Copied from the shipped snare.
func withTestInstDoc(t *testing.T) {
	t.Helper()
	if err := InstallInstrumentDocForTest(testInstID, roster.Snare); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { RemoveInstrumentDocForTest(testInstID) })
}

func TestSetInstrumentParam_RoundTripsThroughGet(t *testing.T) {
	withTestInstDoc(t)
	SetInstrumentParam(testInstID, "pitch", 3)

	got := GetInstrumentParams(testInstID)
	if got["pitch"] != 3 {
		t.Errorf("got pitch=%v want 3", got["pitch"])
	}
}

func TestSetInstrumentParam_OverwritesPriorValue(t *testing.T) {
	withTestInstDoc(t)
	SetInstrumentParam(testInstID, "pitch", 3)
	SetInstrumentParam(testInstID, "pitch", 7)

	if got := GetInstrumentParams(testInstID)["pitch"]; got != 7 {
		t.Errorf("got pitch=%v want 7", got)
	}
}

func TestSetInstrumentParam_AccumulatesAcrossNames(t *testing.T) {
	withTestInstDoc(t)
	SetInstrumentParam(testInstID, "pitch", 1)
	SetInstrumentParam(testInstID, "amp_decay", 0.5)

	got := GetInstrumentParams(testInstID)
	if got["pitch"] != 1 || got["amp_decay"] != 0.5 {
		t.Errorf("got pitch=%v amp_decay=%v, want 1 and 0.5", got["pitch"], got["amp_decay"])
	}
}

func TestSetInstrumentParams_BulkReplacesAllParams(t *testing.T) {
	withTestInstDoc(t)
	lib := GetInstrumentParams(testInstID)
	SetInstrumentParam(testInstID, "pitch", 1)
	SetInstrumentParam(testInstID, "amp_decay", 0.5)

	SetInstrumentParams(testInstID, RecipeParams{"pitch": 2})
	got := GetInstrumentParams(testInstID)
	if got["pitch"] != 2 || got["amp_decay"] != lib["amp_decay"] {
		t.Errorf("got pitch=%v amp_decay=%v; bulk Set must start from the library document (amp_decay=%v), not merge",
			got["pitch"], got["amp_decay"], lib["amp_decay"])
	}
}

func TestResetInstrumentParams_ClearsState(t *testing.T) {
	withTestInstDoc(t)
	SetInstrumentParam(testInstID, "pitch", 5)
	ResetInstrumentParams(testInstID)

	if InstrumentDocModified(testInstID) {
		t.Errorf("Reset must return the document to its library entry")
	}
}

func TestGetInstrumentParams_UnknownReturnsEmptyMap(t *testing.T) {
	got := GetInstrumentParams("nobody-here")
	if got == nil {
		t.Error("got=nil; want empty map (not nil) so range is safe")
	}
	if len(got) != 0 {
		t.Errorf("got=%v want empty map", got)
	}
}

func TestGetInstrumentParams_ReturnsCopy(t *testing.T) {
	withTestInstDoc(t)
	SetInstrumentParam(testInstID, "pitch", 4)

	a := GetInstrumentParams(testInstID)
	a["pitch"] = 99
	b := GetInstrumentParams(testInstID)
	if b["pitch"] != 4 {
		t.Errorf("Get returned shared map; mutation leaked: got=%v", b["pitch"])
	}
}

// The browser receives every document change through the platform push.
func TestSetInstrumentParam_FiresPlatformCallback(t *testing.T) {
	withTestInstDoc(t)

	oldCb := platformInstrumentDocChanged
	t.Cleanup(func() { platformInstrumentDocChanged = oldCb })

	var fired atomic.Int64
	var lastID string
	var lastPitch float64
	var mu sync.Mutex
	platformInstrumentDocChanged = func(id instrument.ID, d instrument.Doc) {
		mu.Lock()
		defer mu.Unlock()
		fired.Add(1)
		lastID = string(id)
		lastPitch = d.Stages["core"].Values["pitch"]
	}

	SetInstrumentParam(testInstID, "pitch", 5)

	if fired.Load() != 1 {
		t.Fatalf("platform callback fired %d times, want 1", fired.Load())
	}
	mu.Lock()
	defer mu.Unlock()
	if lastID != testInstID || lastPitch != 5 {
		t.Errorf("callback id=%q pitch=%v want %q / 5", lastID, lastPitch, testInstID)
	}
}

func TestSetInstrumentParam_PublishesHookEvent(t *testing.T) {
	withTestInstDoc(t)

	received := make(chan hooks.InstrumentParamPayload, 1)
	unsub := hooks.Subscribe(hooks.EventInstrumentParamChanged, func(e hooks.Event) {
		p, _ := e.Payload.(hooks.InstrumentParamPayload)
		select {
		case received <- p:
		default:
		}
	})
	t.Cleanup(unsub)

	SetInstrumentParam(testInstID, "pitch", 2.5)

	select {
	case got := <-received:
		if got.Instrument != testInstID {
			t.Errorf("got Channel=%q want %q", got.Instrument, testInstID)
		}
		if got.Param != "pitch" {
			t.Errorf("got Param=%q want pitch", got.Param)
		}
		if got.Value != 2.5 {
			t.Errorf("got Value=%v want 2.5", got.Value)
		}
	case <-time.After(time.Second):
		t.Fatal("EventInstrumentParamChanged never delivered")
	}
}

func TestSetInstrumentParams_ResetIsNoop_WhenAlreadyEmpty(t *testing.T) {
	// Reset twice on an unedited document; the second is a no-op. No panic.
	withTestInstDoc(t)
	ResetInstrumentParams(testInstID)
	ResetInstrumentParams(testInstID)
	if InstrumentDocModified(testInstID) {
		t.Errorf("an unedited document must stay unmodified")
	}
}
