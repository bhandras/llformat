package scanner

// IsLineCommentStart returns true if position i starts a // comment.
func IsLineCommentStart(b []byte, i int) bool {
	return i+1 < len(b) && b[i] == '/' && b[i+1] == '/'
}

// IsBlockCommentStart returns true if position i starts a /* comment.
func IsBlockCommentStart(b []byte, i int) bool {
	return i+1 < len(b) && b[i] == '/' && b[i+1] == '*'
}

// ScanLineComment advances past a // comment starting at position i.
// Returns the position after the newline (or end of input).
func ScanLineComment(src []byte, i int) int {
	for i < len(src) && src[i] != '\n' {
		i++
	}
	if i < len(src) {
		i++ // skip the newline
	}
	return i
}

// ScanBlockComment advances past a /* */ comment starting at position i.
// Returns the position after the closing */, or len(src) if unterminated.
func ScanBlockComment(src []byte, i int) int {
	i += 2 // skip /*
	for i+1 < len(src) {
		if src[i] == '*' && src[i+1] == '/' {
			return i + 2
		}
		i++
	}
	return len(src)
}
