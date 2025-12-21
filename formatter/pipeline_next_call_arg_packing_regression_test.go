package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_CallArgs_PacksArgsWithoutOverflow_InNestedClosure(
	t *testing.T) {

	// Regression for cases where a packed multiline call packs too many
	// args onto one continuation line. Prefer breaking before a long
	// argument like coinSelectionStrategy rather than letting it spill past
	// the limit.
	const in = `package p

type walletT struct{}

func (walletT) WithCoinSelectLock(fn func() error) error { return fn() }

func (walletT) CreateSimpleTx(a any, b any, feePerKw, minConfs int, coinSelectionStrategy int, dryRun bool) (int, error) {
	_ = a
	_ = b
	_ = feePerKw
	_ = minConfs
	_ = coinSelectionStrategy
	_ = dryRun
	return 0, nil
}

func f(wallet walletT, outputs []int, feePerKw, minConfs int, coinSelectionStrategy int) error {
	err := wallet.WithCoinSelectLock(func() error {
		var tx int
		var err error

		tx, err = wallet.CreateSimpleTx(nil, outputs, feePerKw, minConfs, coinSelectionStrategy, true)
		_ = tx

		return err
	})

	return err
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          64,
		TabStop:              8,
		UseDSLMultiLineCalls: true,
		// Keep other DSL stages off to make this test focused.
		UseDSLLogCalls:   false,
		UseDSLExpr:       false,
		UseDSLComments:   false,
		UseDSLFuncSigs:   false,
		UseDSLBlankLines: false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(
		t, out, "tx, err = wallet.CreateSimpleTx(\n",
		"expected multiline call formatting to kick in",
	)
	require.NotContains(
		t, out, "tx, err =\n		wallet.CreateSimpleTx(", "mus"+
			"t not detach the call from the assignment with a "+
			"break-before-call rewrite",
	)
	require.NotContains(
		t, out, "feePerKw, minConfs, coinSelectionStrategy,", "expect"+
			"ed the formatter to break before the long "+
			"`coinSelectionStrategy` argument to avoid overflow",
	)
}

func TestPipelineNext_CallArgs_RePacksAlreadyMultilineCallWhenContinuationLineOverflows(
	t *testing.T) {

	// Regression: when the call is already multiline, the multiline-call
	// stage must still be willing to re-pack it if one of its continuation
	// lines exceeds the column limit.
	const in = `package p

type walletT struct{}

func (walletT) WithCoinSelectLock(fn func() error) error { return fn() }

func (walletT) CreateSimpleTx(a any, b any, feePerKw, minConfs int, coinSelectionStrategy int, dryRun bool) (int, error) {
	_ = a
	_ = b
	_ = feePerKw
	_ = minConfs
	_ = coinSelectionStrategy
	_ = dryRun
	return 0, nil
}

func f(wallet walletT, outputs []int, feePerKw, minConfs int, coinSelectionStrategy int) error {
	var tx int
	var err error

	err = wallet.WithCoinSelectLock(
		func() error {
			// Intentionally already multiline, but the first
			// continuation line overflows due to
			// coinSelectionStrategy.
			tx, err = wallet.CreateSimpleTx(
				nil, outputs, feePerKw, minConfs, coinSelectionStrategy,
				true,
			)
			_ = tx
			return err
		},
	)

	return err
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          64,
		TabStop:              8,
		UseDSLMultiLineCalls: true,
		// Keep other DSL stages off to make this test focused.
		UseDSLLogCalls:   false,
		UseDSLExpr:       false,
		UseDSLComments:   false,
		UseDSLFuncSigs:   false,
		UseDSLBlankLines: false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(
		t, out, "tx, err = wallet.CreateSimpleTx(\n",
		"expected multiline call formatting to run",
	)
	require.NotContains(
		t, out, "feePerKw, minConfs, coinSelectionStrategy,", "expect"+
			"ed the formatter to break before the long "+
			"`coinSelectionStrategy` argument when it overflows "+
			"a continuation line",
	)
}

func TestPipelineNext_CallArgs_RePacksWhenCollapsedWidthFitsButContinuationLineOverflows(
	t *testing.T) {

	// Subtle case:
	// - call is already multiline
	// - collapsed single-line estimation fits under the limit
	// - but a specific continuation line still exceeds the limit due to
	//   indent.
	const in = `package p

type wT struct{}

func (wT) C(a any, b any, feePerKw, minConfs int, strategy int, coinSelectionStrategy int) {}

func f(w wT, outputs []int, feePerKw, minConfs int, strategy int, coinSelectionStrategy int) {
	w.C(
		nil, outputs, feePerKw, minConfs, strategy, coinSelectionStrategy,
	)
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          80,
		TabStop:              8,
		UseDSLMultiLineCalls: true,
		// Keep other DSL stages off to make this test focused.
		UseDSLLogCalls:   false,
		UseDSLExpr:       false,
		UseDSLComments:   false,
		UseDSLFuncSigs:   false,
		UseDSLBlankLines: false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(
		t, out, "w.C(\n", "expected multiline call formatting to run",
	)
	require.Contains(
		t, out, "\n		coinSelectionStrategy,", "expected "+
			"the formatter to re-pack the call and break before "+
			"the long `coinSelectionStrategy` argument",
	)
}
