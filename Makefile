include ci/versions.env

GO ?= $(shell if [ -x $(CURDIR)/.tools/go/bin/go ]; then echo $(CURDIR)/.tools/go/bin/go; else command -v go || echo go; fi)
WASM_OUT = ../js/main.wasm
MA_JS = src/js/drums.single.js
C_ARCHIVE = build/libdrums.a
# What cgo consumers depend on: the archive plus its content hash keyed into the
# cgo package, so Go relinks exactly when the archive's bytes change.
C_LIB = src/go/internal/audio/clib_hash.gen.go
C_SRC = src/c/drums.c src/c/miniaudio.c src/c/fmsynth.c src/c/insert_fx.c src/c/fx_host.c src/c/mix_dsp.c src/c/wavetable.c src/c/adsr.c src/c/noise.c src/c/modular.c src/c/modular_stages.c src/c/modal_bank.c src/c/gcov_flush.c
# Test-only oracle shim (real ma_noise). Linked into the NATIVE archive so
# noise_ma_parity_test.go can compare the transplant against miniaudio, but
# deliberately kept OUT of $(C_SRC) — the emcc WASM build must not ship this
# test scaffold (nothing references it at runtime, and it exports no symbol).
C_SRC_TEST = src/c/noise_ma_oracle.c
C_HDRS = src/c/once.h src/c/drums.h src/c/miniaudio.h src/c/fmsynth.h src/c/synth_post.h src/c/modular_params.gen.h src/c/synth_dsp_primitives.h src/c/insert_fx.h src/c/fx_host.h src/c/mix_dsp.h src/c/wavetable.h src/c/adsr.h src/c/noise.h src/c/modular.h src/c/modular_stages.h src/c/modal_bank.h

# Generated synth ABI bridge: maps the C synth_params / modular_params struct
# field order to the JS heap-write indices audio.js uses (renderToCache). It is
# DERIVED from the Go schema (which is drift-tested against the C structs), so
# it MUST be regenerated whenever the schema changes. If it goes stale relative
# to the C module, audio.js allocates an undersized param block and the C voice
# reads uninitialised fields — e.g. a missing osc_enabled silences the modular
# generator. Every WASM build depends on it so the browser ABI can never drift.
SYNTH_ABI_JS = src/js/synth_param_abi.gen.js

# Opt-in test logging (keeps defaults quiet). Usage:
#   TEST_LOG=1 [TEST_LOG_LEVEL=TRACE] make test
# Leaving TEST_LOG unset keeps loggers silent during passing tests.
ifdef TEST_LOG
LOG_ENV := BEATMO_TEST_LOG=$(TEST_LOG)
ifdef TEST_LOG_LEVEL
LOG_ENV += BEATMO_TEST_LOG_LEVEL=$(TEST_LOG_LEVEL)
endif
endif

EMCC_FLAGS = -O3 -DMA_ENABLE_MP3 -sWASM=1 -sSINGLE_FILE=1 -sENVIRONMENT=web -sASSERTIONS=0 -sALLOW_MEMORY_GROWTH=0 -sSTACK_SIZE=1048576 -sEXPORTED_FUNCTIONS='[_render_modular_p,_modular_param_count,_ifx_param_names,_ifx_create,_ifx_run,_ifx_set,_ifx_clear,_ifx_destroy,_eq_create,_eq_update,_eq_run,_eq_clear,_eq_destroy,_soft_clip_run,_malloc,_free]' -sEXPORTED_RUNTIME_METHODS='["ccall","HEAPU8","HEAPF32","HEAPF64"]' -sMODULARIZE=1 -sEXPORT_ES6=1

# The committed browser engine must be what the pinned emcc builds from src/c.
check-ma-js:
	@command -v emcc >/dev/null 2>&1 || { echo "check-ma-js needs emcc $(EMSDK_VERSION) (make doctor)"; exit 1; }
	@mkdir -p build
	emcc $(C_SRC) $(EMCC_FLAGS) -o build/drums.single.js
	@cmp -s build/drums.single.js $(MA_JS) || { echo "$(MA_JS) is stale: rebuild it with emcc $(EMSDK_VERSION) (make $(MA_JS)) and commit it"; exit 1; }

$(MA_JS): $(C_SRC) $(C_HDRS)
	@if command -v emcc >/dev/null 2>&1; then \
		emcc $(C_SRC) $(EMCC_FLAGS) -o $(MA_JS); \
	else \
		if [ -f "$(MA_JS)" ]; then \
			echo "[INFO] emcc not found; using existing $(MA_JS)"; \
		else \
			echo "[WARN] emcc not found and $(MA_JS) missing; skipping MA build"; \
		fi; \
	fi

# Per-object compilation. The old monolithic rule recompiled everything in ONE
# recipe gated on "any .c/.h newer than the .a" — which lost timestamp races and
# silently SHIPPED STALE C (a C edit appeared to have zero effect). With one rule
# per object, make rebuilds exactly the object whose source (or any header,
# conservatively) changed, and the archive rebuilds whenever any object does.
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

$(C_ARCHIVE): $(C_OBJS)
	ar rcsD $@ $(C_OBJS)

$(C_LIB): $(C_ARCHIVE)
	@scripts/clib-hash.sh --write >/dev/null

clean:
	rm -f build/drums.o build/fmsynth.o build/insert_fx.o build/fx_host.o build/mix_dsp.o build/wavetable.o build/adsr.o build/noise.o build/noise_ma_oracle.o build/modular.o build/modular_stages.o build/modal_bank.o build/gcov_flush.o
	rm -f $(C_ARCHIVE) $(C_LIB)
	rm -f $(MA_JS)

# Regenerate the synth ABI bridge from the Go schema. Cheap (no CGo); run on
# every WASM build so src/js/synth_param_abi.gen.js can never lag the C structs.
# UNCONDITIONAL regen (phony, not mtime-gated). A stale synth_param_abi.gen.js
# that happened to be newer than the schema source would NOT be regenerated by a
# file-target rule — and a stale ABI silently silences re-voiced instruments
# (undersized param block → C reads uninitialised enable flags). So every WASM
# build regenerates it from scratch. The Go run is ~1s; correctness > the cache.
.PHONY: gen-synth-abi
gen-synth-abi:
	@# The JS ABI and the C struct header come from ONE table
	@# (internal/audio/modular_fields.go). Built through the stub (-tags test)
	@# path: no cgo, so it never needs the header it is generating.
	cd src/go && $(GO) run -tags test -modfile=go.test.mod ./cmd/gen-synth-abi -o ../js/synth_param_abi.gen.js
	cd src/go && $(GO) run -tags test -modfile=go.test.mod ./cmd/gen-synth-abi -c -o ../c/modular_params.gen.h

# src/js/chain_spec.gen.js from chain_spec.go, so the browser's chain values never
# drift from desktop's. Its generator links the native audio package (cgo), so it
# needs libdrums.a even on a WASM-only build.
.PHONY: gen-chain-spec
gen-chain-spec: $(C_LIB)
	cd src/go && $(GO) run ./cmd/gen-chain-spec > ../js/chain_spec.gen.js

