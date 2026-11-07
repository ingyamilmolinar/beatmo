GO ?= $(shell if [ -x $(CURDIR)/.tools/go/bin/go ]; then echo $(CURDIR)/.tools/go/bin/go; else echo go; fi)
WASM_OUT = ../js/main.wasm
MA_JS = src/js/drums.single.js
C_LIB = build/libdrums.a
C_SRC = src/c/drums.c src/c/miniaudio.c src/c/fmsynth.c src/c/insert_fx.c src/c/fx_host.c src/c/mix_dsp.c src/c/wavetable.c src/c/adsr.c src/c/noise.c src/c/modular.c src/c/modular_stages.c src/c/modal_bank.c src/c/gcov_flush.c
# Test-only oracle (real ma_noise), linked into the NATIVE archive for noise_ma_parity_test.go and kept out of C_SRC
# so the emcc WASM build never ships it.
C_SRC_TEST = src/c/noise_ma_oracle.c
C_HDRS = src/c/once.h src/c/drums.h src/c/miniaudio.h src/c/fmsynth.h src/c/synth_post.h src/c/modular_params.gen.h src/c/synth_dsp_primitives.h src/c/insert_fx.h src/c/fx_host.h src/c/mix_dsp.h src/c/wavetable.h src/c/adsr.h src/c/noise.h src/c/modular.h src/c/modular_stages.h src/c/modal_bank.h

# Derived from the Go schema, so it is regenerated whenever the schema changes: a stale ABI makes audio.js allocate an
# undersized param block and the C voice reads uninitialised fields.
SYNTH_ABI_JS = src/js/synth_param_abi.gen.js

# Opt-in test logging: TEST_LOG=1 [TEST_LOG_LEVEL=TRACE] make test; unset keeps passing tests silent.
ifdef TEST_LOG
LOG_ENV := BEATMO_TEST_LOG=$(TEST_LOG)
ifdef TEST_LOG_LEVEL
LOG_ENV += BEATMO_TEST_LOG_LEVEL=$(TEST_LOG_LEVEL)
endif
endif

$(MA_JS): $(C_SRC) $(C_HDRS)
	@if command -v emcc >/dev/null 2>&1; then \
		emcc $(C_SRC) -O3 -DMA_ENABLE_MP3 -sWASM=1 -sSINGLE_FILE=1 -sENVIRONMENT=web -sASSERTIONS=0 -sALLOW_MEMORY_GROWTH=0 -sSTACK_SIZE=1048576 -sEXPORTED_FUNCTIONS='[_render_modular_p,_modular_param_count,_ifx_param_names,_ifx_create,_ifx_run,_ifx_set,_ifx_clear,_ifx_destroy,_eq_create,_eq_update,_eq_run,_eq_clear,_eq_destroy,_soft_clip_run,_malloc,_free]' -sEXPORTED_RUNTIME_METHODS='["ccall","HEAPU8","HEAPF32","HEAPF64"]' -sMODULARIZE=1 -sEXPORT_ES6=1 -o $(MA_JS); \
	else \
		if [ -f "$(MA_JS)" ]; then \
			echo "[INFO] emcc not found; using existing $(MA_JS)"; \
		else \
			echo "[WARN] emcc not found and $(MA_JS) missing; skipping MA build"; \
		fi; \
	fi

# One rule per object, never one recipe gated on "any source newer than the .a": that lost timestamp races and linked
# stale C.
C_OBJS = build/miniaudio.o build/drums.o build/fmsynth.o build/insert_fx.o build/fx_host.o build/mix_dsp.o build/wavetable.o build/adsr.o build/noise.o build/noise_ma_oracle.o build/modular.o build/modular_stages.o build/modal_bank.o build/gcov_flush.o

# miniaudio + the two files that #include it need -DMA_ENABLE_MP3; the rest don't.
MA_MP3_OBJS = build/miniaudio.o build/drums.o build/noise_ma_oracle.o
$(MA_MP3_OBJS): MP3FLAG = -DMA_ENABLE_MP3

build:
	mkdir -p build

# Every object depends on ALL headers (conservative but correct: any header edit
# rebuilds everything that could include it — no missed transitive recompiles).
build/%.o: src/c/%.c $(C_HDRS) | build
	$(CC) -O2 $(MP3FLAG) -c $< -o $@

$(C_LIB): $(C_OBJS)
	ar rcs $@ $(C_OBJS)
	@# The archive was (re)built, so a C source/header changed. Go links this static
	@# lib via cgo LDFLAGS but keys its build cache on the .go sources + the LDFLAGS
	@# STRING, NOT the .a's content — so it would silently reuse a stale cgo link.
	@# Clear the Go build cache here so every downstream `go build`/`go test` relinks
	@# against the freshly-built libdrums.a. (Without this, C edits don't take effect.)
	$(GO) clean -cache

clean:
	rm -f build/drums.o build/fmsynth.o build/insert_fx.o build/fx_host.o build/mix_dsp.o build/wavetable.o build/adsr.o build/noise.o build/noise_ma_oracle.o build/modular.o build/modular_stages.o build/modal_bank.o build/gcov_flush.o
	rm -f $(C_LIB)
	rm -f $(MA_JS)

# Phony, not mtime-gated: a stale synth_param_abi.gen.js newer than its schema would never regenerate, and a stale ABI
# silently silences re-voiced instruments. The Go run is cheap.
.PHONY: gen-synth-abi
gen-synth-abi:
	@# The JS ABI and the C struct header come from ONE table
	@# (internal/audio/modular_fields.go). Built through the stub (-tags test)
	@# path: no cgo, so it never needs the header it is generating.
	cd src/go && $(GO) run -tags test -modfile=go.test.mod ./cmd/gen-synth-abi -o ../js/synth_param_abi.gen.js
	cd src/go && $(GO) run -tags test -modfile=go.test.mod ./cmd/gen-synth-abi -c -o ../c/modular_params.gen.h

# Phony for the same reason as gen-synth-abi: a stale chain_spec.gen.js lets the browser's chain drift from desktop's.
.PHONY: gen-chain-spec
gen-chain-spec:
	cd src/go && $(GO) run ./cmd/gen-chain-spec > ../js/chain_spec.gen.js

# Rebuilds the embedded instrument pack from the readable documents in internal/assets/instruments.
.PHONY: asset-pack
asset-pack: ## Rebuild the embedded instrument pack from internal/assets/instruments
	cd src/go && $(GO) run ./cmd/gen-asset-pack

# Subsets the Inter faces embedded in main.wasm (every byte is downloaded by every visitor). Not run by make wasm: the
# subsets are committed and regenerating needs node + subset-font; re-run only when the TTFs or KEEP_RANGES change.
.PHONY: gen-fonts
gen-fonts:
	npm install --no-audit --no-fund --silent subset-font
	node scripts/subset_fonts.mjs
	@echo "regenerated src/go/internal/assets/fonts/*.subset.ttf -- run the assets tests"

.PHONY: measure-loudness
measure-loudness: ## Measure each instrument's loudness amplitude into its shipped document, then rebuild the pack
	cd src/go && ../../.tools/go/bin/go run ./cmd/measure-loudness -write
	$(MAKE) asset-pack
	@echo "updated render.amplitude in internal/assets/instruments/*.json + rebuilt the pack"

# ─── Synth Matching / Fingerprinting ─────────────────────────────────────────

.PHONY: synth-match synth-fingerprint fetch-refs refs-doctor refs-families refs-adopt refs-adopt-all refs-verify refs-check scores-verify scores-notice templates ab ab-list ab-all ab-notes wav-match wav-match-map wav-match-nuisance wav-match-validate wav-match-calibrate ab-match probe-sounds refs-validate song-timeline song-render song-compare

synth-match: ## Tune a synth toward a reference WAV (INST=, REF=, [BUDGET=], [RESTARTS=], [DRY_RUN=1])
	cd src/go && ../../.tools/go/bin/go run ./cmd/synth-match -inst $(INST) -ref $(REF) \
		$(if $(BUDGET),-budget $(BUDGET),) $(if $(RESTARTS),-restarts $(RESTARTS),) $(if $(DRY_RUN),-dry-run,)

synth-fingerprint: ## Print the analysis fingerprint of a WAV (FILE=)
	cd src/go && ../../.tools/go/bin/go run ./cmd/synth-analyze ref -file $(FILE)

