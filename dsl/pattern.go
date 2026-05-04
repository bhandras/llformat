package dsl

import (
	"go/ast"
	"go/token"
)

// NodePattern matches a specific AST node type with field constraints.
type NodePattern struct {
	Type   string                // "CallExpr", "BinaryExpr", etc.
	Fields map[string]FieldMatch // Field constraints
}

// FieldMatch specifies how to match a field.
type FieldMatch struct {
	Capture    string   // Variable name to capture (without $)
	SubPattern Pattern  // Nested pattern to match
	Literal    string   // Literal value to match (for operators, identifiers)
	OneOf      []string // Match any of these literal values
}

// Match implements Pattern for NodePattern.
func (p *NodePattern) Match(n ast.Node, fset *token.FileSet) (Captures, bool) {
	if n == nil {
		return nil, false
	}

	// Check node type
	if !p.matchType(n) {
		return nil, false
	}

	caps := make(Captures)

	// Match each field constraint
	for fieldName, fm := range p.Fields {
		if !p.matchField(n, fset, fieldName, fm, caps) {
			return nil, false
		}
	}

	return caps, true
}

func (p *NodePattern) matchField(n ast.Node, fset *token.FileSet,
	fieldName string, fm FieldMatch, caps Captures) bool {

	child := getField(n, fieldName)

	// Capture if requested.
	if fm.Capture != "" {
		caps[fm.Capture] = child
	}

	// Check literal match.
	if fm.Literal != "" && !matchLiteral(n, child, fm.Literal) {
		return false
	}

	// Check OneOf match.
	if len(fm.OneOf) > 0 && !matchOneOfLiteral(n, child, fm.OneOf) {
		return false
	}

	// Recurse into sub-pattern.
	if fm.SubPattern == nil {
		return true
	}
	if child == nil {
		return false
	}
	childCaps, ok := fm.SubPattern.Match(child, fset)
	if !ok {
		return false
	}
	mergeCaps(caps, childCaps)

	return true
}

func matchOneOfLiteral(n ast.Node, child ast.Node, oneOf []string) bool {
	for _, lit := range oneOf {
		if matchLiteral(n, child, lit) {
			return true
		}
	}

	return false
}

func (p *NodePattern) matchType(n ast.Node) bool {
	switch p.Type {
	case "File":
		_, ok := n.(*ast.File)

		return ok

	case "CallExpr":
		_, ok := n.(*ast.CallExpr)

		return ok

	case "BinaryExpr":
		_, ok := n.(*ast.BinaryExpr)

		return ok

	case "UnaryExpr":
		_, ok := n.(*ast.UnaryExpr)

		return ok

	case "AssignStmt":
		_, ok := n.(*ast.AssignStmt)

		return ok

	case "IfStmt":
		_, ok := n.(*ast.IfStmt)

		return ok

	case "ForStmt":
		_, ok := n.(*ast.ForStmt)

		return ok

	case "ReturnStmt":
		_, ok := n.(*ast.ReturnStmt)

		return ok

	case "CaseClause":
		_, ok := n.(*ast.CaseClause)

		return ok

	case "SwitchStmt":
		_, ok := n.(*ast.SwitchStmt)

		return ok

	case "TypeSwitchStmt":
		_, ok := n.(*ast.TypeSwitchStmt)

		return ok

	case "FuncDecl":
		_, ok := n.(*ast.FuncDecl)

		return ok

	case "FuncLit":
		_, ok := n.(*ast.FuncLit)

		return ok

	case "BasicLit":
		_, ok := n.(*ast.BasicLit)

		return ok

	case "Ident":
		_, ok := n.(*ast.Ident)

		return ok

	case "SelectorExpr":
		_, ok := n.(*ast.SelectorExpr)

		return ok

	case "IndexExpr":
		_, ok := n.(*ast.IndexExpr)

		return ok

	case "SliceExpr":
		_, ok := n.(*ast.SliceExpr)

		return ok

	case "TypeAssertExpr":
		_, ok := n.(*ast.TypeAssertExpr)

		return ok

	case "CompositeLit":
		_, ok := n.(*ast.CompositeLit)

		return ok

	case "KeyValueExpr":
		_, ok := n.(*ast.KeyValueExpr)

		return ok

	case "ParenExpr":
		_, ok := n.(*ast.ParenExpr)

		return ok

	case "StarExpr":
		_, ok := n.(*ast.StarExpr)

		return ok

	case "Ellipsis":
		_, ok := n.(*ast.Ellipsis)

		return ok

	case "ValueSpec":
		_, ok := n.(*ast.ValueSpec)

		return ok

	case "FuncType":
		_, ok := n.(*ast.FuncType)

		return ok

	case "FieldList":
		_, ok := n.(*ast.FieldList)

		return ok

	case "Field":
		_, ok := n.(*ast.Field)

		return ok

	case "InterfaceType":
		_, ok := n.(*ast.InterfaceType)

		return ok

	default:
		return false
	}
}

