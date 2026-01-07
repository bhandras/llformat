package formatter

import (
	"github.com/bhandras/llformat/dsl"
	"github.com/bhandras/llformat/internal/compat"
)

// dslRulesForComments returns the DSL rule list for the comment stage.
//
// llformat is next-only, but comment formatting still delegates to the legacy
// comment formatter implementation (now located under internal/compat) because
// it remains the spec oracle for comment reflow semantics.
func dslRulesForComments(commentMoveInline bool) []dsl.Rule {
	return dsl.LegacyCommentRules(
		compat.FormatCommentsInSource, commentMoveInline,
	)
}

// dslRulesForLogCalls returns the DSL rule list for the log/printf call stage.
//
// This uses the call-formatting helpers from compact_call_formatter.go (the
// formatter core), with next-specific defaults.
func dslRulesForLogCalls(opts StageOptions) []dsl.Rule {
	minTailLen := opts.Style.DSLLogCallsMinTailLen
	if minTailLen == 0 {
		minTailLen = 8
	}
	formatFunc := func(call []byte, wsIndent string, baseLen int, colLimit,
		ts int) string {

		return formatCallGreedyNextWithMinTailLen(
			call, wsIndent, baseLen, colLimit, ts, minTailLen,
		)
	}

	return dsl.LogPrintfRulesWithOptions(
		dsl.LogPrintfOptions{
			MatchAnySelectorPrefix: true,
			SelectorNames:          opts.Style.DSLLogCallsSelectorNames,
			SelectorPrefixes:       opts.Style.DSLLogCallsSelectorPrefixes,
			IncludeNonFStringCalls: true,
		},
		formatFunc,
	)
}

func dslRulesForExpr(opts StageOptions) []dsl.Rule {
	callArgsPolicy := dsl.CallArgsPolicyOff
	if opts.DSL.AutoCallArgs {
		callArgsPolicy = dsl.CallArgsPolicyAuto
	}
	if opts.DSL.AllowCallArgs {
		callArgsPolicy = dsl.CallArgsPolicyForce
	}

	allowlist := opts.Style.Excludes
	if opts.DSL.AutoCallArgs {
		allowlist = append(
			append(
				[]string{}, DefaultMultilineExcludes()...,
			),
			opts.Style.Excludes...,
		)
	}

	// "next" defaults:
	// - selector chains: layout-based breaking
	// - logical/arithmetic/case: legacy packed breaking unless explicitly
	//   set
	logicalStyle := opts.Style.DSLExprLogicalStyle
	arithStyle := opts.Style.DSLExprArithmeticStyle
	caseClauseStyle := opts.Style.DSLExprCaseClauseStyle
	selectorChainStyle := opts.Style.DSLExprSelectorChainStyle
	if selectorChainStyle == "" {
		selectorChainStyle = "layout"
	}

	exprRules := dsl.LongExprRulesWithOptions(
		dsl.LongExprOptions{
			CallArgsPolicy:       callArgsPolicy,
			CallArgsAllowlist:    allowlist,
			LogicalChainStyle:    logicalStyle,
			ArithmeticChainStyle: arithStyle,
			CaseClauseStyle:      caseClauseStyle,
			SelectorChainStyle:   selectorChainStyle,
		},
	)

	// In next mode, the "expression" stage also owns formatting for
	// composite literals and single-line function literals.
	exprRules = append(exprRules, dsl.ExpandFuncLitBodyRules()...)
	exprRules = append(exprRules, dsl.ExpandCompositeLitRules()...)
	exprRules = append(
		exprRules, dsl.SplitLongStringLiteralRules(
			dsl.SplitLongStringLiteralOptions{
				MinTailLen: 0,
			},
		)...,
	)

	return exprRules
}

func dslRulesForMultiLineCalls(opts StageOptions) (rules []dsl.Rule,
	nodeOrder dsl.NodeOrder) {

	style := opts.Style.DSLMultiLineStyle
	if style == "" {
		style = "packed-chain-layout"
	}

	packedFallback := FormatCallPackedMultiLineNext

	nodeOrder = dsl.NodeOrderPreorder
	switch style {
	case "layout-args", "layout-all", "layout-args-groups-pairs",
		"layout-all-groups-pairs":

		nodeOrder = dsl.NodeOrderDeepestFirst
	}

	optsCommon := dsl.MultiLineCallOptions{
		Excludes:                opts.Style.Excludes,
		LogCallSelectorNames:    opts.Style.DSLLogCallsSelectorNames,
		LogCallSelectorPrefixes: opts.Style.DSLLogCallsSelectorPrefixes,
		DisableBreakBeforeCallOnLongMultiAssignPrefix: true,
		CheckMaxSpanLineWidth:                         true,
	}

	switch style {
	case "packed":
		rules = dsl.PackedMultiLineOnlyRulesWithOptions(
			optsCommon, packedFallback,
		)

	case "packed-chain":
		rules = dsl.MultiLineCallRulesWithOptions(
			optsCommon, packedFallback,
		)

	case "layout-args":
		o := optsCommon
		o.CallArgsStyle = "layout"
		rules = dsl.MultiLineCallRulesWithOptions(o, packedFallback)

	case "layout-args-groups-pairs":
		o := optsCommon
		o.CallArgsStyle = "layout"
		o.CallArgsGrouping = "pairs"
		rules = dsl.MultiLineCallRulesWithOptions(o, packedFallback)

	case "packed-chain-layout", "layout-chain":
		o := optsCommon
		o.MethodChainStyle = "layout"
		rules = dsl.MultiLineCallRulesWithOptions(o, packedFallback)

	case "layout-all":
		o := optsCommon
		o.MethodChainStyle = "layout"
		o.CallArgsStyle = "layout"
		rules = dsl.MultiLineCallRulesWithOptions(o, packedFallback)

	case "layout-all-groups-pairs":
		o := optsCommon
		o.MethodChainStyle = "layout"
		o.CallArgsStyle = "layout"
		o.CallArgsGrouping = "pairs"
		rules = dsl.MultiLineCallRulesWithOptions(o, packedFallback)

	default:
		// Unknown style: fall back to packed multiline.
		rules = dsl.PackedMultiLineOnlyRulesWithOptions(
			optsCommon, packedFallback,
		)
	}

	return rules, nodeOrder
}

