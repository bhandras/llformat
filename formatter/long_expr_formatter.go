package formatter

import (
	"bytes"
	formatstd "go/format"
	"strings"

	"github.com/lightninglabs/llformat/scanner"
	"github.com/lightninglabs/llformat/width"
)

// LongExprConfig holds configuration for the long expression formatter.
type LongExprConfig struct {
	ColumnLimit   int
	TabStop       int
	MaxIterations int // Maximum iterations for breaking (default 5)

	// ParseSafe enables parse-safe behavior: a rewrite is only accepted if gofmt
	// succeeds on the candidate output. This prevents the formatter from
	// returning syntactically invalid Go when a heuristic rewrite goes wrong.
	ParseSafe bool
}

// LongExprFormatter breaks long expressions that exceed the column limit.
type LongExprFormatter struct {
	cfg LongExprConfig
}

// NewLongExprFormatter creates a new long expression formatter with defaults.
func NewLongExprFormatter(cfg LongExprConfig) *LongExprFormatter {
	if cfg.ColumnLimit <= 0 {
		cfg.ColumnLimit = 80
	}
	if cfg.TabStop <= 0 {
		cfg.TabStop = 8
	}
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = 20 // enough for files with many long lines
	}
	return &LongExprFormatter{cfg: cfg}
}

// FormatFile breaks long expressions in the source file.
func (f *LongExprFormatter) FormatFile(src []byte) []byte {
	// Iterate: break one expression per pass, run gofmt, check if still too long
	result := src
	skippedLines := make(map[int]bool) // Track lines we've tried and couldn't break

	for iter := 0; iter < f.cfg.MaxIterations; iter++ {
		// Find lines that exceed the column limit
		longLines := f.findLongLines(result)
		if len(longLines) == 0 {
			break // All lines fit
		}

		// Try to break a long line (skip ones we've already tried).
		// If ParseSafe is enabled, we keep trying other lines in the same pass
		// when a candidate rewrite would make gofmt fail.
		var modified []byte
		changed := false
		for _, lineInfo := range longLines {
			if skippedLines[lineInfo.lineNum] {
				continue
			}
			candidate, candidateChanged := f.breakLongLine(result, lineInfo)
			if !candidateChanged {
				// Couldn't break this line, remember to skip it.
				skippedLines[lineInfo.lineNum] = true
				continue
			}

			if !f.cfg.ParseSafe {
				modified = candidate
				changed = true
				break
			}

			// ParseSafe: only accept this rewrite if gofmt succeeds.
			if formatted, err := formatstd.Source(candidate); err == nil {
				result = formatted
				// Reset skipped lines since line numbers may have changed.
				skippedLines = make(map[int]bool)
				changed = true
				break
			}

			// Reject rewrite and keep searching.
			skippedLines[lineInfo.lineNum] = true
		}

		if !changed {
			break // No more lines we can break
		}

		// Run gofmt to normalize (only for non-ParseSafe mode, where we accept
		// candidate rewrites even if gofmt fails).
		if !f.cfg.ParseSafe {
			if formatted, err := formatstd.Source(modified); err == nil {
				result = formatted
				// Reset skipped lines since line numbers may have changed.
				skippedLines = make(map[int]bool)
			} else {
				result = modified
			}
		}
	}
	return result
}

// lineInfo contains information about a line that exceeds the column limit.
type lineInfo struct {
	lineNum    int
	start      int // byte offset of line start
	end        int // byte offset of line end (before newline)
	content    string
	visualLen  int
	indent     string
	indentLen  int
}

// findLongLines returns information about lines exceeding the column limit.
func (f *LongExprFormatter) findLongLines(src []byte) []lineInfo {
	var result []lineInfo
	lines := bytes.Split(src, []byte("\n"))
	offset := 0
	for i, line := range lines {
		content := string(line)
		vlen := width.VisualLenWithTab(content, f.cfg.TabStop)
		if vlen > f.cfg.ColumnLimit {
			// Extract indent
			indent := ""
			for _, c := range content {
				if c == ' ' || c == '\t' {
					indent += string(c)
				} else {
					break
				}
			}
			result = append(result, lineInfo{
				lineNum:   i,
				start:     offset,
				end:       offset + len(line),
				content:   content,
				visualLen: vlen,
				indent:    indent,
				indentLen: width.VisualLenWithTab(indent, f.cfg.TabStop),
			})
		}
		offset += len(line) + 1 // +1 for newline
	}
	return result
}

