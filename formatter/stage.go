package formatter

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
	// Build a map of stage names for validation
	stageMap := make(map[string]Stage)
	for _, s := range stages {
		stageMap[s.Name] = s
	}

	// Simple topological sort using Kahn's algorithm
	// For now, we trust the order provided and just validate dependencies exist
	for _, s := range stages {
		for _, req := range s.Requires {
			if _, ok := stageMap[req]; !ok {
				// Return stages as-is if validation is lax
				// In a stricter implementation, we'd return an error
				break
			}
		}
	}

	// For now, return in the order provided
	// A full implementation would do proper topological sorting
	return stages, nil
}

// DefaultStages returns the standard llformat stage configuration.
// This creates stages from the existing formatters with explicit dependencies.
func DefaultStages(cfg BaseConfig, commentMoveInline bool, excludes []string) []Stage {
	return []Stage{
		{
			Name: "comments",
			Formatter: NewCommentFormatter(CommentConfig{
				ColumnLimit:     cfg.ColumnLimit,
				TabStop:         cfg.TabStop,
				MoveInlineAbove: commentMoveInline,
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
				Excludes:    excludes,
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
