package dsl

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"hash/maphash"
	"os"
	"reflect"
	"sort"
	"strings"

	llast "github.com/bhandras/llformat/ast"
)

// Engine executes formatting rules.
type Engine struct {
	Rules             []Rule
	ColumnLimit       int
	TabStop           int
	MaxIterations     int
	Trace             bool // Enable trace logging
	TraceReasons      bool // Include "why fired/didn't fire" reasons in trace output
	NodeOrder         NodeOrder
	AutoMaxIterations bool // Derive MaxIterations from the AST when true
	DetectCycles      bool // Stop if the engine repeats a previous output

	// ForbiddenSpans holds the union of spans that this engine instance
	// should not rewrite (e.g. spans owned by later pipeline stages).
	ForbiddenSpans llast.OffsetSpanSet

	// Budget provides optional guardrails against pathological rule
	// behavior (e.g. exponential growth, accidental whole-file rewrites).
	Budget RewriteBudget

	// StageName is an optional label (e.g. "expressions",
	// "multiline-calls") that will be included in trace output for easier
	// debugging.
	StageName string
}

// RewriteBudget describes optional safety limits for the DSL engine. Zero
// values mean "no limit".
type RewriteBudget struct {
	// MaxOutputBytes rejects a rewrite if the candidate output exceeds this
	// absolute size.
	MaxOutputBytes int

	// MaxBytesIncrease rejects a rewrite if the candidate output grows more
	// than this many bytes beyond the initial input size for the engine
	// run.
	MaxBytesIncrease int
}

// NewEngine creates a rule engine with default settings.
func NewEngine(rules []Rule) *Engine {
	// Sort rules by priority (descending - higher priority first). Keep
	// relative order stable for equal priority to avoid nondeterminism.
	sorted := make([]Rule, len(rules))
	copy(sorted, rules)
	sort.SliceStable(
		sorted,
		func(i, j int) bool {
			return sorted[i].Priority > sorted[j].Priority
		},
	)

	return &Engine{
		Rules:         sorted,
		ColumnLimit:   80,
		TabStop:       8,
		MaxIterations: 100,
		NodeOrder:     NodeOrderPreorder,
	}
}

// Format applies rules to source code.
func (e *Engine) Format(src []byte) ([]byte, error) {
	result := src
	initialLen := len(src)

	maxIters := e.MaxIterations
	if e.AutoMaxIterations {
		maxIters = e.estimateMaxIterations(result)
	}
	if maxIters <= 0 {
		// Defensive fallback: never run an unbounded loop.
		maxIters = 1
	}

	var seed maphash.Seed
	var seen map[uint64]struct{}
	if e.DetectCycles {
		seed = maphash.MakeSeed()
		seen = make(map[uint64]struct{}, 8)
		seen[e.hashBytes(seed, result)] = struct{}{}
	}

	for iter := 0; iter < maxIters; iter++ {
		// Parse current source
		fset := token.NewFileSet()
		file, err := parser.ParseFile(
			fset, "", result, parser.AllErrors|parser.ParseComments,
		)
		if file == nil {
			// If we can't parse, we can still try to apply
			// file-scoped scan/delegation rules that don't require
			// an AST (e.g. legacy comment formatting).
			ctx := NewContext(
				token.NewFileSet(), result, e.ColumnLimit,
				e.TabStop,
			)
			ctx.Parseable = false
			ctx.ForbiddenSpans = e.ForbiddenSpans
			modified, changed := e.applyOneFileRuleWithoutAST(
				iter+1, ctx,
			)
			if !changed {
				return result, nil
			}

			e.traceEdit(ctx, modified, iter+1)

			result = modified

			if e.DetectCycles &&
				e.detectCycle(seen, seed, iter+1, result) {

				break
			}
			continue
		}

		ctx := NewContext(fset, result, e.ColumnLimit, e.TabStop)
		ctx.Parseable = err == nil
		ctx.ForbiddenSpans = e.ForbiddenSpans

		// First pass: apply atomic markers (high priority keep_together
		// rules)
		e.applyAtomicMarkers(file, ctx)

		// Second pass: try to apply one transforming rule
		modified, changed := e.applyOneRule(iter+1, file, ctx)
		if !changed {
			break
		}

		if !e.withinBudget(initialLen, modified) {
			// Refuse the rewrite and stop: a later rule in this
			// iteration might have produced a smaller/safer result,
			// but we intentionally apply at most one transforming
			// rule per iteration for determinism.
			e.traceBudgetExceeded(ctx, iter+1)
			break
		}

		e.traceAppliedRule(ctx, modified, iter+1)
		e.traceEdit(ctx, modified, iter+1)

		// Keep the edited source as-is and rely on the outer pipeline
		// to run gofmt once at the end. Running gofmt here would
		// reformat unrelated code and violate llformat's "only touch
		// targeted regions" goal.
		result = modified

		if e.DetectCycles &&
			e.detectCycle(seen, seed, iter+1, result) {

			break
		}
	}

	return result, nil
}

