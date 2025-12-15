package dsl

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"

	llast "github.com/lightninglabs/llformat/ast"
	"github.com/lightninglabs/llformat/scanner"
	"github.com/lightninglabs/llformat/text"
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

	// For method chains, try each call and use the first one that helps
	targetCall := findCallToReflow(call, ctx)
	if targetCall == nil {
		return nil, false
	}

	// Get source positions
	start := ctx.Fset.Position(targetCall.Pos()).Offset
	end := ctx.Fset.Position(targetCall.End()).Offset
	if start < 0 || end > len(ctx.Source) || start >= end {
		return nil, false
	}

	original := string(ctx.Source[start:end])

	// Skip calls that contain inline comments - reformatting via AST rendering
	// would drop them.
	if hasLineComment(original) {
		return nil, false
	}

	// Get indentation
	indent := ctx.IndentAt(targetCall)

	// Format the call
	var formatted string
	switch a.Strategy {
	case StrategyOnePerLine:
		formatted = formatCallOnePerLine(targetCall, indent, ctx)
	case StrategyLeftPack:
		formatted = formatCallLeftPack(targetCall, indent, ctx)
	case StrategyAdaptive:
		formatted = formatCallAdaptive(targetCall, indent, ctx)
	default:
		formatted = formatCallOnePerLine(targetCall, indent, ctx)
	}

	// Check if formatting actually changed anything
	if formatted == original {
		return nil, false
	}

	// Build result by replacing the call in source
	out, err := ApplySingleEdit(ctx.Source, start, end, []byte(formatted))
	if err != nil {
		return nil, false
	}
	return out, true
}

// findCallToReflow finds a call in a method chain that benefits from reflowing.
// Returns the first call where reflowing reduces line width below the limit.
func findCallToReflow(call *ast.CallExpr, ctx *Context) *ast.CallExpr {
	// Collect all calls in the method chain (innermost to outermost)
	var calls []*ast.CallExpr
	current := call
	for current != nil {
		calls = append(calls, current)
		// Check if this call's Fun is another call (method chain)
		if sel, ok := current.Fun.(*ast.SelectorExpr); ok {
			if nextCall, ok := sel.X.(*ast.CallExpr); ok {
				current = nextCall
				continue
			}
		}
		break
	}

	// Process from outermost call first (calls[0] is outermost)
	// This way we reflow the call that keeps most of the chain intact

	// Try each call - return the first one where reflowing helps
	for _, c := range calls {
		if len(c.Args) == 0 {
			continue
		}

		// Check if the line containing this call exceeds the limit
		if ctx.LineWidth(c) <= ctx.ColumnLimit {
			continue
		}

		// Try reflowing this call and see if it helps
		indent := ctx.IndentAt(c)
		formatted := formatCallOnePerLine(c, indent, ctx)

		start := ctx.Fset.Position(c.Pos()).Offset
		end := ctx.Fset.Position(c.End()).Offset
		original := string(ctx.Source[start:end])

		if formatted == original {
			continue
		}

		// Verify the reflow actually reduces line width
		// Build temporary result and check.
		newBytes, err := ApplySingleEdit(ctx.Source, start, end, []byte(formatted))
		if err != nil {
			continue
		}
		newFset := token.NewFileSet()
		newFile, err := parser.ParseFile(newFset, "", newBytes, 0)
		if err != nil {
			continue
		}

		// Find the first line width in the reformatted area
		newCtx := NewContext(newFset, newBytes, ctx.ColumnLimit, ctx.TabStop)
		improved := false
		ast.Inspect(newFile, func(n ast.Node) bool {
			if nc, ok := n.(*ast.CallExpr); ok {
				// Check if this is our reformatted call (by position approximation)
				pos := newFset.Position(nc.Pos())
				if pos.Offset >= start && pos.Offset <= start+len(formatted) {
					if newCtx.LineWidth(nc) <= ctx.ColumnLimit {
						improved = true
						return false
					}
				}
			}
			return true
		})

		if improved {
			return c
		}
	}

	return nil
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

	// Skip whitespace after the break point
	out, changed, err := applyContinuationIndentAfter(ctx.Source, end, indent)
	if err != nil {
		return nil, false
	}
	return out, changed
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
	out, changed, err := applyContinuationIndentBefore(ctx.Source, pos, indent)
	if err != nil {
		return nil, false
	}
	return out, changed
}

// BreakAtOpAction breaks a binary expression at the best operator position.
// It finds the rightmost operator that keeps the first part under the column limit.
// Prefers logical operators (&&, ||) over comparison/arithmetic when possible.
type BreakAtOpAction struct {
	Target     string
	BreakAfter bool // true = break after op (Go style), false = break before
}

// opPriority returns a priority for operators (lower = prefer to break here).
// Prefer breaking at && over || to keep "|| operand" together on the next line.
// Logical operators are preferred over comparison/arithmetic.
func opPriority(op string) int {
	switch op {
	case "&&":
		return 1 // Best break point for logical chains
	case "||":
		return 2 // Keep || with its right operand
	case "+", "-":
		return 3
	case "*", "/", "%":
		return 4
	case "==", "!=", "<", ">", "<=", ">=":
		return 5 // Comparison - avoid breaking here
	default:
		return 10
	}
}

// Execute implements Action for BreakAtOpAction.
func (a *BreakAtOpAction) Execute(caps Captures, ctx *Context) ([]byte, bool) {
	edits, changed, err := a.ExecuteEdits(caps, ctx)
	if err != nil || !changed {
		return nil, false
	}

	out, err := ApplyEdits(ctx.Source, edits)
	if err != nil {
		return nil, false
	}
	return out, true
}

