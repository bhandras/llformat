package formatter

import (
	"bytes"
	"go/token"
	"strings"

	"github.com/lightninglabs/llformat/scanner"
	"github.com/lightninglabs/llformat/width"
)

// FuncSigConfig holds configuration for the function signature formatter.
type FuncSigConfig struct {
	ColumnLimit int
	TabStop     int
	// CanonicalMultilineSigLists forces a gofmt-like formatting style for
	// multiline parameter and result lists. In particular, if a
	// parenthesized return list doesn't fit on one line, we prefer: f(...)
	// ( a, b, ) rather than partially breaking inside the list, e.g.:
	// f(...) (a, b)
	//
	// This is enabled for the "next" profile, but kept off for legacy
	// behavior to avoid changing golden fixtures.
	CanonicalMultilineSigLists bool

	// ReserveTrailingComma reserves space for a trailing comma on a line
	// when we might need to break before the next element. This helps avoid
	// placing a comma exactly on the column boundary (or overflowing by one
	// column) due to late comma insertion.
	ReserveTrailingComma bool

	// PreferInlineSmallReturnList prefers keeping small parenthesized
	// return lists (e.g. `([]T, error)`) on one line by breaking parameters
	// earlier.
	PreferInlineSmallReturnList bool

	// BreakLongFuncTypeParams enables breaking of function-typed parameters
	// when their inner parameter list exceeds the column limit (even when
	// there is no nested struct type).
	//
	// This is intentionally opt-in to preserve legacy fixtures; it is used
	// by the "next" profile to improve readability of long callback
	// signatures.
	BreakLongFuncTypeParams bool

	// FormatInlineStructParams forces signature reflow when parameters
	// include inline struct types with semicolons (which gofmt will expand
	// into multiline blocks). This is intentionally opt-in because it can
	// be a readability-only change even when no single line exceeds the
	// column limit.
	FormatInlineStructParams bool
}

// FuncSigFormatter formats long function signatures by breaking them across
// lines.
type FuncSigFormatter struct {
	cfg FuncSigConfig
}

// NewFuncSigFormatter creates a new function signature formatter with defaults.
func NewFuncSigFormatter(cfg FuncSigConfig) *FuncSigFormatter {
	if cfg.ColumnLimit <= 0 {
		cfg.ColumnLimit = 80
	}
	if cfg.TabStop <= 0 {
		cfg.TabStop = 8
	}
	// Canonical multiline signature lists are a "next-profile" opt-in for
	// this formatter. Enable the related heuristics by default when callers
	// request canonical formatting.
	if cfg.CanonicalMultilineSigLists {
		cfg.ReserveTrailingComma = true
		cfg.PreferInlineSmallReturnList = true
	}

	return &FuncSigFormatter{cfg: cfg}
}

// FormatFile formats long function signatures in the source file.
func (f *FuncSigFormatter) FormatFile(src []byte) []byte {
	var out bytes.Buffer
	lines := bytes.Split(src, []byte("\n"))

	// Track if we're inside an interface block
	inInterface := false
	braceDepth := 0

	i := 0
	for i < len(lines) {
		line := string(lines[i])
		trimmed := strings.TrimLeft(line, " \t")

		// Track interface blocks
		if strings.Contains(trimmed, "interface") &&
			strings.Contains(trimmed, "{") {

			inInterface = true
			braceDepth = 1
		} else if inInterface {
			// Count braces to track when we exit the interface
			for _, c := range trimmed {
				if c == '{' {
					braceDepth++
				} else if c == '}' {
					braceDepth--
					if braceDepth == 0 {
						inInterface = false
					}
				}
			}
		}

		// Check if this looks like a function signature
		if f.isFuncSignature(trimmed) {
			// Get the indent
			indent := line[:len(line)-len(trimmed)]

			// Check if line exceeds column limit
			visualLen := width.VisualLenWithTab(line, f.cfg.TabStop)
			if visualLen > f.cfg.ColumnLimit {
				// Format the signature
				formatted, linesConsumed := f.formatSignature(
					lines, i, indent,
				)
				out.WriteString(formatted)
				i += linesConsumed
				continue
			}
		} else if inInterface && f.isInterfaceMethod(trimmed) {
			// Get the indent
			indent := line[:len(line)-len(trimmed)]

			// Check if line exceeds column limit
			visualLen := width.VisualLenWithTab(line, f.cfg.TabStop)
			if visualLen > f.cfg.ColumnLimit {
				// Format the interface method
				formatted, linesConsumed := f.formatInterfaceMethod(
					lines, i, indent,
				)
				out.WriteString(formatted)
				i += linesConsumed
				continue
			}
		}

		out.WriteString(line)
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
		i++
	}

	return out.Bytes()
}

// FormatFuncSigsInSource applies the legacy function signature formatter to src
// and reports whether it changed anything.
//
// This is exported so DSL stages can delegate to the legacy implementation
// without creating an import cycle.
func FormatFuncSigsInSource(src []byte, colLimit, tabStop int) ([]byte, bool) {
	f := NewFuncSigFormatter(
		FuncSigConfig{
			ColumnLimit: colLimit,
			TabStop:     tabStop,
		},
	)
	out := f.FormatFile(src)
	if len(out) == len(src) {
		same := true
		for i := range out {
			if out[i] != src[i] {
				same = false
				break
			}
		}
		if same {
			return nil, false
		}
	}

	return out, true
}

// FormatFuncSignatureLegacy formats a function declaration signature using the
// legacy FuncSigFormatter implementation. This is used by native DSL signature
// rules to preserve parity while applying edits at AST-selected spans.
func FormatFuncSignatureLegacy(signature, indent string, colLimit,
	tabStop int) (string, bool) {

	f := NewFuncSigFormatter(
		FuncSigConfig{
			ColumnLimit: colLimit,
			TabStop:     tabStop,
		},
	)
	formatted := f.breakSignature(signature, indent)
	// needsBlank in the DSL action is used only to decide whether to insert
	// an extra blank line after the opening brace. Only treat "top-level"
	// signature breaks as requiring an extra blank line; embedded multiline
	// types (struct blocks, etc) should not force additional spacing.
	needsBlank := hasNewlineOutsideBraces(formatted)
	// Legacy: when multiline content comes only from nested multiline
	// function types (e.g. `handler func(\n...`), inserting an extra blank
	// line in the body is usually too much vertical whitespace.
	if strings.Contains(formatted, "func(\n") {
		needsBlank = false
	}

	return formatted, needsBlank
}

