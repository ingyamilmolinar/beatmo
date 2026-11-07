package deploy

import (
	"go/build"
	"path/filepath"
	"slices"
	"testing"
)

// The shipped WASM is a release build, so the developer-only surfaces are hidden on beatmo.io (still compiled in).
func TestShippedWasmIsAReleaseBuild(t *testing.T) {
	root := repoRoot(t)
	tags := shippedWasmTags(t, root)
	if !slices.Contains(tags, "release") {
		t.Fatalf("WASM_RELEASE_TAGS = %v: the shipped WASM must carry the `release` tag "+
			"(comma-separated: -tags takes ONE argument)", tags)
	}
	uiDir := filepath.Join(root, "src", "go", "internal", "ui")
	files := func(tags []string) []string {
		ctx := build.Default
		ctx.GOOS, ctx.GOARCH, ctx.BuildTags = "js", "wasm", tags
		pkg, err := ctx.ImportDir(uiDir, 0)
		if err != nil {
			t.Fatalf("import %s: %v", uiDir, err)
		}
		return pkg.GoFiles
	}
	shipped := files(tags)
	if !slices.Contains(shipped, "devtools_build_release.go") || slices.Contains(shipped, "devtools_build_dev.go") {
		t.Errorf("the shipped WASM must compile devtools_build_release.go and not devtools_build_dev.go; got release=%v dev=%v",
			slices.Contains(shipped, "devtools_build_release.go"), slices.Contains(shipped, "devtools_build_dev.go"))
	}
	// Not vacuous: a dev build compiles the dev file.
	if dev := files(nil); !slices.Contains(dev, "devtools_build_dev.go") {
		t.Error("a js/wasm build without -tags release does not compile devtools_build_dev.go -- the scan is broken")
	}
}
