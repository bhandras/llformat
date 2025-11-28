package dsl

import (
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
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
	// Sort rules by priority (descending - higher priority first)
	sorted := make([]Rule, len(rules))
	copy(sorted, rules)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority > sorted[j].Priority
	})

	return &Engine{
		Rules:         sorted,
		ColumnLimit:   80,
		TabStop:       8,
		MaxIterations: 20,
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

	// Collect nodes in post-order (children before parents)
	var nodes []ast.Node
	ast.Inspect(file, func(n ast.Node) bool {
		if n != nil {
			nodes = append(nodes, n)
		}
		return true
	})

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