func (e *Engine) traceEdit(ctx *Context, modified []byte, iter int) {
	if !e.Trace {
		return
	}

	start, endBefore, endAfter := changedSpan(
		ctx.Source, modified,
	)
	line, col := offsetToLineCol(ctx.Source, start)
	fmt.Fprintf(
		os.Stderr, "dsl: stage=%s iter=%d rule=%s prio=%d node=%s "+
			"nodeSpan=[%d:%d] editSpan=[%d:%d]->[%d:%d] @%d:%d "+
			"snippet=%q\n", e.StageName, iter, ctx.LastAppliedRule,
		ctx.LastAppliedRulePriority, ctx.LastAppliedNodeType,
		ctx.LastAppliedNodeStart, ctx.LastAppliedNodeEnd, start,
		endBefore, start, endAfter, line, col,
		snippetForRange(ctx.Source, start, endBefore),
	)
}

func (e *Engine) traceAppliedRule(ctx *Context, modified []byte, iter int) {
	if !e.TraceReasons || e.Trace {
		return
	}

	start, endBefore, endAfter := changedSpan(
		ctx.Source, modified,
	)
	line, col := offsetToLineCol(ctx.Source, start)
	fmt.Fprintf(
		os.Stderr, "dsl: stage=%s iter=%d applied rule=%s prio=%d "+
			"node=%s nodeSpan=[%d:%d] editSpan=[%d:%d]->[%d:%d] "+
			"@%d:%d snippet=%q\n", e.StageName, iter,
		ctx.LastAppliedRule, ctx.LastAppliedRulePriority,
		ctx.LastAppliedNodeType, ctx.LastAppliedNodeStart,
		ctx.LastAppliedNodeEnd, start, endBefore, start, endAfter, line,
		col, snippetForRange(ctx.Source, start, endBefore),
	)
}

func (e *Engine) traceBudgetExceeded(ctx *Context, iter int) {
	if !e.TraceReasons || e.Trace {
		return
	}

	fmt.Fprintf(
		os.Stderr, "dsl: stage=%s iter=%d applied rule=%s prio=%d "+
			"node=%s nodeSpan=[%d:%d] reason=%s\n", e.StageName,
		iter, ctx.LastAppliedRule, ctx.LastAppliedRulePriority,
		ctx.LastAppliedNodeType, ctx.LastAppliedNodeStart,
		ctx.LastAppliedNodeEnd, "budget_exceeded",
	)
}

func (e *Engine) detectCycle(seen map[uint64]struct{}, seed maphash.Seed,
	iter int, result []byte) bool {

	h := e.hashBytes(seed, result)
	if _, ok := seen[h]; ok {
		e.traceCycleDetected(iter)

		return true
	}
	seen[h] = struct{}{}

	return false
}

func (e *Engine) traceCycleDetected(iter int) {
	if !e.TraceReasons || e.Trace {
		return
	}

	fmt.Fprintf(
		os.Stderr, "dsl: stage=%s iter=%d reason=%s\n", e.StageName,
		iter, "cycle_detected",
	)
}