fetch-refs: ## Fetch reference audio into the corpus ([TIER=instrument|song], [ID=], [INSTRUMENT=], [ONLY_MISSING=1], [FORCE=1])
	cd src/go && ../../.tools/go/bin/go run ./cmd/refs fetch \
		$(if $(TIER),-tier $(TIER),) $(if $(ID),-id $(ID),) \
		$(if $(INSTRUMENT),-instrument $(INSTRUMENT),) \
		$(if $(ONLY_MISSING),-only-missing,) $(if $(FORCE),-force,)

refs-doctor: ## Report reference coverage for every instrument and template
	cd src/go && ../../.tools/go/bin/go run ./cmd/refs doctor

refs-families: ## List every instrument family, the reference its members are auditioned against, and the last ear verdict (INST=<family-or-instrument> for one)
	cd src/go && ../../.tools/go/bin/go run ./cmd/refs families \
		$(if $(INST),-instrument '$(INST)',)

# The file is not copied into the tree: it is converted like every catalogued reference into $BEATMO_WAV_REFS and only
# its provenance is committed. Reference audio never ships, so any recording will do.
refs-adopt: ## Point an instrument at your own WAV (WAV=, [INST=] defaults to the filename, [NOTE=], [ID=], [F0=] pins the fundamental past a bad detection, [START_SEC=], [MAX_SEC=])
	@test -n "$(WAV)" || { echo "refs-adopt: WAV is required: a filename inside $(BEATMO_WAV_REFS)/sources, or any path -- or use \`make refs-adopt-all\`" >&2; exit 1; }
	cd src/go && ../../.tools/go/bin/go run ./cmd/refs adopt \
		$(if $(INST),-instrument '$(INST)',) -wav '$(WAV)' $(if $(NOTE),-note '$(NOTE)',) \
		$(if $(ID),-id '$(ID)',) $(if $(F0),-f0 $(F0),) \
		$(if $(START_SEC),-start-sec $(START_SEC),) $(if $(MAX_SEC),-max-sec $(MAX_SEC),)

# Bulk form: name each file after the instrument it is for (snare-lofi.wav,
# kick-deep_take3.wav), drop them all in, run this once.
refs-adopt-all: ## Adopt every resolvable WAV waiting in $(BEATMO_WAV_REFS)/sources ([NOTE=], [START_SEC=], [MAX_SEC=])
	cd src/go && ../../.tools/go/bin/go run ./cmd/refs adopt -scan $(if $(NOTE),-note '$(NOTE)',) \
		$(if $(START_SEC),-start-sec $(START_SEC),) $(if $(MAX_SEC),-max-sec $(MAX_SEC),)

refs-verify: ## Hash every local reference against the catalogue
	cd src/go && ../../.tools/go/bin/go run ./cmd/refs verify

refs-check: ## Re-derive every corpus calibration (thresholds, organic range, f0 confidence, audition candidates) against the local refs and fail on drift ([CHECKS=a,b], [V=1]); never part of `go test`
	cd src/go && ../../.tools/go/bin/go run ./cmd/refs check \
		$(if $(CHECKS),-only $(CHECKS),) $(if $(V),-v,)

scores-verify: ## Re-fetch every reference SCORE upstream and check it against the catalogue hash
	scripts/verify_scores.sh $(ID)

scores-notice: ## Regenerate internal/scoreref/NOTICE-scores.md from the score catalogue
	cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod \
		./internal/scoreref/ -run TestNoticeIsUpToDate -update

templates: ## Regenerate the built-in template circuits from the committed reference scores
	cd src/go && ../../.tools/go/bin/go run ./cmd/gen-showcase

# Defaulted so make ab works out of the box, and exported so the go run subprocess sees it.
BEATMO_WAV_REFS ?= $(HOME)/.beatmo/wav-refs
export BEATMO_WAV_REFS
# Raw recordings to adopt go in $(BEATMO_WAV_REFS)/sources, invisible to lookups until adopted; the root holds only
# converted, catalogued audio. OPEN falls back to xdg-open where open is missing.
OPEN ?= $(shell command -v open >/dev/null 2>&1 && echo open || echo xdg-open)

ab: ## A/B listen: real reference vs Beatmo's synthesized version, played sequentially (INST=; see `make ab-list`)
	@test -n "$(INST)" || { echo "ab: INST is required, e.g. \`make ab INST=oboe\` -- see \`make ab-list\` for valid ids" >&2; exit 1; }
	@bash -c '\
	  set -e; \
	  PLAN="$$(cd src/go && ../../.tools/go/bin/go run ./cmd/refs compare -instrument $(INST) -sh)"; \
	  eval "$$PLAN"; \
	  echo "playing REAL then SYNTH, one after the other -- open/xdg-open hands off to a player and returns immediately, so we sleep the clip'"'"'s duration between the two"; \
	  echo "▶ REAL  $$BEATMO_AB_REF -> $$BEATMO_AB_REAL"; \
	  $(OPEN) "$$BEATMO_AB_REAL" >/dev/null 2>&1 || true; \
	  sleep "$$BEATMO_AB_DUR"; \
	  sleep 0.6; \
	  echo "▶ SYNTH $(INST) -> $$BEATMO_AB_SYNTH"; \
	  $(OPEN) "$$BEATMO_AB_SYNTH" >/dev/null 2>&1 || true'

# Go renders and prints an eval-able plan; the shell plays it (the same hand-off as ab).
probe-sounds: ## Play the isolated-property probe tones in order so you can name each property and where you first hear a difference (SET=<name> for one set; see `refs probe` for the list)
	@bash -c '\
	  set -e; \
	  PLAN="$$(cd src/go && ../../.tools/go/bin/go run ./cmd/refs probe $(if $(SET),-set $(SET),) -sh)"; \
	  eval "$$PLAN"; \
	  echo "playing $$BEATMO_PROBE_N probe clips in order -- all identical in pitch and loudness"; \
	  echo "say what word you would use at each end of a set, and the first step where you hear a clear difference"; \
	  i=1; \
	  while [ "$$i" -le "$$BEATMO_PROBE_N" ]; do \
	    nvar="BEATMO_PROBE$${i}_NAME"; pvar="BEATMO_PROBE$${i}_PATH"; \
	    echo "▶ $$i. $${!nvar}"; \
	    $(OPEN) "$${!pvar}" >/dev/null 2>&1 || true; \
	    sleep "$$BEATMO_PROBE_DUR"; \
	    sleep 0.5; \
	    i=$$((i+1)); \
	  done; \
	  echo "done -- `cd src/go && ../../.tools/go/bin/go run ./cmd/refs probe` prints the measured value for every step"'

ab-list: ## List every instrument comparable via `make ab INST=...`
	cd src/go && ../../.tools/go/bin/go run ./cmd/refs compare -list

# Shared by ab-all and ab-notes ($(1) progress label, $(2) refs audition flags). A PREBUILT binary: go run reports 1
# for any nonzero exit, and the loop tells replay (4) from quit (3) from done (0) by exit code. PLAN is assigned
# before its eval because eval "$$(cmd)" does not trip set -e when cmd fails.
define AUDITION_LOOP
	@cd src/go && ../../.tools/go/bin/go build -o /tmp/beatmo-refs-audition ./cmd/refs
	@bash -c '\
	  set -e; \
	  BIN=/tmp/beatmo-refs-audition; \
	  LABEL="$(1)"; \
	  cd src/go; \
	  SKIPPED=""; \
	  while true; do \
	    PLAN="$$($$BIN audition $(2) $(if $(FAMILIES),-families $(FAMILIES),) -skip "$$SKIPPED" $(if $(INST),-instrument $(INST),) -sh)"; \
	    eval "$$PLAN"; \
	    if [ -n "$$BEATMO_AB_DONE" ]; then \
	      echo "$$LABEL: no families left"; \
	      break; \
	    fi; \
	    while true; do \
	      echo "playing REFERENCE then each OPTION in turn -- open/xdg-open hands off to a player and returns immediately, so we sleep the clip'"'"'s duration between clips"; \
	      echo "all clips are level-matched to one common loudness, so judge timbre, not volume"; \
	      echo "▶ REFERENCE $$BEATMO_AB_FAMILY ($$BEATMO_AB_REF) -> $$BEATMO_AB_REAL"; \
	      $(OPEN) "$$BEATMO_AB_REAL" >/dev/null 2>&1 || true; \
	      sleep "$$BEATMO_AB_DUR"; \
	      sleep 0.6; \
	      i=1; \
	      while [ "$$i" -le "$$BEATMO_AB_N" ]; do \
	        optvar="BEATMO_AB_OPT$$i"; pathvar="BEATMO_AB_OPT$${i}_PATH"; \
	        opt="$${!optvar}"; optpath="$${!pathvar}"; \
	        echo "▶ OPTION $$i: $$opt"; \
	        $(OPEN) "$$optpath" >/dev/null 2>&1 || true; \
	        sleep "$$BEATMO_AB_DUR"; \
	        sleep 0.6; \
	        i=$$((i+1)); \
	      done; \
	      set +e; \
	      $$BIN audition $(2) $(if $(FAMILIES),-families $(FAMILIES),) -instrument "$$BEATMO_AB_FAMILY" -skip "$$SKIPPED"; \
	      rc=$$?; \
	      set -e; \
	      case "$$rc" in \
	        4) continue ;; \
	        3) echo "$$LABEL: quitting"; break 2 ;; \
	        0) break ;; \
	        *) exit "$$rc" ;; \
	      esac; \
	    done; \
	    SKIPPED="$$SKIPPED,$$BEATMO_AB_FAMILY"; \
	    if [ -n "$(INST)" ]; then break; fi; \
	  done'
