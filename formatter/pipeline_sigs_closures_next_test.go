package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_Signatures_CollapsesMultilineClosureReturnListWhenItFits(
	t *testing.T) {

	const in = `package p

type OpenChannelRequest struct{}
type InitFundingMsg struct{}

type rpcServer struct{}

func (r *rpcServer) parseOpenChannelReq(*OpenChannelRequest, bool) (*InitFundingMsg, error) {
	return nil, nil
}

func f(r *rpcServer) {
	rp := func(req *OpenChannelRequest) (
		*InitFundingMsg,
		error) {
		_ = req
		return r.parseOpenChannelReq(req, false)
	}

	_ = rp
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          80,
		TabStop:              8,
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

	require.Contains(
		t, out, "rp := func(req *OpenChannelRequest) "+
			"(*InitFundingMsg, error) {", "expected the "+
			"multiline closure return list to be collapsed "+
			"when it fits under the column limit",
	)
	require.NotContains(
		t, out, "rp := func(req *OpenChannelRequest) (\n", "must "+
			"not keep the split return list for a short "+
			"closure signature in next profile",
	)
	require.NotContains(
		t, out, "rp := func(req *OpenChannelRequest) "+
			"(*InitFundingMsg, error) {\n\n		_ = req", "m"+
			"ust not add a blank line after the opening brace "+
			"when the closure signature is single-line",
	)
}

func TestPipelineNext_Signatures_RemovesBlankAfterCollapsedClosureSignature(
	t *testing.T) {

	const in = `package p

type OpenChannelRequest struct{}
type InitFundingMsg struct{}

func f() {
	rp := func(req *OpenChannelRequest) (
		*InitFundingMsg,
		error) {

		_ = req
	}

	_ = rp
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          80,
		TabStop:              8,
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

	require.Contains(
		t, out, "rp := func(req *OpenChannelRequest) "+
			"(*InitFundingMsg, error) {\n		_ = req",
	)
	require.NotContains(
		t, out, "rp := func(req *OpenChannelRequest) "+
			"(*InitFundingMsg, error) {\n\n",
	)
}

func TestPipelineNext_Signatures_BlankAfterMultilineClosureWithComment(
	t *testing.T) {

	const in = `package p

func run(name string, fn func(*VeryLongTestTypeName)) {}
type VeryLongTestTypeName struct{}

func f() {
	run("case", func(
		t *VeryLongTestTypeName) { //nolint:ll
		t.Helper()
	})
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          40,
		TabStop:              8,
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

	require.Contains(t, out, "{ //nolint:ll\n\n\t\tt.Helper()")
}

func TestPipelineNext_Signatures_BreaksClosureSignatureWhenPrefixOverflowsColumnLimit(
	t *testing.T) {

	const in = `package p

type OpenChannelRequest struct{}
type InitFundingMsg struct{}

func f() {
	requestParserForFundingInit := func(req *OpenChannelRequest) (*InitFundingMsg, error) {
		_ = req
		return nil, nil
	}

	_ = requestParserForFundingInit
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          80,
		TabStop:              8,
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

	require.Contains(
		t, out, "	requestParserForFundingInit := func(req "+
			"*OpenChannelRequest) "+
			"(\n		*InitFundingMsg, error) {", "expecte"+
			"d the closure signature to break when the "+
			"assignment prefix makes it exceed the column limit",
	)
	require.NotContains(
		t, out, "	requestParserForFundingInit := func(req "+
			"*OpenChannelRequest) (*InitFundingMsg, error) {",
		"must not keep the closure signature on a single line when "+
			"it overflows due to its prefix",
	)
	require.Contains(
		t, out, "	requestParserForFundingInit := func(req "+
			"*OpenChannelRequest) "+
			"(\n		*InitFundingMsg, error) "+
			"{\n\n		_ = req", "expected a blank line "+
			"after the opening brace for a multiline closure "+
			"signature",
	)

	require.NotContains(
		t, out, "		*InitFundingMsg,\n		error",
		"must not force each return type onto its own line for "+
			"closure signatures; keep them packed when possible",
	)
}

func TestPipelineNext_Signatures_InsertsBlankLineAfterAlreadyMultilineClosureSignature(
	t *testing.T) {

	const in = `package p

type SomeRidiculouslyLongParameterTypeNameThatForcesLineBreakUnder80Columns struct{}
type AnotherRidiculouslyLongParameterTypeNameThatAlsoForcesLineBreak struct{}

func f() {
	alreadyFormatted := func(
		first SomeRidiculouslyLongParameterTypeNameThatForcesLineBreakUnder80Columns,
		second AnotherRidiculouslyLongParameterTypeNameThatAlsoForcesLineBreak) {
		_ = first
		_ = second
	}

	_ = alreadyFormatted
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          80,
		TabStop:              8,
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

	require.Contains(
		t, out, "	alreadyFormatted := "+
			"func(\n		first "+
			"SomeRidiculouslyLongParameterTypeNameThatForcesLine"+
			"BreakUnder80Columns,\n		second "+
			"AnotherRidiculouslyLongParameterTypeNameThatAlsoFor"+
			"cesLineBreak) {", "expected the already-multiline "+
			"closure signature to remain multiline",
	)
	require.Contains(
		t, out, ") {\n\n		_ = first", "expected a "+
			"blank line after the opening brace for an "+
			"already-multiline closure signature",
	)
}
