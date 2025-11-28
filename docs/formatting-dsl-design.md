# Formatting DSL Design for llformat

## Overview

This document proposes a domain-specific language (DSL) for declaring formatting rules in llformat. The goal is to make formatting behavior more declarative, composable, and easier to reason about.

## Design Philosophy

### Core Principles

1. **Pattern-First**: Rules match AST patterns, not strings
2. **Prioritized Actions**: When multiple strategies could apply, try them in order
3. **Composition over Mutation**: Build formatted output, don't mutate source
4. **Width-Aware**: Decisions depend on whether content fits within column limit

### Key Insight: "Compressibility"

Not all expressions are equal when it comes to formatting:

```
# Function calls are "compressible" - can be expanded to multiline
someFunc(a, b, c, d)  →  someFunc(
                            a, b, c, d,
                         )

# Simple comparisons are "atomic" - should not be split
x > 0    →  NEVER split this

# Binary expressions with function calls are "partially compressible"
len(foo) > 0  →  Can reformat len(foo) but keep "> 0" together
```

## Grammar

```ebnf
program     = rule* ;

rule        = "rule" IDENT "{" rule_body "}" ;

rule_body   = pattern_clause
            | when_clause?
            | priority_clause?
            | action_clause ;

pattern_clause  = "pattern" ":" pattern ;
when_clause     = "when" ":" condition ;
priority_clause = "priority" ":" NUMBER ;
action_clause   = "action" ":" action_list ;

pattern     = node_pattern | "_" ;
node_pattern = NODE_TYPE "(" field_list? ")" ;
field_list  = field ("," field)* ;
field       = IDENT ":" (pattern | "$" IDENT | literal) ;

condition   = expr (("&&" | "||") expr)* ;
expr        = term (comp_op term)? ;
term        = "$" IDENT "." IDENT
            | IDENT "(" arg_list ")"
            | NUMBER
            | STRING ;
comp_op     = ">" | "<" | ">=" | "<=" | "==" | "!=" ;

action_list = action_step+ ;
action_step = "try" ":" action ("else" ":" action)?
            | action ;
action      = IDENT "(" arg_list? ")" ;
arg_list    = arg ("," arg)* ;
arg         = "$" IDENT | literal | IDENT ":" literal ;

literal     = STRING | NUMBER | "true" | "false" ;

NODE_TYPE   = "call_expr" | "binary_expr" | "if_stmt" | "assign_stmt"
            | "return_stmt" | "case_clause" | "func_decl" | ... ;

IDENT       = [a-zA-Z_][a-zA-Z0-9_]* ;
NUMBER      = [0-9]+ ;
STRING      = '"' [^"]* '"' ;
```

## Pattern Matching

### Node Types (Go AST mapped)

| DSL Node Type    | Go AST Type           | Description                    |
|------------------|-----------------------|--------------------------------|
| `call_expr`      | `*ast.CallExpr`       | Function/method call           |
| `binary_expr`    | `*ast.BinaryExpr`     | Binary operation (a + b)       |
| `unary_expr`     | `*ast.UnaryExpr`      | Unary operation (!a, -b)       |
| `selector_expr`  | `*ast.SelectorExpr`   | Field/method access (a.B)      |
| `if_stmt`        | `*ast.IfStmt`         | If statement                   |
| `assign_stmt`    | `*ast.AssignStmt`     | Assignment (a = b, a := b)     |
| `return_stmt`    | `*ast.ReturnStmt`     | Return statement               |
| `case_clause`    | `*ast.CaseClause`     | Case in switch                 |
| `func_decl`      | `*ast.FuncDecl`       | Function declaration           |
| `literal`        | `*ast.BasicLit`       | Literal value                  |
| `ident`          | `*ast.Ident`          | Identifier                     |

### Captures

Variables prefixed with `$` capture matched nodes:

```
pattern: call_expr(func: $fn, args: $args)
# $fn captures the function expression
# $args captures the argument list
```

### Wildcards

- `_` matches any single node
- `...` matches zero or more nodes (in lists)

