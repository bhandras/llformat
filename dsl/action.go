package dsl

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	stdscanner "go/scanner"
	"go/token"
	"strings"
	"unicode"

	llast "github.com/bhandras/llformat/ast"
	"github.com/bhandras/llformat/dsl/layout"
	"github.com/bhandras/llformat/scanner"
	"github.com/bhandras/llformat/text"
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
func (a *KeepTogetherAction) Execute(caps Captures, ctx *Context) ([]byte,
	bool) {

	node := resolveTarget(caps, a.Target)
	if node != nil {
		ctx.MarkAtomic(node)
	}

	// This doesn't change source, just marks the node
	return nil, false
}

// TryElseAction tries the first action, falls back to second if it doesn't
// help.
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
	// StrategyAdaptive uses one-per-line if any arg is multiline, else
	// left-pack.
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

	// Skip calls that contain inline comments - reformatting via AST
	// rendering would drop them.
	if !isSafeStandaloneExprSpan(original) {
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
	out, err := replaceSpan(ctx.Source, start, end, []byte(formatted))
	if err != nil {
		return nil, false
	}

	// Safety: never emit syntactically invalid Go. Layout-driven formatting
	// can interact with semicolon insertion and nested rewrites in
	// surprising ways; if the result is not parseable, bail out so the
	// caller can fall back to a safer formatter (e.g. packed/legacy
	// multiline).
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(
		fset, "out.go", out, parser.AllErrors,
	); err != nil {
		return nil, false
	}

	return out, true
}

// findCallToReflow finds a call in a method chain that benefits from reflowing.
// Returns the first call where reflowing reduces line width below the limit.
func findCallToReflow(call *ast.CallExpr, ctx *Context) *ast.CallExpr {
	chain := collectMethodChainCalls(call)
	if len(chain) == 0 {
		return nil
	}

	// Try each call from the outermost inward; we prefer keeping the most
	// structure of the chain intact.
	for _, c := range chain {
		if len(c.Args) == 0 {
			continue
		}

		// Check if the line containing this call exceeds the limit
		if ctx.LineWidth(c) <= ctx.ColumnLimit {
			continue
		}

		indent := ctx.IndentAt(c)
		start := ctx.Fset.Position(c.Pos()).Offset
		end := ctx.Fset.Position(c.End()).Offset
		if start < 0 || end < 0 || start >= end ||
			end > len(ctx.Source) {

			continue
		}

		formatted := formatCallOnePerLine(c, indent, ctx)
		original := string(ctx.Source[start:end])

		if formatted == original {
			continue
		}

		if !reflowedCallFitsAnyLineWithinLimit(
			ctx, start, formatted, end,
		) {

			continue
		}

		return c
	}

	return nil
}

func collectMethodChainCalls(call *ast.CallExpr) []*ast.CallExpr {
	// Given an outermost call like `a().b().c()`, walk the `SelectorExpr`
	// receiver chain and return calls from outermost to innermost: `[c(),
	// b(), a()]`.
	var calls []*ast.CallExpr
	for current := call; current != nil; {
		calls = append(calls, current)

		sel, ok := current.Fun.(*ast.SelectorExpr)
		if !ok {
			break
		}

		nextCall, ok := sel.X.(*ast.CallExpr)
		if !ok {
			break
		}

		current = nextCall
	}

	return calls
}

func reflowedCallFitsAnyLineWithinLimit(ctx *Context, start int,
	replacement string, end int) bool {

	newBytes, err := ApplySingleEdit(
		ctx.Source, start, end, []byte(replacement),
	)
	if err != nil {
		return false
	}

	newFset := token.NewFileSet()
	newFile, err := parser.ParseFile(newFset, "", newBytes, 0)
	if err != nil {
		return false
	}

	newCtx := NewContext(newFset, newBytes, ctx.ColumnLimit, ctx.TabStop)

	// We only need a coarse sanity check: if *any* call in the rewritten
	// region now fits within the column limit, the rewrite is considered an
	// improvement.
	//
	// Note: we identify the rewritten call by position approximation (byte
	// offsets in the rewritten region). This is sufficient here because we
	// only use it as a filter; the actual call rewrite is applied later by
	// a separate action.
	rewrittenEnd := start + len(replacement)
	improved := false
	ast.Inspect(
		newFile,
		func(n ast.Node) bool {
			nc, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			pos := newFset.Position(nc.Pos()).Offset
			if pos < start || pos > rewrittenEnd {
				return true
			}

			if newCtx.LineWidth(nc) <= ctx.ColumnLimit {
				improved = true

				return false
			}

			return true
		},
	)

	return improved
}

