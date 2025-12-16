package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineDSLMultiLineLayoutArgsFormatsGenericCalleeInNestedCall(t *testing.T) {
	const in = `package p

func f() {
	_ = veryLongFunctionNameForTestingPurposes(
		genericCall[VeryLongTypeNameOne, VeryLongTypeNameTwo, VeryLongTypeNameThree](firstConditionThatIsVeryLong && secondConditionThatIsVeryLong && thirdConditionThatIsVeryLong),
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

	// The generic callee should be allowed to break its type arguments, and the
	// call should still keep `](` tightly coupled (no newline between them).
	require.Contains(t, outStr, "genericCall[")
	require.Contains(t, outStr, "VeryLongTypeNameOne,\n\t\t\tVeryLongTypeNameTwo,")
	require.Contains(t, outStr, "VeryLongTypeNameThree](")

	// The call arg should also break.
	require.Contains(t, outStr, "&&\n\t\t\t\tsecondConditionThatIsVeryLong")

	// Idempotent.
	out2 := p.Format(out)
	require.Equal(t, outStr, string(out2))

	// Parseable.
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors)
	require.NoError(t, err)
}