// FormatFuncSignatureNext formats a function declaration signature for the
// "next" profile. Unlike the legacy behavior, it will reflow some
// already-multiline signatures into a single line when they fit within the
// column limit.
//
// Example:
//
// func f() ( *T, error) {
//
// becomes:
//
// func f() (*T, error) {
func FormatFuncSignatureNext(signature, indent string, colLimit,
	tabStop int) (string, bool) {

	f := NewFuncSigFormatter(FuncSigConfig{
		ColumnLimit: colLimit,
		TabStop:     tabStop,
		// "next" uses a greedy/packed style for multiline param/result
		// lists. Avoid gofmt-like canonicalization to match the "next"
		// golden spec.
		CanonicalMultilineSigLists: false,
		ReserveTrailingComma:       true,
		// Prefer keeping very small return lists inline by breaking
		// parameters earlier (when feasible).
		PreferInlineSmallReturnList: true,
		BreakLongFuncTypeParams:     true,
		FormatInlineStructParams:    true,
	})

	// When the signature already contains newlines, prefer to collapse it
	// if it fits. This is safe for the "next" profile and handles common
	// patterns where return lists were manually split but are short.
	//
	// However, do not collapse signatures that contain struct blocks:
	// collapsing those tends to erase readability-driven formatting (and
	// gofmt will expand struct types again anyway).
	if strings.Contains(signature, "\n") {
		if !strings.Contains(signature, "struct") &&
			!hasInlineStructWithSemicolons(signature) {

			// Preserve any leading whitespace in the input
			// signature. This is important for function literals
			// where we sometimes inject a synthetic first-line
			// prefix width using spaces so that collapsing
			// decisions take the surrounding prefix into account.
			// `collapseSignatureWhitespace` deliberately drops
			// leading whitespace, so we add it back here.
			//
			// This also prevents non-idempotent behavior where:
			// - a multiline func-literal signature collapses
			//   (because it "fits")
			// - then immediately re-expands on the next iteration
			//   due to the real prefix before `func` pushing it
			//   over the column limit which otherwise triggers DSL
			//   cycle detection and can prevent later signatures in
			//   the file from being formatted at all.
			leading := leadingWhitespace(signature)

			collapsed := collapseSignatureWhitespace(signature)
			collapsed = tightenSignatureParens(collapsed)
			if leading != "" &&
				strings.HasPrefix(
					strings.TrimLeft(signature, " 	"),
					"func",
				) {

				collapsed = leading + collapsed
			}

			if width.VisualLenWithTab(indent+collapsed, tabStop) <= colLimit {
				return indent + collapsed, false
			}
			// Even if it doesn't fit, collapsing whitespace makes
			// subsequent breaking decisions more consistent.
			signature = collapsed
		}
	}

	// If this isn't a function decl signature, do not try to "tighten"
	// around the leading indent: callers for interface methods pass
	// `indent` and already include the right whitespace in `signature`.
	formatted := f.breakSignature(signature, indent)
	formatted = collapseMultilineParenReturnListIfFits(
		formatted, colLimit, tabStop,
	)
	needsBlank := hasNewlineOutsideBraces(formatted)

	return formatted, needsBlank
}

func collapseMultilineParenReturnListIfFits(signature string, colLimit,
	tabStop int) string {

	// If a parenthesized return list was broken across multiple lines, try
	// to collapse it back onto the `) (` line when it fits.
	//
	// Example target: func(x T, y U) (Out, error) { => func(x T, y U) (Out,
	// error) {
	lines := strings.Split(signature, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimRight(line, " \t")
		openIdx := strings.LastIndex(trimmed, ") (")
		if openIdx < 0 {
			continue
		}
		// Require the `) (` to be the end of the non-whitespace prefix
		// of this line. We allow return content after it (e.g. `)
		// (Out,`), but we must not be in the middle of something else.
		prefix := trimmed[:openIdx+len(") (")]
		afterOpen := strings.TrimSpace(trimmed[openIdx+len(") ("):])

		var retParts []string
		if afterOpen != "" {
			retParts = append(retParts, afterOpen)
		}

		closeLine := -1
		closeIdx := -1
		tail := ""

		// Consume subsequent lines until we find the close paren.
		for j := i + 1; j < len(lines); j++ {
			part := strings.TrimLeft(lines[j], " \t")
			if k := strings.IndexByte(part, ')'); k >= 0 {
				closeLine = j
				closeIdx = k
				front := strings.TrimSpace(part[:k])
				if front != "" {
					retParts = append(retParts, front)
				}
				tail = part[k:] // includes ')', plus anything after it (` {`, etc)
				break
			}
			if part != "" {
				retParts = append(
					retParts, strings.TrimSpace(part),
				)
			}
		}
		if closeLine == -1 || closeIdx == -1 {
			continue
		}

		ret := strings.Join(retParts, " ")
		ret = strings.TrimSpace(ret)
		if ret == "" {
			continue
		}
		// Only collapse simple return lists; avoid nested types that
		// require a real parser (function types, composite literal
		// types, etc).
		if strings.ContainsAny(ret, "{}") ||
			strings.Contains(ret, "func(") {

			continue
		}
		// Normalize comma spacing.
		ret = strings.Join(strings.Fields(ret), " ")
		ret = strings.ReplaceAll(ret, " ,", ",")
		ret = strings.ReplaceAll(ret, ",", ", ")
		ret = strings.Join(strings.Fields(ret), " ")

		candidate := prefix + ret + tail
		if width.VisualLenWithTab(candidate, tabStop) > colLimit {
			continue
		}

		// Replace lines[i..closeLine] with a single collapsed line.
		newLines := append([]string{}, lines[:i]...)
		newLines = append(newLines, candidate)
		newLines = append(newLines, lines[closeLine+1:]...)

		return strings.Join(newLines, "\n")
	}

	return signature
}

func isFuncLitSignature(signature string) bool {
	trimmed := strings.TrimSpace(signature)
	// Signatures passed to the formatter may include non-whitespace
	// prefixes before the `func` keyword (e.g. `Field: func(...) {` or `x
	// := func(...) {`) when we want to model the first-line width
	// constraints. Locate the `func` keyword and classify based on the
	// substring starting there.
	if !strings.HasPrefix(trimmed, "func") {
		funcIdx := -1
		for i := 0; i+4 <= len(trimmed); i++ {
			if trimmed[i] != 'f' || trimmed[i:i+4] != "func" {
				continue
			}
			// Word boundary: avoid matching `somefunc` or `funcX`.
			if i > 0 && isIdentChar(trimmed[i-1]) {
				continue
			}
			if i+4 < len(trimmed) && isIdentChar(trimmed[i+4]) {
				continue
			}
			funcIdx = i
			break
		}
		if funcIdx >= 0 {
			trimmed = strings.TrimSpace(trimmed[funcIdx:])
		}
	}

	if strings.HasPrefix(trimmed, "func(") {
		return true
	}
	if !strings.HasPrefix(trimmed, "func (") {
		return false
	}

	// `func (` can either be a function literal or a method receiver.
	// Distinguish by checking whether the first paren group is followed by
	// `ident(`.
	open := strings.IndexByte(trimmed, '(')
	if open < 0 {
		return false
	}
	f := NewFuncSigFormatter(FuncSigConfig{ColumnLimit: 80, TabStop: 8})
	close := f.findMatchingParen(trimmed, open)
	if close < 0 || close+1 >= len(trimmed) {
		return true
	}

	rest := strings.TrimLeft(trimmed[close+1:], " \t")
	if rest == "" {
		return true
	}
	// Read an identifier.
	i := 0
	if !isIdentStart(rest[0]) {
		return true
	}
	i++
	for i < len(rest) && isIdentChar(rest[i]) {
		i++
	}
	rest = rest[i:]
	rest = strings.TrimLeft(rest, " \t")

	return !(len(rest) > 0 && rest[0] == '(')
}

// hasNewlineOutsideBraces reports whether s contains a newline at brace nesting
// depth 0. This is used to decide whether to insert an extra blank line after a
// signature's opening brace: only line breaks in the outer signature lists
// should trigger that spacing, not multiline embedded types (e.g. `struct { ...
// }`).
func hasNewlineOutsideBraces(s string) bool {
	braceDepth := 0
	inString := byte(0)
	escaped := false

	for i := 0; i < len(s); i++ {
		c := s[i]

		if inString != 0 {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' && inString == '"' {
				escaped = true
				continue
			}
			if c == inString {
				inString = 0
			}
			continue
		}

		switch c {
		case '"', '`', '\'':
			inString = c

		case '{':
			braceDepth++

		case '}':
			if braceDepth > 0 {
				braceDepth--
			}

		case '\n':
			if braceDepth == 0 {
				return true
			}
		}
	}

	return false
}

