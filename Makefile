GO ?= $(shell if [ -x $(CURDIR)/.tools/go/bin/go ]; then echo $(CURDIR)/.tools/go/bin/go; else echo go; fi)
WASM_OUT = ../js/main.wasm
MA_JS = src/js/drums.single.js
C_LIB = build/libdrums.a
C_SRC = src/c/drums.c src/c/miniaudio.c src/c/fmsynth.c src/c/effects.c src/c/insert_fx.c src/c/wavetable.c src/c/adsr.c src/c/pan.c src/c/noise.c src/c/modular.c src/c/modular_stages.c src/c/gcov_flush.c
# Test-only oracle shim (real ma_noise). Linked into the NATIVE archive so
# noise_ma_parity_test.go can compare the transplant against miniaudio, but
# deliberately kept OUT of $(C_SRC) — the emcc WASM build must not ship this
# test scaffold (nothing references it at runtime, and it exports no symbol).
C_SRC_TEST = src/c/noise_ma_oracle.c
C_HDRS = src/c/drums.h src/c/miniaudio.h src/c/fmsynth.h src/c/effects.h src/c/synth_params.h src/c/synth_post.h src/c/synth_dsp_primitives.h src/c/insert_fx.h src/c/wavetable.h src/c/adsr.h src/c/pan.h src/c/noise.h src/c/modular.h src/c/modular_stages.h

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

$(MA_JS): $(C_SRC) $(C_HDRS)
	@if command -v emcc >/dev/null 2>&1; then \
		emcc $(C_SRC) -O3 -DMA_ENABLE_MP3 -sWASM=1 -sSINGLE_FILE=1 -sENVIRONMENT=web -sASSERTIONS=0 -sALLOW_MEMORY_GROWTH=0 -sSTACK_SIZE=1048576 -sEXPORTED_FUNCTIONS='[_render_modular,_render_modular_p,_result_description,_delay_init,_delay_process,_delay_reset,_reverb_buffer_size,_reverb_init,_reverb_process,_reverb_reset,_ifx_distortion_init,_ifx_distortion_process,_ifx_distortion_reset,_ifx_distortion_set_param,_ifx_delay_init,_ifx_delay_process,_ifx_delay_reset,_ifx_delay_set_param,_ifx_reverb_mem_size,_ifx_reverb_init,_ifx_reverb_process,_ifx_reverb_reset,_ifx_reverb_set_param,_ifx_chorus_init,_ifx_chorus_process,_ifx_chorus_reset,_ifx_chorus_set_param,_ifx_bitcrusher_init,_ifx_bitcrusher_process,_ifx_bitcrusher_reset,_ifx_bitcrusher_set_param,_ifx_filter_init,_ifx_filter_process,_ifx_filter_reset,_ifx_filter_set_param,_ifx_waveshaper_init,_ifx_waveshaper_process,_ifx_waveshaper_reset,_ifx_waveshaper_set_param,_ifx_ringmod_init,_ifx_ringmod_process,_ifx_ringmod_reset,_ifx_ringmod_set_param,_ifx_tremolo_init,_ifx_tremolo_process,_ifx_tremolo_reset,_ifx_tremolo_set_param,_ifx_gate_init,_ifx_gate_process,_ifx_gate_reset,_ifx_gate_set_param,_ifx_limiter_init,_ifx_limiter_process,_ifx_limiter_reset,_ifx_limiter_set_param,_ifx_flanger_init,_ifx_flanger_process,_ifx_flanger_reset,_ifx_flanger_set_param,_ifx_phaser_init,_ifx_phaser_process,_ifx_phaser_reset,_ifx_phaser_set_param,_ifx_autowah_init,_ifx_autowah_process,_ifx_autowah_reset,_ifx_autowah_set_param,_ifx_compressor_init,_ifx_compressor_process,_ifx_compressor_reset,_ifx_compressor_set_param,_ifx_transient_init,_ifx_transient_process,_ifx_transient_reset,_ifx_transient_set_param,_ifx_tape_init,_ifx_tape_process,_ifx_tape_reset,_ifx_tape_set_param,_ifx_pitchshift_init,_ifx_pitchshift_process,_ifx_pitchshift_reset,_ifx_pitchshift_set_param,_malloc,_free]' -sEXPORTED_RUNTIME_METHODS='["cwrap","ccall","HEAPF32"]' -sMODULARIZE=1 -sEXPORT_ES6=1 -o $(MA_JS); \
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
C_OBJS = build/miniaudio.o build/drums.o build/fmsynth.o build/effects.o build/insert_fx.o build/wavetable.o build/adsr.o build/pan.o build/noise.o build/noise_ma_oracle.o build/modular.o build/modular_stages.o build/gcov_flush.o

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
	rm -f build/drums.o build/fmsynth.o build/effects.o build/insert_fx.o build/wavetable.o build/adsr.o build/pan.o build/noise.o build/noise_ma_oracle.o build/modular.o build/modular_stages.o build/gcov_flush.o
	rm -f $(C_LIB)
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
	cd src/go && $(GO) run ./cmd/gen-synth-abi > ../js/synth_param_abi.gen.js

