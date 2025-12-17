package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipeline_LegacyHardening_MoreSnippets_IdempotentAndParseable(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "call_returning_func_then_call",
			src: `package p

func f(x int) func(int) int {
	return func(y int) int { return x + y }
}

func g() {
	_ = f(1)(2)
}
`,
		},
		{
			name: "paren_callee_call",
			src: `package p

func f() {
	_ = (veryLongIdentOne + veryLongIdentTwo + veryLongIdentThree)(x)
}
`,
		},
		{
			name: "selector_index_type_assert_chain",
			src: `package p

type T struct{ V any }

func f(v any) {
	_ = v.(interface{ Method() []T }).Method()[0].V
}
`,
		},
		{
			name: "index_list_generics_in_arg",
			src: `package p

func f[T any](v T) T { return v }

func g() {
	_ = outer(f[int](1), 2)
}
`,
		},
		{
			name: "func_lit_in_arg",
			src: `package p

func g() {
	_ = outer(func(x int) int { return x + 1 }, 2)
}
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := []byte(tc.src)
			p := NewPipeline(PipelineConfig{
				ColumnLimit:     48,
				TabStop:         8,
				LegacyHardening: true,
				UseDSLExpr:      false,
				UseDSLLogCalls:  false,
				UseDSLMultiLineCalls: false,
			})

			out1 := p.Format(in)
			out2 := p.Format(out1)
			require.Equal(t, string(out1), string(out2), "expected idempotence under LegacyHardening")
			requireASTEquivalent(t, in, out1)
		})
	}
}
