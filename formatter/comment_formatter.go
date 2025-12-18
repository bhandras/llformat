package formatter

import (
	"strings"
	"unicode"
)

// CommentConfig holds configuration for comment reflowing.
type CommentConfig struct {
	ColumnLimit int
	TabStop     int
	// MoveInlineAbove hoists trailing inline comments (// and single-line
	// /* */) above the code line as standalone comment lines for reflowing.
	MoveInlineAbove bool
}

// CommentFormatter reflows standalone comment blocks greedily.
//
// Rules (summary):
//   - Only format pure comment lines: lines that begin with "//" after optional
//     indentation, or standalone block comments that begin with "/*" on their
//     own line and end with "*/" on their own line. Trailing comments after
//     code are left intact.
//   - Preserve indentation. Normalize markers:
//   - Line comments: non-empty lines as "// ", empty lines as "//".
//   - Block comments: keep opening "/*" and closing "*/" lines intact; interior
//     lines emit as " * " for non-empty, " *" for empty.
//   - Preserve empty lines within a comment block as paragraph breaks.
//   - Lists ("- ") inside comments are reflowed as items: first line gets "- ",
//     continuation lines align with two spaces instead of the dash.
//   - Greedy reflow by words; no hyphenation. A single word longer than the
//     available width is placed on its own line.
type CommentFormatter struct{ cfg CommentConfig }

// NewCommentFormatter creates a new comment formatter with defaults.
func NewCommentFormatter(cfg CommentConfig) *CommentFormatter {
	if cfg.ColumnLimit <= 0 {
		cfg.ColumnLimit = 80
	}
	if cfg.TabStop <= 0 {
		cfg.TabStop = 8
	}
	return &CommentFormatter{cfg: cfg}
}

// FormatFile implements greedy reflowing of comment-only lines.
func (f *CommentFormatter) FormatFile(src []byte) []byte {
	// Use package-level width helpers
	if f.cfg.ColumnLimit > 0 {
		columnLimit = f.cfg.ColumnLimit
	}
	if f.cfg.TabStop > 0 {
		tabStop = f.cfg.TabStop
	}

	// Optional pre-pass: hoist inline comments above their line.
	if f.cfg.MoveInlineAbove {
		src = hoistInlineComments(src)
	}

	lines := splitLines(string(src))
	var out []string

	i := 0
	for i < len(lines) {
		line := lines[i]
		// Try line comment block
		if isStandaloneLineComment(line) {
			// Preserve directive comments verbatim. These lines are not
			// "text" in the normal sense; tools expect specific formats.
			if isDirectiveLineComment(line) {
				out = append(out, line)
				i++
				continue
			}

			indent, _ := splitIndent(line)
			// Collect consecutive standalone `//` lines.
			start := i
			for i < len(lines) && isStandaloneLineComment(lines[i]) && !isDirectiveLineComment(lines[i]) {
				i++
			}
			block := lines[start:i]
			out = append(out, reflowLineCommentBlock(block, indent)...)
			continue
		}

		// Try standalone block comment
		if isStandaloneBlockStart(line) {
			indent, _ := splitIndent(line)
			// Find end line.
			j := i + 1
			for j < len(lines) && !isStandaloneBlockEnd(lines[j]) {
				j++
			}
			if j < len(lines) && isStandaloneBlockEnd(lines[j]) {
				block := lines[i : j+1]
				// Preserve directive-like blocks verbatim (e.g. cgo directives).
				if isDirectiveBlockComment(block) {
					out = append(out, block...)
					i = j + 1
					continue
				}
				out = append(out, reflowBlockComment(block, indent)...)
				i = j + 1
				continue
			}
			// Not a complete standalone block; fall through, copy
			// as-is.
		}

		// Default: copy unchanged.
		out = append(out, line)
		i++
	}

	return []byte(strings.Join(out, "\n"))
}

