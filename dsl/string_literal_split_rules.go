package dsl

// SplitLongStringLiteralOptions configures string literal splitting.
type SplitLongStringLiteralOptions struct {
	// MinTailLen avoids creating tiny trailing pieces when splitting at
	// spaces.
	MinTailLen int
}

// SplitLongStringLiteralRules returns DSL rules that split long quoted string
// literals in call-argument position into concatenations.
func SplitLongStringLiteralRules(opts SplitLongStringLiteralOptions) []Rule {
	return []Rule{
		{
			Name: "split_long_string_literal_in_call_args",
			Pattern: &NodePattern{
				Type: "BasicLit",
			},
			When: &IsCallArgCond{
				Target: "node",
			},
			Priority: 20,
			Action: &SplitLongStringLiteralAction{
				Target:     "node",
				MinTailLen: opts.MinTailLen,
			},
		},
	}
}
