# Formatter Architecture Refactoring Plan

## Status: Phase 1 Complete

**Completed:**
- Phase 1: Core Building Blocks (config.go, scan.go, indent.go)
- Phase 2: Breaking Strategies (breaker.go with LeftFlowBreaker, VerticalBreaker)
- Phase 3: Rules as Data (rules.go with Rule interface, CallRule)
- Phase 5: Pipeline (stage.go with Stage type and dependencies, updated pipeline.go)
- Phase 6: Presets (presets.go with DefaultPreset)

**Remaining:**
- Phase 4: Migrate existing formatters to use new building blocks (optional, can be done incrementally)
- Remove global mutable state from compact_call_formatter.go (needs careful migration)

## Overview

This plan redesigns the llformat formatter architecture to be more composable and formalize formatting rules as data-driven building blocks. Breaking changes to the public API are allowed.

## Current Issues

1. **Global mutable state**: `columnLimit`, `tabStop`, `fallbackNonTargets` are package-level variables
2. **Duplicated patterns**: Config defaults, scanner skipping, width calculations repeated 5+ times
3. **Hard-coded rules**: Formatting logic embedded in procedural code, not configurable
4. **Implicit dependencies**: Pipeline stages assume specific ordering without enforcement
5. **Monolithic formatters**: Each formatter does everything (scanning, breaking, width calc)

## New Architecture

### Phase 1: Core Building Blocks

#### 1.1 Base Configuration (`formatter/config.go`)

```go
// BaseConfig holds common formatting configuration.
type BaseConfig struct {
    ColumnLimit int // Default: 80
    TabStop     int // Default: 8
}

// NewBaseConfig creates a BaseConfig with defaults applied.
func NewBaseConfig(col, tab int) BaseConfig

// Width returns the visual width of a string.
func (c BaseConfig) Width(s string) int

// FitsInLimit checks if content at currentCol fits within limit.
func (c BaseConfig) FitsInLimit(currentCol int, content string) bool
```

#### 1.2 Scanner Context (`formatter/scan.go`)

```go
// Scanner provides stateful source scanning that skips strings/comments.
type Scanner struct {
    src []byte
    pos int
}

func NewScanner(src []byte) *Scanner
func (s *Scanner) Pos() int
func (s *Scanner) Advance() byte
func (s *Scanner) SkipLiterals() // Skip strings and comments
func (s *Scanner) AtEnd() bool
func (s *Scanner) Peek() byte
func (s *Scanner) Slice(start, end int) []byte
func (s *Scanner) RemainingFrom(start int) []byte
```

#### 1.3 Indent Manager (`formatter/indent.go`)

```go
// Indent tracks indentation for a formatting context.
type Indent struct {
    Base     string // Leading whitespace of the line
    TabStop  int
}

func IndentFromLine(line string, tabStop int) Indent
func (i Indent) Width() int
func (i Indent) Continuation() Indent  // Returns Base + "\t"
func (i Indent) String() string
```

### Phase 2: Breaking Strategies (`formatter/breaker.go`)

#### 2.1 Breaker Interface

```go
// Breaker defines a strategy for breaking content across lines.
type Breaker interface {
    // Break splits content to fit within the column limit.
    // Returns the formatted content and whether a break was made.
    Break(ctx BreakContext) (string, bool)
}

// BreakContext provides context for breaking decisions.
type BreakContext struct {
    Content     string  // The content to potentially break
    Indent      Indent  // Current indentation
    CurrentCol  int     // Current column position
    Config      BaseConfig
}
```

#### 2.2 Breaker Implementations

