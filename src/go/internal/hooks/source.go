package hooks

import (
	"path/filepath"
	"runtime"
	"strings"
)

// skip=0 is CaptureSource's immediate caller; emit helpers pass 1 to name THEIR caller, the user-action site, and wrappers
// add more. Source{} when the stack is too shallow (inlining); renderers handle it.
func CaptureSource(skip int) Source {
	// runtime.Caller(0) reports CaptureSource itself, so we add 1 to skip
	// to make skip=0 mean "my caller" — the most natural API.
	_, file, line, ok := runtime.Caller(skip + 1)
	if !ok {
		return Source{}
	}
	pkg, base := splitPkgFile(file)
	return Source{Pkg: pkg, File: base, Line: line}
}

// Rooted at internal/, core/, cmd/ or src/js/ so the source is stable across machines (no $HOME) and fits an INFO
// column; else the parent dir + base file.
func splitPkgFile(absPath string) (pkg, base string) {
	base = filepath.Base(absPath)
	dir := filepath.Dir(absPath)
	// Walk up until we hit one of the project root segments.
	for _, root := range projectRoots {
		if i := strings.Index(dir, string(filepath.Separator)+root+string(filepath.Separator)); i >= 0 {
			// Keep from "internal/..." (or root/...) onward.
			pkg = filepath.ToSlash(dir[i+1:])
			return pkg, base
		}
		// Trailing match (path ends in /root) — produce just the root.
		if strings.HasSuffix(dir, string(filepath.Separator)+root) {
			pkg = root
			return pkg, base
		}
	}
	// Fallback: last directory segment only — keeps output bounded.
	pkg = filepath.ToSlash(filepath.Base(dir))
	return pkg, base
}

// Order matters only for speed (most common first).
var projectRoots = []string{"internal", "core", "cmd"}