### Examples

```
# Match any function call
pattern: call_expr(func: _, args: _)

# Match binary && expression
pattern: binary_expr(op: "&&", left: $l, right: $r)

# Match comparison with literal on right
pattern: binary_expr(op: comparison_op, left: $l, right: literal)

# Match assignment with call on right side
pattern: assign_stmt(lhs: $var, rhs: call_expr(func: $fn, args: $args))

# Match if with comparison condition
pattern: if_stmt(cond: binary_expr(op: comparison_op, left: $l, right: $r))
```

## Conditions

Conditions control when a rule applies:

```
# Width-based
when: line_width($node) > 80

# Node properties
when: is_simple_literal($r)
when: has_function_call($expr)
when: depth($node) < 3

# Compound
when: line_width($node) > 80 && has_function_call($expr)
```

### Built-in Predicates

| Predicate                    | Description                                        |
|------------------------------|----------------------------------------------------|
| `line_width($n)`             | Visual width of node on current line               |
| `total_width($n)`            | Total width if rendered single-line                |
| `is_simple_literal($n)`      | True for numbers, true, false, nil                 |
| `is_string_literal($n)`      | True for string literals                           |
| `has_function_call($n)`      | True if node contains any call expression          |
| `is_compressible($n)`        | True if node can be reformatted to fit             |
| `depth($n)`                  | Nesting depth of node                              |
| `arg_count($call)`           | Number of arguments in call                        |
| `is_method_chain($n)`        | True if selector chain (a.B().C())                 |

## Actions

### Core Actions

| Action                       | Description                                        |
|------------------------------|----------------------------------------------------|
| `keep_together($n)`          | Never break this node across lines                 |
| `break_after($n)`            | Insert newline after node                          |
| `break_before($n)`           | Insert newline before node                         |
| `reflow($call, strategy)`    | Reformat function call with strategy               |
| `indent($n, amount)`         | Indent node by amount                              |
| `wrap_binary($expr, op)`     | Break at binary operator boundaries                |

### Reflow Strategies

```
# Left-pack: fill lines greedily from left
reflow($call, strategy: "left-pack")

# One-per-line: each argument on its own line
reflow($call, strategy: "one-per-line")

# Adaptive: one-per-line if any arg is multiline, else left-pack
reflow($call, strategy: "adaptive")
```

### Action Sequencing

```
# Try first action, fall back to second if it doesn't help
action:
  try: reflow($inner_call)
  else: break_before($op)
```

## Complete Rule Examples

### Rule 1: Never Break Simple Comparisons

```
rule keep_simple_comparison {
  pattern: binary_expr(
    op: comparison_op,
    left: $l,
    right: $r
  )
  when: is_simple_literal($r)
  priority: 100  # High priority - apply first
  action: keep_together($node)
}
```

### Rule 2: Reflow Function Calls Before Breaking Operators

```
rule prefer_call_reflow {
  pattern: binary_expr(
    left: call_expr(func: $fn, args: $args),
    op: $op,
    right: $r
  )
  when: line_width($node) > 80
  priority: 50
  action:
    try: reflow($left)
    else: break_before($op)
}
```

### Rule 3: Handle Assignment with Long Call

```
rule assignment_with_call {
  pattern: assign_stmt(
    lhs: $var,
    rhs: call_expr(func: $fn, args: $args)
  )
  when: line_width($node) > 80
  priority: 40
  action: reflow($rhs, strategy: "left-pack")
}
```

### Rule 4: If Condition with Function Call

```
rule if_with_call_condition {
  pattern: if_stmt(
    cond: binary_expr(
      left: call_expr(func: $fn, args: $args),
      op: comparison_op,
      right: $r
    )
  )
  when: line_width($cond) > 80 && is_simple_literal($r)
  priority: 45
  action: reflow($left, strategy: "one-per-line")
}
```

### Rule 5: Long Binary Expression Chain

