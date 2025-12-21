package dsl

import (
	"go/parser"
	"strings"
)

func hasAnyComment(s string) bool {
	if hasLineComment(s) {
		return true
	}

	return hasBlockComment(s)
}

// isSafeStandaloneExprSpan reports whether s looks safe to replace as a single
// Go expression span. It is intentionally conservative.
func isSafeStandaloneExprSpan(s string) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	if hasAnyComment(s) {
		return false
	}
	_, err := parser.ParseExpr(s)

	return err == nil
}