func collapseSignatureWhitespace(sig string) string {
	var b strings.Builder
	b.Grow(len(sig))

	inString := byte(0)
	escaped := false
	pendingSpace := false
	hasWritten := false
	lastWritten := byte(0)

	for i := 0; i < len(sig); i++ {
		c := sig[i]

		if inString != 0 {
			b.WriteByte(c)
			hasWritten = true
			lastWritten = c
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' && inString == '"' {
				escaped = true
				continue
			}
			if c == inString {
				inString = 0
			}
			continue
		}

		if c == '"' || c == '`' || c == '\'' {
			if pendingSpace && hasWritten && lastWritten != ' ' {
				b.WriteByte(' ')
				lastWritten = ' '
			}
			pendingSpace = false

			inString = c
			b.WriteByte(c)
			hasWritten = true
			lastWritten = c
			continue
		}

		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			pendingSpace = true
			continue
		}

		if pendingSpace && hasWritten && lastWritten != ' ' {
			b.WriteByte(' ')
			lastWritten = ' '
		}
		pendingSpace = false

		b.WriteByte(c)
		hasWritten = true
		lastWritten = c
	}

	return b.String()
}

func tightenSignatureParens(sig string) string {
	// Remove spaces immediately after '(' and immediately before ')'. This
	// is a simple whitespace tightening to get from: "() ( *T, error) {"
	// to: "() (*T, error) {"
	//
	// Avoid touching content inside string literals.
	out := make([]byte, 0, len(sig))

	inString := byte(0)
	escaped := false

	for i := 0; i < len(sig); i++ {
		c := sig[i]

		if inString != 0 {
			out = append(out, c)
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' && inString == '"' {
				escaped = true
				continue
			}
			if c == inString {
				inString = 0
			}
			continue
		}

		if c == '"' || c == '`' || c == '\'' {
			inString = c
			out = append(out, c)
			continue
		}

		if c == '(' {
			out = append(out, c)
			// Skip spaces after '('
			for i+1 < len(sig) && sig[i+1] == ' ' {
				i++
			}
			continue
		}

		if c == ')' {
			// Drop trailing space before ')'
			if len(out) > 0 && out[len(out)-1] == ' ' {
				out = out[:len(out)-1]
			}
			out = append(out, c)
			continue
		}

		out = append(out, c)
	}

	return string(out)
}

// FormatInterfaceMethodLegacy formats an interface method signature using the
// legacy FuncSigFormatter implementation.
func FormatInterfaceMethodLegacy(method, indent string, colLimit,
	tabStop int) string {

	f := NewFuncSigFormatter(
		FuncSigConfig{
			ColumnLimit: colLimit,
			TabStop:     tabStop,
		},
	)

	return f.breakInterfaceMethod(method, indent)
}

// isFuncSignature checks if a line starts a function signature.
func (f *FuncSigFormatter) isFuncSignature(trimmed string) bool {
	// Match: func name(, func (receiver) name(, or method in interface
	if strings.HasPrefix(trimmed, "func ") {
		return true
	}

	return false
}

// isInterfaceMethod checks if a line looks like an interface method
// declaration. Interface methods are: identifier(params) [returns] - no func
// keyword, no brace.
func (f *FuncSigFormatter) isInterfaceMethod(trimmed string) bool {
	// Must start with an identifier
	if len(trimmed) == 0 || !isIdentStart(trimmed[0]) {
		return false
	}

	// Find the opening paren
	parenIdx := strings.Index(trimmed, "(")
	if parenIdx == -1 {
		return false
	}

	// Everything before paren must be identifier
	methodName := trimmed[:parenIdx]
	for _, c := range methodName {
		if !isIdentChar(byte(c)) {
			return false
		}
	}

	// Should not end with { (that would be a function definition)
	if strings.HasSuffix(strings.TrimSpace(trimmed), "{") {
		return false
	}

	return true
}

// formatSignature formats a long function signature. Returns the formatted
// string and number of source lines consumed.
func (f *FuncSigFormatter) formatSignature(lines [][]byte, startIdx int,
	indent string) (string, int) {

	// First, collect the complete signature (might span multiple lines
	// already)
	var sigBuilder strings.Builder
	linesConsumed := 0
	braceFound := false
	inlineBodyFound := false
	inlineBody := ""
	inlineAfterClose := ""
	parenDepth := 0
	inString := false
	escaped := false

	for idx := startIdx; idx < len(lines); idx++ {
		line := string(lines[idx])
		linesConsumed++

		for i := 0; i < len(line); i++ {
			c := line[i]

			if escaped {
				escaped = false
				continue
			}
			if c == '\\' && inString {
				escaped = true
				continue
			}
			if c == '"' || c == '`' || c == '\'' {
				inString = !inString
				continue
			}
			if inString {
				continue
			}

			if c == '(' {
				parenDepth++
			} else if c == ')' {
				parenDepth--
			} else if c == '{' && parenDepth == 0 {
				braceFound = true
				// Include everything up to and including the
				// brace.
				//
				// Note: if the function body is also on this
				// same line (e.g. `func f(...) { return 0 }`)
				// we must not drop it. We'll detect and
				// preserve the inline body below, after
				// formatting the signature.
				sigBuilder.WriteString(
					strings.TrimRight(
						line[:i+1], " 	",
					),
				)

				// Preserve inline bodies like `{ return 0 }` so
				// that signature formatting does not delete
				// code.
				//
				// This uses a balanced-brace scan to correctly
				// handle nested braces in composite literals
				// (e.g. `return T{}`).
				closeIdx := scanner.ScanBalanced(
					[]byte(line), i, '{', '}',
				)
				if closeIdx != -1 && closeIdx > i {
					inlineBodyFound = true
					inlineBody = line[i+1 : closeIdx]
					inlineAfterClose = line[closeIdx+1:]
				}
				break
			}
		}

		if braceFound {
			break
		}

		// Add this line to signature (normalize whitespace)
		if sigBuilder.Len() > 0 {
			sigBuilder.WriteByte(' ')
		}
		sigBuilder.WriteString(strings.TrimSpace(line))
	}

	sig := sigBuilder.String()
	if !braceFound {
		// Not a complete signature, return as-is
		var result strings.Builder
		for j := 0; j < linesConsumed; j++ {
			result.Write(lines[startIdx+j])
			if startIdx+j < len(lines)-1 {
				result.WriteByte('\n')
			}
		}

		return result.String(), linesConsumed
	}

	// Now format the signature
	formatted := f.breakSignature(sig, indent)

	// If this was a one-line function with an inline body, preserve that
	// body. The formatter's job is to reflow the signature, not to delete
	// code.
	if inlineBodyFound {
		body := strings.TrimSpace(inlineBody)
		afterClose := strings.TrimRight(inlineAfterClose, " \t")

		if strings.Count(formatted, "\n") == 0 {
			// Signature stayed on a single line; keep the body
			// inline.
			//
			// Always emit the closing brace, since we trimmed at
			// the opening brace when building `sig`.
			if body != "" {
				formatted += " " + body + " }"
			} else {
				formatted += " }"
			}
			formatted += afterClose
		} else {
			// Signature became multi-line; put the body on the next
			// line and keep the closing brace on its own line (plus
			// any trailing comment).
			if body != "" {
				formatted += "\n" + indent + "\t" + body
			}
			formatted += "\n" + indent + "}" + afterClose
		}
	} else {
		// Check if we need to add a blank line after the opening brace
		// (only if signature became multi-line)
		isMultiLine := strings.Count(formatted, "\n") > 0
		if isMultiLine {
			// Check if next non-empty line is not already blank
			nextLineIdx := startIdx + linesConsumed
			if nextLineIdx < len(lines) {
				nextLine := strings.TrimSpace(
					string(lines[nextLineIdx]),
				)
				if nextLine != "" && nextLine != "}" {
					// Add blank line after the signature
					formatted += "\n"
				}
			}
		}
	}

	// Add trailing newline if not the last line
	if startIdx+linesConsumed <= len(lines) {
		formatted += "\n"
	}

	return formatted, linesConsumed
}

