// Package scanner provides low-level scanning utilities for Go source code.
// These primitives handle string literals, comments, and bracket balancing
// while properly skipping over nested constructs.
package scanner

// IsStringStart returns true if position i starts a string literal
// (either double-quoted or backtick raw string).
func IsStringStart(b []byte, i int) bool {
	if i >= len(b) {
		return false
	}
	return b[i] == '"' || b[i] == '`'
}

// ScanString advances past a string literal starting at position i.
// Handles both double-quoted strings (with escape sequences) and
// backtick raw strings. Returns the position after the closing quote,
// or len(src) if unterminated.
func ScanString(src []byte, i int) int {
	if i >= len(src) {
		return i
	}
	quote := src[i]
	i++
	for i < len(src) {
		if src[i] == quote {
			return i + 1
		}
		if quote == '"' && src[i] == '\\' && i+1 < len(src) {
			i += 2 // skip escape sequence
			continue
		}
		i++
	}
	return len(src)
}
