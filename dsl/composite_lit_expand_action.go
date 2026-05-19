package dsl

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"

	"github.com/bhandras/llformat/dsl/layout"
	llscanner "github.com/bhandras/llformat/scanner"
)

// ExpandCompositeLitAction expands a composite literal into a multiline form
// when it appears on an overflowing line, or when nested inside a multiline
// composite literal.
type ExpandCompositeLitAction struct {
	Target string
}

// BreakCompositeKeyValueAction breaks the value side of an overlong keyed
// composite-literal element without expanding the whole literal.
type BreakCompositeKeyValueAction struct {
	Target string
}

// MoveCompositeTrailingCommentAction moves an overflowing trailing line comment
// above a multiline composite literal element when both resulting lines fit.
type MoveCompositeTrailingCommentAction struct {
	Target string
}

// Execute implements Action for MoveCompositeTrailingCommentAction.
func (a *MoveCompositeTrailingCommentAction) Execute(caps Captures,
	ctx *Context) ([]byte, bool) {

	node := resolveTarget(caps, a.Target)
	lit, ok := node.(*ast.CompositeLit)
	if !ok || lit == nil || ctx == nil {
		return nil, false
	}

	start := ctx.Fset.Position(lit.Pos()).Offset
	end := ctx.Fset.Position(lit.End()).Offset
	if start < 0 || end > len(ctx.Source) || start >= end {
		return nil, false
	}
	if !bytes.Contains(ctx.Source[start:end], []byte("\n")) {
		return nil, false
	}

	editStart, editEnd, replacement, ok := compositeTrailingCommentEdit(
		ctx, start, end,
	)
	if !ok {
		return nil, false
	}

	out, err := ApplySingleEdit(ctx.Source, editStart, editEnd, replacement)
	if err != nil {
		return nil, false
	}
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(
		fset, "out.go", out, parser.AllErrors,
	); err != nil {
		return nil, false
	}

	return out, true
}

func compositeTrailingCommentEdit(ctx *Context, start, end int) (int, int,
	[]byte, bool) {

	for lineStartIdx := lineStart(ctx.Source, start); lineStartIdx < end; {
		lineEndIdx := lineEnd(ctx.Source, lineStartIdx)
		if lineEndIdx <= start {
			lineStartIdx = nextLineStart(
				lineEndIdx, len(ctx.Source),
			)
			continue
		}
		if lineStartIdx >= end {
			break
		}

		line := string(ctx.Source[lineStartIdx:lineEndIdx])
		if visualLen(line, ctx.TabStop) <= ctx.ColumnLimit {
			lineStartIdx = nextLineStart(
				lineEndIdx, len(ctx.Source),
			)
			continue
		}

		commentIdx := trailingLineCommentIndex(line)
		if commentIdx < 0 {
			lineStartIdx = nextLineStart(
				lineEndIdx, len(ctx.Source),
			)
			continue
		}

		code := strings.TrimRight(line[:commentIdx], " \t")
		if !strings.HasSuffix(strings.TrimSpace(code), ",") {
			lineStartIdx = nextLineStart(
				lineEndIdx, len(ctx.Source),
			)
			continue
		}

		comment := strings.TrimSpace(line[commentIdx:])
		if isDirectiveLikeLineComment(comment) {
			lineStartIdx = nextLineStart(
				lineEndIdx, len(ctx.Source),
			)
			continue
		}

		indent := lineLeadingWhitespace(line)
		commentLine := indent + comment
		if visualLen(code, ctx.TabStop) > ctx.ColumnLimit ||
			visualLen(commentLine, ctx.TabStop) > ctx.ColumnLimit {

			lineStartIdx = nextLineStart(
				lineEndIdx, len(ctx.Source),
			)
			continue
		}

		replacement := []byte(commentLine + "\n" + code)

		return lineStartIdx, lineEndIdx, replacement, true
	}

	return 0, 0, nil, false
}

func nextLineStart(lineEndIdx, srcLen int) int {
	if lineEndIdx < srcLen {
		return lineEndIdx + 1
	}

	return lineEndIdx
}

