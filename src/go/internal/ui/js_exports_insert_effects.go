//go:build js && !test

package ui

import (
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// effectSlotToJS converts a single audio.EffectSlot into a plain JS object so
// callers don't need to JSON.parse a serialized blob on every read.
func effectSlotToJS(slot audio.EffectSlot) js.Value {
	obj := js.Global().Get("Object").New()
	obj.Set("type", string(slot.Type))
	obj.Set("enabled", slot.Enabled)
	params := js.Global().Get("Object").New()
	for k, v := range slot.Params {
		params.Set(k, v)
	}
	obj.Set("params", params)
	return obj
}

// effectParamDefToJS converts a single audio.EffectParamDef into a plain JS
// object with the same field names as the JSON tags.
func effectParamDefToJS(p audio.EffectParamDef) js.Value {
	obj := js.Global().Get("Object").New()
	obj.Set("name", p.Name)
	obj.Set("min", p.Min)
	obj.Set("max", p.Max)
	obj.Set("default", p.Default)
	if p.Unit != "" {
		obj.Set("unit", p.Unit)
	}
	return obj
}

func (g *Game) initJSInsertEffects() {
	// addInsertEffect(instrumentID, effectType) -> slotIndex
	js.Global().Set("addInsertEffect", jsFn(func(args jsArgs) any {
		if args.Len() < 2 {
			return -1
		}
		id := args.Str(0)
		et := audio.EffectType(args.Str(1))
		g.bumpParityGen("insert-fx-add", structuralMutationOptions{SkipPathsDirty: true, SkipPathChangeMark: true})
		idx := audio.AddInsertEffect(id, et, nil)
		emitInsertEffectAdded(id, idx, string(et))
		return idx
	}))

	// removeInsertEffect(instrumentID, slotIndex)
	js.Global().Set("removeInsertEffect", jsFn(func(args jsArgs) any {
		if args.Len() < 2 {
			return nil
		}
		id := args.Str(0)
		slot := args.Int(1)
		g.bumpParityGen("insert-fx-remove", structuralMutationOptions{SkipPathsDirty: true, SkipPathChangeMark: true})
		audio.RemoveInsertEffect(id, slot)
		emitInsertEffectRemoved(id, slot)
		return nil
	}))

	// moveInsertEffect(instrumentID, fromIndex, toIndex)
	js.Global().Set("moveInsertEffect", jsFn(func(args jsArgs) any {
		if args.Len() < 3 {
			return nil
		}
		id := args.Str(0)
		from := args.Int(1)
		to := args.Int(2)
		g.bumpParityGen("insert-fx-move", structuralMutationOptions{SkipPathsDirty: true, SkipPathChangeMark: true})
		audio.MoveInsertEffect(id, from, to)
		emitInsertEffectMoved(id, from, to)
		g.drum.recordUndoStep(hooks.EventInsertEffectMoved)
		return nil
	}))

	// setInsertEffectParam(instrumentID, slotIndex, paramName, value)
	js.Global().Set("setInsertEffectParam", jsFn(func(args jsArgs) any {
		if args.Len() < 4 {
			return nil
		}
		id := args.Str(0)
		slot := args.Int(1)
		param := args.Str(2)
		value := args.Float(3)
		// Param tweaks are continuous (slider drag); skip the gen bump on
		// param changes so we don't burn the grace window on every micro-step
		// — buffer hygiene cost would be too high. The chain shape isn't
		// changing, only an audio knob value, so no parity coordination is
		// needed.
		audio.SetInsertEffectParam(id, slot, param, value)
		emitInsertEffectParam(id, slot, param, value)
		return nil
	}))

	// toggleInsertEffect(instrumentID, slotIndex, enabled)
	js.Global().Set("toggleInsertEffect", jsFn(func(args jsArgs) any {
		if args.Len() < 3 {
			return nil
		}
		id := args.Str(0)
		slot := args.Int(1)
		enabled := args.Bool(2)
		g.bumpParityGen("insert-fx-toggle", structuralMutationOptions{SkipPathsDirty: true, SkipPathChangeMark: true})
		audio.ToggleInsertEffect(id, slot, enabled)
		emitInsertEffectToggled(id, slot, enabled)
		g.drum.recordUndoStep(hooks.EventInsertEffectToggled)
		return nil
	}))

	// getInsertEffects(instrumentID) -> Array of {type, enabled, params}
	// Returns a native JS array; callers must NOT JSON.parse it.
	js.Global().Set("getInsertEffects", jsFn(func(args jsArgs) any {
		arr := js.Global().Get("Array").New()
		if args.Len() < 1 {
			return arr
		}
		id := args.Str(0)
		slots := audio.GetInsertEffects(id)
		for _, s := range slots {
			arr.Call("push", effectSlotToJS(s))
		}
		return arr
	}))

	// insertEffectCatalog() -> Object keyed by effect type, each value an
	// Array of {name, min, max, default, unit?}. Returns native JS objects;
	// callers must NOT JSON.parse it.
	js.Global().Set("insertEffectCatalog", jsFn(func(args jsArgs) any {
		out := js.Global().Get("Object").New()
		for t, defs := range audio.InsertEffectCatalog() {
			arr := js.Global().Get("Array").New()
			for _, d := range defs {
				arr.Call("push", effectParamDefToJS(d))
			}
			out.Set(string(t), arr)
		}
		return out
	}))
}
