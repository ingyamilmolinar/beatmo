package rank

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// BuildConfig is one build of the module: a GOOS/GOARCH/tags combination plus
// the packages to load. Roots names the main packages whose reachability
// counts (production); test configs root at every test main instead.
type BuildConfig struct {
	Name     string
	GOOS     string
	GOARCH   string
	Tags     string
	ModFile  string
	Patterns []string
	Roots    []string
	Tests    bool
}

// Config is a whole ranking run.
type Config struct {
	Root     string
	Module   string
	Prod     []BuildConfig
	TestCfgs []BuildConfig
	Covers   []string
	Fast     bool // skip TestCfgs: unreachable is reported as "unsplit"
	// ToolPrefixes mark module-relative package paths that are dev tooling,
	// not the product: their candidates count at Weights.ToolShare.
	ToolPrefixes []string
}

// DefaultConfig is the beatmo module: linux (every main, tools included) and
// the js/wasm app as production; the untagged and the `test`-tagged test
// builds for test reachability.
func DefaultConfig(root, module string) Config {
	return Config{
		Root:   root,
		Module: module,
		// Every main under cmd/ except the app itself (package cmd) is tooling.
		ToolPrefixes: []string{"cmd/"},
		Prod: []BuildConfig{
			{Name: "linux", GOOS: "linux", GOARCH: "amd64", Patterns: []string{"./..."}, Roots: []string{"./..."}},
			{Name: "js", GOOS: "js", GOARCH: "wasm", Patterns: []string{"./cmd"}, Roots: []string{"./cmd"}},
		},
		TestCfgs: []BuildConfig{
			{Name: "test-untagged", GOOS: "linux", GOARCH: "amd64", Patterns: []string{"./..."}, Tests: true},
			{Name: "test-tagged", GOOS: "linux", GOARCH: "amd64", Tags: "test", ModFile: "go.test.mod", Patterns: []string{"./..."}, Tests: true},
		},
	}
}

// cfgLoad is one loaded build configuration.
type cfgLoad struct {
	cfg BuildConfig
	// initial is what packages.Load returned (test variants included when
	// cfg.Tests); every dependency is reachable from it via packages.Visit.
	initial []*packages.Package
	// roots are the main packages that root reachability.
	roots []*packages.Package
	// module lists the in-module, non-test-variant packages (production code).
	module []*packages.Package
}

// Loaded is every configuration of one run.
type Loaded struct {
	Root   string // absolute
	Module string
	Prod   []*cfgLoad
	// TestCfgs are loaded one at a time by reachability and dropped after
	// their analysis: holding every config at once costs gigabytes.
	TestCfgs []BuildConfig
	// compiled holds every root-relative file some analysed config compiles
	// (test files included); a file outside it belongs to a build this run
	// never sees, such as js && test.
	compiled map[string]bool
}

const loadMode = packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
	packages.NeedImports | packages.NeedDeps | packages.NeedTypes | packages.NeedSyntax |
	packages.NeedTypesInfo | packages.NeedModule

// Load loads every configuration. Type errors never abort: a package with
// errors is still measured, and is left out of that configuration's
// reachability (see Reachability).
func Load(cfg Config) (*Loaded, error) {
	root, err := filepath.Abs(cfg.Root)
	if err != nil {
		return nil, err
	}
	l := &Loaded{Root: root, Module: cfg.Module, compiled: map[string]bool{}}
	for _, bc := range cfg.Prod {
		cl, err := loadOne(root, cfg.Module, bc)
		if err != nil {
			return nil, err
		}
		l.noteCompiled(cl)
		l.Prod = append(l.Prod, cl)
	}
	if !cfg.Fast {
		l.TestCfgs = cfg.TestCfgs
	}
	return l, nil
}

func loadOne(root, module string, bc BuildConfig) (*cfgLoad, error) {
	env := append(os.Environ(), "GOOS="+bc.GOOS, "GOARCH="+bc.GOARCH, "GOFLAGS=")
	if bc.GOOS == "js" {
		env = append(env, "CGO_ENABLED=0")
	} else {
		env = append(env, "CGO_ENABLED=1")
	}
	var flags []string
	if bc.Tags != "" {
		flags = append(flags, "-tags="+bc.Tags)
	}
	if bc.ModFile != "" {
		flags = append(flags, "-modfile="+bc.ModFile)
	}
	pc := &packages.Config{Mode: loadMode, Dir: root, Env: env, BuildFlags: flags, Tests: bc.Tests}
	initial, err := packages.Load(pc, bc.Patterns...)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", bc.Name, err)
	}
	cl := &cfgLoad{cfg: bc, initial: initial}

	rootPaths := map[string]bool{}
	if len(bc.Roots) > 0 && !bc.Tests {
		rp, err := packages.Load(&packages.Config{Mode: packages.NeedName, Dir: root, Env: env, BuildFlags: flags}, bc.Roots...)
		if err != nil {
			return nil, fmt.Errorf("load %s roots: %w", bc.Name, err)
		}
		for _, p := range rp {
			if p.Name == "main" {
				rootPaths[p.PkgPath] = true
			}
		}
	}
	seen := map[string]bool{}
	nErr := 0
	packages.Visit(initial, nil, func(p *packages.Package) {
		if seen[p.ID] {
			return
		}
		seen[p.ID] = true
		if !inModule(p.PkgPath, module) && !strings.HasSuffix(p.PkgPath, ".test") {
			return
		}
		nErr += len(p.Errors)
		switch {
		case bc.Tests && p.Name == "main" && strings.HasSuffix(p.ID, ".test"):
			cl.roots = append(cl.roots, p)
		case !bc.Tests && p.Name == "main" && rootPaths[p.PkgPath] && p.ID == p.PkgPath:
			cl.roots = append(cl.roots, p)
		}
		if p.ID == p.PkgPath && inModule(p.PkgPath, module) {
			cl.module = append(cl.module, p)
		}
	})
	if nErr > 0 {
		fmt.Fprintf(os.Stderr, "gograph rank: %s: %d package errors (affected packages are measured but skipped for reachability)\n", bc.Name, nErr)
	}
	sort.Slice(cl.module, func(i, j int) bool { return cl.module[i].PkgPath < cl.module[j].PkgPath })
	sort.Slice(cl.roots, func(i, j int) bool { return cl.roots[i].ID < cl.roots[j].ID })
	return cl, nil
}

// noteCompiled records every in-module file a loaded config compiles.
func (l *Loaded) noteCompiled(c *cfgLoad) {
	packages.Visit(c.initial, nil, func(p *packages.Package) {
		if !inModule(p.PkgPath, l.Module) && !strings.HasSuffix(p.PkgPath, ".test") {
			return
		}
		for _, f := range p.GoFiles {
			if rel, ok := relFile(l.Root, f); ok {
				l.compiled[rel] = true
			}
		}
	})
}

// inModule reports whether pkgPath is the module or a package under it.
func inModule(pkgPath, module string) bool {
	return pkgPath == module || strings.HasPrefix(pkgPath, module+"/")
}

// relPkg is a package's path relative to the module ("." for the root).
func relPkg(pkgPath, module string) string {
	if pkgPath == module {
		return "."
	}
	return strings.TrimPrefix(pkgPath, module+"/")
}
