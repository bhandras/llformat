package dsl

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
)

// SplitLongStringLiteralAction splits a long quoted string literal into a
// concatenation of multiple quoted literals, adding newlines and indentation.
//
// This is intentionally conservative and currently targets call-argument string
// literals (to avoid rewriting standalone expressions unexpectedly).
type SplitLongStringLiteralAction struct {
	Target     string
	MinTailLen int
}

// Execute implements Action for SplitLongStringLiteralAction.
func (a *SplitLongStringLiteralAction) Execute(caps Captures, ctx *Context) (
	[]byte, bool) {

	node := resolveTarget(caps, a.Target)
	lit, ok := node.(*ast.BasicLit)
	if !ok || lit == nil {
		return nil, false
	}
	if lit.Kind != token.STRING {
		return nil, false
	}

	start := ctx.Fset.Position(lit.Pos()).Offset
	end := ctx.Fset.Position(lit.End()).Offset
	if start < 0 || end > len(ctx.Source) || start >= end {
		return nil, false
	}

	orig := string(ctx.Source[start:end])
	if orig == "" || orig[0] == '`' {

		// Do not split raw string literals.
		return nil, false
	}

	text, err := strconv.Unquote(orig)
	if err != nil {
		return nil, false
	}
	// Do not split strings with no natural word boundary. Hard-splitting
	// identifiers like "item6" into "it" + "em6" is rarely desirable.
	if !strings.Contains(text, " ") {
		return nil, false
	}

	// Only split when this literal is a direct call argument.
	parent := ctx.Parent(lit)
	call, ok := parent.(*ast.CallExpr)
	if !ok || call == nil {
		return nil, false
	}

	argIndex := -1
	for i, arg := range call.Args {
		if arg == lit {
			argIndex = i
			break
		}
	}
	if argIndex < 0 {
		return nil, false
	}
	hasTrailingArgs := argIndex < len(call.Args)-1

	startCol := prefixWidthAt(ctx.Source, start, ctx.TabStop)
	wsIndent := ctx.IndentAt(lit)

	plain := quoteGoString(text)
	if startCol+visualLen(plain, ctx.TabStop) <= ctx.ColumnLimit {
		return nil, false
	}

	minTailLen := a.MinTailLen
	repl := buildSplitQuotedForCallArgDSL(
		text, startCol, wsIndent, ctx.ColumnLimit, ctx.TabStop,
		hasTrailingArgs, minTailLen,
	)
	if repl == "" || repl == orig {
		return nil, false
	}

	out, err := ApplySingleEdit(ctx.Source, start, end, []byte(repl))
	if err != nil {
		return nil, false
	}

	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors); err != nil {
		return nil, false
	}

	return out, true
}

func buildSplitQuotedForCallArgDSL(text string, startCol int, wsIndent string,
	colLimit int, tabStop int, hasTrailingArgs bool,
	minTailLen int) string {

	var out strings.Builder
	rest := text
	curStart := startCol

	// String continuation lines get an extra tab beyond the line's
	// indentation.
	stringContIndent := wsIndent + "\t"
	contStart := visualLen(stringContIndent, tabStop)

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

	for {
		if rest == "" {
			break
		}

		// If indentation already exceeds the available width budget,
		// splitting can't help.
		if curStart >= colLimit {
			out.WriteString(quoteGoString(rest))
			break
		}

		// If the whole rest fits as a quoted literal on this line, emit
		// and finish.
		if advanceColsFrom(curStart, quoteGoString(rest), tabStop) <= colLimit {
			out.WriteString(quoteGoString(rest))
			break
		}

		// Choose split point at last space that fits with trailing
		// join.
		cut := lastQuotedSpaceBeforeWithJoinMinTail(
			curStart, rest, colLimit, tabStop, hasTrailingArgs,
			minTailLen,
		)
		if cut <= 0 {
			// Hard cut by width: ensure we cut at a rune boundary.
			cut = cutIndexForWidthFrom(
				curStart, rest, colLimit, tabStop,
				hasTrailingArgs,
			)
		}
		if cut <= 0 || cut >= len(rest) {
			out.WriteString(quoteGoString(rest))
			break
		}

		seg := rest[:cut]
		writeJoin(seg)
		rest = rest[cut:]
		curStart = contStart
	}

	return out.String()
}

func advanceColsFrom(startCol int, s string, tabStop int) int {
	col := startCol
	for _, r := range s {
		if r == '\t' {
			col += tabStop - (col % tabStop)
			continue
		}
		col++
	}

	return col
}

func lastQuotedSpaceBeforeWithJoinMinTail(startCol int, s string, boundary int,
	tabStop int, hasTrailingArgs bool, minTailLen int) int {

	last := -1
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' {
			continue
		}
		if isTinyTail(s[i+1:], minTailLen) {
			continue
		}
		piece := s[:i+1]
		joinCols := 2 // " +"
		if hasTrailingArgs && len(piece) > 0 &&
			piece[len(piece)-1] == ' ' {

			joinCols = 1 // "+", gofmt removes the space before '+' in this context
		}
		used := advanceColsFrom(startCol, quoteGoString(piece), tabStop) +
			joinCols
		if used <= boundary {
			last = i + 1
		} else {
			break
		}
	}

	return last
}

func cutIndexForWidthFrom(startCol int, s string, boundary int, tabStop int,
	hasTrailingArgs bool) int {

	last := -1
	for idx, r := range s {
		piece := s[:idx+utf8Len(r)]
		joinCols := 2
		if hasTrailingArgs && len(piece) > 0 &&
			piece[len(piece)-1] == ' ' {

			joinCols = 1
		}
		used := advanceColsFrom(startCol, quoteGoString(piece), tabStop) +
			joinCols
		if used <= boundary {
			last = len(piece)
		} else {
			break
		}
	}

	return last
}

func utf8Len(r rune) int {
	switch {
	case r <= 0x7F:
		return 1

	case r <= 0x7FF:
		return 2

	case r <= 0xFFFF:
		return 3

	default:
		return 4
	}
}

func isTinyTail(s string, minTailLen int) bool {
	if minTailLen <= 0 {
		return false
	}
	trimmed := strings.TrimLeft(s, " ")

	return len(trimmed) > 0 && len(trimmed) < minTailLen
}

// quoteGoString emits a double-quoted Go string literal, preserving runes as-is
// where possible. This mirrors the behavior used by the call formatter.
func quoteGoString(s string) string {
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
			// Keep literal tab to match golden behavior.
			b.WriteByte('\t')

		default:
			if r < 0x20 {
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
