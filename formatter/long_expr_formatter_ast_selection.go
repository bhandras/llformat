package formatter

import (
	llast "github.com/lightninglabs/llformat/ast"
)

func (f *LongExprFormatter) forbiddenSpansForASTSelection(src []byte) llast.OffsetSpanSet {
	return llast.OwnedSpansFromSource(src, llast.OwnedSpanOptions{
		IncludeCallArgLists:    true,
		IncludeCompositeBodies: true,
		IncludeFuncBodies:      true,
	})
}
