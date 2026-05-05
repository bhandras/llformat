package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_ConvergesWithinSingleRun_WhenEnabled(t *testing.T) {
	// NewPipeline defaults to a single pass; when explicitly enabled, it
	// should be able to converge to a stable fixpoint within a single run.
	in := []byte(
		`package p

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
`,
	)

	p := NewPipeline(
		PipelineConfig{
			ColumnLimit:           60,
			TabStop:               8,
			UseOwnershipRegistry:  true,
			MaxPipelineIterations: 3,
		},
	)

	out1 := p.Format(in)
	out2 := p.Format(out1)

	require.Equal(t, string(out1), string(out2))
}

func TestPipelineNext_CycleDetectionReturnsStableMember(t *testing.T) {
	const in = `package p

import "fmt"

type itemStats struct {
	Name string
	Status string
	LiveBalance int
	Reserved int
	Available int
	LiveOutputs int
}

type selectionStats struct {
	Items []itemStats
}

func (s selectionStats) scanSummary(limit int) string {
	parts := make([]string, 0, len(s.Items))
	for _, item := range s.Items {
		switch item.Status {
		case "idle":
			parts = append(parts, item.Name)

		default:
			parts = append(
				parts,
				fmt.Sprintf("%s:%s/live=%d/reserved=%d"+
					"/available=%d/outputs=%d", item.Name,
					item.Status, item.LiveBalance,
					item.Reserved, item.Available,
					item.LiveOutputs),
			)
		}
	}
	return fmt.Sprint(parts)
}
`

	p := NewPipeline(
		PipelineConfig{
			ColumnLimit:           80,
			TabStop:               8,
			UseOwnershipRegistry:  true,
			MaxPipelineIterations: 3,
		},
	)

	out1 := p.Format([]byte(in))
	out2 := p.Format(out1)

	require.Equal(t, string(out1), string(out2))
}