// ExecuteEdits implements EditAction for BreakAtOpAction.
func (a *BreakAtOpAction) ExecuteEdits(caps Captures, ctx *Context) ([]Edit, bool, error) {
	node := resolveTarget(caps, a.Target)
	binExpr, ok := node.(*ast.BinaryExpr)
	if !ok {
		return nil, false, nil
	}

	// Check if line already fits
	if ctx.LineWidth(binExpr) <= ctx.ColumnLimit {
		return nil, false, nil
	}

	// Collect all operators in this expression chain
	type opInfo struct {
		pos      int    // byte offset of operator
		opLen    int    // length of operator string
		opStr    string // operator string
		prefix   int    // visual width of content before this operator
		priority int    // operator priority (lower = prefer)
	}

	var ops []opInfo
	pos := ctx.Fset.Position(binExpr.Pos())
	indent := ctx.IndentAt(binExpr)

	// Calculate line start offset (not node start)
	lineStart := pos.Offset - pos.Column + 1

	// Walk the expression to find all operators
	var collectOps func(expr ast.Expr)
	collectOps = func(expr ast.Expr) {
		bin, ok := expr.(*ast.BinaryExpr)
		if !ok {
			return
		}
		// Recurse left first (for left-associative chains)
		collectOps(bin.X)

		opPos := ctx.Fset.Position(bin.OpPos).Offset
		opStr := bin.Op.String()

		// Calculate prefix width (from line start to end of operator)
		prefixEnd := opPos + len(opStr)
		prefix := string(ctx.Source[lineStart:prefixEnd])
		prefixWidth := visualLen(prefix, ctx.TabStop)

		ops = append(ops, opInfo{
			pos:      opPos,
			opLen:    len(opStr),
			opStr:    opStr,
			prefix:   prefixWidth,
			priority: opPriority(opStr),
		})

		// Recurse right
		collectOps(bin.Y)
	}
	collectOps(binExpr)

	if len(ops) == 0 {
		return nil, false, nil
	}

	// Find the best operator: prefer lower priority (logical) operators,
	// and among those, pick the rightmost that fits under column limit
	var bestOp *opInfo
	for i := len(ops) - 1; i >= 0; i-- {
		op := &ops[i]
		if op.prefix <= ctx.ColumnLimit {
			// Check if this is a better candidate than current best
			if bestOp == nil || op.priority < bestOp.priority ||
				(op.priority == bestOp.priority && op.prefix > bestOp.prefix) {
				bestOp = op
			}
		}
	}

	// Fallback: if no good break point, pick lowest priority operator
	if bestOp == nil {
		for i := range ops {
			op := &ops[i]
			if bestOp == nil || op.priority < bestOp.priority {
				bestOp = op
			}
		}
	}

	if bestOp == nil {
		return nil, false, nil
	}

	opEnd := bestOp.pos + bestOp.opLen

	// Check if there's already a newline after this operator.
	i := skipHorizontalWhitespace(ctx.Source, opEnd)
	if i < len(ctx.Source) && ctx.Source[i] == '\n' {
		// Already broken here, don't add another break
		return nil, false, nil
	}

	end := skipHorizontalWhitespace(ctx.Source, opEnd)
	replacement := continuationIndentBytes(indent)
	if opEnd >= 0 && end >= opEnd && end <= len(ctx.Source) {
		if bytes.Equal(ctx.Source[opEnd:end], replacement) {
			return nil, false, nil
		}
	}

	return []Edit{
		{Start: opEnd, End: end, Replace: replacement},
	}, true, nil
}

// ReflowStringConcatAction rewrites a long string concatenation expression into
// a multi-line concatenation with stable indentation.
type ReflowStringConcatAction struct {
	Target string
}

// Execute implements Action for ReflowStringConcatAction.
func (a *ReflowStringConcatAction) Execute(caps Captures, ctx *Context) ([]byte, bool) {
	edits, changed, err := a.ExecuteEdits(caps, ctx)
	if err != nil || !changed {
		return nil, false
	}
	out, err := ApplyEdits(ctx.Source, edits)
	if err != nil {
		return nil, false
	}
	return out, true
}

func hasRawStringLit(n ast.Node) bool {
	found := false
	ast.Inspect(n, func(node ast.Node) bool {
		lit, ok := node.(*ast.BasicLit)
		if !ok {
			return !found
		}
		if lit.Kind.String() != "STRING" {
			return !found
		}
		if strings.HasPrefix(lit.Value, "`") {
			found = true
			return false
		}
		return !found
	})
	return found
}

// ExecuteEdits implements EditAction for ReflowStringConcatAction.
func (a *ReflowStringConcatAction) ExecuteEdits(caps Captures, ctx *Context) ([]Edit, bool, error) {
	node := resolveTarget(caps, a.Target)
	if node == nil {
		return nil, false, nil
	}

	start := ctx.Fset.Position(node.Pos()).Offset
	end := ctx.Fset.Position(node.End()).Offset
	if start < 0 || end > len(ctx.Source) || start >= end {
		return nil, false, nil
	}

	original := string(ctx.Source[start:end])
	if hasLineComment(original) {
		return nil, false, nil
	}

	expr, err := parser.ParseExpr(original)
	if err != nil {
		return nil, false, nil
	}

	// Be conservative: do not rewrite raw string literals (backticks). While the
	// value is constant, changing literal style can be surprising.
	if hasRawStringLit(expr) {
		return nil, false, nil
	}

	strText, ok := llast.FlattenStringExprAST(expr)
	if !ok {
		return nil, false, nil
	}

	indent := ctx.IndentAt(node)
	contIndent := indent + "\t"

	// Account for non-whitespace prefix before the expression (e.g. "return ").
	prefixWidth := prefixWidthAt(ctx.Source, start, ctx.TabStop)

	formatted := text.SplitQuotedString(strText, prefixWidth, contIndent, ctx.ColumnLimit, ctx.TabStop)
	if formatted == original {
		return nil, false, nil
	}

	return []Edit{
		{Start: start, End: end, Replace: []byte(formatted)},
	}, true, nil
}

// BreakCaseClauseAction breaks a long case clause at comma boundaries.
type BreakCaseClauseAction struct {
	Target string
}

// Execute implements Action for BreakCaseClauseAction.
func (a *BreakCaseClauseAction) Execute(caps Captures, ctx *Context) ([]byte, bool) {
	edits, changed, err := a.ExecuteEdits(caps, ctx)
	if err != nil || !changed {
		return nil, false
	}
	out, err := ApplyEdits(ctx.Source, edits)
	if err != nil {
		return nil, false
	}
	return out, true
}

