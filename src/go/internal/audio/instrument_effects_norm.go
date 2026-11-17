package audio

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// NormalizeEffects turns an ordered insert-effect chain into a deduplicated
// library ([]EffectDef, each defined once) plus an ordered list of id references
// (the "effect_chain"). Two placements of the same effect (identical Type,
// Params, and Enabled) share one library entry and one id; the id list preserves
// order and repeats. This is the on-file/in-store representation that keeps
// instruments decoupled from effect definitions.
//
// Identity for dedup is (Type, sorted Params, Enabled) — Enabled is part of the
// definition, so a bypassed copy is a distinct def.
func NormalizeEffects(chain []EffectSlot) ([]EffectDef, []string) {
	if len(chain) == 0 {
		return nil, nil
	}
	byKey := make(map[string]string, len(chain)) // canonical key -> assigned id
	var defs []EffectDef
	ids := make([]string, 0, len(chain))
	for _, s := range chain {
		key := effectSlotKey(s)
		id, ok := byKey[key]
		if !ok {
			id = fmt.Sprintf("fx%d", len(defs)+1)
			byKey[key] = id
			defs = append(defs, EffectDef{
				ID:      id,
				Type:    s.Type,
				Params:  copyParams(s.Params),
				Enabled: s.Enabled,
			})
		}
		ids = append(ids, id)
	}
	return defs, ids
}

// DenormalizeEffects resolves an ordered id chain back into a concrete
// []EffectSlot, deep-copying each referenced def's params. Unknown ids are
// skipped (lenient — a partial/edited bundle degrades rather than fails).
func DenormalizeEffects(defs []EffectDef, chain []string) []EffectSlot {
	if len(chain) == 0 {
		return nil
	}
	byID := make(map[string]EffectDef, len(defs))
	for _, d := range defs {
		byID[d.ID] = d
	}
	out := make([]EffectSlot, 0, len(chain))
	for _, id := range chain {
		d, ok := byID[id]
		if !ok {
			continue
		}
		out = append(out, EffectSlot{
			Type:    d.Type,
			Enabled: d.Enabled,
			Params:  copyParams(d.Params),
		})
	}
	return out
}

// effectSlotKey builds a canonical, order-independent key for dedup: the effect
// type, its params sorted by name, and the enabled flag.
func effectSlotKey(s EffectSlot) string {
	keys := make([]string, 0, len(s.Params))
	for k := range s.Params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(string(s.Type))
	b.WriteByte('|')
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(strconv.FormatFloat(s.Params[k], 'g', -1, 64))
		b.WriteByte(';')
	}
	b.WriteByte('|')
	if s.Enabled {
		b.WriteByte('1')
	} else {
		b.WriteByte('0')
	}
	return b.String()
}
