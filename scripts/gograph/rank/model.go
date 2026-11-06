package rank

import "strings"

// Unit is one function, method or closure of production code.
type Unit struct {
	ID        string   `json:"id"`   // file:line:name (closures: file:line:col)
	Pkg       string   `json:"pkg"`  // module-relative package path
	File      string   `json:"file"` // root-relative file path
	Name      string   `json:"name"` // F, T.M, or T.M.func1
	Recv      string   `json:"recv,omitempty"`
	Kind      string   `json:"kind"` // func | method | closure
	Parent    string   `json:"parent,omitempty"`
	Line      int      `json:"line"`
	EndLine   int      `json:"endLine"`
	Exported  bool     `json:"exported,omitempty"`
	Generated bool     `json:"generated,omitempty"`
	Data      bool     `json:"data,omitempty"`
	JSOnly    bool     `json:"jsOnly,omitempty"`
	Tool      bool     `json:"tool,omitempty"` // dev tooling (Config.ToolPrefixes)
	Configs   []string `json:"configs"`

	LOC       int       `json:"loc"`
	SLOC      int       `json:"sloc"`
	OwnSLOC   int       `json:"ownSloc"` // SLOC minus nested closures (their own units)
	Cyclo     int       `json:"cyclo"`
	Cognitive int       `json:"cognitive"`
	NestBonus int       `json:"nestBonus"`
	Nesting   int       `json:"nesting"`
	Params    int       `json:"params"`
	Results   int       `json:"results"`
	FanIn     int       `json:"fanIn"`
	FanOut    int       `json:"fanOut"`
	DataRatio float64   `json:"dataRatio"`
	Verb      Verbosity `json:"verb"`
	Coverage  *float64  `json:"coverage,omitempty"`
	Reach     string    `json:"reach,omitempty"` // prod | test-only | dead | unsplit | variant
	// UnanalyzedRefs are files no analysed config compiles that name this
	// unreachable unit: deleting it could break their build.
	UnanalyzedRefs []string `json:"unanalyzedRefs,omitempty"`
	TestRefs       []string `json:"testRefs,omitempty"`
	// CrossPkgTestRefs are test files of OTHER packages that import this
	// package and name this (exported) unit: it cannot move into _test.go.
	CrossPkgTestRefs []string `json:"crossPkgTestRefs,omitempty"`
	declKey          string   // file:nameLine:bareName, joins typed objects to units
	bareName         string
	recvIdent        string // receiver variable name, "" if unnamed
	callers          map[string]bool
	callees          map[string]bool
	fieldsUsed       map[string]bool
}

// Verbosity holds deterministic proxies for "longer or harder to read than it
// needs to be".
type Verbosity struct {
	RepeatedLines int `json:"repeatedLines,omitempty"` // lines in 3-statement runs repeated inside the unit
	SimilarCases  int `json:"similarCases,omitempty"`  // switch arms sharing a body shape with another arm
	MaxChain      int `json:"maxChain,omitempty"`      // deepest selector chain a.b.c.d (= 4)
	MaxCondOps    int `json:"maxCondOps,omitempty"`    // most &&/|| in one condition
}

// File is one production source file.
type File struct {
	Path       string   `json:"path"`
	Pkg        string   `json:"pkg"`
	Area       string   `json:"area"`
	Lines      int      `json:"lines"`
	Units      int      `json:"units"`
	TopUnitLOC int      `json:"topUnitLoc"`
	Generated  bool     `json:"generated,omitempty"`
	Configs    []string `json:"configs"`
}

// TypeStat is one struct type and how its methods spread over files.
type TypeStat struct {
	Pkg          string          `json:"pkg"`
	Name         string          `json:"name"`
	Fields       int             `json:"fields"`
	Methods      int             `json:"methods"`
	MethodLOC    int             `json:"methodLoc"`
	FilesSpanned int             `json:"filesSpanned"`
	PerFile      []FileReadiness `json:"perFile,omitempty"`
}

// FileReadiness is how separable one file's slice of a type is: fields only
// that file's methods touch could move with them into a new type.
type FileReadiness struct {
	File       string   `json:"file"`
	Methods    int      `json:"methods"`
	MethodCog  int      `json:"methodCog"`
	FieldsUsed int      `json:"fieldsUsed"`
	Exclusive  int      `json:"exclusive"`
	Fields     []string `json:"exclusiveFields,omitempty"`
}

