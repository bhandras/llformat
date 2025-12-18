package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNextModeAppliesExpectedDefaults(t *testing.T) {
	p := NewPipeline(PipelineConfig{
		Mode:        "next",
		ColumnLimit: 60,
		TabStop:     8,
	})

	// "next" is intended to be a convenience alias for the most aggressive
	// DSL-first config. These checks are intentionally on internal config fields
	// rather than exact output formatting so the mode can evolve without
	// modifying golden fixtures.
	require.True(t, p.cfg.UseDSLComments)
	require.True(t, p.cfg.UseDSLLogCalls)
	require.True(t, p.cfg.UseDSLMultiLineCalls)
	require.True(t, p.cfg.UseDSLExpr)
	require.True(t, p.cfg.UseDSLFuncSigs)
	require.True(t, p.cfg.UseDSLFuncSigsNative)
	require.True(t, p.cfg.UseDSLBlankLines)
	require.True(t, p.cfg.UseDSLBlankLinesNative)

	require.Equal(t, "modern", p.cfg.DSLCallPolicy)
	require.Equal(t, "layout-all", p.cfg.DSLMultiLineStyle)

	// In next-mode, multiline layout owns call-arg formatting; the DSL
	// expression stage should not also break inside call args.
	require.False(t, p.cfg.AllowDSLCallArgs)
	require.True(t, p.cfg.AutoDSLCallArgs)
}

func TestPipelineNextModeIdempotentAndASTEquivalent(t *testing.T) {
	const in = `package p

func g(x int) int { return x }

func outerVeryLongFunctionNameForNextModeTesting(a int, b bool, c int) {}

func f(a, b, c, d, e, f2, g2 bool, x int) {
	outerVeryLongFunctionNameForNextModeTesting(
		g(g(g(x))),
		(a && b && c && d) || (e && f2 && g2),
		x,
	)
}
`

	p := NewPipeline(PipelineConfig{
		Mode:        "next",
		ColumnLimit: 50,
		TabStop:     8,
	})

	out1 := p.Format([]byte(in))
	out2 := p.Format(out1)
	require.Equal(t, string(out1), string(out2))
	requireASTEquivalent(t, []byte(in), out1)
}