# Rebuild the embedded instrument pack (internal/assets/instrumentpack/instruments.pack.json.gz)
# from the readable documents in src/go/internal/assets/instruments.
# Drift-tested by internal/assets/instrumentpack TestAssetPackIsCurrent.
.PHONY: asset-pack
asset-pack: ## Rebuild the embedded instrument pack from internal/assets/instruments
	cd src/go && $(GO) run ./cmd/gen-asset-pack

# Regenerate the subset Inter faces embedded in main.wasm from the full upstream
# TTFs in src/go/internal/assets/fonts/. Stock Inter ships 2,851 reachable
# glyphs; Beatmo renders ~1,100 of them, and every embedded byte is downloaded by
# every visitor -- subsetting halves both faces (812 KiB -> 495 KiB raw,
# 397 KiB -> 236 KiB gzip). NOT run by `make wasm`: the subsets are committed,
# and regenerating needs node + the subset-font (harfbuzz) devDependency. Re-run
# this only when the upstream TTFs or KEEP_RANGES change. Coverage is pinned by
# internal/assets/fonts_coverage_test.go, which fails if a narrower subset drops
# a codepoint the UI draws.
.PHONY: gen-fonts
gen-fonts:
	npm install --no-audit --no-fund --silent subset-font
	node scripts/subset_fonts.mjs
	@echo "regenerated src/go/internal/assets/fonts/*.subset.ttf -- run the assets tests"

.PHONY: measure-loudness
measure-loudness: $(C_LIB) ## Measure each instrument's loudness amplitude into its shipped document, then rebuild the pack
	cd src/go && $(GO) run ./cmd/measure-loudness -write
	$(MAKE) asset-pack
	@echo "updated render.amplitude in internal/assets/instruments/*.json + rebuilt the pack"

# ─── Synth Matching / Fingerprinting ─────────────────────────────────────────

.PHONY: synth-match synth-fingerprint fetch-refs refs-doctor refs-families refs-adopt refs-adopt-all refs-verify refs-check scores-verify scores-notice templates ab ab-list ab-all ab-notes wav-match wav-match-map wav-match-nuisance wav-match-validate wav-match-calibrate ab-match probe-sounds refs-validate song-timeline song-render song-compare

synth-match: $(C_LIB) ## Tune a synth toward a reference WAV (INST=, REF=, [BUDGET=], [RESTARTS=], [DRY_RUN=1])
	cd src/go && $(GO) run ./cmd/synth-match -inst $(INST) -ref $(REF) \
		$(if $(BUDGET),-budget $(BUDGET),) $(if $(RESTARTS),-restarts $(RESTARTS),) $(if $(DRY_RUN),-dry-run,)

synth-fingerprint: $(C_LIB) ## Print the analysis fingerprint of a WAV (FILE=)
	cd src/go && $(GO) run ./cmd/synth-analyze ref -file $(FILE)

fetch-refs: $(C_LIB) ## Fetch reference audio into the corpus ([TIER=instrument|song], [ID=], [INSTRUMENT=], [ONLY_MISSING=1], [FORCE=1])
	cd src/go && $(GO) run ./cmd/refs fetch \
		$(if $(TIER),-tier $(TIER),) $(if $(ID),-id $(ID),) \
		$(if $(INSTRUMENT),-instrument $(INSTRUMENT),) \
		$(if $(ONLY_MISSING),-only-missing,) $(if $(FORCE),-force,)

refs-doctor: $(C_LIB) ## Report reference coverage for every instrument and template
	cd src/go && $(GO) run ./cmd/refs doctor

refs-families: $(C_LIB) ## List every instrument family, the reference its members are auditioned against, and the last ear verdict (INST=<family-or-instrument> for one)
	cd src/go && $(GO) run ./cmd/refs families \
		$(if $(INST),-instrument '$(INST)',)

# refs-adopt points ONE instrument at a recording of your own. The file is
# not copied into the tree: it goes through the same deterministic ffmpeg
# convert every catalogued reference goes through, lands in
# $BEATMO_WAV_REFS, and is measured + hashed by the tool. Only the
# provenance (source path, trim, measurements) is committed, in
# internal/wavref/catalogue.json. Reference audio is local dev tooling that
# never ships, so any recording you have will do -- there is no licence.
refs-adopt: $(C_LIB) ## Point an instrument at your own WAV (WAV=, [INST=] defaults to the filename, [NOTE=], [ID=], [F0=] pins the fundamental past a bad detection, [START_SEC=], [MAX_SEC=])
	@test -n "$(WAV)" || { echo "refs-adopt: WAV is required: a filename inside $(BEATMO_WAV_REFS)/sources, or any path -- or use \`make refs-adopt-all\`" >&2; exit 1; }
	cd src/go && $(GO) run ./cmd/refs adopt \
		$(if $(INST),-instrument '$(INST)',) -wav '$(WAV)' $(if $(NOTE),-note '$(NOTE)',) \
		$(if $(ID),-id '$(ID)',) $(if $(F0),-f0 $(F0),) \
		$(if $(START_SEC),-start-sec $(START_SEC),) $(if $(MAX_SEC),-max-sec $(MAX_SEC),)

# Bulk form: name each file after the instrument it is for (snare-lofi.wav,
# kick-deep_take3.wav), drop them all in, run this once.
refs-adopt-all: $(C_LIB) ## Adopt every resolvable WAV waiting in $(BEATMO_WAV_REFS)/sources ([NOTE=], [START_SEC=], [MAX_SEC=])
	cd src/go && $(GO) run ./cmd/refs adopt -scan $(if $(NOTE),-note '$(NOTE)',) \
		$(if $(START_SEC),-start-sec $(START_SEC),) $(if $(MAX_SEC),-max-sec $(MAX_SEC),)

refs-verify: $(C_LIB) ## Hash every local reference against the catalogue
	cd src/go && $(GO) run ./cmd/refs verify

refs-check: $(C_LIB) ## Re-derive every corpus calibration (thresholds, organic range, f0 confidence, audition candidates) against the local refs and fail on drift ([CHECKS=a,b], [V=1]); never part of `go test`
	cd src/go && $(GO) run ./cmd/refs check \
		$(if $(CHECKS),-only $(CHECKS),) $(if $(V),-v,)

scores-verify: ## Re-fetch every reference SCORE upstream and check it against the catalogue hash
	scripts/verify_scores.sh $(ID)

scores-notice: ## Regenerate internal/scoreref/NOTICE-scores.md from the score catalogue
	cd src/go && $(GO) test -tags test -modfile=go.test.mod \
		./internal/scoreref/ -run TestNoticeIsUpToDate -update

templates: $(C_LIB) ## Regenerate the built-in template circuits from the committed reference scores
	cd src/go && $(GO) run ./cmd/gen-showcase

# BEATMO_WAV_REFS is the corpus dir `refs compare` needs to resolve reference
# audio; default it so `make ab` works out of the box, and export it so the
# `go run` subprocess below (which reads it via os.Getenv) sees it.
BEATMO_WAV_REFS ?= $(HOME)/.beatmo/wav-refs
export BEATMO_WAV_REFS
# Raw recordings you want to adopt go in $(BEATMO_WAV_REFS)/sources -- one
# root, one variable. The root itself holds what the tooling PRODUCED
# (converted, trimmed, hashed into catalogue.json), which is the only audio
# anything reads back; sources/ sits beside candidates/, compare/ and
# probes/ and is invisible to lookups until adopted.
# `open` resolves to `xdg-open` on this machine; fall back explicitly in case
# it's ever missing. A variable so swapping players is a one-line change.
OPEN ?= $(shell command -v open >/dev/null 2>&1 && echo open || echo xdg-open)

