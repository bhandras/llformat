package dsl

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
)

// Engine executes formatting rules.
type Engine struct {
	Rules         []Rule
	ColumnLimit   int
	TabStop       int
	MaxIterations int
	Trace         bool // Enable trace logging
	NodeOrder     NodeOrder
}

// NewEngine creates a rule engine with default settings.
func NewEngine(rules []Rule) *Engine {
	// Sort rules by priority (descending - higher priority first).
	// Keep relative order stable for equal priority to avoid nondeterminism.
	sorted := make([]Rule, len(rules))
	copy(sorted, rules)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Priority > sorted[j].Priority
	})

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

	for iter := 0; iter < e.MaxIterations; iter++ {
		// Parse current source
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "", result, parser.ParseComments)
		if err != nil {
			// If we can't parse, we can still try to apply file-scoped
			// scan/delegation rules that don't require an AST (e.g. legacy
			// comment formatting).
			ctx := NewContext(token.NewFileSet(), result, e.ColumnLimit, e.TabStop)
			modified, changed := e.applyOneFileRuleWithoutAST(ctx)
			if !changed {
				return result, nil
			}

			if e.Trace {
				start, endBefore, endAfter := changedSpan(ctx.Source, modified)
				line, col := offsetToLineCol(ctx.Source, start)
				fmt.Fprintf(os.Stderr, "dsl: iter=%d rule=%s prio=%d node=%s nodeSpan=[%d:%d] editSpan=[%d:%d]->[%d:%d] @%d:%d snippet=%q\n",
					iter+1,
					ctx.LastAppliedRule,
					ctx.LastAppliedRulePriority,
					ctx.LastAppliedNodeType,
					ctx.LastAppliedNodeStart,
					ctx.LastAppliedNodeEnd,
					start,
					endBefore,
					start,
					endAfter,
					line,
					col,
					snippetForRange(ctx.Source, start, endBefore),
				)
			}

			result = modified
			continue
		}

		ctx := NewContext(fset, result, e.ColumnLimit, e.TabStop)

		// First pass: apply atomic markers (high priority keep_together rules)
		e.applyAtomicMarkers(file, ctx)

		// Second pass: try to apply one transforming rule
		modified, changed := e.applyOneRule(file, ctx)
		if !changed {
			break
		}

		if e.Trace {
			start, endBefore, endAfter := changedSpan(ctx.Source, modified)
			line, col := offsetToLineCol(ctx.Source, start)
			fmt.Fprintf(os.Stderr, "dsl: iter=%d rule=%s prio=%d node=%s nodeSpan=[%d:%d] editSpan=[%d:%d]->[%d:%d] @%d:%d snippet=%q\n",
				iter+1,
				ctx.LastAppliedRule,
				ctx.LastAppliedRulePriority,
				ctx.LastAppliedNodeType,
				ctx.LastAppliedNodeStart,
				ctx.LastAppliedNodeEnd,
				start,
				endBefore,
				start,
				endAfter,
				line,
				col,
				snippetForRange(ctx.Source, start, endBefore),
			)
		}

		// Keep the edited source as-is and rely on the outer pipeline to run
		// gofmt once at the end. Running gofmt here would reformat unrelated
		// code and violate llformat's "only touch targeted regions" goal.
		result = modified
	}

	return result, nil
}

