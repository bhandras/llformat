package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineDSLMultiLineLayoutArgsBreaksNestedCallWithNestedLogical(t *testing.T) {
	const in = `package p

func f() {
	_ = outerFunctionNameThatIsVeryLong(
		innerFunctionNameThatIsVeryLong(firstConditionThatIsVeryLong && secondConditionThatIsVeryLong && thirdConditionThatIsVeryLong, 7),
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

	// The inner call should break its logical-chain arg with an extra
	// continuation indent relative to its own argument indentation.
	//
	// Note: the exact break positioning is heuristic; assert only the
	// continuation indentation is present.
	require.Contains(t, outStr, "\n\t\t\t\tsecondConditionThatIsVeryLong")

	// Idempotent.
	out2 := p.Format(out)
	require.Equal(t, outStr, string(out2))

	// Parseable.
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors)
	require.NoError(t, err)
}
