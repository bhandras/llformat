package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineModernLayoutBreaksSelectorChains(t *testing.T) {
	const in = `package p

func f() {
	_ = veryLongPackageNameWithManySegments.veryLongSubpackageNameWithManySegments.EvenLongerSymbolName.WithFields
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:   40,
		TabStop:       8,
		DSLCallPolicy: "modern",
	})
	out := p.Format([]byte(in))

	// Selector chain breaks after dots with a continuation indent.
	require.Contains(t, string(out), ".\n\t\t")
	require.NotContains(t, string(out), ". ") // should never introduce spaces after dots

	out2 := p.Format(out)
	require.Equal(t, string(out), string(out2))

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors)
	require.NoError(t, err)

	requireASTEquivalent(t, []byte(in), out)
}
