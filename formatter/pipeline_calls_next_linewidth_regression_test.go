package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_MultiLineCalls_ReflowsWhenLineTooLongEvenIfCollapsedFits(t *testing.T) {
	// This snippet is intentionally *not* gofmt'd: it has extra spacing after the
	// commas. The generic call formatting should still trigger when the line
	// exceeds the column limit.
	//
	// Important: the "collapsed" width of this call (with whitespace normalized)
	// is <= 40 columns, but the actual source line is > 40 columns.
	const in = `package p

func f() {
  makeConcatString("Hello",  "Yo",  "up")
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          40,
		TabStop:              8,
		UseDSLMultiLineCalls: true,
		// Use the layout-based call-args formatter to exercise its internal "long
		// call" gating logic.
		DSLMultiLineStyle: "layout-args",
		// Keep other stages off so the test is focused.
		UseDSLLogCalls:       false,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLFuncSigs:       false,
		UseDSLBlankLines:     false,
		UseOwnershipRegistry: false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(t, out, "makeConcatString(\n",
		"expected the long source line to be rewritten as multiline even if its collapsed width fits")
	require.NotContains(t, out, `makeConcatString("Hello",  "Yo",  "up")`,
		"expected the original long line to be rewritten")
}