# Regenerate the master-chain config bridge (src/js/chain_spec.gen.js) from Go's
# chain_spec.go. Same rationale as gen-synth-abi: a stale chain_spec.gen.js would
# let the browser's WebAudio compressor/soft-clip drift from the desktop Go/C
# chain, so the same instrument sounds different across platforms. Cheap (no CGo);
# regenerated unconditionally on every WASM build. Drift-tested by
# internal/audio/chain_spec_test.go.
.PHONY: gen-chain-spec
gen-chain-spec:
	cd src/go && $(GO) run ./cmd/gen-chain-spec > ../js/chain_spec.gen.js

# Regenerate the modular-instrument render table bridge
# (src/js/modular_instruments.gen.js) from the config-first Go table
# modularInstrumentDefs (internal/audio/modular_instruments.go). Adding/cloning a
# modular instrument is a one-row table edit; this generates the audio.js
# RENDER/RENDER_INFO entries so no manual JS change is needed. A stale gen file
# would leave a new instrument silent on WASM ("Unknown sound"). Cheap (no CGo);
# regenerated on every WASM build. Drift-tested by
# internal/audio/modular_instruments_gen_test.go.
.PHONY: gen-modular-instruments
gen-modular-instruments:
	cd src/go && $(GO) run ./cmd/gen-modular-instruments > ../js/modular_instruments.gen.js

.PHONY: measure-loudness
measure-loudness: ## Measure + regenerate per-instrument loudness amplitudes
	cd src/go && ../../.tools/go/bin/go run ./cmd/measure-loudness -write
	@echo "regenerated instrument_loudness_gen.go + instrument_loudness.gen.js"

# Generate JavaScript config from Go audio config (single source of truth)
.PHONY: audio-config
audio-config: $(C_LIB)
	cd src/go && CGO_ENABLED=1 $(GO) run ./cmd/export_audio --config > ../js/audio_config.generated.js

# Sync audio config and regenerate desktop audio reference
.PHONY: sync-audio
sync-audio: audio-config
	cd src/go && CGO_ENABLED=1 $(GO) run ./cmd/export_audio > ../js/desktop_audio.json

# ─── Synth Matching / Fingerprinting ─────────────────────────────────────────

.PHONY: synth-match synth-fingerprint fetch-synth-refs song-timeline song-render song-compare

synth-match: ## Tune a synth toward a reference WAV (INST=, REF=, [BUDGET=], [RESTARTS=], [DRY_RUN=1])
	cd src/go && ../../.tools/go/bin/go run ./cmd/synth-match -inst $(INST) -ref $(REF) \
		$(if $(BUDGET),-budget $(BUDGET),) $(if $(RESTARTS),-restarts $(RESTARTS),) $(if $(DRY_RUN),-dry-run,)

synth-fingerprint: ## Print the analysis fingerprint of a WAV (FILE=)
	cd src/go && ../../.tools/go/bin/go run ./cmd/synth-analyze ref -file $(FILE)

fetch-synth-refs: ## Download + convert the reference instrument WAVs (Task 15)
	bash scripts/fetch_synth_refs.sh

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

wasm: gen-synth-abi gen-chain-spec gen-modular-instruments $(MA_JS)
	cd src/go && ($(GO) mod download || true)
	cd src/go && GOOS=js GOARCH=wasm $(GO) build -ldflags "-s -w -X main.defaultLog=INFO" -o $(WASM_OUT) ./cmd

