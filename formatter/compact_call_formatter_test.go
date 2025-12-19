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

func TestBuildSplitQuotedForCallArg_UsesGofmtCompatibleSpacingWhenTrailingArgs(t *testing.T) {
	// When a split string literal is a call argument and there are additional args
	// after it, gofmt may elide the space before '+' when the segment ends with a
	// space. We mirror that behavior for idempotence and to preserve one extra
	// column of budget.
	out := buildSplitQuotedForCallArg(
		"unable to lookup peer alias: %v with a tail that forces wrapping",
		0,
		"\t",
		36,
		true,
	)
	require.Contains(t, out, "\"+\n\t\t\"", "expected at least one join rendered as `\"+\\n` when the segment ends with a space")
}

func TestFormatCallPackedMultiLine_DoesNotOverBreakAfterSplitStringUnderDeepIndent(t *testing.T) {
	// Regression test: when a string arg is split across multiple lines, the
	// packed multiline call formatter must compute the "current column" after the
	// split based on the split string's last line (which already includes its own
	// indentation), otherwise we can break subsequent args unnecessarily under
	// deeper indentation.
	call := []byte(`ProcessData(ctx, "some long string argument that makes the line exceed the configured limit", 42, true)`)
	out := FormatCallPackedMultiLine(call, "\t\t", 80, 8)

	// When the following args fit, keep them on the same line as the final string
	// segment.
	require.Contains(t, out, `", 42, true,`)
	require.NotContains(t, out, "\",\n\t\t\t42", "must not break before 42 when it fits after the split string")
}

func TestFormatCallPackedMultiLine_DoesNotReflowNestedLenCallWhenItFitsOnItsOwnLine(t *testing.T) {
	// Regression test: nested calls like len(...) should not be recursively
	// reformatted into packed multiline when they would fit on their own
	// continuation line. This avoids ugly results like:
	//   len(
	//     x,
	//   )
	call := []byte(`make([][]byte, 0, len(chanBackupsProtos.ChanBackups))`)
	out := FormatCallPackedMultiLineNext(call, "\t\t", 60, 8)
	require.Contains(t, out, "len(chanBackupsProtos.ChanBackups)")
	require.NotContains(t, out, "len(\n", "must not expand len(...) into a nested multiline call when it fits as-is")
}

func TestFormatCallGreedy_DoesNotSplitShortFormatStringToFitCommaSpace(t *testing.T) {
	// Regression test for "early break" in deeply indented return statements:
	// when the format string itself fits on the current line, but there isn't
	// enough room for a trailing ", " before the next arg, we prefer keeping the
	// format string intact and breaking before the next argument rather than
	// splitting the string into `"..." +\n"...", err`.
	call := []byte(`fmt.Errorf("error parsing psbt: %w", err)`)

	// Choose a baseLen such that the quoted format string ends at column 79,
	// leaving room for a comma but not for ", " (comma+space).
	//
	// In formatCallGreedy:
	//   curLen starts at baseLen + len("fmt.Errorf") + 1.
	// The quoted string literal here is 24 columns wide.
	//
	// len("fmt.Errorf") is 10, so curLen = baseLen + 11.
	// We want curLen + 24 == 79 => curLen == 55 => baseLen == 44.
	out := FormatCallGreedy(call, "\t\t\t\t\t", 44, 80, 8)

	require.Contains(t, out, `"error parsing psbt: %w",`)
	require.Contains(t, out, "\n\t\t\t\t\t\terr")
	require.NotContains(t, out, "\" +\n", "must not split a short format string just to make room for a following space")
}

func TestFormatCallGreedyNext_JoinAwareSplit_PreservesSpaceBeforePlus(t *testing.T) {
	// Regression test for "peer alias" getting collapsed to "peeralias" due to
	// overly conservative width accounting around `+` when there are trailing
	// call args.
	//
	// With a tight column limit, we want the first split to be able to end at a
	// space boundary and still fit the join when gofmt would render `"..." +`
	// as `"..." +` or `"..." +` vs `"..." +` (context-sensitive).
	call := []byte(`fmt.Errorf("unable to lookup peer alias more: %v", err)`)
	out := FormatCallGreedyNext(call, "", 0, 36, 8)

	// Ensure the split preserves the space boundary before `+`, allowing the
	// next segment to start with "alias..." rather than joining words.
	require.Contains(t, out, "\"unable to lookup peer \"+\n\t\"alias", "expected join-aware split at the `peer ` boundary")
	require.NotContains(t, out, "\"peer\"+\n\t\"alias", "must not drop the trailing space at the split point")
}