ab: $(C_LIB) ## A/B listen: real reference vs Beatmo's synthesized version, played sequentially (INST=; see `make ab-list`)
	@test -n "$(INST)" || { echo "ab: INST is required, e.g. \`make ab INST=oboe\` -- see \`make ab-list\` for valid ids" >&2; exit 1; }
	@bash -c '\
	  set -e; \
	  PLAN="$$(cd src/go && $(GO) run ./cmd/refs compare -instrument $(INST) -sh)"; \
	  eval "$$PLAN"; \
	  echo "playing REAL then SYNTH, one after the other -- open/xdg-open hands off to a player and returns immediately, so we sleep the clip'"'"'s duration between the two"; \
	  echo "▶ REAL  $$BEATMO_AB_REF -> $$BEATMO_AB_REAL"; \
	  $(OPEN) "$$BEATMO_AB_REAL" >/dev/null 2>&1 || true; \
	  sleep "$$BEATMO_AB_DUR"; \
	  sleep 0.6; \
	  echo "▶ SYNTH $(INST) -> $$BEATMO_AB_SYNTH"; \
	  $(OPEN) "$$BEATMO_AB_SYNTH" >/dev/null 2>&1 || true'

# probe-sounds plays the isolated-property stimuli in order so the listener can
# name each property and say where they first hear a step. Same OPEN + sleep
# hand-off as `ab`: Go renders and prints an eval-able plan, the shell plays it.
probe-sounds: $(C_LIB) ## Play the isolated-property probe tones in order so you can name each property and where you first hear a difference (SET=<name> for one set; see `refs probe` for the list)
	@bash -c '\
	  set -e; \
	  PLAN="$$(cd src/go && $(GO) run ./cmd/refs probe $(if $(SET),-set $(SET),) -sh)"; \
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
	  echo "done -- `cd src/go && $(GO) run ./cmd/refs probe` prints the measured value for every step"'

ab-list: $(C_LIB) ## List every instrument comparable via `make ab INST=...`
	cd src/go && $(GO) run ./cmd/refs compare -list

# AUDITION_LOOP is the shared driver behind `ab-all` and `ab-notes`.
#
# The two used to be ~40 lines of byte-identical shell differing only in the
# flags handed to `refs audition` -- the second even carried a comment saying
# it was structurally a copy of the first. One `define` means a fix to the
# replay/quit/skip protocol lands once instead of twice.
#
# $(1) = label used in progress messages
# $(2) = mode flags passed to BOTH `refs audition` invocations
#
# FAMILIES=a,b,c restricts the session to a named subset -- the shape a re-audit
# takes after a render or level-match change invalidates specific families'
# verdicts. Combine with RESUME=false so families that already have a verdict
# are offered again; `refs audition` supersedes the old row rather than
# appending a second one.
#
# A PREBUILT binary, not `go run`: `go run` swallows the child's real exit code
# (it always reports 1 on any nonzero exit), and this loop has to tell "replay"
# (4) apart from "quit" (3) apart from "recorded/skipped" (0) by that exact
# code. Rebuilt every invocation so it never runs stale.
#
# Two `refs audition` calls per family, exactly mirroring `ab`'s own
# render-then-play split: `-sh` renders + prints the eval-able BEATMO_AB_* plan
# (Go never touches a player itself); the second, no-`-sh` call owns the
# interactive prompt on the real stdin/tty and records the answer. The whole
# loop is ONE shell invocation -- a Makefile recipe line starts a fresh shell
# each time, so looping across recipe lines could not share the piped-answers
# stdin the way one `bash -c` can.
#
# PLAN is ASSIGNED before it is eval'd, deliberately: `eval "$$(cmd)"` does not
# trip `set -e` when cmd fails (eval's own status is what counts), so a failed
# render would fall through to playing an empty path. A bare assignment does
# trip it.
define AUDITION_LOOP
	@cd src/go && $(GO) build -o /tmp/beatmo-refs-audition ./cmd/refs
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

# ab-notes is a SECOND PASS over `make ab-all`'s recorded verdicts: it never
# re-asks the verdict, only WHY -- it replays the same reference+options for a
# family that already has a verdict, then prompts for prose (updating the
# existing JSONL row in place via `refs audition -notes`).
ONLY ?= n,p
REANNOTATE ?= false
ab-notes: ## Second pass over `make ab-all`: capture WHY a family sounds wrong as prose notes, without re-asking the verdict (answers -> src/go/internal/audio/testdata/ear_verdicts.jsonl; ONLY=n,p filters which verdict classes to revisit, REANNOTATE=true to revisit families that already have a note, INST=<family> for just one, FAMILIES=a,b,c for a named subset)
	$(call AUDITION_LOOP,ab-notes,-notes -only $(ONLY) $(if $(filter true,$(REANNOTATE)),-reannotate,))

