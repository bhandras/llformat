package dsl

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatCallAdaptive_PacksSimpleArgsButSplitsComplexArgs(t *testing.T) {
	const src = `package p

func f(a, b int) {
	_ = testFn(a, b)
	_ = testFn(a > 0, b)
}
`

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "in.go", []byte(src), parser.AllErrors)
	require.NoError(t, err)

	var simpleCall *ast.CallExpr
	var complexCall *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if simpleCall == nil {
			simpleCall = call
			return true
		}
		if complexCall == nil {
			complexCall = call
			return true
		}
		return true
	})
	require.NotNil(t, simpleCall)
	require.NotNil(t, complexCall)

	ctx := NewContext(fset, []byte(src), 10, 8) // force wrapping
	indent := ctx.IndentAt(simpleCall)
	require.Equal(t, "\t", indent)

	simpleOut := formatCallAdaptive(simpleCall, indent, ctx)
	require.Contains(t, simpleOut, "testFn(\n")
	// With a sufficiently wide column limit, the simple args should pack on the
	// same continuation line.
	ctxWide := NewContext(fset, []byte(src), 80, 8)
	simpleOut = formatCallAdaptive(simpleCall, indent, ctxWide)
	require.Contains(t, simpleOut, "\t\ta, b,\n\t)")

	complexOut := formatCallAdaptive(complexCall, indent, ctx)
	require.Contains(t, complexOut, "testFn(\n")
	require.Contains(t, complexOut, "\t\ta > 0,\n")
	require.Contains(t, complexOut, "\t\tb,\n\t)")
}
