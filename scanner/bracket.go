package scanner

// ScanBalancedParen finds the matching ')' for '(' at position open.
// Skips over strings and comments. Returns the position of the closing ')',
// or -1 if not found.
func ScanBalancedParen(src []byte, open int) int {
	return ScanBalanced(src, open, '(', ')')
}

// ScanBalanced finds the matching close bracket for the open bracket at position.
// Generic version supporting any bracket pair. Returns -1 if not found.
func ScanBalanced(src []byte, open int, openChar, closeChar byte) int {
	if open >= len(src) || src[open] != openChar {
		return -1
	}
	depth := 1
	i := open + 1
	for i < len(src) && depth > 0 {
		switch {
		case IsStringStart(src, i):
			i = ScanString(src, i)
		case IsLineCommentStart(src, i):
			i = ScanLineComment(src, i)
		case IsBlockCommentStart(src, i):
			i = ScanBlockComment(src, i)
		case src[i] == openChar:
			depth++
			i++
		case src[i] == closeChar:
			depth--
			if depth == 0 {
				return i
			}
			i++
		default:
			i++
		}
	}
	return -1
}