func dslRulesForSignatures(opts StageOptions) []dsl.Rule {
	// Signature selection is intentionally conservative: native rules are
	// opt-in.
	if !opts.DSL.UseFuncSigsNative {
		return dsl.LegacyFuncSigRules(FormatFuncSigsInSource)
	}

	style := opts.Style.DSLSigsStyle
	if style == "" {
		style = "legacy"
	}

	var rules []dsl.Rule
	switch style {
	case "legacy":
		funcFormatter := FormatFuncSignatureNext
		methodFormatter := func(signature, indent string, colLimit,
			tabStop int) (string, bool) {

			return FormatInterfaceMethodNext(
				signature, indent, colLimit, tabStop,
			), false
		}

		rules = append(
			[]dsl.Rule{}, dsl.SignatureRules(
				dsl.SignatureConfig{
					FuncFormatter:   funcFormatter,
					MethodFormatter: methodFormatter,
				},
			)...,
		)

		// Reflow already-multiline signatures + func literals.
		funcAction := rules[0].Action
		var methodAction dsl.Action
		if len(rules) > 1 {
			methodAction = rules[1].Action
		}

		rules = append(
			rules, dsl.Rule{
				Name:     "multiline_func_decl",
				Pattern:  &dsl.NodePattern{Type: "FuncDecl"},
				When:     &dsl.HasMultilineFuncSignatureCond{Target: "node"},
				Priority: 89,
				Action:   funcAction,
			},
		)
		if methodAction != nil {
			rules = append(
				rules, dsl.Rule{
					Name:    "multiline_interface_method",
					Pattern: &dsl.NodePattern{Type: "Field"},
					When: &dsl.AndCond{
						Conds: []dsl.Condition{
							&dsl.IsInterfaceMethodCond{Target: "node"},
							&dsl.HasMultilineInterfaceMethodCond{Target: "node"},
						},
					},
					Priority: 89,
					Action:   methodAction,
				},
			)
		}

		rules = append(
			rules, dsl.Rule{
				Name:    "func_lit_signature",
				Pattern: &dsl.NodePattern{Type: "FuncLit"},
				When: &dsl.OrCond{
					Conds: []dsl.Condition{
						&dsl.AnyLineWidthFuncLitSignatureCond{Target: "node", Op: ">", Value: 0},
						&dsl.HasMultilineFuncLitSignatureCond{Target: "node"},
					},
				},
				Priority: 89,
				Action: &dsl.BreakFuncLitSignatureAction{
					Target:     "node",
					FormatFunc: funcFormatter,
				},
			},
		)

	case "dsl":
		rules = append([]dsl.Rule{}, dsl.SignatureRules()...)

	default:
		rules = append(
			[]dsl.Rule{}, dsl.SignatureRules(
				dsl.SignatureConfig{
					FuncFormatter: FormatFuncSignatureNext,
					MethodFormatter: func(signature,
						indent string, colLimit,
						tabStop int) (string, bool) {

						return FormatInterfaceMethodNext(
							signature, indent,
							colLimit, tabStop,
						), false
					},
				},
			)...,
		)
	}

	// Final fallback for corner cases.
	rules = append(
		rules,
		dsl.LegacyFuncSigFallbackRules(FormatFuncSigsInSource)...,
	)

	return rules
}

func dslRulesForBlankLines(opts StageOptions) []dsl.Rule {
	blankOpts := dsl.BlankLineOptions{
		ExtraIfErrReturn: opts.Style.DSLBlankLinesExtraIfErrReturn,
	}
	rules := append(
		[]dsl.Rule{}, dsl.BlankLineRulesWithOptions(blankOpts)...,
	)

	// Extra readability rules: when a control statement header is already
	// multiline, insert a blank line before the first statement in the
	// body/case.
	rules = append(
		rules, dsl.Rule{
			Name:     "blank_after_multiline_if_header",
			Pattern:  &dsl.NodePattern{Type: "IfStmt"},
			When:     &dsl.HasMultilineIfHeaderCond{Target: "node"},
			Priority: 9,
			Action:   &dsl.InsertBlankBeforeFirstStmtInBlockAction{Target: "node"},
		}, dsl.Rule{
			Name:     "blank_after_multiline_for_header",
			Pattern:  &dsl.NodePattern{Type: "ForStmt"},
			When:     &dsl.HasMultilineForHeaderCond{Target: "node"},
			Priority: 9,
			Action:   &dsl.InsertBlankBeforeFirstStmtInBlockAction{Target: "node"},
		}, dsl.Rule{
			Name:     "blank_after_multiline_case_header",
			Pattern:  &dsl.NodePattern{Type: "CaseClause"},
			When:     &dsl.HasMultilineCaseHeaderCond{Target: "node"},
			Priority: 9,
			Action:   &dsl.InsertBlankBeforeFirstStmtInBlockAction{Target: "node"},
		},
	)

	return rules
}
