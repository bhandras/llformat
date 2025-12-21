package dsl

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func ctxWithParents(t *testing.T, src string) (*Context, *ast.File) {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	require.NoError(t, err)

	ctx := NewContext(fset, []byte(src), 80, 8)

	parentMap := make(map[ast.Node]ast.Node)
	var stack []ast.Node
	ast.Inspect(
		file,
		func(n ast.Node) bool {
			if n != nil {
				if len(stack) > 0 {
					parentMap[n] = stack[len(stack)-1]
				}
				stack = append(stack, n)
			} else if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}

			return true
		},
	)
	ctx.SetParentMap(parentMap)

	return ctx, file
}

func TestIsSimpleLiteralCond(t *testing.T) {
	tests := []struct {
		expr   string
		wantOK bool
	}{
		{
			"0",
			true,
		},
		{
			"42",
			true,
		},
		{
			"3.14",
			true,
		},
		{
			"true",
			true,
		},
		{
			"false",
			true,
		},
		{
			"nil",
			true,
		},
		{
			"-1",
			true,
		},
		{
			"x",
			false,
		},
		{
			"foo()",
			false,
		},
		{
			"a + b",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.expr,
			func(t *testing.T) {
				expr, err := parser.ParseExpr(tt.expr)
				require.NoError(t, err)

				caps := Captures{"r": expr}
				cond := &IsSimpleLiteralCond{Target: "r"}
				ctx := &Context{}

				got := cond.Eval(caps, ctx)
				require.Equal(t, tt.wantOK, got)
			},
		)
	}
}

func TestHasCallExprCond(t *testing.T) {
	fset := token.NewFileSet()

	tests := []struct {
		expr   string
		wantOK bool
	}{
		{
			"foo()",
			true,
		},
		{
			"x.Method()",
			true,
		},
		{
			"len(s) > 0",
			true,
		},
		{
			"a && b && c",
			false,
		},
		{
			"x + y",
			false,
		},
		{
			"42",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.expr,
			func(t *testing.T) {
				expr, err := parser.ParseExpr(tt.expr)
				require.NoError(t, err)

				caps := Captures{"node": expr}
				cond := &HasCallExprCond{Target: "node"}
				ctx := &Context{Fset: fset}

				got := cond.Eval(caps, ctx)
				require.Equal(t, tt.wantOK, got)
			},
		)
	}
}

func TestOpIsCond(t *testing.T) {
	fset := token.NewFileSet()

	tests := []struct {
		expr   string
		ops    []string
		wantOK bool
	}{
		{
			"a && b",
			[]string{
				"&&",
				"||",
			},
			true,
		},
		{
			"a || b",
			[]string{
				"&&",
				"||",
			},
			true,
		},
		{
			"a + b",
			[]string{
				"&&",
				"||",
			},
			false,
		},
		{
			"a > b",
			ComparisonOps(),
			true,
		},
		{
			"a == b",
			ComparisonOps(),
			true,
		},
		{
			"a + b",
			ComparisonOps(),
			false,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.expr,
			func(t *testing.T) {
				expr, err := parser.ParseExpr(tt.expr)
				require.NoError(t, err)

				caps := Captures{"node": expr}
				cond := &OpIsCond{Target: "node", Operators: tt.ops}
				ctx := &Context{Fset: fset}

				got := cond.Eval(caps, ctx)
				require.Equal(t, tt.wantOK, got)
			},
		)
	}
}

func TestAndCond(t *testing.T) {
	fset := token.NewFileSet()

	expr, _ := parser.ParseExpr("len(s) > 0")
	caps := Captures{"node": expr}
	ctx := &Context{Fset: fset}

	// Both true
	cond := &AndCond{
		Conds: []Condition{
			&HasCallExprCond{
				Target: "node",
			},
			TrueCond{},
		},
	}
	require.True(t, cond.Eval(caps, ctx))

	// One false
	cond = &AndCond{
		Conds: []Condition{
			TrueCond{},
			FalseCond{},
		},
	}
	require.False(t, cond.Eval(caps, ctx))
}

