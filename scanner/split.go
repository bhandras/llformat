package scanner

import "strings"

// SplitTopLevel splits a string by commas at depth 0, respecting parentheses only.
// Used for function argument lists where brackets and braces are not relevant.
func SplitTopLevel(s string) []string {
	return splitAtDepthZero(s, false)
}

// SplitTopLevelAny splits by commas at depth 0, respecting (), [], and {}.
// Used when arguments may contain composite literals or array indexing.
func SplitTopLevelAny(s string) []string {
	return splitAtDepthZero(s, true)
}

// splitAtDepthZero is the shared implementation for both split functions.
func splitAtDepthZero(s string, allBrackets bool) []string {
	var parts []string
	b := []byte(s)
	start := 0
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0

	for i := 0; i < len(b); {
		switch {
		case IsStringStart(b, i):
			i = ScanString(b, i)
		case IsLineCommentStart(b, i):
			i = ScanLineComment(b, i)
		case IsBlockCommentStart(b, i):
			i = ScanBlockComment(b, i)
		case b[i] == '(':
			parenDepth++
			i++
		case b[i] == ')':
			parenDepth--
			i++
		case allBrackets && b[i] == '[':
			bracketDepth++
			i++
		case allBrackets && b[i] == ']':
			bracketDepth--
			i++
		case allBrackets && b[i] == '{':
			braceDepth++
			i++
		case allBrackets && b[i] == '}':
			braceDepth--
			i++
		case b[i] == ',':
			atDepthZero := parenDepth == 0
			if allBrackets {
				atDepthZero = atDepthZero && bracketDepth == 0 && braceDepth == 0
			}
			if atDepthZero {
				parts = append(parts, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
			i++
		default:
			i++
		}
	}
	// Add the final part if non-empty
	if tail := strings.TrimSpace(s[start:]); tail != "" {
		parts = append(parts, tail)
	}
	return parts
}
