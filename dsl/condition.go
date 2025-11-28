package dsl

import (
	"go/ast"
)

// TrueCond always returns true (no condition / always applies).
type TrueCond struct{}

// Eval implements Condition for TrueCond.
func (c TrueCond) Eval(caps Captures, ctx *Context) bool {
	return true
}

// FalseCond always returns false (never applies).
type FalseCond struct{}

// Eval implements Condition for FalseCond.
func (c FalseCond) Eval(caps Captures, ctx *Context) bool {
	return false
}

// AndCond combines conditions with AND.
type AndCond struct {
	Conds []Condition
}

// Eval implements Condition for AndCond.
func (c *AndCond) Eval(caps Captures, ctx *Context) bool {
	for _, cond := range c.Conds {
		if !cond.Eval(caps, ctx) {
			return false
		}
	}
	return true
}

// OrCond combines conditions with OR.
type OrCond struct {
	Conds []Condition
}

// Eval implements Condition for OrCond.
func (c *OrCond) Eval(caps Captures, ctx *Context) bool {
	for _, cond := range c.Conds {
		if cond.Eval(caps, ctx) {
			return true
		}
	}
	return false
}

// NotCond negates a condition.
type NotCond struct {
	Cond Condition
}

// Eval implements Condition for NotCond.
func (c *NotCond) Eval(caps Captures, ctx *Context) bool {
	return !c.Cond.Eval(caps, ctx)
}

// LineWidthCond checks if a node's line width exceeds a threshold.
type LineWidthCond struct {
	Target string // Capture name (without $) or "node" for matched node
	Op     string // ">", "<", ">=", "<=", "=="
	Value  int    // If 0, uses ctx.ColumnLimit
}

// Eval implements Condition for LineWidthCond.
func (c *LineWidthCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	if node == nil {
		return false
	}

	width := ctx.LineWidth(node)
	threshold := c.Value
	if threshold == 0 {
		threshold = ctx.ColumnLimit
	}

	return compareInt(width, c.Op, threshold)
}

// NodeWidthCond checks if a node's total width exceeds a threshold.
type NodeWidthCond struct {
	Target string
	Op     string
	Value  int
}

// Eval implements Condition for NodeWidthCond.
func (c *NodeWidthCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	if node == nil {
		return false
	}

	width := ctx.NodeWidth(node)
	threshold := c.Value
	if threshold == 0 {
		threshold = ctx.ColumnLimit
	}

	return compareInt(width, c.Op, threshold)
}

// IsSimpleLiteralCond checks if a node is a simple literal (number, bool, nil).
type IsSimpleLiteralCond struct {
	Target string
}

// Eval implements Condition for IsSimpleLiteralCond.
func (c *IsSimpleLiteralCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	return isSimpleLiteral(node)
}

// isSimpleLiteral checks if a node is a simple literal value.
func isSimpleLiteral(n ast.Node) bool {
	switch node := n.(type) {
	case *ast.BasicLit:
		// Numbers and strings
		return true
	case *ast.Ident:
		// true, false, nil
		return node.Name == "true" || node.Name == "false" || node.Name == "nil"
	case *ast.UnaryExpr:
		// -1, +1 etc.
		if lit, ok := node.X.(*ast.BasicLit); ok {
			return lit != nil
		}
	}
	return false
}

// HasCallExprCond checks if a node contains a function call.
type HasCallExprCond struct {
	Target string
}

// Eval implements Condition for HasCallExprCond.
func (c *HasCallExprCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	return containsCallExpr(node)
}

// containsCallExpr checks if n contains any CallExpr.
func containsCallExpr(n ast.Node) bool {
	if n == nil {
		return false
	}
	found := false
	ast.Inspect(n, func(node ast.Node) bool {
		if _, ok := node.(*ast.CallExpr); ok {
			found = true
			return false // stop walking
		}
		return !found
	})
	return found
}

// IsCallExprCond checks if a node is a function call.
type IsCallExprCond struct {
	Target string
}

// Eval implements Condition for IsCallExprCond.
func (c *IsCallExprCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	_, ok := node.(*ast.CallExpr)
	return ok
}

// IsBinaryExprCond checks if a node is a binary expression.
type IsBinaryExprCond struct {
	Target string
}

// Eval implements Condition for IsBinaryExprCond.
func (c *IsBinaryExprCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	_, ok := node.(*ast.BinaryExpr)
	return ok
}

// OpIsCond checks if a binary/unary expression has a specific operator.
type OpIsCond struct {
	Target    string
	Operators []string // List of operators to match
}

// Eval implements Condition for OpIsCond.
func (c *OpIsCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	if node == nil {
		return false
	}

	var opStr string
	switch n := node.(type) {
	case *ast.BinaryExpr:
		opStr = n.Op.String()
	case *ast.UnaryExpr:
		opStr = n.Op.String()
	default:
		return false
	}

	for _, op := range c.Operators {
		if opStr == op {
			return true
		}
	}
	return false
}

// IsComparisonOpCond checks if a binary expression uses a comparison operator.
type IsComparisonOpCond struct {
	Target string
}

// Eval implements Condition for IsComparisonOpCond.
func (c *IsComparisonOpCond) Eval(caps Captures, ctx *Context) bool {
	return (&OpIsCond{
		Target:    c.Target,
		Operators: []string{"==", "!=", "<", ">", "<=", ">="},
	}).Eval(caps, ctx)
}

// IsLogicalOpCond checks if a binary expression uses a logical operator.
type IsLogicalOpCond struct {
	Target string
}

// Eval implements Condition for IsLogicalOpCond.
func (c *IsLogicalOpCond) Eval(caps Captures, ctx *Context) bool {
	return (&OpIsCond{
		Target:    c.Target,
		Operators: []string{"&&", "||"},
	}).Eval(caps, ctx)
}

// IsArithmeticOpCond checks if a binary expression uses an arithmetic operator.
type IsArithmeticOpCond struct {
	Target string
}

// Eval implements Condition for IsArithmeticOpCond.
func (c *IsArithmeticOpCond) Eval(caps Captures, ctx *Context) bool {
	return (&OpIsCond{
		Target:    c.Target,
		Operators: []string{"+", "-", "*", "/", "%"},
	}).Eval(caps, ctx)
}

// Helper functions

func resolveTarget(caps Captures, target string) ast.Node {
	if target == "node" || target == "$node" {
		return caps["node"]
	}
	// Remove $ prefix if present
	if len(target) > 0 && target[0] == '$' {
		target = target[1:]
	}
	return caps[target]
}

func compareInt(value int, op string, threshold int) bool {
	switch op {
	case ">":
		return value > threshold
	case ">=":
		return value >= threshold
	case "<":
		return value < threshold
	case "<=":
		return value <= threshold
	case "==":
		return value == threshold
	case "!=":
		return value != threshold
	default:
		return false
	}
}
