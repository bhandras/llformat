package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStageBuilders_ChooseLegacyVsDSLTypes(t *testing.T) {
	t.Parallel()

	cfg := NewBaseConfig(80, 8)

	legacyBundle := dslBundleForOptions(StageOptions{})

	f := buildCommentStageFormatter(cfg, StageOptions{UseDSLComments: false}, legacyBundle)
	_, ok := f.(*CommentFormatter)
	require.True(t, ok)

	f = buildCommentStageFormatter(cfg, StageOptions{UseDSLComments: true}, legacyBundle)
	_, ok = f.(*DSLExprFormatter)
	require.True(t, ok)

	f = buildCompactCallStageFormatter(cfg, StageOptions{UseDSLLogCalls: false}, legacyBundle)
	_, ok = f.(*CompactCallFormatter)
	require.True(t, ok)

	f = buildCompactCallStageFormatter(cfg, StageOptions{UseDSLLogCalls: true}, legacyBundle)
	_, ok = f.(*DSLExprFormatter)
	require.True(t, ok)

	f = buildExpressionStageFormatter(cfg, StageOptions{UseDSLExpr: false}, legacyBundle)
	_, ok = f.(*LongExprFormatter)
	require.True(t, ok)

	f = buildExpressionStageFormatter(cfg, StageOptions{UseDSLExpr: true}, legacyBundle)
	_, ok = f.(*DSLExprFormatter)
	require.True(t, ok)

	f = buildMultiLineCallStageFormatter(cfg, StageOptions{UseDSLMultiLineCalls: false}, legacyBundle)
	_, ok = f.(*MultiLineCallFormatter)
	require.True(t, ok)

	f = buildMultiLineCallStageFormatter(cfg, StageOptions{UseDSLMultiLineCalls: true}, legacyBundle)
	_, ok = f.(*DSLExprFormatter)
	require.True(t, ok)

	f = buildSignatureStageFormatter(cfg, StageOptions{UseDSLFuncSigs: false}, legacyBundle)
	_, ok = f.(*FuncSigFormatter)
	require.True(t, ok)

	f = buildSignatureStageFormatter(cfg, StageOptions{UseDSLFuncSigs: true}, legacyBundle)
	_, ok = f.(*DSLExprFormatter)
	require.True(t, ok)

	f = buildBlankLineStageFormatter(cfg, StageOptions{UseDSLBlankLines: false}, legacyBundle)
	_, ok = f.(*BlankLineFormatter)
	require.True(t, ok)

	f = buildBlankLineStageFormatter(cfg, StageOptions{UseDSLBlankLines: true}, legacyBundle)
	_, ok = f.(*DSLExprFormatter)
	require.True(t, ok)
}

