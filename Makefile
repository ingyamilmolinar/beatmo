WASM_OUT = ../js/main.wasm
MA_JS = src/js/drums.js
C_LIB = build/libdrums.a
C_SRC = src/c/drums.c src/c/miniaudio.c

$(MA_JS): $(C_SRC) src/c/miniaudio.h
	emcc $(C_SRC) -sWASM=1 -sEXPORTED_FUNCTIONS='[_render_snare,_render_kick,_render_hihat,_render_tom,_render_clap,_load_wav,_malloc,_free]' -sEXPORTED_RUNTIME_METHODS='["cwrap","ccall","HEAPF32"]' -sMODULARIZE=1 -sEXPORT_ES6=1 -o $(MA_JS)

$(C_LIB): $(C_SRC)
	mkdir -p build
	$(CC) -O2 -c src/c/miniaudio.c -o build/miniaudio.o
	$(CC) -O2 -c src/c/drums.c -o build/drums.o
	ar rcs $@ build/miniaudio.o build/drums.o

clean:
	rm -f build/drums.o
	rm -f $(C_LIB)

wasm: $(MA_JS)
	cd src/go && go mod download
	cd src/go && GOOS=js GOARCH=wasm go build -o $(WASM_OUT) ./cmd/...

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
	cd src/go; go test -timeout 1s ./...
	cd src/go; go test -timeout 1s ./internal/audio
	$(MAKE) wasm
	node src/js/audio.browser.test.js

test-xvfb:
	cd src/go; xvfb-run go test -tags test -timeout 1s ./...

dependencies:
	./scripts/setup-env.sh
