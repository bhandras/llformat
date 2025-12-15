package dsl

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/lightninglabs/llformat/dsl/layout"
)

type exprDocInfo struct {
	Doc layout.Doc

	// NeedsContinuationIndent indicates the caller should wrap the doc in a
	// standard continuation indentation (`layout.N("\t", ...)`) so that any
	// internal breaks are indented by one extra tab relative to the current
	// indentation.
	//
	// For some docs (notably generic CallExpr docs), this is false because the
	// doc already controls indentation and expects to align closing parens with
	// the current indentation level.
	NeedsContinuationIndent bool
}

type exprDocKind int

const (
	exprDocKindTopLevel exprDocKind = iota
	exprDocKindCallArg
)

// exprDoc is a small AST-to-layout.Doc builder for a limited subset of Go
// expressions.
//
// This is intentionally conservative and is only meant to support the current
// "modern layout" actions (selector chains, method chains, same-op binary
// chains). It returns ok=false for unsupported forms so callers can fall back
// to legacy/parity logic.
func exprDoc(expr ast.Expr, ctx *Context) (info exprDocInfo, ok bool) {
	return exprDocWithKind(expr, ctx, exprDocKindTopLevel)
}

func exprDocWithKind(expr ast.Expr, ctx *Context, kind exprDocKind) (info exprDocInfo, ok bool) {
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		doc, ok := selectorChainDoc(e, ctx)
		if !ok {
			return exprDocInfo{}, false
		}
		return exprDocInfo{Doc: doc, NeedsContinuationIndent: true}, true
	case *ast.CallExpr:
		if doc, ok := methodChainDoc(e, ctx); ok {
			return exprDocInfo{Doc: doc, NeedsContinuationIndent: true}, true
		}
		if kind == exprDocKindCallArg {
			if doc, ok := genericCallDoc(e, ctx); ok {
				return exprDocInfo{Doc: doc, NeedsContinuationIndent: false}, true
			}
		}
		// For top-level expression layout rules, we intentionally do not build
		// generic call docs yet: call formatting is owned by the call/multiline
		// stages.
		if kind != exprDocKindCallArg {
			return exprDocInfo{}, false
		}
		if doc, ok := genericCallDoc(e, ctx); ok {
			return exprDocInfo{Doc: doc, NeedsContinuationIndent: false}, true
		}
		return exprDocInfo{}, false
	case *ast.BinaryExpr:
		if kind == exprDocKindCallArg && (e.Op == token.LAND || e.Op == token.LOR) {
			doc, ok := logicalBinaryExprDoc(e, ctx, kind)
			if !ok {
				return exprDocInfo{}, false
			}
			return exprDocInfo{Doc: doc, NeedsContinuationIndent: true}, true
		}
		doc, ok := sameOpBinaryChainDoc(e, ctx)
		if !ok {
			return exprDocInfo{}, false
		}
		return exprDocInfo{Doc: doc, NeedsContinuationIndent: true}, true
	case *ast.ParenExpr:
		doc, ok := parenExprDoc(e, ctx)
		if !ok {
			return exprDocInfo{}, false
		}
		// ParenExpr controls its own indentation; callers should treat it like a
		// self-contained block.
		return exprDocInfo{Doc: doc, NeedsContinuationIndent: false}, true
	case *ast.CompositeLit:
		if kind != exprDocKindCallArg {
			return exprDocInfo{}, false
		}
		doc, ok := compositeLitDoc(e, ctx, kind)
		if !ok {
			return exprDocInfo{}, false
		}
		// CompositeLit includes its own braces and indentation decisions.
		return exprDocInfo{Doc: doc, NeedsContinuationIndent: false}, true
	case *ast.Ident, *ast.BasicLit, *ast.UnaryExpr, *ast.IndexExpr, *ast.SliceExpr:
		// Render atomic expressions as-is. These are safe to embed as docs but do
		// not participate in internal breaking yet.
		return exprDocInfo{Doc: layout.T(renderNode(expr, ctx.Fset)), NeedsContinuationIndent: false}, true
	default:
		return exprDocInfo{}, false
	}
}

func selectorChainDoc(sel *ast.SelectorExpr, ctx *Context) (layout.Doc, bool) {
	if sel == nil {
		return nil, false
	}

	// Collect selector chain components.
	var sels []string
	base := ast.Expr(sel)
	for {
		cur, ok := base.(*ast.SelectorExpr)
		if !ok || cur == nil {
			break
		}
		sels = append(sels, cur.Sel.Name)
		base = cur.X
	}
	if len(sels) < 2 || base == nil {
		return nil, false
	}

	// Reverse sels to get left-to-right order.
	for i, j := 0, len(sels)-1; i < j; i, j = i+1, j-1 {
		sels[i], sels[j] = sels[j], sels[i]
	}

	var docs []layout.Doc
	docs = append(docs, layout.T(renderNode(base, ctx.Fset)))
	for _, name := range sels {
		docs = append(docs, layout.T("."), layout.SL(), layout.T(name))
	}
	return layout.G(layout.C(docs...)), true
}