endef

RESUME ?= true
ab-all: ## Interactive ear-listening session over every instrument family: real reference vs every Beatmo synth option, one family at a time (answers -> src/go/internal/audio/testdata/ear_verdicts.jsonl; RESUME=false to start over, INST=<family> for just one, FAMILIES=a,b,c to re-audit a named subset)
	$(call AUDITION_LOOP,ab-all,-resume=$(RESUME))

# A second pass over ab-all's verdicts: replays a judged family and asks only WHY (refs audition -notes).
ONLY ?= n,p
REANNOTATE ?= false
ab-notes: ## Second pass over `make ab-all`: capture WHY a family sounds wrong as prose notes, without re-asking the verdict (answers -> src/go/internal/audio/testdata/ear_verdicts.jsonl; ONLY=n,p filters which verdict classes to revisit, REANNOTATE=true to revisit families that already have a note, INST=<family> for just one, FAMILIES=a,b,c for a named subset)
	$(call AUDITION_LOOP,ab-notes,-notes -only $(ONLY) $(if $(filter true,$(REANNOTATE)),-reannotate,))

# Which shipped instrument is closest to an uncatalogued recording (refs match; never touches catalogue.json). WAVS is
# a glob (or several, comma-separated) handed to Go unexpanded, so spaces and "(1)" in download names survive.
WAVS ?= $(HOME)/Downloads/*.wav
wav-match: ## Rank every shipped instrument against your own WAVs and write level-matched clips of the closest ones (WAVS='<glob>[,<glob>...]' default ~/Downloads/*.wav, [TOP=3], [FORCE=1] re-analyses)
	cd src/go && ../../.tools/go/bin/go run ./cmd/refs match -wavs '$(WAVS)' \
		$(if $(TOP),-top $(TOP),) $(if $(FORCE),-force,)

# The coverage map both ways round: recordings to voices, and every roster instrument as MATCHED / FIDELITY GAP /
# STAND-IN / UNCOVERED. Default set: your downloads plus every catalogued instrument reference.
MAP_WAVS ?= $(HOME)/Downloads/*.wav,catalogue
wav-match-map: ## Map recordings <-> instruments: matches, fidelity gaps, stand-ins and uncovered instruments -> $$BEATMO_WAV_REFS/matches/MAP.md (MAP_WAVS='<glob>[,<glob>|catalogue...]' default downloads + catalogued refs, [FORCE=1])
	cd src/go && ../../.tools/go/bin/go run ./cmd/refs match -map -wavs '$(MAP_WAVS)' $(if $(FORCE),-force,)

wav-match-nuisance: ## Re-play shipped voices with ONE nuisance each (pitch +/-5, a note cut short, room, reverb, bright/dark mic) and report whether the matcher still finds the voice ([CASES=a,b | all])
	cd src/go && ../../.tools/go/bin/go run ./cmd/refs match -nuisance $(if $(CASES),-cases $(CASES),)

wav-match-validate: ## Grade `refs match` on every catalogued reference (leave-one-out): own family #1 / top-3 / MRR and the broad KIND of the first-ranked voice
	cd src/go && ../../.tools/go/bin/go run ./cmd/refs match -validate

wav-match-calibrate: ## Re-derive the matcher's hubness calibration (src/go/cmd/refs/testdata/match_hubness.json) over the catalogued references -- after retuning or adding instruments; checked by `make refs-check`
	cd src/go && ../../.tools/go/bin/go run ./cmd/refs match -calibrate

# ONLY is honoured only when YOU set it: the Makefile's own ONLY ?= n,p (ab-notes' filter) would match no filename.
ab-match: ## Listen to each WAV next to its closest instrument and approve/reject it with notes (runs wav-match first; WAVS='<glob>', ONLY=<filename text>, RESUME=false re-asks answered files; answers -> src/go/internal/audio/testdata/wav_match_verdicts.jsonl)
	@cd src/go && ../../.tools/go/bin/go build -o /tmp/beatmo-refs-match ./cmd/refs
	@cd src/go && /tmp/beatmo-refs-match match -listen -wavs '$(WAVS)' -resume=$(RESUME) \
		$(if $(TOP),-top $(TOP),) $(if $(filter-out file,$(origin ONLY)),-only '$(ONLY)',)

ab-session: ## Audition round: reference + current + two candidates per family, rated and described from ONE word salad shared by every instrument (FAMILIES=a,b; DEEP=1 also rates the instrument's own words 1-5; resumes at the first family not yet reviewed for its current candidates, RESUME=false asks all again; CLONE=1 adds each family's WAV clone as candidate K with a y/n/p accept; answers -> src/go/internal/audio/testdata/ear_sessions.jsonl)
	@cd src/go && ../../.tools/go/bin/go build -o /tmp/beatmo-refs-audition ./cmd/refs
	@cd src/go && /tmp/beatmo-refs-audition session -resume=$(RESUME) $(if $(FAMILIES),-families $(FAMILIES),) $(if $(CLONE),-clone,) $(if $(DEEP),-deep,)

# ab-tuned plays each family exactly as refs tune scored it; tune-adopt ships a candidate only after the newest
# verdict on that exact doc_sha is y.
TUNE_WAVS ?= $(HOME)/Downloads/*.wav
tune: ## Essence-tune ONE instrument into a candidate document (INST=<id>[,<id>], [GENS=40] [SEED=1] [HARMONICS=N] [TUNE_WAVS=<glob>]; writes $$BEATMO_WAV_REFS/tuned/<inst>/)
	@test -n "$(INST)" || { echo "tune: INST is required (or \`make tune-picks\`)" >&2; exit 1; }
	@cd src/go && ../../.tools/go/bin/go build -o /tmp/beatmo-refs-tune ./cmd/refs
	@cd src/go && /tmp/beatmo-refs-tune tune -inst $(INST) -wavs '$(TUNE_WAVS)' \
		$(if $(GENS),-gens $(GENS),) $(if $(SEED),-seed $(SEED),) $(if $(HARMONICS),-add-harmonics $(HARMONICS),)

tune-picks: ## Essence-tune every curated pick (src/go/cmd/refs/testdata/tune_picks.json) into candidates, then listen with `make ab-tuned` ([PICKS=a,b] [GENS=40] [SEED=1])
	@cd src/go && ../../.tools/go/bin/go build -o /tmp/beatmo-refs-tune ./cmd/refs
	@cd src/go && /tmp/beatmo-refs-tune tune -picks $(if $(PICKS),-inst $(PICKS),) -wavs '$(TUNE_WAVS)' \
		$(if $(GENS),-gens $(GENS),) $(if $(SEED),-seed $(SEED),)

ab-tuned: ## A/B by ear: reference, then the CURRENT instrument, then its essence-tuned candidate T, per family; rate it, pick its words from the shared word salad, and y/n/p whether it ships ([FAMILIES=a,b] default every tuned candidate; DEEP=1 also rates the instrument's own words 1-5; resumes at the first family not yet reviewed for its current candidate, RESUME=false asks all again, a family in FAMILIES is always asked; answers are appended -> src/go/internal/audio/testdata/ear_sessions.jsonl)
	@cd src/go && ../../.tools/go/bin/go build -o /tmp/beatmo-refs-audition ./cmd/refs
	@cd src/go && /tmp/beatmo-refs-audition session -tuned -resume=$(RESUME) $(if $(FAMILIES),-families $(FAMILIES),) $(if $(DEEP),-deep,)

tune-adopt: ## Ship a tuned candidate into internal/assets/instruments — ONLY after `make ab-tuned FAMILIES=<inst>` recorded y for that exact candidate (INST=<instrument>)
	@test -n "$(INST)" || { echo "tune-adopt: INST is required" >&2; exit 1; }
	@cd src/go && ../../.tools/go/bin/go run ./cmd/clone adopt -kind tuned -inst $(INST)

clone: ## Clone a struck/modal reference into a candidate instrument document: analysis → invert → CMA-ES refine (INST=<instrument>, [REF=] [WAV=] [F0=] [AS=<newid> | INTO=<id>] [NO_REFINE=1] [SEED=] [GENS=]; writes $$BEATMO_WAV_REFS/clones/<inst>/)
	@cd src/go && ../../.tools/go/bin/go run ./cmd/clone -inst $(INST) $(if $(REF),-ref $(REF),) $(if $(WAV),-wav $(WAV),) $(if $(F0),-f0 $(F0),) \
		$(if $(AS),-as $(AS),) $(if $(INTO),-into $(INTO),) $(if $(NO_REFINE),-no-refine,) $(if $(SEED),-seed $(SEED),) $(if $(GENS),-gens $(GENS),)

clone-adopt: ## Ship a clone into internal/assets/instruments — ONLY after `make ab-session FAMILIES=<inst> CLONE=1` recorded accept=y for that exact candidate (INST=<instrument>)
	@cd src/go && ../../.tools/go/bin/go run ./cmd/clone adopt -inst $(INST)

# The slice-1 benchmark (spec §6b): local corpus only, never part of `go test`.
CLONE_BENCH ?= tubular-bells kalimba glockenspiel cowbell marimba xylophone vibraphone tom conga
clone-bench: ## Clone every slice-1 benchmark instrument and print the summary (CLONE_BENCH="a b" to choose; verdicts are the ear's — `make ab-session FAMILIES=… CLONE=1`)
	@cd src/go && ../../.tools/go/bin/go build -o /tmp/beatmo-clone ./cmd/clone
	@for i in $(CLONE_BENCH); do (cd src/go && /tmp/beatmo-clone -inst $$i $(if $(GENS),-gens $(GENS),)); done

song-timeline: ## Second-by-second analysis of a WAV (FILE=, [HOP=], [CONFIG=], [OUT=])
	cd src/go && ../../.tools/go/bin/go run ./cmd/song-match timeline -file $(FILE) \
		$(if $(HOP),-hop $(HOP),) $(if $(CONFIG),-config $(CONFIG),) $(if $(OUT),-out $(OUT),)

song-render: ## Render a template to master.wav + per-instrument stems (TEMPLATE=, OUT=, [BARS=], [SR=])
	cd src/go && ../../.tools/go/bin/go run ./cmd/song-match render -template $(TEMPLATE) -out $(OUT) \
		$(if $(BARS),-bars $(BARS),) $(if $(SR),-sr $(SR),)

song-compare: ## Compare a template to a reference WAV (TEMPLATE=, REF=, [MANIFEST=], [CONFIG=], [TUNE=1], [DRY_RUN=1])
	cd src/go && ../../.tools/go/bin/go run ./cmd/song-match compare -template $(TEMPLATE) -ref $(REF) \
		$(if $(MANIFEST),-manifest $(MANIFEST),) $(if $(CONFIG),-config $(CONFIG),) \
		$(if $(TUNE),-tune,) $(if $(DRY_RUN),-dry-run,)

# ─── WASM Builds ─────────────────────────────────────────────────────────────

# make wasm is the SHIPPED binary: noscenes drops the screenshot scene catalog, release hides the developer surfaces
# (still compiled in). wasm-dev keeps the catalog for runScene/listScenes. Comma-separated: -tags takes ONE argument.
WASM_RELEASE_TAGS = noscenes,release

wasm: gen-synth-abi gen-chain-spec $(MA_JS)
	cd src/go && ($(GO) mod download || true)
	cd src/go && GOOS=js GOARCH=wasm $(GO) build -tags $(WASM_RELEASE_TAGS) -ldflags "-s -w -X main.defaultLog=INFO" -o $(WASM_OUT) ./cmd

wasm-dev: gen-synth-abi gen-chain-spec $(MA_JS)
	cd src/go && ($(GO) mod download || true)
	cd src/go && GOOS=js GOARCH=wasm $(GO) build -ldflags "-s -w -X main.defaultLog=INFO" -o $(WASM_OUT) ./cmd

wasm-debug: gen-synth-abi gen-chain-spec $(MA_JS)
	cd src/go && ($(GO) mod download || true)
	cd src/go && GOOS=js GOARCH=wasm $(GO) build -ldflags "-s -w -X main.defaultLog=DEBUG" -o $(WASM_OUT) ./cmd

wasm-cover: gen-synth-abi gen-chain-spec $(MA_JS)
	cd src/go && ($(GO) mod download || true)
	cd src/go && GOOS=js GOARCH=wasm $(GO) build \
		-cover -covermode=atomic \
		-coverpkg=github.com/ingyamilmolinar/beatmo/... \
		-ldflags "-X main.defaultLog=INFO" \
		-o $(WASM_OUT) ./cmd

wasm-playtest:
	cd src/go && GOOS=js GOARCH=wasm $(GO) build -o ../js/play_ui.wasm ./internal/ui/playtest

wasm-audio-playtest:
	cd src/go && GOOS=js GOARCH=wasm $(GO) build -o ../js/playtest.wasm ./internal/audio/playtest

wasm-all: wasm-dev wasm-playtest wasm-audio-playtest

serve:
	cd src/js && python3 ../../scripts/serve_nocache.py 8080

# Records the landing videos from the real game's scripted tour; manual, run when the video should change
# (CUTS=desktop for one cut).
.PHONY: showcase-video
showcase-video:
	./scripts/showcase_video.sh

# Serves the app as beatmo.io does (hashed wasm stamped into index.html, compression, Range), so the landing demo
# shows on every load until opted out; BEATMO_LANDING_HOST=0.0.0.0 opens it to a phone.
.PHONY: serve-landing
serve-landing: wasm
	node src/js/landing_server.js

.PHONY: serve-lan
serve-lan: wasm
	@./scripts/serve-lan.sh

generate: gen-design-tokens
	cd src/go && $(GO) generate ./internal/audio/...

# Source of truth: DESIGN.md's YAML front matter; the generated design_*.gen.go is committed.
.PHONY: gen-design-tokens
gen-design-tokens:
	cd src/go && $(GO) run ./cmd/gen_design_tokens \
		-design ../../DESIGN.md \
		-out internal/ui/design_tokens.gen.go \
		-out-components internal/ui/design_components.gen.go \
		-out-profile internal/ui/design_profile.gen.go \
		-out-density internal/ui/design_density.gen.go \
		-out-sequence internal/templates/instrument_sequence.gen.go
	cd src/go && $(GO) fmt ./internal/ui/design_tokens.gen.go ./internal/ui/design_components.gen.go ./internal/ui/design_profile.gen.go ./internal/ui/design_density.gen.go >/dev/null
	cd src/go && $(GO) fmt ./internal/templates/instrument_sequence.gen.go >/dev/null

# Measures every instrument through the real C DSP; needs cgo and the C library, so it is manual rather than part of
# generate (the browser build cannot produce these numbers).
.PHONY: gen-fundamentals
# A dial whose 0→100 sweep is inaudible on a shipped sound is hidden there; re-measure after a tuning pass, a new
# instrument, a dial edit or an engine change.
.PHONY: gen-dial-relevance
gen-dial-relevance: $(C_LIB)
	cd src/go && DIAL_RELEVANCE_UPDATE=1 CGO_ENABLED=1 xvfb-run -a $(GO) test -count=1 -run 'TestDialRelevanceTableIsCurrent$$' -v ./internal/ui/ | grep -E 'wrote|FAIL|ok'

gen-fundamentals: $(C_LIB)
	cd src/go && CGO_ENABLED=1 $(GO) run ./cmd/gen-fundamentals \
		-o internal/audio/instrument_fundamentals.gen.go
	cd src/go && $(GO) fmt ./internal/audio/instrument_fundamentals.gen.go >/dev/null

.PHONY: fundamentals-report
fundamentals-report: $(C_LIB)
	cd src/go && CGO_ENABLED=1 $(GO) run ./cmd/gen-fundamentals -report

# The pre-commit dispatcher rejects a commit that adds or modifies an audio file or carries stale design tokens.
.PHONY: install-hooks
install-hooks:
	@if [ ! -d .git ]; then echo "not a git repo"; exit 1; fi
	@mkdir -p .git/hooks
	@cp scripts/precommit/pre-commit .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "installed pre-commit hook (rejects audio files; regenerates design tokens, rejects stale)"

fmt:
	cd src/go && $(GO) fmt ./...

# Usage: make dep-graph [MAX_FUNCS=15] [MAX_METHODS=8]
MAX_FUNCS ?= 15
MAX_METHODS ?= 8
dep-graph:
	@mkdir -p build
	$(GO) run scripts/depgraph/main.go \
		-root src/go \
		-module github.com/ingyamilmolinar/beatmo \
		-max-funcs $(MAX_FUNCS) \
		-max-methods $(MAX_METHODS) \
		> build/dep-graph.dot
	@echo "DOT: build/dep-graph.dot"
	@if command -v dot >/dev/null 2>&1; then \
		dot -Tsvg build/dep-graph.dot -o build/dep-graph.svg && \
		echo "SVG: build/dep-graph.svg"; \
		dot -Tpng build/dep-graph.dot -o build/dep-graph.png && \
		echo "PNG: build/dep-graph.png"; \
	else \
		echo "(install graphviz for SVG/PNG: sudo apt install graphviz)"; \
	fi

# Usage: make go-graph [GG_GOOS=linux] [GG_TAGS=] [GG_ROOT=src/go]
GG_GOOS ?= linux
GG_TAGS ?=
GG_ROOT ?= $(CURDIR)/src/go
go-graph:
	@mkdir -p build
	cd scripts/gograph && $(GO) run . \
		-root $(GG_ROOT) \
		-module github.com/ingyamilmolinar/beatmo \
		-goos $(GG_GOOS) -tags '$(GG_TAGS)' \
		-out $(CURDIR)/build/go-graph.html \
		-json $(CURDIR)/build/go-graph.json
	@echo "Open build/go-graph.html in a browser"

# Usage: make refactor-rank [RANK_FAST=1] [RANK_COVER=...] [RANK_BASELINE=old/rank.json] [RANK_TOP=40], then
# make refactor-packets [RANK_PACKETS=40]. Existing coverage profiles are picked up; agent verdicts come from
# build/refactor/verdicts/ (local only).
RANK_COVER ?= $(wildcard coverage/go.out coverage/go-native.out)
RANK_TOP ?= 40
RANK_PACKETS ?= 40
RANK_ARGS = -root $(CURDIR)/src/go -module github.com/ingyamilmolinar/beatmo \
	-out $(CURDIR)/build/refactor -suppress $(CURDIR)/scripts/gograph/refactor_suppress.json \
	-top $(RANK_TOP) $(foreach c,$(RANK_COVER),-cover $(CURDIR)/$(c)) \
	$(if $(RANK_BASELINE),-baseline $(abspath $(RANK_BASELINE))) $(if $(RANK_FAST),-fast)
refactor-rank:
	cd scripts/gograph && $(GO) run . rank $(RANK_ARGS)

refactor-packets:
	cd scripts/gograph && $(GO) run . rank $(RANK_ARGS) -packets $(RANK_PACKETS)

# Part of make test so an x/tools or Go upgrade that breaks the tool fails the day it happens.
gograph-test:
	cd scripts/gograph && $(GO) test ./...

# A typed id or enum reaching syscall/js panics in the browser only, invisible to the compiler and every native test.
js-audit:
	cd scripts/jsaudit && $(GO) test ./... && $(GO) run . $(CURDIR)/src/go ./...

# Comment census + policy lint: make comment-census [ARGS="--check" | "--top 60" | "--json"].
.PHONY: comment-census test-comment-census
comment-census:
	python3 scripts/comment_census.py $(ARGS)

test-comment-census:
	python3 scripts/comment_census_test.py

# ─── Linting ─────────────────────────────────────────────────────────────────
GOLANGCI_LINT = .tools/golangci-lint

$(GOLANGCI_LINT):
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b .tools latest

lint-go: $(GOLANGCI_LINT)
	cd src/go && ../../$(GOLANGCI_LINT) run ./...

lint-go-fix: $(GOLANGCI_LINT)
	cd src/go && ../../$(GOLANGCI_LINT) run --fix ./...

lint-js:
	cd src/js && npx eslint .

lint-js-fix:
	cd src/js && npx eslint --fix .

lint-c:
	cppcheck --enable=warning,style,performance,portability \
		--error-exitcode=1 --suppress=missingIncludeSystem \
		--suppress='*:src/c/miniaudio.h' \
		--inline-suppr -I src/c \
		src/c/drums.c src/c/drums.h \
		src/c/fmsynth.c src/c/fmsynth.h \
		src/c/insert_fx.c src/c/insert_fx.h \
		src/c/wavetable.c src/c/wavetable.h \
		src/c/adsr.c src/c/adsr.h \
		src/c/modular.c src/c/modular.h \
		src/c/noise.c src/c/noise.h \
		src/c/synth_post.h

lint: lint-go lint-js lint-c
lint-fix: lint-go-fix lint-js-fix

# Capture screenshots of desktop, browser desktop, and browser mobile layouts.
# Output goes to screenshots/ (override with OUTDIR=).
screenshot: $(C_LIB) wasm-dev
	GO=$(GO) OUTDIR=$(or $(OUTDIR),screenshots) ./scripts/screenshot.sh

# Every scene in the catalog → screenshots/all/ (OUTDIR=); MOBILE=1 adds a mobile pass, SCENES=a,b filters.
screenshots-all: $(C_LIB) wasm-dev
	GO=$(GO) OUTDIR=$(or $(OUTDIR),screenshots/all) MOBILE=$(or $(MOBILE),0) \
		SCENES=$(or $(SCENES),) ./scripts/capture_all_ui.sh

# Alias used by docs / future baseline workflows.
screenshots-all-update: screenshots-all
	@echo "screenshots refreshed"

# One screenshot per distinct surface (scripts/basic_scenes.txt) → screenshots/basic/ (OUTDIR=); mobile pass on.
screenshots-basic: $(C_LIB) wasm-dev
	GO=$(GO) OUTDIR=$(or $(OUTDIR),screenshots/basic) MOBILE=$(or $(MOBILE),1) \
		SCENES=$$(grep -vE '^[[:space:]]*(\#|$$)' scripts/basic_scenes.txt | paste -sd, -) \
		./scripts/capture_all_ui.sh

run: $(C_LIB)
	cd src/go && CGO_ENABLED=1 $(GO) build -o /tmp/beatmo ./cmd
	/tmp/beatmo -log INFO $(RUN_ARGS)

run-debug: $(C_LIB)
	cd src/go && TIMELINE_TRACE=1 PERF_LOG=1 CGO_ENABLED=1 $(GO) build -o /tmp/beatmo ./cmd
	TIMELINE_TRACE=1 PERF_LOG=1 /tmp/beatmo -log DEBUG $(RUN_ARGS)

bench: $(C_LIB)
	cd src/go; PERF_LOG=1 CGO_ENABLED=1 $(GO) run ./cmd -log INFO -bench-bpm $(or $(BPM),200) -bench-secs $(or $(SECS),10) $(RUN_ARGS)

# Self-contained desktop benchmark: 4 BPM levels (120/200/240/300), 15s each,
# pprof CPU profile, structured JSON + ASCII table output.
# Override: BPM_LEVELS="120 200" DURATION=5 make bench-desktop
bench-desktop: $(C_LIB)
	GO=$(GO) BPM_LEVELS="$(or $(BPM_LEVELS),120 200 240 300)" DURATION=$(or $(DURATION),15) ./scripts/bench-desktop.sh

# Desktop synth param-mutation storm bench + pprof (toggle|ctrl|combined).
# Override: STORM=toggle BPM=120 DURATION=10 make bench-synth-storm
bench-synth-storm: $(C_LIB)
	GO=$(GO) BENCH_SYNTH_STORM=$(or $(STORM),combined) STORM_BPM=$(or $(BPM),200) DURATION=$(or $(DURATION),15) ./scripts/bench-desktop.sh

# Seek-path microbenchmarks (forward/backward seeks, timeline edits during playback). Override BENCHTIME/COUNT.
bench-seek:
	cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run '^$$' \
		-bench 'BenchmarkSeek|BenchmarkTimelineEdit' -benchmem \
		-benchtime=$(or $(BENCHTIME),1s) -count=$(or $(COUNT),2) ./internal/ui/

# Self-contained browser benchmark: same 4 BPM levels, startup demo circuit.
# Produces bench-results/browser.json + ASCII table.
bench-browser: wasm
	@mkdir -p bench-results
	GO=$(GO) node src/js/webaudio_bench_startup.browser.test.js

# Run perf_e2e with the realistic startup demo circuit (58 nodes, 7 instruments).
bench-perf-e2e: wasm
	BENCH_CIRCUIT=startup BENCH_DURATION=$(or $(DURATION),15000) BENCH_BPM=$(or $(BPM),200) GO=$(GO) node src/js/webaudio_perf_e2e.browser.test.js

# Raw WAV vs synth vs synth-under-param-storm trigger latency → bench-results/render_latency.json;
# RENDER_BENCH_STRICT=1 gates the storm scenarios' deferAbsMsP90.
bench-render: wasm
	@mkdir -p bench-results
	GO=$(GO) node src/js/webaudio_render_latency_bench.browser.test.js

# What is HEARD under UI loads (the playback probe) → bench-results/audio_isolation.json; AUDIO_PROBE_STRICT=1
# enforces lateness, SCENARIOS=a,b filters.
bench-isolation: wasm
	@mkdir -p bench-results
	ISOLATION_SCENARIOS=$(SCENARIOS) GO=$(GO) WASM_PREBUILT=1 node src/js/webaudio_audio_isolation_bench.browser.test.js

# Interleaved baseline / lane-off / lane-on, ROUNDS times with load recorded. AB_SCRATCH holds baseline_site/ and
# lane.wasm (GOOS=js GOARCH=wasm go build -o $AB_SCRATCH/lane.wasm ./cmd).
bench-lane-ab:
	AB_SCRATCH=$(AB_SCRATCH) ROUNDS=$(or $(ROUNDS),3) GO=$(GO) node scripts/audio_lane_ab.mjs

# Import-cold bench: load a heavy circuit, play immediately, record how long
# the engine keeps rendering under the user. Produces bench-results/import_cold.json.
bench-import-cold: wasm
	@mkdir -p bench-results
	GO=$(GO) node src/js/webaudio_import_cold_bench.browser.test.js

# Run both desktop and browser benchmarks, then produce a comparison report.
bench-all: bench-desktop bench-browser
	node scripts/bench-compare.js

# Record-while-playing with profiling → bench-results/record-<timestamp>/; scripts/compare-bench.sh then exits
# non-zero on a regression against bench-results/REPORT.md.
PERF_RECORD_SECS ?= 30
PERF_RECORD_BPM  ?= 140
perf-record-bench: $(C_LIB)
	cd src/go; CGO_ENABLED=1 $(GO) build -o /tmp/beatmo ./cmd
	xvfb-run -a /tmp/beatmo -record-bench $(PERF_RECORD_SECS) -record-bench-bpm $(PERF_RECORD_BPM) -log INFO

# perf-record-noisy: same as perf-record-bench but with stress-ng running
# alongside to emulate noisy-neighbor CPU/IO/memory pressure. Requires
# stress-ng installed; install with `apt-get install stress-ng` on Debian.
perf-record-noisy: $(C_LIB)
	@command -v stress-ng >/dev/null 2>&1 || { echo "stress-ng required: apt-get install stress-ng"; exit 1; }
	cd src/go; CGO_ENABLED=1 $(GO) build -o /tmp/beatmo ./cmd
	stress-ng --cpu 2 --vm 1 --io 1 --timeout $(PERF_RECORD_SECS)s & \
	xvfb-run -a /tmp/beatmo -record-bench $(PERF_RECORD_SECS) -record-bench-bpm $(PERF_RECORD_BPM) -log INFO; \
	wait

# perf-record-diagnose: record-while-playing with block + mutex profiles
# enabled. Use this BEFORE believing any "fixed" claim — it tells us
# where the audio thread is actually blocked.
perf-record-diagnose: $(C_LIB)
	cd src/go; CGO_ENABLED=1 $(GO) build -o /tmp/beatmo ./cmd
	BEATMO_BLOCK_PROFILE=1 BEATMO_MUTEX_PROFILE=1 \
		xvfb-run -a /tmp/beatmo -record-bench $(PERF_RECORD_SECS) -record-bench-bpm $(PERF_RECORD_BPM) -log INFO
	@echo "Inspect with:"
	@echo "  go tool pprof -top -cum bench-results/record-<ts>/block.pprof"
	@echo "  go tool pprof -top -cum bench-results/record-<ts>/mutex.pprof"

# Run with camera/node/edge detailed logs enabled
run-cam-logs: $(C_LIB)
	cd src/go; CGO_ENABLED=1 $(GO) build -o /tmp/beatmo ./cmd && DEBUG_DRAW_NODES=1 DEBUG_GEOM=1 /tmp/beatmo -log DEBUG $(RUN_ARGS)

test: $(C_LIB)
# -timeout is per package binary. internal/ui paces it (its soak/leak canaries run full length only here) and races
# sibling packages for CPU under ./..., so the budget is cumulative test work, not a hang.
	cd src/go; $(LOG_ENV) $(GO) test -tags test -modfile=go.test.mod -timeout 1500s ./...
# Untagged = the real C DSP; the budget is whole-package, not per-test.
	cd src/go; $(LOG_ENV) $(GO) test -timeout 900s ./internal/audio
	$(MAKE) gograph-test
	$(MAKE) js-audit
	$(MAKE) test-comment-census
	$(MAKE) comment-census ARGS=--check

# Native only (-race cannot instrument js/wasm). Excluded: single-threaded numeric suites and the huge ui/timeline
# suites, which take 15-20 min under -race and add no goroutine coverage. RACE_EXCLUDE= widens it.
RACE_EXCLUDE ?= internal/ui$$|internal/timeline$$|audio/songmatch|audio/synthmatch|audio/fingerprint|cmd/synth-analyze|internal/deploy
.PHONY: test-race
test-race: $(C_LIB)
	cd src/go; $(LOG_ENV) $(GO) test -race -tags test -modfile=go.test.mod -timeout 900s \
	  $$($(GO) list -tags test -modfile=go.test.mod -f '{{if or .GoFiles .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./... | grep -vE '$(RACE_EXCLUDE)')

# Run all browser tests (auto-discovered). Timing-sensitive tests run sequentially
# after the parallel batch. BROWSER_JOBS=N controls parallelism (default: 4).
test-browser: wasm-all
	@if ! command -v node >/dev/null 2>&1; then echo 'Node not found'; exit 1; fi
	GO=$(GO) BROWSER_JOBS=$(or $(BROWSER_JOBS),4) ./scripts/run-browser-tests.sh

# Run browser tests with a filter pattern
# Usage: make test-browser-filter FILTER=touch
test-browser-filter: wasm-all
	@if ! command -v node >/dev/null 2>&1; then echo 'Node not found'; exit 1; fi
	GO=$(GO) BROWSER_JOBS=$(or $(BROWSER_JOBS),4) ./scripts/run-browser-tests.sh --filter "$(FILTER)"

# List all browser tests that would run
test-browser-list:
	@./scripts/run-browser-tests.sh --list

# Generate JSON matrix of browser test batches (for CI or local inspection)
# Usage: make test-browser-matrix [BATCH_SIZE=4]
test-browser-matrix:
	@./scripts/browser-test-matrix.sh --batch-size $(or $(BATCH_SIZE),4)

# Alias for convenience; many developers instinctively run `make tests`.
tests: test

test-debug: $(C_LIB)
# The same -tags test ./... runner as make test, so the same budget.
	cd src/go; $(LOG_ENV) $(GO) test -tags test -modfile=go.test.mod -timeout 1500s ./...
	cd src/go; $(LOG_ENV) $(GO) test -timeout 900s ./internal/audio
	$(MAKE) test-browser

# Real Ebiten (X11/xvfb) + every browser test. 30m because synthmatch runs its optimizer against the real cgo synth.
test-real: $(C_LIB)
	cd src/go && BPM_TIMING_TEST=1 xvfb-run -a $(GO) test -timeout 30m ./...
	$(MAKE) test-browser

# Run cross-platform parity tests: Go golden files + browser comparison
test-parity: $(C_LIB)
	cd src/go; $(LOG_ENV) $(GO) test -tags test -modfile=go.test.mod -run TestCrossPlatformParity -timeout 30s ./internal/ui
	$(MAKE) test-browser-filter FILTER=xplat_parity

# Visual regression tests (screenshots + comparison)
test-visual: wasm
	@if ! command -v node >/dev/null 2>&1; then echo 'Node not found'; exit 1; fi
	cd src/js && npm install --no-save 2>/dev/null; \
	GO=$(GO) node visual_mobile_parity.browser.test.js && \
	GO=$(GO) node visual_regression.browser.test.js

# Visual tests on real mobile devices via BrowserStack (BROWSERSTACK_USERNAME + BROWSERSTACK_ACCESS_KEY).
test-visual-device: wasm
	@if [ -z "$$BROWSERSTACK_USERNAME" ] || [ -z "$$BROWSERSTACK_ACCESS_KEY" ]; then \
		echo "ERROR: Set BROWSERSTACK_USERNAME and BROWSERSTACK_ACCESS_KEY"; exit 1; fi
	cd src/js && npm install --no-save 2>/dev/null; \
	GO=$(GO) node visual_device_parity.browser.test.js

# Real-device user-action bridge tests (real <input type=file>, downloads) via BrowserStack; DEVICE_FILTER=iPhone
# narrows. Develop locally first (free): GO=$(GO) node src/js/device_user_actions.browser.test.js
test-device-actions: wasm
	@if [ -z "$$BROWSERSTACK_USERNAME" ] || [ -z "$$BROWSERSTACK_ACCESS_KEY" ]; then \
		echo "ERROR: Set BROWSERSTACK_USERNAME and BROWSERSTACK_ACCESS_KEY"; exit 1; fi
	cd src/js && npm install --no-save 2>/dev/null; \
	TEST_PLATFORM=browserstack GO=$(GO) node device_user_actions.browser.test.js

test-visual-update: wasm
	@rm -f src/js/testdata/golden/*.png
	cd src/js && npm install --no-save 2>/dev/null; \
	GO=$(GO) node visual_regression.browser.test.js

test-xvfb:
	cd src/go; xvfb-run -a $(GO) test -tags test -timeout 10s ./...

# Record any browser test: record-test TEST=<name> [NAME=], record-browser-filter FILTER=, record (interactive),
# replay NAME= [SPEED=], evaluate NAME= [TEMPLATE=|PROMPT=] (needs ANTHROPIC_API_KEY).

# Record a single browser test. TEST= is the test name without .browser.test.js suffix.
# Optional: NAME= to set recording session name (default: test name).
# Optional: INTERVAL= screenshot interval in ms (default: 500).
record-test:
	@if [ -z "$(TEST)" ]; then echo "Usage: make record-test TEST=<name> [NAME=<session>]"; exit 1; fi
	@if ! command -v node >/dev/null 2>&1; then echo 'Node not found'; exit 1; fi
	cd src/js && LLM_RECORD=1 \
		LLM_RECORD_NAME=$(or $(NAME),$(TEST)) \
		$(if $(INTERVAL),LLM_RECORD_INTERVAL=$(INTERVAL)) \
		GO=$(GO) node $(TEST).browser.test.js

# Record a filtered set of browser tests. Each test gets its own recording.
record-browser-filter: wasm-dev
	@if ! command -v node >/dev/null 2>&1; then echo 'Node not found'; exit 1; fi
	LLM_RECORD=1 $(if $(INTERVAL),LLM_RECORD_INTERVAL=$(INTERVAL)) \
		GO=$(GO) ./scripts/run-browser-tests.sh --filter "$(FILTER)"

# Interactive recording: opens non-headless browser, Ctrl+C to stop.
record:
	@if ! command -v node >/dev/null 2>&1; then echo 'Node not found'; exit 1; fi
	GO=$(GO) node src/js/llm_test/cli.js record \
		$(if $(NAME),--name $(NAME)) \
		$(if $(VIEWPORT),--viewport $(VIEWPORT)) \
		$(if $(INTERVAL),--frame-interval $(INTERVAL))

# Replay a recorded session.
replay:
	@if [ -z "$(NAME)" ]; then echo "Usage: make replay NAME=<session> [SPEED=1.0]"; exit 1; fi
	@if ! command -v node >/dev/null 2>&1; then echo 'Node not found'; exit 1; fi
	GO=$(GO) node src/js/llm_test/cli.js replay \
		src/js/recordings/$(NAME) \
		$(if $(SPEED),--speed $(SPEED))

# Evaluate a recording with Claude vision.
evaluate:
	@if [ -z "$(NAME)" ]; then echo "Usage: make evaluate NAME=<session> [TEMPLATE=general_quality]"; exit 1; fi
	@if [ -z "$$ANTHROPIC_API_KEY" ]; then echo "ERROR: Set ANTHROPIC_API_KEY"; exit 1; fi
	@if ! command -v node >/dev/null 2>&1; then echo 'Node not found'; exit 1; fi
	node src/js/llm_test/cli.js evaluate \
		src/js/recordings/$(NAME) \
		$(if $(TEMPLATE),--template $(TEMPLATE)) \
		$(if $(PROMPT),--prompt "$(PROMPT)") \
		$(if $(MAX_FRAMES),--max-frames $(MAX_FRAMES)) \
		--save

# A Claude agent drives Beatmo in the browser from screenshots: make agent PROMPT="..." [MAX_ITER=] [NAME=]
# [MODEL=], or TEST=<case>.
agent:
	@if [ -z "$(PROMPT)" ] && [ -z "$(TEST)" ]; then echo "Usage: make agent PROMPT=\"...\" or make agent TEST=<name> [MAX_ITER=50] [NAME=<session>] [MODEL=claude-haiku-4-5-20251001]"; exit 1; fi
	@if [ -z "$$ANTHROPIC_API_KEY" ]; then echo "ERROR: Set ANTHROPIC_API_KEY"; exit 1; fi
	@if ! command -v node >/dev/null 2>&1; then echo 'Node not found'; exit 1; fi
	GO=$(GO) node src/js/llm_test/cli.js agent \
		$(if $(PROMPT),--prompt "$(PROMPT)") \
		$(if $(TEST),--test $(TEST)) \
		$(if $(MAX_ITER),--max-iterations $(MAX_ITER)) \
		$(if $(NAME),--name $(NAME)) \
		$(if $(MODEL),--model $(MODEL)) \
		$(if $(VIEWPORT),--viewport $(VIEWPORT)) \
		$(if $(PLATFORM),--platform $(PLATFORM))

# make agent-list [FILTER=] [TAGS=] [PLATFORM=]
agent-list:
	@if ! command -v node >/dev/null 2>&1; then echo 'Node not found'; exit 1; fi
	@node src/js/llm_test/cli.js list-tests \
		$(if $(FILTER),--filter "$(FILTER)") \
		$(if $(TAGS),--tags "$(TAGS)") \
		$(if $(PLATFORM),--platform $(PLATFORM))

# make agent-tests [TAGS=] [FILTER=] [PLATFORM=]
agent-tests:
	@if [ -z "$$ANTHROPIC_API_KEY" ]; then echo "ERROR: Set ANTHROPIC_API_KEY"; exit 1; fi
	@if ! command -v node >/dev/null 2>&1; then echo 'Node not found'; exit 1; fi
	GO=$(GO) node src/js/llm_test/cli.js run-tests \
		$(if $(FILTER),--filter "$(FILTER)") \
		$(if $(TAGS),--tags "$(TAGS)") \
		$(if $(PLATFORM),--platform $(PLATFORM)) \
		$(if $(MODEL),--model $(MODEL))

# Generate HTML report from an agent/recording session.
#   make agent-report NAME=my_session
agent-report:
	@if [ -z "$(NAME)" ]; then echo "Usage: make agent-report NAME=<session>"; exit 1; fi
	@if ! command -v node >/dev/null 2>&1; then echo 'Node not found'; exit 1; fi
	node src/js/llm_test/cli.js report --name $(NAME) \
		$(if $(OUTPUT),--output $(OUTPUT))

# List recorded sessions.
recordings-list:
	@ls -1d src/js/recordings/*/ 2>/dev/null | while read d; do \
		name=$$(basename "$$d"); \
		frames=$$(ls "$$d"frame_*.png 2>/dev/null | wc -l); \
		if [ -f "$$d/recording.json" ]; then \
			echo "  $$name  ($$frames frames)"; \
		fi; \
	done || echo "  (no recordings)"

