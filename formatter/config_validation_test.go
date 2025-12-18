package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidatePipelineConfig_DefaultsAreValid(t *testing.T) {
	t.Parallel()

	require.NoError(t, ValidatePipelineConfig(PipelineConfig{}))
}

func TestValidatePipelineConfig_RejectsUnknownMode(t *testing.T) {
	t.Parallel()

	err := ValidatePipelineConfig(PipelineConfig{Mode: "weird"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Mode")
	require.Contains(t, err.Error(), "allowed")
}

func TestValidatePipelineConfig_RejectsUnknownRuleProfile(t *testing.T) {
	t.Parallel()

	err := ValidatePipelineConfig(PipelineConfig{RuleProfile: "experimental"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "RuleProfile")
}

func TestValidatePipelineConfig_RejectsUnknownMultiLineStyle(t *testing.T) {
	t.Parallel()

	err := ValidatePipelineConfig(PipelineConfig{DSLMultiLineStyle: "layout-everything"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "DSLMultiLineStyle")
}

func TestValidatePipelineConfig_RejectsUnknownExprStyle(t *testing.T) {
	t.Parallel()

	err := ValidatePipelineConfig(PipelineConfig{DSLExprLogicalStyle: "pretty"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "DSLExprLogicalStyle")
}

func TestValidatePipelineConfig_RejectsUnknownStagePlanOverrideMode(t *testing.T) {
	t.Parallel()

	invalid := StageMode("future")
	err := ValidatePipelineConfig(PipelineConfig{
		StagePlanOverride: &StagePlan{
			Comments: invalid,
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "StagePlanOverride.Comments")
}
