package audio

import (
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

const testInstID = "test-mgr-snare"

func makeFakeSnareRecipe(t *testing.T) string {
	t.Helper()
	const recipeID = "test-mgr-drum-snare"
	t.Cleanup(func() { unregisterRecipeForTest(recipeID) })
	RegisterRecipe(newFakeRecipe(recipeID,
		ParamDef{Name: "pitch", Min: -24, Max: 24, Default: 0},
		ParamDef{Name: "decay", Min: 0, Max: 4, Default: 1},
	))
	return recipeID
}

func TestSetInstrumentParam_RoundTripsThroughGet(t *testing.T) {
	t.Cleanup(func() { ResetInstrumentParams(testInstID) })
	SetInstrumentParam(testInstID, "pitch", 3)

	got := GetInstrumentParams(testInstID)
	if got["pitch"] != 3 {
		t.Errorf("got=%v want pitch=3", got)
	}
}

func TestSetInstrumentParam_OverwritesPriorValue(t *testing.T) {
	t.Cleanup(func() { ResetInstrumentParams(testInstID) })
	SetInstrumentParam(testInstID, "pitch", 3)
	SetInstrumentParam(testInstID, "pitch", 7)

	got := GetInstrumentParams(testInstID)
	if got["pitch"] != 7 {
		t.Errorf("got=%v want pitch=7", got)
	}
}

func TestSetInstrumentParam_AccumulatesAcrossNames(t *testing.T) {
	t.Cleanup(func() { ResetInstrumentParams(testInstID) })
	SetInstrumentParam(testInstID, "pitch", 1)
	SetInstrumentParam(testInstID, "decay", 0.5)

	got := GetInstrumentParams(testInstID)
	want := RecipeParams{"pitch": 1, "decay": 0.5}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got=%v want=%v", got, want)
	}
}

func TestSetInstrumentParams_BulkReplacesAllParams(t *testing.T) {
	t.Cleanup(func() { ResetInstrumentParams(testInstID) })
	SetInstrumentParam(testInstID, "pitch", 1)
	SetInstrumentParam(testInstID, "decay", 0.5)

	SetInstrumentParams(testInstID, RecipeParams{"pitch": 2})
	got := GetInstrumentParams(testInstID)
	want := RecipeParams{"pitch": 2}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got=%v want=%v (bulk Set should replace, not merge)", got, want)
	}
}

func TestResetInstrumentParams_ClearsState(t *testing.T) {
	SetInstrumentParam(testInstID, "pitch", 5)
	ResetInstrumentParams(testInstID)

	got := GetInstrumentParams(testInstID)
	if len(got) != 0 {
		t.Errorf("got=%v want empty after Reset", got)
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
	t.Cleanup(func() { ResetInstrumentParams(testInstID) })
	SetInstrumentParam(testInstID, "pitch", 4)

	a := GetInstrumentParams(testInstID)
	a["pitch"] = 99
	b := GetInstrumentParams(testInstID)
	if b["pitch"] != 4 {
		t.Errorf("Get returned shared map; mutation leaked: got=%v", b)
	}
}

func TestSetInstrumentParam_FiresPlatformCallback(t *testing.T) {
	t.Cleanup(func() { ResetInstrumentParams(testInstID) })

	oldCb := platformInstrumentParamsChanged
	t.Cleanup(func() { platformInstrumentParamsChanged = oldCb })

	var fired atomic.Int64
	var lastID string
	var lastParams RecipeParams
	var mu sync.Mutex
	platformInstrumentParamsChanged = func(id string, p RecipeParams) {
		mu.Lock()
		defer mu.Unlock()
		fired.Add(1)
		lastID = id
		lastParams = p
	}

	SetInstrumentParam(testInstID, "pitch", 5)

	if fired.Load() != 1 {
		t.Fatalf("platform callback fired %d times, want 1", fired.Load())
	}
	mu.Lock()
	defer mu.Unlock()
	if lastID != testInstID {
		t.Errorf("callback id=%q want %q", lastID, testInstID)
	}
	if lastParams["pitch"] != 5 {
		t.Errorf("callback params=%v want pitch=5", lastParams)
	}
}

func TestSetInstrumentParam_PublishesHookEvent(t *testing.T) {
	t.Cleanup(func() { ResetInstrumentParams(testInstID) })

	recipeID := makeFakeSnareRecipe(t)
	BindInstrumentToRecipe(testInstID, recipeID)
	t.Cleanup(func() { BindInstrumentToRecipe(testInstID, "") })

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
		if got.Channel != testInstID {
			t.Errorf("got Channel=%q want %q", got.Channel, testInstID)
		}
		if got.Recipe != recipeID {
			t.Errorf("got Recipe=%q want %q", got.Recipe, recipeID)
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

func TestBindInstrumentToRecipe_RoundTrips(t *testing.T) {
	t.Cleanup(func() { BindInstrumentToRecipe(testInstID, "") })

	BindInstrumentToRecipe(testInstID, "drum-snare")
	if got := RecipeForInstrument(testInstID); got != "drum-snare" {
		t.Errorf("got=%q want drum-snare", got)
	}
}

func TestBindInstrumentToRecipe_EmptyClears(t *testing.T) {
	BindInstrumentToRecipe(testInstID, "drum-snare")
	BindInstrumentToRecipe(testInstID, "")
	if got := RecipeForInstrument(testInstID); got != "" {
		t.Errorf("got=%q want empty (cleared)", got)
	}
}

func TestSetInstrumentParams_ResetIsNoop_WhenAlreadyEmpty(t *testing.T) {
	// Reset twice; second is a no-op. No panic, no callback fired.
	ResetInstrumentParams(testInstID)
	ResetInstrumentParams(testInstID)
	got := GetInstrumentParams(testInstID)
	if len(got) != 0 {
		t.Errorf("got=%v want empty", got)
	}
}
