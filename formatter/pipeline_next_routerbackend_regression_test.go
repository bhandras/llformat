package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_Regressions_RouterBackendFuncFields_PackSignaturesAndAvoidOverBreakingCalls(t *testing.T) {
	// Regression snippet extracted from reported router backend composite literal.
	// Key expectations:
	// - For func literals used as composite literal fields, keep short signatures
	//   packed when possible.
	// - For multi-assign statements where the call itself would fit on a clean
	//   continuation line, prefer breaking before the call rather than rewriting
	//   it into a multiline call with one arg per line.
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

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          80,
		TabStop:              8,
		RuleProfile:          "next",
		UseDSLFuncSigs:       true,
		UseDSLFuncSigsNative: true,
		DSLSigsStyle:         "legacy",
		UseDSLMultiLineCalls: true,
		UseDSLLogCalls:       true,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLBlankLines:     false,
	})

	out := string(p.Format([]byte(in)))

	// Func literal return lists should stay packed, but if the full single-line
	// signature would overflow due to the composite literal field prefix,
	// prefer breaking the parameter list instead of forcing multiline returns.
	require.Contains(t, out, "FetchChannelCapacity: func(\n\t\t\tchanID uint64,\n\t\t) (btcutilAmount, error) {",
		"expected func literal to keep return list packed and break params when needed")
	require.Contains(t, out, "FetchChannelEndpoints: func(chanID uint64) (int, int, error) {",
		"expected func literal signature return list to be packed like normal signatures")

	// Multi-assign call: prefer breaking before the call instead of rewriting the
	// call args into a multiline call.
	require.NotContains(t, out, "FetchChannelEdgesByID(\n", "must not reflow a one-arg call into multiline just because the prefix is long")
	require.Contains(t, out, "graph.FetchChannelEdgesByID(chanID)", "expected the call to remain single-line")
}

func TestPipelineNext_Regressions_FuncLitFieldPrefix_OverflowsLine_BreakBeforeFunc(t *testing.T) {
	// This matches the reported case where the func literal signature is short
	// but the field name prefix pushes the overall line past the column limit.
	//
	// Note: the pipeline runs gofmt as the final stage, and gofmt will not keep
	// a newline between `Field:` and `func(...) {`. For very long field names,
	// we can't guarantee a hard column limit for the `Field: func(` line. The
	// main goal here is to keep the signature itself packed and avoid
	// introducing awkward multiline return lists due to prefix-width budget
	// reduction.
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

	require.Contains(t, out, "FetchChannelCapacityWithVeryLongFieldNameThatForcesBreakingBecauseOfPrefix: func(\n\t\t\tchanID uint64) (btcutilAmount, error) {",
		"expected the signature to keep a packed return list under prefix pressure")
	require.NotContains(t, out, "func(chanID uint64) (\n", "must not break the return list when it can remain packed")
}
