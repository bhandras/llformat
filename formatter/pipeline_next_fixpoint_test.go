package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_ConvergesWithinSingleRun_WhenEnabled(t *testing.T) {
	// The pipeline's default behavior is a single pass (to preserve golden
	// fixtures). When explicitly requested, it should be able to iterate to a
	// stable fixpoint so users can avoid running llformat multiple times.
	in := []byte(`package p

import "fmt"

func f(wallet interface{ WithCoinSelectLock(func() error, ...interface{}) error }) error {
	var (
		tx   interface{}
		err  error
		outs interface{}
	)
	coinSelectionStrategy := "coinSelectionStrategy"
	feePerKw := 123
	minConfs := 1

	err = wallet.WithCoinSelectLock(
		func() error {
			tx, err = fmt.Sprintf("%v%v%v%v%v%v", tx, outs, feePerKw, minConfs, coinSelectionStrategy, true)
			_ = tx
			return err
		},
	)
	return err
}
`)

	p := NewPipeline(PipelineConfig{
		Mode:                 "next",
		RuleProfile:          "next",
		ColumnLimit:          60,
		TabStop:              8,
		UseOwnershipRegistry: true,
		MaxPipelineIterations: 3,
	})

	out1 := p.Format(in)
	out2 := p.Format(out1)

	require.Equal(t, string(out1), string(out2))
}
