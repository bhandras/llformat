package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipeline_CallArgsGroupingPairs_GroupsPairsPerLine(t *testing.T) {
	t.Parallel()

	const in = `package p

func f() {
	_ = someFunc("a", 1, "b", 2, "c", 3, "d", 4)
}

func someFunc(args ...any) any { return nil }
`

	p := NewPipeline(
		PipelineConfig{
			ColumnLimit:          40,
			TabStop:              8,
			UseDSLMultiLineCalls: true,
			DSLMultiLineStyle:    "layout-args-groups-pairs",
			UseDSLLogCalls:       false,
			UseDSLExpr:           false,
			UseDSLComments:       false,
			UseDSLFuncSigs:       false,
			UseDSLBlankLines:     false,
		},
	)

	out1 := p.Format([]byte(in))
	out2 := p.Format(out1)

	require.Contains(t, string(out1), "\"a\", 1,")
	require.Contains(t, string(out1), "\"b\", 2,")
	require.Contains(t, string(out1), "\"c\", 3,")
	require.Contains(t, string(out1), "\"d\", 4,")

	require.Equal(t, string(out1), string(out2), "not idempotent")
	requireASTEquivalent(t, []byte(in), out1)
}

func TestPipeline_CallArgsGroupingPairs_OddArgCountLeavesLastSolo(
	t *testing.T) {

	t.Parallel()

	const in = `package p

func f() {
	_ = someFunc("a", 1, "b", 2, "tail")
}

func someFunc(args ...any) any { return nil }
`

	p := NewPipeline(
		PipelineConfig{
			ColumnLimit:          40,
			TabStop:              8,
			UseDSLMultiLineCalls: true,
			DSLMultiLineStyle:    "layout-args-groups-pairs",
			UseDSLLogCalls:       false,
			UseDSLExpr:           false,
			UseDSLComments:       false,
			UseDSLFuncSigs:       false,
			UseDSLBlankLines:     false,
		},
	)

	out := string(p.Format([]byte(in)))
	require.Contains(t, out, "\"a\", 1,")
	require.Contains(t, out, "\"b\", 2,")
	require.Contains(t, out, "\"tail\",")
}
