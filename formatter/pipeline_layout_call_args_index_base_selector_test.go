package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineDSLMultiLineLayoutArgsBreaksSelectorChainBaseInIndexExpr(
	t *testing.T) {

	const in = `package p

func f() {
	_ = veryLongFunctionNameForTestingPurposes(
		someReallyLongPackageName.someReallyLongTypeName.someReallyLongField[firstConditionThatIsVeryLong && secondConditionThatIsVeryLong && thirdConditionThatIsVeryLong],
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

	// The selector chain base should be allowed to break (after `.`)
	// without ever inserting a newline between the base and `[` (semicolon
	// insertion hazard).
	require.Contains(t, outStr, "someReallyLongPackageName.\n")
	require.Contains(t, outStr, "someReallyLongField[")

	// The index expression should still allow the index to break.
	require.Contains(t, outStr, "&&\n\t\t\tsecondConditionThatIsVeryLong")

	// Idempotent.
	out2 := p.Format(out)
	require.Equal(t, outStr, string(out2))

	// Parseable.
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors)
	require.NoError(t, err)
}
