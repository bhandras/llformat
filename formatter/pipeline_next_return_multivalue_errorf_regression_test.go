package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_Regressions_ReturnMultiValue_DontBreakBeforeErrorf(t *testing.T) {
	// Regression: a multi-value return has a comma in the prefix (`return nil, `),
	// but it should not trigger the "break before call to avoid over-reflowing"
	// heuristic intended for multi-assign LHS prefixes.
	const in = `package p

import "fmt"

func f(reqTimeLockDelta, minTimeLockDelta int) (any, error) {
	if reqTimeLockDelta < minTimeLockDelta {
		return nil, fmt.Errorf("time lock delta of %v is too small, " +
			"minimum supported is %v", reqTimeLockDelta,
			minTimeLockDelta)
	}

	return nil, nil
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          80,
		TabStop:              8,
		UseDSLFuncSigs:       true,
		UseDSLFuncSigsNative: true,
		DSLSigsStyle:         "legacy",
		UseDSLMultiLineCalls: true,
		UseDSLLogCalls:       true,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLBlankLines:     false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(t, out, "return nil, fmt.Errorf(", "should keep the call on the same line as the multi-value return")
	require.NotContains(t, out, "return nil,\n\tfmt.Errorf(", "must not treat `return nil,` as a multi-assign prefix")
	require.NotContains(t, out, "fmt.Errorf(\n", "must avoid hanging-paren layout for fmt.Errorf in packed next profile")
}
