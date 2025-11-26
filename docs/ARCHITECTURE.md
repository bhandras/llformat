# llformat Architecture: Modular Building Blocks

This document proposes a refactored architecture that breaks the formatter into
reusable, composable building blocks. The goal is to eliminate duplication
between formatters (log/printf vs general multiline) and create a foundation
for consistent, testable formatting behavior.

## Current State Analysis

### Problems Identified

1. **Duplicated Scanning Logic**: Both `CompactCallFormatter` and
   `MultiLineCallFormatter` have their own implementations of string scanning,
   comment scanning, and bracket balancing.

2. **Inconsistent String Splitting**: `formatCallGreedy` and
   `formatCallPackedMultiLine` both split strings but use slightly different
   approaches.

3. **Global State**: `columnLimit`, `tabStop`, `fallbackNonTargets` are
   package-level variables that get mutated, making testing harder and
   concurrent use impossible.

4. **Mixed Responsibilities**: `compact_call_formatter.go` contains scanning
   utilities, width calculations, string quoting, AST helpers, and formatting
   logic all mixed together.

5. **Two Multiline Formatters**: `MultiLineCallFormatter.formatAsMultiLine` and
   `formatCallPackedMultiLine` do similar things but are separate
   implementations.

### Shared Functionality Between Formatters

| Functionality | CompactCallFormatter | MultiLineCallFormatter | CommentFormatter |
|--------------|-------------------|------------------------|------------------|
| String scanning | ✓ | ✓ | - |
| Comment scanning | ✓ | ✓ | ✓ |
| Balanced paren scan | ✓ | ✓ | - |
| Visual width calc | ✓ | ✓ | ✓ |
| Top-level splitting | ✓ | ✓ | - |
| String quoting | ✓ | - | - |
| String splitting | ✓ | - | - |
| Identifier detection | ✓ | ✓ | - |
| Indent extraction | ✓ | ✓ | ✓ |
| gofmt pass | ✓ | ✓ | - |

## Proposed Architecture

### Layer 1: Core Primitives (`scanner/`)

Lowest-level utilities with zero dependencies on other formatter code.

```
scanner/
├── string.go      # String literal scanning
├── comment.go     # Comment scanning
├── bracket.go     # Balanced bracket/paren scanning
└── split.go       # Top-level argument splitting
```

#### scanner/string.go
```go
// ScanString advances past a string literal starting at position i.
// Handles both double-quoted and backtick strings with escape sequences.
func ScanString(src []byte, i int) int

// IsStringStart returns true if position i starts a string literal.
func IsStringStart(b []byte, i int) bool
```

#### scanner/comment.go
```go
// ScanLineComment advances past a // comment starting at position i.
func ScanLineComment(src []byte, i int) int

// ScanBlockComment advances past a /* */ comment starting at position i.
func ScanBlockComment(src []byte, i int) int

// IsLineCommentStart returns true if position i starts a // comment.
func IsLineCommentStart(b []byte, i int) bool

// IsBlockCommentStart returns true if position i starts a /* comment.
func IsBlockCommentStart(b []byte, i int) bool

// StripComments removes // and /* */ comments from s while preserving strings.
func StripComments(s string) string
```

#### scanner/bracket.go
```go
// ScanBalancedParen finds the matching ')' for '(' at position open.
// Skips over strings and comments. Returns -1 if not found.
func ScanBalancedParen(src []byte, open int) int

// ScanBalanced finds the matching close bracket for open bracket at position.
// Generic version supporting (), [], {}.
func ScanBalanced(src []byte, open int, openChar, closeChar byte) int
```

#### scanner/split.go
```go
// SplitTopLevel splits a string by commas at depth 0, respecting parentheses.
// Used for function argument lists.
func SplitTopLevel(s string) []string

// SplitTopLevelAny splits by commas at depth 0, respecting (), [], and {}.
// Used when arguments may contain composite literals.
func SplitTopLevelAny(s string) []string
```

### Layer 2: Width Calculation (`width/`)

Visual width calculation with tab stop support.

```
width/
└── visual.go      # All width-related calculations
```