// ExecuteEdits implements EditAction for BreakCaseClauseAction.
func (a *BreakCaseClauseAction) ExecuteEdits(caps Captures, ctx *Context) ([]Edit, bool, error) {
	node := resolveTarget(caps, a.Target)
	caseClause, ok := node.(*ast.CaseClause)
	if !ok || len(caseClause.List) == 0 {
		return nil, false, nil
	}

	// Check if line already fits
	if ctx.LineWidth(caseClause) <= ctx.ColumnLimit {
		return nil, false, nil
	}

	// Find the rightmost comma that keeps prefix under column limit
	indent := ctx.IndentAt(caseClause)
	indentWidth := visualLen(indent, ctx.TabStop)
	caseStart := ctx.Fset.Position(caseClause.Pos()).Offset

	// Collect comma positions
	type commaInfo struct {
		afterExpr int // position right after the expression (where comma is)
		prefix    int // visual width up to and including this comma
	}
	var commas []commaInfo

	for i := 0; i < len(caseClause.List)-1; i++ {
		expr := caseClause.List[i]
		exprEnd := ctx.Fset.Position(expr.End()).Offset

		// Find comma after this expression
		commaPos := exprEnd
		for commaPos < len(ctx.Source) && ctx.Source[commaPos] != ',' {
			commaPos++
		}
		if commaPos >= len(ctx.Source) {
			continue
		}

		// Calculate prefix width (from line start to comma inclusive)
		prefix := string(ctx.Source[caseStart : commaPos+1])
		prefixWidth := indentWidth + visualLen(prefix, ctx.TabStop)

		commas = append(commas, commaInfo{
			afterExpr: commaPos + 1,
			prefix:    prefixWidth,
		})
	}

	if len(commas) == 0 {
		return nil, false, nil
	}

	// Find the rightmost comma that keeps prefix under column limit
	var bestComma *commaInfo
	for i := len(commas) - 1; i >= 0; i-- {
		c := &commas[i]
		if c.prefix <= ctx.ColumnLimit {
			bestComma = c
			break
		}
	}

	// Fallback to first comma
	if bestComma == nil {
		bestComma = &commas[0]
	}

	pos := bestComma.afterExpr

	// If there is already a newline here, skip.
	i := skipHorizontalWhitespace(ctx.Source, pos)
	if i < len(ctx.Source) && ctx.Source[i] == '\n' {
		return nil, false, nil
	}

	end := skipHorizontalWhitespace(ctx.Source, pos)
	replacement := continuationIndentBytes(indent)
	if pos >= 0 && end >= pos && end <= len(ctx.Source) {
		if bytes.Equal(ctx.Source[pos:end], replacement) {
			return nil, false, nil
		}
	}

	return []Edit{
		{Start: pos, End: end, Replace: replacement},
	}, true, nil
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

// LeftFlowCallAction formats log/printf calls using left-flow packing with
// string splitting. This action delegates to the legacy formatter to ensure
// identical output behavior.
type LeftFlowCallAction struct {
	Target string

	// FormatFunc is an optional function that formats the call using legacy
	// logic. If nil, a simplified fallback is used.
	FormatFunc func(call []byte, wsIndent string, baseLen int, colLimit, tabStop int) string
}

// Execute implements Action for LeftFlowCallAction.
func (a *LeftFlowCallAction) Execute(caps Captures, ctx *Context) ([]byte, bool) {
	node := resolveTarget(caps, a.Target)
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return nil, false
	}

	if len(call.Args) == 0 {
		return nil, false
	}

	// Note: We don't check LineWidth here because the legacy formatter always
	// reformats targeted calls to normalize them. The comparison with original
	// output at the end will skip changes if the format is already correct.

	// Get source positions
	start := ctx.Fset.Position(call.Pos()).Offset
	end := ctx.Fset.Position(call.End()).Offset
	if start < 0 || end > len(ctx.Source) || start >= end {
		return nil, false
	}

	original := ctx.Source[start:end]
	wsIndent := ctx.IndentAt(call)

	// Find the base length (visual width from line start to call start)
	baseLen := prefixWidthAt(ctx.Source, start, ctx.TabStop)

	var formatted string
	if a.FormatFunc != nil {
		// Use the provided formatter (legacy formatter)
		formatted = a.FormatFunc(original, wsIndent, baseLen, ctx.ColumnLimit, ctx.TabStop)
	} else {
		// Fallback to simplified formatting
		formatted = formatCallLeftFlowSimple(call, wsIndent, ctx)
	}

	if formatted == string(original) {
		return nil, false
	}

	// Normalize both original and formatted calls with gofmt to check if
	// they're actually different. gofmt may change indentation of string
	// continuations in nested calls, so we need to compare post-gofmt output.
	origNorm := normalizeCallWithGofmt(string(original), wsIndent)
	fmtNorm := normalizeCallWithGofmt(formatted, wsIndent)

	if origNorm == fmtNorm {
		// After gofmt normalization, both produce the same output.
		// This means our change would be undone by gofmt - skip it.
		return nil, false
	}

	// Build result with formatted call
	out, err := ApplySingleEdit(ctx.Source, start, end, []byte(formatted))
	if err != nil {
		return nil, false
	}
	return out, true
}

// normalizeCallWithGofmt wraps a call expression in a minimal Go file,
// runs gofmt, and extracts the normalized call. This allows comparing
// two versions of a call that may differ only in gofmt-level formatting.
func normalizeCallWithGofmt(call string, wsIndent string) string {
	// Wrap in minimal Go file at the same indent level
	wrapped := "package p\nfunc f() {\n" + wsIndent + call + "\n}"
	formatted, err := format.Source([]byte(wrapped))
	if err != nil {
		// If gofmt fails, return original
		return call
	}

	// Extract the call from the formatted output.
	// gofmt may change the indent, so we find the actual call start
	// by looking for the opening brace of the function.
	s := string(formatted)

	// Find "func f() {\n" and then the actual call
	marker := "func f() {\n"
	idx := strings.Index(s, marker)
	if idx < 0 {
		return call
	}
	start := idx + len(marker)

	// Find the closing brace. The call ends just before "\n}\n" or "\n}"
	end := len(s)
	if strings.HasSuffix(s, "\n}\n") {
		end = len(s) - 3
	} else if strings.HasSuffix(s, "\n}") {
		end = len(s) - 2
	} else {
		return call
	}

	// Skip leading whitespace to get the call
	extracted := s[start:end]
	extracted = strings.TrimLeft(extracted, " \t")

	// Also trim trailing whitespace
	extracted = strings.TrimRight(extracted, " \t\n")

	return extracted
}

// PackedMultiLineCallAction formats generic function calls using packed
// multi-line style when they exceed the column limit. This action delegates
// to the legacy formatter to ensure identical output behavior.
type PackedMultiLineCallAction struct {
	Target string

	// FormatFunc is an optional function that formats the call using legacy
	// logic. If nil, a simplified fallback is used.
	FormatFunc func(call []byte, wsIndent string, colLimit, tabStop int) string
}

// LegacyOnePerLineCallAction formats generic function calls using the legacy
// MultiLineCallFormatter style (one argument per line). Unlike AST-based call
// actions, this action preserves comments inside argument lists because it only
// rearranges the existing source bytes.
type LegacyOnePerLineCallAction struct {
	Target string

	// FormatFunc is an optional function that formats the call using legacy
	// logic. If nil, a simplified fallback is used.
	FormatFunc func(call []byte, wsIndent string, colLimit, tabStop int) string
}

// LegacyMultiLineScanFunc applies a single legacy multiline-call formatting
// pass to src and reports whether it changed anything.
//
// This intentionally mirrors the legacy MultiLineCallFormatter behavior of
// making at most one change per pass and repeating up to a fixed iteration cap.
type LegacyMultiLineScanFunc func(src []byte, colLimit, tabStop int, excludes []string) ([]byte, bool)

// LegacyMultiLineScanAction delegates multiline-call detection + rewriting to a
// scan-based implementation, matching the legacy formatter's behavior (including
// its lexical detection quirks).
type LegacyMultiLineScanAction struct {
	Excludes []string
	ScanFunc LegacyMultiLineScanFunc
}

// Execute implements Action for LegacyMultiLineScanAction.
func (a *LegacyMultiLineScanAction) Execute(_ Captures, ctx *Context) ([]byte, bool) {
	if a.ScanFunc == nil {
		return nil, false
	}

	out, changed := a.ScanFunc(ctx.Source, ctx.ColumnLimit, ctx.TabStop, a.Excludes)
	if !changed {
		return nil, false
	}
	return out, true
}

