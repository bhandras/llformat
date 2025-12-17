package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipeline_LegacyASTSelectionKnobs_IdempotentAndASTEquivalent(t *testing.T) {
	cfg := PipelineConfig{
		ColumnLimit: 50,
		TabStop:     8,

		// Turn on all internal hardening/migration knobs together. This is the
		// configuration that historically risked stage fighting/oscillation.
		MultiLineUseASTSelect:   true,
		CompactCallUseASTSelect: true,
		LongExprParseSafe:       true,
		LongExprUseASTSelect:    true,
	}

	p := NewPipeline(cfg)

	tests := []struct {
		name string
		src  []byte
	}{
		{
			name: "standalone_long_logical_chain",
			src: []byte(`package p

func f() {
	_ = firstConditionThatIsVeryLong && secondConditionThatIsVeryLong && thirdConditionThatIsVeryLong && fourthConditionThatIsVeryLong
}
`),
		},
		{
			name: "long_call_with_nested_long_expr_arg",
			src: []byte(`package p

func f() {
	_ = outerFunctionNameThatIsVeryLong(innerFunctionNameThatIsVeryLong(firstConditionThatIsVeryLong && secondConditionThatIsVeryLong && thirdConditionThatIsVeryLong && fourthConditionThatIsVeryLong), 42)
}
`),
		},
		{
			name: "mixed_outside_operator_then_call",
			src: []byte(`package p

func f() {
	_ = firstConditionThatIsVeryLong && outerFunctionNameThatIsVeryLong(innerFunctionNameThatIsVeryLong(firstConditionThatIsVeryLong && secondConditionThatIsVeryLong && thirdConditionThatIsVeryLong && fourthConditionThatIsVeryLong), 42)
}
`),
		},
		{
			name: "composite_literal_value_long_expr",
			src: []byte(`package p

func f() {
	_ = map[string]bool{
		"a": firstConditionThatIsVeryLong && secondConditionThatIsVeryLong && thirdConditionThatIsVeryLong && fourthConditionThatIsVeryLong,
	}
}
`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out1 := p.Format(tc.src)
			out2 := p.Format(out1)
			require.Equal(t, string(out1), string(out2))
			requireASTEquivalent(t, tc.src, out1)
		})
	}
}