```
rule long_binary_chain {
  pattern: binary_expr(op: "&&" | "||", left: $l, right: $r)
  when: line_width($node) > 80 && !has_function_call($node)
  priority: 30
  action: wrap_binary($node, after_op: true)
}
```

### Rule 6: Method Chain

```
rule method_chain {
  pattern: call_expr(
    func: selector_expr(x: call_expr(...), sel: $method),
    args: $args
  )
  when: line_width($node) > 80
  priority: 35
  action:
    try: reflow($node, strategy: "adaptive")
    else: break_before($func.sel)
}
```

### Rule 7: Case Clause with Many Values

```
rule long_case_clause {
  pattern: case_clause(list: $vals)
  when: line_width($node) > 80 && len($vals) > 3
  priority: 40
  action: wrap_list($vals, after: ",")
}
```

### Rule 8: Return with Long Expression

```
rule long_return {
  pattern: return_stmt(results: $exprs)
  when: line_width($node) > 80 && has_function_call($exprs)
  priority: 35
  action:
    try: reflow_all_calls($exprs)
    else: wrap_binary($exprs, after_op: true)
}
```

## Operator Groups

Define operator categories for pattern matching:

```
define comparison_op = "==" | "!=" | "<" | ">" | "<=" | ">="
define logical_op = "&&" | "||"
define arithmetic_op = "+" | "-" | "*" | "/" | "%"
define simple_literal = number | "true" | "false" | "nil"
```

## Execution Model

### Rule Application Order

1. Sort rules by priority (higher numbers first)
2. For each rule in order:
   a. Try to match pattern against AST node
   b. If matched, evaluate `when` condition
   c. If condition true, execute action
   d. If action changes the code, re-parse and restart

### Fixpoint Iteration

```
repeat:
  for each node in AST (post-order):
    for each rule (by priority):
      if matches(rule.pattern, node) && eval(rule.when):
        result = execute(rule.action)
        if result.changed:
          AST = re-parse(result.code)
          continue repeat  # restart from top
until no changes
```

### Conflict Resolution

When multiple rules match the same node:
1. Higher priority wins
2. Equal priority: more specific pattern wins
3. Still equal: first defined wins

## Implementation Sketch

### Phase 1: Parser

```go
type Rule struct {
    Name     string
    Pattern  Pattern
    When     Condition
    Priority int
    Action   Action
}

type Pattern interface {
    Match(ast.Node) (map[string]ast.Node, bool)
}

type Condition interface {
    Eval(captures map[string]ast.Node, ctx *Context) bool
}

type Action interface {
    Execute(captures map[string]ast.Node, ctx *Context) ([]byte, bool)
}
```

### Phase 2: Matcher

```go
func (p *NodePattern) Match(n ast.Node) (map[string]ast.Node, bool) {
    // Check node type matches
    if !p.TypeMatches(n) {
        return nil, false
    }

    captures := make(map[string]ast.Node)

    // Match each field
    for _, field := range p.Fields {
        child := getField(n, field.Name)
        if field.Capture != "" {
            captures[field.Capture] = child
        }
        if field.SubPattern != nil {
            childCaptures, ok := field.SubPattern.Match(child)
            if !ok {
                return nil, false
            }
            mergeCaptures(captures, childCaptures)
        }
    }

    return captures, true
}
```

### Phase 3: Action Executor

```go
func (a *ReflowAction) Execute(
    captures map[string]ast.Node,
    ctx *Context,
) ([]byte, bool) {
    call := captures[a.Target].(*ast.CallExpr)

    // Get current rendering
    current := ctx.Render(call)
    if ctx.Width(current) <= ctx.ColumnLimit {
        return nil, false  // Already fits
    }

    // Apply reflow strategy
    switch a.Strategy {
    case "left-pack":
        return ctx.LeftPackCall(call), true
    case "one-per-line":
        return ctx.OnePerLineCall(call), true
    case "adaptive":
        return ctx.AdaptiveCall(call), true
    }

    return nil, false
}
```

## Benefits Over Current Approach

### Current llformat

