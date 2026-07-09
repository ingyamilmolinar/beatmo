package audio

// drum_gen_defaults.go is the DECLARATIVE source of truth for the drum-gen
// curated per-variant defaults (spec P3: "the value lives in one declarative
// table, not a C literal"). The C voices still carry the same values as kp_get
// fallback literals — deliberately: passing these through the float32 param ABI
// would change their double value ((double)float32(1.12) != 1.12) and break
// byte-identity. Instead this table is the authority and
// TestDrumGenDefaultsMatchCSentinels parses modular_stages.c and fails on any
// divergence, so every default change starts here and the guard forces the C
// sync. Structural per-variant tables (fStartMul, baseFreq partial banks, …)
// are voice character without knobs and stay C-owned.

// drumGenDefaults: family → variant → C param field name → default value.
// Families: "snare" (variants 0 snare / 1 rimshot / 2 sidestick), "clap"
// (single variant, reuses the snare fields), "cymbal" (0 hihat / 1 open-hihat /
// 2 cowbell / 3 shaker / 4 ride / 5 crash — per modular_gen_slot_cymbal's own
// gen_cym_variant doc comment), "tom" (0/1/2).
var drumGenDefaults = map[string]map[int]map[string]float64{
	"snare": {
		0: {
			"gen_snare_tone2": 280.0, "gen_snare_tune": 0.55,
			"gen_snare_tone_d": 46.0, "gen_snare_noise_d": 7.0,
			"gen_snare_tail_d": 10.0, "gen_snare_tone_m": 1.12,
			"gen_snare_noise_m": 0.56, "gen_snare_wire_m": 0.8,
			"gen_snare_attack": 0.5,
		},
		1: {
			"gen_snare_tone2": 1050.0, "gen_snare_tune": 1.0,
			"gen_snare_tone_d": 40.0, "gen_snare_noise_d": 200.0,
			"gen_snare_tone_m": 1.0, "gen_snare_noise_m": 0.7,
			"gen_snare_attack": 2.0,
		},
		2: {
			"gen_snare_tone2": 1200.0, "gen_snare_tune": 1.0,
			"gen_snare_tone_d": 100.0, "gen_snare_noise_d": 150.0,
			"gen_snare_tone_m": 0.5, "gen_snare_noise_m": 0.5,
			"gen_snare_attack": 1.0,
		},
	},
	"clap": {
		0: {
			"gen_snare_tune": 1.0, "gen_snare_noise_d": 7.0,
			"gen_snare_tail_d": 4.0, "gen_snare_noise_m": 0.15,
			"gen_snare_attack": 110.0,
		},
	},
	"cymbal": {
		0: {"gen_cym_tune": 1.0, "gen_cym_env_fast": 180.0, "gen_cym_env_tail": 35.0, "gen_cym_tone_m": 0.85, "gen_cym_noise_m": 0.45, "gen_cym_noise_d": 100.0},
		1: {"gen_cym_tune": 1.0, "gen_cym_env_fast": 120.0, "gen_cym_env_tail": 22.0, "gen_cym_tone_m": 0.7, "gen_cym_noise_m": 0.9, "gen_cym_noise_d": 18.0},
		2: {"gen_cym_tune": 1.0, "gen_cym_env_fast": 260.0, "gen_cym_env_tail": 9.0, "gen_cym_tone_m": 1.0, "gen_cym_noise_m": 0.55, "gen_cym_noise_d": 60.0},
		3: {"gen_cym_tune": 1.0, "gen_cym_env_fast": 200.0, "gen_cym_env_tail": 25.0, "gen_cym_tone_m": 0.6, "gen_cym_noise_m": 0.5},
		4: {"gen_cym_tune": 1.0, "gen_cym_env_fast": 40.0, "gen_cym_env_tail": 8.0, "gen_cym_tone_m": 1.0, "gen_cym_noise_m": 0.3, "gen_cym_noise_d": 12.0},
		5: {"gen_cym_tune": 1.0, "gen_cym_env_fast": 10.0, "gen_cym_env_tail": 3.0, "gen_cym_tone_m": 1.0, "gen_cym_noise_m": 0.35, "gen_cym_noise_d": 8.0},
	},
	"tom": {
		0: {"gen_tom_sweep": 18.0, "gen_tom_ring": 2.8, "gen_tom_o1": 0.5, "gen_tom_o2": 0.25, "gen_tom_stick": 0.35, "gen_tom_room": 0.08},
		1: {"gen_tom_sweep": 22.0, "gen_tom_ring": 3.5, "gen_tom_o1": 0.55, "gen_tom_o2": 0.28, "gen_tom_stick": 0.38, "gen_tom_room": 0.06},
		2: {"gen_tom_sweep": 14.0, "gen_tom_ring": 2.2, "gen_tom_o1": 0.45, "gen_tom_o2": 0.22, "gen_tom_stick": 0.32, "gen_tom_room": 0.10},
	},
}
