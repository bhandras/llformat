package formatter

import (
	"bytes"
	"go/ast"
	formatstd "go/format"
	"go/parser"
	"go/token"
	"strings"
	"unicode"
	"unicode/utf8"

	llast "github.com/lightninglabs/llformat/ast"
	"github.com/lightninglabs/llformat/scanner"
	"github.com/lightninglabs/llformat/text"
	"github.com/lightninglabs/llformat/width"
)

// Formatter defines a generic source formatter.
type Formatter interface {
	FormatFile(src []byte) []byte
}

// Config holds configuration for the compact packing formatter.
type Config struct {
	ColumnLimit int
	TabStop     int
	Targets     []string
	// FallbackNonTargets enables formatting of non-targeted function calls
	// that exceed the column limit using a packed multi-line style.
	FallbackNonTargets bool

	// UseASTSelection switches this legacy call formatter from scan-based call
	// detection to AST-based call selection. The formatting logic remains the
	// same; only the "which calls do we consider next?" selector changes.
	//
	// This is intentionally opt-in to preserve golden fixtures.
	UseASTSelection bool

	// SkipGofmt skips the internal gofmt pass, useful when running in a pipeline
	// that will run gofmt at the end.
	SkipGofmt bool

	// ParseSafe enables parse-safe behavior: if the formatter's output does not
	// successfully gofmt, the original input is returned unchanged. This avoids
	// returning syntactically invalid Go when a heuristic rewrite goes wrong.
	ParseSafe bool
}

// CompactCallFormatter implements compact packing formatting for function calls.
type CompactCallFormatter struct{ cfg Config }

// OwnedSpans returns the spans of calls that the compact call formatter would
// consider formatting. In the legacy pipeline, this stage typically runs
// before expression formatting, but exposing ownership allows the pipeline to
// enforce boundaries regardless of stage order.
func (f *CompactCallFormatter) OwnedSpans(src []byte) llast.OffsetSpanSet {
	// For now, treat all scan-selectable calls as owned when fallback is
	// enabled, and targeted calls as owned otherwise. This errs on the side of
	// preventing stage fighting.
	owned := make([]llast.OffsetSpan, 0, 64)

	i := 0
	for i < len(src) {
		if scanner.IsStringStart(src, i) {
			i = scanner.ScanString(src, i)
			continue
		}
		if scanner.IsLineCommentStart(src, i) {
			i = scanner.ScanLineComment(src, i)
			continue
		}
		if scanner.IsBlockCommentStart(src, i) {
			i = scanner.ScanBlockComment(src, i)
			continue
		}

		matched := ""
		for _, t := range f.cfg.Targets {
			if hasPrefixAt(src, i, t) {
				matched = t
				break
			}
		}
		if matched != "" {
			openIdx := i + len(matched) - 1
			endIdx := scanner.ScanBalancedParen(src, openIdx)
			if endIdx > openIdx {
				owned = append(owned, llast.OffsetSpan{Start: i, End: endIdx + 1})
				i = endIdx + 1
				continue
			}
		}

		if f.cfg.FallbackNonTargets {
			if start, end := findGenericCallAt(src, i); end > start {
				owned = append(owned, llast.OffsetSpan{Start: start, End: end})
				i = end
				continue
			}
		}

		i++
	}

	return llast.NewOffsetSpanSet(owned)
}

// NewCompactCallFormatter creates a new compact packing formatter with defaults.
func NewCompactCallFormatter(cfg Config) *CompactCallFormatter {
	if cfg.ColumnLimit <= 0 {
		cfg.ColumnLimit = 80
	}
	if cfg.TabStop <= 0 {
		cfg.TabStop = 8
	}
	if len(cfg.Targets) == 0 {
		cfg.Targets = defaultTargets()
	}
	return &CompactCallFormatter{cfg: cfg}
}

// Package-level defaults (used by compatibility wrapper and helpers).
var columnLimit = 80
var tabStop = 8
var fallbackNonTargets = false
var skipGofmt = false

func defaultTargets() []string {
	return []string{
		"log.Infof(", "log.Debugf(", "log.Tracef(", "log.Errorf(", "log.Warnf(",
		"fmt.Printf(", "fmt.Sprintf(", "fmt.Errorf(",
	}
}

// FormatFile applies formatting with default config and default targets. This
// is a convenience wrapper for callers that don't need custom config.
func FormatFile(src []byte) []byte {
	formatGlobalsMu.Lock()
	defer formatGlobalsMu.Unlock()

	// Reset to defaults for single-shot formatting.
	columnLimit = 80
	tabStop = 8
	return formatWithTargets(src, defaultTargets())
}

// FormatFile formats with the formatter's config.
func (f *CompactCallFormatter) FormatFile(src []byte) []byte {
	formatGlobalsMu.Lock()
	defer formatGlobalsMu.Unlock()

	// Apply config to package-level parameters used by helpers.
	if f.cfg.ColumnLimit > 0 {
		columnLimit = f.cfg.ColumnLimit
	}
	if f.cfg.TabStop > 0 {
		tabStop = f.cfg.TabStop
	}
	fallbackNonTargets = f.cfg.FallbackNonTargets
	skipGofmt = f.cfg.SkipGofmt
	currentTargets = f.cfg.Targets

	var out []byte
	if f.cfg.UseASTSelection {
		out = formatWithTargetsAST(src, f.cfg.Targets)
	} else {
		out = formatWithTargetsScan(src, f.cfg.Targets)
	}

	if f.cfg.ParseSafe {
		// Validate parseability using go/format (it parses and pretty-prints).
		// We intentionally keep the original output (rather than the formatted
		// result) to avoid changing stage interactions in pipelines that rely on
		// raw source layout for selection heuristics.
		if _, err := formatstd.Source(out); err == nil {
			return out
		}
		return src
	}

	return out
}

// Core formatting driver given a target signature list.
var currentTargets []string

func formatWithTargets(src []byte, targets []string) []byte {
	return formatWithTargetsScan(src, targets)
}

func formatWithTargetsScan(src []byte, targets []string) []byte {
	currentTargets = targets
	// We'll scan for target callsites and rewrite them in-place into a
	// buffer.
	var out bytes.Buffer
	i := 0
	for i < len(src) {
		// Try to match any target at this position (skipping when
		// inside string/comment handled by a lightweight scanner).
		if scanner.IsStringStart(src, i) {
			// Copy string literal as-is.
			start := i
			i = scanner.ScanString(src, i)
			out.Write(src[start:i])
			continue
		}
		if scanner.IsLineCommentStart(src, i) {
			start := i
			i = scanner.ScanLineComment(src, i)
			out.Write(src[start:i])
			continue
		}
		if scanner.IsBlockCommentStart(src, i) {
			start := i
			i = scanner.ScanBlockComment(src, i)
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
			// If enabled, try fallback formatting for non-target calls.
			if fallbackNonTargets {
				if start, end := findGenericCallAt(src, i); end > start {
					// Evaluate whether the call should be wrapped. Consider both
					// an estimated single-line width (collapsing whitespace) and
					// whether it already spans multiple lines.
					lineStart := text.LastLineStart(src, start)
					indentPrefix := string(src[lineStart:start])
					// Heuristic: when the call is part of a short method chain
					// right after a closing paren, ignore the preceding ')'
					// and optional '.' for width calculation so short calls
					// like ".Limit(100)" stay on one line.
					tp := strings.TrimSpace(indentPrefix)
					if tp == ")" || tp == ")." {
						indentPrefix = string(text.LeadingWhitespace(src, lineStart))
					}
					callText := string(src[start:end])
					// Collapse all whitespace to single spaces to approximate a
					// single-line rendering width.
					flat := strings.Join(strings.Fields(stripComments(callText)), " ")
					singleLineLen := visualLen(flat)
					currentLineLen := visualLen(indentPrefix) + singleLineLen
					needsWrap := currentLineLen > columnLimit
					if needsWrap && !isChainedShortCall(src, start, end) {
						// Format as packed multi-line and align closing paren with function name.
						wsIndent := string(text.LeadingWhitespace(src, lineStart))
						formatted := formatCallPackedMultiLine(src[start:end], wsIndent, wsIndent, true)
						out.WriteString(formatted)
						i = end
						continue
					}
					// Keep as-is when within limit.
					out.Write(src[start:end])
					i = end
					continue
				}
				if text.IsIdentifierStart(src[i]) {
					j := i + 1
					for j < len(src) && text.IsIdentifierChar(src[j]) {
						j++
					}
					out.Write(src[i:j])
					i = j
					continue
				}
			}
			out.WriteByte(src[i])
			i++
			continue
		}

		// We found a target call. Find its full extent (balanced
		// parentheses).
		callStart := i
		// Find the opening parenthesis index right after the target.
		openIdx := callStart + len(matched) - 1 // points to '('
		endIdx := scanner.ScanBalancedParen(src, openIdx)
		if endIdx <= openIdx {
			// Could not find a balanced call; copy verbatim and
			// continue to avoid mangling.
			out.Write(src[callStart : callStart+len(matched)])
			i = callStart + len(matched)
			continue
		}

		// Split around to get indent and call head.
		lineStart := text.LastLineStart(src, callStart)
		// indentBytes is the entire slice from line start to call start
		// (may include non-whitespace like "return ").
		indentBytes := src[lineStart:callStart]
		// wsIndent is only the leading whitespace of the line.
		wsIndent := text.LeadingWhitespace(src, lineStart)

		// Build formatted call.
		formatted := formatCallGreedy(src[callStart:endIdx+1], string(wsIndent), visualLen(string(indentBytes)))
		out.WriteString(formatted)
		i = endIdx + 1
	}
	res := out.Bytes()

	if skipGofmt {
		return res
	}
	if formatted, err := formatstd.Source(res); err == nil {
		return formatted
	}
	return res
}

