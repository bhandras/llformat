package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineDSLExpr_AutoCallArgs_OnlyForExcludedCallees_UserList(
	t *testing.T) {

	t.Parallel()

	const in = `package p

func f(a, b, c, d, e, f2, g bool) {
	// foo is excluded from multiline call formatting, so AutoDSLCallArgs
	// should allow breaking inside its argument expressions.
	_ = foo(a && b && c && d && e && f2 && g)

	// bar is not excluded, so AutoDSLCallArgs should not break inside its
	// args.
	_ = bar(a || b || c || d || e || f2 || g)
}

func foo(x bool) bool { return x }
func bar(x bool) bool { return x }
`

	p := NewPipeline(
		PipelineConfig{
			UseDSLExpr:      true,
			AutoDSLCallArgs: true,
			Excludes:        []string{"foo", "genericCall"},
			ColumnLimit:     40,
			TabStop:         8,
		},
	)

	out := string(p.Format([]byte(in)))
	require.Contains(t, out, "foo(")
	require.Contains(t, out, "bar(")

	// The foo call should have a broken chain.
	require.Contains(t, out, "foo(")
	require.Contains(t, out, "&&\n")

	// The bar call should remain unbroken within the `||` chain.
	require.NotContains(t, out, "||\n")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(
		fset, "out.go", []byte(out), parser.AllErrors,
	)
	require.NoError(t, err)
}

func TestPipelineDSLExpr_AutoCallArgs_AllowsNestedGenericCalleeBreaks(
	t *testing.T) {

	t.Parallel()

	const in = `package p

func f(a, b, c bool) {
	_ = foo(genericCall[VeryLongTypeNameOne, VeryLongTypeNameTwo, VeryLongTypeNameThree](
		firstConditionThatIsVeryLong && secondConditionThatIsVeryLong && thirdConditionThatIsVeryLong &&
			fourthConditionThatIsVeryLong && fifthConditionThatIsVeryLong && sixthConditionThatIsVeryLong &&
			seventhConditionThatIsVeryLong,
	))
}

func foo(x any) any { return x }
func genericCall[T1 any, T2 any, T3 any](x bool) bool { return x }
`

	p := NewPipeline(
		PipelineConfig{
			UseDSLExpr:      true,
			AutoDSLCallArgs: true,
			Excludes:        []string{"foo"},
			ColumnLimit:     40,
			TabStop:         8,
		},
	)

	out := string(p.Format([]byte(in)))

	// Ensure we can break both the nested call's callee type args and the
	// nested logical chain, while keeping the `](` coupling intact.
	require.Contains(t, out, "genericCall[")
	require.Contains(t, out, "VeryLongTypeNameOne,")
	require.Contains(t, out, "VeryLongTypeNameThree](")
	require.Contains(t, out, "&&\n")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(
		fset, "out.go", []byte(out), parser.AllErrors,
	)
	require.NoError(t, err)
}
