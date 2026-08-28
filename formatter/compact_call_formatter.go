package formatter

import (
	"bytes"
	"go/ast"
	formatstd "go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
	"unicode"
	"unicode/utf8"

	llast "github.com/bhandras/llformat/ast"
	"github.com/bhandras/llformat/scanner"
	"github.com/bhandras/llformat/text"
	"github.com/bhandras/llformat/width"
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
	// Excludes is a list of substrings; if any matches the callee name of a
	// non-target call, fallback formatting will skip that call. This
	// mirrors the legacy multiline-call exclude semantics
	// (strings.Contains).
	//
	// Note: This only applies to the fallback formatter
	// (FallbackNonTargets); targeted calls (Targets) are still formatted as
	// usual.
	Excludes []string
	// FallbackNonTargets enables formatting of non-targeted function calls
	// that exceed the column limit using a packed multi-line style.
	FallbackNonTargets bool
	// FallbackNonTargetsExcludeSelectors disables fallback formatting for
	// selector calls (e.g. `pkg.Func(...)` or `x.Method(...)`). This can be
	// useful in pipelines that want selector calls handled by a dedicated
	// multiline stage instead.
	FallbackNonTargetsExcludeSelectors bool

	// UseASTSelection switches this legacy call formatter from scan-based
	// call detection to AST-based call selection. The formatting logic
	// remains the same; only the "which calls do we consider next?"
	// selector changes.
	//
	// This is intentionally opt-in to preserve golden fixtures.
	UseASTSelection bool

	// SkipGofmt skips the internal gofmt pass, useful when running in a
	// pipeline that will run gofmt at the end.
	SkipGofmt bool

	// ParseSafe enables parse-safe behavior: if the formatter's output does
	// not successfully gofmt, the original input is returned unchanged.
	// This avoids returning syntactically invalid Go when a heuristic
	// rewrite goes wrong.
	ParseSafe bool
}

// CompactCallFormatter implements compact packing formatting for function
// calls.
type CompactCallFormatter struct{ cfg Config }

// OwnedSpans returns the spans of calls that the compact call formatter would
// consider formatting. In the legacy pipeline, this stage typically runs before
// expression formatting, but exposing ownership allows the pipeline to enforce
// boundaries regardless of stage order.
func (f *CompactCallFormatter) OwnedSpans(src []byte) llast.OffsetSpanSet {
	// For now, treat all scan-selectable calls as owned when fallback is
	// enabled, and targeted calls as owned otherwise. This errs on the side
	// of preventing stage fighting.
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
				owned = append(
					owned, llast.OffsetSpan{
						Start: i,
						End:   endIdx + 1,
					},
				)
				i = endIdx + 1
				continue
			}
		}

		if f.cfg.FallbackNonTargets {
			if next, ok := f.tryOwnGenericCall(src, i, &owned); ok {
				i = next
				continue
			}
		}

		i++
	}

	return llast.NewOffsetSpanSet(owned)
}

func (f *CompactCallFormatter) tryOwnGenericCall(src []byte, start int,
	owned *[]llast.OffsetSpan) (int, bool) {

	callStart, callEnd := findGenericCallAt(src, start)
	if callEnd <= callStart {
		return start, false
	}
	if !f.shouldOwnGenericCall(src, callStart, callEnd) {
		return callEnd, true
	}

	*owned = append(
		*owned, llast.OffsetSpan{
			Start: callStart,
			End:   callEnd,
		},
	)

	return callEnd, true
}

func (f *CompactCallFormatter) shouldOwnGenericCall(src []byte, start int,
	end int) bool {

	// Avoid claiming ownership of calls that contain inline comments;
	// rewriting those can cause non-idempotent comment attachment across
	// pipeline runs.
	span := src[start:end]
	if spanHasCommentOutsideStrings(span) {
		return false
	}

	// Keep ownership consistent with formatting selection: excluded calls
	// should not be considered owned by this stage.
	openRel := bytes.IndexByte(span, '(')
	if openRel <= 0 {
		return true
	}

	callee := strings.TrimSpace(string(src[start : start+openRel]))
	if callNameContainsAny(callee, f.cfg.Excludes) {
		return false
	}
	if !f.cfg.FallbackNonTargetsExcludeSelectors {
		return true
	}
	if isSelectorChainCallStart(src, start) ||
		strings.Contains(callee, ".") {
		return false
	}

	return true
}

// NewCompactCallFormatter creates a new compact packing formatter with
// defaults.
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
var fallbackNonTargetsExcludeSelectors = false
var fallbackNonTargetsExcludes []string
var skipGofmt = false

func defaultTargets() []string {
	return []string{
		"log.Infof(", "log.Debugf(", "log.Tracef(", "log.Errorf(", "log.Warnf(",
		"log.Fatalf(",
		"fmt.Printf(", "fmt.Sprintf(", "fmt.Errorf(",
	}
}

