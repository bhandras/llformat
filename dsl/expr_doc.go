package dsl

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/lightninglabs/llformat/dsl/layout"
)

// exprDoc is a small AST-to-layout.Doc builder for a limited subset of Go
// expressions.
//
// This is intentionally conservative and is only meant to support the current
// "modern layout" actions (selector chains, method chains, same-op binary
// chains). It returns ok=false for unsupported forms so callers can fall back
// to legacy/parity logic.
func exprDoc(expr ast.Expr, ctx *Context) (doc layout.Doc, ok bool) {
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		return selectorChainDoc(e, ctx)
	case *ast.CallExpr:
		// Only method-call chains for now.
		return methodChainDoc(e, ctx)
	case *ast.BinaryExpr:
		return sameOpBinaryChainDoc(e, ctx)
	default:
		return nil, false
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