// Execute implements Action for LegacyOnePerLineCallAction.
func (a *LegacyOnePerLineCallAction) Execute(caps Captures, ctx *Context) ([]byte, bool) {
	node := resolveTarget(caps, a.Target)
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return nil, false
	}

	if len(call.Args) == 0 {
		return nil, false
	}

	open := ctx.Fset.Position(call.Lparen).Offset
	close := ctx.Fset.Position(call.Rparen).Offset
	if open < 0 || close < open || close >= len(ctx.Source) {
		return nil, false
	}

	start := legacyCallStartForLparen(ctx.Source, open)
	end := close + 1
	if start < 0 || end > len(ctx.Source) || start >= end {
		return nil, false
	}

	original := ctx.Source[start:end]
	wsIndent := ctx.IndentAt(call)

	// Mirror legacy decision: compute the visual width of the prefix before the
	// call on the current line plus the call itself. This intentionally does not
	// collapse whitespace.
	ls := lineStart(ctx.Source, start)
	prefixLen := visualLen(string(ctx.Source[ls:start]), ctx.TabStop)
	callLen := visualLen(string(original), ctx.TabStop)
	if prefixLen+callLen <= ctx.ColumnLimit {
		return nil, false
	}

	var formatted string
	if a.FormatFunc != nil {
		formatted = a.FormatFunc(original, wsIndent, ctx.ColumnLimit, ctx.TabStop)
	} else {
		formatted = legacyFormatCallOnePerLine(original, wsIndent)
	}

	if formatted == string(original) {
		return nil, false
	}

	out, err := ApplySingleEdit(ctx.Source, start, end, []byte(formatted))
	if err != nil {
		return nil, false
	}
	return out, true
}

func legacyCallStartForLparen(src []byte, lparen int) int {
	if lparen <= 0 {
		return 0
	}
	if lparen > len(src) {
		lparen = len(src)
	}

	// Scan left from the '(' to find the start of the identifier/selector chain.
	i := lparen - 1
	for i >= 0 && (src[i] == ' ' || src[i] == '\t') {
		i--
	}

	for i >= 0 {
		if text.IsIdentifierChar(src[i]) {
			i--
			continue
		}
		if src[i] == '.' {
			// If the selector is applied to something like a call or composite
			// literal, stop at the method name (legacy scanner starts at the
			// selector, not the receiver call).
			if i-1 >= 0 {
				prev := src[i-1]
				if prev == ')' || prev == ']' || prev == '}' {
					break
				}
			}
			i--
			continue
		}
		break
	}

	return i + 1
}

func legacyFormatCallOnePerLine(callBytes []byte, wsIndent string) string {
	s := string(callBytes)
	open := strings.IndexByte(s, '(')
	if open == -1 || !strings.HasSuffix(s, ")") {
		return s
	}

	head := s[:open]
	argsBody := s[open+1 : len(s)-1]
	args := scanner.SplitTopLevel(argsBody)
	if len(args) == 0 {
		return s
	}

	var b strings.Builder
	b.WriteString(head)
	b.WriteString("(\n")

	argIndent := wsIndent + "\t"
	for i, arg := range args {
		trimmedArg := strings.TrimSpace(arg)
		if trimmedArg == "" {
			continue
		}
		b.WriteString(argIndent)
		lines := strings.Split(trimmedArg, "\n")
		for j, line := range lines {
			if j > 0 {
				b.WriteString("\n")
				b.WriteString(argIndent)
			}
			b.WriteString(strings.TrimSpace(line))
		}
		b.WriteString(",")
		if i < len(args)-1 {
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(wsIndent)
	b.WriteString(")")
	return b.String()
}

// Execute implements Action for PackedMultiLineCallAction.
func (a *PackedMultiLineCallAction) Execute(caps Captures, ctx *Context) ([]byte, bool) {
	node := resolveTarget(caps, a.Target)
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return nil, false
	}

	if len(call.Args) == 0 {
		return nil, false
	}

	// Get source positions
	start := ctx.Fset.Position(call.Pos()).Offset
	end := ctx.Fset.Position(call.End()).Offset
	if start < 0 || end > len(ctx.Source) || start >= end {
		return nil, false
	}

	original := ctx.Source[start:end]
	wsIndent := ctx.IndentAt(call)

	// Skip calls that contain inline comments - reformatting would lose them
	if hasLineComment(string(original)) {
		return nil, false
	}

	// Check if call fits on one line - if so, skip formatting
	// Collapse whitespace to estimate single-line width
	callText := string(original)
	currentLineLen := collapsedLineLenAt(ctx.Source, start, callText, ctx.TabStop)

	if currentLineLen <= ctx.ColumnLimit {
		// Call fits on one line, no need to wrap
		return nil, false
	}

	var formatted string
	if a.FormatFunc != nil {
		// Use the provided formatter (legacy formatter)
		formatted = a.FormatFunc(original, wsIndent, ctx.ColumnLimit, ctx.TabStop)
	} else {
		// Fallback to ReflowCallAction with StrategyLeftFlow
		formatted = formatCallPackedSimple(call, wsIndent, ctx)
	}

	if formatted == string(original) {
		return nil, false
	}

	// Only normalize the original with gofmt to get a canonical form.
	// We compare the formatted output against this normalized original.
	// We do NOT normalize the formatted output because our formatter may
	// intentionally expand composite literals (maps, structs) that gofmt
	// would keep inline - we want to preserve our intentional expansions.
	origNorm := normalizeCallWithGofmt(string(original), wsIndent)

	if formatted == origNorm {
		// The formatted version equals the gofmt-normalized original,
		// so no meaningful change was made.
		return nil, false
	}

	// Build result with formatted call
	out, err := ApplySingleEdit(ctx.Source, start, end, []byte(formatted))
	if err != nil {
		return nil, false
	}
	return out, true
}

// OnePerLineMultiLineCallAction formats a call as a multiline call with one
// argument per line. This matches the legacy MultiLineCallFormatter style more
// closely than PackedMultiLineCallAction (which packs args when possible).
type OnePerLineMultiLineCallAction struct {
	Target string
}

// Execute implements Action for OnePerLineMultiLineCallAction.
func (a *OnePerLineMultiLineCallAction) Execute(caps Captures, ctx *Context) ([]byte, bool) {
	node := resolveTarget(caps, a.Target)
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return nil, false
	}
	if len(call.Args) == 0 {
		return nil, false
	}

	start := ctx.Fset.Position(call.Pos()).Offset
	end := ctx.Fset.Position(call.End()).Offset
	if start < 0 || end > len(ctx.Source) || start >= end {
		return nil, false
	}

	original := ctx.Source[start:end]
	wsIndent := ctx.IndentAt(call)

	// Skip calls that contain inline comments - AST rendering would drop them.
	if hasLineComment(string(original)) {
		return nil, false
	}

	// Skip if the call fits when collapsed to a single line.
	callText := string(original)
	currentLineLen := collapsedLineLenAt(ctx.Source, start, callText, ctx.TabStop)
	if currentLineLen <= ctx.ColumnLimit {
		return nil, false
	}

	formatted := formatCallOnePerLine(call, wsIndent, ctx)
	if formatted == string(original) {
		return nil, false
	}

	out, err := ApplySingleEdit(ctx.Source, start, end, []byte(formatted))
	if err != nil {
		return nil, false
	}
	return out, true
}

