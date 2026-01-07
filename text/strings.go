package text

import (
	"strings"
	"unicode/utf8"

	"github.com/bhandras/llformat/width"
)

// QuoteGoString returns s as a double-quoted Go string literal. It preserves
// printable runes and escapes only what's required.
func QuoteGoString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)

		case '\n':
			b.WriteString("\\n")

		case '\r':
			b.WriteString("\\r")

		case '\t':
			// Keep literal tab to match golden behavior
			b.WriteByte('\t')

		default:
			if r < 0x20 {
				// Control chars: emit as \xNN
				b.WriteString("\\x")
				const hexdigits = "0123456789abcdef"
				b.WriteByte(hexdigits[(r>>4)&0xF])
				b.WriteByte(hexdigits[r&0xF])
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')

	return b.String()
}

// SplitQuotedString splits text into quoted segments that fit within the given
// width starting at startCol for the first segment. Continuation lines are
// indented with contIndent. Segments are joined with " +" at line breaks.
func SplitQuotedString(text string, startCol int, contIndent string, colLimit,
	tabStop int) string {

	var out strings.Builder
	rest := text
	curStart := startCol
	// String continuation lines get an extra tab beyond the argument
	// indent.
	stringContIndent := contIndent + "\t"
	contStart := width.VisualLenWithTab(stringContIndent, tabStop)

	for rest != "" {
		// If the whole rest fits as a quoted literal on this line, emit
		// and finish.
		quoted := QuoteGoString(rest)
		if width.AdvanceColsWithTab(curStart, quoted, tabStop) <= colLimit {
			out.WriteString(quoted)
			break
		}
		// Choose split point at last space that fits with trailing "
		// +".
		cut := lastQuotedSpaceBefore(curStart, rest, colLimit, tabStop)
		if cut <= 0 {
			// Hard cut by visual width capacity for content
			// excluding quotes + " +".
			capCols := colLimit - curStart - 2 - 2 // quotes + " +"
			if capCols <= 0 {
				capCols = 1
			}
			idx := cutIndexForWidth(
				curStart, rest, capCols, tabStop,
			)
			if idx <= 0 {
				idx = 1
			}
			seg := rest[:idx]
			out.WriteString(QuoteGoString(seg))
			out.WriteString(" +\n")
			out.WriteString(stringContIndent)
			rest = rest[idx:]
			curStart = contStart
			continue
		}
		seg := rest[:cut+1]
		out.WriteString(QuoteGoString(seg))
		out.WriteString(" +\n")
		out.WriteString(stringContIndent)
		rest = rest[cut+1:]
		curStart = contStart
	}

	return out.String()
}

// lastQuotedSpaceBefore returns the last index of an ASCII space in s such that
// the quoted prefix up to and including that space would fit within the
// boundary when starting from startCol and accounting for " +" at the end.
// Returns -1 if no such boundary exists.
func lastQuotedSpaceBefore(startCol int, s string, boundary, tabStop int) int {
	last := -1
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' {
			continue
		}
		piece := s[:i+1]
		used := width.AdvanceColsWithTab(
			startCol, QuoteGoString(piece), tabStop,
		) +
			2 // " +"
		if used < boundary { // strict inequality
			last = i
		} else {
			break
		}
	}

	return last
}

// cutIndexForWidth returns the number of bytes from the start of s that fit
// within maxCols additional columns when starting from startCol. It avoids
// splitting runes.
func cutIndexForWidth(startCol int, s string, maxCols, tabStop int) int {
	col := startCol
	i := 0
	for i < len(s) {
		r, sz := utf8.DecodeRuneInString(s[i:])
		var w int
		if r == '\t' {
			next := ((col / tabStop) + 1) * tabStop
			w = next - col
		} else if r == '\n' {
			break
		} else {
			w = width.RuneWidth(r)
		}
		if (col + w - startCol) > maxCols {
			break
		}
		col += w
		i += sz
	}
	if i <= 0 {
		return 1
	}

	return i
}
