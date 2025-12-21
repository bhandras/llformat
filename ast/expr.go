// Package ast provides AST parsing and inspection utilities for Go source code.
package ast

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
)

// ParseExpr parses s as a Go expression. Returns nil if parsing fails.
func ParseExpr(s string) ast.Expr {
	expr, err := parser.ParseExpr(s)
	if err != nil {
		return nil
	}

	return expr
}

func callExprFromParsed(expr ast.Expr) (*ast.CallExpr, bool) {
	if expr == nil {
		return nil, false
	}
	if call, ok := expr.(*ast.CallExpr); ok {
		return call, true
	}
	if pe, ok := expr.(*ast.ParenExpr); ok {
		call, ok := pe.X.(*ast.CallExpr)

		return call, ok
	}

	return nil, false
}

// IsCallExpr returns true if s parses as a function call expression. Also
// matches parenthesized call expressions like (fn()).
func IsCallExpr(s string) bool {
	expr := ParseExpr(s)
	_, ok := callExprFromParsed(expr)

	return ok
}

// IsCompositeLit returns true if s parses as a composite literal. Also matches
// parenthesized composite literals like (T{}).
func IsCompositeLit(s string) bool {
	expr := ParseExpr(s)
	if expr == nil {
		return false
	}
	if _, ok := expr.(*ast.CompositeLit); ok {
		return true
	}
	if pe, ok := expr.(*ast.ParenExpr); ok {
		_, ok2 := pe.X.(*ast.CompositeLit)

		return ok2
	}

	return false
}

// HasNestedCall returns true if any argument of the call expression is itself a
// call expression or contains a call expression in parentheses.
func HasNestedCall(s string) bool {
	expr := ParseExpr(s)
	ce, ok := callExprFromParsed(expr)
	if !ok {
		return false
	}
	for _, a := range ce.Args {
		switch aa := a.(type) {
		case *ast.CallExpr:
			return true

		case *ast.ParenExpr:
			if _, ok := aa.X.(*ast.CallExpr); ok {
				return true
			}
		}
	}

	return false
}

// FlattenStringExprAST recursively extracts string content from an expression
// tree consisting of string literals and binary '+' operations. This is the AST
// version that takes an already-parsed expression.
func FlattenStringExprAST(e ast.Expr) (string, bool) {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return "", false
		}
		s := v.Value
		// Only support double-quoted strings
		if len(s) < 2 || s[0] != '"' {
			return "", false
		}
		// Properly unquote to handle escape sequences
		unquoted, err := strconv.Unquote(s)
		if err != nil {
			return "", false
		}

		return unquoted, true

	case *ast.ParenExpr:
		return FlattenStringExprAST(v.X)

	case *ast.BinaryExpr:
		if v.Op != token.ADD {
			return "", false
		}
		left, okL := FlattenStringExprAST(v.X)
		right, okR := FlattenStringExprAST(v.Y)
		if !okL || !okR {
			return "", false
		}

		return left + right, true
	}

	return "", false
}
