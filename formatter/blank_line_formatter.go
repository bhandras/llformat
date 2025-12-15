package formatter

import (
	"bytes"
	"strings"
)

// BlankLineConfig holds configuration for the blank line formatter.
type BlankLineConfig struct {
	// BeforeReturn adds a blank line before return statements.
	BeforeReturn bool
	// BetweenCases adds a blank line between case/default clauses in
	// switch.
	BetweenCases bool
	// BetweenInterfaceMethods adds a blank line between interface method
	// declarations.
	BetweenInterfaceMethods bool
}

// BlankLineFormatter ensures consistent blank lines in specific locations.
type BlankLineFormatter struct {
	cfg BlankLineConfig
}

// NewBlankLineFormatter creates a new blank line formatter with defaults.
func NewBlankLineFormatter(cfg BlankLineConfig) *BlankLineFormatter {
	return &BlankLineFormatter{cfg: cfg}
}

// FormatFile adds blank lines where configured.
func (f *BlankLineFormatter) FormatFile(src []byte) []byte {
	lines := bytes.Split(src, []byte("\n"))
	var out bytes.Buffer

	// Track context
	inSwitch := 0      // nesting depth of switch statements
	inInterface := 0   // nesting depth of interface blocks
	prevLineType := "" // "case", "return", "interface_method", "other", "blank"

	for i, line := range lines {
		lineStr := string(line)
		trimmed := strings.TrimSpace(lineStr)

		// Determine line type
		lineType := f.classifyLine(trimmed, inSwitch > 0, inInterface >
			0)

		// Track switch nesting
		if strings.HasPrefix(trimmed, "switch ") ||
			trimmed == "switch" ||
			strings.HasPrefix(trimmed, "switch{") {
			inSwitch++
		}

		// Track interface nesting
		if strings.Contains(trimmed, "interface") &&
			strings.HasSuffix(trimmed, "{") {
			inInterface++
		}

		// Track closing braces
		if trimmed == "}" {
			// Check what we're closing based on context This is
			// approximate - we decrement the innermost open block
			if inInterface > 0 && f.isClosingInterface(lines, i) {
				inInterface--
			} else if inSwitch > 0 {
				inSwitch--
			}
		}

		// Determine if we need to add a blank line before this line
		needsBlankBefore := false

		if f.cfg.BeforeReturn && lineType == "return" {
			// Add blank before return if previous line is not blank
			// and not opening brace/block
			if prevLineType != "blank" &&
				prevLineType != "open_brace" &&
				prevLineType != "block_open" &&
				prevLineType != "case" {
				needsBlankBefore = true
			}
		}

		if f.cfg.BetweenCases && lineType == "case" {
			// Add blank before case/default if previous line is not
			// blank and we're not at the first case (previous would
			// be switch line or open brace)
			if prevLineType != "blank" &&
				prevLineType != "open_brace" &&
				prevLineType != "switch" {
				needsBlankBefore = true
			}
		}

		if f.cfg.BetweenInterfaceMethods &&
			lineType == "interface_method" {
			// Add blank before interface method if previous line is
			// not blank and not the opening of interface But DO add
			// blank after embedded interfaces (transition from
			// embeds to methods)
			if prevLineType == "embedded_interface" {
				needsBlankBefore = true
			} else if prevLineType != "blank" &&
				prevLineType != "open_brace" &&
				prevLineType != "interface_open" {
				needsBlankBefore = true
			}
		}

		// Write blank line if needed
		if needsBlankBefore {

			out.WriteByte('\n')
		}

		// Write the line

		out.Write(line)
		if i < len(lines)-1 {

			out.WriteByte('\n')
		}

		// Update previous line type
		if trimmed == "" {
			prevLineType = "blank"
		} else {
			prevLineType = lineType
		}
	}

	return out.Bytes()
}

