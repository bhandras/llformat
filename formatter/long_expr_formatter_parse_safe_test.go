package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLongExprFormatter_ParseSafeDoesNotRewriteUnparseableSources(t *testing.T) {
	// Intentionally invalid Go. The legacy long-expr formatter historically
	// attempted to rewrite lines even when gofmt would fail, potentially
	// returning "more broken" output. ParseSafe mode should refuse changes.
	const in = `package p

func f() {
	if veryLongLeftHandSideNameThatIsVeryLong == veryLongRightHandSideNameThatIsVeryLong && anotherVeryLongConditionNameThatIsVeryLong {
		return
	}
`

	f := NewLongExprFormatter(LongExprConfig{
		ColumnLimit:   50,
		TabStop:       8,
		MaxIterations: 10,
		ParseSafe:     true,
	})

	out := f.FormatFile([]byte(in))
	require.Equal(t, in, string(out))
}

func TestLongExprFormatter_ParseSafeStillRewritesValidSources(t *testing.T) {
	const in = `package p

func f() bool {
	return veryLongLeftHandSideNameThatIsVeryLong == veryLongRightHandSideNameThatIsVeryLong && anotherVeryLongConditionNameThatIsVeryLong
}
`

	f := NewLongExprFormatter(LongExprConfig{
		ColumnLimit:   50,
		TabStop:       8,
		MaxIterations: 10,
		ParseSafe:     true,
	})

	out := f.FormatFile([]byte(in))
	require.NotEqual(t, in, string(out))
	require.Contains(t, string(out), "\n")

	// Semantic equivalence guard: whitespace-only changes must keep AST.
	requireASTEquivalent(t, []byte(in), out)
}