// breakSignature breaks a function signature to fit within column limit.
func (f *FuncSigFormatter) breakSignature(sig, indent string) string {
	// Parse the signature parts Format: [func ][(receiver) ][name](params)[
	// (returns)][ {]

	// Check if it already fits and doesn't need formatting for readability
	// For multiline signatures, check each line
	needsFormatting := false
	if strings.Contains(sig, "\n") {
		// Multiline signature - check if any line exceeds limit
		for _, line := range strings.Split(sig, "\n") {
			if width.VisualLenWithTab(indent+line, f.cfg.TabStop) > f.
				cfg.
				ColumnLimit {

				needsFormatting = true
				break
			}
		}
		// Also check if there are func types with multiline content
		// that need breaking
		if !needsFormatting && strings.Contains(sig, "func(") &&
			strings.Contains(sig, "struct") {

			needsFormatting = true
		}
	} else {
		// Single line - simple check
		if width.VisualLenWithTab(indent+sig, f.cfg.TabStop) > f.
			cfg.
			ColumnLimit {

			needsFormatting = true
		}
	}
	if !needsFormatting && f.cfg.FormatInlineStructParams &&
		hasInlineStructWithSemicolons(sig) {

		needsFormatting = true
	}
	if !needsFormatting {
		return indent + sig
	}

	var result strings.Builder
	result.WriteString(indent)

	// Handle receiver if present: func (r *Type) Name(params)
	var funcPart string
	var rest string
	var paramStart int

	if strings.HasPrefix(sig, "func (") {
		// Method with receiver - find end of receiver paren
		recvEnd := f.findMatchingParen(sig, 5)
		if recvEnd == -1 {
			return indent + sig
		}
		// Find the actual params start (after receiver and method name)
		paramStart = strings.Index(sig[recvEnd+1:], "(")
		if paramStart == -1 {
			return indent + sig
		}
		paramStart += recvEnd + 1
		funcPart = sig[:paramStart]
		rest = sig[paramStart:]
	} else {
		// Regular function
		paramStart = strings.Index(sig, "(")
		if paramStart == -1 {
			return indent + sig
		}
		funcPart = sig[:paramStart]
		rest = sig[paramStart:]
	}

	// Find matching close paren for params
	paramEnd := f.findMatchingParen(rest, 0)
	if paramEnd == -1 {
		return indent + sig
	}

	params := rest[1:paramEnd] // content inside parens
	afterParams := rest[paramEnd+1:]

	// Check for return values
	var returns string
	var afterReturns string

	afterParams = strings.TrimLeft(afterParams, " ")
	if strings.HasPrefix(afterParams, "(") {
		// Has return value list in parens
		retEnd := f.findMatchingParen(afterParams, 0)
		if retEnd != -1 {
			returns = afterParams[:retEnd+1]
			afterReturns = afterParams[retEnd+1:]
		}
	} else if afterParams != "" && afterParams != "{" &&
		!strings.HasPrefix(afterParams, "{") {

		// Simple return type (no parens) - find where it ends
		braceIdx := strings.Index(afterParams, "{")
		if braceIdx != -1 {
			returns = strings.TrimSpace(afterParams[:braceIdx])
			afterReturns = afterParams[braceIdx:]
		} else {
			returns = strings.TrimSpace(afterParams)
			afterReturns = ""
		}
	} else {
		afterReturns = afterParams
	}

	// Now build the formatted signature
	result.WriteString(funcPart)
	result.WriteByte('(')

	// Break params
	contIndent := indent + "\t"
	currentLine := result.String()
	paramList := f.splitFuncParamList(params)
	paramList = filterNonEmptyTrimmed(paramList)

	forceParamListNewline := false
	if f.cfg.FormatInlineStructParams {
		for _, p := range paramList {
			if strings.Contains(p, "\n") ||
				hasInlineStructWithSemicolons(p) {

				forceParamListNewline = true
				break
			}
		}
	}

	// Calculate trailing for last param For params, we only consider ") ("
	// as trailing if there are returns that might need to break
	hasBrace := strings.TrimSpace(afterReturns) == "{" ||
		strings.HasPrefix(strings.TrimSpace(afterReturns), "{")
	hasParenReturns := returns != "" && strings.HasPrefix(returns, "(")

	// Minimal trailing when we might break returns to a new line
	trailingMinimal := ")"
	if hasParenReturns {
		trailingMinimal = ") ("
	} else if returns != "" {
		trailingMinimal = ") " + returns
		if hasBrace {
			trailingMinimal += " {"
		}
	} else if hasBrace {
		trailingMinimal += " {"
	}

	// Full trailing if everything fits on one line
	trailingFull := ")"
	if returns != "" {
		trailingFull = ") " + returns
	}
	if hasBrace {
		trailingFull += " {"
	}

	// In the "next" profile we prefer to keep short return lists inline,
	// and instead break parameters earlier if necessary.
	//
	// This avoids awkward (but parseable) formats like: M(a, b) ([]T,
	// error)
	//
	// and also helps avoid edge cases where a trailing comma ends up
	// exactly on the column boundary. Keep this conservative: apply it for
	// - function declarations without a receiver
	// - interface methods but not for receiver methods, where breaking
	//   results is often clearer.
	trimmedSig := strings.TrimSpace(sig)
	hasFuncKeyword := strings.Contains(trimmedSig, "func")
	// When a signature includes a prefix before `func` (e.g. `Field:
	// func...`), classify using the substring starting at `func` so
	// heuristics don't treat it like an interface method.
	sigAtFunc := trimmedSig
	if hasFuncKeyword && !strings.HasPrefix(sigAtFunc, "func") {
		if idx := strings.Index(sigAtFunc, "func"); idx >= 0 {
			sigAtFunc = strings.TrimSpace(sigAtFunc[idx:])
		}
	}

	isFuncDeclNoRecv := strings.HasPrefix(sigAtFunc, "func ") &&
		!strings.HasPrefix(sigAtFunc, "func (")
	isInterfaceMethod := !hasFuncKeyword
	isFuncLit := isFuncLitSignature(sigAtFunc)
	// Function literals are common in call-arg position; for readability we
	// prefer keeping their parameter list intact and breaking the return
	// list instead (matching the "next" golden spec).
	shouldPreferInlineReturns := (isFuncDeclNoRecv || isInterfaceMethod) &&
		!isFuncLit
	if f.cfg.PreferInlineSmallReturnList && shouldPreferInlineReturns &&
		hasParenReturns && len(paramList) > 1 && isSmallParenReturnList(
		returns,
	) {

		trailingMinimal = trailingFull
	}

	for i, param := range paramList {
		param = strings.TrimSpace(param)
		if param == "" {
			continue
		}

		separator := ""
		if i > 0 {
			separator = ", "
		}

		// Check if this param is a function type that needs breaking We
		// break func types when they contain nested complex types
		// (struct{}, etc.)
		paramToWrite := param
		isFuncParam := strings.Contains(param, "func(")
		needsFuncBreak := false
		currentLineIndent := leadingWhitespace(currentLine)
		if isFuncParam &&
			f.funcParamNeedsBreaking(param, currentLineIndent) {

			needsFuncBreak = true
			paramToWrite = f.formatFuncTypeParam(
				param, currentLineIndent,
			)
		}

		testAdd := separator + param
		testLine := currentLine + testAdd

		// For the last param, check both scenarios: 1. With minimal
		// trailing (allows returns to break to next line) 2. With full
		// trailing (everything on same line)
		isLast := i == len(paramList)-1
		var lineWithTrailing string
		if isLast {
			// Use minimal trailing - we can always break returns
			// later if needed
			lineWithTrailing = testLine + trailingMinimal
		} else {
			lineWithTrailing = testLine
		}

		// In the "next" profile, when we break before the next
		// parameter we append a comma to the current line. Reserve
		// space for that comma so we don't end up with punctuation
		// exactly on the column boundary (or a one-column overflow) due
		// to a late comma insertion.
		lineToCheck := lineWithTrailing
		if f.cfg.ReserveTrailingComma && !isLast {
			lineToCheck = testLine + ","
		}

		if needsFuncBreak {
			// If the function-typed parameter itself needs
			// multiline formatting, keep it on its own line (unless
			// the current line ends with `}` which commonly
			// indicates an inline struct param that will expand and
			// should keep `}, handler func(` on the same line after
			// gofmt).
			if i > 0 && strings.Contains(paramToWrite, "\n") &&
				!strings.HasSuffix(
					strings.TrimSpace(currentLine), "}",
				) {

				result.WriteByte(',')
				result.WriteByte('\n')
				result.WriteString(contIndent)
				result.WriteString(paramToWrite)
				currentLine = lastLine(paramToWrite)
				continue
			}

			// For func params that need internal breaking, try to
			// keep the func header inline (e.g., ", handler func(")
			// and only break the internal params
			if i > 0 {
				result.WriteString(", ")
			}
			result.WriteString(paramToWrite)
			if strings.Contains(paramToWrite, "\n") {
				currentLine = lastLine(paramToWrite)
			} else {
				currentLine = currentLine + ", " + paramToWrite
			}
		} else if forceParamListNewline && i == 0 {
			// Inline struct types (with semicolons) will become
			// multiline after gofmt; start the parameter list on a
			// fresh line so the expanded struct block is indented
			// cleanly and follow-up params can pack on the same
			// continuation line.
			result.WriteByte('\n')
			result.WriteString(contIndent)
			result.WriteString(paramToWrite)
			if strings.Contains(paramToWrite, "\n") {
				currentLine = lastLine(paramToWrite)
			} else {
				currentLine = contIndent + paramToWrite
			}
		} else if width.VisualLenWithTab(lineToCheck, f.cfg.TabStop) > f.
			cfg.
			ColumnLimit {

			// Need to break - put param on new line
			if i > 0 {
				result.WriteByte(',')
			}
			result.WriteByte('\n')
			result.WriteString(contIndent)
			result.WriteString(paramToWrite)
			if strings.Contains(paramToWrite, "\n") {
				currentLine = lastLine(paramToWrite)
			} else {
				currentLine = contIndent + paramToWrite
			}
		} else {
			if i > 0 {
				result.WriteString(", ")
			}
			result.WriteString(paramToWrite)
			if strings.Contains(paramToWrite, "\n") {
				currentLine = lastLine(paramToWrite)
			} else {
				currentLine = testLine
			}
		}
	}

	result.WriteByte(')')

	// Handle returns
	if returns != "" {
		if strings.HasPrefix(returns, "(") {
			// Check if returns fit on current line (after the
			// closing paren we just wrote)
			testLine := currentLine + ") " + returns
			if hasBrace {
				testLine += " {"
			}

			if width.VisualLenWithTab(testLine, f.cfg.TabStop) > f.
				cfg.
				ColumnLimit {

				// Returns don't all fit inline
				retContent := returns[1 : len(returns)-1]
				retList := f.splitParams(retContent)
				retList = filterNonEmptyTrimmed(retList)

				if f.cfg.CanonicalMultilineSigLists {
					// For the "next" profile, avoid
					// partially breaking the return list
					// like: f(...) (a, b) Prefer the
					// gofmt-like style: f(...) ( a, b, )
					result.WriteString(" (")
					result.WriteByte('\n')

					for _, ret := range retList {
						result.WriteString(contIndent)
						result.WriteString(ret)
						result.WriteString(",")
						result.WriteByte('\n')
					}

					result.WriteString(indent)
					result.WriteByte(')')
				} else {
					// Legacy behavior: attempt to keep the
					// first return inline after ") (" and
					// left-flow pack the remainder.
					//
					// Note: this may produce
					// partially-broken return lists which
					// are less gofmt-like, but is kept to
					// preserve golden fixtures.
					firstRet := ""
					if len(retList) > 0 {
						firstRet = strings.TrimSpace(
							retList[0],
						)
					}
					testFirstInline := currentLine + ") (" +
						firstRet
					if len(retList) > 1 {
						testFirstInline += ","
					} else {
						testFirstInline += ")"
						if hasBrace {
							testFirstInline += " {"
						}
					}

					if width.VisualLenWithTab(
						testFirstInline, f.cfg.TabStop,
					) > f.
						cfg.
						ColumnLimit {

						// First return doesn't fit
						// inline - put all returns on
						// fresh line
						result.WriteString(" (")
						result.WriteByte('\n')
						result.WriteString(contIndent)
						currentLine = contIndent

						// Now left-flow pack the
						// returns from the fresh line
						for i, ret := range retList {
							ret = strings.TrimSpace(
								ret,
							)
							if ret == "" {
								continue
							}

							separator := ""
							if i > 0 {
								separator = ", "
							}

							testAdd := separator +
								ret
							isLastRet := i == len(
								retList,
							)-
								1
							testCheck := currentLine +
								testAdd
							if isLastRet {
								testCheck += ")"
								if hasBrace {
									testCheck += " {"
								}
							}

							if width.VisualLenWithTab(
								testCheck,
								f.cfg.TabStop,
							) > f.
								cfg.
								ColumnLimit {

								if i > 0 {
									result.WriteByte(',')
								}
								result.WriteByte(
									'\n',
								)
								result.WriteString(
									contIndent,
								)
								currentLine = contIndent +
									ret
								result.WriteString(
									ret,
								)
							} else {
								if i > 0 {
									result.WriteString(
										", ",
									)
									currentLine += ", "
								}
								result.WriteString(
									ret,
								)
								currentLine += ret
							}
						}
						result.WriteByte(')')
					} else {
						// First return fits inline -
						// use left-flow packing
						result.WriteString(" (")
						currentLine = currentLine +
							") ("

						for i, ret := range retList {
							ret = strings.TrimSpace(
								ret,
							)
							if ret == "" {
								continue
							}

							separator := ""
							if i > 0 {
								separator = ", "
							}

							testAdd := separator +
								ret
							isLastRet := i == len(
								retList,
							)-
								1
							testCheck := currentLine +
								testAdd
							if isLastRet {
								testCheck += ")"
								if hasBrace {
									testCheck += " {"
								}
							}

							if width.VisualLenWithTab(
								testCheck,
								f.cfg.TabStop,
							) > f.
								cfg.
								ColumnLimit {

								if i > 0 {
									result.WriteByte(',')
								}
								result.WriteByte(
									'\n',
								)
								result.WriteString(
									contIndent,
								)
								currentLine = contIndent +
									ret
								result.WriteString(
									ret,
								)
							} else {
								if i > 0 {
									result.WriteString(
										", ",
									)
									currentLine += ", "
								}
								result.WriteString(
									ret,
								)
								currentLine += ret
							}
						}
						result.WriteByte(')')
					}
				}
			} else {
				// Returns fit on the same line
				result.WriteByte(' ')
				result.WriteString(returns)
			}
		} else {
			// Simple return type
			result.WriteByte(' ')
			result.WriteString(returns)
		}
	}

	// Add brace if present
	afterReturns = strings.TrimSpace(afterReturns)
	if strings.HasPrefix(afterReturns, "{") {
		result.WriteString(" {")
	}

	return result.String()
}

