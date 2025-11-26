// Package width provides visual width calculations for text,
// accounting for tabs and multi-byte characters.
package width

// DefaultTabStop is the default tab width (8 spaces).
const DefaultTabStop = 8

// VisualLenWithTab returns the visual column width of s with a custom tab stop.
func VisualLenWithTab(s string, tabStop int) int {
	return AdvanceColsWithTab(0, s, tabStop)
}

// AdvanceColsWithTab returns the column after writing s starting from startCol,
// with a custom tab stop.
func AdvanceColsWithTab(startCol int, s string, tabStop int) int {
	col := startCol
	for _, r := range s {
		switch r {
		case '\n':
			col = 0
		case '\t':
			if tabStop > 0 {
				col = ((col / tabStop) + 1) * tabStop
			}
		default:
			col += RuneWidth(r)
		}
	}
	return col
}

// RuneWidth returns the display width of a rune.
// Most runes are width 1, but wide CJK characters and some emoji are width 2.
// Zero-width characters (combining marks, control chars, etc.) return 0.
func RuneWidth(r rune) int {
	// Control and non-spacing combining marks have zero width.
	if r == 0 {
		return 0
	}
	if r < 32 || (r >= 0x7f && r < 0xa0) {
		return 0
	}
	// Combining diacritical marks and other non-spacing marks
	if r >= 0x0300 && r <= 0x036F {
		return 0
	}
	// Wide characters (CJK, fullwidth, emoji, etc.)
	if isWideRune(r) || isEmojiRune(r) {
		return 2
	}
	return 1
}

// isWideRune returns true for characters that typically display as double-width.
func isWideRune(r rune) bool {
	// CJK Unified Ideographs and related blocks
	if r >= 0x4E00 && r <= 0x9FFF {
		return true
	}
	// CJK Extension blocks
	if r >= 0x3400 && r <= 0x4DBF {
		return true
	}
	// CJK Compatibility Ideographs
	if r >= 0xF900 && r <= 0xFAFF {
		return true
	}
	// Supplementary Ideographic Plane (coarse)
	if r >= 0x20000 && r <= 0x2FFFD {
		return true
	}
	// Fullwidth ASCII variants
	if r >= 0xFF01 && r <= 0xFF60 {
		return true
	}
	// Fullwidth currency, etc.
	if r >= 0xFFE0 && r <= 0xFFE6 {
		return true
	}
	// Hangul syllables
	if r >= 0xAC00 && r <= 0xD7AF {
		return true
	}
	// Hiragana (Japanese)
	if r >= 0x3040 && r <= 0x309F {
		return true
	}
	// Katakana (Japanese)
	if r >= 0x30A0 && r <= 0x30FF {
		return true
	}
	// Katakana Phonetic Extensions
	if r >= 0x31F0 && r <= 0x31FF {
		return true
	}
	// CJK Symbols and Punctuation
	if r >= 0x3000 && r <= 0x303F {
		return true
	}
	return false
}

// isEmojiRune returns true for common emoji that are typically double-width.
func isEmojiRune(r rune) bool {
	// Miscellaneous Symbols and Pictographs
	if r >= 0x1F300 && r <= 0x1F5FF {
		return true
	}
	// Emoticons
	if r >= 0x1F600 && r <= 0x1F64F {
		return true
	}
	// Transport and Map Symbols
	if r >= 0x1F680 && r <= 0x1F6FF {
		return true
	}
	// Alchemical Symbols (some emoji-like)
	if r >= 0x1F700 && r <= 0x1F77F {
		return true
	}
	// Supplemental Symbols and Pictographs
	if r >= 0x1F900 && r <= 0x1F9FF {
		return true
	}
	// Symbols and Pictographs Extended-A
	if r >= 0x1FA70 && r <= 0x1FAFF {
		return true
	}
	// Miscellaneous Symbols
	if r >= 0x2600 && r <= 0x26FF {
		return true
	}
	// Dingbats
	if r >= 0x2700 && r <= 0x27BF {
		return true
	}
	return false
}

// FirstLineLenWithTab returns the visual width of the first line with custom tab stop.
func FirstLineLenWithTab(s string, tabStop int) int {
	for i, r := range s {
		if r == '\n' {
			return VisualLenWithTab(s[:i], tabStop)
		}
	}
	return VisualLenWithTab(s, tabStop)
}

// LastLineLenWithTab returns the visual width of the last line with custom tab stop.
func LastLineLenWithTab(s string, tabStop int) int {
	lastNewline := -1
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '\n' {
			lastNewline = i
			break
		}
	}
	if lastNewline == -1 {
		return VisualLenWithTab(s, tabStop)
	}
	return VisualLenWithTab(s[lastNewline+1:], tabStop)
}
