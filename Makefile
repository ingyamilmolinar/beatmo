GO ?= $(shell if [ -x $(CURDIR)/.tools/go/bin/go ]; then echo $(CURDIR)/.tools/go/bin/go; else echo go; fi)
WASM_OUT = ../js/main.wasm
MA_JS = src/js/drums.single.js
C_LIB = build/libdrums.a
C_SRC = src/c/drums.c src/c/miniaudio.c src/c/fmsynth.c src/c/effects.c src/c/insert_fx.c src/c/wavetable.c src/c/adsr.c src/c/lfo.c src/c/pan.c
C_HDRS = src/c/drums.h src/c/miniaudio.h src/c/fmsynth.h src/c/effects.h src/c/synth_params.h src/c/insert_fx.h src/c/wavetable.h src/c/adsr.h src/c/lfo.h src/c/pan.h

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
		emcc $(C_SRC) -O3 -DMA_ENABLE_MP3 -sWASM=1 -sSINGLE_FILE=1 -sENVIRONMENT=web -sASSERTIONS=0 -sALLOW_MEMORY_GROWTH=0 -sEXPORTED_FUNCTIONS='[_render_snare,_render_kick,_render_hihat,_render_open_hihat,_render_tom,_render_tom_high,_render_tom_low,_render_clap,_render_cowbell,_render_bass_guitar,_render_sub_bass,_render_snare_rimshot,_render_snare_sidestick,_render_kick_deep,_render_kick_punchy,_render_kick_lofi,_render_kick_tight,_render_shaker,_render_ride,_render_crash,_render_fm_bass,_render_fm_bell,_render_fm_lead,_render_fm_epiano,_render_fm_pluck,_render_snare_p,_render_kick_p,_render_hihat_p,_render_clap_p,_render_tom_p,_render_cowbell_p,_load_wav,_result_description,_delay_init,_delay_process,_delay_reset,_reverb_buffer_size,_reverb_init,_reverb_process,_reverb_reset,_ifx_distortion_init,_ifx_distortion_process,_ifx_distortion_reset,_ifx_distortion_set_param,_ifx_delay_init,_ifx_delay_process,_ifx_delay_reset,_ifx_delay_set_param,_ifx_reverb_mem_size,_ifx_reverb_init,_ifx_reverb_process,_ifx_reverb_reset,_ifx_reverb_set_param,_ifx_chorus_init,_ifx_chorus_process,_ifx_chorus_reset,_ifx_chorus_set_param,_ifx_bitcrusher_init,_ifx_bitcrusher_process,_ifx_bitcrusher_reset,_ifx_bitcrusher_set_param,_ifx_filter_init,_ifx_filter_process,_ifx_filter_reset,_ifx_filter_set_param,_malloc,_free]' -sEXPORTED_RUNTIME_METHODS='["cwrap","ccall","HEAPF32"]' -sMODULARIZE=1 -sEXPORT_ES6=1 -o $(MA_JS); \
	else \
		if [ -f "$(MA_JS)" ]; then \
			echo "[INFO] emcc not found; using existing $(MA_JS)"; \
		else \
			echo "[WARN] emcc not found and $(MA_JS) missing; skipping MA build"; \
		fi; \
	fi

$(C_LIB): $(C_SRC) $(C_HDRS)
	mkdir -p build
	$(CC) -O2 -DMA_ENABLE_MP3 -c src/c/miniaudio.c -o build/miniaudio.o
	$(CC) -O2 -DMA_ENABLE_MP3 -c src/c/drums.c -o build/drums.o
	$(CC) -O2 -c src/c/fmsynth.c -o build/fmsynth.o
	$(CC) -O2 -c src/c/effects.c -o build/effects.o
	$(CC) -O2 -c src/c/insert_fx.c -o build/insert_fx.o
	$(CC) -O2 -c src/c/wavetable.c -o build/wavetable.o
	$(CC) -O2 -c src/c/adsr.c -o build/adsr.o
	$(CC) -O2 -c src/c/lfo.c -o build/lfo.o
	$(CC) -O2 -c src/c/pan.c -o build/pan.o
	ar rcs $@ build/miniaudio.o build/drums.o build/fmsynth.o build/effects.o build/insert_fx.o build/wavetable.o build/adsr.o build/lfo.o build/pan.o

