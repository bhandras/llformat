package formatter

import (
	"bytes"
	formatstd "go/format"
	"strings"

	"github.com/lightninglabs/llformat/scanner"
	"github.com/lightninglabs/llformat/text"
)

// MultiLineConfig holds configuration for the multi-line call formatter.
type MultiLineConfig struct {
	ColumnLimit int
	TabStop     int
	Excludes    []string // Functions to exclude from formatting

	// UseASTSelection switches the legacy multiline call formatter from
	// scan-based call detection to AST-based call selection. The formatting
	// logic remains unchanged; only the "what should we format next?" selector
	// changes. This is intentionally opt-in to preserve golden fixtures.
	UseASTSelection bool

	// SkipGofmt skips the internal gofmt pass, useful when running in a pipeline
	// that will run gofmt at the end.
	SkipGofmt bool

	// ParseSafe enables parse-safe behavior: if the formatter's output does not
	// successfully gofmt, the original input is returned unchanged. This avoids
	// returning syntactically invalid Go when a heuristic rewrite goes wrong.
	ParseSafe bool
}

// MultiLineCallFormatter implements multi-line function call formatting.
type MultiLineCallFormatter struct{ cfg MultiLineConfig }

// DefaultMultilineExcludes returns function name substrings that the legacy
// multiline call formatter always excludes from formatting.
//
// These are calls handled by CompactCallFormatter, and keeping this list in one
// place avoids mismatches between stages (e.g. auto call-arg expression edits).
func DefaultMultilineExcludes() []string {
	return []string{
		"log.Infof", "log.Debugf", "log.Tracef", "log.Errorf", "log.Warnf",
		"fmt.Printf", "fmt.Sprintf", "fmt.Errorf",
	}
}

// NewMultiLineCallFormatter creates a new multi-line call formatter with defaults.
func NewMultiLineCallFormatter(cfg MultiLineConfig) *MultiLineCallFormatter {
	if cfg.ColumnLimit <= 0 {
		cfg.ColumnLimit = 80
	}
	if cfg.TabStop <= 0 {
		cfg.TabStop = 8
	}
	return &MultiLineCallFormatter{cfg: cfg}
}

// FormatFile formats with the formatter's config.
func (f *MultiLineCallFormatter) FormatFile(src []byte) []byte {
	// Apply config to package-level parameters used by helpers.
	columnLimit = f.cfg.ColumnLimit
	tabStop = f.cfg.TabStop
	if f.cfg.UseASTSelection {
		out := f.formatMultiLineCallsInSourceAST(src)
		if f.cfg.ParseSafe {
			if formatted, err := formatstd.Source(out); err == nil {
				return formatted
			}
			return src
		}
		return out
	}

	out := f.formatMultiLineCallsInSourceScan(src)
	if f.cfg.ParseSafe {
		if formatted, err := formatstd.Source(out); err == nil {
			return formatted
		}
		return src
	}
	return out
}

// formatMultiLineCallsInSourceScan scans source and reformats function calls
// that exceed column limit.
// It works iteratively - expanding one call at a time until no more long lines remain.
func (f *MultiLineCallFormatter) formatMultiLineCallsInSourceScan(src []byte) []byte {
	result := src
	maxIterations := 20

	for iter := 0; iter < maxIterations; iter++ {
		modified, changed := f.formatOneCallInSourceScan(result)
		if !changed {
			break
		}
		result = modified
	}

	if f.cfg.SkipGofmt {
		return result
	}
	if formatted, err := formatstd.Source(result); err == nil {
		return formatted
	}
	return result
}

