//go:build js && !test

package ui

import (
	"encoding/json"
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func (g *Game) initJSInsertEffects() {
	// addInsertEffectJS(instrumentID, effectType) -> slotIndex
	js.Global().Set("addInsertEffectJS", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 2 {
			return -1
		}
		id := args[0].String()
		et := audio.EffectType(args[1].String())
		idx := audio.AddInsertEffect(id, et, nil)
		return idx
	}))

	// removeInsertEffectJS(instrumentID, slotIndex)
	js.Global().Set("removeInsertEffectJS", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 2 {
			return nil
		}
		id := args[0].String()
		slot := args[1].Int()
		audio.RemoveInsertEffect(id, slot)
		return nil
	}))

	// moveInsertEffectJS(instrumentID, fromIndex, toIndex)
	js.Global().Set("moveInsertEffectJS", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 3 {
			return nil
		}
		id := args[0].String()
		from := args[1].Int()
		to := args[2].Int()
		audio.MoveInsertEffect(id, from, to)
		return nil
	}))

	// setInsertEffectParamJS(instrumentID, slotIndex, paramName, value)
	js.Global().Set("setInsertEffectParamJS", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 4 {
			return nil
		}
		id := args[0].String()
		slot := args[1].Int()
		param := args[2].String()
		value := args[3].Float()
		audio.SetInsertEffectParam(id, slot, param, value)
		return nil
	}))

	// toggleInsertEffectJS(instrumentID, slotIndex, enabled)
	js.Global().Set("toggleInsertEffectJS", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 3 {
			return nil
		}
		id := args[0].String()
		slot := args[1].Int()
		enabled := args[2].Bool()
		audio.ToggleInsertEffect(id, slot, enabled)
		return nil
	}))

	// getInsertEffectsJS(instrumentID) -> JSON string of []EffectSlot
	js.Global().Set("getInsertEffectsJS", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return "[]"
		}
		id := args[0].String()
		slots := audio.GetInsertEffects(id)
		if slots == nil {
			slots = []audio.EffectSlot{}
		}
		data, err := json.Marshal(slots)
		if err != nil {
			return "[]"
		}
		return string(data)
	}))

	// insertEffectCatalogJS() -> JSON string of map[EffectType][]EffectParamDef
	js.Global().Set("insertEffectCatalogJS", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		cat := audio.InsertEffectCatalog()
		data, err := json.Marshal(cat)
		if err != nil {
			return "{}"
		}
		return string(data)
	}))
}