func lineLeadingWhitespace(line string) string {
	for i := 0; i < len(line); i++ {
		if line[i] != ' ' && line[i] != '\t' {
			return line[:i]
		}
	}

	return line
}

func trailingLineCommentIndex(line string) int {
	src := []byte(line)
	for i := 0; i < len(src); {
		switch {
		case llscanner.IsStringStart(src, i):
			i = llscanner.ScanString(src, i)

		case llscanner.IsLineCommentStart(src, i):
			return i

		case llscanner.IsBlockCommentStart(src, i):
			i = llscanner.ScanBlockComment(src, i)

		default:
			i++
		}
	}

	return -1
}

func isDirectiveLikeLineComment(comment string) bool {
	rest := strings.TrimSpace(comment)
	lower := strings.ToLower(rest)

	return strings.HasPrefix(rest, "//go:") ||
		strings.HasPrefix(rest, "// +build") ||
		strings.HasPrefix(rest, "//+build") ||
		strings.HasPrefix(rest, "//line") ||
		strings.HasPrefix(rest, "//export") ||
		strings.Contains(lower, "nolint") ||
		strings.HasPrefix(lower, "//lint:") ||
		strings.HasPrefix(lower, "// lint:") ||
		strings.HasPrefix(lower, "//staticcheck:") ||
		strings.HasPrefix(lower, "// staticcheck:") ||
		strings.HasPrefix(lower, "//gosec:") ||
		strings.HasPrefix(lower, "// gosec:") ||
		strings.HasPrefix(lower, "//revive:") ||
		strings.HasPrefix(lower, "// revive:")
}

// Execute implements Action for BreakCompositeKeyValueAction.
func (a *BreakCompositeKeyValueAction) Execute(caps Captures, ctx *Context) (
	[]byte, bool) {

	node := resolveTarget(caps, a.Target)
	kv, ok := node.(*ast.KeyValueExpr)
	if !ok || kv == nil || kv.Key == nil || kv.Value == nil {
		return nil, false
	}

	if !shouldBreakCompositeKeyValue(kv, ctx) {
		return nil, false
	}

	start := ctx.Fset.Position(kv.Pos()).Offset
	end := ctx.Fset.Position(kv.End()).Offset
	if start < 0 || end > len(ctx.Source) || start >= end {
		return nil, false
	}

	orig := string(ctx.Source[start:end])
	if orig == "" || hasLineComment(orig) || hasBlockComment(orig) {
		return nil, false
	}

	keyText := renderNode(kv.Key, ctx.Fset)
	valueText := renderNode(kv.Value, ctx.Fset)
	if keyText == "" || valueText == "" ||
		strings.Contains(keyText, "\n") ||
		strings.Contains(valueText, "\n") ||
		hasAnyComment(keyText) || hasAnyComment(valueText) {
		return nil, false
	}

	info, ok := exprDocWithKind(kv.Value, ctx, exprDocKindCallArg)
	if !ok {
		return nil, false
	}
	valueDoc := info.Doc
	if info.NeedsContinuationIndent {
		valueDoc = layout.N("\t", valueDoc)
	}

	doc := layout.G(
		layout.C(
			layout.T(keyText), layout.T(": "),
			layout.N("\t", valueDoc),
		),
	)

	formatted := layout.RenderAt(
		doc, ctx.ColumnLimit, ctx.TabStop, ctx.IndentAt(kv),
		prefixWidthAt(ctx.Source, start, ctx.TabStop),
	)
	if formatted == "" || formatted == orig ||
		!strings.Contains(formatted, "\n") {
		return nil, false
	}

	out, err := ApplySingleEdit(ctx.Source, start, end, []byte(formatted))
	if err != nil {
		return nil, false
	}

	return out, true
}

func shouldBreakCompositeKeyValue(kv *ast.KeyValueExpr, ctx *Context) bool {
	if kv == nil || ctx == nil {
		return false
	}

	if ctx.LineWidth(kv) <= ctx.ColumnLimit {
		return false
	}

	if _, ok := kv.Key.(*ast.Ident); !ok {
		return false
	}

	if exprContainsCompositeLit(kv.Value) {
		return false
	}

	lit, ok := ctx.Parent(kv).(*ast.CompositeLit)
	if !ok || lit == nil {
		return false
	}

	if !isMultilineCompositeLit(lit, ctx) {
		return false
	}

	return true
}

