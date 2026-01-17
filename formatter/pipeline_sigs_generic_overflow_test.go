package formatter

import (
	"strings"
	"testing"

	"github.com/bhandras/llformat/width"
	"github.com/stretchr/testify/require"
)

func TestPipelineNext_Signatures_BreaksGenericTypeArgsToAvoidOverflow(
	t *testing.T) {

	const in = `package p

	type ReceiverXXXXXXXXXXXXXXXXXXXXXXXXXXXX[T1, T2, T3 any] struct{}
	type ReturnXXXXXXXXXXXXXXXXXXXXXXXXXXXX[T1, T2, T3 any] struct{}

func (r *ReceiverXXXXXXXXXXXXXXXXXXXXXXXXXXXX[T1, T2, T3]) VeryLongMethodNameForSig() ReturnXXXXXXXXXXXXXXXXXXXXXXXXXXXX[T1, T2, T3] {
	return ReturnXXXXXXXXXXXXXXXXXXXXXXXXXXXX[T1, T2, T3]{}
}

func VeryLongFunctionNameForSig() ReturnXXXXXXXXXXXXXXXXXXXXXXXXXXXX[T1, T2, T3] {
	return ReturnXXXXXXXXXXXXXXXXXXXXXXXXXXXX[T1, T2, T3]{}
}
`

	p := NewPipeline(
		PipelineConfig{
			ColumnLimit:            80,
			TabStop:                8,
			UseDSLFuncSigs:         true,
			UseDSLFuncSigsNative:   true,
			DSLSigsStyle:           "legacy",
			UseDSLLogCalls:         false,
			UseDSLMultiLineCalls:   false,
			UseDSLExpr:             false,
			UseDSLComments:         false,
			UseDSLBlankLines:       false,
			UseDSLBlankLinesNative: false,
		},
	)

	out := string(p.Format([]byte(in)))

	require.Contains(
		t, out, "ReceiverXXXXXXXXXXXXXXXXXXXXXXXXXXXX["+
			"\n	T1,\n	T2,\n	T3,\n]",
	)
	require.Contains(
		t, out,
		"ReturnXXXXXXXXXXXXXXXXXXXXXXXXXXXX[\n	T1,\n	T2,\n	T3,\n]",
	)

	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		require.LessOrEqual(
			t, width.VisualLenWithTab(line, 8), 80, "line "+
				"exceeds configured column limit: %q", line,
		)
	}
}
