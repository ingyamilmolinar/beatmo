package instrument

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"sort"
)

// Diff returns the sorted paths where a and b differ. A module present on one
// side only is reported by its own path; otherwise its type/on/v/notes and
// changed values are listed.
func Diff(a, b Doc) []string {
	var out []string
	add := func(p string) { out = append(out, p) }
	if a.Name != b.Name {
		add("name")
	}
	if a.Category != b.Category {
		add("category")
	}
	if a.Notes != b.Notes {
		add("notes")
	}
	if a.Render != b.Render {
		add("render")
	}
	keys := map[string]bool{}
	for k := range a.Stages {
		keys[k] = true
	}
	for k := range b.Stages {
		keys[k] = true
	}
	for _, k := range sortedKeys(keys) {
		sa, oka := a.Stages[k]
		sb, okb := b.Stages[k]
		if oka != okb {
			add("stages." + k)
			continue
		}
		out = append(out, diffModule("stages."+k,
			Module{Type: k, V: sa.V, On: sa.On, Values: sa.Values, Notes: sa.Notes},
			Module{Type: k, V: sb.V, On: sb.On, Values: sb.Values, Notes: sb.Notes})...)
		if !tablesEqual(sa.Table, sb.Table) {
			add("stages." + k + ".table")
		}
	}
	switch {
	case (a.Source == nil) != (b.Source == nil):
		add("source")
	case a.Source != nil:
		out = append(out, diffModule("source", *a.Source, *b.Source)...)
	}
	for i := 0; i < max(len(a.Layers), len(b.Layers)); i++ {
		var la, lb *Module
		if i < len(a.Layers) {
			la = a.Layers[i]
		}
		if i < len(b.Layers) {
			lb = b.Layers[i]
		}
		p := fmt.Sprintf("layers[%d]", i)
		switch {
		case (la == nil) != (lb == nil):
			add(p)
		case la != nil:
			out = append(out, diffModule(p, *la, *lb)...)
		}
	}
	if len(a.FX) != len(b.FX) {
		add("fx")
	} else {
		for i := range a.FX {
			out = append(out, diffModule(fmt.Sprintf("fx[%d]", i), a.FX[i], b.FX[i])...)
		}
	}
	sort.Strings(out)
	return out
}

func diffModule(path string, a, b Module) []string {
	if a.Type != b.Type {
		return []string{path + ".type"}
	}
	var out []string
	if a.On != b.On {
		out = append(out, path+".on")
	}
	if a.V != b.V {
		out = append(out, path+".v")
	}
	if a.Notes != b.Notes {
		out = append(out, path+".notes")
	}
	names := map[string]bool{}
	for k := range a.Values {
		names[k] = true
	}
	for k := range b.Values {
		names[k] = true
	}
	for _, k := range sortedKeys(names) {
		va, oka := a.Values[k]
		vb, okb := b.Values[k]
		if oka != okb || va != vb {
			out = append(out, path+".values."+k)
		}
	}
	return out
}

// BlockHash is the voice-cache key of a flattened block: FNV-1a 64 over the
// exact float64 bits, little endian.
func BlockHash(block []float64) uint64 {
	h := fnv.New64a()
	var buf [8]byte
	for _, x := range block {
		binary.LittleEndian.PutUint64(buf[:], math.Float64bits(x))
		h.Write(buf[:])
	}
	return h.Sum64()
}

// BlockHashWith is BlockHash of block with block[idx] replaced by v, computed
// without copying the block (the per-pitch voice key on the dispatch path).
func BlockHashWith(block []float64, idx int, v float64) uint64 {
	h := fnv.New64a()
	var buf [8]byte
	for i, x := range block {
		if i == idx {
			x = v
		}
		binary.LittleEndian.PutUint64(buf[:], math.Float64bits(x))
		h.Write(buf[:])
	}
	return h.Sum64()
}

// "sha256:" + 16 hex digits over the canonical JSON (fields in declaration order, map keys sorted): content only, never
// ID or Origin.
func ContentHash(d Doc) string {
	d.ID, d.Origin = "", ""
	b, err := json.Marshal(d)
	if err != nil {
		panic("instrument: ContentHash of an unmarshalable document: " + err.Error())
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:8])
}