func methodChainDoc(call *ast.CallExpr, ctx *Context) (layout.Doc, bool) {
	if call == nil {
		return nil, false
	}

	// Method chains are a series of CallExpr nodes whose Fun is a SelectorExpr
	// and whose receiver is another CallExpr (except for the first).
	type segment struct {
		name     string
		args     []string
		ellipsis bool
	}

	var segs []segment
	cur := call
	var base ast.Expr

	for {
		sel, ok := cur.Fun.(*ast.SelectorExpr)
		if !ok || sel == nil {
			return nil, false
		}

		seg := segment{name: sel.Sel.Name, ellipsis: cur.Ellipsis.IsValid()}

		// Collect args as rendered text; skip multiline args to avoid awkward
		// interactions with the chain layout (leave to other formatters).
		for _, arg := range cur.Args {
			argText := renderNode(arg, ctx.Fset)
			if strings.Contains(argText, "\n") {
				return nil, false
			}
			seg.args = append(seg.args, argText)
		}
		segs = append(segs, seg)

		next, ok := sel.X.(*ast.CallExpr)
		if !ok {
			base = sel.X
			break
		}
		cur = next
	}

	if base == nil || len(segs) < 2 {
		return nil, false
	}

	// Reverse to left-to-right chain order.
	for i, j := 0, len(segs)-1; i < j; i, j = i+1, j-1 {
		segs[i], segs[j] = segs[j], segs[i]
	}

	var docs []layout.Doc
	docs = append(docs, layout.T(renderNode(base, ctx.Fset)))
	for _, seg := range segs {
		docs = append(docs, layout.T("."), layout.SL(), layout.T(seg.name))
		docs = append(docs, layout.T("("))
		if len(seg.args) > 0 {
			for i, arg := range seg.args {
				if i > 0 {
					docs = append(docs, layout.T(", "))
				}
				docs = append(docs, layout.T(arg))
			}
			if seg.ellipsis && len(seg.args) > 0 {
				docs = append(docs, layout.T("..."))
			}
		}
		docs = append(docs, layout.T(")"))
	}

	return layout.G(layout.C(docs...)), true
}

func genericCallDoc(call *ast.CallExpr, ctx *Context) (layout.Doc, bool) {
	if call == nil || ctx == nil {
		return nil, false
	}
	if len(call.Args) == 0 {
		return nil, false
	}

	// Be conservative: skip any comment-containing args, or args that already
	// contain newlines (we don't try to reindent nested multiline spans yet).
	var argDocs []layout.Doc
	for i, arg := range call.Args {
		argText := renderNode(arg, ctx.Fset)
		if strings.Contains(argText, "\n") {
			return nil, false
		}
		if hasAnyComment(argText) {
			return nil, false
		}

		// Prefer a structured doc for supported expression forms so nested
		// expressions can lay out cleanly within nested calls.
		if expr, okCast := arg.(ast.Expr); okCast {
			if info, okDoc := exprDocWithKind(expr, ctx, exprDocKindCallArg); okDoc {
				d := info.Doc
				if info.NeedsContinuationIndent {
					d = layout.N("\t", d)
				}
				if i > 0 {
					argDocs = append(argDocs, layout.T(","), layout.L())
				}
				argDocs = append(argDocs, d)
				continue
			}
		}

		if i > 0 {
			argDocs = append(argDocs, layout.T(","), layout.L())
		}
		argDocs = append(argDocs, layout.T(argText))
	}

	// Handle ellipsis call syntax: f(args...)
	if call.Ellipsis.IsValid() && len(argDocs) > 0 {
		argDocs[len(argDocs)-1] = layout.C(argDocs[len(argDocs)-1], layout.T("..."))
	}

	argsGroup := layout.G(layout.C(
		layout.SL(),
		layout.C(argDocs...),
		layout.IB(layout.T(","), layout.T("")),
	))

	doc := layout.G(layout.C(
		layout.T(renderNode(call.Fun, ctx.Fset)),
		layout.T("("),
		layout.N("\t", argsGroup),
		layout.SL(),
		layout.T(")"),
	))

	return doc, true
}

