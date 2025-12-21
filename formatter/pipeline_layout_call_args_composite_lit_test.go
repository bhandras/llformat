package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineDSLMultiLineLayoutArgsBreaksCompositeLiteralArg(t *testing.T) {
	const in = `package p

type cfg struct {
	A int
	B int
	C int
	D int
}

func outerFunctionNameThatIsVeryLong(c cfg) {}

func f() {
	outerFunctionNameThatIsVeryLong(cfg{A: 1, B: 2, C: 3, D: 4})
}
`

	p := NewPipeline(
		PipelineConfig{
			ColumnLimit:          50,
			TabStop:              8,
			UseDSLMultiLineCalls: true,
			DSLMultiLineStyle:    "layout-args",
		},
	)

	out := p.Format([]byte(in))
	outStr := string(out)

	// Ensure the call breaks and the composite literal is formatted as a
	// block.
	require.Contains(
		t, outStr, "outerFunctionNameThatIsVeryLong(\n		cfg{",
	)
	require.Contains(t, outStr, "\n\t\t\tA: 1,")
	require.Contains(t, outStr, "\n\t\t\tB: 2,")
	require.Contains(t, outStr, "\n\t\t\tC: 3,")
	require.Contains(t, outStr, "\n\t\t\tD: 4,")
	require.Contains(t, outStr, "\n\t\t},\n\t)")

	// Idempotent.
	out2 := p.Format(out)
	require.Equal(t, outStr, string(out2))

	// Parseable.
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors)
	require.NoError(t, err)
}
