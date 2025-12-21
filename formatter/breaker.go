package formatter

import (
	"strings"

	"github.com/lightninglabs/llformat/scanner"
)

// BreakContext provides context for breaking decisions.
type BreakContext struct {
	Elements   []string   // The elements to format (e.g., function arguments)
	Indent     Indent     // Current base indentation
	CurrentCol int        // Starting column position
	Config     BaseConfig // Formatting configuration
}

// BreakResult contains the result of a breaking operation.
type BreakResult struct {
	Content string // The formatted content
	Broke   bool   // Whether line breaks were added
}

// Breaker defines a strategy for breaking content across lines.
type Breaker interface {
	// Break splits elements to fit within the column limit.
	Break(ctx BreakContext) BreakResult
}

// LeftFlowBreaker packs elements left-to-right, breaking when limit exceeded.
// Used for: function call arguments, list elements.
type LeftFlowBreaker struct {
	Separator     string // Separator between elements (e.g., ", ")
	OpenBracket   string // Opening bracket (e.g., "(")
	CloseBracket  string // Closing bracket (e.g., ")")
	TrailingComma bool   // Add trailing comma before closing bracket
}

// NewLeftFlowBreaker creates a new LeftFlowBreaker with default settings for
// function calls.
func NewLeftFlowBreaker() *LeftFlowBreaker {
	return &LeftFlowBreaker{
		Separator:     ", ",
		OpenBracket:   "(",
		CloseBracket:  ")",
		TrailingComma: false,
	}
}

// Break implements Breaker for left-flow packing.
func (b *LeftFlowBreaker) Break(ctx BreakContext) BreakResult {
	if len(ctx.Elements) == 0 {
		return BreakResult{
			Content: b.OpenBracket + b.CloseBracket,
			Broke:   false,
		}
	}

	// Try single line first
	singleLine := b.formatSingleLine(ctx.Elements)
	if ctx.Config.FitsInLimit(ctx.CurrentCol, singleLine) {
		return BreakResult{
			Content: singleLine,
			Broke:   false,
		}
	}

	// Need to break - use left-flow packing
	return b.formatLeftFlow(ctx)
}

// formatSingleLine formats all elements on a single line.
func (b *LeftFlowBreaker) formatSingleLine(elements []string) string {
	var sb strings.Builder
	sb.WriteString(b.OpenBracket)
	for i, elem := range elements {
		if i > 0 {
			sb.WriteString(b.Separator)
		}
		sb.WriteString(elem)
	}
	sb.WriteString(b.CloseBracket)

	return sb.String()
}

// formatLeftFlow formats elements using left-flow packing.
func (b *LeftFlowBreaker) formatLeftFlow(ctx BreakContext) BreakResult {
	contIndent := ctx.Indent.Continuation()
	contIndentStr := contIndent.String()
	contIndentWidth := contIndent.Width()

	var sb strings.Builder
	sb.WriteString(b.OpenBracket)
	sb.WriteByte('\n')
	sb.WriteString(contIndentStr)

	curCol := contIndentWidth
	first := true

	for _, elem := range ctx.Elements {
		elemWidth := ctx.Config.Width(elem)

		if first {
			sb.WriteString(elem)
			curCol += elemWidth
			first = false
			continue
		}

		// Check if element fits on current line
		sepWidth := ctx.Config.Width(b.Separator)
		if curCol+sepWidth+elemWidth <= ctx.Config.ColumnLimit {
			sb.WriteString(b.Separator)
			sb.WriteString(elem)
			curCol += sepWidth + elemWidth
		} else {
			// Break to new line
			sb.WriteString(",\n")
			sb.WriteString(contIndentStr)
			sb.WriteString(elem)
			curCol = contIndentWidth + elemWidth
		}
	}

	// Trailing comma and closing bracket
	if b.TrailingComma {
		sb.WriteString(",\n")
	} else {
		sb.WriteByte('\n')
	}
	sb.WriteString(ctx.Indent.String())
	sb.WriteString(b.CloseBracket)

	return BreakResult{
		Content: sb.String(),
		Broke:   true,
	}
}

// VerticalBreaker puts each element on its own line. Used for: multi-line
// function calls (one arg per line).
type VerticalBreaker struct {
	Separator     string // Separator (e.g., ",")
	OpenBracket   string // Opening bracket
	CloseBracket  string // Closing bracket
	TrailingComma bool   // Add trailing comma
}

// NewVerticalBreaker creates a new VerticalBreaker with default settings.
func NewVerticalBreaker() *VerticalBreaker {
	return &VerticalBreaker{
		Separator:     ",",
		OpenBracket:   "(",
		CloseBracket:  ")",
		TrailingComma: true,
	}
}

// Break implements Breaker for vertical layout.
func (b *VerticalBreaker) Break(ctx BreakContext) BreakResult {
	if len(ctx.Elements) == 0 {
		return BreakResult{
			Content: b.OpenBracket + b.CloseBracket,
			Broke:   false,
		}
	}

	contIndent := ctx.Indent.Continuation()
	contIndentStr := contIndent.String()

	var sb strings.Builder
	sb.WriteString(b.OpenBracket)
	sb.WriteByte('\n')

	for i, elem := range ctx.Elements {
		sb.WriteString(contIndentStr)
		sb.WriteString(strings.TrimSpace(elem))

		// Add separator
		if i < len(ctx.Elements)-1 || b.TrailingComma {
			sb.WriteString(b.Separator)
		}
		sb.WriteByte('\n')
	}

	sb.WriteString(ctx.Indent.String())
	sb.WriteString(b.CloseBracket)

	return BreakResult{
		Content: sb.String(),
		Broke:   true,
	}
}

// ElementSplitter defines how to split content into elements.
type ElementSplitter interface {
	Split(content string) []string
}

// CommaSplitter splits content by commas at depth 0.
type CommaSplitter struct {
	RespectAllBrackets bool // If true, respects (), [], and {}; otherwise just ()
}

// Split implements ElementSplitter.
func (s *CommaSplitter) Split(content string) []string {
	if s.RespectAllBrackets {
		return scanner.SplitTopLevelAny(content)
	}

	return scanner.SplitTopLevel(content)
}

// DefaultCommaSplitter creates a comma splitter that respects all bracket
// types.
func DefaultCommaSplitter() *CommaSplitter {
	return &CommaSplitter{RespectAllBrackets: true}
}
