package dsl

import (
	"go/ast"
	"go/token"
	"strings"
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

// EditAction is an optional interface for actions that can express changes as a
// list of validated byte-range edits rather than returning a whole rewritten
// file. The engine will apply these edits to ctx.Source.
type EditAction interface {
	ExecuteEdits(caps Captures, ctx *Context) ([]Edit, bool, error)
}

// NodeOrder controls the order the DSL engine considers nodes when searching for
// a matching rule.
type NodeOrder int

const (
	// NodeOrderPreorder processes nodes in AST pre-order (current default).
	NodeOrderPreorder NodeOrder = iota

	// NodeOrderSourceOrder processes nodes in ascending source offset order.
	// This more closely matches behavior of the legacy scanner-based formatters
	// which operate left-to-right through the file.
	NodeOrderSourceOrder
)

// Context provides formatting context to conditions and actions.
type Context struct {
	Fset        *token.FileSet
	Source      []byte
	ColumnLimit int
	TabStop     int

	// atomicNodes tracks nodes that should not be broken.
	atomicNodes map[ast.Node]bool

	// parentMap maps each node to its parent in the AST.
	parentMap map[ast.Node]ast.Node

	// LastAppliedRule records the most recent transforming rule that applied.
	// This is used for optional tracing.
	LastAppliedRule string

	// LastAppliedRulePriority is the priority of the last applied rule.
	LastAppliedRulePriority int

	// LastAppliedNodeType is a short node type label (e.g. "*ast.CallExpr").
	LastAppliedNodeType string

	// LastAppliedNodeStart/End are byte offsets into Source for the node that
	// triggered the last applied rule.
	LastAppliedNodeStart int
	LastAppliedNodeEnd   int
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

// SetParentMap sets the parent map for the context.
func (ctx *Context) SetParentMap(m map[ast.Node]ast.Node) {
	ctx.parentMap = m
}

// Parent returns the parent node of n, or nil if not found.
func (ctx *Context) Parent(n ast.Node) ast.Node {
	if ctx.parentMap == nil {
		return nil
	}
	return ctx.parentMap[n]
}

// IsChildOfCallExpr checks if node n is a direct child (argument) of a CallExpr.
func (ctx *Context) IsChildOfCallExpr(n ast.Node) bool {
	parent := ctx.Parent(n)
	if parent == nil {
		return false
	}
	call, ok := parent.(*ast.CallExpr)
	if !ok {
		return false
	}
	// Check if n is one of the arguments
	for _, arg := range call.Args {
		if arg == n {
			return true
		}
	}
	return false
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
	return VisualLen(s, tabStop)
}

// VisualLen calculates the visual width of a string with tab expansion.
func VisualLen(s string, tabStop int) int {
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

// hasLineComment checks if a string contains a line comment (//) outside of
// string literals. This is used to detect inline comments that would be lost
// during reformatting.
func hasLineComment(s string) bool {
	inStr := byte(0)
	esc := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr != 0 {
			if inStr == '"' && c == '\\' && !esc {
				esc = true
				continue
			}
			if esc {
				esc = false
			} else if c == inStr {
				inStr = 0
			}
			continue
		}
		switch c {
		case '"', '`':
			inStr = c
		case '/':
			if i+1 < len(s) && s[i+1] == '/' {
				return true
			}
		}
	}
	return false
}

// hasBlockComment checks if a string contains a block comment (/* */) outside of
// string literals. This is used to detect inline comments that would be lost
// during reformatting.
func hasBlockComment(s string) bool {
	inStr := byte(0)
	esc := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr != 0 {
			if inStr == '"' && c == '\\' && !esc {
				esc = true
				continue
			}
			if esc {
				esc = false
			} else if c == inStr {
				inStr = 0
			}
			continue
		}
		switch c {
		case '"', '`':
			inStr = c
		case '/':
			if i+1 < len(s) && s[i+1] == '*' {
				return true
			}
		}
	}
	return false
}

// anyLineExceedsLimit checks if any line in the given string exceeds the column limit.
func anyLineExceedsLimit(s string, colLimit, tabStop int) bool {
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		if visualLen(line, tabStop) > colLimit {
			return true
		}
	}
	return false
}
