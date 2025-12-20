package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_CallArgs_PacksArgsWithoutOverflow_InNestedClosure(t *testing.T) {
	// Regression for cases where the packed multiline call formatter keeps too
	// many arguments on one continuation line, causing an overflow at the column
	// limit. Prefer breaking before a long argument like `coinSelectionStrategy`
	// rather than letting it spill past the limit.
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
		RuleProfile:          "next",
		UseDSLMultiLineCalls: true,
		// Keep other DSL stages off to make this test focused.
		UseDSLLogCalls:   false,
		UseDSLExpr:       false,
		UseDSLComments:   false,
		UseDSLFuncSigs:   false,
		UseDSLBlankLines: false,
	})

	out := string(p.Format([]byte(in)))

	// Must rewrite the long call into packed multiline.
	require.Contains(t, out, "tx, err = wallet.CreateSimpleTx(\n", "expected multiline call formatting to kick in")
	// Must not detach the call from `tx, err =`.
	require.NotContains(t, out, "tx, err =\n\t\twallet.CreateSimpleTx(",
		"must not break before the call for a multi-assign prefix; keep the call attached")
	// Ensure we don't keep the long arg on the same line as minConfs when it would overflow.
	require.NotContains(t, out, "feePerKw, minConfs, coinSelectionStrategy,",
		"expected the formatter to break before the long `coinSelectionStrategy` argument to avoid overflow")
	// The long arg should appear at the start of a continuation line.
	require.Contains(t, out, "\t\tcoinSelectionStrategy, true,\n",
		"expected the long argument to be moved to the next packed continuation line")
}

