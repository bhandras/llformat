package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNextModeAppliesExpectedDefaults(t *testing.T) {
	p := NewPipeline(PipelineConfig{
		ColumnLimit: 60,
		TabStop:     8,
	})

	// llformat is next-only: PipelineConfig defaults should enable the full
	// DSL pipeline without requiring an explicit "mode" selector.
	require.True(t, p.cfg.UseDSLComments)
	require.True(t, p.cfg.UseDSLLogCalls)
	require.True(t, p.cfg.UseDSLMultiLineCalls)
	require.True(t, p.cfg.UseDSLExpr)
	require.True(t, p.cfg.UseDSLFuncSigs)
	require.True(t, p.cfg.UseDSLFuncSigsNative)
	require.True(t, p.cfg.UseDSLBlankLines)
	require.True(t, p.cfg.UseDSLBlankLinesNative)

	require.Equal(t, "packed-chain-layout", p.cfg.DSLMultiLineStyle)

	// In next-mode, multiline layout owns call-arg formatting; the DSL
	// expression stage should not also break inside call args.
	require.False(t, p.cfg.AllowDSLCallArgs)
	require.True(t, p.cfg.AutoDSLCallArgs)
}

func TestPipelineDefaultConfigIdempotentAndASTEquivalent(t *testing.T) {
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
		ColumnLimit: 50,
		TabStop:     8,
	})

	out1 := p.Format([]byte(in))
	out2 := p.Format(out1)
	require.Equal(t, string(out1), string(out2))
	requireASTEquivalent(t, []byte(in), out1)

	// The default pipeline should reflow a long outer call while preserving
	// AST.
	outStr := string(out1)
	require.Contains(
		t, outStr, "outerVeryLongFunctionNameForNextModeTesting(\n",
		"expected the long call to be rewritten as multiline",
	)
}
