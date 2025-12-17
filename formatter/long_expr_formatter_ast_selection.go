package formatter

import (
	llast "github.com/lightninglabs/llformat/ast"
)

func (f *LongExprFormatter) forbiddenSpansForASTSelection(src []byte, includeCallArgLists bool) llast.OffsetSpanSet {
	return llast.OwnedSpansFromSource(src, llast.OwnedSpanOptions{
		IncludeCallExprs:       f.cfg.ExcludeCallExprs,
		IncludeCallArgLists:    includeCallArgLists,
		IncludeCompositeBodies: true,
		IncludeFuncBodies:      true,
	})
}
