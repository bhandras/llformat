package formatter

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

	spans := ownedSpansFromSource(src, ownedSpanOptions{
		IncludeCallArgLists: true,
	})

	offOuterIdent := strings.Index(string(src), "outer")
	require.Greater(t, offOuterIdent, 0)
	require.False(t, spans.containsOffset(offOuterIdent))

	offArgs := strings.Index(string(src), "a && b")
	require.Greater(t, offArgs, 0)
	require.True(t, spans.containsOffset(offArgs))
}

func TestOwnedSpansFromSource_CompositeBodies(t *testing.T) {
	src := []byte(`package p

func f() {
	_ = []int{1, 2, 3}
}
`)

	spans := ownedSpansFromSource(src, ownedSpanOptions{
		IncludeCompositeBodies: true,
	})

	offLit := strings.Index(string(src), "1, 2, 3")
	require.Greater(t, offLit, 0)
	require.True(t, spans.containsOffset(offLit))

	offAssign := strings.Index(string(src), "_ =")
	require.Greater(t, offAssign, 0)
	require.False(t, spans.containsOffset(offAssign))
}

func TestOwnedSpansFromSource_FuncBodies(t *testing.T) {
	src := []byte(`package p

func f() {
	_ = func() int { return 1 }()
}
`)

	spans := ownedSpansFromSource(src, ownedSpanOptions{
		IncludeFuncBodies: true,
	})

	offReturn := strings.Index(string(src), "return 1")
	require.Greater(t, offReturn, 0)
	require.True(t, spans.containsOffset(offReturn))

	offFuncKeyword := strings.Index(string(src), "func() int")
	require.Greater(t, offFuncKeyword, 0)
	require.False(t, spans.containsOffset(offFuncKeyword))
}
