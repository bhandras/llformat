package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStageBuilders_ChooseLegacyVsDSLTypes(t *testing.T) {
	t.Parallel()

	cfg := NewBaseConfig(80, 8)

	legacyBundle := dslBundleForOptions(StageOptions{})
	legacyPlan := StagePlan{
		Comments:       StageModeLegacy,
		LogCalls:       StageModeLegacy,
		Expressions:    StageModeLegacy,
		MultiLineCalls: StageModeLegacy,
		Signatures:     StageModeLegacy,
		BlankLines:     StageModeLegacy,
	}
	dslPlan := StagePlan{
		Comments:       StageModeDSL,
		LogCalls:       StageModeDSL,
		Expressions:    StageModeDSL,
		MultiLineCalls: StageModeDSL,
		Signatures:     StageModeDSL,
		BlankLines:     StageModeDSL,
	}

	f := buildCommentStageFormatter("comments", cfg, StageOptions{UseDSLComments: false}, legacyPlan, legacyBundle)
	_, ok := f.(*CommentFormatter)
	require.True(t, ok)

	f = buildCommentStageFormatter("comments", cfg, StageOptions{UseDSLComments: true}, dslPlan, legacyBundle)
	_, ok = f.(*DSLExprFormatter)
	require.True(t, ok)

	f = buildCompactCallStageFormatter("compact-calls", cfg, StageOptions{UseDSLLogCalls: false}, legacyPlan, legacyBundle)
	_, ok = f.(*CompactCallFormatter)
	require.True(t, ok)

	f = buildCompactCallStageFormatter("compact-calls", cfg, StageOptions{UseDSLLogCalls: true}, dslPlan, legacyBundle)
	_, ok = f.(*DSLExprFormatter)
	require.True(t, ok)

	f = buildExpressionStageFormatter("expressions", cfg, StageOptions{UseDSLExpr: false}, legacyPlan, legacyBundle)
	_, ok = f.(*LongExprFormatter)
	require.True(t, ok)

	f = buildExpressionStageFormatter("expressions", cfg, StageOptions{UseDSLExpr: true}, dslPlan, legacyBundle)
	_, ok = f.(*DSLExprFormatter)
	require.True(t, ok)

	f = buildMultiLineCallStageFormatter("multiline-calls", cfg, StageOptions{UseDSLMultiLineCalls: false}, legacyPlan, legacyBundle)
	_, ok = f.(*MultiLineCallFormatter)
	require.True(t, ok)

	f = buildMultiLineCallStageFormatter("multiline-calls", cfg, StageOptions{UseDSLMultiLineCalls: true}, dslPlan, legacyBundle)
	_, ok = f.(*DSLExprFormatter)
	require.True(t, ok)

	f = buildSignatureStageFormatter("signatures", cfg, StageOptions{UseDSLFuncSigs: false}, legacyPlan, legacyBundle)
	_, ok = f.(*FuncSigFormatter)
	require.True(t, ok)

	f = buildSignatureStageFormatter("signatures", cfg, StageOptions{UseDSLFuncSigs: true}, dslPlan, legacyBundle)
	_, ok = f.(*DSLExprFormatter)
	require.True(t, ok)

	f = buildBlankLineStageFormatter("blank-lines", cfg, StageOptions{UseDSLBlankLines: false}, legacyPlan, legacyBundle)
	_, ok = f.(*BlankLineFormatter)
	require.True(t, ok)

	f = buildBlankLineStageFormatter("blank-lines", cfg, StageOptions{UseDSLBlankLines: true}, dslPlan, legacyBundle)
	_, ok = f.(*DSLExprFormatter)
	require.True(t, ok)
}

func TestStagePlanFromOptions_UsesExplicitStagePlanWhenProvided(t *testing.T) {
	t.Parallel()

	opts := StageOptions{
		// These would normally enable DSL via the legacy toggles.
		UseDSLComments:       true,
		UseDSLLogCalls:       true,
		UseDSLExpr:           true,
		UseDSLMultiLineCalls: true,
		UseDSLFuncSigs:       true,
		UseDSLBlankLines:     true,
		StagePlan: &StagePlan{
			Comments:       StageModeLegacy,
			LogCalls:       StageModeLegacy,
			Expressions:    StageModeLegacy,
			MultiLineCalls: StageModeLegacy,
			Signatures:     StageModeLegacy,
			BlankLines:     StageModeLegacy,
		},
	}

	plan := stagePlanFromOptions(opts)
	require.Equal(t, StageModeLegacy, plan.Comments)
	require.Equal(t, StageModeLegacy, plan.LogCalls)
	require.Equal(t, StageModeLegacy, plan.Expressions)
	require.Equal(t, StageModeLegacy, plan.MultiLineCalls)
	require.Equal(t, StageModeLegacy, plan.Signatures)
	require.Equal(t, StageModeLegacy, plan.BlankLines)
}
