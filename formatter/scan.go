package formatter

import "github.com/bhandras/llformat/scanner"

// Scanner provides stateful source scanning that skips strings and comments. It
// wraps the low-level scanner package utilities into a convenient struct.
type Scanner struct {
	src []byte
	pos int
}

// NewScanner creates a new Scanner for the given source.
func NewScanner(src []byte) *Scanner {
	return &Scanner{src: src, pos: 0}
}

// Pos returns the current position in the source.
func (s *Scanner) Pos() int {
	return s.pos
}

// SetPos sets the current position.
func (s *Scanner) SetPos(pos int) {
	if pos < 0 {
		pos = 0
	}
	if pos > len(s.src) {
		pos = len(s.src)
	}
	s.pos = pos
}

// AtEnd returns true if the scanner is at the end of input.
func (s *Scanner) AtEnd() bool {
	return s.pos >= len(s.src)
}

// Peek returns the byte at the current position, or 0 if at end.
func (s *Scanner) Peek() byte {
	if s.pos >= len(s.src) {
		return 0
	}

	return s.src[s.pos]
}

// PeekAt returns the byte at the given offset from current position, or 0 if
// out of bounds.
func (s *Scanner) PeekAt(offset int) byte {
	idx := s.pos + offset
	if idx < 0 || idx >= len(s.src) {
		return 0
	}

	return s.src[idx]
}

// Advance moves forward one byte and returns the byte that was at the current
// position. Returns 0 if at end.
func (s *Scanner) Advance() byte {
	if s.pos >= len(s.src) {
		return 0
	}
	b := s.src[s.pos]
	s.pos++

	return b
}

// AdvanceBy moves forward by n bytes.
func (s *Scanner) AdvanceBy(n int) {
	s.pos += n
	if s.pos > len(s.src) {
		s.pos = len(s.src)
	}
}

// SkipLiteral skips a string literal or comment at the current position.
// Returns true if something was skipped.
func (s *Scanner) SkipLiteral() bool {
	if s.AtEnd() {
		return false
	}

	switch {
	case scanner.IsStringStart(s.src, s.pos):
		s.pos = scanner.ScanString(s.src, s.pos)

		return true

	case scanner.IsLineCommentStart(s.src, s.pos):
		s.pos = scanner.ScanLineComment(s.src, s.pos)

		return true

	case scanner.IsBlockCommentStart(s.src, s.pos):
		s.pos = scanner.ScanBlockComment(s.src, s.pos)

		return true
	}

	return false
}

// SkipLiterals advances past all contiguous string literals and comments.
func (s *Scanner) SkipLiterals() {
	for s.SkipLiteral() {
		// Keep skipping
	}
}

// IsAtString returns true if the current position is at a string start.
func (s *Scanner) IsAtString() bool {
	return scanner.IsStringStart(s.src, s.pos)
}

// IsAtLineComment returns true if the current position is at a // comment.
func (s *Scanner) IsAtLineComment() bool {
	return scanner.IsLineCommentStart(s.src, s.pos)
}

// IsAtBlockComment returns true if the current position is at a /* comment.
func (s *Scanner) IsAtBlockComment() bool {
	return scanner.IsBlockCommentStart(s.src, s.pos)
}

// ScanBalancedParen finds the matching ')' starting from the current '('.
// Returns the position of ')' or -1 if not found. Does not advance position.
func (s *Scanner) ScanBalancedParen() int {
	return scanner.ScanBalancedParen(s.src, s.pos)
}

// Slice returns a slice of the source from start to end.
func (s *Scanner) Slice(start, end int) []byte {
	if start < 0 {
		start = 0
	}
	if end > len(s.src) {
		end = len(s.src)
	}
	if start >= end {
		return nil
	}

	return s.src[start:end]
}

// SliceFrom returns a slice from start to current position.
func (s *Scanner) SliceFrom(start int) []byte {
	return s.Slice(start, s.pos)
}

// RemainingFrom returns the source from start to the end.
func (s *Scanner) RemainingFrom(start int) []byte {
	return s.Slice(start, len(s.src))
}

// Remaining returns the source from current position to end.
func (s *Scanner) Remaining() []byte {
	return s.Slice(s.pos, len(s.src))
}

// Source returns the full source.
func (s *Scanner) Source() []byte {
	return s.src
}

// Len returns the total length of the source.
func (s *Scanner) Len() int {
	return len(s.src)
}

// Match returns true if the source at current position matches the given
// string.
func (s *Scanner) Match(pattern string) bool {
	if s.pos+len(pattern) > len(s.src) {
		return false
	}
	for i := 0; i < len(pattern); i++ {
		if s.src[s.pos+i] != pattern[i] {
			return false
		}
	}

	return true
}

// SkipWhitespace advances past spaces and tabs (but not newlines).
func (s *Scanner) SkipWhitespace() {
	for s.pos < len(s.src) {
		if s.src[s.pos] == ' ' || s.src[s.pos] == '\t' {
			s.pos++
		} else {
			break
		}
	}
}

// SkipToNewline advances to the next newline or end of input.
func (s *Scanner) SkipToNewline() {
	for s.pos < len(s.src) && s.src[s.pos] != '\n' {
		s.pos++
	}
}

// SkipPastNewline advances past the next newline (if present).
func (s *Scanner) SkipPastNewline() bool {
	s.SkipToNewline()
	if s.pos < len(s.src) && s.src[s.pos] == '\n' {
		s.pos++

		return true
	}

	return false
}