func filterNonEmptyTrimmed(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
	}

	return out
}

func isSmallParenReturnList(returns string) bool {
	trimmed := strings.TrimSpace(returns)
	if len(trimmed) < 2 || trimmed[0] != '(' ||
		trimmed[len(trimmed)-1] != ')' {

		return false
	}
	if strings.Contains(trimmed, "\n") {
		return false
	}
	content := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	if content == "" {
		return true
	}
	parts := filterNonEmptyTrimmed(scanner.SplitTopLevel(content))

	// "Short" here is intentionally conservative; keeping `([]T, error)` on
	// one line is almost always desirable.
	return len(parts) <= 2
}

func leadingWhitespace(s string) string {
	if s == "" {
		return ""
	}
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}

	return s[:i]
}

func lastLine(s string) string {
	if s == "" {
		return ""
	}
	if idx := strings.LastIndexByte(s, '\n'); idx >= 0 {
		return s[idx+1:]
	}

	return s
}

func hasInlineStructWithSemicolons(s string) bool {
	// This is intentionally a lightweight heuristic; we are only looking
	// for the gofmt-expands-inline-struct-types pattern `struct{ a; b; }`
	// (or `struct {`).
	if !strings.Contains(s, ";") {
		return false
	}

	return strings.Contains(s, "struct{") || strings.Contains(s, "struct {")
}

