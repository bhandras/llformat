package dsl

// DefaultRules returns the standard formatting rules.
func DefaultRules() []Rule {
	return []Rule{
		// Rule 1: Never break simple comparisons (x > 0, flag == true, etc.)
		{
			Name: "keep_simple_comparison",
			Pattern: &NodePattern{
				Type: "BinaryExpr",
				Fields: map[string]FieldMatch{
					"op":    {OneOf: []string{"==", "!=", "<", ">", "<=", ">="}},
					"right": {Capture: "r"},
				},
			},
			When:     &IsSimpleLiteralCond{Target: "r"},
			Priority: 100,
			Action:   &KeepTogetherAction{Target: "node"},
		},

		// Rule 2: If condition with call and simple comparison
		// e.g., if len(foo) > 10 { ... }
		{
			Name: "if_call_comparison",
			Pattern: &NodePattern{
				Type: "IfStmt",
				Fields: map[string]FieldMatch{
					"cond": {
						Capture: "cond",
						SubPattern: &NodePattern{
							Type: "BinaryExpr",
							Fields: map[string]FieldMatch{
								"left": {
									Capture:    "call",
									SubPattern: &NodePattern{Type: "CallExpr"},
								},
								"op":    {OneOf: []string{"==", "!=", "<", ">", "<=", ">="}},
								"right": {Capture: "r"},
							},
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{Target: "node", Op: ">", Value: 0},
					&IsSimpleLiteralCond{Target: "r"},
				},
			},
			Priority: 60,
			Action: &ReflowCallAction{
				Target:   "call",
				Strategy: StrategyOnePerLine,
			},
		},

		// Rule 3: Assignment with long function call
		// e.g., result := someFunc(arg1, arg2, arg3)
		{
			Name: "assignment_with_long_call",
			Pattern: &NodePattern{
				Type: "AssignStmt",
				Fields: map[string]FieldMatch{
					"lhs": {Capture: "var"},
					"rhs": {
						Capture:    "rhs",
						SubPattern: &NodePattern{Type: "CallExpr"},
					},
				},
			},
			When:     &LineWidthCond{Target: "node", Op: ">", Value: 0},
			Priority: 50,
			Action: &ReflowCallAction{
				Target:   "rhs",
				Strategy: StrategyOnePerLine,
			},
		},

		// Rule 4: Long logical chain with function calls - try reflow first
		{
			Name: "logical_chain_with_call",
			Pattern: &NodePattern{
				Type: "BinaryExpr",
				Fields: map[string]FieldMatch{
					"op": {OneOf: []string{"&&", "||"}},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{Target: "node", Op: ">", Value: 0},
					&HasCallExprCond{Target: "node"},
				},
			},
			Priority: 40,
			Action: &TryElseAction{
				Try: &ReflowNestedCallsAction{
					Target:   "node",
					Strategy: StrategyOnePerLine,
				},
				Else: &BreakAtOpAction{
					Target:     "node",
					BreakAfter: true,
				},
			},
		},

		// Rule 5: Long case clause
		{
			Name: "long_case_clause",
			Pattern: &NodePattern{
				Type: "CaseClause",
			},
			When:     &LineWidthCond{Target: "node", Op: ">", Value: 0},
			Priority: 35,
			// For now, use break at first comma
			// TODO: implement WrapListAction for proper comma-separated breaking
			Action: &NoOpAction{}, // Placeholder
		},

		// Rule 6: Long logical chain without calls
		{
			Name: "long_logical_chain",
			Pattern: &NodePattern{
				Type: "BinaryExpr",
				Fields: map[string]FieldMatch{
					"op": {OneOf: []string{"&&", "||"}},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{Target: "node", Op: ">", Value: 0},
					&NotCond{Cond: &HasCallExprCond{Target: "node"}},
				},
			},
			Priority: 30,
			Action: &BreakAtOpAction{
				Target:     "node",
				BreakAfter: true,
			},
		},

		// Rule 7: Long arithmetic expression
		{
			Name: "long_arithmetic_expr",
			Pattern: &NodePattern{
				Type: "BinaryExpr",
				Fields: map[string]FieldMatch{
					"op": {OneOf: []string{"+", "-", "*", "/", "%"}},
				},
			},
			When:     &LineWidthCond{Target: "node", Op: ">", Value: 0},
			Priority: 20,
			Action: &BreakAtOpAction{
				Target:     "node",
				BreakAfter: true,
			},
		},

		// Rule 8: Return statement with long call
		{
			Name: "return_with_long_call",
			Pattern: &NodePattern{
				Type: "ReturnStmt",
				Fields: map[string]FieldMatch{
					"results": {
						Capture:    "call",
						SubPattern: &NodePattern{Type: "CallExpr"},
					},
				},
			},
			When:     &LineWidthCond{Target: "node", Op: ">", Value: 0},
			Priority: 45,
			Action: &ReflowCallAction{
				Target:   "call",
				Strategy: StrategyOnePerLine,
			},
		},

		// Rule 9: For loop with long condition
		{
			Name: "for_with_long_cond",
			Pattern: &NodePattern{
				Type: "ForStmt",
				Fields: map[string]FieldMatch{
					"cond": {Capture: "cond"},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{Target: "node", Op: ">", Value: 0},
					&HasCallExprCond{Target: "cond"},
				},
			},
			Priority: 45,
			Action: &TryElseAction{
				Try: &ReflowNestedCallsAction{
					Target:   "cond",
					Strategy: StrategyOnePerLine,
				},
				Else: &NoOpAction{},
			},
		},
	}
}

// ComparisonOps returns the list of comparison operators.
func ComparisonOps() []string {
	return []string{"==", "!=", "<", ">", "<=", ">="}
}

// LogicalOps returns the list of logical operators.
func LogicalOps() []string {
	return []string{"&&", "||"}
}

// ArithmeticOps returns the list of arithmetic operators.
func ArithmeticOps() []string {
	return []string{"+", "-", "*", "/", "%"}
}
