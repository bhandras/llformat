package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineRuleProfile_DefaultsToParity(t *testing.T) {
	p := NewPipeline(PipelineConfig{
		ColumnLimit: 80,
		TabStop:     8,
	})
	require.Equal(t, "parity", p.cfg.RuleProfile)
}

func TestPipelineRuleProfile_FollowsMode(t *testing.T) {
	t.Run("dsl-parity", func(t *testing.T) {
		p := NewPipeline(PipelineConfig{Mode: "dsl-parity"})
		require.Equal(t, "parity", p.cfg.RuleProfile)
	})
	t.Run("dsl-modern", func(t *testing.T) {
		p := NewPipeline(PipelineConfig{Mode: "dsl-modern"})
		require.Equal(t, "modern", p.cfg.RuleProfile)
	})
	t.Run("next", func(t *testing.T) {
		p := NewPipeline(PipelineConfig{Mode: "next"})
		require.Equal(t, "next", p.cfg.RuleProfile)
	})
}

func TestPipelineRuleProfile_FollowsPolicyBundle(t *testing.T) {
	p := NewPipeline(PipelineConfig{DSLCallPolicy: "modern"})
	require.Equal(t, "modern", p.cfg.RuleProfile)
}

func TestPipelineRuleProfile_UserOverrideWins(t *testing.T) {
	p := NewPipeline(PipelineConfig{
		Mode:        "dsl-modern",
		RuleProfile: "parity",
	})
	require.Equal(t, "parity", p.cfg.RuleProfile)
}
