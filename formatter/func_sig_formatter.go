package formatter

import (
	"bytes"
	"strings"

	"github.com/lightninglabs/llformat/scanner"
	"github.com/lightninglabs/llformat/width"
)

// FuncSigConfig holds configuration for the function signature formatter.
type FuncSigConfig struct {
	ColumnLimit int
	TabStop     int
}

// FuncSigFormatter formats long function signatures by breaking them across lines.
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
		if strings.Contains(trimmed, "interface") && strings.Contains(trimmed, "{") {
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
				formatted, linesConsumed := f.formatSignature(lines, i, indent)
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
				formatted, linesConsumed := f.formatInterfaceMethod(lines, i, indent)
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

// isFuncSignature checks if a line starts a function signature.
func (f *FuncSigFormatter) isFuncSignature(trimmed string) bool {
	// Match: func name(, func (receiver) name(, or method in interface
	if strings.HasPrefix(trimmed, "func ") {
		return true
	}
	return false
}

// isInterfaceMethod checks if a line looks like an interface method declaration.
// Interface methods are: identifier(params) [returns] - no func keyword, no brace.
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

// formatSignature formats a long function signature.
// Returns the formatted string and number of source lines consumed.
func (f *FuncSigFormatter) formatSignature(lines [][]byte, startIdx int, indent string) (string, int) {
	// First, collect the complete signature (might span multiple lines already)
	var sigBuilder strings.Builder
	linesConsumed := 0
	braceFound := false
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
				// Include everything up to and including the brace
				sigBuilder.WriteString(strings.TrimRight(line[:i+1], " \t"))
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

	// Check if we need to add a blank line after the opening brace
	// (only if signature became multi-line)
	isMultiLine := strings.Count(formatted, "\n") > 0
	if isMultiLine {
		// Check if next non-empty line is not already blank
		nextLineIdx := startIdx + linesConsumed
		if nextLineIdx < len(lines) {
			nextLine := strings.TrimSpace(string(lines[nextLineIdx]))
			if nextLine != "" && nextLine != "}" {
				// Add blank line after the signature
				formatted += "\n"
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
	// Parse the signature parts
	// Format: [func ][(receiver) ][name](params)[ (returns)][ {]

	// Check if it already fits and doesn't need formatting for readability
	// For multiline signatures, check each line
	needsFormatting := false
	if strings.Contains(sig, "\n") {
		// Multiline signature - check if any line exceeds limit
		for _, line := range strings.Split(sig, "\n") {
			if width.VisualLenWithTab(indent+line, f.cfg.TabStop) > f.cfg.ColumnLimit {
				needsFormatting = true
				break
			}
		}
		// Also check if there are func types with multiline content that need breaking
		if !needsFormatting && strings.Contains(sig, "func(") && strings.Contains(sig, "struct") {
			needsFormatting = true
		}
	} else {
		// Single line - simple check
		if width.VisualLenWithTab(indent+sig, f.cfg.TabStop) > f.cfg.ColumnLimit {
			needsFormatting = true
		}
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
	} else if afterParams != "" && afterParams != "{" && !strings.HasPrefix(afterParams, "{") {
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
	paramList := f.splitParams(params)

	// Calculate trailing for last param
	// For params, we only consider ") (" as trailing if there are returns that might need to break
	hasBrace := strings.TrimSpace(afterReturns) == "{" || strings.HasPrefix(strings.TrimSpace(afterReturns), "{")
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

	for i, param := range paramList {
		param = strings.TrimSpace(param)
		if param == "" {
			continue
		}

		separator := ""
		if i > 0 {
			separator = ", "
		}

		// Check if this param is a function type that needs breaking
		// We break func types when they contain nested complex types (struct{}, etc.)
		paramToWrite := param
		isFuncParam := strings.Contains(param, "func(")
		needsFuncBreak := false
		if isFuncParam && f.funcParamNeedsBreaking(param, contIndent) {
			needsFuncBreak = true
			paramToWrite = f.formatFuncTypeParam(param, contIndent)
		}

		testAdd := separator + param
		testLine := currentLine + testAdd

		// For the last param, check both scenarios:
		// 1. With minimal trailing (allows returns to break to next line)
		// 2. With full trailing (everything on same line)
		isLast := i == len(paramList)-1
		var lineWithTrailing string
		if isLast {
			// Use minimal trailing - we can always break returns later if needed
			lineWithTrailing = testLine + trailingMinimal
		} else {
			lineWithTrailing = testLine
		}

		if needsFuncBreak {
			// For func params that need internal breaking, try to keep the func header
			// inline (e.g., ", handler func(") and only break the internal params
			if i > 0 {
				result.WriteString(", ")
			}
			result.WriteString(paramToWrite)
			if strings.Contains(paramToWrite, "\n") {
				// Get the last line of the formatted param to track current position
				lines := strings.Split(paramToWrite, "\n")
				currentLine = lines[len(lines)-1]
			} else {
				currentLine = currentLine + ", " + paramToWrite
			}
		} else if width.VisualLenWithTab(lineWithTrailing, f.cfg.TabStop) > f.cfg.ColumnLimit {
			// Need to break - put param on new line
			if i > 0 {
				result.WriteByte(',')
			}
			result.WriteByte('\n')
			result.WriteString(contIndent)
			result.WriteString(paramToWrite)
			currentLine = contIndent + paramToWrite
		} else {
			if i > 0 {
				result.WriteString(", ")
			}
			result.WriteString(paramToWrite)
			currentLine = testLine
		}
	}

	result.WriteByte(')')

	// Handle returns
	if returns != "" {
		if strings.HasPrefix(returns, "(") {
			// Check if returns fit on current line (after the closing paren we just wrote)
			testLine := currentLine + ") " + returns
			if hasBrace {
				testLine += " {"
			}

			if width.VisualLenWithTab(testLine, f.cfg.TabStop) > f.cfg.ColumnLimit {
				// Returns don't all fit inline
				retContent := returns[1 : len(returns)-1]
				retList := f.splitParams(retContent)

				// Check if the first return item fits inline after ") ("
				// If not, put all returns on a fresh line
				firstRet := ""
				if len(retList) > 0 {
					firstRet = strings.TrimSpace(retList[0])
				}
				testFirstInline := currentLine + ") (" + firstRet
				if len(retList) > 1 {
					testFirstInline += ","
				} else {
					testFirstInline += ")"
					if hasBrace {
						testFirstInline += " {"
					}
				}

				if width.VisualLenWithTab(testFirstInline, f.cfg.TabStop) > f.cfg.ColumnLimit {
					// First return doesn't fit inline - put all returns on fresh line
					result.WriteString(" (")
					result.WriteByte('\n')
					result.WriteString(contIndent)
					currentLine = contIndent

					// Now left-flow pack the returns from the fresh line
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
						isLastRet := i == len(retList)-1
						testCheck := currentLine + testAdd
						if isLastRet {
							testCheck += ")"
							if hasBrace {
								testCheck += " {"
							}
						}

						if width.VisualLenWithTab(testCheck, f.cfg.TabStop) > f.cfg.ColumnLimit {
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
					// First return fits inline - use left-flow packing
					result.WriteString(" (")
					currentLine = currentLine + ") ("

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
						isLastRet := i == len(retList)-1
						testCheck := currentLine + testAdd
						if isLastRet {
							testCheck += ")"
							if hasBrace {
								testCheck += " {"
							}
						}

						if width.VisualLenWithTab(testCheck, f.cfg.TabStop) > f.cfg.ColumnLimit {
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

// findMatchingParen finds the index of the closing paren matching the one at start.
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

// splitParams splits parameter list by commas, respecting nested parens/brackets.
func (f *FuncSigFormatter) splitParams(params string) []string {
	return scanner.SplitTopLevel(params)
}

// funcParamNeedsBreaking checks if a function-typed parameter needs to be broken
// into multiple lines. This is true when the param contains a nested complex type
// like struct{} that will be expanded by gofmt, or already contains multiline content.
func (f *FuncSigFormatter) funcParamNeedsBreaking(param, baseIndent string) bool {
	// Check for inline struct definition with semicolons (will be expanded by gofmt)
	if strings.Contains(param, "struct{") && strings.Contains(param, ";") {
		return true
	}

	// Check if the param is a func type containing a multiline struct (already expanded)
	if strings.Contains(param, "func(") && strings.Contains(param, "struct") {
		// If param contains newlines, it has embedded multiline content
		if strings.Contains(param, "\n") {
			return true
		}
	}

	// Check if the param itself exceeds the limit when placed on a continuation line
	testLine := baseIndent + param
	if width.VisualLenWithTab(testLine, f.cfg.TabStop) > f.cfg.ColumnLimit {
		// Only break if it's a func type with complex params
		if strings.Contains(param, "func(") && strings.Contains(param, "struct") {
			return true
		}
	}
	return false
}

// formatFuncTypeParam formats a function-typed parameter, breaking its inner params
// if they exceed the column limit. Returns the formatted parameter text.
// param should be like "handler func(cfg struct{ ... }) error"
// baseIndent is the indent for the parameter itself (e.g., "\t" if on a continuation line).
func (f *FuncSigFormatter) formatFuncTypeParam(param, baseIndent string) string {
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
	innerList := f.splitParams(innerParams)
	if len(innerList) == 0 {
		return param
	}

	// Check if inner params need breaking
	// They need breaking if:
	// 1. Any param is multiline (contains struct expansion), or
	// 2. The full line exceeds the limit
	needsBreaking := false
	for _, p := range innerList {
		p = strings.TrimSpace(p)
		if strings.Contains(p, "\n") {
			// Inner param is multiline - needs breaking for readability
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
		if width.VisualLenWithTab(testLine, f.cfg.TabStop) > f.cfg.ColumnLimit {
			needsBreaking = true
		}
	}

	if !needsBreaking {
		return param // Fits on one line and no multiline content
	}

	// Need to break the inner function params
	// Inner params use one tab indent (same as outer signature continuation)
	innerIndent := "\t"
	closingIndent := ""

	var result strings.Builder
	result.WriteString(prefix)
	result.WriteString("(\n")

	// Put each param on its own line with trailing comma
	for _, p := range innerList {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		result.WriteString(innerIndent)
		result.WriteString(p)
		result.WriteString(",\n")
	}

	result.WriteString(closingIndent)
	result.WriteString(")")
	result.WriteString(afterParams)

	return result.String()
}

// formatInterfaceMethod formats a long interface method declaration.
// Returns the formatted string and number of source lines consumed.
func (f *FuncSigFormatter) formatInterfaceMethod(lines [][]byte, startIdx int, indent string) (string, int) {
	// Collect the complete method signature (might span multiple lines)
	var sigBuilder strings.Builder
	linesConsumed := 0
	parenDepth := 0
	inString := false
	escaped := false
	complete := false

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
				// Check if we're done (paren depth back to 0 and this is the return parens)
				if parenDepth == 0 {
					// Look ahead to see if there's more after this
					rest := strings.TrimSpace(line[i+1:])
					if rest == "" || rest[0] == '}' {
						// End of method signature
						sigBuilder.WriteString(strings.TrimSpace(line))
						complete = true
						break
					}
				}
			}
		}

		if complete {
			break
		}

		// Add this line to signature (normalize whitespace)
		if sigBuilder.Len() > 0 {
			sigBuilder.WriteByte(' ')
		}
		sigBuilder.WriteString(strings.TrimSpace(line))
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

// breakInterfaceMethod breaks an interface method to fit within column limit.
func (f *FuncSigFormatter) breakInterfaceMethod(sig, indent string) string {
	// Check if it already fits
	if width.VisualLenWithTab(indent+sig, f.cfg.TabStop) <= f.cfg.ColumnLimit {
		return indent + sig
	}

	var result strings.Builder
	result.WriteString(indent)

	// Parse: MethodName(params) [(returns)]
	parenIdx := strings.Index(sig, "(")
	if parenIdx == -1 {
		return indent + sig
	}

	methodName := sig[:parenIdx]
	rest := sig[parenIdx:]

	// Find matching close paren for params
	paramEnd := f.findMatchingParen(rest, 0)
	if paramEnd == -1 {
		return indent + sig
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

	// Calculate trailing for last param - use minimal ") (" if there are returns
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

		if width.VisualLenWithTab(lineWithTrailing, f.cfg.TabStop) > f.cfg.ColumnLimit {
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
			// Check if returns fit on current line (after the closing paren we just wrote)
			testLine := currentLine + ") " + returns
			if width.VisualLenWithTab(testLine, f.cfg.TabStop) > f.cfg.ColumnLimit {
				// Returns don't all fit inline - use left-flow to pack
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
					// For last return, account for closing paren
					isLastRet := i == len(retList)-1
					testCheck := currentLine + testAdd
					if isLastRet {
						testCheck += ")"
					}

					if width.VisualLenWithTab(testCheck, f.cfg.TabStop) > f.cfg.ColumnLimit {
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

	return result.String()
}

// FormatFuncSignature formats a function signature (the line starting with "func"
// up to and including the opening brace) using left-flow packing. This function
// is exported for use by the DSL engine.
// The signature parameter should be the complete func line ending with "{".
// Returns the formatted signature with a trailing newline if multi-line, and
// a boolean indicating if a blank line should be added after the opening brace.
func FormatFuncSignature(signature, indent string, colLimit, tabStop int) (string, bool) {
	f := &FuncSigFormatter{cfg: FuncSigConfig{
		ColumnLimit: colLimit,
		TabStop:     tabStop,
	}}

	formatted := f.breakSignature(signature, indent)
	isMultiLine := strings.Count(formatted, "\n") > 0

	return formatted, isMultiLine
}

// FormatInterfaceMethod formats an interface method declaration using left-flow
// packing. This function is exported for use by the DSL engine.
func FormatInterfaceMethod(method, indent string, colLimit, tabStop int) string {
	f := &FuncSigFormatter{cfg: FuncSigConfig{
		ColumnLimit: colLimit,
		TabStop:     tabStop,
	}}

	return f.breakInterfaceMethod(method, indent)
}
