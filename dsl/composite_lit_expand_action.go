package dsl

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
)

// ExpandCompositeLitAction expands a composite literal into a multiline form
// when it appears on an overflowing line, or when nested inside a multiline
// composite literal.
type ExpandCompositeLitAction struct {
	Target string
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
	if _, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors); err != nil {
		return nil, false
	}

	return out, true
}

func shouldExpandCompositeLit(lit *ast.CompositeLit, ctx *Context) bool {
	// Avoid rewriting composite literals inside var/const declarations.
	// gofmt's alignment behavior in `var (...)` blocks is sensitive to
	// multiline initializers, and the "next" goldens treat these as stable
	// fixtures.
	for cur := ast.Node(lit); cur != nil; cur = ctx.Parent(cur) {
		if _, ok := cur.(*ast.ValueSpec); ok {
			return false
		}
		if gen, ok := cur.(*ast.GenDecl); ok && gen != nil {
			if gen.Tok == token.VAR || gen.Tok == token.CONST {
				return false
			}
		}
	}

	// Avoid rewriting composite literals that are inside call argument
	// lists. Call formatting owns these and has better context for
	// indentation.
	for cur := ast.Node(lit); cur != nil; {
		parent := ctx.Parent(cur)
		if call, ok := parent.(*ast.CallExpr); ok &&
			isCallArg(call, cur) {

			return false
		}
		cur = parent
	}

	// If already multiline, keep as-is (nested literals will be handled
	// separately if needed).
	orig := string(ctx.NodeSource(lit))
	if strings.Contains(orig, "\n") {
		return false
	}

	if ctx.LineWidth(lit) > ctx.ColumnLimit {
		return true
	}

	// If this literal is nested inside another composite literal that is
	// already multiline, prefer expanding it as well for readability and
	// consistency.
	for cur := ctx.Parent(lit); cur != nil; cur = ctx.Parent(cur) {
		if _, ok := cur.(*ast.CompositeLit); ok {
			if strings.Contains(string(ctx.NodeSource(cur)), "\n") {
				return true
			}
		}
	}

	return false
}

func formatCompositeLitMultiline(lit *ast.CompositeLit, ctx *Context) string {
	wsIndent := ctx.IndentAt(lit)
	elemIndent := wsIndent + "\t"

	var typeBuf bytes.Buffer
	if lit.Type != nil {
		_ = printer.Fprint(&typeBuf, ctx.Fset, lit.Type)
	}
	typeText := strings.TrimSpace(typeBuf.String())

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
		out.WriteString(",\n")
	}

	out.WriteString(wsIndent)
	out.WriteString("}")

	return out.String()
}
