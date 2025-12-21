package formatter

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_CallFuncArgSignature_BreaksAtC1(t *testing.T) {
	// Regression: in next mode, func literals used as call arguments should
	// use the same signature formatter behavior as other signatures (no
	// overflow).
	//
	// This case is a common pattern in large codebases: a callback with a
	// long first argument name/type followed by multiple parameters sharing
	// a long pointer type. The desired behavior is to break after the first
	// parameter (at c1), not to keep everything on one line and overflow
	// the column limit.
	in := []byte(
		`package p

import (
	"errors"

	"graphdb"
	"models"
)

type Graph struct{}

func (Graph) ForEachChannel(ctx interface{},
	cb func(edgeInfo *models.ChannelEdgeInfo, c1, c2 *models.ChannelEdgePolicy) error,
	onDone func(),
) error {
	return nil
}

type Req struct {
	IncludeAuthProof bool
}

type Resp struct {
	Edges []interface{}
}

func marshalDBEdge(edgeInfo *models.ChannelEdgeInfo, c1, c2 *models.ChannelEdgePolicy, includeAuthProof bool) interface{} {
	return nil
}

func f(graph Graph, ctx interface{}, includeUnannounced bool, req Req) (*Resp, error) {
	resp := &Resp{}
	graphWithSomewhatLongButNotOverLimitPrefix := graph
	ctxWithSomewhatLongButNotOverLimitPrefix := ctx

	// Next, for each active channel we know of within the graph, create a
	// similar response which details both the edge information as well as
	// the routing policies of th nodes connecting the two edges.
	err := graphWithSomewhatLongButNotOverLimitPrefix.ForEachChannel(ctxWithSomewhatLongButNotOverLimitPrefix, func(edgeInfo *models.ChannelEdgeInfo, c1, c2 *models.ChannelEdgePolicy) error {

		// Do not include unannounced channels unless specifically
		// requested. Unannounced channels include both private channels
		// as well as public channels whose authentication proof were
		// not confirmed yet, hence were not announced.
		if !includeUnannounced && edgeInfo.AuthProof == nil {
			return nil
		}

		edge := marshalDBEdge(edgeInfo, c1, c2, req.IncludeAuthProof)
		resp.Edges = append(resp.Edges, edge)

		return nil
	}, func() {
		resp.Edges = nil
	})
	if err != nil && !errors.Is(err, graphdb.ErrGraphNoEdgesFound) {
		return nil, err
	}

	return resp, nil
}
`,
	)

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          80,
		TabStop:              8,
		UseOwnershipRegistry: true,
		// Be resilient to cross-stage convergence within a single
		// formatting run.
		MaxPipelineIterations: 3,
	})

	out := string(p.Format(in))

	// Ensure the callback signature breaks between edgeInfo and c1 (the key
	// fix) rather than overflowing on a single line or splitting inside the
	// shared `c1, c2 *models.ChannelEdgePolicy` group.
	require.Regexp(
		t, regexp.MustCompile(
			`func\(edgeInfo \*models\.ChannelEdgeInfo,\n\t\tc1, c2 \*models\.ChannelEdgePolicy\) error \{`,
		), out,
	)
}
