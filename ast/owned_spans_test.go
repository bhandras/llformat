package ast

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOwnedSpansFromSource_CallArgLists(t *testing.T) {
	src := []byte(`package p

func f() {
	_ = outer(inner(a && b), 42)
}
`)

	spans := OwnedSpansFromSource(
		src, OwnedSpanOptions{
			IncludeCallArgLists: true,
		},
	)

	offOuterIdent := strings.Index(string(src), "outer")
	require.Greater(t, offOuterIdent, 0)
	require.False(t, spans.Contains(offOuterIdent))

	offArgs := strings.Index(string(src), "a && b")
	require.Greater(t, offArgs, 0)
	require.True(t, spans.Contains(offArgs))
}

func TestOwnedSpansFromSource_CallExprs(t *testing.T) {
	src := []byte(`package p

func f() {
	_ = (a + b)(c)
}
`)

	spans := OwnedSpansFromSource(
		src, OwnedSpanOptions{
			IncludeCallExprs: true,
		},
	)

	offCalleeOp := strings.Index(string(src), "a + b")
	require.Greater(t, offCalleeOp, 0)
	require.True(t, spans.Contains(offCalleeOp))

	offArgs := strings.Index(string(src), "(c)")
	require.Greater(t, offArgs, 0)
	require.True(t, spans.Contains(offArgs))

	offAssign := strings.Index(string(src), "_ =")
	require.Greater(t, offAssign, 0)
	require.False(t, spans.Contains(offAssign))
}

func TestOwnedSpansFromSource_CompositeBodies(t *testing.T) {
	src := []byte(`package p

func f() {
	_ = []int{1, 2, 3}
}
`)

	spans := OwnedSpansFromSource(
		src, OwnedSpanOptions{
			IncludeCompositeBodies: true,
		},
	)

	offLit := strings.Index(string(src), "1, 2, 3")
	require.Greater(t, offLit, 0)
	require.True(t, spans.Contains(offLit))

	offAssign := strings.Index(string(src), "_ =")
	require.Greater(t, offAssign, 0)
	require.False(t, spans.Contains(offAssign))
}

func TestOwnedSpansFromSource_FuncBodies(t *testing.T) {
	src := []byte(`package p

func f() {
	_ = func() int { return 1 }()
}
`)

	spans := OwnedSpansFromSource(
		src, OwnedSpanOptions{
			IncludeFuncBodies: true,
		},
	)

	offReturn := strings.Index(string(src), "return 1")
	require.Greater(t, offReturn, 0)
	require.True(t, spans.Contains(offReturn))

	offFuncKeyword := strings.Index(string(src), "func() int")
	require.Greater(t, offFuncKeyword, 0)
	require.False(t, spans.Contains(offFuncKeyword))
}
