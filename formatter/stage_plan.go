package formatter

// StageMode describes whether a stage runs via the DSL engine or is disabled.
type StageMode string

const (
	StageModeOff StageMode = "off"
	StageModeDSL StageMode = "dsl"
)

// StagePlan is the coherent per-stage execution plan for a pipeline run.
//
// llformat is next-only: callers can override the per-stage DSL/off plan, but
// there is no longer a user-facing "profile" selector.
type StagePlan struct {
	Comments       StageMode
	LogCalls       StageMode
	Expressions    StageMode
	MultiLineCalls StageMode
	Signatures     StageMode
	BlankLines     StageMode
}

func allDSLStagePlan() StagePlan {
	return StagePlan{
		Comments:       StageModeDSL,
		LogCalls:       StageModeDSL,
		Expressions:    StageModeDSL,
		MultiLineCalls: StageModeDSL,
		Signatures:     StageModeDSL,
		BlankLines:     StageModeDSL,
	}
}

func stagePlanFromOptions(opts StageOptions) StagePlan {
	if opts.Selection.StagePlan != nil {
		return *opts.Selection.StagePlan
	}

	// llformat is next-only: run all DSL stages by default.
	return allDSLStagePlan()
}

func stageModeFromBool(useDSL bool) StageMode {
	if useDSL {
		return StageModeDSL
	}
	return StageModeOff
}