// findMatchingParen finds the index of the closing paren matching the one at
// start.
func (f *FuncSigFormatter) findMatchingParen(s string, start int) int {
	if start >= len(s) || s[start] != '(' {
		return -1
	}

	depth := 0
	inString := false
	escaped := false

	for i := start; i < len(s); i++ {
		c := s[i]

		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && inString {
			escaped = true
			continue
		}
		if c == '"' || c == '`' {
			inString = !inString
			continue
		}
		if inString {
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
	}

	return -1
}

// splitParams splits parameter list by commas, respecting nested
// parens/brackets.
func (f *FuncSigFormatter) splitParams(params string) []string {
	return scanner.SplitTopLevel(params)
}

func (f *FuncSigFormatter) splitFuncParamList(params string) []string {
	parts := filterNonEmptyTrimmed(scanner.SplitTopLevel(params))
	if len(parts) <= 1 {
		return parts
	}

	// In the "next" profile, treat identifier name lists that share a type
	// as a single parameter group.
	//
	// Raw comma splitting will incorrectly split: edgeInfo *T, c1, c2 *U
	// into: "edgeInfo *T", "c1", "c2 *U" which can yield ugly wraps like:
	// func(edgeInfo *T, c1, c2 *U)
	//
	// But do NOT apply this to return lists: return elements are types, and
	// merging would collapse `Result, error` into a single list element and
	// change next goldens.
	isNextLike := f.cfg.CanonicalMultilineSigLists ||
		f.cfg.ReserveTrailingComma ||
		f.cfg.PreferInlineSmallReturnList
	if !isNextLike {
		return parts
	}

	isBareName := func(s string) bool {
		s = strings.TrimSpace(s)
		if s == "" {
			return false
		}

		return token.IsIdentifier(s)
	}

	isComplexSharedType := func(typePart string) bool {
		typePart = strings.TrimSpace(typePart)
		if typePart == "" {
			return false
		}
		// Keep this deliberately narrow to preserve existing next
		// goldens: prefer merging only for long, package-qualified
		// pointer types (the common "c1, c2 *pkg.Type" callback
		// pattern).
		if strings.HasPrefix(typePart, "*") && strings.Contains(
			typePart, ".",
		) {

			return true
		}

		// Fallback for unusually long types where splitting inside the
		// shared-type name list tends to look bad.
		return len(typePart) >= 32
	}

	var merged []string
	for i := 0; i < len(parts); i++ {
		cur := strings.TrimSpace(parts[i])
		if cur == "" {
			continue
		}

		// Merge only the bare name immediately preceding the typed
		// segment. This avoids breaking `c1, c2 *T` into `c1,` + `c2
		// *T` (ugly), while still allowing longer name lists like `a,
		// b, c, d int` to be wrapped progressively (matching existing
		// goldens).
		if isBareName(cur) && i+1 < len(parts) {
			next := strings.TrimSpace(parts[i+1])
			if strings.IndexAny(next, " \t\n") >= 0 {
				typePart := next[strings.IndexAny(
					next, " 	\n",
				):]
				if isComplexSharedType(typePart) {
					merged = append(merged, cur+", "+next)
					i++
					continue
				}
			}
		}

		merged = append(merged, cur)
	}

	return merged
}

// funcParamNeedsBreaking checks if a function-typed parameter needs to be
// broken into multiple lines. This is true when the param contains a nested
// complex type like struct{} that will be expanded by gofmt, or already
// contains multiline content.
func (f *FuncSigFormatter) funcParamNeedsBreaking(param,
	baseIndent string) bool {

	// Check for inline struct definition with semicolons (will be expanded
	// by gofmt)
	if strings.Contains(param, "struct{") && strings.Contains(param, ";") {
		return true
	}

	// Check if the param is a func type containing a multiline struct
	// (already expanded)
	if strings.Contains(param, "func(") &&
		strings.Contains(param, "struct") {

		// If param contains newlines, it has embedded multiline content
		if strings.Contains(param, "\n") {
			return true
		}
	}

	// Check if the param itself exceeds the limit when placed on a
	// continuation line
	testLine := baseIndent + param
	if width.VisualLenWithTab(testLine, f.cfg.TabStop) > f.cfg.ColumnLimit {
		// In next-profile mode, allow breaking long function-typed
		// parameters even when they do not contain nested struct types,
		// but avoid breaking the inner parameter list for function
		// types that already have explicit return types: those tend to
		// look worse when we break both the inner params and the outer
		// signature.
		if f.cfg.BreakLongFuncTypeParams {
			if strings.Contains(param, "func(") {
				funcIdx := strings.Index(param, "func(")
				if funcIdx >= 0 {
					rest := param[funcIdx+
						4:] // starts with "("
					if len(rest) > 0 && rest[0] == '(' {
						end := f.findMatchingParen(
							rest, 0,
						)
						if end >= 0 &&
							end+1 < len(rest) {

							afterParams := strings.TrimSpace(
								rest[end+1:],
							)
							// Has explicit results
							// (e.g. `error` or `(T,
							// error)`): don't break
							// inner params unless
							// we are forced by an
							// inline struct
							// (handled above).
							if afterParams != "" {
								return false
							}
						}
					}
				}
			}

			return true
		}

		// Legacy behavior: only break if it's a func type with complex
		// params.
		if strings.Contains(param, "func(") &&
			strings.Contains(param, "struct") {

			return true
		}
	}

	return false
}

// formatFuncTypeParam formats a function-typed parameter, breaking its inner
// params if they exceed the column limit. Returns the formatted parameter text.
// param should be like "handler func(cfg struct{ ... }) error" baseIndent is
// the indent for the parameter itself (e.g., "\t" if on a continuation line).
func (f *FuncSigFormatter) formatFuncTypeParam(param,
	baseIndent string) string {

	// Find "func(" in the parameter
	funcIdx := strings.Index(param, "func(")
	if funcIdx == -1 {
		return param
	}

	// Get the prefix (variable name before func)
	prefix := param[:funcIdx+4] // includes "func"
	rest := param[funcIdx+4:]   // starts with "("

	// Find matching close paren for the func params
	if len(rest) == 0 || rest[0] != '(' {
		return param
	}

	paramEnd := f.findMatchingParen(rest, 0)
	if paramEnd == -1 {
		return param
	}

	innerParams := rest[1:paramEnd]
	afterParams := rest[paramEnd+1:] // e.g., " error"

	// Split inner params
	innerList := f.splitFuncParamList(innerParams)
	if len(innerList) == 0 {
		return param
	}

	// Check if inner params need breaking They need breaking if: 1. Any
	// param is multiline (contains struct expansion), or 2. The full line
	// exceeds the limit
	needsBreaking := false
	for _, p := range innerList {
		p = strings.TrimSpace(p)
		if strings.Contains(p, "\n") {
			// Inner param is multiline - needs breaking for
			// readability
			needsBreaking = true
			break
		}
	}

	if !needsBreaking {
		// Check if the single line form exceeds the limit
		testLine := baseIndent + prefix + "("
		for i, p := range innerList {
			p = strings.TrimSpace(p)
			if i > 0 {
				testLine += ", "
			}
			testLine += p
		}
		testLine += ")" + afterParams
		if width.VisualLenWithTab(testLine, f.cfg.TabStop) > f.
			cfg.
			ColumnLimit {

			needsBreaking = true
		}
	}

	if !needsBreaking {
		return param // Fits on one line and no multiline content
	}

	// Decide whether to force a canonical multiline shape. This is required
	// for inline struct params, because splitting across lines without a
	// trailing comma is not parseable.
	forceCanonical := false
	for _, p := range innerList {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if hasInlineStructWithSemicolons(p) ||
			strings.Contains(p, "\n") {

			forceCanonical = true
			break
		}
	}

	var result strings.Builder
	result.WriteString(prefix)
	if forceCanonical {
		contIndent := baseIndent + "\t"
		result.WriteString("(\n")
		for _, p := range innerList {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			result.WriteString(contIndent)
			result.WriteString(p)
			result.WriteString(",\n")
		}
		result.WriteString(baseIndent)
		result.WriteString(")")
		result.WriteString(afterParams)

		return result.String()
	}

	// If this function type already has explicit results, prefer keeping
	// its inner parameter list intact. Breaking both the inner params and
	// the outer signature often produces a noisier result than leaving the
	// inner list packed and breaking only at the outer signature boundary.
	if strings.TrimSpace(afterParams) != "" {
		return param
	}

	// Greedy/packed breaking: keep as many inner params as fit on the same
	// line, breaking only when needed (partial break style).
	contIndent := baseIndent + "\t"
	result.WriteString("(")
	currentLine := baseIndent + prefix + "("

	for i, p := range innerList {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		sep := ""
		if i > 0 {
			sep = ", "
		}

		testLine := currentLine + sep + p
		testCheck := testLine
		if i == len(innerList)-1 {
			testCheck += ")" + afterParams
		}

		if i > 0 &&
			width.VisualLenWithTab(testCheck, f.cfg.TabStop) > f.
				cfg.
				ColumnLimit {

			// Break before this param; ensure previous param had
			// its comma.
			result.WriteByte(',')
			result.WriteByte('\n')
			result.WriteString(contIndent)
			result.WriteString(p)
			currentLine = contIndent + p
			continue
		}

		if i > 0 {
			result.WriteString(", ")
			currentLine += ", "
		}
		result.WriteString(p)
		currentLine += p
	}

	result.WriteString(")")
	result.WriteString(afterParams)

	return result.String()
}

// formatInterfaceMethod formats a long interface method declaration. Returns
// the formatted string and number of source lines consumed.
func (f *FuncSigFormatter) formatInterfaceMethod(lines [][]byte, startIdx int,
	indent string) (string, int) {

	// Collect the complete method signature (might span multiple lines)
	var sigBuilder strings.Builder
	linesConsumed := 0
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0

	for idx := startIdx; idx < len(lines); idx++ {
		line := string(lines[idx])
		linesConsumed++

		// Add this line to signature (normalize whitespace). Interface
		// method declarations can legally span multiple lines only
		// while a delimiter (paren/bracket/brace) is unbalanced at the
		// end of the line.
		if sigBuilder.Len() > 0 {
			sigBuilder.WriteByte(' ')
		}
		sigBuilder.WriteString(strings.TrimSpace(line))

		b := []byte(line)
		i := 0
		for i < len(b) {
			switch {
			case scanner.IsStringStart(b, i):
				i = scanner.ScanString(b, i)
				continue

			case scanner.IsLineCommentStart(b, i):
				// Line comment: ignore the rest for delimiter
				// tracking.
				i = len(b)
				continue

			case scanner.IsBlockCommentStart(b, i):
				// Block comment: skip the whole comment for
				// delimiter tracking.
				i = scanner.ScanBlockComment(b, i)
				continue
			}

			switch b[i] {
			case '(':
				parenDepth++

			case ')':
				parenDepth--

			case '[':
				bracketDepth++

			case ']':
				bracketDepth--

			case '{':
				braceDepth++

			case '}':
				braceDepth--
			}
			i++
		}

		if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
			break
		}
	}

	sig := sigBuilder.String()

	// Format it using breakInterfaceMethod
	formatted := f.breakInterfaceMethod(sig, indent)

	// Add trailing newline if not the last line
	if startIdx+linesConsumed <= len(lines) {
		formatted += "\n"
	}

	return formatted, linesConsumed
}

