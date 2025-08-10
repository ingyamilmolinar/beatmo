WASM_OUT = ../js/main.wasm
MA_JS = src/js/drums.js

wasm:
	cd src/go && go mod download
	cd src/go && GOOS=js GOARCH=wasm go build -o $(WASM_OUT) ./cmd/...
	emcc src/c/drums.c src/c/miniaudio.c -sWASM=1 -sEXPORTED_FUNCTIONS='[_render_snare,_render_kick,_malloc,_free]' -sEXPORTED_RUNTIME_METHODS='["cwrap","ccall"]' -sMODULARIZE=1 -sEXPORT_ES6=1 -o $(MA_JS)

serve:
	cd src/js && python3 -m http.server 8080

run:
	cd src/go; CGO_ENABLED=1 go run ./cmd/tunkul.go $(RUN_ARGS)

test:
	cd src/go; go test -tags test -modfile=go.test.mod -timeout 1s ./...
	cd src/go; go test -timeout 1s ./internal/audio
	$(MAKE) wasm
	node src/js/audio.browser.test.js


test-real:
	cd src/go; go test -timeout 1s ./...
	cd src/go; go test -timeout 1s ./internal/audio
	$(MAKE) wasm
	node src/js/audio.browser.test.js


test-xvfb:
	cd src/go; xvfb-run go test -tags test -timeout 1s ./...

dependencies:
	./scripts/setup-env.sh