#### width/visual.go
```go
// Config for width calculations.
type Config struct {
    TabStop int // Default: 8
}

// VisualLen returns the visual column width of s.
func VisualLen(s string, cfg Config) int

// AdvanceCols returns the column after writing s starting from startCol.
func AdvanceCols(startCol int, s string, cfg Config) int

// RuneWidth returns the display width of a rune (0, 1, or 2).
func RuneWidth(r rune) int

// FirstLineLen returns the visual width of the first line of s.
func FirstLineLen(s string, cfg Config) int

// LastLineLen returns the visual width of the last line of s.
func LastLineLen(s string, cfg Config) int

// CutIndexForWidth returns the byte index where s exceeds maxCols.
func CutIndexForWidth(startCol int, s string, maxCols int, cfg Config) int
```

### Layer 3: Text Utilities (`text/`)

Higher-level text manipulation building on scanner and width.

```
text/
├── indent.go      # Indentation handling
├── identifier.go  # Go identifier recognition
└── quote.go       # String quoting and splitting
```

#### text/indent.go
```go
// LeadingWhitespace extracts the whitespace prefix from a line.
func LeadingWhitespace(b []byte, lineStart int) []byte

// LastLineStart finds the start position of the line containing pos.
func LastLineStart(b []byte, pos int) int

// SplitIndent splits a line into (indent, rest).
func SplitIndent(s string) (indent, rest string)
```

#### text/identifier.go
```go
// IsIdentifierStart returns true if c can start a Go identifier.
func IsIdentifierStart(c byte) bool

// IsIdentifierChar returns true if c can be part of a Go identifier.
func IsIdentifierChar(c byte) bool

// IsKeyword returns true if s is a Go keyword.
func IsKeyword(s string) bool

// ScanIdentifier extracts a Go identifier (with dots for selectors) from pos.
func ScanIdentifier(src []byte, pos int) (end int, ident string)
```

#### text/quote.go
```go
// QuoteGoString produces a double-quoted Go string literal.
func QuoteGoString(s string) string

// SplitQuoted splits text into quoted segments that fit within width.
// Returns segments joined with " +" for continuation.
type SplitConfig struct {
    StartCol   int
    ContIndent string
    Width      int
    TabStop    int
}
func SplitQuoted(text string, cfg SplitConfig) string

// LastQuotedSpaceBefore finds the last space where a quoted prefix fits.
func LastQuotedSpaceBefore(startCol int, s string, boundary int, tabStop int) int
```

### Layer 4: AST Helpers (`ast/`)

Go AST-based analysis for expressions and calls.

```
ast/
├── string.go      # String expression flattening
├── call.go        # Call expression analysis
└── composite.go   # Composite literal handling
```

#### ast/string.go
```go
// FlattenStringExpr extracts the concatenated string value from an expression
// that is purely string literals (or concatenations thereof).
func FlattenStringExpr(e ast.Expr) (string, bool)

// FlattenStringExprDoubleQuoted is like FlattenStringExpr but only accepts
// double-quoted literals (not raw strings).
func FlattenStringExprDoubleQuoted(e ast.Expr) (string, bool)
```

#### ast/call.go
```go
// IsCallExpr returns true if s parses as a function call.
func IsCallExpr(s string) bool

// HasNestedCall returns true if the call has nested calls in its arguments.
func HasNestedCall(s string) bool

// HasMultilineComposite returns true if arguments contain map/struct literals.
func HasMultilineComposite(s string) bool

// IsFunctionDefinition returns true if position looks like a func definition.
func IsFunctionDefinition(src []byte, pos int) bool

// FindCallAt finds a function call starting at position i.
// Returns (start, end) positions or (0, 0) if not found.
func FindCallAt(src []byte, i int) (start, end int)
```

#### ast/composite.go
```go
// IsCompositeLiteral returns true if s parses as a composite literal.
func IsCompositeLiteral(s string) bool

// FindTopLevelBraces finds the first top-level { and its matching }.
func FindTopLevelBraces(s string) (open, close int)

// FormatComposite formats a composite literal with proper indentation.
type CompositeConfig struct {
    ContIndent  string
    ColumnLimit int
    TabStop     int
}
func FormatComposite(arg string, cfg CompositeConfig) (string, bool)
```