func splitTrailingLineComment(s string) (before, comment string) {
	b := []byte(s)
	for i := 0; i < len(b); {
		switch {
		case scanner.IsStringStart(b, i):
			i = scanner.ScanString(b, i)
			continue

		case scanner.IsLineCommentStart(b, i):
			return strings.TrimRight(string(b[:i]), " \t"), string(
				b[i:],
			)

		case scanner.IsBlockCommentStart(b, i):
			i = scanner.ScanBlockComment(b, i)
			continue
		}
		i++
	}

	return s, ""
}

// breakInterfaceMethod breaks an interface method to fit within column limit.
func (f *FuncSigFormatter) breakInterfaceMethod(sig, indent string) string {
	sig, trailingComment := splitTrailingLineComment(sig)
	commentSuffix := ""
	if trailingComment != "" {
		commentSuffix = " " + strings.TrimLeft(
			trailingComment, " 	",
		)
	}

	// In the "next" profile we want interface methods to follow the same
	// signature-breaking behavior as regular function signatures (including
	// preferring to break params before partially breaking short return
	// lists).
	//
	// The legacy interface-method formatter historically had its own logic
	// and can end up with awkward edge cases like a comma exactly on the
	// column boundary: M(a, b) ([]T, error)
	//
	// When next-profile behaviors are enabled, delegate to the main
	// signature formatter to keep behavior consistent across contexts.
	if f.cfg.PreferInlineSmallReturnList || f.cfg.ReserveTrailingComma ||
		f.cfg.CanonicalMultilineSigLists {

		return f.breakSignature(sig, indent) + commentSuffix
	}

	// Check if it already fits
	if width.VisualLenWithTab(indent+sig+commentSuffix, f.cfg.TabStop) <= f.
		cfg.
		ColumnLimit {

		return indent + sig + commentSuffix
	}

	var result strings.Builder
	result.WriteString(indent)

	// Parse: MethodName(params) [(returns)]
	parenIdx := strings.Index(sig, "(")
	if parenIdx == -1 {
		return indent + sig + commentSuffix
	}

	methodName := sig[:parenIdx]
	rest := sig[parenIdx:]

	// Find matching close paren for params
	paramEnd := f.findMatchingParen(rest, 0)
	if paramEnd == -1 {
		return indent + sig + commentSuffix
	}

	params := rest[1:paramEnd]
	afterParams := strings.TrimSpace(rest[paramEnd+1:])

	// Check for return values
	var returns string
	if strings.HasPrefix(afterParams, "(") {
		retEnd := f.findMatchingParen(afterParams, 0)
		if retEnd != -1 {
			returns = afterParams[:retEnd+1]
		}
	} else if afterParams != "" {
		returns = afterParams
	}

	// Build the formatted method
	result.WriteString(methodName)
	result.WriteByte('(')

	// Break params
	contIndent := indent + "\t"
	currentLine := result.String()
	paramList := f.splitParams(params)

	// Calculate trailing for last param - use minimal ") (" if there are
	// returns
	hasParenReturns := returns != "" && strings.HasPrefix(returns, "(")
	trailingMinimal := ")"
	if hasParenReturns {
		trailingMinimal = ") ("
	} else if returns != "" {
		trailingMinimal = ") " + returns
	}

	for i, param := range paramList {
		param = strings.TrimSpace(param)
		if param == "" {
			continue
		}

		separator := ""
		if i > 0 {
			separator = ", "
		}

		testAdd := separator + param
		testLine := currentLine + testAdd

		// For the last param, use minimal trailing
		isLast := i == len(paramList)-1
		var lineWithTrailing string
		if isLast {
			lineWithTrailing = testLine + trailingMinimal
		} else {
			lineWithTrailing = testLine
		}

		if width.VisualLenWithTab(lineWithTrailing, f.cfg.TabStop) > f.
			cfg.
			ColumnLimit {

			// Need to break
			if i > 0 {
				result.WriteByte(',')
			}
			result.WriteByte('\n')
			result.WriteString(contIndent)
			currentLine = contIndent + param
			result.WriteString(param)
		} else {
			if i > 0 {
				result.WriteString(", ")
			}
			result.WriteString(param)
			currentLine = testLine
		}
	}

	result.WriteByte(')')

	// Handle returns
	if returns != "" {
		if strings.HasPrefix(returns, "(") {
			// Check if returns fit on current line (after the
			// closing paren we just wrote)
			testLine := currentLine + ") " + returns
			if width.VisualLenWithTab(testLine, f.cfg.TabStop) > f.
				cfg.
				ColumnLimit {

				// Returns don't all fit inline - use left-flow
				// to pack
				result.WriteString(" (")
				currentLine = currentLine + ") ("

				retContent := returns[1 : len(returns)-1]
				retList := f.splitParams(retContent)

				for i, ret := range retList {
					ret = strings.TrimSpace(ret)
					if ret == "" {
						continue
					}

					separator := ""
					if i > 0 {
						separator = ", "
					}

					testAdd := separator + ret
					// For last return, account for closing
					// paren
					isLastRet := i == len(retList)-1
					testCheck := currentLine + testAdd
					if isLastRet {
						testCheck += ")"
					}

					if width.VisualLenWithTab(
						testCheck, f.cfg.TabStop,
					) > f.
						cfg.
						ColumnLimit {

						// Need to break
						if i > 0 {
							result.WriteByte(',')
						}
						result.WriteByte('\n')
						result.WriteString(contIndent)
						currentLine = contIndent + ret
						result.WriteString(ret)
					} else {
						if i > 0 {
							result.WriteString(", ")
							currentLine += ", "
						}
						result.WriteString(ret)
						currentLine += ret
					}
				}
				result.WriteByte(')')
			} else {
				// Returns fit on the same line
				result.WriteByte(' ')
				result.WriteString(returns)
			}
		} else {
			// Simple return type
			result.WriteByte(' ')
			result.WriteString(returns)
		}
	}

	return result.String() + commentSuffix
}

