package dsl

// ExpandCompositeLitRules returns DSL rules that expand composite literals into
// a multiline form when needed.
func ExpandCompositeLitRules() []Rule {
	return []Rule{
		{
			Name:     "expand_composite_lit",
			Pattern:  &NodePattern{Type: "CompositeLit"},
			When:     TrueCond{},
			Priority: 25,
			Action: &ExpandCompositeLitAction{
				Target: "node",
			},
		},
	}
}
