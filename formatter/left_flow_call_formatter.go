package formatter

import (
	"bytes"
	"go/ast"
	formatstd "go/format"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Formatter defines a generic source formatter.
type Formatter interface {
	FormatFile(src []byte) []byte
}

// Config holds configuration for the greedy formatter.
type Config struct {
	ColumnLimit int
	TabStop     int
	Targets     []string
}

// LeftFlowFormatter implements the simple left-flowing greedy formatting.
type LeftFlowFormatter struct{ cfg Config }

// NewLeftFlowFormatter creates a new left-flowing greedy formatter with defaults.
func NewLeftFlowFormatter(cfg Config) *LeftFlowFormatter {
	if cfg.ColumnLimit <= 0 {
		cfg.ColumnLimit = 80
	}
	if cfg.TabStop <= 0 {
		cfg.TabStop = 8
	}
	if len(cfg.Targets) == 0 {
		cfg.Targets = defaultTargets()
	}
	return &LeftFlowFormatter{cfg: cfg}
}

// Package-level defaults (used by compatibility wrapper and helpers).
var columnLimit = 80
var tabStop = 8

func defaultTargets() []string {
	return []string{
		"log.Infof(", "log.Debugf(", "log.Tracef(", "log.Errorf(", "log.Warnf(",
		"fmt.Printf(", "fmt.Sprintf(", "fmt.Errorf(",
	}
}

// FormatFile applies formatting with default config and default targets.
// This is a convenience wrapper for callers that don't need custom config.
func FormatFile(src []byte) []byte {
	// Reset to defaults for single-shot formatting.
	columnLimit = 80
	tabStop = 8
	return formatWithTargets(src, defaultTargets())
}

// FormatFile formats with the formatter's config.
func (f *LeftFlowFormatter) FormatFile(src []byte) []byte {
	// Apply config to package-level parameters used by helpers.
	if f.cfg.ColumnLimit > 0 {
		columnLimit = f.cfg.ColumnLimit
	}
	if f.cfg.TabStop > 0 {
		tabStop = f.cfg.TabStop
	}
	return formatWithTargets(src, f.cfg.Targets)
}

// Core formatting driver given a target signature list.
var currentTargets []string

func formatWithTargets(src []byte, targets []string) []byte {
	currentTargets = targets
	// We'll scan for target callsites and rewrite them in-place into a buffer.
	var out bytes.Buffer
	i := 0
	for i < len(src) {
		// Try to match any target at this position (skipping when inside string/comment
		// handled by a lightweight scanner).
		if isStringStart(src, i) {
			// Copy string literal as-is.
			start := i
			i = scanString(src, i)
			out.Write(src[start:i])
			continue
		}
		if isLineCommentStart(src, i) {
			start := i
			i = scanLineComment(src, i)
			out.Write(src[start:i])
			continue
		}
		if isBlockCommentStart(src, i) {
			start := i
			i = scanBlockComment(src, i)
			out.Write(src[start:i])
			continue
		}

		matched := ""
		for _, t := range targets {
			if hasPrefixAt(src, i, t) {
				matched = t
				break
			}
		}
		if matched == "" {
			out.WriteByte(src[i])
			i++
			continue
		}

		// We found a target call. Find its full extent (balanced parentheses).
		callStart := i
		// Find the opening parenthesis index right after the target.
		openIdx := callStart + len(matched) - 1 // points to '('
		endIdx := scanBalancedParen(src, openIdx)
		if endIdx <= openIdx {
			// Could not find a balanced call; copy verbatim and continue to avoid mangling.
			out.Write(src[callStart : callStart+len(matched)])
			i = callStart + len(matched)
			continue
		}

		// Split around to get indent and call head.
		lineStart := lastLineStart(src, callStart)
		// indentBytes is the entire slice from line start to call start (may include
		// non-whitespace like "return ").
		indentBytes := src[lineStart:callStart]
		// wsIndent is only the leading whitespace of the line.
		wsIndent := leadingWhitespace(src, lineStart)

		// Build formatted call.
		formatted := formatCallGreedy(src[callStart:endIdx+1], string(wsIndent), visualLen(string(indentBytes)))
		out.WriteString(formatted)
		i = endIdx + 1
	}
	res := out.Bytes()

	if formatted, err := formatstd.Source(res); err == nil {
		return formatted
	}
	return res
}

// formatCallGreedy applies a simple greedy layout: keep arguments on the
// current line if they fit (including a preceding ", "), otherwise break
// before the argument. String literals are split at the last space before the
// boundary (or hard-cut) and joined with " +" on continuation lines.
func formatCallGreedy(call []byte, wsIndent string, baseLen int) string {
	s := string(call)
	open := strings.IndexByte(s, '(')
	if open == -1 || !strings.HasSuffix(s, ")") {
		return s
	}
	head := s[:open]
	argsBody := s[open+1 : len(s)-1]

	// No pre-scan; we will attach leading comments of the next arg (// or /* */)
	// to the previous argument inline when emitting.

	rawArgs := splitTopLevel(argsBody)
	hasInlineComment := strings.Contains(argsBody, "/*") || strings.Contains(argsBody, "//")
	normArgs := make([]arg, 0, len(rawArgs))
	for _, ra := range rawArgs {
		trimmed := strings.TrimSpace(ra)
		if e, err := parser.ParseExpr(trimmed); err == nil {
			if str, ok := flattenStringExprOnlyDoubleQuoted(e); ok {
				normArgs = append(normArgs, arg{kind: argText, text: str})
				continue
			}
		}
		normArgs = append(normArgs, arg{kind: argExpr, expr: trimmed})
	}

	width := columnLimit
	var b strings.Builder
	b.WriteString(head)
	b.WriteByte('(')
	curLen := baseLen + visualLen(head) + 1
	contIndent := wsIndent + "\t"

	writeSplit := func(seg string) {
		q := quoteGoString(seg)
		b.WriteString(q)
		curLen = advanceCols(curLen, q)
		b.WriteByte(' ')
		b.WriteByte('+')
		curLen += 2
		b.WriteByte('\n')
		b.WriteString(contIndent)
		curLen = visualLen(contIndent)
	}

	// Track if the previous string argument wrapped across lines.
	lastTextWrapped := false
	for i, a := range normArgs {
		justBroke := false
		if i > 0 {
			// If this arg starts with a comment, detach it so we can place it
			// next to the preceding argument in the correct position.
			lineCommentPrefix := ""
			blockCommentPrefix := ""
			if a.kind == argExpr {
				tl := strings.TrimLeftFunc(a.expr, unicode.IsSpace)
				if strings.HasPrefix(tl, "//") {
					k := 0
					for k < len(tl) && tl[k] != '\n' {
						k++
					}
					lineCommentPrefix = tl[:k]
					a.expr = strings.TrimLeftFunc(tl[k:], unicode.IsSpace)
					tl = strings.TrimLeftFunc(a.expr, unicode.IsSpace)
				}
				if strings.HasPrefix(tl, "/*") {
					if end := strings.Index(tl, "*/"); end >= 0 {
						blockCommentPrefix = tl[:end+2]
						a.expr = strings.TrimLeftFunc(tl[end+2:], unicode.IsSpace)
					}
				}
			}
			if hasInlineComment {
				// Separator on same line; attach trailing line comment to
				// previous arg, then place any block comment before next arg.
				b.WriteString(", ")
				curLen += 2
				if lineCommentPrefix != "" {
					b.WriteString(lineCommentPrefix)
					curLen += visualLen(lineCommentPrefix)
				}
				if blockCommentPrefix != "" {
					b.WriteByte(' ')
					b.WriteString(blockCommentPrefix)
					curLen += 1 + visualLen(blockCommentPrefix)
				}
				// Fall through to printing arg on same line.
			} else {
				// After a wrapped text, keep pairs of expressions together on
				// the continuation line when the pair wouldn't both fit on the
				// current line. This is a minimal, deterministic lookahead to
				// match the intended greedy flow without ad-hoc tie-breakers.
				forceBreak := false
				if lastTextWrapped && a.kind == argExpr {
					if i+1 < len(normArgs) && normArgs[i+1].kind == argExpr {
						need1 := firstLineLen(a.expr)
						need2 := firstLineLen(normArgs[i+1].expr)
						if curLen+2+need1+2+need2 > width {
							forceBreak = true
						}
					}
				}
				switch a.kind {
				case argExpr:
					need := firstLineLen(a.expr)
					if isTargetedCallStart(a.expr) {
						need = exprHeadLen(a.expr)
					}
					if !forceBreak && curLen+2+need < width {
						b.WriteString(", ")
						curLen += 2
						if lineCommentPrefix != "" {
							b.WriteString(lineCommentPrefix)
							curLen += visualLen(lineCommentPrefix)
						}
						if blockCommentPrefix != "" {
							b.WriteByte(' ')
							b.WriteString(blockCommentPrefix)
							curLen += 1 + visualLen(blockCommentPrefix)
						}
						// Only consider the lookahead for the very first
						// expression after a wrapped text.
						lastTextWrapped = false
					} else {
						// Put trailing line comment on the same line as the comma.
						b.WriteByte(',')
						if lineCommentPrefix != "" {
							b.WriteByte(' ')
							b.WriteString(lineCommentPrefix)
						}
						b.WriteByte('\n')
						b.WriteString(contIndent)
						curLen = visualLen(contIndent)
						justBroke = true
						if blockCommentPrefix != "" {
							// Place block comment before the arg on the new line.
							b.WriteString(blockCommentPrefix)
							b.WriteByte(' ')
							curLen += visualLen(blockCommentPrefix) + 1
						}
						// Reset lookahead after the first decision.
						lastTextWrapped = false
					}
				case argText:
					// minimal placeable segment on same line: "X" +
					if curLen+2+(2+1+2) <= width { // ", " + (quotes+char+ +)
						b.WriteString(", ")
						curLen += 2
						if lineCommentPrefix != "" {
							b.WriteString(lineCommentPrefix)
							curLen += visualLen(lineCommentPrefix)
						}
						if blockCommentPrefix != "" {
							b.WriteByte(' ')
							b.WriteString(blockCommentPrefix)
							curLen += 1 + visualLen(blockCommentPrefix)
						}
					} else {
						b.WriteByte(',')
						if lineCommentPrefix != "" {
							b.WriteByte(' ')
							b.WriteString(lineCommentPrefix)
						}
						b.WriteByte('\n')
						b.WriteString(contIndent)
						curLen = visualLen(contIndent)
						justBroke = true
						if blockCommentPrefix != "" {
							b.WriteString(blockCommentPrefix)
							b.WriteByte(' ')
							curLen += visualLen(blockCommentPrefix) + 1
						}
					}
				}
			}
		}

		if a.kind == argExpr {
			// For nested targeted calls, use the head length to decide fit.
			need := firstLineLen(a.expr)
			if isTargetedCallStart(a.expr) {
				need = exprHeadLen(a.expr)
			}
			if !justBroke && !isRawStringLiteral(a.expr) && curLen+need > width {
				b.WriteByte('\n')
				b.WriteString(contIndent)
				curLen = visualLen(contIndent)
			}
			if isTargetedCallStart(a.expr) {
				formatted := formatCallGreedy([]byte(a.expr), wsIndent, curLen)
				b.WriteString(formatted)
				curLen = lastLineLen(formatted)
			} else {
				b.WriteString(a.expr)
				curLen = advanceCols(curLen, a.expr)
			}
			continue
		}

		// String arg: split greedily
		rest := a.text
		didSplit := false
		for len(rest) > 0 {
			q := quoteGoString(rest)
			// If there are more args after this string, reserve ", " suffix.
			suffix := 0
			if i < len(normArgs)-1 {
				suffix = 2
			}
			if advanceCols(curLen, q)+suffix <= width {
				b.WriteString(q)
				curLen = advanceCols(curLen, q)
				rest = ""
				break
			}
			// Capacity for content (excluding quotes and " +") of this split
			// segment. This is a non-final segment (we are splitting), so we
			// allow exact fill up to the boundary with the trailing " +".
			capCols := (width) - curLen - 2 - 2 // quotes + " +"
			if capCols <= 0 {
				b.WriteByte('\n')
				b.WriteString(contIndent)
				curLen = visualLen(contIndent)
				capCols = width - curLen - 2 - 2
				if capCols <= 0 {
					capCols = 1
				}
			}
			// Choose the last ASCII space whose QUOTED prefix fits, taking
			// into account escape expansion inside the literal.
			cut := lastQuotedSpaceBefore(curLen, rest, width)
			if cut <= 0 {
				// No space within capacity.
				// If we are not on a continuation line and the upcoming word
				// (up to the next space) would fit on a continuation line,
				// wrap before it to avoid splitting a word on the head line.
				if curLen != visualLen(contIndent) {
					if sp := strings.IndexByte(rest, ' '); sp > 0 {
						base := visualLen(contIndent)
						// compute content width of the first word at cont indent
						wordCols := advanceCols(base, rest[:sp]) - base
						nextCap := (width) - base - 2 - 2 // quotes + " +"
						if wordCols <= nextCap {
							b.WriteByte('\n')
							b.WriteString(contIndent)
							curLen = visualLen(contIndent)
							// Recompute capacity on the fresh continuation line
							capCols = (width) - curLen - 2 - 2
							if capCols <= 0 {
								capCols = 1
							}
							continue
						}
					}
				}
				// Hard cut by visual columns.
				idx := cutIndexForWidthFrom(curLen, rest, capCols)
				seg := rest[:idx]
				writeSplit(seg)
				didSplit = true
				rest = rest[idx:]
				continue
			}
			// Pure greedy: no additional word-pushing heuristics.
			// Pure greedy: take the last space within capacity.
			seg := rest[:cut+1] // keep the space at end
			writeSplit(seg)
			didSplit = true
			rest = rest[cut+1:]
		}
		lastTextWrapped = didSplit
	}
	b.WriteByte(')')
	return b.String()
}

type argKind int

const (
	argExpr argKind = iota
	argText
)

type arg struct {
	kind argKind
	expr string
	text string
}

func hasPrefixAt(b []byte, i int, s string) bool {
	if i+len(s) > len(b) {
		return false
	}
	return string(b[i:i+len(s)]) == s
}

// (containsTarget removed: legacy helper no longer used)

func isRawStringLiteral(s string) bool {
	t := strings.TrimSpace(s)
	return len(t) >= 2 && t[0] == '`' && t[len(t)-1] == '`'
}

func isTargetedCallStart(s string) bool {
	ts := strings.TrimSpace(s)
	for _, t := range currentTargets {
		if strings.HasPrefix(ts, t) {
			return true
		}
	}
	return false
}

// lastLineLen returns the visual length of the last line of s.
func lastLineLen(s string) int {
	idx := strings.LastIndexByte(s, '\n')
	if idx == -1 {
		return visualLen(s)
	}
	return visualLen(s[idx+1:])
}

func lastLineStart(b []byte, i int) int {
	j := i - 1
	for j >= 0 {
		if b[j] == '\n' {
			return j + 1
		}
		j--
	}
	return 0
}

func isStringStart(b []byte, i int) bool {
	return b[i] == '"' || b[i] == '`'
}

func scanString(b []byte, i int) int {
	quote := b[i]
	i++
	for i < len(b) {
		if b[i] == '\\' && quote == '"' {
			i += 2
			continue
		}
		if b[i] == quote {
			i++
			break
		}
		i++
	}
	return i
}

func isLineCommentStart(b []byte, i int) bool {
	return i+1 < len(b) && b[i] == '/' && b[i+1] == '/'
}

func scanLineComment(b []byte, i int) int {
	for i < len(b) && b[i] != '\n' {
		i++
	}
	return i
}

func isBlockCommentStart(b []byte, i int) bool {
	return i+1 < len(b) && b[i] == '/' && b[i+1] == '*'
}

func scanBlockComment(b []byte, i int) int {
	i += 2
	for i+1 < len(b) {
		if b[i] == '*' && b[i+1] == '/' {
			i += 2
			break
		}
		i++
	}
	return i
}

func scanBalancedParen(b []byte, open int) int {
	// open points at '('.
	depth := 0
	i := open
	for i < len(b) {
		c := b[i]
		if isStringStart(b, i) {
			i = scanString(b, i)
			continue
		}
		if isLineCommentStart(b, i) {
			i = scanLineComment(b, i)
			continue
		}
		if isBlockCommentStart(b, i) {
			i = scanBlockComment(b, i)
			continue
		}
		if c == '(' {
			depth++
		} else if c == ')' {
			depth--
			if depth == 0 {
				return i
			}
		}
		i++
	}
	return -1
}

func splitTopLevel(s string) []string {
	var out []string
	start := 0
	depth := 0
	inStr := byte(0)
	esc := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr != 0 {
			if inStr == '"' && c == '\\' && !esc {
				esc = true
				continue
			}
			if esc {
				esc = false
			} else if c == inStr {
				inStr = 0
			}
			continue
		}
		switch c {
		case '"', '`':
			inStr = c
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	// tail
	if tail := strings.TrimSpace(s[start:]); tail != "" {
		out = append(out, tail)
	}
	return out
}

func quoteGoString(s string) string {
	// Emit a double-quoted Go string literal, preserving runes as-is where
	// possible. Escape only what Go requires or what would break the literal:
	// - '"' and '\\' are escaped
	// - tabs are kept as a literal tab (not \t)
	// - control runes below space (except tab) are emitted as \xNN
	// - newlines should not appear in segments (we split lines before), but
	//   if present, escape as \n
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
				// Other control chars: emit as \xNN
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

// chunkText splits s into segments suitable for line-wrapping with ` +` between
// lines. It prefers splitting on spaces and ensures that every segment except
// the last ends with a space, so that words remain separated across the join.
// maxCur is the remaining space on the current line; width is the total limit.
// (legacy chunkTextFit removed)

// chunkTextFitWithLastLimit splits s into segments honoring different limits
// for the first/middle segments and a tighter limit for the final segment. All
// limits are total per-line available columns including the two surrounding
// quotes.
// (legacy chunkTextFitWithLastLimit removed)

// chooseSuffixStart returns an index in s where a final suffix segment can
// start so that the quoted suffix fits in lastAvail columns. It prefers
// starting at word boundaries (the character after a space). Returns 0 if no
// reasonable boundary is found.
// (legacy chooseSuffixStart removed)

// (legacy lastSpaceBefore removed)

func visualLen(s string) int {
	col := 0
	for _, r := range s {
		switch r {
		case '\n':
			// Treat newline as reset of column; visualLen is generally used on
			// single-line segments, but guard anyway.
			col = 0
			continue
		case '\t':
			// Tabs advance to the next tab stop.
			ts := tabStop
			if ts <= 0 {
				ts = 8
			}
			next := ((col / ts) + 1) * ts
			col = next
			continue
		}
		col += runeWidth(r)
	}
	return col
}

// advanceCols returns the absolute column after writing s starting from
// startCol, accounting for tabs advancing to the next tab stop.
func advanceCols(startCol int, s string) int {
	col := startCol
	for _, r := range s {
		switch r {
		case '\n':
			col = 0
			continue
		case '\t':
			ts := tabStop
			if ts <= 0 {
				ts = 8
			}
			next := ((col / ts) + 1) * ts
			col = next
			continue
		}
		col += runeWidth(r)
	}
	return col
}

// lastSpaceBeforeFrom returns the last byte index of an ASCII space such that
// the substring up to that index fits within maxCols additional columns when
// starting from startCol.
// (legacy lastSpaceBeforeFrom removed)

// lastQuotedSpaceBefore returns the last index of an ASCII space in s such
// that the quoted prefix up to and including that space would fit within the
// boundary when starting from startCol and accounting for " +" at the end of
// the segment. Returns -1 if no such boundary exists.
func lastQuotedSpaceBefore(startCol int, s string, boundary int) int {
	last := -1
	// Scan forward; the quoted width is monotonic with i.
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' {
			continue
		}
		piece := s[:i+1]
		used := advanceCols(startCol, quoteGoString(piece)) + 2 // account for " +"
		if used <= boundary {
			last = i
		} else {
			break
		}
	}
	return last
}

// cutIndexForWidthFrom returns the number of bytes from the start of s that
// fit within maxCols additional columns when starting from startCol. It avoids
// splitting runes.
func cutIndexForWidthFrom(startCol int, s string, maxCols int) int {
	col := startCol
	i := 0
	for i < len(s) {
		r, sz := utf8.DecodeRuneInString(s[i:])
		var w int
		if r == '\t' {
			ts := tabStop
			if ts <= 0 {
				ts = 8
			}
			next := ((col / ts) + 1) * ts
			w = next - col
		} else if r == '\n' {
			break
		} else {
			w = runeWidth(r)
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

func runeWidth(r rune) int {
	// Control and non-spacing combining marks have zero width.
	if r == 0 {
		return 0
	}
	if r < 32 || (r >= 0x7f && r < 0xa0) {
		return 0
	}
	if unicode.Is(unicode.Mn, r) {
		return 0
	}
	if isWideRune(r) || isEmojiRune(r) {
		return 2
	}
	return 1
}

func isWideRune(r rune) bool {
	// Heuristic: treat common East Asian scripts and fullwidth ranges as wide.
	if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) {
		return true
	}
	// CJK Extensions and compatibility blocks.
	if (r >= 0x3400 && r <= 0x4DBF) || // CJK Unified Ideographs Ext A
		(r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
		(r >= 0xF900 && r <= 0xFAFF) || // CJK Compatibility Ideographs
		(r >= 0x20000 && r <= 0x2FFFD) || // Supplementary Ideographic Plane (coarse)
		(r >= 0xFF01 && r <= 0xFF60) || // Fullwidth ASCII variants
		(r >= 0xFFE0 && r <= 0xFFE6) { // Fullwidth currency, etc.
		return true
	}
	return false
}

func isEmojiRune(r rune) bool {
	// Broad coverage of emoji ranges.
	if (r >= 0x1F300 && r <= 0x1F5FF) || // Misc Symbols and Pictographs
		(r >= 0x1F600 && r <= 0x1F64F) || // Emoticons
		(r >= 0x1F680 && r <= 0x1F6FF) || // Transport and Map
		(r >= 0x1F700 && r <= 0x1F77F) || // Alchemical (some emoji-like)
		(r >= 0x1F900 && r <= 0x1F9FF) || // Supplemental Symbols and Pictographs
		(r >= 0x1FA70 && r <= 0x1FAFF) || // Symbols & Pictographs Ext-A
		(r >= 0x2600 && r <= 0x26FF) || // Misc Symbols
		(r >= 0x2700 && r <= 0x27BF) { // Dingbats
		return true
	}
	return false
}

// (no language-specific heuristics)

// firstLineLen returns the visual length (tabs as 8) of s up to its first
// newline (or full length if no newline is present).
func firstLineLen(s string) int {
	i := strings.IndexByte(s, '\n')
	if i == -1 {
		return visualLen(s)
	}
	return visualLen(s[:i])
}

// exprHeadLen returns the visual length of the expression head up to and
// including the first opening parenthesis '(' that is not inside a string or
// comment. If no such parenthesis exists, it falls back to the first line
// length.
func exprHeadLen(s string) int {
	i := 0
	inStr := byte(0)
	esc := false
	for i < len(s) {
		c := s[i]
		if inStr != 0 {
			if inStr == '"' && c == '\\' && !esc {
				esc = true
				i++
				continue
			}
			if esc {
				esc = false
			} else if c == inStr {
				inStr = 0
			}
			i++
			continue
		}
		// Skip comments line/block
		if i+1 < len(s) && s[i] == '/' {
			if s[i+1] == '/' { // line comment
				for i < len(s) && s[i] != '\n' {
					i++
				}
				continue
			}
			if s[i+1] == '*' { // block comment
				i += 2
				for i+1 < len(s) {
					if s[i] == '*' && s[i+1] == '/' {
						i += 2
						break
					}
					i++
				}
				continue
			}
		}
		if c == '"' || c == '`' {
			inStr = c
			i++
			continue
		}
		if c == '(' {
			// Include the '('
			return visualLen(s[:i+1])
		}
		// Stop on newline since we only want head of first line by default.
		if c == '\n' {
			return visualLen(s[:i])
		}
		i++
	}
	return firstLineLen(s)
}

// (legacy shrinkLastSegment removed)

// foldTrailingSegments greedily merges the last segments into one as long as
// the combined quoted length fits into lastAvail columns. This helps avoid
// cases where the tail like "%v with result %v." is split unnecessarily.
// (legacy foldTrailingSegmentsSafe removed)

// (legacy max removed)

// splitPrefixIntoTwo splits s into exactly two segments that fit within the
// first and next available widths (including quotes). It prefers a split that
// makes the second segment as full as possible (close to nextAvail), which
// mirrors the example layout. It returns nil if it can't find a suitable split.
// (legacy splitPrefixIntoTwo removed)

// splitPrefixHeadCont tries to split s into 1 or 2 segments so that:
// - segment 1 fits within firstAvail (including quotes), ending on a word boundary
// - the remainder fits within nextAvail (including quotes)
// Returns nil if it can't satisfy the constraints.
// (legacy splitPrefixHeadCont removed)

// ensureHeadFits splits segs[0] if needed so that the first quoted segment
// fits within firstAvail columns. It preserves word boundaries and adds a
// trailing space to the first part if split.
// (legacy ensureHeadFits removed)

// leadingWhitespace returns the whitespace prefix of the line starting at idx.
func leadingWhitespace(b []byte, idx int) []byte {
	i := idx
	for i < len(b) {
		if b[i] == ' ' || b[i] == '\t' {
			i++
			continue
		}
		break
	}
	return b[idx:i]
}

// AST helpers

// (legacy printNode removed)

// (legacy flattenStringExpr removed)

// flattenStringExprOnlyDoubleQuoted is like flattenStringExpr but only returns
// true if the expression is a string literal (or concatenation thereof) using
// exclusively double-quoted literals. Raw string literals (backticks) cause a
// false result so the caller can preserve raw quoting.
func flattenStringExprOnlyDoubleQuoted(e ast.Expr) (string, bool) {
	switch x := e.(type) {
	case *ast.BasicLit:
		if x.Kind == token.STRING {
			// Accept only double-quoted literals.
			if len(x.Value) > 0 && x.Value[0] == '"' {
				s, err := strconv.Unquote(x.Value)
				if err != nil {
					return "", false
				}
				return s, true
			}
		}
		return "", false
	case *ast.ParenExpr:
		return flattenStringExprOnlyDoubleQuoted(x.X)
	case *ast.BinaryExpr:
		if x.Op != token.ADD {
			return "", false
		}
		l, ok1 := flattenStringExprOnlyDoubleQuoted(x.X)
		r, ok2 := flattenStringExprOnlyDoubleQuoted(x.Y)
		if ok1 && ok2 {
			return l + r, true
		}
		return "", false
	default:
		return "", false
	}
}
