package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineDSLMultiLineLayoutArgsBreaksSelectorTermInsideArithmeticChain(
	t *testing.T) {

	const in = `package p

func f() {
	_ = veryLongFunctionNameForTestingPurposes(
		firstVeryLongIdentifierName + someReallyLongPackageName.someReallyLongTypeName.someReallyLongField + thirdVeryLongIdentifierName,
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

	// The arithmetic chain should break, and the selector chain term should
	// be allowed to break within the argument expression.
	require.Contains(t, outStr, "+\n\t\t\tsomeReallyLongPackageName.")
	require.Contains(t, outStr, "someReallyLongTypeName.")
	require.Contains(t, outStr, "+\n\t\t\tthirdVeryLongIdentifierName")

	// Idempotent.
	out2 := p.Format(out)
	require.Equal(t, outStr, string(out2))

	// Parseable.
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors)
	require.NoError(t, err)
}