// FormatBlankLinesInSource applies the legacy blank-line formatter to src and
// reports whether it changed anything.
//
// This is exported so DSL stages can delegate without creating an import cycle.
func FormatBlankLinesInSource(src []byte) ([]byte, bool) {
	f := NewBlankLineFormatter(BlankLineConfig{
		BeforeReturn:            true,
		BetweenCases:            true,
		BetweenInterfaceMethods: true,
	})
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

// classifyLine determines the type of a line for blank line insertion logic.
func (f *BlankLineFormatter) classifyLine(trimmed string, inSwitch,
	inInterface bool) string {

	if trimmed == "" {
		return "blank"
	}

	if trimmed == "{" {
		return "open_brace"
	}

	if trimmed == "}" {
		return "close_brace"
	}

	// Switch-related
	if strings.HasPrefix(trimmed, "switch ") || trimmed == "switch" ||
		strings.HasPrefix(trimmed, "switch{") {
		return "switch"
	}

	if inSwitch && (strings.HasPrefix(trimmed, "case ") ||
		trimmed == "default:" ||
		strings.HasPrefix(trimmed, "default:")) {
		return "case"
	}

	// Return statements
	if strings.HasPrefix(trimmed, "return") {
		// Make sure it's actually a return statement, not a variable
		// named returnXxx
		rest := strings.TrimPrefix(trimmed, "return")
		if rest == "" || rest[0] == ' ' || rest[0] == '\t' ||
			rest[0] == '(' {
			return "return"
		}
	}

	// Interface-related
	if strings.Contains(trimmed, "interface") &&
		strings.HasSuffix(trimmed, "{") {
		return "interface_open"
	}

	// Embedded interface (like io.Reader)
	if inInterface && !strings.Contains(trimmed, "(") &&
		!strings.HasSuffix(trimmed, "{") &&
		trimmed != "}" {
		// Could be embedded interface - check if it looks like a type
		// reference Embedded interfaces don't have parens
		if !strings.Contains(trimmed, "(") &&
			(strings.Contains(trimmed, ".") ||
				(len(trimmed) > 0 && trimmed[0] >= 'A' &&
					trimmed[0] <= 'Z')) {
			return "embedded_interface"
		}
	}

	// Interface method declaration Must start with an identifier (uppercase
	// letter) followed by ( Continuation lines typically start with
	// lowercase params or special chars
	if inInterface && strings.Contains(trimmed, "(") &&
		!strings.HasSuffix(trimmed, "{") {
		// Check if this looks like a method declaration start
		// (Identifier followed by paren) vs a continuation line (param
		// name, type, etc.)
		parenIdx := strings.Index(trimmed, "(")
		if parenIdx > 0 {
			beforeParen := trimmed[:parenIdx]
			// Method names start with uppercase in exported
			// interfaces or lowercase for unexported - but must be
			// a simple identifier Continuation lines often have
			// things like "param Type" before (
			if !strings.Contains(beforeParen, " ") &&
				len(beforeParen) > 0 {
				firstChar := beforeParen[0]
				if (firstChar >= 'A' && firstChar <= 'Z') ||
					(firstChar >= 'a' && firstChar <= 'z') {
					return "interface_method"
				}
			}
		}
	}

	// Lines ending with { are block openers (if, for, func, etc.) These
	// should be treated like open braces for return formatting
	if strings.HasSuffix(trimmed, "{") {
		return "block_open"
	}

	return "other"
}

// isClosingInterface checks if the closing brace at line i is closing an
// interface. This is a heuristic based on looking back for the interface
// keyword.
func (f *BlankLineFormatter) isClosingInterface(lines [][]byte, i int) bool {
	braceCount := 0
	for j := i; j >= 0; j-- {
		line := string(lines[j])
		trimmed := strings.TrimSpace(line)

		// Count braces
		for _, c := range trimmed {
			if c == '}' {
				braceCount++
			} else if c == '{' {
				braceCount--
				if braceCount == 0 {
					// Found the matching open brace

					return strings.Contains(
						trimmed,
						"interface",
					)
				}
			}
		}
	}

	return false
}
