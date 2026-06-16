//go:build js && !test

package ui

import (
	"encoding/json"
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
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
	return obj
}

func (g *Game) initJSSynthRecipe() {
	// setInstrumentParam(instrumentID, paramName, value)
	js.Global().Set("setInstrumentParam", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 3 {
			return nil
		}
		id := args[0].String()
		name := args[1].String()
		value := args[2].Float()
		audio.SetInstrumentParam(id, name, value)
		return nil
	}))

	// setInstrumentParams(instrumentID, paramsObject) — bulk set, replaces
	// the entire per-instrument param map (mirrors audio.SetInstrumentParams).
	js.Global().Set("setInstrumentParams", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 2 {
			return nil
		}
		id := args[0].String()
		obj := args[1]
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

	// getInstrumentParams(instrumentID) -> {name: value, ...}
	js.Global().Set("getInstrumentParams", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return js.Global().Get("Object").New()
		}
		id := args[0].String()
		return recipeParamsToJS(audio.GetInstrumentParams(id))
	}))

	// resetInstrumentParams(instrumentID)
	js.Global().Set("resetInstrumentParams", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		audio.ResetInstrumentParams(args[0].String())
		return nil
	}))

	// __setSynthDragDebug(bool) — flip verbose Synth-tab drag/schema-swap logging
	// (sdbg) on/off at runtime so a browser repro can capture the full UI→audio
	// cascade in the console. Debug-only; never called by production JS.
	js.Global().Set("__setSynthDragDebug", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		synthDragDebug = len(args) > 0 && args[0].Truthy()
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
	js.Global().Set("setActiveEQTab", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 || args[0].Type() != js.TypeString {
			return false
		}
		if err := g.SetActiveEQTab(args[0].String()); err != nil {
			return false
		}
		return true
	}))

	// saveActiveRecipe(instrumentID) -> recipeID
	// Mirrors the Synth-tab Save button for an explicit instrument id: folds
	// the instrument's effective params into the recipe defaults and persists,
	// keeping the per-instrument overlay so the WebAudio voice cache retains
	// the just-saved tone. Routes through the same Go core as the button.
	js.Global().Set("saveActiveRecipe", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 || args[0].Type() != js.TypeString {
			return ""
		}
		return saveRecipeForInstrument(args[0].String())
	}))

	// resetActiveRecipe(instrumentID) -> recipeID
	// Mirrors the Synth-tab Reset button for an explicit instrument id:
	// restores the recipe's original shipped defaults (undoing any Save) and
	// clears the per-instrument overlay. Routes through the same Go core.
	js.Global().Set("resetActiveRecipe", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 || args[0].Type() != js.TypeString {
			return ""
		}
		return resetRecipeForInstrument(args[0].String())
	}))

	// recipeForInstrument(instrumentID) -> recipeID string
	js.Global().Set("recipeForInstrument", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return ""
		}
		return audio.RecipeForInstrument(args[0].String())
	}))

	// synthRecipeCatalog() -> { recipeID: { displayName, category, params: [paramDef...] } }
	// Mirrors insertEffectCatalog() for the recipe registry.
	js.Global().Set("synthRecipeCatalog", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		out := js.Global().Get("Object").New()
		regs := audio.RecipeRegistrations()
		for _, id := range audio.RecipeOrder() {
			r := regs[id]
			if r == nil {
				continue
			}
			entry := js.Global().Get("Object").New()
			entry.Set("displayName", r.DisplayName)
			entry.Set("category", r.Category)
			params := js.Global().Get("Array").New(len(r.Params))
			for i, d := range r.Params {
				params.SetIndex(i, recipeParamDefToJS(d))
			}
			entry.Set("params", params)
			out.Set(id, entry)
		}
		return out
	}))

	// recipeDefaultParams(recipeID) -> {name: value}
	js.Global().Set("recipeDefaultParams", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return js.Global().Get("Object").New()
		}
		return recipeParamsToJS(audio.RecipeDefaultParams(args[0].String()))
	}))

	// synthMirrorPCMLen() -> int
	// Stage 5 debug bridge: renders the right-pane synth mirror SYNCHRONOUSLY
	// for the active synth instrument and returns the resulting PCM sample
	// count. Lets a browser test verify the WASM↔audio mirror path produces a
	// non-empty render without reaching into DSP internals. Returns 0 when no
	// synth instrument is active. (DSP correctness is covered in Go — see the
	// synth_mirror_test.go suite.)
	js.Global().Set("synthMirrorPCMLen", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return 0
		}
		inst := g.drum.resolveSynthInstrument(g.drum.synthTabActiveInstrument())
		if inst == "" {
			return 0
		}
		g.drum.renderSynthMirrorNow(inst)
		return g.drum.synthMirrorPCMLen()
	}))

	// synthMirrorPCMChecksum() -> int
	// Companion to synthMirrorPCMLen: renders the mirror SYNCHRONOUSLY for the
	// active synth instrument and returns a 32-bit fingerprint of the PCM
	// CONTENT. A browser test calls it before/after a knob change to prove the
	// WASM mirror REACTS to params (the fingerprint differs) — a length-only
	// check would pass even for a frozen render. Returns 0 when no synth is
	// active. (DSP correctness is covered in Go — see synth_mirror_test.go.)
	js.Global().Set("synthMirrorPCMChecksum", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return 0
		}
		inst := g.drum.resolveSynthInstrument(g.drum.synthTabActiveInstrument())
		if inst == "" {
			return 0
		}
		g.drum.renderSynthMirrorNow(inst)
		return g.drum.synthMirrorPCMChecksum()
	}))

	g.initJSSynthRecipeManagement()
	g.initJSKitManagement()

	// Bootstrap-push the shipped defaults of any divergent preset (e.g.
	// modular-pad) into the JS audio bridge now that window.seedInstrumentDefaults
	// is defined (audio.js evaluates before the Go runtime starts) and the
	// recipe registry is populated. Runs for every browser entry point
	// (production main.wasm + the playtest harness), so a preset whose defaults
	// diverge from the C render built-ins renders correctly before any edit.
	// Desktop never reaches here (js build only); its native push hook is a
	// no-op because the instrument's Render closure already bakes the preset.
	audio.SeedInstrumentDefaultsToPlatform()
}

