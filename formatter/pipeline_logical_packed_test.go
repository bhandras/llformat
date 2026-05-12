package formatter

import (
	"strings"
	"testing"

	"github.com/bhandras/llformat/dsl"
	"github.com/stretchr/testify/require"
)

func TestPipelineLogicalPackedBreaksOverflowingContinuationLine(t *testing.T) {
	t.Parallel()

	const in = `package p

func f(itemType string) {
	for {
		if itemType != "" && itemType != "text" &&
			itemType != "input_text" && itemType != "output_text" {
			continue
		}
	}
}
`

	out := string(
		NewPipeline(
			PipelineConfig{
				ColumnLimit: 72,
				TabStop:     8,
			}).Format([]byte(in)),
	)

	require.Contains(
		t, out, "itemType != \"input_text\" "+
			"&&\n			itemType != \"output_text\" {",
	)
	require.NotContains(
		t, out,
		"itemType != \"input_text\" && itemType != \"output_text\" {",
	)
	require.Contains(t, out, "{\n\n\t\t\tcontinue")
	requireNoOverflowingLineInSnippet(t, out, "itemType !=", 72)
}

func TestPipelineLogicalPackedPreservesParenthesizedGroups(t *testing.T) {
	t.Parallel()

	const in = `package p

func f(firstConditionName, secondConditionName, thirdConditionName, fourthConditionName, fallbackEnabled bool) {
	if (firstConditionName && secondConditionName) || (thirdConditionName && fourthConditionName) || fallbackEnabled {
		doSomething()
	}
}

func doSomething() {}
`

	out := string(
		NewPipeline(
			PipelineConfig{
				ColumnLimit: 72,
				TabStop:     8,
			}).Format([]byte(in)),
	)

	require.Contains(
		t, out, "(firstConditionName && secondConditionName) ||\n",
	)
	require.Contains(
		t, out, "(thirdConditionName && fourthConditionName) ||\n",
	)
	require.NotContains(t, out, "(firstConditionName &&\n")
	require.NotContains(t, out, "(thirdConditionName &&\n")
	require.Contains(t, out, "{\n\n\t\tdoSomething()")
	requireNoOverflowingLineInSnippet(t, out, "ConditionName", 72)
}

func requireNoOverflowingLineInSnippet(t *testing.T, src, marker string,
	colLimit int) {

	t.Helper()

	for _, line := range strings.Split(src, "\n") {
		if !strings.Contains(line, marker) {
			continue
		}
		require.LessOrEqual(
			t, dsl.VisualLen(line, DefaultTabStop), colLimit, "l"+
				"ine overflows: %q", line,
		)
	}
}
