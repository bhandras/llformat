package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineDSLMultiLineLayoutArgsBreaksIndexListExprArg(t *testing.T) {
	const in = `package p

func f() {
	_ = veryLongFunctionNameForTestingPurposes(
		genericThing[VeryLongTypeNameOne, VeryLongTypeNameTwo, VeryLongTypeNameThree],
	)
}
`

	p := NewPipeline(
		PipelineConfig{
			ColumnLimit:          60,
			TabStop:              8,
			UseDSLMultiLineCalls: true,
			DSLMultiLineStyle:    "layout-args",
		},
	)

	out := p.Format([]byte(in))
	outStr := string(out)

	// The type-arg list should break at commas, and should never place `]`
	// on its own line (Go semicolon insertion hazard).
	require.Contains(
		t, outStr, "genericThing["+
			"\n"+
			"			VeryLongTypeNameOne,"+
			"\n			VeryLongTypeNameTwo,",
	)
	require.Contains(t, outStr, "VeryLongTypeNameThree],")

	// Idempotent.
	out2 := p.Format(out)
	require.Equal(t, outStr, string(out2))

	// Parseable.
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors)
	require.NoError(t, err)
}