// breakLongLine attempts to break a long line at an appropriate point.
// Returns the modified source and whether a change was made.
func (f *LongExprFormatter) breakLongLine(src []byte, info lineInfo) ([]byte, bool) {
	line := info.content
	trimmed := strings.TrimLeft(line, " \t")

	// Skip comment-only lines
	if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
		return src, false
	}

	// Skip function/method signatures (these need different handling)
	if strings.HasPrefix(trimmed, "func ") && strings.HasSuffix(strings.TrimSpace(line), "{") {
		return src, false
	}

	// Check if this is a case statement (needs comma breaking)
	isCaseStmt := strings.HasPrefix(trimmed, "case ")

	// Try to handle string concatenation specially - combine and re-split
	if newLine, ok := f.tryReformatStringConcat(line, info.indent); ok {
		var result bytes.Buffer
		result.Write(src[:info.start])
		result.WriteString(newLine)
		if info.end < len(src) {
			result.Write(src[info.end:])
		}
		return result.Bytes(), true
	}

	// Try to find a break point
	breakPoint := f.findBreakPoint(line, info.indentLen, isCaseStmt)
	if breakPoint == nil {
		return src, false
	}

	// In Go, we must break AFTER the operator (can't have newline before binary op)
	// So we include the operator in beforeBreak and put the rest on the next line
	breakAfterIdx := breakPoint.pos + len(breakPoint.op)

	// Skip any whitespace after the operator
	afterIdx := breakAfterIdx
	for afterIdx < len(line) && (line[afterIdx] == ' ' || line[afterIdx] == '\t') {
		afterIdx++
	}

	beforeBreak := strings.TrimRight(line[:breakAfterIdx], " \t")
	afterBreak := line[afterIdx:]

	// Build new content with break
	var newContent strings.Builder
	newContent.WriteString(beforeBreak)
	newContent.WriteString("\n")
	newContent.WriteString(info.indent)
	newContent.WriteString("\t") // continuation indent
	newContent.WriteString(afterBreak)

	// Replace the line in source
	var result bytes.Buffer
	result.Write(src[:info.start])
	result.WriteString(newContent.String())
	if info.end < len(src) {
		result.Write(src[info.end:])
	}

	return result.Bytes(), true
}