### Layer 5: Formatting Engines (`format/`)

Core formatting algorithms that combine the building blocks.

```
format/
├── config.go      # Shared configuration
├── argument.go    # Individual argument formatting
├── call.go        # Call expression formatting strategies
├── comment.go     # Comment block formatting
└── wrap.go        # Text wrapping utilities
```

#### format/config.go
```go
// Config holds all formatting configuration.
type Config struct {
    ColumnLimit int      // Default: 80
    TabStop     int      // Default: 8
    Targets     []string // Functions to format specially (log.Infof, etc.)
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config
```

#### format/argument.go
```go
// ArgType classifies an argument for formatting decisions.
type ArgType int

const (
    ArgString     ArgType = iota // Pure string literal or concatenation
    ArgComposite                 // Map, struct, slice, or array literal
    ArgCall                      // Nested function call
    ArgExpression                // Any other expression
)

// ClassifyArg determines the type of an argument string.
func ClassifyArg(arg string) ArgType

// FormatArg formats a single argument according to its type and context.
type ArgContext struct {
    CurrentCol  int
    ContIndent  string
    Config      Config
    IsFirst     bool
    IsLast      bool
}
func FormatArg(arg string, ctx ArgContext) (formatted string, multiline bool)
```

#### format/call.go
```go
// FormatCall formats a function call according to the specified strategy.
type Strategy int

const (
    StrategyGreedy     Strategy = iota // Pack arguments left-to-right
    StrategyPacked                     // Each arg on its own line, packed
    StrategyOnePerLine                 // Each arg on its own line
)

// CallContext provides context for call formatting.
type CallContext struct {
    WSIndent  string   // Leading whitespace of the line
    BaseLen   int      // Visual column where call starts
    Config    Config
    Strategy  Strategy
}

// FormatCall formats a complete function call.
func FormatCall(call []byte, ctx CallContext) string

// NeedsWrapping determines if a call exceeds the column limit.
func NeedsWrapping(call []byte, ctx CallContext) bool
```

#### format/comment.go
```go
// ReflowLineComments reflows a block of consecutive // comments.
func ReflowLineComments(block []string, indent string, cfg Config) []string

// ReflowBlockComment reflows a /* */ comment block.
func ReflowBlockComment(block []string, indent string, cfg Config) []string

// WrapWithPrefixes wraps text using firstPrefix and contPrefix.
func WrapWithPrefixes(firstPrefix, contPrefix, text string, columnLimit int) []string

// HoistInlineComments moves trailing comments above their code lines.
func HoistInlineComments(src []byte) []byte
```

#### format/wrap.go
```go
// WrapText wraps text to fit within width, starting at startCol.
type WrapConfig struct {
    StartCol   int
    Width      int
    ContIndent string
    TabStop    int
}
func WrapText(text string, cfg WrapConfig) string
```

### Layer 6: High-Level Formatters (`formatter/`)

User-facing formatters that compose the building blocks.

```
formatter/
├── pipeline.go    # Formatter chaining
├── log.go         # Log/printf formatter (CompactCallFormatter)
├── call.go        # General call formatter
└── comment.go     # Comment formatter
```

#### formatter/pipeline.go
```go
// Formatter is the common interface for all formatters.
type Formatter interface {
    FormatFile(src []byte) []byte
}

// Pipeline chains multiple formatters and optionally runs gofmt.
type Pipeline struct {
    Formatters []Formatter
    RunGofmt   bool
}

func (p *Pipeline) FormatFile(src []byte) []byte
```

#### formatter/log.go
```go
// LogFormatter formats targeted log/printf calls with greedy left-flow.
type LogFormatter struct {
    Config format.Config
}

func NewLogFormatter(cfg format.Config) *LogFormatter
func (f *LogFormatter) FormatFile(src []byte) []byte
```

#### formatter/call.go
```go
// CallFormatter formats non-targeted function calls that exceed column limit.
type CallFormatter struct {
    Config   format.Config
    Excludes []string // Functions to skip (includes log targets by default)
}

func NewCallFormatter(cfg format.Config) *CallFormatter
func (f *CallFormatter) FormatFile(src []byte) []byte
```

