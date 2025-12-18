package formatter

import "github.com/lightninglabs/llformat/dsl"

// DSLStageSpec describes how a single stage should run under the DSL engine.
// It intentionally mirrors DSLExprConfig fields that are policy-driven rather
// than derived from the file formatting config (column limit/tab stop).
type DSLStageSpec struct {
	Rules                       []dsl.Rule
	NodeOrder                   dsl.NodeOrder
	MaxIterations               int
	DisableLegacyBlankLinesShim bool
}

// DSLBundle is the cohesive per-stage DSL rule/config selection for a given
// RuleProfile and StageOptions.
//
// The intent is to make it easy to reason about "what rules run where" and to
// make profile selection an explicit, testable piece of logic.
type DSLBundle struct {
	Comments       DSLStageSpec
	LogCalls       DSLStageSpec
	Expressions    DSLStageSpec
	MultiLineCalls DSLStageSpec
	Signatures     DSLStageSpec
	BlankLines     DSLStageSpec
}

func dslBundleForOptions(opts StageOptions) DSLBundle {
	multiLineRules, multiLineNodeOrder := dslRulesForMultiLineCalls(opts)

	blankLineRules := dslRulesForBlankLines(opts)
	disableBlankLinesShim := opts.UseDSLBlankLinesNative

	return DSLBundle{
		Comments: DSLStageSpec{
			Rules:         dslRulesForComments(opts.CommentMoveInline),
			MaxIterations: 1,
		},
		LogCalls: DSLStageSpec{
			Rules: dslRulesForLogCalls(),
		},
		Expressions: DSLStageSpec{
			Rules: dslRulesForExpr(opts),
		},
		MultiLineCalls: DSLStageSpec{
			Rules:         multiLineRules,
			NodeOrder:     multiLineNodeOrder,
			MaxIterations: 20,
		},
		Signatures: DSLStageSpec{
			Rules:         dslRulesForSignatures(opts),
			MaxIterations: dslMaxItersForSignatures(opts),
		},
		BlankLines: DSLStageSpec{
			Rules:                       blankLineRules,
			MaxIterations:               dslMaxItersForBlankLines(opts),
			DisableLegacyBlankLinesShim: disableBlankLinesShim,
		},
	}
}

func dslMaxItersForSignatures(opts StageOptions) int {
	if !opts.UseDSLFuncSigsNative {
		return 1
	}
	return 100
}

func dslMaxItersForBlankLines(opts StageOptions) int {
	if !opts.UseDSLBlankLinesNative {
		return 1
	}
	return 200
}

