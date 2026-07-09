package audio

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

// TestDrumGenDefaultsMatchCSentinels parses src/c/modular_stages.c and asserts
// every kp_get fallback literal for the drum-gen curated knobs equals the
// declarative Go table (drum_gen_defaults.go). The C literal is the runtime
// fallback (byte-identity: values must NOT round-trip the float32 ABI); this
// guard makes the Go table the enforced source of truth — change a default in
// C without the table (or vice versa) and this fails.
//
// Occurrence order in the C file is load-bearing and pinned here: snare
// variant==1 branch, then ==2, then the else (variant 0) branch, then the clap
// voice; cymbal branches 0..5 in file order. The tom fallbacks are per-variant
// tables (sweepDef[3] etc.), parsed separately.
func TestDrumGenDefaultsMatchCSentinels(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "c", "modular_stages.c"))
	if err != nil {
		t.Fatalf("read modular_stages.c: %v", err)
	}
	text := string(src)

	// --- kp_get scalar fallbacks: field name -> ordered occurrence list.
	// Narrowed to the exact snare/clap/cymbal field-name alternation so this
	// does NOT accidentally capture gen_wave (shared across every source, not
	// owned by this table) or gen_snare_variant / gen_cym_variant (read via
	// lrintf, never kp_get).
	kpRe := regexp.MustCompile(`kp_get\(p->(gen_snare_(?:tone2|tune|tone_d|noise_d|tail_d|tone_m|noise_m|wire_m|attack)|gen_cym_(?:tune|env_fast|env_tail|tone_m|noise_m|noise_d))\[k\],\s*(-?[0-9.]+)\)`)
	occ := map[string][]float64{}
	for _, m := range kpRe.FindAllStringSubmatch(text, -1) {
		v, err := strconv.ParseFloat(m[2], 64)
		if err != nil {
			t.Fatalf("unparsable literal %q for %s", m[2], m[1])
		}
		occ[m[1]] = append(occ[m[1]], v)
	}

	// Expected occurrence order per snare/clap field: variant1, variant2,
	// variant0, clap (fields absent from a branch simply skip it).
	snareOrder := [][2]any{{"snare", 1}, {"snare", 2}, {"snare", 0}, {"clap", 0}}
	for _, field := range []string{
		"gen_snare_tone2", "gen_snare_tune", "gen_snare_tone_d", "gen_snare_noise_d",
		"gen_snare_tail_d", "gen_snare_tone_m", "gen_snare_noise_m", "gen_snare_wire_m",
		"gen_snare_attack",
	} {
		var want []float64
		var wantAt []string
		for _, fv := range snareOrder {
			fam, variant := fv[0].(string), fv[1].(int)
			if v, ok := drumGenDefaults[fam][variant][field]; ok {
				want = append(want, v)
				wantAt = append(wantAt, fmt.Sprintf("%s/v%d", fam, variant))
			}
		}
		assertOccurrences(t, field, occ[field], want, wantAt)
	}

	// Cymbal: branches appear in variant order 0..5 in the file.
	for _, field := range []string{
		"gen_cym_tune", "gen_cym_env_fast", "gen_cym_env_tail",
		"gen_cym_tone_m", "gen_cym_noise_m", "gen_cym_noise_d",
	} {
		var want []float64
		var wantAt []string
		for variant := range 6 {
			if v, ok := drumGenDefaults["cymbal"][variant][field]; ok {
				want = append(want, v)
				wantAt = append(wantAt, fmt.Sprintf("cymbal/v%d", variant))
			}
		}
		assertOccurrences(t, field, occ[field], want, wantAt)
	}

	// --- Tom: per-variant static tables `static const double sweepDef[3] = { a, b, c };`
	tomTables := map[string]string{
		"sweepDef": "gen_tom_sweep", "ringDef": "gen_tom_ring",
		"o1Def": "gen_tom_o1", "o2Def": "gen_tom_o2",
		"stickDef": "gen_tom_stick", "roomDef": "gen_tom_room",
	}
	for cName, field := range tomTables {
		re := regexp.MustCompile(cName + `\[3\]\s*=\s*\{\s*(-?[0-9.]+),\s*(-?[0-9.]+),\s*(-?[0-9.]+)\s*\}`)
		m := re.FindStringSubmatch(text)
		if m == nil {
			t.Errorf("tom table %s not found in modular_stages.c", cName)
			continue
		}
		for variant := range 3 {
			got, _ := strconv.ParseFloat(m[variant+1], 64)
			want := drumGenDefaults["tom"][variant][field]
			if got != want {
				t.Errorf("tom/v%d %s: C literal %v != table %v", variant, field, got, want)
			}
		}
	}
}

func assertOccurrences(t *testing.T, field string, got, want []float64, wantAt []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: %d kp_get occurrences in C, table expects %d (%v) — branch added/removed without updating drum_gen_defaults.go", field, len(got), len(want), wantAt)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s occurrence %d (%s): C literal %v != table %v", field, i, wantAt[i], got[i], want[i])
		}
	}
}

