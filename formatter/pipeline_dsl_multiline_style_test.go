package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDSLMultiLineStylePackedVsLegacy(t *testing.T) {
	const in = `package p

func f() {
	a := 1
	b := 2
	c := 3
	d := 4

	// Force multiline formatting with a small column limit.
	_ = someFunc(a, b, c, d)
}

func someFunc(a, b, c, d int) int { return 0 }
`

	legacy := NewPipeline(PipelineConfig{
		ColumnLimit:          21,
		TabStop:              8,
		UseDSLLogCalls:       false,
		UseDSLMultiLineCalls: true,
		DSLMultiLineStyle:    "legacy",
		UseDSLExpr:           false,
	})
	legacyOut := string(legacy.Format([]byte(in)))

	packed := NewPipeline(PipelineConfig{
		ColumnLimit:          21,
		TabStop:              8,
		UseDSLLogCalls:       false,
		UseDSLMultiLineCalls: true,
		DSLMultiLineStyle:    "packed",
		UseDSLExpr:           false,
	})
	packedOut := string(packed.Format([]byte(in)))

	// Legacy style is strictly one argument per line.
	require.Contains(t, legacyOut, "\n\t_ = someFunc(\n\t\ta,\n\t\tb,\n\t\tc,\n\t\td,\n\t)\n")

	// Packed style should place multiple args on a single line when they fit.
	require.Contains(t, packedOut, "\n\t_ = someFunc(\n\t\ta, b,\n\t\tc, d,\n\t)\n")
}
