package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_Regressions_RouterBackendFuncFields_PackSignaturesAndAvoidOverBreakingCalls(
	t *testing.T) {

	// Regression snippet extracted from reported router backend composite
	// literal. Key expectations:
	// - For func literals used as composite literal fields, keep short
	//   signatures packed when possible.
	// - For multi-assign statements where the call itself would fit on a
	//   clean continuation line, prefer breaking before the call rather
	//   than rewriting it into a multiline call with one arg per line.
	const in = `package p

import "fmt"

type btcutilAmount int64

type graphT struct{}

type infoT struct{ Capacity btcutilAmount }

func (graphT) FetchChannelEdgesByID(uint64) (int, int, int, infoT, error) {
	return 0, 0, 0, infoT{}, nil
}

type RouterBackend struct {
	FetchChannelCapacity  func(chanID uint64) (btcutilAmount, error)
	FetchChannelEndpoints func(chanID uint64) (int, int, error)
}

func f(graph graphT) {
	routerBackend := &RouterBackend{
		FetchChannelCapacity: func(chanID uint64) (
			btcutilAmount, error,
		) {
			_, _, _, info, err := graph.FetchChannelEdgesByID(chanID)
			if err != nil {
				return 0, err
			}

			return info.Capacity, nil
		},
		FetchChannelEndpoints: func(chanID uint64) (
			int,
			int,
			error,
		) {
			_, _, _, info, err := graph.FetchChannelEdgesByID(
				chanID,
			)
			if err != nil {
				return 0, 0,
					fmt.Errorf("unable to fetch channel "+
						"edges by channel ID %d: %v",
						chanID, err)
			}

			return 0, 0, nil
		},
	}
	_ = routerBackend
}
`

	p := NewPipeline(
		PipelineConfig{
			ColumnLimit:          80,
			TabStop:              8,
			UseDSLFuncSigs:       true,
			UseDSLFuncSigsNative: true,
			DSLSigsStyle:         "legacy",
			UseDSLMultiLineCalls: true,
			UseDSLLogCalls:       true,
			UseDSLExpr:           false,
			UseDSLComments:       false,
			UseDSLBlankLines:     false,
		},
	)

	out := string(p.Format([]byte(in)))

	// Prefer keeping params compact. If the field prefix causes overflow,
	// wrap the return list instead of breaking a simple single-arg param
	// list.
	require.Contains(
		t, out, "FetchChannelCapacity: func(chanID uint64) (", "expe"+
			"cted single-arg closure params to stay on the "+
			"same line as `func(`",
	)
	require.Contains(
		t, out, "FetchChannelCapacity: func(chanID uint64) "+
			"(btcutilAmount,\n			error) {",
		"expected the return list to wrap under prefix pressure "+
			"(keeping params compact)",
	)
	require.Contains(
		t, out, "FetchChannelEndpoints: func(chanID uint64) (int, "+
			"int, error) {", "expected func literal signature "+
			"return list to be packed like normal signatures",
	)

	// Multi-assign call: prefer keeping the assignment attached by
	// formatting the call itself as packed multiline when needed (avoids
	// detaching with a break-before-call rewrite).
	require.Contains(
		t, out, "err := graph.FetchChannelEdgesByID(\n", "expected "+
			"packed multiline call for a long multi-assign "+
			"prefix in next",
	)
	require.NotContains(
		t, out, "err "+
			":=\n			graph.FetchChannelEdgesByID(",
		"must not detach the call from the assignment",
	)
}

func TestPipelineNext_Regressions_FuncLitFieldPrefix_OverflowsLine_BreakBeforeFunc(
	t *testing.T) {

	// This matches the reported case where the func literal signature is
	// short but the field name prefix pushes the overall line past the
	// column limit.
	//
	// Note: the pipeline runs gofmt as the final stage, and gofmt will not
	// keep a newline between `Field:` and `func(...) {`. For very long
	// field names, we can't guarantee a hard column limit for the `Field:
	// func(` line. The main goal here is to keep the signature itself
	// packed and avoid introducing awkward multiline return lists due to
	// prefix-width budget reduction.
	const in = `package p

type btcutilAmount int64

type RouterBackend struct {
	FetchChannelCapacityWithVeryLongFieldNameThatForcesBreakingBecauseOfPrefix func(chanID uint64) (btcutilAmount, error)
}

func f() {
	_ = &RouterBackend{
		FetchChannelCapacityWithVeryLongFieldNameThatForcesBreakingBecauseOfPrefix:      func(chanID uint64) (btcutilAmount, error) {
			return 0, nil
		},
	}
}
`

	p := NewPipeline(
		PipelineConfig{
			ColumnLimit:          80,
			TabStop:              8,
			UseDSLFuncSigs:       true,
			UseDSLFuncSigsNative: true,
			DSLSigsStyle:         "legacy",
			UseDSLMultiLineCalls: false,
			UseDSLLogCalls:       false,
			UseDSLExpr:           false,
			UseDSLComments:       false,
			UseDSLBlankLines:     false,
		},
	)

	out := string(p.Format([]byte(in)))

	require.Contains(
		t, out, "FetchChannelCapacityWithVeryLongFieldNameThatForces"+
			"BreakingBecauseOfPrefix: func(chanID uint64) "+
			"(btcutilAmount, error) {", "expected to avoid "+
			"over-breaking the signature when the prefix alone "+
			"already overflows the limit",
	)
}