// findBreakPoint finds the best position to break a long line.
// Returns the break candidate or nil if no suitable break point.
func (f *LongExprFormatter) findBreakPoint(line string, indentLen int, isCaseStmt bool) *breakCandidate {
	// Priority order for break points:
	// 0. Comma (for case statements)
	// 1. Before || (lowest precedence logical)
	// 2. Before && (higher precedence logical)
	// 3. Before comparison operators (==, !=, <, >, <=, >=)
	// 4. Before arithmetic operators (+, -, *, /)
	// 5. Before . in method chains (when followed by identifier and ()

	// We want to find the rightmost operator that keeps the first part under the limit

	b := []byte(line)
	candidates := []breakCandidate{}

	for i := 0; i < len(b); {
		// Skip strings
		if scanner.IsStringStart(b, i) {
			i = scanner.ScanString(b, i)
			continue
		}
		// Skip comments
		if scanner.IsLineCommentStart(b, i) {
			break // rest of line is comment
		}
		if scanner.IsBlockCommentStart(b, i) {
			i = scanner.ScanBlockComment(b, i)
			continue
		}

		// Comma for case statements (highest priority for case)
		if isCaseStmt && b[i] == ',' {
			candidates = append(candidates, breakCandidate{pos: i, priority: 0, op: ","})
			i++
			continue
		}

		// Check for operators
		if i+1 < len(b) && b[i] == '|' && b[i+1] == '|' {
			candidates = append(candidates, breakCandidate{pos: i, priority: 1, op: "||"})
			i += 2
			continue
		}
		if i+1 < len(b) && b[i] == '&' && b[i+1] == '&' {
			candidates = append(candidates, breakCandidate{pos: i, priority: 2, op: "&&"})
			i += 2
			continue
		}
		// Comparison operators - only add as candidates if not followed by a simple literal
		// This avoids breaking "x > 0" into "x >" and "0"
		if i+1 < len(b) && b[i] == '=' && b[i+1] == '=' {
			if !isFollowedBySimpleLiteral(b, i+2) {
				candidates = append(candidates, breakCandidate{pos: i, priority: 3, op: "=="})
			}
			i += 2
			continue
		}
		if i+1 < len(b) && b[i] == '!' && b[i+1] == '=' {
			if !isFollowedBySimpleLiteral(b, i+2) {
				candidates = append(candidates, breakCandidate{pos: i, priority: 3, op: "!="})
			}
			i += 2
			continue
		}
		if i+1 < len(b) && b[i] == '<' && b[i+1] == '=' {
			if !isFollowedBySimpleLiteral(b, i+2) {
				candidates = append(candidates, breakCandidate{pos: i, priority: 3, op: "<="})
			}
			i += 2
			continue
		}
		if i+1 < len(b) && b[i] == '>' && b[i+1] == '=' {
			if !isFollowedBySimpleLiteral(b, i+2) {
				candidates = append(candidates, breakCandidate{pos: i, priority: 3, op: ">="})
			}
			i += 2
			continue
		}
		if b[i] == '<' && (i+1 >= len(b) || b[i+1] != '<') {
			if !isFollowedBySimpleLiteral(b, i+1) {
				candidates = append(candidates, breakCandidate{pos: i, priority: 3, op: "<"})
			}
			i++
			continue
		}
		if b[i] == '>' && (i+1 >= len(b) || b[i+1] != '>') {
			if !isFollowedBySimpleLiteral(b, i+1) {
				candidates = append(candidates, breakCandidate{pos: i, priority: 3, op: ">"})
			}
			i++
			continue
		}
		// Arithmetic/binary operators (but not in +=, -=, etc.)
		if b[i] == '+' && (i+1 >= len(b) || b[i+1] != '=') && (i == 0 || b[i-1] != '+') {
			candidates = append(candidates, breakCandidate{pos: i, priority: 4, op: "+"})
			i++
			continue
		}
		if b[i] == '-' && (i+1 >= len(b) || b[i+1] != '=') && (i == 0 || b[i-1] != '-') {
			// Avoid breaking unary minus
			if i > 0 && !isOperatorContext(b, i-1) {
				candidates = append(candidates, breakCandidate{pos: i, priority: 4, op: "-"})
			}
			i++
			continue
		}
		// Method chain breaking is disabled - let multiline call formatter handle these
		// by expanding the function call arguments instead of breaking the chain
		// if b[i] == '.' && i+1 < len(b) && isIdentStart(b[i+1]) {
		// 	...
		// }
		i++
	}

	if len(candidates) == 0 {
		return nil
	}

	// Find the best candidate: highest priority (lowest number) that keeps first part under limit
	// We want to break at the RIGHTMOST position that fits within the column limit
	// Since we break AFTER the operator, include the operator length in the prefix calculation

	var best *breakCandidate
	var bestPrefixLen int
	for i := range candidates {
		c := &candidates[i]
		// Include the operator in the prefix since we break AFTER it
		prefixLen := width.VisualLenWithTab(line[:c.pos+len(c.op)], f.cfg.TabStop)
		if prefixLen <= f.cfg.ColumnLimit {
			// Prefer: 1) higher priority (lower number), 2) rightmost position (larger prefixLen)
			if best == nil || c.priority < best.priority ||
				(c.priority == best.priority && prefixLen > bestPrefixLen) {
				best = c
				bestPrefixLen = prefixLen
			}
		}
	}

	if best != nil {
		return best
	}

	// Fallback: just pick the first candidate that fits
	for i := range candidates {
		c := &candidates[i]
		prefixLen := width.VisualLenWithTab(line[:c.pos+len(c.op)], f.cfg.TabStop)
		if prefixLen <= f.cfg.ColumnLimit {
			return &candidates[i]
		}
	}
	return &candidates[0]
}

type breakCandidate struct {
	pos      int
	priority int
	op       string
}

// isOperatorContext checks if position i-1 is likely an operator context (for unary detection)
func isOperatorContext(b []byte, i int) bool {
	if i < 0 {
		return true
	}
	c := b[i]
	return c == '(' || c == '[' || c == '{' || c == ',' || c == '=' ||
		c == '+' || c == '-' || c == '*' || c == '/' || c == '%' ||
		c == '<' || c == '>' || c == '&' || c == '|' || c == '^' ||
		c == '!' || c == ':' || c == ';'
}

func isIdentStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

