GO ?= $(shell if [ -x $(CURDIR)/.tools/go/bin/go ]; then echo $(CURDIR)/.tools/go/bin/go; else echo go; fi)
WASM_OUT = ../js/main.wasm
MA_JS = src/js/drums.single.js
C_LIB = build/libdrums.a
C_SRC = src/c/drums.c src/c/miniaudio.c

# Opt-in test logging (keeps defaults quiet). Usage:
#   TEST_LOG=1 [TEST_LOG_LEVEL=TRACE] make test
# Leaving TEST_LOG unset keeps loggers silent during passing tests.
ifdef TEST_LOG
LOG_ENV := TUNKUL_TEST_LOG=$(TEST_LOG)
ifdef TEST_LOG_LEVEL
LOG_ENV += TUNKUL_TEST_LOG_LEVEL=$(TEST_LOG_LEVEL)
endif
endif

$(MA_JS): $(C_SRC) src/c/miniaudio.h
	@if command -v emcc >/dev/null 2>&1; then \
		emcc $(C_SRC) -O3 -DMA_ENABLE_MP3 -sWASM=1 -sSINGLE_FILE=1 -sENVIRONMENT=web -sASSERTIONS=0 -sALLOW_MEMORY_GROWTH=0 -sEXPORTED_FUNCTIONS='[_render_snare,_render_kick,_render_hihat,_render_open_hihat,_render_tom,_render_tom_high,_render_tom_low,_render_clap,_render_cowbell,_render_bass_guitar,_render_sub_bass,_load_wav,_result_description,_malloc,_free]' -sEXPORTED_RUNTIME_METHODS='["cwrap","ccall","HEAPF32"]' -sMODULARIZE=1 -sEXPORT_ES6=1 -o $(MA_JS); \
	else \
		if [ -f "$(MA_JS)" ]; then \
			echo "[INFO] emcc not found; using existing $(MA_JS)"; \
		else \
			echo "[WARN] emcc not found and $(MA_JS) missing; skipping MA build"; \
		fi; \
	fi

$(C_LIB): $(C_SRC)
	mkdir -p build
	$(CC) -O2 -DMA_ENABLE_MP3 -c src/c/miniaudio.c -o build/miniaudio.o
	$(CC) -O2 -DMA_ENABLE_MP3 -c src/c/drums.c -o build/drums.o
	ar rcs $@ build/miniaudio.o build/drums.o

clean:
	rm -f build/drums.o
	rm -f $(C_LIB)
	rm -f $(MA_JS)

sync-wav:
	mkdir -p src/go/internal/assets/wav
	cp -a assets/wav/. src/go/internal/assets/wav/

wasm: $(MA_JS) sync-wav
	cd src/go && ($(GO) mod download || true)
	cd src/go && GOOS=js GOARCH=wasm $(GO) build -ldflags "-X main.defaultLog=INFO" -o $(WASM_OUT) ./cmd/...

wasm-debug: $(MA_JS) sync-wav
	cd src/go && ($(GO) mod download || true)
	cd src/go && GOOS=js GOARCH=wasm $(GO) build -ldflags "-X main.defaultLog=DEBUG" -o $(WASM_OUT) ./cmd/...

serve:
	cd src/js && python3 -m http.server 8080

fmt:
	cd src/go && $(GO) fmt ./...

run: $(C_LIB) sync-wav
	cd src/go; CGO_ENABLED=1 $(GO) run ./cmd -log INFO $(RUN_ARGS)

run-debug: $(C_LIB) sync-wav
	cd src/go; TIMELINE_TRACE=1 PERF_LOG=1 CGO_ENABLED=1 $(GO) run ./cmd -log DEBUG $(RUN_ARGS)

# Run with camera/node/edge detailed logs enabled
run-cam-logs: $(C_LIB) sync-wav
	cd src/go; DEBUG_DRAW_NODES=1 DEBUG_GEOM=1 CGO_ENABLED=1 $(GO) run ./cmd -log DEBUG $(RUN_ARGS)

