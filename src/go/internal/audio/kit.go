package audio

import (
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Kit is the Phase-6 "instrument set / drum kit" abstraction. A kit
// declares which instrument id each role maps to (kick → "kick-808",
// snare → "snare-fat", …); applying the kit reassigns every row whose
// resolved role matches a kit entry, preserving the row's mix (volume,
// EQ, insert effects). The data model lands now per the "code infra, no
// UI" scope; the Synth-tab / Mixer surfaces follow in a later round.
type Kit struct {
	ID          string                            `json:"id"`
	DisplayName string                            `json:"display_name"`
	Members     map[instrument.Role]instrument.ID `json:"members"` // role → instrument id
}

// KitRowBinder is the minimal contract Kit.Apply needs from the UI layer.
// internal/ui.DrumView satisfies it; tests pass a stub. Keeping the
// surface to the two methods Apply actually calls means the audio package
// stays free of any UI-domain type dependency.
type KitRowBinder interface {
	RowCount() int
	RowInstrument(idx int) instrument.ID
	RowRole(idx int) instrument.Role // explicit Role field; empty means "the instrument's own role"
	SetRowInstrument(idx int, newInstID instrument.ID)
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
		Members:     maps.Clone(k.Members),
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
		Members:     maps.Clone(k.Members),
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
			Members:     maps.Clone(k.Members),
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
// instrument id for that role. Row volume / EQ / insert
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
	plan := KitPlan(k, binder)
	for _, i := range slices.Sorted(maps.Keys(plan)) {
		binder.SetRowInstrument(i, plan[i])
	}
	rebound := len(plan)
	hooks.PublishKind(hooks.EventKitApplied, hooks.KitPayload{
		KitID:       k.ID,
		DisplayName: k.DisplayName,
		Members:     maps.Clone(k.Members),
	})
	return rebound
}

// KitPlan is what applying k changes, without changing it: row index → the
// kit member that row will play (rows whose role the kit does not fill, or
// that already play its member, are absent).
func KitPlan(k Kit, binder KitRowBinder) map[int]instrument.ID {
	plan := map[int]instrument.ID{}
	for i := 0; i < binder.RowCount(); i++ {
		curInst := binder.RowInstrument(i)
		role := binder.RowRole(i)
		if role == instrument.RoleNone {
			role = RoleForInstrument(curInst)
		}
		if role == instrument.RoleNone {
			continue
		}
		if newInst, ok := k.Members[role]; ok && newInst != "" && newInst != curInst {
			plan[i] = newInst
		}
	}
	return plan
}

// RoleForInstrument returns the kit role an instrument plays: a shipped
// instrument's own role (builtinProperties; an instance "x--N" has x's),
// else the role its name suggests for an instrument the user named, else ""
// (the kit walk skips it). An explicit per-row Role overrides this.
func RoleForInstrument(instID instrument.ID) instrument.Role {
	if instID == "" {
		return instrument.RoleNone
	}
	if p, ok := builtinPropertyOf(instID); ok {
		return p.Role
	}
	return roleFromName(InstrumentDisplayName(instID))
}

// roleFromName infers the kit role of an instrument the user named, by
// case-insensitive substring. Order matters: "open-hihat" must hit the hat
// rule before any other.
func roleFromName(name string) instrument.Role {
	lc := strings.ToLower(name)
	switch {
	case strings.Contains(lc, "hihat"), strings.Contains(lc, "hi-hat"), strings.Contains(lc, "hat"):
		return instrument.RoleHat
	case strings.Contains(lc, "kick"):
		return instrument.RoleKick
	case strings.Contains(lc, "snare"), strings.Contains(lc, "rimshot"), strings.Contains(lc, "sidestick"):
		return instrument.RoleSnare
	case strings.Contains(lc, "tom"):
		return instrument.RoleTom
	case strings.Contains(lc, "clap"):
		return instrument.RoleClap
	case strings.Contains(lc, "cowbell"):
		return instrument.RoleCowbell
	case strings.Contains(lc, "ride"):
		return instrument.RoleRide
	case strings.Contains(lc, "crash"):
		return instrument.RoleCrash
	case strings.Contains(lc, "shaker"):
		return instrument.RoleShaker
	case strings.Contains(lc, "bass"):
		return instrument.RoleBass
	}
	return instrument.RoleNone
}
