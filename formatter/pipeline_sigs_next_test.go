package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_Signatures_CollapsesMultilineReturnListWhenItFits(t *testing.T) {
	const in = `package p

type chainSyncInfo struct{}

type rpcServer struct{}

func (r *rpcServer) getChainSyncInfo() (
	*chainSyncInfo,
	error) {
	return nil, nil
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
		UseDSLLogCalls:        false,
		UseDSLMultiLineCalls:  false,
		UseDSLExpr:            false,
		UseDSLComments:        false,
		UseDSLBlankLines:      false,
		UseDSLBlankLinesNative: false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(t, out, "getChainSyncInfo() (*chainSyncInfo, error) {",
		"expected the multiline return list to be collapsed when it fits under the column limit")
	require.NotContains(t, out, "getChainSyncInfo() (\n",
		"must not keep the split return list for a short signature in next profile")
}

