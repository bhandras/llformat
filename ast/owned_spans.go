// Package ast provides AST parsing and inspection utilities for Go source code.
package ast

import (
	"go/ast"
	"go/parser"
	"go/token"
)

type OwnedSpanOptions struct {
	IncludeCallArgLists    bool
	IncludeCallExprs       bool
	IncludeCompositeBodies bool
	IncludeFuncBodies      bool
}

func OwnedSpansFromSource(src []byte, opts OwnedSpanOptions) OffsetSpanSet {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "in.go", src, parser.AllErrors)
	if err != nil || file == nil {

		// On parse failure, return an empty set so callers naturally
		// fall back to legacy behavior.
		return OffsetSpanSet{}
	}

	return OwnedSpansFromAST(file, fset, src, opts)
}

func OwnedSpansFromAST(file *ast.File, fset *token.FileSet, src []byte,
	opts OwnedSpanOptions) OffsetSpanSet {

	if file == nil || fset == nil {
		return OffsetSpanSet{}
	}

	var spans []OffsetSpan
	addSpan := func(startPos, endPos token.Pos) {
		if startPos == token.NoPos || endPos == token.NoPos {
			return
		}
		start := fset.Position(startPos).Offset
		end := fset.Position(endPos).Offset
		if start < 0 || end < 0 {
			return
		}
		if start >= end {
			return
		}
		if start >= len(src) {
			return
		}
		if end > len(src) {
			end = len(src)
		}
		spans = append(spans, OffsetSpan{Start: start, End: end})
	}

	ast.Inspect(
		file,
		func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.CallExpr:
				if opts.IncludeCallExprs {
					if v == nil || v.Pos() == token.NoPos || v.End() == token.NoPos {
						return true
					}
					addSpan(v.Pos(), v.End())
				}
				if !opts.IncludeCallArgLists {
					return true
				}
				if v == nil || v.Lparen == token.NoPos || v.Rparen == token.NoPos {
					return true
				}
				addSpan(v.Lparen, v.Rparen+1)

			case *ast.CompositeLit:
				if !opts.IncludeCompositeBodies {
					return true
				}
				if v == nil || v.Lbrace == token.NoPos || v.Rbrace == token.NoPos {
					return true
				}
				addSpan(v.Lbrace, v.Rbrace+1)

			case *ast.FuncLit:
				if !opts.IncludeFuncBodies {
					return true
				}
				if v == nil || v.Body == nil {
					return true
				}
				addSpan(v.Body.Lbrace, v.Body.Rbrace+1)
			}

			return true
		},
	)

	return NewOffsetSpanSet(spans)
}
