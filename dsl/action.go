package dsl

import (
	"bytes"
	"go/ast"
	"go/printer"
	"go/token"
	"strings"
)

// NoOpAction does nothing (used for keep_together which just marks nodes).
type NoOpAction struct{}

// Execute implements Action for NoOpAction.
func (a *NoOpAction) Execute(caps Captures, ctx *Context) ([]byte, bool) {
	return nil, false
}

// KeepTogetherAction marks a node as atomic (won't be broken by other rules).
type KeepTogetherAction struct {
	Target string
}

// Execute implements Action for KeepTogetherAction.
func (a *KeepTogetherAction) Execute(caps Captures, ctx *Context) ([]byte, bool) {
	node := resolveTarget(caps, a.Target)
	if node != nil {
		ctx.MarkAtomic(node)
	}
	// This doesn't change source, just marks the node
	return nil, false
}

// TryElseAction tries the first action, falls back to second if it doesn't help.
type TryElseAction struct {
	Try  Action
	Else Action
}

// Execute implements Action for TryElseAction.
func (a *TryElseAction) Execute(caps Captures, ctx *Context) ([]byte, bool) {
	result, changed := a.Try.Execute(caps, ctx)
	if changed {
		return result, true
	}
	return a.Else.Execute(caps, ctx)
}

// SequenceAction executes actions in sequence until one succeeds.
type SequenceAction struct {
	Actions []Action
}

// Execute implements Action for SequenceAction.
func (a *SequenceAction) Execute(caps Captures, ctx *Context) ([]byte, bool) {
	for _, action := range a.Actions {
		result, changed := action.Execute(caps, ctx)
		if changed {
			return result, true
		}
	}
	return nil, false
}

// ReflowStrategy defines how to reformat a function call.
type ReflowStrategy string

const (
	// StrategyOnePerLine puts each argument on its own line.
	StrategyOnePerLine ReflowStrategy = "one-per-line"
	// StrategyLeftPack packs arguments greedily from left.
	StrategyLeftPack ReflowStrategy = "left-pack"
	// StrategyAdaptive uses one-per-line if any arg is multiline, else left-pack.
	StrategyAdaptive ReflowStrategy = "adaptive"
)

// ReflowCallAction reformats a function call.
type ReflowCallAction struct {
	Target   string
	Strategy ReflowStrategy
}

// Execute implements Action for ReflowCallAction.
func (a *ReflowCallAction) Execute(caps Captures, ctx *Context) ([]byte, bool) {
	node := resolveTarget(caps, a.Target)
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return nil, false
	}

	// Get source positions
	start := ctx.Fset.Position(call.Pos()).Offset
	end := ctx.Fset.Position(call.End()).Offset
	if start < 0 || end > len(ctx.Source) || start >= end {
		return nil, false
	}

	// Check if the line already fits (not just the node)
	original := string(ctx.Source[start:end])
	if ctx.LineWidth(call) <= ctx.ColumnLimit {
		return nil, false
	}

	// Get indentation
	indent := ctx.IndentAt(call)

	// Format the call
	var formatted string
	switch a.Strategy {
	case StrategyOnePerLine:
		formatted = formatCallOnePerLine(call, indent, ctx)
	case StrategyLeftPack:
		formatted = formatCallLeftPack(call, indent, ctx)
	case StrategyAdaptive:
		formatted = formatCallAdaptive(call, indent, ctx)
	default:
		formatted = formatCallOnePerLine(call, indent, ctx)
	}

	// Check if formatting actually changed anything
	if formatted == original {
		return nil, false
	}

	// Build result by replacing the call in source
	var result bytes.Buffer
	result.Write(ctx.Source[:start])
	result.WriteString(formatted)
	result.Write(ctx.Source[end:])

	return result.Bytes(), true
}