// Execute implements Action for ExpandCompositeLitAction.
func (a *ExpandCompositeLitAction) Execute(caps Captures, ctx *Context) ([]byte,
	bool) {

	node := resolveTarget(caps, a.Target)
	lit, ok := node.(*ast.CompositeLit)
	if !ok || lit == nil {
		return nil, false
	}

	start := ctx.Fset.Position(lit.Pos()).Offset
	end := ctx.Fset.Position(lit.End()).Offset
	if start < 0 || end > len(ctx.Source) || start >= end {
		return nil, false
	}

	orig := string(ctx.Source[start:end])
	if orig == "" {
		return nil, false
	}
	// Avoid spans containing inline comments; AST printing doesn't preserve
	// them inside expressions reliably.
	if hasLineComment(orig) || hasBlockComment(orig) {
		return nil, false
	}

	if strings.Contains(orig, "\n") {
		if out, changed := breakAdjacentCompositeLitElements(
			lit, ctx,
		); changed {
			return out, true
		}
		if formatted, ok := collapseSimpleCompositeLit(
			lit, ctx, start, end,
		); ok {

			out, err := ApplySingleEdit(
				ctx.Source, start, end, []byte(formatted),
			)
			if err != nil || !parseCheckOK(out) {
				return nil, false
			}

			return out, true
		}
		if out, changed := breakCompositeLitTypeArgs(
			lit, ctx,
		); changed {
			return out, true
		}
	}

	// Only expand when it helps: either the literal participates in a line
	// that exceeds the limit, or it's nested inside a multiline composite
	// literal.
	if !shouldExpandCompositeLit(lit, ctx) {
		return nil, false
	}

	formatted := formatCompositeLitMultiline(lit, ctx)
	if formatted == "" || formatted == orig {
		return nil, false
	}

	out, err := ApplySingleEdit(ctx.Source, start, end, []byte(formatted))
	if err != nil {
		return nil, false
	}
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(
		fset, "out.go", out, parser.AllErrors,
	); err != nil {
		return nil, false
	}

	return out, true
}

func shouldExpandCompositeLit(lit *ast.CompositeLit, ctx *Context) bool {
	// Avoid rewriting composite literals inside var/const declarations.
	// gofmt's alignment behavior in `var (...)` blocks is sensitive to
	// multiline initializers, and the "next" goldens treat these as stable
	// fixtures.
	if hasVarOrConstParent(lit, ctx) {
		return false
	}

	// Avoid rewriting composite literals that are inside call argument
	// lists. Call formatting owns these and has better context for
	// indentation.
	if isCallArgNested(lit, ctx) {
		return false
	}

	// If already multiline, keep as-is (nested literals will be handled
	// separately if needed).
	orig := string(ctx.NodeSource(lit))
	if strings.Contains(orig, "\n") {
		return false
	}

	if compositeLitCompactSingleUnkeyedFits(lit, ctx) {
		return false
	}

	if ctx.LineWidth(lit) > ctx.ColumnLimit {
		return true
	}

	// If this literal is nested inside another composite literal that is
	// already multiline, prefer expanding it as well for readability and
	// consistency.
	for cur := ctx.Parent(lit); cur != nil; cur = ctx.Parent(cur) {
		if isMultilineCompositeLit(cur, ctx) {
			return true
		}
	}

	return false
}