// formatOneCallInSourceScan finds and expands one function call that exceeds the column limit.
// Returns the modified source and whether a change was made.
func (f *MultiLineCallFormatter) formatOneCallInSourceScan(src []byte) ([]byte, bool) {
	var out bytes.Buffer
	i := 0
	changed := false

	for i < len(src) {
		// Skip strings and comments
		if scanner.IsStringStart(src, i) {
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

		// Look for function calls (only if we haven't already made a change)
		if !changed {
			if callInfo := f.findFunctionCallAt(src, i); callInfo != nil {
				// Check if this function should be excluded
				if f.shouldExclude(callInfo.funcName) {
					out.Write(src[callInfo.start:callInfo.end])
					i = callInfo.end
					continue
				}

				// Check if call needs wrapping
				lineStart := text.LastLineStart(src, callInfo.start)
				indentBytes := src[lineStart:callInfo.start]
				currentLineLen := visualLen(string(indentBytes)) + callInfo.singleLineLen

				if currentLineLen > f.cfg.ColumnLimit {
					// Format as multi-line
					formatted := f.formatAsMultiLine(src[callInfo.start:callInfo.end], string(text.LeadingWhitespace(src, lineStart)))
					out.WriteString(formatted)
					i = callInfo.end
					changed = true
					// Continue writing the rest of the source unchanged
					out.Write(src[i:])
					return out.Bytes(), true
				} else {
					// Keep as single line
					out.Write(src[callInfo.start:callInfo.end])
				}
				i = callInfo.end
				continue
			}
		}

		out.WriteByte(src[i])
		i++
	}

	return out.Bytes(), changed
}

// callInfo contains information about a function call.
type callInfo struct {
	start         int
	end           int
	funcName      string
	singleLineLen int
}

// findFunctionCallAt attempts to find a function call starting at position i.
func (f *MultiLineCallFormatter) findFunctionCallAt(src []byte, i int) *callInfo {
	// Look for identifier patterns that could be function calls
	if i >= len(src) || !text.IsIdentifierStart(src[i]) {
		return nil
	}

	// Find end of potential function name (including selectors like pkg.Func)
	j := i
	for j < len(src) && (text.IsIdentifierChar(src[j]) || src[j] == '.') {
		j++
	}

	// Reject incomplete selector chains like `pkg.` or `x.`. This prevents
	// mis-detecting type assertions (`x.(T)`) as calls on `x.`.
	if j > i && src[j-1] == '.' {
		return nil
	}

	// Skip whitespace
	for j < len(src) && (src[j] == ' ' || src[j] == '\t') {
		j++
	}

	// Must be followed by opening parenthesis
	if j >= len(src) || src[j] != '(' {
		return nil
	}

	// Check if this looks like a function definition rather than a call
	if f.isFunctionDefinition(src, i) {
		return nil
	}

	// Find matching closing parenthesis
	endIdx := scanner.ScanBalancedParen(src, j)
	if endIdx <= j {
		return nil
	}

	funcName := string(src[i:j])
	funcName = strings.TrimSpace(funcName)

	// Calculate single-line length
	singleLine := string(src[i : endIdx+1])
	singleLineLen := visualLen(singleLine)

	return &callInfo{
		start:         i,
		end:           endIdx + 1,
		funcName:      funcName,
		singleLineLen: singleLineLen,
	}
}

// isFunctionDefinition checks if this looks like a function definition rather than a call.
func (f *MultiLineCallFormatter) isFunctionDefinition(src []byte, i int) bool {
	// Look backwards to see if there's a "func" keyword
	j := i - 1
	// Skip whitespace backwards
	for j >= 0 && (src[j] == ' ' || src[j] == '\t') {
		j--
	}

	// Check if we have "func" before this identifier
	if j >= 3 && string(src[j-3:j+1]) == "func" {
		return true
	}

	// Also check if this is a method definition by looking for ) before func
	k := j
	for k >= 0 && src[k] != '\n' {
		if k >= 3 && string(src[k-3:k+1]) == "func" {
			return true
		}
		k--
	}

	// Check if we're inside an interface block (interface method declaration)
	if f.isInsideInterface(src, i) {
		return true
	}

	return false
}

// isInsideInterface checks if position i is inside an interface block.
func (f *MultiLineCallFormatter) isInsideInterface(src []byte, i int) bool {
	// Walk backwards to find if we're inside an interface { } block
	// Track brace depth
	braceDepth := 0

	for j := i - 1; j >= 0; j-- {
		c := src[j]

		// Skip if inside string - walk backwards past strings
		// This is approximate but should work for most cases
		if c == '}' {
			braceDepth++
		} else if c == '{' {
			if braceDepth > 0 {
				braceDepth--
			} else {
				// Found an unmatched opening brace - check if it's an interface
				// Look for "interface" keyword before this brace
				k := j - 1
				for k >= 0 && (src[k] == ' ' || src[k] == '\t' || src[k] == '\n') {
					k--
				}
				if k >= 8 && string(src[k-8:k+1]) == "interface" {
					return true
				}
				// Also check for "interface" with identifier after (named interface)
				// Pattern: "type Name interface {"
				// Keep looking backward for interface keyword on same declaration
				for k >= 0 && src[k] != '\n' {
					if k >= 8 && string(src[k-8:k+1]) == "interface" {
						return true
					}
					k--
				}
				return false
			}
		}
	}
	return false
}

// shouldExclude checks if a function name should be excluded from formatting.
func (f *MultiLineCallFormatter) shouldExclude(funcName string) bool {
	// Always exclude functions handled by CompactCallFormatter
	for _, exclude := range DefaultMultilineExcludes() {
		if strings.Contains(funcName, exclude) {
			return true
		}
	}

	// Check user-provided excludes
	for _, exclude := range f.cfg.Excludes {
		if strings.Contains(funcName, exclude) {
			return true
		}
	}
	return false
}

// formatAsMultiLine formats a function call in multi-line style.
func (f *MultiLineCallFormatter) formatAsMultiLine(callBytes []byte, wsIndent string) string {
	s := string(callBytes)
	open := strings.IndexByte(s, '(')
	if open == -1 || !strings.HasSuffix(s, ")") {
		return s
	}

	head := s[:open]
	argsBody := s[open+1 : len(s)-1]

	// Parse arguments
	args := scanner.SplitTopLevel(argsBody)
	if len(args) == 0 {
		return s // No args, keep as-is
	}

	var b strings.Builder
	b.WriteString(head)
	b.WriteString("(\n")

	// Indentation for arguments
	argIndent := wsIndent + "\t"

	// Format each argument on its own line with continuation
	for i, arg := range args {
		trimmedArg := strings.TrimSpace(arg)
		if trimmedArg == "" {
			continue
		}

		b.WriteString(argIndent)

		// Handle multi-line arguments by re-indenting them
		lines := strings.Split(trimmedArg, "\n")
		for j, line := range lines {
			if j > 0 {
				b.WriteString("\n")
				b.WriteString(argIndent)
			}
			b.WriteString(strings.TrimSpace(line))
		}

		// Add comma after each argument except potentially the last
		b.WriteString(",")

		// Add newline
		if i < len(args)-1 {
			b.WriteString("\n")
		}
	}

	// Close with proper indentation
	b.WriteString("\n")
	b.WriteString(wsIndent)
	b.WriteString(")")

	return b.String()
}

// FormatOneMultiLineCallInSource applies exactly one legacy multiline call
// rewrite pass to src, matching MultiLineCallFormatter's internal scanner.
//
// This is exported so the DSL stage can delegate to the legacy scan-based
// implementation without creating an import cycle.
func FormatOneMultiLineCallInSource(src []byte, colLimit, tabStop int, excludes []string) ([]byte, bool) {
	cfg := MultiLineConfig{
		ColumnLimit: colLimit,
		TabStop:     tabStop,
		Excludes:    excludes,
		SkipGofmt:   true,
	}

	// Keep behavior consistent with MultiLineCallFormatter.FormatFile, which
	// sets these package-level values used by width helpers.
	columnLimit = cfg.ColumnLimit
	tabStop = cfg.TabStop

	f := NewMultiLineCallFormatter(cfg)
	return f.formatOneCallInSourceScan(src)
}

// FormatCallOnePerLineMultiLine formats a call using the legacy MultiLineCallFormatter
// style (one argument per line).
//
// This is used by DSL call rules to delegate formatting to the established
// string-scanner behavior to preserve output parity.
func FormatCallOnePerLineMultiLine(call []byte, wsIndent string, colLimit, tabStop int) string {
	// colLimit/tabStop are accepted for parity with other injected formatter
	// functions; the legacy one-per-line formatter is not width-adaptive.
	_ = colLimit
	_ = tabStop

	s := string(call)
	open := strings.IndexByte(s, '(')
	if open == -1 || !strings.HasSuffix(s, ")") {
		return s
	}

	head := s[:open]
	argsBody := s[open+1 : len(s)-1]

	args := scanner.SplitTopLevel(argsBody)
	if len(args) == 0 {
		return s
	}

	var b strings.Builder
	b.WriteString(head)
	b.WriteString("(\n")

	argIndent := wsIndent + "\t"
	for i, arg := range args {
		trimmedArg := strings.TrimSpace(arg)
		if trimmedArg == "" {
			continue
		}

		b.WriteString(argIndent)
		lines := strings.Split(trimmedArg, "\n")
		for j, line := range lines {
			if j > 0 {
				b.WriteString("\n")
				b.WriteString(argIndent)
			}
			b.WriteString(strings.TrimSpace(line))
		}
		b.WriteString(",")
		if i < len(args)-1 {
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(wsIndent)
	b.WriteString(")")
	return b.String()
}
