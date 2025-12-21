package formatter

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_MultiLineCalls_DoesNotStopAfterTwentyRewrites(t *testing.T) {
	// The DSL engine applies at most one transforming edit per iteration.
	// Historically the multiline-calls stage used a fixed 20-iteration cap, which
	// can leave later long calls untouched in large files.
	var b strings.Builder
	b.WriteString("package p\n\nfunc f() {\n")
	for i := 0; i < 35; i++ {
		fmt.Fprintf(&b, "\t_ = veryLongCalleeNameForIterationTesting(%d, a, b, c, d, e, f)\n", i)
	}
	b.WriteString("}\n")
	in := b.String()

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          30,
		TabStop:              8,
		UseDSLMultiLineCalls: true,
		// Leave style empty to use next defaults.
		DSLMultiLineStyle: "",
		// Keep other stages off so only multiline-calls is responsible.
		UseDSLLogCalls:       false,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLFuncSigs:       false,
		UseDSLBlankLines:     false,
		UseOwnershipRegistry: false,
	})

	out := string(p.Format([]byte(in)))

	// The last call should also be rewritten (not only the first ~20).
	require.Contains(t, out, "\t_ = veryLongCalleeNameForIterationTesting(\n",
		"expected long calls to be rewritten as multiline")
	require.Contains(t, out, "\t\t34,",
		"expected the final long call (index 34) to be rewritten too")
}
