package dsl

import "strings"

// prefixWidthAt returns the visual width from the start of the line containing
// pos up to pos.
func prefixWidthAt(src []byte, pos int, tabStop int) int {
	ls := lineStart(src, pos)
	return visualLen(string(src[ls:pos]), tabStop)
}

// collapsedSingleLineLen returns the visual width of s when collapsed to a
// single line by normalizing all whitespace runs to a single space.
func collapsedSingleLineLen(s string, tabStop int) int {
	flat := strings.Join(strings.Fields(s), " ")
	return visualLen(flat, tabStop)
}

// collapsedLineLenAt returns the visual width of the line prefix up to pos plus
// the collapsed width of s.
func collapsedLineLenAt(src []byte, pos int, s string, tabStop int) int {
	return prefixWidthAt(src, pos, tabStop) + collapsedSingleLineLen(s, tabStop)
}
