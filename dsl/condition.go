package dsl

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// CallArgsPolicy controls whether the expression stage may edit expressions
// that are contained within call argument expressions.
type CallArgsPolicy int

const (
	// CallArgsPolicyOff forbids edits inside call argument expressions.
	CallArgsPolicyOff CallArgsPolicy = iota

	// CallArgsPolicyAuto allows edits inside call argument expressions only
	// when the enclosing call is known to be ignored by later
	// call-formatting stages.
	CallArgsPolicyAuto

	// CallArgsPolicyForce always allows edits inside call argument
	// expressions.
	CallArgsPolicyForce
)

// IsCallFuncInListCond checks whether the target node is a CallExpr whose
// callee name matches one of the provided names (e.g. "foo", "pkg.Foo"). If the
// target is not a CallExpr, it returns false.
type IsCallFuncInListCond struct {
	Target string
	Names  []string
}

func (c *IsCallFuncInListCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return false
	}

	return stringInSlice(callExprFuncName(call), c.Names)
}

// IsCallFuncContainsAnyCond checks whether the target node is a CallExpr whose
// callee name contains any of the provided substrings. This matches legacy
// multiline-exclude semantics (strings.Contains).
type IsCallFuncContainsAnyCond struct {
	Target string
	Names  []string
}

func (c *IsCallFuncContainsAnyCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return false
	}
	name := callExprFuncName(call)
	if name == "" {
		return false
	}
	for _, sub := range c.Names {
		if sub == "" {
			continue
		}
		if strings.Contains(name, sub) {
			return true
		}
	}

	return false
}

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

// IsParseableCond checks whether the current ctx.Source parses as a Go file.
// This is primarily intended to gate file-scoped legacy fallback rules so they
// only run on parse failures.
type IsParseableCond struct {
	// Want indicates the desired parseability:
	// - Want == true => condition passes only if the source parses
	// - Want == false => condition passes only if the source does not parse
	Want bool
}

func (c *IsParseableCond) Eval(_ Captures, ctx *Context) bool {
	if ctx == nil {
		return false
	}
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "", ctx.Source, parser.ParseComments)
	parseable := err == nil
	if c.Want {
		return parseable
	}

	return !parseable
}

// IsInCallArgsCond checks whether the target node is contained within an
// argument expression of any enclosing CallExpr. This rejects not only direct
// call arguments, but also nested subexpressions inside an argument.
type IsInCallArgsCond struct {
	Target string
}

// Eval implements Condition for IsInCallArgsCond.
func (c *IsInCallArgsCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	if node == nil || ctx == nil {
		return false
	}

	cur := node
	for cur != nil {
		parent := ctx.Parent(cur)
		call, ok := parent.(*ast.CallExpr)
		if ok {
			for _, arg := range call.Args {
				if arg == cur {
					return true
				}
			}
		}
		cur = parent
	}

	return false
}

// ExprEditSafeCond is a conservative guard intended for the expression stage:
// it allows edits only when we are unlikely to interfere with other formatters
// (calls, composite literals, func literals) and when inline comments would not
// be lost.
type ExprEditSafeCond struct {
	Target string

	// CallArgsPolicy controls whether edits inside call argument
	// expressions are allowed.
	CallArgsPolicy CallArgsPolicy

	// CallArgsAllowlist is used when CallArgsPolicy == CallArgsPolicyAuto.
	// When set, edits are allowed only when the enclosing call's function
	// name matches an entry.
	CallArgsAllowlist []string
}

// Eval implements Condition for ExprEditSafeCond.
func (c *ExprEditSafeCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	if node == nil || ctx == nil {
		return false
	}

	// Avoid editing inside call arguments; call formatting is owned by the
	// call stages and AST-based rewrites can easily change call layout.
	if (&IsInCallArgsCond{Target: c.Target}).Eval(caps, ctx) {
		switch c.CallArgsPolicy {
		case CallArgsPolicyForce:

			// OK.
		case CallArgsPolicyAuto:
			call := enclosingCallForArg(node, ctx)
			if call == nil {
				return false
			}
			if !stringInSlice(
				callExprFuncName(call), c.CallArgsAllowlist,
			) {

				return false
			}

		default:
			return false
		}
	}

	// Avoid composite literals and func literals; these tend to be
	// formatting sensitive and are not owned by the expression stage.
	if (&IsAncestorTypeCond{
		Target: c.Target,
		Type:   "CompositeLit",
	}).Eval(caps,
		ctx,
	) {

		return false
	}
	if (&IsAncestorTypeCond{
		Target: c.Target,
		Type:   "FuncLit",
	}).Eval(caps,
		ctx,
	) {

		return false
	}

	// Avoid spans containing inline comments (AST printing does not
	// preserve them inside expressions/argument lists).
	if hasLineComment(string(ctx.NodeSource(node))) ||
		hasBlockComment(string(ctx.NodeSource(node))) {

		return false
	}

	return true
}

func stringInSlice(s string, list []string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}

	return false
}

func enclosingCallForArg(node ast.Node, ctx *Context) *ast.CallExpr {
	cur := node
	for cur != nil {
		parent := ctx.Parent(cur)
		call, ok := parent.(*ast.CallExpr)
		if ok {
			for _, arg := range call.Args {
				if arg == cur {
					return call
				}
			}
		}
		cur = parent
	}

	return nil
}

func callExprFuncName(call *ast.CallExpr) string {
	if call == nil {
		return ""
	}

	return callExprFuncNameFromExpr(call.Fun)
}