func (e *Engine) applyOneFileRuleWithoutAST(ctx *Context) ([]byte, bool) {
	for _, rule := range e.Rules {
		np, ok := rule.Pattern.(*NodePattern)
		if !ok || np.Type != "File" {
			continue
		}
		// Only support truly file-scoped rules in this fallback: no field
		// constraints (which would require an AST).
		if len(np.Fields) != 0 {
			continue
		}
		if rule.When == nil {
			continue
		}

		caps := Captures{"node": nil}
		if !rule.When.Eval(caps, ctx) {
			continue
		}

		out, changed := rule.Action.Execute(caps, ctx)
		if !changed || out == nil {
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

// changedSpan finds a minimal differing span between before and after.
// It returns start offset, end offset in before, and end offset in after.
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

	// Handle insertion-only changes (end == start) by providing a little context.
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
func (e *Engine) applyOneRule(file *ast.File, ctx *Context) ([]byte, bool) {
	var result []byte
	changed := false

	// Build parent map and collect nodes
	parentMap := make(map[ast.Node]ast.Node)
	var nodes []ast.Node
	var stack []ast.Node
	ast.Inspect(file, func(n ast.Node) bool {
		if n != nil {
			// Set parent for this node (parent is top of stack)
			if len(stack) > 0 {
				parentMap[n] = stack[len(stack)-1]
			}
			stack = append(stack, n)
			nodes = append(nodes, n)
		} else {
			// Pop from stack when leaving a node
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
		return true
	})
	ctx.SetParentMap(parentMap)

	// Try each node
	if e.NodeOrder == NodeOrderSourceOrder {
		sort.SliceStable(nodes, func(i, j int) bool {
			pi := nodeOrderOffset(ctx, nodes[i])
			pj := nodeOrderOffset(ctx, nodes[j])
			return pi < pj
		})
	}
	if e.NodeOrder == NodeOrderDeepestFirst {
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
	}

	for _, n := range nodes {
		if changed {
			break
		}

		// Skip nodes marked as atomic
		if ctx.IsAtomic(n) {
			continue
		}

		// Try each rule
		for _, rule := range e.Rules {
			// Skip keep_together rules (handled in first pass)
			if _, ok := rule.Action.(*KeepTogetherAction); ok {
				continue
			}

			caps, ok := rule.Pattern.Match(n, ctx.Fset)
			if !ok {
				continue
			}

			// Add matched node to captures
			caps["node"] = n

			// Evaluate condition
			if !rule.When.Eval(caps, ctx) {
				continue
			}

			modified, actionChanged, ok := e.executeAction(rule, caps, ctx)
			if !ok {
				continue
			}
			if actionChanged {
				result = modified
				changed = true
				break
			}
		}
	}

	return result, changed
}

func nodeOrderOffset(ctx *Context, n ast.Node) int {
	if ctx == nil || ctx.Fset == nil || n == nil {
		return 0
	}

	// For call expressions, use the '(' position. For selector calls this avoids
	// the "all calls start at the receiver" ambiguity and more closely matches
	// legacy scanner left-to-right behavior.
	if call, ok := n.(*ast.CallExpr); ok {
		if call.Lparen.IsValid() {
			return ctx.Fset.Position(call.Lparen).Offset
		}
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

func (e *Engine) executeAction(rule Rule, caps Captures, ctx *Context) (modified []byte, changed bool, ok bool) {
	n, _ := caps["node"]
	if ctx != nil && n != nil {
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

	// Prefer edit-based actions when available.
	if editAction, okCast := rule.Action.(EditAction); okCast {
		edits, changedEdits, err := editAction.ExecuteEdits(caps, ctx)
		if err != nil || !changedEdits {
			return nil, false, false
		}
		applied, err := ApplyEdits(ctx.Source, edits)
		if err != nil {
			return nil, false, false
		}
		// Never accept a transformation that produces syntactically invalid Go.
		// This ensures the DSL engine won't “brick” a file even if a rule is
		// imperfect or interacts badly with semicolon insertion.
		fset := token.NewFileSet()
		if _, err := parser.ParseFile(fset, "", applied, parser.ParseComments); err != nil {
			return nil, false, false
		}
		return applied, true, true
	}

	modified, actionChanged := rule.Action.Execute(caps, ctx)
	if !actionChanged {
		return nil, false, false
	}
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "", modified, parser.ParseComments); err != nil {
		return nil, false, false
	}
	return modified, true, true
}

// FormatFile is a convenience method that reads, formats, and returns source.
func (e *Engine) FormatFile(src []byte) []byte {
	result, _ := e.Format(src)
	return result
}