func TestOrCond(t *testing.T) {
	fset := token.NewFileSet()

	expr, _ := parser.ParseExpr("a + b")
	caps := Captures{"node": expr}
	ctx := &Context{Fset: fset}

	// One true
	cond := &OrCond{
		Conds: []Condition{
			&HasCallExprCond{
				Target: "node",
			}, // false
			TrueCond{},
		},
	}
	require.True(t, cond.Eval(caps, ctx))

	// Both false
	cond = &OrCond{
		Conds: []Condition{
			FalseCond{},
			FalseCond{},
		},
	}
	require.False(t, cond.Eval(caps, ctx))
}

func TestNotCond(t *testing.T) {
	ctx := &Context{}
	caps := Captures{}

	require.True(t, (&NotCond{Cond: FalseCond{}}).Eval(caps, ctx))
	require.False(t, (&NotCond{Cond: TrueCond{}}).Eval(caps, ctx))
}

func TestParentAndScopeConds(t *testing.T) {
	src := `package p

func f() {
	x := foo(bar(1))
	_ = x
	return foo(2)
}
`
	ctx, file := ctxWithParents(t, src)

	var assign *ast.AssignStmt
	var ret *ast.ReturnStmt
	ast.Inspect(
		file,
		func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.AssignStmt:
				if assign == nil && len(node.Rhs) > 0 {
					if _, ok := node.Rhs[0].(*ast.CallExpr); ok {
						assign = node
					}
				}

			case *ast.ReturnStmt:
				ret = node
			}

			return true
		},
	)
	require.NotNil(t, assign)
	require.NotNil(t, ret)
	require.NotEmpty(t, assign.Rhs)
	require.NotEmpty(t, assign.Lhs)
	require.NotEmpty(t, ret.Results)

	rhsCall, ok := assign.Rhs[0].(*ast.CallExpr)
	require.True(t, ok)
	lhsIdent, ok := assign.Lhs[0].(*ast.Ident)
	require.True(t, ok)

	// IsParentTypeCond: ident "foo" is the Fun of rhsCall.
	funIdent, ok := rhsCall.Fun.(*ast.Ident)
	require.True(t, ok)
	require.True(
		t,
		(&IsParentTypeCond{Target: "node", Type: "CallExpr"}).Eval(Captures{"node": funIdent},
			ctx,
		),
	)
	require.False(
		t,
		(&IsParentTypeCond{Target: "node", Type: "ReturnStmt"}).Eval(Captures{"node": funIdent},
			ctx,
		),
	)

	// IsAncestorTypeCond: literal 1 is nested inside rhsCall as an argument
	// of bar(1).
	innerCall, ok := rhsCall.Args[0].(*ast.CallExpr)
	require.True(t, ok)
	lit, ok := innerCall.Args[0].(*ast.BasicLit)
	require.True(t, ok)
	require.True(
		t,
		(&IsAncestorTypeCond{Target: "node", Type: "CallExpr"}).Eval(Captures{"node": lit},
			ctx,
		),
	)

	// IsInAssignRHSCond: rhsCall is a direct RHS expression; lhsIdent is
	// not.
	require.True(
		t,
		(&IsInAssignRHSCond{Target: "node"}).Eval(Captures{"node": rhsCall},
			ctx,
		),
	)
	require.False(
		t,
		(&IsInAssignRHSCond{Target: "node"}).Eval(Captures{"node": lhsIdent},
			ctx,
		),
	)

	// IsInReturnResultsCond: return call is a direct result expression.
	retCall, ok := ret.Results[0].(*ast.CallExpr)
	require.True(t, ok)
	require.True(
		t,
		(&IsInReturnResultsCond{Target: "node"}).Eval(Captures{"node": retCall},
			ctx,
		),
	)
	require.False(
		t,
		(&IsInReturnResultsCond{Target: "node"}).Eval(Captures{"node": rhsCall},
			ctx,
		),
	)
}

func TestHasLineCommentCond(t *testing.T) {
	src := `package p

func f() {
	_ = foo(a,
		// comment
		b)
	_ = foo(a, b)
}
`
	ctx, file := ctxWithParents(t, src)

	var calls []*ast.CallExpr
	ast.Inspect(
		file,
		func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				calls = append(calls, call)
			}

			return true
		},
	)
	require.Len(t, calls, 2)

	cond := &HasLineCommentCond{Target: "node"}
	require.True(t, cond.Eval(Captures{"node": calls[0]}, ctx))
	require.False(t, cond.Eval(Captures{"node": calls[1]}, ctx))
}
