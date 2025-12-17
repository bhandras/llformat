package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipeline_LegacyHardeningKnobs_IdempotentAndParseable(t *testing.T) {
	cfg := PipelineConfig{
		ColumnLimit: 50,
		TabStop:     8,

		// Turn on internal hardening/migration knobs together. This is intended
		// to reduce stage fighting and ensure the pipeline never emits invalid Go
		// even if a heuristic rewrite would otherwise do so.
		MultiLineUseASTSelect:   true,
		CompactCallUseASTSelect: true,
		LongExprUseASTSelect:    true,

		CompactCallParseSafe: true,
		MultiLineParseSafe:   true,
		LongExprParseSafe:    true,
	}

	p := NewPipeline(cfg)

	tests := []struct {
		name string
		src  []byte
	}{
		{
			name: "mixed_long_expr_and_call",
			src: []byte(`package p

func f() {
	_ = firstConditionThatIsVeryLong && outerFunctionNameThatIsVeryLong(innerFunctionNameThatIsVeryLong(1, 2, 3, 4, 5, 6, 7, 8, 9))
}
`),
		},
		{
			name: "multiline_call_only",
			src: []byte(`package p

func f() {
	_ = outerFunctionNameThatIsVeryLong(innerFunctionNameThatIsVeryLong(1, 2, 3, 4, 5, 6, 7, 8, 9))
}
`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out1 := p.Format(tc.src)
			out2 := p.Format(out1)
			require.Equal(t, string(out1), string(out2))

			// Parseable output.
			fset := token.NewFileSet()
			_, err := parser.ParseFile(fset, "out.go", out1, parser.AllErrors)
			require.NoError(t, err)
		})
	}
}

