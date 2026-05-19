package compat

import (
	"strings"
	"unicode"
)

const (
	// CommentModeProse greedily reflows all standalone prose-like comment
	// blocks. This preserves the historical formatter behavior.
	CommentModeProse = "prose"

	// CommentModeOverflow only reflows standalone comment blocks when at
	// least one physical line exceeds the column limit. Blocks that already
	// fit, or that look preformatted, are preserved verbatim.
	CommentModeOverflow = "overflow"

	// CommentModeOff disables comment reflow.
	CommentModeOff = "off"
)

// CommentConfig holds configuration for comment reflowing.
type CommentConfig struct {
	ColumnLimit int
	TabStop     int
	// Mode controls how aggressively standalone comments are reflowed.
	// Empty defaults to CommentModeProse for compatibility.
	Mode string
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
	if cfg.Mode == "" {
		cfg.Mode = CommentModeProse
	}

	return &CommentFormatter{cfg: cfg}
}

// FormatFile implements greedy reflowing of comment-only lines.
func (f *CommentFormatter) FormatFile(src []byte) []byte {
	if f.cfg.Mode == CommentModeOff {
		return src
	}

	formatGlobalsMu.Lock()
	defer formatGlobalsMu.Unlock()

	// Use package-level width helpers
	if f.cfg.ColumnLimit > 0 {
		columnLimit = f.cfg.ColumnLimit
	}
	if f.cfg.TabStop > 0 {
		tabStop = f.cfg.TabStop
	}
	commentMode = f.cfg.Mode

	// Optional pre-pass: hoist inline comments above their line.
	if f.cfg.MoveInlineAbove {
		src = hoistInlineComments(src)
	}

	lines := splitLines(string(src))
	var out []string

	i := 0
	inRawString := false
	for i < len(lines) {
		line := lines[i]

		if inRawString {
			out = append(out, line)
			inRawString = rawStringStateAfterLine(line, inRawString)
			i++
			continue
		}

		// Try line comment block
		if newOut, newIdx, handled := processLineCommentBlock(
			lines, i, out,
		); handled {

			out = newOut
			i = newIdx
			continue
		}

		// Try standalone block comment
		if newOut, newIdx, handled := processBlockComment(
			lines, i, out,
		); handled {

			out = newOut
			i = newIdx
			continue
		}

		// Default: copy unchanged.
		out = append(out, line)
		inRawString = rawStringStateAfterLine(line, inRawString)
		i++
	}

	return []byte(strings.Join(out, "\n"))
}