func (e *Engine) hashBytes(seed maphash.Seed, b []byte) uint64 {
	var h maphash.Hash
	h.SetSeed(seed)
	h.Write(b)

	return h.Sum64()
}

func (e *Engine) estimateMaxIterations(src []byte) int {
	// If the file isn't parseable, auto-iteration can't estimate safely.
	// The caller may still have file-level fallback rules, so allow a
	// single pass.
	fset := token.NewFileSet()
	file, err := parser.ParseFile(
		fset, "", src, parser.AllErrors|parser.ParseComments,
	)
	if file == nil {
		return 1
	}
	_ = err

	nodeTypes := e.ruleNodeTypes()
	if len(nodeTypes) == 0 {
		return 1
	}

	candidates := 0
	ast.Inspect(
		file,
		func(n ast.Node) bool {
			if n == nil {
				return true
			}
			rt := reflect.TypeOf(n)
			if rt == nil {
				return true
			}
			if rt.Kind() == reflect.Ptr {
				rt = rt.Elem()
			}
			if rt == nil {
				return true
			}
			if _, ok := nodeTypes[rt.Name()]; ok {
				candidates++
			}

			return true
		},
	)

	// Apply at most one transforming rewrite per iteration. In the common
	// case, each iteration fixes one candidate node, so a small multiplier
	// is enough. Clamp to keep pathological files from running too long by
	// accident.
	const (
		minIters = 20
		maxIters = 5000
	)
	estimate := candidates*2 + 20
	if estimate < minIters {
		estimate = minIters
	}
	if estimate > maxIters {
		estimate = maxIters
	}

	return estimate
}

func (e *Engine) ruleNodeTypes() map[string]struct{} {
	out := make(map[string]struct{}, len(e.Rules))
	for _, r := range e.Rules {
		np, ok := r.Pattern.(*NodePattern)
		if !ok || np == nil {
			continue
		}
		if np.Type == "" {
			continue
		}
		out[np.Type] = struct{}{}
	}

	return out
}

func (e *Engine) withinBudget(initialLen int, candidate []byte) bool {
	if e == nil {
		return true
	}
	if e.Budget.MaxOutputBytes > 0 &&
		len(candidate) > e.Budget.MaxOutputBytes {

		return false
	}
	if e.Budget.MaxBytesIncrease > 0 &&
		len(candidate) > initialLen+e.Budget.MaxBytesIncrease {

		return false
	}

	return true
}

func (e *Engine) applyOneFileRuleWithoutAST(iter int, ctx *Context) ([]byte,
	bool) {

	const maxReasonsPerIter = 30
	reasonsPrinted := 0

	for _, rule := range e.Rules {
		np, ok := rule.Pattern.(*NodePattern)
		if !ok || np.Type != "File" {
			continue
		}
		// Only support truly file-scoped rules in this fallback: no
		// field constraints (which would require an AST).
		if len(np.Fields) != 0 {
			continue
		}
		if rule.When == nil {
			continue
		}

		caps := Captures{"node": nil}
		if !rule.When.Eval(caps, ctx) {
			e.traceSkipFileRule(
				iter, ctx, rule, "when=false", &reasonsPrinted,
				maxReasonsPerIter,
			)
			continue
		}

		out, changed := rule.Action.Execute(caps, ctx)
		if !changed || out == nil {
			e.traceSkipFileRule(
				iter, ctx, rule, "action=no_change",
				&reasonsPrinted, maxReasonsPerIter,
			)
			continue
		}

		start, endBefore, _ := changedSpan(ctx.Source, out)
		if ctx.editOverlapsForbidden(start, endBefore) {
			e.traceSkipFileRule(
				iter, ctx, rule, "blocked_by_ownership",
				&reasonsPrinted, maxReasonsPerIter,
			)
			continue
		}

		ctx.LastAppliedRule = rule.Name
		ctx.LastAppliedRulePriority = rule.Priority
		ctx.LastAppliedNodeType = "File"
		ctx.LastAppliedNodeStart = 0
		ctx.LastAppliedNodeEnd = len(ctx.Source)

		return out, true
	}

	return nil, false
}