clean:
	rm -f build/drums.o build/fmsynth.o build/effects.o build/insert_fx.o build/wavetable.o build/adsr.o build/lfo.o build/pan.o
	rm -f $(C_LIB)
	rm -f $(MA_JS)

sync-wav:
	mkdir -p src/go/internal/assets/wav
	cp -a assets/wav/. src/go/internal/assets/wav/

# Generate JavaScript config from Go audio config (single source of truth)
.PHONY: audio-config
audio-config: $(C_LIB)
	cd src/go && CGO_ENABLED=1 $(GO) run ./cmd/export_audio --config > ../js/audio_config.generated.js

# Sync audio config and regenerate desktop audio reference
.PHONY: sync-audio
sync-audio: audio-config
	cd src/go && CGO_ENABLED=1 $(GO) run ./cmd/export_audio > ../js/desktop_audio.json

wasm: $(MA_JS) sync-wav
	cd src/go && ($(GO) mod download || true)
	cd src/go && GOOS=js GOARCH=wasm $(GO) build -ldflags "-X main.defaultLog=INFO" -o $(WASM_OUT) ./cmd/...

wasm-debug: $(MA_JS) sync-wav
	cd src/go && ($(GO) mod download || true)
	cd src/go && GOOS=js GOARCH=wasm $(GO) build -ldflags "-X main.defaultLog=DEBUG" -o $(WASM_OUT) ./cmd/...

wasm-cover: $(MA_JS) sync-wav
	cd src/go && ($(GO) mod download || true)
	cd src/go && GOOS=js GOARCH=wasm $(GO) build \
		-cover -covermode=atomic \
		-coverpkg=github.com/ingyamilmolinar/beatmo/... \
		-ldflags "-X main.defaultLog=INFO" \
		-o $(WASM_OUT) ./cmd/...

wasm-playtest: sync-wav
	cd src/go && GOOS=js GOARCH=wasm $(GO) build -o ../js/play_ui.wasm ./internal/ui/playtest

wasm-audio-playtest: sync-wav
	cd src/go && GOOS=js GOARCH=wasm $(GO) build -o ../js/playtest.wasm ./internal/audio/playtest

wasm-all: wasm wasm-playtest wasm-audio-playtest

serve:
	cd src/js && python3 -m http.server 8080

generate:
	cd src/go && $(GO) generate ./internal/audio/...

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
		src/c/lfo.c src/c/lfo.h \
		src/c/pan.c src/c/pan.h \
		src/c/synth_params.h

lint: lint-go lint-js lint-c
lint-fix: lint-go-fix lint-js-fix

# Capture screenshots of desktop, browser desktop, and browser mobile layouts.
# Output goes to screenshots/ (override with OUTDIR=).
screenshot: $(C_LIB) wasm
	GO=$(GO) OUTDIR=$(or $(OUTDIR),screenshots) ./scripts/screenshot.sh

run: $(C_LIB) sync-wav
	cd src/go; CGO_ENABLED=1 $(GO) build -o /tmp/beatmo ./cmd && /tmp/beatmo -log INFO $(RUN_ARGS)

run-debug: $(C_LIB) sync-wav
	cd src/go; TIMELINE_TRACE=1 PERF_LOG=1 CGO_ENABLED=1 $(GO) build -o /tmp/beatmo ./cmd && TIMELINE_TRACE=1 PERF_LOG=1 /tmp/beatmo -log DEBUG $(RUN_ARGS)

bench: $(C_LIB) sync-wav
	cd src/go; PERF_LOG=1 CGO_ENABLED=1 $(GO) run ./cmd -log INFO -bench-bpm $(or $(BPM),200) -bench-secs $(or $(SECS),10) $(RUN_ARGS)

