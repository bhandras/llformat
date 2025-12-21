package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func requireParseableGo(t *testing.T, src []byte) {
	t.Helper()
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", src, parser.ParseComments)
	require.NoError(t, err)
}

func TestPipeline_Invariants_ParseableIdempotentASTEquivalent_AcrossModes(t *testing.T) {
	t.Parallel()

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

type S struct{ V int }

func f(m map[string]S, k string) int {
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

func f(x any) bool {
	_, ok := x.(interface{ M() })
	return ok
}
`,
		`package p

func f(xs []int, i, j int) int {
	return xs[i:j][0]
}
`,
	}

	configs := []struct {
		name string
		cfg  PipelineConfig
	}{
		{name: "next", cfg: PipelineConfig{Mode: "next", ColumnLimit: 60, TabStop: 8}},
		{
			name: "next_with_ownership",
			cfg:  PipelineConfig{Mode: "next", ColumnLimit: 60, TabStop: 8, UseOwnershipRegistry: true},
		},
	}

	for _, cfg := range configs {
		cfg := cfg
		t.Run(cfg.name, func(t *testing.T) {
			t.Parallel()

			p := NewPipeline(cfg.cfg)
			for _, in := range snippets {
				in := in
				t.Run("snippet", func(t *testing.T) {
					out1 := p.Format([]byte(in))
					out2 := p.Format(out1)

					requireParseableGo(t, out1)
					require.Equal(t, string(out1), string(out2), "not idempotent for config=%s", cfg.name)
					requireASTEquivalent(t, []byte(in), out1)
				})
			}
		})
	}
}
