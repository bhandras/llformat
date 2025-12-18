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
	case *ast.IndexListExpr:
		if kind != exprDocKindCallArg {
			return exprDocInfo{}, false
		}
		doc, ok := indexListExprDoc(e, ctx, kind)
		if !ok {
			return exprDocInfo{}, false
		}
		// IndexListExpr controls its own bracket indentation decisions.
		return exprDocInfo{Doc: doc, NeedsContinuationIndent: false}, true
	case *ast.CallExpr:
		if kind == exprDocKindCallArg {
			if doc, ok := methodChainDocWithKind(e, ctx, kind); ok {
				return exprDocInfo{Doc: doc, NeedsContinuationIndent: true}, true
			}
			// Fall back to generic call formatting so nested calls can break their
			// arguments (including generic callees like `f[T, U](...)`).
			if doc, ok := genericCallDoc(e, ctx); ok {
				return exprDocInfo{Doc: doc, NeedsContinuationIndent: false}, true
			}
			return exprDocInfo{}, false
		}

		// Top-level expression rules: call formatting is owned by the call/multiline
		// stages, but we still support method-call chains (which do not alter call
		// argument structure).
		if doc, ok := methodChainDoc(e, ctx); ok {
			return exprDocInfo{Doc: doc, NeedsContinuationIndent: true}, true
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
		if kind == exprDocKindCallArg && isComparisonOp(e.Op) {
			doc, ok := comparisonBinaryExprDoc(e, ctx, kind)
			if !ok {
				return exprDocInfo{}, false
			}
			return exprDocInfo{Doc: doc, NeedsContinuationIndent: true}, true
		}
		doc, ok := sameOpBinaryChainDocWithKind(e, ctx, kind)
		if !ok {
			return exprDocInfo{}, false
		}
		return exprDocInfo{Doc: doc, NeedsContinuationIndent: true}, true
	case *ast.ParenExpr:
		// Be conservative with parenthesized calls in call-arg context. These are
		// particularly prone to producing parse hazards when combined with
		// argument-list commas and nested call rewrites (Go semicolon insertion is
		// unforgiving around closing parens).
		if kind == exprDocKindCallArg {
			if _, ok := e.X.(*ast.CallExpr); ok {
				return exprDocInfo{}, false
			}
		}
		doc, ok := parenExprDoc(e, ctx)
		if !ok {
			return exprDocInfo{}, false
		}
		// ParenExpr controls its own indentation; callers should treat it like a
		// self-contained block.
		return exprDocInfo{Doc: doc, NeedsContinuationIndent: false}, true
	case *ast.KeyValueExpr:
		if kind != exprDocKindCallArg {
			return exprDocInfo{}, false
		}
		doc, ok := keyValueExprDoc(e, ctx, kind)
		if !ok {
			return exprDocInfo{}, false
		}
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
	case *ast.UnaryExpr:
		if kind != exprDocKindCallArg {
			return exprDocInfo{}, false
		}
		info, ok := unaryExprDoc(e, ctx, kind)
		if !ok {
			return exprDocInfo{}, false
		}
		return info, true
	case *ast.StarExpr:
		if kind != exprDocKindCallArg {
			return exprDocInfo{}, false
		}
		info, ok := starExprDoc(e, ctx, kind)
		if !ok {
			return exprDocInfo{}, false
		}
		return info, true
	case *ast.TypeAssertExpr:
		if kind != exprDocKindCallArg {
			return exprDocInfo{}, false
		}
		doc, ok := typeAssertExprDoc(e, ctx, kind)
		if !ok {
			return exprDocInfo{}, false
		}
		// TypeAssertExpr includes `.(` and `)` and controls its own indentation.
		return exprDocInfo{Doc: doc, NeedsContinuationIndent: false}, true
	case *ast.IndexExpr:
		if kind != exprDocKindCallArg {
			return exprDocInfo{}, false
		}
		doc, ok := indexExprDoc(e, ctx, kind)
		if !ok {
			return exprDocInfo{}, false
		}
		// IndexExpr controls its own bracket indentation decisions.
		return exprDocInfo{Doc: doc, NeedsContinuationIndent: false}, true
	case *ast.SliceExpr:
		if kind != exprDocKindCallArg {
			return exprDocInfo{}, false
		}
		doc, ok := sliceExprDoc(e, ctx, kind)
		if !ok {
			return exprDocInfo{}, false
		}
		// SliceExpr controls its own bracket indentation decisions.
		return exprDocInfo{Doc: doc, NeedsContinuationIndent: false}, true
	case *ast.Ident, *ast.BasicLit:
		// Render atomic expressions as-is. These are safe to embed as docs but do
		// not participate in internal breaking yet.
		return exprDocInfo{Doc: layout.T(renderNode(expr, ctx.Fset)), NeedsContinuationIndent: false}, true
	default:
		return exprDocInfo{}, false
	}
}