// formatCallPackedSimple is a simplified packed multiline formatter used when
// no legacy formatter is provided.
func formatCallPackedSimple(call *ast.CallExpr, indent string, ctx *Context) string {
	if len(call.Args) == 0 {
		return renderNode(call, ctx.Fset)
	}

	var b strings.Builder

	// Write function name and opening paren, then newline
	funcSrc := renderNode(call.Fun, ctx.Fset)
	b.WriteString(funcSrc)
	b.WriteString("(\n")

	contIndent := indent + "\t"
	contIndentWidth := visualLen(contIndent, ctx.TabStop)

	// Start on new line
	lineWidth := contIndentWidth
	b.WriteString(contIndent)

	for i, arg := range call.Args {
		argSrc := renderNode(arg, ctx.Fset)
		argWidth := visualLen(argSrc, ctx.TabStop)

		if i > 0 {
			// Check if arg fits on current line
			commaSpace := 2 // ", "
			if lineWidth+commaSpace+argWidth <= ctx.ColumnLimit {
				b.WriteString(", ")
				b.WriteString(argSrc)
				lineWidth += commaSpace + argWidth
			} else {
				// Break to new line
				b.WriteString(",\n")
				b.WriteString(contIndent)
				b.WriteString(argSrc)
				lineWidth = contIndentWidth + argWidth
			}
		} else {
			b.WriteString(argSrc)
			lineWidth += argWidth
		}
	}

	// Trailing comma and closing paren on its own line
	b.WriteString(",\n")
	b.WriteString(indent)
	b.WriteString(")")

	return b.String()
}

// formatCallLeftFlowSimple is a simplified left-flow formatter used when
// no legacy formatter is provided.
func formatCallLeftFlowSimple(call *ast.CallExpr, indent string, ctx *Context) string {
	if len(call.Args) == 0 {
		return renderNode(call, ctx.Fset)
	}

	var b strings.Builder

	// Write function name and opening paren
	funcSrc := renderNode(call.Fun, ctx.Fset)
	b.WriteString(funcSrc)
	b.WriteString("(")

	contIndent := indent + "\t"
	contIndentWidth := visualLen(contIndent, ctx.TabStop)

	// Start on same line as opening paren
	lineWidth := visualLen(indent, ctx.TabStop) + len(funcSrc) + 1

	for i, arg := range call.Args {
		argSrc := renderNode(arg, ctx.Fset)

		// Check if this is a string literal that can be split
		if expr, err := parser.ParseExpr(argSrc); err == nil {
			if strText, ok := llast.FlattenStringExprAST(expr); ok {
				// This is a string - use special handling
				if i > 0 {
					// Try to fit on current line with ", "
					quoted := text.QuoteGoString(strText)
					if lineWidth+2+visualLen(quoted, ctx.TabStop) <= ctx.ColumnLimit {
						b.WriteString(", ")
						b.WriteString(quoted)
						lineWidth += 2 + visualLen(quoted, ctx.TabStop)
						continue
					}
					// Need to break - end current line and start new
					b.WriteString(",\n")
					b.WriteString(contIndent)
					lineWidth = contIndentWidth
				}

				// Split the string if needed
				split := text.SplitQuotedString(strText, lineWidth, contIndent,
					ctx.ColumnLimit, ctx.TabStop)
				b.WriteString(split)
				// Update lineWidth to last line of split
				if idx := strings.LastIndex(split, "\n"); idx >= 0 {
					lineWidth = contIndentWidth + visualLen(split[idx+1:], ctx.TabStop)
				} else {
					lineWidth += visualLen(split, ctx.TabStop)
				}
				continue
			}
		}

		// Non-string argument
		argWidth := visualLen(argSrc, ctx.TabStop)

		if i == 0 {
			// First arg - write directly after opening paren
			b.WriteString(argSrc)
			lineWidth += argWidth
		} else {
			// Check if this arg fits on current line
			needed := 2 + argWidth // ", " + arg
			if lineWidth+needed <= ctx.ColumnLimit {
				b.WriteString(", ")
				b.WriteString(argSrc)
				lineWidth += needed
			} else {
				// Start new line
				b.WriteString(",\n")
				b.WriteString(contIndent)
				b.WriteString(argSrc)
				lineWidth = contIndentWidth + argWidth
			}
		}
	}

	b.WriteString(")")
	return b.String()
}

// SignatureFormatFunc is the signature for the function signature formatting function.
// This allows injecting the legacy formatter implementation to avoid circular imports.
// Returns the formatted signature and whether a blank line should be added after.
type SignatureFormatFunc func(signature, indent string, colLimit, tabStop int) (string, bool)

// BreakFuncSignatureAction breaks a long function signature using left-flow packing.
// It extracts the entire signature line and reformats it.
type BreakFuncSignatureAction struct {
	Target     string
	FormatFunc SignatureFormatFunc
}

// Execute implements Action for BreakFuncSignatureAction.
func (a *BreakFuncSignatureAction) Execute(caps Captures, ctx *Context) ([]byte, bool) {
	node := resolveTarget(caps, a.Target)

	funcDecl, ok := node.(*ast.FuncDecl)
	if !ok || funcDecl == nil {
		return nil, false
	}

	// Find the opening brace of the function body
	if funcDecl.Body == nil || !funcDecl.Body.Lbrace.IsValid() {
		return nil, false
	}

	// Get positions
	funcStart := ctx.Fset.Position(funcDecl.Pos()).Offset
	bracePos := ctx.Fset.Position(funcDecl.Body.Lbrace).Offset

	// Extract the signature from "func" to "{"
	signature := strings.TrimSpace(string(ctx.Source[funcStart : bracePos+1]))

	// Get the indent
	indent := ctx.IndentAt(node)

	// Format using the injected formatter or fallback
	var formatted string
	var needsBlank bool

	if a.FormatFunc != nil {
		formatted, needsBlank = a.FormatFunc(signature, indent, ctx.ColumnLimit, ctx.TabStop)
	} else {
		// Fallback: use simple break after first comma that exceeds limit
		formatted, needsBlank = formatSignatureSimple(signature, indent, ctx.ColumnLimit, ctx.TabStop)
	}

	// Check if the formatted signature is different
	// We need to compare the actual strings, not normalized versions,
	// since formatting adds newlines and indentation
	if formatted == indent+signature {
		return nil, false
	}

	// Find where to resume after the opening brace
	afterBrace := bracePos + 1

	// Find the start of the line containing the signature
	lineStart := funcStart
	for lineStart > 0 && ctx.Source[lineStart-1] != '\n' {
		lineStart--
	}

	// If multi-line and there's content after the brace, add blank line
	// But skip blank line if the signature already has nested multiline content
	// (e.g., func types with multiline struct params) - adding more space would be excessive
	hasNestedMultiline := strings.Contains(formatted, "func(\n")
	if needsBlank && !hasNestedMultiline {
		// Check if next non-whitespace is a newline
		pos := afterBrace
		for pos < len(ctx.Source) && (ctx.Source[pos] == ' ' || ctx.Source[pos] == '\t') {
			pos++
		}
		if pos < len(ctx.Source) && ctx.Source[pos] == '\n' {
			// There's already a newline after brace, check next line
			pos++
			// Skip leading whitespace on next line
			lineContentStart := pos
			for pos < len(ctx.Source) && (ctx.Source[pos] == ' ' || ctx.Source[pos] == '\t') {
				pos++
			}
			if pos < len(ctx.Source) && ctx.Source[pos] != '\n' && ctx.Source[pos] != '}' {
				// Next line has content and is not just a closing brace.
				// Apply two non-overlapping edits:
				// 1) Replace the signature (including the opening brace).
				// 2) Insert one extra newline after the existing newline following the brace.
				var b EditBuilder
				b.Replace(lineStart, afterBrace, []byte(formatted))
				b.Insert(lineContentStart, []byte("\n"))
				out, changed, err := b.Apply(ctx.Source)
				if err != nil {
					return nil, false
				}
				return out, changed
			}
		}
	}

	out, err := ApplySingleEdit(ctx.Source, lineStart, afterBrace, []byte(formatted))
	if err != nil {
		return nil, false
	}
	return out, true
}

