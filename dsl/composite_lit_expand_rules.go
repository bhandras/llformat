package dsl

// ExpandCompositeLitRules returns DSL rules that expand composite literals into
// a multiline form when needed.
func ExpandCompositeLitRules() []Rule {
	rules := BreakCompositeKeyValueRules()

	return append(rules, Rule{
		Name: "expand_composite_lit",
		Pattern: &NodePattern{
			Type: "CompositeLit",
		},
		When:     TrueCond{},
		Priority: 25,
		Action: &ExpandCompositeLitAction{
			Target: "node",
		},
	})
}

// BreakCompositeKeyValueRules returns rules that break overlong keyed
// composite-literal values without expanding the whole literal.
func BreakCompositeKeyValueRules() []Rule {
	return []Rule{
		{
			Name: "break_composite_key_value",
			Pattern: &NodePattern{
				Type: "KeyValueExpr",
			},
			When:     TrueCond{},
			Priority: 26,
			Action: &BreakCompositeKeyValueAction{
				Target: "node",
			},
		},
	}
}
