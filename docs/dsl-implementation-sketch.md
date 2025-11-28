# DSL Implementation Sketch

This document provides a concrete implementation approach for the formatting DSL in Go.

## Directory Structure

```
llformat/
├── dsl/
│   ├── ast.go          # DSL AST types
│   ├── parser.go       # DSL parser (optional, can start with Go DSL)
│   ├── pattern.go      # Pattern matching against Go AST
│   ├── condition.go    # Condition evaluation
│   ├── action.go       # Action execution
│   ├── engine.go       # Rule engine (matching + execution)
│   └── builtin.go      # Built-in predicates and actions
├── rules/
│   └── default.go      # Default rules encoded in Go
```

## Phase 1: Internal Go DSL

Instead of parsing a text DSL, encode rules directly in Go. This is simpler to start and provides type safety.

### ast.go - DSL Types

```go
package dsl

import (
    "go/ast"
    "go/token"
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

// Context provides formatting context to conditions and actions.
type Context struct {
    Fset        *token.FileSet
    Source      []byte
    ColumnLimit int
    TabStop     int
}
```

### pattern.go - Pattern Matching

```go
package dsl

import (
    "go/ast"
    "go/token"
)

// NodePattern matches a specific AST node type with field constraints.
type NodePattern struct {
    Type   string                 // "CallExpr", "BinaryExpr", etc.
    Fields map[string]FieldMatch
}

// FieldMatch specifies how to match a field.
type FieldMatch struct {
    Capture    string   // Variable name to capture (e.g., "$fn")
    SubPattern Pattern  // Nested pattern to match
    Literal    string   // Literal value to match (for operators)
    OneOf      []string // Match any of these values
}

// Wildcard matches any node.
type Wildcard struct{}

func (w Wildcard) Match(n ast.Node, fset *token.FileSet) (Captures, bool) {
    return Captures{}, n != nil
}

func (p *NodePattern) Match(n ast.Node, fset *token.FileSet) (Captures, bool) {
    // Check node type
    if !p.matchType(n) {
        return nil, false
    }

    caps := make(Captures)

    // Match each field constraint
    for fieldName, fm := range p.Fields {
        child := getField(n, fieldName)

        // Capture if requested
        if fm.Capture != "" {
            caps[fm.Capture] = child
        }

        // Check literal match
        if fm.Literal != "" {
            if !matchLiteral(child, fm.Literal) {
                return nil, false
            }
        }

        // Check OneOf match
        if len(fm.OneOf) > 0 {
            matched := false
            for _, lit := range fm.OneOf {
                if matchLiteral(child, lit) {
                    matched = true
                    break
                }
            }
            if !matched {
                return nil, false
            }
        }

        // Recurse into sub-pattern
        if fm.SubPattern != nil {
            childCaps, ok := fm.SubPattern.Match(child, fset)
            if !ok {
                return nil, false
            }
            mergeCaps(caps, childCaps)
        }
    }

    return caps, true
}

func (p *NodePattern) matchType(n ast.Node) bool {
    switch p.Type {
    case "CallExpr":
        _, ok := n.(*ast.CallExpr)
        return ok
    case "BinaryExpr":
        _, ok := n.(*ast.BinaryExpr)
        return ok
    case "AssignStmt":
        _, ok := n.(*ast.AssignStmt)
        return ok
    case "IfStmt":
        _, ok := n.(*ast.IfStmt)
        return ok
    case "ReturnStmt":
        _, ok := n.(*ast.ReturnStmt)
        return ok
    case "CaseClause":
        _, ok := n.(*ast.CaseClause)
        return ok
    case "BasicLit":
        _, ok := n.(*ast.BasicLit)
        return ok
    case "Ident":
        _, ok := n.(*ast.Ident)
        return ok
    // ... more types
    default:
        return false
    }
}

func getField(n ast.Node, name string) ast.Node {
    switch node := n.(type) {
    case *ast.CallExpr:
        switch name {
        case "Fun", "func":
            return node.Fun
        case "Args", "args":
            // Return as a pseudo-node or handle specially
            return &argListNode{args: node.Args}
        }
    case *ast.BinaryExpr:
        switch name {
        case "X", "left":
            return node.X
        case "Y", "right":
            return node.Y
        case "Op", "op":
            return &opNode{op: node.Op}
        }
    case *ast.AssignStmt:
        switch name {
        case "Lhs", "lhs":
            if len(node.Lhs) > 0 {
                return node.Lhs[0]
            }
        case "Rhs", "rhs":
            if len(node.Rhs) > 0 {
                return node.Rhs[0]
            }
        }
    case *ast.IfStmt:
        switch name {
        case "Cond", "cond":
            return node.Cond
        case "Body", "body":
            return node.Body
        }
    // ... more node types
    }
    return nil
}

func matchLiteral(n ast.Node, lit string) bool {
    switch node := n.(type) {
    case *opNode:
        return node.op.String() == lit
    case *ast.Ident:
        return node.Name == lit
    case *ast.BasicLit:
        return node.Value == lit
    }
    return false
}

// Helper types for fields that aren't ast.Node
type opNode struct {
    op token.Token
}

func (o *opNode) Pos() token.Pos { return token.NoPos }
func (o *opNode) End() token.Pos { return token.NoPos }

type argListNode struct {
    args []ast.Expr
}

func (a *argListNode) Pos() token.Pos { return token.NoPos }
func (a *argListNode) End() token.Pos { return token.NoPos }

func mergeCaps(dst, src Captures) {
    for k, v := range src {
        dst[k] = v
    }
}
```

