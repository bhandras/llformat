package formatter

import (
	"fmt"
)

// Stage represents a named formatting stage in the pipeline.
type Stage struct {
	// Name uniquely identifies this stage.
	Name string

	// Formatter is the formatter to apply.
	Formatter Formatter

	// Requires lists the names of stages that must run before this one.
	// This allows explicit dependency declaration rather than implicit ordering.
	Requires []string
}

// NewStage creates a new stage with the given name and formatter.
func NewStage(name string, formatter Formatter) Stage {
	return Stage{
		Name:      name,
		Formatter: formatter,
		Requires:  nil,
	}
}

// WithRequires returns a new Stage with the given dependencies.
func (s Stage) WithRequires(requires ...string) Stage {
	s.Requires = append(s.Requires, requires...)
	return s
}

// StageOrder validates and returns the execution order for stages.
// Returns an error if there are cycles or missing dependencies.
func StageOrder(stages []Stage) ([]Stage, error) {
	// Kahn's algorithm with stable ordering.
	stageMap := make(map[string]Stage, len(stages))
	order := make([]string, 0, len(stages))
	for _, s := range stages {
		if s.Name == "" {
			return nil, fmt.Errorf("stage with empty name")
		}
		if _, exists := stageMap[s.Name]; exists {
			return nil, fmt.Errorf("duplicate stage name: %q", s.Name)
		}
		stageMap[s.Name] = s
		order = append(order, s.Name)
	}

	inDegree := make(map[string]int, len(stages))
	dependents := make(map[string][]string, len(stages))

	for _, s := range stages {
		inDegree[s.Name] = 0
	}

	for _, s := range stages {
		for _, req := range s.Requires {
			if _, ok := stageMap[req]; !ok {
				return nil, fmt.Errorf("stage %q requires missing stage %q", s.Name, req)
			}
			inDegree[s.Name]++
			dependents[req] = append(dependents[req], s.Name)
		}
	}

	ready := make([]string, 0, len(stages))
	for _, name := range order {
		if inDegree[name] == 0 {
			ready = append(ready, name)
		}
	}

	out := make([]Stage, 0, len(stages))
	for len(ready) > 0 {
		// Pop front for stability.
		name := ready[0]
		ready = ready[1:]

		out = append(out, stageMap[name])
		for _, dep := range dependents[name] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				ready = append(ready, dep)
			}
		}
	}

	if len(out) != len(stages) {
		return nil, fmt.Errorf("cycle detected in stage dependencies")
	}

	return out, nil
}

// StageOptions contains options for configuring the stage pipeline.
type StageOptions struct {
	CommentMoveInline        bool
	Excludes                 []string
	RuleProfile              string
	UseDSLComments           bool // Use DSL-based comment stage (delegates to legacy)
	UseDSLLogCalls           bool // Use DSL-based log/printf call stage
	UseDSLMultiLineCalls     bool // Use DSL-based multiline call stage
	DSLMultiLineStyle        string
	UseDSLExpr               bool // Use DSL-based expression stage
	UseDSLFuncSigs           bool // Use DSL-based signature stage (delegates to legacy)
	UseDSLBlankLines         bool // Use DSL-based blank line stage (pure DSL)
	UseDSLBlankLinesNative   bool // Use native DSL blank-line rules (fallback to legacy)
	UseDSLFuncSigsNative     bool // Use native DSL signature rules (fallback to legacy)
	DSLSigsStyle             string
	TraceDSL                 bool // Enable DSL rule tracing (DSL stages only)
	TraceDSLReasons          bool // Include "why fired/didn't fire" reasons (DSL stages only)
	MultiLineUseASTSelect    bool // Use AST-based selection in legacy multiline formatter
	CompactCallUseASTSelect  bool // Use AST-based selection in legacy compact call formatter
	CompactCallParseSafe     bool // Parse-safe behavior in legacy compact call formatter
	LongExprParseSafe        bool // Parse-safe behavior in legacy long expr formatter
	LongExprUseASTSelect     bool // Use AST-based selection in legacy long expr formatter
	LongExprExcludeCallExprs bool // Forbid breaking inside call exprs in legacy long expr formatter
	MultiLineParseSafe       bool // Parse-safe behavior in legacy multiline call formatter

	// AllowDSLCallArgs enables limited expression formatting within call
	// arguments when using the DSL expression stage.
	// This is intentionally opt-in because it can interact with call formatting.
	AllowDSLCallArgs bool

	// AutoDSLCallArgs enables limited expression formatting within call arguments
	// only for calls excluded from multiline formatting.
	AutoDSLCallArgs bool

	// DSLExprLogicalStyle controls long &&/|| chain formatting inside the DSL
	// expression stage. Empty means legacy behavior.
	DSLExprLogicalStyle string

	// DSLExprArithmeticStyle controls long arithmetic chain formatting inside
	// the DSL expression stage. Empty means legacy behavior.
	DSLExprArithmeticStyle string

	// DSLExprCaseClauseStyle controls long `case A, B, ...:` list formatting
	// inside the DSL expression stage. Empty means legacy behavior.
	DSLExprCaseClauseStyle string

	// DSLExprSelectorChainStyle controls long selector chain formatting
	// inside the DSL expression stage. Empty means legacy behavior.
	DSLExprSelectorChainStyle string
}

