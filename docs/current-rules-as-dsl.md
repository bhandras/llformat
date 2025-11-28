# Current llformat Rules Expressed as DSL

This document maps the current llformat formatting logic to the proposed DSL syntax.

## Operator Definitions

```dsl
# Operator groups for pattern matching
define comparison_op = "==" | "!=" | "<" | ">" | "<=" | ">="
define logical_op = "&&" | "||"
define arithmetic_op = "+" | "-" | "*" | "/" | "%"
define simple_literal = number | "true" | "false" | "nil"

# Log/format functions that get special handling
define log_func = "log.Infof" | "log.Debugf" | "log.Tracef"
                | "log.Errorf" | "log.Warnf"
define fmt_func = "fmt.Printf" | "fmt.Sprintf" | "fmt.Errorf"
define format_func = log_func | fmt_func
```

## Rule 1: Keep Simple Comparisons Together (NEW - fixes Examples 11, 12)

**Source**: `long_expr_formatter.go:isFollowedBySimpleLiteral()`

```dsl
rule keep_simple_comparison {
  pattern: binary_expr(
    op: comparison_op,
    left: _,
    right: $r
  )
  when: is_simple_literal($r)
  priority: 100  # Highest - never break these
  action: keep_together($node)
}
```

**Rationale**: `x > 0`, `count == nil`, `flag == true` should never be split across lines.

## Rule 2: Assignment with Function Call

**Source**: `multiline_call_formatter.go:formatOneCallInSource()`

```dsl
rule assignment_with_long_call {
  pattern: assign_stmt(
    lhs: $var,
    rhs: call_expr(func: $fn, args: $args)
  )
  when: line_width($node) > column_limit
      && !matches($fn, format_func)  # Exclude log/fmt functions
  priority: 50
  action: reflow($rhs, strategy: "one-per-line")
}
```

**Example**:
```go
# Before
lineType := f.classifyLine(trimmedLongVariable, inSwitch > 0, inInterface > 0)

# After
lineType := f.classifyLine(
    trimmedLongVariable, inSwitch > 0, inInterface > 0,
)
```

## Rule 3: If Condition with Function Call and Simple Comparison

**Source**: NEW (to fix Example 12)

```dsl
rule if_call_comparison {
  pattern: if_stmt(
    cond: binary_expr(
      left: call_expr(func: $fn, args: $args),
      op: comparison_op,
      right: $r
    )
  )
  when: line_width($cond) > column_limit
      && is_simple_literal($r)
  priority: 60
  action: reflow($left, strategy: "one-per-line")
}
```

**Example**:
```go
# Before
if len(fmt.Sprintf("%d%d%d", alpha, beta, gamma)) > 10 {

# After
if len(
    fmt.Sprintf(
        "%d%d%d", alpha, beta, gamma,
    ),
) > 10 {
```

## Rule 4: Long Logical Expression Chain

**Source**: `long_expr_formatter.go:findBreakPoint()`

```dsl
rule long_logical_chain {
  pattern: binary_expr(
    op: logical_op,
    left: $l,
    right: $r
  )
  when: line_width($node) > column_limit
      && !has_compressible_call($node)
  priority: 30
  action: wrap_binary($node, break_after: $op)
}
```

**Example**:
```go
# Before
if userIsAuthenticated && userHasPermission && !accountIsLocked && sessionIsValid {

# After
if userIsAuthenticated && userHasPermission && !accountIsLocked &&
    sessionIsValid {
```

## Rule 5: Long Logical Expression with Function Call

**Source**: NEW (general principle from user)

```dsl
rule logical_chain_with_call {
  pattern: binary_expr(
    op: logical_op,
    left: $l,
    right: $r
  )
  when: line_width($node) > column_limit
      && has_compressible_call($node)
  priority: 40  # Higher than plain logical chain
  action:
    try: reflow_nested_calls($node)
    else: wrap_binary($node, break_after: $op)
}
```

