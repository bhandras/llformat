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

func TestDSLBlankLinesNative_SelectCasesSeparateClausesNotHeaders(
	t *testing.T) {

	const in = `package p

import "context"

func f(ctx context.Context, done chan struct{}) error {
	select {
	case <-done:
		// Work completed.
		return nil
	case <-ctx.Done():
		// Context cancelled before work finished.
		close(done)
		return ctx.Err()
	}
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
	require.Contains(
		t, out, "case <-done:\n		// Work "+
			"completed.\n		return nil\n\n	case "+
			"<-ctx.Done():",
	)
	require.Contains(
		t, out,
		"close(done)\n\n\t\treturn ctx.Err()\n\t}",
	)
	require.NotContains(t, out, "case <-done:\n\n		// Work "+
		"completed.")

	const commentOnlyCase = `package p

func f(done chan struct{}, cancel chan struct{}) {
	select {
	case <-done:
		// Good.
	case <-cancel:
		return
	}
}
`

	commentOnlyOut := string(p.Format([]byte(commentOnlyCase)))
	require.Contains(
		t, commentOnlyOut,
		"case <-done:\n		// Good.\n\n	case <-cancel:",
	)
	require.NotContains(t, commentOnlyOut, "case "+
		"<-done:\n\n		// Good.")

	const commentOnlyWithHeaderBlank = `package p

func f(done chan struct{}, cancel chan struct{}) {
	select {
	case <-done:

		// Good.

	case <-cancel:
		return
	}
}
`

	cleanedCommentOnlyOut := string(
		p.Format(
			[]byte(commentOnlyWithHeaderBlank),
		),
	)
	require.Contains(
		t, cleanedCommentOnlyOut,
		"case <-done:\n		// Good.\n\n	case <-cancel:",
	)
	require.NotContains(
		t, cleanedCommentOnlyOut,
		"case <-done:\n\n		// Good.",
	)

	const commentAndReturnWithHeaderBlank = `package p

func f(done chan struct{}, cancel chan struct{}) {
	select {
	case <-done:

		// Good.
		return
	case <-cancel:
		return
	}
}
`

	cleanedCommentAndReturnOut := string(
		p.Format(
			[]byte(commentAndReturnWithHeaderBlank),
		),
	)
	require.Contains(
		t, cleanedCommentAndReturnOut, "case <-done:\n		// "+
			"Good.\n		return\n\n	case <-cancel:",
	)
	require.NotContains(
		t, cleanedCommentAndReturnOut,
		"case <-done:\n\n		// Good.",
	)

	const emptyCases = `package p

func f(notify chan struct{}, timer <-chan struct{}, done chan struct{}) {
	select {
	case <-notify:
	case <-timer:
	case <-done:
		return
	}
}
`

	emptyCasesOut := string(p.Format([]byte(emptyCases)))
	require.Contains(
		t, emptyCasesOut,
		"case <-notify:\n	case <-timer:\n	case <-done:",
	)
	require.NotContains(t, emptyCasesOut, "case <-notify:\n\n")
	require.NotContains(t, emptyCasesOut, "case <-timer:\n\n")

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

func TestDSLBlankLinesNative_RemovesBlankAfterSingleLineIfWithWrappedFatalf(
	t *testing.T) {

	const in = `package p

import "testing"

func f(t *testing.T, got, want []int) {
	if len(got) != len(want) {

		t.Fatalf("input_indices length mismatch: got %d want %d",
			len(got), len(want))
	}
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
		t, out, "if len(got) != len(want) "+
			"{\n		t.Fatalf(\"input_indices length "+
			"mismatch: got %d want %d\",",
	)
	require.Contains(t, out, "\n\t\t\tlen(got), len(want))")
	require.NotContains(
		t, out, "if len(got) != len(want) {\n\n		t.Fatalf(",
	)
	require.NotContains(t, out, "t.Fatalf(\n")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", first, parser.AllErrors)
	require.NoError(t, err)
}

// Note: legacy/parity profiles were removed; llformat is next-only.