### condition.go - Condition Evaluation

```go
package dsl

import (
    "go/ast"
)

// LineWidthCond checks if a node's line exceeds the limit.
type LineWidthCond struct {
    Target string // Capture name or "$node"
    Op     string // ">", "<", ">=", etc.
    Value  int
}

func (c *LineWidthCond) Eval(caps Captures, ctx *Context) bool {
    node := resolveTarget(caps, c.Target)
    if node == nil {
        return false
    }

    width := ctx.lineWidth(node)

    switch c.Op {
    case ">":
        return width > c.Value
    case ">=":
        return width >= c.Value
    case "<":
        return width < c.Value
    case "<=":
        return width <= c.Value
    case "==":
        return width == c.Value
    }
    return false
}

// IsSimpleLiteralCond checks if a node is a simple literal.
type IsSimpleLiteralCond struct {
    Target string
}

func (c *IsSimpleLiteralCond) Eval(caps Captures, ctx *Context) bool {
    node := resolveTarget(caps, c.Target)
    return isSimpleLiteral(node)
}

func isSimpleLiteral(n ast.Node) bool {
    switch node := n.(type) {
    case *ast.BasicLit:
        return true // Numbers, strings (could refine)
    case *ast.Ident:
        return node.Name == "true" || node.Name == "false" || node.Name == "nil"
    }
    return false
}

// HasFunctionCallCond checks if node contains a function call.
type HasFunctionCallCond struct {
    Target string
}

func (c *HasFunctionCallCond) Eval(caps Captures, ctx *Context) bool {
    node := resolveTarget(caps, c.Target)
    return containsCallExpr(node)
}

func containsCallExpr(n ast.Node) bool {
    found := false
    ast.Inspect(n, func(node ast.Node) bool {
        if _, ok := node.(*ast.CallExpr); ok {
            found = true
            return false
        }
        return true
    })
    return found
}

// AndCond combines conditions with AND.
type AndCond struct {
    Conds []Condition
}

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

func (c *NotCond) Eval(caps Captures, ctx *Context) bool {
    return !c.Cond.Eval(caps, ctx)
}

// TrueCond always returns true (no condition).
type TrueCond struct{}

func (c TrueCond) Eval(caps Captures, ctx *Context) bool {
    return true
}

func resolveTarget(caps Captures, target string) ast.Node {
    if target == "$node" {
        // Special: refers to the matched node itself
        // This would need to be set during matching
        return caps["$node"]
    }
    if len(target) > 0 && target[0] == '$' {
        return caps[target[1:]]
    }
    return caps[target]
}
```