test: $(C_LIB) sync-wav
	cd src/go; $(LOG_ENV) $(GO) test -tags test -modfile=go.test.mod -timeout 60s ./...
	cd src/go; $(LOG_ENV) $(GO) test -timeout 10s ./internal/audio
	$(MAKE) wasm
	/bin/bash -c "if command -v node >/dev/null 2>&1; then GO=$(GO) node src/js/audio.browser.test.js; else echo 'Node not found'; exit 1; fi"
	/bin/bash -c "GO=$(GO) node src/js/volume.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/slider_volume.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/import_export.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/timeline_center.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/popup_node.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/ui_layout.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/zoom_grid.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/zoom_anchor.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/pan_camera.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/mute_logic.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/logic_nodes_audio.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/probability_nodes.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/bpm_editor.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/pause_no_advance.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/pause_resume_burst.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/playback_highlight.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/drum_playback.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/drum_future_refresh.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/track_follow.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/track_center.browser.test.js"
	/bin/bash -c "GO=$(GO) PERF_BROWSER_UPDATE_MAX_MS=1.0 PERF_BROWSER_FPS_MIN=50 node src/js/perf.browser.test.js"
	/bin/bash -c "GO=$(GO) PERF_E2E_FPS_MIN=3 node src/js/perf_e2e.browser.test.js"

# Alias for convenience; many developers instinctively run `make tests`.
tests: test

test-debug: $(C_LIB) sync-wav
	cd src/go; $(LOG_ENV) $(GO) test -tags test -modfile=go.test.mod -timeout 60s ./...
	cd src/go; $(LOG_ENV) $(GO) test -timeout 10s ./internal/audio
	$(MAKE) wasm
	/bin/bash -c "if command -v node >/dev/null 2>&1; then GO=$(GO) node src/js/audio.browser.test.js; else echo 'Node not found'; exit 1; fi"
	/bin/bash -c "GO=$(GO) node src/js/volume.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/slider_volume.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/import_export.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/timeline_center.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/popup_node.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/ui_layout.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/zoom_grid.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/zoom_anchor.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/pan_camera.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/mute_logic.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/logic_nodes_audio.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/probability_nodes.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/bpm_editor.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/pause_no_advance.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/pause_resume_burst.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/playback_highlight.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/drum_playback.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/drum_future_refresh.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/track_follow.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/track_center.browser.test.js"
	/bin/bash -c "GO=$(GO) PERF_BROWSER_UPDATE_MAX_MS=1.0 PERF_BROWSER_FPS_MIN=50 node src/js/perf.browser.test.js"
	/bin/bash -c "GO=$(GO) PERF_E2E_FPS_MIN=3 node src/js/perf_e2e.browser.test.js"

test-real: $(C_LIB) sync-wav
	cd src/go && BPM_TIMING_TEST=1 xvfb-run -a $(GO) test ./...
	$(MAKE) wasm
	env GO=$(GO) node src/js/audio.browser.test.js
	env GO=$(GO) node src/js/bpm.browser.test.js
	env GO=$(GO) node src/js/volume.browser.test.js
	env GO=$(GO) node src/js/slider_volume.browser.test.js
	env GO=$(GO) node src/js/import_export.browser.test.js
	env GO=$(GO) node src/js/timeline_center.browser.test.js
	env GO=$(GO) node src/js/popup_node.browser.test.js
	env GO=$(GO) node src/js/ui_layout.browser.test.js
	env GO=$(GO) node src/js/zoom_grid.browser.test.js
	env GO=$(GO) node src/js/zoom_anchor.browser.test.js
	env GO=$(GO) node src/js/pan_camera.browser.test.js
	env GO=$(GO) node src/js/mute_logic.browser.test.js
	env GO=$(GO) node src/js/logic_nodes_audio.browser.test.js
	env GO=$(GO) node src/js/probability_nodes.browser.test.js
	env GO=$(GO) node src/js/bpm_editor.browser.test.js
	env GO=$(GO) node src/js/pause_no_advance.browser.test.js
	env GO=$(GO) node src/js/pause_resume_burst.browser.test.js
	env GO=$(GO) node src/js/playback_highlight.browser.test.js
	env GO=$(GO) node src/js/drum_playback.browser.test.js
	env GO=$(GO) node src/js/drum_future_refresh.browser.test.js
	env GO=$(GO) PERF_BROWSER_UPDATE_MAX_MS=0.85 PERF_BROWSER_FPS_MIN=50 node src/js/perf.browser.test.js
	env GO=$(GO) PERF_E2E_FPS_MIN=3 node src/js/perf_e2e.browser.test.js
	env GO=$(GO) node src/js/transport_real.browser.test.js
	env GO=$(GO) node src/js/drum_row_controls_real.browser.test.js
	env GO=$(GO) node src/js/camera_real.browser.test.js
	env GO=$(GO) node src/js/timeline_seek_real.browser.test.js
	env GO=$(GO) node src/js/node_click_real.browser.test.js

test-xvfb:
	cd src/go; xvfb-run -a $(GO) test -tags test -timeout 10s ./...

dependencies:
	./scripts/setup-env.sh