func (e *Engine) traceSkipFileRule(iter int, ctx *Context, rule Rule,
	reason string, reasonsPrinted *int, maxReasons int) {

	if !e.TraceReasons || reasonsPrinted == nil ||
		*reasonsPrinted >= maxReasons {

		return
	}
	*reasonsPrinted++

	fmt.Fprintf(
		os.Stderr, "dsl: stage=%s iter=%d skip rule=%s prio=%d "+
			"node=%s nodeSpan=[%d:%d] @%d:%d reason=%s\n",
		e.StageName, iter, rule.Name, rule.Priority, "File", 0,
		len(ctx.Source), 1, 1, reason,
	)
}

// changedSpan finds a minimal differing span between before and after. It
// returns start offset, end offset in before, and end offset in after.
func changedSpan(before, after []byte) (start, endBefore, endAfter int) {
	minLen := len(before)
	if len(after) < minLen {
		minLen = len(after)
	}

	start = 0
	for start < minLen && before[start] == after[start] {
		start++
	}
	if start == len(before) && start == len(after) {
		return start, start, start
	}

	i := len(before) - 1
	j := len(after) - 1
	for i >= start && j >= start && before[i] == after[j] {
		i--
		j--
	}

	return start, i + 1, j + 1
}

func offsetToLineCol(src []byte, off int) (line, col int) {
	if off < 0 {
		off = 0
	}
	if off > len(src) {
		off = len(src)
	}
	line = 1
	lastNL := -1
	for i := 0; i < off; i++ {
		if src[i] == '\n' {
			line++
			lastNL = i
		}
	}
	col = off - lastNL

	return line, col
}

func snippetForRange(src []byte, start, end int) string {
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = 0
	}
	if start > len(src) {
		start = len(src)
	}
	if end > len(src) {
		end = len(src)
	}

	// Handle insertion-only changes (end == start) by providing a little
	// context.
	if end <= start {
		left := start - 30
		if left < 0 {
			left = 0
		}
		right := start + 30
		if right > len(src) {
			right = len(src)
		}
		start = left
		end = right
	}

	fields := strings.Fields(string(src[start:end]))
	s := strings.Join(fields, " ")
	const maxLen = 120
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}

	return s
}

// applyAtomicMarkers runs through rules that just mark nodes as atomic.
func (e *Engine) applyAtomicMarkers(file *ast.File, ctx *Context) {
	for _, rule := range e.Rules {
		// Only process KeepTogetherAction rules
		if _, ok := rule.Action.(*KeepTogetherAction); !ok {
			continue
		}

		ast.Inspect(file, func(n ast.Node) bool {
			if n == nil {
				return false
			}

			caps, ok := rule.Pattern.Match(n, ctx.Fset)
			if !ok {
				return true
			}

			// Add matched node to captures
			caps["node"] = n

			// Evaluate condition
			if !rule.When.Eval(caps, ctx) {
				return true
			}

			// Execute action (marks as atomic)
			rule.Action.Execute(caps, ctx)

			return true
		})
	}
}

// applyOneRule finds and applies the first matching transforming rule.
func (e *Engine) applyOneRule(iter int, file *ast.File, ctx *Context) ([]byte,
	bool) {

	const maxReasonsPerIter = 60
	reasonsPrinted := 0

	nodes, parentMap := collectNodesAndParents(file)
	ctx.SetParentMap(parentMap)

	sortNodesForOrder(nodes, ctx, e.NodeOrder)

	for _, n := range nodes {
		if ctx.IsAtomic(n) {
			continue
		}

		modified, changed := e.applyRulesForNode(
			iter, ctx, n, &reasonsPrinted, maxReasonsPerIter,
		)
		if changed {
			return modified, true
		}
	}

	return nil, false
}

