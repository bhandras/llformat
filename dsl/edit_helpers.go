package dsl

import "bytes"

// continuationIndentBytes returns the standard llformat continuation
// indentation sequence used by many DSL actions: newline + original indent +
// one tab.
func continuationIndentBytes(indent string) []byte {
	return []byte("\n" + indent + "\t")
}

func lineStart(src []byte, i int) int {
	if i <= 0 {
		return 0
	}
	if i > len(src) {
		i = len(src)
	}
	for i > 0 && src[i-1] != '\n' {
		i--
	}

	return i
}

// lineEnd returns the index of the newline byte that ends the line containing
// i, or len(src) if the line is the last line with no trailing newline.
func lineEnd(src []byte, i int) int {
	if i < 0 {
		i = 0
	}
	if i > len(src) {
		i = len(src)
	}
	for i < len(src) && src[i] != '\n' {
		i++
	}

	return i
}

func skipHorizontalWhitespace(src []byte, i int) int {
	for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
		i++
	}

	return i
}

func backtrackHorizontalWhitespace(src []byte, i int) int {
	for i > 0 && (src[i-1] == ' ' || src[i-1] == '\t') {
		i--
	}

	return i
}

func applyContinuationIndent(src []byte, start, end int, indent string) ([]byte,
	bool, error) {

	replacement := continuationIndentBytes(indent)
	if hasReplacement(src, start, end, replacement) {
		return src, false, nil
	}

	out, err := ApplySingleEdit(src, start, end, replacement)
	if err != nil {
		return nil, false, err
	}

	return out, true, nil
}

func hasReplacement(src []byte, start, end int, replacement []byte) bool {
	if start < 0 || end < start || end > len(src) {
		return false
	}

	return bytes.Equal(src[start:end], replacement)
}

// applyContinuationIndentAfter replaces horizontal whitespace after pos with
// the continuation indent sequence.
func applyContinuationIndentAfter(src []byte, pos int, indent string) ([]byte,
	bool, error) {

	end := skipHorizontalWhitespace(src, pos)

	return applyContinuationIndent(src, pos, end, indent)
}

// applyContinuationIndentBefore replaces horizontal whitespace before pos with
// the continuation indent sequence.
func applyContinuationIndentBefore(src []byte, pos int, indent string) ([]byte,
	bool, error) {

	start := backtrackHorizontalWhitespace(src, pos)

	return applyContinuationIndent(src, start, pos, indent)
}