// getField extracts a field from an AST node by name.
func getField(n ast.Node, name string) ast.Node {
	switch node := n.(type) {
	case *ast.CallExpr:
		return getFieldCallExpr(node, name)

	case *ast.BinaryExpr:
		return getFieldBinaryExpr(node, name)

	case *ast.UnaryExpr:
		return getFieldUnaryExpr(node, name)

	case *ast.AssignStmt:
		return getFieldAssignStmt(node, name)

	case *ast.IfStmt:
		return getFieldIfStmt(node, name)

	case *ast.ForStmt:
		return getFieldForStmt(node, name)

	case *ast.ReturnStmt:
		return getFieldReturnStmt(node, name)

	case *ast.CaseClause:
		return getFieldCaseClause(node, name)

	case *ast.SelectorExpr:
		return getFieldSelectorExpr(node, name)

	case *ast.ParenExpr:
		return getFieldParenExpr(node, name)

	case *ast.StarExpr:
		return getFieldStarExpr(node, name)

	case *ast.CompositeLit:
		return getFieldCompositeLit(node, name)

	case *ast.FuncDecl:
		return getFieldFuncDecl(node, name)

	case *ast.FuncType:
		return getFieldFuncType(node, name)

	case *ast.FieldList:
		return getFieldFieldList(node, name)
	}

	return nil
}

// Field extraction is intentionally explicit rather than reflection-based:
// patterns form part of the formatter's "language" and we want failures to be
// obvious and deterministic.
func getFieldCallExpr(node *ast.CallExpr, name string) ast.Node {
	switch name {
	case "Fun", "func":
		return node.Fun

	default:
		return nil
	}
}

func getFieldBinaryExpr(node *ast.BinaryExpr, name string) ast.Node {
	switch name {
	case "X", "left":
		return node.X

	case "Y", "right":
		return node.Y

	case "Op", "op":

		// Op is handled specially in matchLiteral.
		return nil

	default:
		return nil
	}
}

func getFieldUnaryExpr(node *ast.UnaryExpr, name string) ast.Node {
	switch name {
	case "X", "operand":
		return node.X

	case "Op", "op":

		// Op is handled specially in matchLiteral.
		return nil

	default:
		return nil
	}
}

func getFieldAssignStmt(node *ast.AssignStmt, name string) ast.Node {
	switch name {
	case "Lhs", "lhs":
		if len(node.Lhs) > 0 {
			return node.Lhs[0]
		}

		return nil

	case "Rhs", "rhs":
		if len(node.Rhs) > 0 {
			return node.Rhs[0]
		}

		return nil

	default:
		return nil
	}
}

func getFieldIfStmt(node *ast.IfStmt, name string) ast.Node {
	switch name {
	case "Cond", "cond":
		return node.Cond

	case "Body", "body":
		return node.Body

	case "Init", "init":
		return node.Init

	case "Else", "else":
		return node.Else

	default:
		return nil
	}
}