### action.go - Action Execution

```go
package dsl

import (
    "bytes"
    "go/ast"
    "go/format"
    "go/printer"
)

// KeepTogetherAction marks a node as atomic (won't be broken).
type KeepTogetherAction struct {
    Target string
}

func (a *KeepTogetherAction) Execute(caps Captures, ctx *Context) ([]byte, bool) {
    // Mark the node as atomic in context
    // Actual effect: prevent other rules from breaking it
    node := resolveTarget(caps, a.Target)
    ctx.markAtomic(node)
    return nil, false // No source change, just marking
}

// ReflowAction reformats a function call.
type ReflowAction struct {
    Target   string
    Strategy string // "left-pack", "one-per-line", "adaptive"
}

func (a *ReflowAction) Execute(caps Captures, ctx *Context) ([]byte, bool) {
    node := resolveTarget(caps, a.Target)
    call, ok := node.(*ast.CallExpr)
    if !ok {
        return nil, false
    }

    // Get current source for this node
    start := ctx.Fset.Position(call.Pos()).Offset
    end := ctx.Fset.Position(call.End()).Offset
    original := ctx.Source[start:end]

    // Check if it already fits
    if ctx.visualWidth(string(original)) <= ctx.ColumnLimit {
        return nil, false
    }

    // Apply reflow strategy
    var formatted string
    switch a.Strategy {
    case "one-per-line":
        formatted = ctx.formatOnePerLine(call)
    case "left-pack":
        formatted = ctx.formatLeftPack(call)
    case "adaptive":
        formatted = ctx.formatAdaptive(call)
    default:
        formatted = ctx.formatOnePerLine(call)
    }

    // Build result by replacing the call in source
    var result bytes.Buffer
    result.Write(ctx.Source[:start])
    result.WriteString(formatted)
    result.Write(ctx.Source[end:])

    return result.Bytes(), true
}

// BreakAfterAction inserts a line break after a node.
type BreakAfterAction struct {
    Target string
}

func (a *BreakAfterAction) Execute(caps Captures, ctx *Context) ([]byte, bool) {
    node := resolveTarget(caps, a.Target)
    if node == nil {
        return nil, false
    }

    end := ctx.Fset.Position(node.End()).Offset
    indent := ctx.indentAt(node)

    var result bytes.Buffer
    result.Write(ctx.Source[:end])
    result.WriteString("\n")
    result.WriteString(indent)
    result.WriteString("\t") // continuation indent
    // Skip whitespace after the break point
    i := end
    for i < len(ctx.Source) && (ctx.Source[i] == ' ' || ctx.Source[i] == '\t') {
        i++
    }
    result.Write(ctx.Source[i:])

    return result.Bytes(), true
}

// TryElseAction tries first action, falls back to second.
type TryElseAction struct {
    Try  Action
    Else Action
}

func (a *TryElseAction) Execute(caps Captures, ctx *Context) ([]byte, bool) {
    result, changed := a.Try.Execute(caps, ctx)
    if changed {
        return result, true
    }
    return a.Else.Execute(caps, ctx)
}

// WrapBinaryAction breaks a binary expression at operator boundaries.
type WrapBinaryAction struct {
    Target     string
    BreakAfter bool // Break after operator (Go style) vs before
}

func (a *WrapBinaryAction) Execute(caps Captures, ctx *Context) ([]byte, bool) {
    node := resolveTarget(caps, a.Target)
    binExpr, ok := node.(*ast.BinaryExpr)
    if !ok {
        return nil, false
    }

    // Find the operator position
    opPos := ctx.Fset.Position(binExpr.OpPos).Offset

    // Get surrounding source
    start := ctx.Fset.Position(binExpr.Pos()).Offset
    end := ctx.Fset.Position(binExpr.End()).Offset

    if a.BreakAfter {
        // Break after operator: "a && \n\t b"
        opEnd := opPos + len(binExpr.Op.String())

        var result bytes.Buffer
        result.Write(ctx.Source[:opEnd])
        result.WriteString("\n")
        result.WriteString(ctx.indentAt(binExpr))
        result.WriteString("\t")

        // Skip whitespace after operator
        i := opEnd
        for i < len(ctx.Source) && (ctx.Source[i] == ' ' || ctx.Source[i] == '\t') {
            i++
        }
        result.Write(ctx.Source[i:])

        return result.Bytes(), true
    }

    // Break before operator (not Go style, but for reference)
    var result bytes.Buffer
    result.Write(ctx.Source[:opPos])
    result.WriteString("\n")
    result.WriteString(ctx.indentAt(binExpr))
    result.WriteString("\t")
    result.Write(ctx.Source[opPos:])

    return result.Bytes(), true
}
```

