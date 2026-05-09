package hooks

import (
	"path/filepath"
	"runtime"
	"strings"
)

// CaptureSource records the file:line of the caller's caller (or further up
// the stack via skip). The returned Source is intended to be passed into
// PublishWithSource so the originating user-code site flows through the bus
// to subscribers, rather than the middleware (helper / bus / formatter) site.
//
// skip semantics: skip=0 returns the immediate caller of CaptureSource;
// skip=1 returns the caller of *that* caller, and so on. Emit helpers in
// internal/ui/event_helpers.go that call CaptureSource directly typically
// pass skip=1 (they want to identify their own caller — the user-action
// site — not themselves). Wrapper helpers add to the skip count.
//
// Returns Source{} if the runtime can't unwind that far (e.g. inlining
// too aggressive); callers must handle the zero value (renderers do).
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

// splitPkgFile turns an absolute Go source path into (pkg, basefile) where
// pkg is rooted at the project's `internal/`, `core/`, `cmd/`, or `src/js/`
// segment so the rendered source is stable across machines (no $HOME prefix)
// and short enough to fit in an INFO log column. Falls back to the parent
// dir + base file if no known root segment is found.
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

// projectRoots are the path segments under which Beatmo Go source lives.
// Order matters only for performance (most-common first).
var projectRoots = []string{"internal", "core", "cmd"}