// normalizeSignature normalizes a signature for comparison.
func normalizeSignature(sig, indent string) string {
	// Remove all leading/trailing whitespace and normalize internal spaces
	sig = strings.TrimPrefix(sig, indent)
	var normalized strings.Builder
	inSpace := false
	for _, c := range sig {
		if c == ' ' || c == '\t' || c == '\n' {
			if !inSpace {
				normalized.WriteByte(' ')
				inSpace = true
			}
		} else {
			normalized.WriteRune(c)
			inSpace = false
		}
	}
	return strings.TrimSpace(normalized.String())
}

// formatSignatureSimple is a fallback formatter that breaks at commas.
// Uses left-flow packing: break BEFORE elements that would exceed the limit.
func formatSignatureSimple(sig, indent string, colLimit, tabStop int) (string, bool) {
	// Parse out params from signature: func name(params) returns {
	openParen := strings.Index(sig, "(")
	if openParen == -1 {
		return indent + sig, false
	}

	// Find matching close paren
	depth := 0
	closeParen := -1
	for i := openParen; i < len(sig); i++ {
		if sig[i] == '(' {
			depth++
		} else if sig[i] == ')' {
			depth--
			if depth == 0 {
				closeParen = i
				break
			}
		}
	}
	if closeParen == -1 {
		return indent + sig, false
	}

	prefix := sig[:openParen+1] // "func name("
	params := sig[openParen+1 : closeParen]
	suffix := sig[closeParen:] // ") returns {"

	// Split params
	paramList := splitTopLevelSimple(params)
	if len(paramList) == 0 {
		return indent + sig, false
	}

	// Build result using left-flow packing
	var result strings.Builder
	result.WriteString(indent)
	result.WriteString(prefix)

	contIndent := indent + "\t"
	currentLine := indent + prefix

	for i, param := range paramList {
		param = strings.TrimSpace(param)
		if param == "" {
			continue
		}

		separator := ""
		if i > 0 {
			separator = ", "
		}

		testAdd := separator + param
		testLine := currentLine + testAdd

		// For last param, include suffix in check
		isLast := i == len(paramList)-1
		lineWithSuffix := testLine
		if isLast {
			lineWithSuffix = testLine + suffix
		}

		if visualLen(lineWithSuffix, tabStop) > colLimit {
			// Break to new line
			if i > 0 {
				result.WriteByte(',')
			}
			result.WriteByte('\n')
			result.WriteString(contIndent)
			result.WriteString(param)
			currentLine = contIndent + param
		} else {
			if i > 0 {
				result.WriteString(", ")
			}
			result.WriteString(param)
			currentLine = testLine
		}
	}

	result.WriteString(suffix)

	isMultiLine := strings.Contains(result.String(), "\n")
	return result.String(), isMultiLine
}

// splitTopLevelSimple splits a string at top-level commas.
func splitTopLevelSimple(s string) []string {
	var result []string
	var current strings.Builder
	depth := 0
	inString := false
	escaped := false

	for i := 0; i < len(s); i++ {
		c := s[i]

		if escaped {
			escaped = false
			current.WriteByte(c)
			continue
		}
		if c == '\\' && inString {
			escaped = true
			current.WriteByte(c)
			continue
		}
		if c == '"' || c == '`' {
			inString = !inString
			current.WriteByte(c)
			continue
		}
		if inString {
			current.WriteByte(c)
			continue
		}

		if c == '(' || c == '[' || c == '{' {
			depth++
		} else if c == ')' || c == ']' || c == '}' {
			depth--
		}

		if c == ',' && depth == 0 {
			result = append(result, current.String())
			current.Reset()
			continue
		}

		current.WriteByte(c)
	}

	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}

// findSignatureBreakPoint finds the best position to break in a field list.
// Returns the offset and whether it's after a comma.
func findSignatureBreakPoint(fields []*ast.Field, listStart, listEnd int,
	indent string, ctx *Context) (int, bool) {

	if len(fields) == 0 {
		return -1, false
	}

	// For parameters/results with multiple fields, find the rightmost comma
	// that keeps the prefix under the column limit
	type breakInfo struct {
		pos       int  // position right after comma
		prefixLen int  // visual width of prefix
		isComma   bool // whether this is after a comma
	}

	var breaks []breakInfo
	pos := ctx.Fset.Position(fields[0].Pos())
	lineStart := pos.Offset - pos.Column + 1

	for i := 0; i < len(fields)-1; i++ {
		field := fields[i]
		fieldEnd := ctx.Fset.Position(field.End()).Offset

		// Find comma after this field
		commaPos := fieldEnd
		for commaPos < listEnd && ctx.Source[commaPos] != ',' {
			commaPos++
		}
		if commaPos >= listEnd {
			continue
		}

		// Calculate prefix width
		prefix := string(ctx.Source[lineStart : commaPos+1])
		prefixWidth := visualLen(prefix, ctx.TabStop)

		breaks = append(breaks, breakInfo{
			pos:       commaPos + 1,
			prefixLen: prefixWidth,
			isComma:   true,
		})
	}

	if len(breaks) == 0 {
		return -1, false
	}

	// Find the rightmost break point that keeps prefix under limit
	for i := len(breaks) - 1; i >= 0; i-- {
		b := breaks[i]
		if b.prefixLen <= ctx.ColumnLimit {
			return b.pos, b.isComma
		}
	}

	// Fallback to first break point
	return breaks[0].pos, breaks[0].isComma
}

// InterfaceMethodFormatFunc is the signature for interface method formatting.
type InterfaceMethodFormatFunc func(method, indent string, colLimit, tabStop int) string

// BreakInterfaceMethodAction formats a long interface method declaration.
type BreakInterfaceMethodAction struct {
	Target     string
	FormatFunc InterfaceMethodFormatFunc
}

