package dsl

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNodePatternMatchBinaryExpr(t *testing.T) {
	fset := token.NewFileSet()

	tests := []struct {
		name    string
		expr    string
		pattern *NodePattern
		wantOK  bool
		wantCap map[string]string // capture name -> expected source
	}{
		{
			name: "match comparison with literal",
			expr: "x > 0",
			pattern: &NodePattern{
				Type: "BinaryExpr",
				Fields: map[string]FieldMatch{
					"op": {
						Literal: ">",
					},
					"right": {
						Capture: "r",
					},
				},
			},
			wantOK: true,
			wantCap: map[string]string{
				"r": "0",
			},
		},
		{
			name: "match comparison with wrong op",
			expr: "x > 0",
			pattern: &NodePattern{
				Type: "BinaryExpr",
				Fields: map[string]FieldMatch{
					"op": {
						Literal: "<",
					},
				},
			},
			wantOK: false,
		},
		{
			name: "match logical with OneOf",
			expr: "a && b",
			pattern: &NodePattern{
				Type: "BinaryExpr",
				Fields: map[string]FieldMatch{
					"op": {
						OneOf: []string{
							"&&",
							"||",
						},
					},
					"left": {
						Capture: "l",
					},
				},
			},
			wantOK: true,
			wantCap: map[string]string{
				"l": "a",
			},
		},
		{
			name: "match nested call in binary",
			expr: "len(s) > 0",
			pattern: &NodePattern{
				Type: "BinaryExpr",
				Fields: map[string]FieldMatch{
					"left": {
						Capture: "call",
						SubPattern: &NodePattern{
							Type: "CallExpr",
						},
					},
					"op": {
						OneOf: ComparisonOps(),
					},
					"right": {
						Capture: "r",
					},
				},
			},
			wantOK: true,
			wantCap: map[string]string{
				"call": "len(s)",
				"r":    "0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				expr, err := parser.ParseExpr(tt.expr)
				require.NoError(t, err)

				caps, ok := tt.pattern.Match(expr, fset)
				require.Equal(t, tt.wantOK, ok)

				if ok && tt.wantCap != nil {
					for name, wantSrc := range tt.wantCap {
						node := caps[name]
						require.NotNil(
							t, node,
							"capture %q should exist",
							name,
						)
						gotSrc := renderNode(node, fset)
						require.Equal(
							t, wantSrc, gotSrc,
							"capture %q", name,
						)
					}
				}
			},
		)
	}
}

func TestNodePatternMatchCallExpr(t *testing.T) {
	fset := token.NewFileSet()

	tests := []struct {
		name    string
		expr    string
		pattern *NodePattern
		wantOK  bool
	}{
		{
			name: "match simple call",
			expr: "foo()",
			pattern: &NodePattern{
				Type: "CallExpr",
			},
			wantOK: true,
		},
		{
			name: "match call with func capture",
			expr: "pkg.Func(a, b)",
			pattern: &NodePattern{
				Type: "CallExpr",
				Fields: map[string]FieldMatch{
					"func": {
						Capture: "fn",
					},
				},
			},
			wantOK: true,
		},
		{
			name: "not a call",
			expr: "x + y",
			pattern: &NodePattern{
				Type: "CallExpr",
			},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				expr, err := parser.ParseExpr(tt.expr)
				require.NoError(t, err)

				_, ok := tt.pattern.Match(expr, fset)
				require.Equal(t, tt.wantOK, ok)
			},
		)
	}
}

func TestNodePatternMatchKeyValueExpr(t *testing.T) {
	fset := token.NewFileSet()

	expr, err := parser.ParseExpr("T{Field: MakeValue()}")
	require.NoError(t, err)
	lit, ok := expr.(*ast.CompositeLit)
	require.True(t, ok)
	require.Len(t, lit.Elts, 1)
	kv := lit.Elts[0]

	_, ok = (&NodePattern{Type: "KeyValueExpr"}).Match(kv, fset)
	require.True(t, ok)

	_, ok = (&NodePattern{Type: "CompositeLit"}).Match(kv, fset)
	require.False(t, ok)
}

func TestWildcard(t *testing.T) {
	fset := token.NewFileSet()

	expr, err := parser.ParseExpr("x + 1")
	require.NoError(t, err)

	w := Wildcard{}
	caps, ok := w.Match(expr, fset)
	require.True(t, ok)
	require.NotNil(t, caps)

	_, ok = w.Match(nil, fset)
	require.False(t, ok)
}

func TestAnyOf(t *testing.T) {
	fset := token.NewFileSet()

	pattern := &AnyOf{
		Patterns: []Pattern{
			&NodePattern{
				Type: "CallExpr",
			},
			&NodePattern{
				Type: "BinaryExpr",
			},
		},
	}

	// Should match call
	call, _ := parser.ParseExpr("foo()")
	_, ok := pattern.Match(call, fset)
	require.True(t, ok)

	// Should match binary
	binary, _ := parser.ParseExpr("a + b")
	_, ok = pattern.Match(binary, fset)
	require.True(t, ok)

	// Should not match ident
	ident, _ := parser.ParseExpr("x")
	_, ok = pattern.Match(ident, fset)
	require.False(t, ok)
}
