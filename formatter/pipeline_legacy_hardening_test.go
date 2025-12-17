package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewPipeline_LegacyHardeningEnablesExpectedKnobs(t *testing.T) {
	p := NewPipeline(PipelineConfig{
		ColumnLimit:      80,
		TabStop:          8,
		LegacyHardening:  true,
		UseDSLExpr:       false,
		UseDSLLogCalls:   false,
		UseDSLMultiLineCalls: false,
	})

	var sawCompact, sawExpr, sawMulti bool

	for _, s := range p.Stages() {
		switch s.Name {
		case "compact-calls":
			cc, ok := s.Formatter.(*CompactCallFormatter)
			require.True(t, ok, "compact-calls must use CompactCallFormatter under legacy pipeline")
			require.True(t, cc.cfg.UseASTSelection)
			require.True(t, cc.cfg.ParseSafe)
			sawCompact = true
		case "expressions":
			le, ok := s.Formatter.(*LongExprFormatter)
			require.True(t, ok, "expressions must use LongExprFormatter under legacy pipeline")
			require.True(t, le.cfg.UseASTSelection)
			require.True(t, le.cfg.ParseSafe)
			sawExpr = true
		case "multiline-calls":
			ml, ok := s.Formatter.(*MultiLineCallFormatter)
			require.True(t, ok, "multiline-calls must use MultiLineCallFormatter under legacy pipeline")
			require.True(t, ml.cfg.UseASTSelection)
			require.True(t, ml.cfg.ParseSafe)
			sawMulti = true
		}
	}

	require.True(t, sawCompact, "expected compact-calls stage")
	require.True(t, sawExpr, "expected expressions stage")
	require.True(t, sawMulti, "expected multiline-calls stage")
}

