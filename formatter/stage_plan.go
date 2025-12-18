package formatter

// StageMode describes whether a stage runs via legacy formatter code or the DSL
// engine.
type StageMode string

const (
	StageModeLegacy StageMode = "legacy"
	StageModeDSL    StageMode = "dsl"
)

// StagePlan is the coherent per-stage execution plan for a pipeline run.
//
// This is intended to become the primary way a RuleProfile selects "which
// stages are DSL vs legacy" while still allowing explicit overrides.
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

func stagePlanForRuleProfile(profile string) (StagePlan, bool) {
	switch normalizedRuleProfile(profile) {
	case "parity", "modern", "next":
		return allDSLStagePlan(), true
	default:
		return StagePlan{}, false
	}
}

func stagePlanFromOptions(opts StageOptions) StagePlan {
	if opts.StagePlan != nil {
		return *opts.StagePlan
	}

	// If callers opt into a non-parity profile without specifying stage toggles,
	// treat the profile as the stage selector. This keeps legacy parity as the
	// default while allowing "modern"/"next" to be selected more ergonomically.
	if !opts.UseDSLComments &&
		!opts.UseDSLLogCalls &&
		!opts.UseDSLMultiLineCalls &&
		!opts.UseDSLExpr &&
		!opts.UseDSLFuncSigs &&
		!opts.UseDSLBlankLines &&
		normalizedRuleProfile(opts.RuleProfile) != "parity" {
		if plan, ok := stagePlanForRuleProfile(opts.RuleProfile); ok {
			return plan
		}
	}

	return StagePlan{
		Comments:       stageModeFromBool(opts.UseDSLComments),
		LogCalls:       stageModeFromBool(opts.UseDSLLogCalls),
		Expressions:    stageModeFromBool(opts.UseDSLExpr),
		MultiLineCalls: stageModeFromBool(opts.UseDSLMultiLineCalls),
		Signatures:     stageModeFromBool(opts.UseDSLFuncSigs),
		BlankLines:     stageModeFromBool(opts.UseDSLBlankLines),
	}
}

func stageModeFromBool(useDSL bool) StageMode {
	if useDSL {
		return StageModeDSL
	}
	return StageModeLegacy
}
