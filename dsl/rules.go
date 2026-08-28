package dsl

// DefaultRuleOptions controls which legacy formatter functions are used by the
// default DSL rule set. Callers in other packages (e.g. formatter) can inject
// existing implementations without creating import cycles.
type DefaultRuleOptions struct {
	LeftFlowFormat        LeftFlowFormatFunc
	PackedMultiLineFormat PackedMultiLineFormatFunc
	FuncSignatureFormat   SignatureFormatFunc
	InterfaceMethodFormat SignatureFormatFunc
}

// DefaultRules returns the standard formatting rules, including blank line
// rules. The optional formatFunc parameter allows injecting the legacy
// formatter for left-flow call formatting.
func DefaultRules(formatFunc ...LeftFlowFormatFunc) []Rule {
	var fn LeftFlowFormatFunc
	if len(formatFunc) > 0 {
		fn = formatFunc[0]
	}

	return DefaultRulesWithOptions(
		DefaultRuleOptions{
			LeftFlowFormat: fn,
		},
	)
}

// DefaultRulesWithOptions returns the default rule set used by the DSL engine.
// It is intended to reproduce the legacy pipeline behavior via injected
// formatter functions where available.
func DefaultRulesWithOptions(opts DefaultRuleOptions) []Rule {
	sigRules := SignatureRules(
		SignatureConfig{
			FuncFormatter:   opts.FuncSignatureFormat,
			MethodFormatter: opts.InterfaceMethodFormat,
		},
	)
	logRules := LogPrintfRules(opts.LeftFlowFormat)
	multiLineRules := MultiLineCallRules(opts.PackedMultiLineFormat)
	exprRules := expressionOnlyRules()
	blankRules := BlankLineRules()

	rules := make(
		[]Rule, 0,
		len(sigRules)+len(logRules)+len(multiLineRules)+len(exprRules)+len(
			blankRules,
		),
	)
	rules = append(rules, sigRules...)
	rules = append(rules, logRules...)
	rules = append(rules, multiLineRules...)
	rules = append(rules, exprRules...)
	rules = append(rules, blankRules...)

	return rules
}

