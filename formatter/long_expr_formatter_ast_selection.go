package formatter

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
)

type offsetSpan struct {
	start int
	end   int
}

func (s offsetSpan) contains(off int) bool {
	return off >= s.start && off < s.end
}

func (f *LongExprFormatter) forbiddenSpansForASTSelection(src []byte) []offsetSpan {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "in.go", src, parser.AllErrors)
	if err != nil || file == nil {
		// On parse failure, fall back to legacy behavior.
		return nil
	}

	return collectForbiddenLongExprSpans(file, fset, src)
}

func isOffsetInAnySpan(off int, spans []offsetSpan) bool {
	// spans are sorted by start.
	for _, s := range spans {
		if off < s.start {
			return false
		}
		if s.contains(off) {
			return true
		}
	}
	return false
}

func collectForbiddenLongExprSpans(file *ast.File, fset *token.FileSet, src []byte) []offsetSpan {
	if file == nil || fset == nil {
		return nil
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

	sort.Slice(spans, func(i, j int) bool {
		if spans[i].start != spans[j].start {
			return spans[i].start < spans[j].start
		}
		return spans[i].end < spans[j].end
	})

	return spans
}
