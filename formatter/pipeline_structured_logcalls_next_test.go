package formatter

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_StructuredLogCalls_PacksKeyValuePairs(t *testing.T) {
	t.Parallel()

	const in = `package p

type Logger interface { InfoS(string, ...any) }

func f(log Logger, itemID string, count int, retry bool, reason string) {
	log.InfoS("processed event with a longer message", "item_id", itemID, "count", count, "retry", retry, "reason", reason)
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          80,
		TabStop:              8,
		UseDSLLogCalls:       true,
		UseDSLMultiLineCalls: false,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLFuncSigs:       false,
		UseDSLBlankLines:     false,
	})

	out1 := p.Format([]byte(in))
	out2 := p.Format(out1)
	out := string(out1)

	require.Contains(
		t, out, `log.InfoS("processed event with a longer message",`,
	)
	require.Contains(t, out, "\"item_id\", itemID,")
	require.Contains(t, out, "\"count\", count,")
	require.Contains(t, out, "\"retry\", retry,")
	require.Contains(t, out, `"reason", reason,`)
	require.NotContains(t, out, "\"item_id\",\n\t\titemID")
	require.NotContains(t, out, `"item_id", itemID, "count", count`)
	require.Equal(t, string(out1), string(out2), "not idempotent")
	requireASTEquivalent(t, []byte(in), out1)
}

func TestPipelineNext_StructuredLogCalls_PacksContextPrelude(t *testing.T) {
	t.Parallel()

	const in = `package p

type Logger interface { DebugS(any, string, ...any) }

func f(log Logger, ctx any, batchID string, nextHeight int, blocksRemaining int) {
	log.DebugS(ctx, "sweep candidates not yet mature", "batch_id", batchID, "next_maturity_height", nextHeight, "blocks_remaining", blocksRemaining)
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          80,
		TabStop:              8,
		UseDSLLogCalls:       true,
		UseDSLMultiLineCalls: false,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLFuncSigs:       false,
		UseDSLBlankLines:     false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(
		t, out,
		"log.DebugS(ctx, \"sweep candidates not yet mature\",\n",
	)
	require.Contains(t, out, "\t\t\"batch_id\", batchID,\n")
	require.Contains(
		t, out,
		"		\"next_maturity_height\", nextHeight,\n",
	)
	require.Contains(
		t, out,
		"		\"blocks_remaining\", blocksRemaining,\n",
	)
	require.NotContains(
		t, out, `"batch_id", batchID, "next_maturity_height"`,
	)
}

func TestPipelineNext_StructuredLogCalls_PacksSlogAttrs(t *testing.T) {
	t.Parallel()

	const in = `package p

import "log/slog"

type Logger interface { InfoS(any, string, ...any) }

func f(log Logger, ctx any, signers []string, signerTxIndex []int) {
	log.InfoS(ctx, "created tree sign coordinator", slog.Int("tx_count", len(signers)), slog.Int("cosigner_count", len(signerTxIndex)))
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          80,
		TabStop:              8,
		UseDSLLogCalls:       true,
		UseDSLMultiLineCalls: false,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLFuncSigs:       false,
		UseDSLBlankLines:     false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(
		t, out, "log.InfoS(ctx, \"created tree sign coordinator\",\n",
	)
	require.Contains(
		t, out,
		"		slog.Int(\"tx_count\", len(signers)),\n",
	)
	require.Contains(
		t, out, "		slog.Int(\"cosigner_count\", "+
			"len(signerTxIndex)),\n",
	)
	require.NotContains(t, out, "slog.Int(\n")
	requireNoLineLongerThan(t, out, 80)
}

func TestPipelineNext_StructuredLogCalls_ErrorSKeepsPreludeCompact(
	t *testing.T) {

	t.Parallel()

	const in = `package p

type Logger interface { ErrorS(error, string, ...any) }

func f(log Logger, itemID string, count int, err error) {
	log.ErrorS(err, "failed processing event with a longer message", "item_id", itemID, "count", count)
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          80,
		TabStop:              8,
		UseDSLLogCalls:       true,
		UseDSLMultiLineCalls: false,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLFuncSigs:       false,
		UseDSLBlankLines:     false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(
		t, out,
		`log.ErrorS(err, "failed processing event with a longer message",`,
	)
	require.Contains(t, out, "\"item_id\", itemID,\n")
	require.Contains(t, out, "\"count\", count,\n")
	require.NotContains(t, out, `"failed processing event", "item_id"`)
	require.NotContains(t, out, `"item_id", itemID, "count", count`)
}

func TestPipelineNext_StructuredLogCalls_SplitsPairOnlyWhenUnavoidable(
	t *testing.T) {

	t.Parallel()

	const in = `package p

type Logger interface { InfoS(string, ...any) }

func f(log Logger, value any) {
	log.InfoS("processed event", "short_key", value, "oversized_key", veryLongWrapperCall(value, value, value, value))
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          48,
		TabStop:              8,
		UseDSLLogCalls:       true,
		UseDSLMultiLineCalls: false,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLFuncSigs:       false,
		UseDSLBlankLines:     false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(t, out, `"short_key", value,`)
	require.Contains(t, out, `"oversized_key",`)
	require.Contains(t, out, "veryLongWrapperCall")
	require.Less(
		t, strings.Index(out, `"oversized_key",`),
		strings.Index(out, "veryLongWrapperCall"),
	)
}

func TestPipelineNext_StructuredLogCalls_DoesNotChangePrintfCalls(
	t *testing.T) {

	t.Parallel()

	const in = `package p

type Logger interface { Infof(string, ...any) }

func f(log Logger, itemID string, count int) {
	log.Infof("processed event for item %s with count %d", itemID, count)
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          55,
		TabStop:              8,
		UseDSLLogCalls:       true,
		UseDSLMultiLineCalls: false,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLFuncSigs:       false,
		UseDSLBlankLines:     false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(t, out, `log.Infof(`)
	require.NotContains(t, out, `"itemID", count`)
	require.NotContains(t, out, `"itemID",`)
}