func callExprFuncNameFromExpr(fun ast.Expr) string {
	if fun == nil {
		return ""
	}

	switch v := fun.(type) {
	case *ast.Ident:
		return v.Name

	case *ast.SelectorExpr:
		// Try to build the same representation as the legacy call
		// scanners (including selectors like "pkg.Func").
		prefix := selectorPrefix(v.X)
		if prefix == "" {
			return v.Sel.Name
		}

		return prefix + "." + v.Sel.Name

	case *ast.IndexExpr:

		// Generic instantiation: f[T](...) is represented as
		// CallExpr{Fun: IndexExpr}.
		return callExprFuncNameFromExpr(v.X)

	case *ast.IndexListExpr:

		// Generic instantiation: f[T, U](...) is represented as
		// CallExpr{Fun: IndexListExpr}.
		return callExprFuncNameFromExpr(v.X)

	default:

		// Unknown callee shape (e.g. type conversions / other
		// expressions). Return empty to indicate "not
		// allowlist-addressable".
		return ""
	}
}

func selectorPrefix(x ast.Expr) string {
	switch v := x.(type) {
	case *ast.Ident:
		return v.Name

	case *ast.SelectorExpr:
		p := selectorPrefix(v.X)
		if p == "" {
			return v.Sel.Name
		}

		return p + "." + v.Sel.Name

	default:
		return ""
	}
}

// IsParentTypeCond checks if the target node's direct parent matches a given
// AST node type name (e.g. "CallExpr", "AssignStmt").
type IsParentTypeCond struct {
	Target string
	Type   string
}

// Eval implements Condition for IsParentTypeCond.
func (c *IsParentTypeCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	if node == nil || ctx == nil {
		return false
	}
	parent := ctx.Parent(node)
	if parent == nil {
		return false
	}

	return (&NodePattern{Type: c.Type}).matchType(parent)
}

// IsAncestorTypeCond checks if the target node has an ancestor that matches a
// given AST node type name.
type IsAncestorTypeCond struct {
	Target string
	Type   string
}

// Eval implements Condition for IsAncestorTypeCond.
func (c *IsAncestorTypeCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	if node == nil || ctx == nil {
		return false
	}
	for cur := node; cur != nil; cur = ctx.Parent(cur) {
		if (&NodePattern{Type: c.Type}).matchType(cur) {
			return true
		}
	}

	return false
}

// IsInAssignRHSCond checks whether the target node is a direct RHS expression
// in an assignment statement (AssignStmt).
type IsInAssignRHSCond struct {
	Target string
}

// Eval implements Condition for IsInAssignRHSCond.
func (c *IsInAssignRHSCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	if node == nil || ctx == nil {
		return false
	}

	cur := node
	for cur != nil {
		parent := ctx.Parent(cur)
		assign, ok := parent.(*ast.AssignStmt)
		if !ok {
			cur = parent
			continue
		}

		for _, rhs := range assign.Rhs {
			if rhs == cur {
				return true
			}
		}

		return false
	}

	return false
}

// IsInReturnResultsCond checks whether the target node is a direct result
// expression in a return statement.
type IsInReturnResultsCond struct {
	Target string
}

// Eval implements Condition for IsInReturnResultsCond.
func (c *IsInReturnResultsCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	if node == nil || ctx == nil {
		return false
	}

	cur := node
	for cur != nil {
		parent := ctx.Parent(cur)
		ret, ok := parent.(*ast.ReturnStmt)
		if !ok {
			cur = parent
			continue
		}
		for _, res := range ret.Results {
			if res == cur {
				return true
			}
		}

		return false
	}

	return false
}

// HasLineCommentCond checks whether the target node's source includes an inline
// line comment ("//") outside of string literals.
type HasLineCommentCond struct {
	Target string
}

// Eval implements Condition for HasLineCommentCond.
func (c *HasLineCommentCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	if node == nil || ctx == nil {
		return false
	}

	return hasLineComment(string(ctx.NodeSource(node)))
}

// HasAnyCommentCond checks whether the target node's source includes any
// comment token (line or block) outside of string literals. This is used to
// conservatively avoid AST-based rewrites that would drop comments inside
// spans.
type HasAnyCommentCond struct {
	Target string
}