// Execute implements Action for BreakInterfaceMethodAction.
func (a *BreakInterfaceMethodAction) Execute(caps Captures, ctx *Context) ([]byte, bool) {
	node := resolveTarget(caps, a.Target)

	field, ok := node.(*ast.Field)
	if !ok || field == nil {
		return nil, false
	}

	// Check this is actually a method (has function type)
	funcType, ok := field.Type.(*ast.FuncType)
	if !ok || funcType == nil {
		return nil, false
	}

	// Check if line width exceeds limit
	if ctx.LineWidth(node) <= ctx.ColumnLimit {
		return nil, false
	}

	// Get positions
	fieldStart := ctx.Fset.Position(field.Pos()).Offset
	fieldEnd := ctx.Fset.Position(field.End()).Offset

	// Extract the method declaration
	method := strings.TrimSpace(string(ctx.Source[fieldStart:fieldEnd]))

	// Get the indent
	indent := ctx.IndentAt(node)

	// Format using the injected formatter or fallback
	var formatted string

	if a.FormatFunc != nil {
		formatted = a.FormatFunc(method, indent, ctx.ColumnLimit, ctx.TabStop)
	} else {
		// Fallback: use simple break
		formatted = formatMethodSimple(method, indent, ctx.ColumnLimit, ctx.TabStop)
	}

	// Check if formatted is different
	if formatted == indent+method {
		return nil, false
	}

	// Find the start of the line
	ls := lineStart(ctx.Source, fieldStart)

	// Find end of line after the method
	le := lineEnd(ctx.Source, fieldEnd)
	out, err := ApplySingleEdit(ctx.Source, ls, le, []byte(formatted))
	if err != nil {
		return nil, false
	}
	return out, true
}

// formatMethodSimple is a fallback formatter for interface methods.
func formatMethodSimple(method, indent string, colLimit, tabStop int) string {
	// Simple fallback - break at first comma that exceeds limit
	prefixLen := visualLen(indent, tabStop)
	var result strings.Builder
	result.WriteString(indent)

	currentLen := prefixLen
	lastBreak := 0
	inParen := 0

	for i, c := range method {
		if c == '(' {
			inParen++
		} else if c == ')' {
			inParen--
		}

		currentLen++

		if c == ',' && inParen >= 1 && currentLen > colLimit {
			result.WriteString(method[lastBreak : i+1])
			result.WriteByte('\n')
			result.WriteString(indent)
			result.WriteByte('\t')
			lastBreak = i + 1
			for lastBreak < len(method) && method[lastBreak] == ' ' {
				lastBreak++
			}
			currentLen = prefixLen + tabStop
		}
	}

	if lastBreak < len(method) {
		result.WriteString(method[lastBreak:])
	}

	return result.String()
}

// InsertBlankBeforeAction inserts a blank line before a node if not already present.
type InsertBlankBeforeAction struct {
	Target string
}

// Execute implements Action for InsertBlankBeforeAction.
func (a *InsertBlankBeforeAction) Execute(caps Captures, ctx *Context) ([]byte, bool) {
	node := resolveTarget(caps, a.Target)
	if node == nil {
		return nil, false
	}

	pos := ctx.Fset.Position(node.Pos())
	nodeStart := pos.Offset

	// Find the start of the line
	ls := lineStart(ctx.Source, nodeStart)

	// Check if there's already a blank line before this line
	if ls >= 2 {
		// Look for two consecutive newlines before lineStart
		checkPos := ls - 1
		// Skip the newline at lineStart-1
		if checkPos > 0 && ctx.Source[checkPos] == '\n' {
			checkPos--
		}
		// Skip any trailing whitespace on the previous line
		for checkPos > 0 && (ctx.Source[checkPos] == ' ' || ctx.Source[checkPos] == '\t') {
			checkPos--
		}
		// If we hit another newline, there's already a blank line
		if checkPos >= 0 && ctx.Source[checkPos] == '\n' {
			return nil, false
		}
	}

	// Insert blank line before the current line
	out, err := ApplySingleEdit(ctx.Source, ls, ls, []byte("\n"))
	if err != nil {
		return nil, false
	}
	return out, true
}

// InsertBlankAfterAction inserts a blank line after a node if not already present.
type InsertBlankAfterAction struct {
	Target string
}

// Execute implements Action for InsertBlankAfterAction.
func (a *InsertBlankAfterAction) Execute(caps Captures, ctx *Context) ([]byte, bool) {
	node := resolveTarget(caps, a.Target)
	if node == nil {
		return nil, false
	}

	endPos := ctx.Fset.Position(node.End()).Offset
	if endPos >= len(ctx.Source) {
		return nil, false
	}

	// Find the end of the line
	le := lineEnd(ctx.Source, endPos)

	// Skip the newline if present
	if le < len(ctx.Source) && ctx.Source[le] == '\n' {
		le++
	}

	// Check if there's already a blank line after
	if le < len(ctx.Source) {
		// Skip whitespace-only lines
		checkPos := le
		for checkPos < len(ctx.Source) && (ctx.Source[checkPos] == ' ' || ctx.Source[checkPos] == '\t') {
			checkPos++
		}
		if checkPos < len(ctx.Source) && ctx.Source[checkPos] == '\n' {
			// There's already a blank line
			return nil, false
		}
	}

	// Insert blank line after the current line
	out, err := ApplySingleEdit(ctx.Source, le, le, []byte("\n"))
	if err != nil {
		return nil, false
	}
	return out, true
}

// BreakMethodChainAction breaks a method chain with one call per line.
// The dot is placed at the end of each line (trailing dot style).
type BreakMethodChainAction struct {
	Target string
}

