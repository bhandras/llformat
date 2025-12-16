package formatter

import (
	"go/ast"
	"go/token"
)

// legacyScanCallStartPos returns the token.Pos where the legacy scan-based call
// detectors (multiline + compact fallback) would "start" matching a call.
//
// Those scanners recognize calls whose callee consists only of identifier
// chars and '.' selectors, so they cannot start at non-identifier receiver
// bases like `factory().Method(...)`. For such calls, the scan-based behavior
// is closest to starting at the selector identifier (`Method`).
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
		// The legacy scan detectors do not understand generic instantiations
		// (`f[T](`) or other non-ident callees.
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