dependencies:
	./scripts/setup-env.sh

# GCP configuration
GCP_PROJECT ?= beatmo
GCS_BUCKET ?= www-beatmo-io-static
DOMAIN ?= beatmo.io

# Set up GCP infrastructure for static site hosting (idempotent)
# Usage: make setup-gcp
setup-gcp:
	GCP_PROJECT=$(GCP_PROJECT) GCS_BUCKET=$(GCS_BUCKET) DOMAIN=$(DOMAIN) ./scripts/setup-gcs-static-site.sh

# Deploy WASM to GCS bucket
# Usage: make deploy
# For dry-run: DRY_RUN=1 make deploy
deploy:
	GCP_PROJECT=$(GCP_PROJECT) GCS_BUCKET=$(GCS_BUCKET) ./scripts/deploy-wasm-gcs.sh

# ─── Coverage ────────────────────────────────────────────────────────────────

# .PHONY because the coverage/ output directory shadows the target name: make coverage would report "up to date"
# and run nothing.
.PHONY: coverage coverage-go coverage-go-check coverage-c coverage-c-check \
	coverage-browser coverage-js coverage-report coverage-graph

# Go test coverage (continues on test failures, warns instead of aborting)
coverage-go: $(C_LIB)
	@mkdir -p coverage
	@cd src/go; $(LOG_ENV) $(GO) test -tags test -modfile=go.test.mod \
		-coverprofile=../../coverage/go.out -covermode=atomic -timeout 360s ./... \
		|| echo "WARNING: Some Go tests failed — coverage data still collected"
	@if [ -f coverage/go.out ]; then cd src/go && $(GO) tool cover -func=../../coverage/go.out | tail -1; fi

