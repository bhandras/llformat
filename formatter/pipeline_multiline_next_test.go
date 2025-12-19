package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_MultiLineCalls_MakeKeepsFirstArgInline(t *testing.T) {
	const in = `package p

func f(chanBackupsProtos struct{ ChanBackups []int }) {
	packedBackups := make([][]byte, 0, len(chanBackupsProtos.ChanBackups))
	_ = packedBackups
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          52,
		TabStop:              8,
		RuleProfile:          "next",
		UseDSLMultiLineCalls: true,
		// Keep other DSL stages off to make this test focused.
		UseDSLLogCalls:     false,
		UseDSLExpr:         false,
		UseDSLComments:     false,
		UseDSLFuncSigs:     false,
		UseDSLBlankLines:   false,
		DSLMultiLineStyle:  "",
	})

	out := string(p.Format([]byte(in)))

	require.Contains(t, out, "packedBackups := make([][]byte", "prefer keeping the make() type argument on the same line as `make(`")
	require.NotContains(t, out, "make(\n", "must not break immediately after `make(` for type-bearing make() calls")
	require.Contains(t, out, "len(chanBackupsProtos.ChanBackups)", "must not explode nested len(...) into a multiline call when it can remain flat")
}

