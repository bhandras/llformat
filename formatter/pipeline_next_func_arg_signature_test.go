package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_FuncLitArgSignatureCanReflowInsideOwnedCall(t *testing.T) {
	// Regression: when ownership registry is enabled, the multiline-calls stage
	// owns call spans. The signatures stage still needs to be able to reflow
	// function literal signatures used as call arguments (otherwise long callback
	// signatures remain unwrapped).
	in := []byte(`package p

type ChannelEdgeInfoWithVeryLongTypeName struct{}
type ChannelEdgePolicyWithVeryLongTypeName struct{}

type Graph struct{}

func (Graph) ForEachChannel(interface{}, func(edgeInfo *ChannelEdgeInfoWithVeryLongTypeName, c1, c2 *ChannelEdgePolicyWithVeryLongTypeName) error) error {
	return nil
}

func f(graph Graph, ctx interface{}) error {
	err := graph.ForEachChannel(ctx, func(edgeInfo *ChannelEdgeInfoWithVeryLongTypeName, c1, c2 *ChannelEdgePolicyWithVeryLongTypeName) error {
		_ = edgeInfo
		_ = c1
		_ = c2
		return nil
	})
	return err
}
`)

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          60,
		TabStop:              8,
		UseOwnershipRegistry: true,
	})

	out := string(p.Format(in))

	// Expect the callback signature to reflow (it should not remain a single
	// long line).
	require.Contains(t, out, "func(\n\t\t\tedgeInfo *ChannelEdgeInfoWithVeryLongTypeName,")
}