// FormatFuncSignature formats a function signature (the line starting with
// "func" up to and including the opening brace) using left-flow packing. This
// function is exported for use by the DSL engine. The signature parameter
// should be the complete func line ending with "{". Returns the formatted
// signature with a trailing newline if multi-line, and a boolean indicating if
// a blank line should be added after the opening brace.
func FormatFuncSignature(signature, indent string, colLimit,
	tabStop int) (string, bool) {

	f := &FuncSigFormatter{cfg: FuncSigConfig{
		ColumnLimit: colLimit,
		TabStop:     tabStop,
	}}

	formatted := f.breakSignature(signature, indent)
	isMultiLine := hasNewlineOutsideBraces(formatted)
	// Legacy: avoid adding a blank line after signatures whose multiline
	// content comes only from nested multiline function types.
	if strings.Contains(formatted, "func(\n") {
		isMultiLine = false
	}

	return formatted, isMultiLine
}

// FormatInterfaceMethod formats an interface method declaration using left-flow
// packing. This function is exported for use by the DSL engine.
func FormatInterfaceMethod(method, indent string, colLimit,
	tabStop int) string {

	f := &FuncSigFormatter{cfg: FuncSigConfig{
		ColumnLimit: colLimit,
		TabStop:     tabStop,
	}}

	return f.breakInterfaceMethod(method, indent)
}

// FormatInterfaceMethodNext formats an interface method signature for the
// "next" profile, using the same normalization/collapse behavior as
// FormatFuncSignatureNext.
func FormatInterfaceMethodNext(method, indent string, colLimit,
	tabStop int) string {

	formatted, _ := FormatFuncSignatureNext(
		method, indent, colLimit, tabStop,
	)
	if canon, ok := canonicalizeInterfaceMethodParenReturnList(
		formatted, indent, tabStop,
	); ok {

		return canon
	}

	return formatted
}

func canonicalizeInterfaceMethodParenReturnList(methodSig, indent string,
	tabStop int) (string, bool) {

	// Only apply to interface methods that already have a multiline,
	// parenthesized return list. Keep this conservative to avoid drifting
	// from the next golden fixtures (which currently expect a packed style
	// for many signatures).
	//
	// The next signature tests require canonical multiline results for
	// "long" two-item return lists like: M() (map[K]V, error) => rewrite
	// to: M() ( map[K]V, error, )
	f := NewFuncSigFormatter(
		FuncSigConfig{
			ColumnLimit: 80,
			TabStop:     tabStop,
		},
	)

	paramsOpen := strings.IndexByte(methodSig, '(')
	if paramsOpen < 0 {
		return "", false
	}
	paramsClose := f.findMatchingParen(methodSig, paramsOpen)
	if paramsClose < 0 {
		return "", false
	}

	i := paramsClose + 1
	for i < len(methodSig) &&
		(methodSig[i] == ' ' || methodSig[i] == '\t' ||
			methodSig[i] == '\n') {

		i++
	}
	if i >= len(methodSig) || methodSig[i] != '(' {
		return "", false
	}
	retOpen := i
	retClose := f.findMatchingParen(methodSig, retOpen)
	if retClose < 0 {
		return "", false
	}

	retContent := strings.TrimSpace(methodSig[retOpen+1 : retClose])
	if !strings.Contains(retContent, "\n") {
		return "", false
	}

	parts := filterNonEmptyTrimmed(scanner.SplitTopLevel(retContent))
	if len(parts) != 2 {
		return "", false
	}

	contIndent := indent + "\t"
	var b strings.Builder
	b.Grow(len(methodSig) + len(parts)*4)
	b.WriteString(methodSig[:retOpen])
	b.WriteString("(\n")
	for _, p := range parts {
		b.WriteString(contIndent)
		b.WriteString(strings.TrimSpace(p))
		b.WriteString(",\n")
	}
	b.WriteString(indent)
	b.WriteByte(')')
	b.WriteString(methodSig[retClose+1:])
	out := b.String()
	if out == methodSig {
		return "", false
	}

	return out, true
}
