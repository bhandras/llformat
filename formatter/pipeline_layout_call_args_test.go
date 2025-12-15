package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineDSLMultiLineLayoutArgsBreaksLongCalls(t *testing.T) {
	const in = `package p

func f() {
	a := 1
	b := 2
	c := 3
	d := 4
	_ = veryLongFunctionNameForTestingPurposes(a, b, c, d)
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          30,
		TabStop:              8,
		UseDSLMultiLineCalls: true,
		DSLMultiLineStyle:    "layout-args",
	})

	out := p.Format([]byte(in))
	outStr := string(out)

	require.Contains(t, outStr, "veryLongFunctionNameForTestingPurposes(\n\t\ta,")
	// The layout formatter may keep args packed on a single continuation line
	// if they fit, but it must still emit a trailing comma for a multiline call
	// and place the closing paren on its own line.
	require.Contains(t, outStr, "d,")
	require.Contains(t, outStr, "\n\t)")

	// Idempotent.
	out2 := p.Format(out)
	require.Equal(t, outStr, string(out2))

	// Parseable.
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors)
	require.NoError(t, err)
}