// Eval implements Condition for HasAnyCommentCond.
func (c *HasAnyCommentCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	if node == nil || ctx == nil {
		return false
	}

	return hasAnyComment(string(ctx.NodeSource(node)))
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

// CollapsedWidthCond checks if a node, when collapsed to a single line
// (whitespace normalized), would exceed a threshold. This is useful for
// multiline nodes where the first line might be short but the total content is
// long.
type CollapsedWidthCond struct {
	Target string
	Op     string
	Value  int
}

// Eval implements Condition for CollapsedWidthCond.
func (c *CollapsedWidthCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	if node == nil {
		return false
	}

	// Get the node's source text
	start := ctx.Fset.Position(node.Pos()).Offset
	end := ctx.Fset.Position(node.End()).Offset
	if start < 0 || end > len(ctx.Source) || start >= end {
		return false
	}
	nodeText := string(ctx.Source[start:end])

	// Find the indent prefix (content before the node on the same line)
	lineStart := start
	for lineStart > 0 && ctx.Source[lineStart-1] != '\n' {
		lineStart--
	}
	indentPrefix := string(ctx.Source[lineStart:start])

	// Collapse whitespace to estimate single-line width
	flat := strings.Join(strings.Fields(nodeText), " ")
	collapsedLen := visualLen(indentPrefix, ctx.TabStop) +
		visualLen(flat, ctx.TabStop)

	threshold := c.Value
	if threshold == 0 {
		threshold = ctx.ColumnLimit
	}

	return compareInt(collapsedLen, c.Op, threshold)
}

// IsChainedCallReceiverCond checks whether a CallExpr is used as the receiver
// of another call in a method-chain-like expression, i.e. it appears as:
//
// <call>().Method(...)
//
// This is useful to prevent “double ownership” where both the receiver call and
// the full method chain are rewritten independently, causing oscillation.
type IsChainedCallReceiverCond struct {
	Target string
}

// Eval implements Condition for IsChainedCallReceiverCond.
func (c *IsChainedCallReceiverCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	if node == nil || ctx == nil {
		return false
	}

	call, ok := node.(*ast.CallExpr)
	if !ok || call == nil {
		return false
	}

	parent := ctx.Parent(call)
	sel, ok := parent.(*ast.SelectorExpr)
	if !ok || sel == nil {
		return false
	}

	grandparent := ctx.Parent(sel)
	_, ok = grandparent.(*ast.CallExpr)

	return ok
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
		return node.Name == "true" || node.Name == "false" ||
			node.Name == "nil"

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
		Target: c.Target,
		Operators: []string{
			"==",
			"!=",
			"<",
			">",
			"<=",
			">=",
		},
	}).Eval(caps,
		ctx,
	)
}

// IsLogicalOpCond checks if a binary expression uses a logical operator.
type IsLogicalOpCond struct {
	Target string
}

// Eval implements Condition for IsLogicalOpCond.
func (c *IsLogicalOpCond) Eval(caps Captures, ctx *Context) bool {
	return (&OpIsCond{
		Target: c.Target,
		Operators: []string{
			"&&",
			"||",
		},
	}).Eval(caps,
		ctx,
	)
}

// IsArithmeticOpCond checks if a binary expression uses an arithmetic operator.
type IsArithmeticOpCond struct {
	Target string
}

// Eval implements Condition for IsArithmeticOpCond.
func (c *IsArithmeticOpCond) Eval(caps Captures, ctx *Context) bool {
	return (&OpIsCond{
		Target: c.Target,
		Operators: []string{
			"+",
			"-",
			"*",
			"/",
			"%",
		},
	}).Eval(caps,
		ctx,
	)
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

// MaxSpanLineWidthCond checks the maximum visual width of any line spanned by a
// node's source range (including continuation lines for multiline constructs).
//
// This is useful for cases where:
//   - the node starts on a short line (so LineWidthCond is a false negative),
//     and
//   - collapsing whitespace produces a short estimate (so CollapsedWidthCond is
//     a false negative), but
//   - an indented continuation line still exceeds the column limit.
type MaxSpanLineWidthCond struct {
	Target string
	Op     string
	Value  int // If 0, uses ctx.ColumnLimit
}

func (c *MaxSpanLineWidthCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	if node == nil {
		return false
	}
	start := ctx.Fset.Position(node.Pos()).Offset
	end := ctx.Fset.Position(node.End()).Offset
	if start < 0 || end > len(ctx.Source) || start >= end {
		return false
	}

	width := maxVisualLineLenInSpan(ctx.Source, start, end, ctx.TabStop)

	threshold := c.Value
	if threshold == 0 {
		threshold = ctx.ColumnLimit
	}

	return compareInt(width, c.Op, threshold)
}

// IsOutermostBinaryExprCond checks if this binary expression is not a child of
// another binary expression with the same operator type. This prevents matching
// inner nodes in chains like "a && b && c".
type IsOutermostBinaryExprCond struct {
	Target string
}

// Eval implements Condition for IsOutermostBinaryExprCond.
func (c *IsOutermostBinaryExprCond) Eval(caps Captures, ctx *Context) bool {

	// This condition relies on tracking parent nodes during traversal For
	// now, always return true - the BreakAtOpAction handles finding the
	// best break point within the whole expression
	return true
}

// IsStringConcatCond checks if a binary expression involves string
// concatenation. String concatenation uses + but should be handled differently
// than arithmetic.
type IsStringConcatCond struct {
	Target string
}

// Eval implements Condition for IsStringConcatCond.
func (c *IsStringConcatCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)

	return isStringConcat(node)
}

// isStringConcat checks if a node is or contains string concatenation.
func isStringConcat(n ast.Node) bool {
	if n == nil {
		return false
	}
	found := false
	ast.Inspect(n, func(node ast.Node) bool {
		if bin, ok := node.(*ast.BinaryExpr); ok {
			// Check if either operand is a string literal
			if bin.Op.String() == "+" {
				if isStringLit(bin.X) || isStringLit(bin.Y) {
					found = true

					return false
				}
			}
		}

		return !found
	})

	return found
}

// isStringLit checks if a node is a string literal (directly or in concat
// chain).
func isStringLit(n ast.Node) bool {
	if lit, ok := n.(*ast.BasicLit); ok {

		// STRING is token type 9 in go/token
		return lit.Kind.String() == "STRING"
	}
	// Check nested binary + with strings
	if bin, ok := n.(*ast.BinaryExpr); ok && bin.Op.String() == "+" {
		return isStringLit(bin.X) || isStringLit(bin.Y)
	}

	return false
}

