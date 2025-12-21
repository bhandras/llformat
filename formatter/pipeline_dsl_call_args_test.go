package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineDSLAllowCallArgsBreaksLogicalChain(t *testing.T) {
	src := []byte(
		`package p

func f(alpha, beta, gamma, delta bool) {
	_ = foo(alpha && beta && gamma && delta)
}

func foo(bool) {}
`,
	)

	p := NewPipeline(
		PipelineConfig{
			ColumnLimit:      20,
			TabStop:          8,
			UseDSLExpr:       true,
			AllowDSLCallArgs: true,
			MoveInlineAbove:  false,
			Excludes:         nil,
		},
	)

	out := p.Format(src)
	require.Contains(t, string(out), "_ = foo(alpha &&\n")
}

func TestPipelineDSLAutoCallArgsOnlyForExcludedCallees(t *testing.T) {
	src := []byte(
		`package p

func f(a1, a2, a3, a4, b1, b2, b3, b4 bool) {
	_ = foo(a1 && a2 && a3 && a4)
	_ = bar(b1 && b2 && b3 && b4)
}

func foo(bool) {}
func bar(bool) {}
`,
	)

	p := NewPipeline(
		PipelineConfig{
			ColumnLimit:     20,
			TabStop:         8,
			UseDSLExpr:      true,
			AutoDSLCallArgs: true,
			Excludes:        []string{"foo"},
		},
	)

	out := p.Format(src)
	got := string(out)

	// foo is excluded from multiline formatting, so auto call-arg
	// formatting may safely break the logical chain inside its argument.
	require.Contains(t, got, "a1 &&\n")

	// bar is not excluded, so auto mode should not introduce breaks inside
	// its call arguments.
	require.NotContains(t, got, "b1 &&\n")
}

func TestPipelineDSLCallArgsIdempotent(t *testing.T) {
	src := []byte(
		`package p

func f(alpha, beta, gamma, delta bool) {
	_ = foo(alpha && beta && gamma && delta)
}

func foo(bool) {}
`,
	)

	p := NewPipeline(
		PipelineConfig{
			ColumnLimit:      20,
			TabStop:          8,
			UseDSLExpr:       true,
			AllowDSLCallArgs: true,
		},
	)

	out1 := p.Format(src)
	out2 := p.Format(out1)
	require.Equal(t, string(out1), string(out2))
}
