package formatter

import (
	formatstd "go/format"

	llast "github.com/bhandras/llformat/ast"
	"github.com/bhandras/llformat/dsl"
)

// DSLExprFormatter uses the DSL engine to format expressions.
type DSLExprFormatter struct {
	engine    *dsl.Engine
	skipGofmt bool

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
	MaxIterations int // Override engine MaxIterations (0 keeps default)
	// AutoMaxIterations enables an AST-informed iteration cap (node-count
	// based) rather than a fixed constant. This is intended for stages that
	// legitimately need many iterations (e.g. signature formatting across
	// many declarations) while remaining protected against cycles.
	AutoMaxIterations bool
	DetectCycles      bool
	SkipGofmt         bool // Skip gofmt (pipelines may run gofmt once at end)

	StageName string

	// OwnedSpansFunc optionally declares which regions of the source this
	// stage "owns" for pipeline-level stage fighting prevention. When nil,
	// the stage declares no ownership.
	OwnedSpansFunc func(src []byte) llast.OffsetSpanSet

	// Budget provides optional safety guardrails for the DSL engine.
	Budget dsl.RewriteBudget
}

// NewDSLExprFormatter creates a new DSL-based expression formatter.
func NewDSLExprFormatter(cfg DSLExprConfig) *DSLExprFormatter {
	rules := cfg.Rules
	if rules == nil {
		rules = dsl.DefaultRules()
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
	engine.AutoMaxIterations = cfg.AutoMaxIterations
	engine.DetectCycles = cfg.DetectCycles
	if cfg.AutoMaxIterations {
		// When auto iteration is enabled we intentionally disable the
		// fixed iteration cap so the engine can derive an appropriate
		// limit from the AST. Safety is enforced by budgets + cycle
		// detection.
		engine.MaxIterations = 0
	} else if cfg.MaxIterations > 0 {
		engine.MaxIterations = cfg.MaxIterations
	}

	return &DSLExprFormatter{
		engine:       engine,
		skipGofmt:    cfg.SkipGofmt,
		stageName:    cfg.StageName,
		ownedSpansFn: cfg.OwnedSpansFunc,
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

	if f.skipGofmt {
		return out
	}
	if formatted, err := formatstd.Source(out); err == nil {
		return formatted
	}

	return out
}
