package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_Regressions_SignaturesStage_DoesNotCycleAndMissLaterFuncLitSignatures(t *testing.T) {
	// Regression:
	// The signatures stage applies at most one rewrite per iteration, and in the
	// "next" profile it enables cycle detection. A non-idempotent func-literal
	// signature rewrite early in the file can trigger cycle detection and stop
	// the stage before it reaches later signatures.
	//
	// This snippet includes:
	// 1) A func literal used as a composite-literal field where the prefix pushes
	//    the overall line over the column limit (common in router backend structs).
	// 2) A separate func literal used as a callback argument where the prefix
	//    before `func` must be accounted for when deciding to break the signature.
	const in = `package p

type btcutilAmount int64

type graphT struct{}

func (graphT) ForEachChannel(ctx any, fn func(edgeInfo *models.ChannelEdgeInfo, c1, c2 *models.ChannelEdgePolicy) error, reset func()) error {
	return fn(nil, nil, nil)
}

type RouterBackend struct {
	FetchChannelCapacity func(chanID uint64) (btcutilAmount, error)
}

func f(graph graphT) {
	_ = &RouterBackend{
		FetchChannelCapacity: func(chanID uint64) (
			btcutilAmount,
			error,
		) {
			return 0, nil
		},
	}

	// Next, for each active channel we know of within the graph, create a
	// similar response which details both the edge information as well as
	// the routing policies of th nodes connecting the two edges.
	err := graph.ForEachChannel(ctx, func(edgeInfo *models.ChannelEdgeInfo, c1, c2 *models.ChannelEdgePolicy) error {
		return nil
	}, func() {})
	_ = err
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          80,
		TabStop:              8,
		RuleProfile:          "next",
		UseDSLFuncSigs:       true,
		UseDSLFuncSigsNative: true,
		DSLSigsStyle:         "legacy",
		UseDSLMultiLineCalls: false,
		UseDSLLogCalls:       false,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLBlankLines:     false,
	})

	out := string(p.Format([]byte(in)))

	// The field prefix forces the overall line over the limit, so the signature
	// should not be collapsed into a single line (which would overflow and can
	// trigger an expand/collapse cycle).
	require.Contains(t, out, "FetchChannelCapacity: func(chanID uint64) (btcutilAmount,\n\t\t\terror) {")

	// Ensure later func-literal signatures still get formatted: the callback
	// signature should break before the shared-type name list.
	require.Contains(t, out, "func(edgeInfo *models.ChannelEdgeInfo,\n\t\tc1, c2 *models.ChannelEdgePolicy) error {")
}