// formatCallOnePerLine formats a call with each argument on its own line.
func formatCallOnePerLine(call *ast.CallExpr, indent string,
	ctx *Context) string {

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
func formatCallLeftPack(call *ast.CallExpr, indent string,
	ctx *Context) string {

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
			// Check if this arg fits on current line Need space for
			// ", " + arg + potential trailing comma
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
func formatCallAdaptive(call *ast.CallExpr, indent string,
	ctx *Context) string {

	// Check if any argument is multi-line
	hasMultiLine := false
	hasComplex := false
	for _, arg := range call.Args {
		argSrc := renderNode(arg, ctx.Fset)
		if strings.Contains(argSrc, "\n") {
			hasMultiLine = true
			break
		}
		if !isSimpleCallArg(arg) {
			hasComplex = true
		}
	}

	// Prefer one-per-line when args are already multiline or contain
	// "complex" expressions (binary ops, calls, composites, etc). This
	// mirrors the legacy fixtures which keep complex argument expressions
	// visually separate.
	if hasMultiLine || hasComplex {
		return formatCallOnePerLine(call, indent, ctx)
	}

	return formatCallLeftPack(call, indent, ctx)
}

func isSimpleCallArg(arg ast.Expr) bool {
	switch a := arg.(type) {
	case *ast.Ident, *ast.BasicLit, *ast.SelectorExpr:
		return true

	case *ast.StarExpr:
		return isSimpleCallArg(a.X)

	case *ast.UnaryExpr:
		// Treat common unary wrappers (&x, -1) as simple when their
		// operand is.
		return isSimpleCallArg(a.X)

	case *ast.ParenExpr:
		return isSimpleCallArg(a.X)

	default:
		return false
	}
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
	out, changed, err := applyContinuationIndentAfter(
		ctx.Source, end, indent,
	)
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
func (a *BreakBeforeAction) Execute(caps Captures, ctx *Context) ([]byte,
	bool) {

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
	out, changed, err := applyContinuationIndentBefore(
		ctx.Source, pos, indent,
	)
	if err != nil {
		return nil, false
	}

	return out, changed
}

// BreakAtOpAction breaks a binary expression at the best operator position. It
// finds the rightmost operator that keeps the first part under the column
// limit. Prefers logical operators (&&, ||) over comparison/arithmetic when
// possible.
type BreakAtOpAction struct {
	Target     string
	BreakAfter bool // true = break after op (Go style), false = break before
}

// BreakLogicalChainLayoutAction breaks long &&/|| chains using the layout
// engine. It prefers breaking after each operator (Go style) and uses the
// standard continuation indent (newline + indent + one tab).
//
// This action is intended for opt-in layout formatting; the default packed
// logical-chain style should use BreakLogicalChainPackedAction.
type BreakLogicalChainLayoutAction struct {
	Target string
}

// BreakLogicalChainPackedAction breaks long &&/|| chains while preserving the
// compact packed style. It edits only whitespace after logical operators and
// can continue fixing already-multiline chains whose later physical lines still
// overflow.
type BreakLogicalChainPackedAction struct {
	Target string
}

// BreakArithmeticChainLayoutAction breaks long arithmetic chains for a single
// operator (e.g. `a + b + c + d`) using the layout engine.
//
// This is intentionally conservative: it will not rewrite mixed-op trees such
// as `a + b - c`, because those often carry intent and are more error-prone to
// restructure without a full layout/precedence engine.
type BreakArithmeticChainLayoutAction struct {
	Target string
}

// BreakAssignRHSAction moves an overlong single RHS expression onto a
// continuation line after the assignment operator.
type BreakAssignRHSAction struct {
	Target string
}

// BreakValueSpecGenericConversionAction breaks the type-argument list in a
// generic conversion value inside an overlong var declaration.
type BreakValueSpecGenericConversionAction struct {
	Target string
}

// BreakCaseClauseLayoutAction formats a long case clause list (`case A, B, C:`)
// using the layout engine. It breaks after commas using the standard
// continuation indentation (`indent + "\t"`).
type BreakCaseClauseLayoutAction struct {
	Target string
}

// BreakSelectorChainLayoutAction breaks long selector chains such as
// `pkg.subpkg.Symbol.Field` using the layout engine.
//
// It breaks after dots using a soft line break so that flat rendering produces
// `a.b.c` (no illegal spaces), while broken rendering produces:
//
// a. b. c
type BreakSelectorChainLayoutAction struct {
	Target string
}

// BreakMethodChainLayoutAction breaks method call chains such as:
//
// client.WithTimeout(30*time.Second).WithRetry(3).Execute(ctx, req)
//
// It breaks after dots (to avoid semicolon insertion) and indents continuation
// lines with the standard continuation indent (`indent + "\t"`).
//
// This action is intentionally conservative:
// - skips chains with inline comments (AST printing would drop them)
// - skips chains with multiline argument expressions
// - handles only selector-based calls (skips generic instantiation / index fun)
type BreakMethodChainLayoutAction struct {
	Target string
}

// BreakCallArgsLayoutAction formats a call expression by breaking its arguments
// across lines using the layout engine (with a gofmt-like tab indent).
//
// This is intended as an opt-in style that can fall back to existing packed/
// legacy formatters when it cannot safely operate (e.g. comments, multiline
// args).
type BreakCallArgsLayoutAction struct {
	Target string

	// Grouping optionally controls how the call argument list is grouped.
	//
	// Supported values:
	// - "" (default): one argument per line (forced break)
	// - "pairs": group args as (a, b) pairs when possible
	Grouping string
}

// StructuredLogCallAction formats structured logging calls by keeping the
// message prelude compact and packing trailing key/value pairs as units.
type StructuredLogCallAction struct {
	Target string
}

// BreakBinaryExprLayoutAction tries to break a binary expression using the
// configured operator-specific formatter.
//
// This provides a single entry point for contexts that capture a BinaryExpr
// (e.g. `for` conditions or `return` statements) without duplicating operator-
// specific rule logic.
type BreakBinaryExprLayoutAction struct {
	Target          string
	LogicalStyle    string // ""|"legacy" => packed, "layout" => layout engine
	ArithmeticStyle string // "legacy"|"layout"
}

// InsertBlankBeforeFirstStmtInBlockAction inserts a blank line between the
// opening delimiter of a block-like construct and its first body statement.
//
// This is used for the "next" profile to improve readability when a control
// statement header is already multiline (e.g. long `if` conditions, long `case`
// lists).
type InsertBlankBeforeFirstStmtInBlockAction struct {
	Target                 string
	SuppressWhenOnlyReturn bool
}

func (a *InsertBlankBeforeFirstStmtInBlockAction) Execute(caps Captures,
	ctx *Context) ([]byte, bool) {

	node := resolveTarget(caps, a.Target)
	if node == nil {
		return nil, false
	}

	var first ast.Stmt
	onlyReturn := false

	switch n := node.(type) {
	case *ast.IfStmt:
		if n == nil || n.Body == nil || len(n.Body.List) == 0 {
			return nil, false
		}
		first = n.Body.List[0]
		_, onlyReturn = first.(*ast.ReturnStmt)
		onlyReturn = onlyReturn && len(n.Body.List) == 1

	case *ast.ForStmt:
		if n == nil || n.Body == nil || len(n.Body.List) == 0 {
			return nil, false
		}
		first = n.Body.List[0]
		_, onlyReturn = first.(*ast.ReturnStmt)
		onlyReturn = onlyReturn && len(n.Body.List) == 1

	case *ast.CaseClause:
		if n == nil || len(n.Body) == 0 {
			return nil, false
		}
		first = n.Body[0]
		_, onlyReturn = first.(*ast.ReturnStmt)
		onlyReturn = onlyReturn && len(n.Body) == 1

	default:
		return nil, false
	}

	pos := ctx.Fset.Position(first.Pos())
	if pos.Offset < 0 || pos.Offset > len(ctx.Source) {
		return nil, false
	}

	// Insert at the start of the first statement's line so its indentation
	// remains. If the first statement has a leading comment-only block,
	// insert the blank line above the comment block so we don't split the
	// comment and the statement apart.
	lineStartIdx := lineStart(ctx.Source, pos.Offset)
	lineStartIdx = leadingCommentBlockLineStart(ctx.Source, lineStartIdx)
	if lineStartIdx < 0 || lineStartIdx > len(ctx.Source) {
		return nil, false
	}

	if a.SuppressWhenOnlyReturn && onlyReturn {
		return removeBlankBeforeLineStart(ctx.Source, lineStartIdx)
	}

	// Already has a blank line?
	if hasBlankLineBeforeLineStart(ctx.Source, lineStartIdx) {
		return nil, false
	}

	out, err := ApplySingleEdit(
		ctx.Source, lineStartIdx, lineStartIdx, []byte("\n"),
	)
	if err != nil {
		return nil, false
	}

	return out, true
}

func removeBlankBeforeLineStart(src []byte,
	targetLineStart int) ([]byte, bool) {

	if !hasBlankLineBeforeLineStart(src, targetLineStart) {
		return nil, false
	}

	prevLineEnd := targetLineStart - 1
	prevStart := lineStart(src, prevLineEnd)
	if prevStart < 0 || prevStart > targetLineStart {
		return nil, false
	}

	out, err := ApplySingleEdit(src, prevStart, targetLineStart, nil)
	if err != nil {
		return nil, false
	}

	return out, true
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
func (a *BreakAtOpAction) ExecuteEdits(caps Captures, ctx *Context) ([]Edit,
	bool, error) {

	node := resolveTarget(caps, a.Target)
	binExpr, ok := node.(*ast.BinaryExpr)
	if !ok {
		return nil, false, nil
	}

	// Check if line already fits
	if ctx.LineWidth(binExpr) <= ctx.ColumnLimit {
		return nil, false, nil
	}

	pos := ctx.Fset.Position(binExpr.Pos())
	indent := ctx.IndentAt(binExpr)

	// Calculate line start offset (not node start)
	lineStart := pos.Offset - pos.Column + 1

	ops := collectOpInfos(binExpr, ctx, lineStart)

	if len(ops) == 0 {
		return nil, false, nil
	}

	bestOp := selectBestOp(ops, ctx.ColumnLimit)

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
		{
			Start:   opEnd,
			End:     end,
			Replace: replacement,
		},
	}, true, nil
}

type opInfo struct {
	pos      int    // byte offset of operator
	opLen    int    // length of operator string
	opStr    string // operator string
	prefix   int    // visual width of content before this operator
	priority int    // operator priority (lower = prefer)
}

func collectOpInfos(binExpr *ast.BinaryExpr, ctx *Context,
	lineStart int) []opInfo {

	var ops []opInfo
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

		ops = append(
			ops, opInfo{
				pos:      opPos,
				opLen:    len(opStr),
				opStr:    opStr,
				prefix:   prefixWidth,
				priority: opPriority(opStr),
			},
		)

		// Recurse right
		collectOps(bin.Y)
	}
	collectOps(binExpr)

	return ops
}

func selectBestOp(ops []opInfo, colLimit int) *opInfo {
	// Find the best operator: prefer lower priority (logical) operators,
	// and among those, pick the rightmost that fits under column limit.
	var bestOp *opInfo
	for i := len(ops) - 1; i >= 0; i-- {
		op := &ops[i]
		if op.prefix > colLimit {
			continue
		}
		if bestOp == nil || op.priority < bestOp.priority ||
			(op.priority == bestOp.priority &&
				op.prefix > bestOp.prefix) {

			bestOp = op
		}
	}
	if bestOp != nil {
		return bestOp
	}

	// Fallback: if no good break point, pick lowest priority operator.
	for i := range ops {
		op := &ops[i]
		if bestOp == nil || op.priority < bestOp.priority {
			bestOp = op
		}
	}

	return bestOp
}

// Execute implements Action for BreakLogicalChainPackedAction.
func (a *BreakLogicalChainPackedAction) Execute(caps Captures, ctx *Context) (
	[]byte, bool) {

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

// ExecuteEdits implements EditAction for BreakLogicalChainPackedAction.
func (a *BreakLogicalChainPackedAction) ExecuteEdits(caps Captures,
	ctx *Context) ([]Edit, bool, error) {

	node := resolveTarget(caps, a.Target)
	binExpr, ok := node.(*ast.BinaryExpr)
	if !ok || binExpr == nil {
		return nil, false, nil
	}
	if binExpr.Op != token.LAND && binExpr.Op != token.LOR {
		return nil, false, nil
	}

	start := ctx.Fset.Position(binExpr.Pos()).Offset
	end := ctx.Fset.Position(binExpr.End()).Offset
	if start < 0 || end < 0 || start >= end || end > len(ctx.Source) {
		return nil, false, nil
	}
	if maxVisualFullLineLenInSpan(ctx.Source, start, end,
		ctx.TabStop) <= ctx.ColumnLimit {
		return nil, false, nil
	}

	original := string(ctx.Source[start:end])
	if !isSafeStandaloneExprSpan(original) {
		return nil, false, nil
	}

	candidates := collectLogicalBreakCandidates(
		ctx.Source[start:end], start,
	)
	if len(candidates) == 0 {
		return nil, false, nil
	}

	indent := ctx.IndentAt(binExpr)
	replacement := continuationIndentBytes(indent)

	for _, line := range overlongLinesInSpan(
		ctx.Source, start, end, ctx.TabStop, ctx.ColumnLimit,
	) {
		candidate := selectPackedLogicalBreak(
			ctx.Source, candidates, line, ctx.ColumnLimit,
			ctx.TabStop,
		)
		if candidate == nil {
			continue
		}

		opEnd := candidate.offset + candidate.length
		wsEnd := skipHorizontalWhitespace(ctx.Source, opEnd)
		if wsEnd < len(ctx.Source) && ctx.Source[wsEnd] == '\n' {
			continue
		}
		if bytes.Equal(ctx.Source[opEnd:wsEnd], replacement) {
			continue
		}

		return []Edit{
			{
				Start:   opEnd,
				End:     wsEnd,
				Replace: replacement,
			},
		}, true, nil
	}

	return nil, false, nil
}

type logicalBreakCandidate struct {
	offset int
	length int
	op     token.Token
	depth  int
}

type spanLine struct {
	start int
	end   int
}

func collectLogicalBreakCandidates(src []byte,
	baseOffset int) []logicalBreakCandidate {

	fset := token.NewFileSet()
	file := fset.AddFile("expr.go", -1, len(src))

	var s stdscanner.Scanner
	s.Init(file, src, nil, 0)

	var candidates []logicalBreakCandidate
	depth := 0
	for {
		pos, tok, _ := s.Scan()
		if tok == token.EOF {
			break
		}

		switch tok {
		case token.RPAREN, token.RBRACK, token.RBRACE:
			if depth > 0 {
				depth--
			}

		case token.LAND, token.LOR:
			rel := file.Offset(pos)
			candidates = append(
				candidates,
				logicalBreakCandidate{
					offset: baseOffset + rel,
					length: len(tok.String()),
					op:     tok,
					depth:  depth,
				},
			)

		case token.LPAREN, token.LBRACK, token.LBRACE:
			depth++
		}
	}

	return candidates
}

func overlongLinesInSpan(src []byte, start, end, tabStop,
	colLimit int) []spanLine {

	var lines []spanLine
	for lineStartOff := lineStart(src, start); lineStartOff < end; {
		lineEnd := lineStartOff
		for lineEnd < len(src) && src[lineEnd] != '\n' {
			lineEnd++
		}

		spanStart := lineStartOff
		if spanStart < start {
			spanStart = start
		}
		spanEnd := lineEnd
		if spanEnd > end {
			spanEnd = end
		}
		if spanStart < spanEnd &&
			visualLen(string(src[lineStartOff:lineEnd]),
				tabStop) > colLimit {

			lines = append(
				lines, spanLine{
					start: lineStartOff,
					end:   lineEnd,
				},
			)
		}

		if lineEnd >= len(src) || lineEnd >= end {
			break
		}
		lineStartOff = lineEnd + 1
	}

	return lines
}

func selectPackedLogicalBreak(src []byte, candidates []logicalBreakCandidate,
	line spanLine, colLimit, tabStop int) *logicalBreakCandidate {

	var onLine []logicalBreakCandidate
	for _, candidate := range candidates {
		if candidate.offset >= line.start &&
			candidate.offset < line.end {

			onLine = append(onLine, candidate)
		}
	}
	if len(onLine) == 0 {
		return nil
	}

	if candidate := bestFittingLogicalBreak(
		src, onLine, line.start, colLimit, tabStop,
	); candidate != nil {
		return candidate
	}

	return fallbackLogicalBreak(onLine)
}

func bestFittingLogicalBreak(src []byte, candidates []logicalBreakCandidate,
	lineStartOff, colLimit, tabStop int) *logicalBreakCandidate {

	var best *logicalBreakCandidate
	for i := range candidates {
		candidate := &candidates[i]
		opEnd := candidate.offset + candidate.length
		if visualLen(string(src[lineStartOff:opEnd]), tabStop) > colLimit {
			continue
		}
		if best == nil || logicalBreakLess(best, candidate) {
			best = candidate
		}
	}

	return best
}

func fallbackLogicalBreak(
	candidates []logicalBreakCandidate) *logicalBreakCandidate {

	var best *logicalBreakCandidate
	for i := range candidates {
		candidate := &candidates[i]
		if best == nil || logicalBreakFallbackLess(candidate, best) {
			best = candidate
		}
	}

	return best
}

func logicalBreakLess(current, candidate *logicalBreakCandidate) bool {
	if candidate.depth != current.depth {
		return candidate.depth < current.depth
	}
	if logicalBreakOpRank(candidate.op) != logicalBreakOpRank(current.op) {
		return logicalBreakOpRank(candidate.op) <
			logicalBreakOpRank(current.op)
	}

	return candidate.offset > current.offset
}

func logicalBreakFallbackLess(candidate, current *logicalBreakCandidate) bool {
	if candidate.depth != current.depth {
		return candidate.depth < current.depth
	}
	if logicalBreakOpRank(candidate.op) != logicalBreakOpRank(current.op) {
		return logicalBreakOpRank(candidate.op) <
			logicalBreakOpRank(current.op)
	}

	return candidate.offset < current.offset
}

func logicalBreakOpRank(op token.Token) int {
	switch op {
	case token.LOR:
		return 0

	case token.LAND:
		return 1

	default:
		return 2
	}
}

func flattenSameOpBinaryChain(expr ast.Expr, op token.Token,
	out *[]ast.Expr) bool {

	if expr == nil {
		return true
	}

	// Treat parenthesized expressions as atomic so we don't remove explicit
	// parentheses in the source.
	if _, ok := expr.(*ast.ParenExpr); ok {
		*out = append(*out, expr)

		return true
	}

	bin, ok := expr.(*ast.BinaryExpr)
	if !ok || bin.Op != op {
		*out = append(*out, expr)

		return true
	}

	if !flattenSameOpBinaryChain(bin.X, op, out) {
		return false
	}
	if !flattenSameOpBinaryChain(bin.Y, op, out) {
		return false
	}

	return true
}

// Execute implements Action for BreakLogicalChainLayoutAction.
func (a *BreakLogicalChainLayoutAction) Execute(caps Captures, ctx *Context) (
	[]byte, bool) {

	node := resolveTarget(caps, a.Target)
	binExpr, ok := node.(*ast.BinaryExpr)
	if !ok || binExpr == nil {
		return nil, false
	}

	if ctx.LineWidth(binExpr) <= ctx.ColumnLimit {
		return nil, false
	}

	if binExpr.Op != token.LAND && binExpr.Op != token.LOR {
		return nil, false
	}

	start := ctx.Fset.Position(binExpr.Pos()).Offset
	end := ctx.Fset.Position(binExpr.End()).Offset
	if start < 0 || end > len(ctx.Source) || start >= end {
		return nil, false
	}
	original := string(ctx.Source[start:end])
	if !isSafeStandaloneExprSpan(original) {
		return nil, false
	}

	info, ok := exprDoc(binExpr, ctx)
	if !ok {
		return nil, false
	}

	indent := ctx.IndentAt(binExpr)

	// Account for any non-whitespace prefix before the expression (e.g. "if
	// ").
	startCol := prefixWidthAt(ctx.Source, start, ctx.TabStop)

	doc := info.Doc
	if info.NeedsContinuationIndent {
		doc = layout.N("\t", doc)
	}
	formatted := layout.RenderAt(
		doc, ctx.ColumnLimit, ctx.TabStop, indent, startCol,
	)
	if formatted == "" || formatted == original {
		return nil, false
	}

	out, err := replaceSpan(ctx.Source, start, end, []byte(formatted))
	if err != nil {
		return nil, false
	}

	return out, true
}

// Execute implements Action for BreakArithmeticChainLayoutAction.
func (a *BreakArithmeticChainLayoutAction) Execute(caps Captures,
	ctx *Context) ([]byte, bool) {

	node := resolveTarget(caps, a.Target)
	binExpr, ok := node.(*ast.BinaryExpr)
	if !ok || binExpr == nil {
		return nil, false
	}

	if ctx.LineWidth(binExpr) <= ctx.ColumnLimit {
		return nil, false
	}

	switch binExpr.Op {
	case token.ADD, token.SUB, token.MUL, token.QUO, token.REM:
	default:
		return nil, false
	}

	start := ctx.Fset.Position(binExpr.Pos()).Offset
	end := ctx.Fset.Position(binExpr.End()).Offset
	if start < 0 || end > len(ctx.Source) || start >= end {
		return nil, false
	}
	original := string(ctx.Source[start:end])
	if !isSafeStandaloneExprSpan(original) {
		return nil, false
	}

	info, ok := exprDoc(binExpr, ctx)
	if !ok {
		return nil, false
	}

	indent := ctx.IndentAt(binExpr)

	startCol := prefixWidthAt(ctx.Source, start, ctx.TabStop)

	doc := info.Doc
	if info.NeedsContinuationIndent {
		doc = layout.N("\t", doc)
	}
	formatted := layout.RenderAt(
		doc, ctx.ColumnLimit, ctx.TabStop, indent, startCol,
	)
	if formatted == "" || formatted == original {
		return nil, false
	}

	out, err := replaceSpan(ctx.Source, start, end, []byte(formatted))
	if err != nil {
		return nil, false
	}

	return out, true
}

// Execute implements Action for BreakAssignRHSAction.
func (a *BreakAssignRHSAction) Execute(caps Captures, ctx *Context) ([]byte,
	bool) {

	node := resolveTarget(caps, a.Target)
	assign, ok := node.(*ast.AssignStmt)
	if !ok || assign == nil || ctx == nil || len(assign.Rhs) != 1 {
		return nil, false
	}
	if assign.Tok != token.ASSIGN && assign.Tok != token.DEFINE {
		return nil, false
	}
	if stmtIsControlHeaderInit(ctx, assign) {
		return nil, false
	}
	if ctx.LineWidth(assign) <= ctx.ColumnLimit {
		return nil, false
	}

	start := ctx.Fset.Position(assign.Pos()).Offset
	end := ctx.Fset.Position(assign.End()).Offset
	if start < 0 || end > len(ctx.Source) || start >= end {
		return nil, false
	}
	original := string(ctx.Source[start:end])
	if strings.Contains(original, "\n") || hasAnyComment(original) {
		return nil, false
	}

	rhs := assign.Rhs[0]
	rhsStart := ctx.Fset.Position(rhs.Pos()).Offset
	rhsEnd := ctx.Fset.Position(rhs.End()).Offset
	opStart := ctx.Fset.Position(assign.TokPos).Offset
	opEnd := opStart + len(assign.Tok.String())
	if opStart < start || opEnd > rhsStart || rhsEnd > len(ctx.Source) {
		return nil, false
	}
	if strings.TrimSpace(string(ctx.Source[opEnd:rhsStart])) != "" {
		return nil, false
	}

	info, ok := exprDocWithKind(rhs, ctx, exprDocKindCallArg)
	if !ok {
		return nil, false
	}
	doc := info.Doc
	if info.NeedsContinuationIndent {
		doc = layout.N("\t", doc)
	}

	contIndent := ctx.IndentAt(assign) + "\t"
	formatted := layout.RenderAt(
		doc, ctx.ColumnLimit, ctx.TabStop, contIndent,
		visualLen(contIndent, ctx.TabStop),
	)
	if formatted == "" {
		return nil, false
	}

	prefixStart := lineStart(ctx.Source, opStart)
	if visualLen(string(ctx.Source[prefixStart:opEnd]), ctx.TabStop) >
		ctx.ColumnLimit {
		return nil, false
	}

	replacement := []byte("\n" + contIndent + formatted)
	if !replacementLinesFit(replacement[1:], ctx.ColumnLimit, ctx.TabStop) {
		return nil, false
	}

	out, err := ApplySingleEdit(ctx.Source, opEnd, rhsEnd, replacement)
	if err != nil {
		return nil, false
	}
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(
		fset, "out.go", out, parser.AllErrors,
	); err != nil {
		return nil, false
	}

	return out, true
}

func replacementLinesFit(src []byte, colLimit, tabStop int) bool {
	for start := 0; start <= len(src); {
		end := start
		for end < len(src) && src[end] != '\n' {
			end++
		}
		if visualLen(string(src[start:end]), tabStop) > colLimit {
			return false
		}
		if end == len(src) {
			return true
		}
		start = end + 1
	}

	return true
}

// Execute implements Action for BreakValueSpecGenericConversionAction.
func (a *BreakValueSpecGenericConversionAction) Execute(caps Captures,
	ctx *Context) ([]byte, bool) {

	node := resolveTarget(caps, a.Target)
	spec, ok := node.(*ast.ValueSpec)
	if !ok || spec == nil || ctx == nil || len(spec.Values) != 1 {
		return nil, false
	}
	if ctx.LineWidth(spec) <= ctx.ColumnLimit {
		return nil, false
	}

	specStart := ctx.Fset.Position(spec.Pos()).Offset
	specEnd := ctx.Fset.Position(spec.End()).Offset
	if specStart < 0 || specEnd > len(ctx.Source) || specStart >= specEnd {
		return nil, false
	}
	original := string(ctx.Source[specStart:specEnd])
	if strings.Contains(original, "\n") || hasAnyComment(original) {
		return nil, false
	}

	idx, ok := genericConversionIndexList(spec.Values[0])
	if !ok {
		return nil, false
	}
	start := ctx.Fset.Position(idx.Pos()).Offset
	end := ctx.Fset.Position(idx.End()).Offset
	if start < specStart || end > specEnd || start >= end {
		return nil, false
	}

	formatted, ok := formatGenericIndexListForDecl(
		idx, ctx.IndentAt(spec), ctx,
	)
	if !ok {
		return nil, false
	}
	if formatted == string(ctx.Source[start:end]) {
		return nil, false
	}

	affectedStart := lineStart(ctx.Source, start)
	affectedEnd := lineEnd(ctx.Source, end)
	if affectedEnd < affectedStart || affectedEnd > len(ctx.Source) {
		return nil, false
	}
	var affected bytes.Buffer
	affected.Write(ctx.Source[affectedStart:start])
	affected.WriteString(formatted)
	affected.Write(ctx.Source[end:affectedEnd])
	if !replacementLinesFit(
		affected.Bytes(), ctx.ColumnLimit, ctx.TabStop,
	) {
		return nil, false
	}

	out, err := ApplySingleEdit(ctx.Source, start, end, []byte(formatted))
	if err != nil {
		return nil, false
	}
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(
		fset, "out.go", out, parser.AllErrors,
	); err != nil {
		return nil, false
	}

	return out, true
}

func genericConversionIndexList(expr ast.Expr) (*ast.IndexListExpr, bool) {
	call, ok := expr.(*ast.CallExpr)
	if !ok || call == nil || call.Fun == nil {
		return nil, false
	}

	fun := call.Fun
	if paren, ok := fun.(*ast.ParenExpr); ok {
		fun = paren.X
	}
	if star, ok := fun.(*ast.StarExpr); ok {
		fun = star.X
	}

	idx, ok := fun.(*ast.IndexListExpr)
	if !ok || idx == nil || len(idx.Indices) < 2 {
		return nil, false
	}

	return idx, true
}

func formatGenericIndexListForDecl(idx *ast.IndexListExpr, indent string,
	ctx *Context) (string, bool) {

	if idx == nil || idx.X == nil || len(idx.Indices) < 2 {
		return "", false
	}

	base := renderNode(idx.X, ctx.Fset)
	if base == "" || strings.Contains(base, "\n") || hasAnyComment(base) {
		return "", false
	}

	typeIndent := indent + "\t"
	var out strings.Builder
	out.WriteString(base)
	out.WriteString("[\n")
	for i, index := range idx.Indices {
		indexText := renderNode(index, ctx.Fset)
		if indexText == "" || strings.Contains(indexText, "\n") ||
			hasAnyComment(indexText) {
			return "", false
		}
		out.WriteString(typeIndent)
		out.WriteString(indexText)
		if i < len(idx.Indices)-1 {
			out.WriteByte(',')
			out.WriteByte('\n')
		}
	}
	out.WriteByte(']')

	return out.String(), true
}

// Execute implements Action for BreakCaseClauseLayoutAction.
func (a *BreakCaseClauseLayoutAction) Execute(caps Captures, ctx *Context) (
	[]byte, bool) {

	node := resolveTarget(caps, a.Target)
	caseClause, ok := node.(*ast.CaseClause)
	if !ok || caseClause == nil || len(caseClause.List) == 0 {
		return nil, false
	}

	if ctx.LineWidth(caseClause) <= ctx.ColumnLimit {
		return nil, false
	}

	start := ctx.Fset.Position(caseClause.List[0].Pos()).Offset
	end := ctx.Fset.Position(caseClause.List[len(caseClause.List)-1].End()).Offset
	if start < 0 || end > len(ctx.Source) || start >= end {
		return nil, false
	}

	original := string(ctx.Source[start:end])
	if hasAnyComment(original) {
		return nil, false
	}

	indent := ctx.IndentAt(caseClause)
	contIndent := indent + "\t"

	startCol := prefixWidthAt(ctx.Source, start, ctx.TabStop)

	var docs []layout.Doc
	for i, expr := range caseClause.List {
		if i > 0 {
			docs = append(docs, layout.T(","), layout.L())
		}
		docs = append(docs, layout.T(renderNode(expr, ctx.Fset)))
	}

	formatted := layout.RenderAt(
		layout.G(
			layout.C(docs...),
		),
		ctx.ColumnLimit,
		ctx.TabStop,
		contIndent,
		startCol,
	)
	if formatted == "" || formatted == original {
		return nil, false
	}

	out, err := replaceSpan(ctx.Source, start, end, []byte(formatted))
	if err != nil {
		return nil, false
	}

	return out, true
}

// Execute implements Action for BreakSelectorChainLayoutAction.
func (a *BreakSelectorChainLayoutAction) Execute(caps Captures, ctx *Context) (
	[]byte, bool) {

	node := resolveTarget(caps, a.Target)
	sel, ok := node.(*ast.SelectorExpr)
	if !ok || sel == nil {
		return nil, false
	}

	if ctx.LineWidth(sel) <= ctx.ColumnLimit {
		return nil, false
	}

	start := ctx.Fset.Position(sel.Pos()).Offset
	end := ctx.Fset.Position(sel.End()).Offset
	if start < 0 || end > len(ctx.Source) || start >= end {
		return nil, false
	}
	original := string(ctx.Source[start:end])
	if !isSafeStandaloneExprSpan(original) {
		return nil, false
	}

	indent := ctx.IndentAt(sel)

	startCol := prefixWidthAt(ctx.Source, start, ctx.TabStop)

	info, ok := exprDoc(sel, ctx)
	if !ok {
		return nil, false
	}
	doc := info.Doc
	if info.NeedsContinuationIndent {
		doc = layout.N("\t", doc)
	}
	formatted := layout.RenderAt(
		doc, ctx.ColumnLimit, ctx.TabStop, indent, startCol,
	)
	if formatted == "" || formatted == original {
		return nil, false
	}

	out, err := replaceSpan(ctx.Source, start, end, []byte(formatted))
	if err != nil {
		return nil, false
	}

	return out, true
}

// Execute implements Action for BreakMethodChainLayoutAction.
func (a *BreakMethodChainLayoutAction) Execute(caps Captures, ctx *Context) (
	[]byte, bool) {

	node := resolveTarget(caps, a.Target)
	call, ok := node.(*ast.CallExpr)
	if !ok || call == nil {
		return nil, false
	}

	if ctx.LineWidth(call) <= ctx.ColumnLimit {
		return nil, false
	}

	start := ctx.Fset.Position(call.Pos()).Offset
	end := ctx.Fset.Position(call.End()).Offset
	if start < 0 || end > len(ctx.Source) || start >= end {
		return nil, false
	}
	original := string(ctx.Source[start:end])
	if !isSafeStandaloneExprSpan(original) {
		return nil, false
	}

	indent := ctx.IndentAt(call)
	startCol := prefixWidthAt(ctx.Source, start, ctx.TabStop)

	info, ok := exprDoc(call, ctx)
	if !ok {
		return nil, false
	}

	doc := info.Doc
	if info.NeedsContinuationIndent {
		doc = layout.N("\t", doc)
	}
	formatted := layout.RenderAt(
		doc, ctx.ColumnLimit, ctx.TabStop, indent, startCol,
	)
	if formatted == "" || formatted == original {
		return nil, false
	}

	out, err := replaceSpan(ctx.Source, start, end, []byte(formatted))
	if err != nil {
		return nil, false
	}

	return out, true
}

// getNodeSpan extracts start, end offsets and original text for a node. Returns
// false if the span is invalid.
func getNodeSpan(node ast.Node, ctx *Context) (start, end int, original string,
	ok bool) {

	start = ctx.Fset.Position(node.Pos()).Offset
	end = ctx.Fset.Position(node.End()).Offset
	if start < 0 || end > len(ctx.Source) || start >= end {
		return 0, 0, "", false
	}

	return start, end, string(ctx.Source[start:end]), true
}

// isWideCallExpr returns true if the call expression exceeds width limits.
func isWideCallExpr(caps Captures, ctx *Context, target string) bool {
	return (&OrCond{Conds: []Condition{
		&LineWidthCond{
			Target: target,
			Op:     ">",
			Value:  0,
		},
		&CollapsedWidthCond{
			Target: target,
			Op:     ">",
			Value:  0,
		},
	}}).Eval(caps,
		ctx,
	)
}

// callFunDoc builds a layout.Doc for the function part of a call expression.
func callFunDoc(fun ast.Expr, ctx *Context) layout.Doc {
	doc := layout.T(renderNode(fun, ctx.Fset))
	if fun != nil {
		if info, ok := exprDocWithKind(
			fun, ctx, exprDocKindCallArg,
		); ok {

			doc = info.Doc
		}
	}

	return doc
}

// isCallToMake returns true if the call is to the builtin make function.
func isCallToMake(call *ast.CallExpr) bool {
	ident, ok := call.Fun.(*ast.Ident)

	return ok && ident.Name == "make"
}

// Execute implements Action for BreakCallArgsLayoutAction.
func (a *BreakCallArgsLayoutAction) Execute(caps Captures, ctx *Context) (
	[]byte, bool) {

	node := resolveTarget(caps, a.Target)
	call, ok := node.(*ast.CallExpr)
	if !ok || call == nil || len(call.Args) == 0 {
		return nil, false
	}

	start, end, original, ok := getNodeSpan(call, ctx)
	if !ok || !isSafeStandaloneExprSpan(original) {
		return nil, false
	}

	// Only attempt on "long" calls (line or collapsed width exceeds limit).
	if !isWideCallExpr(caps, ctx, a.Target) {
		return nil, false
	}

	indent := ctx.IndentAt(call)
	startCol := prefixWidthAt(ctx.Source, start, ctx.TabStop)

	funDoc := callFunDoc(call.Fun, ctx)
	argDocs, ok := buildCallArgsDocs(call.Args, a.Grouping, ctx)
	if !ok {
		return nil, false
	}
	isMake := isCallToMake(call)

	// Note: this action is only selected for "long" calls, so we always
	// include a ForceBreak barrier to ensure the output becomes multiline
	// when a rule chooses this action.
	//
	// For most calls we follow the existing "newline after (" style to
	// match next-mode expectations and existing tests. For `make(...)`
	// calls we keep the first argument inline (so `make([]T, ...)` remains
	// readable), but still break subsequent arguments onto new lines.
	argsGroupDocs := buildCallArgsGroupDocs(argDocs, isMake)

	// Trailing comma in broken form. This is key to making gofmt preserve
	// the multi-line call layout.
	argsGroupDocs = append(
		argsGroupDocs,
		layout.IB(
			layout.T(","), layout.T(""),
		),
	)

	argsGroup := layout.G(layout.C(argsGroupDocs...))

	doc := layout.G(
		layout.C(
			funDoc, layout.T("("), layout.N("\t", argsGroup),
			layout.SL(), layout.T(")"),
		),
	)

	formatted := layout.RenderAt(
		doc, ctx.ColumnLimit, ctx.TabStop, indent, startCol,
	)
	if formatted == "" || formatted == original {
		return nil, false
	}

	out, err := replaceSpan(ctx.Source, start, end, []byte(formatted))
	if err != nil {
		return nil, false
	}

	return out, true
}

// Execute implements Action for StructuredLogCallAction.
func (a *StructuredLogCallAction) Execute(caps Captures, ctx *Context) ([]byte,
	bool) {

	node := resolveTarget(caps, a.Target)
	call, ok := node.(*ast.CallExpr)
	if !ok || call == nil {
		return nil, false
	}

	pairStart, ok := structuredLogPairStart(call)
	if !ok || len(call.Args) <= pairStart {
		return nil, false
	}
	if !structuredLogArgsAreSafe(call.Args[pairStart:]) {
		return nil, false
	}

	start, end, original, ok := getNodeSpan(call, ctx)
	if !ok || !isSafeStandaloneExprSpan(original) {
		return nil, false
	}
	if !isWideCallExpr(caps, ctx, a.Target) {
		return nil, false
	}

	formatted, ok := formatStructuredLogCallPacked(
		call, pairStart, ctx, start,
	)
	if !ok || formatted == "" || formatted == original {
		return nil, false
	}

	out, err := ApplySingleEdit(ctx.Source, start, end, []byte(formatted))
	if err != nil {
		return nil, false
	}

	return out, true
}

func structuredLogArgsAreSafe(args []ast.Expr) bool {
	if len(args) == 0 {
		return false
	}

	if structuredLogArgsAreStringPairs(args) {
		return true
	}

	for _, arg := range args {
		if isStringLit(arg) {
			return false
		}
	}

	return true
}

func structuredLogArgsAreStringPairs(args []ast.Expr) bool {
	if len(args) < 2 || len(args)%2 != 0 {
		return false
	}

	for i := 0; i < len(args); i += 2 {
		if !isStringLit(args[i]) {
			return false
		}
	}

	return true
}

func formatStructuredLogCallPacked(call *ast.CallExpr, pairStart int,
	ctx *Context, start int) (string, bool) {

	funText := renderNode(call.Fun, ctx.Fset)
	if funText == "" {
		return "", false
	}

	argTexts := make([]string, 0, len(call.Args))
	for _, arg := range call.Args {
		argText := renderNode(arg, ctx.Fset)
		if argText == "" || strings.Contains(argText, "\n") {
			return "", false
		}
		argTexts = append(argTexts, argText)
	}

	prelude := argTexts[:pairStart]
	units := structuredLogArgUnits(
		call.Args[pairStart:], argTexts[pairStart:],
	)
	if len(prelude) == 0 || len(units) == 0 {
		return "", false
	}

	wsIndent := ctx.IndentAt(call)
	contIndent := wsIndent + "\t"
	startCol := prefixWidthAt(ctx.Source, start, ctx.TabStop)

	var b strings.Builder
	head := funText + "(" + strings.Join(prelude, ", ")
	if startCol+visualLen(head+",", ctx.TabStop) <= ctx.ColumnLimit {
		b.WriteString(head)
		b.WriteByte(',')
	} else {
		b.WriteString(funText)
		b.WriteByte('(')
		for _, arg := range prelude {
			b.WriteByte('\n')
			b.WriteString(contIndent)
			b.WriteString(arg)
			b.WriteByte(',')
		}
	}

	for _, line := range structuredLogArgUnitLines(
		units, contIndent, ctx.ColumnLimit, ctx.TabStop,
	) {
		b.WriteByte('\n')
		b.WriteString(contIndent)
		b.WriteString(line)
	}

	b.WriteByte('\n')
	b.WriteString(wsIndent)
	b.WriteByte(')')

	return b.String(), true
}

func structuredLogArgUnits(args []ast.Expr, texts []string) []string {
	if structuredLogArgsAreStringPairs(args) {
		units := make([]string, 0, len(texts)/2)
		for i := 0; i+1 < len(texts); i += 2 {
			units = append(units, texts[i]+", "+texts[i+1])
		}

		return units
	}

	return texts
}

func structuredLogArgUnitLines(units []string, indent string, colLimit int,
	tabStop int) []string {

	if len(units) == 0 {
		return nil
	}

	indentWidth := visualLen(indent, tabStop)
	var lines []string
	for _, unit := range units {
		unitWithComma := unit + ","
		if indentWidth+visualLen(unitWithComma, tabStop) > colLimit {
			parts := strings.SplitN(unit, ", ", 2)
			if len(parts) == 2 {
				lines = append(
					lines, parts[0]+",", parts[1]+",",
				)
				continue
			}
		}

		lines = append(lines, unitWithComma)
	}

	return lines
}

func buildCallArgsGroupDocs(argDocs []layout.Doc, isMake bool) []layout.Doc {
	var argsGroupDocs []layout.Doc
	if isMake && len(argDocs) > 1 {
		// Keep the first argument inline, then force a break so the
		// remaining arguments are laid out one-per-line. This yields:
		// make([]T, 0, n, )
		argsGroupDocs = append(argsGroupDocs, argDocs[0], layout.FB())
		for i := 1; i < len(argDocs); i++ {
			argsGroupDocs = append(
				argsGroupDocs, layout.T(","), layout.L(),
				argDocs[i],
			)
		}

		return argsGroupDocs
	}

	// Standard style: newline right after "(".
	argsGroupDocs = append(argsGroupDocs, layout.FB(), layout.SL())
	for i, d := range argDocs {
		if i > 0 {
			argsGroupDocs = append(
				argsGroupDocs, layout.T(","), layout.L(),
			)
		}
		argsGroupDocs = append(argsGroupDocs, d)
	}

	return argsGroupDocs
}

func buildCallArgsDocs(args []ast.Expr, grouping string,
	ctx *Context) ([]layout.Doc, bool) {

	if len(args) == 0 {
		return nil, false
	}

	switch grouping {
	case "pairs":
		return buildCallArgPairs(args, ctx)

	default:
		// Default: one argument per line (forced break).
		docs := make([]layout.Doc, 0, len(args))
		for _, arg := range args {
			argDoc, ok := callArgDoc(arg, ctx)
			if !ok {
				return nil, false
			}
			docs = append(docs, argDoc)
		}

		return docs, true
	}
}

func buildCallArgPairs(args []ast.Expr, ctx *Context) ([]layout.Doc, bool) {
	// Group args as (a, b) pairs when possible. This is useful for call
	// sites that conceptually operate on tuples of arguments.
	var docs []layout.Doc
	for i := 0; i < len(args); {
		left, ok := callArgDoc(args[i], ctx)
		if !ok {
			return nil, false
		}

		if i+1 >= len(args) {
			docs = append(docs, left)
			i++
			continue
		}

		right, ok := callArgDoc(args[i+1], ctx)
		if !ok {
			return nil, false
		}

		// Within a group, keep the pair flat when possible but allow
		// the second element to wrap if it doesn't fit.
		group := layout.G(
			layout.C(
				left, layout.T(","), layout.L(), right,
			),
		)
		docs = append(docs, group)
		i += 2
	}

	return docs, true
}

func callArgDoc(arg ast.Expr, ctx *Context) (layout.Doc, bool) {
	if arg == nil || ctx == nil {
		return nil, false
	}

	argText := renderNode(arg, ctx.Fset)
	// Be conservative for now: if AST printing produced multiline text, we
	// intentionally do not try to re-indent it inside an argument list.
	if strings.Contains(argText, "\n") {
		return nil, false
	}

	// Use a structured doc for known expression forms so nested long
	// expressions can be laid out consistently within argument lists.
	if info, ok := exprDocWithKind(arg, ctx, exprDocKindCallArg); ok {
		argDoc := info.Doc
		if info.NeedsContinuationIndent {
			argDoc = layout.N("\t", argDoc)
		}

		return argDoc, true
	}

	return layout.T(argText), true
}

// Execute implements Action for BreakBinaryExprLayoutAction.
func (a *BreakBinaryExprLayoutAction) Execute(caps Captures, ctx *Context) (
	[]byte, bool) {

	node := resolveTarget(caps, a.Target)
	binExpr, ok := node.(*ast.BinaryExpr)
	if !ok || binExpr == nil {
		return nil, false
	}

	// Try the operator-specific layout action if the style enables it.
	switch binExpr.Op {
	case token.LAND, token.LOR:
		if a.LogicalStyle == "layout" {
			return (&BreakLogicalChainLayoutAction{
				Target: a.Target,
			}).Execute(caps,
				ctx,
			)
		}

		return (&BreakLogicalChainPackedAction{
			Target: a.Target,
		}).Execute(caps,
			ctx,
		)

	case token.ADD, token.SUB, token.MUL, token.QUO, token.REM:
		if a.ArithmeticStyle == "layout" {
			return (&BreakArithmeticChainLayoutAction{
				Target: a.Target,
			}).Execute(caps,
				ctx,
			)
		}
	}

	return nil, false
}

// ReflowStringConcatAction rewrites a long string concatenation expression into
// a multi-line concatenation with stable indentation.
type ReflowStringConcatAction struct {
	Target string
}

// Execute implements Action for ReflowStringConcatAction.
func (a *ReflowStringConcatAction) Execute(caps Captures, ctx *Context) ([]byte,
	bool) {

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
	ast.Inspect(
		n,
		func(node ast.Node) bool {
			lit, ok := node.(*ast.BasicLit)
			if ok && isRawStringLiteral(lit) {
				found = true

				return false
			}

			return !found
		},
	)

	return found
}

func isRawStringLiteral(lit *ast.BasicLit) bool {
	return lit.Kind.String() == "STRING" &&
		strings.HasPrefix(lit.Value, "`")
}

// ExecuteEdits implements EditAction for ReflowStringConcatAction.
func (a *ReflowStringConcatAction) ExecuteEdits(caps Captures, ctx *Context) (
	[]Edit, bool, error) {

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

	// Be conservative: do not rewrite raw string literals (backticks).
	// While the value is constant, changing literal style can be
	// surprising.
	if hasRawStringLit(expr) {
		return nil, false, nil
	}

	strText, ok := llast.FlattenStringExprAST(expr)
	if !ok {
		return nil, false, nil
	}

	indent := ctx.IndentAt(node)
	contIndent := indent + "\t"

	// Account for non-whitespace prefix before the expression (e.g. "return
	// ").
	prefixWidth := prefixWidthAt(ctx.Source, start, ctx.TabStop)

	formatted := text.SplitQuotedString(
		strText, prefixWidth, contIndent, ctx.ColumnLimit, ctx.TabStop,
	)
	if formatted == original {
		return nil, false, nil
	}

	return []Edit{
		{
			Start:   start,
			End:     end,
			Replace: []byte(formatted),
		},
	}, true, nil
}

// BreakCaseClauseAction breaks a long case clause at comma boundaries.
type BreakCaseClauseAction struct {
	Target string
}

// Execute implements Action for BreakCaseClauseAction.
func (a *BreakCaseClauseAction) Execute(caps Captures, ctx *Context) ([]byte,
	bool) {

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
func (a *BreakCaseClauseAction) ExecuteEdits(caps Captures, ctx *Context) (
	[]Edit, bool, error) {

	node := resolveTarget(caps, a.Target)
	caseClause, ok := node.(*ast.CaseClause)
	if !ok || len(caseClause.List) == 0 {
		return nil, false, nil
	}

	// Check if line already fits
	if ctx.LineWidth(caseClause) <= ctx.ColumnLimit {
		return nil, false, nil
	}

	indent := ctx.IndentAt(caseClause)
	pos, ok := findCaseClauseBreakPos(ctx, caseClause, indent)
	if !ok {
		return nil, false, nil
	}

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
		{
			Start:   pos,
			End:     end,
			Replace: replacement,
		},
	}, true, nil
}

func findCaseClauseBreakPos(ctx *Context, clause *ast.CaseClause,
	indent string) (int, bool) {

	// Find the rightmost comma that keeps prefix under column limit.
	indentWidth := visualLen(indent, ctx.TabStop)
	caseStart := ctx.Fset.Position(clause.Pos()).Offset

	type commaInfo struct {
		afterExpr int // position right after the expression (where comma is)
		prefix    int // visual width up to and including this comma
	}
	var commas []commaInfo

	for i := 0; i < len(clause.List)-1; i++ {
		expr := clause.List[i]
		exprEnd := ctx.Fset.Position(expr.End()).Offset

		// Find comma after this expression.
		commaPos := exprEnd
		for commaPos < len(ctx.Source) && ctx.Source[commaPos] != ',' {
			commaPos++
		}
		if commaPos >= len(ctx.Source) {
			continue
		}

		// Calculate prefix width (from line start to comma inclusive).
		prefix := string(ctx.Source[caseStart : commaPos+1])
		prefixWidth := indentWidth + visualLen(prefix, ctx.TabStop)

		commas = append(
			commas, commaInfo{
				afterExpr: commaPos + 1,
				prefix:    prefixWidth,
			},
		)
	}

	if len(commas) == 0 {
		return 0, false
	}

	var bestComma *commaInfo
	for i := len(commas) - 1; i >= 0; i-- {
		c := &commas[i]
		if c.prefix <= ctx.ColumnLimit {
			bestComma = c
			break
		}
	}
	if bestComma == nil {
		bestComma = &commas[0]
	}

	return bestComma.afterExpr, true
}

// ReflowNestedCallsAction finds and reflows function calls within an
// expression.
type ReflowNestedCallsAction struct {
	Target   string
	Strategy ReflowStrategy
}

// Execute implements Action for ReflowNestedCallsAction.
func (a *ReflowNestedCallsAction) Execute(caps Captures, ctx *Context) ([]byte,
	bool) {

	node := resolveTarget(caps, a.Target)
	if node == nil {
		return nil, false
	}

	// Find the first call expression that would benefit from reflow
	targetCall := findReflowTargetCall(node, ctx)

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
	}).Execute(tempCaps,
		ctx,
	)
}

func findReflowTargetCall(node ast.Node, ctx *Context) *ast.CallExpr {
	var targetCall *ast.CallExpr
	ast.Inspect(node, func(n ast.Node) bool {
		if targetCall != nil {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		// Check if this call is worth reflowing.
		if len(call.Args) > 1 && ctx.NodeWidth(call) > ctx.ColumnLimit/2 {
			targetCall = call

			return false
		}

		return true
	})

	return targetCall
}

// Helper to render an AST node back to source.
func renderNode(n ast.Node, fset *token.FileSet) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, n); err != nil {
		return ""
	}

	return buf.String()
}

