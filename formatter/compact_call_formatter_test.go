package formatter

import (
	formatstd "go/format"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lightninglabs/llformat/dsl"
	"github.com/stretchr/testify/require"
)

func TestLogsExamples(t *testing.T) {
	dir := filepath.Join("..", "testdata", "logs")
	in, err := os.ReadFile(filepath.Join(dir, "input.go"))
	if err != nil {
		t.Fatal(err)
	}
	f := NewCompactCallFormatter(Config{})
	got := f.FormatFile(in)
	want, err := os.ReadFile(filepath.Join(dir, "output.go"))
	if err != nil {
		t.Fatal(err)
	}
	if formatted, err := formatstd.Source(want); err == nil {
		want = formatted
	}
	require.Equal(t, string(want), string(got))
}

func TestLogsExamplesDSL(t *testing.T) {
	dir := filepath.Join("..", "testdata", "logs")
	in, err := os.ReadFile(filepath.Join(dir, "input.go"))
	if err != nil {
		t.Fatal(err)
	}

	// Use DSL formatter with only log/printf rules, wired to legacy formatter
	f := NewDSLExprFormatter(DSLExprConfig{
		ColumnLimit: 80,
		TabStop:     8,
		Rules:       dsl.LogPrintfRules(FormatCallGreedy),
	})
	got := f.FormatFile(in)

	want, err := os.ReadFile(filepath.Join(dir, "output.go"))
	if err != nil {
		t.Fatal(err)
	}
	if formatted, err := formatstd.Source(want); err == nil {
		want = formatted
	}
	require.Equal(t, string(want), string(got))
}

func TestBuildSplitQuoted_DoesNotEmitDanglingPlusWhenIndentTooDeep(t *testing.T) {
	// When the indentation itself exceeds the configured width, splitting a
	// string literal cannot make it fit. We should still emit valid Go (no
	// dangling '+' followed by a comma/newline in the caller).
	out := buildSplitQuoted("%s", 200, "\t\t", 48)
	trimmed := strings.TrimRight(out, " \t")
	require.NotEmpty(t, trimmed)
	require.NotEqual(t, '+', trimmed[len(trimmed)-1], "split output must not end with '+'")
	require.NotContains(t, out, "+\n\t\t\t,", "must not end with a dangling '+', then comma on the next line")
}

func TestBuildSplitQuoted_UsesGofmtCompatibleSpacingAroundPlus(t *testing.T) {
	t.Run("space_split", func(t *testing.T) {
		out := buildSplitQuoted("this is a very long string that should split on spaces", 0, "\t", 36)
		require.Contains(t, out, "\" +\n\t\t\"")
		require.NotContains(t, out, "\"+\n", "must not emit a quote immediately followed by '+' and newline")
	})

	t.Run("hard_split_no_spaces", func(t *testing.T) {
		out := buildSplitQuoted("this_is_a_very_long_string_with_no_spaces_so_it_must_hard_split", 0, "\t", 36)
		require.Contains(t, out, "\" +\n\t\t\"")
		require.NotContains(t, out, "\"+\n", "must not emit a quote immediately followed by '+' and newline")
	})
}
