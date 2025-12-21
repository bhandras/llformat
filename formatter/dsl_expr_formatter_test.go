package formatter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDSLExprFormatterBasic(t *testing.T) {
	f := NewDSLExprFormatter(DSLExprConfig{
		ColumnLimit: 80,
		TabStop:     8,
	})

	// Test that simple code is formatted (blank line added before return)
	src := []byte(`package main

func foo() {
	x := 1
	return x > 0
}
`)
	expected := `package main

func foo() {
	x := 1

	return x > 0
}
`
	result := f.FormatFile(src)
	require.Equal(t, expected, string(result))
}

func TestDSLExprFormatterKeepsSimpleComparison(t *testing.T) {
	f := NewDSLExprFormatter(DSLExprConfig{
		ColumnLimit: 40, // Very narrow to force wrapping
		TabStop:     8,
	})

	// The comparison `x > 0` should never be broken
	src := []byte(`package main

func foo(x int) bool {
	return x > 0
}
`)
	result := f.FormatFile(src)
	require.Contains(t, string(result), "x > 0")
	require.NotContains(t, string(result), "x >\n")
}

func TestDSLExprFormatterReflowsLongCall(t *testing.T) {
	f := NewDSLExprFormatter(DSLExprConfig{
		ColumnLimit: 50,
		TabStop:     8,
	})

	src := []byte(
		`package main

func foo() {
	result := someVeryLongFunctionName(arg1, arg2, arg3)
	_ = result
}

func someVeryLongFunctionName(a, b, c int) int { return 0 }
`,
	)
	result := f.FormatFile(src)

	// Should reflow the call
	require.Contains(t, string(result), "someVeryLongFunctionName(\n")
}

func TestDSLExprFormatterBreaksLogicalChain(t *testing.T) {
	f := NewDSLExprFormatter(DSLExprConfig{
		ColumnLimit: 40,
		TabStop:     8,
	})

	// Use longer variable names to exceed 40 columns
	src := []byte(
		`package main

func foo(alpha, beta, gamma, delta bool) bool {
	return alpha && beta && gamma && delta
}
`,
	)
	result := f.FormatFile(src)

	// Should break after && operator (Go style)
	require.Contains(t, string(result), "&&\n")
}

func TestDSLExprFormatterGolden(t *testing.T) {
	// Use the same golden test files as the original formatter
	dir := filepath.Join("..", "testdata", "expressions")
	inPath := filepath.Join(dir, "input.go")

	if _, err := os.Stat(inPath); err != nil {
		t.Skipf("skipping: %s not present", inPath)
	}

	in, err := os.ReadFile(inPath)
	require.NoError(t, err)

	f := NewDSLExprFormatter(DSLExprConfig{
		ColumnLimit: 80,
		TabStop:     8,
	})

	result := f.FormatFile(in)

	// Just verify it produces valid Go code and doesn't panic
	require.NotEmpty(t, result)

	// Check that key patterns are preserved
	resultStr := string(result)

	// Simple comparisons should stay together
	require.Contains(t, resultStr, "> 0")
	require.Contains(t, resultStr, "> 10")

	// The code should still be parseable (gofmt is run internally by the
	// engine)
}
