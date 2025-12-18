package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineStagePlan_LegacyModeUsesLegacyFormatters(t *testing.T) {
	t.Parallel()

	p := NewPipeline(PipelineConfig{Mode: "legacy"})
	stages := p.Stages()
	require.NotEmpty(t, stages)

	for _, s := range stages {
		switch s.Name {
		case "comments":
			_, ok := s.Formatter.(*CommentFormatter)
			require.True(t, ok)
		case "compact-calls":
			_, ok := s.Formatter.(*CompactCallFormatter)
			require.True(t, ok)
		case "expressions":
			_, ok := s.Formatter.(*LongExprFormatter)
			require.True(t, ok)
		case "multiline-calls":
			_, ok := s.Formatter.(*MultiLineCallFormatter)
			require.True(t, ok)
		case "signatures":
			_, ok := s.Formatter.(*FuncSigFormatter)
			require.True(t, ok)
		case "blank-lines":
			_, ok := s.Formatter.(*BlankLineFormatter)
			require.True(t, ok)
		default:
			t.Fatalf("unknown stage %q", s.Name)
		}
	}
}

func TestPipelineStagePlan_DefaultUsesLegacyFormatters(t *testing.T) {
	t.Parallel()

	p := NewPipeline(PipelineConfig{})
	stages := p.Stages()
	require.NotEmpty(t, stages)

	for _, s := range stages {
		_, ok := s.Formatter.(*DSLExprFormatter)
		require.False(t, ok, "stage=%s", s.Name)
	}
}

func TestPipelineStagePlan_DSLParityModeUsesDSLFormatters(t *testing.T) {
	t.Parallel()

	p := NewPipeline(PipelineConfig{Mode: "dsl-parity"})
	stages := p.Stages()
	require.NotEmpty(t, stages)

	for _, s := range stages {
		_, ok := s.Formatter.(*DSLExprFormatter)
		require.True(t, ok, "stage=%s", s.Name)
	}
}

func TestPipelineStagePlan_RuleProfileModernUsesDSLFormatters(t *testing.T) {
	t.Parallel()

	p := NewPipeline(PipelineConfig{RuleProfile: "modern"})
	stages := p.Stages()
	require.NotEmpty(t, stages)

	for _, s := range stages {
		_, ok := s.Formatter.(*DSLExprFormatter)
		require.True(t, ok, "stage=%s", s.Name)
	}
}

func TestPipelineStagePlanOverrideWins(t *testing.T) {
	t.Parallel()

	override := &StagePlan{
		Comments:       StageModeLegacy,
		LogCalls:       StageModeLegacy,
		Expressions:    StageModeLegacy,
		MultiLineCalls: StageModeLegacy,
		Signatures:     StageModeLegacy,
		BlankLines:     StageModeLegacy,
	}

	p := NewPipeline(PipelineConfig{
		Mode:              "dsl-parity",
		StagePlanOverride: override,
	})
	stages := p.Stages()
	require.NotEmpty(t, stages)

	for _, s := range stages {
		_, ok := s.Formatter.(*DSLExprFormatter)
		require.False(t, ok, "stage=%s", s.Name)
	}
}
