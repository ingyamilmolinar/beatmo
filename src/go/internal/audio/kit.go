package audio

import (
	"strings"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// Kit is the Phase-6 "instrument set / drum kit" abstraction. A kit
// declares which instrument id each role maps to (kick → "kick-808",
// snare → "snare-fat", …); applying the kit reassigns every row whose
// resolved role matches a kit entry, preserving the row's mix (volume,
// pan, sends, EQ, insert effects). The data model lands now per the
// "code infra, no UI" scope; the Synth-tab / Mixer surfaces follow in a
// later round.
//
// InsertChain is reserved for the future kit-bus: a shared insert
// effects chain that processes the sum of audio from rows currently
// bound to kit members before master. The field round-trips through
// JSON (so kits authored today survive the rollout) but the audio-graph
// routing — extending internal/audio/send_effects.go with a third bus
// class — is deferred.
//
// INERT TODAY. Until kit-bus lands, ApplyKit ignores this field and
// the kit only rebinds row instruments. Callers populating it should
// expect zero audible effect; the contract is "store the configuration
// so it isn't lost across a save/load cycle." TestKit_InsertChainIsInert
// is the regression guard.
type Kit struct {
	ID          string            `json:"id"`
	DisplayName string            `json:"display_name"`
	Members     map[string]string `json:"members"`                // role → instrument id
	InsertChain []EffectSlot      `json:"insert_chain,omitempty"` // INERT — see field doc
}

// KitRowBinder is the minimal contract Kit.Apply needs from the UI layer.
// internal/ui.DrumView satisfies it; tests pass a stub. Keeping the
// surface to the two methods Apply actually calls means the audio package
// stays free of any UI-domain type dependency.
type KitRowBinder interface {
	RowCount() int
	RowInstrument(idx int) string
	RowRole(idx int) string // explicit Role field; empty means "fall back to heuristic"
	SetRowInstrument(idx int, newInstID string)
}

// Kit registry — process-global; mirrors the recipe registry shape so
// kit lifecycle events can flow through the same hooks bus.
var (
	kitRegMu sync.RWMutex
	kitReg   = map[string]Kit{}
	kitOrder []string
)

// RegisterKit upserts a kit. Empty ID is a no-op. Replacing an existing
// id preserves insertion order. Safe to call concurrently with
// ApplyKit / KitsForExport (mutual exclusion via kitRegMu).
func RegisterKit(k Kit) {
	if k.ID == "" {
		return
	}
	kitRegMu.Lock()
	defer kitRegMu.Unlock()
	if _, exists := kitReg[k.ID]; !exists {
		kitOrder = append(kitOrder, k.ID)
	}
	cp := Kit{
		ID:          k.ID,
		DisplayName: k.DisplayName,
		Members:     copyStringMap(k.Members),
		InsertChain: append([]EffectSlot(nil), k.InsertChain...),
	}
	kitReg[k.ID] = cp
}

// UnregisterKit removes a kit by id. No-op if not present.
func UnregisterKit(id string) {
	kitRegMu.Lock()
	defer kitRegMu.Unlock()
	if _, ok := kitReg[id]; !ok {
		return
	}
	delete(kitReg, id)
	for i, x := range kitOrder {
		if x == id {
			kitOrder = append(kitOrder[:i], kitOrder[i+1:]...)
			break
		}
	}
}

// KitForID returns a copy of the registered kit (or zero-value + false).
func KitForID(id string) (Kit, bool) {
	kitRegMu.RLock()
	defer kitRegMu.RUnlock()
	k, ok := kitReg[id]
	if !ok {
		return Kit{}, false
	}
	return Kit{
		ID:          k.ID,
		DisplayName: k.DisplayName,
		Members:     copyStringMap(k.Members),
		InsertChain: append([]EffectSlot(nil), k.InsertChain...),
	}, true
}

// KitsForExport returns every registered kit in registration order.
// Used by internal/ui/export.go to embed kits in the project file.
func KitsForExport() []Kit {
	kitRegMu.RLock()
	defer kitRegMu.RUnlock()
	out := make([]Kit, 0, len(kitOrder))
	for _, id := range kitOrder {
		k := kitReg[id]
		out = append(out, Kit{
			ID:          k.ID,
			DisplayName: k.DisplayName,
			Members:     copyStringMap(k.Members),
			InsertChain: append([]EffectSlot(nil), k.InsertChain...),
		})
	}
	return out
}

// ResetKitsForTest wipes the kit registry. Test-only helper.
func ResetKitsForTest() {
	kitRegMu.Lock()
	defer kitRegMu.Unlock()
	kitReg = map[string]Kit{}
	kitOrder = nil
}

// ApplyKit walks the binder's rows and, for each row whose role
// resolves to a kit member, calls SetRowInstrument with the kit's
// instrument id for that role. Row volume / pan / sends / EQ / insert
// effects are untouched — only the row's instrument binding changes.
//
// After the walk, publishes hooks.EventKitApplied with the kit id +
// the members map snapshot so external subscribers (eventlogger,
// scripting) see the change. Returns the number of rows actually
// rebound (zero is valid: a kit with no matching roles is a no-op).
func ApplyKit(k Kit, binder KitRowBinder) int {
	if binder == nil || len(k.Members) == 0 {
		return 0
	}
	rebound := 0
	for i := 0; i < binder.RowCount(); i++ {
		curInst := binder.RowInstrument(i)
		role := binder.RowRole(i)
		if role == "" {
			role = roleHeuristic(curInst)
		}
		if role == "" {
			continue
		}
		newInst, ok := k.Members[role]
		if !ok || newInst == "" || newInst == curInst {
			continue
		}
		binder.SetRowInstrument(i, newInst)
		rebound++
	}
	hooks.PublishKind(hooks.EventKitApplied, hooks.KitPayload{
		KitID:       k.ID,
		DisplayName: k.DisplayName,
		Members:     copyStringMap(k.Members),
	})
	return rebound
}

// roleHeuristic infers a role name from an instrument id when the row
// doesn't carry an explicit Role override. The map covers the shipped
// instrument families; a user instrument id that doesn't match any
// pattern returns "" so the kit walk skips it.
//
// Patterns are case-insensitive substring matches. Order matters: more
// specific suffixes (open-hihat → hat) come before generic ones (snare).
func roleHeuristic(instID string) string {
	if instID == "" {
		return ""
	}
	lc := strings.ToLower(instID)
	switch {
	case strings.Contains(lc, "open-hihat"), strings.Contains(lc, "hihat"), strings.Contains(lc, "hi-hat"), strings.Contains(lc, "hat"):
		return "hat"
	case strings.Contains(lc, "kick"):
		return "kick"
	case strings.Contains(lc, "snare"), strings.Contains(lc, "rimshot"), strings.Contains(lc, "sidestick"):
		return "snare"
	case strings.Contains(lc, "tom"):
		return "tom"
	case strings.Contains(lc, "clap"):
		return "clap"
	case strings.Contains(lc, "cowbell"):
		return "cowbell"
	case strings.Contains(lc, "ride"):
		return "ride"
	case strings.Contains(lc, "crash"):
		return "crash"
	case strings.Contains(lc, "shaker"):
		return "shaker"
	case strings.Contains(lc, "bass"):
		return "bass"
	}
	return ""
}

// RoleForInstrument is the public companion to the heuristic. Returns
// the inferred role for an instrument id, or empty string when no
// pattern matches. UI / JSON callers use this to populate a default
// DrumRow.Role on row creation; explicit per-row Role overrides are
// preferred when the user has tagged a row.
func RoleForInstrument(instID string) string {
	return roleHeuristic(instID)
}

// copyStringMap returns a defensive copy of a string→string map. Used
// when handing kit data across the registry boundary so callers can't
// mutate the registered state through the returned reference.
func copyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
