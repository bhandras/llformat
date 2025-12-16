package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineDSLMultiLineLayoutArgsBreaksTypeAssertWithGenericType(t *testing.T) {
	const in = `package p

func f() {
	_ = veryLongFunctionNameForTestingPurposes(
		x.(GenericType[VeryLongTypeNameOne, VeryLongTypeNameTwo, VeryLongTypeNameThree]),
	)
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          60,
		TabStop:              8,
		UseDSLMultiLineCalls: true,
		DSLMultiLineStyle:    "layout-args",
	})

	out := p.Format([]byte(in))
	outStr := string(out)

	// The type assertion should allow nested breaking inside the asserted type,
	// and the generic type-arg list should break at commas. The close paren
	// should not be on its own line.
	require.Contains(t, outStr, "x.(GenericType[")
	require.Contains(t, outStr, "VeryLongTypeNameOne,\n\t\t\tVeryLongTypeNameTwo,")
	require.Contains(t, outStr, "VeryLongTypeNameThree])")
	require.NotContains(t, outStr, "\n\t\t)\n")

	// Idempotent.
	out2 := p.Format(out)
	require.Equal(t, outStr, string(out2))

	// Parseable.
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors)
	require.NoError(t, err)
}