func trailingCommaSuffixWidth(src []byte, start, tabStop int) int {
	lineEnd := start
	for lineEnd < len(src) && src[lineEnd] != '\n' {
		lineEnd++
	}
	if lineEnd <= start {
		return 0
	}
	suffix := string(src[start:lineEnd])
	if !strings.HasPrefix(strings.TrimSpace(suffix), ",") {
		return 0
	}

	return visualLen(suffix, tabStop)
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
func (a *LeftFlowCallAction) Execute(caps Captures, ctx *Context) ([]byte,
	bool) {

	node := resolveTarget(caps, a.Target)
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return nil, false
	}

	if len(call.Args) == 0 {
		return nil, false
	}
	if callIsCompositeKeyValue(call, ctx) {
		return nil, false
	}

	// Note: We don't check LineWidth here because the legacy formatter
	// always reformats targeted calls to normalize them. The comparison
	// with original output at the end will skip changes if the format is
	// already correct.

	// Get source positions
	start := ctx.Fset.Position(call.Pos()).Offset
	end := ctx.Fset.Position(call.End()).Offset
	if start < 0 || end > len(ctx.Source) || start >= end {
		return nil, false
	}

	original := ctx.Source[start:end]
	wsIndent := ctx.IndentAt(call)

	// Find the base length (visual width from line start to call start).
	baseLen := prefixWidthAt(ctx.Source, start, ctx.TabStop)
	effectiveLimit := ctx.ColumnLimit
	isPrintfStyle := isLogPrintfFuncName(getFuncName(call))
	if isPrintfStyle {
		suffixWidth := trailingCallCommaSuffixWidth(
			ctx.Source, end, ctx.TabStop,
		)
		if suffixWidth > 0 && effectiveLimit > suffixWidth {
			effectiveLimit -= suffixWidth
		}
	}

	var formatted string
	if a.FormatFunc != nil {
		// Use the provided formatter (legacy formatter)
		formatted = a.FormatFunc(
			original, wsIndent, baseLen, effectiveLimit,
			ctx.TabStop,
		)
	} else {
		// Fallback to simplified formatting
		formatted = formatCallLeftFlowSimple(call, wsIndent, ctx)
	}

	if formatted == string(original) && isPrintfStyle {
		if fixed, ok := breakCallClosingParenBeforeTrailingSuffix(
			ctx, start, end, string(original), wsIndent,
		); ok {

			formatted = fixed
		} else {
			return nil, false
		}
	} else if formatted == string(original) {
		return nil, false
	}

	// Normalize both original and formatted calls with gofmt to check if
	// they're actually different. gofmt may change indentation of string
	// continuations in nested calls, so we need to compare post-gofmt
	// output.
	origNorm := normalizeCallWithGofmt(string(original), wsIndent)
	fmtNorm := normalizeCallWithGofmt(formatted, wsIndent)

	if origNorm == fmtNorm && isPrintfStyle {
		if fixed, ok := breakCallClosingParenBeforeTrailingSuffix(
			ctx, start, end, string(original), wsIndent,
		); ok {

			formatted = fixed
		} else {

			// After gofmt normalization, both produce the same
			// output. This means our change would be undone by
			// gofmt - skip it.
			return nil, false
		}
	} else if origNorm == fmtNorm {
		return nil, false
	}

	// Build result with formatted call
	out, err := ApplySingleEdit(ctx.Source, start, end, []byte(formatted))
	if err != nil {
		return nil, false
	}

	return out, true
}

