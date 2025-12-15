package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDSLSigsStyleDSLIsParseableAndIdempotent(t *testing.T) {
	const in = `package p

import "time"

// Intentionally long signature that legacy keeps fairly compact while the pure
// DSL fallback formatter may choose a different break layout.
func processInlineConfig(config struct{ Timeout time.Duration; MaxRetries int; EnableCache bool }, handler func(cfg struct{ Timeout time.Duration; MaxRetries int; EnableCache bool }) error) error {
	return nil
}
`

	dslStyle := NewPipeline(PipelineConfig{
		ColumnLimit:          80,
		TabStop:              8,
		UseDSLFuncSigs:       true,
		UseDSLFuncSigsNative: true,
		DSLSigsStyle:         "dsl",
	})
	first := dslStyle.Format([]byte(in))
	second := dslStyle.Format(first)
	require.Equal(t, string(first), string(second))

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", first, parser.AllErrors)
	require.NoError(t, err)
}

func TestDSLSigsStyleDSLFallbackExpandsInlineStructs(t *testing.T) {
	const in = `package p

import "time"

func processInlineConfig(config struct{ Timeout time.Duration; MaxRetries int; EnableCache bool }, handler func(cfg struct{ Timeout time.Duration; MaxRetries int; EnableCache bool }) error) error {
	return nil
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          80,
		TabStop:              8,
		UseDSLFuncSigs:       true,
		UseDSLFuncSigsNative: true,
		DSLSigsStyle:         "dsl",
	})
	out := string(p.Format([]byte(in)))

	// When the pure DSL signature formatter chooses a multiline signature, it
	// should expand inline struct type literals into a multiline form. This is a
	// deliberate, opt-in readability improvement.
	require.Contains(t, out, "config struct {\n")
	require.Contains(t, out, "\n\t\tTimeout")
	require.Contains(t, out, "\n\t\tMaxRetries")
	require.Contains(t, out, "\n\t\tEnableCache bool")
	require.NotContains(t, out, "struct{ Timeout")
}

func TestDSLSigsStyleDSLFallbackFormatsResultLists(t *testing.T) {
	const in = `package p

import "time"

func buildResult(a int) (out struct{ Timeout time.Duration; MaxRetries int; EnableCache bool }, meta struct{ ID string; Rev int }, err error) {
	return out, meta, nil
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          70,
		TabStop:              8,
		UseDSLFuncSigs:       true,
		UseDSLFuncSigsNative: true,
		DSLSigsStyle:         "dsl",
	})

	first := p.Format([]byte(in))
	second := p.Format(first)
	require.Equal(t, string(first), string(second))

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", first, parser.AllErrors)
	require.NoError(t, err)

	out := string(first)
	require.Contains(t, out, ") (\n")
	require.Contains(t, out, "out struct {\n")
	require.Contains(t, out, "meta struct {\n")
	require.Contains(t, out, "err error")
}
