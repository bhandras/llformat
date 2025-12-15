package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineDSLMultiLineLayoutArgsBreaksCompositeKeyValueLogicalValue(t *testing.T) {
	const in = `package p

type cfg struct {
	Allow bool
}

func outerFunctionNameThatIsVeryLong(c cfg) {}

func f() {
	outerFunctionNameThatIsVeryLong(cfg{Allow: firstConditionThatIsVeryLong && secondConditionThatIsVeryLong && thirdConditionThatIsVeryLong})
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          70,
		TabStop:              8,
		UseDSLMultiLineCalls: true,
		DSLMultiLineStyle:    "layout-args",
	})

	out := p.Format([]byte(in))
	outStr := string(out)

	// Ensure the call breaks and the KV value breaks with continuation indent.
	require.Contains(t, outStr, "outerFunctionNameThatIsVeryLong(\n\t\tcfg{")
	require.Contains(t, outStr, "Allow: firstConditionThatIsVeryLong &&")
	require.Contains(t, outStr, "\n\t\t\t\tsecondConditionThatIsVeryLong &&")
	require.Contains(t, outStr, "\n\t\t\t\tthirdConditionThatIsVeryLong,")

	// Idempotent.
	out2 := p.Format(out)
	require.Equal(t, outStr, string(out2))

	// Parseable.
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors)
	require.NoError(t, err)
}

