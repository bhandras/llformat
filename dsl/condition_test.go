package dsl

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsSimpleLiteralCond(t *testing.T) {
	tests := []struct {
		expr   string
		wantOK bool
	}{
		{"0", true},
		{"42", true},
		{"3.14", true},
		{"true", true},
		{"false", true},
		{"nil", true},
		{"-1", true},
		{"x", false},
		{"foo()", false},
		{"a + b", false},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			expr, err := parser.ParseExpr(tt.expr)
			require.NoError(t, err)

			caps := Captures{"r": expr}
			cond := &IsSimpleLiteralCond{Target: "r"}
			ctx := &Context{}

			got := cond.Eval(caps, ctx)
			require.Equal(t, tt.wantOK, got)
		})
	}
}

func TestHasCallExprCond(t *testing.T) {
	fset := token.NewFileSet()

	tests := []struct {
		expr   string
		wantOK bool
	}{
		{"foo()", true},
		{"x.Method()", true},
		{"len(s) > 0", true},
		{"a && b && c", false},
		{"x + y", false},
		{"42", false},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			expr, err := parser.ParseExpr(tt.expr)
			require.NoError(t, err)

			caps := Captures{"node": expr}
			cond := &HasCallExprCond{Target: "node"}
			ctx := &Context{Fset: fset}

			got := cond.Eval(caps, ctx)
			require.Equal(t, tt.wantOK, got)
		})
	}
}

func TestOpIsCond(t *testing.T) {
	fset := token.NewFileSet()

	tests := []struct {
		expr   string
		ops    []string
		wantOK bool
	}{
		{"a && b", []string{"&&", "||"}, true},
		{"a || b", []string{"&&", "||"}, true},
		{"a + b", []string{"&&", "||"}, false},
		{"a > b", ComparisonOps(), true},
		{"a == b", ComparisonOps(), true},
		{"a + b", ComparisonOps(), false},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			expr, err := parser.ParseExpr(tt.expr)
			require.NoError(t, err)

			caps := Captures{"node": expr}
			cond := &OpIsCond{Target: "node", Operators: tt.ops}
			ctx := &Context{Fset: fset}

			got := cond.Eval(caps, ctx)
			require.Equal(t, tt.wantOK, got)
		})
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
			&HasCallExprCond{Target: "node"},
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
			&HasCallExprCond{Target: "node"}, // false
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