// FormatCommentsInSource applies the legacy comment formatter to src and
// reports whether it changed anything.
func FormatCommentsInSource(src []byte, colLimit, tabStop int,
	moveInlineAbove bool, mode ...string) ([]byte, bool) {

	commentMode := CommentModeProse
	if len(mode) > 0 && mode[0] != "" {
		commentMode = mode[0]
	}
	f := NewCommentFormatter(
		CommentConfig{
			ColumnLimit:     colLimit,
			TabStop:         tabStop,
			Mode:            commentMode,
			MoveInlineAbove: moveInlineAbove,
		},
	)
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
	// the go/format pass later (if any) can normalize final newline
	// sentinel; the go/format pass later (if any) can normalize final
	// newline. Here we keep behavior consistent with our other formatters
	// which operate on bytes.
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

func rawStringStateAfterLine(line string, inRaw bool) bool {
	for i := 0; i < len(line); i++ {
		if inRaw {
			if line[i] == '`' {
				inRaw = false
			}
			continue
		}

		switch line[i] {
		case '`':
			inRaw = true

		case '"', '\'':
			i = skipQuotedLiteral(line, i)

		case '/':
			if i+1 >= len(line) {
				continue
			}
			switch line[i+1] {
			case '/':
				return inRaw

			case '*':
				i = skipInlineBlockComment(line, i+2)
			}
		}
	}

	return inRaw
}

func skipQuotedLiteral(line string, start int) int {
	quote := line[start]
	for i := start + 1; i < len(line); i++ {
		if line[i] == '\\' {
			i++
			continue
		}
		if line[i] == quote {
			return i
		}
	}

	return len(line) - 1
}

func skipInlineBlockComment(line string, start int) int {
	for i := start; i+1 < len(line); i++ {
		if line[i] == '*' && line[i+1] == '/' {
			return i + 1
		}
	}

	return len(line) - 1
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

	// Build tags are typically `// +build ...` but be tolerant of `//+build
	// ...`.
	if strings.HasPrefix(rest, "// +build") ||
		strings.HasPrefix(rest, "//+build") {
		return true
	}

	// `//line` directives must have no space between `//` and `line`. Avoid
	// treating ordinary comment prose like "the next line ..." as a
	// directive.
	if strings.HasPrefix(rest, "//line") {
		return true
	}

	// cgo `//export` directives must have no space between `//` and
	// `export`. Reflowing (e.g. turning it into `// export`) can break cgo.
	if strings.HasPrefix(rest, "//export") {
		if len(rest) == len("//export") {
			return true
		}
		switch rest[len("//export")] {
		case ' ', '\t':
			return true
		}
	}

	// Common lint directives are typically tool-specific and should not be
	// wrapped.
	if strings.HasPrefix(rest, "//nolint:") ||
		strings.HasPrefix(rest, "// nolint:") {
		return true
	}
	if strings.HasPrefix(rest, "//lint:") ||
		strings.HasPrefix(rest, "// lint:") {
		return true
	}
	if strings.HasPrefix(rest, "//staticcheck:") ||
		strings.HasPrefix(rest, "// staticcheck:") {
		return true
	}
	if strings.HasPrefix(rest, "//gosec:") ||
		strings.HasPrefix(rest, "// gosec:") {
		return true
	}
	if strings.HasPrefix(rest, "//revive:") ||
		strings.HasPrefix(rest, "// revive:") {
		return true
	}

	return false
}

func isStandaloneBlockStart(s string) bool {
	_, rest := splitIndent(s)

	return rest == "/*"
}

func isStandaloneBlockEnd(s string) bool {
	_, rest := splitIndent(s)

	return rest == "*/"
}

func isDirectiveBlockComment(block []string) bool {
	// Preserve any cgo directive blocks starting with #cgo, or blocks
	// containing #include/#define lines. We check the raw interior lines.
	for _, line := range block {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "#cgo") ||
			strings.HasPrefix(trim, "#include") ||
			strings.HasPrefix(
				trim, "#define",
			) {
			return true
		}
	}

	return false
}

func reflowLineCommentBlock(block []string, indent string) []string {
	if commentBlockHasGoExampleOutput(block) {
		return block
	}
	if commentModePreservesBlock(block, commentMode) {
		return block
	}

	type paraKind int
	const (
		paraBlank paraKind = iota
		paraText
		paraListItem
	)
	type para struct {
		kind paraKind
		lead string
		// For text paragraphs, lines are trimmed text lines. For list
		// items, lines are item text fragments (dash + continuation).
		lines []string
	}

	var paras []para
	var curText []string
	var curList *para

	flushText := func() {
		if len(curText) == 0 {
			return
		}
		paras = append(paras, para{kind: paraText, lines: curText})
		curText = nil
	}
	flushList := func() {
		if curList == nil {
			return
		}
		paras = append(paras, *curList)
		curList = nil
	}
	pushBlank := func() {
		flushList()
		flushText()
		paras = append(paras, para{kind: paraBlank})
	}

	for _, line := range block {
		_, rest := splitIndent(line)
		content := strings.TrimPrefix(rest, "//")
		if strings.TrimSpace(content) == "" {
			pushBlank()
			continue
		}

		lead, afterLead := splitIndent(content)
		if strings.HasPrefix(afterLead, "- ") {
			flushText()
			flushList()
			itemText := strings.TrimSpace(afterLead[2:])
			curList = &para{
				kind:  paraListItem,
				lead:  lead,
				lines: []string{itemText},
			}
			continue
		}

		if curList != nil {
			// Treat any non-empty non-list line immediately
			// following a list item as a continuation line, even if
			// it isn't already indented. This lets the formatter
			// repair "broken" list indentation.
			_, contText := splitIndent(content)
			curList.lines = append(
				curList.lines, strings.TrimSpace(contText),
			)
			continue
		}

		curText = append(curText, strings.TrimSpace(content))
	}

	flushList()
	flushText()

	var out []string
	for _, p := range paras {
		switch p.kind {
		case paraBlank:
			out = append(out, indent+"//")

		case paraListItem:
			item := strings.Join(p.lines, " ")
			lines := reflowWords(
				item, indent+"//"+p.lead+"- ",
				indent+"//"+p.lead+"  ",
			)
			out = append(out, lines...)

		case paraText:
			text := strings.Join(p.lines, " ")
			lines := reflowWords(text, indent+"// ", indent+"// ")
			out = append(out, lines...)
		}
	}

	return out
}

