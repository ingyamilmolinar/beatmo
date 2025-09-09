GO ?= $(shell if [ -x $(CURDIR)/.tools/go/bin/go ]; then echo $(CURDIR)/.tools/go/bin/go; else echo go; fi)
WASM_OUT = ../js/main.wasm
MA_JS = src/js/drums.single.js
C_LIB = build/libdrums.a
C_SRC = src/c/drums.c src/c/miniaudio.c

$(MA_JS): $(C_SRC) src/c/miniaudio.h
	emcc $(C_SRC) -O3 -sWASM=1 -sSINGLE_FILE=1 -sENVIRONMENT=web -sASSERTIONS=0 -sALLOW_MEMORY_GROWTH=0 -sEXPORTED_FUNCTIONS='[_render_snare,_render_kick,_render_hihat,_render_tom,_render_clap,_load_wav,_result_description,_malloc,_free]' -sEXPORTED_RUNTIME_METHODS='["cwrap","ccall","HEAPF32"]' -sMODULARIZE=1 -sEXPORT_ES6=1 -o $(MA_JS)

$(C_LIB): $(C_SRC)
	mkdir -p build
	$(CC) -O2 -c src/c/miniaudio.c -o build/miniaudio.o
	$(CC) -O2 -c src/c/drums.c -o build/drums.o
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

run: $(C_LIB) sync-wav
	cd src/go; CGO_ENABLED=1 $(GO) run ./cmd/tunkul.go -log INFO $(RUN_ARGS)

run-debug: $(C_LIB) sync-wav
	cd src/go; CGO_ENABLED=1 $(GO) run ./cmd/tunkul.go -log DEBUG $(RUN_ARGS)

test: $(C_LIB) sync-wav
	cd src/go; $(GO) test -tags test -modfile=go.test.mod -timeout 10s ./...
	cd src/go; $(GO) test -timeout 10s ./internal/audio
	$(MAKE) wasm
	/bin/bash -c "if command -v node >/dev/null 2>&1; then GO=$(GO) node src/js/audio.browser.test.js; else echo 'Node not found'; exit 1; fi"
	/bin/bash -c "GO=$(GO) node src/js/volume.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/slider_volume.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/import_export.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/timeline_center.browser.test.js"

# Alias for convenience; many developers instinctively run `make tests`.
tests: test

test-real: $(C_LIB) sync-wav
	cd src/go && BPM_TIMING_TEST=1 xvfb-run $(GO) test ./...
	$(MAKE) wasm
	/bin/bash -c "GO=$(GO) node src/js/audio.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/bpm.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/volume.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/slider_volume.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/import_export.browser.test.js"
	/bin/bash -c "GO=$(GO) node src/js/timeline_center.browser.test.js"

test-xvfb:
	cd src/go; xvfb-run $(GO) test -tags test -timeout 10s ./...

dependencies:
	./scripts/setup-env.sh
