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
	if opts.Selection.StagePlan != nil {
		return *opts.Selection.StagePlan
	}

	// Default behavior for DefaultStagesWithOptions (and most internal callers):
	// if no explicit StagePlan is provided, treat RuleProfile as a stage selector
	// only for non-parity profiles.
	profile := normalizedRuleProfile(opts.Selection.RuleProfile)
	if profile != "parity" {
		if plan, ok := stagePlanForRuleProfile(profile); ok {
			return plan
		}
	}

	return StagePlan{
		Comments:       StageModeLegacy,
		LogCalls:       StageModeLegacy,
		Expressions:    StageModeLegacy,
		MultiLineCalls: StageModeLegacy,
		Signatures:     StageModeLegacy,
		BlankLines:     StageModeLegacy,
	}
}

func stageModeFromBool(useDSL bool) StageMode {
	if useDSL {
		return StageModeDSL
	}
	return StageModeLegacy
}