# Self-contained desktop benchmark: 4 BPM levels (120/200/240/300), 15s each,
# pprof CPU profile, structured JSON + ASCII table output.
# Override: BPM_LEVELS="120 200" DURATION=5 make bench-desktop
bench-desktop: $(C_LIB) sync-wav
	GO=$(GO) BPM_LEVELS="$(or $(BPM_LEVELS),120 200 240 300)" DURATION=$(or $(DURATION),15) ./scripts/bench-desktop.sh

# Self-contained browser benchmark: same 4 BPM levels, startup demo circuit.
# Produces bench-results/browser.json + ASCII table.
bench-browser: wasm
	@mkdir -p bench-results
	GO=$(GO) node src/js/bench_startup_demo.browser.test.js

# Run perf_e2e with the realistic startup demo circuit (58 nodes, 7 instruments).
bench-perf-e2e: wasm
	BENCH_CIRCUIT=startup BENCH_DURATION=$(or $(DURATION),15000) BENCH_BPM=$(or $(BPM),200) GO=$(GO) node src/js/perf_e2e.browser.test.js

# Run both desktop and browser benchmarks, then produce a comparison report.
bench-all: bench-desktop bench-browser
	node scripts/bench-compare.js

# Run with camera/node/edge detailed logs enabled
run-cam-logs: $(C_LIB) sync-wav
	cd src/go; CGO_ENABLED=1 $(GO) build -o /tmp/beatmo ./cmd && DEBUG_DRAW_NODES=1 DEBUG_GEOM=1 /tmp/beatmo -log DEBUG $(RUN_ARGS)

test: $(C_LIB) sync-wav
	cd src/go; $(LOG_ENV) $(GO) test -tags test -modfile=go.test.mod -timeout 60s ./...
	cd src/go; $(LOG_ENV) $(GO) test -timeout 10s ./internal/audio

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

test-debug: $(C_LIB) sync-wav
	cd src/go; $(LOG_ENV) $(GO) test -tags test -modfile=go.test.mod -timeout 60s ./...
	cd src/go; $(LOG_ENV) $(GO) test -timeout 10s ./internal/audio
	$(MAKE) test-browser

# Run all Go tests with real Ebiten (requires X11/xvfb) + all browser tests
test-real: $(C_LIB) sync-wav
	cd src/go && BPM_TIMING_TEST=1 xvfb-run -a $(GO) test ./...
	$(MAKE) test-browser

# Run cross-platform parity tests: Go golden files + browser comparison
test-parity: $(C_LIB) sync-wav
	cd src/go; $(LOG_ENV) $(GO) test -tags test -modfile=go.test.mod -run TestCrossPlatformParity -timeout 30s ./internal/ui
	$(MAKE) test-browser-filter FILTER=parity_cross_platform

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

# Go test coverage
coverage-go: $(C_LIB) sync-wav
	@mkdir -p coverage
	cd src/go; $(LOG_ENV) $(GO) test -tags test -modfile=go.test.mod \
		-coverprofile=../../coverage/go.out -covermode=atomic -timeout 60s ./...
	@cd src/go && $(GO) tool cover -func=../../coverage/go.out | tail -1

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

# Both Go and browser coverage
coverage: coverage-go coverage-browser coverage-js

# Generate HTML reports from coverage profiles
coverage-report:
	@[ -f coverage/go.out ] && cd src/go && $(GO) tool cover -html=../../coverage/go.out -o ../../coverage/go.html && echo "Go coverage report: coverage/go.html" || echo "No Go coverage data (run make coverage-go first)"
	@[ -f coverage/browser.out ] && cd src/go && $(GO) tool cover -html=../../coverage/browser.out -o ../../coverage/browser.html && echo "Browser coverage report: coverage/browser.html" || echo "No browser coverage data (run make coverage-browser first)"

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
