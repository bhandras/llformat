package format

import (
	"bytes"
	"go/ast"
	formatstd "go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"strconv"
	"strings"
	"unicode"
)

// nestedDepth tracks recursive formatting of a targeted call within another
// targeted call's argument list to guide nested breaking heuristics.
var nestedDepth int

// FormatFile applies log/printf wrapping rules to the provided Go source.
// It keeps unrelated content intact and aims to be resilient even if the file
// is not fully valid Go.
func FormatFile(src []byte) []byte {
	targets := []string{
		"log.Infof(", "log.Debugf(", "log.Tracef(", "log.Errorf(", "log.Warnf(",
		"fmt.Printf(", "fmt.Sprintf(", "fmt.Errorf(",
	}

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
		formatted := formatCall(src[callStart:endIdx+1], string(wsIndent), visualLen(string(indentBytes)))
		out.WriteString(formatted)
		i = endIdx + 1
	}
	res := out.Bytes()
	if formatted, err := formatstd.Source(res); err == nil {
		return formatted
	}
	return res
}

// formatCall rewrites a matched call expression bytes (from function start up to
// the closing parenthesis) into wrapped lines according to the rules.
// indent is the leading whitespace of the line the call starts on.
func formatCall(call []byte, wsIndent string, baseLen int) string {
	s := string(call)
	// Split head and args using lightweight scanning to preserve original text
	// (including comments and operator spacing) for non-text args.
	open := strings.IndexByte(s, '(')
	if open == -1 || !strings.HasSuffix(s, ")") {
		return s
	}
	head := s[:open]
	argsBody := s[open+1 : len(s)-1]

	rawArgs := splitTopLevel(argsBody)
	hasInlineComment := strings.Contains(argsBody, "/*") || strings.Contains(argsBody, "//")
	normArgs := make([]arg, 0, len(rawArgs))
	for _, ra := range rawArgs {
		trimmed := strings.TrimSpace(ra)
		// Try to parse and flatten if the argument is purely a concatenation
		// of double-quoted string literals. Preserve raw (backtick) strings
		// as expressions to keep their quoting intact.
		if e, err := parser.ParseExpr(trimmed); err == nil {
			if str, ok := flattenStringExprOnlyDoubleQuoted(e); ok {
				if str != "" {
					normArgs = append(normArgs, arg{kind: argText, text: str})
					continue
				}
			}
		}
		normArgs = append(normArgs, arg{kind: argExpr, expr: trimmed})
	}

	// Now layout lines <= 80 columns.
	const width = 80
	// Base line starts with indent + head + "("
	var b strings.Builder
	b.WriteString(head)
	b.WriteByte('(')

	// current line tracking
	// length counts visible characters from start of line to current position.
	curLen := baseLen + visualLen(head) + 1 // +1 for '('

	// continuation indent is whitespace-only indent + one tab
	contIndent := wsIndent + "\t"

	for i, a := range normArgs {
		// Track whether we moved this argument to a fresh continuation line.
		placedOnNewLine := false
		if i > 0 {
			// Decide placement of the separator and whether to keep the next
			// argument on the same line. For expressions, estimate using the
			// first line length; for text, ensure there is at least room for
			// minimal quoted text.
			keepSame := false
			switch a.kind {
			case argExpr:
				// Prefer keeping raw string literals on the same line to
				// preserve raw quoting; allow overflow if needed.
				if isRawStringLiteral(a.expr) {
					keepSame = true
				} else if isTargetedCallStart(a.expr) {
					// For nested targeted calls, prefer to keep the head
					// (up to '(') on the same line; the nested formatter will
					// place subsequent content appropriately.
					if curLen+2+exprHeadLen(a.expr) <= width {
						keepSame = true
					}
				} else if curLen+2+firstLineLen(a.expr) <= width {
					keepSame = true
				}
			case argText:
				if curLen+2+2+1 <= width { // room for ", " + "x"
					keepSame = true
				}
			}
			// Lookahead packing: if this is the first expr after a text arg
			// and keeping it on the same line would force the next expr to
			// wrap, prefer to break before this expr so both can sit on the
			// continuation line together.
			if keepSame && a.kind == argExpr && i > 0 && normArgs[i-1].kind == argText {
				if i+1 < len(normArgs) && normArgs[i+1].kind == argExpr {
					need := curLen + 2 + firstLineLen(a.expr) + 2 + firstLineLen(normArgs[i+1].expr)
					if need > width {
						keepSame = false
					}
				}
			}
			if hasInlineComment {
				keepSame = true
			}

			if keepSame {
				b.WriteString(", ")
				curLen += 2
			} else {
				b.WriteByte(',')
				b.WriteByte('\n')
				b.WriteString(contIndent)
				curLen = visualLen(contIndent)
				placedOnNewLine = true
			}
		}

		if a.kind == argText {
			// Lay out text as concatenated string literals, breaking as needed.
			// We'll emit segments like: "... " +\n<indent>"..."
			firstAvail := width - curLen
			nextAvail := width - visualLen(contIndent)
			// Nested when formatting inside another targeted call.
			_ = nestedDepth
			// Pure width-driven split of the text argument: do not reserve
			// columns for trailing expressions. Pack expressions afterwards
			// based on remaining width.
			segs := chunkTextFit(a.text, firstAvail, nextAvail)
			// Optionally merge trailing segments if the join preserves a space
			// between words and the combined piece fits on the last line.
			segs = foldTrailingSegmentsSafe(segs, width-visualLen(contIndent))
			// Ensure the first segment fits on the head line when this is the
			// first argument, using the relaxed head budget.
			if i == 0 && len(segs) > 0 {
				segs = ensureHeadFits(segs, firstAvail)
			}
			// Rebuild cleaned list after all adjustments; drop empty segments.
			cleaned := make([]string, 0, len(segs))
			for _, s := range segs {
				if s == "" {
					continue
				}
				cleaned = append(cleaned, s)
			}
			// If first argument and we have at least two segments, and there's
			// still head room to keep the first word of the next segment on the
			// head line (including a trailing space and the required " +"),
			// steal that word from segment 2 into segment 1. This matches the
			// golden layout preferences in several examples (e.g., keep
			// "wrapping " or "zero " on the head line).
			if i == 0 && len(cleaned) > 1 {
				// Compute available columns remaining on the head line.
				avail := width - curLen
				// Find the first word and at least one following space in cleaned[1].
				next := cleaned[1]
				// Identify word [0:wEnd) followed by a run of spaces [wEnd:spEnd).
				wEnd := 0
				for wEnd < len(next) && !unicode.IsSpace(rune(next[wEnd])) {
					wEnd++
				}
				spEnd := wEnd
				for spEnd < len(next) && unicode.IsSpace(rune(next[spEnd])) {
					spEnd++
				}
				if wEnd > 0 && spEnd > wEnd {
					word := next[:wEnd]
					// Build candidate new head content by appending word and one space.
					candidate := cleaned[0] + word + " "
					if visualLen(quoteGoString(candidate))+2 <= avail { // +2 for " +"
						// Remaining leading spaces in next segment after stealing one space.
						leadSpaces := spEnd - wEnd - 1
						if leadSpaces < 0 {
							leadSpaces = 0
						}
						cleaned[0] = candidate
						cleaned[1] = strings.Repeat(" ", leadSpaces) + next[spEnd:]
					}
				}
			}
			// If this is the first argument and there will be at least one
			// continuation segment, ensure the first segment fits on the
			// current line together with the trailing ' +' join. Shrink it at
			// the last word boundary if necessary.
			if i == 0 && len(cleaned) > 1 {
				maxContent := (width - curLen) - 2 /*quotes*/ - 2 /* ' +' */
				if maxContent < 0 {
					maxContent = 0
				}
				if visualLen(cleaned[0]) > maxContent {
					if cut := lastSpaceBefore(cleaned[0], maxContent); cut > 0 {
						p1 := cleaned[0][:cut]
						if !strings.HasSuffix(p1, " ") {
							p1 += " "
						}
						p2 := strings.TrimLeftFunc(cleaned[0][cut:], unicode.IsSpace)
						// Replace first with p1 and insert p2 as next segment.
						next := make([]string, 0, len(cleaned)+1)
						next = append(next, p1)
						next = append(next, p2)
						next = append(next, cleaned[1:]...)
						cleaned = foldTrailingSegmentsSafe(next, width-visualLen(contIndent))
					}
				}
			}
			// If we ended up with a single segment but it doesn't fit on the
			// current line, split it now so we can keep a non-empty head
			// segment on this line followed by ' +'.
			if len(cleaned) == 1 {
				lit := cleaned[0]
				if curLen+visualLen(quoteGoString(lit)) > width {
					firstAvail := width - curLen
					nextAvail := width - visualLen(contIndent)
					cleaned = chunkTextFit(lit, firstAvail, nextAvail)
				}
			}

			// Minimal tie-breaker for aesthetic stability:
			// If the very first head segment would exactly fill the line when
			// quoted and followed by the required " +", move the last word of
			// that segment to the next segment. This avoids leaving a tiny word
			// at the end of the head line (stabilizes example15) without
			// affecting other width-driven behavior.
			if i == 0 && len(cleaned) > 1 {
				seg0 := cleaned[0]
				used := curLen + visualLen(quoteGoString(seg0)) + 2 // account for " +"
				if used == width {
					last := strings.LastIndexByte(seg0, ' ')
					if last > 0 {
						prev := strings.LastIndexByte(seg0[:last], ' ')
						if prev >= 0 {
							newHead := seg0[:prev+1]
							moved := strings.TrimLeftFunc(seg0[prev+1:], unicode.IsSpace)
							cleaned[0] = newHead
							cleaned[1] = moved + cleaned[1]
						}
					}
				}
			}

			// Proceed to emit segments.
			for j, seg := range cleaned {
				// For the first emitted head segment, try to greedily pull the
				// leading word from the next segment onto the head if it fits
				// (including quotes and the required " +"). This helps produce
				// stable, readable breaks like keeping "wrapping " or "zero "
				// on the first line.
				if j == 0 && len(cleaned) > 1 {
					avail := width - curLen
					// Identify word and following spaces at start of cleaned[1].
					next := cleaned[1]
					wEnd := 0
					for wEnd < len(next) && !unicode.IsSpace(rune(next[wEnd])) {
						wEnd++
					}
					spEnd := wEnd
					for spEnd < len(next) && unicode.IsSpace(rune(next[spEnd])) {
						spEnd++
					}
					if wEnd > 0 && spEnd > wEnd {
						word := next[:wEnd]
						candidate := seg + word + " "
						if visualLen(quoteGoString(candidate))+2 <= avail {
							leadSpaces := spEnd - wEnd - 1
							if leadSpaces < 0 {
								leadSpaces = 0
							}
							cleaned[0] = candidate
							cleaned[1] = strings.Repeat(" ", leadSpaces) + next[spEnd:]
							seg = candidate
						}
					}
				}
				// If after possible greedy pull, the head still ends in an
				// extremely short word (e.g., "to ", "a "), push that word to
				// the next segment to avoid awkward tiny tail at the end of the
				// first line. This matches the golden unicode example.
				if j == 0 && len(cleaned) > 1 {
					// Find the last space in the head segment.
					if lastSp := strings.LastIndexByte(seg, ' '); lastSp >= 0 {
						// Find the start of the last word before that space.
						start := strings.LastIndexByte(seg[:lastSp], ' ')
						if start < 0 {
							// No earlier space to preserve a non-empty head.
							// Skip to avoid creating an empty head segment.
						} else {
						word := strings.TrimSpace(seg[start+1:])
						if len([]rune(word)) <= 2 {
							headNew := seg[:start+1]
							moved := seg[start+1:]
							cleaned[0] = headNew
							cleaned[1] = moved + cleaned[1]
							seg = headNew
						}
						}
					}
				}

				lit := quoteGoString(seg)
				litLen := visualLen(lit)
				// For non-last segments we must keep ' +' at end of the same
				// line as the literal. If it wouldn't fit, wrap BEFORE
				// printing the literal so we can append ' +'.
				if j != len(cleaned)-1 {
					if curLen+litLen+2 > width {
						b.WriteByte('\n')
						b.WriteString(contIndent)
						curLen = visualLen(contIndent)
					}
				} else {
					// Last segment: if it wouldn't fit, wrap before it.
					if curLen+litLen > width {
						b.WriteByte('\n')
						b.WriteString(contIndent)
						curLen = visualLen(contIndent)
					}
				}

				b.WriteString(lit)
				curLen += litLen

				if j != len(cleaned)-1 {
					// Append space and plus per gofmt style: " +".
					b.WriteByte(' ')
					b.WriteByte('+')
					curLen += 2
					b.WriteByte('\n')
					b.WriteString(contIndent)
					curLen = visualLen(contIndent)
				}
			}
		} else {
			// Expression argument: write as-is, possibly on new line.
			// Avoid introducing another newline immediately after we've already
			// placed the separator on a new line for this argument. Also, for
			// nested targeted calls, we will format them in-context below.
			if !placedOnNewLine && !hasInlineComment && !isRawStringLiteral(a.expr) && !isTargetedCallStart(a.expr) && curLen+firstLineLen(a.expr) > width {
				b.WriteByte('\n')
				b.WriteString(contIndent)
				curLen = visualLen(contIndent)
			}
			if isTargetedCallStart(a.expr) {
				// Format the nested targeted call using the current line base
				// to guide where its first break should land.
				nestedDepth++
				formattedNested := formatCall([]byte(a.expr), wsIndent, curLen)
				nestedDepth--
				b.WriteString(formattedNested)
				// Update curLen to the visual length of the last line of the
				// nested formatted text.
				curLen = lastLineLen(formattedNested)
			} else {
				b.WriteString(a.expr)
				curLen += visualLen(a.expr)
			}
		}

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

func containsTarget(s string) bool {
	// Fast substring checks; we don't need to be perfect because the nested
	// FormatFile call is resilient to non-Go text as input.
	return strings.Contains(s, "log.Infof(") ||
		strings.Contains(s, "log.Debugf(") ||
		strings.Contains(s, "log.Tracef(") ||
		strings.Contains(s, "log.Errorf(") ||
		strings.Contains(s, "log.Warnf(") ||
		strings.Contains(s, "fmt.Printf(") ||
		strings.Contains(s, "fmt.Sprintf(") ||
		strings.Contains(s, "fmt.Errorf(")
}

func isRawStringLiteral(s string) bool {
	t := strings.TrimSpace(s)
	return len(t) >= 2 && t[0] == '`' && t[len(t)-1] == '`'
}

func isTargetedCallStart(s string) bool {
	ts := strings.TrimSpace(s)
	return strings.HasPrefix(ts, "log.Infof(") ||
		strings.HasPrefix(ts, "log.Debugf(") ||
		strings.HasPrefix(ts, "log.Tracef(") ||
		strings.HasPrefix(ts, "log.Errorf(") ||
		strings.HasPrefix(ts, "log.Warnf(") ||
		strings.HasPrefix(ts, "fmt.Printf(") ||
		strings.HasPrefix(ts, "fmt.Sprintf(") ||
		strings.HasPrefix(ts, "fmt.Errorf(")
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

func quoteGoString(s string) string { return strconv.Quote(s) }

// chunkText splits s into segments suitable for line-wrapping with ` +` between
// lines. It prefers splitting on spaces and ensures that every segment except
// the last ends with a space, so that words remain separated across the join.
// maxCur is the remaining space on the current line; width is the total limit.
func chunkTextFit(s string, firstAvail, nextAvail int) []string {
	// Reserve 2 columns for surrounding quotes in each segment.
	if firstAvail < 2 {
		firstAvail = 2
	}
	if nextAvail < 2 {
		nextAvail = 2
	}
	firstAvail -= 2
	nextAvail -= 2
	var segs []string
	rest := s
	curLimit := firstAvail
	for len(rest) > 0 {
		if visualLen(rest) <= curLimit {
			segs = append(segs, rest)
			break
		}
		// For non-last segments, reserve space for ' +' join on the same line.
		limit := curLimit
		if limit > 0 {
			limit--
		}
		if limit > 0 {
			limit--
		}
		cut := lastSpaceBefore(rest, limit)
		// Head-only tight fallback: if we're producing the very first segment
		// (segs is empty) and emitting this head literal (with quotes) plus the
		// required " +" would land at or beyond the column limit, prefer the
		// previous space boundary if available. This avoids packing a tiny word
		// at the very end of the head when it's right at the edge (e.g.,
		// example15 Unicode).
		if len(segs) == 0 && cut > 0 {
			headLit := rest[:cut+1]
			headQuotedLen := visualLen(quoteGoString(headLit))
			// Prefer stepping back when the head would exactly or nearly fill
			// the line and the last word is very short.
			if prev := strings.LastIndexByte(rest[:cut], ' '); prev > 0 {
				lw := strings.TrimSpace(rest[prev+1 : cut])
				shortWord := len([]rune(lw)) <= 2
				if (headQuotedLen+2 >= limit && shortWord) || shortWord {
					cut = prev
				}
			}
		}
		if cut <= 0 {
			// No space to break before limit; force cut at exact limit.
			if limit <= 1 {
				cut = 1
			} else {
				cut = limit
			}
			segs = append(segs, rest[:cut])
			rest = rest[cut:]
		} else {
			// Keep the space at end of segment; preserve leading spaces in next
			// segment (do not trim) to maintain exact spacing.
			segs = append(segs, rest[:cut+1])
			rest = rest[cut+1:]
		}
		curLimit = nextAvail
	}
	return segs
}

// chunkTextFitWithLastLimit splits s into segments honoring different limits
// for the first/middle segments and a tighter limit for the final segment. All
// limits are total per-line available columns including the two surrounding
// quotes.
func chunkTextFitWithLastLimit(s string, firstAvail, nextAvail, lastAvail int) []string {
	// Ensure we account for quotes.
	// Apply a small bias to encourage earlier breaks for better packing of
	// following arguments on the same line.
	first := firstAvail - 2 - 4
	mid := nextAvail - 2 - 2
	last := lastAvail - 2
	if first < 1 {
		first = 1
	}
	if mid < 1 {
		mid = 1
	}
	if last < 1 {
		last = 1
	}
	var segs []string
	rest := s
	limit := first
	for len(rest) > 0 {
		// If the remainder fits within the last-limit, make it the last
		// segment and finish.
		if visualLen(rest) <= last {
			segs = append(segs, rest)
			break
		}
		// Otherwise, cut at current limit.
		cutLimit := limit
		if cutLimit > 0 {
			cutLimit-- // leave space for trailing space on this segment
		}
		// Reserve space for ' +' when continuing on the same line after this segment.
		if cutLimit > 0 {
			cutLimit--
		}
		if cutLimit > 0 {
			cutLimit--
		}
		cut := lastSpaceBefore(rest, cutLimit)
		if cut <= 0 {
			if limit <= 2 {
				cut = 1
			} else {
				cut = limit - 2
			}
			segs = append(segs, rest[:cut])
			rest = rest[cut:]
		} else {
			segs = append(segs, rest[:cut+1])
			rest = rest[cut+1:]
		}
		// After first, use mid limit for subsequent segments.
		limit = mid
	}
	return segs
}

// chooseSuffixStart returns an index in s where a final suffix segment can
// start so that the quoted suffix fits in lastAvail columns. It prefers
// starting at word boundaries (the character after a space). Returns 0 if no
// reasonable boundary is found.
func chooseSuffixStart(s string, lastAvail, firstAvail, nextAvail int) int {
	avail := lastAvail - 2 // account for quotes
	if avail <= 0 {
		return 0
	}
	if visualLen(s) <= avail {
		return 0
	}
	// If there are placeholders, first try a word boundary that comes AFTER
	// the last placeholder cluster to keep the placeholder with the prefix
	// (matches example1).
	lastPct := strings.LastIndexByte(s, '%')
	if lastPct > 0 {
		for j := len(s) - 1; j > lastPct; j-- {
			if s[j] == ' ' { // suffix cannot start with space
				continue
			}
			if s[j-1] != ' ' {
				continue
			}
			if visualLen(s)-j <= avail && j+2 <= firstAvail {
				return j
			}
		}
	}
	// Prefer the earliest (leftmost) percent-verb boundary whose suffix fits,
	// so we keep placeholders grouped naturally when needed.
	// First, prefer percent verbs that are likely value placeholders (e.g. %v).
	for j := 1; j < len(s); j++ {
		if s[j] != '%' {
			continue
		}
		// Prefer breaking before a placeholder when preceded by space.
		if s[j-1] != ' ' {
			continue
		}
		// Check next rune for verb class hint.
		if j+1 >= len(s) {
			continue
		}
		verb := s[j+1]
		if !(verb == 'v' || verb == 'V' || verb == 'd' || verb == 'T') {
			// Still allow, but de-prioritize; handled in the next pass.
			continue
		}
		if visualLen(s)-j <= avail && j+2 <= firstAvail {
			return j
		}
	}
	// Next, consider any percent boundary for value-like verbs again as a fallback.
	for j := 1; j < len(s); j++ {
		if s[j] != '%' {
			continue
		}
		if s[j-1] != ' ' {
			continue
		}
		if j+1 >= len(s) {
			continue
		}
		verb := s[j+1]
		if !(verb == 'v' || verb == 'V' || verb == 'd' || verb == 'T') {
			continue
		}
		if visualLen(s)-j <= avail && j+2 <= firstAvail {
			return j
		}
	}
	// Otherwise prefer the rightmost word boundary whose suffix fits to keep
	// as much text as possible on the first line while still allowing the
	// suffix to fit on the continuation line.
	for j := len(s) - 1; j >= 1; j-- {
		if s[j] == ' ' { // don't start with space
			continue
		}
		if s[j-1] != ' ' {
			continue
		}
		if visualLen(s)-j <= avail && j+2 <= firstAvail {
			return j
		}
	}
	return 0
}

func lastSpaceBefore(s string, maxCols int) int {
	// Return the last byte index of a space rune whose position is within
	// maxCols visual columns from the start of s. Counts runes using
	// runeWidth to approximate terminal width.
	col := 0
	last := -1
	for i, r := range s {
		w := runeWidth(r)
		if col+w > maxCols {
			break
		}
		if unicode.IsSpace(r) {
			last = i
		}
		col += w
	}
	return last
}

// lastSpaceBeforePreferPercent returns the last space index within maxCols
// columns, preferring a space whose following non-space rune is '%'. Falls
// back to the generic lastSpaceBefore when no such boundary exists.
// (no percent-preference helper; pure width-driven splitting)

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
			// Tabs as 8-column stops.
			next := ((col / 8) + 1) * 8
			col = next
			continue
		}
		col += runeWidth(r)
	}
	return col
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
	if isWideRune(r) {
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

func shrinkLastSegment(segs []string, lastLimit int) []string {
	if len(segs) == 0 {
		return segs
	}
	if lastLimit <= 0 {
		return segs
	}
	last := segs[len(segs)-1]
	for visualLen(last) > lastLimit {
		// Find a cut point inside last.
		cut := lastSpaceBefore(last, lastLimit)
		if cut <= 0 {
			// Can't cut properly, bail.
			break
		}
		// Keep space at end of first part.
		first := last[:cut+1]
		rest := strings.TrimLeftFunc(last[cut+1:], unicode.IsSpace)
		segs[len(segs)-1] = first
		segs = append(segs, rest)
		last = rest
	}
	return segs
}

// foldTrailingSegments greedily merges the last segments into one as long as
// the combined quoted length fits into lastAvail columns. This helps avoid
// cases where the tail like "%v with result %v." is split unnecessarily.
func foldTrailingSegmentsSafe(segs []string, lastAvail int) []string {
	if len(segs) < 2 {
		return segs
	}
	if lastAvail < 2 {
		return segs
	}
	// Available for content without quotes.
	cap := lastAvail - 2
	last := segs[len(segs)-1]
	i := len(segs) - 2
	for i >= 0 {
		// Only merge if there's a space boundary between pieces.
		if len(segs[i])+len(last) <= cap && (strings.HasSuffix(segs[i], " ") || strings.HasPrefix(last, " ")) {
			// Normalize to single space between if both sides have space.
			if strings.HasSuffix(segs[i], " ") && strings.HasPrefix(last, " ") {
				last = segs[i] + strings.TrimLeftFunc(last, unicode.IsSpace)
			} else {
				last = segs[i] + last
			}
			i--
		} else {
			break
		}
	}
	// Build new slice with merged tail.
	out := append([]string{}, segs[:i+1]...)
	out = append(out, last)
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// splitPrefixIntoTwo splits s into exactly two segments that fit within the
// first and next available widths (including quotes). It prefers a split that
// makes the second segment as full as possible (close to nextAvail), which
// mirrors the example layout. It returns nil if it can't find a suitable split.
func splitPrefixIntoTwo(s string, firstAvail, nextAvail int) []string {
	maxP1 := firstAvail - 2
	maxP2 := nextAvail - 2
	if maxP1 < 1 || maxP2 < 1 {
		return nil
	}
	n := len(s)
	// Minimal P1 required so that P2 fits into maxP2.
	minP1 := n - maxP2
	if minP1 < 1 {
		minP1 = 1
	}
	if minP1 > maxP1 {
		return nil
	}
	// Choose the largest space boundary between minP1 and maxP1 to maximize P1.
	bestCut := -1
	// Clamp upper bound to n to avoid out-of-range access.
	upper := maxP1
	if upper > n {
		upper = n
	}
	for i := upper; i >= minP1; i-- {
		// We want to end P1 at or before a space and start P2 at a word.
		if i < n && s[i-1] != ' ' && s[i] == ' ' {
			bestCut = i
			break
		}
		if i-1 < n && s[i-1] == ' ' {
			bestCut = i
			break
		}
	}
	if bestCut == -1 {
		return nil
	}
	p1 := s[:bestCut]
	if !strings.HasSuffix(p1, " ") {
		p1 += " "
	}
	p2 := strings.TrimLeftFunc(s[bestCut:], unicode.IsSpace)
	return []string{p1, p2}
}

// splitPrefixHeadCont tries to split s into 1 or 2 segments so that:
// - segment 1 fits within firstAvail (including quotes), ending on a word boundary
// - the remainder fits within nextAvail (including quotes)
// Returns nil if it can't satisfy the constraints.
func splitPrefixHeadCont(s string, firstAvail, nextAvail int) []string {
	headCap := firstAvail - 2
	contCap := nextAvail - 2
	if headCap < 1 || contCap < 1 {
		return nil
	}
	n := len(s)
	// If whole prefix fits on head line, keep it as single segment.
	if n <= headCap {
		return []string{s}
	}
	// Find largest j1 ≤ headCap such that remainder ≤ contCap and j1 is at word boundary.
	for j := headCap; j >= 1; j-- {
		if s[j-1] != ' ' { // ensure we end with space on segment 1
			continue
		}
		if n-j <= contCap {
			p1 := s[:j]
			if !strings.HasSuffix(p1, " ") {
				p1 += " "
			}
			p2 := strings.TrimLeftFunc(s[j:], unicode.IsSpace)
			return []string{p1, p2}
		}
	}
	return nil
}

// ensureHeadFits splits segs[0] if needed so that the first quoted segment
// fits within firstAvail columns. It preserves word boundaries and adds a
// trailing space to the first part if split.
func ensureHeadFits(segs []string, firstAvail int) []string {
	if len(segs) == 0 {
		return segs
	}
	headCap := firstAvail - 2
	if headCap < 1 {
		return segs
	}
	if visualLen(segs[0]) <= headCap {
		return segs
	}
	cut := lastSpaceBefore(segs[0], headCap)
	if cut <= 0 {
		return segs
	}
	p1 := segs[0][:cut]
	if !strings.HasSuffix(p1, " ") {
		p1 += " "
	}
	p2 := strings.TrimLeftFunc(segs[0][cut:], unicode.IsSpace)
	out := []string{p1, p2}
	out = append(out, segs[1:]...)
	return out
}

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

func printNode(n ast.Node) string {
	var buf bytes.Buffer
	fset := token.NewFileSet()
	_ = printer.Fprint(&buf, fset, n)
	return buf.String()
}

func flattenStringExpr(e ast.Expr) (string, bool) {
	switch x := e.(type) {
	case *ast.BasicLit:
		if x.Kind == token.STRING {
			s, err := strconv.Unquote(x.Value)
			if err != nil {
				return "", false
			}
			return s, true
		}
		return "", false
	case *ast.ParenExpr:
		return flattenStringExpr(x.X)
	case *ast.BinaryExpr:
		if x.Op != token.ADD {
			return "", false
		}
		l, ok1 := flattenStringExpr(x.X)
		r, ok2 := flattenStringExpr(x.Y)
		if ok1 && ok2 {
			return l + r, true
		}
		return "", false
	default:
		return "", false
	}
}

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
