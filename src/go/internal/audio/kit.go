package audio

import (
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Maps each role to an instrument id (kick → "kick-808", …); applying it rebinds every row whose role matches, keeping
// the row's mix.
type Kit struct {
	ID          string                            `json:"id"`
	DisplayName string                            `json:"display_name"`
	Members     map[instrument.Role]instrument.ID `json:"members"` // role → instrument id
}

// Only the two methods Apply calls, so the audio package depends on no UI type.
type KitRowBinder interface {
	RowCount() int
	RowInstrument(idx int) instrument.ID
	RowRole(idx int) instrument.Role // explicit Role field; empty means "the instrument's own role"
	SetRowInstrument(idx int, newInstID instrument.ID)
}

var (
	kitRegMu sync.RWMutex
	kitReg   = map[string]Kit{}
	kitOrder []string
)

// An empty ID is a no-op; replacing an id keeps its position.
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

// In registration order.
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

// Only the row's instrument binding changes. Returns the rows actually rebound (zero is valid).
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

// Without changing anything. Rows the kit does not fill, or already on its member, are absent.
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

// A per-row Role overrides this; "" makes the kit walk skip the row.
func RoleForInstrument(instID instrument.ID) instrument.Role {
	if instID == "" {
		return instrument.RoleNone
	}
	if p, ok := builtinPropertyOf(instID); ok {
		return p.Role
	}
	return roleFromName(InstrumentDisplayName(instID))
}

// Order matters: "open-hihat" must hit the hat rule first.
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
