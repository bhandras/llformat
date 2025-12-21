package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineDSLMultiLineLayoutArgsBreaksNestedLogicalArg(t *testing.T) {
	const in = `package p

func f() {
	_ = veryLongFunctionNameForTestingPurposes(
		firstConditionThatIsVeryLong && secondConditionThatIsVeryLong && thirdConditionThatIsVeryLong,
		42,
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

	// The nested logical chain should break with continuation indentation
	// within the argument list.
	require.Contains(t, outStr, "&&\n\t\t\tsecondConditionThatIsVeryLong")

	// Idempotent.
	out2 := p.Format(out)
	require.Equal(t, outStr, string(out2))

	// Parseable.
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors)
	require.NoError(t, err)
}