// isChainedShortCall returns true if the call at [start,end) appears as a short
// method/function chained after ") ." and the call text itself is short.
func isChainedShortCall(src []byte, start, end int) bool {
	// Look back to previous non-space rune.
	i := start - 1
	for i >= 0 && (src[i] == ' ' || src[i] == '\t') {
		i--
	}
	if i < 1 {
		return false
	}
	// Allow ")." just before the call.
	if src[i] != '.' {
		return false
	}
	j := i - 1
	for j >= 0 && (src[j] == ' ' || src[j] == '\t') {
		j--
	}
	if j < 0 || src[j] != ')' {
		return false
	}
	// Consider the call short if its visual width is small.
	callText := string(src[start:end])
	if visualLen(callText) <= 16 { // heuristic threshold
		return true
	}
	return false
}

// findGenericCallAt attempts to find a function call starting at position i.
// It matches identifiers (including selectors like pkg.Func) immediately
// followed by '(', and excludes function/method declarations.
func findGenericCallAt(src []byte, i int) (start int, end int) {
	if i >= len(src) || !text.IsIdentifierStart(src[i]) {
		return 0, 0
	}
	// Identify full name (allow selectors).
	j := i
	for j < len(src) && (text.IsIdentifierChar(src[j]) || src[j] == '.') {
		j++
	}
	// Reject incomplete selector chains like `pkg.` or `x.`. This prevents
	// mis-detecting type assertions (`x.(T)`) as calls on `x.`.
	if j > i && src[j-1] == '.' {
		return 0, 0
	}
	// Skip language keywords (e.g., import, var) which are not calls.
	if text.IsKeyword(string(src[i:j])) {
		return 0, 0
	}
	// Skip whitespace
	for j < len(src) && (src[j] == ' ' || src[j] == '\t') {
		j++
	}
	if j >= len(src) || src[j] != '(' {
		return 0, 0
	}
	// Exclude function/method definitions (look back for "func").
	if isFunctionDefinitionAt(src, i) {
		return 0, 0
	}
	// Find matching ')'
	endIdx := scanner.ScanBalancedParen(src, j)
	if endIdx <= j {
		return 0, 0
	}
	return i, endIdx + 1
}

// isFunctionDefinitionAt checks if this looks like a function or method definition.
func isFunctionDefinitionAt(src []byte, i int) bool {
	j := i - 1
	for j >= 0 && (src[j] == ' ' || src[j] == '\t') {
		j--
	}
	if j >= 3 && string(src[j-3:j+1]) == "func" {
		return true
	}
	// Also scan back on the line for a method definition like: func (r R) Name(
	k := j
	for k >= 0 && src[k] != '\n' {
		if k >= 3 && string(src[k-3:k+1]) == "func" {
			return true
		}
		k--
	}
	return false
}