func breakCallClosingParenBeforeTrailingSuffix(ctx *Context, start, end int,
	original string, wsIndent string) (string, bool) {

	if !strings.Contains(original, "\n") {
		return "", false
	}
	suffixWidth := trailingCallCommaSuffixWidth(
		ctx.Source, end, ctx.TabStop,
	)
	if suffixWidth == 0 {
		return "", false
	}

	lastLen := visualLen(
		string(ctx.Source[lineStart(ctx.Source, end):end]), ctx.TabStop,
	)
	if lastLen+suffixWidth <= ctx.ColumnLimit {
		return "", false
	}

	closeIdx := strings.LastIndex(original, ")")
	if closeIdx < 0 && end < len(ctx.Source) && ctx.Source[end] == ')' {
		before := strings.TrimRight(original, " \t")
		if before == "" || strings.HasSuffix(before, ",") {
			return "", false
		}

		return before + ",\n" + wsIndent, true
	}
	if closeIdx < 0 || strings.TrimSpace(original[closeIdx+1:]) != "" {
		return "", false
	}
	lineStartIdx := strings.LastIndex(original[:closeIdx], "\n") + 1
	if strings.TrimSpace(original[lineStartIdx:closeIdx]) == "" {
		return "", false
	}
	before := strings.TrimRight(original[:closeIdx], " \t")
	if strings.HasSuffix(before, ",") {
		return "", false
	}

	return before + ",\n" + wsIndent + ")", true
}

func trailingCallCommaSuffixWidth(src []byte, start, tabStop int) int {
	width := trailingCommaSuffixWidth(src, start, tabStop)
	if width > 0 {
		return width
	}
	if start >= len(src) || src[start] != ')' {
		return 0
	}

	lineEnd := start
	for lineEnd < len(src) && src[lineEnd] != '\n' {
		lineEnd++
	}
	suffix := string(src[start:lineEnd])
	if !strings.HasPrefix(strings.TrimSpace(suffix), "),") {
		return 0
	}

	return visualLen(suffix, tabStop)
}

func isLogPrintfFuncName(name string) bool {
	if isLogPrintfName(name, nil) {
		return true
	}
	if dot := strings.LastIndex(name, "."); dot >= 0 {
		return isLogPrintfName(name[dot+1:], nil)
	}

	return false
}

func callIsCompositeKeyValue(call *ast.CallExpr, ctx *Context) bool {
	if call == nil || ctx == nil {
		return false
	}
	kv, ok := ctx.Parent(call).(*ast.KeyValueExpr)

	return ok && kv.Value == call
}

// normalizeCallWithGofmt wraps a call expression in a minimal Go file, runs
// gofmt, and extracts the normalized call. This allows comparing two versions
// of a call that may differ only in gofmt-level formatting.
func normalizeCallWithGofmt(call string, wsIndent string) string {
	// Wrap in minimal Go file at the same indent level
	wrapped := "package p\nfunc f() {\n" + wsIndent + call + "\n}"
	formatted, err := format.Source([]byte(wrapped))
	if err != nil {

		// If gofmt fails, return original
		return call
	}

	// Extract the call from the formatted output. gofmt may change the
	// indent, so we find the actual call start by looking for the opening
	// brace of the function.
	s := string(formatted)

	// Find "func f() {\n" and then the actual call
	marker := "func f() {\n"
	idx := strings.Index(s, marker)
	if idx < 0 {
		return call
	}
	start := idx + len(marker)

	// Find the closing brace. The call ends just before "\n}\n" or "\n}"
	var end int
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
// multi-line style when they exceed the column limit. This action delegates to
// the legacy formatter to ensure identical output behavior.
type PackedMultiLineCallAction struct {
	Target string

	// FormatFunc is an optional function that formats the call using legacy
	// logic. If nil, a simplified fallback is used.
	FormatFunc PackedMultiLineFormatFunc

	// DisableBreakBeforeCallOnLongMultiAssignPrefix disables a readability
	// heuristic that prefers breaking before a call (keeping it
	// single-line) when the only overflow is caused by a long
	// multi-assignment prefix.
	//
	// Some modes (e.g. "next") intentionally prefer preserving the
	// assignment shape by formatting the call itself as multiline rather
	// than detaching the call from the assignment with a newline.
	DisableBreakBeforeCallOnLongMultiAssignPrefix bool

	// OnlyIfSingleLine restricts this action to calls that are currently
	// rendered on a single line in the source span. This is useful as a
	// fallback when a layout-based formatter "owns" the multiline shape and
	// we only want the packed formatter to run when the call is still a
	// long single-line expression.
	OnlyIfSingleLine bool
}

// CollapseSimpleCallArgsAction collapses over-expanded single-argument calls
// when the compact argument list fits at the call's current line position.
type CollapseSimpleCallArgsAction struct {
	Target string
}

// PackReturnStmtAction greedily packs multi-result return statements while
// keeping simple nested calls on one line when they fit.
type PackReturnStmtAction struct {
	Target string
}

// BreakSelectorCallArgsAction breaks only the argument list of a selector call
// whose callee head should remain flat, such as `make(x).Check(...)`.
type BreakSelectorCallArgsAction struct {
	Target string

	// FormatFunc formats a synthetic call whose callee has no parentheses.
	// This lets us reuse packed call-argument formatting without confusing
	// lexical call parsers with receiver-call parentheses in the real head.
	FormatFunc PackedMultiLineFormatFunc
}

// LegacyOnePerLineCallAction formats generic function calls using the legacy
// MultiLineCallFormatter style (one argument per line). Unlike AST-based call
// actions, this action preserves comments inside argument lists because it only
// rearranges the existing source bytes.
type LegacyOnePerLineCallAction struct {
	Target string

	// FormatFunc is an optional function that formats the call using legacy
	// logic. If nil, a simplified fallback is used.
	FormatFunc PackedMultiLineFormatFunc
}

// LegacyMultiLineScanFunc applies a single legacy multiline-call formatting
// pass to src and reports whether it changed anything.
//
// This intentionally mirrors the legacy MultiLineCallFormatter behavior of
// making at most one change per pass and repeating up to a fixed iteration cap.
type LegacyMultiLineScanFunc func(src []byte, colLimit, tabStop int, excludes []string) ([]byte, bool)

// LegacyMultiLineScanAction delegates multiline-call detection + rewriting to a
// scan-based implementation, matching the legacy formatter's behavior
// (including its lexical detection quirks).
type LegacyMultiLineScanAction struct {
	Excludes []string
	ScanFunc LegacyMultiLineScanFunc
}

// LegacyCompactCallFormatFunc formats compact call targets (and optionally
// fallback non-targets) in src and reports whether it changed anything.
type LegacyCompactCallFormatFunc func(src []byte, colLimit, tabStop int) ([]byte, bool)

// LegacyCompactCallFormatAction delegates compact-call formatting to an
// injected legacy formatter implementation.
type LegacyCompactCallFormatAction struct {
	FormatFunc LegacyCompactCallFormatFunc
}

// Execute implements Action for LegacyCompactCallFormatAction.
func (a *LegacyCompactCallFormatAction) Execute(_ Captures, ctx *Context) (
	[]byte, bool) {

	if a.FormatFunc == nil {
		return nil, false
	}
	out, changed := a.FormatFunc(ctx.Source, ctx.ColumnLimit, ctx.TabStop)
	if !changed {
		return nil, false
	}

	return out, true
}

// Execute implements Action for LegacyMultiLineScanAction.
func (a *LegacyMultiLineScanAction) Execute(_ Captures, ctx *Context) ([]byte,
	bool) {

	if a.ScanFunc == nil {
		return nil, false
	}

	out, changed := a.ScanFunc(
		ctx.Source, ctx.ColumnLimit, ctx.TabStop, a.Excludes,
	)
	if !changed {
		return nil, false
	}

	return out, true
}

// LegacyCommentFormatFunc formats comments in src and reports whether it
// changed anything.
type LegacyCommentFormatFunc func(src []byte, colLimit, tabStop int, moveInlineAbove bool) ([]byte, bool)

// LegacyCommentFormatAction delegates comment formatting to an injected legacy
// formatter implementation.
type LegacyCommentFormatAction struct {
	MoveInlineAbove bool
	FormatFunc      LegacyCommentFormatFunc
}

// Execute implements Action for LegacyCommentFormatAction.
func (a *LegacyCommentFormatAction) Execute(_ Captures, ctx *Context) ([]byte,
	bool) {

	if a.FormatFunc == nil {
		return nil, false
	}
	out, changed := a.FormatFunc(
		ctx.Source, ctx.ColumnLimit, ctx.TabStop, a.MoveInlineAbove,
	)
	if !changed {
		return nil, false
	}

	return out, true
}

// LegacyFuncSigFormatFunc formats function signatures in src and reports
// whether it changed anything.
type LegacyFuncSigFormatFunc func(src []byte, colLimit, tabStop int) ([]byte, bool)

// LegacyFuncSigFormatAction delegates function signature formatting to an
// injected legacy formatter implementation.
type LegacyFuncSigFormatAction struct {
	FormatFunc LegacyFuncSigFormatFunc
}

// Execute implements Action for LegacyFuncSigFormatAction.
func (a *LegacyFuncSigFormatAction) Execute(_ Captures, ctx *Context) ([]byte,
	bool) {

	if a.FormatFunc == nil {
		return nil, false
	}
	out, changed := a.FormatFunc(ctx.Source, ctx.ColumnLimit, ctx.TabStop)
	if !changed {
		return nil, false
	}

	return out, true
}

// LegacyBlankLinesFormatFunc formats blank lines in src and reports whether it
// changed anything.
type LegacyBlankLinesFormatFunc func(src []byte) ([]byte, bool)

// LegacyBlankLinesFormatAction delegates blank line formatting to an injected
// legacy formatter implementation.
type LegacyBlankLinesFormatAction struct {
	FormatFunc LegacyBlankLinesFormatFunc
}

// Execute implements Action for LegacyBlankLinesFormatAction.
func (a *LegacyBlankLinesFormatAction) Execute(_ Captures, ctx *Context) (
	[]byte, bool) {

	if a.FormatFunc == nil {
		return nil, false
	}
	out, changed := a.FormatFunc(ctx.Source)
	if !changed {
		return nil, false
	}

	return out, true
}

// Execute implements Action for LegacyOnePerLineCallAction.
func (a *LegacyOnePerLineCallAction) Execute(caps Captures, ctx *Context) (
	[]byte, bool) {

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

	// Mirror legacy decision: compute the visual width of the prefix before
	// the call on the current line plus the call itself. This intentionally
	// does not collapse whitespace.
	ls := lineStart(ctx.Source, start)
	prefixLen := visualLen(string(ctx.Source[ls:start]), ctx.TabStop)
	callLen := visualLen(string(original), ctx.TabStop)
	if prefixLen+callLen <= ctx.ColumnLimit {
		return nil, false
	}

	var formatted string
	if a.FormatFunc != nil {
		fullPrefix := string(ctx.Source[ls:start])
		formatted = a.FormatFunc(
			original, wsIndent, fullPrefix, ctx.ColumnLimit,
			ctx.TabStop,
		)
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

	// Scan left from the '(' to find the start of the identifier/selector
	// chain.
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
			// If the selector is applied to something like a call
			// or composite literal, stop at the method name (legacy
			// scanner starts at the selector, not the receiver
			// call).
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

// Execute implements Action for BreakSelectorCallArgsAction.
func (a *BreakSelectorCallArgsAction) Execute(caps Captures, ctx *Context) (
	[]byte, bool) {

	node := resolveTarget(caps, a.Target)
	call, ok := node.(*ast.CallExpr)
	if !ok || call == nil || len(call.Args) == 0 {
		return nil, false
	}
	if _, ok := call.Fun.(*ast.SelectorExpr); !ok {
		return nil, false
	}

	start, end, ok := callSpanOffsets(ctx, call)
	if !ok {
		return nil, false
	}
	open := ctx.Fset.Position(call.Lparen).Offset
	close := ctx.Fset.Position(call.Rparen).Offset
	if open <= start || close < open || close >= len(ctx.Source) {
		return nil, false
	}

	original := ctx.Source[start:end]
	if hasAnyComment(string(original)) ||
		bytes.Contains(original, []byte("\n")) {
		return nil, false
	}
	if ctx.LineWidth(call) <= ctx.ColumnLimit {
		return nil, false
	}

	head := string(ctx.Source[start:open])
	if !strings.Contains(head, "(") {
		return nil, false
	}
	headLineWidth := prefixWidthAt(ctx.Source, start, ctx.TabStop) +
		visualLen(head+"(", ctx.TabStop)
	if headLineWidth > ctx.ColumnLimit {
		return nil, false
	}

	const placeholder = "__llformat_selector_call__"
	argsBody := string(ctx.Source[open+1 : close])
	synthetic := []byte(placeholder + "(" + argsBody + ")")
	wsIndent := ctx.IndentAt(call)

	var formattedSynthetic string
	if a.FormatFunc != nil {
		formattedSynthetic = a.FormatFunc(
			synthetic, wsIndent, placeholder, ctx.ColumnLimit,
			ctx.TabStop,
		)
	} else {
		formattedSynthetic = legacyFormatCallOnePerLine(
			synthetic, wsIndent,
		)
	}
	if !strings.HasPrefix(formattedSynthetic, placeholder) {
		return nil, false
	}

	formatted := head + strings.TrimPrefix(formattedSynthetic, placeholder)
	if formatted == string(original) {
		return nil, false
	}

	out, err := ApplySingleEdit(ctx.Source, start, end, []byte(formatted))
	if err != nil {
		return nil, false
	}

	fset := token.NewFileSet()
	if _, err := parser.ParseFile(
		fset, "out.go", out, parser.AllErrors,
	); err != nil {
		return nil, false
	}

	return out, true
}

// Execute implements Action for PackedMultiLineCallAction.
func (a *PackedMultiLineCallAction) Execute(caps Captures, ctx *Context) (
	[]byte, bool) {

	node := resolveTarget(caps, a.Target)
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return nil, false
	}

	if len(call.Args) == 0 {
		return nil, false
	}

	start, end, ok := callSpanOffsets(ctx, call)
	if !ok {
		return nil, false
	}

	// Skip calls that contain inline comments - reformatting would lose
	// them
	original := ctx.Source[start:end]
	if hasAnyComment(string(original)) {
		return nil, false
	}
	if compact, ok := compactSingleArgCallText(ctx, call, start, end); ok &&
		replacementLinesFitWithinLimit(ctx, start, end, compact) {

		return nil, false
	}

	wsIndent := ctx.IndentAt(call)
	callText := string(original)
	if a.OnlyIfSingleLine && strings.Contains(callText, "\n") {
		return nil, false
	}
	if callAlreadyAcceptableWithMultilineLiteralArg(ctx, call, start, end) {
		return nil, false
	}
	if callFitsSingleLineWithinLimit(ctx, start, end, callText) &&
		!controlHeaderInitCallSuffixOverflows(
			ctx, call, start, end, callText,
		) {
		return nil, false
	}

	// If the call itself would fit on a clean continuation line, and the
	// line overflow is caused by a long multi-assignment prefix, prefer
	// breaking before the call rather than reflowing it into a multiline
	// call.
	//
	// This targets cases like: info, _, _, err := graph.FetchX(arg) where
	// turning the call into: graph.FetchX( arg, ) is a net readability
	// loss.
	if !a.DisableBreakBeforeCallOnLongMultiAssignPrefix {
		if out, changed, ok := maybeBreakBeforeCallForLongMultiAssignPrefix(
			ctx, start, end, wsIndent, callText, len(call.Args),
		); ok {
			return out, changed
		}
	}

	fullPrefix := string(ctx.Source[lineStart(ctx.Source, start):start])
	formatted := formatPackedCall(
		original, wsIndent, fullPrefix, ctx, call, a.FormatFunc,
	)
	if formatted == callText {
		return nil, false
	}

	// Only normalize the original with gofmt to get a canonical form. We
	// compare the formatted output against this normalized original. We do
	// NOT normalize the formatted output because our formatter may
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

// Execute implements Action for CollapseSimpleCallArgsAction.
func (a *CollapseSimpleCallArgsAction) Execute(caps Captures, ctx *Context) (
	[]byte, bool) {

	node := resolveTarget(caps, a.Target)
	call, ok := node.(*ast.CallExpr)
	if !ok || call == nil || len(call.Args) != 1 {
		return nil, false
	}

	start, end, ok := callSpanOffsets(ctx, call)
	if !ok {
		return nil, false
	}

	original := string(ctx.Source[start:end])
	if !strings.Contains(original, "\n") || hasAnyComment(original) {
		return nil, false
	}
	if !isSimpleCallArg(call.Args[0]) {
		return nil, false
	}

	formatted, ok := compactSingleArgCallText(ctx, call, start, end)
	if !ok {
		return nil, false
	}
	if formatted == original ||
		!replacementLinesFitWithinLimit(ctx, start, end, formatted) {

		return nil, false
	}

	out, err := ApplySingleEdit(
		ctx.Source, start, end, []byte(formatted),
	)
	if err != nil || !parseCheckOK(out) {
		return nil, false
	}

	return out, true
}

func compactSingleArgCallText(ctx *Context, call *ast.CallExpr, start,
	end int) (string, bool) {

	if ctx == nil || call == nil || len(call.Args) != 1 ||
		!isSimpleCallArg(call.Args[0]) {

		return "", false
	}

	lparen := ctx.Fset.Position(call.Lparen).Offset
	rparen := ctx.Fset.Position(call.Rparen).Offset
	if lparen < start || rparen <= lparen || rparen > end {
		return "", false
	}

	head := string(ctx.Source[start:lparen])
	if strings.Contains(head, "[\n") {
		return "", false
	}

	arg := strings.TrimSpace(renderNode(call.Args[0], ctx.Fset))
	if arg == "" || strings.Contains(arg, "\n") || hasAnyComment(arg) {
		return "", false
	}

	return head + "(" + arg + ")", true
}

// Execute implements Action for PackReturnStmtAction.
func (a *PackReturnStmtAction) Execute(caps Captures, ctx *Context) ([]byte,
	bool) {

	node := resolveTarget(caps, a.Target)
	ret, ok := node.(*ast.ReturnStmt)
	if !ok || ret == nil || len(ret.Results) < 2 {
		return nil, false
	}

	start := ctx.Fset.Position(ret.Pos()).Offset
	end := ctx.Fset.Position(ret.End()).Offset
	if start < 0 || end > len(ctx.Source) || start >= end {
		return nil, false
	}

	original := string(ctx.Source[start:end])
	if hasAnyComment(original) {
		return nil, false
	}

	exprs := make([]string, 0, len(ret.Results))
	for _, expr := range ret.Results {
		text, ok := compactReturnExprText(ctx, expr)
		if !ok {
			return nil, false
		}
		exprs = append(exprs, text)
	}

	formatted := formatReturnResultsPacked(
		exprs, ctx.IndentAt(ret), ctx.ColumnLimit, ctx.TabStop,
	)
	if formatted == original ||
		!replacementLinesFitWithinLimit(ctx, start, end, formatted) {

		return nil, false
	}

	out, err := ApplySingleEdit(
		ctx.Source, start, end, []byte(formatted),
	)
	if err != nil || !parseCheckOK(out) {
		return nil, false
	}

	return out, true
}

func compactReturnExprText(ctx *Context, expr ast.Expr) (string, bool) {
	start := ctx.Fset.Position(expr.Pos()).Offset
	end := ctx.Fset.Position(expr.End()).Offset
	if start < 0 || end > len(ctx.Source) || start >= end {
		return "", false
	}

	orig := string(ctx.Source[start:end])
	if hasAnyComment(orig) {
		return "", false
	}
	if call, ok := expr.(*ast.CallExpr); ok {
		if compact, ok := compactSingleArgCallText(
			ctx, call, start, end,
		); ok {
			return compact, true
		}
	}
	if strings.Contains(orig, "\n") {
		return "", false
	}

	text := strings.TrimSpace(renderNode(expr, ctx.Fset))
	if text == "" || strings.Contains(text, "\n") || hasAnyComment(text) {
		return "", false
	}

	return text, true
}

func formatReturnResultsPacked(exprs []string, indent string, colLimit,
	tabStop int) string {

	contIndent := indent + "\t"
	var b strings.Builder
	b.WriteString("return ")
	current := indent + "return "
	for i, expr := range exprs {
		sep := ""
		if i > 0 {
			sep = ", "
		}
		candidate := current + sep + expr
		if i < len(exprs)-1 {
			candidate += ","
		}
		if i > 0 && visualLen(candidate, tabStop) > colLimit {
			b.WriteString(",\n")
			b.WriteString(contIndent)
			b.WriteString(expr)
			current = contIndent + expr
			continue
		}

		b.WriteString(sep)
		b.WriteString(expr)
		current += sep + expr
	}

	return b.String()
}

func replacementLinesFitWithinLimit(ctx *Context, start, end int,
	replacement string) bool {

	if ctx == nil {
		return false
	}
	prefix := string(ctx.Source[lineStart(ctx.Source, start):start])
	suffix := string(ctx.Source[end:lineEnd(ctx.Source, end)])
	lines := strings.Split(replacement, "\n")
	if len(lines) == 0 {
		return true
	}
	if len(lines) == 1 {
		return visualLen(prefix+lines[0]+suffix, ctx.TabStop) <=
			ctx.ColumnLimit
	}

	if visualLen(prefix+lines[0], ctx.TabStop) > ctx.ColumnLimit {
		return false
	}
	for _, line := range lines[1 : len(lines)-1] {
		if visualLen(line, ctx.TabStop) > ctx.ColumnLimit {
			return false
		}
	}

	return visualLen(lines[len(lines)-1]+suffix, ctx.TabStop) <=
		ctx.ColumnLimit
}

func callAlreadyAcceptableWithMultilineLiteralArg(ctx *Context,
	call *ast.CallExpr, start, end int) bool {

	if ctx == nil || call == nil {
		return false
	}
	if callHasOverlongKeyedCompositeValueLine(ctx, call) {
		return false
	}
	if !callHasDirectMultilineLiteralArg(ctx, call) {
		return false
	}

	openingEnd := lineEnd(ctx.Source, start)
	openingStart := lineStart(ctx.Source, start)
	if openingEnd < openingStart {
		return false
	}
	openingWidth := visualLen(
		string(ctx.Source[openingStart:openingEnd]), ctx.TabStop,
	)

	return openingWidth <= ctx.ColumnLimit
}

func callHasDirectMultilineLiteralArg(ctx *Context, call *ast.CallExpr) bool {
	for _, arg := range call.Args {
		if arg == nil {
			continue
		}
		if !isDirectLiteralArg(arg) && !hasRawStringLit(arg) {
			continue
		}
		start := ctx.Fset.Position(arg.Pos()).Offset
		end := ctx.Fset.Position(arg.End()).Offset
		if start < 0 || end > len(ctx.Source) || start >= end {
			continue
		}
		if bytes.Contains(ctx.Source[start:end], []byte("\n")) {
			return true
		}
	}

	return false
}

func isDirectLiteralArg(arg ast.Expr) bool {
	switch a := arg.(type) {
	case *ast.CompositeLit, *ast.FuncLit:
		return true

	case *ast.UnaryExpr:
		_, ok := a.X.(*ast.CompositeLit)

		return ok

	default:
		return false
	}
}

func callHasOverlongKeyedCompositeValueLine(ctx *Context,
	call *ast.CallExpr) bool {

	for _, arg := range call.Args {
		lit := directCompositeLitArg(arg)
		if lit == nil || !isMultilineCompositeLit(lit, ctx) {
			continue
		}
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok || kv.Value == nil {
				continue
			}
			if ctx.LineWidth(kv) <= ctx.ColumnLimit {
				continue
			}
			if exprContainsCompositeLit(kv.Value) {
				return true
			}
		}
	}

	return false
}

func directCompositeLitArg(arg ast.Expr) *ast.CompositeLit {
	switch a := arg.(type) {
	case *ast.CompositeLit:
		return a

	case *ast.UnaryExpr:
		lit, _ := a.X.(*ast.CompositeLit)

		return lit

	default:
		return nil
	}
}

func exprContainsCompositeLit(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if _, ok := n.(*ast.CompositeLit); ok {
			found = true

			return false
		}

		return !found
	})

	return found
}