// IsCallArgCond checks if the target node is a direct argument of a CallExpr.
// This uses proper parent tracking from the AST traversal.
type IsCallArgCond struct {
	Target string
}

// Eval implements Condition for IsCallArgCond.
func (c *IsCallArgCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	if node == nil {
		return false
	}

	return ctx.IsChildOfCallExpr(node)
}

// IsAfterBlockOpenCond checks if a node is immediately after a block opening
// brace. This is used to avoid adding blank lines right after { or case:.
type IsAfterBlockOpenCond struct {
	Target string
}

// Eval implements Condition for IsAfterBlockOpenCond.
func (c *IsAfterBlockOpenCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	if node == nil {
		return false
	}

	pos := ctx.Fset.Position(node.Pos())
	nodeStart := pos.Offset

	// Find the start of this line
	lineStart := nodeStart
	for lineStart > 0 && ctx.Source[lineStart-1] != '\n' {
		lineStart--
	}

	// Look at the previous line
	if lineStart == 0 {
		return true // At start of file
	}

	// Find start of previous line
	prevLineEnd := lineStart - 1
	prevLineStart := prevLineEnd
	for prevLineStart > 0 && ctx.Source[prevLineStart-1] != '\n' {
		prevLineStart--
	}

	// Get the previous line content, trimming whitespace
	prevLine := string(ctx.Source[prevLineStart:prevLineEnd])
	trimmed := trimWhitespace(prevLine)

	// Check if previous line ends with { or :
	if len(trimmed) > 0 {
		lastChar := trimmed[len(trimmed)-1]
		if lastChar == '{' || lastChar == ':' {
			return true
		}
	}

	return false
}

// trimWhitespace removes leading and trailing whitespace from a string.
func trimWhitespace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}

	return s[start:end]
}

// IsFinalReturnCond checks if a return statement is the final statement in its
// function body (not in a nested block like if/for). Returns true if: 1. It's
// the last statement in the function body 2. There's more than one statement
// before it
type IsFinalReturnCond struct {
	Target string
}

// Eval implements Condition for IsFinalReturnCond.
func (c *IsFinalReturnCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	if node == nil {
		return false
	}

	// Get the parent block statement
	parent := ctx.Parent(node)
	if parent == nil {
		return false
	}

	// Check if we're in a block statement
	blockStmt, ok := parent.(*ast.BlockStmt)
	if !ok {
		return false
	}

	// Check if this is the last statement in the block
	if len(blockStmt.List) == 0 {
		return false
	}

	if blockStmt.List[len(blockStmt.List)-1] != node {
		return false
	}

	// Check if the parent of the block is a FuncDecl or FuncLit (function
	// body)
	blockParent := ctx.Parent(blockStmt)
	if blockParent == nil {
		return false
	}

	switch blockParent.(type) {
	case *ast.FuncDecl, *ast.FuncLit:

		// This is a function body, check if there are multiple
		// statements
		return len(blockStmt.List) > 1

	default:

		// Inside an if/for/switch block - don't add blank line
		return false
	}
}

// HasPrecedingSiblingCond checks if a case clause has a preceding sibling case.
// Used to determine if blank line is needed before a case.
type HasPrecedingSiblingCond struct {
	Target string
}

// Eval implements Condition for HasPrecedingSiblingCond.
func (c *HasPrecedingSiblingCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	if node == nil {
		return false
	}

	caseClause, ok := node.(*ast.CaseClause)
	if !ok {
		return false
	}

	// Get the parent switch statement
	parent := ctx.Parent(node)
	if parent == nil {
		return false
	}

	// Check if parent is a BlockStmt (switch body)
	blockStmt, ok := parent.(*ast.BlockStmt)
	if !ok {
		return false
	}

	// Find this case in the block's statements
	for i, stmt := range blockStmt.List {
		if stmt == caseClause {
			return i > 0 // Has preceding sibling if not first
		}
	}

	return false
}

// IsInInterfaceCond checks if a node is inside an interface type declaration.
type IsInInterfaceCond struct {
	Target string
}

// Eval implements Condition for IsInInterfaceCond.
func (c *IsInInterfaceCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	if node == nil {
		return false
	}

	// Walk up the parent chain looking for InterfaceType
	current := node
	for current != nil {
		if _, ok := current.(*ast.InterfaceType); ok {
			return true
		}
		current = ctx.Parent(current)
	}

	return false
}

// HasMultipleMethodsCond checks if an interface has multiple methods.
type HasMultipleMethodsCond struct {
	Target string
}

// Eval implements Condition for HasMultipleMethodsCond.
func (c *HasMultipleMethodsCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)

	var interfaceType *ast.InterfaceType
	switch n := node.(type) {
	case *ast.InterfaceType:
		interfaceType = n

	case *ast.TypeSpec:
		if it, ok := n.Type.(*ast.InterfaceType); ok {
			interfaceType = it
		}
	}

	if interfaceType == nil || interfaceType.Methods == nil {
		return false
	}

	return len(interfaceType.Methods.List) > 1
}

// CaseHasBodyCond checks if a case clause has a non-empty body.
type CaseHasBodyCond struct {
	Target string
}

// Eval implements Condition for CaseHasBodyCond.
func (c *CaseHasBodyCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	if node == nil {
		return false
	}

	caseClause, ok := node.(*ast.CaseClause)
	if !ok {
		return false
	}

	return len(caseClause.Body) > 0
}

