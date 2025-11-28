package dsl

import (
	"go/ast"
	"go/token"
)

// Rule represents a formatting rule.
type Rule struct {
	Name     string
	Pattern  Pattern
	When     Condition
	Priority int
	Action   Action
}

// Pattern matches against Go AST nodes.
type Pattern interface {
	Match(n ast.Node, fset *token.FileSet) (Captures, bool)
}

// Captures holds captured nodes from pattern matching.
type Captures map[string]ast.Node

// Condition evaluates whether a rule should apply.
type Condition interface {
	Eval(caps Captures, ctx *Context) bool
}

// Action performs the formatting transformation.
type Action interface {
	Execute(caps Captures, ctx *Context) ([]byte, bool)
}

// Context provides formatting context to conditions and actions.
type Context struct {
	Fset        *token.FileSet
	Source      []byte
	ColumnLimit int
	TabStop     int

	// atomicNodes tracks nodes that should not be broken.
	atomicNodes map[ast.Node]bool
}

// NewContext creates a new formatting context.
func NewContext(fset *token.FileSet, source []byte, columnLimit, tabStop int) *Context {
	return &Context{
		Fset:        fset,
		Source:      source,
		ColumnLimit: columnLimit,
		TabStop:     tabStop,
		atomicNodes: make(map[ast.Node]bool),
	}
}

// MarkAtomic marks a node as atomic (should not be broken).
func (ctx *Context) MarkAtomic(n ast.Node) {
	if ctx.atomicNodes == nil {
		ctx.atomicNodes = make(map[ast.Node]bool)
	}
	ctx.atomicNodes[n] = true
}

// IsAtomic checks if a node is marked as atomic.
func (ctx *Context) IsAtomic(n ast.Node) bool {
	if ctx.atomicNodes == nil {
		return false
	}
	return ctx.atomicNodes[n]
}

// LineWidth returns the visual width of the line containing node n.
func (ctx *Context) LineWidth(n ast.Node) int {
	if n == nil {
		return 0
	}
	pos := ctx.Fset.Position(n.Pos())
	if pos.Offset < pos.Column {
		return 0
	}
	lineStart := pos.Offset - pos.Column + 1

	// Find end of line
	lineEnd := lineStart
	for lineEnd < len(ctx.Source) && ctx.Source[lineEnd] != '\n' {
		lineEnd++
	}

	line := string(ctx.Source[lineStart:lineEnd])
	return visualLen(line, ctx.TabStop)
}

// NodeWidth returns the visual width of a node rendered as single line.
func (ctx *Context) NodeWidth(n ast.Node) int {
	if n == nil {
		return 0
	}
	start := ctx.Fset.Position(n.Pos()).Offset
	end := ctx.Fset.Position(n.End()).Offset
	if start < 0 || end > len(ctx.Source) || start >= end {
		return 0
	}
	return visualLen(string(ctx.Source[start:end]), ctx.TabStop)
}

// IndentAt returns the indentation string at node n.
func (ctx *Context) IndentAt(n ast.Node) string {
	if n == nil {
		return ""
	}
	pos := ctx.Fset.Position(n.Pos())
	if pos.Offset < pos.Column {
		return ""
	}
	lineStart := pos.Offset - pos.Column + 1

	var indent []byte
	for i := lineStart; i < pos.Offset && i < len(ctx.Source); i++ {
		c := ctx.Source[i]
		if c == ' ' || c == '\t' {
			indent = append(indent, c)
		} else {
			break
		}
	}
	return string(indent)
}

// NodeSource returns the source code for a node.
func (ctx *Context) NodeSource(n ast.Node) []byte {
	if n == nil {
		return nil
	}
	start := ctx.Fset.Position(n.Pos()).Offset
	end := ctx.Fset.Position(n.End()).Offset
	if start < 0 || end > len(ctx.Source) || start >= end {
		return nil
	}
	return ctx.Source[start:end]
}

// visualLen calculates the visual width of a string with tab expansion.
func visualLen(s string, tabStop int) int {
	width := 0
	for _, c := range s {
		if c == '\t' {
			width += tabStop - (width % tabStop)
		} else {
			width++
		}
	}
	return width
}
