package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipeline_OwnershipRegistryAllowsLongExprInUnownedCallArgs(t *testing.T) {
	in := []byte(`package p

func f() {
	_ = outerFunctionNameThatIsVeryLong(firstConditionThatIsVeryLong && secondConditionThatIsVeryLong && thirdConditionThatIsVeryLong && fourthConditionThatIsVeryLong)
}
`)

	base := NewBaseConfig(48, 8)
	stages := DefaultStagesWithOptions(base, StageOptions{
		Legacy: LegacyStageOptions{
			// Ensure the legacy long-expr stage is in AST selection + parse-safe mode.
			LongExprParseSafe:    true,
			LongExprUseASTSelect: true,
		},

		Style: StageStyleOptions{
			// Ensure the legacy multiline call stage will not claim ownership of
			// this call (so the expr stage can safely format inside it under the
			// ownership registry model).
			Excludes: []string{"outerFunctionNameThatIsVeryLong"},
		},
	})

	// Without ownership registry, AST selection stays conservative and skips all
	// call-arg lists, so no stage should touch this file.
	outNoRegistry := NewPipelineWithStages(
		PipelineConfig{ColumnLimit: 48, TabStop: 8},
		stages,
	).Format(in)
	require.Equal(t, string(in), string(outNoRegistry))
	requireASTEquivalent(t, in, outNoRegistry)

	// With ownership registry enabled, the expr stage should be allowed to
	// rewrite inside call args that are not owned by any call formatting stage.
	outRegistry := NewPipelineWithStages(
		PipelineConfig{ColumnLimit: 48, TabStop: 8, UseOwnershipRegistry: true},
		stages,
	).Format(in)
	require.NotEqual(t, string(in), string(outRegistry))
	require.Contains(t, string(outRegistry), "\n\t\tsecondConditionThatIsVeryLong")
	requireASTEquivalent(t, in, outRegistry)
}