// IsMethodChainCond checks if a call expression is part of a method chain with
// at least minCalls chained method calls.
type IsMethodChainCond struct {
	Target   string
	MinCalls int // Minimum number of calls to consider it a chain (default 2)
}

// Eval implements Condition for IsMethodChainCond.
func (c *IsMethodChainCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	if node == nil {
		return false
	}

	call, ok := node.(*ast.CallExpr)
	if !ok {
		// Check if it's an assignment with a call on RHS
		if assign, ok := node.(*ast.AssignStmt); ok &&
			len(assign.Rhs) > 0 {

			call, ok = assign.Rhs[0].(*ast.CallExpr)
			if !ok {
				return false
			}
		} else {
			return false
		}
	}

	minCalls := c.MinCalls
	if minCalls == 0 {
		minCalls = 2
	}

	count := countMethodChainCalls(call)

	return count >= minCalls
}

// countMethodChainCalls counts the number of method calls in a chain.
func countMethodChainCalls(call *ast.CallExpr) int {
	count := 0
	current := call

	for current != nil {
		count++

		// Check if this call's Fun is a selector on another call
		sel, ok := current.Fun.(*ast.SelectorExpr)
		if !ok {
			break
		}

		// Check if the selector's X is another call
		nextCall, ok := sel.X.(*ast.CallExpr)
		if !ok {
			break
		}

		current = nextCall
	}

	return count
}

// IsLogOrPrintfCallCond checks if a call expression is a log or printf-style
// call.
type IsLogOrPrintfCallCond struct {
	Target string

	// MatchAnySelectorPrefix enables suffix-only matching for selector
	// calls.
	//
	// When false (default), this condition matches only the canonical
	// patterns: "log.Infof", "fmt.Sprintf", etc.
	//
	// When true, any selector expression whose Sel is one of the known
	// log/printf function names is matched, regardless of the selector
	// prefix (e.g. "myLogger.Infof", "rpcLog.Errorf"). This is
	// intentionally opt-in because some repos include custom loggers that
	// should not be rewritten.
	MatchAnySelectorPrefix bool

	// IncludeNonFStringCalls enables matching a small subset of non-`*f`
	// log calls (e.g. `logger.Error("...")`) when the first argument is a
	// string. This is intended for "next" where string-call formatting is
	// more broadly useful.
	IncludeNonFStringCalls bool
}

// logPrintfPatterns are the function call patterns to match.
var logPrintfPatterns = []string{
	"log.Infof", "log.Debugf", "log.Tracef", "log.Errorf", "log.Warnf",
	"fmt.Printf", "fmt.Sprintf", "fmt.Errorf",
	"errors.New",
}

// IsNonFLogCallCond checks if a call expression is a non-printf-style logging
// call such as `log.Info(...)`.
//
// This is used to keep the generic multiline-call formatter from rewriting
// these calls. The log/printf formatter intentionally targets only the `*f`
// variants, and some repos rely on the non-`f` variants being left alone.
type IsNonFLogCallCond struct {
	Target string
}

// Eval implements Condition for IsLogOrPrintfCallCond.
func (c *IsLogOrPrintfCallCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	if node == nil {
		return false
	}

	call, ok := node.(*ast.CallExpr)
	if !ok {
		return false
	}

	if isErrorsNewCall(call) {
		return true
	}

	if c.IncludeNonFStringCalls && isNonFStringLogCall(call) {
		return true
	}

	if c.MatchAnySelectorPrefix {
		return isLogPrintfCallWithAnyPrefix(call)
	}

	// Get the function name
	funcName := getFuncName(call)
	if funcName == "" {
		return false
	}

	for _, pattern := range logPrintfPatterns {
		if funcName == pattern {
			return true
		}
	}

	return false
}

func isErrorsNewCall(call *ast.CallExpr) bool {

	// Even when MatchAnySelectorPrefix is enabled (suffix-only matching),
	// explicitly recognize a small set of well-known non-`*f` string calls
	// that should use the same formatting behavior (e.g.
	// errors.New("...")).
	return getFuncName(call) == "errors.New"
}

func isNonFStringLogCall(call *ast.CallExpr) bool {
	// In "next", optionally treat some non-`*f` log calls as targeted
	// string calls when their first argument is a string literal/concat.
	// This captures patterns like: rpcLog.Error("long message ...") without
	// changing legacy/parity behavior.
	if len(call.Args) == 0 || !isStringLit(call.Args[0]) {
		return false
	}

	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		return fun.Sel != nil && isNonFStringLogName(fun.Sel.Name)

	case *ast.Ident:

		// Support dot-imported variants (e.g. `Error(...)`) when
		// enabled.
		return isNonFStringLogName(fun.Name)

	default:
		return false
	}
}

func isLogPrintfCallWithAnyPrefix(call *ast.CallExpr) bool {
	switch fun := call.Fun.(type) {
	case *ast.Ident:

		// Support dot-imported variants (e.g. `Infof(...)`) when
		// enabled.
		return isLogPrintfName(fun.Name)

	case *ast.SelectorExpr:
		return fun.Sel != nil && isLogPrintfName(fun.Sel.Name)

	default:
		return false
	}
}

func isLogPrintfName(name string) bool {
	switch name {
	case "Infof", "Debugf", "Tracef", "Errorf", "Warnf", "Printf",
		"Sprintf":

		return true

	default:
		return false
	}
}

