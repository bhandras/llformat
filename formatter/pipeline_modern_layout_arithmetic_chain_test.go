package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineModernLayoutBreaksArithmeticChains(t *testing.T) {
	const in = `package p

func f(alpha, beta, gamma, delta, epsilon int) int {
	return alpha + beta + gamma + delta + epsilon
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:   30,
		TabStop:       8,
		DSLCallPolicy: "modern",
	})
	out := p.Format([]byte(in))

	require.Contains(t, string(out), "+\n")

	out2 := p.Format(out)
	require.Equal(t, string(out), string(out2))

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors)
	require.NoError(t, err)

	requireASTEquivalent(t, []byte(in), out)
}
