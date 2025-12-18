package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDSLBlankLinesNative_InsertsBlankAboveLeadingReturnComment(t *testing.T) {
	const in = `package p

func f(x int) int {
	x++
	// comment belongs to return
	return x
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:            80,
		TabStop:                8,
		UseDSLBlankLines:       true,
		UseDSLBlankLinesNative: true,
	})

	first := p.Format([]byte(in))
	second := p.Format(first)
	require.Equal(t, string(first), string(second))

	out := string(first)
	require.Contains(t, out, "x++\n\n\t// comment belongs to return\n\treturn x\n")
	require.NotContains(t, out, "// comment belongs to return\n\n\treturn x")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", first, parser.AllErrors)
	require.NoError(t, err)
}

func TestDSLBlankLinesNative_InsertsBlankAboveLeadingCaseComment(t *testing.T) {
	const in = `package p

import "fmt"

func f(x int) {
	switch x {
	case 1:
		fmt.Println("one")
		// comment belongs to next case
	case 2:
		fmt.Println("two")
	}
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:            80,
		TabStop:                8,
		UseDSLBlankLines:       true,
		UseDSLBlankLinesNative: true,
	})

	first := p.Format([]byte(in))
	second := p.Format(first)
	require.Equal(t, string(first), string(second))

	out := string(first)
	require.Contains(t, out, "fmt.Println(\"one\")\n\n\t\t// comment belongs to next case\n\tcase 2:\n")
	require.NotContains(t, out, "// comment belongs to next case\n\n\tcase 2:")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", first, parser.AllErrors)
	require.NoError(t, err)
}

func TestDSLBlankLinesNative_InsertsBlankAboveLeadingBlockComment(t *testing.T) {
	const in = `package p

func f(x int) int {
	x++
	/* comment belongs to return */
	return x
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:            80,
		TabStop:                8,
		UseDSLBlankLines:       true,
		UseDSLBlankLinesNative: true,
	})

	first := p.Format([]byte(in))
	second := p.Format(first)
	require.Equal(t, string(first), string(second))

	out := string(first)
	require.Contains(t, out, "x++\n\n\t/* comment belongs to return */\n\treturn x\n")
	require.NotContains(t, out, "/* comment belongs to return */\n\n\treturn x")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", first, parser.AllErrors)
	require.NoError(t, err)
}