// FormatCommentsInSource applies the legacy comment formatter to src and reports
// whether it changed anything.
//
// This is exported so DSL stages can delegate to the legacy implementation
// without creating an import cycle.
func FormatCommentsInSource(src []byte, colLimit, tabStop int, moveInlineAbove bool) ([]byte, bool) {
	f := NewCommentFormatter(CommentConfig{
		ColumnLimit:     colLimit,
		TabStop:         tabStop,
		MoveInlineAbove: moveInlineAbove,
	})
	out := f.FormatFile(src)
	if bytesEqual(out, src) {
		return nil, false
	}
	return out, true
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// splitLines preserves all lines without dropping trailing empty line info.
func splitLines(s string) []string {
	// Normalize to raw lines without retaining trailing newline sentinel;
	// the go/format pass later (if any) can normalize final newline. Here
	// we keep behavior consistent with our other formatters which operate
	// on bytes.
	if s == "" {
		return []string{""}
	}
	// strings.Split keeps a trailing empty element if s ends with '\n'.
	return strings.Split(s, "\n")
}

func splitIndent(s string) (indent, rest string) {
	i := 0
	for i < len(s) {
		if s[i] == ' ' || s[i] == '\t' {
			i++
			continue
		}
		break
	}
	return s[:i], s[i:]
}

func isStandaloneLineComment(s string) bool {
	indent, rest := splitIndent(s)
	_ = indent
	return strings.HasPrefix(rest, "//")
}

// isDirectiveLineComment reports whether s is a line comment that should be
// preserved verbatim because it encodes a tool/build directive.
//
// This intentionally errs on the conservative side: it's better to leave a
// directive-looking comment unchanged than to wrap/hoist it and break tools.
func isDirectiveLineComment(s string) bool {
	_, rest := splitIndent(s)
	if !strings.HasPrefix(rest, "//") {
		return false
	}

	// Go toolchain directives must have no space between `//` and `go:`.
	if strings.HasPrefix(rest, "//go:") {
		return true
	}

	// Build tags are typically `// +build ...` but be tolerant of `//+build ...`.
	if strings.HasPrefix(rest, "// +build") || strings.HasPrefix(rest, "//+build") {
		return true
	}

	// `//line` directives must have no space between `//` and `line`.
	// Avoid treating ordinary comment prose like "the next line ..." as a directive.
	if strings.HasPrefix(rest, "//line") {
		return true
	}

	// cgo `//export` directives must have no space between `//` and `export`.
	// Reflowing (e.g. turning it into `// export`) can break cgo.
	if strings.HasPrefix(rest, "//export") {
		if len(rest) == len("//export") {
			return true
		}
		switch rest[len("//export")] {
		case ' ', '\t':
			return true
		}
	}

	// Linter directives are commonly written without a space, but some linters
	// also accept a space. Keep this conservative but avoid matching arbitrary
	// prose: require the directive token to be the first non-space after `//`.
	text := strings.TrimPrefix(rest, "//")
	text = strings.TrimLeft(text, " \t")
	switch {
	case strings.HasPrefix(text, "nolint"):
		return true
	case strings.HasPrefix(text, "lint:"):
		return true
	case strings.HasPrefix(text, "revive:"):
		return true
	case strings.HasPrefix(text, "staticcheck:"):
		return true
	case strings.HasPrefix(text, "gosec:"):
		return true
	default:
		return false
	}
}

func isStandaloneBlockStart(s string) bool {
	_, rest := splitIndent(s)
	return strings.HasPrefix(rest, "/*") && !strings.Contains(rest, "*/")
}

func isStandaloneBlockEnd(s string) bool {
	_, rest := splitIndent(s)
	return strings.HasPrefix(strings.TrimSpace(rest), "*/")
}

func blockCommentLineText(ln string) string {
	// Normalize a block comment interior line to its "payload" text for directive
	// checks: trim indentation and an optional leading `*` prefix.
	_, rest := splitIndent(ln)
	rest = strings.TrimLeft(rest, " ")
	if strings.HasPrefix(rest, "*") {
		rest = strings.TrimPrefix(rest, "*")
		rest = strings.TrimLeft(rest, " ")
	}
	return strings.TrimSpace(rest)
}

// isDirectiveBlockComment reports whether a standalone block comment should be
// preserved verbatim because it likely contains tool directives (e.g. cgo).
func isDirectiveBlockComment(block []string) bool {
	for _, ln := range block {
		text := blockCommentLineText(ln)
		if text == "" {
			continue
		}

		// cgo directives must be preserved exactly. Reflowing or adding `*`-style
		// formatting can break cgo parsing.
		if strings.HasPrefix(text, "#cgo") || strings.HasPrefix(text, "#include") || strings.HasPrefix(text, "#pragma") {
			return true
		}
	}
	return false
}

func trimLineCommentText(s string) (indent, text string, empty bool) {
	indent, rest := splitIndent(s)
	if !strings.HasPrefix(rest, "//") {
		return indent, rest, false
	}
	t := strings.TrimPrefix(rest, "//")
	// Only trim the first space after // if present, preserve internal
	// indentation
	if strings.HasPrefix(t, " ") {
		t = t[1:]
	}
	if strings.TrimSpace(t) == "" {
		return indent, "", true
	}
	return indent, t, false
}

func reflowLineCommentBlock(block []string, indent string) []string {
	// Gather paragraphs and list items.
	var out []string
	i := 0
	for i < len(block) {
		_, t, empty := trimLineCommentText(block[i])
		if empty {
			// Preserve empty comment line
			out = append(out, indent+"//")
			i++
			continue
		}

		// Determine if this paragraph starts with a list item
		isList := strings.HasPrefix(strings.TrimLeft(t, " \t"), "- ")
		// Collect lines for this paragraph (until next empty or next
		// list item when in list mode)
		var texts []string
		for i < len(block) {
			_, tt, e2 := trimLineCommentText(block[i])
			if e2 {
				break
			}
			if isList {
				if len(texts) > 0 && strings.HasPrefix(strings.TrimLeft(tt, " \t"), "- ") {
					break
				}
			} else {
				if strings.HasPrefix(strings.TrimLeft(tt, " \t"), "- ") {
					// Start of a new list section; end
					// paragraph
					break
				}
			}
			texts = append(texts, tt)
			i++
		}
		// Reflow texts
		if isList {
			// Each item is its own paragraph. The first collected
			// line begins with "- ". Split the first item's marker
			// from text.
			items := collectDashItems(texts)
			for _, item := range items {
				if len(item) > 0 {
					// Extract the original indentation from
					// the first item
					firstLine := item[0]
					leadingSpaces := ""
					for _, r := range firstLine {
						if r == ' ' || r == '\t' {
							leadingSpaces += string(r)
						} else {
							break
						}
					}
					// Create prefixes with the original
					// indentation preserved
					firstPrefix := indent + "//" + leadingSpaces + " - "
					contPrefix := indent + "//" + leadingSpaces + "   "
					out = append(out, wrapWithPrefixes(firstPrefix, contPrefix, item)...)
				}
			}
		} else {
			out = append(out, wrapWithPrefixes(indent+"// ", indent+"// ", strings.Join(texts, " "))...)
		}
		// If we stopped due to an empty line, it will be handled at top
		// of loop.
	}
	return out
}

func collectDashItems(lines []string) [][]string {
	var items [][]string
	var cur []string
	for _, ln := range lines {
		trimmed := strings.TrimLeft(ln, " \t")
		if strings.HasPrefix(trimmed, "- ") {
			if len(cur) > 0 {
				items = append(items, cur)
			}
			// Preserve the original indentation but remove the dash
			leading := ln[:len(ln)-len(trimmed)]
			itemText := strings.TrimPrefix(trimmed, "- ")
			cur = []string{leading + itemText}
		} else {
			if len(cur) == 0 {
				// If malformed (no leading - ), start a new one
				// anyway
				cur = []string{ln}
			} else {
				cur = append(cur, ln)
			}
		}
	}
	if len(cur) > 0 {
		items = append(items, cur)
	}
	return items
}

func reflowBlockComment(block []string, indent string) []string {
	// Keep opening and closing intact; reflow interior.
	if len(block) == 0 {
		return block
	}
	var out []string
	out = append(out, block[0])

	// Collect interior until last line
	interior := block[1:]
	if len(interior) == 0 {
		return out
	}
	// Find closing index
	last := len(interior) - 1
	closing := interior[last]
	interior = interior[:last]

	// Process interior lines Convert each line to text: strip leading
	// optional "*" and a single space. Detect empty lines.
	var lines []string
	for _, ln := range interior {
		_, rest := splitIndent(ln)
		rest = strings.TrimLeft(rest, " ")
		if strings.HasPrefix(rest, "*") {
			rest = strings.TrimPrefix(rest, "*")
			rest = strings.TrimLeft(rest, " ")
		}
		if strings.TrimSpace(rest) == "" {
			lines = append(lines, "")
		} else {
			lines = append(lines, rest)
		}
	}

	// Reflow paragraphs similarly to line comments
	i := 0
	for i < len(lines) {
		t := lines[i]
		if strings.TrimSpace(t) == "" {
			out = append(out, indent+" *")
			i++
			continue
		}
		isList := strings.HasPrefix(strings.TrimLeftFunc(t, unicode.IsSpace), "- ")
		var texts []string
		for i < len(lines) {
			tt := lines[i]
			if strings.TrimSpace(tt) == "" {
				break
			}
			leadTrim := strings.TrimLeftFunc(tt, unicode.IsSpace)
			if isList {
				if len(texts) > 0 && strings.HasPrefix(leadTrim, "- ") {
					break
				}
			} else {
				if strings.HasPrefix(leadTrim, "- ") {
					break
				}
			}
			texts = append(texts, leadTrim)
			i++
		}
		if isList {
			items := collectDashItems(texts)
			for _, item := range items {
				out = append(out, wrapWithPrefixes(indent+" * - ", indent+" *   ", item)...)
			}
		} else {
			out = append(out, wrapWithPrefixes(indent+" * ", indent+" * ", strings.Join(texts, " "))...)
		}
	}

	out = append(out, closing)
	return out
}

// wrapWithPrefixes greedily wraps text into lines using firstPrefix for the
// first line and contPrefix for subsequent lines. The text may be provided as a
// string or a slice of lines (joined with spaces).
func wrapWithPrefixes(firstPrefix, contPrefix string, text interface{}) []string {
	var s string
	switch v := text.(type) {
	case string:
		s = v
	case []string:
		s = strings.Join(v, " ")
	default:
		s = ""
	}
	words := splitWords(s)
	if len(words) == 0 {
		return []string{strings.TrimRight(firstPrefix, " ")}
	}
	lines := make([]string, 0)
	prefix := firstPrefix
	cur := prefix
	curLen := visualLen(prefix)
	avail := columnLimit
	for i, w := range words {
		// Determine needed width including space if not first word on
		// line
		need := visualLen(w)
		if curLen > visualLen(prefix) {
			need++ // space
		}
		if curLen+need <= avail {
			if curLen > visualLen(prefix) {
				cur += " "
				curLen++
			}
			cur += w
			curLen += visualLen(w)
		} else {
			// Emit current
			lines = append(lines, cur)
			// Start new line with continuation prefix
			prefix = contPrefix
			cur = prefix + w
			curLen = visualLen(prefix) + visualLen(w)
		}
		// Last word emits the final line
		if i == len(words)-1 {
			lines = append(lines, cur)
		}
	}
	return lines
}

func splitWords(s string) []string {
	// Collapse internal whitespace to single spaces between words.
	f := func(r rune) bool { return unicode.IsSpace(r) }
	parts := strings.FieldsFunc(s, f)
	return parts
}

// hoistInlineComments moves trailing inline comments to standalone comment
// lines placed directly above the original code line. Supports:
//   - Trailing // comments after code
//   - Trailing single-line /* ... */ comments after code Multi-line trailing
//     block comments are left unchanged.
func hoistInlineComments(src []byte) []byte {
	lines := splitLines(string(src))
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		// Quick checks: skip pure comment lines
		if isStandaloneLineComment(line) || isStandaloneBlockStart(line) || strings.TrimSpace(line) == "" {
			out = append(out, line)
			continue
		}
		// Scan for inline comment tokens outside of strings.
		kind, start, end := findInlineCommentOnLine(line)
		if kind == "" {
			out = append(out, line)
			continue
		}
		indent, _ := splitIndent(line)
		code := strings.TrimRight(line[:start], " \t")
		commentText := strings.TrimSpace(line[end:])
		switch kind {
		case "//":
			// Never hoist directive-like comments; tools can require them
			// to remain trailing on the same line.
			if isDirectiveLineComment(indent + line[start:]) {
				out = append(out, line)
				continue
			}
			text := strings.TrimSpace(line[start+2:])
			if text != "" {
				out = append(out, indent+"// "+text)
			} else {
				out = append(out, indent+"//")
			}
			if code != "" {
				out = append(out, code)
			}
		case "/*":
			// Only handle when */ is on the same line (end points
			// to char after */) commentText currently is the suffix
			// after end; we need the inner text between /* and */.
			closeIdx := strings.Index(line[start+2:], "*/")
			inner := ""
			if closeIdx >= 0 {
				inner = line[start+2 : start+2+closeIdx]
			}
			inner = strings.TrimSpace(inner)
			if inner != "" {
				out = append(out, indent+"// "+inner)
			} else {
				out = append(out, indent+"//")
			}
			// Append code plus any suffix after */
			suffix := strings.TrimLeft(commentText, " \t")
			merged := code
			if suffix != "" {
				if merged == "" {
					merged = suffix
				} else {
					merged = merged + " " + suffix
				}
			}
			out = append(out, merged)
		default:
			out = append(out, line)
		}
	}
	return []byte(strings.Join(out, "\n"))
}

