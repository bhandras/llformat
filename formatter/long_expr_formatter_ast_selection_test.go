package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLongExprFormatter_ASTSelectionDoesNotTouchCallArgsOnly(t *testing.T) {
	in := []byte(`package p

func f() {
	_ = outerFunctionNameThatIsVeryLong(innerFunctionNameThatIsVeryLong(firstConditionThatIsVeryLong && secondConditionThatIsVeryLong && thirdConditionThatIsVeryLong && fourthConditionThatIsVeryLong), 42)
}
`)

	f := NewLongExprFormatter(LongExprConfig{
		ColumnLimit:     48,
		TabStop:         8,
		MaxIterations:   10,
		ParseSafe:       true,
		UseASTSelection: true,
	})
	out := f.FormatFile(in)

	// With AST selection enabled, the long-expr formatter should avoid rewriting
	// inside call args entirely; since this file has no other long lines, it
	// should be unchanged.
	require.Equal(t, string(in), string(out))
	requireASTEquivalent(t, in, out)
}

func TestLongExprFormatter_ASTSelectionRewritesStandaloneLongExprButNotCallArgs(t *testing.T) {
	in := []byte(`package p

func f() {
	_ = firstConditionThatIsVeryLong && secondConditionThatIsVeryLong && thirdConditionThatIsVeryLong && fourthConditionThatIsVeryLong
	_ = outerFunctionNameThatIsVeryLong(innerFunctionNameThatIsVeryLong(firstConditionThatIsVeryLong && secondConditionThatIsVeryLong && thirdConditionThatIsVeryLong && fourthConditionThatIsVeryLong), 42)
}
`)

	f := NewLongExprFormatter(LongExprConfig{
		ColumnLimit:     48,
		TabStop:         8,
		MaxIterations:   10,
		ParseSafe:       true,
		UseASTSelection: true,
	})
	out := f.FormatFile(in)
	outStr := string(out)

	// The standalone long line should be broken across lines.
	require.Contains(t, outStr, "\n\t\tsecondConditionThatIsVeryLong")

	// The call-arg long expression should remain unbroken.
	require.NotContains(t, outStr, "innerFunctionNameThatIsVeryLong(firstConditionThatIsVeryLong &&\n")

	requireASTEquivalent(t, in, out)
}
