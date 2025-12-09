package formatter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lightninglabs/llformat/dsl"
	"github.com/stretchr/testify/require"
)

func TestBlankLinesGolden(t *testing.T) {
	dir := filepath.Join("..", "testdata", "blanklines")
	inPath := filepath.Join(dir, "input.go")
	outPath := filepath.Join(dir, "output.go")

	if _, err := os.Stat(inPath); err != nil {
		t.Skipf("skipping: %s not present (add testdata/blanklines)", inPath)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Skipf("skipping: %s not present (add testdata/blanklines)", outPath)
	}

	in, err := os.ReadFile(inPath)
	if err != nil {
		t.Fatal(err)
	}
	f := NewBlankLineFormatter(BlankLineConfig{
		BeforeReturn:            true,
		BetweenCases:            true,
		BetweenInterfaceMethods: true,
	})
	got := f.FormatFile(in)

	want, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	ng, nw := normalizeBlankLines(string(got)), normalizeBlankLines(string(want))
	require.Equal(t, nw, ng)
}

func normalizeBlankLines(s string) string {
	// Normalize newlines and trim trailing newlines for a stable comparison
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimRight(s, "\n")

	// Normalize multiple spaces to single space (gofmt alignment differences)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		// Collapse multiple spaces outside of strings
		// Simple approach: collapse runs of spaces
		var result strings.Builder
		inSpace := false
		for _, r := range line {
			if r == ' ' || r == '\t' {
				if !inSpace {
					result.WriteRune(' ')
					inSpace = true
				}
			} else {
				result.WriteRune(r)
				inSpace = false
			}
		}
		lines[i] = strings.TrimRight(result.String(), " ")
	}

	return strings.Join(lines, "\n")
}

func TestBlankLinesGoldenDSL(t *testing.T) {
	dir := filepath.Join("..", "testdata", "blanklines")
	inPath := filepath.Join(dir, "input.go")
	outPath := filepath.Join(dir, "output.go")

	if _, err := os.Stat(inPath); err != nil {
		t.Skipf("skipping: %s not present (add testdata/blanklines)", inPath)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Skipf("skipping: %s not present (add testdata/blanklines)", outPath)
	}

	in, err := os.ReadFile(inPath)
	if err != nil {
		t.Fatal(err)
	}

	// Use DSL formatter with only blank line rules
	f := NewDSLExprFormatter(DSLExprConfig{
		ColumnLimit: 80,
		TabStop:     8,
		Rules:       dsl.BlankLineRules(),
	})
	got := f.FormatFile(in)

	want, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	ng, nw := normalizeBlankLines(string(got)), normalizeBlankLines(string(want))
	require.Equal(t, nw, ng)
}

func TestClassifyLine(t *testing.T) {
	f := NewBlankLineFormatter(BlankLineConfig{})

	tests := []struct {
		trimmed     string
		inSwitch    bool
		inInterface bool
		want        string
	}{
		// Basic cases
		{"", false, false, "blank"},
		{"{", false, false, "open_brace"},
		{"}", false, false, "close_brace"},

		// Switch-related
		{"switch x {", false, false, "switch"},
		{"switch {", false, false, "switch"},
		{"switch", false, false, "switch"},
		{"case 1:", true, false, "case"},
		{"case foo:", true, false, "case"},
		{"default:", true, false, "case"},
		{"case 1:", false, false, "other"}, // not in switch

		// Return statements
		{"return", false, false, "return"},
		{"return nil", false, false, "return"},
		{"return (x)", false, false, "return"},
		{"returnValue := 1", false, false, "other"}, // variable name

		// Interface-related
		{"type Foo interface {", false, false, "interface_open"},
		{"io.Reader", false, true, "embedded_interface"},
		{"Read(p []byte) (n int, err error)", false, true, "interface_method"},

		// Regular code
		{"x := 1", false, false, "other"},
		{"fmt.Println(x)", false, false, "other"},
	}

	for _, tt := range tests {
		got := f.classifyLine(tt.trimmed, tt.inSwitch, tt.inInterface)
		if got != tt.want {
			t.Errorf("classifyLine(%q, inSwitch=%v, inInterface=%v) = %q, want %q",
				tt.trimmed, tt.inSwitch, tt.inInterface, got, tt.want)
		}
	}
}