### engine.go - Rule Engine

```go
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
}

// NewEngine creates a rule engine with default settings.
func NewEngine(rules []Rule) *Engine {
    // Sort rules by priority (descending)
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
            return result, err
        }

        ctx := &Context{
            Fset:        fset,
            Source:      result,
            ColumnLimit: e.ColumnLimit,
            TabStop:     e.TabStop,
            atomicNodes: make(map[ast.Node]bool),
        }

        // Try to apply one rule
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

// applyOneRule finds and applies the first matching rule.
func (e *Engine) applyOneRule(file *ast.File, ctx *Context) ([]byte, bool) {
    var result []byte
    changed := false

    // Walk AST in post-order (children before parents)
    ast.Inspect(file, func(n ast.Node) bool {
        if n == nil || changed {
            return false
        }

        // Skip nodes marked as atomic
        if ctx.isAtomic(n) {
            return true
        }

        // Try each rule
        for _, rule := range e.Rules {
            caps, ok := rule.Pattern.Match(n, ctx.Fset)
            if !ok {
                continue
            }

            // Add $node to captures
            caps["$node"] = n

            // Evaluate condition
            if !rule.When.Eval(caps, ctx) {
                continue
            }

            // Execute action
            modified, actionChanged := rule.Action.Execute(caps, ctx)
            if actionChanged {
                result = modified
                changed = true
                return false // Stop walking
            }
        }

        return true
    })

    return result, changed
}

// Context methods

func (ctx *Context) lineWidth(n ast.Node) int {
    // Get the source line containing this node
    pos := ctx.Fset.Position(n.Pos())
    lineStart := pos.Offset - pos.Column + 1

    // Find end of line
    lineEnd := lineStart
    for lineEnd < len(ctx.Source) && ctx.Source[lineEnd] != '\n' {
        lineEnd++
    }

    line := string(ctx.Source[lineStart:lineEnd])
    return visualLen(line, ctx.TabStop)
}

func (ctx *Context) visualWidth(s string) int {
    return visualLen(s, ctx.TabStop)
}

func (ctx *Context) indentAt(n ast.Node) string {
    pos := ctx.Fset.Position(n.Pos())
    lineStart := pos.Offset - pos.Column + 1

    var indent []byte
    for i := lineStart; i < pos.Offset; i++ {
        c := ctx.Source[i]
        if c == ' ' || c == '\t' {
            indent = append(indent, c)
        } else {
            break
        }
    }
    return string(indent)
}

func (ctx *Context) markAtomic(n ast.Node) {
    ctx.atomicNodes[n] = true
}

func (ctx *Context) isAtomic(n ast.Node) bool {
    return ctx.atomicNodes[n]
}

func visualLen(s string, tabStop int) int {
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
```

### rules/default.go - Default Rules

