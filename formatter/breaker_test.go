package formatter

import (
	"strings"
	"testing"
)

func TestLeftFlowBreakerSingleLine(t *testing.T) {
	breaker := NewLeftFlowBreaker()
	cfg := NewBaseConfig(80, 8)
	indent := NewIndent("\t", 8)

	ctx := BreakContext{
		Elements: []string{
			"a",
			"b",
			"c",
		},
		Indent:     indent,
		CurrentCol: 16, // After "funcName("
		Config:     cfg,
	}

	result := breaker.Break(ctx)
	if result.Broke {
		t.Error("Expected single line, got multiline")
	}
	if result.Content != "(a, b, c)" {
		t.Errorf("Content = %q, want %q", result.Content, "(a, b, c)")
	}
}

func TestLeftFlowBreakerMultiLine(t *testing.T) {
	breaker := NewLeftFlowBreaker()
	cfg := NewBaseConfig(40, 8) // Narrow limit
	indent := NewIndent("\t", 8)

	ctx := BreakContext{
		Elements: []string{
			"longArgument1",
			"longArgument2",
			"longArgument3",
		},
		Indent:     indent,
		CurrentCol: 20, // Starts mid-line
		Config:     cfg,
	}

	result := breaker.Break(ctx)
	if !result.Broke {
		t.Error("Expected multiline, got single line")
	}
	// Should have newlines
	if !strings.Contains(result.Content, "\n") {
		t.Error("Expected newlines in content")
	}
	// Should start with ( and end with )
	if !strings.HasPrefix(result.Content, "(") {
		t.Error("Expected to start with (")
	}
	if !strings.HasSuffix(result.Content, ")") {
		t.Error("Expected to end with )")
	}
}

func TestLeftFlowBreakerEmpty(t *testing.T) {
	breaker := NewLeftFlowBreaker()
	cfg := NewBaseConfig(80, 8)
	indent := NewIndent("", 8)

	ctx := BreakContext{
		Elements:   []string{},
		Indent:     indent,
		CurrentCol: 0,
		Config:     cfg,
	}

	result := breaker.Break(ctx)
	if result.Content != "()" {
		t.Errorf("Content = %q, want %q", result.Content, "()")
	}
}

func TestLeftFlowBreakerTrailingComma(t *testing.T) {
	breaker := &LeftFlowBreaker{
		Separator:     ", ",
		OpenBracket:   "(",
		CloseBracket:  ")",
		TrailingComma: true,
	}
	cfg := NewBaseConfig(30, 8)
	indent := NewIndent("", 8)

	ctx := BreakContext{
		Elements: []string{
			"longArg1",
			"longArg2",
		},
		Indent:     indent,
		CurrentCol: 15,
		Config:     cfg,
	}

	result := breaker.Break(ctx)
	if !result.Broke {
		t.Error("Expected multiline")
	}
	// Should have trailing comma before closing paren
	lines := strings.Split(result.Content, "\n")
	lastLine := lines[len(lines)-1]
	secondLastLine := lines[len(lines)-2]
	if !strings.HasSuffix(secondLastLine, ",") {
		t.Errorf("Expected trailing comma, got %q", secondLastLine)
	}
	if !strings.HasSuffix(lastLine, ")") {
		t.Errorf("Expected closing paren on last line, got %q",
			lastLine)
	}
}

func TestVerticalBreakerBasic(t *testing.T) {
	breaker := NewVerticalBreaker()
	cfg := NewBaseConfig(80, 8)
	indent := NewIndent("\t", 8)

	ctx := BreakContext{
		Elements: []string{
			"a",
			"b",
			"c",
		},
		Indent:     indent,
		CurrentCol: 0,
		Config:     cfg,
	}

	result := breaker.Break(ctx)
	if !result.Broke {
		t.Error("Vertical breaker should always break")
	}

	// Check structure
	lines := strings.Split(result.Content, "\n")
	if len(lines) != 5 { // (, a, b, c, )
		t.Errorf("Expected 5 lines, got %d", len(lines))
	}
	if lines[0] != "(" {
		t.Errorf("First line should be (, got %q", lines[0])
	}
	if !strings.HasSuffix(lines[len(lines)-1], ")") {
		t.Errorf("Last line should end with ), got %q",
			lines[len(lines)-1])
	}
}

func TestVerticalBreakerEmpty(t *testing.T) {
	breaker := NewVerticalBreaker()
	cfg := NewBaseConfig(80, 8)
	indent := NewIndent("", 8)

	ctx := BreakContext{
		Elements:   []string{},
		Indent:     indent,
		CurrentCol: 0,
		Config:     cfg,
	}

	result := breaker.Break(ctx)
	if result.Content != "()" {
		t.Errorf("Content = %q, want %q", result.Content, "()")
	}
}

func TestVerticalBreakerNoTrailingComma(t *testing.T) {
	breaker := &VerticalBreaker{
		Separator:     ",",
		OpenBracket:   "(",
		CloseBracket:  ")",
		TrailingComma: false,
	}
	cfg := NewBaseConfig(80, 8)
	indent := NewIndent("", 8)

	ctx := BreakContext{
		Elements: []string{
			"a",
			"b",
		},
		Indent:     indent,
		CurrentCol: 0,
		Config:     cfg,
	}

	result := breaker.Break(ctx)
	lines := strings.Split(result.Content, "\n")
	// Last element line should not have comma
	lastElemLine := lines[len(lines)-2]
	if strings.HasSuffix(lastElemLine, ",") {
		t.Errorf("Expected no trailing comma, got %q", lastElemLine)
	}
}

func TestCommaSplitter(t *testing.T) {
	splitter := DefaultCommaSplitter()

	tests := []struct {
		input string
		want  []string
	}{
		{
			"a, b, c",
			[]string{
				"a",
				"b",
				"c",
			},
		},
		{
			"a, (b, c), d",
			[]string{
				"a",
				"(b, c)",
				"d",
			},
		},
		{
			`a, "b,c", d`,
			[]string{
				"a",
				`"b,c"`,
				"d",
			},
		},
		{
			"a, {b, c}, d",
			[]string{
				"a",
				"{b, c}",
				"d",
			},
		},
	}

	for _, tt := range tests {
		got := splitter.Split(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("Split(%q) = %v, want %v", tt.input, got,
				tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("Split(%q)[%d] = %q, want %q",
					tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestCommaSplitterParensOnly(t *testing.T) {
	splitter := &CommaSplitter{RespectAllBrackets: false}

	// With parens-only, brackets and braces are not respected
	input := "a, [b, c], d"
	got := splitter.Split(input)
	// Should split at all commas since brackets aren't respected
	if len(got) != 4 {
		t.Errorf("Split(%q) = %v, expected 4 parts", input, got)
	}
}
