package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineModernLayoutBreaksLogicalChainsInOnePass(t *testing.T) {
	const in = `package p

func f(alpha, beta, gamma, delta bool) bool {
	if alpha && beta && gamma && delta {
		return true
	}
	return false
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:   30,
		TabStop:       8,
		DSLCallPolicy: "modern",
	})
	out := p.Format([]byte(in))

	// The layout-based chain breaker breaks at each operator (Go style).
	require.Contains(t, string(out), "&&\n")

	// Idempotent.
	out2 := p.Format(out)
	require.Equal(t, string(out), string(out2))

	// Parseable.
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors)
	require.NoError(t, err)

	// Semantic equivalence guard (formatting must not change AST structure).
	requireASTEquivalent(t, []byte(in), out)
}
