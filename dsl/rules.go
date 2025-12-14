package dsl

// DefaultRuleOptions controls which legacy formatter functions are used by the
// default DSL rule set. Callers in other packages (e.g. formatter) can inject
// existing implementations without creating import cycles.
type DefaultRuleOptions struct {
	LeftFlowFormat        LeftFlowFormatFunc
	PackedMultiLineFormat PackedMultiLineFormatFunc
	FuncSignatureFormat   SignatureFormatFunc
	InterfaceMethodFormat InterfaceMethodFormatFunc
}

// DefaultRules returns the standard formatting rules, including blank line rules.
// The optional formatFunc parameter allows injecting the legacy formatter for
// left-flow call formatting.
func DefaultRules(formatFunc ...LeftFlowFormatFunc) []Rule {
	var fn LeftFlowFormatFunc
	if len(formatFunc) > 0 {
		fn = formatFunc[0]
	}
	return DefaultRulesWithOptions(DefaultRuleOptions{
		LeftFlowFormat: fn,
	})
}

// DefaultRulesWithOptions returns the default rule set used by the DSL engine.
// It is intended to reproduce the legacy pipeline behavior via injected
// formatter functions where available.
func DefaultRulesWithOptions(opts DefaultRuleOptions) []Rule {
	sigRules := SignatureRules(SignatureConfig{
		FuncFormatter:   opts.FuncSignatureFormat,
		MethodFormatter: opts.InterfaceMethodFormat,
	})
	logRules := LogPrintfRules(opts.LeftFlowFormat)
	multiLineRules := MultiLineCallRules(opts.PackedMultiLineFormat)
	exprRules := expressionOnlyRules()
	blankRules := BlankLineRules()

	rules := make([]Rule, 0, len(sigRules)+len(logRules)+len(multiLineRules)+len(exprRules)+len(blankRules))
	rules = append(rules, sigRules...)
	rules = append(rules, logRules...)
	rules = append(rules, multiLineRules...)
	rules = append(rules, exprRules...)
	rules = append(rules, blankRules...)
	return rules
}

