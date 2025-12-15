package formatter

import (
	"fmt"

	"github.com/lightninglabs/llformat/dsl"
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
	CommentMoveInline bool
	Excludes          []string
	UseDSLExpr        bool // Use DSL-based formatter (now the default)
	TraceDSL          bool // Enable DSL rule tracing (only when UseDSLExpr)

	// AllowDSLCallArgs enables limited expression formatting within call
	// arguments when using the DSL expression stage.
	// This is intentionally opt-in because it can interact with call formatting.
	AllowDSLCallArgs bool

	// AutoDSLCallArgs enables limited expression formatting within call arguments
	// only for calls excluded from multiline formatting.
	AutoDSLCallArgs bool
}

// DefaultStages returns the standard llformat stage configuration.
// This creates stages from the existing formatters with explicit dependencies.
func DefaultStages(cfg BaseConfig, commentMoveInline bool, excludes []string) []Stage {
	return DefaultStagesWithOptions(cfg, StageOptions{
		CommentMoveInline: commentMoveInline,
		Excludes:          excludes,
		UseDSLExpr:        false,
	})
}

// DefaultStagesWithOptions returns stages with full configuration options.
func DefaultStagesWithOptions(cfg BaseConfig, opts StageOptions) []Stage {
	if opts.UseDSLExpr {
		exprRules := dsl.LongExprRules()
		if opts.AllowDSLCallArgs || opts.AutoDSLCallArgs {
			callArgsPolicy := dsl.CallArgsPolicyOff
			if opts.AutoDSLCallArgs {
				callArgsPolicy = dsl.CallArgsPolicyAuto
			}
			if opts.AllowDSLCallArgs {
				callArgsPolicy = dsl.CallArgsPolicyForce
			}

			exprRules = dsl.LongExprRulesWithOptions(dsl.LongExprOptions{
				CallArgsPolicy:    callArgsPolicy,
				CallArgsAllowlist: opts.Excludes,
			})
		}

		// Legacy stage pipeline with DSL expression formatting.
		return []Stage{
			{
				Name: "comments",
				Formatter: NewCommentFormatter(CommentConfig{
					ColumnLimit:     cfg.ColumnLimit,
					TabStop:         cfg.TabStop,
					MoveInlineAbove: opts.CommentMoveInline,
				}),
				Requires: nil,
			},
			{
				Name: "compact-calls",
				Formatter: NewCompactCallFormatter(Config{
					ColumnLimit: cfg.ColumnLimit,
					TabStop:     cfg.TabStop,
					SkipGofmt:   true,
				}),
				Requires: []string{"comments"},
			},
			{
				Name: "expressions",
				Formatter: NewDSLExprFormatter(DSLExprConfig{
					ColumnLimit: cfg.ColumnLimit,
					TabStop:     cfg.TabStop,
					Rules:       exprRules,
					Trace:       opts.TraceDSL,
					SkipGofmt:   true,
				}),
				Requires: []string{"compact-calls"},
			},
			{
				Name: "multiline-calls",
				Formatter: NewMultiLineCallFormatter(MultiLineConfig{
					ColumnLimit: cfg.ColumnLimit,
					TabStop:     cfg.TabStop,
					Excludes:    opts.Excludes,
					SkipGofmt:   true,
				}),
				Requires: []string{"expressions"},
			},
			{
				Name: "signatures",
				Formatter: NewFuncSigFormatter(FuncSigConfig{
					ColumnLimit: cfg.ColumnLimit,
					TabStop:     cfg.TabStop,
				}),
				Requires: []string{"multiline-calls"},
			},
			{
				Name: "blank-lines",
				Formatter: NewBlankLineFormatter(BlankLineConfig{
					BeforeReturn:            true,
					BetweenCases:            true,
					BetweenInterfaceMethods: true,
				}),
				Requires: []string{"signatures"},
			},
		}
	}

	// Legacy pipeline - kept for backwards compatibility
	return []Stage{
		{
			Name: "comments",
			Formatter: NewCommentFormatter(CommentConfig{
				ColumnLimit:     cfg.ColumnLimit,
				TabStop:         cfg.TabStop,
				MoveInlineAbove: opts.CommentMoveInline,
			}),
			Requires: nil, // First stage, no dependencies
		},
		{
			Name: "compact-calls",
			Formatter: NewCompactCallFormatter(Config{
				ColumnLimit: cfg.ColumnLimit,
				TabStop:     cfg.TabStop,
				SkipGofmt:   true,
			}),
			Requires: []string{"comments"}, // After comment formatting
		},
		{
			Name: "expressions",
			Formatter: NewLongExprFormatter(LongExprConfig{
				ColumnLimit: cfg.ColumnLimit,
				TabStop:     cfg.TabStop,
			}),
			Requires: []string{"compact-calls"}, // After call formatting
		},
		{
			Name: "multiline-calls",
			Formatter: NewMultiLineCallFormatter(MultiLineConfig{
				ColumnLimit: cfg.ColumnLimit,
				TabStop:     cfg.TabStop,
				Excludes:    opts.Excludes,
				SkipGofmt:   true,
			}),
			Requires: []string{"expressions"}, // After expression formatting
		},
		{
			Name: "signatures",
			Formatter: NewFuncSigFormatter(FuncSigConfig{
				ColumnLimit: cfg.ColumnLimit,
				TabStop:     cfg.TabStop,
			}),
			Requires: []string{"multiline-calls"}, // After call formatting
		},
		{
			Name: "blank-lines",
			Formatter: NewBlankLineFormatter(BlankLineConfig{
				BeforeReturn:            true,
				BetweenCases:            true,
				BetweenInterfaceMethods: true,
			}),
			Requires: []string{"signatures"}, // After signature formatting
		},
	}
}
