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

// ResolveDSLBundle returns the cohesive per-stage DSL rule/config selection for
// the given stage options.
//
// This is intentionally a single entrypoint so profile- and option-driven rule
// selection stays centralized and easy to test.
func ResolveDSLBundle(opts StageOptions) DSLBundle {
	return dslBundleForOptions(opts)
}

func dslBundleForOptions(opts StageOptions) DSLBundle {
	multiLineRules, multiLineNodeOrder := dslRulesForMultiLineCalls(opts)

	blankLineRules := dslRulesForBlankLines(opts)
	disableBlankLinesShim := opts.UseDSLBlankLinesNative

	return DSLBundle{
		Comments: DSLStageSpec{
			Rules:         dslRulesForComments(opts.CommentMoveInline),
			NodeOrder:     dsl.NodeOrderPreorder,
			MaxIterations: 1,
		},
		LogCalls: DSLStageSpec{
			Rules:         dslRulesForLogCalls(),
			NodeOrder:     dsl.NodeOrderPreorder,
			MaxIterations: 100,
		},
		Expressions: DSLStageSpec{
			Rules:         dslRulesForExpr(opts),
			NodeOrder:     dsl.NodeOrderPreorder,
			MaxIterations: 100,
		},
		MultiLineCalls: DSLStageSpec{
			Rules:         multiLineRules,
			NodeOrder:     multiLineNodeOrder,
			MaxIterations: 20,
		},
		Signatures: DSLStageSpec{
			Rules:         dslRulesForSignatures(opts),
			NodeOrder:     dsl.NodeOrderPreorder,
			MaxIterations: dslMaxItersForSignatures(opts),
		},
		BlankLines: DSLStageSpec{
			Rules:                       blankLineRules,
			NodeOrder:                   dsl.NodeOrderPreorder,
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