// expressionOnlyRules returns expression-breaking rules that are intended to run
// alongside the call/signature rule sets (MultiLineCallRules, SignatureRules,
// LogPrintfRules) without overlapping responsibilities.
func expressionOnlyRules() []Rule {
	return []Rule{
		// Never break simple comparisons (x > 0, flag == true, etc.)
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

		// Long logical chain with function calls - try reflow first.
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

		// Long logical chain without calls.
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

		// Long arithmetic expression (excluding string concat and call args).
		{
			Name: "long_arithmetic_expr",
			Pattern: &NodePattern{
				Type: "BinaryExpr",
				Fields: map[string]FieldMatch{
					"op": {OneOf: []string{"+", "-", "*", "/", "%"}},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{Target: "node", Op: ">", Value: 0},
					&NotCond{Cond: &IsStringConcatCond{Target: "node"}},
					&NotCond{Cond: &IsCallArgCond{Target: "node"}},
				},
			},
			Priority: 20,
			Action: &BreakAtOpAction{
				Target:     "node",
				BreakAfter: true,
			},
		},

		// For loop with long condition - break at operators.
		{
			Name: "for_with_long_cond",
			Pattern: &NodePattern{
				Type: "ForStmt",
				Fields: map[string]FieldMatch{
					"cond": {
						Capture:    "cond",
						SubPattern: &NodePattern{Type: "BinaryExpr"},
					},
				},
			},
			When:     &LineWidthCond{Target: "node", Op: ">", Value: 0},
			Priority: 45,
			Action:   &BreakAtOpAction{Target: "cond", BreakAfter: true},
		},

		// Return statement with long binary expression (excluding string concat).
		{
			Name: "return_with_long_binary",
			Pattern: &NodePattern{
				Type: "ReturnStmt",
				Fields: map[string]FieldMatch{
					"results": {
						Capture:    "expr",
						SubPattern: &NodePattern{Type: "BinaryExpr"},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{Target: "node", Op: ">", Value: 0},
					&NotCond{Cond: &IsStringConcatCond{Target: "expr"}},
				},
			},
			Priority: 35,
			Action:   &BreakAtOpAction{Target: "expr", BreakAfter: true},
		},

		// Long case clause - break at comma.
		{
			Name:     "long_case_clause",
			Pattern:  &NodePattern{Type: "CaseClause"},
			When:     &LineWidthCond{Target: "node", Op: ">", Value: 0},
			Priority: 35,
			Action:   &BreakCaseClauseAction{Target: "node"},
		},

		// Assignment with long binary expression (not call).
		{
			Name: "assignment_with_long_binary",
			Pattern: &NodePattern{
				Type: "AssignStmt",
				Fields: map[string]FieldMatch{
					"rhs": {
						Capture:    "expr",
						SubPattern: &NodePattern{Type: "BinaryExpr"},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{Target: "node", Op: ">", Value: 0},
					&NotCond{Cond: &HasCallExprCond{Target: "expr"}},
				},
			},
			Priority: 32,
			Action:   &BreakAtOpAction{Target: "expr", BreakAfter: true},
		},
	}
}

// expressionRules returns the expression formatting rules.
func expressionRules(formatFunc LeftFlowFormatFunc) []Rule {
	// Create the left-flow call action with optional legacy formatter
	leftFlowAction := &LeftFlowCallAction{Target: "node"}
	if formatFunc != nil {
		leftFlowAction.FormatFunc = formatFunc
	}

	return []Rule{
		// Signature rules - high priority to process before other rules

		// Rule: Long function declaration - break parameters
		{
			Name:     "long_func_decl_params",
			Pattern:  &NodePattern{Type: "FuncDecl"},
			When:     &LineWidthCond{Target: "node", Op: ">", Value: 0},
			Priority: 90,
			Action:   &BreakFuncSignatureAction{Target: "node"},
		},

		// Rule: Long function return values - break after opening paren
		{
			Name:     "long_func_return_values",
			Pattern:  &NodePattern{Type: "FuncDecl"},
			When:     &LineWidthCond{Target: "node", Op: ">", Value: 0},
			Priority: 85,
			Action:   &BreakReturnValuesAction{Target: "node"},
		},

		// Rule: Long method chain in assignment - break with one call per line
		// Priority 80 to run before call reflow rules
		{
			Name: "long_method_chain_assign",
			Pattern: &NodePattern{
				Type: "AssignStmt",
				Fields: map[string]FieldMatch{
					"rhs": {
						Capture:    "call",
						SubPattern: &NodePattern{Type: "CallExpr"},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{Target: "node", Op: ">", Value: 0},
					&IsMethodChainCond{Target: "call", MinCalls: 2},
				},
			},
			Priority: 80,
			Action:   &BreakMethodChainAction{Target: "call"},
		},

		// Rule: Log/printf calls - use left-flow packing with string splitting
		// Priority 75 - after method chains, before generic reflow
		// Note: No line width check - the action normalizes format regardless
		{
			Name:     "log_printf_call",
			Pattern:  &NodePattern{Type: "CallExpr"},
			When:     &IsLogOrPrintfCallCond{Target: "node"},
			Priority: 75,
			Action:   leftFlowAction,
		},

		// Rule: Long method chain in return - break with one call per line
		{
			Name: "long_method_chain_return",
			Pattern: &NodePattern{
				Type: "ReturnStmt",
				Fields: map[string]FieldMatch{
					"results": {
						Capture:    "call",
						SubPattern: &NodePattern{Type: "CallExpr"},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{Target: "node", Op: ">", Value: 0},
					&IsMethodChainCond{Target: "call", MinCalls: 2},
				},
			},
			Priority: 80,
			Action:   &BreakMethodChainAction{Target: "call"},
		},

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
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{Target: "node", Op: ">", Value: 0},
					&NotCond{Cond: &IsLogOrPrintfCallCond{Target: "rhs"}},
					&NotCond{Cond: &IsMethodChainCond{Target: "rhs", MinCalls: 2}},
				},
			},
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

		// Rule 5: Long logical chain without calls
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

		// Rule 7: Long arithmetic expression (excluding string concat and call args)
		{
			Name: "long_arithmetic_expr",
			Pattern: &NodePattern{
				Type: "BinaryExpr",
				Fields: map[string]FieldMatch{
					"op": {OneOf: []string{"+", "-", "*", "/", "%"}},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{Target: "node", Op: ">", Value: 0},
					&NotCond{Cond: &IsStringConcatCond{Target: "node"}},
					&NotCond{Cond: &IsCallArgCond{Target: "node"}},
				},
			},
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
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{Target: "node", Op: ">", Value: 0},
					&NotCond{Cond: &IsLogOrPrintfCallCond{Target: "call"}},
					&NotCond{Cond: &IsMethodChainCond{Target: "call", MinCalls: 2}},
				},
			},
			Priority: 45,
			Action: &ReflowCallAction{
				Target:   "call",
				Strategy: StrategyOnePerLine,
			},
		},

		// Rule 9: For loop with long condition - break at operators
		{
			Name: "for_with_long_cond",
			Pattern: &NodePattern{
				Type: "ForStmt",
				Fields: map[string]FieldMatch{
					"cond": {
						Capture:    "cond",
						SubPattern: &NodePattern{Type: "BinaryExpr"},
					},
				},
			},
			When:     &LineWidthCond{Target: "node", Op: ">", Value: 0},
			Priority: 45,
			Action:   &BreakAtOpAction{Target: "cond", BreakAfter: true},
		},

		// Rule 10: Return statement with long binary expression (excluding string concat)
		{
			Name: "return_with_long_binary",
			Pattern: &NodePattern{
				Type: "ReturnStmt",
				Fields: map[string]FieldMatch{
					"results": {
						Capture:    "expr",
						SubPattern: &NodePattern{Type: "BinaryExpr"},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{Target: "node", Op: ">", Value: 0},
					&NotCond{Cond: &IsStringConcatCond{Target: "expr"}},
				},
			},
			Priority: 35,
			Action:   &BreakAtOpAction{Target: "expr", BreakAfter: true},
		},

		// Rule 11: Long case clause - break at comma
		{
			Name: "long_case_clause",
			Pattern: &NodePattern{
				Type: "CaseClause",
			},
			When:     &LineWidthCond{Target: "node", Op: ">", Value: 0},
			Priority: 35,
			Action:   &BreakCaseClauseAction{Target: "node"},
		},

		// Rule 12: Assignment with long binary expression (not call)
		{
			Name: "assignment_with_long_binary",
			Pattern: &NodePattern{
				Type: "AssignStmt",
				Fields: map[string]FieldMatch{
					"rhs": {
						Capture:    "expr",
						SubPattern: &NodePattern{Type: "BinaryExpr"},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{Target: "node", Op: ">", Value: 0},
					&NotCond{Cond: &HasCallExprCond{Target: "expr"}},
				},
			},
			Priority: 32,
			Action:   &BreakAtOpAction{Target: "expr", BreakAfter: true},
		},
	}
}

// BlankLineRules returns rules for inserting blank lines.
// These are separate from expression formatting rules.
func BlankLineRules() []Rule {
	return []Rule{
		// Rule: Blank line before case clause (if preceded by another case)
		{
			Name:    "blank_before_case",
			Pattern: &NodePattern{Type: "CaseClause"},
			When: &AndCond{
				Conds: []Condition{
					&HasPrecedingSiblingCond{Target: "node"},
				},
			},
			Priority: 10,
			Action:   &InsertBlankBeforeAction{Target: "node"},
		},

		// Rule: Blank line before return (if not after block open or case:)
		{
			Name:     "blank_before_return",
			Pattern:  &NodePattern{Type: "ReturnStmt"},
			When:     &IsReturnNeedingBlankCond{Target: "node"},
			Priority: 10,
			Action:   &InsertBlankBeforeAction{Target: "node"},
		},

		// Rule: Blank line between interface methods
		{
			Name:    "blank_between_interface_methods",
			Pattern: &NodePattern{Type: "Field"},
			When: &AndCond{
				Conds: []Condition{
					&IsInterfaceMethodCond{Target: "node"},
					&HasPrecedingInterfaceFieldCond{Target: "node"},
				},
			},
			Priority: 10,
			Action:   &InsertBlankBeforeAction{Target: "node"},
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

// LeftFlowFormatFunc is the signature for the left-flow call formatting function.
// This allows injecting the legacy formatter implementation to avoid circular imports.
type LeftFlowFormatFunc func(call []byte, wsIndent string, baseLen int, colLimit, tabStop int) string

// LogPrintfRules returns only the log/printf formatting rule.
// Use this for isolated testing of log call formatting.
// The optional formatFunc parameter allows injecting the legacy formatter.
func LogPrintfRules(formatFunc ...LeftFlowFormatFunc) []Rule {
	action := &LeftFlowCallAction{Target: "node"}
	if len(formatFunc) > 0 && formatFunc[0] != nil {
		action.FormatFunc = formatFunc[0]
	}
	return []Rule{
		{
			Name:    "log_printf_call",
			Pattern: &NodePattern{Type: "CallExpr"},
			// Only check if this is a log/printf call - the action will
			// normalize the format and skip if already in correct format.
			When:     &IsLogOrPrintfCallCond{Target: "node"},
			Priority: 75,
			Action:   action,
		},
	}
}

// PackedMultiLineFormatFunc is the signature for the packed multiline formatter.
// Unlike LeftFlowFormatFunc, it doesn't need baseLen since it always puts
// opening paren on its own line.
type PackedMultiLineFormatFunc func(call []byte, wsIndent string, colLimit, tabStop int) string

// MultiLineCallRules returns rules for multiline call formatting.
// Use this for isolated testing of generic call reflow.
// The optional formatFunc parameter allows injecting the legacy formatter.
func MultiLineCallRules(formatFunc ...PackedMultiLineFormatFunc) []Rule {
	action := &PackedMultiLineCallAction{Target: "node"}
	if len(formatFunc) > 0 && formatFunc[0] != nil {
		action.FormatFunc = formatFunc[0]
	}
	return []Rule{
		// Method chain rule - higher priority, handles chains specially
		{
			Name:    "long_method_chain",
			Pattern: &NodePattern{Type: "CallExpr"},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{Target: "node", Op: ">", Value: 0},
					&IsMethodChainCond{Target: "node", MinCalls: 2},
				},
			},
			Priority: 60,
			Action:   &BreakMethodChainAction{Target: "node"},
		},
		// Generic call expression that exceeds column limit
		// Skip method chains (handled above) and log/printf calls
		// Use CollapsedWidthCond to handle multiline calls where the first line
		// is short but the total content exceeds the column limit.
		{
			Name:    "long_call_expr",
			Pattern: &NodePattern{Type: "CallExpr"},
			When: &AndCond{
				Conds: []Condition{
					&CollapsedWidthCond{Target: "node", Op: ">", Value: 0},
					&NotCond{Cond: &IsLogOrPrintfCallCond{Target: "node"}},
					&NotCond{Cond: &IsMethodChainCond{Target: "node", MinCalls: 2}},
				},
			},
			Priority: 50,
			Action:   action,
		},
	}
}

// SignatureConfig holds optional formatters for signature rules.
type SignatureConfig struct {
	FuncFormatter   SignatureFormatFunc
	MethodFormatter InterfaceMethodFormatFunc
}

// SignatureRules returns rules for function signature formatting.
// Use this for isolated testing of signature breaking.
// The optional config parameter allows injecting the legacy formatters.
func SignatureRules(config ...SignatureConfig) []Rule {
	action := &BreakFuncSignatureAction{Target: "node"}
	methodAction := &BreakInterfaceMethodAction{Target: "node"}

	if len(config) > 0 {
		if config[0].FuncFormatter != nil {
			action.FormatFunc = config[0].FuncFormatter
		}
		if config[0].MethodFormatter != nil {
			methodAction.FormatFunc = config[0].MethodFormatter
		}
	}

	return []Rule{
		// Function declarations - trigger when any line exceeds limit
		// OR when there are nested func types with multiline content (for readability)
		{
			Name:    "long_func_decl",
			Pattern: &NodePattern{Type: "FuncDecl"},
			When: &OrCond{
				Conds: []Condition{
					&AnyLineWidthCond{Target: "node", Op: ">", Value: 0},
					&HasNestedMultilineTypeCond{Target: "node"},
				},
			},
			Priority: 90,
			Action:   action,
		},
		// Interface method declarations
		{
			Name:    "long_interface_method",
			Pattern: &NodePattern{Type: "Field"},
			When: &AndCond{
				Conds: []Condition{
					&IsInterfaceMethodCond{Target: "node"},
					&AnyLineWidthCond{Target: "node", Op: ">", Value: 0},
				},
			},
			Priority: 90,
			Action:   methodAction,
		},
	}
}

// MethodChainRules returns rules for method chain formatting.
// Use this for isolated testing of method chain breaking.
func MethodChainRules() []Rule {
	return []Rule{
		{
			Name: "long_method_chain_assign",
			Pattern: &NodePattern{
				Type: "AssignStmt",
				Fields: map[string]FieldMatch{
					"rhs": {
						Capture:    "call",
						SubPattern: &NodePattern{Type: "CallExpr"},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{Target: "node", Op: ">", Value: 0},
					&IsMethodChainCond{Target: "call", MinCalls: 2},
				},
			},
			Priority: 80,
			Action:   &BreakMethodChainAction{Target: "call"},
		},
		{
			Name: "long_method_chain_return",
			Pattern: &NodePattern{
				Type: "ReturnStmt",
				Fields: map[string]FieldMatch{
					"results": {
						Capture:    "call",
						SubPattern: &NodePattern{Type: "CallExpr"},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{Target: "node", Op: ">", Value: 0},
					&IsMethodChainCond{Target: "call", MinCalls: 2},
				},
			},
			Priority: 80,
			Action:   &BreakMethodChainAction{Target: "call"},
		},
	}
}

// ExpressionRules returns rules for expression formatting (logical chains, etc).
// Use this for isolated testing of expression breaking.
func ExpressionRules() []Rule {
	rules := []Rule{
		// Signature rules - high priority
		{
			Name:    "long_func_decl",
			Pattern: &NodePattern{Type: "FuncDecl"},
			When: &OrCond{
				Conds: []Condition{
					&AnyLineWidthCond{Target: "node", Op: ">", Value: 0},
					&HasNestedMultilineTypeCond{Target: "node"},
				},
			},
			Priority: 90,
			Action:   &BreakFuncSignatureAction{Target: "node"},
		},

		// Method chain rules
		{
			Name: "long_method_chain_assign",
			Pattern: &NodePattern{
				Type: "AssignStmt",
				Fields: map[string]FieldMatch{
					"rhs": {
						Capture:    "call",
						SubPattern: &NodePattern{Type: "CallExpr"},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{Target: "node", Op: ">", Value: 0},
					&IsMethodChainCond{Target: "call", MinCalls: 2},
				},
			},
			Priority: 80,
			Action:   &BreakMethodChainAction{Target: "call"},
		},

		// Assignment with long call (non-method-chain)
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
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{Target: "node", Op: ">", Value: 0},
					&NotCond{Cond: &IsLogOrPrintfCallCond{Target: "rhs"}},
					&NotCond{Cond: &IsMethodChainCond{Target: "rhs", MinCalls: 2}},
				},
			},
			Priority: 50,
			Action: &ReflowCallAction{
				Target:   "rhs",
				Strategy: StrategyOnePerLine,
			},
		},

		// For loop with long condition
		{
			Name: "for_with_long_cond",
			Pattern: &NodePattern{
				Type: "ForStmt",
				Fields: map[string]FieldMatch{
					"cond": {
						Capture:    "cond",
						SubPattern: &NodePattern{Type: "BinaryExpr"},
					},
				},
			},
			When:     &LineWidthCond{Target: "node", Op: ">", Value: 0},
			Priority: 45,
			Action:   &BreakAtOpAction{Target: "cond", BreakAfter: true},
		},

		// Multiline call rules for nested function calls
		{
			Name:    "long_call_expr",
			Pattern: &NodePattern{Type: "CallExpr"},
			When: &AndCond{
				Conds: []Condition{
					&CollapsedWidthCond{Target: "node", Op: ">", Value: 0},
					&NotCond{Cond: &IsLogOrPrintfCallCond{Target: "node"}},
					&NotCond{Cond: &IsMethodChainCond{Target: "node", MinCalls: 2}},
				},
			},
			Priority: 50,
			Action:   &OnePerLineMultiLineCallAction{Target: "node"},
		},

		// Never break simple comparisons
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

		// Long logical chain with function calls
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

		// Return statement with long binary expression (excluding string concat)
		{
			Name: "return_with_long_binary",
			Pattern: &NodePattern{
				Type: "ReturnStmt",
				Fields: map[string]FieldMatch{
					"results": {
						Capture:    "expr",
						SubPattern: &NodePattern{Type: "BinaryExpr"},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{Target: "node", Op: ">", Value: 0},
					&NotCond{Cond: &IsStringConcatCond{Target: "expr"}},
				},
			},
			Priority: 35,
			Action:   &BreakAtOpAction{Target: "expr", BreakAfter: true},
		},

		// Long case clause
		{
			Name: "long_case_clause",
			Pattern: &NodePattern{
				Type: "CaseClause",
			},
			When:     &LineWidthCond{Target: "node", Op: ">", Value: 0},
			Priority: 35,
			Action:   &BreakCaseClauseAction{Target: "node"},
		},

		// Long logical chain without calls
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

		// Assignment with long binary expression (not call)
		{
			Name: "assignment_with_long_binary",
			Pattern: &NodePattern{
				Type: "AssignStmt",
				Fields: map[string]FieldMatch{
					"rhs": {
						Capture:    "expr",
						SubPattern: &NodePattern{Type: "BinaryExpr"},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{Target: "node", Op: ">", Value: 0},
					&NotCond{Cond: &HasCallExprCond{Target: "expr"}},
				},
			},
			Priority: 32,
			Action:   &BreakAtOpAction{Target: "expr", BreakAfter: true},
		},

		// Long arithmetic expression (excluding string concat)
		{
			Name: "long_arithmetic_expr",
			Pattern: &NodePattern{
				Type: "BinaryExpr",
				Fields: map[string]FieldMatch{
					"op": {OneOf: []string{"+", "-", "*", "/", "%"}},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{Target: "node", Op: ">", Value: 0},
					&NotCond{Cond: &IsStringConcatCond{Target: "node"}},
					&NotCond{Cond: &IsCallArgCond{Target: "node"}},
				},
			},
			Priority: 20,
			Action: &BreakAtOpAction{
				Target:     "node",
				BreakAfter: true,
			},
		},
	}

	// Add blank line rules
	rules = append(rules, BlankLineRules()...)

	return rules
}
