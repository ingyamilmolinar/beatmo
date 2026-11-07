package rank

// Weights is every threshold and weight the ranking uses. Payoffs are in "complexity points", calibrated to cognitive
// complexity: splitting a function removes roughly the cognitive complexity above SplitKeepCog.
type Weights struct {
	// split-function: cognitive at or above SplitCog, or SLOC at or above SplitSLOC.
	SplitCog, SplitSLOC int
	SplitKeepCog        int     // cognitive a well-split piece may keep
	SplitSLOCFree       int     // SLOC that costs nothing
	SplitSLOCPer        float64 // payoff per SLOC beyond SplitSLOCFree
	// flatten: nesting at or above FlattenNesting, or a nesting bonus at least
	// FlattenBonusMin that is FlattenBonusShare of the cognitive total.
	FlattenNesting    int
	FlattenBonusMin   int
	FlattenBonusShare float64
	// simplify-verbose triggers and payoff factors.
	VerbRepeatedLines, VerbSimilarCases, VerbChain, VerbCondOps int
	VerbChainFree, VerbCondFree                                 int
	// Secondary signals of a unit add this share of their payoff.
	SecondaryShare float64
	// delete-unreachable / move-test-only payoff per line (+ cognitive for dead).
	DeadPerLine, TestOnlyPerLine float64
	// clones: payoff per redundant line.
	ClonePerLine float64
	// extract-struct: a type with GodMinMethods methods; a file with
	// ExtractMinExclusive fields only its methods touch.
	GodMinMethods, ExtractMinExclusive int
	// ExtractMaxShare: a file holding more than this share of the type's
	// methods is the type itself, not a slice that could move out.
	ExtractMaxShare                  float64
	ExtractPerField, ExtractCogShare float64
	ExtractSharedRisk                float64
	// split-file: files at or above SplitFileLines; payoff per line above
	// SplitFileKeep. A file whose largest unit is SplitFileOneUnit of its lines
	// is that function: split-function covers it.
	SplitFileLines, SplitFileKeep int
	SplitFilePer                  float64
	SplitFileOneUnit              float64
	// restructure-area: the top AreaTop areas with at least AreaMinUnits units.
	AreaTop, AreaMinUnits int
	AreaPayoffScale       float64 // payoff = scale × sqrt(compound)
	// restructure-type: the top TypeTop god types; payoff = AreaPayoffScale × √(Σ method cognitive^1.3) + TypeFieldWeight
	// × fields, because part of a god type's cost is its state, which no function metric sees.
	TypeTop         int
	TypeFieldWeight float64
	// data-to-asset: data units of at least DataMinLOC lines.
	DataMinLOC int
	DataPer    float64
	// ToolShare scales the payoff of candidates entirely inside dev tooling.
	ToolShare float64
	// Confidence per rule.
	ConfHigh, ConfMedium, ConfLow float64
	// Risk terms.
	RiskFanIn, RiskUncovered, RiskJSOnly, RiskExported float64
	UnknownCoverage                                    float64
}

// DefaultWeights are the shipped weights. Change them here only, and record
// why in the spec's calibration appendix.
var DefaultWeights = Weights{
	SplitCog: 30, SplitSLOC: 150, SplitKeepCog: 15, SplitSLOCFree: 80, SplitSLOCPer: 0.1,
	FlattenNesting: 5, FlattenBonusMin: 20, FlattenBonusShare: 0.4,
	VerbRepeatedLines: 10, VerbSimilarCases: 4, VerbChain: 6, VerbCondOps: 6,
	VerbChainFree: 4, VerbCondFree: 3,
	SecondaryShare: 0.5,
	DeadPerLine:    0.5, TestOnlyPerLine: 0.25,
	ClonePerLine:  0.6,
	GodMinMethods: 20, ExtractMinExclusive: 8, ExtractMaxShare: 0.5, ExtractPerField: 2, ExtractCogShare: 0.25, ExtractSharedRisk: 0.02,
	SplitFileLines: 1500, SplitFileKeep: 800, SplitFilePer: 0.02, SplitFileOneUnit: 0.5,
	AreaTop: 10, AreaMinUnits: 10, AreaPayoffScale: 2,
	TypeTop: 8, TypeFieldWeight: 0.3,
	DataMinLOC: 200, DataPer: 0.05,
	ToolShare: 0.75,
	ConfHigh:  1, ConfMedium: 0.6, ConfLow: 0.3,
	RiskFanIn: 0.35, RiskUncovered: 1, RiskJSOnly: 0.5, RiskExported: 0.3,
	UnknownCoverage: 0.5,
}
