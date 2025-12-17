package formatter

import (
	"go/ast"
	"go/token"
)

func (f *LongExprFormatter) forbiddenSpansForASTSelection(src []byte) offsetSpanSet {
	return collectForbiddenLongExprSpans(nil, nil, src)
}

func collectForbiddenLongExprSpans(file *ast.File, fset *token.FileSet, src []byte) offsetSpanSet {
	if file == nil || fset == nil {
		return ownedSpansFromSource(src, defaultLongExprOwnedSpanOptions())
	}
	return ownedSpansFromAST(file, fset, src, defaultLongExprOwnedSpanOptions())
}