wasm-debug: gen-synth-abi gen-chain-spec gen-modular-instruments $(MA_JS)
	cd src/go && ($(GO) mod download || true)
	cd src/go && GOOS=js GOARCH=wasm $(GO) build -ldflags "-s -w -X main.defaultLog=DEBUG" -o $(WASM_OUT) ./cmd

wasm-cover: gen-synth-abi gen-chain-spec gen-modular-instruments $(MA_JS)
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

wasm-all: wasm wasm-playtest wasm-audio-playtest

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

# Install local git hooks (currently: pre-commit runs gen-design-tokens
# and rejects commits where the generated file is stale).
.PHONY: install-hooks
install-hooks:
	@if [ ! -d .git ]; then echo "not a git repo"; exit 1; fi
	@mkdir -p .git/hooks
	@cp scripts/precommit/gen-design-tokens.sh .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "installed pre-commit hook (regenerates design tokens, rejects stale)"

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
		src/c/effects.c src/c/effects.h \
		src/c/fmsynth.c src/c/fmsynth.h \
		src/c/insert_fx.c src/c/insert_fx.h \
		src/c/wavetable.c src/c/wavetable.h \
		src/c/adsr.c src/c/adsr.h \
		src/c/modular.c src/c/modular.h \
		src/c/noise.c src/c/noise.h \
		src/c/pan.c src/c/pan.h \
		src/c/synth_params.h src/c/synth_post.h

lint: lint-go lint-js lint-c
lint-fix: lint-go-fix lint-js-fix

# Capture screenshots of desktop, browser desktop, and browser mobile layouts.
# Output goes to screenshots/ (override with OUTDIR=).
screenshot: $(C_LIB) wasm
	GO=$(GO) OUTDIR=$(or $(OUTDIR),screenshots) ./scripts/screenshot.sh

# Capture every UI surface in the scene catalog.
# Output goes to screenshots/all/ (override with OUTDIR=).
# Set MOBILE=1 to add a mobile-viewport browser pass.
# Set SCENES=name1,name2 to filter to specific scenes.
screenshots-all: $(C_LIB) wasm
	GO=$(GO) OUTDIR=$(or $(OUTDIR),screenshots/all) MOBILE=$(or $(MOBILE),0) \
		SCENES=$(or $(SCENES),) ./scripts/capture_all_ui.sh

# Alias used by docs / future baseline workflows.
screenshots-all-update: screenshots-all
	@echo "screenshots refreshed"

# Curated minimal capture: one screenshot per distinct surface / popup / tab
# (scene list in scripts/basic_scenes.txt; drift-guarded by
# internal/ui/basic_scenes_test.go). Mobile pass on by default.
# Output goes to screenshots/basic/ (override with OUTDIR=).
screenshots-basic: $(C_LIB) wasm
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
	cd src/go; $(LOG_ENV) $(GO) test -tags test -modfile=go.test.mod -timeout 900s ./...
	cd src/go; $(LOG_ENV) $(GO) test -timeout 10s ./internal/audio

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
	cd src/go; $(LOG_ENV) $(GO) test -tags test -modfile=go.test.mod -timeout 300s ./...
	cd src/go; $(LOG_ENV) $(GO) test -timeout 10s ./internal/audio
	$(MAKE) test-browser

# Run all Go tests with real Ebiten (requires X11/xvfb) + all browser tests.
# -timeout 30m (up from the 10m per-package default): internal/audio/synthmatch
# runs the Hooke-Jeeves optimizer against the REAL CGo synth + FFT fingerprint
# (TestMatch_* ~ 300/40/300 evals at ~1s each), so the package alone takes ~12m.
# Under -tags test (make test/test-debug) the synth is stubbed and stays fast, so
# only this real-build target needs the longer cap.
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
record-browser-filter: wasm
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
# reports; this target enforces). Baseline (2026-06-05) is 99.28% after the
# native gap-closure pass; the only uncovered lines are documented defensive
# guards (drums.c ma_noise_init failures + load_audio mid-decode failures,
# insert_fx.c NULL-buffer passthroughs, fmsynth.c past-envelope return — see
# legacy_render_native_test.go header). 99 leaves ~0.3pt headroom; ratchet
# when the defensive set shrinks.
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