```go
package rules

import "github.com/lightninglabs/llformat/dsl"

// DefaultRules returns the standard formatting rules.
func DefaultRules() []dsl.Rule {
    return []dsl.Rule{
        // Rule 1: Never break simple comparisons
        {
            Name: "keep_simple_comparison",
            Pattern: &dsl.NodePattern{
                Type: "BinaryExpr",
                Fields: map[string]dsl.FieldMatch{
                    "op": {OneOf: []string{"==", "!=", "<", ">", "<=", ">="}},
                    "right": {Capture: "r"},
                },
            },
            When: &dsl.IsSimpleLiteralCond{Target: "$r"},
            Priority: 100,
            Action: &dsl.KeepTogetherAction{Target: "$node"},
        },

        // Rule 2: Reflow assignment with long function call
        {
            Name: "assignment_with_long_call",
            Pattern: &dsl.NodePattern{
                Type: "AssignStmt",
                Fields: map[string]dsl.FieldMatch{
                    "lhs": {Capture: "var"},
                    "rhs": {
                        Capture: "rhs",
                        SubPattern: &dsl.NodePattern{Type: "CallExpr"},
                    },
                },
            },
            When: &dsl.LineWidthCond{
                Target: "$node",
                Op:     ">",
                Value:  80,
            },
            Priority: 50,
            Action: &dsl.ReflowAction{
                Target:   "$rhs",
                Strategy: "one-per-line",
            },
        },

        // Rule 3: If condition with call and simple comparison
        {
            Name: "if_call_comparison",
            Pattern: &dsl.NodePattern{
                Type: "IfStmt",
                Fields: map[string]dsl.FieldMatch{
                    "cond": {
                        Capture: "cond",
                        SubPattern: &dsl.NodePattern{
                            Type: "BinaryExpr",
                            Fields: map[string]dsl.FieldMatch{
                                "left": {
                                    Capture: "call",
                                    SubPattern: &dsl.NodePattern{Type: "CallExpr"},
                                },
                                "op": {OneOf: []string{"==", "!=", "<", ">", "<=", ">="}},
                                "right": {Capture: "r"},
                            },
                        },
                    },
                },
            },
            When: &dsl.AndCond{
                Conds: []dsl.Condition{
                    &dsl.LineWidthCond{Target: "$cond", Op: ">", Value: 80},
                    &dsl.IsSimpleLiteralCond{Target: "$r"},
                },
            },
            Priority: 60,
            Action: &dsl.ReflowAction{
                Target:   "$call",
                Strategy: "one-per-line",
            },
        },

        // Rule 4: Long logical chain without calls
        {
            Name: "long_logical_chain",
            Pattern: &dsl.NodePattern{
                Type: "BinaryExpr",
                Fields: map[string]dsl.FieldMatch{
                    "op": {OneOf: []string{"&&", "||"}},
                },
            },
            When: &dsl.AndCond{
                Conds: []dsl.Condition{
                    &dsl.LineWidthCond{Target: "$node", Op: ">", Value: 80},
                    &dsl.NotCond{Cond: &dsl.HasFunctionCallCond{Target: "$node"}},
                },
            },
            Priority: 30,
            Action: &dsl.WrapBinaryAction{
                Target:     "$node",
                BreakAfter: true,
            },
        },

        // Rule 5: Long logical chain with calls - try reflow first
        {
            Name: "logical_chain_with_call",
            Pattern: &dsl.NodePattern{
                Type: "BinaryExpr",
                Fields: map[string]dsl.FieldMatch{
                    "op": {OneOf: []string{"&&", "||"}},
                },
            },
            When: &dsl.AndCond{
                Conds: []dsl.Condition{
                    &dsl.LineWidthCond{Target: "$node", Op: ">", Value: 80},
                    &dsl.HasFunctionCallCond{Target: "$node"},
                },
            },
            Priority: 40,
            Action: &dsl.TryElseAction{
                Try:  &dsl.ReflowNestedCallsAction{Target: "$node"},
                Else: &dsl.WrapBinaryAction{Target: "$node", BreakAfter: true},
            },
        },

        // Rule 6: Long case clause
        {
            Name: "long_case_clause",
            Pattern: &dsl.NodePattern{
                Type: "CaseClause",
                Fields: map[string]dsl.FieldMatch{
                    "list": {Capture: "vals"},
                },
            },
            When: &dsl.LineWidthCond{Target: "$node", Op: ">", Value: 80},
            Priority: 35,
            Action: &dsl.WrapListAction{
                Target:     "$vals",
                BreakAfter: ",",
            },
        },
    }
}
```

