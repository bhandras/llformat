package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineDSLMultiLineLayoutArgsBreaksUnaryParenLogicalArg(
	t *testing.T) {

	const in = `package p

func f() {
	_ = veryLongFunctionNameForTestingPurposes(
		!(firstConditionThatIsVeryLong && secondConditionThatIsVeryLong && thirdConditionThatIsVeryLong),
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

	// The unary expression should allow the nested paren + logical chain to
	// be formatted, rather than treating it atomically.
	require.Contains(t, outStr, "&&\n\t\t\tsecondConditionThatIsVeryLong")
	require.Contains(t, outStr, "thirdConditionThatIsVeryLong)")

	// Idempotent.
	out2 := p.Format(out)
	require.Equal(t, outStr, string(out2))

	// Parseable.
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors)
	require.NoError(t, err)
}