func breakAdjacentCompositeLitElements(lit *ast.CompositeLit,
	ctx *Context) ([]byte, bool) {

	if lit == nil || ctx == nil || len(lit.Elts) < 2 {
		return nil, false
	}

	elemIndent := ctx.IndentAt(lit) + "\t"
	var b EditBuilder
	changed := false
	for i := 1; i < len(lit.Elts); i++ {
		prev, ok := lit.Elts[i-1].(*ast.CompositeLit)
		if !ok || prev == nil {
			continue
		}
		next, ok := lit.Elts[i].(*ast.CompositeLit)
		if !ok || next == nil {
			continue
		}

		prevEnd := ctx.Fset.Position(prev.End()).Offset
		nextStart := ctx.Fset.Position(next.Pos()).Offset
		if prevEnd < 0 || nextStart <= prevEnd ||
			nextStart > len(ctx.Source) {

			continue
		}
		if lineStart(ctx.Source, prevEnd) !=
			lineStart(ctx.Source, nextStart) {

			continue
		}
		if strings.TrimSpace(
			string(ctx.Source[prevEnd:nextStart]),
		) != "," {

			continue
		}

		b.Replace(prevEnd, nextStart, []byte(",\n"+elemIndent))
		changed = true
	}
	if !changed {
		return nil, false
	}

	out, changed, err := b.Apply(ctx.Source)
	if err != nil || !changed || !parseCheckOK(out) {
		return nil, false
	}

	return out, true
}

func collapseSimpleCompositeLit(lit *ast.CompositeLit, ctx *Context, start,
	end int) (string, bool) {

	formatted, ok := compactSingleUnkeyedCompositeLit(lit, ctx)
	if !ok || formatted == string(ctx.Source[start:end]) {
		return "", false
	}
	if !replacementLinesFitWithinLimit(ctx, start, end, formatted) {
		return "", false
	}

	return formatted, true
}

func compositeLitCompactSingleUnkeyedFits(lit *ast.CompositeLit,
	ctx *Context) bool {

	formatted, ok := compactSingleUnkeyedCompositeLit(lit, ctx)
	if !ok {
		return false
	}

	start := ctx.Fset.Position(lit.Pos()).Offset
	end := ctx.Fset.Position(lit.End()).Offset
	if start < 0 || end > len(ctx.Source) || start >= end {
		return false
	}

	return replacementLinesFitWithinLimit(ctx, start, end, formatted)
}

func compactSingleUnkeyedCompositeLit(lit *ast.CompositeLit,
	ctx *Context) (string, bool) {

	if lit == nil || ctx == nil || lit.Type == nil || len(lit.Elts) != 1 {
		return "", false
	}
	if _, ok := lit.Elts[0].(*ast.KeyValueExpr); ok {
		return "", false
	}

	typeText := strings.TrimSpace(renderNode(lit.Type, ctx.Fset))
	eltText := strings.TrimSpace(renderNode(lit.Elts[0], ctx.Fset))
	if typeText == "" || eltText == "" ||
		strings.Contains(typeText, "\n") ||
		strings.Contains(eltText, "\n") ||
		hasAnyComment(typeText) || hasAnyComment(eltText) {
		return "", false
	}

	return typeText + "{" + eltText + "}", true
}

func hasVarOrConstParent(lit *ast.CompositeLit, ctx *Context) bool {
	for cur := ast.Node(lit); cur != nil; cur = ctx.Parent(cur) {
		if _, ok := cur.(*ast.ValueSpec); ok {
			return true
		}
		gen, ok := cur.(*ast.GenDecl)
		if !ok || gen == nil {
			continue
		}
		if gen.Tok == token.VAR || gen.Tok == token.CONST {
			return true
		}
	}

	return false
}

func isCallArgNested(lit *ast.CompositeLit, ctx *Context) bool {
	for cur := ast.Node(lit); cur != nil; {
		parent := ctx.Parent(cur)
		if fn, ok := parent.(*ast.FuncLit); ok && fn.Body == cur {
			return false
		}
		call, ok := parent.(*ast.CallExpr)
		if ok && isCallArg(call, cur) {
			return true
		}
		cur = parent
	}

	return false
}

func isMultilineCompositeLit(node ast.Node, ctx *Context) bool {
	lit, ok := node.(*ast.CompositeLit)
	if !ok || lit == nil {
		return false
	}

	return strings.Contains(string(ctx.NodeSource(lit)), "\n")
}

