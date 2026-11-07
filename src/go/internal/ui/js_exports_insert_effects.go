//go:build js && !test

package ui

import (
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// Plain JS objects, so callers need not JSON.parse.
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
		inst := args.Instrument(0)
		et, err := audio.ParseEffectType(args.Str(1))
		if inst == "" || err != nil {
			return -1
		}
		g.bumpParityGen("insert-fx-add", structuralMutationOptions{SkipPathsDirty: true, SkipPathChangeMark: true})
		idx := audio.AddInsertEffect(inst, et, nil)
		emitInsertEffectAdded(channel.Of(inst), idx, string(et))
		return idx
	}))

	// removeInsertEffect(instrumentID, slotIndex)
	js.Global().Set("removeInsertEffect", jsFn(func(args jsArgs) any {
		if args.Len() < 2 {
			return nil
		}
		inst := args.Instrument(0)
		if inst == "" {
			return nil
		}
		slot := args.Int(1)
		g.bumpParityGen("insert-fx-remove", structuralMutationOptions{SkipPathsDirty: true, SkipPathChangeMark: true})
		audio.RemoveInsertEffect(inst, slot)
		emitInsertEffectRemoved(channel.Of(inst), slot)
		return nil
	}))

	// moveInsertEffect(instrumentID, fromIndex, toIndex)
	js.Global().Set("moveInsertEffect", jsFn(func(args jsArgs) any {
		if args.Len() < 3 {
			return nil
		}
		inst := args.Instrument(0)
		if inst == "" {
			return nil
		}
		from := args.Int(1)
		to := args.Int(2)
		g.bumpParityGen("insert-fx-move", structuralMutationOptions{SkipPathsDirty: true, SkipPathChangeMark: true})
		audio.MoveInsertEffect(inst, from, to)
		emitInsertEffectMoved(channel.Of(inst), from, to)
		g.drum.recordUndoStep(hooks.EventInsertEffectMoved)
		return nil
	}))

	// setInsertEffectParam(instrumentID, slotIndex, paramName, value)
	js.Global().Set("setInsertEffectParam", jsFn(func(args jsArgs) any {
		if args.Len() < 4 {
			return nil
		}
		inst := args.Instrument(0)
		if inst == "" {
			return nil
		}
		slot := args.Int(1)
		param := args.Str(2)
		value := args.Float(3)
		// No gen bump: a drag would burn the grace window every step, and only a knob value changes, not the chain.
		audio.SetInsertEffectParam(inst, slot, param, value)
		emitInsertEffectParam(channel.Of(inst), slot, param, value)
		return nil
	}))

	// toggleInsertEffect(instrumentID, slotIndex, enabled)
	js.Global().Set("toggleInsertEffect", jsFn(func(args jsArgs) any {
		if args.Len() < 3 {
			return nil
		}
		inst := args.Instrument(0)
		if inst == "" {
			return nil
		}
		slot := args.Int(1)
		enabled := args.Bool(2)
		g.bumpParityGen("insert-fx-toggle", structuralMutationOptions{SkipPathsDirty: true, SkipPathChangeMark: true})
		audio.ToggleInsertEffect(inst, slot, enabled)
		emitInsertEffectToggled(channel.Of(inst), slot, enabled)
		g.drum.recordUndoStep(hooks.EventInsertEffectToggled)
		return nil
	}))

	// getInsertEffects(id) -> [{type,enabled,params}], a native array: do not JSON.parse.
	js.Global().Set("getInsertEffects", jsFn(func(args jsArgs) any {
		arr := js.Global().Get("Array").New()
		if args.Len() < 1 {
			return arr
		}
		inst := args.Instrument(0)
		slots := audio.GetInsertEffects(inst)
		for _, s := range slots {
			arr.Call("push", effectSlotToJS(s))
		}
		return arr
	}))

	// insertEffectCatalog() -> {type: [{name,min,max,default,unit?}]}, native: do not JSON.parse.
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
