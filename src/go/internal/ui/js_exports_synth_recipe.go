//go:build js && !test

package ui

import (
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
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
}
