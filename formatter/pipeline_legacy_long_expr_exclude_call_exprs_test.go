package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipeline_LegacyLongExprExcludeCallExprsPreventsCalleeBreak(t *testing.T) {
	in := []byte(`package p

func f() {
	_ = (firstVeryLongIdentifier + secondVeryLongIdentifier + thirdVeryLongIdentifier)(x)
}
`)

	base := NewBaseConfig(48, 8)

	stagesWithout := DefaultStagesWithOptions(base, StageOptions{
		Legacy: LegacyStageOptions{
			LongExprParseSafe:    true,
			LongExprUseASTSelect: true,
		},
		// Intentionally not setting LongExprExcludeCallExprs.
	})
	outWithout := NewPipelineWithStages(PipelineConfig{ColumnLimit: 48, TabStop: 8}, stagesWithout).Format(in)
	require.NotEqual(t, string(in), string(outWithout), "expected pipeline to break inside callee when call exprs are allowed")
	requireASTEquivalent(t, in, outWithout)

	stagesWith := DefaultStagesWithOptions(base, StageOptions{
		Legacy: LegacyStageOptions{
			LongExprParseSafe:        true,
			LongExprUseASTSelect:     true,
			LongExprExcludeCallExprs: true,
		},
	})
	outWith := NewPipelineWithStages(PipelineConfig{ColumnLimit: 48, TabStop: 8}, stagesWith).Format(in)
	require.Equal(t, string(in), string(outWith), "expected pipeline to leave call expr callee unchanged when excluded")
	requireASTEquivalent(t, in, outWith)
}
