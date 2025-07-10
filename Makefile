WASM_OUT = ../js/main.wasm

wasm:
	cd src/go && go mod download
	cd src/go && GOOS=js GOARCH=wasm go build -o $(WASM_OUT) ./cmd/...

serve:
	cd src/js && python3 -m http.server 8080

run:
	cd src/go; go run ./cmd/tunkul.go

test-mock:
	cd src/go; go test -tags test -modfile=go.test.mod -timeout 1s ./...

test-real:
	cd src/go; go test -timeout 1s ./...

test:
	$(MAKE) test-mock

test-xvfb:
	cd src/go; xvfb-run go test -tags test -timeout 1s ./...

dependencies:
	./scripts/setup-ebiten-env.sh

