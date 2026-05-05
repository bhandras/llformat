package formatter

import (
	"go/ast"
	"go/token"
	"sort"
	"strings"
)

// legacyScanCallStartPos returns the token.Pos where the legacy scan-based call
// detectors (multiline + compact fallback) would "start" matching a call.
//
// Those scanners recognize calls whose callee consists only of identifier chars
// and '.' selectors, so they cannot start at non-identifier receiver bases like
// `factory().Method(...)`. For such calls, the scan-based behavior is closest
// to starting at the selector identifier (`Method`).
func legacyScanCallStartPos(fun ast.Expr) token.Pos {
	switch v := fun.(type) {
	case *ast.Ident:
		return v.Pos()

	case *ast.SelectorExpr:
		if pos, ok := leftmostIdentPosInSelectorChain(v); ok {
			return pos
		}
		if v.Sel != nil {
			return v.Sel.Pos()
		}

		return token.NoPos

	default:

		// The legacy scan detectors do not understand generic
		// instantiations (`f[T](`) or other non-ident callees.
		return token.NoPos
	}
}

func leftmostIdentPosInSelectorChain(sel *ast.SelectorExpr) (token.Pos, bool) {
	var current ast.Expr = sel
	for {
		s, ok := current.(*ast.SelectorExpr)
		if !ok {
			break
		}
		current = s.X
	}

	base, ok := current.(*ast.Ident)
	if !ok || base == nil {
		return token.NoPos, false
	}

	return base.Pos(), true
}

type legacyCallSpan struct {
	Start    int
	Lparen   int
	End      int
	FuncName string
}

// legacyCallSpansFromAST returns call spans whose selection semantics match the
// legacy scan-based call detectors. The returned spans are sorted by Start.
func legacyCallSpansFromAST(file *ast.File, fset *token.FileSet,
	src []byte) []legacyCallSpan {

	if file == nil || fset == nil {
		return nil
	}

	var spans []legacyCallSpan
	ast.Inspect(
		file,
		func(n ast.Node) bool {
			ce, ok := n.(*ast.CallExpr)
			if !ok || ce == nil {
				return true
			}
			if ce.Lparen == token.NoPos || ce.Rparen == token.NoPos {
				return true
			}

			startPos := legacyScanCallStartPos(ce.Fun)
			if startPos == token.NoPos {
				return true
			}

			start := fset.Position(startPos).Offset
			lparen := fset.Position(ce.Lparen).Offset
			end := fset.Position(ce.Rparen).Offset + 1

			if start < 0 || lparen < 0 || end < 0 {
				return true
			}
			if start >= len(src) || lparen > len(src) || end > len(
				src,
			) {
				return true
			}
			if start >= lparen || lparen >= end {
				return true
			}

			funcName := strings.TrimSpace(string(src[start:lparen]))
			spans = append(
				spans, legacyCallSpan{
					Start:    start,
					Lparen:   lparen,
					End:      end,
					FuncName: funcName,
				},
			)

			return true
		},
	)

	sort.Slice(
		spans,
		func(i, j int) bool {
			if spans[i].Start != spans[j].Start {
				return spans[i].Start < spans[j].Start
			}

			return spans[i].End < spans[j].End
		},
	)

	return spans
}
