// Package text provides text manipulation utilities for formatting.
package text

// LeadingWhitespace extracts the whitespace prefix from the line starting at
// lineStart in b.
func LeadingWhitespace(b []byte, lineStart int) []byte {
	end := lineStart
	for end < len(b) && (b[end] == ' ' || b[end] == '\t') {
		end++
	}

	return b[lineStart:end]
}

// LastLineStart finds the start position of the line containing pos. Returns 0
// if pos is on the first line.
func LastLineStart(b []byte, pos int) int {
	if pos > len(b) {
		pos = len(b)
	}
	for i := pos - 1; i >= 0; i-- {
		if b[i] == '\n' {
			return i + 1
		}
	}

	return 0
}
