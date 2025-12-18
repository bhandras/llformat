package dsl

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsLogOrPrintfCallCond_MatchAnySelectorPrefix(t *testing.T) {
	fset := token.NewFileSet()

	expr, err := parser.ParseExprFrom(fset, "snippet.go", `mylog.Infof("a very long message that should wrap at some point: %v", x)`, 0)
	require.NoError(t, err)

	call, ok := expr.(*ast.CallExpr)
	require.True(t, ok)

	condDefault := &IsLogOrPrintfCallCond{Target: "node"}
	require.False(t, condDefault.Eval(Captures{"node": call}, &Context{}))

	condAny := &IsLogOrPrintfCallCond{Target: "node", MatchAnySelectorPrefix: true}
	require.True(t, condAny.Eval(Captures{"node": call}, &Context{}))
}
