package formatter

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineDSLExpr_ParenStability_WhenBreakingInsideCallArgs(t *testing.T) {
	t.Parallel()

	const in = `package p

func f(a, b, c, d, e, f2, g bool) {
	// When explicitly allowed, the DSL expr stage may break inside call args.
	// Ensure we do not place a close-paren token on its own line (semicolon
	// insertion hazards) for parenthesized expressions.
	_ = foo((a && b && c && d && e && f2 && g))
}

func foo(x bool) bool { return x }
`

	p := NewPipeline(PipelineConfig{
		UseDSLExpr:       true,
		AllowDSLCallArgs: true,
		ColumnLimit:      40,
		TabStop:          8,
	})

	out := string(p.Format([]byte(in)))
	require.Contains(t, out, "&&\n")
	require.NotContains(t, out, "\n\t\t)")

	// Idempotent.
	out2 := string(p.Format([]byte(out)))
	require.Equal(t, out, out2)

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", []byte(out), parser.AllErrors)
	require.NoError(t, err)

	// Ensure the paren-wrapped call arg remains parseable and still contains
	// the original operand sequence.
	require.True(t, strings.Contains(out, "foo((") || strings.Contains(out, "foo(\n"))
}
