package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineDSLMultiLineLayoutArgsBreaksIndexExprWithLogicalIndex(t *testing.T) {
	const in = `package p

func f() {
	_ = veryLongFunctionNameForTestingPurposes(
		m[firstConditionThatIsVeryLong && secondConditionThatIsVeryLong && thirdConditionThatIsVeryLong],
		42,
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

	// The logical chain should break and the index expression should wrap with
	// bracket-aware indentation.
	require.Contains(t, outStr, "m[firstConditionThatIsVeryLong &&\n\t\t\tsecondConditionThatIsVeryLong")

	// Idempotent.
	out2 := p.Format(out)
	require.Equal(t, outStr, string(out2))

	// Parseable.
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors)
	require.NoError(t, err)
}

func TestPipelineDSLMultiLineLayoutArgsBreaksSliceExprWithLogicalHigh(t *testing.T) {
	const in = `package p

func f() {
	_ = veryLongFunctionNameForTestingPurposes(
		m[:firstConditionThatIsVeryLong && secondConditionThatIsVeryLong && thirdConditionThatIsVeryLong],
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

	// The slice expression should wrap and allow the high expression to break
	// within the brackets.
	require.Contains(t, outStr, "m[:firstConditionThatIsVeryLong &&\n\t\t\tsecondConditionThatIsVeryLong")

	// Idempotent.
	out2 := p.Format(out)
	require.Equal(t, outStr, string(out2))

	// Parseable.
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors)
	require.NoError(t, err)
}