func indentExprDocIfNeeded(info exprDocInfo) layout.Doc {
	if info.NeedsContinuationIndent {
		return layout.N("\t", info.Doc)
	}
	return info.Doc
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
	return methodChainDocWithKind(call, ctx, exprDocKindTopLevel)
}

func methodChainDocWithKind(call *ast.CallExpr, ctx *Context, kind exprDocKind) (layout.Doc, bool) {
	if call == nil {
		return nil, false
	}

	// Method chains are a series of CallExpr nodes whose Fun is a SelectorExpr
	// and whose receiver is another CallExpr (except for the first).
	type segment struct {
		name     string
		args     []ast.Expr
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

		seg := segment{name: sel.Sel.Name, ellipsis: cur.Ellipsis.IsValid(), args: cur.Args}
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

		// In call-arg context, allow formatting the per-segment argument lists
		// using the same layout approach as generic calls.
		if kind == exprDocKindCallArg && len(seg.args) > 0 {
			var argDocs []layout.Doc
			for i, arg := range seg.args {
				argText := renderNode(arg, ctx.Fset)
				if hasAnyComment(argText) {
					return nil, false
				}

				// If the arg already contains newlines (e.g. produced by a previous
				// layout pass), only proceed if we can represent it structurally. This
				// avoids mixing “preformatted” spans with stringified fallbacks.
				if info, ok := exprDocWithKind(arg, ctx, kind); ok {
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

				if strings.Contains(argText, "\n") {
					return nil, false
				}

				if i > 0 {
					argDocs = append(argDocs, layout.T(","), layout.L())
				}
				argDocs = append(argDocs, layout.T(argText))
			}

			// Handle ellipsis call syntax: f(args...)
			if seg.ellipsis && len(argDocs) > 0 {
				argDocs[len(argDocs)-1] = layout.C(argDocs[len(argDocs)-1], layout.T("..."))
			}

			argsGroup := layout.G(layout.C(
				layout.SL(),
				layout.C(argDocs...),
			))

			docs = append(docs, layout.N("\t", argsGroup))
			// Keep `)` tightly coupled to the last token of the last argument to
			// avoid semicolon-insertion hazards. In particular, avoid producing a
			// line that starts with `)` after an identifier/literal on the previous
			// line (which would make the source unparseable).
			docs = append(docs, layout.T(")"))
			continue
		}

		// Default behavior (top-level / parity): keep argument lists on one line.
		if len(seg.args) > 0 {
			for i, arg := range seg.args {
				argText := renderNode(arg, ctx.Fset)
				if strings.Contains(argText, "\n") {
					return nil, false
				}
				if i > 0 {
					docs = append(docs, layout.T(", "))
				}
				docs = append(docs, layout.T(argText))
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

	funDoc := layout.T(renderNode(call.Fun, ctx.Fset))
	// Prefer structured docs for the callee too (useful for generic instantiation
	// expressions like `f[T, U]`), but keep it tightly coupled to the `(` to
	// avoid semicolon-insertion hazards (`f\n(` is not valid Go).
	if call.Fun != nil {
		if info, ok := exprDocWithKind(call.Fun, ctx, exprDocKindCallArg); ok {
			funDoc = info.Doc
		}
	}

	// Be conservative: skip any comment-containing args, or args that already
	// contain newlines (we don't try to reindent nested multiline spans yet).
	var argDocs []layout.Doc
	for i, arg := range call.Args {
		argText := renderNode(arg, ctx.Fset)
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

		if strings.Contains(argText, "\n") {
			return nil, false
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
		funDoc,
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
	//        inner)
	//
	// Note: we intentionally do not place `)` on its own line; doing so can break
	// parsing due to Go's semicolon insertion.
	return layout.G(layout.C(
		layout.T("("),
		layout.N("\t", layout.G(layout.C(layout.SL(), inner))),
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
	typeDoc := layout.T("")
	if lit.Type != nil {
		typeText = renderNode(lit.Type, ctx.Fset)
		if strings.Contains(typeText, "\n") || hasAnyComment(typeText) {
			return nil, false
		}

		typeDoc = layout.T(typeText)
		// Prefer structured docs for generic instantiations like `T[A, B]`, but
		// keep the opening `{` tightly coupled to the type to avoid semicolon
		// insertion hazards.
		if info, ok := exprDocWithKind(lit.Type, ctx, kind); ok {
			typeDoc = info.Doc
		}
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
		typeDoc,
		layout.T("{"),
		layout.N("\t", body),
		layout.SL(),
		layout.T("}"),
	)), true
}

func keyValueExprDoc(kv *ast.KeyValueExpr, ctx *Context, kind exprDocKind) (layout.Doc, bool) {
	if kv == nil || ctx == nil || kv.Key == nil || kv.Value == nil {
		return nil, false
	}

	keyText := renderNode(kv.Key, ctx.Fset)
	if strings.Contains(keyText, "\n") || hasAnyComment(keyText) {
		return nil, false
	}

	// Prefer a structured doc for the value (call-arg context).
	valueText := renderNode(kv.Value, ctx.Fset)
	if strings.Contains(valueText, "\n") || hasAnyComment(valueText) {
		return nil, false
	}

	valueDoc := layout.T(valueText)
	if info, ok := exprDocWithKind(kv.Value, ctx, kind); ok {
		valueDoc = info.Doc
	}

	// Keep `Key: ` on the same line, but allow value to break with an extra
	// continuation indentation.
	return layout.G(layout.C(
		layout.T(keyText),
		layout.T(": "),
		layout.N("\t", valueDoc),
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
		left = indentExprDocIfNeeded(leftInfo)
	}

	rightInfo, rightOK := exprDocWithKind(bin.Y, ctx, kind)
	right := layout.T(renderNode(bin.Y, ctx.Fset))
	if rightOK {
		right = indentExprDocIfNeeded(rightInfo)
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
	return sameOpBinaryChainDocWithKind(bin, ctx, exprDocKindTopLevel)
}

func isComparisonOp(op token.Token) bool {
	switch op {
	case token.EQL, token.NEQ, token.LSS, token.GTR, token.LEQ, token.GEQ:
		return true
	default:
		return false
	}
}

func comparisonBinaryExprDoc(bin *ast.BinaryExpr, ctx *Context, kind exprDocKind) (layout.Doc, bool) {
	if bin == nil || ctx == nil || bin.X == nil || bin.Y == nil {
		return nil, false
	}
	if !isComparisonOp(bin.Op) {
		return nil, false
	}

	leftText := renderNode(bin.X, ctx.Fset)
	rightText := renderNode(bin.Y, ctx.Fset)
	if strings.Contains(leftText, "\n") || strings.Contains(rightText, "\n") {
		return nil, false
	}
	if hasAnyComment(leftText) || hasAnyComment(rightText) {
		return nil, false
	}

	leftDoc := layout.T(leftText)
	if info, ok := exprDocWithKind(bin.X, ctx, kind); ok {
		leftDoc = indentExprDocIfNeeded(info)
	}

	rightDoc := layout.T(rightText)
	if info, ok := exprDocWithKind(bin.Y, ctx, kind); ok {
		rightDoc = indentExprDocIfNeeded(info)
	}

	// flat:  left == right
	// break: left ==\nright
	return layout.G(layout.C(
		leftDoc,
		layout.T(" "),
		layout.T(bin.Op.String()),
		layout.SL(),
		rightDoc,
	)), true
}

func sameOpBinaryChainDocWithKind(bin *ast.BinaryExpr, ctx *Context, kind exprDocKind) (layout.Doc, bool) {
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

		// In call-arg context, prefer structured docs for terms so selector/method
		// chains can break within the binary expression.
		if kind == exprDocKindCallArg {
			if info, ok := exprDocWithKind(term, ctx, kind); ok {
				docs = append(docs, indentExprDocIfNeeded(info))
				continue
			}
		}

		docs = append(docs, layout.T(renderNode(term, ctx.Fset)))
	}
	return layout.G(layout.C(docs...)), true
}

func indexExprDoc(idx *ast.IndexExpr, ctx *Context, kind exprDocKind) (layout.Doc, bool) {
	if idx == nil || ctx == nil || idx.X == nil || idx.Index == nil {
		return nil, false
	}

	baseText := renderNode(idx.X, ctx.Fset)
	if strings.Contains(baseText, "\n") || hasAnyComment(baseText) {
		return nil, false
	}

	baseDoc := layout.T(baseText)
	if info, ok := exprDocWithKind(idx.X, ctx, kind); ok {
		baseDoc = indentExprDocIfNeeded(info)
	}

	indexText := renderNode(idx.Index, ctx.Fset)
	if strings.Contains(indexText, "\n") || hasAnyComment(indexText) {
		return nil, false
	}

	indexDoc := layout.T(indexText)
	if info, ok := exprDocWithKind(idx.Index, ctx, kind); ok {
		indexDoc = indentExprDocIfNeeded(info)
	}

	// flat:  a[b]
	// break:
	//   a[
	//       b
	//   ]
	return layout.G(layout.C(
		baseDoc,
		layout.T("["),
		layout.N("\t", layout.G(layout.C(layout.SL(), indexDoc))),
		layout.T("]"),
	)), true
}

func indexListExprDoc(idx *ast.IndexListExpr, ctx *Context, kind exprDocKind) (layout.Doc, bool) {
	if idx == nil || ctx == nil || idx.X == nil || len(idx.Indices) == 0 {
		return nil, false
	}

	baseText := renderNode(idx.X, ctx.Fset)
	if strings.Contains(baseText, "\n") || hasAnyComment(baseText) {
		return nil, false
	}

	baseDoc := layout.T(baseText)
	if info, ok := exprDocWithKind(idx.X, ctx, kind); ok {
		baseDoc = indentExprDocIfNeeded(info)
	}

	var indexDocs []layout.Doc
	for i, index := range idx.Indices {
		indexText := renderNode(index, ctx.Fset)
		if strings.Contains(indexText, "\n") || hasAnyComment(indexText) {
			return nil, false
		}

		indexDoc := layout.T(indexText)
		if info, ok := exprDocWithKind(index, ctx, kind); ok {
			indexDoc = indentExprDocIfNeeded(info)
		}

		if i > 0 {
			indexDocs = append(indexDocs, layout.T(","), layout.L())
		}
		indexDocs = append(indexDocs, indexDoc)
	}

	inner := layout.G(layout.C(layout.SL(), layout.C(indexDocs...)))

	// flat:  x[T, U]
	// break:
	//   x[T,
	//       U]
	//
	// Note: we intentionally do not put `]` on its own line; doing so can break
	// parsing due to Go's semicolon insertion.
	return layout.G(layout.C(
		baseDoc,
		layout.T("["),
		layout.N("\t", inner),
		layout.T("]"),
	)), true
}

func unaryExprDoc(u *ast.UnaryExpr, ctx *Context, kind exprDocKind) (exprDocInfo, bool) {
	if u == nil || ctx == nil || u.X == nil {
		return exprDocInfo{}, false
	}

	operandText := renderNode(u.X, ctx.Fset)
	if strings.Contains(operandText, "\n") || hasAnyComment(operandText) {
		return exprDocInfo{}, false
	}

	operandInfo, ok := exprDocWithKind(u.X, ctx, kind)
	if !ok {
		return exprDocInfo{}, false
	}

	// Keep the operator tightly coupled to the operand. Internal breaking (if
	// any) is owned by the operand doc.
	doc := layout.G(layout.C(
		layout.T(u.Op.String()),
		operandInfo.Doc,
	))
	return exprDocInfo{Doc: doc, NeedsContinuationIndent: operandInfo.NeedsContinuationIndent}, true
}

func starExprDoc(s *ast.StarExpr, ctx *Context, kind exprDocKind) (exprDocInfo, bool) {
	if s == nil || ctx == nil || s.X == nil {
		return exprDocInfo{}, false
	}

	operandText := renderNode(s.X, ctx.Fset)
	if strings.Contains(operandText, "\n") || hasAnyComment(operandText) {
		return exprDocInfo{}, false
	}

	operandInfo, ok := exprDocWithKind(s.X, ctx, kind)
	if !ok {
		return exprDocInfo{}, false
	}

	doc := layout.G(layout.C(
		layout.T("*"),
		operandInfo.Doc,
	))
	return exprDocInfo{Doc: doc, NeedsContinuationIndent: operandInfo.NeedsContinuationIndent}, true
}

func typeAssertExprDoc(t *ast.TypeAssertExpr, ctx *Context, kind exprDocKind) (layout.Doc, bool) {
	if t == nil || ctx == nil || t.X == nil || t.Type == nil {
		return nil, false
	}

	recvText := renderNode(t.X, ctx.Fset)
	if strings.Contains(recvText, "\n") || hasAnyComment(recvText) {
		return nil, false
	}

	recvDoc := layout.T(recvText)
	if info, ok := exprDocWithKind(t.X, ctx, kind); ok {
		recvDoc = indentExprDocIfNeeded(info)
	}

	typeText := renderNode(t.Type, ctx.Fset)
	if strings.Contains(typeText, "\n") || hasAnyComment(typeText) {
		return nil, false
	}

	typeDoc := layout.T(typeText)
	if info, ok := exprDocWithKind(t.Type, ctx, kind); ok {
		typeDoc = indentExprDocIfNeeded(info)
	}

	// flat:  x.(T)
	// break:
	//   x.(
	//       T)
	//
	// Note: we intentionally do not allow a newline between `x` and `.(` (Go
	// semicolon insertion hazard), and we intentionally do not put `)` on its own
	// line.
	return layout.G(layout.C(
		recvDoc,
		layout.T(".("),
		layout.N("\t", layout.G(layout.C(layout.SL(), typeDoc))),
		layout.T(")"),
	)), true
}

func sliceExprDoc(s *ast.SliceExpr, ctx *Context, kind exprDocKind) (layout.Doc, bool) {
	if s == nil || ctx == nil || s.X == nil {
		return nil, false
	}

	baseText := renderNode(s.X, ctx.Fset)
	if strings.Contains(baseText, "\n") || hasAnyComment(baseText) {
		return nil, false
	}

	baseDoc := layout.T(baseText)
	if info, ok := exprDocWithKind(s.X, ctx, kind); ok {
		baseDoc = indentExprDocIfNeeded(info)
	}

	partDoc := func(e ast.Expr) (layout.Doc, bool) {
		text := renderNode(e, ctx.Fset)
		if strings.Contains(text, "\n") || hasAnyComment(text) {
			return nil, false
		}
		if info, ok := exprDocWithKind(e, ctx, kind); ok {
			return indentExprDocIfNeeded(info), true
		}
		return layout.T(text), true
	}

	var docs []layout.Doc

	// low:
	if s.Low != nil {
		d, ok := partDoc(s.Low)
		if !ok {
			return nil, false
		}
		docs = append(docs, d)
	}

	// Always emit the first colon for slice expressions.
	docs = append(docs, layout.T(":"))

	// high:
	if s.High != nil {
		d, ok := partDoc(s.High)
		if !ok {
			return nil, false
		}
		docs = append(docs, d)
	}

	// max:
	if s.Slice3 {
		docs = append(docs, layout.T(":"))
		if s.Max != nil {
			d, ok := partDoc(s.Max)
			if !ok {
				return nil, false
			}
			docs = append(docs, d)
		}
	}

	inner := layout.C(docs...)

	// flat:  a[low:high]
	// break:
	//   a[
	//       low:high
	//   ]
	return layout.G(layout.C(
		baseDoc,
		layout.T("["),
		layout.N("\t", layout.G(layout.C(layout.SL(), inner))),
		layout.T("]"),
	)), true
}
