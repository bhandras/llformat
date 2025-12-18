package formatter

import (
	formatstd "go/format"

	llast "github.com/lightninglabs/llformat/ast"
	"github.com/lightninglabs/llformat/dsl"
)

// DSLExprFormatter uses the DSL engine to format expressions.
type DSLExprFormatter struct {
	engine                *dsl.Engine
	applyLegacyBlankLines bool
	skipGofmt             bool

	stageName    string
	ownedSpansFn func(src []byte) llast.OffsetSpanSet
}

// DSLExprConfig holds configuration for the DSL expression formatter.
type DSLExprConfig struct {
	ColumnLimit   int
	TabStop       int
	Rules         []dsl.Rule // Custom rules (if nil, uses defaults)
	Trace         bool       // Enable DSL rule tracing to stderr
	TraceReasons  bool       // Include "why fired/didn't fire" reasons in DSL tracing
	NodeOrder     dsl.NodeOrder
	MaxIterations int  // Override engine MaxIterations (0 keeps default)
	SkipGofmt     bool // Skip gofmt (pipelines may run gofmt once at end)

	StageName string

	// OwnedSpansFunc optionally declares which regions of the source this stage
	// "owns" for pipeline-level stage fighting prevention. When nil, the stage
	// declares no ownership.
	OwnedSpansFunc func(src []byte) llast.OffsetSpanSet

	// Budget provides optional safety guardrails for the DSL engine.
	Budget dsl.RewriteBudget

	// DisableLegacyBlankLinesShim controls whether DSL blank-line rules are
	// delegated to the legacy blank-line formatter for parity. When true, DSL
	// blank-line rules are executed directly by the DSL engine.
	DisableLegacyBlankLinesShim bool
}

// NewDSLExprFormatter creates a new DSL-based expression formatter.
func NewDSLExprFormatter(cfg DSLExprConfig) *DSLExprFormatter {
	rules := cfg.Rules
	if rules == nil {
		rules = dsl.DefaultRules()
	}

	applyLegacyBlankLines := hasDSLBlankLineRules(rules) && !cfg.DisableLegacyBlankLinesShim
	if applyLegacyBlankLines {
		rules = filterDSLBlankLineRules(rules)
	}

	engine := dsl.NewEngine(rules)
	if cfg.ColumnLimit > 0 {
		engine.ColumnLimit = cfg.ColumnLimit
	}
	if cfg.TabStop > 0 {
		engine.TabStop = cfg.TabStop
	}
	engine.Trace = cfg.Trace
	engine.TraceReasons = cfg.TraceReasons
	engine.NodeOrder = cfg.NodeOrder
	engine.Budget = cfg.Budget
	engine.StageName = cfg.StageName
	if cfg.MaxIterations > 0 {
		engine.MaxIterations = cfg.MaxIterations
	}

	return &DSLExprFormatter{
		engine:                engine,
		applyLegacyBlankLines: applyLegacyBlankLines,
		skipGofmt:             cfg.SkipGofmt,
		stageName:             cfg.StageName,
		ownedSpansFn:          cfg.OwnedSpansFunc,
	}
}

func (f *DSLExprFormatter) SetOwnershipRegistry(reg *OwnershipRegistry) {
	policy := NewOwnershipPolicy(reg, f.stageName)
	f.engine.ForbiddenSpans = policy.ForbiddenSpans()
}

func (f *DSLExprFormatter) OwnedSpans(src []byte) llast.OffsetSpanSet {
	if f == nil || f.ownedSpansFn == nil {
		return llast.OffsetSpanSet{}
	}
	return f.ownedSpansFn(src)
}

// FormatFile formats the source file using DSL rules.
func (f *DSLExprFormatter) FormatFile(src []byte) []byte {
	out := f.engine.FormatFile(src)

	if !f.applyLegacyBlankLines {
		if f.skipGofmt {
			return out
		}
		if formatted, err := formatstd.Source(out); err == nil {
			return formatted
		}
		return out
	}

	// Preserve existing llformat behavior for blank line insertion by running
	// the legacy blank-line formatter after DSL transformations.
	out = NewBlankLineFormatter(BlankLineConfig{
		BeforeReturn:            true,
		BetweenCases:            true,
		BetweenInterfaceMethods: true,
	}).FormatFile(out)

	if f.skipGofmt {
		return out
	}
	if formatted, err := formatstd.Source(out); err == nil {
		return formatted
	}
	return out
}

func hasDSLBlankLineRules(rules []dsl.Rule) bool {
	for _, r := range rules {
		switch r.Name {
		case "blank_before_case", "blank_before_return", "blank_between_interface_methods":
			return true
		}
	}
	return false
}

func filterDSLBlankLineRules(rules []dsl.Rule) []dsl.Rule {
	filtered := make([]dsl.Rule, 0, len(rules))
	for _, r := range rules {
		switch r.Name {
		case "blank_before_case", "blank_before_return", "blank_between_interface_methods":
			continue
		default:
			filtered = append(filtered, r)
		}
	}
	return filtered
}
