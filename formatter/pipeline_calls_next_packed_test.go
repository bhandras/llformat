package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_MultiLineCalls_DefaultsToPackedForGenericCalls(t *testing.T) {
	const in = `package p

func f() {
	a := 1
	b := 2
	c := 3
	d := 4
	e := 5
	_ = veryLongFunctionNameForTestingPurposes(a, b, c, d, e)
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          30,
		TabStop:              8,
		UseDSLMultiLineCalls: true,
		// Leave DSLMultiLineStyle empty to test the default.
		DSLMultiLineStyle: "",
		// Keep other stages off so the test is focused.
		UseDSLLogCalls:   false,
		UseDSLExpr:       false,
		UseDSLComments:   false,
		UseDSLFuncSigs:   false,
		UseDSLBlankLines: false,
	})

	out := string(p.Format([]byte(in)))

	// Packed multiline: callee + "(" on the first line, then args packed, then
	// closing paren on its own line.
	require.Contains(t, out, "\t_ = veryLongFunctionNameForTestingPurposes(\n\t\t",
		"expected the long call to be rewritten as multiline")
	require.Contains(t, out, "\n\t)",
		"expected the closing paren to be on its own line")
	// Should not fall back to a one-arg-per-line legacy style.
	require.Contains(t, out, "a, b, c,",
		"expected args to be packed tightly, not forced one-per-line")
}

func TestPipelineNext_MultiLineCalls_DoesNotRewriteCallsThatFit(t *testing.T) {
	const in = `package p

func f() {
	_ = short(a, b, c)
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          80,
		TabStop:              8,
		UseDSLMultiLineCalls: true,
		DSLMultiLineStyle:    "",
		// Keep other stages off so the test is focused.
		UseDSLLogCalls:   false,
		UseDSLExpr:       false,
		UseDSLComments:   false,
		UseDSLFuncSigs:   false,
		UseDSLBlankLines: false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(t, out, "\t_ = short(a, b, c)", "expected short calls to remain single-line")
	require.NotContains(t, out, "short(\n", "must not reflow calls that fit within the column limit")
}

func TestPipelineNext_MultiLineCalls_ExcludesLogPrintfStyleCalls(t *testing.T) {
	const in = `package p

type logger struct{}

func (l *logger) Infof(string, ...any) {}

func f(l *logger) {
	a := 1
	b := 2
	c := 3
	d := 4
	l.Infof("a=%v b=%v c=%v d=%v", a, b, c, d)
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          30,
		TabStop:              8,
		UseDSLMultiLineCalls: true,
		DSLMultiLineStyle:    "",
		// Keep log call stage disabled so this test proves multiline-calls does
		// not rewrite printf-style calls on its own.
		UseDSLLogCalls:   false,
		UseDSLExpr:       false,
		UseDSLComments:   false,
		UseDSLFuncSigs:   false,
		UseDSLBlankLines: false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(t, out, `l.Infof("a=%v b=%v c=%v d=%v", a, b, c, d)`,
		"expected printf-style calls to be excluded from generic call reflow")
	require.NotContains(t, out, "l.Infof(\n", "must not rewrite printf-style calls in multiline-call stage")
}