// DefaultStages returns the standard llformat stage configuration.
// This creates stages from the existing formatters with explicit dependencies.
func DefaultStages(cfg BaseConfig, commentMoveInline bool, excludes []string) []Stage {
	return DefaultStagesWithOptions(cfg, StageOptions{
		CommentMoveInline:        commentMoveInline,
		Excludes:                 excludes,
		RuleProfile:              "parity",
		UseDSLComments:           false,
		UseDSLLogCalls:           false,
		UseDSLMultiLineCalls:     false,
		DSLMultiLineStyle:        "",
		UseDSLExpr:               false,
		UseDSLFuncSigs:           false,
		UseDSLBlankLines:         false,
		UseDSLBlankLinesNative:   false,
		UseDSLFuncSigsNative:     false,
		DSLSigsStyle:             "",
		TraceDSL:                 false,
		TraceDSLReasons:          false,
		MultiLineUseASTSelect:    false,
		CompactCallUseASTSelect:  false,
		CompactCallParseSafe:     false,
		LongExprParseSafe:        false,
		LongExprUseASTSelect:     false,
		LongExprExcludeCallExprs: false,
		MultiLineParseSafe:       false,
	})
}

// DefaultStagesWithOptions returns stages with full configuration options.
func DefaultStagesWithOptions(cfg BaseConfig, opts StageOptions) []Stage {
	dslBundle := dslBundleForOptions(opts)

	var commentFormatter Formatter = NewCommentFormatter(CommentConfig{
		ColumnLimit:     cfg.ColumnLimit,
		TabStop:         cfg.TabStop,
		MoveInlineAbove: opts.CommentMoveInline,
	})
	if opts.UseDSLComments {
		commentFormatter = NewDSLExprFormatter(DSLExprConfig{
			ColumnLimit:   cfg.ColumnLimit,
			TabStop:       cfg.TabStop,
			Rules:         dslBundle.Comments.Rules,
			Trace:         opts.TraceDSL,
			TraceReasons:  opts.TraceDSLReasons,
			MaxIterations: dslBundle.Comments.MaxIterations,
			SkipGofmt:     true,
		})
	}

	var callFormatter Formatter = NewCompactCallFormatter(Config{
		ColumnLimit:     cfg.ColumnLimit,
		TabStop:         cfg.TabStop,
		UseASTSelection: opts.CompactCallUseASTSelect,
		SkipGofmt:       true,
		ParseSafe:       opts.CompactCallParseSafe,
	})
	if opts.UseDSLLogCalls {
		callFormatter = NewDSLExprFormatter(DSLExprConfig{
			ColumnLimit:  cfg.ColumnLimit,
			TabStop:      cfg.TabStop,
			Rules:        dslBundle.LogCalls.Rules,
			Trace:        opts.TraceDSL,
			TraceReasons: opts.TraceDSLReasons,
			SkipGofmt:    true,
		})
	}

	var exprFormatter Formatter = NewLongExprFormatter(LongExprConfig{
		ColumnLimit:      cfg.ColumnLimit,
		TabStop:          cfg.TabStop,
		ParseSafe:        opts.LongExprParseSafe,
		UseASTSelection:  opts.LongExprUseASTSelect,
		ExcludeCallExprs: opts.LongExprExcludeCallExprs,
	})
	if opts.UseDSLExpr {
		exprFormatter = NewDSLExprFormatter(DSLExprConfig{
			ColumnLimit:  cfg.ColumnLimit,
			TabStop:      cfg.TabStop,
			Rules:        dslBundle.Expressions.Rules,
			Trace:        opts.TraceDSL,
			TraceReasons: opts.TraceDSLReasons,
			SkipGofmt:    true,
		})
	}

	var multiLineFormatter Formatter = NewMultiLineCallFormatter(MultiLineConfig{
		ColumnLimit:     cfg.ColumnLimit,
		TabStop:         cfg.TabStop,
		Excludes:        opts.Excludes,
		UseASTSelection: opts.MultiLineUseASTSelect,
		SkipGofmt:       true,
		ParseSafe:       opts.MultiLineParseSafe,
	})
	if opts.UseDSLMultiLineCalls {
		multiLineFormatter = NewDSLExprFormatter(DSLExprConfig{
			ColumnLimit:   cfg.ColumnLimit,
			TabStop:       cfg.TabStop,
			Rules:         dslBundle.MultiLineCalls.Rules,
			Trace:         opts.TraceDSL,
			TraceReasons:  opts.TraceDSLReasons,
			NodeOrder:     dslBundle.MultiLineCalls.NodeOrder,
			MaxIterations: dslBundle.MultiLineCalls.MaxIterations,
			SkipGofmt:     true,
		})
	}

	return []Stage{
		{
			Name:      "comments",
			Formatter: commentFormatter,
			Requires:  nil, // First stage, no dependencies
		},
		{
			Name:      "compact-calls",
			Formatter: callFormatter,
			Requires:  []string{"comments"}, // After comment formatting
		},
		{
			Name:      "expressions",
			Formatter: exprFormatter,
			Requires:  []string{"compact-calls"}, // After call formatting
		},
		{
			Name:      "multiline-calls",
			Formatter: multiLineFormatter,
			Requires:  []string{"expressions"}, // After expression formatting
		},
		{
			Name: "signatures",
			Formatter: func() Formatter {
				if !opts.UseDSLFuncSigs {
					return NewFuncSigFormatter(FuncSigConfig{
						ColumnLimit: cfg.ColumnLimit,
						TabStop:     cfg.TabStop,
					})
				}
				return NewDSLExprFormatter(DSLExprConfig{
					ColumnLimit:   cfg.ColumnLimit,
					TabStop:       cfg.TabStop,
					Rules:         dslBundle.Signatures.Rules,
					Trace:         opts.TraceDSL,
					TraceReasons:  opts.TraceDSLReasons,
					MaxIterations: dslBundle.Signatures.MaxIterations,
					SkipGofmt:     true,
				})
			}(),
			Requires: []string{"multiline-calls"}, // After call formatting
		},
		{
			Name: "blank-lines",
			Formatter: func() Formatter {
				if !opts.UseDSLBlankLines {
					return NewBlankLineFormatter(BlankLineConfig{
						BeforeReturn:            true,
						BetweenCases:            true,
						BetweenInterfaceMethods: true,
					})
				}
				return NewDSLExprFormatter(DSLExprConfig{
					ColumnLimit:                 cfg.ColumnLimit,
					TabStop:                     cfg.TabStop,
					Rules:                       dslBundle.BlankLines.Rules,
					Trace:                       opts.TraceDSL,
					TraceReasons:                opts.TraceDSLReasons,
					MaxIterations:               dslBundle.BlankLines.MaxIterations,
					DisableLegacyBlankLinesShim: dslBundle.BlankLines.DisableLegacyBlankLinesShim,
					SkipGofmt:                   true,
				})
			}(),
			Requires: []string{"signatures"}, // After signature formatting
		},
	}
}