# Fails when total Go coverage drops below COVERAGE_GO_FLOOR; raise the floor when a significant batch of tests lands.
COVERAGE_GO_FLOOR ?= 80
coverage-go-check: coverage-go
	@total=$$(cd src/go && $(GO) tool cover -func=../../coverage/go.out | awk '/^total:/ {gsub("%","",$$NF); print $$NF}'); \
	floor=$(COVERAGE_GO_FLOOR); \
	if [ -z "$$total" ]; then echo "coverage-go-check: could not parse total"; exit 1; fi; \
	awk -v t="$$total" -v f="$$floor" 'BEGIN { exit (t+0 < f+0) }'; \
	rc=$$?; \
	if [ $$rc -ne 0 ]; then \
		printf "coverage-go-check: FAIL — total %s%% < floor %s%%\n" "$$total" "$$floor"; \
		exit 1; \
	fi; \
	printf "coverage-go-check: OK — total %s%% >= floor %s%%\n" "$$total" "$$floor"

# Browser test coverage (builds coverage-instrumented WASM, runs all browser tests)
coverage-browser: wasm-cover
	GO=$(GO) ./scripts/run-browser-coverage.sh

# JS source coverage (V8 coverage via Playwright's page.coverage API, reported by c8)
coverage-js:
	@echo "=== JS Source Coverage ==="
	@if [ -d coverage/js-raw ] && ls coverage/js-raw/*.v8cov.json >/dev/null 2>&1; then \
		cd src/js && npx c8 report --reporter=text --temp-directory=../../coverage/js-raw --src=.; \
	else \
		echo "No JS coverage data. Run browser tests with JS_COVERAGE=1 first."; \
	fi

# gcov over our own src/c via the native audio tests, plus coverage/go-native.out for the bridge files coverage-go
# never compiles. No $(C_LIB) dependency: the script swaps in an instrumented lib and removes it on exit.
# COVERAGE_C_FLOOR=NN enforces a floor; COVERAGE_C_XVFB=1 adds an xvfb pass for the display-requiring mixer tests.
coverage-c:
	@mkdir -p coverage/c
	CC=$(CC) GO=$(GO) COVERAGE_C_FLOOR=$(COVERAGE_C_FLOOR) \
	COVERAGE_C_XVFB=$(or $(COVERAGE_C_XVFB),0) ./scripts/coverage-c.sh

# Enforces the C coverage floor; the only uncovered lines are defensive guards, so ratchet when that set shrinks.
# coverage-c deletes build/libdrums.a on exit: run make build/libdrums.a afterwards.
coverage-c-check:
	$(MAKE) coverage-c COVERAGE_C_FLOOR=$(or $(COVERAGE_C_FLOOR),99)

# All coverage (continues through failures, warns at end)
coverage:
	@rc=0; \
	$(MAKE) coverage-go || rc=1; \
	$(MAKE) coverage-c || rc=1; \
	$(MAKE) coverage-browser || rc=1; \
	$(MAKE) coverage-js || rc=1; \
	if [ $$rc -ne 0 ]; then \
		echo ""; \
		echo "WARNING: Some coverage targets had failures — see output above"; \
	fi

# Generate HTML reports from coverage profiles (go-native = the CGo-bridge
# build of internal/audio captured by coverage-c; C summary printed at the end)
coverage-report:
	@for prof in go go-native browser; do \
		if [ -f coverage/$$prof.out ]; then \
			head -1 coverage/$$prof.out > coverage/$$prof.clean.out; \
			tail -n +2 coverage/$$prof.out | while IFS= read -r line; do \
				f=$$(echo "$$line" | cut -d: -f1); \
				resolved="src/go/$${f#github.com/ingyamilmolinar/beatmo/}"; \
				[ -f "$$resolved" ] && echo "$$line"; \
			done >> coverage/$$prof.clean.out; \
			cd src/go && $(GO) tool cover -html=../../coverage/$$prof.clean.out -o ../../coverage/$$prof.html && cd ../.. && \
			rm -f coverage/$$prof.clean.out && \
			echo "$$(echo $$prof | sed 's/^b/B/;s/^g/G/') coverage report: coverage/$$prof.html"; \
		else \
			echo "No $$prof coverage data (run make coverage-$$prof first)"; \
		fi; \
	done; \
	if [ -f coverage/c-summary.txt ]; then \
		echo ""; \
		echo "=== C Coverage (annotated: coverage/c/*.c.gcov) ==="; \
		cat coverage/c-summary.txt; \
	else \
		echo "No C coverage data (run make coverage-c first)"; \
	fi

# Generate coverage graph (DOT + SVG/PNG) from Go coverage profile.
# Usage: make coverage-graph [MAX_FUNCS=20] [MAX_METHODS=10] [MAX_TYPES=15]
coverage-graph: coverage/go.out
	@mkdir -p build
	$(GO) run scripts/covgraph/main.go \
		-root src/go \
		-module github.com/ingyamilmolinar/beatmo \
		-cover coverage/go.out \
		-max-funcs $(or $(COV_MAX_FUNCS),20) \
		-max-methods $(or $(COV_MAX_METHODS),10) \
		-max-types $(or $(COV_MAX_TYPES),15) \
		> build/cov-graph.dot
	@echo "DOT: build/cov-graph.dot"
	@if command -v dot >/dev/null 2>&1; then \
		dot -Tsvg build/cov-graph.dot -o build/cov-graph.svg && \
		echo "SVG: build/cov-graph.svg"; \
		dot -Tpng build/cov-graph.dot -o build/cov-graph.png && \
		echo "PNG: build/cov-graph.png"; \
	else \
		echo "(install graphviz for SVG/PNG: sudo apt install graphviz)"; \
	fi