- Rules embedded in Go code
- Hard to see all formatting logic at once
- Adding new rules requires understanding implementation
- No formal priority system

### With DSL

- Rules are data, not code
- Can be viewed, edited, validated independently
- Clear priority and conflict resolution
- Easier to test individual rules
- Could support user-defined rules via config file

## Trade-offs and Considerations

### Pros

1. **Clarity**: Formatting behavior is explicit and documented
2. **Maintainability**: Add/modify rules without touching Go code
3. **Testability**: Test pattern matching and actions independently
4. **Extensibility**: Users could add custom rules

### Cons

1. **Complexity**: Another language to learn and maintain
2. **Performance**: Pattern matching overhead (mitigated by caching)
3. **Debugging**: Need tooling to trace rule application
4. **AST Dependency**: Tightly coupled to Go's AST structure

### Mitigation Strategies

1. **Start Small**: Implement core patterns first, add features as needed
2. **Good Errors**: Clear messages when rules don't match or conflict
3. **Tracing Mode**: `--trace-rules` flag to show which rules fire
4. **Validation**: Warn about unreachable rules or ambiguous priorities

## Migration Path

### Phase 1: Internal DSL

Encode current rules in DSL format but interpret in Go:

```go
var rules = []Rule{
    {
        Name: "keep_simple_comparison",
        Pattern: BinaryExpr(Op(ComparisonOp), Right(SimpleLiteral)),
        Priority: 100,
        Action: KeepTogether("$node"),
    },
    // ...
}
```

### Phase 2: External DSL

Move rules to `.llformat` files:

```
# ~/.llformat/rules.llf
rule keep_simple_comparison { ... }
```

### Phase 3: User Rules

Allow users to define custom rules that augment defaults.

## Examples: Fixing Current Test Failures

### Example 11: `f.classifyLine(trimmed, inSwitch > 0, inInterface > 0)`

Current problem: Breaks at `>` operator, leaving `0` orphaned.

DSL solution:

```
rule assignment_call_with_comparisons {
  pattern: assign_stmt(
    lhs: $var,
    rhs: call_expr(
      func: $fn,
      args: $args
    )
  )
  when: line_width($node) > 80
  priority: 50
  action: reflow($rhs, strategy: "one-per-line")
}

rule keep_simple_comparison {
  pattern: binary_expr(op: comparison_op, right: $r)
  when: is_simple_literal($r)
  priority: 100
  action: keep_together($node)
}
```

Result:
```go
lineType := f.classifyLine(
    trimmedLongVariable, inSwitch > 0, inInterface > 0,
)
```

### Example 12: `if len(fmt.Sprintf(...)) > 10`

Current problem: Breaks at `>`, leaving `10` orphaned.

DSL solution:

```
rule if_call_comparison {
  pattern: if_stmt(
    cond: binary_expr(
      left: call_expr(func: $fn, args: $args),
      op: comparison_op,
      right: $r
    )
  )
  when: line_width($cond) > 80 && is_simple_literal($r)
  priority: 60
  action: reflow($left, strategy: "one-per-line")
}
```

Result:
```go
if len(
    fmt.Sprintf(
        "%d%d%d", alpha, beta, gamma,
    ),
) > 10 {
```

## Conclusion

A formatting DSL provides a clean separation between "what to format" (patterns and conditions) and "how to format" (actions). This makes the formatting logic more explicit, testable, and maintainable.

The key insight is modeling **compressibility** - understanding which constructs can be reformatted (function calls) versus which should stay atomic (simple comparisons), and preferring to compress what we can before breaking what we can't.

The proposed DSL syntax is inspired by:
- **Wadler's Prettier**: The "try X else Y" action sequencing
- **Topiary**: Pattern matching on AST nodes with captures
- **Go's AST**: Node types map directly to Go's `go/ast` package

Next steps:
1. Implement pattern matching for core Go AST nodes
2. Implement condition evaluation with built-in predicates
3. Implement core actions (keep_together, reflow, break_before/after)
4. Encode existing llformat rules in DSL format
5. Add rule tracing for debugging
