package audio

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Effects stay decoupled from instruments on file: each is defined once and referenced by id, in order, repeats
// allowed. Enabled is part of the identity, so a bypassed copy is a distinct def.
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

// Unknown ids are skipped: a partial or edited bundle degrades rather than fails.
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
