package dsl

// ExpandFuncLitBodyRules returns DSL rules that expand single-line function
// literals into multi-line blocks.
func ExpandFuncLitBodyRules() []Rule {
	return []Rule{
		{
			Name:     "expand_func_lit_body",
			Pattern:  &NodePattern{Type: "FuncLit"},
			When:     TrueCond{},
			Priority: 24,
			Action: &ExpandFuncLitBodyAction{
				Target: "node",
			},
		},
	}
}
