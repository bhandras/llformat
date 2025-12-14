package dsl

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"sort"
)

// Engine executes formatting rules.
type Engine struct {
	Rules         []Rule
	ColumnLimit   int
	TabStop       int
	MaxIterations int
	Trace         bool // Enable trace logging
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
			// If we can't parse, return what we have
			return result, nil
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
			fmt.Fprintf(os.Stderr, "dsl: iter=%d applied %s\n", iter+1, ctx.LastAppliedRule)
		}

		// Run gofmt to normalize
		formatted, err := format.Source(modified)
		if err != nil {
			result = modified
		} else {
			result = formatted
		}
	}

	return result, nil
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

			// Execute action
			modified, actionChanged := rule.Action.Execute(caps, ctx)
			if actionChanged {
				if ctx != nil {
					ctx.LastAppliedRule = rule.Name
				}
				result = modified
				changed = true
				break
			}
		}
	}

	return result, changed
}

// FormatFile is a convenience method that reads, formats, and returns source.
func (e *Engine) FormatFile(src []byte) []byte {
	result, _ := e.Format(src)
	return result
}
