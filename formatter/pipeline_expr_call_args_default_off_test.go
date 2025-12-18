package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineDSLExpr_DefaultDoesNotBreakInsideCallArgs(t *testing.T) {
	t.Parallel()

	const in = `package p

func f(a, b, c, d, e, f2, g bool) {
	// Long logical chain inside call args: the default DSL expr policy should not
	// insert line breaks inside the chain.
	_ = foo(a && b && c && d && e && f2 && g)
}

func foo(x bool) bool { return x }
`

	p := NewPipeline(PipelineConfig{
		// Enable the DSL expression stage, but do not allow call-arg edits.
		UseDSLExpr:       true,
		ColumnLimit:      40,
		TabStop:          8,
		AllowDSLCallArgs: false,
		AutoDSLCallArgs:  false,
	})

	out := string(p.Format([]byte(in)))

	// gofmt may wrap the call args across lines, but it should not introduce
	// breaks inside the boolean chain itself.
	require.NotContains(t, out, "&&\n")
}