```go
// LeftFlowBreaker packs elements left-to-right, breaking when limit exceeded.
// Used for: function call arguments, list elements
type LeftFlowBreaker struct {
    Separator   string // ", " for args
    Terminator  string // ")" for calls
}

// VerticalBreaker puts each element on its own line.
// Used for: multi-line function calls (one arg per line)
type VerticalBreaker struct {
    Separator      string
    TrailingComma  bool
}

// BinaryExprBreaker breaks at binary operators with precedence.
// Used for: long boolean expressions, arithmetic
type BinaryExprBreaker struct {
    Operators []OperatorRule // Ordered by break preference
}

// StringBreaker splits long string literals at word boundaries.
// Used for: log format strings, error messages
type StringBreaker struct {
    QuoteChar byte   // '"' or '`'
    JoinOp    string // " +" for concatenation
}
```

### Phase 3: Rules as Data (`formatter/rules.go`)

#### 3.1 Rule Definitions

```go
// Rule defines a formatting rule that can be applied to source.
type Rule interface {
    // Match returns true if this rule applies at the given position.
    Match(src []byte, pos int) bool
    // Apply formats the matched content.
    Apply(src []byte, pos int, cfg BaseConfig) (replacement []byte, consumed int)
}

// CallRule formats function calls matching specific patterns.
type CallRule struct {
    Patterns []string      // e.g., "log.Infof(", "fmt.Errorf("
    Breaker  Breaker       // Strategy for breaking arguments
    Priority int           // Higher = applied first
}

// ExprRule formats expressions exceeding column limit.
type ExprRule struct {
    Operators []OperatorRule // Operators to break at
    Breaker   Breaker
}

// SignatureRule formats function/method signatures.
type SignatureRule struct {
    ParamBreaker  Breaker // For parameter lists
    ReturnBreaker Breaker // For return types
}
```

#### 3.2 Operator Rules for Expression Breaking

```go
// OperatorRule defines how to break at a specific operator.
type OperatorRule struct {
    Op       string // "&&", "||", ",", etc.
    Priority int    // Lower = break first (prefer breaking here)
    Context  string // "expr", "case", "call" - where this applies
}

// DefaultOperatorRules defines standard Go formatting preferences.
var DefaultOperatorRules = []OperatorRule{
    {Op: ",", Priority: 0, Context: "case"},   // Break case lists first
    {Op: "||", Priority: 1, Context: "expr"},  // Then logical OR
    {Op: "&&", Priority: 2, Context: "expr"},  // Then logical AND
    {Op: "==", Priority: 3, Context: "expr"},  // Then comparisons
    {Op: "!=", Priority: 3, Context: "expr"},
    {Op: "+", Priority: 4, Context: "expr"},   // Then arithmetic
    {Op: "-", Priority: 4, Context: "expr"},
}
```

### Phase 4: Formatter Interface (`formatter/formatter.go`)

```go
// Formatter transforms source code.
type Formatter interface {
    Format(src []byte) []byte
}

// RuleFormatter applies a set of rules to source.
type RuleFormatter struct {
    Config BaseConfig
    Rules  []Rule
}

func (f *RuleFormatter) Format(src []byte) []byte
```

### Phase 5: Pipeline (`formatter/pipeline.go`)

```go
// Stage represents a named formatting stage.
type Stage struct {
    Name      string
    Formatter Formatter
    Requires  []string // Names of stages that must run first
}

// Pipeline orchestrates multiple formatting stages.
type Pipeline struct {
    Config BaseConfig
    Stages []Stage
}

// NewDefaultPipeline creates the standard llformat pipeline.
func NewDefaultPipeline(cfg BaseConfig) *Pipeline {
    return &Pipeline{
        Config: cfg,
        Stages: []Stage{
            {Name: "comments", Formatter: NewCommentFormatter(cfg)},
            {Name: "calls", Formatter: NewCallFormatter(cfg, DefaultCallRules)},
            {Name: "expressions", Formatter: NewExprFormatter(cfg)},
            {Name: "signatures", Formatter: NewSignatureFormatter(cfg)},
            {Name: "spacing", Formatter: NewSpacingFormatter(cfg)},
        },
    }
}

