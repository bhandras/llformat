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

	require.Contains(t, out, "getChainSyncInfo() (*chainSyncInfo, error) {",
		"expected the multiline return list to be collapsed when it fits under the column limit")
	require.NotContains(t, out, "getChainSyncInfo() (\n",
		"must not keep the split return list for a short signature in next profile")
}

func TestPipelineNext_Signatures_InsertsBlankLineAfterAlreadyMultilineSignature(t *testing.T) {
	const in = `package p

type SomeRidiculouslyLongParameterTypeNameThatForcesLineBreakUnder80Columns struct{}
type AnotherRidiculouslyLongParameterTypeNameThatAlsoForcesLineBreak struct{}

func alreadyFormatted(
	first SomeRidiculouslyLongParameterTypeNameThatForcesLineBreakUnder80Columns,
	second AnotherRidiculouslyLongParameterTypeNameThatAlsoForcesLineBreak) {
	return
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

	require.Contains(t, out,
		"func alreadyFormatted(\n\tfirst SomeRidiculouslyLongParameterTypeNameThatForcesLineBreakUnder80Columns,\n\tsecond AnotherRidiculouslyLongParameterTypeNameThatAlsoForcesLineBreak) {",
		"expected the already-multiline signature to remain multiline")
	require.Contains(t, out, ") {\n\n\treturn",
		"expected a blank line after the opening brace for an already-multiline signature")
}