func parenExprDoc(p *ast.ParenExpr, ctx *Context) (layout.Doc, bool) {
	if p == nil || ctx == nil || p.X == nil {
		return nil, false
	}

	info, ok := exprDocWithKind(p.X, ctx, exprDocKindCallArg)
	if !ok {
		return nil, false
	}
	inner := info.Doc
	if info.NeedsContinuationIndent {
		inner = layout.N("\t", inner)
	}

	// Keep parens explicit while allowing the inner expression to break:
	// flat:  (inner)
	// break: (
	//          inner
	//        )
	return layout.G(layout.C(
		layout.T("("),
		layout.N("\t", layout.G(layout.C(layout.SL(), inner))),
		layout.SL(),
		layout.T(")"),
	)), true
}

func compositeLitDoc(lit *ast.CompositeLit, ctx *Context, kind exprDocKind) (layout.Doc, bool) {
	if lit == nil || ctx == nil {
		return nil, false
	}

	// Be conservative: do not attempt to reindent existing multiline composite
	// literals; leave those to other formatters (or gofmt) to avoid surprising
	// changes.
	litText := renderNode(lit, ctx.Fset)
	if strings.Contains(litText, "\n") {
		return nil, false
	}
	if hasAnyComment(litText) {
		return nil, false
	}

	typeText := ""
	if lit.Type != nil {
		typeText = renderNode(lit.Type, ctx.Fset)
	}

	var eltDocs []layout.Doc
	for i, elt := range lit.Elts {
		eltText := renderNode(elt, ctx.Fset)
		if strings.Contains(eltText, "\n") {
			return nil, false
		}
		if hasAnyComment(eltText) {
			return nil, false
		}

		// Prefer structured docs when possible (e.g. nested calls / logical chains).
		if expr, okCast := elt.(ast.Expr); okCast {
			if info, okDoc := exprDocWithKind(expr, ctx, kind); okDoc {
				d := info.Doc
				if info.NeedsContinuationIndent {
					d = layout.N("\t", d)
				}
				if i > 0 {
					eltDocs = append(eltDocs, layout.T(","), layout.L())
				}
				eltDocs = append(eltDocs, d)
				continue
			}
		}

		if i > 0 {
			eltDocs = append(eltDocs, layout.T(","), layout.L())
		}
		eltDocs = append(eltDocs, layout.T(eltText))
	}

	body := layout.G(layout.C(
		layout.SL(),
		layout.C(eltDocs...),
		layout.IB(layout.T(","), layout.T("")),
	))

	// flat:  T{a, b}
	// break:
	//   T{
	//       a,
	//       b,
	//   }
	return layout.G(layout.C(
		layout.T(typeText),
		layout.T("{"),
		layout.N("\t", body),
		layout.SL(),
		layout.T("}"),
	)), true
}

func logicalBinaryExprDoc(bin *ast.BinaryExpr, ctx *Context, kind exprDocKind) (layout.Doc, bool) {
	if bin == nil || ctx == nil {
		return nil, false
	}
	if bin.Op != token.LAND && bin.Op != token.LOR {
		return nil, false
	}

	// Left operand: prefer doc, fall back to printed node.
	leftInfo, leftOK := exprDocWithKind(bin.X, ctx, kind)
	left := layout.T(renderNode(bin.X, ctx.Fset))
	if leftOK {
		left = leftInfo.Doc
		if leftInfo.NeedsContinuationIndent {
			left = layout.N("\t", left)
		}
	}

	rightInfo, rightOK := exprDocWithKind(bin.Y, ctx, kind)
	right := layout.T(renderNode(bin.Y, ctx.Fset))
	if rightOK {
		right = rightInfo.Doc
		if rightInfo.NeedsContinuationIndent {
			right = layout.N("\t", right)
		}
	}

	return layout.G(layout.C(
		left,
		layout.T(" "),
		layout.T(bin.Op.String()),
		layout.L(),
		right,
	)), true
}

func sameOpBinaryChainDoc(bin *ast.BinaryExpr, ctx *Context) (layout.Doc, bool) {
	if bin == nil {
		return nil, false
	}

	switch bin.Op {
	case token.LAND, token.LOR, token.ADD, token.SUB, token.MUL, token.QUO, token.REM:
	default:
		return nil, false
	}

	var terms []ast.Expr
	if !flattenSameOpBinaryChain(bin, bin.Op, &terms) || len(terms) < 2 {
		return nil, false
	}

	opStr := bin.Op.String()
	var docs []layout.Doc
	for i, term := range terms {
		if i > 0 {
			docs = append(docs, layout.T(" "), layout.T(opStr), layout.L())
		}
		docs = append(docs, layout.T(renderNode(term, ctx.Fset)))
	}
	return layout.G(layout.C(docs...)), true
}