// Execute implements Action for BreakMethodChainAction.
func (a *BreakMethodChainAction) Execute(caps Captures, ctx *Context) ([]byte, bool) {
	node := resolveTarget(caps, a.Target)
	if node == nil {
		return nil, false
	}

	// Find the outermost call expression in the chain
	call, ok := node.(*ast.CallExpr)
	if !ok {
		// Could be an assignment, extract the RHS
		if assign, ok := node.(*ast.AssignStmt); ok && len(assign.Rhs) > 0 {
			call, ok = assign.Rhs[0].(*ast.CallExpr)
			if !ok {
				return nil, false
			}
		} else {
			return nil, false
		}
	}

	// Check if this is actually a method chain (has selector expressions)
	chainCalls := collectMethodChain(call)
	if len(chainCalls) < 2 {
		return nil, false
	}

	// Get source for the call chain
	callStart := ctx.Fset.Position(call.Pos()).Offset
	callEnd := ctx.Fset.Position(call.End()).Offset
	if callStart < 0 || callEnd > len(ctx.Source) || callStart >= callEnd {
		return nil, false
	}
	originalCall := string(ctx.Source[callStart:callEnd])

	// Skip method chains that contain inline comments. Reformatting via AST
	// rendering would drop these comments.
	if hasLineComment(originalCall) {
		return nil, false
	}

	// Skip if chain is already multi-line (has been formatted)
	if strings.Contains(originalCall, "\n") {
		return nil, false
	}

	// Check if line already fits
	if ctx.LineWidth(node) <= ctx.ColumnLimit {
		return nil, false
	}

	// Get the original source positions
	// We need to find the start of the chain (the receiver) and end of the last call
	firstCall := chainCalls[len(chainCalls)-1] // innermost
	var chainStart int

	// Walk back to find the actual start (receiver expression)
	if sel, ok := firstCall.Fun.(*ast.SelectorExpr); ok {
		chainStart = ctx.Fset.Position(sel.X.Pos()).Offset
	} else {
		chainStart = ctx.Fset.Position(firstCall.Pos()).Offset
	}

	lastCall := chainCalls[0] // outermost
	chainEnd := ctx.Fset.Position(lastCall.End()).Offset

	if chainStart < 0 || chainEnd > len(ctx.Source) || chainStart >= chainEnd {
		return nil, false
	}

	// Compute the already-present prefix width on the line before the chain
	// (e.g. "result := " before "client.Foo().Bar()"). This avoids decisions
	// that would fit if the chain started at column 0, but overflow once the
	// actual prefix is accounted for.
	ls := lineStart(ctx.Source, chainStart)
	prefixWidth := visualLen(string(ctx.Source[ls:chainStart]), ctx.TabStop)

	// Get the base indentation (whitespace only; used for continuation lines).
	indent := ctx.IndentAt(node)

	formatted := formatMethodChain(chainCalls, indent, prefixWidth, ctx)

	original := string(ctx.Source[chainStart:chainEnd])

	// Check if formatting actually changed anything
	if formatted == original {
		return nil, false
	}

	// Build result by replacing the chain in source
	out, err := ApplySingleEdit(ctx.Source, chainStart, chainEnd, []byte(formatted))
	if err != nil {
		return nil, false
	}
	return out, true
}

// collectMethodChain collects all calls in a method chain from outermost to innermost.
func collectMethodChain(call *ast.CallExpr) []*ast.CallExpr {
	var calls []*ast.CallExpr
	current := call

	for current != nil {
		calls = append(calls, current)

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

	return calls
}

// formatMethodChain formats a method chain with smart breaking.
// It packs as many calls as fit on each line and only breaks when needed.
// When a call's arguments need wrapping, the arguments are placed on a new line
// with the closing ) followed by the next method call.
func formatMethodChain(calls []*ast.CallExpr, indent string, initialLineWidth int, ctx *Context) string {
	if len(calls) == 0 {
		return ""
	}

	var b strings.Builder
	contIndent := indent + "\t"

	// Process calls from innermost to outermost
	currentLineWidth := initialLineWidth

	for i := len(calls) - 1; i >= 0; i-- {
		call := calls[i]

		// Build the method part: ".MethodName" or "receiver.MethodName" for first
		var methodPart string
		if i == len(calls)-1 {
			// First call - include the receiver
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if ok {
				receiverSrc := renderNode(sel.X, ctx.Fset)
				methodPart = receiverSrc + "." + sel.Sel.Name
			} else {
				methodPart = renderNode(call.Fun, ctx.Fset)
			}
		} else {
			// Subsequent calls - just .Method name (the dot is added when we write)
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if ok {
				methodPart = sel.Sel.Name
			}
		}

		// Build the arguments part
		var argParts []string
		for _, arg := range call.Args {
			argParts = append(argParts, renderNode(arg, ctx.Fset))
		}
		argsInline := strings.Join(argParts, ", ")

		// Calculate the inline version of this call
		var callInline string
		if i == len(calls)-1 {
			callInline = methodPart + "(" + argsInline + ")"
		} else {
			callInline = "." + methodPart + "(" + argsInline + ")"
		}
		callWidth := visualLen(callInline, ctx.TabStop)

		// Check if the call fits on the current line
		if currentLineWidth+callWidth <= ctx.ColumnLimit {
			// It fits - write inline
			if i == len(calls)-1 {
				b.WriteString(methodPart)
			} else {
				b.WriteString(".")
				b.WriteString(methodPart)
			}
			b.WriteString("(")
			b.WriteString(argsInline)
			b.WriteString(")")
			currentLineWidth += callWidth
		} else {
			// Doesn't fit - need to wrap
			// Check if the method call with just opening paren fits
			methodWidth := visualLen("."+methodPart+"(", ctx.TabStop)
			if i == len(calls)-1 {
				methodWidth = visualLen(methodPart+"(", ctx.TabStop)
			}

			if currentLineWidth+methodWidth+visualLen(argsInline+")", ctx.TabStop) > ctx.ColumnLimit &&
				len(call.Args) > 0 {
				// Arguments need to be on a new line
				if i == len(calls)-1 {
					b.WriteString(methodPart)
				} else {
					b.WriteString(".")
					b.WriteString(methodPart)
				}
				b.WriteString("(\n")
				b.WriteString(contIndent)
				b.WriteString(argsInline)
				b.WriteString(",\n")
				b.WriteString(indent)
				b.WriteString(")")
				currentLineWidth = visualLen(indent, ctx.TabStop) + 1 // After the closing paren
			} else {
				// The call itself is too long but args are short
				// Just write it inline from current position
				if i == len(calls)-1 {
					b.WriteString(methodPart)
				} else {
					b.WriteString(".")
					b.WriteString(methodPart)
				}
				b.WriteString("(")
				b.WriteString(argsInline)
				b.WriteString(")")
				currentLineWidth += callWidth
			}
		}
	}

	return b.String()
}

// BreakReturnValuesAction breaks long return values to a new line.
type BreakReturnValuesAction struct {
	Target string
}

// Execute implements Action for BreakReturnValuesAction.
func (a *BreakReturnValuesAction) Execute(caps Captures, ctx *Context) ([]byte, bool) {
	node := resolveTarget(caps, a.Target)

	var funcType *ast.FuncType

	switch n := node.(type) {
	case *ast.FuncDecl:
		funcType = n.Type
	case *ast.FuncType:
		funcType = n
	default:
		return nil, false
	}

	if funcType == nil || funcType.Results == nil || len(funcType.Results.List) == 0 {
		return nil, false
	}

	// Check if line exceeds limit
	if ctx.LineWidth(node) <= ctx.ColumnLimit {
		return nil, false
	}

	// Check if results have opening paren
	if !funcType.Results.Opening.IsValid() {
		return nil, false
	}

	resultsOpen := ctx.Fset.Position(funcType.Results.Opening).Offset

	// Check if there's already a newline after opening paren
	i := resultsOpen + 1
	i = skipHorizontalWhitespace(ctx.Source, i)
	if i < len(ctx.Source) && ctx.Source[i] == '\n' {
		return nil, false
	}

	indent := ctx.IndentAt(node)

	out, changed, err := applyContinuationIndentAfter(ctx.Source, resultsOpen+1, indent)
	if err != nil {
		return nil, false
	}
	return out, changed
}
