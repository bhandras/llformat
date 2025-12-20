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

// maxVisualLineLenInSpan returns the maximum visual line length within the byte
// span [start:end) when measured as it appears in src.
//
// For the first line, the span may start mid-line; this function includes the
// prefix from the true line start up to start so the returned width reflects
// the actual line width in the source file.
func maxVisualLineLenInSpan(src []byte, start, end int, tabStop int) int {
	if start < 0 {
		start = 0
	}
	if end > len(src) {
		end = len(src)
	}
	if start >= end {
		return 0
	}

	max := 0

	// First line: include prefix from line start to `start`.
	ls := lineStart(src, start)
	i := start
	for i < end && src[i] != '\n' {
		i++
	}
	if w := visualLen(string(src[ls:i]), tabStop); w > max {
		max = w
	}

	// Subsequent full lines within the span.
	for i < end {
		// Skip newline.
		if src[i] == '\n' {
			i++
		}
		lineStartOff := i
		for i < end && src[i] != '\n' {
			i++
		}
		if lineStartOff < i {
			if w := visualLen(string(src[lineStartOff:i]), tabStop); w > max {
				max = w
			}
		}
	}

	return max
}