func reflowBlockComment(block []string, indent string) []string {
	if len(block) < 2 {
		return block
	}
	if commentBlockHasGoExampleOutput(block) {
		return block
	}
	if commentModePreservesBlock(block, commentMode) {
		return block
	}
	open := block[0]
	close := block[len(block)-1]

	type paraKind int
	const (
		paraBlank paraKind = iota
		paraText
		paraListItem
	)
	type para struct {
		kind  paraKind
		lead  string
		lines []string
	}

	var paras []para
	var curText []string
	var curList *para

	flushText := func() {
		if len(curText) == 0 {
			return
		}
		paras = append(paras, para{kind: paraText, lines: curText})
		curText = nil
	}
	flushList := func() {
		if curList == nil {
			return
		}
		paras = append(paras, *curList)
		curList = nil
	}
	pushBlank := func() {
		flushList()
		flushText()
		paras = append(paras, para{kind: paraBlank})
	}

	for _, line := range block[1 : len(block)-1] {
		trim := strings.TrimSpace(line)
		trim = strings.TrimPrefix(trim, "*")
		content := strings.TrimPrefix(trim, " ")
		if strings.TrimSpace(content) == "" {
			pushBlank()
			continue
		}

		lead, afterLead := splitIndent(content)
		if strings.HasPrefix(afterLead, "- ") {
			flushText()
			flushList()
			itemText := strings.TrimSpace(afterLead[2:])
			curList = &para{
				kind:  paraListItem,
				lead:  lead,
				lines: []string{itemText},
			}
			continue
		}

		if curList != nil {
			// Treat any non-empty non-list line immediately
			// following a list item as a continuation line, even if
			// it isn't already indented. This lets the formatter
			// repair "broken" list indentation.
			_, contText := splitIndent(content)
			curList.lines = append(
				curList.lines, strings.TrimSpace(contText),
			)
			continue
		}

		curText = append(curText, strings.TrimSpace(content))
	}

	flushList()
	flushText()

	var out []string
	out = append(out, open)
	for _, p := range paras {
		switch p.kind {
		case paraBlank:
			out = append(out, indent+" *")

		case paraListItem:
			item := strings.Join(p.lines, " ")
			lines := reflowWords(
				item, indent+" * "+p.lead+"- ",
				indent+" * "+p.lead+"  ",
			)
			out = append(out, lines...)

		case paraText:
			text := strings.Join(p.lines, " ")
			lines := reflowWords(text, indent+" * ", indent+" * ")
			out = append(out, lines...)
		}
	}
	out = append(out, close)

	return out
}

func commentModePreservesBlock(block []string, mode string) bool {
	if mode != CommentModeOverflow {
		return false
	}
	if commentBlockFits(block) {
		return true
	}

	return commentBlockLooksPreformatted(block)
}