func getFieldForStmt(node *ast.ForStmt, name string) ast.Node {
	switch name {
	case "Cond", "cond":
		return node.Cond

	case "Body", "body":
		return node.Body

	case "Init", "init":
		return node.Init

	case "Post", "post":
		return node.Post

	default:
		return nil
	}
}

func getFieldReturnStmt(node *ast.ReturnStmt, name string) ast.Node {
	switch name {
	case "Results", "results":
		if len(node.Results) > 0 {
			return node.Results[0]
		}

		return nil

	default:
		return nil
	}
}

func getFieldCaseClause(node *ast.CaseClause, name string) ast.Node {
	switch name {
	case "List", "list":
		if len(node.List) > 0 {
			return node.List[0]
		}

		return nil

	default:
		return nil
	}
}

func getFieldSelectorExpr(node *ast.SelectorExpr, name string) ast.Node {
	switch name {
	case "X", "x":
		return node.X

	case "Sel", "sel":
		return node.Sel

	default:
		return nil
	}
}

func getFieldParenExpr(node *ast.ParenExpr, name string) ast.Node {
	switch name {
	case "X", "x":
		return node.X

	default:
		return nil
	}
}

func getFieldStarExpr(node *ast.StarExpr, name string) ast.Node {
	switch name {
	case "X", "x":
		return node.X

	default:
		return nil
	}
}

func getFieldCompositeLit(node *ast.CompositeLit, name string) ast.Node {
	switch name {
	case "Type", "type":
		return node.Type

	default:
		return nil
	}
}

func getFieldFuncDecl(node *ast.FuncDecl, name string) ast.Node {
	switch name {
	case "Type", "type":
		return node.Type

	case "Name", "name":
		return node.Name

	case "Recv", "recv":
		return node.Recv

	case "Body", "body":
		return node.Body

	default:
		return nil
	}
}

func getFieldFuncType(node *ast.FuncType, name string) ast.Node {
	switch name {
	case "Params", "params":
		return node.Params

	case "Results", "results":
		return node.Results

	default:
		return nil
	}
}

func getFieldFieldList(node *ast.FieldList, name string) ast.Node {
	switch name {
	case "List", "list":
		if len(node.List) > 0 {
			return node.List[0]
		}

		return nil

	default:
		return nil
	}
}

// matchLiteral checks if a field matches a literal value.
func matchLiteral(parent, child ast.Node, lit string) bool {
	// Handle operator matching for BinaryExpr and UnaryExpr
	if binExpr, ok := parent.(*ast.BinaryExpr); ok && child == nil {
		return binExpr.Op.String() == lit
	}
	if unaryExpr, ok := parent.(*ast.UnaryExpr); ok && child == nil {
		return unaryExpr.Op.String() == lit
	}

	// Handle identifier matching
	if ident, ok := child.(*ast.Ident); ok {
		return ident.Name == lit
	}

	// Handle basic literal matching
	if basicLit, ok := child.(*ast.BasicLit); ok {
		return basicLit.Value == lit
	}

	return false
}

func mergeCaps(dst, src Captures) {
	for k, v := range src {
		dst[k] = v
	}
}

// Wildcard matches any node.
type Wildcard struct{}

// Match implements Pattern for Wildcard.
func (w Wildcard) Match(n ast.Node, fset *token.FileSet) (Captures, bool) {
	return Captures{}, n != nil
}

// AnyOf matches if any of the sub-patterns match.
type AnyOf struct {
	Patterns []Pattern
}

// Match implements Pattern for AnyOf.
func (a *AnyOf) Match(n ast.Node, fset *token.FileSet) (Captures, bool) {
	for _, p := range a.Patterns {
		if caps, ok := p.Match(n, fset); ok {
			return caps, true
		}
	}

	return nil, false
}

// HasType matches nodes of specific types without field constraints.
type HasType struct {
	Types []string
}

// Match implements Pattern for HasType.
func (h *HasType) Match(n ast.Node, fset *token.FileSet) (Captures, bool) {
	for _, t := range h.Types {
		p := &NodePattern{Type: t}
		if p.matchType(n) {
			return Captures{}, true
		}
	}

	return nil, false
}
