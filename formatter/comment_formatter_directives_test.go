package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCommentFormatterPreservesGoDirectives(t *testing.T) {
	in := []byte(`package p

//go:build linux && amd64
// +build linux,amd64

//go:generate go run ./tools/gen
//line other.go:123

func f() {}
`)

	f := NewCommentFormatter(CommentConfig{ColumnLimit: 30})
	out := string(f.FormatFile(in))

	require.Contains(t, out, "//go:build linux && amd64\n")
	require.Contains(t, out, "// +build linux,amd64\n")
	require.Contains(t, out, "//go:generate go run ./tools/gen\n")
	require.Contains(t, out, "//line other.go:123\n")
}

func TestCommentFormatterDoesNotHoistDirectiveInlineComments(t *testing.T) {
	in := []byte(`package p

func f() {
	x := 1 //nolint:deadcode
	_ = x
}
`)

	f := NewCommentFormatter(CommentConfig{
		ColumnLimit:     20,
		MoveInlineAbove: true,
	})
	out := string(f.FormatFile(in))

	// Ensure the directive comment stays trailing and is not hoisted above.
	require.Contains(t, out, "x := 1 //nolint:deadcode\n")
	require.NotContains(t, out, "// nolint:deadcode\n")
}
