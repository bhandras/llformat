package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_Signatures_CollapsesMultilineClosureReturnListWhenItFits(t *testing.T) {
	const in = `package p

type OpenChannelRequest struct{}
type InitFundingMsg struct{}

type rpcServer struct{}

func (r *rpcServer) parseOpenChannelReq(*OpenChannelRequest, bool) (*InitFundingMsg, error) {
	return nil, nil
}

func f(r *rpcServer) {
	requestParser := func(req *OpenChannelRequest) (
		*InitFundingMsg,
		error) {
		_ = req
		return r.parseOpenChannelReq(req, false)
	}

	_ = requestParser
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          80,
		TabStop:              8,
		RuleProfile:          "next",
		UseDSLFuncSigs:       true,
		UseDSLFuncSigsNative: true,
		DSLSigsStyle:         "legacy",
		// Keep other stages off to make this test focused.
		UseDSLLogCalls:       false,
		UseDSLMultiLineCalls: false,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLBlankLines:     false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(t, out, "requestParser := func(req *OpenChannelRequest) (*InitFundingMsg, error) {",
		"expected the multiline closure return list to be collapsed when it fits under the column limit")
	require.NotContains(t, out, "requestParser := func(req *OpenChannelRequest) (\n",
		"must not keep the split return list for a short closure signature in next profile")
}

