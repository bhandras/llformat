package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_Signatures_InterfaceMethod_CollapsesMultilineReturnListWhenItFits(t *testing.T) {
	const in = `package p

type chainSyncInfo struct{}

type I interface {
	getChainSyncInfo() (
		*chainSyncInfo,
		error)
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          80,
		TabStop:              8,
		RuleProfile:          "next",
		UseDSLFuncSigs:       true,
		UseDSLFuncSigsNative: true,
		DSLSigsStyle:         "legacy",
		// Keep other DSL stages off so this test stays focused.
		UseDSLLogCalls:         false,
		UseDSLMultiLineCalls:   false,
		UseDSLExpr:             false,
		UseDSLComments:         false,
		UseDSLBlankLines:       false,
		UseDSLBlankLinesNative: false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(t, out, "getChainSyncInfo() (*chainSyncInfo, error)",
		"expected the multiline return list in an interface method to be collapsed when it fits under the column limit")
	require.NotContains(t, out, "getChainSyncInfo() (\n",
		"must not keep the split return list for a short interface method signature in next profile")
}

func TestPipelineNext_Signatures_InterfaceMethod_BreaksLongSingleLineMethod(t *testing.T) {
	const in = `package p

type I interface {
	VeryLongMethodName(a, b, c, d, e, f, g int, s string, other map[string]*struct{ X, Y, Z int }) (int, error)
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          80,
		TabStop:              8,
		RuleProfile:          "next",
		UseDSLFuncSigs:       true,
		UseDSLFuncSigsNative: true,
		DSLSigsStyle:         "legacy",
		// Keep other DSL stages off so this test stays focused.
		UseDSLLogCalls:         false,
		UseDSLMultiLineCalls:   false,
		UseDSLExpr:             false,
		UseDSLComments:         false,
		UseDSLBlankLines:       false,
		UseDSLBlankLinesNative: false,
	})

	out := string(p.Format([]byte(in)))

	require.NotContains(t, out, "VeryLongMethodName(a, b, c, d, e, f, g int, s string, other map[string]*struct{ X, Y, Z int }) (int, error)",
		"expected the interface method signature to be broken across lines when it exceeds the column limit")
	require.Contains(t, out, "VeryLongMethodName(",
		"expected the method name to remain intact after reformatting")
	require.Contains(t, out, "s string,\n\t\tother",
		"expected the interface method signature to be broken across lines when it exceeds the column limit")
}

func TestPipelineNext_Signatures_InterfaceMethod_UsesCanonicalMultilineReturnList(t *testing.T) {
	const in = `package p

import "context"

type Invoice struct{}

type I interface {
	InvoicesAddedSince(ctx context.Context, sinceAddIndex uint64) ([]Invoice, error)
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          80,
		TabStop:              8,
		RuleProfile:          "next",
		UseDSLFuncSigs:       true,
		UseDSLFuncSigsNative: true,
		DSLSigsStyle:         "legacy",
		// Keep other DSL stages off so this test stays focused.
		UseDSLLogCalls:         false,
		UseDSLMultiLineCalls:   false,
		UseDSLExpr:             false,
		UseDSLComments:         false,
		UseDSLBlankLines:       false,
		UseDSLBlankLinesNative: false,
	})

	out := string(p.Format([]byte(in)))

	// Don't partially break inside the return list like:
	//   ... ([]Invoice,
	//     error)
	require.NotContains(t, out, "([]Invoice,\n\t\terror)",
		"next profile should not partially break inside a parenthesized return list")

	// Prefer gofmt-like multiline results when it doesn't fit.
	require.Contains(t, out, ") (\n\t\t[]Invoice,\n\t\terror,\n\t)",
		"next profile should use canonical multiline return lists with the closing paren on its own line")

	// No blank lines inside the return list.
	require.NotContains(t, out, "[]Invoice,\n\n\t\terror",
		"should not introduce empty lines inside the return list")
}
