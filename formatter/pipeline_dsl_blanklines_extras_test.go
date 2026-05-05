package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDSLBlankLinesNative_ExtraIfErrReturnOptIn_BlanksBeforeIfErrReturn(
	t *testing.T) {

	const in = `package p

func f(err error) error {
	doSomething()
	// keep comment attached to if
	if err != nil {
		return err
	}
	return nil
}

func doSomething() {}
`

	p := NewPipeline(
		PipelineConfig{
			ColumnLimit:                   80,
			TabStop:                       8,
			UseDSLBlankLines:              true,
			UseDSLBlankLinesNative:        true,
			DSLBlankLinesExtraIfErrReturn: true,
		},
	)

	first := p.Format([]byte(in))
	second := p.Format(first)
	require.Equal(t, string(first), string(second))

	out := string(first)
	require.Contains(
		t, out, "doSomething()\n\n	// keep comment attached "+
			"to if\n	if err != nil",
	)
	require.NotContains(
		t, out, "// keep comment attached to if\n\n	if err != nil",
	)

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", first, parser.AllErrors)
	require.NoError(t, err)
}

func TestDSLBlankLinesNative_NextProfile_DoesNotBlankBeforeIfErrReturnByDefault(
	t *testing.T) {

	const in = `package p

func f(err error) error {
	doSomething()
	if err != nil {
		return err
	}
	return nil
}

func doSomething() {}
`

	p := NewPipeline(
		PipelineConfig{
			ColumnLimit:            80,
			TabStop:                8,
			UseDSLBlankLines:       true,
			UseDSLBlankLinesNative: true,
		},
	)

	first := p.Format([]byte(in))
	second := p.Format(first)
	require.Equal(t, string(first), string(second))

	out := string(first)
	require.NotContains(t, out, "doSomething()\n\n\tif err != nil")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", first, parser.AllErrors)
	require.NoError(t, err)
}

func TestDSLBlankLinesNative_RemovesBlankAfterMultilineIfWithSingleReturn(
	t *testing.T) {

	const in = `package p

func f(first, second, third bool) int {
	if first &&
		second &&
		third {

		return 1
	}

	return 0
}
`

	p := NewPipeline(
		PipelineConfig{
			ColumnLimit:            80,
			TabStop:                8,
			UseDSLBlankLines:       true,
			UseDSLBlankLinesNative: true,
		},
	)

	first := p.Format([]byte(in))
	second := p.Format(first)
	require.Equal(t, string(first), string(second))

	out := string(first)
	require.Contains(t, out, "third {\n\t\treturn 1")
	require.NotContains(t, out, "third {\n\n\t\treturn 1")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", first, parser.AllErrors)
	require.NoError(t, err)
}

func TestDSLBlankLinesNative_BlankAfterMultilineIfWithMultipleStatements(
	t *testing.T) {

	const in = `package p

func f(first, second, third bool) int {
	if first &&
		second &&
		third {
		record()
		return 1
	}

	return 0
}

func record() {}
`

	p := NewPipeline(
		PipelineConfig{
			ColumnLimit:            80,
			TabStop:                8,
			UseDSLBlankLines:       true,
			UseDSLBlankLinesNative: true,
		},
	)

	first := p.Format([]byte(in))
	second := p.Format(first)
	require.Equal(t, string(first), string(second))

	out := string(first)
	require.Contains(t, out, "third {\n\n\t\trecord()")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", first, parser.AllErrors)
	require.NoError(t, err)
}

func TestDSLBlankLinesNative_DoesNotBlankBeforeOnlyWrappedReturn(t *testing.T) {
	const in = `package p

import "fmt"

func f(i int, childIdx uint32) error {
	if childIdx <= uint32(i) {
		return fmt.Errorf("node[%d] child index "+
			"%d must be > parent index (cycle or "+
			"back-reference)", i, childIdx)
	}

	return nil
}
`

	p := NewPipeline(
		PipelineConfig{
			ColumnLimit:            80,
			TabStop:                8,
			UseDSLLogCalls:         true,
			UseDSLBlankLines:       true,
			UseDSLBlankLinesNative: true,
		},
	)

	first := p.Format([]byte(in))
	second := p.Format(first)
	require.Equal(t, string(first), string(second))

	out := string(first)
	require.Contains(
		t, out,
		"if childIdx <= uint32(i) {\n		return fmt.Errorf(",
	)
	require.NotContains(
		t, out,
		"if childIdx <= uint32(i) {\n\n		return fmt.Errorf(",
	)

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", first, parser.AllErrors)
	require.NoError(t, err)
}

// Note: legacy/parity profiles were removed; llformat is next-only.
