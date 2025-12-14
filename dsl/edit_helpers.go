package dsl

import "bytes"

// continuationIndentBytes returns the standard llformat continuation indentation
// sequence used by many DSL actions: newline + original indent + one tab.
func continuationIndentBytes(indent string) []byte {
	return []byte("\n" + indent + "\t")
}

func skipHorizontalWhitespace(src []byte, i int) int {
	for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
		i++
	}
	return i
}

func applyContinuationIndent(src []byte, start, end int, indent string) ([]byte, bool, error) {
	replacement := continuationIndentBytes(indent)
	if start >= 0 && end >= start && end <= len(src) {
		if bytes.Equal(src[start:end], replacement) {
			return src, false, nil
		}
	}

	out, err := ApplySingleEdit(src, start, end, replacement)
	if err != nil {
		return nil, false, err
	}
	return out, true, nil
}