// legacyLitToGen maps the legacy per-recipe knob names (the lit maps in
// *_modular_push.go) to the C-side gen field names of the declarative table.
var legacyLitToGen = map[string]string{
	"snare_tone2_freq": "gen_snare_tone2", "snare_noise_tune": "gen_snare_tune",
	"snare_tone_decay": "gen_snare_tone_d", "snare_noise_decay": "gen_snare_noise_d",
	"snare_tail_decay": "gen_snare_tail_d", "snare_tone_mix": "gen_snare_tone_m",
	"snare_noise_mix": "gen_snare_noise_m", "snare_wire_mix": "gen_snare_wire_m",
	"snare_attack": "gen_snare_attack",
	"tom_sweep_rate": "gen_tom_sweep", "tom_ring_rate": "gen_tom_ring",
	"tom_o1_gain": "gen_tom_o1", "tom_o2_gain": "gen_tom_o2",
	"tom_stick": "gen_tom_stick", "tom_room": "gen_tom_room",
	"cym_tune": "gen_cym_tune", "cym_env_fast": "gen_cym_env_fast",
	"cym_env_tail": "gen_cym_env_tail", "cym_tone_mix": "gen_cym_tone_m",
	"cym_noise_mix": "gen_cym_noise_m", "cym_noise_decay": "gen_cym_noise_d",
}

// knownStaleSnareSpecLits documents any KNOWN divergence between
// snareVariantSpecs' lit fallback map (snare_modular_push.go) and the
// declarative table. Currently EMPTY: the FAT-BOTTOM snare retune's push-lit
// drift (drum-snare all 9 curated fields) was fixed by syncing
// snare_modular_push.go, and the stray rimshot tone-mix bump (1.0→1.12,
// never mirrored in synth_recipe_wired.go or the push lits) was reverted in
// modular_stages.c — all four copies agree again.
//
// RATCHET: this is not a "silence and forget" list. For every (recipeID,
// legacy-field) pair in this map, TestDrumGenSpecLitsMatchDeclarativeTable
// asserts the spec lit is STILL stale (still differs from the declarative
// table). If a fix in snare_modular_push.go makes the values match, the test
// FAILS with "exception entry is stale" — forcing the entry's removal instead
// of letting it silently rot as a permanent skip. The list must shrink,
// never grow.
var knownStaleSnareSpecLits = map[string]map[string]bool{}

// TestDrumGenSpecLitsMatchDeclarativeTable keeps the THIRD copy of these
// values (the per-recipe spec lit maps in snare_modular_push.go /
// tom_modular_push.go / cymbal_modular_push.go that feed the browser-push
// fallback literals) in sync with the declarative table, so there is exactly
// one place a default can change — modulo the documented pre-existing
// exceptions above, which are themselves ratcheted (see
// knownStaleSnareSpecLits doc comment): an excepted pair must still be stale,
// or the test fails and demands the entry be removed.
func TestDrumGenSpecLitsMatchDeclarativeTable(t *testing.T) {
	check := func(recipeID, family string, variant int, lits map[string]float64) {
		t.Helper()
		for legacy, v := range lits {
			gen, ok := legacyLitToGen[legacy]
			if !ok {
				continue // knob with no C kp_get fallback (e.g. wave) — table doesn't own it
			}
			want, ok := drumGenDefaults[family][variant][gen]
			if !ok {
				t.Errorf("%s/v%d: spec lit %s (%v) has no declarative-table entry %s", family, variant, legacy, v, gen)
				continue
			}
			if knownStaleSnareSpecLits[recipeID][legacy] {
				if v == want {
					t.Errorf("%s (%s/v%d) %s: exception entry is stale — the drift was fixed, remove the entry from knownStaleSnareSpecLits", recipeID, family, variant, legacy)
				}
				continue // documented pre-existing drift, and confirmed still-stale above
			}
			if v != want {
				t.Errorf("%s (%s/v%d) %s: spec lit %v != declarative table %v", recipeID, family, variant, legacy, v, want)
			}
		}
	}
	// snareVariantSpecs' discriminator is the C source==7 gen_snare_variant
	// (0 snare / 1 rimshot / 2 sidestick); variant 3 is the separate source==8
	// clap voice, which reuses the snare field names but is a distinct family
	// (fam="clap", variant 0) in the declarative table.
	for recipeID, spec := range snareVariantSpecs {
		fam := "snare"
		variant := int(spec.variant)
		if spec.variant == 3 {
			fam = "clap"
			variant = 0
		}
		check(recipeID, fam, variant, spec.lit)
	}
	for recipeID, spec := range tomVariantSpecs {
		check(recipeID, "tom", int(spec.variant), spec.lit)
	}
	// cymbalVariantSpecs' discriminator matches modular_gen_slot_cymbal's own
	// gen_cym_variant switch 1:1 (0=hihat 1=open-hihat 2=cowbell 3=shaker
	// 4=ride 5=crash, per modular_stages.c:1523-1528 and cymbal_modular_push.go
	// recipe IDs drum-hihat/drum-open-hihat/drum-cowbell/drum-shaker/drum-ride/
	// drum-crash) — no reconciliation needed, values verified identical to the
	// C branch literals directly.
	for recipeID, spec := range cymbalVariantSpecs {
		check(recipeID, "cymbal", int(spec.variant), spec.lit)
	}
}