func (e *Engine) applyRulesForNode(iter int, ctx *Context, n ast.Node,
	reasonsPrinted *int, maxReasons int) ([]byte, bool) {

	for _, rule := range e.Rules {
		if _, ok := rule.Action.(*KeepTogetherAction); ok {
			continue
		}

		caps, ok := rule.Pattern.Match(n, ctx.Fset)
		if !ok {
			continue
		}

		caps["node"] = n

		if !rule.When.Eval(caps, ctx) {
			e.traceSkipRule(
				iter, ctx, rule, n, "when=false",
				reasonsPrinted, maxReasons,
			)
			continue
		}

		modified, actionChanged, ok, reason := e.executeAction(
			rule, caps, ctx,
		)
		if !ok {
			e.traceSkipRule(
				iter, ctx, rule, n, reason, reasonsPrinted,
				maxReasons,
			)
			continue
		}
		if actionChanged {
			return modified, true
		}
	}

	return nil, false
}

func nodeOrderOffset(ctx *Context, n ast.Node) int {
	if ctx == nil || ctx.Fset == nil || n == nil {
		return 0
	}

	// For call expressions, use the '(' position. For selector calls this
	// avoids the "all calls start at the receiver" ambiguity and more
	// closely matches legacy scanner left-to-right behavior.
	if call, ok := n.(*ast.CallExpr); ok && call.Lparen.IsValid() {
		return ctx.Fset.Position(call.Lparen).Offset
	}

	return ctx.Fset.Position(n.Pos()).Offset
}

func nodeSpanOffsets(ctx *Context, n ast.Node) (start, end int) {
	if ctx == nil || ctx.Fset == nil || n == nil {
		return 0, 0
	}
	start = ctx.Fset.Position(n.Pos()).Offset
	end = ctx.Fset.Position(n.End()).Offset
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}

	return start, end
}

func (e *Engine) executeAction(rule Rule, caps Captures, ctx *Context) (
	modified []byte, changed bool, ok bool, reason string) {

	e.recordLastApplied(ctx, rule, caps["node"])

	// Prefer edit-based actions when available.
	if editAction, okCast := rule.Action.(EditAction); okCast {
		return e.executeEditAction(editAction, caps, ctx)
	}

	modified, actionChanged := rule.Action.Execute(caps, ctx)
	if !actionChanged {
		return nil, false, false, "action=no_change"
	}
	start, endBefore, _ := changedSpan(ctx.Source, modified)
	if ctx.editOverlapsForbidden(start, endBefore) {
		return nil, false, false, "blocked_by_ownership"
	}
	if ok, reason := ensureParseable(ctx, modified, "action"); !ok {
		return nil, false, false, reason
	}

	return modified, true, true, ""
}

func ensureParseable(ctx *Context, src []byte,
	actionLabel string) (bool, string) {

	if !ctx.Parseable {
		return true, ""
	}

	fset := token.NewFileSet()
	if _, err := parser.ParseFile(
		fset, "", src, parser.ParseComments,
	); err != nil {

		return false, actionLabel + "=parse_failed=" + err.Error()
	}

	return true, ""
}

func collectNodesAndParents(file *ast.File) ([]ast.Node,
	map[ast.Node]ast.Node) {

	// We need parent links for conditions like parent()/scope(). Capture
	// parents while traversing the AST once to avoid repeated reflection-
	// heavy searches later.
	parentMap := make(map[ast.Node]ast.Node)
	var nodes []ast.Node
	var stack []ast.Node
	ast.Inspect(
		file,
		func(n ast.Node) bool {
			if n == nil {
				if len(stack) > 0 {
					stack = stack[:len(stack)-1]
				}

				return true
			}

			if len(stack) > 0 {
				parentMap[n] = stack[len(stack)-1]
			}
			stack = append(stack, n)
			nodes = append(nodes, n)

			return true
		},
	)

	return nodes, parentMap
}