// AreaStat is compounded complexity of a package or of an area (a file-prefix
// group inside a large package).
type AreaStat struct {
	Name       string   `json:"name"`
	Pkg        string   `json:"pkg"`
	Files      int      `json:"files"`
	Lines      int      `json:"lines"`
	Units      int      `json:"units"`
	SumCog     int      `json:"sumCog"`
	Over15     int      `json:"over15"`
	Over30     int      `json:"over30"`
	Over60     int      `json:"over60"`
	CloneLOC   int      `json:"cloneLoc"`
	DeadLOC    int      `json:"deadLoc"`
	ExtCallees int      `json:"extCallees"`
	ExtCallers int      `json:"extCallers"`
	Gini       float64  `json:"gini"`
	Coupling   float64  `json:"coupling"`
	Compound   float64  `json:"compound"`
	TopUnits   []string `json:"topUnits,omitempty"`
}

// CloneGroup is a set of code regions with the same normalised shape.
type CloneGroup struct {
	ID       string     `json:"id"`
	Tier     string     `json:"tier"` // exact | renamed | structural
	Lines    int        `json:"lines"`
	Members  []CloneLoc `json:"members"`
	CrossPkg bool       `json:"crossPkg,omitempty"`
	Whole    bool       `json:"whole,omitempty"` // whole function bodies
}

// CloneLoc is one member region of a clone group.
type CloneLoc struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	EndLine int    `json:"endLine"`
	Unit    string `json:"unit"` // unit ID
}

// Totals summarise the module for baseline comparisons.
type Totals struct {
	Units          int `json:"units"`
	SLOC           int `json:"sloc"`
	SumCognitive   int `json:"sumCognitive"`
	SumCyclo       int `json:"sumCyclo"`
	UnitsOverCog15 int `json:"unitsOverCog15"`
	UnitsOverCog30 int `json:"unitsOverCog30"`
	UnitsOverCog60 int `json:"unitsOverCog60"`
	CloneGroups    int `json:"cloneGroups"`
	CloneLOC       int `json:"cloneLoc"`
	DeadUnits      int `json:"deadUnits"`
	DeadLOC        int `json:"deadLoc"`
	TestOnlyUnits  int `json:"testOnlyUnits"`
	TestOnlyLOC    int `json:"testOnlyLoc"`
	UnsplitUnits   int `json:"unsplitUnits,omitempty"`
	GeneratedSLOC  int `json:"generatedSloc"`
	FilesOver1500  int `json:"filesOver1500"`
	MergedIDs      int `json:"mergedIds"` // must stay 0: two units sharing one ID
}

// Model is everything measured in one run.
type Model struct {
	Module       string        `json:"module"`
	Configs      []string      `json:"configs"`
	Totals       Totals        `json:"totals"`
	Units        []*Unit       `json:"-"`
	Files        []*File       `json:"files"`
	Types        []*TypeStat   `json:"types"`
	Areas        []*AreaStat   `json:"areas"`
	Pkgs         []*AreaStat   `json:"packages"`
	Clones       []*CloneGroup `json:"clones"`
	Warnings     []string      `json:"warnings,omitempty"`
	src          *sourceSet
	toolPrefixes []string
	byID         map[string]*Unit
}

// Analyze runs every measurement pass over cfg.
func Analyze(cfg Config) (*Model, error) {
	l, err := Load(cfg)
	if err != nil {
		return nil, err
	}
	src, err := collectSources(l)
	if err != nil {
		return nil, err
	}
	m := &Model{Module: cfg.Module, src: src}
	for _, c := range l.Prod {
		m.Configs = append(m.Configs, c.cfg.Name)
	}
	for _, c := range l.TestCfgs {
		m.Configs = append(m.Configs, c.Name)
	}
	m.Units = collectUnits(src)
	for _, u := range m.Units {
		u.Tool = isTool(u.Pkg, cfg.ToolPrefixes)
	}
	m.toolPrefixes = cfg.ToolPrefixes
	m.byID = map[string]*Unit{}
	for _, u := range m.Units {
		if m.byID[u.ID] != nil {
			m.Totals.MergedIDs++
		}
		m.byID[u.ID] = u
	}
	computeFans(l, m)
	if err := reachability(l, m, cfg.Fast); err != nil {
		return nil, err
	}
	l.Prod = nil // release the type-checked syntax before the source-only passes
	warns, err := applyCoverage(m, l.Root, cfg.Module, cfg.Covers)
	if err != nil {
		return nil, err
	}
	m.Warnings = append(m.Warnings, warns...)
	collectTestRefs(m, l.Root, cfg.Module)
	m.Clones = findClones(m)
	buildStructure(m)
	computeTotals(m)
	return m, nil
}

// isTool reports whether a module-relative path is dev tooling.
func isTool(pkg string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(pkg, p) {
			return true
		}
	}
	return false
}
