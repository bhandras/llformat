package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineModernLayoutBreaksForCondAndReturnBinary(t *testing.T) {
	const in = `package p

func f(items []int, stopRequested bool, retryCount, maxRetries int, a, b, c, d, e bool) bool {
	for i := 0; i < len(items) && !stopRequested && retryCount < maxRetries; i++ {
		if items[i] > 0 {
			retryCount++
		}
	}

	return a && b && c && d && e
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:   35,
		TabStop:       8,
		DSLCallPolicy: "modern",
	})
	out := p.Format([]byte(in))

	// Expect breaks in for condition and return logical chain.
	require.Contains(t, string(out), "&&\n")
	require.Contains(t, string(out), "for i := 0;")
	require.Contains(t, string(out), "return a &&")

	// Idempotent.
	out2 := p.Format(out)
	require.Equal(t, string(out), string(out2))

	// Parseable and AST-equivalent.
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors)
	require.NoError(t, err)
	requireASTEquivalent(t, []byte(in), out)
}