// expressionOnlyRules returns expression-breaking rules that are intended to
// run alongside the call/signature rule sets (MultiLineCallRules,
// SignatureRules, LogPrintfRules) without overlapping responsibilities.
func expressionOnlyRules() []Rule {
	return []Rule{
		// Never break simple comparisons (x > 0, flag == true, etc.)
		{
			Name: "keep_simple_comparison",
			Pattern: &NodePattern{
				Type: "BinaryExpr",
				Fields: map[string]FieldMatch{
					"op": {
						OneOf: []string{
							"==",
							"!=",
							"<",
							">",
							"<=",
							">=",
						},
					},
					"right": {
						Capture: "r",
					},
				},
			},
			When: &IsSimpleLiteralCond{
				Target: "r",
			},
			Priority: 100,
			Action: &KeepTogetherAction{
				Target: "node",
			},
		},

		// Long logical chain with function calls - try reflow first.
		{
			Name: "logical_chain_with_call",
			Pattern: &NodePattern{
				Type: "BinaryExpr",
				Fields: map[string]FieldMatch{
					"op": {
						OneOf: []string{
							"&&",
							"||",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&MaxLineWidthInSpanCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&HasCallExprCond{
						Target: "node",
					},
				},
			},
			Priority: 40,
			Action: &TryElseAction{
				Try: &ReflowNestedCallsAction{
					Target:   "node",
					Strategy: StrategyOnePerLine,
				},
				Else: &BreakLogicalChainPackedAction{
					Target: "node",
				},
			},
		},

		// Long logical chain without calls.
		{
			Name: "long_logical_chain",
			Pattern: &NodePattern{
				Type: "BinaryExpr",
				Fields: map[string]FieldMatch{
					"op": {
						OneOf: []string{
							"&&",
							"||",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&MaxLineWidthInSpanCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&NotCond{
						Cond: &HasCallExprCond{
							Target: "node",
						},
					},
				},
			},
			Priority: 30,
			Action: &BreakLogicalChainPackedAction{
				Target: "node",
			},
		},

		// Long arithmetic expression (excluding string concat and call
		// args).
		{
			Name: "long_arithmetic_expr",
			Pattern: &NodePattern{
				Type: "BinaryExpr",
				Fields: map[string]FieldMatch{
					"op": {
						OneOf: []string{
							"+",
							"-",
							"*",
							"/",
							"%",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&NotCond{
						Cond: &IsStringConcatCond{
							Target: "node",
						},
					},
					&NotCond{
						Cond: &IsCallArgCond{
							Target: "node",
						},
					},
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
						Capture: "cond",
						SubPattern: &NodePattern{
							Type: "BinaryExpr",
						},
					},
				},
			},
			When: &LineWidthCond{
				Target: "node",
				Op:     ">",
				Value:  0,
			},
			Priority: 45,
			Action: &TryElseAction{
				Try: &BreakBinaryExprLayoutAction{
					Target: "cond",
				},
				Else: &BreakAtOpAction{
					Target:     "cond",
					BreakAfter: true,
				},
			},
		},

		// Return statement with long binary expression (excluding
		// string concat).
		{
			Name: "return_with_long_binary",
			Pattern: &NodePattern{
				Type: "ReturnStmt",
				Fields: map[string]FieldMatch{
					"results": {
						Capture: "expr",
						SubPattern: &NodePattern{
							Type: "BinaryExpr",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&NotCond{
						Cond: &IsStringConcatCond{
							Target: "expr",
						},
					},
				},
			},
			Priority: 35,
			Action: &TryElseAction{
				Try: &BreakBinaryExprLayoutAction{
					Target: "expr",
				},
				Else: &BreakAtOpAction{
					Target:     "expr",
					BreakAfter: true,
				},
			},
		},

		// Long case clause - break at comma.
		{
			Name: "long_case_clause",
			Pattern: &NodePattern{
				Type: "CaseClause",
			},
			When: &LineWidthCond{
				Target: "node",
				Op:     ">",
				Value:  0,
			},
			Priority: 35,
			Action: &BreakCaseClauseAction{
				Target: "node",
			},
		},

		// Assignment with long binary expression (not call).
		{
			Name: "assignment_with_long_binary",
			Pattern: &NodePattern{
				Type: "AssignStmt",
				Fields: map[string]FieldMatch{
					"rhs": {
						Capture: "expr",
						SubPattern: &NodePattern{
							Type: "BinaryExpr",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&NotCond{
						Cond: &HasCallExprCond{
							Target: "expr",
						},
					},
				},
			},
			Priority: 32,
			Action: &BreakAtOpAction{
				Target:     "expr",
				BreakAfter: true,
			},
		},
	}
}

// expressionRules returns the expression formatting rules.
//
//nolint:unused // Historical legacy rule bundle; kept temporarily during next-mode migration.
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
			Name: "long_func_decl_params",
			Pattern: &NodePattern{
				Type: "FuncDecl",
			},
			When: &LineWidthCond{
				Target: "node",
				Op:     ">",
				Value:  0,
			},
			Priority: 90,
			Action: &BreakFuncSignatureAction{
				Target: "node",
			},
		},

		// Rule: Long function return values - break after opening paren
		{
			Name: "long_func_return_values",
			Pattern: &NodePattern{
				Type: "FuncDecl",
			},
			When: &LineWidthCond{
				Target: "node",
				Op:     ">",
				Value:  0,
			},
			Priority: 85,
			Action: &BreakReturnValuesAction{
				Target: "node",
			},
		},

		// Rule: Long method chain in assignment - break with one call
		// per line Priority 80 to run before call reflow rules
		{
			Name: "long_method_chain_assign",
			Pattern: &NodePattern{
				Type: "AssignStmt",
				Fields: map[string]FieldMatch{
					"rhs": {
						Capture: "call",
						SubPattern: &NodePattern{
							Type: "CallExpr",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&IsMethodChainCond{
						Target:   "call",
						MinCalls: 2,
					},
				},
			},
			Priority: 80,
			Action: &BreakMethodChainAction{
				Target: "call",
			},
		},

		// Rule: Log/printf calls - use left-flow packing with string
		// splitting Priority 75 - after method chains, before generic
		// reflow Note: No line width check - the action normalizes
		// format regardless
		{
			Name: "log_printf_call",
			Pattern: &NodePattern{
				Type: "CallExpr",
			},
			When: &IsLogOrPrintfCallCond{
				Target: "node",
			},
			Priority: 75,
			Action:   leftFlowAction,
		},

		// Rule: Long method chain in return - break with one call per
		// line
		{
			Name: "long_method_chain_return",
			Pattern: &NodePattern{
				Type: "ReturnStmt",
				Fields: map[string]FieldMatch{
					"results": {
						Capture: "call",
						SubPattern: &NodePattern{
							Type: "CallExpr",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&IsMethodChainCond{
						Target:   "call",
						MinCalls: 2,
					},
				},
			},
			Priority: 80,
			Action: &BreakMethodChainAction{
				Target: "call",
			},
		},

		// Rule 1: Never break simple comparisons (x > 0, flag == true,
		// etc.)
		{
			Name: "keep_simple_comparison",
			Pattern: &NodePattern{
				Type: "BinaryExpr",
				Fields: map[string]FieldMatch{
					"op": {
						OneOf: []string{
							"==",
							"!=",
							"<",
							">",
							"<=",
							">=",
						},
					},
					"right": {
						Capture: "r",
					},
				},
			},
			When: &IsSimpleLiteralCond{
				Target: "r",
			},
			Priority: 100,
			Action: &KeepTogetherAction{
				Target: "node",
			},
		},

		// Rule 2: If condition with call and simple comparison e.g., if
		// len(foo) > 10 { ... }
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
									Capture: "call",
									SubPattern: &NodePattern{
										Type: "CallExpr",
									},
								},
								"op": {
									OneOf: []string{
										"==",
										"!=",
										"<",
										">",
										"<=",
										">=",
									},
								},
								"right": {
									Capture: "r",
								},
							},
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&IsSimpleLiteralCond{
						Target: "r",
					},
				},
			},
			Priority: 60,
			Action: &ReflowCallAction{
				Target:   "call",
				Strategy: StrategyOnePerLine,
			},
		},

		// Rule 3: Assignment with long function call e.g., result :=
		// someFunc(arg1, arg2, arg3)
		{
			Name: "assignment_with_long_call",
			Pattern: &NodePattern{
				Type: "AssignStmt",
				Fields: map[string]FieldMatch{
					"lhs": {
						Capture: "var",
					},
					"rhs": {
						Capture: "rhs",
						SubPattern: &NodePattern{
							Type: "CallExpr",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&NotCond{
						Cond: &IsLogOrPrintfCallCond{
							Target: "rhs",
						},
					},
					&NotCond{
						Cond: &IsMethodChainCond{
							Target:   "rhs",
							MinCalls: 2,
						},
					},
				},
			},
			Priority: 50,
			Action: &ReflowCallAction{
				Target: "rhs",
				// Prefer a packed style for simple argument
				// lists so we can keep multiple short args on
				// the same continuation line when they fit.
				// Fall back to one-per-line when any arg is
				// already multiline.
				Strategy: StrategyAdaptive,
			},
		},

		// Rule 4: Long logical chain with function calls - try reflow
		// first
		{
			Name: "logical_chain_with_call",
			Pattern: &NodePattern{
				Type: "BinaryExpr",
				Fields: map[string]FieldMatch{
					"op": {
						OneOf: []string{
							"&&",
							"||",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&MaxLineWidthInSpanCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&HasCallExprCond{
						Target: "node",
					},
				},
			},
			Priority: 40,
			Action: &TryElseAction{
				Try: &ReflowNestedCallsAction{
					Target:   "node",
					Strategy: StrategyOnePerLine,
				},
				Else: &BreakLogicalChainPackedAction{
					Target: "node",
				},
			},
		},

		// Rule 5: Long logical chain without calls
		{
			Name: "long_logical_chain",
			Pattern: &NodePattern{
				Type: "BinaryExpr",
				Fields: map[string]FieldMatch{
					"op": {
						OneOf: []string{
							"&&",
							"||",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&MaxLineWidthInSpanCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&NotCond{
						Cond: &HasCallExprCond{
							Target: "node",
						},
					},
				},
			},
			Priority: 30,
			Action: &BreakLogicalChainPackedAction{
				Target: "node",
			},
		},

		// Rule 7: Long arithmetic expression (excluding string concat
		// and call args)
		{
			Name: "long_arithmetic_expr",
			Pattern: &NodePattern{
				Type: "BinaryExpr",
				Fields: map[string]FieldMatch{
					"op": {
						OneOf: []string{
							"+",
							"-",
							"*",
							"/",
							"%",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&NotCond{
						Cond: &IsStringConcatCond{
							Target: "node",
						},
					},
					&NotCond{
						Cond: &IsCallArgCond{
							Target: "node",
						},
					},
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
						Capture: "call",
						SubPattern: &NodePattern{
							Type: "CallExpr",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&NotCond{
						Cond: &IsLogOrPrintfCallCond{
							Target: "call",
						},
					},
					&NotCond{
						Cond: &IsMethodChainCond{
							Target:   "call",
							MinCalls: 2,
						},
					},
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
						Capture: "cond",
						SubPattern: &NodePattern{
							Type: "BinaryExpr",
						},
					},
				},
			},
			When: &LineWidthCond{
				Target: "node",
				Op:     ">",
				Value:  0,
			},
			Priority: 45,
			Action: &TryElseAction{
				Try: &BreakBinaryExprLayoutAction{
					Target: "cond",
				},
				Else: &BreakAtOpAction{
					Target:     "cond",
					BreakAfter: true,
				},
			},
		},

		// Rule 10: Return statement with long binary expression
		// (excluding string concat)
		{
			Name: "return_with_long_binary",
			Pattern: &NodePattern{
				Type: "ReturnStmt",
				Fields: map[string]FieldMatch{
					"results": {
						Capture: "expr",
						SubPattern: &NodePattern{
							Type: "BinaryExpr",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&NotCond{
						Cond: &IsStringConcatCond{
							Target: "expr",
						},
					},
				},
			},
			Priority: 35,
			Action: &TryElseAction{
				Try: &BreakBinaryExprLayoutAction{
					Target: "expr",
				},
				Else: &BreakAtOpAction{
					Target:     "expr",
					BreakAfter: true,
				},
			},
		},

		// Rule 11: Long case clause - break at comma
		{
			Name: "long_case_clause",
			Pattern: &NodePattern{
				Type: "CaseClause",
			},
			When: &LineWidthCond{
				Target: "node",
				Op:     ">",
				Value:  0,
			},
			Priority: 35,
			Action: &BreakCaseClauseAction{
				Target: "node",
			},
		},

		// Rule 12: Assignment with long binary expression (not call)
		{
			Name: "assignment_with_long_binary",
			Pattern: &NodePattern{
				Type: "AssignStmt",
				Fields: map[string]FieldMatch{
					"rhs": {
						Capture: "expr",
						SubPattern: &NodePattern{
							Type: "BinaryExpr",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&NotCond{
						Cond: &HasCallExprCond{
							Target: "expr",
						},
					},
				},
			},
			Priority: 32,
			Action: &TryElseAction{
				Try: &BreakBinaryExprLayoutAction{
					Target: "expr",
				},
				Else: &BreakAtOpAction{
					Target:     "expr",
					BreakAfter: true,
				},
			},
		},
	}
}

// BlankLineRules returns rules for inserting blank lines. These are separate
// from expression formatting rules.
func BlankLineRules() []Rule {
	return BlankLineRulesWithOptions(BlankLineOptions{})
}

type BlankLineOptions struct {
	// ExtraIfErrReturn inserts a blank line before:
	//
	// if err != nil { return ... }
	//
	// This is intentionally opt-in because it is opinionated and may
	// interact with users' desired grouping/spacing style.
	ExtraIfErrReturn bool
}

func BlankLineRulesWithOptions(opts BlankLineOptions) []Rule {
	rules := []Rule{
		// Batch blank-line insertion to avoid hundreds of iterations on
		// files with many cases/returns/methods.
		{
			Name: "blank_lines_batch",
			Pattern: &NodePattern{
				Type: "File",
			},
			When: &IsParseableCond{
				Want: true,
			},
			Priority: 20,
			Action: &BlankLinesBatchAction{
				Options: opts,
			},
		},

		// Rule: Blank line before case clause (if preceded by another
		// case)
		{
			Name: "blank_before_case",
			Pattern: &NodePattern{
				Type: "CaseClause",
			},
			When: &AndCond{
				Conds: []Condition{
					&HasPrecedingSiblingCond{
						Target: "node",
					},
				},
			},
			Priority: 10,
			Action: &InsertBlankBeforeClauseAction{
				Target: "node",
			},
		},
		{
			Name: "blank_before_comm_clause",
			Pattern: &NodePattern{
				Type: "CommClause",
			},
			When: &AndCond{
				Conds: []Condition{
					&HasPrecedingSiblingCond{
						Target: "node",
					},
				},
			},
			Priority: 10,
			Action: &InsertBlankBeforeClauseAction{
				Target: "node",
			},
		},

		// Rule: Blank line before return (if not after block open or
		// case:)
		{
			Name: "blank_before_return",
			Pattern: &NodePattern{
				Type: "ReturnStmt",
			},
			When: &IsReturnNeedingBlankCond{
				Target: "node",
			},
			Priority: 10,
			Action: &InsertBlankBeforeAction{
				Target: "node",
			},
		},

		// Rule: Blank line between interface methods
		{
			Name: "blank_between_interface_methods",
			Pattern: &NodePattern{
				Type: "Field",
			},
			When: &AndCond{
				Conds: []Condition{
					&IsInterfaceMethodCond{
						Target: "node",
					},
					&HasPrecedingInterfaceFieldCond{
						Target: "node",
					},
				},
			},
			Priority: 10,
			Action: &InsertBlankBeforeAction{
				Target: "node",
			},
		},
	}

	if opts.ExtraIfErrReturn {
		rules = append(
			rules, Rule{
				Name:    "blank_before_if_err_return",
				Pattern: &NodePattern{Type: "IfStmt"},
				When: &IsIfErrReturnNeedingBlankCond{
					Target: "node",
				},
				Priority: 9,
				Action: &InsertBlankBeforeAction{
					Target: "node",
				},
			},
		)
	}

	return rules
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

// LeftFlowFormatFunc is the signature for the left-flow call formatting
// function. This allows injecting the legacy formatter implementation to avoid
// circular imports.
type LeftFlowFormatFunc func(call []byte, wsIndent string, baseLen int, colLimit, tabStop int) string

// LogPrintfOptions configures LogPrintfRulesWithOptions behavior.
type LogPrintfOptions struct {
	// MatchAnySelectorPrefix enables suffix-only matching for selector
	// calls. See IsLogOrPrintfCallCond.MatchAnySelectorPrefix for details.
	MatchAnySelectorPrefix bool

	// SelectorNames overrides the set of recognized printf-style selector
	// names for suffix-only matching. See
	// IsLogOrPrintfCallCond.SelectorNames.
	SelectorNames []string

	// SelectorPrefixes restricts selector-prefix matching for log/printf
	// calls (allowlist). See IsLogOrPrintfCallCond.SelectorPrefixes.
	SelectorPrefixes []string

	// IncludeNonFStringCalls enables matching a small subset of non-`*f`
	// log calls (e.g. `logger.Error("...")`) when the first argument is a
	// string.
	//
	// This is intended for the "next" profile only; it expands the set of
	// targeted string calls beyond the printf-style patterns.
	IncludeNonFStringCalls bool
}

// LogPrintfRules returns only the log/printf formatting rule. Use this for
// isolated testing of log call formatting. The optional formatFunc parameter
// allows injecting the legacy formatter.
func LogPrintfRules(formatFunc ...LeftFlowFormatFunc) []Rule {
	return LogPrintfRulesWithOptions(LogPrintfOptions{}, formatFunc...)
}

// LogPrintfRulesWithOptions returns only the log/printf formatting rule, with
// explicit options.
func LogPrintfRulesWithOptions(opts LogPrintfOptions,
	formatFunc ...LeftFlowFormatFunc) []Rule {

	action := &LeftFlowCallAction{Target: "node"}
	if len(formatFunc) > 0 && formatFunc[0] != nil {
		action.FormatFunc = formatFunc[0]
	}

	return []Rule{
		{
			Name: "structured_log_call",
			Pattern: &NodePattern{
				Type: "CallExpr",
			},
			When: &AndCond{
				Conds: []Condition{
					&IsStructuredLogCallCond{
						Target: "node",
					},
					&NotCond{
						Cond: &HasAnyCommentCond{
							Target: "node",
						},
					},
				},
			},
			Priority: 80,
			Action: &StructuredLogCallAction{
				Target: "node",
			},
		},
		{
			Name: "log_printf_call",
			Pattern: &NodePattern{
				Type: "CallExpr",
			},
			// Only check if this is a log/printf call - the action
			// will normalize the format and skip if already in
			// correct format.
			When: &AndCond{
				Conds: []Condition{
					&IsLogOrPrintfCallCond{
						Target:                 "node",
						MatchAnySelectorPrefix: opts.MatchAnySelectorPrefix,
						SelectorNames:          opts.SelectorNames,
						SelectorPrefixes:       opts.SelectorPrefixes,
						IncludeNonFStringCalls: opts.IncludeNonFStringCalls,
					},
					&NotCond{
						Cond: &AndCond{
							Conds: []Condition{
								&IsCallArgCond{
									Target: "node",
								},
								&IsCallFuncInListCond{
									Target: "node",
									Names: []string{
										"fmt.Errorf",
									},
								},
								&CallArgCountCond{
									Target: "node",
									Op:     ">=",
									Value:  3,
								},
							},
						},
					},
				},
			},
			Priority: 75,
			Action:   action,
		},
	}
}

// PackedMultiLineFormatFunc is the signature for the packed multiline
// formatter. The fullPrefix argument is the current line text before the call
// expression and lets formatters account for assignment prefixes when deciding
// whether the call head itself overflows.
type PackedMultiLineFormatFunc func(call []byte, wsIndent, fullPrefix string, colLimit, tabStop int) string

// MultiLineCallRules returns rules for multiline call formatting. Use this for
// isolated testing of generic call reflow. The optional formatFunc parameter
// allows injecting the legacy formatter.
func MultiLineCallRules(formatFunc ...PackedMultiLineFormatFunc) []Rule {
	return MultiLineCallRulesWithOptions(
		MultiLineCallOptions{}, formatFunc...,
	)
}

// MultiLineCallOptions configures MultiLineCallRules behavior.
type MultiLineCallOptions struct {
	// Excludes is a list of function names that should be excluded from
	// multiline call formatting (matches "foo" or "pkg.Foo").
	Excludes []string

	// LogCallSelectorNames configures which selector/ident names should be
	// treated as log/printf-style calls for the purpose of excluding them
	// from generic multiline call formatting. When empty, a built-in
	// default set is used.
	LogCallSelectorNames []string

	// LogCallSelectorPrefixes optionally restricts selector-prefix matching
	// for log/printf call exclusion (allowlist). See
	// IsLogOrPrintfCallCond.SelectorPrefixes.
	LogCallSelectorPrefixes []string

	// MethodChainStyle controls how long method chains are broken.
	// Supported: ""/"legacy" (existing BreakMethodChainAction) and "layout"
	// (layout engine).
	MethodChainStyle string

	// CallArgsStyle controls how long generic call expressions are broken.
	// Supported: ""/"legacy" (existing packed/legacy formatters) and
	// "layout" (layout engine).
	CallArgsStyle string

	// CallArgsGrouping optionally enables an explicit grouping heuristic
	// for call argument lists when CallArgsStyle == "layout".
	//
	// Supported values:
	// - "" (default): one argument per line (forced break)
	// - "pairs": group args as (a, b) pairs when possible
	CallArgsGrouping string

	// DisableBreakBeforeCallOnLongMultiAssignPrefix disables a heuristic in
	// the packed multiline call action that prefers breaking before a call
	// (keeping it single-line) when the only overflow is caused by a long
	// multi-assignment prefix. Some profiles intentionally prefer
	// formatting the call itself as multiline instead.
	DisableBreakBeforeCallOnLongMultiAssignPrefix bool

	// CheckMaxSpanLineWidth enables detection of overlong continuation
	// lines for already-multiline calls. This is useful in styles that want
	// to enforce column limits even when a call's first line and collapsed
	// width appear to fit.
	//
	// This is intentionally opt-in to avoid changing legacy/parity
	// behavior.
	CheckMaxSpanLineWidth bool
}

// MultiLineCallRulesWithOptions returns MultiLineCallRules with explicit
// options.
func MultiLineCallRulesWithOptions(opts MultiLineCallOptions,
	formatFunc ...PackedMultiLineFormatFunc) []Rule {

	action := &PackedMultiLineCallAction{Target: "node"}
	action.DisableBreakBeforeCallOnLongMultiAssignPrefix = opts.DisableBreakBeforeCallOnLongMultiAssignPrefix
	if len(formatFunc) > 0 && formatFunc[0] != nil {
		action.FormatFunc = formatFunc[0]
	}
	// When used as a fallback from layout-based formatting, only apply the
	// packed formatter to still-single-line long calls. This prevents a
	// layout-shaped multiline call from being immediately "re-packed" on a
	// subsequent iteration, which would cause oscillation and defeat the
	// layout owner.
	fallbackSingleLineOnly := &PackedMultiLineCallAction{
		Target:           "node",
		FormatFunc:       action.FormatFunc,
		OnlyIfSingleLine: true,
		DisableBreakBeforeCallOnLongMultiAssignPrefix: opts.DisableBreakBeforeCallOnLongMultiAssignPrefix,
	}

	longCallConds := []Condition{
		// Use CollapsedWidthCond for multiline nodes (first line may be
		// short), but also consider actual line width for single-line
		// nodes. This avoids false negatives when the source contains
		// extra spacing.
		&OrCond{Conds: []Condition{
			&LineWidthCond{
				Target: "node",
				Op:     ">",
				Value:  0,
			},
			&CollapsedWidthCond{
				Target: "node",
				Op:     ">",
				Value:  0,
			},
		}},
		&NotCond{
			Cond: &IsLogOrPrintfCallCond{
				Target:                 "node",
				MatchAnySelectorPrefix: true,
				SelectorNames:          opts.LogCallSelectorNames,
				SelectorPrefixes:       opts.LogCallSelectorPrefixes,
			},
		},
		&NotCond{
			Cond: &IsNonFLogCallCond{
				Target: "node",
			},
		},
		&NotCond{
			Cond: &IsStructuredLogCallCond{
				Target: "node",
			},
		},
		&NotCond{
			Cond: &IsMethodChainCond{
				Target:   "node",
				MinCalls: 2,
			},
		},
		&NotCond{
			Cond: &IsCallFuncContainsAnyCond{
				Target: "node",
				Names:  opts.Excludes,
			},
		},
		// Avoid rewriting calls that contain inline comments; AST-based
		// rendering would drop them.
		&NotCond{
			Cond: &HasAnyCommentCond{
				Target: "node",
			},
		},
	}
	if opts.CheckMaxSpanLineWidth {
		// For already-multiline calls, LineWidthCond and
		// CollapsedWidthCond can be false negatives when a continuation
		// line overflows due to indentation.
		longCallConds[0].(*OrCond).Conds = append(
			longCallConds[0].(*OrCond).Conds,
			&MaxSpanLineWidthCond{
				Target: "node",
				Op:     ">",
				Value:  0,
			},
		)
	}
	// When layout is enabled, avoid independently rewriting receiver calls
	// inside method chains. Those are better handled by the outer method
	// chain / call argument formatting to prevent oscillation and parse
	// hazards.
	if opts.CallArgsStyle == "layout" || opts.MethodChainStyle == "layout" {
		longCallConds = append(
			longCallConds, &NotCond{
				Cond: &IsChainedCallReceiverCond{
					Target: "node",
				},
			},
		)
	}
	// For layout-driven call-argument formatting, avoid independently
	// rewriting nested calls that appear inside another call's argument
	// list. The outer call-arg layout pass can format these as structured
	// docs and will make a better whole-call decision; rewriting the inner
	// call first can force it multiline due to the (irrelevant) prefix
	// width, causing the outer layout formatter to bail out and fall back
	// to packed/legacy.
	if opts.CallArgsStyle == "layout" {
		longCallConds = append(
			longCallConds, &NotCond{
				Cond: &IsCallArgCond{Target: "node"},
			},
		)
	}

	var rules []Rule
	rules = append(
		rules, collapseSimpleCallArgsRule(), packReturnStmtRule(),
	)

	// Method chain rule - higher priority, handles chains specially.
	//
	// For `layout-args` we intentionally do not run the method-chain rule:
	// method chains are expected to be formatted as expressions inside
	// outer call argument lists (via `BreakCallArgsLayoutAction` + expr
	// docs), and rewriting the chain independently can introduce parse
	// hazards and oscillation.
	if opts.CallArgsStyle != "layout" || opts.MethodChainStyle != "" {
		rules = append(
			rules, Rule{
				Name:    "long_method_chain",
				Pattern: &NodePattern{Type: "CallExpr"},
				When: &AndCond{
					Conds: []Condition{
						&LineWidthCond{Target: "node", Op: ">", Value: 0},
						&IsMethodChainCond{Target: "node", MinCalls: 2},
						&NotCond{Cond: &IsCallFuncContainsAnyCond{Target: "node", Names: opts.Excludes}},
						&NotCond{Cond: &HasAnyCommentCond{Target: "node"}},
					},
				},
				Priority: 60,
				Action: func() Action {
					switch opts.MethodChainStyle {
					case "layout":
						return &TryElseAction{
							Try: &BreakSelectorCallArgsAction{
								Target:     "node",
								FormatFunc: action.FormatFunc,
							},
							Else: &BreakMethodChainLayoutAction{
								Target: "node",
							},
						}

					default:
						return &BreakMethodChainAction{
							Target: "node",
						}
					}
				}(),
			},
		)
	}

	// Generic call expression that exceeds column limit. Skip method chains
	// (handled by long_method_chain, when enabled) and log/printf calls.
	// Use CollapsedWidthCond (plus LineWidthCond) to handle multiline calls
	// where the first line is short but the total content exceeds the
	// column limit.
	rules = append(
		rules, Rule{
			Name:    "long_call_expr",
			Pattern: &NodePattern{Type: "CallExpr"},
			When: &AndCond{
				Conds: longCallConds,
			},
			Priority: 50,
			Action: func() Action {
				switch opts.CallArgsStyle {
				case "layout":
					return &TryElseAction{
						Try: &BreakCallArgsLayoutAction{
							Target:   "node",
							Grouping: opts.CallArgsGrouping,
						},
						Else: fallbackSingleLineOnly,
					}

				default:
					return action
				}
			}(),
		},
	)

	return rules
}

func collapseSimpleCallArgsRule() Rule {
	return Rule{
		Name: "collapse_simple_call_args",
		Pattern: &NodePattern{
			Type: "CallExpr",
		},
		When:     TrueCond{},
		Priority: 70,
		Action: &CollapseSimpleCallArgsAction{
			Target: "node",
		},
	}
}

func packReturnStmtRule() Rule {
	return Rule{
		Name: "pack_return_stmt",
		Pattern: &NodePattern{
			Type: "ReturnStmt",
		},
		When:     TrueCond{},
		Priority: 69,
		Action: &PackReturnStmtAction{
			Target: "node",
		},
	}
}

// PackedMultiLineOnlyRules returns multiline call rules that format only
// non-method-chain call expressions using packed multiline formatting.
//
// This is a smaller-scope variant of MultiLineCallRules that avoids
// method-chain breaking (which is a more opinionated behavior change).
func PackedMultiLineOnlyRules(formatFunc ...PackedMultiLineFormatFunc) []Rule {
	return PackedMultiLineOnlyRulesWithOptions(
		MultiLineCallOptions{}, formatFunc...,
	)
}

// PackedMultiLineOnlyRulesWithOptions is the configurable form of
// PackedMultiLineOnlyRules.
func PackedMultiLineOnlyRulesWithOptions(opts MultiLineCallOptions,
	formatFunc ...PackedMultiLineFormatFunc) []Rule {

	action := &PackedMultiLineCallAction{Target: "node"}
	if len(formatFunc) > 0 && formatFunc[0] != nil {
		action.FormatFunc = formatFunc[0]
	}

	spanWidthConds := []Condition{
		&LineWidthCond{
			Target: "node",
			Op:     ">",
			Value:  0,
		},
		&CollapsedWidthCond{
			Target: "node",
			Op:     ">",
			Value:  0,
		},
	}
	if opts.CheckMaxSpanLineWidth {
		spanWidthConds = append(
			spanWidthConds, &MaxSpanLineWidthCond{
				Target: "node",
				Op:     ">",
				Value:  0,
			},
		)
	}

	rules := []Rule{
		collapseSimpleCallArgsRule(),
		packReturnStmtRule(),
	}

	return append(rules, Rule{
		Name: "long_call_expr_packed",
		Pattern: &NodePattern{
			Type: "CallExpr",
		},
		When: &AndCond{
			Conds: []Condition{
				&OrCond{
					Conds: spanWidthConds,
				},
				&NotCond{
					Cond: &IsLogOrPrintfCallCond{
						Target:                 "node",
						MatchAnySelectorPrefix: true,
						SelectorNames:          opts.LogCallSelectorNames,
						SelectorPrefixes:       opts.LogCallSelectorPrefixes,
					},
				},
				&NotCond{
					Cond: &IsMethodChainCond{
						Target:   "node",
						MinCalls: 2,
					},
				},
				&NotCond{
					Cond: &IsCallFuncContainsAnyCond{
						Target: "node",
						Names:  opts.Excludes,
					},
				},
				&NotCond{
					Cond: &HasAnyCommentCond{
						Target: "node",
					},
				},
			},
		},
		Priority: 50,
		Action:   action,
	})
}

// LegacyMultiLineCallRules returns a rule set intended to match the legacy
// MultiLineCallFormatter behavior more closely (one argument per line, no
// method-chain breaking).
func LegacyMultiLineCallRules(formatFunc ...PackedMultiLineFormatFunc) []Rule {
	return LegacyMultiLineCallRulesWithOptions(
		MultiLineCallOptions{}, formatFunc...,
	)
}

// LegacyMultiLineCallRulesWithOptions returns LegacyMultiLineCallRules with
// explicit options.
func LegacyMultiLineCallRulesWithOptions(opts MultiLineCallOptions,
	formatFunc ...PackedMultiLineFormatFunc) []Rule {

	action := &LegacyOnePerLineCallAction{Target: "node"}
	if len(formatFunc) > 0 && formatFunc[0] != nil {
		action.FormatFunc = formatFunc[0]
	}

	return []Rule{
		{
			Name: "legacy_long_call_expr",
			Pattern: &NodePattern{
				Type: "CallExpr",
			},
			When: &AndCond{
				Conds: []Condition{
					&NotCond{
						Cond: &IsLogOrPrintfCallCond{
							Target:                 "node",
							MatchAnySelectorPrefix: true,
							SelectorNames:          opts.LogCallSelectorNames,
							SelectorPrefixes:       opts.LogCallSelectorPrefixes,
						},
					},
					&NotCond{
						Cond: &IsCallFuncContainsAnyCond{
							Target: "node",
							Names:  opts.Excludes,
						},
					},
				},
			},
			Priority: 50,
			Action:   action,
		},
	}
}

// LegacyMultiLineScanRules returns a rule set that delegates to the legacy
// scan-based multiline call formatter. This is used to preserve exact legacy
// behavior (including detection quirks) while running in the DSL engine.
func LegacyMultiLineScanRules(scanFunc LegacyMultiLineScanFunc) []Rule {
	return LegacyMultiLineScanRulesWithOptions(
		MultiLineCallOptions{}, scanFunc,
	)
}

// LegacyMultiLineScanRulesWithOptions is the configurable form of
// LegacyMultiLineScanRules.
func LegacyMultiLineScanRulesWithOptions(opts MultiLineCallOptions,
	scanFunc LegacyMultiLineScanFunc) []Rule {

	return []Rule{
		{
			Name: "legacy_multiline_scan",
			Pattern: &NodePattern{
				Type: "File",
			},
			When: &TrueCond{},
			// Keep priority in the same band as other legacy-format
			// parity rules.
			Priority: 50,
			Action: &LegacyMultiLineScanAction{
				Excludes: opts.Excludes,
				ScanFunc: scanFunc,
			},
		},
	}
}

// LegacyCompactCallRules delegates the compact-call stage to a legacy
// formatter. This exists to preserve parity with the legacy pipeline while
// running under the DSL engine.
func LegacyCompactCallRules(formatFunc LegacyCompactCallFormatFunc) []Rule {
	return []Rule{
		{
			Name: "legacy_compact_calls_format",
			Pattern: &NodePattern{
				Type: "File",
			},
			When:     &TrueCond{},
			Priority: 75,
			Action: &LegacyCompactCallFormatAction{
				FormatFunc: formatFunc,
			},
		},
	}
}

// LegacyCommentRules delegates comment formatting to a legacy formatter.
func LegacyCommentRules(formatFunc LegacyCommentFormatFunc,
	moveInlineAbove bool) []Rule {

	return []Rule{
		{
			Name: "legacy_comment_format",
			Pattern: &NodePattern{
				Type: "File",
			},
			When:     &TrueCond{},
			Priority: 90,
			Action: &LegacyCommentFormatAction{
				MoveInlineAbove: moveInlineAbove,
				FormatFunc:      formatFunc,
			},
		},
	}
}

// LegacyFuncSigRules delegates function signature formatting to a legacy
// formatter.
func LegacyFuncSigRules(formatFunc LegacyFuncSigFormatFunc) []Rule {
	return []Rule{
		{
			Name: "legacy_func_sig_format",
			Pattern: &NodePattern{
				Type: "File",
			},
			When:     &TrueCond{},
			Priority: 60,
			Action: &LegacyFuncSigFormatAction{
				FormatFunc: formatFunc,
			},
		},
	}
}

// LegacyFuncSigFallbackRules delegates function signature formatting to a
// legacy formatter, but at a low priority intended only as a fallback (e.g.
// when the source cannot be parsed).
func LegacyFuncSigFallbackRules(formatFunc LegacyFuncSigFormatFunc) []Rule {
	return []Rule{
		{
			Name: "legacy_func_sig_fallback",
			Pattern: &NodePattern{
				Type: "File",
			},
			When: &IsParseableCond{
				Want: false,
			},
			Priority: -100,
			Action: &LegacyFuncSigFormatAction{
				FormatFunc: formatFunc,
			},
		},
	}
}

// LegacyBlankLinesRules delegates blank line formatting to a legacy formatter.
func LegacyBlankLinesRules(formatFunc LegacyBlankLinesFormatFunc) []Rule {
	return []Rule{
		{
			Name: "legacy_blank_lines_format",
			Pattern: &NodePattern{
				Type: "File",
			},
			When:     &TrueCond{},
			Priority: 40,
			Action: &LegacyBlankLinesFormatAction{
				FormatFunc: formatFunc,
			},
		},
	}
}

// LegacyBlankLinesFallbackRules delegates blank line formatting to a legacy
// formatter, but at a low priority intended only as a fallback (e.g. when the
// source cannot be parsed, or as a last resort after native DSL rules).
func LegacyBlankLinesFallbackRules(
	formatFunc LegacyBlankLinesFormatFunc) []Rule {

	return []Rule{
		{
			Name: "legacy_blank_lines_fallback",
			Pattern: &NodePattern{
				Type: "File",
			},
			When: &IsParseableCond{
				Want: false,
			},
			Priority: -100,
			Action: &LegacyBlankLinesFormatAction{
				FormatFunc: formatFunc,
			},
		},
	}
}

// LongExprRules returns a rule set intended to match the legacy long expression
// formatter behavior: break long boolean/arithmetic chains and case clauses,
// without reformatting calls or signatures.
func LongExprRules() []Rule {
	return LongExprRulesWithOptions(LongExprOptions{})
}

// LongExprOptions configures LongExprRules behavior.
type LongExprOptions struct {
	// AllowCallArgs forces breaking long logical chains inside call
	// arguments. This can interact with call-formatting stages, so it is
	// disabled by default.
	AllowCallArgs bool

	// CallArgsPolicy controls call-argument editing for the expression
	// stage. When set to CallArgsPolicyAuto, CallArgsAllowlist is used.
	CallArgsPolicy CallArgsPolicy

	// CallArgsAllowlist is used when CallArgsPolicy == CallArgsPolicyAuto.
	// Typically, this should be a list of call names excluded from later
	// call-formatting stages.
	CallArgsAllowlist []string

	// LogicalChainStyle controls how long &&/|| chains are broken.
	// Supported: "legacy" (packed source edits) and "layout" (layout
	// engine). Empty defaults to "legacy".
	LogicalChainStyle string

	// ArithmeticChainStyle controls how long arithmetic chains are broken.
	// Supported: "legacy" (BreakAtOpAction) and "layout" (layout engine).
	// Empty defaults to "legacy".
	ArithmeticChainStyle string

	// CaseClauseStyle controls how long `case A, B, C:` lists are broken.
	// Supported: "legacy" (single-break BreakCaseClauseAction) and "layout"
	// (layout engine, may break multiple times). Empty defaults to
	// "legacy".
	CaseClauseStyle string

	// SelectorChainStyle controls formatting of long selector chains.
	// Supported: "legacy" (disabled) and "layout" (layout engine). Empty
	// defaults to "legacy".
	SelectorChainStyle string
}

// LongExprRulesWithOptions returns LongExprRules with explicit options.
func LongExprRulesWithOptions(opts LongExprOptions) []Rule {
	callArgsPolicy := opts.CallArgsPolicy
	if opts.AllowCallArgs {
		callArgsPolicy = CallArgsPolicyForce
	}
	style := opts.LogicalChainStyle
	if style == "" {
		style = "legacy"
	}
	arithStyle := opts.ArithmeticChainStyle
	if arithStyle == "" {
		arithStyle = "legacy"
	}
	caseStyle := opts.CaseClauseStyle
	if caseStyle == "" {
		caseStyle = "legacy"
	}
	selectorStyle := opts.SelectorChainStyle
	if selectorStyle == "" {
		selectorStyle = "legacy"
	}

	return []Rule{
		// Never break simple comparisons (x > 0, flag == true, etc.)
		{
			Name: "keep_simple_comparison",
			Pattern: &NodePattern{
				Type: "BinaryExpr",
				Fields: map[string]FieldMatch{
					"op": {
						OneOf: []string{
							"==",
							"!=",
							"<",
							">",
							"<=",
							">=",
						},
					},
					"right": {
						Capture: "r",
					},
				},
			},
			When: &IsSimpleLiteralCond{
				Target: "r",
			},
			Priority: 100,
			Action: &KeepTogetherAction{
				Target: "node",
			},
		},

		// Long string concatenation - flatten and re-split into stable
		// wrapped concatenation joins.
		{
			Name: "long_string_concat",
			Pattern: &NodePattern{
				Type: "BinaryExpr",
				Fields: map[string]FieldMatch{
					"op": {
						Literal: "+",
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&IsStringConcatCond{
						Target: "node",
					},
					// Call-argument formatting is owned by
					// call stages (log/printf and multiline
					// call rules). Avoid rewriting string
					// concatenations inside call arguments
					// to prevent surprising changes in
					// non-target calls.
					&NotCond{
						Cond: &IsInCallArgsCond{
							Target: "node",
						},
					},
					&ExprEditSafeCond{
						Target:            "node",
						CallArgsPolicy:    callArgsPolicy,
						CallArgsAllowlist: opts.CallArgsAllowlist,
					},
				},
			},
			Priority: 25,
			Action: &ReflowStringConcatAction{
				Target: "node",
			},
		},

		// Long selector chains (`a.b.c.d`) - prefer breaking after dots
		// using the layout engine (modern opt-in only). Skip if the
		// selector chain is part of a call expression; call stages own
		// method chains.
		{
			Name: "long_selector_chain",
			Pattern: &NodePattern{
				Type: "SelectorExpr",
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&ExprEditSafeCond{
						Target: "node",
					},
					&NotCond{
						Cond: &IsParentTypeCond{
							Target: "node",
							Type:   "SelectorExpr",
						},
					},
					&NotCond{
						Cond: &IsAncestorTypeCond{
							Target: "node",
							Type:   "CallExpr",
						},
					},
				},
			},
			Priority: 24,
			Action: func() Action {
				switch selectorStyle {
				case "layout":
					return &BreakSelectorChainLayoutAction{
						Target: "node",
					}

				default:
					return &NoOpAction{}
				}
			}(),
		},

		// Long logical chain (with or without calls) - break after && /
		// ||.
		{
			Name: "long_logical_chain",
			Pattern: &NodePattern{
				Type: "BinaryExpr",
				Fields: map[string]FieldMatch{
					"op": {
						OneOf: []string{
							"&&",
							"||",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&MaxLineWidthInSpanCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&ExprEditSafeCond{
						Target:            "node",
						CallArgsPolicy:    callArgsPolicy,
						CallArgsAllowlist: opts.CallArgsAllowlist,
					},
				},
			},
			Priority: 40,
			Action: func() Action {
				switch style {
				case "layout":
					return &TryElseAction{
						Try: &BreakLogicalChainLayoutAction{
							Target: "node",
						},
						Else: &BreakLogicalChainPackedAction{
							Target: "node",
						},
					}

				default:
					return &BreakLogicalChainPackedAction{
						Target: "node",
					}
				}
			}(),
		},

		// Long arithmetic expression (excluding string concat and call
		// args).
		{
			Name: "long_arithmetic_expr",
			Pattern: &NodePattern{
				Type: "BinaryExpr",
				Fields: map[string]FieldMatch{
					"op": {
						OneOf: []string{
							"+",
							"-",
							"*",
							"/",
							"%",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&NotCond{
						Cond: &IsStringConcatCond{
							Target: "node",
						},
					},
					&ExprEditSafeCond{
						Target: "node",
					},
				},
			},
			Priority: 20,
			Action: func() Action {
				switch arithStyle {
				case "layout":
					return &TryElseAction{
						Try: &BreakArithmeticChainLayoutAction{
							Target: "node",
						},
						Else: &BreakAtOpAction{
							Target:     "node",
							BreakAfter: true,
						},
					}

				default:
					return &BreakAtOpAction{
						Target:     "node",
						BreakAfter: true,
					}
				}
			}(),
		},

		// For loop with long condition - break at operators.
		{
			Name: "for_with_long_cond",
			Pattern: &NodePattern{
				Type: "ForStmt",
				Fields: map[string]FieldMatch{
					"cond": {
						Capture: "cond",
						SubPattern: &NodePattern{
							Type: "BinaryExpr",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&ExprEditSafeCond{
						Target: "cond",
					},
				},
			},
			Priority: 45,
			Action: &TryElseAction{
				Try: &BreakBinaryExprLayoutAction{
					Target:          "cond",
					LogicalStyle:    style,
					ArithmeticStyle: arithStyle,
				},
				Else: &BreakAtOpAction{
					Target:     "cond",
					BreakAfter: true,
				},
			},
		},

		// Return statement with long binary expression (excluding
		// string concat).
		{
			Name: "return_with_long_binary",
			Pattern: &NodePattern{
				Type: "ReturnStmt",
				Fields: map[string]FieldMatch{
					"results": {
						Capture: "expr",
						SubPattern: &NodePattern{
							Type: "BinaryExpr",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&NotCond{
						Cond: &IsStringConcatCond{
							Target: "expr",
						},
					},
					&ExprEditSafeCond{
						Target: "expr",
					},
				},
			},
			Priority: 35,
			Action: &TryElseAction{
				Try: &BreakBinaryExprLayoutAction{
					Target:          "expr",
					LogicalStyle:    style,
					ArithmeticStyle: arithStyle,
				},
				Else: &BreakAtOpAction{
					Target:     "expr",
					BreakAfter: true,
				},
			},
		},

		// Long case clause - break at comma.
		{
			Name: "long_case_clause",
			Pattern: &NodePattern{
				Type: "CaseClause",
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&ExprEditSafeCond{
						Target: "node",
					},
				},
			},
			Priority: 35,
			Action: func() Action {
				switch caseStyle {
				case "layout":
					return &TryElseAction{
						Try: &BreakCaseClauseLayoutAction{
							Target: "node",
						},
						Else: &BreakCaseClauseAction{
							Target: "node",
						},
					}

				default:
					return &BreakCaseClauseAction{
						Target: "node",
					}
				}
			}(),
		},

		// Assignment with long binary expression (not call).
		{
			Name: "assignment_with_long_binary",
			Pattern: &NodePattern{
				Type: "AssignStmt",
				Fields: map[string]FieldMatch{
					"rhs": {
						Capture: "expr",
						SubPattern: &NodePattern{
							Type: "BinaryExpr",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&NotCond{
						Cond: &HasCallExprCond{
							Target: "expr",
						},
					},
					&ExprEditSafeCond{
						Target: "expr",
					},
				},
			},
			Priority: 32,
			Action: &TryElseAction{
				Try: &BreakBinaryExprLayoutAction{
					Target:          "expr",
					LogicalStyle:    style,
					ArithmeticStyle: arithStyle,
				},
				Else: &BreakAtOpAction{
					Target:     "expr",
					BreakAfter: true,
				},
			},
		},

		// Assignment with long type assertion RHS. Calls are handled by
		// multiline-call rules, but type assertions can only be
		// shortened by moving the RHS onto a continuation line.
		{
			Name: "assignment_with_long_type_assert_rhs",
			Pattern: &NodePattern{
				Type: "AssignStmt",
				Fields: map[string]FieldMatch{
					"rhs": {
						Capture: "expr",
						SubPattern: &NodePattern{
							Type: "TypeAssertExpr",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&ExprEditSafeCond{
						Target: "expr",
					},
				},
			},
			Priority: 31,
			Action: &BreakAssignRHSAction{
				Target: "node",
			},
		},

		// Value spec with a long generic conversion value. This covers
		// compile-time interface assertions such as:
		// var _ Interface[T, U] = (*Concrete[T, U])(nil)
		{
			Name: "value_spec_with_long_generic_conversion",
			Pattern: &NodePattern{
				Type: "ValueSpec",
			},
			When: &LineWidthCond{
				Target: "node",
				Op:     ">",
				Value:  0,
			},
			Priority: 30,
			Action: &BreakValueSpecGenericConversionAction{
				Target: "node",
			},
		},
	}
}

// SignatureConfig holds optional formatters for signature rules.
type SignatureConfig struct {
	FuncFormatter   SignatureFormatFunc
	MethodFormatter SignatureFormatFunc
}

// SignatureRules returns rules for function signature formatting. Use this for
// isolated testing of signature breaking. The optional config parameter allows
// injecting the legacy formatters.
func SignatureRules(config ...SignatureConfig) []Rule {
	action := &BreakFuncSignatureAction{Target: "node"}
	methodAction := &BreakInterfaceMethodAction{Target: "node"}

	if len(config) > 0 {
		applySignatureConfig(config[0], action, methodAction)
	}

	return []Rule{
		// Function declarations - trigger when any line exceeds limit
		// OR when there are nested func types with multiline content
		// (for readability)
		{
			Name: "long_func_decl",
			Pattern: &NodePattern{
				Type: "FuncDecl",
			},
			When: &OrCond{
				Conds: []Condition{
					&AnyLineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&HasNestedMultilineTypeCond{
						Target: "node",
					},
				},
			},
			Priority: 90,
			Action:   action,
		},
		// Interface method declarations
		{
			Name: "long_interface_method",
			Pattern: &NodePattern{
				Type: "Field",
			},
			When: &AndCond{
				Conds: []Condition{
					&IsInterfaceMethodCond{
						Target: "node",
					},
					&AnyLineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
				},
			},
			Priority: 90,
			Action:   methodAction,
		},
	}
}

func applySignatureConfig(cfg SignatureConfig, action *BreakFuncSignatureAction,
	methodAction *BreakInterfaceMethodAction) {

	if cfg.FuncFormatter != nil {
		action.FormatFunc = cfg.FuncFormatter
	}
	if cfg.MethodFormatter != nil {
		methodAction.FormatFunc = cfg.MethodFormatter
	}
}

// MethodChainRules returns rules for method chain formatting. Use this for
// isolated testing of method chain breaking.
func MethodChainRules() []Rule {
	return []Rule{
		{
			Name: "long_method_chain_assign",
			Pattern: &NodePattern{
				Type: "AssignStmt",
				Fields: map[string]FieldMatch{
					"rhs": {
						Capture: "call",
						SubPattern: &NodePattern{
							Type: "CallExpr",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&IsMethodChainCond{
						Target:   "call",
						MinCalls: 2,
					},
				},
			},
			Priority: 80,
			Action: &BreakMethodChainAction{
				Target: "call",
			},
		},
		{
			Name: "long_method_chain_return",
			Pattern: &NodePattern{
				Type: "ReturnStmt",
				Fields: map[string]FieldMatch{
					"results": {
						Capture: "call",
						SubPattern: &NodePattern{
							Type: "CallExpr",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&IsMethodChainCond{
						Target:   "call",
						MinCalls: 2,
					},
				},
			},
			Priority: 80,
			Action: &BreakMethodChainAction{
				Target: "call",
			},
		},
	}
}

// ExpressionRules returns rules for expression formatting (logical chains,
// etc). Use this for isolated testing of expression breaking.
func ExpressionRules() []Rule {
	rules := []Rule{
		// Signature rules - high priority
		{
			Name: "long_func_decl",
			Pattern: &NodePattern{
				Type: "FuncDecl",
			},
			When: &OrCond{
				Conds: []Condition{
					&AnyLineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&HasNestedMultilineTypeCond{
						Target: "node",
					},
				},
			},
			Priority: 90,
			// Keep ExpressionRules legacy-parity: signatures are
			// formatted with a conservative fallback (no multiline
			// `(\n...\n)` packing) to match existing fixtures.
			Action: &BreakFuncSignatureAction{
				Target:     "node",
				FormatFunc: formatSignatureCompat,
			},
		},

		// Method chain rules
		{
			Name: "long_method_chain_assign",
			Pattern: &NodePattern{
				Type: "AssignStmt",
				Fields: map[string]FieldMatch{
					"rhs": {
						Capture: "call",
						SubPattern: &NodePattern{
							Type: "CallExpr",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&IsMethodChainCond{
						Target:   "call",
						MinCalls: 2,
					},
				},
			},
			Priority: 80,
			Action: &BreakMethodChainAction{
				Target: "call",
			},
		},

		// Assignment with long call (non-method-chain)
		{
			Name: "assignment_with_long_call",
			Pattern: &NodePattern{
				Type: "AssignStmt",
				Fields: map[string]FieldMatch{
					"lhs": {
						Capture: "var",
					},
					"rhs": {
						Capture: "rhs",
						SubPattern: &NodePattern{
							Type: "CallExpr",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&NotCond{
						Cond: &IsLogOrPrintfCallCond{
							Target: "rhs",
						},
					},
					&NotCond{
						Cond: &IsMethodChainCond{
							Target:   "rhs",
							MinCalls: 2,
						},
					},
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
						Capture: "cond",
						SubPattern: &NodePattern{
							Type: "BinaryExpr",
						},
					},
				},
			},
			When: &LineWidthCond{
				Target: "node",
				Op:     ">",
				Value:  0,
			},
			Priority: 45,
			Action: &TryElseAction{
				Try: &BreakBinaryExprLayoutAction{
					Target: "cond",
				},
				Else: &BreakAtOpAction{
					Target:     "cond",
					BreakAfter: true,
				},
			},
		},

		// Multiline call rules for nested function calls
		{
			Name: "long_call_expr",
			Pattern: &NodePattern{
				Type: "CallExpr",
			},
			When: &AndCond{
				Conds: []Condition{
					&CollapsedWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&NotCond{
						Cond: &IsLogOrPrintfCallCond{
							Target: "node",
						},
					},
					&NotCond{
						Cond: &IsMethodChainCond{
							Target:   "node",
							MinCalls: 2,
						},
					},
				},
			},
			Priority: 50,
			Action: &OnePerLineMultiLineCallAction{
				Target: "node",
			},
		},

		// Never break simple comparisons
		{
			Name: "keep_simple_comparison",
			Pattern: &NodePattern{
				Type: "BinaryExpr",
				Fields: map[string]FieldMatch{
					"op": {
						OneOf: []string{
							"==",
							"!=",
							"<",
							">",
							"<=",
							">=",
						},
					},
					"right": {
						Capture: "r",
					},
				},
			},
			When: &IsSimpleLiteralCond{
				Target: "r",
			},
			Priority: 100,
			Action: &KeepTogetherAction{
				Target: "node",
			},
		},

		// Long logical chain with function calls
		{
			Name: "logical_chain_with_call",
			Pattern: &NodePattern{
				Type: "BinaryExpr",
				Fields: map[string]FieldMatch{
					"op": {
						OneOf: []string{
							"&&",
							"||",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&MaxLineWidthInSpanCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&HasCallExprCond{
						Target: "node",
					},
				},
			},
			Priority: 40,
			Action: &TryElseAction{
				Try: &ReflowNestedCallsAction{
					Target:   "node",
					Strategy: StrategyOnePerLine,
				},
				Else: &BreakLogicalChainPackedAction{
					Target: "node",
				},
			},
		},

		// Return statement with long binary expression (excluding
		// string concat)
		{
			Name: "return_with_long_binary",
			Pattern: &NodePattern{
				Type: "ReturnStmt",
				Fields: map[string]FieldMatch{
					"results": {
						Capture: "expr",
						SubPattern: &NodePattern{
							Type: "BinaryExpr",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&NotCond{
						Cond: &IsStringConcatCond{
							Target: "expr",
						},
					},
				},
			},
			Priority: 35,
			Action: &TryElseAction{
				Try: &BreakBinaryExprLayoutAction{
					Target: "expr",
				},
				Else: &BreakAtOpAction{
					Target:     "expr",
					BreakAfter: true,
				},
			},
		},

		// Long case clause
		{
			Name: "long_case_clause",
			Pattern: &NodePattern{
				Type: "CaseClause",
			},
			When: &LineWidthCond{
				Target: "node",
				Op:     ">",
				Value:  0,
			},
			Priority: 35,
			Action: &BreakCaseClauseAction{
				Target: "node",
			},
		},

		// Long logical chain without calls
		{
			Name: "long_logical_chain",
			Pattern: &NodePattern{
				Type: "BinaryExpr",
				Fields: map[string]FieldMatch{
					"op": {
						OneOf: []string{
							"&&",
							"||",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&MaxLineWidthInSpanCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&NotCond{
						Cond: &HasCallExprCond{
							Target: "node",
						},
					},
				},
			},
			Priority: 30,
			Action: &BreakLogicalChainPackedAction{
				Target: "node",
			},
		},

		// Assignment with long binary expression (not call)
		{
			Name: "assignment_with_long_binary",
			Pattern: &NodePattern{
				Type: "AssignStmt",
				Fields: map[string]FieldMatch{
					"rhs": {
						Capture: "expr",
						SubPattern: &NodePattern{
							Type: "BinaryExpr",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&NotCond{
						Cond: &HasCallExprCond{
							Target: "expr",
						},
					},
				},
			},
			Priority: 32,
			Action: &TryElseAction{
				Try: &BreakBinaryExprLayoutAction{
					Target: "expr",
				},
				Else: &BreakAtOpAction{
					Target:     "expr",
					BreakAfter: true,
				},
			},
		},

		// Long arithmetic expression (excluding string concat)
		{
			Name: "long_arithmetic_expr",
			Pattern: &NodePattern{
				Type: "BinaryExpr",
				Fields: map[string]FieldMatch{
					"op": {
						OneOf: []string{
							"+",
							"-",
							"*",
							"/",
							"%",
						},
					},
				},
			},
			When: &AndCond{
				Conds: []Condition{
					&LineWidthCond{
						Target: "node",
						Op:     ">",
						Value:  0,
					},
					&NotCond{
						Cond: &IsStringConcatCond{
							Target: "node",
						},
					},
					&NotCond{
						Cond: &IsCallArgCond{
							Target: "node",
						},
					},
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