# wav-match / ab-match: which shipped instrument is closest to a recording
# nobody has catalogued (a download, a take of your own)? `refs match`
# converts each file like a fetched reference, measures its pitch, plays every
# instrument the way a player would to imitate it -- at the recording's note
# (an unpitched sound: whichever pitch within an octave sounds closest), held
# as long as the recording sounds -- and ranks by instrument ESSENCE (colour,
# its evolution from attack to body, noise texture, how the note dies away;
# the attack counts lightly). Pitch, note length, level and room tone are
# removed by construction (docs/superpowers/specs/2026-10-17-wav-match-essence-design.md).
# The table shows what each recording is LABELLED as (its catalogue entry, else
# an instrument its filename names) and where that family ranked.
# Rankings are cached in $(BEATMO_WAV_REFS)/matches/manifest.json by file hash
# + roster sound hash, so a re-run only analyses new or changed files and
# anything retuned. Nothing touches catalogue.json -- adopting a file stays
# `make refs-adopt`. `make wav-match-validate` grades the matcher on the
# catalogued references.
#
# WAVS is a GLOB (or several, comma-separated) handed to Go unexpanded
# (quoted), so spaces and "(1)" duplicates in download names survive;
# byte-identical files are listed once.
WAVS ?= $(HOME)/Downloads/*.wav
wav-match: $(C_LIB) ## Rank every shipped instrument against your own WAVs and write level-matched clips of the closest ones (WAVS='<glob>[,<glob>...]' default ~/Downloads/*.wav, [TOP=3], [FORCE=1] re-analyses)
	cd src/go && $(GO) run ./cmd/refs match -wavs '$(WAVS)' \
		$(if $(TOP),-top $(TOP),) $(if $(FORCE),-force,)

# wav-match-map: the coverage map, both ways round -- every recording's
# closest voices against what the catalogue (or its filename) says it is, and
# every roster instrument as MATCHED (a recording of it ranks it top 3),
# FIDELITY GAP (recordings of it exist, the synth ranks lower; the map says
# which essence term loses it and whether the voice is darker/noisier/dies
# faster), STAND-IN (no recording of its own, closest voice to another) or
# UNCOVERED. Default set: your downloads + every catalogued instrument
# reference (`catalogue`; byte-identical files are matched once).
MAP_WAVS ?= $(HOME)/Downloads/*.wav,catalogue
wav-match-map: $(C_LIB) ## Map recordings <-> instruments: matches, fidelity gaps, stand-ins and uncovered instruments -> $$BEATMO_WAV_REFS/matches/MAP.md (MAP_WAVS='<glob>[,<glob>|catalogue...]' default downloads + catalogued refs, [FORCE=1])
	cd src/go && $(GO) run ./cmd/refs match -map -wavs '$(MAP_WAVS)' $(if $(FORCE),-force,)

wav-match-nuisance: $(C_LIB) ## Re-play shipped voices with ONE nuisance each (pitch +/-5, a note cut short, room, reverb, bright/dark mic) and report whether the matcher still finds the voice ([CASES=a,b | all])
	cd src/go && $(GO) run ./cmd/refs match -nuisance $(if $(CASES),-cases $(CASES),)

wav-match-validate: $(C_LIB) ## Grade `refs match` on every catalogued reference (leave-one-out): own family #1 / top-3 / MRR and the broad KIND of the first-ranked voice
	cd src/go && $(GO) run ./cmd/refs match -validate

wav-match-calibrate: $(C_LIB) ## Re-derive the matcher's hubness calibration (src/go/cmd/refs/testdata/match_hubness.json) over the catalogued references -- after retuning or adding instruments; checked by `make refs-check`
	cd src/go && $(GO) run ./cmd/refs match -calibrate

# ab-match plays each recording, then its closest instrument (blocking player:
# paplay/aplay/ffplay, or $$BEATMO_PLAYER), and asks y/p/n with a free-text note
# about the SYNTH; a rejection plays the runners-up and asks whether one is
# closer. Answers -> src/go/internal/audio/testdata/wav_match_verdicts.jsonl,
# one row per recording (re-judging supersedes), pinned to the instrument's
# sound hash so a later retune can tell its verdict is stale.
# ONLY is honoured only when YOU set it: the Makefile's own `ONLY ?= n,p`
# (ab-notes' verdict-class filter) would otherwise match no filename at all.
ab-match: ## Listen to each WAV next to its closest instrument and approve/reject it with notes (runs wav-match first; WAVS='<glob>', ONLY=<filename text>, RESUME=false re-asks answered files; answers -> src/go/internal/audio/testdata/wav_match_verdicts.jsonl)
	@cd src/go && $(GO) build -o /tmp/beatmo-refs-match ./cmd/refs
	@cd src/go && /tmp/beatmo-refs-match match -listen -wavs '$(WAVS)' -resume=$(RESUME) \
		$(if $(TOP),-top $(TOP),) $(if $(filter-out file,$(origin ONLY)),-only '$(ONLY)',)

ab-session: ## Audition round: reference + current + two candidates per family, rated and described from ONE word salad shared by every instrument (FAMILIES=a,b; DEEP=1 also rates the instrument's own words 1-5; resumes at the first family not yet reviewed for its current candidates, RESUME=false asks all again; CLONE=1 adds each family's WAV clone as candidate K with a y/n/p accept; answers -> src/go/internal/audio/testdata/ear_sessions.jsonl)
	@cd src/go && $(GO) build -o /tmp/beatmo-refs-audition ./cmd/refs
	@cd src/go && /tmp/beatmo-refs-audition session -resume=$(RESUME) $(if $(FAMILIES),-families $(FAMILIES),) $(if $(CLONE),-clone,) $(if $(DEEP),-deep,)

# tune / ab-tuned / tune-adopt: essence-driven candidates for the curated
# picks (src/go/cmd/refs/testdata/tune_picks.json: the listener's notes + the
# match map's gaps). `refs tune` moves a document's own values toward EVERY
# recording of the instrument (catalogue + any $(TUNE_WAVS) file that names it)
# by the essence distance, writes $$BEATMO_WAV_REFS/tuned/<inst>/ (candidate +
# report.md), and never touches a shipped document. ab-tuned plays
# reference / current / tuned per family exactly as the search scored them
# (held as long as the recording; clips -> compare/<inst>/TUNED_AB__*.wav)
# and asks for a y/n/p; tune-adopt
# ships a candidate only after the newest verdict on that exact doc_sha is y.
TUNE_WAVS ?= $(HOME)/Downloads/*.wav
tune: ## Essence-tune ONE instrument into a candidate document (INST=<id>[,<id>], [GENS=40] [SEED=1] [HARMONICS=N] [TUNE_WAVS=<glob>]; writes $$BEATMO_WAV_REFS/tuned/<inst>/)
	@test -n "$(INST)" || { echo "tune: INST is required (or \`make tune-picks\`)" >&2; exit 1; }
	@cd src/go && $(GO) build -o /tmp/beatmo-refs-tune ./cmd/refs
	@cd src/go && /tmp/beatmo-refs-tune tune -inst $(INST) -wavs '$(TUNE_WAVS)' \
		$(if $(GENS),-gens $(GENS),) $(if $(SEED),-seed $(SEED),) $(if $(HARMONICS),-add-harmonics $(HARMONICS),)

tune-picks: ## Essence-tune every curated pick (src/go/cmd/refs/testdata/tune_picks.json) into candidates, then listen with `make ab-tuned` ([PICKS=a,b] [GENS=40] [SEED=1])
	@cd src/go && $(GO) build -o /tmp/beatmo-refs-tune ./cmd/refs
	@cd src/go && /tmp/beatmo-refs-tune tune -picks $(if $(PICKS),-inst $(PICKS),) -wavs '$(TUNE_WAVS)' \
		$(if $(GENS),-gens $(GENS),) $(if $(SEED),-seed $(SEED),)

ab-tuned: ## A/B by ear: reference, then the CURRENT instrument, then its essence-tuned candidate T, per family; rate it, pick its words from the shared word salad, and y/n/p whether it ships ([FAMILIES=a,b] default every tuned candidate; DEEP=1 also rates the instrument's own words 1-5; resumes at the first family not yet reviewed for its current candidate, RESUME=false asks all again, a family in FAMILIES is always asked; answers are appended -> src/go/internal/audio/testdata/ear_sessions.jsonl)
	@cd src/go && $(GO) build -o /tmp/beatmo-refs-audition ./cmd/refs
	@cd src/go && /tmp/beatmo-refs-audition session -tuned -resume=$(RESUME) $(if $(FAMILIES),-families $(FAMILIES),) $(if $(DEEP),-deep,)

tune-adopt: $(C_LIB) ## Ship a tuned candidate into internal/assets/instruments — ONLY after `make ab-tuned FAMILIES=<inst>` recorded y for that exact candidate (INST=<instrument>)
	@test -n "$(INST)" || { echo "tune-adopt: INST is required" >&2; exit 1; }
	@cd src/go && $(GO) run ./cmd/clone adopt -kind tuned -inst $(INST)

clone: $(C_LIB) ## Clone a struck/modal reference into a candidate instrument document: analysis → invert → CMA-ES refine (INST=<instrument>, [REF=] [WAV=] [F0=] [AS=<newid> | INTO=<id>] [NO_REFINE=1] [SEED=] [GENS=]; writes $$BEATMO_WAV_REFS/clones/<inst>/)
	@cd src/go && $(GO) run ./cmd/clone -inst $(INST) $(if $(REF),-ref $(REF),) $(if $(WAV),-wav $(WAV),) $(if $(F0),-f0 $(F0),) \
		$(if $(AS),-as $(AS),) $(if $(INTO),-into $(INTO),) $(if $(NO_REFINE),-no-refine,) $(if $(SEED),-seed $(SEED),) $(if $(GENS),-gens $(GENS),)

clone-adopt: $(C_LIB) ## Ship a clone into internal/assets/instruments — ONLY after `make ab-session FAMILIES=<inst> CLONE=1` recorded accept=y for that exact candidate (INST=<instrument>)
	@cd src/go && $(GO) run ./cmd/clone adopt -inst $(INST)

# The slice-1 benchmark (spec §6b): local corpus only, never part of `go test`.
CLONE_BENCH ?= tubular-bells kalimba glockenspiel cowbell marimba xylophone vibraphone tom conga
clone-bench: ## Clone every slice-1 benchmark instrument and print the summary (CLONE_BENCH="a b" to choose; verdicts are the ear's — `make ab-session FAMILIES=… CLONE=1`)
	@cd src/go && $(GO) build -o /tmp/beatmo-clone ./cmd/clone
	@for i in $(CLONE_BENCH); do (cd src/go && /tmp/beatmo-clone -inst $$i $(if $(GENS),-gens $(GENS),)); done

song-timeline: $(C_LIB) ## Second-by-second analysis of a WAV (FILE=, [HOP=], [CONFIG=], [OUT=])
	cd src/go && $(GO) run ./cmd/song-match timeline -file $(FILE) \
		$(if $(HOP),-hop $(HOP),) $(if $(CONFIG),-config $(CONFIG),) $(if $(OUT),-out $(OUT),)

song-render: $(C_LIB) ## Render a template to master.wav + per-instrument stems (TEMPLATE=, OUT=, [BARS=], [SR=])
	cd src/go && $(GO) run ./cmd/song-match render -template $(TEMPLATE) -out $(OUT) \
		$(if $(BARS),-bars $(BARS),) $(if $(SR),-sr $(SR),)

song-compare: $(C_LIB) ## Compare a template to a reference WAV (TEMPLATE=, REF=, [MANIFEST=], [CONFIG=], [TUNE=1], [DRY_RUN=1])
	cd src/go && $(GO) run ./cmd/song-match compare -template $(TEMPLATE) -ref $(REF) \
		$(if $(MANIFEST),-manifest $(MANIFEST),) $(if $(CONFIG),-config $(CONFIG),) \
		$(if $(TUNE),-tune,) $(if $(DRY_RUN),-dry-run,)

# ─── WASM Builds ─────────────────────────────────────────────────────────────

# `make wasm` is the SHIPPED binary (deploy-wasm-gcs.sh builds through it and
# TestWasmAssetSizeBudget measures it): -tags noscenes leaves out the screenshot
# scene catalog (internal/ui/scene_catalog*.go, ~150 KB of code no visitor
# runs). `make wasm-dev` is the same build WITH the catalog, for whatever
# drives runScene/listScenes: test-browser, the screenshot targets.
# `release` hides the developer-only surfaces (the Chain tab, the Synth tab's
# low-level controls; internal/ui/devtools.go) — compiled in, hidden from view.
# Comma-separated: `-tags` takes ONE argument.
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

# Serve the WASM build on the LAN (0.0.0.0) and print a URL + QR code so you
# can run the existing Playwright audio tests on your phone via the in-page
# test runner (src/js/test_runner.html). The runner loads every scenario
# module from src/js/scenarios/ and exposes a button per scenario.
#   make serve-lan                        -> binds to 0.0.0.0:8089
#   BEATMO_LAN_PORT=9000 make serve-lan   -> custom port
# Port 8089 is offset from `make serve` (8080) so both can run together.
# Hit "/" for the full app or "/?run=tests" for the test runner.
# Record the landing page's showcase videos (desktop + phone) from the real
# game's scripted tour: src/js/landing/{desktop,phone}.{mp4,webp,vtt}. Manual:
# run it when the video should change (CUTS="desktop" for one cut).
.PHONY: showcase-video
showcase-video:
	./scripts/showcase_video.sh

# Serve the app the way beatmo.io serves it (landing_server.js: hashed
# main.wasm stamped into index.html, compressed, video with Range), so the
# landing tutorial demo shows on EVERY load until "Do not show tutorial demo
# again" is ticked; /?landing=1 always shows it. Port 8090
# (BEATMO_LANDING_PORT); BEATMO_LANDING_HOST=0.0.0.0 to open it from a phone.
.PHONY: serve-landing
serve-landing: wasm
	node src/js/landing_server.js

.PHONY: serve-lan
serve-lan: wasm
	@./scripts/serve-lan.sh

generate: gen-design-tokens
	cd src/go && $(GO) generate ./internal/audio/...

# Regenerate UI design tokens from DESIGN.md (Phase 0: primitives only).
# Source of truth: DESIGN.md YAML front matter.
# Output: src/go/internal/ui/design_tokens.gen.go (committed).
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

# Regenerate the instrument fundamentals table (committed).
#
# Renders every built-in instrument through the real C DSP and measures its
# sounding fundamental, so the UI can name the note a node plays for EVERY
# instrument rather than only the curated melodic set. Needs CGO and the
# prebuilt C library, which is why it is a manual target rather than part of
# `generate` — the browser build cannot produce these numbers at all.
#
# TestInstrumentFundamentalsUpToDate (run by `make test-real`) fails when the
# committed table drifts from a re-measurement, so a retuned instrument cannot
# silently leave a wrong note name behind.
.PHONY: gen-fundamentals
# Which Synth-tab dials each shipped instrument shows is decided by the DSP:
# a dial whose 0→100 sweep is inaudible on an instrument's sound is hidden
# there (internal/ui/synth_dial_relevance.go). This re-measures every shipped
# sound on the real engine and rewrites the table the app embeds; run it after
# a tuning pass, a new instrument, a dial edit or an engine change
# (TestDialRelevanceTableIsCurrent fails until you do).
.PHONY: gen-dial-relevance
gen-dial-relevance: $(C_LIB)
	cd src/go && DIAL_RELEVANCE_UPDATE=1 CGO_ENABLED=1 xvfb-run -a $(GO) test -count=1 -run 'TestDialRelevanceTableIsCurrent$$' -v ./internal/ui/ | grep -E 'wrote|FAIL|ok'

gen-fundamentals: $(C_LIB)
	cd src/go && CGO_ENABLED=1 $(GO) run ./cmd/gen-fundamentals \
		-o internal/audio/instrument_fundamentals.gen.go
	cd src/go && $(GO) fmt ./internal/audio/instrument_fundamentals.gen.go >/dev/null

# Print the fundamentals table as a human-readable report (what each instrument
# measured, which source won, and the confidence behind it).
.PHONY: fundamentals-report
fundamentals-report: $(C_LIB)
	cd src/go && CGO_ENABLED=1 $(GO) run ./cmd/gen-fundamentals -report

# Install local git hooks: the pre-commit dispatcher rejects any commit that
# adds or modifies an audio file (no-audio-files.sh) and any commit with stale
# generated design tokens (gen-design-tokens.sh).
.PHONY: install-hooks
install-hooks:
	@if [ ! -d .git ]; then echo "not a git repo"; exit 1; fi
	@mkdir -p .git/hooks
	@cp scripts/precommit/pre-commit .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "installed pre-commit hook (rejects audio files; regenerates design tokens, rejects stale)"

fmt:
	cd src/go && $(GO) fmt ./...

# ─── Dependency Graph ────────────────────────────────────────────────────────
# Generate internal package structure graph with types and functions.
# Outputs DOT + SVG/PNG (if graphviz installed).
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

# ─── Go Graph (interactive architecture visualizer) ──────────────────────────
# Emit a self-contained interactive HTML view of packages/types/functions with
# LOC sizing, nesting/cyclomatic/call-graph depth, and type-resolved call edges.
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

# ─── Refactor ranking (gograph rank) ─────────────────────────────────────────
# Rank refactoring candidates: correct per-build-config metrics (linux + js/wasm),
# cognitive complexity, clones, RTA reachability (dead vs test-only over the
# tagged AND untagged test builds), god-type extraction readiness and compounded
# area complexity → build/refactor/{rank.md,rank.json,rank.html}.
# Spec: docs/superpowers/specs/2026-10-19-refactor-rank-design.md
# Usage: make refactor-rank [RANK_FAST=1] [RANK_COVER="coverage/go.out coverage/go-native.out"]
#                           [RANK_BASELINE=old/rank.json] [RANK_TOP=40]
#        make refactor-packets [RANK_PACKETS=40]   (inputs for the refactor-assess workflow)
# Coverage profiles that exist are picked up automatically; agent verdicts are read
# from build/refactor/verdicts/ (local only, never committed).
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

# The gograph tool's own tests (fixture modules; a few seconds). Part of `make test`
# so an x/tools / Go upgrade that breaks the tool fails the day it happens.
gograph-test:
	cd scripts/gograph && $(GO) test ./...

# The CI Build job runs exactly this target, and `make test` runs it first, so a
# tree that builds locally builds in CI. js/wasm builds ./... (not just ./cmd):
# a native-only file tagged `!test` instead of `!test && !js` breaks only there.
.PHONY: ci-build
ci-build: $(C_LIB)
	cd src/go && CGO_ENABLED=1 $(GO) build ./...
	cd src/go && GOOS=js GOARCH=wasm $(GO) build ./...
	$(MAKE) check-ma-js
	$(MAKE) gograph-test
	$(MAKE) js-audit

# ─── CI images (ci/versions.env + ci/images/) ───────────────────────────────
# `make dev TARGET=test` runs any target in the CI image (podman or docker);
# `make doctor` checks that the host's tools match the pins.
CONTAINER ?= $(shell command -v podman 2>/dev/null || command -v docker 2>/dev/null)
KIND ?= build
IMAGE_REF = $(shell scripts/ci/image-tag.sh $(KIND))
.PHONY: image-tag image-publish dev doctor
image-tag:
	@echo $(IMAGE_REF)

image-publish:
	scripts/ci/image-publish.sh

dev:
	@test -n "$(CONTAINER)" || { echo "make dev needs podman or docker"; exit 1; }
	$(CONTAINER) run --rm -i $$([ -t 0 ] && echo -t) --init --ipc=host --user $$(id -u):$$(id -g) -e HOME=/tmp \
	  -v $(CURDIR):$(CURDIR) -w $(CURDIR) \
	  -v beatmo-gocache:/cache/go-build -v beatmo-gomod:/cache/gomod -v beatmo-npm:/cache/npm \
	  $(IMAGE_REF) make $(TARGET)

doctor:
	@scripts/ci/doctor.sh

# js-audit: no value js.ValueOf cannot convert (a typed id or enum) reaches
# syscall/js — it panics in the browser only, invisible to the compiler and to
# every native test. See scripts/jsaudit/main.go.
js-audit:
	cd scripts/jsaudit && $(GO) test ./... && $(GO) run . $(CURDIR)/src/go ./...

# Comment census + policy lint: make comment-census [ARGS="--check" | "--top 60" | "--json"].
.PHONY: comment-census test-comment-census
comment-census:
	python3 scripts/comment_census.py $(ARGS)

test-comment-census:
	python3 scripts/comment_census_test.py

# ─── Linting ─────────────────────────────────────────────────────────────────
GOLANGCI_LINT ?= $(CURDIR)/.tools/golangci-lint

$(CURDIR)/.tools/golangci-lint:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/v$(GOLANGCI_LINT_VERSION)/install.sh | sh -s -- -b .tools v$(GOLANGCI_LINT_VERSION)

# `unused` runs per build configuration (default, -tags test, js, js release):
# one configuration alone reports every test-only or js-only helper as dead.
lint-go: $(GOLANGCI_LINT)
	cd src/go && $(GOLANGCI_LINT) run --disable unused ./...
	cd src/go && GO=$(GO) GOLANGCI_LINT=$(GOLANGCI_LINT) python3 ../../scripts/ci/lint-unused.py

lint-go-fix: $(GOLANGCI_LINT)
	cd src/go && $(GOLANGCI_LINT) run --fix ./...

JS_DEPS = src/js/node_modules/.package-lock.json
$(JS_DEPS): src/js/package-lock.json
	cd src/js && npm ci
	@touch $@
js-deps: $(JS_DEPS)

# Tracked files only: a scratch script in the working tree is not part of what CI lints.
lint-js: $(JS_DEPS)
	cd src/js && git ls-files -z -- '*.js' '*.mjs' | xargs -0 npx eslint --no-warn-ignored

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

# Capture every UI surface in the scene catalog.
# Output goes to screenshots/all/ (override with OUTDIR=).
# Set MOBILE=1 to add a mobile-viewport browser pass.
# Set SCENES=name1,name2 to filter to specific scenes.
screenshots-all: $(C_LIB) wasm-dev
	GO=$(GO) OUTDIR=$(or $(OUTDIR),screenshots/all) MOBILE=$(or $(MOBILE),0) \
		SCENES=$(or $(SCENES),) ./scripts/capture_all_ui.sh

# Alias used by docs / future baseline workflows.
screenshots-all-update: screenshots-all
	@echo "screenshots refreshed"

# Curated minimal capture: one screenshot per distinct surface / popup / tab
# (scene list in scripts/basic_scenes.txt; drift-guarded by
# internal/ui/basic_scenes_test.go). Mobile pass on by default.
# Output goes to screenshots/basic/ (override with OUTDIR=).
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

# Go microbenchmarks for the timeline-navigation seek path (forward/backward
# seeks + timeline edits during playback). Results assessed in
# bench-results/seek_navigation_bench.md. Override BENCHTIME/COUNT.
bench-seek:
	cd src/go && $(GO) test -tags test -modfile=go.test.mod -run '^$$' \
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

# Render-latency bench: raw WAV vs synth vs synth-under-param-storm trigger
# latency (deferred-play deviation + per-voice render cost). Produces
# bench-results/render_latency.json. RENDER_BENCH_STRICT=1 gates the storm
# scenarios' deferAbsMsP90 at the real-time bar.
bench-render: wasm
	@mkdir -p bench-results
	GO=$(GO) node src/js/webaudio_render_latency_bench.browser.test.js

# Audio-isolation bench: the startup demo under UI loads (synthetic main-thread
# stalls, heavy tabs, zoom/pan storms, main-thread vs worklet insert FX) with
# the playback probe measuring what is HEARD (onset lag vs the predictor,
# rhythm error, silent/dropped notes, clicks, audio-thread underruns).
# Produces bench-results/audio_isolation.json. AUDIO_PROBE_STRICT=1 enforces
# the lateness limits; SCENARIOS=a,b filters.
bench-isolation: wasm
	@mkdir -p bench-results
	ISOLATION_SCENARIOS=$(SCENARIOS) GO=$(GO) WASM_PREBUILT=1 node src/js/webaudio_audio_isolation_bench.browser.test.js

# Interleaved A/B/C of the audio lane (spec §7): frozen baseline build, new
# build lane-off, new build lane-on, rotated ROUNDS times with load recorded.
# AB_SCRATCH must hold baseline_site/ (frozen build + JS) and lane.wasm
# (cd src/go && GOOS=js GOARCH=wasm go build -o $AB_SCRATCH/lane.wasm ./cmd).
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

# perf-record-bench: record-while-playing benchmark with profiling.
# Drops + perf_stats land in bench-results/record-<timestamp>/. After
# the run, scripts/compare-bench.sh validates against the baseline at
# bench-results/REPORT.md (if present) and exits non-zero on regression.
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
	$(MAKE) ci-build
	$(MAKE) test-go-tagged
	$(MAKE) test-audio-real

# -timeout is per package binary; internal/ui sets the pace (its soak/leak
# canaries are -tags test only, so this is their one full-length runner).
# COVER=1 also writes coverage/go.out and coverage/test-output.txt.
TEST_GO_TAGGED_TIMEOUT ?= 1500s
test-go-tagged: $(C_LIB)
	@mkdir -p coverage
	cd src/go; $(LOG_ENV) $(GO) test -tags test -modfile=go.test.mod -timeout $(TEST_GO_TAGGED_TIMEOUT) \
	  $(if $(COVER),-coverprofile=../../coverage/go.out -covermode=atomic) $(or $(PKGS),./...) \
	  $(if $(COVER),> ../../coverage/test-output.txt 2>&1; s=$$?; cat ../../coverage/test-output.txt; exit $$s)

# Shard i of N of test-go-tagged / test-real-go (SHARD=i/N): same flags, so a
# CI shard reproduces locally. COVER=1 writes covdata for `make coverage-merge`.
test-go-shard: $(C_LIB)
	cd src/go && $(LOG_ENV) GO=$(GO) $(if $(COVER),COVERDIR=$(CURDIR)/coverage/covdata/$(subst /,-of-,$(SHARD))) \
	  ../../scripts/ci/go-shard.sh run $(SHARD) -tags test -modfile=go.test.mod -- \
	  -timeout $(TEST_GO_TAGGED_TIMEOUT) $(if $(COVER),-cover -covermode=atomic)

test-real-go-shard: $(C_LIB)
	cd src/go && BPM_TIMING_TEST=1 GO=$(GO) xvfb-run -a ../../scripts/ci/go-shard.sh run $(SHARD) -- -timeout 30m

# Merges every shard's covdata into coverage/go.out + per-package percentages.
coverage-merge:
	cd src/go && dirs=$$(ls -d ../../coverage/covdata/*/ | paste -sd, -) && \
	  $(GO) tool covdata textfmt -i=$$dirs -o ../../coverage/go.out && \
	  $(GO) tool covdata percent -i=$$dirs > ../../coverage/go-percent.txt

