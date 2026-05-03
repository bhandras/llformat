package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_Signatures_InterfaceMethod_CollapsesMultilineReturnListWhenItFits(
	t *testing.T) {

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

	require.Contains(
		t, out, "getChainSyncInfo() (*chainSyncInfo, error)", "expec"+
			"ted the multiline return list in an interface "+
			"method to be collapsed when it fits under the "+
			"column limit",
	)
	require.NotContains(
		t, out, "getChainSyncInfo() (\n", "must not keep the split "+
			"return list for a short interface method "+
			"signature in next profile",
	)
}

func TestPipelineNext_Signatures_InterfaceMethod_BreaksLongSingleLineMethod(
	t *testing.T) {

	const in = `package p

type I interface {
	VeryLongMethodName(a, b, c, d, e, f, g int, s string, other map[string]*struct{ X, Y, Z int }) (int, error)
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          80,
		TabStop:              8,
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

	require.NotContains(
		t, out, "VeryLongMethodName(a, b, c, d, e, f, g int, s "+
			"string, other map[string]*struct{ X, Y, Z int }) "+
			"(int, error)", "expected the interface method "+
			"signature to be broken across lines when it "+
			"exceeds the column limit",
	)
	require.Contains(
		t, out, "VeryLongMethodName(",
		"expected the method name to remain intact after reformatting",
	)
	require.Contains(
		t, out, "s string,\n		other", "expected the "+
			"interface method signature to be broken across "+
			"lines when it exceeds the column limit",
	)
}

func TestPipelineNext_Signatures_InterfaceMethod_PrefersInlineSmallReturnListByBreakingParams(
	t *testing.T) {

	const in = `package p

import "context"

type Invoice struct{}

type I interface {
	// InvoicesAddedSince returns invoices.
	InvoicesAddedSince(ctx context.Context, sinceAddIndex uint64) ([]Invoice, error)
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          80,
		TabStop:              8,
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

	// Keep doc comment directly adjacent to the method (no blank line),
	// otherwise godoc detaches.
	require.Contains(
		t, out, "	// InvoicesAddedSince returns "+
			"invoices.\n	InvoicesAddedSince(", "doc comment "+
			"must remain directly adjacent to its interface method",
	)

	// Keep small return lists inline by breaking params earlier when
	// needed.
	require.Contains(
		t, out, ") ([]Invoice, error)",
		"expected a small return list to stay inline in next profile",
	)

	// Don't partially break inside the return list like: ... ([]Invoice,
	// error)
	require.NotContains(
		t, out, "([]Invoice,\n		error)", "next profile "+
			"should not partially break inside a parenthesized "+
			"return list",
	)
	require.NotContains(
		t, out, ") "+
			"("+
			"\n"+
			"		[]Invoice,"+
			"\n		error,\n	)", "expected "+
			"params to break before forcing a multiline return "+
			"list when the return list is small",
	)
}

func TestPipelineNext_Signatures_InterfaceMethod_UsesCanonicalMultilineReturnListForLongReturns(
	t *testing.T) {

	const in = `package p

import (
	"context"

	"github.com/lightningnetwork/lnd/lntypes"
)

type Invoice struct{}

type I interface {
	FetchPendingInvoices(ctx context.Context) (map[lntypes.Hash]Invoice, error)
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          80,
		TabStop:              8,
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

	// Don't partially break inside the return list.
	require.NotContains(
		t, out, "(map[lntypes.Hash]Invoice,\n		error)", "ne"+
			"xt profile should not partially break inside a "+
			"parenthesized return list",
	)

	// For long returns, prefer gofmt-like multiline results.
	require.Contains(
		t, out, ") "+
			"("+
			"\n"+
			"		map[lntypes.Hash]Invoice,"+
			"\n		error,\n	)", "next profile "+
			"should use canonical multiline return lists for "+
			"long return types",
	)

	// No blank lines inside the return list.
	require.NotContains(
		t, out, "map[lntypes.Hash]Invoice,\n\n		error",
		"should not introduce empty lines inside the return list",
	)
}
