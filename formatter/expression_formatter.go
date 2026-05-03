package formatter

import (
	"strings"

	"github.com/bhandras/llformat/scanner"
)

// FormatCompositeLiteralArg detects a top-level composite literal in arg and
// formats its elements across multiple lines, indented under contIndent. It
// returns the formatted string and true if a composite literal was reformatted.
// If forceExpand is true, keyed maps/structs will always be expanded even if
// they fit inline. This is used when the containing call already has multiline
// elements.
func FormatCompositeLiteralArg(arg, contIndent string,
	forceExpand ...bool) (string, bool) {

	// Keep the public force flag reserved for keyed composite behavior.
	// Slice literals stay inline when they fit unless a keyed field value
	// is being repaired internally.
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

		// Avoid changing complex expressions that append suffixes after
		// the literal.
		return "", false
	}
	// Heuristic: if this looks like a function literal (prefix contains
	// "func"), do not treat as a composite literal.
	if strings.Contains(before, "func") {
		return "", false
	}
	elems := scanner.SplitTopLevelAny(inside)
	// Detect if entries look like key/value (map or struct) vs plain elems
	// (slice/array).
	isKV := isKeyedCompositeLiteral(elems)

	if isKV {
		return formatKeyedCompositeLiteral(before, contIndent, elems), true
	}

	return formatSliceCompositeLiteral(
		arg, before, contIndent, elems, false,
	)
}

func isKeyedCompositeLiteral(elems []string) bool {
	for _, e := range elems {
		if strings.Contains(e, ":") { // heuristic, good enough for our use

			return true
		}
	}

	return false
}

func formatKeyedCompositeLiteral(before, contIndent string,
	elems []string) string {

	var b strings.Builder
	b.WriteString(before)
	b.WriteString("{")
	b.WriteByte('\n')
	innerIndent := contIndent + "\t"
	alignKeyLen := maxTopLevelKeyLen(elems)
	if shouldOutdentCompositeElems(
		contIndent, innerIndent, elems, alignKeyLen,
	) {

		innerIndent = contIndent
	}

	// Keyed literals (map/struct): one entry per line with trailing comma.
	for _, e := range elems {
		t := strings.TrimSpace(e)
		if t == "" {
			continue
		}
		t = formatKeyedCompositeElem(t, innerIndent, alignKeyLen)
		b.WriteString(innerIndent)
		b.WriteString(t)
		b.WriteByte(',')
		b.WriteByte('\n')
	}

	b.WriteString(contIndent)
	b.WriteByte('}')

	return b.String()
}

func formatKeyedCompositeElem(elem, elemIndent string, alignKeyLen int) string {
	elemWidth := keyedCompositeElemWidthWithAlign(
		elem, elemIndent, alignKeyLen,
	)
	if elemWidth <= columnLimit {
		return elem
	}

	key, value, ok := splitTopLevelKeyValue(elem)
	if !ok || value == "" {
		return elem
	}
	formatted, ok := formatCompositeLiteralValue(value, elemIndent)
	if !ok {
		return elem
	}
	candidate := key + " " + formatted
	if maxLineLenWithIndentAndComma(candidate, elemIndent) >= elemWidth {

		return elem
	}

	return candidate
}

func maxTopLevelKeyLen(elems []string) int {
	maxLen := 0
	for _, elem := range elems {
		key, _, ok := splitTopLevelKeyValue(strings.TrimSpace(elem))
		if !ok {
			continue
		}
		if keyLen := visualLen(key); keyLen > maxLen {
			maxLen = keyLen
		}
	}

	return maxLen
}

func keyedCompositeElemWidthWithAlign(elem, elemIndent string,
	alignKeyLen int) int {

	key, value, ok := splitTopLevelKeyValue(elem)
	if !ok || value == "" || strings.Contains(elem, "\n") {
		return maxLineLenWithIndentAndComma(elem, elemIndent)
	}

	keyLen := visualLen(key)
	prefixLen := keyLen + 1
	if alignKeyLen > keyLen {
		prefixLen = alignKeyLen + 1
	}

	return visualLen(elemIndent) + prefixLen + firstLineLen(value) + 1
}

func formatCompositeLiteralValue(value, elemIndent string) (string, bool) {
	open, close := findTopLevelBraces(value)
	if open < 0 || close <= open {
		return "", false
	}
	before := strings.TrimSpace(value[:open])
	inside := value[open+1 : close]
	after := strings.TrimSpace(value[close+1:])
	if after != "" || strings.Contains(before, "func") {
		return "", false
	}

	elems := scanner.SplitTopLevelAny(inside)
	if isKeyedCompositeLiteral(elems) {
		return formatKeyedCompositeLiteral(before, elemIndent, elems), true
	}

	return formatSliceCompositeLiteral(
		value, before, elemIndent, elems, true,
	)
}

func splitTopLevelKeyValue(elem string) (string, string, bool) {
	inStr := byte(0)
	esc := false
	paren := 0
	bracket := 0
	brace := 0
	for i := 0; i < len(elem); i++ {
		c := elem[i]
		if inStr != 0 {
			inStr, esc = updateStringState(c, inStr, esc)
			continue
		}
		if next, ok := skipComment(elem, i); ok {
			i = next - 1
			continue
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
			brace++

		case '}':
			if brace > 0 {
				brace--
			}

		case ':':
			if paren == 0 && bracket == 0 && brace == 0 {
				key := strings.TrimSpace(elem[:i+1])
				value := strings.TrimSpace(elem[i+1:])

				return key, value, key != ""
			}
		}
	}

	return "", "", false
}