func callSpanOffsets(ctx *Context, call *ast.CallExpr) (start, end int,
	ok bool) {

	if ctx == nil || ctx.Fset == nil || call == nil {
		return 0, 0, false
	}

	start = ctx.Fset.Position(call.Pos()).Offset
	end = ctx.Fset.Position(call.End()).Offset
	if start < 0 || end > len(ctx.Source) || start >= end {
		return 0, 0, false
	}

	return start, end, true
}

func callFitsSingleLineWithinLimit(ctx *Context, start, end int,
	callText string) bool {

	// For already-multiline calls, we must be careful:
	// - A collapsed single-line estimate can be a false negative:
	//   indentation on continuation lines can push a specific line over the
	//   limit even when the collapsed form fits.
	// - Conversely, only checking the existing per-line widths can be a
	//   false positive for "should wrap" decisions: earlier formatting
	//   stages (e.g. string splitting) can introduce newlines inside a
	//   still-too-long call, making each individual line fit while the
	//   canonical single-line call would still exceed the limit. In those
	//   cases we still want to apply packed multiline call formatting to
	//   enforce the house style.
	//
	// So: treat the call as fitting only if BOTH the collapsed single-line
	// estimate fits AND no continuation line currently exceeds the limit.
	if strings.Contains(callText, "\n") {
		collapsedLen := collapsedLineLenAt(
			ctx.Source, start, callText, ctx.TabStop,
		)
		maxLen := maxVisualLineLenInSpan(
			ctx.Source, start, end, ctx.TabStop,
		)

		return collapsedLen <= ctx.ColumnLimit &&
			maxLen <= ctx.ColumnLimit
	}

	currentLineLen := collapsedLineLenAt(
		ctx.Source, start, callText, ctx.TabStop,
	)

	return currentLineLen <= ctx.ColumnLimit
}

func controlHeaderInitCallSuffixOverflows(ctx *Context, call *ast.CallExpr,
	start, end int, callText string) bool {

	if ctx == nil || call == nil || strings.Contains(callText, "\n") {
		return false
	}
	if !callIsDirectControlInitRHS(ctx, call) {
		return false
	}

	lineEndIdx := lineEnd(ctx.Source, end)
	if lineEndIdx <= end {
		return false
	}
	suffixStart := skipHorizontalWhitespace(ctx.Source, end)
	if suffixStart >= lineEndIdx || ctx.Source[suffixStart] != ';' {
		return false
	}

	callLineLen := collapsedLineLenAt(
		ctx.Source, start, callText, ctx.TabStop,
	)
	suffixLen := visualLen(string(ctx.Source[end:lineEndIdx]), ctx.TabStop)

	return callLineLen+suffixLen > ctx.ColumnLimit
}

func callIsDirectControlInitRHS(ctx *Context, call *ast.CallExpr) bool {
	parent := ctx.Parent(call)
	switch p := parent.(type) {
	case *ast.AssignStmt:
		if !assignStmtHasDirectRHSCall(p, call) {
			return false
		}

		return stmtIsControlHeaderInit(ctx, p)

	case *ast.ExprStmt:
		if p.X != call {
			return false
		}

		return stmtIsControlHeaderInit(ctx, p)

	default:
		return false
	}
}

func assignStmtHasDirectRHSCall(assign *ast.AssignStmt,
	call *ast.CallExpr) bool {

	for _, rhs := range assign.Rhs {
		if rhs == call {
			return true
		}
	}

	return false
}

func stmtIsControlHeaderInit(ctx *Context, stmt ast.Stmt) bool {
	parent := ctx.Parent(stmt)
	switch p := parent.(type) {
	case *ast.IfStmt:
		return p.Init == stmt

	case *ast.ForStmt:
		return p.Init == stmt

	case *ast.SwitchStmt:
		return p.Init == stmt

	case *ast.TypeSwitchStmt:
		return p.Init == stmt

	default:
		return false
	}
}

func maybeBreakBeforeCallForLongMultiAssignPrefix(ctx *Context, start, end int,
	wsIndent string, callText string, argCount int) (out []byte,
	changed bool, ok bool) {

	ls := lineStart(ctx.Source, start)
	prefixLine := string(ctx.Source[ls:start])
	trimmedPrefix := strings.TrimSpace(prefixLine)
	trimmedPrefixNoWS := strings.TrimRightFunc(prefixLine, unicode.IsSpace)

	isAssignmentPrefix := strings.HasSuffix(trimmedPrefixNoWS, ":=") ||
		(strings.HasSuffix(trimmedPrefixNoWS, "=") &&
			!strings.HasSuffix(trimmedPrefixNoWS, "==") &&
			!strings.HasSuffix(trimmedPrefixNoWS, "!=") &&
			!strings.HasSuffix(trimmedPrefixNoWS, "<=") &&
			!strings.HasSuffix(trimmedPrefixNoWS, ">="))

	isMultiAssignPrefix := isAssignmentPrefix &&
		strings.Contains(trimmedPrefix, ",")
	if !isMultiAssignPrefix || argCount != 1 {
		return nil, false, false
	}

	contIndent := wsIndent + "\t"
	collapsed := strings.Join(strings.Fields(callText), " ")
	callFitsOnContLine := visualLen(contIndent, ctx.TabStop)+
		visualLen(collapsed, ctx.TabStop) <= ctx.ColumnLimit
	if !callFitsOnContLine {
		return nil, false, false
	}

	replaceStart := start
	for replaceStart > ls &&
		(ctx.Source[replaceStart-1] == ' ' ||
			ctx.Source[replaceStart-1] == '\t') {

		replaceStart--
	}
	if replaceStart >= start {
		return nil, false, false
	}

	var b EditBuilder
	b.Replace(replaceStart, start, []byte("\n"+contIndent))

	// If the call was already multiline, collapse it to a single line now
	// that it has moved to a clean continuation line.
	if strings.Contains(callText, "\n") {
		b.Replace(start, end, []byte(collapsed))
	}

	out, changed, err := b.Apply(ctx.Source)
	if err != nil {
		return nil, false, false
	}

	return out, changed, true
}

func formatPackedCall(original []byte, wsIndent, fullPrefix string,
	ctx *Context, call *ast.CallExpr,
	legacyFormatFunc PackedMultiLineFormatFunc) string {

	if legacyFormatFunc != nil {
		return legacyFormatFunc(
			original, wsIndent, fullPrefix, ctx.ColumnLimit,
			ctx.TabStop,
		)
	}

	return formatCallPackedSimple(call, wsIndent, ctx)
}

// OnePerLineMultiLineCallAction formats a call as a multiline call with one
// argument per line. This matches the legacy MultiLineCallFormatter style more
// closely than PackedMultiLineCallAction (which packs args when possible).
type OnePerLineMultiLineCallAction struct {
	Target string
}