func commentBlockFits(block []string) bool {
	for _, line := range block {
		if visualLen(line) > columnLimit {
			return false
		}
	}

	return true
}

func commentBlockLooksPreformatted(block []string) bool {
	for _, raw := range block {
		content := commentLineContent(raw)
		trimmed := strings.TrimSpace(content)
		if trimmed == "" {
			continue
		}
		if isCommentRuler(trimmed) {
			return true
		}
		if strings.HasPrefix(trimmed, "```") ||
			strings.HasPrefix(trimmed, "~~~") {
			return true
		}
		if strings.Contains(trimmed, "|") {
			return true
		}
		if strings.Contains(trimmed, "http://") ||
			strings.Contains(trimmed, "https://") {
			return true
		}
		if hasAlignmentSpaces(content) {
			return true
		}
		if startsNumberedList(trimmed) {
			return true
		}
	}

	return false
}

func commentBlockHasGoExampleOutput(block []string) bool {
	for _, raw := range block {
		switch strings.TrimSpace(commentLineContent(raw)) {
		case "Output:", "Unordered output:":
			return true
		}
	}

	return false
}

func isCommentRuler(s string) bool {
	trimmed := strings.TrimSpace(s)
	if len(trimmed) < 3 {
		return false
	}

	count := 0
	for _, r := range trimmed {
		if r == ' ' || r == '\t' {
			continue
		}
		if !strings.ContainsRune("-=_*~#.", r) {
			return false
		}
		count++
	}

	return count >= 3
}

func commentLineContent(line string) string {
	_, rest := splitIndent(line)
	if strings.HasPrefix(rest, "//") {
		return strings.TrimPrefix(rest, "//")
	}
	trimmed := strings.TrimSpace(rest)
	if strings.HasPrefix(trimmed, "/*") ||
		strings.HasPrefix(trimmed, "*/") {
		return ""
	}
	trimmed = strings.TrimPrefix(trimmed, "*")

	return strings.TrimPrefix(trimmed, " ")
}

func hasAlignmentSpaces(s string) bool {
	prevNonSpace := false
	spaces := 0
	for _, r := range s {
		if r == ' ' || r == '\t' {
			if prevNonSpace {
				spaces++
			}
			continue
		}
		if spaces >= 2 {
			return true
		}
		prevNonSpace = true
		spaces = 0
	}

	return false
}

func startsNumberedList(s string) bool {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 || i == len(s) {
		return false
	}
	if s[i] != '.' && s[i] != ')' {
		return false
	}
	if i+1 == len(s) {
		return true
	}

	return s[i+1] == ' ' || s[i+1] == '\t'
}

func reflowWords(text, prefix, contPrefix string) []string {
	words := splitWords(text)
	if len(words) == 0 {
		return []string{strings.TrimRight(prefix, " ")}
	}

	lines := make([]string, 0, 8)
	cur := prefix
	curLen := visualLen(prefix)
	for _, w := range words {
		wordLen := visualLen(w)
		prefixLen := visualLen(prefix)

		if curLen > prefixLen {
			if curLen+1+wordLen <= columnLimit {
				cur += " " + w
				curLen += 1 + wordLen
				continue
			}
			lines = append(lines, cur)
			prefix = contPrefix
			prefixLen = visualLen(prefix)
		}

		cur = prefix + w
		curLen = prefixLen + wordLen
		if curLen > columnLimit {
			lines = append(lines, cur)
			prefix = contPrefix
			cur = prefix
			curLen = visualLen(prefix)
		}
	}
	if curLen > visualLen(prefix) {
		lines = append(lines, cur)
	}

	return lines
}