// FormatCompactCallsInSource applies the compact-call stage formatting to src
// using cfg and reports whether it changed anything.
//
// This exists so DSL parity stages can delegate to the legacy implementation
// without importing the formatter package from the dsl package.
func FormatCompactCallsInSource(src []byte, cfg Config) ([]byte, bool) {
	if cfg.ColumnLimit <= 0 {
		cfg.ColumnLimit = DefaultColumnLimit
	}
	if cfg.TabStop <= 0 {
		cfg.TabStop = DefaultTabStop
	}
	if len(cfg.Targets) == 0 {
		cfg.Targets = defaultTargets()
	}
	// Pipelines run gofmt at the end; keep compact-call stage gofmt-free.
	cfg.SkipGofmt = true

	out := NewCompactCallFormatter(cfg).FormatFile(src)

	return out, !bytes.Equal(out, src)
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
	fallbackNonTargetsExcludeSelectors = f.
		cfg.
		FallbackNonTargetsExcludeSelectors
	fallbackNonTargetsExcludes = append([]string{}, f.cfg.Excludes...)
	skipGofmt = f.cfg.SkipGofmt
	currentTargets = f.cfg.Targets

	var out []byte
	if f.cfg.UseASTSelection {
		out = formatWithTargetsAST(src, f.cfg.Targets)
	} else {
		out = formatWithTargetsScan(src, f.cfg.Targets)
	}

	if f.cfg.ParseSafe {
		// Validate parseability using go/format (it parses and
		// pretty-prints). We intentionally keep the original output
		// (rather than the formatted result) to avoid changing stage
		// interactions in pipelines that rely on raw source layout for
		// selection heuristics.
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
		if next, handled := copyNonCodeSpan(&out, src, i); handled {
			i = next
			continue
		}

		matched := findTargetAt(src, i, targets)
		if matched == "" {
			// If enabled, try fallback formatting for non-target
			// calls.
			if next, handled := maybeFormatNonTarget(
				&out, src, i,
			); handled {

				i = next
				continue
			}
			out.WriteByte(src[i])
			i++
			continue
		}

		if next, handled := formatTargetCall(
			&out, src, i, matched,
		); handled {

			i = next
			continue
		}
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

func findTargetAt(src []byte, i int, targets []string) string {
	for _, t := range targets {
		if hasPrefixAt(src, i, t) {
			return t
		}
	}

	return ""
}

func formatTargetCall(out *bytes.Buffer, src []byte, callStart int,
	matched string) (next int, handled bool) {

	if matched == "" {
		return 0, false
	}

	// Find the opening parenthesis index right after the target.
	openIdx := callStart + len(matched) - 1 // points to '('
	endIdx := scanner.ScanBalancedParen(src, openIdx)
	if endIdx <= openIdx {
		// Could not find a balanced call; copy verbatim and continue to
		// avoid mangling.
		out.Write(src[callStart : callStart+len(matched)])

		return callStart + len(matched), true
	}

	// Split around to get indent and call head.
	lineStart := text.LastLineStart(src, callStart)
	// indentBytes is the entire slice from line start to call start (may
	// include non-whitespace like "return ").
	indentBytes := src[lineStart:callStart]
	// wsIndent is only the leading whitespace of the line.
	wsIndent := text.LeadingWhitespace(src, lineStart)
	baseLen := visualLen(string(indentBytes))
	trimmedPrefix := strings.TrimSpace(string(indentBytes))
	if trimmedPrefix == "return" ||
		strings.HasPrefix(trimmedPrefix, "return ") {

		baseLen += trailingSameLineWidth(src, endIdx+1)
	}

	// Build formatted call.
	formatted := formatCallGreedy(
		src[callStart:endIdx+1], string(wsIndent), baseLen,
	)
	out.WriteString(formatted)

	return endIdx + 1, true
}

func trailingSameLineWidth(src []byte, start int) int {
	lineEnd := start
	for lineEnd < len(src) && src[lineEnd] != '\n' {
		lineEnd++
	}
	if lineEnd <= start {
		return 0
	}
	suffix := string(src[start:lineEnd])
	if !strings.HasPrefix(strings.TrimSpace(suffix), ",") {
		return 0
	}

	return visualLen(suffix)
}

func copyNonCodeSpan(out *bytes.Buffer, src []byte,
	i int) (next int, handled bool) {

	if scanner.IsStringStart(src, i) {
		start := i
		next = scanner.ScanString(src, i)
		out.Write(src[start:next])

		return next, true
	}
	if scanner.IsLineCommentStart(src, i) {
		start := i
		next = scanner.ScanLineComment(src, i)
		out.Write(src[start:next])

		return next, true
	}
	if scanner.IsBlockCommentStart(src, i) {
		start := i
		next = scanner.ScanBlockComment(src, i)
		out.Write(src[start:next])

		return next, true
	}

	return 0, false
}

func maybeFormatNonTarget(out *bytes.Buffer, src []byte,
	i int) (next int, handled bool) {

	if !fallbackNonTargets {
		return 0, false
	}

	if start, end := findGenericCallAt(src, i); end > start {
		return formatFallbackCall(out, src, start, end)
	}

	if text.IsIdentifierStart(src[i]) {
		j := i + 1
		for j < len(src) && text.IsIdentifierChar(src[j]) {
			j++
		}
		out.Write(src[i:j])

		return j, true
	}

	return 0, false
}

func formatFallbackCall(out *bytes.Buffer, src []byte, start,
	end int) (next int, handled bool) {

	span := src[start:end]
	if spanHasCommentOutsideStrings(span) {
		out.Write(span)

		return end, true
	}

	if isCallNameExcluded(src, start, end) {
		out.Write(span)

		return end, true
	}

	if fallbackNonTargetsExcludeSelectors &&
		isSelectorFallbackExcluded(src, start, end) {

		out.Write(span)

		return end, true
	}

	lineStart := text.LastLineStart(src, start)
	indentPrefix := string(src[lineStart:start])
	allowedByPrefix := isCallAllowedByPrefix(indentPrefix) ||
		isSelectorChainCallStart(src, start)
	if !allowedByPrefix {
		out.Write(span)

		return end, true
	}

	indentPrefix = normalizeShortChainPrefix(src, lineStart, indentPrefix)
	callText := string(src[start:end])
	flat := strings.Join(strings.Fields(stripComments(callText)), " ")
	currentLineLen := visualLen(indentPrefix) + visualLen(flat)
	if currentLineLen <= columnLimit ||
		isChainedShortCall(src, start, end) {

		out.Write(span)

		return end, true
	}

	wsIndent := string(text.LeadingWhitespace(src, lineStart))
	formatted := formatCallPackedMultiLine(
		src[start:end], wsIndent, wsIndent, true,
	)
	out.WriteString(formatted)

	return end, true
}

func isCallNameExcluded(src []byte, start, end int) bool {
	openRel := bytes.IndexByte(src[start:end], '(')
	if openRel <= 0 {
		return false
	}

	callee := strings.TrimSpace(string(src[start : start+openRel]))

	return callNameContainsAny(callee, fallbackNonTargetsExcludes)
}

func isSelectorFallbackExcluded(src []byte, start, end int) bool {
	// Exclude both explicit selector callees (`pkg.Func`) and method-chain
	// selector calls (`x.Foo().Bar(...)`), which the legacy scan-based
	// matcher starts at `Bar` and therefore does not include the '.' in the
	// callee text.
	if isSelectorChainCallStart(src, start) {
		return true
	}

	openRel := bytes.IndexByte(src[start:end], '(')
	if openRel <= 0 {
		return false
	}

	callee := strings.TrimSpace(string(src[start : start+openRel]))

	return strings.Contains(callee, ".")
}

func isCallAllowedByPrefix(prefix string) bool {
	// Avoid formatting nested calls within larger expressions. For example,
	// in: a && b && verifySignature(sig) || c we want the expression stage
	// to handle breaking rather than rewriting the call itself.
	//
	// This fallback is intended for statement-level calls such as:
	// - assignment RHS: `x := foo(...)`, `x = foo(...)`
	// - return/go/defer: `return foo(...)`, `go foo(...)`, `defer foo(...)`
	// - standalone call: `foo(...)`
	trimmedPrefix := strings.TrimSpace(prefix)

	return trimmedPrefix == "" ||
		strings.HasSuffix(trimmedPrefix, ":=") ||
		strings.HasSuffix(trimmedPrefix, "=") ||
		strings.HasSuffix(trimmedPrefix, "return") ||
		strings.HasSuffix(trimmedPrefix, "go") ||
		strings.HasSuffix(trimmedPrefix, "defer")
}

func normalizeShortChainPrefix(src []byte, lineStart int,
	indentPrefix string) string {

	// When the call is part of a short method chain right after a closing
	// paren, ignore the preceding ')' and optional '.' for width
	// calculation so short calls like ".Limit(100)" stay on one line.
	tp := strings.TrimSpace(indentPrefix)
	if tp == ")" || tp == ")." {
		return string(text.LeadingWhitespace(src, lineStart))
	}

	return indentPrefix
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

func isSelectorChainCallStart(src []byte, start int) bool {
	i := start - 1
	for i >= 0 && (src[i] == ' ' || src[i] == '\t') {
		i--
	}

	return i >= 0 && src[i] == '.'
}

// findGenericCallAt attempts to find a function call starting at position i. It
// matches identifiers (including selectors like pkg.Func) immediately followed
// by '(', and excludes function/method declarations.
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

// isFunctionDefinitionAt checks if this looks like a function or method
// definition.
func isFunctionDefinitionAt(src []byte, i int) bool {
	j := i - 1
	for j >= 0 && (src[j] == ' ' || src[j] == '\t') {
		j--
	}
	if j >= 3 && string(src[j-3:j+1]) == "func" {
		return true
	}
	// Also scan back on the line for a method definition like: func (r R)
	// Name(
	k := j
	for k >= 0 && src[k] != '\n' {
		if k >= 3 && string(src[k-3:k+1]) == "func" {
			return true
		}
		k--
	}

	return false
}

func callNameContainsAny(name string, subs []string) bool {
	if name == "" || len(subs) == 0 {
		return false
	}
	for _, sub := range subs {
		if sub == "" {
			continue
		}
		if strings.Contains(name, sub) {
			return true
		}
	}

	return false
}

func spanHasCommentOutsideStrings(b []byte) bool {
	i := 0
	for i < len(b) {
		if scanner.IsStringStart(b, i) {
			i = scanner.ScanString(b, i)
			continue
		}
		if scanner.IsLineCommentStart(b, i) ||
			scanner.IsBlockCommentStart(b, i) {
			return true
		}
		i++
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
			i, inStr, esc = advanceStringState(s, i, inStr, esc)
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
				i = skipLineComment(s, i)
				continue
			}
			if s[i+1] == '*' {
				i = skipBlockComment(s, i)
				continue
			}
		}
		b.WriteByte(c)
		i++
	}

	return b.String()
}

func advanceStringState(s string, i int, inStr byte, esc bool) (int, byte,
	bool) {

	c := s[i]
	if inStr == '"' && c == '\\' && !esc {
		return i + 1, inStr, true
	}
	if esc {
		return i + 1, inStr, false
	}
	if c == inStr {
		return i + 1, 0, false
	}

	return i + 1, inStr, false
}

func skipLineComment(s string, i int) int {
	for i < len(s) && s[i] != '\n' {
		i++
	}

	return i
}

func skipBlockComment(s string, i int) int {
	i += 2
	for i+1 < len(s) {
		if s[i] == '*' && s[i+1] == '/' {
			return i + 2
		}
		i++
	}

	return i
}

// formatCallPackedMultiLine formats a generic function call into a packed
// multi-line style when the single-line form would exceed the column limit.
// Rules:
//   - Emit head and opening paren, then a newline.
//   - Pack arguments separated by ", " on continuation lines of wsIndent+"\t".
//   - Break to a new continuation line when adding the next arg would exceed
//     the column limit; place the comma at the end of the current line.
//   - Always place the closing ")" on its own line aligned with the start of
//     the function name (using closingIndent from the source start-of-line to
//     call).
//   - Emit a trailing comma after the final argument to stabilize layout.
func formatCallPackedMultiLine(call []byte, wsIndent, fullPrefix string,
	trailingComma bool) string {

	s := string(call)
	open := strings.IndexByte(s, '(')
	if open == -1 || !strings.HasSuffix(s, ")") {
		return s
	}
	head := s[:open]
	argsBody := s[open+1 : len(s)-1]
	// Split arguments respecting all bracket types so composites aren't
	// broken.
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
	// Packed multiline always terminates argument lines with a comma
	// (either when breaking before the next arg, or via a trailing comma
	// before the closing paren). Reserve one column so we don't "pack" to
	// the exact column limit and then overflow by 1 when emitting that
	// comma.
	lineWidth := width
	if lineWidth > 0 {
		lineWidth--
	}
	contIndent := wsIndent + "\t"
	contIndentLen := visualLen(contIndent)
	curLenAfterWrite := func(s string) int {
		if strings.Contains(s, "\n") {

			// Multiline formatted args include their own
			// indentation on continuation lines, so the last line
			// length already reflects the active column.
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
	stringCfg := stringLitArgConfig{
		contIndent:       contIndent,
		contIndentLen:    contIndentLen,
		lineWidth:        lineWidth,
		width:            width,
		curLenAfterWrite: curLenAfterWrite,
	}
	for idx, raw := range args {
		a := strings.TrimSpace(raw)
		if a == "" {
			continue
		}
		// After a multiline call expression, always break before next
		// arg. After a multiline composite, the break logic is handled
		// later in shouldBreak.
		forcedBreak := !first && prevWasMultiline && prevWasCall

		// Handle string literals (double-quoted) with potential
		// splitting.
		state := callArgState{
			curLen:                 curLen,
			first:                  first,
			prevWasMultiline:       prevWasMultiline,
			prevWasCall:            prevWasCall,
			seenMultilineCall:      seenMultilineCall,
			seenMultilineComposite: seenMultilineComposite,
		}
		hasMore := hasMoreNonEmptyArgs(args, idx)
		if handled, next := formatPackedStringLitArg(
			&b, a, forcedBreak, hasMore, stringCfg, state,
		); handled {

			curLen = next.curLen
			first = next.first
			prevWasMultiline = next.prevWasMultiline
			continue
		}
		// Pretty-format composite literals only for keyed maps/structs;
		// keep slices/arrays inline to avoid over-wrapping short
		// literals. If we've already seen a multiline call, force
		// expand maps/structs.
		if fa, ok := FormatCompositeLiteralArg(
			a, contIndent, seenMultilineCall,
		); ok {

			a = fa
		}
		// If the argument is itself a call expression and it doesn't
		// fit, recursively format it in packed multiline style without
		// adding a trailing comma inside.
		state = callArgState{
			curLen:                 curLen,
			first:                  first,
			prevWasMultiline:       prevWasMultiline,
			prevWasCall:            prevWasCall,
			seenMultilineCall:      seenMultilineCall,
			seenMultilineComposite: seenMultilineComposite,
		}
		if handled, next := formatPackedCallExprArg(
			&b, a, contIndent, contIndentLen, lineWidth,
			forcedBreak, curLenAfterWrite, state,
		); handled {

			curLen = next.curLen
			first = next.first
			prevWasMultiline = next.prevWasMultiline
			prevWasCall = next.prevWasCall
			seenMultilineCall = next.seenMultilineCall
			seenMultilineComposite = next.seenMultilineComposite
			continue
		}
		if first {
			b.WriteString(contIndent)
			b.WriteString(a)
			// After writing possibly multi-line arg, update curLen
			// to length of last line.
			curLen = curLenAfterWrite(a)
			first = false
			// If this arg spans multiple lines, mark it and force
			// subsequent breaks.
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
		// Determine if we should break to a new line: 1. If we've seen
		// a multiline call, all subsequent args break (including
		// multiline) 2. If we've seen a multiline composite (no call)
		// and current is single-line, break 3. If current wouldn't fit,
		// break But if we've only seen multiline composites (no calls)
		// and current is also multiline, stay on same line (e.g., "},
		// []string{" in Configure)
		shouldBreakAfterCall := seenMultilineCall
		shouldBreakAfterComposite := seenMultilineComposite &&
			!seenMultilineCall
		shouldBreak := curLen+need > lineWidth || forcedBreak ||
			shouldBreakAfterCall || shouldBreakAfterComposite
		if shouldBreak {
			// Break line after comma from previous arg.
			b.WriteByte(',')
			b.WriteByte('\n')
			b.WriteString(contIndent)
			b.WriteString(a)
			curLen = curLenAfterWrite(a)
		} else {
			b.WriteString(", ")
			b.WriteString(a)
			curLen += need
			// If arg had newlines, reset curLen to last line
			// length.
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

type callArgState struct {
	curLen                 int
	first                  bool
	prevWasMultiline       bool
	prevWasCall            bool
	seenMultilineCall      bool
	seenMultilineComposite bool
}

func formatPackedCallExprArg(b *strings.Builder, arg string, contIndent string,
	contIndentLen int, lineWidth int, forcedBreak bool,
	curLenAfterWrite func(string) int,
	state callArgState) (bool, callArgState) {

	if !llast.IsCallExpr(arg) {
		return false, state
	}

	// Greedy, algorithmic rule for nested calls: inline if the entire call
	// fits on the current line and it contains no always-multiline
	// composites and no nested calls among its direct args; otherwise
	// reflow recursively and start on a fresh continuation line.
	fits := false
	if state.first {
		fits = advanceCols(contIndentLen, arg) <= lineWidth
	} else {
		fits = advanceCols(state.curLen+2, arg) <= lineWidth
	}
	hasAlways := callHasAlwaysMultilineComposite(arg)
	hasNested := llast.HasNestedCall(arg)
	if fits && !hasAlways && !hasNested {
		return true, writeInlineCallArg(
			b, arg, contIndent, contIndentLen, forcedBreak, state,
		)
	}

	nested := formatCallPackedMultiLine(
		[]byte(arg), contIndent, contIndent, true,
	)
	state = writeNestedCallArg(
		b, nested, contIndent, curLenAfterWrite, state,
	)

	return true, state
}

func writeInlineCallArg(b *strings.Builder, arg string, contIndent string,
	contIndentLen int, forcedBreak bool, state callArgState) callArgState {

	if state.first {
		b.WriteString(contIndent)
		b.WriteString(arg)
		state.curLen = contIndentLen + firstLineLen(arg)
		state.first = false
		state.prevWasMultiline = false

		return state
	}

	if forcedBreak {
		b.WriteByte(',')
		b.WriteByte('\n')
		b.WriteString(contIndent)
		state.curLen = contIndentLen
	} else {
		b.WriteString(", ")
	}
	b.WriteString(arg)
	state.curLen = advanceCols(state.curLen+2, arg)
	state.prevWasMultiline = false

	return state
}

func writeNestedCallArg(b *strings.Builder, nested string, contIndent string,
	curLenAfterWrite func(string) int, state callArgState) callArgState {

	if state.first {
		b.WriteString(contIndent)
		b.WriteString(nested)
		state.curLen = curLenAfterWrite(nested)
		state.first = false

		return markNestedCallState(nested, state)
	}

	b.WriteByte(',')
	b.WriteByte('\n')
	b.WriteString(contIndent)
	b.WriteString(nested)
	state.curLen = curLenAfterWrite(nested)
	state.first = false

	return markNestedCallState(nested, state)
}

func markNestedCallState(nested string, state callArgState) callArgState {
	nestedMulti := strings.Contains(nested, "\n")
	state.prevWasMultiline = nestedMulti
	state.prevWasCall = true
	if nestedMulti {
		state.seenMultilineCall = true
	}

	return state
}

// stringLitArgConfig holds configuration for formatPackedStringLitArg.
type stringLitArgConfig struct {
	contIndent       string
	contIndentLen    int
	lineWidth        int
	width            int
	curLenAfterWrite func(string) int
}

// formatPackedStringLitArg handles string literal arguments in packed multiline
// call formatting. It returns whether the argument was handled and the updated
// state.
func formatPackedStringLitArg(b *strings.Builder, arg string, forcedBreak bool,
	hasMoreArgs bool, cfg stringLitArgConfig,
	state callArgState) (bool, callArgState) {

	e, err := parser.ParseExpr(arg)
	if err != nil {
		return false, state
	}
	text, ok := llast.FlattenStringExprAST(e)
	if !ok {
		return false, state
	}

	if state.first || forcedBreak {
		state = emitStringLitOnFreshLine(
			b, text, forcedBreak, cfg, state,
		)

		return true, state
	}

	plain := quoteGoString(text)
	if advanceCols(state.curLen+2, plain) <= cfg.lineWidth {
		b.WriteString(", ")
		b.WriteString(plain)
		state.curLen = advanceCols(state.curLen+2, plain)
		state.first = false

		return true, state
	}

	state = emitStringLitOverflow(b, text, plain, hasMoreArgs, cfg, state)

	return true, state
}

// emitStringLitOnFreshLine emits a string literal at continuation indent,
// optionally splitting it if needed.
func emitStringLitOnFreshLine(b *strings.Builder, text string, forcedBreak bool,
	cfg stringLitArgConfig, state callArgState) callArgState {

	split := buildSplitQuoted(
		text, cfg.contIndentLen, cfg.contIndent, cfg.width,
	)
	if forcedBreak {
		b.WriteByte(',')
		b.WriteByte('\n')
		b.WriteString(cfg.contIndent)
	} else {
		b.WriteString(cfg.contIndent)
	}
	b.WriteString(split)
	state.curLen = cfg.curLenAfterWrite(split)
	state.first = false

	return state
}

// emitStringLitOverflow handles a string literal that doesn't fit inline.
func emitStringLitOverflow(b *strings.Builder, text, plain string,
	hasMoreArgs bool, cfg stringLitArgConfig,
	state callArgState) callArgState {

	// If this is the last argument and fits whole on next line, place it
	// there.
	if !hasMoreArgs && advanceCols(cfg.contIndentLen, plain) <= cfg.width {
		b.WriteByte(',')
		b.WriteByte('\n')
		b.WriteString(cfg.contIndent)
		b.WriteString(plain)
		state.curLen = cfg.contIndentLen + firstLineLen(plain)
		state.first = false

		return state
	}

	// Try to split starting on the current line.
	tentative := buildSplitQuoted(
		text, state.curLen+2, cfg.contIndent, cfg.width,
	)
	need := 2 + firstLineLen(tentative)
	if state.curLen+need <= cfg.width {
		b.WriteString(", ")
		b.WriteString(tentative)
		if strings.Contains(tentative, "\n") {
			state.curLen = lastLineLen(tentative)
		} else {
			state.curLen += need
		}
		state.first = false

		return state
	}

	// Start on a fresh continuation line and split there.
	b.WriteByte(',')
	b.WriteByte('\n')
	b.WriteString(cfg.contIndent)
	split := buildSplitQuoted(
		text, cfg.contIndentLen, cfg.contIndent, cfg.width,
	)
	b.WriteString(split)
	state.curLen = cfg.curLenAfterWrite(split)
	state.prevWasMultiline = false
	state.first = false

	return state
}

// hasMoreNonEmptyArgs checks if there are more non-empty arguments after idx.
func hasMoreNonEmptyArgs(args []string, idx int) bool {
	for j := idx + 1; j < len(args); j++ {
		if strings.TrimSpace(args[j]) != "" {
			return true
		}
	}

	return false
}

// stringLitArgNextConfig holds configuration for formatPackedStringLitArgNext.
type stringLitArgNextConfig struct {
	contIndent       string
	contIndentLen    int
	lineWidth        int
	width            int
	curLenAfterWrite func(string) int
}

// formatPackedStringLitArgNext handles string literal arguments in packed
// multiline call formatting for "next" style. It returns whether the argument
// was handled and the updated curLen and first values.
func formatPackedStringLitArgNext(b *strings.Builder, arg string, curLen int,
	first, forcedBreak, hasMore bool, cfg stringLitArgNextConfig) (bool,
	int, bool) {

	e, err := parser.ParseExpr(arg)
	if err != nil {
		return false, curLen, first
	}
	text, ok := llast.FlattenStringExprAST(e)
	if !ok {
		return false, curLen, first
	}

	if first || forcedBreak {
		curLen, first = emitStringLitOnFreshLineNext(
			b, text, forcedBreak, hasMore, cfg,
		)

		return true, curLen, first
	}

	plain := quoteGoString(text)
	if advanceCols(curLen+2, plain) <= cfg.lineWidth {
		b.WriteString(", ")
		b.WriteString(plain)

		return true, advanceCols(curLen+2, plain), false
	}

	curLen, first = emitStringLitOverflowNext(
		b, text, plain, curLen, hasMore, cfg,
	)

	return true, curLen, first
}

// emitStringLitOnFreshLineNext emits a string literal at continuation indent
// for "next" style.
func emitStringLitOnFreshLineNext(b *strings.Builder, text string, forcedBreak,
	hasMore bool, cfg stringLitArgNextConfig) (curLen int, first bool) {

	width := stringLitArgNextWidth(cfg, hasMore)
	split := buildSplitQuotedForCallArg(
		text, cfg.contIndentLen, cfg.contIndent, width, hasMore,
	)
	if forcedBreak {
		b.WriteByte(',')
		b.WriteByte('\n')
		b.WriteString(cfg.contIndent)
	} else {
		b.WriteString(cfg.contIndent)
	}
	b.WriteString(split)

	return cfg.curLenAfterWrite(split), false
}

// emitStringLitOverflowNext handles a string literal that doesn't fit inline
// for "next" style.
func emitStringLitOverflowNext(b *strings.Builder, text, plain string,
	curLen int, hasMore bool, cfg stringLitArgNextConfig) (int, bool) {

	width := stringLitArgNextWidth(cfg, hasMore)
	// If this is the last argument and fits whole on next line, place it
	// there.
	if !hasMore && advanceCols(cfg.contIndentLen, plain) <= width {
		b.WriteByte(',')
		b.WriteByte('\n')
		b.WriteString(cfg.contIndent)
		b.WriteString(plain)

		return cfg.contIndentLen + firstLineLen(plain), false
	}
	if hasMore && !strings.Contains(text, " ") &&
		advanceCols(cfg.contIndentLen, plain) <= width {

		b.WriteByte(',')
		b.WriteByte('\n')
		b.WriteString(cfg.contIndent)
		b.WriteString(plain)

		return cfg.contIndentLen + firstLineLen(plain), false
	}

	// Try to split starting on the current line.
	tentative := buildSplitQuotedForCallArg(
		text, curLen+2, cfg.contIndent, width, hasMore,
	)
	need := 2 + firstLineLen(tentative)
	if curLen+need <= width {
		b.WriteString(", ")
		b.WriteString(tentative)
		if strings.Contains(tentative, "\n") {
			return lastLineLen(tentative), false
		}

		return curLen + need, false
	}

	// Start on a fresh continuation line and split there.
	b.WriteByte(',')
	b.WriteByte('\n')
	b.WriteString(cfg.contIndent)
	split := buildSplitQuotedForCallArg(
		text, cfg.contIndentLen, cfg.contIndent, width, hasMore,
	)
	b.WriteString(split)

	return cfg.curLenAfterWrite(split), false
}

func stringLitArgNextWidth(cfg stringLitArgNextConfig, hasMore bool) int {
	return cfg.lineWidth
}

// formatCallPackedMultiLineNext is an opt-in variant of
// formatCallPackedMultiLine intended for "next" style formatting. It makes two
// key changes:
//   - Avoids recursively reflowing nested call args (e.g. len(...)) when they
//     fit on their own continuation line, preventing ugly nested expansions.
//   - Uses call-argument-aware string splitting that matches gofmt's spacing
//     around '+' when there are trailing call arguments.
func formatCallPackedMultiLineNext(call []byte, wsIndent, fullPrefix string,
	trailingComma bool) string {

	return formatCallPackedMultiLineNextInternal(
		call, wsIndent, fullPrefix, trailingComma, false,
	)
}

func formatCallPackedMultiLineNextInternal(call []byte, wsIndent,
	fullPrefix string, trailingComma bool,
	firstStringArgInline bool) string {

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
	// Packed multiline always terminates argument lines with a comma
	// (either when breaking before the next arg, or via a trailing comma
	// before the closing paren). Reserve one column so we don't "pack" to
	// the exact column limit and then overflow by 1 when emitting that
	// comma.
	lineWidth := width
	if lineWidth > 0 {
		lineWidth--
	}
	contIndent := wsIndent + "\t"
	if strings.Contains(head, "\n") {
		lastHeadLine := head[strings.LastIndexByte(head, '\n')+1:]
		closingIndent = leadingWhitespace(lastHeadLine)
		contIndent = closingIndent + "\t"
	}
	contIndentLen := visualLen(contIndent)
	curLenAfterWrite := func(s string) int {
		if strings.Contains(s, "\n") {
			return lastLineLen(s)
		}

		return contIndentLen + firstLineLen(s)
	}

	if formattedHead, ok := formatGenericCallHeadNext(
		head, wsIndent, fullPrefix,
	); ok {

		head = formattedHead
	}
	if firstStringArgInline {
		if formatted, ok := formatCallFirstStringArgInlineNext(
			head, args, wsIndent, contIndent, lineWidth,
		); ok {
			return formatted
		}
	}

	var b strings.Builder
	b.WriteString(head)
	b.WriteByte('(')
	b.WriteByte('\n')

	curLen := contIndentLen
	first := true
	seenMultilineCall := false
	seenMultilineComposite := false
	prevArgWasCall := false

	// When a call consists only of function literal arguments (e.g.
	// `handle(func() error { ... }, func() error { ... })`), prefer
	// expanding each func literal body into a multiline block for
	// readability.
	//
	// For mixed-argument calls, keep func literals inline so packed layouts
	// remain stable (see testdata/multiline/output_next.go).
	allFuncLitArgs := true
	seenNonEmpty := false
	for _, raw := range args {
		a := strings.TrimSpace(raw)
		if a == "" {
			continue
		}
		seenNonEmpty = true
		e, err := parser.ParseExpr(a)
		if err != nil {
			allFuncLitArgs = false
			break
		}
		if _, ok := e.(*ast.FuncLit); !ok {
			allFuncLitArgs = false
			break
		}
	}
	if !seenNonEmpty {
		allFuncLitArgs = false
	}

	for idx, raw := range args {
		a := strings.TrimSpace(raw)
		if a == "" {
			continue
		}
		if allFuncLitArgs {
			if expanded, ok := expandFuncLitArgBodyNext(
				a, contIndent,
			); ok {

				a = expanded
			}
		}
		trimmedArg := strings.TrimSpace(a)
		curIsFuncLit := strings.HasPrefix(trimmedArg, "func(") ||
			strings.HasPrefix(trimmedArg, "func ")
		// Always start function literals on a fresh line to avoid
		// awkward `}, func(...) { ... }` packing.
		forcedBreak := !first && curIsFuncLit

		// Handle string literals with potential splitting.
		hasMore := hasMoreNonEmptyArgs(args, idx)
		stringCfg := stringLitArgNextConfig{
			contIndent:       contIndent,
			contIndentLen:    contIndentLen,
			lineWidth:        lineWidth,
			width:            width,
			curLenAfterWrite: curLenAfterWrite,
		}
		forceStringBreak := forcedBreak || (!first && prevArgWasCall)
		if handled, newLen, newFirst := formatPackedStringLitArgNext(
			&b, a, curLen, first, forceStringBreak, hasMore,
			stringCfg,
		); handled {

			curLen = newLen
			first = newFirst
			prevArgWasCall = false
			continue
		}

		if fa, ok := FormatCompositeLiteralArg(
			a, contIndent, seenMultilineCall,
		); ok {

			a = fa
		}
		a = formatGenericCompositeArgIfOverflowsNext(
			a, contIndent, lineWidth,
		)

		// Handle call expression arguments with potential nesting.
		callState := &callExprArgNextState{
			b:             &b,
			contIndent:    contIndent,
			contIndentLen: contIndentLen,
			lineWidth:     lineWidth,
			curLen:        curLen,
			first:         first,
			forcedBreak: forcedBreak || (!first &&
				(seenMultilineCall || seenMultilineComposite)),
			seenMultilineCall:      seenMultilineCall,
			seenMultilineComposite: seenMultilineComposite,
		}
		if callState.handleCallExprArgNext(a) {
			curLen = callState.curLen
			first = callState.first
			seenMultilineCall = callState.seenMultilineCall
			prevArgWasCall = true
			continue
		}

		a = formatBinaryArgIfOverflowsNext(a, contIndent, lineWidth)

		if first {
			b.WriteString(contIndent)
			b.WriteString(a)
			curLen = curLenAfterWrite(a)
			first = false
			if strings.Contains(a, "\n") {
				seenMultilineComposite = true
			}
			prevArgWasCall = llast.IsCallExpr(a)
			continue
		}

		need := 2 + firstLineLen(a)
		argIsMultiline := strings.Contains(a, "\n")
		shouldBreakAfterCall := seenMultilineCall
		shouldBreakAfterComposite := seenMultilineComposite &&
			!seenMultilineCall && !argIsMultiline
		shouldBreak := curLen+need > lineWidth || forcedBreak ||
			shouldBreakAfterCall || shouldBreakAfterComposite
		if shouldBreak {
			b.WriteByte(',')
			b.WriteByte('\n')
			b.WriteString(contIndent)
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
		prevArgWasCall = llast.IsCallExpr(a)
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

func formatCallFirstStringArgInlineNext(head string, args []string, wsIndent,
	contIndent string, lineWidth int) (string, bool) {

	if strings.Contains(head, "\n") || len(args) < 2 {
		return "", false
	}

	first := strings.TrimSpace(args[0])
	firstText, ok := flattenedStringExprText(first)
	if first == "" || !ok {
		return "", false
	}

	firstLiteral := quoteGoString(firstText)
	firstLine := head + "(" + firstLiteral + ","
	if visualLen(wsIndent)+firstLineLen(firstLine) > lineWidth {
		firstLine, ok = formatCallSplitFirstStringArgInlineNext(
			head, firstText, wsIndent, contIndent, lineWidth,
		)
		if !ok {
			return "", false
		}
	}

	var b strings.Builder
	b.WriteString(firstLine)

	firstRestAfterInlineString := strings.Contains(firstLine, "\n")
	curLen := lastLineLen(firstLine)
	if !firstRestAfterInlineString {
		b.WriteByte('\n')
		b.WriteString(contIndent)
		curLen = visualLen(contIndent)
	}

	wroteRest := false
	for _, raw := range args[1:] {
		arg := strings.TrimSpace(raw)
		if arg == "" || strings.Contains(arg, "\n") {
			return "", false
		}
		sep := ""
		if wroteRest {
			sep = ", "
		} else if firstRestAfterInlineString {
			sep = " "
		}
		need := firstLineLen(sep) + firstLineLen(arg)
		if (wroteRest || firstRestAfterInlineString) &&
			curLen+need > lineWidth {

			if wroteRest {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
			b.WriteString(contIndent)
			b.WriteString(arg)
			curLen = visualLen(contIndent) + firstLineLen(arg)
		} else {
			b.WriteString(sep)
			b.WriteString(arg)
			curLen += need
		}
		firstRestAfterInlineString = false
		wroteRest = true
	}
	if !wroteRest {
		return "", false
	}
	if curLen+1 > lineWidth {
		b.WriteByte('\n')
		b.WriteString(wsIndent)
		b.WriteByte(')')
	} else {
		b.WriteByte(')')
	}

	return b.String(), true
}

func formatCallSplitFirstStringArgInlineNext(head, firstText, wsIndent,
	contIndent string, lineWidth int) (string, bool) {

	startCol := visualLen(wsIndent) + firstLineLen(head+"(")
	split := buildSplitQuotedForCallArg(
		firstText, startCol, contIndent, lineWidth, true,
	)
	if !strings.Contains(split, "\n") {
		return "", false
	}

	firstLine := head + "(" + split + ","
	if !replacementTextLinesFitWidth(wsIndent, firstLine, lineWidth) {
		return "", false
	}

	return firstLine, true
}

func replacementTextLinesFitWidth(prefix, text string, lineWidth int) bool {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		candidate := line
		if i == 0 {
			candidate = prefix + line
		}
		if visualLen(candidate) > lineWidth {
			return false
		}
	}

	return true
}

func flattenedStringExprText(s string) (string, bool) {
	expr, err := parser.ParseExpr(s)
	if err != nil {
		return "", false
	}

	return llast.FlattenStringExprAST(expr)
}

func formatGenericCallHeadNext(head, wsIndent,
	fullPrefix string) (string, bool) {

	if fullPrefix == "" {
		fullPrefix = wsIndent
	}
	if visualLen(fullPrefix+head+"(") <= columnLimit {
		return "", false
	}
	if spanHasCommentOutsideStrings([]byte(head)) {
		return "", false
	}

	fset := token.NewFileSet()
	expr, err := parser.ParseExprFrom(fset, "", head, parser.AllErrors)
	if err != nil {
		return "", false
	}
	idx, ok := expr.(*ast.IndexListExpr)
	if ok {
		return formatGenericIndexListHeadNext(head, wsIndent, fset, idx)
	}

	outerIdx, ok := expr.(*ast.IndexExpr)
	if !ok {
		return "", false
	}
	nestedIdx, ok := outerIdx.Index.(*ast.IndexListExpr)
	if !ok {
		return "", false
	}

	return formatNestedGenericHeadNext(head, wsIndent, fset, nestedIdx)
}

func formatGenericIndexListHeadNext(head, wsIndent string, fset *token.FileSet,
	idx *ast.IndexListExpr) (string, bool) {

	if idx.Lbrack == token.NoPos || idx.Rbrack == token.NoPos {
		return "", false
	}
	open := fset.Position(idx.Lbrack).Offset
	close := fset.Position(idx.Rbrack).Offset
	if open <= 0 || close <= open || close >= len(head) {
		return "", false
	}
	if strings.TrimSpace(head[close+1:]) != "" {
		return "", false
	}

	typeArgs := scanner.SplitTopLevelAny(head[open+1 : close])
	if len(typeArgs) < 2 {
		return "", false
	}

	typeIndent := wsIndent + "\t"
	var b strings.Builder
	b.WriteString(head[:open])
	b.WriteString("[\n")
	for _, raw := range typeArgs {
		arg := strings.TrimSpace(raw)
		if arg == "" {
			continue
		}
		b.WriteString(typeIndent)
		b.WriteString(arg)
		b.WriteString(",\n")
	}
	b.WriteString(wsIndent)
	b.WriteByte(']')

	formatted := b.String()
	if formatted == head {
		return "", false
	}

	return formatted, true
}

func formatNestedGenericHeadNext(head, wsIndent string, fset *token.FileSet,
	idx *ast.IndexListExpr) (string, bool) {

	if idx.Lbrack == token.NoPos || idx.Rbrack == token.NoPos {
		return "", false
	}
	open := fset.Position(idx.Lbrack).Offset
	close := fset.Position(idx.Rbrack).Offset
	if open <= 0 || close <= open || close >= len(head) {
		return "", false
	}

	typeArgs := scanner.SplitTopLevelAny(head[open+1 : close])
	if len(typeArgs) < 2 {
		return "", false
	}

	typeIndent := wsIndent + "\t"
	var b strings.Builder
	b.WriteString(head[:open])
	b.WriteString("[\n")
	for _, raw := range typeArgs {
		arg := strings.TrimSpace(raw)
		if arg == "" {
			continue
		}
		b.WriteString(typeIndent)
		b.WriteString(arg)
		b.WriteString(",\n")
	}
	b.WriteString(wsIndent)
	b.WriteString(head[close:])

	formatted := b.String()
	if formatted == head {
		return "", false
	}

	return formatted, true
}

func formatBinaryArgIfOverflowsNext(arg, contIndent string,
	lineWidth int) string {

	if maxLineLenWithIndentAndComma(arg, contIndent) <= lineWidth {
		return arg
	}

	fset := token.NewFileSet()
	expr, err := parser.ParseExprFrom(fset, "", arg, parser.AllErrors)
	if err != nil {
		return arg
	}
	bin, ok := expr.(*ast.BinaryExpr)
	if !ok || !isBreakableCallArgBinaryOp(bin.Op) {
		return arg
	}

	left := formatExprForPackedArg(fset, bin.X)
	right := formatExprForPackedArg(fset, bin.Y)
	if left == "" || right == "" || strings.Contains(left, "\n") ||
		strings.Contains(right, "\n") {
		return arg
	}

	candidate := left + " " + bin.Op.String() + "\n" +
		contIndent + right
	if maxLineLenWithIndentAndComma(candidate, contIndent) >
		lineWidth {

		candidate = formatBinaryArgLinesNext(fset, bin, contIndent)
		if candidate == "" ||
			maxLineLenWithIndentAndComma(candidate, contIndent) >
				lineWidth {
			return arg
		}
	}
	if maxLineLenWithIndentAndComma(candidate, contIndent) >=
		maxLineLenWithIndentAndComma(arg, contIndent) {
		return arg
	}

	return candidate
}

func formatGenericCompositeArgIfOverflowsNext(arg, contIndent string,
	lineWidth int) string {

	if maxLineLenWithIndentAndComma(arg, contIndent) <= lineWidth {
		return arg
	}
	if spanHasCommentOutsideStrings([]byte(arg)) {
		return arg
	}

	fset := token.NewFileSet()
	expr, err := parser.ParseExprFrom(fset, "", arg, parser.AllErrors)
	if err != nil {
		return arg
	}
	lit, ok := expr.(*ast.CompositeLit)
	if !ok || len(lit.Elts) != 0 {
		return arg
	}

	idx, ok := lit.Type.(*ast.IndexListExpr)
	if !ok || idx.Lbrack == token.NoPos || idx.Rbrack == token.NoPos {
		return arg
	}
	open := fset.Position(idx.Lbrack).Offset
	close := fset.Position(idx.Rbrack).Offset
	if open <= 0 || close <= open || close >= len(arg) {
		return arg
	}

	typeArgs := filterNonEmptyTrimmed(
		scanner.SplitTopLevelAny(arg[open+1 : close]),
	)
	if len(typeArgs) < 2 {
		return arg
	}

	var b strings.Builder
	b.WriteString(arg[:open])
	b.WriteString("[\n")
	typeIndent := contIndent + "\t"
	for _, typeArg := range typeArgs {
		b.WriteString(typeIndent)
		b.WriteString(typeArg)
		b.WriteString(",\n")
	}
	b.WriteString(contIndent)
	b.WriteByte(']')
	b.WriteString(arg[close+1:])

	formatted := b.String()
	if formatted == arg {
		return arg
	}

	return formatted
}

func formatBinaryArgLinesNext(fset *token.FileSet, bin *ast.BinaryExpr,
	contIndent string) string {

	lines := binaryArgLines(fset, bin)
	if len(lines) < 2 {
		return ""
	}

	return strings.Join(lines, "\n"+contIndent)
}

func binaryArgLines(fset *token.FileSet, expr ast.Expr) []string {
	bin, ok := expr.(*ast.BinaryExpr)
	if !ok || !isBreakableCallArgBinaryOp(bin.Op) {
		text := formatExprForPackedArg(fset, expr)
		if text == "" || strings.Contains(text, "\n") {
			return nil
		}

		return []string{text}
	}

	left := binaryArgLines(fset, bin.X)
	right := binaryArgLines(fset, bin.Y)
	if len(left) == 0 || len(right) == 0 {
		return nil
	}
	left[len(left)-1] += " " + bin.Op.String()

	return append(left, right...)
}

func isBreakableCallArgBinaryOp(op token.Token) bool {
	switch op {
	case token.ADD, token.SUB, token.MUL, token.QUO, token.REM:
		return true

	default:
		return false
	}
}

func formatExprForPackedArg(fset *token.FileSet, expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, expr); err != nil {
		return ""
	}

	return strings.TrimSpace(buf.String())
}

// callExprArgNextState holds state for processing call expression arguments.
type callExprArgNextState struct {
	b                      *strings.Builder
	contIndent             string
	contIndentLen          int
	lineWidth              int
	curLen                 int
	first                  bool
	forcedBreak            bool
	seenMultilineCall      bool
	seenMultilineComposite bool
}

// handleCallExprArgNext processes a call expression argument and returns
// whether it was handled. If handled, it updates the state accordingly.
func (s *callExprArgNextState) handleCallExprArgNext(a string) bool {
	if !llast.IsCallExpr(a) {
		return false
	}

	firstLine := firstLineLen(a)
	isMultiline := strings.Contains(a, "\n")
	fits := false
	fitsFresh := false
	if isMultiline {
		if s.first {
			fits = s.contIndentLen+firstLine <= s.lineWidth
		} else {
			fits = s.curLen+2+firstLine <= s.lineWidth
		}
		fitsFresh = s.contIndentLen+firstLine <= s.lineWidth
	} else if s.first {
		fits = advanceCols(s.contIndentLen, a) <= s.lineWidth
		fitsFresh = fits
	} else {
		fits = advanceCols(s.curLen+2, a) <= s.lineWidth
		fitsFresh = advanceCols(s.contIndentLen, a) <= s.lineWidth
	}
	hasAlways := callHasAlwaysMultilineComposite(a)
	hasNested := llast.HasNestedCall(a)

	// Simple case: fits inline without complications.
	if fits && !hasAlways && !hasNested {
		s.writeArgSimple(a)

		return true
	}

	// If the call fits on its own continuation line, keep it compact even
	// when a slog attribute contains small nested calls such as len(...).
	// Recursing here would turn compact attributes like
	// slog.Int("key", len(xs)) into a multi-line nested call for no
	// column-limit benefit.
	if fitsFresh && !hasAlways && isSlogAttrCall(a) {
		s.writeArgOnFreshLine(a)

		return true
	}

	// Check if we can use generic placement logic.
	if !hasAlways && !hasNested && fitsFresh {

		// Let the caller's generic placement logic handle it.
		return false
	}
	if formatted, ok := formatSelectorCallArgIfOverflowsNext(
		a, s.contIndent, s.lineWidth,
	); ok {

		s.writeNestedCall(formatted)

		return true
	}

	// Need to recursively format the nested call.
	nested := formatCallPackedMultiLineNextInternal(
		[]byte(a), s.contIndent, s.contIndent, true,
		isFmtErrorfCallText(a),
	)
	s.writeNestedCall(nested)

	return true
}

func isFmtErrorfCallText(s string) bool {
	expr, err := parser.ParseExpr(s)
	if err != nil {
		return false
	}

	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil || sel.Sel.Name != "Errorf" {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)

	return ok && ident.Name == "fmt"
}

// writeArgSimple writes an argument that fits inline.
func (s *callExprArgNextState) writeArgSimple(a string) {
	if s.first {
		s.b.WriteString(s.contIndent)
		s.b.WriteString(a)
		s.curLen = s.contIndentLen + firstLineLen(a)
		if strings.Contains(a, "\n") {
			s.curLen = lastLineLen(a)
			s.seenMultilineCall = true
		}
		s.first = false

		return
	}

	if s.forcedBreak {
		s.b.WriteByte(',')
		s.b.WriteByte('\n')
		s.b.WriteString(s.contIndent)
		s.curLen = s.contIndentLen
	} else {
		s.b.WriteString(", ")
	}
	s.b.WriteString(a)
	s.curLen = advanceCols(s.curLen+2, a)
	if strings.Contains(a, "\n") {
		s.curLen = lastLineLen(a)
		s.seenMultilineCall = true
	}
}

func (s *callExprArgNextState) writeArgOnFreshLine(a string) {
	forcedBreak := s.forcedBreak
	s.forcedBreak = true
	s.writeArgSimple(a)
	s.forcedBreak = forcedBreak
}

func isSlogAttrCall(arg string) bool {
	fset := token.NewFileSet()
	expr, err := parser.ParseExprFrom(fset, "", arg, parser.AllErrors)
	if err != nil {
		return false
	}
	call, ok := expr.(*ast.CallExpr)
	if !ok || call == nil {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel == nil {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)

	return ok && ident.Name == "slog"
}

func formatSelectorCallArgIfOverflowsNext(arg, contIndent string,
	lineWidth int) (string, bool) {

	if maxLineLenWithIndentAndComma(arg, contIndent) <= lineWidth {
		return "", false
	}

	fset := token.NewFileSet()
	expr, err := parser.ParseExprFrom(fset, "", arg, parser.AllErrors)
	if err != nil {
		return "", false
	}
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return "", false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil {
		return "", false
	}

	receiver := formatExprForPackedArg(fset, sel.X)
	if receiver == "" || strings.Contains(receiver, "\n") {
		return "", false
	}

	candidate := receiver + ".\n" + contIndent + sel.Sel.Name + "()"
	if maxLineLenWithIndentAndComma(candidate, contIndent) > lineWidth+1 {
		return "", false
	}

	return candidate, true
}

// writeNestedCall writes a recursively formatted nested call.
func (s *callExprArgNextState) writeNestedCall(nested string) {
	if s.first {
		s.b.WriteString(s.contIndent)
		s.b.WriteString(nested)
		s.curLen = s.curLenAfterWrite(nested)
		s.first = false
		if strings.Contains(nested, "\n") {
			s.seenMultilineCall = true
		}

		return
	}

	s.b.WriteByte(',')
	s.b.WriteByte('\n')
	s.b.WriteString(s.contIndent)
	s.b.WriteString(nested)
	s.curLen = s.curLenAfterWrite(nested)
	s.first = false
	if strings.Contains(nested, "\n") {
		s.seenMultilineCall = true
	}
}

// curLenAfterWrite calculates curLen after writing a string.
func (s *callExprArgNextState) curLenAfterWrite(str string) int {
	if strings.Contains(str, "\n") {
		return lastLineLen(str)
	}

	return s.contIndentLen + firstLineLen(str)
}

func expandFuncLitArgBodyNext(arg string, argIndent string) (string, bool) {
	fset := token.NewFileSet()
	expr, err := parser.ParseExprFrom(fset, "", arg, parser.AllErrors)
	if err != nil {
		return "", false
	}
	fn, ok := expr.(*ast.FuncLit)
	if !ok || fn == nil || fn.Body == nil || !fn.Body.Lbrace.IsValid() ||
		!fn.Body.Rbrace.IsValid() {
		return "", false
	}

	lbrace := fset.Position(fn.Body.Lbrace).Offset
	rbrace := fset.Position(fn.Body.Rbrace).Offset
	if lbrace < 0 || rbrace < 0 || lbrace >= rbrace || rbrace >= len(arg) {
		return "", false
	}

	origBody := arg[lbrace : rbrace+1]
	if strings.Contains(origBody, "\n") {
		return "", false
	}
	if len(fn.Body.List) == 0 {
		return "", false
	}

	stmtIndent := argIndent + "\t"
	var out strings.Builder
	out.WriteString("{\n")
	for _, stmt := range fn.Body.List {
		if stmt == nil {
			continue
		}
		var buf bytes.Buffer
		_ = printer.Fprint(&buf, fset, stmt)
		stmtText := strings.TrimSpace(buf.String())
		if stmtText == "" {
			continue
		}
		out.WriteString(stmtIndent)
		out.WriteString(stmtText)
		out.WriteByte('\n')
	}
	out.WriteString(argIndent)
	out.WriteByte('}')

	head := arg[:lbrace]
	tail := arg[rbrace+1:]
	repl := head + out.String() + tail
	if repl == arg {
		return "", false
	}

	return repl, true
}

// callHasAlwaysMultilineComposite reports whether the call expression's
// arguments contain a map/struct composite literal that should be block
// formatted when inside a multiline call.
func callHasAlwaysMultilineComposite(s string) bool {
	expr, err := parser.ParseExpr(s)
	if err == nil {
		call, ok := callExprFromParsedFormatter(expr)
		if !ok {
			return false
		}
		for _, arg := range call.Args {
			if isDirectCompositeLiteralExpr(arg) {
				return true
			}
		}

		return false
	}

	// Fall back to the old syntactic scan only when the expression cannot
	// be parsed. Keep this conservative so function literal bodies do not
	// masquerade as composite literal arguments.
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
		// If this arg is itself a composite literal head (Type{...}) or
		// map literal, detect top-level braces.
		if o, c := findTopLevelBraces(a); o >= 0 && c > o {
			return true
		}
	}

	return false
}

func callExprFromParsedFormatter(expr ast.Expr) (*ast.CallExpr, bool) {
	if expr == nil {
		return nil, false
	}
	if call, ok := expr.(*ast.CallExpr); ok {
		return call, true
	}
	if paren, ok := expr.(*ast.ParenExpr); ok {
		call, ok := paren.X.(*ast.CallExpr)

		return call, ok
	}

	return nil, false
}

func isDirectCompositeLiteralExpr(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.CompositeLit:
		return true

	case *ast.UnaryExpr:
		_, ok := e.X.(*ast.CompositeLit)

		return ok

	case *ast.ParenExpr:
		return isDirectCompositeLiteralExpr(e.X)

	default:
		return false
	}
}

// buildSplitQuoted splits text into quoted segments so they fit within the
// given width starting at startCol for the first segment. Continuation lines
// are indented with contIndent + an extra tab. All but the last segment are
// emitted as "..." + and the last as "...". It preserves spaces when splitting
// at word boundaries when possible.
func buildSplitQuoted(text string, startCol int, contIndent string,
	width int) string {

	return buildSplitQuotedWithOptions(
		text, startCol, contIndent, width, false,
	)
}

// buildSplitQuotedForCallArg is like buildSplitQuoted but applies gofmt's
// context-sensitive spacing around '+' when splitting a string literal that
// appears as a call argument that may have trailing arguments.
func buildSplitQuotedForCallArg(text string, startCol int, contIndent string,
	width int, hasTrailingArgs bool) string {

	return buildSplitQuotedWithOptions(
		text, startCol, contIndent, width, hasTrailingArgs,
	)
}

func buildSplitQuotedWithOptions(text string, startCol int, contIndent string,
	width int, hasTrailingArgs bool) string {

	if !hasUsefulStringSplitBudget(contIndent, width) {
		return quoteGoString(text)
	}

	var out strings.Builder
	rest := text
	curStart := startCol
	// String continuation lines get an extra tab beyond the argument
	// indent.
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

	for rest != "" {
		// If there's not even enough room for a minimal quoted segment
		// (quotes + at least one rune) within the width budget,
		// splitting can't produce a "better" layout. Emit a single
		// quoted literal and stop.
		if width-curStart <= 4 {
			out.WriteString(quoteGoString(rest))
			break
		}
		// If the indentation itself already exceeds the available width
		// budget, splitting can't help (no segment can ever "fit").
		// Emit a single quoted literal and stop to avoid producing
		// degenerate/dangling split output.
		if curStart >= width {
			out.WriteString(quoteGoString(rest))
			break
		}
		// If the whole rest fits as a quoted literal on this line, emit
		// and finish.
		if advanceCols(curStart, quoteGoString(rest)) <= width {
			out.WriteString(quoteGoString(rest))
			break
		}
		// Choose split point at last space that fits with trailing
		// join.
		cut := lastQuotedSpaceBeforeStrictWithJoin(
			curStart, rest, width, hasTrailingArgs,
		)
		if cut <= 0 {
			// Hard cut by visual width capacity for content
			// excluding quotes + " +".
			capCols := width - curStart - 2 - 2 // quotes + " +"
			if capCols <= 0 {
				capCols = 1
			}
			idx := cutIndexForWidthFrom(curStart, rest, capCols)
			if idx <= 0 {
				idx = 1
			}
			// If we end up consuming all remaining content, emit it
			// as a single quoted literal and stop. This matters
			// when curStart > width (deep indentation): we can’t
			// make progress by splitting, and emitting a dangling
			// '+' would produce invalid Go like `"x" +\n,`.
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
		// If the split point consumes all remaining content, emit and
		// stop to avoid emitting a dangling '+'.
		if cut+1 >= len(rest) {
			out.WriteString(quoteGoString(rest))
			break
		}
		writeJoin(seg)
		rest = rest[cut+1:]
		curStart = contStart
	}
	// Defensive cleanup: if we ever end up with a dangling trailing '+',
	// drop it. This can happen when indentation already exceeds the width
	// budget and a split attempt fails to make progress. Leaving a dangling
	// '+' would produce invalid Go when the surrounding call formatter
	// appends a comma/newline.
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
	return formatCallGreedyWithOptions(
		call, wsIndent, baseLen, greedyCallOptions{},
	)
}

type greedyCallOptions struct {
	UseJoinAwareSpaceCut bool
	ReserveClosingParen  bool
	AllowExactFit        bool
	MinTailLen           int

	// PreferBreakBeforeSplit tries to move a string literal to a
	// continuation line (after "(" or after a comma break) if the full
	// literal would fit there, instead of immediately splitting it on the
	// current line.
	PreferBreakBeforeSplit bool

	// AvoidHangingParenForPrintf disables PreferBreakBeforeSplit for
	// printf-style calls (format-string + trailing args). This prevents
	// layouts like:
	//
	// fmt.Errorf( "...: %v", err)
	//
	// where we break right after the signature/paren even though we could
	// split the string and keep the call visually "packed".
	AvoidHangingParenForPrintf bool

	// AvoidTinyFormatVerbTail avoids splitting immediately before a tiny
	// format verb tail like "%v"/"%w", which produces awkward output like:
	// "error: "+\n\t"%v"
	AvoidTinyFormatVerbTail bool

	// ReserveTrailingExprArgs encourages the formatter to keep some number
	// of trailing expression arguments on the same line as the final
	// segment of a format string. This reduces cases where we break `...,
	// maxFee,\n feeRate)` even though splitting the string slightly would
	// allow `..., maxFee, feeRate)` on one continuation line.
	ReserveTrailingExprArgs int

	// PreserveStringConcatExpr keeps explicit string concatenation
	// expressions (e.g. "a"+"b") as expressions rather than flattening them
	// and re-splitting. This helps avoid reflowing user-authored split
	// points into worse ones.
	PreserveStringConcatExpr bool
}

// greedyArgFitParams contains parameters for determining if an argument fits on
// the current line.
type greedyArgFitParams struct {
	curLen   int
	width    int
	numArgs  int
	argIndex int
	opts     greedyCallOptions
}

// exprFitsOnLine returns true if the expression argument fits on the current
// line with the separator, and returns the needed width.
func (p *greedyArgFitParams) exprFitsOnLine(expr string) (fits bool, need int) {
	need = firstLineLen(expr)
	if isTargetedCallStart(expr) {
		need = exprHeadLen(expr)
	}

	reserve := 0
	// Only reserve ')' for single-line expressions.
	if p.opts.ReserveClosingParen && p.argIndex == p.numArgs-1 &&
		!strings.Contains(expr, "\n") {

		reserve = 1
	}

	// Reserve trailing comma if we might break after this arg.
	trailingCommaReserve := 0
	if p.opts.AllowExactFit && p.argIndex < p.numArgs-1 {
		trailingCommaReserve = 1
	}

	total := p.curLen + 2 + need + reserve + trailingCommaReserve
	if p.opts.AllowExactFit {
		fits = total <= p.width
	} else {
		fits = total < p.width
	}

	return fits, need
}

// textFitsOnLine returns true if a text (string) argument's minimal form fits.
func (p *greedyArgFitParams) textFitsOnLine() bool {

	// minimal placeable segment on same line: "X" +
	return p.curLen+2+(2+1+2) <= p.width // ", " + (quotes+char+ +)
}

func formatCallGreedyWithOptions(call []byte, wsIndent string, baseLen int,
	opts greedyCallOptions) string {

	s := string(call)
	open := strings.IndexByte(s, '(')
	if open == -1 || !strings.HasSuffix(s, ")") {
		return s
	}
	head := s[:open]
	argsBody := s[open+1 : len(s)-1]

	// No pre-scan; we will attach leading comments of the next arg (// or
	// /* */) to the previous argument inline when emitting.
	rawArgs := scanner.SplitTopLevelAny(argsBody)
	hasInlineComment := strings.Contains(argsBody, "/*") ||
		strings.Contains(argsBody, "//")
	normArgs := normalizeCallArgs(rawArgs, opts)

	width := columnLimit
	var b strings.Builder
	b.WriteString(head)
	b.WriteByte('(')
	curLen := baseLen + visualLen(head) + 1
	contIndent := wsIndent + "\t"
	prevArgMultiline := false

	if len(normArgs) == 1 && normArgs[0].kind == argText {
		q := quoteGoString(normArgs[0].text)
		candidate := head + "(" + q + ")"
		if advanceCols(baseLen, candidate) <= width {
			return candidate
		}
	}
	if candidate, ok := formatCallSingleLineCandidate(
		head, normArgs, hasInlineComment,
	); ok &&
		advanceCols(baseLen, candidate) <= width {
		return candidate
	}
	writeSplit := func(seg string, hasTrailingArgs bool) {
		q := quoteGoString(seg)
		b.WriteString(q)
		curLen = advanceCols(curLen, q)
		// gofmt normalizes string concatenation spacing based on
		// context:
		// - When string ends with space AND has trailing args: gofmt
		//   removes space before +
		// - When string ends with space AND no trailing args: gofmt
		//   keeps space before +
		// - When string ends with non-space: gofmt keeps space before +
		//   To be idempotent with gofmt, we output what gofmt would
		//   produce.
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
			var prefixes commentPrefixes
			if a.kind == argExpr {
				a.expr, prefixes = extractCommentPrefixes(
					a.expr,
				)
			}
			if prevArgMultiline {
				curLen = emitBreakWithComments(
					&b, prefixes, contIndent,
				)
				justBroke = true
			} else if hasInlineComment {
				// Separator on same line; attach trailing line
				// comment to previous arg, then place any block
				// comment before next arg.
				b.WriteString(", ")
				curLen += 2
				curLen = emitCommentPrefixes(
					&b, prefixes, curLen,
				)
				// Fall through to printing arg on same line.
			} else {
				// Greedy argument packing: keep each argument
				// on the current line if it fits, otherwise
				// break before it.
				fitParams := &greedyArgFitParams{
					curLen:   curLen,
					width:    width,
					numArgs:  len(normArgs),
					argIndex: i,
					opts:     opts,
				}
				var fits bool
				if a.kind == argExpr {
					fits, _ = fitParams.exprFitsOnLine(
						a.expr,
					)
				} else {
					fits = fitParams.textFitsOnLine()
				}
				if fits {
					b.WriteString(", ")
					curLen += 2
					curLen = emitCommentPrefixes(
						&b, prefixes, curLen,
					)
				} else {
					curLen = emitBreakWithComments(
						&b, prefixes, contIndent,
					)
					justBroke = true
				}
			}
		}

		if a.kind == argExpr {
			// Check if we need to break before this expr.
			fitParams := &greedyArgFitParams{
				curLen:   curLen,
				width:    width,
				numArgs:  len(normArgs),
				argIndex: i,
				opts:     opts,
			}
			fits, need := fitParams.exprFitsOnLine(a.expr)
			// Note: fits is for "with separator", but here we're
			// checking without separator (curLen + need > width).
			needsBreak := !justBroke &&
				!isRawStringLiteral(a.expr) &&
				curLen+need > width && !fits
			if needsBreak {
				b.WriteByte('\n')
				b.WriteString(contIndent)
				curLen = visualLen(contIndent)
			}
			curLen = writeExprArg(
				&b, a.expr, wsIndent, curLen, opts,
			)
			prevArgMultiline = strings.Contains(a.expr, "\n")
			continue
		}

		// String arg: split greedily
		if shouldPreserveExplicitStringConcat(a, contIndent, width) {
			b.WriteString(a.raw)
			curLen = advanceCols(curLen, a.raw)
			prevArgMultiline = strings.Contains(a.raw, "\n")

			continue
		}

		rest := a.text
		hasTrailingArgs := i < len(normArgs)-1
		avoidTinyVerbTail := opts.AvoidTinyFormatVerbTail &&
			hasTrailingArgs && a.containsFormatVerb
		minTailLen := opts.MinTailLen
		if !hasTrailingArgs || !a.containsFormatVerb {
			// MinTailLen is primarily a printf-style heuristic to
			// avoid producing awkward tiny tails when there are
			// trailing expression arguments to pack (e.g. starting
			// a segment with "%v" or leaving only a few bytes).
			//
			// For plain strings with no trailing args, allowing a
			// short final word segment (e.g. "cleanly") is
			// typically better than forcing an earlier split point.
			minTailLen = 0
		}
		// If the user explicitly wrote a concatenation expression (and
		// we didn't preserve it as argExpr), avoid producing a tiny
		// trailing verb segment by biasing toward breaking arguments
		// instead of splitting awkwardly.
		for len(rest) > 0 {
			q := quoteGoString(rest)
			// If there are more args after this string, we only
			// need to ensure the trailing comma fits on the current
			// line. The following argument may be placed on a
			// continuation line (comma + newline), so requiring a
			// ", " here is overly strict and can cause unnecessary
			// string splitting (e.g., splitting just to "make room"
			// for the space).
			//
			// We still want to reserve the comma (",") because it
			// must appear at the end of the current line when we
			// break before the next arg.
			commaReserve := 0
			if hasTrailingArgs {
				// At minimum, reserve the comma if we might
				// break before the next arg.
				commaReserve = 1
			} else if opts.ReserveClosingParen {
				// Avoid making the final string literal fit
				// exactly at the boundary and then pushing over
				// by appending ')'.
				commaReserve = 1
			}

			preferredReserve := computePreferredReserve(
				normArgs, i, commaReserve, opts,
				hasTrailingArgs, a.containsFormatVerb,
			)

			contStart := visualLen(contIndent)

			// Try writing the string whole on current line.
			if tryWriteStringWhole(
				&b, q, &curLen, width, preferredReserve,
			) {

				rest = ""
				break
			}
			if tryWriteStringWhole(
				&b, q, &curLen, width, commaReserve,
			) {

				rest = ""
				break
			}

			// Before splitting the string on the current line, try
			// moving the whole literal to a continuation line.
			isPrintfString := hasTrailingArgs &&
				a.containsFormatVerb
			canTryContLine := opts.PreferBreakBeforeSplit &&
				curLen != contStart && hasTrailingArgs &&
				(!opts.AvoidHangingParenForPrintf ||
					!isPrintfString)

			if canTryContLine {
				if tryMoveStringToCont(
					&b, q, contIndent, &curLen, contStart,
					width, preferredReserve,
				) {

					rest = ""
					break
				}
				if tryMoveStringToCont(
					&b, q, contIndent, &curLen, contStart,
					width, commaReserve,
				) {

					rest = ""
					break
				}
			}

			if !hasUsefulStringSplitBudget(contIndent, width) {
				if curLen != contStart {
					b.WriteByte('\n')
					b.WriteString(contIndent)
					curLen = contStart
				}
				b.WriteString(q)
				curLen = advanceCols(curLen, q)
				rest = ""
				break
			}

			// Choose the last ASCII space whose QUOTED prefix fits,
			// taking into account escape expansion inside the
			// literal and gofmt's context-sensitive spacing around
			// '+'.
			cutParams := &spaceCutParams{
				curLen:            curLen,
				rest:              rest,
				width:             width,
				hasTrailingArgs:   hasTrailingArgs,
				minTailLen:        minTailLen,
				avoidTinyVerbTail: avoidTinyVerbTail,
				useJoinAware:      opts.UseJoinAwareSpaceCut,
			}
			cut := cutParams.compute()

			// Capacity for content (excluding quotes and join
			// operator) of this split segment. This is a non-final
			// segment (we are splitting), so we allow exact fill up
			// to the boundary with the trailing join.
			capCols := (width) - curLen - 2 - 2 // quotes + " +"
			if cut <= 0 && capCols <= 0 {
				b.WriteByte('\n')
				b.WriteString(contIndent)
				curLen = visualLen(contIndent)
				capCols = width - curLen - 2 - 2
				if capCols <= 0 {
					capCols = 1
				}
				// Recompute cut now that we're on a
				// continuation line.
				cutParams.curLen = curLen
				cut = cutParams.compute()
			}

			if cut <= 0 {
				q := quoteGoString(rest)
				if curLen != contStart {
					if tryMoveStringToCont(
						&b, q, contIndent, &curLen,
						contStart, width,
						preferredReserve,
					) {

						rest = ""
						break
					}
					if tryMoveStringToCont(
						&b, q, contIndent, &curLen,
						contStart, width, commaReserve,
					) {

						rest = ""
						break
					}
				}
				// No space within capacity. Check if we should
				// wrap to continuation line for the upcoming
				// word.
				if shouldWrapForWord(
					rest, curLen, contIndent, width,
					hasTrailingArgs,
				) {

					b.WriteByte('\n')
					b.WriteString(contIndent)
					curLen = visualLen(contIndent)
					continue
				}
				// Hard cut by visual columns.
				idx := cutIndexForWidthFrom(
					curLen, rest, capCols,
				)
				if idx >= len(rest) {
					// Splitting didn't make progress
					// (typically because indentation
					// already exceeds the width budget).
					// Emit the full literal and stop;
					// emitting a dangling '+' would produce
					// invalid Go when followed by a
					// comma/newline from argument
					// formatting.
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
		prevArgMultiline = false
	}
	b.WriteByte(')')

	return b.String()
}

func formatCallSingleLineCandidate(head string, args []arg,
	hasInlineComment bool) (string, bool) {

	if hasInlineComment {
		return "", false
	}

	parts := make([]string, 0, len(args))
	for _, a := range args {
		switch a.kind {
		case argText:
			parts = append(parts, quoteGoString(a.text))

		case argExpr:
			expr := strings.TrimSpace(a.expr)
			if expr == "" || strings.Contains(expr, "\n") {
				return "", false
			}
			parts = append(parts, expr)

		default:
			return "", false
		}
	}

	return head + "(" + strings.Join(parts, ", ") + ")", true
}

func normalizeCallArgs(rawArgs []string, opts greedyCallOptions) []arg {
	normArgs := make([]arg, 0, len(rawArgs))
	rawCount := len(rawArgs)

	// Only split one "primary" string argument. This avoids extremely poor
	// output for structured logging calls that have many short string key
	// arguments (e.g. "actor_id") that should never be split.
	//
	// This also prevents pathological cases where indentation leaves very
	// little room, and the greedy algorithm would otherwise split even tiny
	// string literals.
	primaryStringIndex := -1
	for i, ra := range rawArgs {
		trimmed := strings.TrimSpace(ra)
		e, err := parser.ParseExpr(trimmed)
		if err != nil {
			continue
		}
		if _, ok := llast.FlattenStringExprAST(e); ok {
			primaryStringIndex = i
			break
		}
	}

	for i, ra := range rawArgs {
		trimmed := strings.TrimSpace(ra)
		normArgs = append(
			normArgs, normalizeCallArg(
				trimmed, i, rawCount, primaryStringIndex, opts,
			),
		)
	}

	return normArgs
}

func normalizeCallArg(trimmed string, argIndex int, rawCount int,
	primaryStringIndex int, opts greedyCallOptions) arg {

	e, err := parser.ParseExpr(trimmed)
	if err != nil {
		return arg{kind: argExpr, expr: trimmed}
	}
	str, ok := llast.FlattenStringExprAST(e)
	if !ok {
		return arg{kind: argExpr, expr: trimmed}
	}

	// For non-primary string args (typically structured logging keys), keep
	// them as expressions so the call formatter can break between arguments
	// instead of splitting inside a short literal. If the user already has
	// an explicit concatenation expression (e.g. "ac"+"tor_id"), collapse
	// it back to a single literal.
	if argIndex != primaryStringIndex {
		if isBasicStringLitExpr(e) {
			return arg{kind: argExpr, expr: trimmed}
		}

		return arg{kind: argExpr, expr: quoteGoString(str)}
	}

	// If this is a concatenation expression used as a format string with
	// trailing arguments, we generally want to flatten it so we can
	// re-split it more intelligently (e.g. to keep `..., a, b)` packed
	// instead of breaking `b` onto its own line).
	//
	// Preserve user-authored split points only when there are no other
	// arguments following this string.
	if opts.PreserveStringConcatExpr &&
		!isBasicStringLitExpr(e) &&
		(rawCount <= 1 || !containsFormatVerb(str)) {
		return arg{kind: argExpr, expr: trimmed}
	}

	return arg{
		kind:                 argText,
		text:                 str,
		raw:                  trimmed,
		containsFormatVerb:   containsFormatVerb(str),
		explicitStringConcat: !isBasicStringLitExpr(e),
	}
}

func shouldPreserveExplicitStringConcat(a arg, contIndent string,
	width int) bool {

	if !a.explicitStringConcat {
		return false
	}
	if isShortFlattenableConcat(a.text) {
		return false
	}
	if strings.Contains(a.text, "\n") {
		return true
	}

	// Deeply indented calls can leave too little room for useful word
	// splits. In that case, keep the author's concatenation boundaries
	// instead of hard-cutting tiny string fragments.
	return !hasUsefulStringSplitBudget(contIndent, width)
}

func isShortFlattenableConcat(text string) bool {
	if strings.Contains(text, "\n") {
		return false
	}

	return visualLen(quoteGoString(text)) <= 48
}

func hasUsefulStringSplitBudget(contIndent string, width int) bool {
	const minUsefulTextCols = 24

	if width < 60 {
		return true
	}

	return width-visualLen(contIndent)-4 >= minUsefulTextCols
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

	containsFormatVerb   bool
	explicitStringConcat bool
}

// commentPrefixes holds extracted comment prefixes from an argument.
type commentPrefixes struct {
	line  string
	block string
}

// extractCommentPrefixes extracts leading line and block comment prefixes from
// an expression argument, returning the cleaned expression and the prefixes.
func extractCommentPrefixes(expr string) (string, commentPrefixes) {
	var prefixes commentPrefixes

	tl := strings.TrimLeftFunc(expr, unicode.IsSpace)
	if strings.HasPrefix(tl, "//") {
		k := 0
		for k < len(tl) && tl[k] != '\n' {
			k++
		}
		prefixes.line = tl[:k]
		expr = strings.TrimLeftFunc(tl[k:], unicode.IsSpace)
		tl = strings.TrimLeftFunc(expr, unicode.IsSpace)
	}
	if strings.HasPrefix(tl, "/*") {
		if end := strings.Index(tl, "*/"); end >= 0 {
			prefixes.block = tl[:end+2]
			expr = strings.TrimLeftFunc(tl[end+2:], unicode.IsSpace)
		}
	}

	return expr, prefixes
}

// emitCommentPrefixes writes comment prefixes to a builder and updates curLen.
func emitCommentPrefixes(b *strings.Builder, prefixes commentPrefixes,
	curLen int) int {

	if prefixes.line != "" {
		b.WriteString(prefixes.line)
		curLen += visualLen(prefixes.line)
	}
	if prefixes.block != "" {
		b.WriteByte(' ')
		b.WriteString(prefixes.block)
		curLen += 1 + visualLen(prefixes.block)
	}

	return curLen
}

// emitBreakWithComments writes a comma, optional line comment, newline, indent,
// and optional block comment. Returns the new curLen.
func emitBreakWithComments(b *strings.Builder, prefixes commentPrefixes,
	contIndent string) int {

	b.WriteByte(',')
	if prefixes.line != "" {
		b.WriteByte(' ')
		b.WriteString(prefixes.line)
	}
	b.WriteByte('\n')
	b.WriteString(contIndent)
	curLen := visualLen(contIndent)
	if prefixes.block != "" {
		b.WriteString(prefixes.block)
		b.WriteByte(' ')
		curLen += visualLen(prefixes.block) + 1
	}

	return curLen
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
	// A minimal heuristic: treat any '%' that is not part of a '%%' escape
	// as a format verb indicator. This is intentionally conservative.
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

func countFormatVerbs(s string) int {
	count := 0
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			continue
		}
		if i+1 < len(s) && s[i+1] == '%' {
			i++
			continue
		}
		count++
	}

	return count
}

func looksLikeTinyFormatVerbTail(s string) bool {
	if s == "" {
		return false
	}
	trimmed := strings.TrimLeft(s, " \t")
	if trimmed == "" || trimmed[0] != '%' {
		return false
	}
	// Treat a tail as "tiny" when it begins with one or more format verb
	// tokens and contains little to no additional context after those
	// tokens.
	//
	// This avoids awkward splits like: "... wraps nicely "+\n\t"%d %d %d"
	//
	// But it allows starting a continuation line with a verb when there is
	// meaningful text after it (e.g. "%v with context").
	rest := trimmed
	consumedAny := false
	for {
		rest = strings.TrimLeft(rest, " \t")
		if rest == "" || rest[0] != '%' {
			break
		}
		// Skip escaped percent ("%%") which is not a verb token.
		if len(rest) >= 2 && rest[1] == '%' {
			break
		}

		// Parse a minimal verb token: "%" + optional "+" + ASCII
		// letter.
		i := 1
		if i < len(rest) && rest[i] == '+' {
			i++
		}
		if i >= len(rest) || !isASCIIAlpha(rest[i]) {
			break
		}
		i++

		consumedAny = true
		rest = rest[i:]

		// Allow trivial closing punctuation immediately following a
		// verb token.
		rest = strings.TrimLeft(rest, " \t")
		rest = strings.TrimLeft(rest, ",.)")

		// If the next non-space token starts another verb, keep
		// consuming. If there's any other content, stop.
		next := strings.TrimLeft(rest, " \t")
		if next == "" {
			break
		}
		if next[0] == '%' {
			continue
		}
		break
	}

	if !consumedAny {
		return false
	}

	rest = strings.TrimSpace(rest)
	rest = strings.Trim(rest, ",.)")

	return rest == ""
}

func isASCIIAlpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func isTinyTail(s string, minTailLen int) bool {
	if minTailLen <= 0 {
		return false
	}
	trimmed := strings.TrimLeft(s, " \t")
	// For printf-style strings, allow short tails that contain multiple
	// format verbs (e.g. "%v: %w") since these are often meaningful enough
	// to stand alone. Still avoid tiny single-verb tails like "%v" or "is
	// %v".
	if countFormatVerbs(trimmed) >= 2 {
		return false
	}

	return len(trimmed) > 0 && len(trimmed) < minTailLen
}

// spaceCutParams encapsulates parameters for computing where to split a string
// at a space boundary.
type spaceCutParams struct {
	curLen            int
	rest              string
	width             int
	hasTrailingArgs   bool
	minTailLen        int
	avoidTinyVerbTail bool
	useJoinAware      bool
}

// compute returns the index of the last space where the quoted prefix fits.
func (p *spaceCutParams) compute() int {
	if p.avoidTinyVerbTail || p.minTailLen > 0 {
		if p.useJoinAware {
			return lastQuotedSpaceBeforeWithJoinAvoidingTails(
				p.curLen, p.rest, p.width, p.hasTrailingArgs,
				p.minTailLen, p.avoidTinyVerbTail,
			)
		}

		return lastQuotedSpaceBeforeAvoidingTails(
			p.curLen, p.rest, p.width, p.minTailLen,
			p.avoidTinyVerbTail,
		)
	}
	if p.useJoinAware {
		return lastQuotedSpaceBeforeWithJoin(
			p.curLen, p.rest, p.width, p.hasTrailingArgs,
		)
	}

	return lastQuotedSpaceBefore(p.curLen, p.rest, p.width)
}

// tryWriteStringWhole attempts to write the quoted string if it fits within
// width with the given reserve. Returns true if written, updating curLen.
func tryWriteStringWhole(b *strings.Builder, q string, curLen *int, width,
	reserve int) bool {

	if advanceCols(*curLen, q)+reserve <= width {
		b.WriteString(q)
		*curLen = advanceCols(*curLen, q)

		return true
	}

	return false
}

// tryMoveStringToCont attempts to move a string to continuation line if it fits
// there. Returns true if moved, updating curLen.
func tryMoveStringToCont(b *strings.Builder, q, contIndent string, curLen *int,
	contStart, width, reserve int) bool {

	if advanceCols(contStart, q)+reserve <= width {
		b.WriteByte('\n')
		b.WriteString(contIndent)
		b.WriteString(q)
		*curLen = advanceCols(contStart, q)

		return true
	}

	return false
}

// writeExprArg writes an expression argument, recursively formatting nested
// calls if needed. Returns the updated curLen.
func writeExprArg(b *strings.Builder, expr, wsIndent string, curLen int,
	opts greedyCallOptions) int {

	if shouldPreserveRawStringConcatExpr(expr, wsIndent+"\t") {
		b.WriteString(expr)

		return advanceCols(curLen, expr)
	}
	if formatted, ok := FormatCompositeLiteralArg(
		expr, wsIndent+"\t",
	); ok && strings.Contains(formatted, "\n") {

		b.WriteString(formatted)

		return lastLineLen(formatted)
	}
	if isTargetedCallStart(expr) {
		formatted := formatCallGreedyWithOptions(
			[]byte(expr), wsIndent, curLen, opts,
		)
		b.WriteString(formatted)

		return lastLineLen(formatted)
	}
	if formatted := formatGreedyBinaryArgIfOverflows(
		expr, wsIndent+"\t", curLen, columnLimit,
	); formatted != expr {

		b.WriteString(formatted)

		return lastLineLen(formatted)
	}
	b.WriteString(expr)

	return advanceCols(curLen, expr)
}

func shouldPreserveRawStringConcatExpr(expr, contIndent string) bool {
	if !strings.Contains(expr, "\n") ||
		!strings.Contains(expr, "+") ||
		!strings.Contains(expr, `"`) {
		return false
	}

	const minUsefulTextCols = 24

	return columnLimit-visualLen(contIndent)-4 < minUsefulTextCols
}

func formatGreedyBinaryArgIfOverflows(expr, contIndent string, curLen,
	width int) string {

	if advanceCols(curLen, expr) <= width {
		return expr
	}

	fset := token.NewFileSet()
	parsed, err := parser.ParseExprFrom(fset, "", expr, parser.AllErrors)
	if err != nil {
		return expr
	}
	bin, ok := parsed.(*ast.BinaryExpr)
	if !ok || !isBreakableCallArgBinaryOp(bin.Op) {
		return expr
	}

	left := formatExprForPackedArg(fset, bin.X)
	right := formatExprForPackedArg(fset, bin.Y)
	if left == "" || right == "" || strings.Contains(left, "\n") ||
		strings.Contains(right, "\n") {
		return expr
	}

	first := left + " " + bin.Op.String()
	candidate := first + "\n" + contIndent + right
	if advanceCols(curLen, first) > width ||
		maxLineLenWithIndentAndComma(right, contIndent) > width {

		candidate = formatBinaryArgLinesNext(fset, bin, contIndent)
		if candidate == "" ||
			advanceCols(curLen, firstLineText(candidate)) > width ||
			maxLineLenWithIndentAndComma(candidate, contIndent) >
				width {
			return expr
		}
	}
	if maxLineLenWithIndentAndComma(candidate, contIndent) >=
		advanceCols(curLen, expr) {
		return expr
	}

	return candidate
}

func firstLineText(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}

	return s
}

// computePreferredReserve calculates how much space to reserve for trailing
// expression arguments when formatting a printf-style string.
func computePreferredReserve(normArgs []arg, argIndex int, commaReserve int,
	opts greedyCallOptions, hasTrailingArgs, containsFormatVerb bool) int {

	if !hasTrailingArgs || opts.ReserveTrailingExprArgs <= 0 ||
		!containsFormatVerb {
		return commaReserve
	}
	reserve := 0
	kept := 0
	for j := argIndex + 1; j < len(normArgs) &&
		kept < opts.ReserveTrailingExprArgs; j++ {

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
	if argIndex+kept == len(normArgs)-1 && kept > 0 {
		reserve++
	}
	if reserve > commaReserve {
		return reserve
	}

	return commaReserve
}

// shouldWrapForWord checks if we're not on a continuation line and the upcoming
// word would fit on a fresh continuation line. Used to decide whether to wrap
// before a hard cut.
func shouldWrapForWord(rest string, curLen int, contIndent string, width int,
	hasTrailingArgs bool) bool {

	base := visualLen(contIndent)
	if curLen == base {

		// Already on continuation line.
		return false
	}
	sp := strings.IndexByte(rest, ' ')
	if sp <= 0 {
		return false
	}
	wordCols := advanceCols(base, rest[:sp]) - base
	nextCap := width - base - 2 - 2 // quotes + " +"
	if hasTrailingArgs {
		nextCap++
	}

	return wordCols <= nextCap
}

func lastQuotedSpaceBeforeAvoidingTails(startCol int, s string, boundary int,
	minTailLen int, avoidTinyVerbTail bool) int {

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
		if strings.TrimSpace(piece) == "" {
			continue
		}
		used := advanceCols(startCol, quoteGoString(piece)) +
			2 // account for " +"
		if used <= boundary {
			last = i
		} else {
			break
		}
	}

	return last
}

func lastQuotedSpaceBeforeWithJoinAvoidingTails(startCol int, s string,
	boundary int, hasTrailingArgs bool, minTailLen int,
	avoidTinyVerbTail bool) int {

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
		if strings.TrimSpace(piece) == "" {
			continue
		}
		joinCols := 2
		if hasTrailingArgs && len(piece) > 0 &&
			piece[len(piece)-1] == ' ' {

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
// the segment. Returns -1 if no such boundary exists. lastQuotedSpaceBefore
// returns the last index of an ASCII space in s such that the quoted prefix up
// to and including that space would fit within the boundary when starting from
// startCol and accounting for " +" at the end of the segment. Returns -1 if no
// such boundary exists.
func lastQuotedSpaceBefore(startCol int, s string, boundary int) int {
	last := -1
	// Scan forward; the quoted width is monotonic with i.
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' {
			continue
		}
		piece := s[:i+1]
		if strings.TrimSpace(piece) == "" {
			continue
		}
		used := advanceCols(startCol, quoteGoString(piece)) +
			2 // account for " +"
		if used <= boundary {
			last = i
		} else {
			break
		}
	}

	return last
}

// lastQuotedSpaceBeforeWithJoin is like lastQuotedSpaceBefore but accounts for
// gofmt's context-sensitive join width: when a segment ends with a space and
// the concatenation is followed by more call arguments, gofmt may elide the
// space before '+' (making the join 1 column instead of 2).
func lastQuotedSpaceBeforeWithJoin(startCol int, s string, boundary int,
	hasTrailingArgs bool) int {

	last := -1
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' {
			continue
		}
		piece := s[:i+1]
		if strings.TrimSpace(piece) == "" {
			continue
		}
		joinCols := 2
		if hasTrailingArgs && len(piece) > 0 &&
			piece[len(piece)-1] == ' ' {

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

func lastQuotedSpaceBeforeStrictWithJoin(startCol int, s string, boundary int,
	hasTrailingArgs bool) int {

	last := -1
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' {
			continue
		}
		piece := s[:i+1]
		if strings.TrimSpace(piece) == "" {
			continue
		}
		joinCols := 2
		if hasTrailingArgs && len(piece) > 0 &&
			piece[len(piece)-1] == ' ' {

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
func FormatCallGreedy(call []byte, wsIndent string, baseLen int, colLimit,
	ts int) string {

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
func FormatCallGreedyNext(call []byte, wsIndent string, baseLen int, colLimit,
	ts int) string {

	formatGlobalsMu.Lock()
	defer formatGlobalsMu.Unlock()

	oldColumnLimit := columnLimit
	oldTabStop := tabStop
	oldTargets := currentTargets

	columnLimit = colLimit
	tabStop = ts
	currentTargets = defaultTargets()

	result := formatCallGreedyWithOptions(
		call, wsIndent, baseLen, greedyCallOptions{
			UseJoinAwareSpaceCut:       true,
			ReserveClosingParen:        true,
			AllowExactFit:              true,
			MinTailLen:                 8,
			PreferBreakBeforeSplit:     false,
			AvoidHangingParenForPrintf: true,
			AvoidTinyFormatVerbTail:    true,
			ReserveTrailingExprArgs:    2,
			PreserveStringConcatExpr:   false,
		},
	)

	columnLimit = oldColumnLimit
	tabStop = oldTabStop
	currentTargets = oldTargets

	return result
}

func formatCallGreedyNextWithMinTailLen(call []byte, wsIndent string,
	baseLen int, colLimit, ts int, minTailLen int) string {

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

	result := formatCallGreedyWithOptions(
		call, wsIndent, baseLen, greedyCallOptions{
			UseJoinAwareSpaceCut:       true,
			ReserveClosingParen:        true,
			AllowExactFit:              true,
			MinTailLen:                 minTailLen,
			PreferBreakBeforeSplit:     false,
			AvoidHangingParenForPrintf: true,
			AvoidTinyFormatVerbTail:    true,
			ReserveTrailingExprArgs:    2,
			PreserveStringConcatExpr:   false,
		},
	)

	columnLimit = oldColumnLimit
	tabStop = oldTabStop
	currentTargets = oldTargets

	return result
}

// FormatCallPackedMultiLine is an exported wrapper around
// formatCallPackedMultiLine for use by the DSL engine. It formats a generic
// function call into a packed multi-line style when the single-line form would
// exceed the column limit.
//
// Parameters:
//   - call: the raw call expression bytes
//   - wsIndent: the whitespace indent string for continuation lines
//   - colLimit: column limit (e.g., 80)
//   - ts: tab stop width (e.g., 8)
//
// Returns the formatted call as a string.
func FormatCallPackedMultiLine(call []byte, wsIndent string, colLimit,
	ts int) string {

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

// FormatCallPackedMultiLineNext is an opt-in wrapper around
// formatCallPackedMultiLineNext for use by the DSL engine and the "next" rule
// profile. It preserves legacy packed multiline output by default
// (FormatCallPackedMultiLine) and only applies next behavior when explicitly
// selected.
func FormatCallPackedMultiLineNext(call []byte, wsIndent string, colLimit,
	ts int) string {

	return FormatCallPackedMultiLineNextWithPrefix(
		call, wsIndent, wsIndent, colLimit, ts,
	)
}

// FormatCallPackedMultiLineNextWithPrefix is the prefix-aware form used by the
// DSL engine when the call expression starts after an assignment or other
// same-line prefix.
func FormatCallPackedMultiLineNextWithPrefix(call []byte, wsIndent,
	fullPrefix string, colLimit, ts int) string {

	formatGlobalsMu.Lock()
	defer formatGlobalsMu.Unlock()

	oldColumnLimit := columnLimit
	oldTabStop := tabStop

	columnLimit = colLimit
	tabStop = ts

	result := formatCallPackedMultiLineNext(
		call, wsIndent, fullPrefix, true,
	)

	columnLimit = oldColumnLimit
	tabStop = oldTabStop

	return result
}
