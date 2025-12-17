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

func TestLongExprFormatter_ASTSelectionCanBreakOutsideCallArgs(t *testing.T) {
	in := []byte(`package p

func f() {
	_ = firstConditionThatIsVeryLong && outerFunctionNameThatIsVeryLong(innerFunctionNameThatIsVeryLong(firstConditionThatIsVeryLong && secondConditionThatIsVeryLong && thirdConditionThatIsVeryLong && fourthConditionThatIsVeryLong), 42)
}
`)

	f := NewLongExprFormatter(LongExprConfig{
		ColumnLimit:     62,
		TabStop:         8,
		MaxIterations:   10,
		ParseSafe:       true,
		UseASTSelection: true,
	})
	out := f.FormatFile(in)
	outStr := string(out)

	// The break should happen at the outer operator, even though the line
	// contains a forbidden call-arg region later on the same line.
	require.Contains(t, outStr, "firstConditionThatIsVeryLong &&\n\t\touterFunctionNameThatIsVeryLong(")

	// The inner call-arg long expression should remain unbroken.
	require.NotContains(t, outStr, "innerFunctionNameThatIsVeryLong(firstConditionThatIsVeryLong &&\n")

	requireASTEquivalent(t, in, out)
}

func TestLongExprFormatter_ASTSelectionDoesNotTouchCompositeLiteralBodies(t *testing.T) {
	in := []byte(`package p

func f() {
	_ = map[string]bool{
		"a": firstConditionThatIsVeryLong && secondConditionThatIsVeryLong && thirdConditionThatIsVeryLong && fourthConditionThatIsVeryLong,
	}
}
`)

	f := NewLongExprFormatter(LongExprConfig{
		ColumnLimit:     60,
		TabStop:         8,
		MaxIterations:   10,
		ParseSafe:       true,
		UseASTSelection: true,
	})
	out := f.FormatFile(in)

	// The only long expression lives inside the composite literal body, which
	// is a forbidden region for the long-expr formatter under AST selection.
	require.Equal(t, string(in), string(out))
	requireASTEquivalent(t, in, out)
}

func TestLongExprFormatter_ASTSelectionCanBreakBeforeCompositeLiteralInsideCallArgs(t *testing.T) {
	in := []byte(`package p

func f() {
	_ = firstConditionThatIsVeryLong && len([]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
}
`)

	f := NewLongExprFormatter(LongExprConfig{
		ColumnLimit:     62,
		TabStop:         8,
		MaxIterations:   10,
		ParseSafe:       true,
		UseASTSelection: true,
	})
	out := f.FormatFile(in)
	outStr := string(out)

	require.Contains(t, outStr, "firstConditionThatIsVeryLong &&\n\t\tlen([]int{")
	requireASTEquivalent(t, in, out)
}

func TestLongExprFormatter_ASTSelectionExcludeCallExprsSkipsCalleeBreaks(t *testing.T) {
	in := []byte(`package p

func f() {
	_ = (firstVeryLongIdentifier + secondVeryLongIdentifier + thirdVeryLongIdentifier)(x)
}
`)

	cfg := LongExprConfig{
		ColumnLimit:     48,
		TabStop:         8,
		MaxIterations:   10,
		ParseSafe:       true,
		UseASTSelection: true,
	}

	// Without ExcludeCallExprs, the formatter is allowed to break inside the
	// callee of `(a + b)(x)` because it's outside the call arg list.
	without := NewLongExprFormatter(cfg).FormatFile(in)
	require.NotEqual(t, string(in), string(without))
	require.Contains(t, string(without), "\n\t\t")
	require.Contains(t, string(without), "(firstVeryLongIdentifier")
	require.Contains(t, string(without), ")(x)")
	requireASTEquivalent(t, in, without)

	// With ExcludeCallExprs enabled, the entire call expression is forbidden and
	// should remain unchanged.
	cfg.ExcludeCallExprs = true
	with := NewLongExprFormatter(cfg).FormatFile(in)
	require.Equal(t, string(in), string(with))
	requireASTEquivalent(t, in, with)
}