func (p *Pipeline) Format(src []byte) []byte
```

### Phase 6: Preset Configurations (`formatter/presets.go`)

```go
// Preset defines a complete formatting configuration.
type Preset struct {
    Config      BaseConfig
    CallRules   []CallRule
    ExprRules   []ExprRule
    SpacingOpts SpacingOptions
}

// DefaultPreset is the standard llformat configuration.
var DefaultPreset = Preset{
    Config: BaseConfig{ColumnLimit: 80, TabStop: 8},
    CallRules: []CallRule{
        {
            Patterns: []string{"log.Infof(", "log.Debugf(", "log.Tracef(",
                              "log.Errorf(", "log.Warnf("},
            Breaker:  &LeftFlowBreaker{Separator: ", ", Terminator: ")"},
            Priority: 10,
        },
        {
            Patterns: []string{"fmt.Printf(", "fmt.Sprintf(", "fmt.Errorf("},
            Breaker:  &LeftFlowBreaker{Separator: ", ", Terminator: ")"},
            Priority: 10,
        },
    },
    ExprRules: []ExprRule{
        {Operators: DefaultOperatorRules, Breaker: &BinaryExprBreaker{}},
    },
    SpacingOpts: SpacingOptions{
        BlankBeforeReturn:      true,
        BlankBetweenCases:      true,
        BlankBetweenMethods:    true,
    },
}
```

## File Structure After Refactoring

```
formatter/
├── config.go        # BaseConfig, width calculations
├── scan.go          # Scanner for source traversal
├── indent.go        # Indent management
├── breaker.go       # Breaker interface + implementations
├── rules.go         # Rule interface + implementations
├── formatter.go     # Formatter interface, RuleFormatter
├── pipeline.go      # Pipeline orchestration
├── presets.go       # Default configurations
├── call.go          # Call-specific formatting (replaces compact_call_formatter)
├── expr.go          # Expression breaking (replaces long_expr_formatter)
├── signature.go     # Function signature formatting (replaces func_sig_formatter)
├── comment.go       # Comment reformatting (simplified comment_formatter)
├── spacing.go       # Blank line formatting (replaces blank_line_formatter)
└── *_test.go        # Tests for each component
```

## Migration Path

### Step 1: Create Core Building Blocks
- Add `config.go` with BaseConfig
- Add `scan.go` with Scanner
- Add `indent.go` with Indent
- Add unit tests for each

### Step 2: Create Breaker Abstraction
- Add `breaker.go` with interface
- Implement LeftFlowBreaker (extract from compact_call_formatter)
- Implement VerticalBreaker (extract from multiline_call_formatter)
- Implement BinaryExprBreaker (extract from long_expr_formatter)
- Implement StringBreaker (extract from compact_call_formatter)

### Step 3: Create Rule System
- Add `rules.go` with Rule interface
- Implement CallRule using Breakers
- Implement ExprRule using Breakers

### Step 4: Migrate Formatters
- Rewrite call.go using RuleFormatter + CallRules
- Rewrite expr.go using RuleFormatter + ExprRules
- Rewrite signature.go using Breakers
- Simplify comment.go and spacing.go

### Step 5: Update Pipeline
- Remove global state
- Add stage dependencies
- Update NewDefaultPipeline

### Step 6: Update CLI
- Use Preset configurations
- Remove deprecated APIs

## Benefits

1. **Testable**: Each breaker/rule can be unit tested in isolation
2. **Composable**: Mix and match breakers for new formatting styles
3. **Configurable**: Rules as data allow runtime customization
4. **No global state**: Thread-safe, parallel test execution
5. **Explicit dependencies**: Pipeline validates stage ordering
6. **Extensible**: Add new rules without modifying existing formatters

## Estimated Effort

- Phase 1 (Core): ~200 lines new code
- Phase 2 (Breakers): ~400 lines (mostly extracted from existing)
- Phase 3 (Rules): ~150 lines
- Phase 4-5 (Integration): ~300 lines
- Phase 6 (Migration): Refactoring existing ~2000 lines

Total: Significant refactoring but reduces overall complexity.
