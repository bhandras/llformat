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
