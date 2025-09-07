WASM_OUT = ../js/main.wasm
MA_JS = src/js/drums.js
C_LIB = build/libdrums.a
C_SRC = src/c/drums.c src/c/miniaudio.c

$(MA_JS): $(C_SRC) src/c/miniaudio.h
	emcc $(C_SRC) -sWASM=1 -sEXPORTED_FUNCTIONS='[_render_snare,_render_kick,_render_hihat,_render_tom,_render_clap,_load_wav,_result_description,_malloc,_free]' -sEXPORTED_RUNTIME_METHODS='["cwrap","ccall","HEAPF32"]' -sMODULARIZE=1 -sEXPORT_ES6=1 -o $(MA_JS)

$(C_LIB): $(C_SRC)
	mkdir -p build
	$(CC) -O2 -c src/c/miniaudio.c -o build/miniaudio.o
	$(CC) -O2 -c src/c/drums.c -o build/drums.o
	ar rcs $@ build/miniaudio.o build/drums.o

clean:
	rm -f build/drums.o
	rm -f $(C_LIB)

sync-wav:
	mkdir -p src/go/internal/assets/wav
	cp -a assets/wav/. src/go/internal/assets/wav/

wasm: $(MA_JS) sync-wav
	cd src/go && (go mod download || true)
	cd src/go && GOOS=js GOARCH=wasm go build -o $(WASM_OUT) ./cmd/...

serve:
	cd src/js && python3 -m http.server 8080

run: $(C_LIB) sync-wav
	cd src/go; CGO_ENABLED=1 go run ./cmd/tunkul.go $(RUN_ARGS)

test: $(C_LIB) sync-wav
	cd src/go; go test -tags test -modfile=go.test.mod -timeout 2s ./...
	cd src/go; go test -timeout 2s ./internal/audio
	$(MAKE) wasm
	/bin/bash -c "node src/js/audio.browser.test.js"
	/bin/bash -c "node src/js/volume.browser.test.js"
	/bin/bash -c "node src/js/slider_volume.browser.test.js"
	/bin/bash -c "node src/js/import_export.browser.test.js"
	/bin/bash -c "node src/js/timeline_center.browser.test.js"

test-real: $(C_LIB) sync-wav
	cd src/go && BPM_TIMING_TEST=1 xvfb-run go test ./...
	$(MAKE) wasm
	/bin/bash -c "node src/js/audio.browser.test.js"
	/bin/bash -c "node src/js/bpm.browser.test.js"
	/bin/bash -c "node src/js/volume.browser.test.js"
	/bin/bash -c "node src/js/slider_volume.browser.test.js"
	/bin/bash -c "node src/js/import_export.browser.test.js"
	/bin/bash -c "node src/js/timeline_center.browser.test.js"

test-xvfb:
	cd src/go; xvfb-run go test -tags test -timeout 2s ./...

dependencies:
	./scripts/setup-env.sh
