package formatter

import (
	"fmt"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDSLSigsStyleDSLAutoIterationsFormatsManySignatures(t *testing.T) {
	// The signatures stage applies at most one rewrite per iteration, so a
	// file with >100 long signatures can legitimately require >100
	// iterations.
	//
	// This test ensures we don't cap out early when native signatures are
	// enabled with the pure DSL signature formatting style.
	var b strings.Builder
	b.WriteString("package p\n\n")

	const n = 150
	for i := 0; i < n; i++ {
		fmt.Fprintf(
			&b, "func f%03d(alpha int, beta int, gamma int, "+
				"delta int, epsilon int, zeta int) int {\n}\n\n",
			i,
		)
	}

	p := NewPipeline(
		PipelineConfig{
			ColumnLimit:          40,
			TabStop:              8,
			UseDSLFuncSigs:       true,
			UseDSLFuncSigsNative: true,
			DSLSigsStyle:         "dsl",
		},
	)

	first := p.Format([]byte(b.String()))
	second := p.Format(first)
	require.Equal(t, string(first), string(second))

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", first, parser.AllErrors)
	require.NoError(t, err)

	out := string(first)
	require.Contains(t, out, "func f149(\n")
}

func TestDSLSigsStyleDSLInterfaceMethodPreservesTrailingComment(t *testing.T) {
	const in = `package p

type I interface {
	VeryLongMethodName(alpha int, beta int, gamma int, delta int) (int, error) // keep me
}
`

	p := NewPipeline(
		PipelineConfig{
			ColumnLimit:          45,
			TabStop:              8,
			UseDSLFuncSigs:       true,
			UseDSLFuncSigsNative: true,
			DSLSigsStyle:         "dsl",
		},
	)

	first := p.Format([]byte(in))
	second := p.Format(first)
	require.Equal(t, string(first), string(second))

	out := string(first)
	require.Contains(t, out, "// keep me")
	require.Contains(t, out, "VeryLongMethodName(\n")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", first, parser.AllErrors)
	require.NoError(t, err)
}

func TestDSLSigsStyleDSLSignatureSplitIgnoresCommasInBlockComments(
	t *testing.T) {

	// Regression: comma-aware splitting must not treat commas inside
	// comments as parameter separators.
	const in = `package p

func f(alpha int /* comma, inside */, beta int, gamma int, delta int, epsilon int) {
}
`

	p := NewPipeline(
		PipelineConfig{
			ColumnLimit:          45,
			TabStop:              8,
			UseDSLFuncSigs:       true,
			UseDSLFuncSigsNative: true,
			DSLSigsStyle:         "dsl",
		},
	)

	first := p.Format([]byte(in))
	second := p.Format(first)
	require.Equal(t, string(first), string(second))

	out := string(first)
	require.Contains(t, out, "/* comma, inside */")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", first, parser.AllErrors)
	require.NoError(t, err)
}
