package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStagePlanFromOptions_ProfileModernDefaultsToDSL(t *testing.T) {
	t.Parallel()

	plan := stagePlanFromOptions(StageOptions{
		Selection: StageSelectionOptions{RuleProfile: "modern"},
	})
	require.Equal(t, StageModeDSL, plan.Comments)
	require.Equal(t, StageModeDSL, plan.LogCalls)
	require.Equal(t, StageModeDSL, plan.Expressions)
	require.Equal(t, StageModeDSL, plan.MultiLineCalls)
	require.Equal(t, StageModeDSL, plan.Signatures)
	require.Equal(t, StageModeDSL, plan.BlankLines)
}

func TestStagePlanFromOptions_ProfileParityDefaultsToLegacy(t *testing.T) {
	t.Parallel()

	plan := stagePlanFromOptions(StageOptions{
		Selection: StageSelectionOptions{RuleProfile: "parity"},
	})
	require.Equal(t, StageModeLegacy, plan.Comments)
	require.Equal(t, StageModeLegacy, plan.LogCalls)
	require.Equal(t, StageModeLegacy, plan.Expressions)
	require.Equal(t, StageModeLegacy, plan.MultiLineCalls)
	require.Equal(t, StageModeLegacy, plan.Signatures)
	require.Equal(t, StageModeLegacy, plan.BlankLines)
}