**Rationale**: Try to compress function calls first before breaking at operators.

## Rule 6: Long Arithmetic Expression

**Source**: `long_expr_formatter.go:findBreakPoint()` priority 4

```dsl
rule long_arithmetic_expr {
  pattern: binary_expr(
    op: arithmetic_op,
    left: $l,
    right: $r
  )
  when: line_width($node) > column_limit
  priority: 20
  action: wrap_binary($node, break_after: $op)
}
```

**Example**:
```go
# Before
result := a + b + c + d + e + f + g + h + someVeryLongFunctionName(a, b)

# After
result := a + b + c + d + e + f + g + h +
    someVeryLongFunctionName(a, b)
```

## Rule 7: String Concatenation

**Source**: `long_expr_formatter.go:tryReformatStringConcat()`

```dsl
rule string_concat {
  pattern: binary_expr(
    op: "+",
    left: string_literal,
    right: $r
  )
  when: line_width($node) > column_limit
      && is_pure_string_concat($node)  # Only strings, no other exprs
  priority: 45
  action: combine_and_resplit($node)
}
```

**Example**:
```go
# Before
return "This is a very long string that " + "spans multiple parts " + "and is concatenated"

# After
return "This is a very long string that spans multiple parts and is " +
    "concatenated"
```

## Rule 8: Case Clause with Many Values

**Source**: `long_expr_formatter.go:findBreakPoint()` with `isCaseStmt`

```dsl
rule long_case_clause {
  pattern: case_clause(list: $vals)
  when: line_width($node) > column_limit
  priority: 35
  action: wrap_list($vals, break_after: ",")
}
```

**Example**:
```go
# Before
case TypeA, TypeB, TypeC, TypeD, TypeE, TypeF, TypeG, TypeH, TypeI, TypeJ:

# After
case TypeA, TypeB, TypeC, TypeD, TypeE, TypeF, TypeG, TypeH, TypeI,
    TypeJ:
```

## Rule 9: Method Chain

**Source**: `multiline_call_formatter.go` (commented out in long_expr)

```dsl
rule method_chain {
  pattern: call_expr(
    func: selector_expr(
      x: call_expr(func: $inner, args: $inner_args),
      sel: $method
    ),
    args: $args
  )
  when: line_width($node) > column_limit
  priority: 35
  action:
    try: reflow($node, strategy: "adaptive")
    else: break_before_selector($method)
}
```

**Example**:
```go
# Before
result := client.WithTimeout(30*time.Second).WithRetry(3).WithHeaders(headers).Execute(ctx, request)

# After (reflow)
result := client.WithTimeout(30*time.Second).WithRetry(3).WithHeaders(
    headers,
).Execute(ctx, request)
```

## Rule 10: Format Function Calls (log.Infof, fmt.Sprintf, etc.)

**Source**: `left_flow_call_formatter.go`

```dsl
rule format_function_call {
  pattern: call_expr(
    func: $fn,
    args: $args
  )
  when: matches($fn, format_func)
      && line_width($node) > column_limit
  priority: 55
  action: reflow($node, strategy: "left-pack-with-text-split")
}
```

**Special behavior**: Text arguments (first arg for log, format string for fmt) can be split at word boundaries with `+` continuation.

## Rule 11: Long Function Declaration Parameters

**Source**: Handled separately, needs width check

```dsl
rule long_func_params {
  pattern: func_decl(
    name: $name,
    params: $params,
    results: $results
  )
  when: line_width($params) > column_limit
  priority: 40
  action: wrap_params($params, one_per_line: false)
}
```

**Example**:
```go
# Before
func example5(items []int, stopRequested bool, ctx context.Context, retryCount, maxRetries int) {

# After
func example5(items []int, stopRequested bool, ctx context.Context, retryCount,
    maxRetries int) {
```

## Rule 12: Long Return Statement