// Execute implements Action for OnePerLineMultiLineCallAction.
func (a *OnePerLineMultiLineCallAction) Execute(caps Captures, ctx *Context) (
	[]byte, bool) {

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

	// Skip calls that contain inline comments - AST rendering would drop
	// them.
	if hasAnyComment(string(original)) {
		return nil, false
	}

	// Skip if the call fits when collapsed to a single line.
	callText := string(original)
	currentLineLen := collapsedLineLenAt(
		ctx.Source, start, callText, ctx.TabStop,
	)
	if currentLineLen <= ctx.ColumnLimit {
		return nil, false
	}

	// Despite the name, use an adaptive strategy: pack simple argument
	// lists tightly, but fall back to one-per-line when any argument is
	// already multiline.
	formatted := formatCallAdaptive(call, wsIndent, ctx)
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
func formatCallPackedSimple(call *ast.CallExpr, indent string,
	ctx *Context) string {

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

// formatCallLeftFlowSimple is a simplified left-flow formatter used when no
// legacy formatter is provided.
func formatCallLeftFlowSimple(call *ast.CallExpr, indent string,
	ctx *Context) string {

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

		if nextWidth, handled := formatLeftFlowStringArg(
			&b, argSrc, i == 0, lineWidth, contIndent,
			contIndentWidth, ctx,
		); handled {

			lineWidth = nextWidth
			continue
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

func formatLeftFlowStringArg(b *strings.Builder, argSrc string, isFirst bool,
	lineWidth int, contIndent string, contIndentWidth int,
	ctx *Context) (int, bool) {

	expr, err := parser.ParseExpr(argSrc)
	if err != nil {
		return lineWidth, false
	}

	strText, ok := llast.FlattenStringExprAST(expr)
	if !ok {
		return lineWidth, false
	}

	if !isFirst {
		// Try to fit on current line with ", ".
		quoted := text.QuoteGoString(strText)
		quotedWidth := visualLen(quoted, ctx.TabStop)
		if lineWidth+2+quotedWidth <= ctx.ColumnLimit {
			b.WriteString(", ")
			b.WriteString(quoted)

			return lineWidth + 2 + quotedWidth, true
		}

		// Need to break - end current line and start new.
		b.WriteString(",\n")
		b.WriteString(contIndent)
		lineWidth = contIndentWidth
	}

	// Split the string if needed.
	split := text.SplitQuotedString(
		strText, lineWidth, contIndent, ctx.ColumnLimit, ctx.TabStop,
	)
	b.WriteString(split)

	return lineWidthAfterSplit(
		split, lineWidth, contIndentWidth, ctx.TabStop,
	), true
}

func lineWidthAfterSplit(split string, lineWidth int, contIndentWidth int,
	tabStop int) int {

	if idx := strings.LastIndex(split, "\n"); idx >= 0 {
		return contIndentWidth + visualLen(split[idx+1:], tabStop)
	}

	return lineWidth + visualLen(split, tabStop)
}

// SignatureFormatFunc is the signature for the function signature formatting
// function. This allows injecting the legacy formatter implementation to avoid
// circular imports. Returns the formatted signature and whether a blank line
// should be added after.
type SignatureFormatFunc func(signature, indent string, colLimit, tabStop int) (string, bool)

func formatSignatureWithFallback(signature, indent string, colLimit,
	tabStop int, formatFunc SignatureFormatFunc,
	fallback SignatureFormatFunc) (string, bool) {

	if formatFunc != nil {
		return formatFunc(signature, indent, colLimit, tabStop)
	}
	if fallback != nil {
		return fallback(signature, indent, colLimit, tabStop)
	}

	return formatSignatureSimple(signature, indent, colLimit, tabStop)
}

// BreakFuncSignatureAction breaks a long function signature using left-flow
// packing. It extracts the entire signature line and reformats it.
type BreakFuncSignatureAction struct {
	Target     string
	FormatFunc SignatureFormatFunc
}

// BreakFuncLitSignatureAction formats function literals (closures) by
// extracting the literal signature from `func` to `{` and reformatting it.
//
// Unlike BreakFuncSignatureAction, this does not insert blank lines after the
// opening brace; function literals should remain compact.
type BreakFuncLitSignatureAction struct {
	Target     string
	FormatFunc SignatureFormatFunc
}

func tryInsertBlankLineAfterBrace(ctx *Context, lineStart, afterBrace int,
	formatted string, signatureUnchanged bool) ([]byte, bool, error) {

	// Check for an existing newline after the signature's opening brace,
	// and if the following line contains code, insert an additional blank
	// line to separate a multiline signature header from the body.
	newline, ok := signatureHeaderLineEnd(ctx.Source, afterBrace)
	if !ok {
		return nil, false, nil
	}

	// There's already a newline after brace; check the next line.
	pos := newline + 1
	lineContentStart := pos
	pos = skipHorizontalWhitespace(ctx.Source, pos)
	if pos >= len(ctx.Source) || ctx.Source[pos] == '\n' ||
		ctx.Source[pos] == '}' {
		return nil, false, nil
	}

	var b EditBuilder
	if !signatureUnchanged {
		b.Replace(lineStart, afterBrace, []byte(formatted))
	}
	b.Insert(lineContentStart, []byte("\n"))

	out, changed, err := b.Apply(ctx.Source)
	if err != nil {
		return nil, false, err
	}
	if !changed {
		return nil, false, nil
	}

	fset := token.NewFileSet()
	if _, err := parser.ParseFile(
		fset, "out.go", out, parser.AllErrors,
	); err != nil {
		return nil, false, err
	}

	return out, true, nil
}

func tryRemoveBlankLineAfterBrace(ctx *Context, lineStart, afterBrace int,
	formatted string, signatureUnchanged bool) ([]byte, bool, error) {

	newline, ok := signatureHeaderLineEnd(ctx.Source, afterBrace)
	if !ok {
		return nil, false, nil
	}

	blankStart := newline + 1
	pos := skipHorizontalWhitespace(ctx.Source, blankStart)
	if pos >= len(ctx.Source) || ctx.Source[pos] != '\n' {
		return nil, false, nil
	}

	var b EditBuilder
	if !signatureUnchanged {
		b.Replace(lineStart, afterBrace, []byte(formatted))
	}
	b.Delete(blankStart, pos+1)

	out, changed, err := b.Apply(ctx.Source)
	if err != nil {
		return nil, false, err
	}
	if !changed {
		return nil, false, nil
	}
	if !parseCheckOK(out) {
		return nil, false, nil
	}

	return out, true, nil
}

func signatureHeaderLineEnd(src []byte, afterBrace int) (int, bool) {
	pos := skipHorizontalWhitespace(src, afterBrace)
	if pos+1 < len(src) && src[pos] == '/' && src[pos+1] == '/' {
		for pos < len(src) && src[pos] != '\n' {
			pos++
		}
	}
	if pos >= len(src) || src[pos] != '\n' {
		return 0, false
	}

	return pos, true
}

// Execute implements Action for BreakFuncLitSignatureAction.
func (a *BreakFuncLitSignatureAction) Execute(caps Captures, ctx *Context) (
	[]byte, bool) {

	node := resolveTarget(caps, a.Target)

	funcLit, ok := node.(*ast.FuncLit)
	if !ok || funcLit == nil || funcLit.Body == nil ||
		!funcLit.Body.Lbrace.IsValid() {
		return nil, false
	}

	// Get source positions.
	start := ctx.Fset.Position(funcLit.Pos()).Offset
	bracePos := ctx.Fset.Position(funcLit.Body.Lbrace).Offset
	if start < 0 || bracePos > len(ctx.Source) || start >= bracePos {
		return nil, false
	}

	// Find the start of the line containing the literal, so we can preserve
	// any prefix before the `func` keyword (e.g. `x := `).
	lineStart := start
	for lineStart > 0 && ctx.Source[lineStart-1] != '\n' {
		lineStart--
	}
	prefix := string(ctx.Source[lineStart:start])
	// Extract the signature including the opening brace (starting at
	// `func`).
	signature := strings.TrimSpace(string(ctx.Source[start : bracePos+1]))
	wsIndent := ctx.IndentAt(node)

	var formatted string
	var needsBlank bool
	signatureForFormat := signature
	// For function literals assigned inline (e.g. `x := func(...) ... {`)
	// or used as composite-literal fields (e.g. `Field: func(...) {`), the
	// available width of the first line is affected by the prefix before
	// the `func` keyword.
	//
	// Do not model this by reducing the column limit globally: continuation
	// lines do not include the prefix, and reducing the budget can cause
	// needless breaking (e.g. splitting small return lists).
	//
	// Instead, include the prefix in the formatted string we pass to the
	// signature formatter, then strip it back out before applying the edit.
	prefixSuffix := strings.TrimPrefix(prefix, wsIndent)
	prefixWidth := visualLen(prefix, ctx.TabStop)
	wsIndentWidth := visualLen(wsIndent, ctx.TabStop)
	prefixTrimmed := strings.TrimRight(prefix, " \t")
	// If the prefix alone already overflows the column limit, modeling it
	// in the signature formatter cannot bring the combined line under the
	// limit, but it can make the signature itself look much worse (e.g.
	// forcing `func(` onto a new line). In that case, keep the signature
	// formatting consistent by ignoring the prefix for width calculations.
	hasSyntheticPrefix := prefixSuffix != "" &&
		prefixWidth > wsIndentWidth &&
		visualLen(prefixTrimmed, ctx.TabStop) < ctx.ColumnLimit
	if hasSyntheticPrefix {
		// Do not inject the full prefix text (which can include
		// parentheses, commas, etc. when the literal is a call
		// argument); that can confuse the signature parser which
		// expects a `func...{` signature.
		//
		// Instead, model the *width* of the prefix using spaces so the
		// formatter makes a correct first-line decision, and then strip
		// it back out.
		pad := prefixWidth - wsIndentWidth
		if pad > 0 {
			signatureForFormat = strings.Repeat(" ", pad) +
				signature
		}
	}
	if a.FormatFunc != nil {
		// FormatFunc expects `indent` to be the leading whitespace
		// indentation, not the full prefix before `func`.
		formatted, needsBlank = a.FormatFunc(
			signatureForFormat, wsIndent, ctx.ColumnLimit,
			ctx.TabStop,
		)
	} else {
		formatted, _ = formatSignatureSimple(
			signatureForFormat, wsIndent, ctx.ColumnLimit,
			ctx.TabStop,
		)
	}
	if hasSyntheticPrefix {
		formatted = stripLeadingNonWhitespaceUpToFuncKeyword(
			formatted, wsIndent,
		)
	}

	if shouldBreakFuncLitCallArgPrefix(
		prefix, formatted, wsIndent, ctx.ColumnLimit, ctx.TabStop,
	) {

		formatted = strings.TrimRight(prefix, " \t") + "\n" +
			indentFuncLitCallArgSignature(
				formatted, wsIndent, wsIndent+"\t",
			)
	} else {
		// Reattach the original prefix (e.g. `x := `) to the first
		// line.
		if nl := strings.IndexByte(formatted, '\n'); nl >= 0 {
			first := formatted[:nl]
			rest := formatted[nl:]
			first = prefix + strings.TrimPrefix(first, wsIndent)
			formatted = first + rest
		} else {
			formatted = prefix +
				strings.TrimPrefix(formatted, wsIndent)
		}
	}

	// For function literals, treat "signature is multiline" as requiring
	// the readability blank line after the opening brace, regardless of how
	// the injected signature formatter computes needsBlank. The native
	// signature formatters can conservatively return needsBlank=false for
	// multiline breaks that occur inside parenthesized return lists, but
	// the "next" golden spec expects the blank line for multiline
	// func-literal signatures.
	if strings.Contains(formatted, "\n") {
		needsBlank = true
	}

	signatureUnchanged := formatted == prefix+signature

	afterBrace := bracePos + 1

	// Match the FuncDecl behavior: when the signature is multi-line, add a
	// blank line after the opening brace (unless the signature already
	// contains nested multiline content like broken func types, where
	// additional spacing is usually excessive).
	hasNestedMultiline := false
	if firstFunc := strings.Index(formatted, "func("); firstFunc >= 0 {
		// Function literals themselves begin with `func(`, so `func(\n`
		// can be normal for a multi-line literal signature. Only treat
		// it as "nested" multiline content if it appears again after
		// the initial literal header.
		searchFrom := firstFunc + len("func(")
		if searchFrom < len(formatted) {
			hasNestedMultiline = strings.Contains(
				formatted[searchFrom:], "func(\n",
			)
		}
	}
	if needsBlank && !hasNestedMultiline {
		out, changed, err := tryInsertBlankLineAfterBrace(
			ctx, lineStart, afterBrace, formatted,
			signatureUnchanged,
		)
		if err != nil {
			return nil, false
		}
		if changed {
			return out, true
		}
	}
	if !needsBlank {
		out, changed, err := tryRemoveBlankLineAfterBrace(
			ctx, lineStart, afterBrace, formatted,
			signatureUnchanged,
		)
		if err != nil {
			return nil, false
		}
		if changed {
			return out, true
		}
	}

	if signatureUnchanged {
		return nil, false
	}

	out, err := ApplySingleEdit(
		ctx.Source, lineStart, afterBrace, []byte(formatted),
	)
	if err != nil {
		return nil, false
	}

	fset := token.NewFileSet()
	if _, err := parser.ParseFile(
		fset, "out.go", out, parser.AllErrors,
	); err != nil {
		return nil, false
	}

	return out, true
}

func shouldBreakFuncLitCallArgPrefix(prefix, formatted, wsIndent string,
	colLimit, tabStop int) bool {

	trimmedPrefix := strings.TrimRight(prefix, " \t")
	if !strings.HasSuffix(trimmedPrefix, ",") {
		return false
	}
	if visualLen(trimmedPrefix, tabStop) > colLimit {
		return false
	}

	first := formatted
	if nl := strings.IndexByte(first, '\n'); nl >= 0 {
		first = first[:nl]
	}
	reattachedFirst := prefix + strings.TrimPrefix(first, wsIndent)

	return visualLen(reattachedFirst, tabStop) > colLimit
}

func indentFuncLitCallArgSignature(formatted, fromIndent,
	toIndent string) string {

	lines := strings.Split(formatted, "\n")
	for i, line := range lines {
		lines[i] = toIndent + strings.TrimPrefix(line, fromIndent)
	}

	return strings.Join(lines, "\n")
}

func stripLeadingNonWhitespaceUpToFuncKeyword(formatted,
	wsIndent string) string {

	// We include the prefix before `func` in the formatted signature to
	// model first-line width constraints, but the FuncLit edit span starts
	// at the `func` keyword. Strip the prefix back out by removing
	// everything between the indentation and the final `func` keyword on
	// the first line.
	//
	// Using the last `func` occurrence avoids false positives for prefixes
	// like `somefunc := func(...) {`.
	nl := strings.IndexByte(formatted, '\n')
	first := formatted
	rest := ""
	if nl >= 0 {
		first = formatted[:nl]
		rest = formatted[nl:]
	}

	firstNoIndent := strings.TrimPrefix(first, wsIndent)
	idx := strings.LastIndex(firstNoIndent, "func")
	if idx < 0 {
		return formatted
	}

	// Keep the indentation intact so downstream code can still strip it
	// when reattaching the original prefix.
	first = wsIndent + firstNoIndent[idx:]

	return first + rest
}

// Execute implements Action for BreakFuncSignatureAction.
func (a *BreakFuncSignatureAction) Execute(caps Captures, ctx *Context) ([]byte,
	bool) {

	node := resolveTarget(caps, a.Target)
	funcDecl, ok := node.(*ast.FuncDecl)
	if !ok || funcDecl == nil || funcDecl.Body == nil ||
		!funcDecl.Body.Lbrace.IsValid() {
		return nil, false
	}

	// Get positions
	funcStart := ctx.Fset.Position(funcDecl.Pos()).Offset
	bracePos := ctx.Fset.Position(funcDecl.Body.Lbrace).Offset

	// Extract the signature from "func" to "{"
	signature := strings.TrimSpace(
		string(ctx.Source[funcStart : bracePos+1]),
	)

	// Get the indent
	indent := ctx.IndentAt(node)

	// Format using the injected formatter or fallback
	var formatted string
	var needsBlank bool

	formatted, needsBlank = formatSignatureWithFallback(
		signature, indent, ctx.ColumnLimit, ctx.TabStop, a.FormatFunc,
		nil,
	)

	// Check if the formatted signature is different We need to compare the
	// actual strings, not normalized versions, since formatting adds
	// newlines and indentation
	signatureUnchanged := formatted == indent+signature

	// Find where to resume after the opening brace
	afterBrace := bracePos + 1

	// Find the start of the line containing the signature
	lineStart := funcStart
	for lineStart > 0 && ctx.Source[lineStart-1] != '\n' {
		lineStart--
	}

	// If multi-line and there's content after the brace, add a blank line
	// for readability.
	if needsBlank {
		out, changed, err := tryInsertBlankLineAfterBrace(
			ctx, lineStart, afterBrace, formatted,
			signatureUnchanged,
		)
		if err != nil {
			return nil, false
		}
		if changed {
			return out, true
		}
	}
	if !needsBlank {
		out, changed, err := tryRemoveBlankLineAfterBrace(
			ctx, lineStart, afterBrace, formatted,
			signatureUnchanged,
		)
		if err != nil {
			return nil, false
		}
		if changed {
			return out, true
		}
	}

	if signatureUnchanged {
		return nil, false
	}

	out, err := ApplySingleEdit(
		ctx.Source, lineStart, afterBrace, []byte(formatted),
	)
	if err != nil || !parseCheckOK(out) {
		return nil, false
	}

	return out, true
}

func lastLineText(s string) string {
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		return s[i+1:]
	}

	return s
}

// indentContinuationLines prefixes every continuation line in s with indent.
// The first line is left unchanged.
func indentContinuationLines(s, indent string) string {
	if !strings.Contains(s, "\n") {
		return s
	}
	parts := strings.Split(s, "\n")
	for i := 1; i < len(parts); i++ {
		parts[i] = indent + parts[i]
	}

	return strings.Join(parts, "\n")
}

func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') || b == '_'
}

func findMatchingBrace(s string, openBrace int) int {
	if openBrace < 0 || openBrace >= len(s) || s[openBrace] != '{' {
		return -1
	}
	depth := 0
	inStr := byte(0)
	escaped := false

	for i := openBrace; i < len(s); i++ {
		c := s[i]

		if inStr != 0 {
			if inStr == '"' && c == '\\' && !escaped {
				escaped = true
				continue
			}
			if escaped {
				escaped = false
				continue
			}
			if c == inStr {
				inStr = 0
			}
			continue
		}

		switch c {
		case '"', '`':
			inStr = c
			continue

		case '{':
			depth++

		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}

	return -1
}

func expandInlineBracedTypeLiteral(s, keyword string) string {
	var out strings.Builder
	i := 0

	for i < len(s) {
		idx := strings.Index(s[i:], keyword)
		if idx == -1 {
			out.WriteString(s[i:])
			break
		}
		idx += i

		// Require a word boundary for the keyword.
		if idx > 0 && isWordChar(s[idx-1]) {
			out.WriteString(s[i : idx+len(keyword)])
			i = idx + len(keyword)
			continue
		}

		j := idx + len(keyword)
		if j < len(s) && isWordChar(s[j]) {
			out.WriteString(s[i : idx+len(keyword)])
			i = idx + len(keyword)
			continue
		}

		// Skip optional whitespace between keyword and brace.
		k := j
		for k < len(s) && (s[k] == ' ' || s[k] == '\t') {
			k++
		}
		if k >= len(s) || s[k] != '{' {
			out.WriteString(s[i : idx+len(keyword)])
			i = idx + len(keyword)
			continue
		}

		end := findMatchingBrace(s, k)
		if end == -1 {
			out.WriteString(s[i:])
			break
		}

		body := s[k+1 : end]
		// If it already spans multiple lines or doesn't use semicolons,
		// keep as-is.
		if strings.Contains(body, "\n") ||
			!strings.Contains(body, ";") {

			out.WriteString(s[i : end+1])
			i = end + 1
			continue
		}

		// Expand `struct{ A; B }` -> `struct {\nA\nB\n}` for
		// readability when signatures become multiline.
		parts := strings.Split(body, ";")
		out.WriteString(s[i:idx])
		out.WriteString(keyword)
		out.WriteString(" {\n")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			out.WriteByte('\t')
			out.WriteString(part)
			out.WriteByte('\n')
		}
		out.WriteByte('}')

		i = end + 1
	}

	return out.String()
}

func expandInlineTypeLiterals(s string) string {
	s = expandInlineBracedTypeLiteral(s, "struct")
	s = expandInlineBracedTypeLiteral(s, "interface")

	return s
}

func skipSpaces(s string, i int) int {
	for i < len(s) &&
		(s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {

		i++
	}

	return i
}

func scanBalanced(s string, open, close byte, i int) (int, bool) {
	if i < 0 || i >= len(s) || s[i] != open {
		return -1, false
	}
	depth := 0
	inStr := byte(0)
	escaped := false

	for ; i < len(s); i++ {
		c := s[i]

		if inStr != 0 {
			if inStr == '"' && c == '\\' && !escaped {
				escaped = true
				continue
			}
			if escaped {
				escaped = false
				continue
			}
			if c == inStr {
				inStr = 0
			}
			continue
		}

		switch c {
		case '"', '`':
			inStr = c
			continue
		}

		if c == open {
			depth++
			continue
		}
		if c == close {
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}

	return -1, false
}

func scanIdent(s string, i int) int {
	if i >= len(s) {
		return i
	}
	if s[i] != '_' && (s[i] < 'A' || s[i] > 'Z') &&
		(s[i] < 'a' || s[i] > 'z') {
		return i
	}
	i++
	for i < len(s) &&
		(s[i] == '_' || (s[i] >= 'A' && s[i] <= 'Z') ||
			(s[i] >= 'a' &&
				s[i] <= 'z') || (s[i] >= '0' && s[i] <= '9')) {

		i++
	}

	return i
}

func formatPackedMultilineTypeList(elems []string, itemIndent,
	baseIndent string, colLimit, tabStop int) string {

	indentWidth := visualLen(itemIndent, tabStop)

	var b strings.Builder
	lineWidth := 0
	wroteAny := false
	atLineStart := true

	for _, elem := range elems {
		elem = strings.TrimSpace(elem)
		if elem == "" {
			continue
		}

		isMultiline := strings.Contains(elem, "\n")

		if atLineStart {
			b.WriteString(itemIndent)
			lineWidth = indentWidth
			atLineStart = false
		} else {
			lineWidth, atLineStart = writePackedTypeListSeparator(
				&b, elem, isMultiline, itemIndent, lineWidth,
				indentWidth, colLimit, tabStop,
			)
		}

		b.WriteString(elem)
		wroteAny = true

		if isMultiline {
			b.WriteString(",\n")
			lineWidth = 0
			atLineStart = true
			continue
		}

		lineWidth += visualLen(elem, tabStop)
	}

	// Ensure trailing comma for multiline lists (helps gofmt keep breaks).
	if wroteAny && !atLineStart {
		b.WriteString(",\n")
		atLineStart = true
	}

	if !atLineStart {
		b.WriteByte('\n')
	}
	b.WriteString(baseIndent)
	b.WriteString(")")

	return b.String()
}

func writePackedTypeListSeparator(b *strings.Builder, elem string,
	isMultiline bool, itemIndent string, lineWidth, indentWidth, colLimit,
	tabStop int) (int, bool) {

	if isMultiline {
		b.WriteString(",\n")
		b.WriteString(itemIndent)

		return indentWidth, false
	}

	need := 2 + visualLen(elem, tabStop) // ", " + elem
	if lineWidth+need > colLimit {
		b.WriteString(",\n")
		b.WriteString(itemIndent)

		return indentWidth, false
	}

	b.WriteString(", ")

	return lineWidth + 2, false
}

func formatSignatureCompat(sig, indent string, colLimit,
	tabStop int) (string, bool) {

	openParen, closeParen, ok := findFuncParamList(sig)
	if !ok {
		return indent + sig, false
	}

	// Ensure we can find the function body brace; if not, bail.
	if findTopLevelFuncBodyBrace(sig, closeParen+1) == -1 {
		return indent + sig, false
	}

	prefix := sig[:openParen+1] // "func name("
	params := sig[openParen+1 : closeParen]
	suffix := sig[closeParen:] // ") ... {"

	paramList := splitTopLevelSimple(params)
	if len(paramList) == 0 {
		return indent + sig, false
	}

	contIndent := indent + "\t"

	var result strings.Builder
	result.WriteString(indent)
	result.WriteString(prefix)
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

		testLine := currentLine + separator + param
		isLast := i == len(paramList)-1
		lineWithSuffix := testLine
		if isLast {
			lineWithSuffix = testLine + suffix
		}

		if visualLen(lineWithSuffix, tabStop) > colLimit {
			// Break to new line (legacy-ish style: no trailing
			// comma, keep parens attached to prefix/suffix).
			if i > 0 {
				result.WriteByte(',')
			}
			result.WriteByte('\n')
			result.WriteString(contIndent)
			result.WriteString(param)
			currentLine = contIndent + param
			continue
		}

		if i > 0 {
			result.WriteString(", ")
		}
		result.WriteString(param)
		currentLine = testLine
	}

	result.WriteString(suffix)
	needsBlank := strings.Contains(result.String(), "\n")

	return result.String(), needsBlank
}

// findFuncParamList locates the opening and closing parenthesis of the function
// parameter list in a full `func ... {` signature, including optional receiver
// and optional generic type parameter list.
func findFuncParamList(sig string) (openParen, closeParen int, ok bool) {
	i := strings.Index(sig, "func")
	if i == -1 {
		return -1, -1, false
	}
	i += len("func")
	i = skipSpaces(sig, i)

	// Optional receiver: func (r *R) Name(...)
	if i < len(sig) && sig[i] == '(' {
		end, ok := scanBalanced(sig, '(', ')', i)
		if !ok {
			return -1, -1, false
		}
		i = end + 1
		i = skipSpaces(sig, i)
	}

	// Function name (may be absent for func literals, but we only format
	// decls).
	i = scanIdent(sig, i)
	i = skipSpaces(sig, i)

	// Optional type parameters: func Name[T any, U ~int](...)
	if i < len(sig) && sig[i] == '[' {
		end, ok := scanBalanced(sig, '[', ']', i)
		if !ok {
			return -1, -1, false
		}
		i = end + 1
		i = skipSpaces(sig, i)
	}

	if i >= len(sig) || sig[i] != '(' {
		return -1, -1, false
	}
	openParen = i
	end, ok := scanBalanced(sig, '(', ')', openParen)
	if !ok {
		return -1, -1, false
	}

	return openParen, end, true
}

func findTopLevelFuncBodyBrace(sig string, start int) int {
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	inStr := byte(0)
	escaped := false

	for i := start; i < len(sig); i++ {
		c := sig[i]
		if consumeStringChar(c, &inStr, &escaped) {
			continue
		}

		switch c {
		case '"', '`':
			inStr = c
			continue

		case '(':
			parenDepth++

		case ')':
			if parenDepth > 0 {
				parenDepth--
			}

		case '[':
			bracketDepth++

		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}

		case '{':
			if isFuncBodyBrace(
				sig, i, parenDepth, bracketDepth, braceDepth,
			) {
				return i
			}
			braceDepth++

		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		}
	}

	return -1
}

func consumeStringChar(c byte, inStr *byte, escaped *bool) bool {
	if *inStr == 0 {
		return false
	}
	if *inStr == '"' && c == '\\' && !*escaped {
		*escaped = true

		return true
	}
	if *escaped {
		*escaped = false

		return true
	}
	if c == *inStr {
		*inStr = 0
	}

	return true
}

func isFuncBodyBrace(sig string, idx, parenDepth, bracketDepth,
	braceDepth int) bool {

	if parenDepth != 0 || bracketDepth != 0 || braceDepth != 0 {
		return false
	}

	// Avoid mis-identifying `struct{...}` / `interface{...}` type literals
	// in result types as the function body brace.
	word := prevIdent(sig, idx-1)

	return word != "struct" && word != "interface"
}

func prevIdent(sig string, idx int) string {
	i := idx
	for i >= 0 && (sig[i] == ' ' || sig[i] == '\t') {
		i--
	}
	j := i
	for j >= 0 && isIdentChar(sig[j]) {
		j--
	}

	return sig[j+1 : i+1]
}

func isIdentChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func isParenthesizedTypeList(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '(' || s[len(s)-1] != ')' {
		return false
	}

	depth := 0
	inStr := byte(0)
	escaped := false

	for i := 0; i < len(s); i++ {
		c := s[i]

		if inStr != 0 {
			if inStr == '"' && c == '\\' && !escaped {
				escaped = true
				continue
			}
			if escaped {
				escaped = false
				continue
			}
			if c == inStr {
				inStr = 0
			}
			continue
		}

		switch c {
		case '"', '`':
			inStr = c

		case '(':
			depth++

		case ')':
			depth--
			if depth == 0 && i != len(s)-1 {
				return false
			}
		}
	}

	return depth == 0
}

// simpleReturnsContext holds context for formatting return types in simple
// signature/method formatters.
type simpleReturnsContext struct {
	result      *strings.Builder
	returns     string
	contIndent  string
	currentLine string
	expandTypes bool
	colLimit    int
	tabStop     int
}

// formatSimpleReturns formats return types for simple signature/method
// formatters. Never breaks the line between ")" and the first return token to
// avoid triggering Go's semicolon insertion.
func (ctx *simpleReturnsContext) formatSimpleReturns() {
	returnsOut := ctx.returns
	if ctx.expandTypes {
		returnsOut = expandInlineTypeLiterals(returnsOut)
	}

	if !isParenthesizedTypeList(ctx.returns) {
		ctx.result.WriteByte(' ')
		ctx.result.WriteString(
			indentContinuationLines(returnsOut, ctx.contIndent),
		)

		return
	}

	inner := strings.TrimSpace(returnsOut[1 : len(returnsOut)-1])
	innerList := splitTopLevelSimple(inner)

	if len(innerList) == 0 {
		ctx.result.WriteByte(' ')
		ctx.result.WriteString(
			indentContinuationLines(returnsOut, ctx.contIndent),
		)

		return
	}

	ctx.formatParenthesizedReturns(innerList, returnsOut)
}

// formatParenthesizedReturns handles parenthesized return type lists.
func (ctx *simpleReturnsContext) formatParenthesizedReturns(innerList []string,
	returnsOut string) {

	needMulti := strings.Contains(ctx.result.String(), "\n") ||
		visualLen(ctx.currentLine+" "+returnsOut, ctx.tabStop) > ctx.colLimit

	itemIndent := ctx.contIndent + "\t"
	formattedElems := make([]string, 0, len(innerList))

	for _, elem := range innerList {
		elem = strings.TrimSpace(elem)
		if elem == "" {
			continue
		}
		elemOut := elem
		if ctx.expandTypes {
			elemOut = expandInlineTypeLiterals(elemOut)
			elemOut = indentContinuationLines(elemOut, itemIndent)
		}
		if strings.Contains(elemOut, "\n") {
			needMulti = true
		}
		formattedElems = append(formattedElems, elemOut)
	}

	if !needMulti {
		ctx.writeInlineReturns(formattedElems)

		return
	}

	// Keep "(" on same line as ")" to avoid semicolon insertion.
	ctx.result.WriteString(" (\n")
	ctx.result.WriteString(
		formatPackedMultilineTypeList(
			formattedElems, itemIndent, ctx.contIndent,
			ctx.colLimit, ctx.tabStop,
		),
	)
}

// writeInlineReturns writes return elements on a single line.
func (ctx *simpleReturnsContext) writeInlineReturns(elems []string) {
	ctx.result.WriteString(" (")
	for i, elem := range elems {
		if i > 0 {
			ctx.result.WriteString(", ")
		}
		ctx.result.WriteString(elem)
	}
	ctx.result.WriteString(")")
}

// formatSignatureSimple is a fallback formatter that breaks at commas. Uses
// left-flow packing: break BEFORE elements that would exceed the limit.
func formatSignatureSimple(sig, indent string, colLimit,
	tabStop int) (string, bool) {

	// Parse out params from signature: func name(params) returns {
	openParen, closeParen, ok := findFuncParamList(sig)
	if !ok {
		return indent + sig, false
	}

	bodyBrace := findTopLevelFuncBodyBrace(sig, closeParen+1)
	if bodyBrace == -1 {
		return indent + sig, false
	}

	prefix := sig[:openParen+1] // "func name("
	params := sig[openParen+1 : closeParen]
	returns := strings.TrimSpace(sig[closeParen+1 : bodyBrace]) // "returns"
	hasReturns := returns != ""
	bodySuffix := sig[bodyBrace:] // "{"

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
	expandTypes := visualLen(indent+sig, tabStop) > colLimit
	needMultiParams := expandTypes
	paramElems := make([]string, 0, len(paramList))
	for _, param := range paramList {
		param = strings.TrimSpace(param)
		if param == "" {
			continue
		}

		paramOut := param
		if expandTypes {
			paramOut = expandInlineTypeLiterals(paramOut)
			paramOut = indentContinuationLines(paramOut, contIndent)
		}
		if strings.Contains(paramOut, "\n") {
			needMultiParams = true
		}
		paramElems = append(paramElems, paramOut)
	}

	if needMultiParams {
		result.WriteByte('\n')
		result.WriteString(
			formatPackedMultilineTypeList(
				paramElems, contIndent, indent, colLimit,
				tabStop,
			),
		)
		currentLine = indent + ")"
	} else {
		for i, paramOut := range paramElems {
			separator := ""
			if i > 0 {
				separator = ", "
			}

			testLine := currentLine + separator + paramOut
			if visualLen(testLine, tabStop) > colLimit {
				if i > 0 {
					result.WriteByte(',')
				}
				result.WriteByte('\n')
				result.WriteString(contIndent)
				result.WriteString(paramOut)
				currentLine = contIndent +
					lastLineText(paramOut)
				continue
			}

			if i > 0 {
				result.WriteString(", ")
			}
			result.WriteString(paramOut)
			currentLine = testLine
		}

		result.WriteByte(')')
		currentLine = lastLineText(result.String())
	}

	// Format results (if present) before the opening brace.
	if hasReturns {
		ctx := &simpleReturnsContext{
			result:      &result,
			returns:     returns,
			contIndent:  contIndent,
			currentLine: currentLine,
			expandTypes: expandTypes,
			colLimit:    colLimit,
			tabStop:     tabStop,
		}
		ctx.formatSimpleReturns()
	}

	// Append body opener.
	built := result.String()
	if len(built) > 0 {
		last := built[len(built)-1]
		if last != '\n' && last != ' ' && last != '\t' {
			result.WriteByte(' ')
		}
	}
	result.WriteString(bodySuffix)

	isMultiLine := strings.Contains(result.String(), "\n")

	return result.String(), isMultiLine
}

// splitTopLevelSimple splits a string at top-level commas.
func splitTopLevelSimple(s string) []string {
	var result []string
	var current strings.Builder
	depth := 0
	inString := byte(0)
	escaped := false

	for i := 0; i < len(s); i++ {
		c := s[i]

		if consumeStringChar(c, &inString, &escaped) {
			current.WriteByte(c)
			continue
		}
		if c == '"' || c == '`' {
			inString = c
			current.WriteByte(c)
			continue
		}

		// Skip over comments while splitting. This is important for
		// signatures that include parameter comments like `/* ... , ...
		// */` where commas should not be treated as separators.
		if nextIdx, ok := consumeComment(s, i, &current); ok {
			i = nextIdx
			continue
		}

		switch c {
		case '(', '[', '{':
			depth++

		case ')', ']', '}':
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

func consumeComment(s string, i int, current *strings.Builder) (int, bool) {
	if s[i] != '/' || i+1 >= len(s) {
		return i, false
	}

	next := s[i+1]
	if next == '/' {
		// Line comment: consume to end of line.
		current.WriteByte(s[i])
		current.WriteByte(next)
		j := i + 2
		for j < len(s) && s[j] != '\n' {
			current.WriteByte(s[j])
			j++
		}
		if j < len(s) && s[j] == '\n' {
			current.WriteByte('\n')
		}

		return j, true
	}

	if next == '*' {
		// Block comment: consume to closing */ (or end of string).
		current.WriteByte(s[i])
		current.WriteByte(next)
		j := i + 2
		for j < len(s) {
			current.WriteByte(s[j])
			if s[j] == '*' && j+1 < len(s) && s[j+1] == '/' {
				current.WriteByte('/')
				j++
				break
			}
			j++
		}

		return j, true
	}

	return i, false
}

// BreakInterfaceMethodAction formats a long interface method declaration.
type BreakInterfaceMethodAction struct {
	Target     string
	FormatFunc SignatureFormatFunc
}

// Execute implements Action for BreakInterfaceMethodAction.
func (a *BreakInterfaceMethodAction) Execute(caps Captures, ctx *Context) (
	[]byte, bool) {

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

	// Get positions
	fieldStart := ctx.Fset.Position(field.Pos()).Offset
	fieldEnd := ctx.Fset.Position(field.End()).Offset
	if fieldStart < 0 || fieldEnd > len(ctx.Source) ||
		fieldStart >= fieldEnd {
		return nil, false
	}

	// The original interface-method rule only fired when the method
	// exceeded the column limit. The "next" profile also wants to reflow
	// signatures that are already multiline (e.g. to collapse short return
	// lists) even when no single line exceeds the limit.
	//
	// Preserve the fast-path skip for single-line methods that already fit.
	sigText := string(ctx.Source[fieldStart:fieldEnd])
	if !strings.Contains(sigText, "\n") &&
		ctx.LineWidth(node) <= ctx.ColumnLimit {
		return nil, false
	}

	// Extract the method declaration
	method := strings.TrimSpace(sigText)

	// Get the indent
	indent := ctx.IndentAt(node)

	// Format using the injected formatter or fallback
	var formatted string
	formatted, _ = formatSignatureWithFallback(
		method, indent, ctx.ColumnLimit, ctx.TabStop, a.FormatFunc,
		func(signature, indent string, colLimit, tabStop int) (string,
			bool) {

			return formatMethodSimple(
				signature, indent, colLimit, tabStop,
			), false
		},
	)

	// Check if formatted is different
	if formatted == indent+method {
		return nil, false
	}

	// Find the start of the line
	ls := lineStart(ctx.Source, fieldStart)

	// Find end of line after the method.
	le := lineEnd(ctx.Source, fieldEnd)

	// Preserve any trailing comment on the same line.
	suffix := extractTrailingComment(ctx.Source, fieldEnd, le)
	formattedWithSuffix := strings.TrimRight(formatted, "\n") + suffix
	out, err := ApplySingleEdit(
		ctx.Source, ls, le, []byte(formattedWithSuffix),
	)
	if err != nil || !parseCheckOK(out) {
		return nil, false
	}

	return out, true
}

// formatMethodSimple is a fallback formatter for interface methods.
func formatMethodSimple(method, indent string, colLimit, tabStop int) string {
	// Parse out params from method: Method(params) returns
	openParen := strings.Index(method, "(")
	if openParen == -1 {
		return indent + method
	}

	// Find matching close paren.
	depth := 0
	closeParen := -1
	for i := openParen; i < len(method); i++ {
		if method[i] == '(' {
			depth++
		} else if method[i] == ')' {
			depth--
			if depth == 0 {
				closeParen = i
				break
			}
		}
	}
	if closeParen == -1 {
		return indent + method
	}

	prefix := method[:openParen+1]
	params := method[openParen+1 : closeParen]
	returns := strings.TrimSpace(method[closeParen+1:]) // "returns"
	hasReturns := returns != ""

	paramList := splitTopLevelSimple(params)
	if len(paramList) == 0 {
		return indent + method
	}

	contIndent := indent + "\t"

	var result strings.Builder
	result.WriteString(indent)
	result.WriteString(prefix)

	currentLine := indent + prefix
	expandTypes := visualLen(indent+method, tabStop) > colLimit
	needMultiParams := expandTypes
	paramElems := make([]string, 0, len(paramList))
	for _, param := range paramList {
		param = strings.TrimSpace(param)
		if param == "" {
			continue
		}

		paramOut := param
		if expandTypes {
			paramOut = expandInlineTypeLiterals(paramOut)
			paramOut = indentContinuationLines(paramOut, contIndent)
		}
		if strings.Contains(paramOut, "\n") {
			needMultiParams = true
		}
		paramElems = append(paramElems, paramOut)
	}

	if needMultiParams {
		result.WriteByte('\n')
		result.WriteString(
			formatPackedMultilineTypeList(
				paramElems, contIndent, indent, colLimit,
				tabStop,
			),
		)
		currentLine = indent + ")"
	} else {
		for i, paramOut := range paramElems {
			separator := ""
			if i > 0 {
				separator = ", "
			}

			testLine := currentLine + separator + paramOut
			if visualLen(testLine, tabStop) > colLimit {
				if i > 0 {
					result.WriteByte(',')
				}
				result.WriteByte('\n')
				result.WriteString(contIndent)
				result.WriteString(paramOut)
				currentLine = contIndent +
					lastLineText(paramOut)
				continue
			}

			if i > 0 {
				result.WriteString(", ")
			}
			result.WriteString(paramOut)
			currentLine = testLine
		}

		result.WriteByte(')')
		currentLine = lastLineText(result.String())
	}

	// Results (if present).
	if hasReturns {
		ctx := &simpleReturnsContext{
			result:      &result,
			returns:     returns,
			contIndent:  contIndent,
			currentLine: currentLine,
			expandTypes: expandTypes,
			colLimit:    colLimit,
			tabStop:     tabStop,
		}
		ctx.formatSimpleReturns()
	}

	return result.String()
}

// InsertBlankBeforeAction inserts a blank line before a node if not already
// present.
type InsertBlankBeforeAction struct {
	Target string
}

// BlankLinesBatchAction inserts all blank lines required by the blank-line DSL
// policy in a single deterministic rewrite.
//
// The default DSL engine applies at most one transforming rule per iteration
// for determinism, which makes per-node blank-line rules expensive for files
// with many cases/returns/methods. This action keeps the logic in the DSL while
// avoiding hundreds of iterations.
type BlankLinesBatchAction struct {
	Options BlankLineOptions
}

func (a *BlankLinesBatchAction) Execute(caps Captures, ctx *Context) ([]byte,
	bool) {

	node := resolveTarget(caps, "node")
	file, ok := node.(*ast.File)
	if !ok || file == nil {
		return nil, false
	}

	bctx := &blankLineBatchContext{
		ctx:           ctx,
		insertOffsets: make(map[int]struct{}),
		opts:          a.Options,
	}
	bctx.initConditions()
	bctx.inspectFile(file)

	out, changed, err := bctx.b.Apply(ctx.Source)
	if err != nil || !changed {
		return nil, false
	}
	if !parseCheckOK(out) {
		return nil, false
	}

	return out, true
}

// blankLineBatchContext holds state for the blank line batch action.
type blankLineBatchContext struct {
	ctx             *Context
	b               EditBuilder
	insertOffsets   map[int]struct{}
	opts            BlankLineOptions
	caseCond        Condition
	returnCond      Condition
	ifaceMethodCond Condition
	ifErrReturnCond Condition
}

func (c *blankLineBatchContext) initConditions() {
	c.caseCond = &HasPrecedingSiblingCond{Target: "node"}
	c.returnCond = &IsReturnNeedingBlankCond{Target: "node"}
	c.ifaceMethodCond = &AndCond{Conds: []Condition{
		&IsInterfaceMethodCond{
			Target: "node",
		},
		&HasPrecedingInterfaceFieldCond{
			Target: "node",
		},
	}}
	c.ifErrReturnCond = &IsIfErrReturnNeedingBlankCond{Target: "node"}
}

func (c *blankLineBatchContext) maybeInsertBlankBefore(n ast.Node) {
	c.maybeInsertBlankBeforeNode(n, true)
}

func (c *blankLineBatchContext) maybeInsertBlankBeforeClause(n ast.Node) {
	insert, attachLeadingComments := clauseSeparatorBehavior(c.ctx, n)
	if !insert {
		return
	}

	c.maybeInsertBlankBeforeNode(n, attachLeadingComments)
}

func (c *blankLineBatchContext) maybeInsertBlankBeforeNode(n ast.Node,
	attachLeadingComments bool) {

	if n == nil {
		return
	}
	start := c.ctx.Fset.Position(n.Pos()).Offset
	if start <= 0 || start >= len(c.ctx.Source) {
		return
	}
	ls := lineStart(c.ctx.Source, start)
	if attachLeadingComments {
		ls = leadingCommentBlockLineStart(c.ctx.Source, ls)
	}
	if ls <= 0 || hasBlankLineBeforeLineStart(c.ctx.Source, ls) {
		return
	}
	if _, ok := c.insertOffsets[ls]; ok {
		return
	}
	c.insertOffsets[ls] = struct{}{}
	c.b.Insert(ls, []byte("\n"))
}

func (c *blankLineBatchContext) inspectFile(file *ast.File) {
	ast.Inspect(
		file,
		func(n ast.Node) bool {
			caps := Captures{"node": n}
			switch n.(type) {
			case *ast.CaseClause:
				c.maybeRemoveBlankAfterSingleLineBlockHeader(n)
				if c.caseCond.Eval(caps, c.ctx) {
					c.maybeInsertBlankBeforeClause(n)
				}

			case *ast.CommClause:
				c.maybeRemoveBlankAfterSingleLineBlockHeader(n)
				if c.caseCond.Eval(caps, c.ctx) {
					c.maybeInsertBlankBeforeClause(n)
				}

			case *ast.ReturnStmt:
				if onlyStmtInBlockWithoutLeadingComment(
					c.ctx, n,
				) {

					start := c.ctx.Fset.Position(n.Pos()).Offset
					c.maybeRemoveBlankBeforeLineStart(
						lineStart(c.ctx.Source, start),
					)
					break
				}
				if c.returnCond.Eval(caps, c.ctx) {
					c.maybeInsertBlankBefore(n)
				}

			case *ast.Field:
				if c.ifaceMethodCond.Eval(caps, c.ctx) {
					c.maybeInsertBlankBefore(n)
				}

			case *ast.IfStmt:
				c.maybeRemoveBlankAfterSingleLineBlockHeader(n)
				if c.opts.ExtraIfErrReturn && c.ifErrReturnCond.Eval(
					caps, c.ctx,
				) {

					c.maybeInsertBlankBefore(n)
				}

			case *ast.ForStmt:
				c.maybeRemoveBlankAfterSingleLineBlockHeader(n)

			case *ast.RangeStmt:
				c.maybeRemoveBlankAfterSingleLineBlockHeader(n)
			}

			return true
		},
	)
}

func (c *blankLineBatchContext) maybeRemoveBlankAfterSingleLineBlockHeader(
	node ast.Node) {

	var first ast.Stmt
	var headerStart token.Position
	var headerEnd token.Position

	switch n := node.(type) {
	case *ast.IfStmt:
		if n == nil || n.Body == nil || len(n.Body.List) == 0 {
			return
		}
		first = n.Body.List[0]
		headerStart = c.ctx.Fset.Position(n.Pos())
		headerEnd = c.ctx.Fset.Position(n.Body.Lbrace)

	case *ast.ForStmt:
		if n == nil || n.Body == nil || len(n.Body.List) == 0 {
			return
		}
		first = n.Body.List[0]
		headerStart = c.ctx.Fset.Position(n.Pos())
		headerEnd = c.ctx.Fset.Position(n.Body.Lbrace)

	case *ast.RangeStmt:
		if n == nil || n.Body == nil || len(n.Body.List) == 0 {
			return
		}
		first = n.Body.List[0]
		headerStart = c.ctx.Fset.Position(n.Pos())
		headerEnd = c.ctx.Fset.Position(n.Body.Lbrace)

	case *ast.CaseClause:
		if n == nil {
			return
		}
		headerStart = c.ctx.Fset.Position(n.Pos())
		headerEnd = c.ctx.Fset.Position(n.Colon)
		c.maybeRemoveBlankAfterHeaderLine(headerStart, headerEnd)
		if headerStart.Line == headerEnd.Line {
			return
		}
		if len(n.Body) == 0 {
			return
		}
		first = n.Body[0]

	case *ast.CommClause:
		if n == nil {
			return
		}
		headerStart = c.ctx.Fset.Position(n.Pos())
		headerEnd = c.ctx.Fset.Position(n.Colon)
		c.maybeRemoveBlankAfterHeaderLine(headerStart, headerEnd)
		if headerStart.Line == headerEnd.Line {
			return
		}
		if len(n.Body) == 0 {
			return
		}
		first = n.Body[0]

	default:
		return
	}

	if headerStart.Line != headerEnd.Line {
		return
	}

	if _, ok := first.(*ast.ReturnStmt); ok &&
		onlyStmtInBlockWithoutLeadingComment(c.ctx, first) {
		return
	}

	start := c.ctx.Fset.Position(first.Pos()).Offset
	if start <= 0 || start > len(c.ctx.Source) {
		return
	}
	lineStartIdx := lineStart(c.ctx.Source, start)
	if leadingCommentBlockLineStart(c.ctx.Source, lineStartIdx) !=
		lineStartIdx {
		return
	}
	c.maybeRemoveBlankBeforeLineStart(lineStartIdx)
}

func (c *blankLineBatchContext) maybeRemoveBlankAfterHeaderLine(
	headerStart, headerEnd token.Position) {

	if headerStart.Line != headerEnd.Line {
		return
	}

	headerLineEnd := lineEnd(c.ctx.Source, headerEnd.Offset)
	if headerLineEnd < 0 || headerLineEnd >= len(c.ctx.Source) ||
		c.ctx.Source[headerLineEnd] != '\n' {
		return
	}

	blankLineStart := headerLineEnd + 1
	blankLineEnd := lineEnd(c.ctx.Source, blankLineStart)
	if blankLineEnd < blankLineStart || blankLineEnd >= len(c.ctx.Source) {
		return
	}
	if !isWhitespaceOnlyLine(c.ctx.Source[blankLineStart:blankLineEnd]) {
		return
	}

	c.b.Delete(blankLineStart, blankLineEnd+1)
}

func (c *blankLineBatchContext) maybeRemoveBlankBeforeLineStart(
	lineStartIdx int) {

	if lineStartIdx <= 0 || lineStartIdx > len(c.ctx.Source) {
		return
	}
	if !hasBlankLineBeforeLineStart(c.ctx.Source, lineStartIdx) {
		return
	}

	prevLineEnd := lineStartIdx - 1
	prevStart := lineStart(c.ctx.Source, prevLineEnd)
	if prevStart < 0 || prevStart > lineStartIdx {
		return
	}

	c.b.Delete(prevStart, lineStartIdx)
}

// parseCheckOK performs a defensive parse check on the output.
func parseCheckOK(out []byte) bool {
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors)

	return err == nil
}

// extractTrailingComment extracts and normalizes trailing comments from a line.
func extractTrailingComment(source []byte, nodeEnd, lineEnd int) string {
	if nodeEnd < 0 || lineEnd <= nodeEnd || lineEnd > len(source) {
		return ""
	}
	lineSuffix := string(source[nodeEnd:lineEnd])
	trimmed := strings.TrimLeft(lineSuffix, " \t")
	if !strings.HasPrefix(trimmed, "//") &&
		!strings.HasPrefix(trimmed, "/*") {
		return lineSuffix
	}
	// Normalize to a single space before the comment.
	newline := ""
	if strings.HasSuffix(lineSuffix, "\n") {
		newline = "\n"
	}

	return " " + strings.TrimRight(trimmed, "\n") + newline
}

func isWhitespaceOnlyLine(b []byte) bool {
	for _, c := range b {
		if c != ' ' && c != '\t' && c != '\r' {
			return false
		}
	}

	return true
}

func leadingCommentBlockLineStart(src []byte, targetLineStart int) int {
	// If the line above the target is a comment-only line, we treat it as a
	// leading comment block and insert the blank line above the comment
	// rather than between the comment and the node.
	//
	// This is intentionally heuristic: we only capture comment blocks that
	// begin at line start (after indentation), so we avoid hoisting
	// trailing comments.
	start := targetLineStart
	inBlockComment := false

	for start > 0 {
		prevLineEnd := start -
			1 // points at '\n' of the previous line (or last byte)
		prevStart := lineStart(src, prevLineEnd)
		if prevStart < 0 || prevStart >= start {
			break
		}

		line := src[prevStart:prevLineEnd]
		if isWhitespaceOnlyLine(line) {
			break
		}

		trimmed := bytes.TrimLeft(line, " \t")
		if len(trimmed) == 0 {
			break
		}

		if inBlockComment {
			start = prevStart
			if bytes.HasPrefix(trimmed, []byte("/*")) {
				inBlockComment = false
			}
			continue
		}

		if bytes.HasPrefix(trimmed, []byte("//")) {
			start = prevStart
			continue
		}

		if bytes.HasPrefix(trimmed, []byte("/*")) {
			start = prevStart
			if !bytes.Contains(trimmed, []byte("*/")) {
				inBlockComment = true
			}
			continue
		}

		// Handle the common "*/" or " * ..." endings of a leading block
		// comment.
		if bytes.Contains(trimmed, []byte("*/")) &&
			(bytes.HasPrefix(trimmed, []byte("*/")) ||
				bytes.HasPrefix(trimmed, []byte("*"))) {

			start = prevStart
			inBlockComment = true
			continue
		}

		break
	}

	return start
}

func hasBlankLineBeforeLineStart(src []byte, targetLineStart int) bool {
	if targetLineStart <= 0 {
		return true // start-of-file: treat as "already separated"
	}
	prevLineEnd := targetLineStart - 1
	prevStart := lineStart(src, prevLineEnd)
	if prevStart < 0 {
		return true
	}
	prevLine := src[prevStart:prevLineEnd]

	return isWhitespaceOnlyLine(prevLine)
}

// Execute implements Action for InsertBlankBeforeAction.
func (a *InsertBlankBeforeAction) Execute(caps Captures, ctx *Context) ([]byte,
	bool) {

	node := resolveTarget(caps, a.Target)
	if node == nil {
		return nil, false
	}

	pos := ctx.Fset.Position(node.Pos())
	nodeStart := pos.Offset

	// Find the start of the line
	ls := lineStart(ctx.Source, nodeStart)
	ls = leadingCommentBlockLineStart(ctx.Source, ls)

	// Check if there's already a blank line before this line
	if hasBlankLineBeforeLineStart(ctx.Source, ls) {
		return nil, false
	}

	// Insert blank line before the current line
	out, err := ApplySingleEdit(ctx.Source, ls, ls, []byte("\n"))
	if err != nil {
		return nil, false
	}

	return out, true
}

// InsertBlankBeforeClauseAction inserts a blank line before a switch/select
// clause label.
type InsertBlankBeforeClauseAction struct {
	Target string
}

// Execute implements Action for InsertBlankBeforeClauseAction.
func (a *InsertBlankBeforeClauseAction) Execute(caps Captures, ctx *Context) (
	[]byte, bool) {

	node := resolveTarget(caps, a.Target)
	if node == nil {
		return nil, false
	}

	pos := ctx.Fset.Position(node.Pos())
	insert, attachLeadingComments := clauseSeparatorBehavior(ctx, node)
	if !insert {
		return nil, false
	}

	ls := lineStart(ctx.Source, pos.Offset)
	if attachLeadingComments {
		ls = leadingCommentBlockLineStart(ctx.Source, ls)
	}
	if hasBlankLineBeforeLineStart(ctx.Source, ls) {
		return nil, false
	}

	out, err := ApplySingleEdit(ctx.Source, ls, ls, []byte("\n"))
	if err != nil {
		return nil, false
	}

	return out, true
}

func clauseSeparatorBehavior(ctx *Context,
	node ast.Node) (insert bool, attachLeadingComments bool) {

	parent, ok := ctx.Parent(node).(*ast.BlockStmt)
	if !ok || parent == nil {
		return true, true
	}

	for i, stmt := range parent.List {
		if stmt != node {
			continue
		}
		if i == 0 {
			return true, true
		}

		prev := parent.List[i-1]
		if clauseBodyLen(prev) > 0 {
			return true, true
		}

		if clauseGapHasComment(ctx, prev, node) {
			return true, false
		}

		return false, false
	}

	return true, true
}

func clauseBodyLen(stmt ast.Stmt) int {
	switch n := stmt.(type) {
	case *ast.CaseClause:
		if n == nil {
			return 0
		}

		return len(n.Body)

	case *ast.CommClause:
		if n == nil {
			return 0
		}

		return len(n.Body)

	default:
		return 0
	}
}

func clauseGapHasComment(ctx *Context, prev ast.Stmt, next ast.Node) bool {
	start := clauseColonOffset(ctx, prev)
	end := ctx.Fset.Position(next.Pos()).Offset
	if start < 0 || end < 0 || start >= end || end > len(ctx.Source) {
		return false
	}

	gap := ctx.Source[start:end]

	return bytes.Contains(gap, []byte("//")) ||
		bytes.Contains(gap, []byte("/*"))
}

func clauseColonOffset(ctx *Context, stmt ast.Stmt) int {
	var pos token.Pos
	switch n := stmt.(type) {
	case *ast.CaseClause:
		if n == nil {
			return -1
		}
		pos = n.Colon

	case *ast.CommClause:
		if n == nil {
			return -1
		}
		pos = n.Colon

	default:
		return -1
	}

	return ctx.Fset.Position(pos).Offset
}

// InsertBlankAfterAction inserts a blank line after a node if not already
// present.
type InsertBlankAfterAction struct {
	Target string
}

// Execute implements Action for InsertBlankAfterAction.
func (a *InsertBlankAfterAction) Execute(caps Captures, ctx *Context) ([]byte,
	bool) {

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
		for checkPos < len(ctx.Source) &&
			(ctx.Source[checkPos] == ' ' ||
				ctx.Source[checkPos] == '\t') {

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

// BreakMethodChainAction breaks a method chain with one call per line. The dot
// is placed at the end of each line (trailing dot style).
type BreakMethodChainAction struct {
	Target string
}

// Execute implements Action for BreakMethodChainAction.
func (a *BreakMethodChainAction) Execute(caps Captures, ctx *Context) ([]byte,
	bool) {

	node := resolveTarget(caps, a.Target)
	if node == nil {
		return nil, false
	}

	// Find the outermost call expression in the chain
	call, ok := node.(*ast.CallExpr)
	if !ok {
		// Could be an assignment, extract the RHS
		if assign, ok := node.(*ast.AssignStmt); ok &&
			len(assign.Rhs) > 0 {

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
	if hasAnyComment(originalCall) {
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

	// Get the original source positions We need to find the start of the
	// chain (the receiver) and end of the last call
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

	if chainStart < 0 || chainEnd > len(ctx.Source) ||
		chainStart >= chainEnd {
		return nil, false
	}

	// Compute the already-present prefix width on the line before the chain
	// (e.g. "result := " before "client.Foo().Bar()"). This avoids
	// decisions that would fit if the chain started at column 0, but
	// overflow once the actual prefix is accounted for.
	ls := lineStart(ctx.Source, chainStart)
	prefixWidth := visualLen(string(ctx.Source[ls:chainStart]), ctx.TabStop)

	// Get the base indentation (whitespace only; used for continuation
	// lines).
	indent := ctx.IndentAt(node)

	formatted := formatMethodChain(chainCalls, indent, prefixWidth, ctx)

	original := string(ctx.Source[chainStart:chainEnd])
	if !isSafeStandaloneExprSpan(original) {
		return nil, false
	}

	// Check if formatting actually changed anything
	if formatted == original {
		return nil, false
	}

	// Build result by replacing the chain in source
	out, err := ApplySingleEdit(
		ctx.Source, chainStart, chainEnd, []byte(formatted),
	)
	if err != nil {
		return nil, false
	}

	return out, true
}

// collectMethodChain collects all calls in a method chain from outermost to
// innermost.
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

// formatMethodChain formats a method chain with smart breaking. It packs as
// many calls as fit on each line and only breaks when needed. When a call's
// arguments need wrapping, the arguments are placed on a new line with the
// closing ) followed by the next method call.
func formatMethodChain(calls []*ast.CallExpr, indent string,
	initialLineWidth int, ctx *Context) string {

	if len(calls) == 0 {
		return ""
	}

	var b strings.Builder
	contIndent := indent + "\t"

	// Process calls from innermost to outermost
	currentLineWidth := initialLineWidth

	for i := len(calls) - 1; i >= 0; i-- {
		call := calls[i]
		isFirst := i == len(calls)-1

		// Build the method part: ".MethodName" or "receiver.MethodName"
		// for first
		methodPart := methodPartForCall(call, isFirst, ctx)

		// Build the arguments part
		var argParts []string
		for _, arg := range call.Args {
			argParts = append(argParts, renderNode(arg, ctx.Fset))
		}
		argsInline := strings.Join(argParts, ", ")

		// Calculate the inline version of this call
		callInline := buildCallInline(methodPart, argsInline, isFirst)
		callWidth := visualLen(callInline, ctx.TabStop)

		// Check if the call fits on the current line
		if currentLineWidth+callWidth <= ctx.ColumnLimit {
			// It fits - write inline
			writeMethodCall(&b, methodPart, argsInline, isFirst)
			currentLineWidth += callWidth
			continue
		}

		// Doesn't fit - check if the method call with just opening
		// paren fits.
		methodWidth := methodOpenWidth(methodPart, isFirst, ctx.TabStop)
		argsWidth := visualLen(argsInline+")", ctx.TabStop)
		if len(call.Args) > 0 &&
			currentLineWidth+methodWidth+
				argsWidth > ctx.ColumnLimit {

			// Arguments need to be on a new line.
			if !isFirst {
				b.WriteString(".")
			}
			b.WriteString(methodPart)
			b.WriteString("(\n")
			b.WriteString(contIndent)
			b.WriteString(argsInline)
			b.WriteString(",\n")
			b.WriteString(indent)
			b.WriteString(")")
			currentLineWidth = visualLen(indent, ctx.TabStop) +
				1 // After the closing paren
			continue
		}

		// The call itself is too long but args are short. Write inline.
		writeMethodCall(&b, methodPart, argsInline, isFirst)
		currentLineWidth += callWidth
	}

	return b.String()
}

func methodPartForCall(call *ast.CallExpr, isFirst bool, ctx *Context) string {
	if isFirst {
		// First call - include the receiver.
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			receiverSrc := renderNode(sel.X, ctx.Fset)

			return receiverSrc + "." + sel.Sel.Name
		}

		return renderNode(call.Fun, ctx.Fset)
	}

	// Subsequent calls - just the method name (the dot is added later).
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		return sel.Sel.Name
	}

	return renderNode(call.Fun, ctx.Fset)
}

func buildCallInline(methodPart, argsInline string, isFirst bool) string {
	if isFirst {
		return methodPart + "(" + argsInline + ")"
	}

	return "." + methodPart + "(" + argsInline + ")"
}

func methodOpenWidth(methodPart string, isFirst bool, tabStop int) int {
	if isFirst {
		return visualLen(methodPart+"(", tabStop)
	}

	return visualLen("."+methodPart+"(", tabStop)
}

func writeMethodCall(b *strings.Builder, methodPart, argsInline string,
	isFirst bool) {

	if !isFirst {
		b.WriteString(".")
	}
	b.WriteString(methodPart)
	b.WriteString("(")
	b.WriteString(argsInline)
	b.WriteString(")")
}

// BreakReturnValuesAction breaks long return values to a new line.
type BreakReturnValuesAction struct {
	Target string
}

// Execute implements Action for BreakReturnValuesAction.
func (a *BreakReturnValuesAction) Execute(caps Captures, ctx *Context) ([]byte,
	bool) {

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

	if funcType == nil || funcType.Results == nil ||
		len(funcType.Results.List) == 0 {
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

	out, changed, err := applyContinuationIndentAfter(
		ctx.Source, resultsOpen+1, indent,
	)
	if err != nil {
		return nil, false
	}

	return out, changed
}
