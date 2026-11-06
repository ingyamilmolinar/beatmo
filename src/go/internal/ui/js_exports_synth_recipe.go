//go:build js && !test

package ui

import (
	"encoding/json"
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Phase 5: JS exports for the SynthRecipe registry + per-instrument
// param manager. Mirrors js_exports_insert_effects.go in shape so browser
// tests can drive the registry the same way they drive insert effects.

// recipeParamsToJS converts an audio.RecipeParams map into a plain JS
// object. Used by getInstrumentParams's return value so callers don't
// need to JSON.parse anything.
func recipeParamsToJS(p audio.RecipeParams) js.Value {
	obj := js.Global().Get("Object").New()
	for k, v := range p {
		obj.Set(k, v)
	}
	return obj
}

// recipeParamDefToJS converts an audio.ParamDef into a plain JS object.
func recipeParamDefToJS(d audio.ParamDef) js.Value {
	obj := js.Global().Get("Object").New()
	obj.Set("name", d.Name)
	obj.Set("min", d.Min)
	obj.Set("max", d.Max)
	obj.Set("default", d.Default)
	if d.Unit != "" {
		obj.Set("unit", d.Unit)
	}
	if d.Group != "" {
		obj.Set("group", d.Group)
	}
	if d.Label != "" {
		obj.Set("label", d.Label)
	}
	if d.Curve != "" {
		obj.Set("curve", d.Curve)
	}
	if len(d.Enum) > 0 {
		enum := js.Global().Get("Array").New(len(d.Enum))
		for i, e := range d.Enum {
			enum.SetIndex(i, e)
		}
		obj.Set("enum", enum)
	}
	return obj
}

func (g *Game) initJSSynthRecipe() {
	// setInstrumentParam(instrumentID, paramName, value)
	js.Global().Set("setInstrumentParam", jsFn(func(args jsArgs) any {
		if args.Len() < 3 {
			return nil
		}
		id := args.Instrument(0)
		name := args.Str(1)
		value := args.Float(2)
		audio.SetInstrumentParam(id, name, value)
		return nil
	}))

	// setInstrumentParams(instrumentID, paramsObject) — bulk set, replaces
	// the entire per-instrument param map (mirrors audio.SetInstrumentParams).
	js.Global().Set("setInstrumentParams", jsFn(func(args jsArgs) any {
		if args.Len() < 2 {
			return nil
		}
		id := args.Instrument(0)
		obj := args.At(1)
		if obj.Type() != js.TypeObject {
			return nil
		}
		p := audio.RecipeParams{}
		keys := js.Global().Get("Object").Call("keys", obj)
		for i := 0; i < keys.Length(); i++ {
			k := keys.Index(i).String()
			p[k] = obj.Get(k).Float()
		}
		audio.SetInstrumentParams(id, p)
		return nil
	}))

	// setSampleEdit(instrumentID, fieldsObject) — applies a non-destructive
	// Sampler edit descriptor (the flat field map the project file carries under
	// "sample_edit": start/end, transpose, gain_db, reverse, …) and announces it.
	// The same audio write the Sampler tab's Save performs; an empty object
	// clears the edit. Lets browser tests reach the sample-edit half of the
	// edit → export → import contract without driving the Sampler UI.
	js.Global().Set("setSampleEdit", jsFn(func(args jsArgs) any {
		if args.Len() < 2 {
			return nil
		}
		id := args.Instrument(0)
		obj := args.At(1)
		if obj.Type() != js.TypeObject {
			return nil
		}
		fields := map[string]float64{}
		keys := js.Global().Get("Object").Call("keys", obj)
		for i := 0; i < keys.Length(); i++ {
			k := keys.Index(i).String()
			fields[k] = obj.Get(k).Float()
		}
		if len(fields) == 0 {
			audio.ClearSampleEdit(id)
			return nil
		}
		audio.SetSampleEdit(id, audio.SampleEditFromFields(fields))
		return nil
	}))

	// getInstrumentParams(instrumentID) -> {name: value, ...}
	// getInstrumentDoc(id) → the instrument's project document as JSON ("" when
	// it has none) — the single source of a synth voice (spec 2026-10-04 §3).
	js.Global().Set("getInstrumentDoc", jsFn(func(args jsArgs) any {
		if args.Len() < 1 || args.At(0).Type() != js.TypeString {
			return ""
		}
		d, ok := audio.InstrumentDoc(args.Instrument(0))
		if !ok {
			return ""
		}
		b, err := json.Marshal(d)
		if err != nil {
			return ""
		}
		return string(b)
	}))

	js.Global().Set("getInstrumentParams", jsFn(func(args jsArgs) any {
		if args.Len() < 1 {
			return js.Global().Get("Object").New()
		}
		id := args.Instrument(0)
		return recipeParamsToJS(audio.GetInstrumentParams(id))
	}))

	// resetInstrumentParams(instrumentID)
	js.Global().Set("resetInstrumentParams", jsFn(func(args jsArgs) any {
		if args.Len() < 1 {
			return nil
		}
		audio.ResetInstrumentParams(args.Instrument(0))
		return nil
	}))

	// __setSynthDragDebug(bool) — flip verbose Synth-tab drag/schema-swap logging
	// (sdbg) on/off at runtime so a browser repro can capture the full UI→audio
	// cascade in the console. Debug-only; never called by production JS.
	js.Global().Set("__setSynthDragDebug", jsFn(func(args jsArgs) any {
		synthDragDebug = args.Len() > 0 && args.At(0).Truthy()
		// One call enables the WHOLE cascade: the Go [synthdrag] stream AND the
		// JS-side audio.js streams ([SYNTH-DISPATCH]/[SYNTH-UPD]/[SYNTH-EVT]/
		// [SYNTH-RDY]/[SYNTH-ERS]). Saves the user from setting three globals.
		js.Global().Set("__beatmoDebugSynthDispatch", synthDragDebug)
		js.Global().Set("__synthEvtTrace", synthDragDebug)
		lastSynthBuildSig = ""
		return synthDragDebug
	}))

	// setActiveEQTab(name) — open one of the bottom audio-panel tabs
	// ("synth","eq","wave","spectrum","levels","chain","sampler"). Lets e2e
	// browser tests drive the Synth tab so its live re-layout (generator
	// re-voicing swaps the section schema every frame) runs against a playing
	// engine — the exact path the user exercises when audio stopped.
	js.Global().Set("setActiveEQTab", jsFn(func(args jsArgs) any {
		if args.Len() < 1 || args.At(0).Type() != js.TypeString {
			return false
		}
		if err := g.SetActiveEQTab(args.Str(0)); err != nil {
			return false
		}
		return true
	}))

	// saveActiveRecipe(instrumentID) -> library id
	// Save As "over" for an explicit instrument id: the project copy and its
	// audio bundle become a new version of the library instrument it comes
	// from (persisted). The copy itself is untouched, so the voice does not
	// change. Same Go core as the dialog.
	js.Global().Set("saveActiveRecipe", jsFn(func(args jsArgs) any {
		if args.Len() < 1 || args.At(0).Type() != js.TypeString {
			return ""
		}
		return saveRecipeForInstrument(args.Instrument(0))
	}))

	// saveActiveRecipeAs(instrumentID, name) -> new library id
	// Save As "new" for an explicit instrument id: a new
	// content-addressed user instrument, with the project copy repointed at it.
	js.Global().Set("saveActiveRecipeAs", jsFn(func(args jsArgs) any {
		if args.Len() < 2 || args.At(0).Type() != js.TypeString || g.drum == nil {
			return ""
		}
		return g.drum.SaveActiveRecipeAsFor(args.Instrument(0), args.Str(1))
	}))

	// instrumentParamDefs(instrumentID) -> [paramDef...]
	// The knob definitions of the instrument's document — the same schema the
	// Synth tab builds its controls from (audio.InstrumentParamDefs). [] for an
	// id without a document.
	js.Global().Set("instrumentParamDefs", jsFn(func(args jsArgs) any {
		if args.Len() < 1 {
			return js.Global().Get("Array").New(0)
		}
		defs := audio.InstrumentParamDefs(args.Instrument(0))
		out := js.Global().Get("Array").New(len(defs))
		for i, d := range defs {
			out.SetIndex(i, recipeParamDefToJS(d))
		}
		return out
	}))

	g.initJSKitManagement()

	// Push every instrument document's flattened block into audio.js now that
	// window.setInstrumentBlock is defined (audio.js evaluates before the Go
	// runtime starts). Every later document change pushes itself
	// (SetInstrumentDoc → platformInstrumentDocChanged).
	audio.PushAllInstrumentDocsToPlatform()
}

// jsObjectToParams converts a plain-object JS value (param map) into a
// Go map[string]float64. Non-finite or non-numeric values are silently
// skipped — same boundary discipline as the userprefs + audio layers.
func jsObjectToParams(obj js.Value) map[string]float64 {
	if obj.Type() != js.TypeObject {
		return nil
	}
	keys := js.Global().Get("Object").Call("keys", obj)
	n := keys.Length()
	out := make(map[string]float64, n)
	for i := 0; i < n; i++ {
		k := keys.Index(i).String()
		v := obj.Get(k)
		if v.Type() != js.TypeNumber {
			continue
		}
		f := v.Float()
		// NaN/Inf check via the same idiom audio.isFiniteParam uses.
		if f != f || f > 1e308 || f < -1e308 {
			continue
		}
		out[k] = f
	}
	return out
}

// // kitsJSValue serialises every registered kit into a JS array of
// { id, displayName, members } objects. Used by listKits to give
// browser tests + future picker UI a single read-only view.
func kitsJSValue() js.Value {
	all := audio.KitsForExport()
	arr := js.Global().Get("Array").New(len(all))
	for i, k := range all {
		entry := js.Global().Get("Object").New()
		entry.Set("id", k.ID)
		entry.Set("displayName", k.DisplayName)
		members := js.Global().Get("Object").New()
		for role, instID := range k.Members {
			members.Set(role.String(), string(instID))
		}
		entry.Set("members", members)
		arr.SetIndex(i, entry)
	}
	return arr
}

// initJSKitManagement wires the Phase 6 kit JS surface. No UI consumer
// in this round; the exports exist so browser-side tests and future
// kit-picker UI can drive the registry today. createKit / setKitMember
// / applyKit mutate in-process state; persistence to project JSON lands
// in a later phase when the export.go schema picks up the Kits field.
func (g *Game) initJSKitManagement() {
	// listKits() → [{id, displayName, members: {role: instID}}]
	js.Global().Set("listKits", jsFn(func(args jsArgs) any {
		return kitsJSValue()
	}))

	// createKit(kitID, displayName, membersObj) → kitID
	// Registers (or replaces) a kit. membersObj is {role: instID, ...}.
	js.Global().Set("createKit", jsFn(func(args jsArgs) any {
		if args.Len() < 1 || args.At(0).Type() != js.TypeString {
			return ""
		}
		kit := audio.Kit{ID: args.Str(0), Members: map[instrument.Role]instrument.ID{}}
		if args.Len() >= 2 && args.At(1).Type() == js.TypeString {
			kit.DisplayName = args.Str(1)
		}
		if args.Len() >= 3 && args.At(2).Type() == js.TypeObject {
			keys := js.Global().Get("Object").Call("keys", args.At(2))
			for i := 0; i < keys.Length(); i++ {
				key := keys.Index(i).String()
				val := args.At(2).Get(key)
				role, err := instrument.ParseRole(key)
				if err != nil || role == instrument.RoleNone || val.Type() != js.TypeString {
					return ""
				}
				id, err := instrument.ParseID(val.String())
				if err != nil {
					return ""
				}
				kit.Members[role] = id
			}
		}
		audio.RegisterKit(kit)
		return kit.ID
	}))

	// setKitMember(kitID, role, instID) — upsert one role's instrument
	// on an existing kit. Returns false if the kit isn't registered.
	js.Global().Set("setKitMember", jsFn(func(args jsArgs) any {
		if args.Len() < 3 || args.At(0).Type() != js.TypeString || args.At(1).Type() != js.TypeString || args.At(2).Type() != js.TypeString {
			return false
		}
		kitID := args.Str(0)
		role, err := instrument.ParseRole(args.Str(1))
		instID := args.Instrument(2)
		if err != nil || role == instrument.RoleNone || instID == "" {
			return false
		}
		k, ok := audio.KitForID(kitID)
		if !ok {
			return false
		}
		if k.Members == nil {
			k.Members = map[instrument.Role]instrument.ID{}
		}
		k.Members[role] = instID
		audio.RegisterKit(k)
		return true
	}))

	// deleteKit(kitID) — remove the kit. Idempotent.
	js.Global().Set("deleteKit", jsFn(func(args jsArgs) any {
		if args.Len() < 1 || args.At(0).Type() != js.TypeString {
			return false
		}
		audio.UnregisterKit(args.Str(0))
		return true
	}))

	// applyKit(kitID) — rebind the active DrumView's rows per the
	// kit's role → instrument map. Returns the rebound row count.
	// Mix state (volume/pan/sends/EQ/effects) is preserved per Phase 6.
	js.Global().Set("applyKit", jsFn(func(args jsArgs) any {
		if args.Len() < 1 || args.At(0).Type() != js.TypeString {
			return 0
		}
		k, ok := audio.KitForID(args.Str(0))
		if !ok || g.drum == nil {
			return 0
		}
		return g.drum.ApplyKit(k)
	}))
}
