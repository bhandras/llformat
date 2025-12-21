package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineDSLMultiLineLayoutArgsBreaksInnerArgsInsideMethodChainArg(
	t *testing.T) {

	const in = `package p

func f() {
	_ = veryLongFunctionNameForTestingPurposes(
		obj.MethodA(firstConditionThatIsVeryLong && secondConditionThatIsVeryLong && thirdConditionThatIsVeryLong).MethodB(),
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

	// The argument list for MethodA should be allowed to break, even though
	// it is nested inside a method chain expression.
	require.Contains(
		t, outStr, "MethodA("+
			"\n"+
			"				firstConditionThatIs"+
			"VeryLong &&",
	)
	require.Contains(
		t, outStr, "&&"+
			"\n"+
			"					secondCondit"+
			"ionThatIsVeryLong",
	)
	require.Contains(t, outStr, ").\n\t\t\tMethodB()")

	// Idempotent.
	out2 := p.Format(out)
	require.Equal(t, outStr, string(out2))

	// Parseable.
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors)
	require.NoError(t, err)
}
