package formatter

import (
	"strings"

	"github.com/lightninglabs/llformat/scanner"
)

// FormatCompositeLiteralArg detects a top-level composite literal in arg and
// formats its elements across multiple lines, indented under contIndent. It
// returns the formatted string and true if a composite literal was reformatted.
// If forceExpand is true, keyed maps/structs will always be expanded even if
// they fit inline. This is used when the containing call already has multiline
// elements.
func FormatCompositeLiteralArg(arg, contIndent string, forceExpand ...bool) (string, bool) {
	// forceExpand is reserved for future use to force-expand keyed maps.
	// Currently keyed maps/structs are always expanded, so this is a no-op.
	_ = forceExpand
	// We rely on brace scan and a simple guard to avoid function literals.
	open, close := findTopLevelBraces(arg)
	if open < 0 || close <= open {
		return "", false
	}
	before := strings.TrimSpace(arg[:open])
	inside := arg[open+1 : close]
	after := strings.TrimSpace(arg[close+1:])
	if after != "" {
		// Avoid changing complex expressions that append suffixes after the literal.
		return "", false
	}
	// Heuristic: if this looks like a function literal (prefix contains "func"),
	// do not treat as a composite literal.
	if strings.Contains(before, "func") {
		return "", false
	}
	elems := scanner.SplitTopLevelAny(inside)
	// Detect if entries look like key/value (map or struct) vs plain elems (slice/array).
	isKV := false
	for _, e := range elems {
		if strings.Contains(e, ":") { // heuristic, good enough for our use
			isKV = true
			break
		}
	}

	var b strings.Builder
	b.WriteString(before)
	b.WriteString("{")
	b.WriteByte('\n')
	innerIndent := contIndent + "\t"
	if isKV {
		// Keyed literals (map/struct): one entry per line with trailing comma.
		for _, e := range elems {
			t := strings.TrimSpace(e)
			if t == "" {
				continue
			}
			b.WriteString(innerIndent)
			b.WriteString(t)
			b.WriteByte(',')
			b.WriteByte('\n')
		}
	} else {
		// Slices/arrays: keep inline if they fit; otherwise pack greedily.
		// Note: Unlike keyed maps/structs, we don't force-expand slices even
		// when the containing call has multiline elements - slices stay inline
		// if they fit.
		inline := strings.TrimSpace(arg)
		if visualLen(inline)+visualLen(contIndent) <= columnLimit {
			return "", false
		}
		lineLen := visualLen(innerIndent)
		firstInLine := true
		for i, e := range elems {
			t := strings.TrimSpace(e)
			if t == "" {
				continue
			}
			if firstInLine {
				b.WriteString(innerIndent)
				b.WriteString(t)
				lineLen += firstLineLen(t)
				firstInLine = false
			} else {
				need := 2 + firstLineLen(t) // ", " + elem
				if lineLen+need <= columnLimit {
					b.WriteString(", ")
					b.WriteString(t)
					lineLen += need
				} else {
					// line break, add trailing comma to previous line
					b.WriteByte(',')
					b.WriteByte('\n')
					b.WriteString(innerIndent)
					b.WriteString(t)
					lineLen = visualLen(innerIndent) + firstLineLen(t)
				}
			}
			// Add trailing comma after last element
			if i == len(elems)-1 {
				b.WriteByte(',')
				b.WriteByte('\n')
			}
		}
		// If we didn't break at all, ensure newline at end
		if firstInLine { // no elements
			b.WriteByte('\n')
		}
	}
	b.WriteString(contIndent)
	b.WriteByte('}')
	return b.String(), true
}

// findTopLevelBraces finds the first top-level '{' and its matching '}' in s,
// skipping over strings and nested brackets/parens/braces.
func findTopLevelBraces(s string) (int, int) {
	inStr := byte(0)
	esc := false
	paren := 0
	bracket := 0
	brace := 0
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
		// Skip comments
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
		switch c {
		case '"', '`':
			inStr = c
		case '(':
			paren++
		case ')':
			if paren > 0 {
				paren--
			}
		case '[':
			bracket++
		case ']':
			if bracket > 0 {
				bracket--
			}
		case '{':
			if paren == 0 && bracket == 0 && brace == 0 {
				// Skip interface type bodies: `interface{ ... }`
				if isInterfaceBefore(s, i) {
					// Let the outer scan continue; this '{' is not a composite literal.
					break
				}
				// candidate start
				start := i
				brace = 1
				i++
				for i < len(s) {
					c2 := s[i]
					if inStr != 0 {
						if inStr == '"' && c2 == '\\' && !esc {
							esc = true
							i++
							continue
						}
						if esc {
							esc = false
						} else if c2 == inStr {
							inStr = 0
						}
						i++
						continue
					}
					if c2 == '"' || c2 == '`' {
						inStr = c2
						i++
						continue
					}
					if c2 == '/' && i+1 < len(s) {
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
					switch c2 {
					case '{':
						brace++
					case '}':
						brace--
						if brace == 0 {
							return start, i
						}
					case '(':
						paren++
					case ')':
						if paren > 0 {
							paren--
						}
					case '[':
						bracket++
					case ']':
						if bracket > 0 {
							bracket--
						}
					}
					i++
				}
				return -1, -1
			}
		}
	}
	return -1, -1
}

// isInterfaceBefore reports whether the word before index i is the `interface` keyword.
func isInterfaceBefore(s string, i int) bool {
	j := i - 1
	// Skip whitespace
	for j >= 0 && (s[j] == ' ' || s[j] == '\t' || s[j] == '\n') {
		j--
	}
	// Scan back an identifier
	end := j + 1
	for j >= 0 && ((s[j] >= 'a' && s[j] <= 'z') || (s[j] >= 'A' && s[j] <= 'Z')) {
		j--
	}
	word := s[j+1 : end]
	return word == "interface"
}