// formatCallOnePerLine formats a call with each argument on its own line.
func formatCallOnePerLine(call *ast.CallExpr, indent string, ctx *Context) string {
	if len(call.Args) == 0 {
		return renderNode(call, ctx.Fset)
	}

	var b strings.Builder

	// Write function name and opening paren
	funcSrc := renderNode(call.Fun, ctx.Fset)
	b.WriteString(funcSrc)
	b.WriteString("(\n")

	argIndent := indent + "\t"

	for i, arg := range call.Args {
		b.WriteString(argIndent)
		argSrc := renderNode(arg, ctx.Fset)
		// Handle multi-line arguments by re-indenting
		lines := strings.Split(argSrc, "\n")
		for j, line := range lines {
			if j > 0 {
				b.WriteString("\n")
				b.WriteString(argIndent)
			}
			b.WriteString(strings.TrimSpace(line))
		}
		b.WriteString(",")
		if i < len(call.Args)-1 {
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(indent)
	b.WriteString(")")

	return b.String()
}

// formatCallLeftPack formats a call by packing arguments greedily.
func formatCallLeftPack(call *ast.CallExpr, indent string, ctx *Context) string {
	if len(call.Args) == 0 {
		return renderNode(call, ctx.Fset)
	}

	var b strings.Builder

	// Write function name and opening paren
	funcSrc := renderNode(call.Fun, ctx.Fset)
	b.WriteString(funcSrc)
	b.WriteString("(\n")

	argIndent := indent + "\t"
	indentWidth := visualLen(argIndent, ctx.TabStop)

	lineWidth := indentWidth
	firstOnLine := true

	for i, arg := range call.Args {
		argSrc := renderNode(arg, ctx.Fset)
		argWidth := visualLen(argSrc, ctx.TabStop)

		if firstOnLine {
			b.WriteString(argIndent)
			b.WriteString(argSrc)
			lineWidth = indentWidth + argWidth
			firstOnLine = false
		} else {
			// Check if this arg fits on current line
			// Need space for ", " + arg + potential trailing comma
			needed := 2 + argWidth
			if lineWidth+needed <= ctx.ColumnLimit {
				b.WriteString(", ")
				b.WriteString(argSrc)
				lineWidth += needed
			} else {
				// Start new line
				b.WriteString(",\n")
				b.WriteString(argIndent)
				b.WriteString(argSrc)
				lineWidth = indentWidth + argWidth
			}
		}

		// Add trailing comma after last arg
		if i == len(call.Args)-1 {
			b.WriteString(",")
		}
	}

	b.WriteString("\n")
	b.WriteString(indent)
	b.WriteString(")")

	return b.String()
}

// formatCallAdaptive chooses between one-per-line and left-pack.
func formatCallAdaptive(call *ast.CallExpr, indent string, ctx *Context) string {
	// Check if any argument is multi-line
	hasMultiLine := false
	for _, arg := range call.Args {
		argSrc := renderNode(arg, ctx.Fset)
		if strings.Contains(argSrc, "\n") {
			hasMultiLine = true
			break
		}
	}

	if hasMultiLine {
		return formatCallOnePerLine(call, indent, ctx)
	}
	return formatCallLeftPack(call, indent, ctx)
}

// BreakAfterAction inserts a line break after a node.
type BreakAfterAction struct {
	Target string
}

// Execute implements Action for BreakAfterAction.
func (a *BreakAfterAction) Execute(caps Captures, ctx *Context) ([]byte, bool) {
	node := resolveTarget(caps, a.Target)
	if node == nil {
		return nil, false
	}

	end := ctx.Fset.Position(node.End()).Offset
	if end < 0 || end > len(ctx.Source) {
		return nil, false
	}

	indent := ctx.IndentAt(node)

	var result bytes.Buffer
	result.Write(ctx.Source[:end])
	result.WriteString("\n")
	result.WriteString(indent)
	result.WriteString("\t") // continuation indent

	// Skip whitespace after the break point
	i := end
	for i < len(ctx.Source) && (ctx.Source[i] == ' ' || ctx.Source[i] == '\t') {
		i++
	}
	result.Write(ctx.Source[i:])

	return result.Bytes(), true
}

// BreakBeforeAction inserts a line break before a node.
type BreakBeforeAction struct {
	Target string
}

// Execute implements Action for BreakBeforeAction.
func (a *BreakBeforeAction) Execute(caps Captures, ctx *Context) ([]byte, bool) {
	node := resolveTarget(caps, a.Target)
	if node == nil {
		return nil, false
	}

	pos := ctx.Fset.Position(node.Pos()).Offset
	if pos < 0 || pos > len(ctx.Source) {
		return nil, false
	}

	indent := ctx.IndentAt(node)

	// Find start of whitespace before node
	start := pos
	for start > 0 && (ctx.Source[start-1] == ' ' || ctx.Source[start-1] == '\t') {
		start--
	}

	var result bytes.Buffer
	result.Write(ctx.Source[:start])
	result.WriteString("\n")
	result.WriteString(indent)
	result.WriteString("\t") // continuation indent
	result.Write(ctx.Source[pos:])

	return result.Bytes(), true
}

// BreakAtOpAction breaks a binary expression at the operator.
type BreakAtOpAction struct {
	Target     string
	BreakAfter bool // true = break after op (Go style), false = break before
}

// Execute implements Action for BreakAtOpAction.
func (a *BreakAtOpAction) Execute(caps Captures, ctx *Context) ([]byte, bool) {
	node := resolveTarget(caps, a.Target)
	binExpr, ok := node.(*ast.BinaryExpr)
	if !ok {
		return nil, false
	}

	opPos := ctx.Fset.Position(binExpr.OpPos).Offset
	if opPos < 0 || opPos > len(ctx.Source) {
		return nil, false
	}

	opLen := len(binExpr.Op.String())
	indent := ctx.IndentAt(binExpr)

	var result bytes.Buffer

	if a.BreakAfter {
		// Break after operator: "a && \n\t b"
		opEnd := opPos + opLen

		result.Write(ctx.Source[:opEnd])
		result.WriteString("\n")
		result.WriteString(indent)
		result.WriteString("\t")

		// Skip whitespace after operator
		i := opEnd
		for i < len(ctx.Source) && (ctx.Source[i] == ' ' || ctx.Source[i] == '\t') {
			i++
		}
		result.Write(ctx.Source[i:])
	} else {
		// Break before operator
		// Skip whitespace before operator
		start := opPos
		for start > 0 && (ctx.Source[start-1] == ' ' || ctx.Source[start-1] == '\t') {
			start--
		}

		result.Write(ctx.Source[:start])
		result.WriteString("\n")
		result.WriteString(indent)
		result.WriteString("\t")
		result.Write(ctx.Source[opPos:])
	}

	return result.Bytes(), true
}

// ReflowNestedCallsAction finds and reflows function calls within an expression.
type ReflowNestedCallsAction struct {
	Target   string
	Strategy ReflowStrategy
}

// Execute implements Action for ReflowNestedCallsAction.
func (a *ReflowNestedCallsAction) Execute(caps Captures, ctx *Context) ([]byte, bool) {
	node := resolveTarget(caps, a.Target)
	if node == nil {
		return nil, false
	}

	// Find the first call expression that would benefit from reflow
	var targetCall *ast.CallExpr
	ast.Inspect(node, func(n ast.Node) bool {
		if targetCall != nil {
			return false
		}
		if call, ok := n.(*ast.CallExpr); ok {
			// Check if this call is worth reflowing
			if len(call.Args) > 1 && ctx.NodeWidth(call) > ctx.ColumnLimit/2 {
				targetCall = call
				return false
			}
		}
		return true
	})

	if targetCall == nil {
		return nil, false
	}

	// Create temporary captures with the call
	tempCaps := make(Captures)
	for k, v := range caps {
		tempCaps[k] = v
	}
	tempCaps["target"] = targetCall

	return (&ReflowCallAction{
		Target:   "target",
		Strategy: a.Strategy,
	}).Execute(tempCaps, ctx)
}

// Helper to render an AST node back to source.
func renderNode(n ast.Node, fset *token.FileSet) string {
	var buf bytes.Buffer
	printer.Fprint(&buf, fset, n)
	return buf.String()
}