func splitWords(s string) []string {
	f := func(r rune) bool {
		return unicode.IsSpace(r)
	}
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
		if isStandaloneLineComment(line) ||
			isStandaloneBlockStart(line) ||
			strings.TrimSpace(line) == "" {

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
			// Never hoist directive-like comments; tools can
			// require them to remain trailing on the same line.
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
		if consumeStringCharCompat(c, &inStr, &esc) {
			continue
		}
		switch c {
		case ' ', '\t':
			// whitespace before code
			continue

		case '"', '\'', '`':
			inStr = c
			seenCode = true
			continue

		case '/':
			if i+1 < len(line) && line[i+1] == '/' {
				// If comment begins at first non-space, treat
				// as standalone comment.
				if !seenCode {
					return "", 0, 0
				}

				return "//", i, len(line)
			}
			if i+1 < len(line) && line[i+1] == '*' {
				// Must have closing */ on same line to be
				// hoisted.
				j := strings.Index(line[i+2:], "*/")
				if j < 0 {
					continue
				}
				if !seenCode {
					return "", 0, 0
				}

				return "/*", i, i + 2 + j + 2
			}
			seenCode = true

		default:
			seenCode = true
		}
	}

	return "", 0, 0
}

func consumeStringCharCompat(c byte, inStr *byte, esc *bool) bool {
	if *inStr == 0 {
		return false
	}
	if *inStr == '"' && c == '\\' && !*esc {
		*esc = true

		return true
	}
	if *esc {
		*esc = false

		return true
	}
	if c == *inStr {
		*inStr = 0
	}

	return true
}

// processLineCommentBlock handles a contiguous block of line comments starting
// at index i. Returns the updated output slice, new index, and whether a block
// was processed.
func processLineCommentBlock(lines []string, i int, out []string) ([]string,
	int, bool) {

	line := lines[i]
	if !isStandaloneLineComment(line) || isDirectiveLineComment(line) {
		return out, i, false
	}

	indent, _ := splitIndent(line)
	block := []string{line}
	j := i + 1
	for j < len(lines) {
		next := lines[j]
		if !isStandaloneLineComment(next) {
			break
		}
		if isDirectiveLineComment(next) {
			break
		}
		nextIndent, _ := splitIndent(next)
		if nextIndent != indent {
			break
		}
		block = append(block, next)
		j++
	}

	if commentMode == CommentModeOverflow &&
		lineCommentBlockHasBlank(block) {

		reflowed := reflowLineCommentBlockByBlankBoundaries(
			block, indent,
		)
		out = append(out, reflowed...)

		return out, j, true
	}

	reflowed := reflowLineCommentBlock(block, indent)
	out = append(out, reflowed...)

	return out, j, true
}

func lineCommentBlockHasBlank(block []string) bool {
	for _, line := range block {
		if isBlankLineComment(line) {
			return true
		}
	}

	return false
}

func isBlankLineComment(line string) bool {
	return strings.TrimSpace(commentLineContent(line)) == ""
}

func reflowLineCommentBlockByBlankBoundaries(block []string,
	indent string) []string {

	var out []string
	var segment []string
	flush := func() {
		if len(segment) == 0 {
			return
		}
		out = append(out, reflowLineCommentBlock(segment, indent)...)
		segment = nil
	}

	for _, line := range block {
		if isBlankLineComment(line) {
			flush()
			out = append(out, line)
			continue
		}
		segment = append(segment, line)
	}
	flush()

	return out
}

// processBlockComment handles a standalone block comment starting at index i.
// Returns the updated output slice, new index, and whether a block was
// processed.
func processBlockComment(lines []string, i int, out []string) ([]string, int,
	bool) {

	line := lines[i]
	if !isStandaloneBlockStart(line) {
		return out, i, false
	}

	indent, _ := splitIndent(line)
	block := []string{line}
	j := i + 1
	for j < len(lines) {
		block = append(block, lines[j])
		if isStandaloneBlockEnd(lines[j]) {
			j++
			break
		}
		j++
	}

	if isDirectiveBlockComment(block) {
		out = append(out, block...)

		return out, j, true
	}

	reflowed := reflowBlockComment(block, indent)
	out = append(out, reflowed...)

	return out, j, true
}
