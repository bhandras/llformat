package dsl

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
)

// ExpandFuncLitBodyAction expands a single-line function literal body into a
// multi-line block with one statement per line.
type ExpandFuncLitBodyAction struct {
	Target string
}

// Execute implements Action for ExpandFuncLitBodyAction.
func (a *ExpandFuncLitBodyAction) Execute(caps Captures, ctx *Context) ([]byte,
	bool) {

	node := resolveTarget(caps, a.Target)
	fn, ok := node.(*ast.FuncLit)
	if !ok || fn == nil || fn.Body == nil || !fn.Body.Lbrace.IsValid() ||
		!fn.Body.Rbrace.IsValid() {
		return nil, false
	}

	lbrace := ctx.Fset.Position(fn.Body.Lbrace).Offset
	rbrace := ctx.Fset.Position(fn.Body.Rbrace).Offset
	if lbrace < 0 || rbrace < 0 || rbrace > len(ctx.Source) ||
		lbrace >= rbrace {
		return nil, false
	}

	origBody := string(ctx.Source[lbrace : rbrace+1])
	if strings.Contains(origBody, "\n") {
		return nil, false
	}
	if hasLineComment(origBody) || hasBlockComment(origBody) {
		return nil, false
	}

	// Policy:
	// - Do not expand function literals in call-arg position here; call
	//   formatting owns that and can decide based on the surrounding
	//   argument list. Expanding early can destroy packed call-arg layouts.
	// - Outside call-arg position, expand only non-trivial bodies. Keep
	//   trivial inline callbacks (including `func() {}` and `func() T {
	//   return ... }`) compact; these frequently appear in var/struct
	//   fixtures.
	if ctx != nil && ctx.IsChildOfCallExpr(fn) {
		return nil, false
	}
	if isTrivialInlineFuncLit(fn) {
		return nil, false
	}

	wsIndent := ctx.IndentAt(fn)
	stmtIndent := wsIndent + "\t"

	var out strings.Builder
	out.WriteString("{\n")

	for _, stmt := range fn.Body.List {
		stmtText := stmtString(ctx.Fset, stmt)
		stmtText = strings.TrimSpace(stmtText)
		if stmtText == "" {
			continue
		}
		out.WriteString(stmtIndent)
		out.WriteString(stmtText)
		out.WriteByte('\n')
	}

	out.WriteString(wsIndent)
	out.WriteByte('}')

	repl := out.String()
	if repl == origBody {
		return nil, false
	}

	newSrc, err := ApplySingleEdit(
		ctx.Source, lbrace, rbrace+1, []byte(repl),
	)
	if err != nil {
		return nil, false
	}
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(
		fset, "out.go", newSrc, parser.AllErrors,
	); err != nil {
		return nil, false
	}

	return newSrc, true
}

func isTrivialInlineFuncLit(fn *ast.FuncLit) bool {
	if fn == nil || fn.Body == nil {
		return false
	}
	// Empty body: keep `func() {}` inline.
	if len(fn.Body.List) == 0 {
		return true
	}
	if len(fn.Body.List) != 1 {
		return false
	}
	ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
	if !ok || ret == nil {
		return false
	}
	// `return` with no results is still "trivial" (e.g. `func() { return
	// }`).
	if len(ret.Results) == 0 {
		return true
	}
	// Consider `return` with only simple identifiers/literals trivial, even
	// when returning multiple values (e.g. `return nil, nil`).
	for _, res := range ret.Results {
		switch res.(type) {
		case *ast.Ident, *ast.BasicLit:

			// OK.
		default:
			return false
		}
	}

	return true
}

func stmtString(fset *token.FileSet, stmt ast.Stmt) string {
	if stmt == nil {
		return ""
	}
	var b bytes.Buffer
	_ = printer.Fprint(&b, fset, stmt)

	return b.String()
}
