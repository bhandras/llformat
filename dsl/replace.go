package dsl

import "fmt"

// replaceSpan applies a validated single-span replacement. It is intentionally
// strict: callers should already have chosen a precise span (typically based on
// AST positions) and computed a replacement.
func replaceSpan(src []byte, start, end int, replace []byte) ([]byte, error) {
	if start < 0 || end < 0 || start > len(src) || end > len(src) ||
		start >= end {
		return nil, fmt.Errorf("invalid span [%d:%d] (len=%d)", start,
			end, len(src))
	}

	return ApplySingleEdit(src, start, end, replace)
}
