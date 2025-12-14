package dsl

import (
	"go/format"
	"testing"

	"github.com/stretchr/testify/require"
)

func gofmtBytes(t *testing.T, src []byte) []byte {
	t.Helper()
	out, err := format.Source(src)
	require.NoError(t, err)
	return out
}

func TestExprStage_StringConcatReflow(t *testing.T) {
	src := []byte(`package p

func f() string {
	return "This is a very long string that " + "spans multiple parts and is " + "concatenated together"
}
`)

	engine := NewEngine(LongExprRules())
	engine.ColumnLimit = 60
	out, err := engine.Format(src)
	require.NoError(t, err)

	// Ensure we actually introduced a line break in the concatenation.
	got := string(gofmtBytes(t, out))
	require.Contains(t, got, "return \"This is a very long string that")
	require.Contains(t, got, "+\n")
}

func TestExprStage_StringConcatSkipsRawString(t *testing.T) {
	src := []byte("package p\n\nfunc f() string {\n\treturn `this is a raw string literal that should not be rewritten even if long`\n}\n")

	engine := NewEngine(LongExprRules())
	engine.ColumnLimit = 20
	out, err := engine.Format(src)
	require.NoError(t, err)

	require.Equal(t, string(src), string(out))
}

func TestExprStage_SkipsInsideCompositeLiteral(t *testing.T) {
	src := []byte(`package p

type S struct {
	Field string
}

func f() {
	_ = S{
		Field: "This is a very long string that " + "spans multiple parts and is " + "concatenated together",
	}
}
`)

	engine := NewEngine(LongExprRules())
	engine.ColumnLimit = 60
	out, err := engine.Format(src)
	require.NoError(t, err)

	// Expression stage should not edit inside composite literals.
	require.Equal(t, string(src), string(out))
}

func TestExprStage_SkipsInsideCallArgs(t *testing.T) {
	src := []byte(`package p

func f(alpha, beta, gamma, delta bool) {
	_ = foo(alpha && beta && gamma && delta)
}

func foo(bool) {}
`)

	engine := NewEngine(LongExprRules())
	engine.ColumnLimit = 20
	out, err := engine.Format(src)
	require.NoError(t, err)

	// Expression stage should not break logical chains inside call arguments.
	require.NotContains(t, string(out), "&&\n")
	require.Equal(t, string(src), string(out))
}
