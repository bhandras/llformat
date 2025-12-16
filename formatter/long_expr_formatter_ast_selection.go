package formatter

import (
	"go/ast"
	"go/parser"
	"go/token"
)

func (f *LongExprFormatter) forbiddenSpansForASTSelection(src []byte) offsetSpanSet {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "in.go", src, parser.AllErrors)
	if err != nil || file == nil {
		// On parse failure, fall back to legacy behavior.
		return offsetSpanSet{}
	}

	return collectForbiddenLongExprSpans(file, fset, src)
}

func collectForbiddenLongExprSpans(file *ast.File, fset *token.FileSet, src []byte) offsetSpanSet {
	if file == nil || fset == nil {
		return offsetSpanSet{}
	}

	var spans []offsetSpan
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
		spans = append(spans, offsetSpan{start: start, end: end})
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.CallExpr:
			// Avoid rewriting inside call-arg lists; call formatting stages own
			// this territory.
			if v == nil || v.Lparen == token.NoPos || v.Rparen == token.NoPos {
				return true
			}
			addSpan(v.Lparen, v.Rparen+1)
		case *ast.CompositeLit:
			// Avoid rewriting inside composite literals (especially maps/structs),
			// which are handled by dedicated call/composite formatting.
			if v == nil || v.Lbrace == token.NoPos || v.Rbrace == token.NoPos {
				return true
			}
			addSpan(v.Lbrace, v.Rbrace+1)
		case *ast.FuncLit:
			// Avoid rewriting inside func literal bodies.
			if v == nil || v.Body == nil {
				return true
			}
			addSpan(v.Body.Lbrace, v.Body.Rbrace+1)
		}
		return true
	})

	return newOffsetSpanSet(spans)
}
