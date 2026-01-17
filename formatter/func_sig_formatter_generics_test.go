package formatter

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/bhandras/llformat/width"
	"github.com/stretchr/testify/require"
)

func TestFormatFuncSigs_GenericReturnListDoesNotOverflow(t *testing.T) {
	const src = `package p

type RecvXXXXXX[T1, T2 any] struct{}
type RetXXXXXXXXXXXXXXXXXXXXXXXXXXX[T1, T2 any] interface{}
type P interface{}

func (s *RecvXXXXXX[T1, T2]) Select(p P) (RetXXXXXXXXXXXXXXXXXXXXXXXXXXX[T1, T2], error) {
	return nil, nil
}
`

	out, changed := FormatFuncSigsInSource([]byte(src), 80, 8)
	if !changed {
		out = []byte(src)
	}

	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		require.LessOrEqual(
			t, width.VisualLenWithTab(line, 8), 80, "line "+
				"exceeds configured column limit: %q", line,
		)
	}
}

func TestBreakLongTypeArgListsIfNeeded_BreaksReceiver(t *testing.T) {
	const sig = "func (r *ReceiverXXXXXXXXXXXXXXXXXXXXXXXXXXXX[T1, T2, " +
		"T3]) VeryLongMethodNameForSig() " +
		"ReturnXXXXXXXXXXXXXXXXXXXXXXXXXXXX[T1, T2, T3] {"

	out := breakLongTypeArgListsIfNeeded(sig, 80, 8)

	require.Contains(
		t, out, "ReceiverXXXXXXXXXXXXXXXXXXXXXXXXXXXX["+
			"\n	T1,\n	T2,\n	T3,\n]",
	)
}

func TestFormatFuncSignatureNext_BreaksGenericTypeArgs(t *testing.T) {
	const sig = "func (r *ReceiverXXXXXXXXXXXXXXXXXXXXXXXXXXXX[T1, T2, " +
		"T3]) VeryLongMethodNameForSig() " +
		"ReturnXXXXXXXXXXXXXXXXXXXXXXXXXXXX[T1, T2, T3] {"

	out, _ := FormatFuncSignatureNext(sig, "", 80, 8)

	require.Contains(
		t, out, "ReceiverXXXXXXXXXXXXXXXXXXXXXXXXXXXX["+
			"\n	T1,\n	T2,\n	T3,\n]",
	)
}

func TestFormatFuncSignatureNext_WithASTSignature(t *testing.T) {
	const src = `package p

type ReceiverXXXXXXXXXXXXXXXXXXXXXXXXXXXX[T1, T2, T3 any] struct{}
type ReturnXXXXXXXXXXXXXXXXXXXXXXXXXXXX[T1, T2, T3 any] struct{}

func (r *ReceiverXXXXXXXXXXXXXXXXXXXXXXXXXXXX[T1, T2, T3]) VeryLongMethodNameForSig() ReturnXXXXXXXXXXXXXXXXXXXXXXXXXXXX[T1, T2, T3] {
	return ReturnXXXXXXXXXXXXXXXXXXXXXXXXXXXX[T1, T2, T3]{}
}
`

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "in.go", src, parser.AllErrors)
	require.NoError(t, err)
	require.NotEmpty(t, file.Decls)

	decl, ok := file.Decls[len(file.Decls)-1].(*ast.FuncDecl)
	require.True(t, ok)
	funcStart := fset.Position(decl.Pos()).Offset
	bracePos := fset.Position(decl.Body.Lbrace).Offset

	signature := strings.TrimSpace(src[funcStart : bracePos+1])
	out, _ := FormatFuncSignatureNext(signature, "", 80, 8)

	require.Contains(
		t, out, "ReceiverXXXXXXXXXXXXXXXXXXXXXXXXXXXX["+
			"\n	T1,\n	T2,\n	T3,\n]",
	)
}