## Usage Example

```go
package main

import (
    "fmt"
    "io/ioutil"

    "github.com/lightninglabs/llformat/dsl"
    "github.com/lightninglabs/llformat/rules"
)

func main() {
    src, _ := ioutil.ReadFile("input.go")

    engine := dsl.NewEngine(rules.DefaultRules())
    engine.ColumnLimit = 80

    result, err := engine.Format(src)
    if err != nil {
        panic(err)
    }

    fmt.Println(string(result))
}
```

## Migration Strategy

### Step 1: Parallel Implementation

Keep existing formatters working, add DSL engine as optional:

```go
// cmd/llformat/main.go
if *useDSL {
    engine := dsl.NewEngine(rules.DefaultRules())
    result, _ = engine.Format(src)
} else {
    // Existing pipeline
    result = pipeline.Format(src)
}
```

### Step 2: Rule Parity

Encode all existing logic as DSL rules. Run both and compare:

```go
func TestParity(t *testing.T) {
    files := findTestFiles()
    for _, f := range files {
        src, _ := ioutil.ReadFile(f)

        oldResult := oldPipeline.Format(src)
        newResult := dslEngine.Format(src)

        if !bytes.Equal(oldResult, newResult) {
            t.Errorf("Mismatch for %s", f)
        }
    }
}
```

### Step 3: Switch Over

Once parity is confirmed, remove old formatters.

### Step 4: External Rules (Optional)

Add parser for `.llformat` rule files:

```go
rules, err := dsl.ParseFile("custom-rules.llf")
engine := dsl.NewEngine(append(rules.DefaultRules(), rules...))
```

## Debugging Support

### Trace Mode

```go
engine := dsl.NewEngine(rules)
engine.Trace = true  // Print rule applications

// Output:
// [line 10] matched rule "keep_simple_comparison" on BinaryExpr
// [line 15] matched rule "assignment_with_long_call" on AssignStmt
//           action: reflow($rhs, one-per-line) -> changed
```

### Rule Testing

```go
func TestKeepSimpleComparison(t *testing.T) {
    rule := rules.DefaultRules()[0] // keep_simple_comparison

    // Parse test expression
    expr, _ := parser.ParseExpr("x > 0")

    caps, ok := rule.Pattern.Match(expr, fset)
    require.True(t, ok)

    ctx := &dsl.Context{ColumnLimit: 80}
    require.True(t, rule.When.Eval(caps, ctx))
}
```

## Performance Considerations

1. **Pattern Caching**: Memoize pattern match results per node
2. **Early Exit**: Stop matching once a rule succeeds
3. **Incremental Parsing**: Consider tree-sitter for incremental updates
4. **Parallel Matching**: Match multiple rules concurrently (read-only)

## Conclusion

This implementation sketch shows how the formatting DSL can be built incrementally:

1. **Start with Go DSL**: Rules as Go structs (type-safe, no parser needed)
2. **Build core matching**: Pattern matching against Go AST
3. **Add conditions and actions**: Predicates and transformations
4. **Integrate with engine**: Fixpoint iteration with priority ordering
5. **Optionally add text DSL**: Parser for external rule files

The key insight is that we can start simple (Go DSL) and evolve toward a full external DSL if the approach proves valuable.
