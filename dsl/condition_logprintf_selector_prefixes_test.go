package dsl

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsLogOrPrintfCallCond_SelectorPrefixes_RestrictsWhenMatchAnyEnabled(
	t *testing.T) {

	fset := token.NewFileSet()

	expr, err := parser.ParseExprFrom(
		fset, "snippet.go",
		`rpcSLog.Errorf("a very long message that should wrap at some point: %v", x)`,
		0,
	)
	require.NoError(t, err)

	call, ok := expr.(*ast.CallExpr)
	require.True(t, ok)

	condAllowed := &IsLogOrPrintfCallCond{
		Target:                 "node",
		MatchAnySelectorPrefix: true,
		SelectorPrefixes: []string{
			"rpcSLog",
		},
	}
	require.True(t, condAllowed.Eval(Captures{"node": call}, &Context{}))

	condDenied := &IsLogOrPrintfCallCond{
		Target:                 "node",
		MatchAnySelectorPrefix: true,
		SelectorPrefixes: []string{
			"otherLog",
		},
	}
	require.False(t, condDenied.Eval(Captures{"node": call}, &Context{}))
}

func TestIsLogOrPrintfCallCond_SelectorPrefixes_ExpandWhenMatchAnyDisabled(
	t *testing.T) {

	fset := token.NewFileSet()

	expr, err := parser.ParseExprFrom(
		fset, "snippet.go",
		`rpcSLog.Errorf("a very long message that should wrap at some point: %v", x)`,
		0,
	)
	require.NoError(t, err)

	call, ok := expr.(*ast.CallExpr)
	require.True(t, ok)

	cond := &IsLogOrPrintfCallCond{
		Target:                 "node",
		MatchAnySelectorPrefix: false,
		SelectorPrefixes: []string{
			"rpcSLog",
		},
	}
	require.True(t, cond.Eval(Captures{"node": call}, &Context{}))
}

func TestIsLogOrPrintfCallCond_SelectorPrefixes_DoNotBlockCanonicalMatches(
	t *testing.T) {

	fset := token.NewFileSet()

	expr, err := parser.ParseExprFrom(
		fset, "snippet.go",
		`fmt.Errorf("a very long message that should wrap at some point: %v", x)`,
		0,
	)
	require.NoError(t, err)

	call, ok := expr.(*ast.CallExpr)
	require.True(t, ok)

	cond := &IsLogOrPrintfCallCond{
		Target:                 "node",
		MatchAnySelectorPrefix: true,
		SelectorPrefixes: []string{
			"rpcSLog",
		},
	}
	require.True(t, cond.Eval(Captures{"node": call}, &Context{}))
}

func TestIsLogOrPrintfCallCond_SelectorNamesOverride(t *testing.T) {
	fset := token.NewFileSet()

	expr, err := parser.ParseExprFrom(
		fset, "snippet.go",
		`rpcSLog.Noticef("a very long message that should wrap at some point: %v", x)`,
		0,
	)
	require.NoError(t, err)

	call, ok := expr.(*ast.CallExpr)
	require.True(t, ok)

	cond := &IsLogOrPrintfCallCond{
		Target:                 "node",
		MatchAnySelectorPrefix: true,
		SelectorNames: []string{
			"Noticef",
		},
	}
	require.True(t, cond.Eval(Captures{"node": call}, &Context{}))

	expr2, err := parser.ParseExprFrom(
		fset, "snippet2.go",
		`rpcSLog.Infof("a very long message that should wrap at some point: %v", x)`,
		0,
	)
	require.NoError(t, err)

	call2, ok := expr2.(*ast.CallExpr)
	require.True(t, ok)
	require.False(t, cond.Eval(Captures{"node": call2}, &Context{}))
}
