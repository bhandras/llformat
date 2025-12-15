package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineModernPreservesASTStructure(t *testing.T) {
	const in = `package p

func f(a, b, c, d, e int) (x int, y int, err error) {
	if (a > 0 && b > 0 && c > 0 && d > 0) || e > 0 {
		return a, b, nil
	}
	return 0, 0, nil
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:   60,
		TabStop:       8,
		DSLCallPolicy: "modern",
	})
	out := p.Format([]byte(in))
	require.NotEmpty(t, out)
	requireASTEquivalent(t, []byte(in), out)
}