// isFollowedBySimpleLiteral checks if the position after the operator is followed
// by a simple literal (number, true, false, nil) that would look bad on its own line.
// This prevents breaking "x > 0" into "x >" and "0".
func isFollowedBySimpleLiteral(b []byte, pos int) bool {
	// Skip whitespace
	for pos < len(b) && (b[pos] == ' ' || b[pos] == '\t') {
		pos++
	}
	if pos >= len(b) {
		return false
	}

	// Check for number literal
	if b[pos] >= '0' && b[pos] <= '9' {
		// Scan past the number
		end := pos
		for end < len(b) && (b[end] >= '0' && b[end] <= '9' || b[end] == '.') {
			end++
		}
		// Check what follows the number - if it's end of expression or simple terminator, it's simple
		for end < len(b) && (b[end] == ' ' || b[end] == '\t') {
			end++
		}
		if end >= len(b) || b[end] == ')' || b[end] == ',' || b[end] == '{' ||
			b[end] == '&' || b[end] == '|' {
			return true
		}
		return false
	}

	// Check for true, false, nil
	keywords := []string{"true", "false", "nil"}
	for _, kw := range keywords {
		if pos+len(kw) <= len(b) && string(b[pos:pos+len(kw)]) == kw {
			end := pos + len(kw)
			// Make sure it's not part of a longer identifier
			if end < len(b) && isIdentChar(b[end]) {
				continue
			}
			// Check what follows
			for end < len(b) && (b[end] == ' ' || b[end] == '\t') {
				end++
			}
			if end >= len(b) || b[end] == ')' || b[end] == ',' || b[end] == '{' ||
				b[end] == '&' || b[end] == '|' {
				return true
			}
		}
	}

	return false
}

// tryReformatStringConcat checks if the line contains a string concatenation
// expression that can be combined and re-split for better formatting.
// Returns the reformatted line and true if successful.
func (f *LongExprFormatter) tryReformatStringConcat(line, indent string) (string, bool) {
	// Look for pattern: prefix + "string" + "string" + ... + "string"
	// We need to find the string concatenation part and extract it

	// Find where the string concatenation starts (first quote after = or return)
	trimmed := strings.TrimLeft(line, " \t")
	var prefix string
	var exprStart int

	if strings.HasPrefix(trimmed, "return ") {
		prefix = indent + "return "
		exprStart = strings.Index(line, "return ") + 7
	} else if idx := strings.Index(trimmed, " = "); idx != -1 {
		// Assignment
		eqPos := strings.Index(line, " = ")
		if eqPos == -1 {
			return "", false
		}
		prefix = line[:eqPos+3]
		exprStart = eqPos + 3
	} else if idx := strings.Index(trimmed, " := "); idx != -1 {
		// Short assignment
		eqPos := strings.Index(line, " := ")
		if eqPos == -1 {
			return "", false
		}
		prefix = line[:eqPos+4]
		exprStart = eqPos + 4
	} else {
		return "", false
	}

	// Skip whitespace after prefix
	for exprStart < len(line) && (line[exprStart] == ' ' || line[exprStart] == '\t') {
		exprStart++
	}

	expr := strings.TrimSpace(line[exprStart:])

	// Check if this is a string concatenation we can flatten
	combined, ok := flattenStringConcat(expr)
	if !ok {
		return "", false
	}

	// Calculate starting column for the string
	prefixLen := width.VisualLenWithTab(prefix, f.cfg.TabStop)

	// Build the reformatted string with proper splitting
	reformatted := buildSplitQuoted(combined, prefixLen, indent, f.cfg.ColumnLimit)

	return prefix + reformatted, true
}

// flattenStringConcat attempts to flatten a string concatenation expression.
// Only handles simple cases of "str1" + "str2" + ... without embedded expressions.
func flattenStringConcat(expr string) (string, bool) {
	// Must contain + and start with a string
	if !strings.Contains(expr, "+") {
		return "", false
	}

	expr = strings.TrimSpace(expr)
	if len(expr) == 0 || expr[0] != '"' {
		return "", false
	}

	// Split by + and check each part is a string literal
	var combined strings.Builder
	parts := splitStringConcat(expr)
	if len(parts) < 2 {
		return "", false
	}

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if len(part) < 2 || part[0] != '"' || part[len(part)-1] != '"' {
			return "", false
		}
		// Unquote and append
		content := part[1 : len(part)-1]
		combined.WriteString(content)
	}

	return combined.String(), true
}

// splitStringConcat splits a string concatenation by + operators,
// being careful to not split inside string literals.
func splitStringConcat(expr string) []string {
	var parts []string
	var current strings.Builder
	inString := false
	escaped := false

	for i := 0; i < len(expr); i++ {
		c := expr[i]

		if escaped {
			current.WriteByte(c)
			escaped = false
			continue
		}

		if c == '\\' && inString {
			current.WriteByte(c)
			escaped = true
			continue
		}

		if c == '"' {
			inString = !inString
			current.WriteByte(c)
			continue
		}

		if !inString && c == '+' {
			part := strings.TrimSpace(current.String())
			if part != "" {
				parts = append(parts, part)
			}
			current.Reset()
			continue
		}

		current.WriteByte(c)
	}

	// Don't forget the last part
	part := strings.TrimSpace(current.String())
	if part != "" {
		parts = append(parts, part)
	}

	return parts
}
