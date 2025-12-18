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

	f := buildCommentStageFormatter("comments", cfg, StageOptions{}, legacyPlan, legacyBundle)
	_, ok := f.(*CommentFormatter)
	require.True(t, ok)

	f = buildCommentStageFormatter("comments", cfg, StageOptions{}, dslPlan, legacyBundle)
	_, ok = f.(*DSLExprFormatter)
	require.True(t, ok)

	f = buildCompactCallStageFormatter("compact-calls", cfg, StageOptions{}, legacyPlan, legacyBundle)
	_, ok = f.(*CompactCallFormatter)
	require.True(t, ok)

	f = buildCompactCallStageFormatter("compact-calls", cfg, StageOptions{}, dslPlan, legacyBundle)
	_, ok = f.(*DSLExprFormatter)
	require.True(t, ok)

	f = buildExpressionStageFormatter("expressions", cfg, StageOptions{}, legacyPlan, legacyBundle)
	_, ok = f.(*LongExprFormatter)
	require.True(t, ok)

	f = buildExpressionStageFormatter("expressions", cfg, StageOptions{}, dslPlan, legacyBundle)
	_, ok = f.(*DSLExprFormatter)
	require.True(t, ok)

	f = buildMultiLineCallStageFormatter("multiline-calls", cfg, StageOptions{}, legacyPlan, legacyBundle)
	_, ok = f.(*MultiLineCallFormatter)
	require.True(t, ok)

	f = buildMultiLineCallStageFormatter("multiline-calls", cfg, StageOptions{}, dslPlan, legacyBundle)
	_, ok = f.(*DSLExprFormatter)
	require.True(t, ok)

	f = buildSignatureStageFormatter("signatures", cfg, StageOptions{}, legacyPlan, legacyBundle)
	_, ok = f.(*FuncSigFormatter)
	require.True(t, ok)

	f = buildSignatureStageFormatter("signatures", cfg, StageOptions{}, dslPlan, legacyBundle)
	_, ok = f.(*DSLExprFormatter)
	require.True(t, ok)

	f = buildBlankLineStageFormatter("blank-lines", cfg, StageOptions{}, legacyPlan, legacyBundle)
	_, ok = f.(*BlankLineFormatter)
	require.True(t, ok)

	f = buildBlankLineStageFormatter("blank-lines", cfg, StageOptions{}, dslPlan, legacyBundle)
	_, ok = f.(*DSLExprFormatter)
	require.True(t, ok)
}

func TestStagePlanFromOptions_UsesExplicitStagePlanWhenProvided(t *testing.T) {
	t.Parallel()

	opts := StageOptions{
		Selection: StageSelectionOptions{
			StagePlan: &StagePlan{
				Comments:       StageModeLegacy,
				LogCalls:       StageModeLegacy,
				Expressions:    StageModeLegacy,
				MultiLineCalls: StageModeLegacy,
				Signatures:     StageModeLegacy,
				BlankLines:     StageModeLegacy,
			},
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
