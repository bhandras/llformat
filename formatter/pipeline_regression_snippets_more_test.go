package formatter

import (
	"fmt"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipeline_RegressionSnippets_MoreASTConstructs(t *testing.T) {
	// This suite is intentionally non-golden: it aims to widen coverage across
	// tricky-but-valid Go syntax without over-constraining exact output.
	//
	// We assert only:
	// - output is parseable (both first and second pass)
	// - output is idempotent (Format(Format(x)) == Format(x))
	// - output is AST-equivalent to the input (comments ignored)
	//
	// Additionally, a few "directive as comment" cases assert that directives
	// survive formatting unchanged because reflowing them can break tooling.
	type policy struct {
		name string
		cfg  PipelineConfig
	}

	policies := []policy{
		{name: "next", cfg: PipelineConfig{ColumnLimit: 48, TabStop: 8}},
		{name: "next_with_ownership", cfg: PipelineConfig{ColumnLimit: 48, TabStop: 8, UseOwnershipRegistry: true}},
	}

	type tc struct {
		name         string
		src          string
		wantContains []string
	}

	cases := []tc{
		{
			name: "select_and_go_defer",
			src: `package p

func f(ch chan int, done <-chan struct{}) int {
	defer func() {
		_ = recover()
	}()
	go func() {
		ch <- 1
	}()

	select {
	case v := <-ch:
		return v
	case <-done:
		return 0
	}
}
`,
		},
		{
			name: "type_switch_with_init",
			src: `package p

func f(x any) int {
	switch v := x.(type) {
	case int:
		return v
	case interface{ M(int) int }:
		return v.M(7)
	default:
		return 0
	}
}
`,
		},
		{
			name: "label_goto_and_comments",
			src: `package p

func f(n int) int {
start:
	// comment stays with statement
	if n <= 0 {
		return 0
	}
	n--
	if n == 1 {
		goto start
	}
	return n
}
`,
		},
		{
			name: "range_and_assign_ok",
			src: `package p

func f(m map[string]int, xs []int) (int, bool) {
	sum := 0
	for _, v := range xs {
		sum += v
	}
	_, ok := m["k"]
	return sum, ok
}
`,
		},
		{
			name: "method_expressions",
			src: `package p

type T struct{ V int }

func (t T) M(x int) int { return t.V + x }

func f() int {
	var t T
	m1 := T.M
	m2 := (T).M
	_ = m1
	_ = m2
	return t.M(7)
}
`,
		},
		{
			name: "generic_constraints_and_instantiation",
			src: `package p

type Number interface{ ~int | ~int64 }

type Box[T any] struct{ V T }

func Add[T Number](a, b T) T { return a + b }

func f() int {
	b := Box[int]{V: Add[int](1, 2)}
	return b.V
}
`,
		},
		{
			name: "generic_index_list_expr_in_selector",
			src: `package p

type S[T any] struct{ V T }

func F[T any](x T) S[T] { return S[T]{V: x} }

func f() int {
	v := F[int](7).V
	return v
}
`,
		},
		{
			name: "cgo_directives_block_and_export",
			src: `package p

/*
#cgo CFLAGS: -I./include
#include <stdint.h>
*/
import "C"

//export ExportedName
func ExportedName(x C.int) C.int {
	return x + 1
}
`,
			wantContains: []string{
				"/*\n#cgo CFLAGS: -I./include\n#include <stdint.h>\n*/\n",
				"//export ExportedName\n",
			},
		},
		{
			name: "line_directive",
			src: `package p

//line generated.go:123
func f() int { return 1 }
`,
			wantContains: []string{"//line generated.go:123\n"},
		},
		{
			name: "parenthesized_composite_and_func_lits",
			src: `package p

func f() int {
	type local struct {
		F func(int) int
	}
	x := (local{
		F: func(v int) int { return v + 1 },
	})
	return x.F(7)
}
`,
		},
	}

	for policyIndex := range policies {
		pol := policies[policyIndex]
		t.Run(pol.name, func(t *testing.T) {
			p := NewPipeline(pol.cfg)

			for _, c := range cases {
				t.Run(c.name, func(t *testing.T) {
					out1 := p.Format([]byte(c.src))
					out2 := p.Format(out1)

					require.NotEmpty(t, out1)
					require.Equal(t, string(out1), string(out2), "not idempotent")

					fset := token.NewFileSet()
					_, err := parser.ParseFile(fset, "out.go", out1, parser.AllErrors|parser.ParseComments)
					require.NoError(t, err, "formatted output was not parseable:\n%s", string(out1))
					_, err = parser.ParseFile(fset, "out2.go", out2, parser.AllErrors|parser.ParseComments)
					require.NoError(t, err, "second pass output was not parseable:\n%s", string(out2))

					requireASTEquivalent(t, []byte(c.src), out1)
					requireASTEquivalent(t, []byte(c.src), out2)

					for _, want := range c.wantContains {
						require.Contains(t, string(out1), want, fmt.Sprintf("missing required substring %q", want))
					}
				})
			}
		})
	}
}
