package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineDSLAllowCallArgsBreaksLogicalChain(t *testing.T) {
	src := []byte(`package p

func f(alpha, beta, gamma, delta bool) {
	_ = foo(alpha && beta && gamma && delta)
}

func foo(bool) {}
`)

	p := NewPipeline(PipelineConfig{
		ColumnLimit:      20,
		TabStop:          8,
		UseDSLExpr:       true,
		AllowDSLCallArgs: true,
		MoveInlineAbove:  false,
		Excludes:         nil,
	})

	out := p.Format(src)
	require.Contains(t, string(out), "_ = foo(\n")
	require.Contains(t, string(out), "alpha &&\n")
}
