package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_FuncLitSignature_CollapsesShortReturnListAfterMultilineParamsWhenItFits(
	t *testing.T) {

	// Regression for router backend pattern:
	//
	// Field: func(nodeFrom, nodeTo T, amount U) (Out, error) {
	//
	// When parameters are multiline but the return list is short, prefer
	// breaking parameters earlier so that the return list can stay inline
	// (consistent with normal function signature formatting).
	const in = `package p

type Vertex struct{}

type Out int

type MilliSatoshi int64

type RouterBackend struct {
	FetchAmountPairCapacity func(nodeFrom, nodeTo Vertex, amount MilliSatoshi) (Out, error)
}

func f() {
	_ = &RouterBackend{
		FetchAmountPairCapacity: func(nodeFrom, nodeTo Vertex,
			amount MilliSatoshi) (
			Out, error) {
			return 0, nil
		},
	}
}
`

	p := NewPipeline(
		PipelineConfig{
			ColumnLimit:          80,
			TabStop:              8,
			UseDSLFuncSigs:       true,
			UseDSLFuncSigsNative: true,
			DSLSigsStyle:         "legacy",
			UseDSLLogCalls:       false,
			UseDSLMultiLineCalls: false,
			UseDSLExpr:           false,
			UseDSLComments:       false,
			UseDSLBlankLines:     false,
		},
	)

	out := string(p.Format([]byte(in)))

	require.NotContains(
		t, out, ") (\n", "return list should not be forced onto a "+
			"fresh line for short return lists",
	)
	require.NotContains(
		t, out, "(Out,\n", "return list should not be split across "+
			"lines when it fits inline",
	)
	require.Contains(
		t, out, ") (Out, error) {",
		"expected short return list to stay inline",
	)
}