**Source**: `long_expr_formatter.go` via `tryReformatStringConcat`

```dsl
rule long_return {
  pattern: return_stmt(results: $exprs)
  when: line_width($node) > column_limit
  priority: 35
  action:
    try: reflow_calls_in($exprs)
    else: wrap_binary($exprs, break_after: logical_op | arithmetic_op)
}
```

## Rule 13: Composite Literal in Argument

**Source**: `expression_formatter.go:FormatCompositeLiteralArg()`

```dsl
rule composite_literal_arg {
  pattern: call_expr(
    args: [... composite_lit(type: $t, elts: $elts) ...]
  )
  when: line_width($node) > column_limit
      && !contains($t, "func")  # Not function literals
  priority: 42
  action: expand_composite($elts,
    style: if has_keys($elts) then "one-per-line" else "left-pack"
  )
}
```

## Rule 14: Interface Method Declarations (Blank Lines)

**Source**: `blank_line_formatter.go`

```dsl
rule interface_methods {
  pattern: interface_type(methods: $methods)
  when: len($methods) > 1
  priority: 10  # Low priority, formatting not width
  action: ensure_blank_lines_between($methods)
}
```

## Rule 15: Switch Case Separation (Blank Lines)

**Source**: `blank_line_formatter.go`

```dsl
rule case_separation {
  pattern: switch_stmt(body: [case_clause ...])
  when: any_case_has_multiple_stmts($body)
  priority: 10
  action: ensure_blank_lines_between_cases($body)
}
```

## Rule 16: Blank Line Before Return

**Source**: `blank_line_formatter.go`

```dsl
rule blank_before_return {
  pattern: block_stmt(
    list: [... $prev return_stmt ...]
  )
  when: !is_trivial($prev)  # Not opening brace, case, etc.
  priority: 10
  action: ensure_blank_line_before_return($prev)
}
```

---

## Priority Summary

| Priority | Rule Category                           |
|----------|----------------------------------------|
| 100      | Atomic constraints (keep_together)      |
| 60       | If conditions with calls                |
| 55       | Format function calls (log, fmt)        |
| 50       | Assignment with calls                   |
| 45       | String concatenation                    |
| 42       | Composite literals                      |
| 40       | Function params, logical with calls     |
| 35       | Method chains, returns, case clauses    |
| 30       | Plain logical chains                    |
| 20       | Arithmetic expressions                  |
| 10       | Blank line rules                        |

## Execution Order

1. **Atomic constraints first** (priority 100): Mark what can't be broken
2. **Compressible constructs** (60-40): Try to reflow function calls
3. **Line breaking** (35-20): Break at operators if reflow didn't help
4. **Cosmetic** (10): Add blank lines

This ensures we always try to "compress" (reflow) before we "break" (split at operators).

---

## Mapping to Current Code

| DSL Rule                    | Current Implementation                          |
|-----------------------------|------------------------------------------------|
| `keep_simple_comparison`    | `isFollowedBySimpleLiteral()` in long_expr     |
| `assignment_with_long_call` | `MultiLineCallFormatter.formatOneCallInSource` |
| `if_call_comparison`        | NEW - not yet implemented                       |
| `long_logical_chain`        | `findBreakPoint()` priority 1,2                |
| `long_arithmetic_expr`      | `findBreakPoint()` priority 4                  |
| `string_concat`             | `tryReformatStringConcat()`                    |
| `long_case_clause`          | `findBreakPoint()` with `isCaseStmt`           |
| `method_chain`              | Commented out in long_expr, partial in multiline|
| `format_function_call`      | `LeftFlowFormatter`                            |
| `composite_literal_arg`     | `FormatCompositeLiteralArg()`                  |
| `interface_methods`         | `BlankLineFormatter`                           |
| `case_separation`           | `BlankLineFormatter`                           |
| `blank_before_return`       | `BlankLineFormatter`                           |