func isNonFLogName(name string) bool {
	switch name {
	case "Info", "Debug", "Trace", "Error", "Warn", "Fatal", "Panic":
		return true

	default:
		return false
	}
}

func isNonFStringLogName(name string) bool {
	// Keep this intentionally small and string-oriented; these are
	// typically used as message-only calls. We intentionally do not include
	// Fatal/Panic to avoid surprising changes in control-flow-adjacent
	// code.
	switch name {
	case "Info", "Debug", "Trace", "Warn", "Error":
		return true

	default:
		return false
	}
}

// Eval implements Condition for IsNonFLogCallCond.
func (c *IsNonFLogCallCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	call, ok := node.(*ast.CallExpr)
	if !ok || call == nil {
		return false
	}

	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel == nil || sel.Sel == nil {
		return false
	}
	if !isNonFLogName(sel.Sel.Name) {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok || ident == nil {
		return false
	}

	return ident.Name == "log"
}

// getFuncName extracts the function name from a call expression. Returns
// "pkg.Func" for selector expressions or "Func" for identifiers.
func getFuncName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name

	case *ast.SelectorExpr:
		if x, ok := fun.X.(*ast.Ident); ok {
			return x.Name + "." + fun.Sel.Name
		}
	}

	return ""
}

// IsInterfaceMethodCond checks if a field is a method in an interface.
type IsInterfaceMethodCond struct {
	Target string
}

// Eval implements Condition for IsInterfaceMethodCond.
func (c *IsInterfaceMethodCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	if node == nil {
		return false
	}

	field, ok := node.(*ast.Field)
	if !ok {
		return false
	}

	// Check if the field type is a function (method signature)
	_, isFunc := field.Type.(*ast.FuncType)
	if !isFunc {
		return false
	}

	// Check if we're inside an interface
	parent := ctx.Parent(node)
	if parent == nil {
		return false
	}

	fieldList, ok := parent.(*ast.FieldList)
	if !ok {
		return false
	}

	grandparent := ctx.Parent(fieldList)
	if grandparent == nil {
		return false
	}

	_, isInterface := grandparent.(*ast.InterfaceType)

	return isInterface
}

// HasPrecedingInterfaceFieldCond checks if a field in an interface has a
// preceding sibling field.
type HasPrecedingInterfaceFieldCond struct {
	Target string
}

// Eval implements Condition for HasPrecedingInterfaceFieldCond.
func (c *HasPrecedingInterfaceFieldCond) Eval(caps Captures,
	ctx *Context) bool {

	node := resolveTarget(caps, c.Target)
	if node == nil {
		return false
	}

	field, ok := node.(*ast.Field)
	if !ok {
		return false
	}

	// Get parent field list
	parent := ctx.Parent(node)
	if parent == nil {
		return false
	}

	fieldList, ok := parent.(*ast.FieldList)
	if !ok || fieldList.List == nil {
		return false
	}

	// Check if this field has a preceding sibling
	for i, f := range fieldList.List {
		if f == field {
			return i > 0
		}
	}

	return false
}

// AnyLineWidthCond checks if ANY line of a node exceeds a threshold. This is
// useful for multiline nodes like function signatures where one line might be
// short but another line exceeds the limit.
type AnyLineWidthCond struct {
	Target string
	Op     string
	Value  int // If 0, uses ctx.ColumnLimit
}

// Eval implements Condition for AnyLineWidthCond.
func (c *AnyLineWidthCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	if node == nil {
		return false
	}

	// Get the node's source text
	start := ctx.Fset.Position(node.Pos()).Offset
	end := ctx.Fset.Position(node.End()).Offset
	if start < 0 || end > len(ctx.Source) || start >= end {
		return false
	}
	nodeText := string(ctx.Source[start:end])

	threshold := c.Value
	if threshold == 0 {
		threshold = ctx.ColumnLimit
	}

	// Check each line
	lines := strings.Split(nodeText, "\n")
	for i, line := range lines {
		// For the first line, include the indent prefix
		if i == 0 {
			lineStart := start
			for lineStart > 0 && ctx.Source[lineStart-1] != '\n' {
				lineStart--
			}
			prefix := string(ctx.Source[lineStart:start])
			line = prefix + line
		}

		width := visualLen(line, ctx.TabStop)
		if compareInt(width, c.Op, threshold) {
			return true
		}
	}

	return false
}

// HasNestedMultilineTypeCond checks if a FuncDecl has parameters containing
// func types with multiline nested content (like multiline structs). This is
// useful to trigger formatting for readability even when no line exceeds the
// column limit.
type HasNestedMultilineTypeCond struct {
	Target string
}

// HasMultilineFuncSignatureCond checks if a function declaration signature
// spans multiple lines (i.e. there is a newline between `func` and the opening
// brace).
//
// This is intentionally signature-specific: checking the entire FuncDecl node
// text would always see newlines in the function body.
type HasMultilineFuncSignatureCond struct {
	Target string
}

func (c *HasMultilineFuncSignatureCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	decl, ok := node.(*ast.FuncDecl)
	if !ok || decl == nil || decl.Body == nil ||
		!decl.Body.Lbrace.IsValid() {

		return false
	}

	start := ctx.Fset.Position(decl.Pos()).Offset
	brace := ctx.Fset.Position(decl.Body.Lbrace).Offset
	if start < 0 || brace > len(ctx.Source) || start >= brace {
		return false
	}

	sig := string(ctx.Source[start:brace])

	return strings.Contains(sig, "\n")
}

