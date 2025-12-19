package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_PackedMultiLine_GenericCallInReturnListUnderSwitch(t *testing.T) {
	const in = `package p

func f(cond bool) (any, error) {
	switch {
	case cond:
		return nil, someVeryLongCalleeNameForTestingPurposes(a, b, c, d, e, f, g)
	default:
		return nil, nil
	}
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          40,
		TabStop:              8,
		RuleProfile:          "next",
		UseDSLMultiLineCalls: true,
		DSLMultiLineStyle:    "",
		UseDSLLogCalls:       false,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLFuncSigs:       false,
		UseDSLBlankLines:     false,
	})

	out := string(p.Format([]byte(in)))

	// The packed formatter should keep `return nil, <callee>(` together on the
	// first line and then pack args on continuation lines.
	require.Contains(t, out, "\t\treturn nil, someVeryLongCalleeNameForTestingPurposes(\n\t\t\t",
		"expected packed multiline call formatting under switch/case with a return list")
	require.Contains(t, out, "\n\t\t)\n",
		"expected closing paren on its own line")
}

func TestPipelineNext_PackedMultiLine_GenericCallInAssignmentUnderIf(t *testing.T) {
	const in = `package p

func f(ok bool) {
	if ok {
		x := someVeryLongCalleeNameForTestingPurposes(a, b, c, d, e, f, g)
		_ = x
	}
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          40,
		TabStop:              8,
		RuleProfile:          "next",
		UseDSLMultiLineCalls: true,
		DSLMultiLineStyle:    "",
		UseDSLLogCalls:       false,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLFuncSigs:       false,
		UseDSLBlankLines:     false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(t, out, "\t\tx := someVeryLongCalleeNameForTestingPurposes(\n\t\t\t",
		"expected packed multiline call formatting under an if statement")
	require.Contains(t, out, "\n\t\t)\n",
		"expected closing paren on its own line")
}

func TestPipelineNext_PackedMultiLine_IsIdempotentForGenericCalls(t *testing.T) {
	const in = `package p

func f() {
	_ = someVeryLongCalleeNameForTestingPurposes(a, b, c, d, e, f, g)
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          40,
		TabStop:              8,
		RuleProfile:          "next",
		UseDSLMultiLineCalls: true,
		DSLMultiLineStyle:    "",
		UseDSLLogCalls:       false,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLFuncSigs:       false,
		UseDSLBlankLines:     false,
	})

	out1 := string(p.Format([]byte(in)))
	out2 := string(p.Format([]byte(out1)))
	require.Equal(t, out1, out2, "expected packed multiline call formatting to be idempotent")
}

