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
	// This allows explicit dependency declaration rather than implicit
	// ordering.
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

// StageOrder validates and returns the execution order for stages. Returns an
// error if there are cycles or missing dependencies.
func StageOrder(stages []Stage) ([]Stage, error) {
	// Kahn's algorithm with stable ordering.
	stageMap, order, err := buildStageMap(stages)
	if err != nil {
		return nil, err
	}

	inDegree, dependents, err := buildDependencies(stages, stageMap)
	if err != nil {
		return nil, err
	}

	ready := initialReady(order, inDegree)
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

func buildStageMap(stages []Stage) (map[string]Stage, []string, error) {
	stageMap := make(map[string]Stage, len(stages))
	order := make([]string, 0, len(stages))
	for _, s := range stages {
		if s.Name == "" {
			return nil, nil, fmt.Errorf("stage with empty name")
		}
		if _, exists := stageMap[s.Name]; exists {
			return nil, nil, fmt.Errorf(
				"duplicate stage name: %q", s.Name,
			)
		}
		stageMap[s.Name] = s
		order = append(order, s.Name)
	}

	return stageMap, order, nil
}

func buildDependencies(stages []Stage, stageMap map[string]Stage) (
	map[string]int, map[string][]string, error) {

	inDegree := make(map[string]int, len(stages))
	dependents := make(map[string][]string, len(stages))

	for _, s := range stages {
		inDegree[s.Name] = 0
	}

	for _, s := range stages {
		for _, req := range s.Requires {
			if _, ok := stageMap[req]; !ok {
				return nil, nil, fmt.Errorf("stage %q "+
					"requires missing stage %q", s.Name,
					req)
			}
			inDegree[s.Name]++
			dependents[req] = append(dependents[req], s.Name)
		}
	}

	return inDegree, dependents, nil
}

func initialReady(order []string, inDegree map[string]int) []string {
	ready := make([]string, 0, len(order))
	for _, name := range order {
		if inDegree[name] == 0 {
			ready = append(ready, name)
		}
	}

	return ready
}

// StageOptions contains options for configuring the stage pipeline.
type StageOptions struct {
	Selection StageSelectionOptions
	Style     StageStyleOptions
	DSL       DSLStageOptions
}

type StageSelectionOptions struct {
	StagePlan *StagePlan
}

type StageStyleOptions struct {
	CommentMoveInline bool
	CommentMode       string
	Excludes          []string

	DSLMultiLineStyle string
	DSLSigsStyle      string
	// DSLLogCallsMinTailLen controls how aggressively the "next" log/printf
	// call formatter avoids leaving a tiny remainder segment on the next
	// line when splitting a long string literal. A value of 0 uses the
	// profile default.
	DSLLogCallsMinTailLen int
	// DSLLogCallsSelectorNames overrides the set of recognized printf-style
	// selector names for suffix-only matching. When empty, a built-in
	// default set is used.
	DSLLogCallsSelectorNames []string
	// DSLLogCallsSelectorPrefixes restricts log/printf call selection to
	// calls whose selector receiver expression has one of these prefixes.
	// When empty, the next profile targets any selector prefix.
	DSLLogCallsSelectorPrefixes []string
	// DSLBlankLinesExtraIfErrReturn controls whether native DSL blank line
	// rules should insert a blank line before `if err != nil { return ...
	// }` patterns.
	DSLBlankLinesExtraIfErrReturn bool

	DSLExprLogicalStyle       string
	DSLExprArithmeticStyle    string
	DSLExprCaseClauseStyle    string
	DSLExprSelectorChainStyle string
}

type DSLStageOptions struct {
	Trace        bool
	TraceReasons bool

	UseBlankLinesNative bool
	UseFuncSigsNative   bool

	AllowCallArgs bool
	AutoCallArgs  bool
}

// DefaultStages returns the standard llformat stage configuration. This creates
// stages from the existing formatters with explicit dependencies.
func DefaultStages(cfg BaseConfig, commentMoveInline bool,
	excludes []string) []Stage {

	return DefaultStagesWithOptions(
		cfg, StageOptions{
			Selection: StageSelectionOptions{},
			Style: StageStyleOptions{
				CommentMoveInline: commentMoveInline,
				CommentMode:       "prose",
				Excludes:          excludes,
			},
		},
	)
}

// DefaultStagesWithOptions returns stages with full configuration options.
func DefaultStagesWithOptions(cfg BaseConfig, opts StageOptions) []Stage {
	dslBundle := ResolveDSLBundle(opts)
	stagePlan := stagePlanFromOptions(opts)

	commentFormatter := buildCommentStageFormatter(
		"comments", cfg, opts, stagePlan, dslBundle,
	)
	callFormatter := buildCompactCallStageFormatter(
		"compact-calls", cfg, opts, stagePlan, dslBundle,
	)
	exprFormatter := buildExpressionStageFormatter(
		"expressions", cfg, opts, stagePlan, dslBundle,
	)
	multiLineFormatter := buildMultiLineCallStageFormatter(
		"multiline-calls", cfg, opts, stagePlan, dslBundle,
	)
	signatureFormatter := buildSignatureStageFormatter(
		"signatures", cfg, opts, stagePlan, dslBundle,
	)
	blankLineFormatter := buildBlankLineStageFormatter(
		"blank-lines", cfg, opts, stagePlan, dslBundle,
	)

	return []Stage{
		{
			Name:      "comments",
			Formatter: commentFormatter,
			Requires:  nil, // First stage, no dependencies
		},
		{
			Name:      "compact-calls",
			Formatter: callFormatter,
			Requires: []string{
				"comments",
			}, // After comment formatting
		},
		{
			Name:      "expressions",
			Formatter: exprFormatter,
			Requires: []string{
				"compact-calls",
			}, // After call formatting
		},
		{
			Name:      "multiline-calls",
			Formatter: multiLineFormatter,
			Requires: []string{
				"expressions",
			}, // After expression formatting
		},
		{
			Name:      "signatures",
			Formatter: signatureFormatter,
			Requires: []string{
				"multiline-calls",
			}, // After call formatting
		},
		{
			Name:      "blank-lines",
			Formatter: blankLineFormatter,
			Requires: []string{
				"signatures",
			}, // After signature formatting
		},
	}
}