// AnyLineWidthFuncLitSignatureCond checks if ANY line of a function literal
// signature exceeds a threshold (defaults to ctx.ColumnLimit).
//
// The signature span is limited to `func`..`{` (exclusive of the body).
type AnyLineWidthFuncLitSignatureCond struct {
	Target string
	Op     string
	Value  int // If 0, uses ctx.ColumnLimit
}

func (c *AnyLineWidthFuncLitSignatureCond) Eval(caps Captures,
	ctx *Context) bool {

	node := resolveTarget(caps, c.Target)
	lit, ok := node.(*ast.FuncLit)
	if !ok || lit == nil || lit.Body == nil || !lit.Body.Lbrace.IsValid() {
		return false
	}

	start := ctx.Fset.Position(lit.Pos()).Offset
	brace := ctx.Fset.Position(lit.Body.Lbrace).Offset
	if start < 0 || brace > len(ctx.Source) || start >= brace {
		return false
	}

	sigText := string(ctx.Source[start:brace])
	threshold := c.Value
	if threshold == 0 {
		threshold = ctx.ColumnLimit
	}

	lines := strings.Split(sigText, "\n")
	for i, line := range lines {
		if i == 0 {
			lineStart := start
			for lineStart > 0 && ctx.Source[lineStart-1] != '\n' {
				lineStart--
			}
			prefix := string(ctx.Source[lineStart:start])
			line = prefix + line
		}
		w := visualLen(line, ctx.TabStop)
		if compareInt(w, c.Op, threshold) {
			return true
		}
	}

	return false
}

// HasMultilineFuncLitSignatureCond checks if a function literal signature spans
// multiple lines (i.e. there is a newline between `func` and the opening
// brace).
type HasMultilineFuncLitSignatureCond struct {
	Target string
}

func (c *HasMultilineFuncLitSignatureCond) Eval(caps Captures,
	ctx *Context) bool {

	node := resolveTarget(caps, c.Target)
	lit, ok := node.(*ast.FuncLit)
	if !ok || lit == nil || lit.Body == nil || !lit.Body.Lbrace.IsValid() {
		return false
	}

	start := ctx.Fset.Position(lit.Pos()).Offset
	brace := ctx.Fset.Position(lit.Body.Lbrace).Offset
	if start < 0 || brace > len(ctx.Source) || start >= brace {
		return false
	}

	sig := string(ctx.Source[start:brace])

	return strings.Contains(sig, "\n")
}

// HasMultilineInterfaceMethodCond checks if an interface method signature spans
// multiple lines (i.e. there is a newline inside the method field text).
//
// This is used by the "next" profile to reflow already-multiline interface
// methods even when no line exceeds the column limit, enabling collapse of
// short manually-split return lists.
type HasMultilineInterfaceMethodCond struct {
	Target string
}

func (c *HasMultilineInterfaceMethodCond) Eval(caps Captures,
	ctx *Context) bool {

	node := resolveTarget(caps, c.Target)
	field, ok := node.(*ast.Field)
	if !ok || field == nil {
		return false
	}

	start := ctx.Fset.Position(field.Pos()).Offset
	end := ctx.Fset.Position(field.End()).Offset
	if start < 0 || end > len(ctx.Source) || start >= end {
		return false
	}

	sig := string(ctx.Source[start:end])

	return strings.Contains(sig, "\n")
}

// HasMultilineIfHeaderCond checks if an `if` statement header spans multiple
// source lines before the opening `{`.
type HasMultilineIfHeaderCond struct {
	Target string
}

func (c *HasMultilineIfHeaderCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	ifStmt, ok := node.(*ast.IfStmt)
	if !ok || ifStmt == nil || ifStmt.Body == nil {
		return false
	}
	start := ctx.Fset.Position(ifStmt.Pos())
	lbrace := ctx.Fset.Position(ifStmt.Body.Lbrace)

	return start.Line != lbrace.Line
}

// HasMultilineForHeaderCond checks if a `for` statement header spans multiple
// source lines before the opening `{`.
type HasMultilineForHeaderCond struct {
	Target string
}

func (c *HasMultilineForHeaderCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	forStmt, ok := node.(*ast.ForStmt)
	if !ok || forStmt == nil || forStmt.Body == nil {
		return false
	}
	start := ctx.Fset.Position(forStmt.Pos())
	lbrace := ctx.Fset.Position(forStmt.Body.Lbrace)

	return start.Line != lbrace.Line
}

// HasMultilineCaseHeaderCond checks if a `case A, B, ...:` clause spans
// multiple source lines before the `:`.
type HasMultilineCaseHeaderCond struct {
	Target string
}

func (c *HasMultilineCaseHeaderCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	cc, ok := node.(*ast.CaseClause)
	if !ok || cc == nil {
		return false
	}
	start := ctx.Fset.Position(cc.Pos())
	colon := ctx.Fset.Position(cc.Colon)

	return start.Line != colon.Line
}