func formatCompositeLitMultiline(lit *ast.CompositeLit, ctx *Context) string {
	wsIndent := ctx.IndentAt(lit)
	elemIndent := wsIndent + "\t"

	var typeBuf bytes.Buffer
	if lit.Type != nil {
		_ = printer.Fprint(&typeBuf, ctx.Fset, lit.Type)
	}
	typeText := strings.TrimSpace(typeBuf.String())
	if formattedType, ok := formatCompositeLitTypeArgs(
		lit, typeText, wsIndent, ctx,
	); ok {

		typeText = formattedType
	}

	open := "{"
	if typeText != "" {
		open = typeText + "{"
	}

	// Empty composite literal: keep compact.
	if len(lit.Elts) == 0 {
		return open + "}"
	}

	var out strings.Builder
	out.WriteString(open)
	out.WriteByte('\n')

	for _, elt := range lit.Elts {
		eltText := strings.TrimSpace(string(ctx.NodeSource(elt)))
		if eltText == "" {
			continue
		}
		writeCompositeElement(&out, elemIndent, eltText)
		out.WriteString(",\n")
	}

	out.WriteString(wsIndent)
	out.WriteString("}")

	return out.String()
}

func breakCompositeLitTypeArgs(lit *ast.CompositeLit,
	ctx *Context) ([]byte, bool) {

	if lit == nil || lit.Type == nil || ctx == nil {
		return nil, false
	}

	typeStart := ctx.Fset.Position(lit.Type.Pos()).Offset
	typeEnd := ctx.Fset.Position(lit.Type.End()).Offset
	if typeStart < 0 || typeEnd > len(ctx.Source) || typeStart >= typeEnd {
		return nil, false
	}

	typeText := string(ctx.Source[typeStart:typeEnd])
	formattedType, ok := formatCompositeLitTypeArgs(
		lit, typeText, ctx.IndentAt(lit), ctx,
	)
	if !ok {
		return nil, false
	}

	out, err := ApplySingleEdit(
		ctx.Source, typeStart, typeEnd, []byte(formattedType),
	)
	if err != nil {
		return nil, false
	}
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(
		fset, "out.go", out, parser.AllErrors,
	); err != nil {
		return nil, false
	}

	return out, true
}

func formatCompositeLitTypeArgs(lit *ast.CompositeLit, typeText,
	wsIndent string, ctx *Context) (string, bool) {

	if lit == nil || lit.Type == nil || typeText == "" ||
		strings.Contains(typeText, "\n") || hasAnyComment(typeText) {
		return "", false
	}

	idx, ok := lit.Type.(*ast.IndexListExpr)
	if !ok || len(idx.Indices) < 2 {
		return "", false
	}

	start := ctx.Fset.Position(lit.Pos()).Offset
	if start < 0 || start > len(ctx.Source) {
		return "", false
	}
	headWidth := prefixWidthAt(ctx.Source, start, ctx.TabStop) +
		visualLen(typeText+"{", ctx.TabStop)
	if headWidth <= ctx.ColumnLimit {
		return "", false
	}

	base := renderNode(idx.X, ctx.Fset)
	if base == "" || strings.Contains(base, "\n") || hasAnyComment(base) {
		return "", false
	}

	typeIndent := wsIndent + "\t"
	var out strings.Builder
	out.WriteString(base)
	out.WriteString("[\n")
	for _, index := range idx.Indices {
		indexText := renderNode(index, ctx.Fset)
		if indexText == "" || strings.Contains(indexText, "\n") ||
			hasAnyComment(indexText) {
			return "", false
		}
		out.WriteString(typeIndent)
		out.WriteString(indexText)
		out.WriteString(",\n")
	}
	out.WriteString(wsIndent)
	out.WriteByte(']')

	formatted := out.String()
	if formatted == typeText {
		return "", false
	}

	return formatted, true
}

func writeCompositeElement(out *strings.Builder, elemIndent, eltText string) {
	lines := strings.Split(eltText, "\n")
	for i, line := range lines {
		line = strings.TrimRight(line, " \t")
		line = strings.TrimLeft(line, " \t")
		if i == 0 {
			out.WriteString(elemIndent)
			out.WriteString(line)
			continue
		}
		out.WriteByte('\n')
		out.WriteString(elemIndent)
		out.WriteString(line)
	}
}
