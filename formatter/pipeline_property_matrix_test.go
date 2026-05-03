package formatter

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipeline_PropertyMatrix_ParseableIdempotentASTEquivalent(
	t *testing.T) {

	type policy struct {
		name string
		cfg  PipelineConfig
	}

	policies := []policy{
		{
			name: "next",
			cfg:  PipelineConfig{},
		},
		{
			name: "next_with_ownership",
			cfg: PipelineConfig{
				UseOwnershipRegistry: true,
			},
		},
	}

	type style struct {
		name            string
		columnLimit     int
		tabStop         int
		moveInlineAbove bool
	}

	styles := []style{
		{
			name:        "col40_tab8",
			columnLimit: 40,
			tabStop:     8,
		},
		{
			name:        "col60_tab8",
			columnLimit: 60,
			tabStop:     8,
		},
		{
			name:        "col60_tab4",
			columnLimit: 60,
			tabStop:     4,
		},
		{
			name:            "col40_tab4_hoist_inline",
			columnLimit:     40,
			tabStop:         4,
			moveInlineAbove: true,
		},
	}

	snippets := []string{
		`package p

func f(a, b, c, d bool, x int) int {
	if (a && b && c) || d {
		return x + 1 + 2 + 3
	}
	return x
}
`,
		`package p

type S[T any] struct {
	V T
}

func f(m map[string]S[int], k string) int {
	return m[k].V
}
`,
		`package p

func g(x int) int { return x }

func f(x int) int {
	return g(g(g(x)))
}
`,
		`package p

func f(xs []int, i, j int) int {
	return xs[i:j][0]
}
`,
		`package p

func f(x any) bool {
	_, ok := x.(interface{ M() })
	return ok
}
`,
		`package p

type S struct {
	FieldA int
	FieldB int
	FieldC int
}

func f(s S) int {
	return s.FieldA + s.FieldB + s.FieldC
}
`,
		`package p

func f() {
	// Keep an inline comment inside args; argument rewrites should avoid
	// dropping it.
	_ = outerFunctionNameThatIsVeryLong(
		firstConditionThatIsVeryLong && secondConditionThatIsVeryLong, // keep me
		thirdConditionThatIsVeryLong || fourthConditionThatIsVeryLong,
	)
}
`,
		`package p

func f(a, b, c, d bool) int {
	switch {
	case a && b && c:
		return 1
	case d:
		// comment belongs to the case body
		return 2
	default:
		return 0
	}
}
`,
		`//go:build linux && amd64
// +build linux,amd64

package p

func f() int { return 1 }
`,
		`package p

import "fmt"

func f(s string) string {
	// raw string containing sequences that look like comments:
	_ = ` + "`" + `not a comment: // or /* */` + "`" + `
	return fmt.Sprintf("x=%q", s)
}
`,
		`package p

type I interface {
	M(a int, b int) (int, error) // trailing comment
}
`,
		`package p

func f() {
	_ = genericFunctionNameThatIsVeryLong[VeryLongTypeNameThatIsVeryLong, AnotherVeryLongTypeNameThatIsVeryLong](
		outerFunctionNameThatIsVeryLong(
			firstConditionThatIsVeryLong && secondConditionThatIsVeryLong,
			7,
		),
		42,
	)
}
`,
	}

	for policyIndex := range policies {
		pol := policies[policyIndex]
		t.Run(pol.name, func(t *testing.T) {
			for styleIndex := range styles {
				st := styles[styleIndex]
				t.Run(st.name, func(t *testing.T) {
					cfg := pol.cfg
					cfg.ColumnLimit = st.columnLimit
					cfg.TabStop = st.tabStop
					cfg.MoveInlineAbove = st.moveInlineAbove

					p := NewPipeline(cfg)

					for snippetIndex, in := range snippets {
						in := in
						t.Run(fmt.Sprintf(
							"snippet_%02d",
							snippetIndex), func(
							t *testing.T) {

							out1 := p.Format(
								[]byte(in),
							)
							out2 := p.Format(out1)

							require.NotEmpty(
								t, out1,
							)
							requireParseableGo(
								t, out1,
							)
							require.Equal(
								t, string(out1),
								string(out2),
								"not "+
									"ide"+
									"mpo"+
									"tent",
							)
							requireASTEquivalent(
								t, []byte(in),
								out1,
							)

							// For sources with
							// inline comments in
							// argument lists,
							// ensure we do not drop
							// the comment text as a
							// side effect of
							// AST-based printing.
							if snippetIndex == 6 {
								require.Contains(
									t,
									string(
										out1,
									),
									"kee"+
										"p me",
								)
							}
						})
					}
				})
			}
		})
	}
}