func sortNodesForOrder(nodes []ast.Node, ctx *Context, order NodeOrder) {
	switch order {
	case NodeOrderSourceOrder:
		// Stable sort keeps deterministic results when multiple nodes
		// have identical "order" offsets.
		sort.SliceStable(
			nodes,
			func(i, j int) bool {
				pi := nodeOrderOffset(ctx, nodes[i])
				pj := nodeOrderOffset(ctx, nodes[j])

				return pi < pj
			},
		)

	case NodeOrderDeepestFirst:
		sort.SliceStable(nodes, func(i, j int) bool {
			si, ei := nodeSpanOffsets(ctx, nodes[i])
			sj, ej := nodeSpanOffsets(ctx, nodes[j])
			li := ei - si
			lj := ej - sj
			if li != lj {
				return li < lj
			}
			// Tie-break by source order for determinism.
			pi := nodeOrderOffset(ctx, nodes[i])
			pj := nodeOrderOffset(ctx, nodes[j])

			return pi < pj
		})

	default:
	}
}

func (e *Engine) traceSkipRule(iter int, ctx *Context, rule Rule, n ast.Node,
	reason string, reasonsPrinted *int, maxReasons int) {

	if !e.TraceReasons || reasonsPrinted == nil ||
		*reasonsPrinted >= maxReasons {

		return
	}
	*reasonsPrinted++

	start, end := nodeSpanOffsets(ctx, n)
	line, col := offsetToLineCol(ctx.Source, start)
	fmt.Fprintf(
		os.Stderr, "dsl: stage=%s iter=%d skip rule=%s prio=%d "+
			"node=%T nodeSpan=[%d:%d] @%d:%d reason=%s "+
			"snippet=%q\n", e.StageName, iter, rule.Name,
		rule.Priority, n, start, end, line, col, reason,
		snippetForRange(ctx.Source, start, end),
	)
}

func (e *Engine) recordLastApplied(ctx *Context, rule Rule, node any) {
	if ctx == nil || ctx.Fset == nil || node == nil {
		return
	}

	// Offsets are clamped to the current source bytes so trace/debug output
	// never panics on partially-invalid AST spans.
	n, ok := node.(ast.Node)
	if !ok || n == nil {
		return
	}

	pos := ctx.Fset.Position(n.Pos()).Offset
	end := ctx.Fset.Position(n.End()).Offset
	if pos < 0 {
		pos = 0
	}
	if end < pos {
		end = pos
	}
	if pos > len(ctx.Source) {
		pos = len(ctx.Source)
	}
	if end > len(ctx.Source) {
		end = len(ctx.Source)
	}

	ctx.LastAppliedRule = rule.Name
	ctx.LastAppliedRulePriority = rule.Priority
	ctx.LastAppliedNodeType = fmt.Sprintf("%T", n)
	ctx.LastAppliedNodeStart = pos
	ctx.LastAppliedNodeEnd = end
}

func (e *Engine) executeEditAction(editAction EditAction, caps Captures,
	ctx *Context) (modified []byte, changed bool, ok bool, reason string) {

	edits, changedEdits, err := editAction.ExecuteEdits(caps, ctx)
	if err != nil {
		return nil, false, false, "edit_action_error=" + err.Error()
	}
	if !changedEdits {
		return nil, false, false, "edit_action=no_edits"
	}

	for _, edit := range edits {
		if ctx.editOverlapsForbidden(edit.Start, edit.End) {
			return nil, false, false, "blocked_by_ownership"
		}
	}

	applied, err := ApplyEdits(ctx.Source, edits)
	if err != nil {
		return nil, false, false, "edit_action=apply_edits_error=" + err.Error()
	}

	// Never accept a transformation that produces syntactically invalid Go
	// when the input was parseable. This ensures the DSL engine won't
	// "brick" a file even if a rule is imperfect or interacts badly with
	// semicolon insertion.
	//
	// However, some legacy fixtures are intentionally unparseable and are
	// still expected to be formatted by scanner-based rules. In those
	// cases, we allow transformations that keep the file unparseable.
	if ok, reason := ensureParseable(ctx, applied, "edit_action"); !ok {
		return nil, false, false, reason
	}

	return applied, true, true, ""
}

// FormatFile is a convenience method that reads, formats, and returns source.
func (e *Engine) FormatFile(src []byte) []byte {
	result, _ := e.Format(src)

	return result
}
