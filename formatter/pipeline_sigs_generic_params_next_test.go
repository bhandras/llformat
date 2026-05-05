package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_Signatures_KeepsShortGenericParamTypesPacked(
	t *testing.T) {

	t.Parallel()

	const in = `package p

type message struct{}
type Behavior[A, B any] struct{}
type Actor[A, B any] struct{}
type Harness struct{}

func (h *Harness) newActor(id string, beh Behavior[*message, string], mailboxSize int) *Actor[*message, string] {
	return nil
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

	require.Contains(t, out, "beh Behavior[*message, string],")
	require.Contains(t, out, "mailboxSize int) *Actor[*message, string] {")
	require.NotContains(t, out, "Behavior[\n")
	require.NotContains(t, out, "*message,\n")
}

func TestPipelineNext_Signatures_KeepsSharedTypeNamePairsPacked(t *testing.T) {

	t.Parallel()

	const in = `package p

import "context"

func mergeContexts(ctx1, ctx2 context.Context) (context.Context, context.CancelFunc) {
	return ctx1, func() {}
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

	require.Contains(t, out, "mergeContexts(ctx1, ctx2 context.Context)")
	require.NotContains(t, out, "mergeContexts(ctx1,\n")
	require.NotContains(t, out, "\n\tctx2 context.Context")
}

func TestPipelineNext_Signatures_PrefersBreakingSharedTypeReturns(
	t *testing.T) {

	t.Parallel()

	const in = `package p

import "context"

func mergeContexts(scope string, ctx1, ctx2 context.Context) (context.Context, context.CancelFunc) {
	return ctx1, func() {}
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

	require.Contains(
		t, out, "mergeContexts(scope string, ctx1, ctx2 "+
			"context.Context) "+
			"(context.Context,\n	context.CancelFunc)",
	)
	require.NotContains(t, out, "ctx1,\n")
	require.NotContains(t, out, "\n\tctx2 context.Context")
}
