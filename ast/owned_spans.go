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
		if start, end, ok := spanOffsets(
			fset, src, startPos, endPos,
		); ok {

			spans = append(
				spans, OffsetSpan{
					Start: start,
					End:   end,
				},
			)
		}
	}

	ast.Inspect(
		file,
		func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.CallExpr:
				addCallSpans(opts, v, addSpan)

			case *ast.CompositeLit:
				addCompositeLitSpan(opts, v, addSpan)

			case *ast.FuncLit:
				addFuncLitSpan(opts, v, addSpan)
			}

			return true
		},
	)

	return NewOffsetSpanSet(spans)
}

func spanOffsets(fset *token.FileSet, src []byte, startPos, endPos token.Pos) (
	start, end int, ok bool) {

	if fset == nil || startPos == token.NoPos || endPos == token.NoPos {
		return 0, 0, false
	}
	start = fset.Position(startPos).Offset
	end = fset.Position(endPos).Offset
	if start < 0 || end < 0 || start >= end || start >= len(src) {
		return 0, 0, false
	}
	if end > len(src) {
		end = len(src)
	}

	return start, end, true
}

func addCallSpans(opts OwnedSpanOptions, call *ast.CallExpr,
	addSpan func(startPos, endPos token.Pos)) {

	if call == nil {
		return
	}
	if opts.IncludeCallExprs {
		addSpan(call.Pos(), call.End())
	}
	if !opts.IncludeCallArgLists {
		return
	}
	if call.Lparen == token.NoPos || call.Rparen == token.NoPos {
		return
	}
	addSpan(call.Lparen, call.Rparen+1)
}

func addCompositeLitSpan(opts OwnedSpanOptions, lit *ast.CompositeLit,
	addSpan func(startPos, endPos token.Pos)) {

	if !opts.IncludeCompositeBodies || lit == nil {
		return
	}
	if lit.Lbrace == token.NoPos || lit.Rbrace == token.NoPos {
		return
	}
	addSpan(lit.Lbrace, lit.Rbrace+1)
}

func addFuncLitSpan(opts OwnedSpanOptions, lit *ast.FuncLit,
	addSpan func(startPos, endPos token.Pos)) {

	if !opts.IncludeFuncBodies || lit == nil || lit.Body == nil {
		return
	}
	addSpan(lit.Body.Lbrace, lit.Body.Rbrace+1)
}
