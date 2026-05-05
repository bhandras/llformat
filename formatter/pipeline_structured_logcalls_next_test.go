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
	require.Contains(
		t, out, `"item_id", itemID, "count", count, "retry", retry,`,
	)
	require.Contains(t, out, `"reason", reason,`)
	require.NotContains(t, out, "\"item_id\",\n\t\titemID")
	require.Equal(t, string(out1), string(out2), "not idempotent")
	requireASTEquivalent(t, []byte(in), out1)
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
	require.Contains(t, out, `"item_id", itemID, "count", count,`)
	require.NotContains(t, out, `"failed processing event", "item_id"`)
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
