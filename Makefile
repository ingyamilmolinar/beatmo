WASM_OUT = ../js/main.wasm
MA_JS = src/js/drums.js
C_LIB = build/libdrums.a
C_SRC = src/c/drums.c src/c/miniaudio.c

$(MA_JS): $(C_SRC) src/c/miniaudio.h
	@if command -v emcc >/dev/null 2>&1; then \
	emcc $(C_SRC) -sWASM=1 -sEXPORTED_FUNCTIONS='[_render_snare,_render_kick,_render_hihat,_render_tom,_render_clap,_load_wav,_result_description,_malloc,_free]' -sEXPORTED_RUNTIME_METHODS='["cwrap","ccall","HEAPF32"]' -sMODULARIZE=1 -sEXPORT_ES6=1 -o $(MA_JS); \
	else \
	echo "emcc not found; skipping drums.js build" && echo '// wasm disabled' > $(MA_JS); \
	fi

$(C_LIB): $(C_SRC)
	mkdir -p build
	$(CC) -O2 -c src/c/miniaudio.c -o build/miniaudio.o
	$(CC) -O2 -c src/c/drums.c -o build/drums.o
	ar rcs $@ build/miniaudio.o build/drums.o

clean:
	rm -f build/drums.o
	rm -f $(C_LIB)

wasm: $(MA_JS)
	cd src/go && (go mod download || true)
	cd src/go && GOOS=js GOARCH=wasm go build -o $(WASM_OUT) ./cmd/... || echo "skipping go wasm build"

serve:
	cd src/js && python3 -m http.server 8080

run: $(C_LIB)
	cd src/go; CGO_ENABLED=1 go run ./cmd/tunkul.go $(RUN_ARGS)

test: $(C_LIB)
	cd src/go; go test -tags test -modfile=go.test.mod -timeout 1s ./...
	cd src/go; go test -timeout 1s ./internal/audio
	$(MAKE) wasm
	/bin/bash -c "node src/js/audio.browser.test.js"

test-real: $(C_LIB)
	@if command -v pkg-config >/dev/null 2>&1 && pkg-config --exists xrandr alsa; then \
	cd src/go && go test -timeout 1s ./... && \
	cd src/go && go test -timeout 1s ./internal/audio && \
	$(MAKE) wasm && \
	node src/js/audio.browser.test.js; \
	else \
	echo "Missing Xrandr or ALSA development headers; skipping real tests"; \
	fi

test-xvfb:
	cd src/go; xvfb-run go test -tags test -timeout 1s ./...

dependencies:
	./scripts/setup-env.sh