// findInlineCommentOnLine finds a trailing inline comment token on a single
// line, ignoring tokens inside string or char literals. Returns kind ("//" or
// "/*"), start index of the token, and end index just after the end of the
// comment (for // it's len(line); for /* */ it's index just after */). Returns
// empty kind if none found or if the token is at column 0 (i.e., pure comment).
func findInlineCommentOnLine(line string) (kind string, start, end int) {
	inStr := byte(0) // '"' or '`' or '\''
	esc := false
	seenCode := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if inStr != 0 {
			if inStr == '"' && c == '\\' && !esc {
				esc = true
				continue
			}
			if esc {
				esc = false
				continue
			}
			if c == inStr {
				inStr = 0
			}
			continue
		}
		switch c {
		case ' ', '\t':
			// whitespace before code
			continue
		case '"', '`', '\'':
			inStr = c
			seenCode = true
			continue
		case '/':
			if i+1 < len(line) {
				if line[i+1] == '/' {
					if !seenCode {
						return "", 0, 0
					}
					return "//", i, len(line)
				}
				if line[i+1] == '*' {
					if !seenCode {
						return "", 0, 0
					}
					// only if same-line close
					if j := strings.Index(line[i+2:], "*/"); j >= 0 {
						return "/*", i, i + 2 + j + 2
					}
					// multi-line trailing block not handled
					// in this pass
					return "", 0, 0
				}
			}
			seenCode = true
		default:
			seenCode = true
		}
	}
	return "", 0, 0
}
