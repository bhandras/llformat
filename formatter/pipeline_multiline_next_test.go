package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_MultiLineCalls_MakeKeepsFirstArgInline(t *testing.T) {
	const in = `package p

func f(chanBackupsProtos struct{ ChanBackups []int }) {
	packedBackupsFromProtoSource := make([][]byte, 0, len(chanBackupsProtos.ChanBackups))
	_ = packedBackups
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          60,
		TabStop:              8,
		UseDSLMultiLineCalls: true,
		// Keep other DSL stages off to make this test focused.
		UseDSLLogCalls:    false,
		UseDSLExpr:        false,
		UseDSLComments:    false,
		UseDSLFuncSigs:    false,
		UseDSLBlankLines:  false,
		DSLMultiLineStyle: "",
	})

	out := string(p.Format([]byte(in)))

	require.Contains(
		t, out, "packedBackupsFromProtoSource := make(\n", "expected"+
			" the multiline make() call to break right after "+
			"`make(`",
	)
	require.Contains(
		t, out, "		[][]byte, 0,", "expected packed "+
			"multiline call args (avoid one-arg-per-line where "+
			"possible)",
	)
	require.NotContains(
		t, out, "		[][]byte,\n		0,", "must "+
			"not force each argument onto its own line for "+
			"make() calls",
	)
	require.Contains(
		t, out, "len(chanBackupsProtos.ChanBackups)", "must not "+
			"explode nested len(...) into a multiline call "+
			"when it can remain flat",
	)
}

func TestPipelineNext_MultiLineCalls_BreaksAfterMultilineCallArg(t *testing.T) {
	const in = `package p

func record(...any) {}

func primitive(int, *int, func() error) any { return nil }
func dynamic(int, *int, func() error, func() error) any { return nil }

func f(a, b int, av, bv *int) {
	record(
		primitive(
			a,
			av,
			func() error {
				return nil
			},
		),
		dynamic(
			b,
			bv,
			func() error {
				return nil
			},
			func() error {
				return nil
			},
		),
	)
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          72,
		TabStop:              8,
		UseDSLMultiLineCalls: true,
		// Keep other DSL stages off to make this test focused.
		UseDSLLogCalls:   false,
		UseDSLExpr:       false,
		UseDSLComments:   false,
		UseDSLFuncSigs:   false,
		UseDSLBlankLines: false,
	})

	out := string(p.Format([]byte(in)))

	require.NotContains(t, out, "), dynamic(")
	require.Contains(t, out, "\t\t),\n\t\tdynamic(")
}