// Eval implements Condition for HasNestedMultilineTypeCond.
func (c *HasNestedMultilineTypeCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	if node == nil {
		return false
	}

	funcDecl, ok := node.(*ast.FuncDecl)
	if !ok || funcDecl == nil || funcDecl.Type == nil ||
		funcDecl.Type.Params == nil {

		return false
	}

	// Check each parameter
	for _, param := range funcDecl.Type.Params.List {
		// Get the parameter type source
		typeStart := ctx.Fset.Position(param.Type.Pos()).Offset
		typeEnd := ctx.Fset.Position(param.Type.End()).Offset
		if typeStart < 0 || typeEnd > len(ctx.Source) ||
			typeStart >= typeEnd {

			continue
		}
		typeText := string(ctx.Source[typeStart:typeEnd])

		// Check if it's a func type with multiline nested content
		if strings.Contains(typeText, "func(") &&
			strings.Contains(typeText, "struct") {

			if strings.Contains(typeText, "\n") {
				return true
			}
		}
	}

	return false
}

// IsReturnNeedingBlankCond checks if a return statement needs a blank line
// before it. This is true if: 1. It's not immediately after a block open ({ or
// case:) 2. It's not already preceded by a blank line
type IsReturnNeedingBlankCond struct {
	Target string
}

// Eval implements Condition for IsReturnNeedingBlankCond.
func (c *IsReturnNeedingBlankCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	if node == nil {
		return false
	}

	_, ok := node.(*ast.ReturnStmt)
	if !ok {
		return false
	}

	pos := ctx.Fset.Position(node.Pos())
	nodeStart := pos.Offset

	// Find the start of this line
	lineStart := nodeStart
	for lineStart > 0 && ctx.Source[lineStart-1] != '\n' {
		lineStart--
	}

	// Check if there's a { on the SAME line before the return This handles
	// single-line functions: func foo() { return bar }
	sameLineBefore := string(ctx.Source[lineStart:nodeStart])
	if strings.Contains(sameLineBefore, "{") {
		return false
	}

	// Look at the previous line
	if lineStart == 0 {
		return false // At start of file, no blank needed
	}

	// Find start of previous line
	prevLineEnd := lineStart - 1
	prevLineStart := prevLineEnd
	for prevLineStart > 0 && ctx.Source[prevLineStart-1] != '\n' {
		prevLineStart--
	}

	// Get the previous line content
	prevLine := string(ctx.Source[prevLineStart:prevLineEnd])
	trimmed := trimWhitespace(prevLine)

	// Don't add blank if previous line is blank
	if trimmed == "" {
		return false
	}

	// Don't add blank if previous line ends with { or :
	if len(trimmed) > 0 {
		lastChar := trimmed[len(trimmed)-1]
		if lastChar == '{' || lastChar == ':' {
			return false
		}
	}

	return true
}

// IsIfErrReturnNeedingBlankCond checks if an `if err != nil { return ... }`
// statement should be preceded by a blank line for readability.
//
// This is intentionally opt-in and should only be enabled under explicit
// profiles (e.g. "next").
type IsIfErrReturnNeedingBlankCond struct {
	Target string
}

func isIdentName(expr ast.Expr, name string) bool {
	id, ok := expr.(*ast.Ident)

	return ok && id != nil && id.Name == name
}

func isErrNotNilCond(expr ast.Expr) bool {
	be, ok := expr.(*ast.BinaryExpr)
	if !ok || be == nil {
		return false
	}
	if be.Op != token.NEQ {
		return false
	}

	return (isIdentName(be.X, "err") && isIdentName(be.Y, "nil")) ||
		(isIdentName(be.X, "nil") && isIdentName(be.Y, "err"))
}

func ifBodyIsSingleReturn(body *ast.BlockStmt) bool {
	if body == nil || len(body.List) != 1 {
		return false
	}
	_, ok := body.List[0].(*ast.ReturnStmt)

	return ok
}

// Eval implements Condition for IsIfErrReturnNeedingBlankCond.
func (c *IsIfErrReturnNeedingBlankCond) Eval(caps Captures, ctx *Context) bool {
	node := resolveTarget(caps, c.Target)
	if node == nil {
		return false
	}

	ifs, ok := node.(*ast.IfStmt)
	if !ok || ifs == nil {
		return false
	}

	// Only apply to a very specific pattern.
	if ifs.Else != nil {
		return false
	}
	if !isErrNotNilCond(ifs.Cond) {
		return false
	}
	if !ifBodyIsSingleReturn(ifs.Body) {
		return false
	}

	// Require a preceding statement in the containing block; we don't want
	// to insert a blank line at the start of a block.
	parent := ctx.Parent(node)
	block, ok := parent.(*ast.BlockStmt)
	if !ok || block == nil {
		return false
	}
	hasPrev := false
	for i, stmt := range block.List {
		if stmt == node {
			hasPrev = i > 0
			break
		}
	}
	if !hasPrev {
		return false
	}

	pos := ctx.Fset.Position(node.Pos())
	nodeStart := pos.Offset
	if nodeStart <= 0 || nodeStart > len(ctx.Source) {
		return false
	}

	// Find the start of this line.
	lineStart := nodeStart
	for lineStart > 0 && ctx.Source[lineStart-1] != '\n' {
		lineStart--
	}

	// Look at the previous line.
	if lineStart == 0 {
		return false
	}

	prevLineEnd := lineStart - 1
	prevLineStart := prevLineEnd
	for prevLineStart > 0 && ctx.Source[prevLineStart-1] != '\n' {
		prevLineStart--
	}

	prevLine := string(ctx.Source[prevLineStart:prevLineEnd])
	trimmed := trimWhitespace(prevLine)
	if trimmed == "" {
		return false
	}
	if len(trimmed) > 0 {
		lastChar := trimmed[len(trimmed)-1]
		if lastChar == '{' || lastChar == ':' {
			return false
		}
	}

	return true
}