func shouldOutdentCompositeElems(contIndent, innerIndent string, elems []string,
	alignKeyLen int) bool {

	if visualLen(contIndent) >= visualLen(innerIndent) {
		return false
	}
	maxInner := 0
	maxOutdent := 0
	for _, e := range elems {
		t := strings.TrimSpace(e)
		if t == "" {
			continue
		}
		innerElem := formatKeyedCompositeElem(
			t, innerIndent, alignKeyLen,
		)
		if l := maxLineLenWithIndentAndComma(
			innerElem, innerIndent,
		); l > maxInner {

			maxInner = l
		}
		if l := maxLineLenWithIndentAndComma(t, contIndent); l > maxOutdent {
			maxOutdent = l
		}
	}

	return maxInner > columnLimit && maxOutdent <= columnLimit
}

func maxLineLenWithIndentAndComma(elem, indent string) int {
	lines := strings.Split(elem, "\n")
	if len(lines) == 0 {
		return visualLen(indent) + 1
	}
	maxLen := 0
	for i, line := range lines {
		lineLen := visualLen(line)
		if i == 0 {
			lineLen += visualLen(indent)
		}
		if i == len(lines)-1 {
			lineLen++
		}
		if lineLen > maxLen {
			maxLen = lineLen
		}
	}

	return maxLen
}

func formatSliceCompositeLiteral(arg, before, contIndent string, elems []string,
	forceExpand bool) (string, bool) {

	// Slices/arrays: keep inline if they fit; otherwise pack greedily.
	// Note: Unlike keyed maps/structs, slices stay inline if they fit
	// unless the caller already proved an enclosing keyed element needs the
	// value broken out to stay within the column limit.
	inline := strings.TrimSpace(arg)
	if !forceExpand &&
		visualLen(inline)+visualLen(contIndent) <= columnLimit {

		return "", false
	}

	var b strings.Builder
	b.WriteString(before)
	b.WriteString("{")
	b.WriteByte('\n')
	innerIndent := contIndent + "\t"

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
				// line break, add trailing comma to previous
				// line.
				b.WriteByte(',')
				b.WriteByte('\n')
				b.WriteString(innerIndent)
				b.WriteString(t)
				lineLen = visualLen(innerIndent) +
					firstLineLen(t)
			}
		}
		// Add trailing comma after last element.
		if i == len(elems)-1 {
			b.WriteByte(',')
			b.WriteByte('\n')
		}
	}
	// If we didn't break at all, ensure newline at end.
	if firstInLine { // no elements
		b.WriteByte('\n')
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
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr != 0 {
			inStr, esc = updateStringState(c, inStr, esc)
			continue
		}
		if next, ok := skipComment(s, i); ok {
			i = next - 1
			continue
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
			if paren != 0 || bracket != 0 {
				break
			}
			// Skip interface type bodies: `interface{ ... }`.
			if isInterfaceBefore(s, i) {
				break
			}
			if end := scanCompositeLiteralClose(s, i); end > i {
				return i, end
			}
		}
	}

	return -1, -1
}

func scanCompositeLiteralClose(s string, start int) int {
	inStr := byte(0)
	esc := false
	paren := 0
	bracket := 0
	brace := 1
	for i := start + 1; i < len(s); i++ {
		c := s[i]
		if inStr != 0 {
			inStr, esc = updateStringState(c, inStr, esc)
			continue
		}
		if next, ok := skipComment(s, i); ok {
			i = next - 1
			continue
		}
		switch c {
		case '"', '`':
			inStr = c

		case '{':
			brace++

		case '}':
			brace--
			if brace == 0 {
				return i
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
	}

	return -1
}

func skipComment(s string, i int) (int, bool) {
	if s[i] != '/' || i+1 >= len(s) {
		return 0, false
	}
	if s[i+1] == '/' {
		j := i + 2
		for j < len(s) && s[j] != '\n' {
			j++
		}

		return j, true
	}
	if s[i+1] == '*' {
		j := i + 2
		for j+1 < len(s) {
			if s[j] == '*' && s[j+1] == '/' {
				return j + 2, true
			}
			j++
		}

		return len(s), true
	}

	return 0, false
}

func updateStringState(c, inStr byte, esc bool) (byte, bool) {
	if inStr == '"' && c == '\\' && !esc {
		return inStr, true
	}
	if esc {
		return inStr, false
	}
	if c == inStr {
		return 0, false
	}

	return inStr, false
}

// isInterfaceBefore reports whether the word before index i is the `interface`
// keyword.
func isInterfaceBefore(s string, i int) bool {
	j := i - 1
	// Skip whitespace
	for j >= 0 && (s[j] == ' ' || s[j] == '\t' || s[j] == '\n') {
		j--
	}
	// Scan back an identifier
	end := j + 1
	for j >= 0 &&
		((s[j] >= 'a' && s[j] <= 'z') || (s[j] >= 'A' && s[j] <= 'Z')) {

		j--
	}
	word := s[j+1 : end]

	return word == "interface"
}
