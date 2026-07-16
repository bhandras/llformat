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

	p := NewPipeline(
		PipelineConfig{
			ColumnLimit:          30,
			TabStop:              8,
			UseDSLMultiLineCalls: true,
			DSLMultiLineStyle:    "layout-args",
		},
	)

	out := p.Format([]byte(in))
	outStr := string(out)

	require.Contains(
		t, outStr,
		"veryLongFunctionNameForTestingPurposes(\n		a,",
	)
	// The layout formatter may keep args packed on a single continuation
	// line if they fit, but it must still emit a trailing comma for a
	// multiline call and place the closing paren on its own line.
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

func TestPipelineDSLMultiLineLayoutArgsPreservesVariadicArgument(t *testing.T) {
	t.Parallel()

	const in = `package p

func f(prefix string, values []any) {
	veryLongFunctionNameForTestingPurposes(prefix, values...)
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          30,
		TabStop:              8,
		UseDSLMultiLineCalls: true,
		DSLMultiLineStyle:    "layout-args",
		UseDSLLogCalls:       false,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLFuncSigs:       false,
		UseDSLBlankLines:     false,
	})

	out1 := p.Format([]byte(in))
	out2 := p.Format(out1)
	out := string(out1)

	require.Contains(t, out, "\t\tvalues...,\n")
	require.NotContains(t, out, "\t\tvalues,\n")
	require.Equal(t, string(out1), string(out2), "not idempotent")
	requireASTEquivalent(t, []byte(in), out1)
}