// initJSSynthRecipeManagement wires the Phase 5 user-recipe lifecycle
// surface: save / create / delete / list / export / import. These are
// the JS-side handles the Synth tab Save / Save-As buttons mirror —
// browser tests and any future scripting consumers go through here.
//
// All write paths funnel through the same ui.RecipeSaveSink the desktop
// builds use; if no sink is registered (test harness without
// SetRecipeSink), the writes silently no-op but the in-process registry
// still mutates so the user hears the change.
func (g *Game) initJSSynthRecipeManagement() {
	// saveUserRecipe(recipeID, paramsObj) — persist the override map as
	// the new defaults for recipeID. Mirrors the Synth-tab Save button.
	// Returns the recipe id on success, empty string on failure.
	js.Global().Set("saveUserRecipe", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 2 || args[0].Type() != js.TypeString {
			return ""
		}
		recipeID := args[0].String()
		params := jsObjectToParams(args[1])
		if sink := activeRecipeSink(); sink != nil {
			_ = sink.SaveRecipeOverride(recipeID, params)
		}
		audio.UpdateRecipeDefaultsAndInvalidate(recipeID, params)
		hooks.PublishKind(hooks.EventRecipeSaved, hooks.RecipePayload{RecipeID: recipeID})
		return recipeID
	}))

	// createUserRecipe(baseRecipeID, displayName, paramsObj, instrumentID?) → newID
	// Registers a new user-scoped recipe id delegating to baseRecipeID;
	// persists the doc via the sink. Mirrors the Synth-tab Save-As button.
	// Optional fourth arg rebinds the given instrument id to the new
	// recipe so the next trigger uses it (matches dv.SaveActiveRecipeAs
	// semantics — passing the row's instrument id from JS keeps the
	// rebind in lockstep with what the Save-As button does in Go).
	js.Global().Set("createUserRecipe", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 || args[0].Type() != js.TypeString {
			return ""
		}
		baseID := args[0].String()
		displayName := ""
		if len(args) >= 2 && args[1].Type() == js.TypeString {
			displayName = args[1].String()
		}
		var seed map[string]float64
		if len(args) >= 3 {
			seed = jsObjectToParams(args[2])
		}
		rebindInst := ""
		if len(args) >= 4 && args[3].Type() == js.TypeString {
			rebindInst = args[3].String()
		}
		if displayName == "" {
			if reg, ok := audio.RecipeRegistrations()[baseID]; ok && reg != nil {
				displayName = reg.DisplayName + " (saved)"
			} else {
				displayName = baseID + " (saved)"
			}
		}
		newID := generateUserRecipeID(baseID, seed)
		doc, err := audio.RegisterUserRecipeFromBase(newID, displayName, baseID, seed)
		if err != nil {
			return ""
		}
		if sink := activeRecipeSink(); sink != nil {
			if raw, mErr := json.Marshal(doc); mErr == nil {
				_ = sink.SaveUserRecipe(newID, raw)
			}
		}
		if rebindInst != "" {
			audio.BindInstrumentToRecipe(rebindInst, newID)
			audio.ResetInstrumentParams(rebindInst)
		}
		hooks.PublishKind(hooks.EventRecipeCreated, hooks.RecipePayload{
			RecipeID:     newID,
			BaseRecipe:   baseID,
			InstrumentID: rebindInst,
			DisplayName:  displayName,
		})
		return newID
	}))

	// deleteUserRecipe(recipeID) — unregister + ask the sink to drop the
	// persisted doc + persisted override (idempotent).
	js.Global().Set("deleteUserRecipe", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 || args[0].Type() != js.TypeString {
			return false
		}
		recipeID := args[0].String()
		audio.UnregisterRecipeForTest(recipeID)
		if sink := activeRecipeSink(); sink != nil {
			if rs, ok := sink.(recipeStoreDeleter); ok {
				_ = rs.DeleteUserRecipe(recipeID)
				_ = rs.DeleteRecipeOverride(recipeID)
			}
		}
		hooks.PublishKind(hooks.EventRecipeDeleted, hooks.RecipePayload{RecipeID: recipeID})
		return true
	}))

	// listUserRecipes() → [{id, displayName, baseRecipe, origin}]
	// Iterates the recipe registry and returns the entries whose
	// category is the user-scope ("user"). Cheap O(N) walk over a
	// process-global table; called rarely (menu open / picker refresh).
	js.Global().Set("listUserRecipes", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		regs := audio.RecipeRegistrations()
		arr := js.Global().Get("Array").New(0)
		i := 0
		for _, id := range audio.RecipeOrder() {
			reg := regs[id]
			if reg == nil || reg.Category != "user" {
				continue
			}
			entry := js.Global().Get("Object").New()
			entry.Set("id", id)
			entry.Set("displayName", reg.DisplayName)
			entry.Set("category", reg.Category)
			arr.SetIndex(i, entry)
			i++
		}
		return arr
	}))

	// exportUserRecipe(recipeID) → JSON string ready to share or save
	// as a .beatmo-preset.json file. Returns empty string if the recipe
	// isn't registered or marshalling fails.
	js.Global().Set("exportUserRecipe", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 || args[0].Type() != js.TypeString {
			return ""
		}
		recipeID := args[0].String()
		reg := audio.RecipeRegistrations()[recipeID]
		if reg == nil {
			return ""
		}
		// Reconstruct a RecipeDoc from the live registration so the
		// exported bytes always reflect the current defaults (which
		// may have drifted from the persisted override since startup).
		doc := audio.RecipeDoc{
			ID:          reg.ID,
			DisplayName: reg.DisplayName,
			Category:    reg.Category,
			ParamDefs:   reg.Params,
			Origin:      audio.OriginUser,
		}
		raw, err := json.Marshal(doc)
		if err != nil {
			return ""
		}
		return string(raw)
	}))

	// importUserRecipe(jsonString) → newID
	// Decodes the JSON into a RecipeDoc, registers it via the plugin
	// path, persists it via the sink. The decoded BaseRecipe must
	// already be registered or the import is rejected.
	js.Global().Set("importUserRecipe", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 || args[0].Type() != js.TypeString {
			return ""
		}
		raw := []byte(args[0].String())
		var doc audio.RecipeDoc
		if err := json.Unmarshal(raw, &doc); err != nil || doc.ID == "" {
			return ""
		}
		if _, err := audio.RegisterUserRecipeFromBase(doc.ID, doc.DisplayName, doc.BaseRecipe, doc.ParamSeed); err != nil {
			return ""
		}
		if sink := activeRecipeSink(); sink != nil {
			_ = sink.SaveUserRecipe(doc.ID, raw)
		}
		hooks.PublishKind(hooks.EventRecipeCreated, hooks.RecipePayload{
			RecipeID:    doc.ID,
			BaseRecipe:  doc.BaseRecipe,
			DisplayName: doc.DisplayName,
		})
		return doc.ID
	}))
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
			members.Set(role, instID)
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
	js.Global().Set("listKits", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return kitsJSValue()
	}))

	// createKit(kitID, displayName, membersObj) → kitID
	// Registers (or replaces) a kit. membersObj is {role: instID, ...}.
	js.Global().Set("createKit", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 || args[0].Type() != js.TypeString {
			return ""
		}
		kit := audio.Kit{ID: args[0].String(), Members: map[string]string{}}
		if len(args) >= 2 && args[1].Type() == js.TypeString {
			kit.DisplayName = args[1].String()
		}
		if len(args) >= 3 && args[2].Type() == js.TypeObject {
			keys := js.Global().Get("Object").Call("keys", args[2])
			for i := 0; i < keys.Length(); i++ {
				role := keys.Index(i).String()
				val := args[2].Get(role)
				if val.Type() == js.TypeString {
					kit.Members[role] = val.String()
				}
			}
		}
		audio.RegisterKit(kit)
		return kit.ID
	}))

	// setKitMember(kitID, role, instID) — upsert one role's instrument
	// on an existing kit. Returns false if the kit isn't registered.
	js.Global().Set("setKitMember", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 3 || args[0].Type() != js.TypeString || args[1].Type() != js.TypeString || args[2].Type() != js.TypeString {
			return false
		}
		kitID, role, instID := args[0].String(), args[1].String(), args[2].String()
		k, ok := audio.KitForID(kitID)
		if !ok {
			return false
		}
		if k.Members == nil {
			k.Members = map[string]string{}
		}
		k.Members[role] = instID
		audio.RegisterKit(k)
		return true
	}))

	// deleteKit(kitID) — remove the kit. Idempotent.
	js.Global().Set("deleteKit", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 || args[0].Type() != js.TypeString {
			return false
		}
		audio.UnregisterKit(args[0].String())
		return true
	}))

	// applyKit(kitID) — rebind the active DrumView's rows per the
	// kit's role → instrument map. Returns the rebound row count.
	// Mix state (volume/pan/sends/EQ/effects) is preserved per Phase 6.
	js.Global().Set("applyKit", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 || args[0].Type() != js.TypeString {
			return 0
		}
		k, ok := audio.KitForID(args[0].String())
		if !ok || g.drum == nil {
			return 0
		}
		return g.drum.ApplyKit(k)
	}))
}

// recipeStoreDeleter is the optional subset of ui.RecipeSaveSink that
// also supports deletion. The sink interface itself is write-only
// (Save*); deletion lives on the underlying userprefs.RecipeStore.
// Type-assert at the call site so test sinks that only implement Save*
// don't need a delete stub.
type recipeStoreDeleter interface {
	DeleteUserRecipe(recipeID string) error
	DeleteRecipeOverride(recipeID string) error
}