# The real C DSP under the untagged build; the budget is whole-package.
test-audio-real: $(C_LIB)
	cd src/go; $(LOG_ENV) $(GO) test -timeout 900s ./internal/audio

# Run the concurrency-relevant Go packages under the race detector (-race is
# native-only; it cannot instrument the js/wasm build, so this is the desktop
# -tags test path). Some packages are EXCLUDED because they are single-threaded
# numeric optimizers or giant suites that take 15-20 min under -race while
# adding no goroutine coverage:
#   internal/ui, internal/timeline                       — huge suites, >15 min under -race
#   audio/songmatch, audio/synthmatch, audio/fingerprint — Hooke-Jeeves / FFT, no goroutines
#   cmd/synth-analyze                                     — offline numeric analysis, no goroutines
#   internal/deploy                                       — asset-size/manifest checks, no goroutines
# Everything else runs here: the engine run loop + scheduler, async pools, the
# hooks bus, the recording lifecycle, the analyzer SPSC ring, the event
# sinks/loggers, and the scope-export service. Override the excludes with
# RACE_EXCLUDE= to widen coverage (e.g. `make test-race RACE_EXCLUDE=xxxxx`).
RACE_EXCLUDE ?= internal/ui$$|internal/timeline$$|audio/songmatch|audio/synthmatch|audio/fingerprint|cmd/synth-analyze|internal/deploy
.PHONY: test-race
test-race: $(C_LIB)
	cd src/go; $(LOG_ENV) $(GO) test -race -tags test -modfile=go.test.mod -timeout 900s \
	  $$($(GO) list -tags test -modfile=go.test.mod -f '{{if or .GoFiles .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./... | grep -vE '$(RACE_EXCLUDE)')

# Run all browser tests (auto-discovered). Timing-sensitive tests run sequentially
# after the parallel batch. BROWSER_JOBS=N controls parallelism (default: 4).
test-browser: wasm-all $(JS_DEPS)
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
# Same -tags test ./... runner as `make test`; internal/ui alone needs ~734s in
# isolation (see the note on the `test` target), so the old 300s cap could never
# have covered it. Match the `test` budget.
	cd src/go; $(LOG_ENV) $(GO) test -tags test -modfile=go.test.mod -timeout 1500s ./...
	cd src/go; $(LOG_ENV) $(GO) test -timeout 900s ./internal/audio
	$(MAKE) test-browser

# Run all Go tests with real Ebiten (requires X11/xvfb) + all browser tests.
# -timeout 30m (up from the 10m per-package default): internal/audio/synthmatch
# runs the Hooke-Jeeves optimizer against the REAL CGo synth + FFT fingerprint
# (TestMatch_* ~ 300/40/300 evals at ~1s each), so the package alone takes ~12m.
# Under -tags test (make test/test-debug) the synth is stubbed and stays fast, so
# only this real-build target needs the longer cap.
test-real: $(C_LIB)
	$(MAKE) test-real-go
	$(MAKE) test-browser

test-real-go: $(C_LIB)
	cd src/go && BPM_TIMING_TEST=1 xvfb-run -a $(GO) test -timeout 30m $(or $(PKGS),./...)

# CI's reference-corpus checks. The calibrations are derived from the WHOLE
# corpus, so when any reference cannot be fetched (upstream down, or a ref only
# its owner's disk has) refs-check skips loudly instead of reporting drift.
refs-ci: $(C_LIB)
	@if $(MAKE) fetch-refs TIER=instrument ONLY_MISSING=1; then fetched=1; else fetched=0; fi; \
	  cd src/go && $(GO) run ./cmd/refs verify || exit 1; cd ../..; \
	  if [ $$fetched = 1 ]; then $(MAKE) refs-check; else \
	    msg="refs-check SKIPPED: the reference corpus is incomplete (see the fetch FAIL lines), so the calibrations cannot be re-derived"; \
	    echo "::warning::$$msg"; [ -z "$$GITHUB_STEP_SUMMARY" ] || echo "- $$msg" >> "$$GITHUB_STEP_SUMMARY"; fi

# Writes the Go/C coverage tables to $GITHUB_STEP_SUMMARY (or stdout).
coverage-summary:
	@scripts/ci/coverage-summary.sh

# One browser matrix entry: TESTS="a.browser.test.js b.browser.test.js".
test-browser-batch: wasm-all $(JS_DEPS)
	GO=$(GO) ./scripts/run-browser-tests.sh --no-fail-fast --tests "$(TESTS)"

# Prints the browser matrix JSON (and hands it to the workflow under Actions).
browser-matrix:
	@m=$$(./scripts/browser-test-matrix.sh --batch-size $(or $(BATCH_SIZE),4)) && echo "$$m" && \
	  if [ -n "$$GITHUB_OUTPUT" ]; then echo "matrix=$$m" >> "$$GITHUB_OUTPUT"; fi

# Fails unless every job in $$NEEDS (the workflow's toJSON(needs)) succeeded.
ci-verdict:
	@scripts/ci/verdict.sh

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

# Regenerate golden baselines for visual regression tests
# Visual tests on real mobile devices via BrowserStack
# Requires: BROWSERSTACK_USERNAME and BROWSERSTACK_ACCESS_KEY env vars
# Usage: make test-visual-device
test-visual-device: wasm
	@if [ -z "$$BROWSERSTACK_USERNAME" ] || [ -z "$$BROWSERSTACK_ACCESS_KEY" ]; then \
		echo "ERROR: Set BROWSERSTACK_USERNAME and BROWSERSTACK_ACCESS_KEY"; exit 1; fi
	cd src/js && npm install --no-save 2>/dev/null; \
	GO=$(GO) node visual_device_parity.browser.test.js

# Real-device user-action bridge tests (WAV import, JSON import/export, ...)
# via BrowserStack, using real OS APIs (real <input type=file>, downloads).
# Requires: BROWSERSTACK_USERNAME and BROWSERSTACK_ACCESS_KEY env vars.
# Usage: make test-device-actions   (or DEVICE_FILTER=iPhone to narrow)
# Develop locally first (free): GO=$(GO) node src/js/device_user_actions.browser.test.js
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

# ─── LLM Visual Testing ──────────────────────────────────────────────────────
# Record video + events from any browser test for replay / visual comparison.
#
# Run a single test with recording:
#   make record-test TEST=circuit_sync
#   make record-test TEST=e2e_workflow NAME=my_session
#
# Run a filtered set of tests with recording:
#   make record-browser-filter FILTER=live_edit
#
# Interactive recording (opens browser, Ctrl+C to stop):
#   make record
#
# Replay a recorded session:
#   make replay NAME=my_session
#   make replay NAME=my_session SPEED=0.5
#
# Evaluate a recording with Claude:
#   ANTHROPIC_API_KEY=sk-... make evaluate NAME=my_session
#   ANTHROPIC_API_KEY=sk-... make evaluate NAME=my_session TEMPLATE=interaction_flow
#   ANTHROPIC_API_KEY=sk-... make evaluate NAME=my_session PROMPT="Is the grid visible?"

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

# Run Claude agent to visually interact with Beatmo in the browser.
# The agent takes screenshots, sends them to Claude, and executes actions.
#   make agent PROMPT="Click Play, wait 3 seconds, click Stop"
#   make agent PROMPT="Explore all UI controls" MAX_ITER=80 NAME=my_session
#   make agent PROMPT="Build a 4-node loop" MODEL=claude-haiku-4-5-20251001
#   make agent TEST=play_stop_basic
#   make agent TEST=pinch_zoom_mobile
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

# List available agent test cases.
#   make agent-list
#   make agent-list FILTER=smoke TAGS=transport PLATFORM=desktop
agent-list:
	@if ! command -v node >/dev/null 2>&1; then echo 'Node not found'; exit 1; fi
	@node src/js/llm_test/cli.js list-tests \
		$(if $(FILTER),--filter "$(FILTER)") \
		$(if $(TAGS),--tags "$(TAGS)") \
		$(if $(PLATFORM),--platform $(PLATFORM))

# Run a batch of agent test cases.
#   make agent-tests
#   make agent-tests TAGS=smoke
#   make agent-tests FILTER=play PLATFORM=desktop
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

# The coverage/ OUTPUT DIRECTORY shadows the `coverage` target name (and the
# sub-targets are not file-producing in make's eyes either): without .PHONY,
# `make coverage` reports "up to date" as soon as the directory exists and
# silently runs nothing.
.PHONY: coverage coverage-go coverage-go-check coverage-c coverage-c-check \
	coverage-browser coverage-js coverage-report coverage-graph

# Go test coverage (continues on test failures, warns instead of aborting)
coverage-go: $(C_LIB)
	@mkdir -p coverage
	@cd src/go; $(LOG_ENV) $(GO) test -tags test -modfile=go.test.mod \
		-coverprofile=../../coverage/go.out -covermode=atomic -timeout 360s ./... \
		|| echo "WARNING: Some Go tests failed — coverage data still collected"
	@if [ -f coverage/go.out ]; then cd src/go && $(GO) tool cover -func=../../coverage/go.out | tail -1; fi

# Coverage floor guard. Runs coverage-go, then fails CI if total Go coverage
# drops below COVERAGE_GO_FLOOR (default 80). Bump the floor when adding a
# significant batch of tests so future regressions are caught. Current
# baseline (2026-05-17) is 80.8% — combined effect of fixing the internal/ui
# goleak/timeout (Phase A of plan hey-please-run-the-rosy-cat.md) and a
# surgical pass over the largest untested UI files plus the timeline/audio
# 0%-functions. 80 leaves ~0.8pt headroom; tighten further when the next
# UI/audio test batch lands.
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

# C coverage (gcov over our own DSP sources in src/c/, miniaudio excluded).
# Builds an instrumented libdrums.a, runs the native (non -tags test) audio
# tests through the CGo bridge, emits a per-file table + annotated
# coverage/c/*.c.gcov (##### = uncovered) and ALSO captures Go coverage of the
# bridge build to coverage/go-native.out — the bridge files (drums_c.go,
# insert_fx_c.go, ...) are //go:build !test && !js, so coverage-go never sees
# them. Does NOT depend on $(C_LIB): the script swaps in an instrumented lib
# and removes it on exit so normal builds always relink a clean -O2 lib.
# Options: COVERAGE_C_FLOOR=NN enforces a floor; COVERAGE_C_XVFB=1 adds an
# xvfb-run pass for the display-requiring mixer tests.
coverage-c:
	@mkdir -p coverage/c
	CC=$(CC) GO=$(GO) COVERAGE_C_FLOOR=$(COVERAGE_C_FLOOR) \
	COVERAGE_C_XVFB=$(or $(COVERAGE_C_XVFB),0) ./scripts/coverage-c.sh

# Floor guard for C coverage, mirroring coverage-go-check (coverage-c only
# reports; this target enforces). 2026-10-02 (config-only engine Phase B, after
# the legacy-only tests + pan.c + test-only C were deleted): 99.18%. The only
# uncovered lines are defensive guards (drums.c load_audio decode failures,
# insert_fx.c NULL-buffer passthroughs, fmsynth.c past-envelope return). 99 is
# the floor; ratchet when the defensive set shrinks. coverage-c DELETES
# build/libdrums.a on exit — run `make build/libdrums.a` afterwards.
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