#### formatter/comment.go
```go
// CommentFormatter reflows standalone comment blocks.
type CommentFormatter struct {
    Config          format.Config
    MoveInlineAbove bool
}

func NewCommentFormatter(cfg format.Config) *CommentFormatter
func (f *CommentFormatter) FormatFile(src []byte) []byte
```

## Dependency Graph

```
                    ┌─────────────────────────────────────────┐
                    │           formatter/ (Layer 6)          │
                    │  Pipeline, LogFormatter, CallFormatter  │
                    │            CommentFormatter             │
                    └─────────────────┬───────────────────────┘
                                      │
                    ┌─────────────────▼───────────────────────┐
                    │            format/ (Layer 5)            │
                    │   Config, Argument, Call, Comment, Wrap │
                    └─────────────────┬───────────────────────┘
                                      │
          ┌───────────────────────────┼───────────────────────────┐
          │                           │                           │
┌─────────▼─────────┐   ┌─────────────▼─────────────┐   ┌────────▼────────┐
│  ast/ (Layer 4)   │   │     text/ (Layer 3)       │   │                 │
│  String, Call,    │   │  Indent, Identifier,      │   │                 │
│  Composite        │   │  Quote                    │   │                 │
└─────────┬─────────┘   └─────────────┬─────────────┘   │                 │
          │                           │                 │                 │
          │             ┌─────────────▼─────────────┐   │                 │
          │             │    width/ (Layer 2)       │   │                 │
          │             │    Visual width calc      │   │                 │
          │             └─────────────┬─────────────┘   │                 │
          │                           │                 │                 │
          └───────────────────────────┼─────────────────┘
                                      │
                    ┌─────────────────▼───────────────────────┐
                    │          scanner/ (Layer 1)             │
                    │   String, Comment, Bracket, Split       │
                    └─────────────────────────────────────────┘
```

## Migration Strategy

### Phase 1: Extract Scanner Layer
1. Create `scanner/` package with existing scanning functions
2. Update imports in existing code
3. Add comprehensive tests for scanner functions
4. No behavior change, pure refactoring

### Phase 2: Extract Width Layer
1. Create `width/` package
2. Remove global `tabStop` variable - pass as config
3. Update all callers to use new package

### Phase 3: Extract Text Layer
1. Create `text/` package with indent, identifier, quote
2. Move `quoteGoString`, `buildSplitQuoted` etc.

### Phase 4: Extract AST Layer
1. Create `ast/` package
2. Move expression analysis functions

### Phase 5: Create Format Layer
1. Create unified `format/` package
2. Implement `FormatCall` that supports both strategies
3. Consolidate `formatCallGreedy` and `formatCallPackedMultiLine`

### Phase 6: Refactor High-Level Formatters
1. Rewrite formatters to use new building blocks
2. Create `Pipeline` for chaining
3. Remove duplicate code

## Benefits

1. **Testability**: Each layer can be unit tested independently
2. **Reusability**: Scanner/width/text utilities are useful beyond this project
3. **Consistency**: Single implementation of each algorithm
4. **No Global State**: Config passed explicitly, enabling concurrent use
5. **Clear Dependencies**: Each layer only depends on lower layers
6. **Easier Debugging**: Isolated components with clear responsibilities

## Key Insight: Unifying the Two Call Formatters

The current `formatCallGreedy` (for log/printf) and `formatCallPackedMultiLine`
(for general calls) should be unified because they differ only in:

1. **Target detection**: Which calls to format
2. **Argument packing**: How aggressively to pack on each line
3. **String splitting**: Slightly different heuristics

Both share the same fundamental algorithm:
1. Parse the call head and argument list
2. For each argument, classify it (string, composite, call, expr)
3. Decide whether to place inline or break
4. Format strings with splitting as needed
5. Recursively format nested calls

A single `FormatCall` function with a `Strategy` parameter can handle both cases,
using shared argument formatting logic. The strategy controls:
- Whether to pack multiple args per line
- How aggressively to split strings
- Whether to preserve certain structures inline

This unification eliminates ~500 lines of duplicate code and ensures consistent
behavior across all formatting scenarios.
