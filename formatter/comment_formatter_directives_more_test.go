package formatter

import (
	"testing"

	"github.com/bhandras/llformat/internal/compat"
	"github.com/stretchr/testify/require"
)

func TestCommentFormatterPreservesMoreDirectiveVariants(t *testing.T) {
	in := []byte(
		`package p

//go:build linux && amd64
// +build linux,amd64

//go:generate go run ./tools/gen -flag="a b c"
//go:embed testdata/*
//go:linkname localname runtime.fastrand
//go:noinline
//go:nosplit

//nolint:errcheck // trailing prose should still be preserved verbatim
// nolint:revive // space after // is still a directive-like comment
//lint:ignore U1000 because it is used by generated code
//staticcheck:ignore SA1019 keep for compatibility
//gosec:ignore G204 reason
//revive:disable:var-naming reason

func f() {}
`,
	)

	f := compat.NewCommentFormatter(compat.CommentConfig{ColumnLimit: 20})
	out := string(f.FormatFile(in))

	// Ensure directive lines are preserved verbatim (no
	// wrapping/normalization).
	require.Contains(t, out, "//go:build linux && amd64\n")
	require.Contains(t, out, "// +build linux,amd64\n")
	require.Contains(
		t, out, "//go:generate go run ./tools/gen -flag=\"a b c\"\n",
	)
	require.Contains(t, out, "//go:embed testdata/*\n")
	require.Contains(t, out, "//go:linkname localname runtime.fastrand\n")
	require.Contains(t, out, "//go:noinline\n")
	require.Contains(t, out, "//go:nosplit\n")

	require.Contains(
		t, out, "//nolint:errcheck // trailing prose should still "+
			"be preserved verbatim\n",
	)
	require.Contains(
		t, out, "// nolint:revive // space after // is still a "+
			"directive-like comment\n",
	)
	require.Contains(
		t, out,
		"//lint:ignore U1000 because it is used by generated code\n",
	)
	require.Contains(
		t, out, "//staticcheck:ignore SA1019 keep for compatibility\n",
	)
	require.Contains(t, out, "//gosec:ignore G204 reason\n")
	require.Contains(t, out, "//revive:disable:var-naming reason\n")
}

func TestCommentFormatterPreservesCGODirectiveBlockComments(t *testing.T) {
	in := []byte(
		`package p

/*
#cgo CFLAGS: -I./include
#include <stdint.h>
#include "my header with spaces.h"
*/
import "C"

func f() {}
`,
	)

	f := compat.NewCommentFormatter(compat.CommentConfig{ColumnLimit: 20})
	out := string(f.FormatFile(in))

	// The cgo directive block must be preserved exactly; wrapping or
	// re-indenting can break cgo parsing.
	require.Contains(
		t, out, "/*\n#cgo CFLAGS: -I./include\n#include "+
			"<stdint.h>\n#include \"my header with spaces.h\"\n*/\n",
	)
}
