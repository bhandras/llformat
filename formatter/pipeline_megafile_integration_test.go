package formatter

import (
	formatstd "go/format"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipeline_MegaFile_InvariantsAcrossModes(t *testing.T) {
	t.Parallel()

	raw := []byte(`//go:build !ignore

package p

import (
	"fmt"
	"strings"
)

//go:generate echo "hi"

type Maybe[T any] struct {
	Value T
	Ok    bool
}

type Pair[A any, B any] struct {
	A A
	B B
}

type C[T any] interface {
	// Long comment that is intentionally written in an awkward way to force wrapping and reflow. It also contains tokens like // and /* */ which should not break directive detection.
	M(x int, y string) error
	N(x T) (T, error)
}

func f[T ~string](x T, y []Pair[int, string]) (Maybe[T], error) {
	// A raw string that looks like it contains comments should stay intact:
	raw := "line1 // not a comment\nline2 /* not a comment */"
	_ = raw

	// Nested generic calls + type assertions + selector chains.
	s := strings.NewReplacer("a", "b", "c", "d").Replace(fmt.Sprintf("%v", x))
	_ = s

	// A deeply nested expression intended to exercise the expression formatter.
	_ = (((firstConditionThatIsVeryLong && secondConditionThatIsVeryLong) || thirdConditionThatIsVeryLong) &&
		(fourthConditionThatIsVeryLong || fifthConditionThatIsVeryLong) &&
		sixthConditionThatIsVeryLong)

	// A call with tricky arguments: nested calls, composites, and long logical chains.
	_ = outerFunctionNameThatIsVeryLong(
		strings.TrimSpace(strings.ToUpper(fmt.Sprintf("x=%v", x))),
		map[string][]Pair[int, string]{
			"k": {
				{A: 1, B: "one"},
				{A: 2, B: "two"},
			},
		}["k"][0:len(y):cap(y)],
		(firstConditionThatIsVeryLong && secondConditionThatIsVeryLong && thirdConditionThatIsVeryLong && fourthConditionThatIsVeryLong),
	)

	// Switch with comments around case/return to exercise blank-line behavior.
	switch {
	// comment above first case
	case x == "a":
		// comment above return
		return Maybe[T]{Value: x, Ok: true}, nil
	// comment above second case
	case x == "b":
		return Maybe[T]{Value: x, Ok: true}, nil
	default:
		return Maybe[T]{}, fmt.Errorf("bad: %v", x)
	}
}

func outerFunctionNameThatIsVeryLong(args ...any) any {
	return args
}
`)

	in, err := formatstd.Source(raw)
	require.NoError(t, err)

	run := func(t *testing.T, cfg PipelineConfig) {
		t.Helper()

		out1 := NewPipeline(cfg).Format(in)

		// Must remain parseable; go/format is a convenient parse check.
		_, err := formatstd.Source(out1)
		require.NoError(t, err)

		// Formatting is allowed to move comments/whitespace, but must preserve
		// AST structure for valid Go sources.
		requireASTEquivalent(t, in, out1)

		// Must converge quickly. Some legacy formatting passes can stabilize
		// layout over multiple runs; require convergence within 2 additional runs.
		out2 := NewPipeline(cfg).Format(out1)
		out3 := NewPipeline(cfg).Format(out2)
		require.Equal(t, string(out2), string(out3))
	}

	t.Run("next_with_ownership", func(t *testing.T) {
		run(t, PipelineConfig{
			Mode:                 "next",
			RuleProfile:          "next",
			ColumnLimit:          60,
			TabStop:              8,
			UseOwnershipRegistry: true,
			Excludes:             []string{"outerFunctionNameThatIsVeryLong"},
		})
	})
}
