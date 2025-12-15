package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineModernLayoutBreaksCaseClauses(t *testing.T) {
	const in = `package p

func f(val int) int {
	switch val {
	case 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11:
		return 1
	default:
		return 0
	}
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:   30,
		TabStop:       8,
		DSLCallPolicy: "modern",
	})
	out := p.Format([]byte(in))

	require.Contains(t, string(out), "case 1,")
	require.Contains(t, string(out), ",\n\t\t")

	out2 := p.Format(out)
	require.Equal(t, string(out), string(out2))

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors)
	require.NoError(t, err)

	requireASTEquivalent(t, []byte(in), out)
}
