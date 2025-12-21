package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_MultiAssign_LongPrefix_PrefersPackedMultilineCallOverBreakingBeforeCall(t *testing.T) {
	// Regression for cases where a multi-assign prefix makes a call overflow:
	//
	// Prefer keeping the assignment visually intact by turning the call into a
	// packed multiline call:
	//   a, b, c := f(
	//     x,
	//   )
	// instead of detaching the call from `:=`:
	//   a, b, c :=
	//     f(x)
	const in = `package p

type graphT struct{}

func (graphT) FetchChannelEdgesByID(uint64) (int, int, int, error) {
	return 0, 0, 0, nil
}

func f(graph graphT, chanID uint64) error {
	if true {
		_, _, _, err := graph.FetchChannelEdgesByID(chanID)
		_ = err
	}
	return nil
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          50,
		TabStop:              8,
		UseDSLMultiLineCalls: true,
		// Keep other stages off to make this test focused.
		UseDSLLogCalls:   false,
		UseDSLExpr:       false,
		UseDSLComments:   false,
		UseDSLFuncSigs:   false,
		UseDSLBlankLines: false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(t, out, "err := graph.FetchChannelEdgesByID(\n\t\t\tchanID,\n\t\t)\n",
		"expected the one-arg call to become a packed multiline call when the multi-assign prefix overflows")
	require.NotContains(t, out, "err :=\n\t\t\tgraph.FetchChannelEdgesByID(",
		"must not detach the call from the assignment with a break-before-call rewrite")
}

func TestPipelineNext_MultiAssign_AlreadyMultilineCall_DoesNotDetachCallFromAssignment(t *testing.T) {
	// Regression for already-multiline packed calls: the multiline stage must not
	// "optimize" them into a break-before-call + single-line call form.
	const in = `package p

type graphT struct{}

func (graphT) FetchChannelEdgesByID(uint64) (int, int, int, error) {
	return 0, 0, 0, nil
}

func f(graph graphT, chanID uint64) error {
	if true {
		_, _, _, err := graph.FetchChannelEdgesByID(
			chanID,
		)
		_ = err
	}
	return nil
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          50,
		TabStop:              8,
		UseDSLMultiLineCalls: true,
		// Keep other stages off to make this test focused.
		UseDSLLogCalls:   false,
		UseDSLExpr:       false,
		UseDSLComments:   false,
		UseDSLFuncSigs:   false,
		UseDSLBlankLines: false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(t, out, "err := graph.FetchChannelEdgesByID(\n\t\t\tchanID,\n\t\t)\n",
		"expected the already-multiline call to remain attached to the assignment")
	require.NotContains(t, out, "err :=\n\t\t\tgraph.FetchChannelEdgesByID(",
		"must not detach the call from the assignment")
}
