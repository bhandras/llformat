package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_MultiLineCalls_MakeAssignment_PacksShortArgsTogether(t *testing.T) {
	// Regression test for the `make([]T, 0, len(...))` pattern:
	// when multiline is required, keep the common `0, len(...)` packed on the
	// same continuation line when they fit.
	const in = `package p

func f(xs []int) {
	packedBackupsFromProtoSource := make([][]byte, 0, len(xs))
	_ = packedBackupsFromProtoSource
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          52,
		TabStop:              8,
		UseDSLMultiLineCalls: true,
		// Keep other DSL stages off to make this test focused.
		UseDSLLogCalls:    false,
		UseDSLExpr:        false,
		UseDSLComments:    false,
		UseDSLFuncSigs:    false,
		UseDSLBlankLines:  false,
		DSLMultiLineStyle: "",
	})

	out := string(p.Format([]byte(in)))

	require.Contains(t, out, "packedBackupsFromProtoSource := make(\n", "expected multiline make() call")
	require.Contains(t, out, "\t\t[][]byte, 0, len(xs),\n",
		"expected packed args to stay on the same continuation line when they fit")
	require.NotContains(t, out, "\t\t[][]byte,\n", "must not force the type argument onto its own line")
}
