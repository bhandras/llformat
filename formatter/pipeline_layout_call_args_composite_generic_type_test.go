package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineDSLMultiLineLayoutArgsBreaksCompositeLiteralWithGenericType(t *testing.T) {
	const in = `package p

func f() {
	_ = veryLongFunctionNameForTestingPurposes(
		GenericType[VeryLongTypeNameOne, VeryLongTypeNameTwo, VeryLongTypeNameThree]{FieldA: firstConditionThatIsVeryLong && secondConditionThatIsVeryLong && thirdConditionThatIsVeryLong},
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

	// The generic type arguments should be allowed to break, but `{` must remain
	// attached to the type to avoid Go semicolon insertion hazards.
	require.Contains(t, outStr, "GenericType[")
	require.Contains(t, outStr, "VeryLongTypeNameOne,\n\t\t\tVeryLongTypeNameTwo,")
	require.Contains(t, outStr, "VeryLongTypeNameThree]{")

	// The field value should also be able to break.
	require.Contains(t, outStr, "&&\n\t\t\t\tsecondConditionThatIsVeryLong")

	// Idempotent.
	out2 := p.Format(out)
	require.Equal(t, outStr, string(out2))

	// Parseable.
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors)
	require.NoError(t, err)
}