// stripComments removes // and /* */ comments from s while preserving strings.
func stripComments(s string) string {
	var b strings.Builder
	inStr := byte(0)
	esc := false
	i := 0
	for i < len(s) {
		c := s[i]
		if inStr != 0 {
			b.WriteByte(c)
			if inStr == '"' && c == '\\' && !esc {
				esc = true
				i++
				continue
			}
			if esc {
				esc = false
				i++
				continue
			}
			if c == inStr {
				inStr = 0
			}
			i++
			continue
		}
		if c == '"' || c == '`' {
			inStr = c
			b.WriteByte(c)
			i++
			continue
		}
		if c == '/' && i+1 < len(s) {
			if s[i+1] == '/' {
				for i < len(s) && s[i] != '\n' {
					i++
				}
				continue
			}
			if s[i+1] == '*' {
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
		b.WriteByte(c)
		i++
	}
	return b.String()
}

// formatCallPackedMultiLine formats a generic function call into a packed
// multi-line style when the single-line form would exceed the column limit.
// Rules:
//   - Emit head and opening paren, then a newline.
//   - Pack arguments separated by ", " on continuation lines of wsIndent+"\t".
//   - Break to a new continuation line when adding the next arg would exceed the
//     column limit; place the comma at the end of the current line.
//   - Always place the closing ")" on its own line aligned with the start of the
//     function name (using closingIndent from the source start-of-line to call).
//   - Emit a trailing comma after the final argument to stabilize layout.
func formatCallPackedMultiLine(call []byte, wsIndent, fullPrefix string, trailingComma bool) string {
	s := string(call)
	open := strings.IndexByte(s, '(')
	if open == -1 || !strings.HasSuffix(s, ")") {
		return s
	}
	head := s[:open]
	argsBody := s[open+1 : len(s)-1]
	// Split arguments respecting all bracket types so composites aren't broken.
	args := scanner.SplitTopLevelAny(argsBody)
	// Closing paren aligns with the line's whitespace indent (house style).
	closingIndent := wsIndent

	if len(args) == 0 {
		var b strings.Builder
		b.WriteString(head)
		b.WriteString("(\n")
		b.WriteString(closingIndent)
		b.WriteByte(')')
		return b.String()
	}

	width := columnLimit
	contIndent := wsIndent + "\t"
	contIndentLen := visualLen(contIndent)
	curLenAfterWrite := func(s string) int {
		if strings.Contains(s, "\n") {
			// Multiline formatted args include their own indentation on continuation
			// lines, so the last line length already reflects the active column.
			return lastLineLen(s)
		}
		return contIndentLen + firstLineLen(s)
	}

	var b strings.Builder
	b.WriteString(head)
	b.WriteByte('(')
	b.WriteByte('\n')

	curLen := contIndentLen
	// Emit first arg on a fresh continuation line (no leading comma).
	first := true
	prevWasMultiline := false
	prevWasCall := false
	// Track if we've seen a multiline call or composite.
	seenMultilineCall := false      // After multiline call, all args break
	seenMultilineComposite := false // After composite then single-line, break
	for idx, raw := range args {
		a := strings.TrimSpace(raw)
		if a == "" {
			continue
		}
		// After a multiline call expression, always break before next arg.
		// After a multiline composite, the break logic is handled later in shouldBreak.
		forcedBreak := !first && prevWasMultiline && prevWasCall
		// Handle string literals (double-quoted) with potential splitting.
		if e, err := parser.ParseExpr(a); err == nil {
			if text, ok := llast.FlattenStringExprAST(e); ok {
				stringWidth := width
				if first || forcedBreak {
					// Emit at continuation indent; split using contIndentLen as start.
					split := buildSplitQuoted(text, contIndentLen, contIndent, stringWidth)
					if !first {
						b.WriteByte(',')
						b.WriteByte('\n')
						b.WriteString(contIndent)
					} else {
						b.WriteString(contIndent)
					}
					b.WriteString(split)
					curLen = curLenAfterWrite(split)
					first = false
					prevWasMultiline, prevWasCall = false, false
					continue
				}
				// Not first: decide placement.
				// 1) If the plain quoted string fits on current line after ", ", keep it.
				plain := quoteGoString(text)
				if advanceCols(curLen+2, plain) <= width {
					b.WriteString(", ")
					b.WriteString(plain)
					curLen = advanceCols(curLen+2, plain)
					prevWasMultiline, prevWasCall = false, false
				} else {
					// Decide whether to keep it whole on a fresh line or split.
					effectiveWidth := stringWidth
					hasMore := false
					for j := idx + 1; j < len(args); j++ {
						if strings.TrimSpace(args[j]) != "" {
							hasMore = true
							break
						}
					}
					if !hasMore && advanceCols(contIndentLen, plain) <= effectiveWidth {
						// Last argument: prefer placing whole on the next line.
						b.WriteByte(',')
						b.WriteByte('\n')
						b.WriteString(contIndent)
						b.WriteString(plain)
						curLen = contIndentLen + firstLineLen(plain)
						first = false
						prevWasMultiline, prevWasCall = false, false
						continue
					}
					// 3) Finally, split across multiple lines.
					tentative := buildSplitQuoted(text, curLen+2, contIndent, effectiveWidth)
					need := 2 + firstLineLen(tentative)
					if curLen+need <= effectiveWidth {
						b.WriteString(", ")
						b.WriteString(tentative)
						if strings.Contains(tentative, "\n") {
							curLen = lastLineLen(tentative)
							prevWasMultiline, prevWasCall = false, false
						} else {
							curLen += need
							prevWasMultiline, prevWasCall = false, false
						}
					} else {
						b.WriteByte(',')
						b.WriteByte('\n')
						b.WriteString(contIndent)
						curLen = contIndentLen
						split := buildSplitQuoted(text, contIndentLen, contIndent, effectiveWidth)
						b.WriteString(split)
						curLen = curLenAfterWrite(split)
						prevWasMultiline, prevWasCall = false, false
					}
				}
				first = false
				continue
			}
		}
		// Pretty-format composite literals only for keyed maps/structs; keep
		// slices/arrays inline to avoid over-wrapping short literals.
		// If we've already seen a multiline call, force expand maps/structs.
		if fa, ok := FormatCompositeLiteralArg(a, contIndent, seenMultilineCall); ok {
			a = fa
		}
		// If the argument is itself a call expression and it doesn't fit,
		// recursively format it in packed multiline style without adding a
		// trailing comma inside.
		if llast.IsCallExpr(a) {
			// Greedy, algorithmic rule for nested calls: inline if the entire
			// call fits on the current line and it contains no always-multiline
			// composites and no nested calls among its direct args; otherwise
			// reflow recursively and start on a fresh continuation line.
			fits := false
			if first {
				fits = advanceCols(contIndentLen, a) <= width
			} else {
				fits = advanceCols(curLen+2, a) <= width
			}
			hasAlways := callHasAlwaysMultilineComposite(a)
			hasNested := llast.HasNestedCall(a)

			if fits && !hasAlways && !hasNested {
				if first {
					b.WriteString(contIndent)
					b.WriteString(a)
					curLen = contIndentLen + firstLineLen(a)
					first = false
				} else {
					if forcedBreak {
						b.WriteByte(',')
						b.WriteByte('\n')
						b.WriteString(contIndent)
						curLen = contIndentLen
					} else {
						b.WriteString(", ")
					}
					b.WriteString(a)
					curLen = advanceCols(curLen+2, a)
				}
				prevWasMultiline, prevWasCall = false, false
				continue
			}
			nested := formatCallPackedMultiLine([]byte(a), contIndent, contIndent, true)
			if first {
				b.WriteString(contIndent)
				b.WriteString(nested)
				curLen = curLenAfterWrite(nested)
				first = false
				nestedMulti := strings.Contains(nested, "\n")
				prevWasMultiline = nestedMulti
				prevWasCall = true
				if nestedMulti {
					seenMultilineCall = true
				}
				continue
			}
			if forcedBreak {
				b.WriteByte(',')
				b.WriteByte('\n')
				b.WriteString(contIndent)
				curLen = contIndentLen
				b.WriteString(nested)
				curLen = curLenAfterWrite(nested)
				first = false
				nestedMulti := strings.Contains(nested, "\n")
				prevWasMultiline = nestedMulti
				prevWasCall = true
				if nestedMulti {
					seenMultilineCall = true
				}
				continue
			}
			b.WriteByte(',')
			b.WriteByte('\n')
			b.WriteString(contIndent)
			b.WriteString(nested)
			curLen = curLenAfterWrite(nested)
			first = false
			nestedMulti := strings.Contains(nested, "\n")
			prevWasMultiline = nestedMulti
			prevWasCall = true
			if nestedMulti {
				seenMultilineCall = true
			}
			continue
		}
		if first {
			b.WriteString(contIndent)
			b.WriteString(a)
			// After writing possibly multi-line arg, update curLen to length of last line.
			curLen = curLenAfterWrite(a)
			first = false
			// If this arg spans multiple lines, mark it and force subsequent breaks.
			argMulti := strings.Contains(a, "\n")
			prevWasMultiline = argMulti
			prevWasCall = false
			if argMulti {
				seenMultilineComposite = true
			}
			continue
		}
		need := 2 + firstLineLen(a) // ", " + arg (first-line width)
		argIsMultiline := strings.Contains(a, "\n")
		// Determine if we should break to a new line:
		// 1. If we've seen a multiline call, all subsequent args break (including multiline)
		// 2. If we've seen a multiline composite (no call) and current is single-line, break
		// 3. If current wouldn't fit, break
		// But if we've only seen multiline composites (no calls) and current is also
		// multiline, stay on same line (e.g., "}, []string{" in Configure)
		shouldBreakAfterCall := seenMultilineCall
		shouldBreakAfterComposite := seenMultilineComposite && !seenMultilineCall && !argIsMultiline
		shouldBreak := curLen+need > width || forcedBreak || shouldBreakAfterCall || shouldBreakAfterComposite
		if shouldBreak {
			// Break line after comma from previous arg.
			b.WriteByte(',')
			b.WriteByte('\n')
			b.WriteString(contIndent)
			curLen = contIndentLen
			b.WriteString(a)
			curLen = curLenAfterWrite(a)
		} else {
			b.WriteString(", ")
			b.WriteString(a)
			curLen += need
			// If arg had newlines, reset curLen to last line length.
			if argIsMultiline {
				curLen = lastLineLen(a)
			}
		}
		// Track multiline status for the next iteration.
		if argIsMultiline {
			seenMultilineComposite = true
		}
		prevWasMultiline = argIsMultiline
		prevWasCall = false
	}
	// Trailing comma for multi-line call, then close with aligned paren.
	if trailingComma {
		b.WriteString(",\n")
	} else {
		b.WriteByte('\n')
	}
	b.WriteString(closingIndent)
	b.WriteByte(')')
	return b.String()
}

// formatCallPackedMultiLineNext is an opt-in variant of formatCallPackedMultiLine
// intended for "next" style formatting. It makes two key changes:
//   - Avoids recursively reflowing nested call args (e.g. len(...)) when they
//     fit on their own continuation line, preventing ugly nested expansions.
//   - Uses call-argument-aware string splitting that matches gofmt's spacing
//     around '+' when there are trailing call arguments.
func formatCallPackedMultiLineNext(call []byte, wsIndent, fullPrefix string, trailingComma bool) string {
	s := string(call)
	open := strings.IndexByte(s, '(')
	if open == -1 || !strings.HasSuffix(s, ")") {
		return s
	}
	head := s[:open]
	argsBody := s[open+1 : len(s)-1]
	args := scanner.SplitTopLevelAny(argsBody)
	closingIndent := wsIndent

	if len(args) == 0 {
		var b strings.Builder
		b.WriteString(head)
		b.WriteString("(\n")
		b.WriteString(closingIndent)
		b.WriteByte(')')
		return b.String()
	}

	width := columnLimit
	contIndent := wsIndent + "\t"
	contIndentLen := visualLen(contIndent)
	curLenAfterWrite := func(s string) int {
		if strings.Contains(s, "\n") {
			return lastLineLen(s)
		}
		return contIndentLen + firstLineLen(s)
	}

	var b strings.Builder
	b.WriteString(head)
	b.WriteByte('(')

	// Special-case `make(T, ...)` to keep the type argument inline with `make(`.
	// This improves readability for the common `make([]T, 0, n)` patterns and
	// avoids the awkward:
	//   make(
	//       []T, ...
	//   )
	//
	// We still force multiline output by inserting a newline right after the
	// first argument's comma when there are additional arguments.
	firstArgInline := false
	firstArgIdx := -1
	if strings.TrimSpace(head) == "make" {
		for i := 0; i < len(args); i++ {
			if strings.TrimSpace(args[i]) == "" {
				continue
			}
			firstArgIdx = i
			break
		}
		if firstArgIdx >= 0 {
			hasAnother := false
			for j := firstArgIdx + 1; j < len(args); j++ {
				if strings.TrimSpace(args[j]) != "" {
					hasAnother = true
					break
				}
			}
			firstArgInline = hasAnother
		}
	}
	if firstArgInline {
		firstArg := strings.TrimSpace(args[firstArgIdx])
		b.WriteString(firstArg)
		b.WriteByte(',')
		b.WriteByte('\n')
		// Continue formatting the remaining args.
		args = args[firstArgIdx+1:]
	} else {
		b.WriteByte('\n')
	}

	curLen := contIndentLen
	first := true
	prevWasMultiline := false
	prevWasCall := false
	seenMultilineCall := false
	seenMultilineComposite := false

	for idx, raw := range args {
		a := strings.TrimSpace(raw)
		if a == "" {
			continue
		}
		forcedBreak := !first && prevWasMultiline && prevWasCall
		// When we keep `make(` + first arg inline, prefer one-arg-per-line for the
		// remaining args to avoid packing `0, len(...)` on the same line.
		if firstArgInline && !first {
			forcedBreak = true
		}

		if e, err := parser.ParseExpr(a); err == nil {
			if text, ok := llast.FlattenStringExprAST(e); ok {
				hasMore := false
				for j := idx + 1; j < len(args); j++ {
					if strings.TrimSpace(args[j]) != "" {
						hasMore = true
						break
					}
				}

				stringWidth := width
				if first || forcedBreak {
					split := buildSplitQuotedForCallArg(text, contIndentLen, contIndent, stringWidth, hasMore)
					if !first {
						b.WriteByte(',')
						b.WriteByte('\n')
						b.WriteString(contIndent)
					} else {
						b.WriteString(contIndent)
					}
					b.WriteString(split)
					curLen = curLenAfterWrite(split)
					first = false
					prevWasMultiline, prevWasCall = false, false
					continue
				}

				plain := quoteGoString(text)
				if advanceCols(curLen+2, plain) <= width {
					b.WriteString(", ")
					b.WriteString(plain)
					curLen = advanceCols(curLen+2, plain)
					prevWasMultiline, prevWasCall = false, false
				} else {
					effectiveWidth := stringWidth
					if !hasMore && advanceCols(contIndentLen, plain) <= effectiveWidth {
						b.WriteByte(',')
						b.WriteByte('\n')
						b.WriteString(contIndent)
						b.WriteString(plain)
						curLen = contIndentLen + firstLineLen(plain)
						first = false
						prevWasMultiline, prevWasCall = false, false
						continue
					}

					tentative := buildSplitQuotedForCallArg(text, curLen+2, contIndent, effectiveWidth, hasMore)
					need := 2 + firstLineLen(tentative)
					if curLen+need <= effectiveWidth {
						b.WriteString(", ")
						b.WriteString(tentative)
						if strings.Contains(tentative, "\n") {
							curLen = lastLineLen(tentative)
							prevWasMultiline, prevWasCall = false, false
						} else {
							curLen += need
							prevWasMultiline, prevWasCall = false, false
						}
					} else {
						b.WriteByte(',')
						b.WriteByte('\n')
						b.WriteString(contIndent)
						curLen = contIndentLen
						split := buildSplitQuotedForCallArg(text, contIndentLen, contIndent, effectiveWidth, hasMore)
						b.WriteString(split)
						curLen = curLenAfterWrite(split)
						prevWasMultiline, prevWasCall = false, false
					}
				}
				first = false
				continue
			}
		}

		if fa, ok := FormatCompositeLiteralArg(a, contIndent, seenMultilineCall); ok {
			a = fa
		}

		if llast.IsCallExpr(a) {
			fits := false
			if first {
				fits = advanceCols(contIndentLen, a) <= width
			} else {
				fits = advanceCols(curLen+2, a) <= width
			}
			fitsFresh := advanceCols(contIndentLen, a) <= width
			hasAlways := callHasAlwaysMultilineComposite(a)
			hasNested := llast.HasNestedCall(a)

			// In the `make(T, ...)` special-case, don't pack additional args onto the
			// same continuation line even if they would fit.
			if firstArgInline && !first {
				fits = false
			}

			if fits && !hasAlways && !hasNested {
				if first {
					b.WriteString(contIndent)
					b.WriteString(a)
					curLen = contIndentLen + firstLineLen(a)
					first = false
				} else {
					if forcedBreak {
						b.WriteByte(',')
						b.WriteByte('\n')
						b.WriteString(contIndent)
						curLen = contIndentLen
					} else {
						b.WriteString(", ")
					}
					b.WriteString(a)
					curLen = advanceCols(curLen+2, a)
				}
				prevWasMultiline, prevWasCall = false, false
				continue
			}

			if !hasAlways && !hasNested && fitsFresh {
				// Keep as-is and let the generic placement logic below decide whether
				// to break before it. This avoids nested-call explosions like:
				//   len(
				//     x,
				//   )
			} else {
				nested := formatCallPackedMultiLineNext([]byte(a), contIndent, contIndent, true)
				if first {
					b.WriteString(contIndent)
					b.WriteString(nested)
					curLen = curLenAfterWrite(nested)
					first = false
					nestedMulti := strings.Contains(nested, "\n")
					prevWasMultiline = nestedMulti
					prevWasCall = true
					if nestedMulti {
						seenMultilineCall = true
					}
					continue
				}
				if forcedBreak {
					b.WriteByte(',')
					b.WriteByte('\n')
					b.WriteString(contIndent)
					curLen = contIndentLen
					b.WriteString(nested)
					curLen = curLenAfterWrite(nested)
					first = false
					nestedMulti := strings.Contains(nested, "\n")
					prevWasMultiline = nestedMulti
					prevWasCall = true
					if nestedMulti {
						seenMultilineCall = true
					}
					continue
				}
				b.WriteByte(',')
				b.WriteByte('\n')
				b.WriteString(contIndent)
				b.WriteString(nested)
				curLen = curLenAfterWrite(nested)
				first = false
				nestedMulti := strings.Contains(nested, "\n")
				prevWasMultiline = nestedMulti
				prevWasCall = true
				if nestedMulti {
					seenMultilineCall = true
				}
				continue
			}
		}

		if first {
			b.WriteString(contIndent)
			b.WriteString(a)
			curLen = curLenAfterWrite(a)
			first = false
			argMulti := strings.Contains(a, "\n")
			prevWasMultiline = argMulti
			prevWasCall = false
			if argMulti {
				seenMultilineComposite = true
			}
			continue
		}

		need := 2 + firstLineLen(a)
		argIsMultiline := strings.Contains(a, "\n")
		shouldBreakAfterCall := seenMultilineCall
		shouldBreakAfterComposite := seenMultilineComposite && !seenMultilineCall && !argIsMultiline
		shouldBreak := curLen+need > width || forcedBreak || shouldBreakAfterCall || shouldBreakAfterComposite
		if shouldBreak {
			b.WriteByte(',')
			b.WriteByte('\n')
			b.WriteString(contIndent)
			curLen = contIndentLen
			b.WriteString(a)
			curLen = curLenAfterWrite(a)
		} else {
			b.WriteString(", ")
			b.WriteString(a)
			curLen += need
			if argIsMultiline {
				curLen = lastLineLen(a)
			}
		}
		if argIsMultiline {
			seenMultilineComposite = true
		}
		prevWasMultiline = argIsMultiline
		prevWasCall = false
	}

	if trailingComma {
		b.WriteString(",\n")
	} else {
		b.WriteByte('\n')
	}
	b.WriteString(closingIndent)
	b.WriteByte(')')
	return b.String()
}

// callHasAlwaysMultilineComposite reports whether the call expression's
// arguments contain a map/struct composite literal that should be block
// formatted when inside a multiline call.
func callHasAlwaysMultilineComposite(s string) bool {
	// Find the first '(' and matching ')', then split args and check each
	// for a top-level brace.
	open := strings.IndexByte(s, '(')
	if open < 0 || s[len(s)-1] != ')' {
		return false
	}
	// We need to find the matching close; reuse the byte scanner.
	end := scanner.ScanBalancedParen([]byte(s), open)
	if end <= open {
		return false
	}
	body := s[open+1 : end]
	args := scanner.SplitTopLevelAny(body)
	for _, raw := range args {
		a := strings.TrimSpace(raw)
		if a == "" {
			continue
		}
		// If this arg is itself a composite literal head (Type{...}) or map
		// literal, detect top-level braces.
		if o, c := findTopLevelBraces(a); o >= 0 && c > o {
			return true
		}
	}
	return false
}

// buildSplitQuoted splits text into quoted segments so they fit within the
// given width starting at startCol for the first segment. Continuation lines
// are indented with contIndent + an extra tab. All but the last segment are
// emitted as "..." + and the last as "...". It preserves spaces when splitting
// at word boundaries when possible.
func buildSplitQuoted(text string, startCol int, contIndent string, width int) string {
	return buildSplitQuotedWithOptions(text, startCol, contIndent, width, false)
}

// buildSplitQuotedForCallArg is like buildSplitQuoted but applies gofmt's
// context-sensitive spacing around '+' when splitting a string literal that
// appears as a call argument that may have trailing arguments.
func buildSplitQuotedForCallArg(text string, startCol int, contIndent string, width int, hasTrailingArgs bool) string {
	return buildSplitQuotedWithOptions(text, startCol, contIndent, width, hasTrailingArgs)
}

func buildSplitQuotedWithOptions(text string, startCol int, contIndent string, width int, hasTrailingArgs bool) string {
	var out strings.Builder
	rest := text
	curStart := startCol
	// String continuation lines get an extra tab beyond the argument indent.
	stringContIndent := contIndent + "\t"
	contStart := visualLen(stringContIndent)

	writeJoin := func(seg string) {
		out.WriteString(quoteGoString(seg))
		endsWithSpace := len(seg) > 0 && seg[len(seg)-1] == ' '
		if endsWithSpace && hasTrailingArgs {
			out.WriteByte('+')
		} else {
			out.WriteByte(' ')
			out.WriteByte('+')
		}
		out.WriteByte('\n')
		out.WriteString(stringContIndent)
	}

	for {
		if rest == "" {
			break
		}
		// If there's not even enough room for a minimal quoted segment (quotes +
		// at least one rune) within the width budget, splitting can't produce a
		// "better" layout. Emit a single quoted literal and stop.
		if width-curStart <= 4 {
			out.WriteString(quoteGoString(rest))
			break
		}
		// If the indentation itself already exceeds the available width budget,
		// splitting can't help (no segment can ever "fit"). Emit a single quoted
		// literal and stop to avoid producing degenerate/dangling split output.
		if curStart >= width {
			out.WriteString(quoteGoString(rest))
			break
		}
		// If the whole rest fits as a quoted literal on this line, emit and finish.
		if advanceCols(curStart, quoteGoString(rest)) <= width {
			out.WriteString(quoteGoString(rest))
			break
		}
		// Choose split point at last space that fits with trailing join.
		cut := lastQuotedSpaceBeforeStrictWithJoin(curStart, rest, width, hasTrailingArgs)
		if cut <= 0 {
			// Hard cut by visual width capacity for content excluding quotes + " +".
			capCols := width - curStart - 2 - 2 // quotes + " +"
			if capCols <= 0 {
				capCols = 1
			}
			idx := cutIndexForWidthFrom(curStart, rest, capCols)
			if idx <= 0 {
				idx = 1
			}
			// If we end up consuming all remaining content, emit it as a single
			// quoted literal and stop. This matters when curStart > width (deep
			// indentation): we can’t make progress by splitting, and emitting a
			// dangling '+' would produce invalid Go like `"x" +\n,`.
			if idx >= len(rest) {
				out.WriteString(quoteGoString(rest))
				break
			}
			seg := rest[:idx]
			writeJoin(seg)
			rest = rest[idx:]
			curStart = contStart
			continue
		}
		seg := rest[:cut+1]
		// If the split point consumes all remaining content, emit and stop to
		// avoid emitting a dangling '+'.
		if cut+1 >= len(rest) {
			out.WriteString(quoteGoString(rest))
			break
		}
		writeJoin(seg)
		rest = rest[cut+1:]
		curStart = contStart
	}
	// Defensive cleanup: if we ever end up with a dangling trailing '+', drop it.
	// This can happen when indentation already exceeds the width budget and a
	// split attempt fails to make progress. Leaving a dangling '+' would produce
	// invalid Go when the surrounding call formatter appends a comma/newline.
	result := out.String()
	trimmed := strings.TrimRight(result, " \t\n")
	if strings.HasSuffix(trimmed, "+") {
		trimmed = strings.TrimRight(trimmed[:len(trimmed)-1], " \t")
		return trimmed
	}
	return result
}

// formatCallGreedy applies a simple greedy layout: keep arguments on the
// current line if they fit (including a preceding ", "), otherwise break before
// the argument. String literals are split at the last space before the boundary
// (or hard-cut) and joined with " +" on continuation lines.
func formatCallGreedy(call []byte, wsIndent string, baseLen int) string {
	return formatCallGreedyWithOptions(call, wsIndent, baseLen, greedyCallOptions{})
}

type greedyCallOptions struct {
	UseJoinAwareSpaceCut bool
	ReserveClosingParen  bool
	AllowExactFit        bool
	MinTailLen           int

	// PreferBreakBeforeSplit tries to move a string literal to a continuation
	// line (after "(" or after a comma break) if the full literal would fit
	// there, instead of immediately splitting it on the current line.
	PreferBreakBeforeSplit bool

	// AvoidHangingParenForPrintf disables PreferBreakBeforeSplit for printf-style
	// calls (format-string + trailing args). This prevents layouts like:
	//
	//   fmt.Errorf(
	//   	"...: %v", err)
	//
	// where we break right after the signature/paren even though we could split
	// the string and keep the call visually "packed".
	AvoidHangingParenForPrintf bool

	// AvoidTinyFormatVerbTail avoids splitting immediately before a tiny format
	// verb tail like "%v"/"%w", which produces awkward output like:
	//   "error: "+\n\t"%v"
	AvoidTinyFormatVerbTail bool

	// ReserveTrailingExprArgs encourages the formatter to keep some number of
	// trailing expression arguments on the same line as the final segment of a
	// format string. This reduces cases where we break `..., maxFee,\n feeRate)`
	// even though splitting the string slightly would allow `..., maxFee, feeRate)`
	// on one continuation line.
	ReserveTrailingExprArgs int

	// PreserveStringConcatExpr keeps explicit string concatenation expressions
	// (e.g. "a"+"b") as expressions rather than flattening them and re-splitting.
	// This helps avoid reflowing user-authored split points into worse ones.
	PreserveStringConcatExpr bool
}

func formatCallGreedyWithOptions(call []byte, wsIndent string, baseLen int, opts greedyCallOptions) string {
	s := string(call)
	open := strings.IndexByte(s, '(')
	if open == -1 || !strings.HasSuffix(s, ")") {
		return s
	}
	head := s[:open]
	argsBody := s[open+1 : len(s)-1]

	// No pre-scan; we will attach leading comments of the next arg (// or
	// /* */) to the previous argument inline when emitting.
	rawArgs := scanner.SplitTopLevel(argsBody)
	hasInlineComment := strings.Contains(argsBody, "/*") || strings.Contains(argsBody, "//")
	normArgs := make([]arg, 0, len(rawArgs))
	for _, ra := range rawArgs {
		trimmed := strings.TrimSpace(ra)
		if e, err := parser.ParseExpr(trimmed); err == nil {
			if str, ok := llast.FlattenStringExprAST(e); ok {
				// If this is a concatenation expression used as a format string with
				// trailing arguments, we generally want to flatten it so we can
				// re-split it more intelligently (e.g. to keep `..., a, b)` packed
				// instead of breaking `b` onto its own line).
				//
				// Preserve user-authored split points only when there are no other
				// arguments following this string.
				if opts.PreserveStringConcatExpr &&
					!isBasicStringLitExpr(e) &&
					!(len(rawArgs) > 1 && containsFormatVerb(str)) {
					normArgs = append(normArgs, arg{kind: argExpr, expr: trimmed})
					continue
				}

				normArgs = append(normArgs, arg{
					kind:              argText,
					text:              str,
					raw:               trimmed,
					containsFormatVerb: containsFormatVerb(str),
				})
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

	writeSplit := func(seg string, hasTrailingArgs bool) {
		q := quoteGoString(seg)
		b.WriteString(q)
		curLen = advanceCols(curLen, q)
		// gofmt normalizes string concatenation spacing based on context:
		// - When string ends with space AND has trailing args: gofmt removes space before +
		// - When string ends with space AND no trailing args: gofmt keeps space before +
		// - When string ends with non-space: gofmt keeps space before +
		// To be idempotent with gofmt, we output what gofmt would produce.
		endsWithSpace := len(seg) > 0 && seg[len(seg)-1] == ' '
		if endsWithSpace && hasTrailingArgs {
			// gofmt removes space before + in this case
			b.WriteByte('+')
			curLen += 1
		} else {
			// gofmt keeps space before + in these cases
			b.WriteByte(' ')
			b.WriteByte('+')
			curLen += 2
		}
		b.WriteByte('\n')
		b.WriteString(contIndent)
		curLen = visualLen(contIndent)
	}

		for i, a := range normArgs {
			justBroke := false
			if i > 0 {
			// If this arg starts with a comment, detach it so we
			// can place it next to the preceding argument in the
			// correct position.
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
				// Separator on same line; attach trailing line
				// comment to previous arg, then place any block
				// comment before next arg.
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
					// Greedy argument packing: keep each argument on the current line if
					// it fits, otherwise break before it.
					//
					// Note: Avoid lookahead that forces a break before the first arg just
					// to keep pairs together; that can under-fill the current line and
					// "repack" arguments in a surprising way.
					switch a.kind {
					case argExpr:
						need := firstLineLen(a.expr)
						if isTargetedCallStart(a.expr) {
							need = exprHeadLen(a.expr)
						}
						// If this is the final argument, also account for the closing paren.
						reserve := 0
						// Only reserve ')' for single-line expressions; for multi-line
						// expressions (including explicit string concatenations), ')' will
						// end up on the last line, so reserving it against the first line can
						// cause unnecessary breaks.
						if opts.ReserveClosingParen && i == len(normArgs)-1 && !strings.Contains(a.expr, "\n") {
							reserve = 1
						}
						fits := curLen+2+need+reserve < width
						if opts.AllowExactFit {
							fits = curLen+2+need+reserve <= width
						}
						if fits {
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
							// Reset after the first decision.
						} else {
							// Put trailing line comment on
							// the same line as the comma.
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
							// Place block comment
							// before the arg on the
							// new line.
							b.WriteString(blockCommentPrefix)
							b.WriteByte(' ')
							curLen += visualLen(blockCommentPrefix) + 1
						}
							// Reset after the first decision.
						}
				case argText:
					// minimal placeable segment on same
					// line: "X" +
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
				// For nested targeted calls, use the head length to
				// decide fit.
				need := firstLineLen(a.expr)
				if isTargetedCallStart(a.expr) {
					need = exprHeadLen(a.expr)
				}
				reserve := 0
				if opts.ReserveClosingParen && i == len(normArgs)-1 && !strings.Contains(a.expr, "\n") {
					reserve = 1
				}
				if !justBroke && !isRawStringLiteral(a.expr) && curLen+need+reserve > width {
					b.WriteByte('\n')
					b.WriteString(contIndent)
					curLen = visualLen(contIndent)
				}
			if isTargetedCallStart(a.expr) {
				formatted := formatCallGreedyWithOptions([]byte(a.expr), wsIndent, curLen, opts)
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
			hasTrailingArgs := i < len(normArgs)-1
			// If the user explicitly wrote a concatenation expression (and we didn't
			// preserve it as argExpr), avoid producing a tiny trailing verb segment
			// by biasing toward breaking arguments instead of splitting awkwardly.
			for len(rest) > 0 {
			q := quoteGoString(rest)
			// If there are more args after this string, we only need to ensure
			// the trailing comma fits on the current line. The following argument
			// may be placed on a continuation line (comma + newline), so requiring
			// a ", " here is overly strict and can cause unnecessary string
			// splitting (e.g., splitting just to "make room" for the space).
			//
			// We still want to reserve the comma (",") because it must appear at
			// the end of the current line when we break before the next arg.
			commaReserve := 0
			if hasTrailingArgs {
				// At minimum, reserve the comma if we might break before the next arg.
				commaReserve = 1
			} else if opts.ReserveClosingParen {
				// Avoid making the final string literal fit exactly at the boundary
				// and then pushing over by appending ')'.
				commaReserve = 1
			}

			// Preferred reserve: if this is a printf-style format string and there
			// are trailing expression args, try to keep a small number of those args
			// packed with the final string segment *when it is actually achievable*.
			//
			// This is a preference, not a hard constraint: if it can't be met, keep
			// the string whole and break args instead of producing awkward extra
			// concatenation lines.
			preferredReserve := commaReserve
			if hasTrailingArgs && opts.ReserveTrailingExprArgs > 0 && a.containsFormatVerb {
				reserve := 0
				kept := 0
				for j := i + 1; j < len(normArgs) && kept < opts.ReserveTrailingExprArgs; j++ {
					if normArgs[j].kind != argExpr {
						break
					}
					need := firstLineLen(normArgs[j].expr)
					if isTargetedCallStart(normArgs[j].expr) {
						need = exprHeadLen(normArgs[j].expr)
					}
					reserve += 2 + need // ", " + expr
					kept++
				}
				// If we reserved all remaining args, also reserve the closing paren.
				if i+kept == len(normArgs)-1 && kept > 0 {
					reserve += 1
				}
				if reserve > preferredReserve {
					preferredReserve = reserve
				}
			}

			contStart := visualLen(contIndent)
			fitsPreferredHere := advanceCols(curLen, q)+preferredReserve <= width
			fitsHere := advanceCols(curLen, q)+commaReserve <= width

				if fitsPreferredHere {
					b.WriteString(q)
					curLen = advanceCols(curLen, q)
					rest = ""
					break
				}
				if fitsHere {
					// Even if we'd like to keep additional args on the same line, avoid
					// splitting a string literal that already fits; let args break instead.
					b.WriteString(q)
					curLen = advanceCols(curLen, q)
					rest = ""
					break
				}

				// Before splitting the string on the current line, try moving the
				// whole literal to a continuation line. This primarily helps when the
				// call starts mid-line (e.g. `return nil, fmt.Errorf(...)`) and the
				// literal would fit cleanly once the prefix is removed.
					isPrintfString := hasTrailingArgs && a.containsFormatVerb
					if opts.PreferBreakBeforeSplit &&
						curLen != visualLen(contIndent) &&
						// Avoid creating hanging-paren layouts for single-string calls like:
						//   fmt.Errorf(
						//     "...")
						// Prefer splitting the string on the current line instead.
						hasTrailingArgs &&
						!(opts.AvoidHangingParenForPrintf && isPrintfString) {
						fitsPreferredCont := advanceCols(contStart, q)+preferredReserve <= width
						fitsCont := advanceCols(contStart, q)+commaReserve <= width

					if fitsPreferredCont {
						b.WriteByte('\n')
						b.WriteString(contIndent)
						curLen = contStart
						b.WriteString(q)
						curLen = advanceCols(curLen, q)
						rest = ""
						break
					}

					if fitsCont {
						// Prefer keeping the string whole on a clean continuation line.
						// If there are trailing args, let them break rather than introducing
						// string concatenation solely to "make room".
						b.WriteByte('\n')
						b.WriteString(contIndent)
						curLen = contStart
						b.WriteString(q)
						curLen = advanceCols(curLen, q)
						rest = ""
						break
					}
				}

				// Choose the last ASCII space whose QUOTED prefix fits, taking into
				// account escape expansion inside the literal and gofmt's
				// context-sensitive spacing around '+'.
				//
				// Important: do this *before* forcing a newline due to "capCols <= 0".
				// When `hasTrailingArgs` and the segment ends with a space, gofmt emits
				// `"..."+"..."` (no space before '+'), which is one column cheaper than
				// `"..." + "..."`. The join-aware cut helpers already account for this.
				cut := lastQuotedSpaceBefore(curLen, rest, width)
				if opts.UseJoinAwareSpaceCut {
					cut = lastQuotedSpaceBeforeWithJoin(curLen, rest, width, hasTrailingArgs)
				}
				if opts.AvoidTinyFormatVerbTail || opts.MinTailLen > 0 {
					if opts.UseJoinAwareSpaceCut {
						cut = lastQuotedSpaceBeforeWithJoinAvoidingTails(curLen, rest, width, hasTrailingArgs, opts.MinTailLen, opts.AvoidTinyFormatVerbTail)
					} else {
						cut = lastQuotedSpaceBeforeAvoidingTails(curLen, rest, width, opts.MinTailLen, opts.AvoidTinyFormatVerbTail)
					}
				}

				// Capacity for content (excluding quotes and join operator) of this
				// split segment. This is a non-final segment (we are splitting), so we
				// allow exact fill up to the boundary with the trailing join.
				capCols := (width) - curLen - 2 - 2 // quotes + " +"
				if cut <= 0 && capCols <= 0 {
					b.WriteByte('\n')
					b.WriteString(contIndent)
					curLen = visualLen(contIndent)
					capCols = width - curLen - 2 - 2
					if capCols <= 0 {
						capCols = 1
					}
					// Recompute cut now that we're on a continuation line.
					cut = lastQuotedSpaceBefore(curLen, rest, width)
					if opts.UseJoinAwareSpaceCut {
						cut = lastQuotedSpaceBeforeWithJoin(curLen, rest, width, hasTrailingArgs)
					}
					if opts.AvoidTinyFormatVerbTail || opts.MinTailLen > 0 {
						if opts.UseJoinAwareSpaceCut {
							cut = lastQuotedSpaceBeforeWithJoinAvoidingTails(curLen, rest, width, hasTrailingArgs, opts.MinTailLen, opts.AvoidTinyFormatVerbTail)
						} else {
							cut = lastQuotedSpaceBeforeAvoidingTails(curLen, rest, width, opts.MinTailLen, opts.AvoidTinyFormatVerbTail)
						}
					}
				}

				if cut <= 0 {
					// No space within capacity. If we are not on a
					// continuation line and the upcoming word (up
					// to the next space) would fit on a
					// continuation line, wrap before it to avoid
					// splitting a word on the head line.
					if curLen != visualLen(contIndent) {
						if sp := strings.IndexByte(rest, ' '); sp > 0 {
							base := visualLen(contIndent)
							// compute content width of the
							// first word at cont indent
						wordCols := advanceCols(base, rest[:sp]) - base
							nextCap := (width) - base - 2 - 2 // quotes + " +"
							if hasTrailingArgs {
								// If we can split at a space, gofmt may emit `"..."+"..."`
								// (no space before '+'), which is one column cheaper.
								nextCap++
							}
							if wordCols <= nextCap {
								b.WriteByte('\n')
								b.WriteString(contIndent)
								curLen = visualLen(contIndent)
							// Recompute capacity on
							// the fresh
							// continuation line
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
				if idx >= len(rest) {
					// Splitting didn't make progress (typically because indentation
					// already exceeds the width budget). Emit the full literal and
					// stop; emitting a dangling '+' would produce invalid Go when
					// followed by a comma/newline from argument formatting.
					q := quoteGoString(rest)
					b.WriteString(q)
					curLen = advanceCols(curLen, q)
					rest = ""
					break
				}
					seg := rest[:idx]
					writeSplit(seg, hasTrailingArgs)
					rest = rest[idx:]
					continue
			}
			// Pure greedy: no additional word-pushing heuristics.
			// Pure greedy: take the last space within capacity.
			if cut+1 >= len(rest) {
				q := quoteGoString(rest)
				b.WriteString(q)
				curLen = advanceCols(curLen, q)
				rest = ""
				break
			}
				seg := rest[:cut+1] // keep the space at end
				writeSplit(seg, hasTrailingArgs)
				rest = rest[cut+1:]
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
	raw  string

	containsFormatVerb bool
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
	ts := tabStop
	if ts <= 0 {
		ts = 8
	}
	return width.LastLineLenWithTab(s, ts)
}

func isBasicStringLitExpr(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	if !ok {
		return false
	}
	return lit.Kind == token.STRING
}

func containsFormatVerb(s string) bool {
	// A minimal heuristic: treat any '%' that is not part of a '%%' escape as a
	// format verb indicator. This is intentionally conservative.
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			continue
		}
		if i+1 < len(s) && s[i+1] == '%' {
			i++
			continue
		}
		return true
	}
	return false
}

func looksLikeTinyFormatVerbTail(s string) bool {
	if s == "" {
		return false
	}
	trimmed := strings.TrimLeft(s, " \t")
	if trimmed == "" || trimmed[0] != '%' {
		return false
	}
	// Only consider the first token (up to next space). We treat small format
	// tokens like "%v", "%w", "%+v" as "tiny" tails to avoid ugly splits like:
	//   "error: "+\n\t"%v"
	end := strings.IndexByte(trimmed, ' ')
	if end < 0 {
		end = len(trimmed)
	}
	token := trimmed[:end]
	token = strings.TrimRight(token, ",.)")
	if len(token) == 2 && token[0] == '%' && isASCIIAlpha(token[1]) {
		return true
	}
	if len(token) == 3 && token[0] == '%' && token[1] == '+' && isASCIIAlpha(token[2]) {
		return true
	}
	return false
}

func isASCIIAlpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func isTinyTail(s string, minTailLen int) bool {
	if minTailLen <= 0 {
		return false
	}
	trimmed := strings.TrimLeft(s, " \t")
	return len(trimmed) > 0 && len(trimmed) < minTailLen
}

func canFitPreferredTailBySplitting(rest string, contStart int, width int, preferredReserve int, opts greedyCallOptions) bool {
	// Look for any split point (space) such that the remainder could fit on a
	// continuation line while leaving room for the preferred reserve (typically
	// some number of trailing expr args).
	//
	// If no such split exists, continuing to split the string to "make room" for
	// args is counterproductive: we'll end up breaking args anyway, so we should
	// keep the string whole.
	if preferredReserve <= 0 {
		return false
	}
	for i := 0; i < len(rest); i++ {
		if rest[i] != ' ' {
			continue
		}
		tail := rest[i+1:]
		if opts.MinTailLen > 0 && isTinyTail(tail, opts.MinTailLen) {
			continue
		}
		if opts.AvoidTinyFormatVerbTail && looksLikeTinyFormatVerbTail(tail) {
			continue
		}
		if advanceCols(contStart, quoteGoString(tail))+preferredReserve <= width {
			return true
		}
	}
	return false
}

func lastQuotedSpaceBeforeAvoidingTails(startCol int, s string, boundary int, minTailLen int, avoidTinyVerbTail bool) int {
	last := -1
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' {
			continue
		}
		if isTinyTail(s[i+1:], minTailLen) {
			continue
		}
		if avoidTinyVerbTail && looksLikeTinyFormatVerbTail(s[i+1:]) {
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

func lastQuotedSpaceBeforeWithJoinAvoidingTails(startCol int, s string, boundary int, hasTrailingArgs bool, minTailLen int, avoidTinyVerbTail bool) int {
	last := -1
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' {
			continue
		}
		if isTinyTail(s[i+1:], minTailLen) {
			continue
		}
		if avoidTinyVerbTail && looksLikeTinyFormatVerbTail(s[i+1:]) {
			continue
		}
		piece := s[:i+1]
		joinCols := 2
		if hasTrailingArgs && len(piece) > 0 && piece[len(piece)-1] == ' ' {
			joinCols = 1
		}
		used := advanceCols(startCol, quoteGoString(piece)) + joinCols
		if used <= boundary {
			last = i
		} else {
			break
		}
	}
	return last
}

func quoteGoString(s string) string {
	// Emit a double-quoted Go string literal, preserving runes as-is where
	// possible. Escape only what Go requires or what would break the
	// literal:
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
// quotes. (legacy chunkTextFitWithLastLimit removed)

// chooseSuffixStart returns an index in s where a final suffix segment can
// start so that the quoted suffix fits in lastAvail columns. It prefers
// starting at word boundaries (the character after a space). Returns 0 if no
// reasonable boundary is found. (legacy chooseSuffixStart removed)

// (legacy lastSpaceBefore removed)

func visualLen(s string) int {
	ts := tabStop
	if ts <= 0 {
		ts = 8
	}
	return width.VisualLenWithTab(s, ts)
}

// advanceCols returns the absolute column after writing s starting from
// startCol, accounting for tabs advancing to the next tab stop.
func advanceCols(startCol int, s string) int {
	ts := tabStop
	if ts <= 0 {
		ts = 8
	}
	return width.AdvanceColsWithTab(startCol, s, ts)
}

// lastSpaceBeforeFrom returns the last byte index of an ASCII space such that
// the substring up to that index fits within maxCols additional columns when
// starting from startCol. (legacy lastSpaceBeforeFrom removed)

// lastQuotedSpaceBefore returns the last index of an ASCII space in s such that
// the quoted prefix up to and including that space would fit within the
// boundary when starting from startCol and accounting for " +" at the end of
// the segment. Returns -1 if no such boundary exists.
// lastQuotedSpaceBefore returns the last index of an ASCII space in s such that
// the quoted prefix up to and including that space would fit within the
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

// lastQuotedSpaceBeforeWithJoin is like lastQuotedSpaceBefore but accounts for
// gofmt's context-sensitive join width: when a segment ends with a space and the
// concatenation is followed by more call arguments, gofmt may elide the space
// before '+' (making the join 1 column instead of 2).
func lastQuotedSpaceBeforeWithJoin(startCol int, s string, boundary int, hasTrailingArgs bool) int {
	last := -1
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' {
			continue
		}
		piece := s[:i+1]
		joinCols := 2
		if hasTrailingArgs && len(piece) > 0 && piece[len(piece)-1] == ' ' {
			joinCols = 1
		}
		used := advanceCols(startCol, quoteGoString(piece)) + joinCols
		if used <= boundary {
			last = i
		} else {
			break
		}
	}
	return last
}

// lastQuotedSpaceBeforeStrict is like lastQuotedSpaceBefore but uses a strict
// inequality to avoid placing the split exactly at the boundary. This helps
// keep editor renderings consistent when counting columns.
func lastQuotedSpaceBeforeStrict(startCol int, s string, boundary int) int {
	last := -1
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' {
			continue
		}
		piece := s[:i+1]
		used := advanceCols(startCol, quoteGoString(piece)) + 2
		if used < boundary {
			last = i
		} else {
			break
		}
	}
	return last
}

func lastQuotedSpaceBeforeStrictWithJoin(startCol int, s string, boundary int, hasTrailingArgs bool) int {
	last := -1
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' {
			continue
		}
		piece := s[:i+1]
		joinCols := 2
		if hasTrailingArgs && len(piece) > 0 && piece[len(piece)-1] == ' ' {
			joinCols = 1
		}
		used := advanceCols(startCol, quoteGoString(piece)) + joinCols
		if used < boundary {
			last = i
		} else {
			break
		}
	}
	return last
}

// cutIndexForWidthFrom returns the number of bytes from the start of s that fit
// within maxCols additional columns when starting from startCol. It avoids
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

// firstLineLen returns the visual length (tabs as 8) of s up to its first
// newline (or full length if no newline is present).
func firstLineLen(s string) int {
	ts := tabStop
	if ts <= 0 {
		ts = 8
	}
	return width.FirstLineLenWithTab(s, ts)
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
		// Stop on newline since we only want head of first line by
		// default.
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
// - segment 1 fits within firstAvail (including quotes), ending on a word
//   boundary
// - the remainder fits within nextAvail (including quotes) Returns nil if it
//   can't satisfy the constraints. (legacy splitPrefixHeadCont removed)

// ensureHeadFits splits segs[0] if needed so that the first quoted segment fits
// within firstAvail columns. It preserves word boundaries and adds a trailing
// space to the first part if split. (legacy ensureHeadFits removed)

// FormatCallGreedy formats a function call using the greedy left-flow packing
// algorithm. This is exported for use by the DSL LeftFlowCallAction to ensure
// identical output between the DSL formatter and the legacy pipeline.
//
// Parameters:
//   - call: the raw bytes of the call expression (e.g., "log.Infof(...)")
//   - wsIndent: the leading whitespace of the line
//   - baseLen: visual width from line start to call start
//   - colLimit: column limit (e.g., 80)
//   - ts: tab stop width (e.g., 8)
//
// Returns the formatted call as a string.
func FormatCallGreedy(call []byte, wsIndent string, baseLen int, colLimit, ts int) string {
	formatGlobalsMu.Lock()
	defer formatGlobalsMu.Unlock()

	// Temporarily set package-level config for helpers that rely on them
	oldColumnLimit := columnLimit
	oldTabStop := tabStop
	oldTargets := currentTargets

	columnLimit = colLimit
	tabStop = ts
	currentTargets = defaultTargets()

	result := formatCallGreedy(call, wsIndent, baseLen)

	// Restore previous values
	columnLimit = oldColumnLimit
	tabStop = oldTabStop
	currentTargets = oldTargets

	return result
}

// FormatCallGreedyNext is an opt-in wrapper around the greedy call formatter
// intended for the "next" rule profile. It enables additional heuristics that
// are safe to evolve without breaking golden fixtures:
//   - Join-aware string splitting when there are trailing call args, matching
//     gofmt's context-sensitive spacing around '+'.
//   - Reservation for the trailing ')' when deciding whether a final string
//     literal fits, avoiding off-by-one overflow at the column limit.
func FormatCallGreedyNext(call []byte, wsIndent string, baseLen int, colLimit, ts int) string {
	formatGlobalsMu.Lock()
	defer formatGlobalsMu.Unlock()

	oldColumnLimit := columnLimit
	oldTabStop := tabStop
	oldTargets := currentTargets

	columnLimit = colLimit
	tabStop = ts
	currentTargets = defaultTargets()

		result := formatCallGreedyWithOptions(call, wsIndent, baseLen, greedyCallOptions{
			UseJoinAwareSpaceCut: true,
			ReserveClosingParen:  true,
			AllowExactFit:        true,
			MinTailLen:           8,
			PreferBreakBeforeSplit:  true,
			AvoidHangingParenForPrintf: true,
			AvoidTinyFormatVerbTail: true,
			ReserveTrailingExprArgs: 2,
			PreserveStringConcatExpr: true,
		})

	columnLimit = oldColumnLimit
	tabStop = oldTabStop
	currentTargets = oldTargets

	return result
}

func formatCallGreedyNextWithMinTailLen(call []byte, wsIndent string, baseLen int, colLimit, ts int, minTailLen int) string {
	formatGlobalsMu.Lock()
	defer formatGlobalsMu.Unlock()

	oldColumnLimit := columnLimit
	oldTabStop := tabStop
	oldTargets := currentTargets

	columnLimit = colLimit
	tabStop = ts
	currentTargets = defaultTargets()

	if minTailLen < 0 {
		minTailLen = 0
	}

		result := formatCallGreedyWithOptions(call, wsIndent, baseLen, greedyCallOptions{
			UseJoinAwareSpaceCut: true,
			ReserveClosingParen:  true,
			AllowExactFit:        true,
			MinTailLen:           minTailLen,
			PreferBreakBeforeSplit:  true,
			AvoidHangingParenForPrintf: true,
			AvoidTinyFormatVerbTail: true,
			ReserveTrailingExprArgs: 2,
			PreserveStringConcatExpr: true,
		})

	columnLimit = oldColumnLimit
	tabStop = oldTabStop
	currentTargets = oldTargets

	return result
}

// FormatCallPackedMultiLine is an exported wrapper around formatCallPackedMultiLine
// for use by the DSL engine. It formats a generic function call into a packed
// multi-line style when the single-line form would exceed the column limit.
//
// Parameters:
//   - call: the raw call expression bytes
//   - wsIndent: the whitespace indent string for continuation lines
//   - colLimit: column limit (e.g., 80)
//   - ts: tab stop width (e.g., 8)
//
// Returns the formatted call as a string.
func FormatCallPackedMultiLine(call []byte, wsIndent string, colLimit, ts int) string {
	formatGlobalsMu.Lock()
	defer formatGlobalsMu.Unlock()

	// Temporarily set package-level config for helpers that rely on them
	oldColumnLimit := columnLimit
	oldTabStop := tabStop

	columnLimit = colLimit
	tabStop = ts

	result := formatCallPackedMultiLine(call, wsIndent, wsIndent, true)

	// Restore previous values
	columnLimit = oldColumnLimit
	tabStop = oldTabStop

	return result
}

// FormatCallPackedMultiLineNext is an opt-in wrapper around formatCallPackedMultiLineNext
// for use by the DSL engine and the "next" rule profile. It preserves legacy
// packed multiline output by default (FormatCallPackedMultiLine) and only
// applies next behavior when explicitly selected.
func FormatCallPackedMultiLineNext(call []byte, wsIndent string, colLimit, ts int) string {
	formatGlobalsMu.Lock()
	defer formatGlobalsMu.Unlock()

	oldColumnLimit := columnLimit
	oldTabStop := tabStop

	columnLimit = colLimit
	tabStop = ts

	result := formatCallPackedMultiLineNext(call, wsIndent, wsIndent, true)

	columnLimit = oldColumnLimit
	tabStop = oldTabStop

	return result
}
