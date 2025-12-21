package formatter

import (
	"github.com/lightninglabs/llformat/dsl"
	"github.com/lightninglabs/llformat/internal/compat"
)

func normalizedRuleProfile(profile string) string {
	if profile == "" {
		return "next"
	}
	if profile == "parity" {
		// Legacy profile name retained for compatibility; llformat is now next-only.
		return "next"
	}
	return profile
}

// dslRulesForComments returns the DSL rule list for the comment stage.
// This intentionally delegates to the legacy comment formatter for parity.
func dslRulesForComments(commentMoveInline bool) []dsl.Rule {
	return dsl.LegacyCommentRules(compat.FormatCommentsInSource, commentMoveInline)
}

// dslRulesForLogCalls returns the DSL rule list for the log/printf call stage.
// This uses the legacy call formatter implementation to ensure parity.
func dslRulesForLogCalls(opts StageOptions) []dsl.Rule {
	profile := normalizedRuleProfile(opts.Selection.RuleProfile)
	// Both "modern" and "next" opt into suffix-only matching for selectors so
	// custom loggers (e.g. `rpcSLog.Errorf`) are formatted the same way as
	// `log.Errorf`/`fmt.Errorf`.
	matchAnyPrefix := profile == "modern" || profile == "next"

	formatFunc := FormatCallGreedy
	if profile == "next" {
		minTailLen := opts.Style.DSLLogCallsMinTailLen
		if minTailLen == 0 {
			minTailLen = 8
		}
		formatFunc = func(call []byte, wsIndent string, baseLen int, colLimit, ts int) string {
			return formatCallGreedyNextWithMinTailLen(call, wsIndent, baseLen, colLimit, ts, minTailLen)
		}
	}

	return dsl.LogPrintfRulesWithOptions(
		dsl.LogPrintfOptions{
			MatchAnySelectorPrefix: matchAnyPrefix,
			// In next we also want to format non-`*f` log calls that take a single
			// string message (custom loggers commonly use `.Info/.Error`).
			IncludeNonFStringCalls: profile == "next",
		},
		formatFunc,
	)
}

func dslRulesForExpr(opts StageOptions) []dsl.Rule {
	profile := normalizedRuleProfile(opts.Selection.RuleProfile)

	exprRules := dsl.LongExprRules()
	if profile != "parity" || opts.DSL.AllowCallArgs || opts.DSL.AutoCallArgs || opts.Style.DSLExprLogicalStyle != "" || opts.Style.DSLExprArithmeticStyle != "" || opts.Style.DSLExprCaseClauseStyle != "" || opts.Style.DSLExprSelectorChainStyle != "" {
		callArgsPolicy := dsl.CallArgsPolicyOff
		if opts.DSL.AutoCallArgs {
			callArgsPolicy = dsl.CallArgsPolicyAuto
		}
		if opts.DSL.AllowCallArgs {
			callArgsPolicy = dsl.CallArgsPolicyForce
		}

		allowlist := opts.Style.Excludes
		if opts.DSL.AutoCallArgs {
			allowlist = append(append([]string{}, DefaultMultilineExcludes()...), opts.Style.Excludes...)
		}

		logicalStyle := opts.Style.DSLExprLogicalStyle
		arithStyle := opts.Style.DSLExprArithmeticStyle
		caseClauseStyle := opts.Style.DSLExprCaseClauseStyle
		selectorChainStyle := opts.Style.DSLExprSelectorChainStyle
		switch profile {
		case "modern":
			// "modern" opts into the layout engine's Go-style "break at every
			// operator/comma once broken" behavior.
			if logicalStyle == "" {
				logicalStyle = "layout"
			}
			if arithStyle == "" {
				arithStyle = "layout"
			}
			if caseClauseStyle == "" {
				caseClauseStyle = "layout"
			}
			if selectorChainStyle == "" {
				selectorChainStyle = "layout"
			}
		case "next":
			// "next" is more conservative for expression chains: prefer a
			// packed/greedy style (legacy breaker) that breaks only as much as
			// needed to satisfy the column limit, while still opting into
			// layout-based selector-chain breaking.
			if selectorChainStyle == "" {
				selectorChainStyle = "layout"
			}
			// Leave logical/arithmetic/case styles empty (=> legacy) unless the
			// caller explicitly requests a style.
		}

		exprRules = dsl.LongExprRulesWithOptions(dsl.LongExprOptions{
			CallArgsPolicy:       callArgsPolicy,
			CallArgsAllowlist:    allowlist,
			LogicalChainStyle:    logicalStyle,
			ArithmeticChainStyle: arithStyle,
			CaseClauseStyle:      caseClauseStyle,
			SelectorChainStyle:   selectorChainStyle,
		})
	}
	// The "next" profile also splits long single string literals used as call
	// arguments into concatenations. This is intentionally separate from the
	// string-concat reflow rule (which only rewrites existing concatenations).
	if profile == "next" {
		// In next mode, the "expression" stage also owns formatting for composite
		// literals and single-line function literals, since they frequently appear
		// inside call args and long lines.
		exprRules = append(exprRules,
			dsl.ExpandFuncLitBodyRules()...,
		)
		exprRules = append(exprRules,
			dsl.ExpandCompositeLitRules()...,
		)
		exprRules = append(exprRules, dsl.SplitLongStringLiteralRules(dsl.SplitLongStringLiteralOptions{
			// Unlike the printf/logcall formatter, generic string splitting should
			// be willing to create short tails (e.g. "%v: %w") to preserve the
			// most natural wrap point.
			MinTailLen: 0,
		})...)
	}

	return exprRules
}

func dslRulesForMultiLineCalls(opts StageOptions) (rules []dsl.Rule, nodeOrder dsl.NodeOrder) {
	profile := normalizedRuleProfile(opts.Selection.RuleProfile)

	style := opts.Style.DSLMultiLineStyle
	if style == "" {
		switch profile {
		case "modern":
			style = "packed-chain-layout"
		case "next":
			// The "next" profile defaults to packed multiline call formatting only.
			// Method-chain breaking is handled by the expression stage (and can be
			// explicitly enabled here via `--dsl-multiline-style`).
			style = "packed"
		default:
			style = "packed"
		}
	}

	packedFallback := FormatCallPackedMultiLine
	if profile == "next" {
		packedFallback = FormatCallPackedMultiLineNext
	}

	nodeOrder = dsl.NodeOrderPreorder
	// For layout-based call-arg formatting, process inner nodes first to
	// avoid non-idempotent “outer before inner” rewrites (e.g. nested calls
	// where the inner call is broken after the outer call has already decided
	// how to pack its arguments).
	switch style {
	case "layout-args", "layout-all", "layout-args-groups-pairs", "layout-all-groups-pairs":
		nodeOrder = dsl.NodeOrderDeepestFirst
	}

	switch style {
	case "packed":
		rules = dsl.PackedMultiLineOnlyRulesWithOptions(
			dsl.MultiLineCallOptions{
				Excludes: opts.Style.Excludes,
				// In next, keep multi-assign statements visually intact by
				// formatting the call itself as packed multiline rather than
				// detaching it from `:=`.
				DisableBreakBeforeCallOnLongMultiAssignPrefix: profile == "next",
				CheckMaxSpanLineWidth:                         profile == "next",
			},
			packedFallback,
		)
	case "packed-chain":
		rules = dsl.MultiLineCallRulesWithOptions(
			dsl.MultiLineCallOptions{
				Excludes: opts.Style.Excludes,
				DisableBreakBeforeCallOnLongMultiAssignPrefix: profile == "next",
				CheckMaxSpanLineWidth:                         profile == "next",
			},
			packedFallback,
		)
	case "layout-args":
		// Try layout-based argument breaking, fall back to packed multiline.
		rules = dsl.MultiLineCallRulesWithOptions(
			dsl.MultiLineCallOptions{
				Excludes: opts.Style.Excludes, CallArgsStyle: "layout",
				DisableBreakBeforeCallOnLongMultiAssignPrefix: profile == "next",
				CheckMaxSpanLineWidth:                         profile == "next",
			},
			packedFallback,
		)
	case "layout-args-groups-pairs":
		// Layout-based argument breaking with explicit pair grouping, falling
		// back to packed multiline when the layout formatter can't safely run.
		rules = dsl.MultiLineCallRulesWithOptions(
			dsl.MultiLineCallOptions{
				Excludes:         opts.Style.Excludes,
				CallArgsStyle:    "layout",
				CallArgsGrouping: "pairs",
				DisableBreakBeforeCallOnLongMultiAssignPrefix: profile == "next",
				CheckMaxSpanLineWidth:                         profile == "next",
			},
			packedFallback,
		)
	case "packed-chain-layout", "layout-chain":
		rules = dsl.MultiLineCallRulesWithOptions(
			dsl.MultiLineCallOptions{
				Excludes: opts.Style.Excludes, MethodChainStyle: "layout",
				DisableBreakBeforeCallOnLongMultiAssignPrefix: profile == "next",
				CheckMaxSpanLineWidth:                         profile == "next",
			},
			packedFallback,
		)
	case "layout-all":
		// Layout-based method-chain breaking + layout-based call-arg breaking.
		rules = dsl.MultiLineCallRulesWithOptions(
			dsl.MultiLineCallOptions{
				Excludes:         opts.Style.Excludes,
				MethodChainStyle: "layout",
				CallArgsStyle:    "layout",
				DisableBreakBeforeCallOnLongMultiAssignPrefix: profile == "next",
				CheckMaxSpanLineWidth:                         profile == "next",
			},
			packedFallback,
		)
	case "layout-all-groups-pairs":
		rules = dsl.MultiLineCallRulesWithOptions(
			dsl.MultiLineCallOptions{
				Excludes:         opts.Style.Excludes,
				MethodChainStyle: "layout",
				CallArgsStyle:    "layout",
				CallArgsGrouping: "pairs",
				DisableBreakBeforeCallOnLongMultiAssignPrefix: profile == "next",
				CheckMaxSpanLineWidth:                         profile == "next",
			},
			packedFallback,
		)
	default:
		// Unknown style: fall back to packed multiline.
		rules = dsl.PackedMultiLineOnlyRulesWithOptions(
			dsl.MultiLineCallOptions{
				Excludes: opts.Style.Excludes,
				DisableBreakBeforeCallOnLongMultiAssignPrefix: profile == "next",
				CheckMaxSpanLineWidth:                         profile == "next",
			},
			packedFallback,
		)
	}

	return rules, nodeOrder
}

func dslRulesForSignatures(opts StageOptions) []dsl.Rule {
	// Signature rule selection is intentionally kept consistent with the legacy
	// stage so golden fixtures remain authoritative.
	if !opts.DSL.UseFuncSigsNative {
		return dsl.LegacyFuncSigRules(FormatFuncSigsInSource)
	}

	profile := normalizedRuleProfile(opts.Selection.RuleProfile)
	style := opts.Style.DSLSigsStyle
	if style == "" {
		style = "legacy"
	}

	var rules []dsl.Rule
	switch style {
	case "legacy":
		funcFormatter := FormatFuncSignatureLegacy
		methodFormatter := func(signature, indent string, colLimit, tabStop int) (string, bool) {
			return FormatInterfaceMethodLegacy(signature, indent, colLimit, tabStop), false
		}
		if profile == "next" {
			funcFormatter = FormatFuncSignatureNext
			methodFormatter = func(signature, indent string, colLimit, tabStop int) (string, bool) {
				return FormatInterfaceMethodNext(signature, indent, colLimit, tabStop), false
			}
		}
		rules = append([]dsl.Rule{}, dsl.SignatureRules(dsl.SignatureConfig{
			FuncFormatter:   funcFormatter,
			MethodFormatter: methodFormatter,
		})...)
		// In the "next" profile, also reflow signatures that are already multiline
		// even if no single line exceeds the column limit.
		if profile == "next" {
			// SignatureRules returns:
			// - rules[0] => BreakFuncSignatureAction
			// - rules[1] => BreakInterfaceMethodAction
			//
			// Reuse those actions so the injected formatters apply.
			var funcAction dsl.Action = rules[0].Action
			var methodAction dsl.Action
			if len(rules) > 1 {
				methodAction = rules[1].Action
			}

			rules = append(rules, dsl.Rule{
				Name:     "multiline_func_decl",
				Pattern:  &dsl.NodePattern{Type: "FuncDecl"},
				When:     &dsl.HasMultilineFuncSignatureCond{Target: "node"},
				Priority: 89,
				Action:   funcAction,
			})

			// Also reflow interface methods that are already multiline. This is
			// important for the next profile so we can collapse short multiline
			// return lists like:
			//   M() (
			//     *T,
			//     error)
			// into:
			//   M() (*T, error)
			if methodAction != nil {
				rules = append(rules, dsl.Rule{
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
				})
			}

			// Also format function literals (closures). These don't use the legacy
			// line-based signature formatter, so we add a dedicated rule that looks
			// only at the `func`..`{` signature span.
			rules = append(rules, dsl.Rule{
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
			})
		}
	case "dsl":
		// Pure DSL signature formatting (fallback algorithms).
		rules = append([]dsl.Rule{}, dsl.SignatureRules()...)
	default:
		rules = append([]dsl.Rule{}, dsl.SignatureRules(dsl.SignatureConfig{
			FuncFormatter: FormatFuncSignatureLegacy,
			MethodFormatter: func(signature, indent string, colLimit, tabStop int) (string, bool) {
				return FormatInterfaceMethodLegacy(signature, indent, colLimit, tabStop), false
			},
		})...)
	}

	// Always include a final fallback for cases the native rules cannot handle.
	rules = append(rules, dsl.LegacyFuncSigFallbackRules(FormatFuncSigsInSource)...)
	return rules
}

func dslRulesForBlankLines(opts StageOptions) []dsl.Rule {
	profile := normalizedRuleProfile(opts.Selection.RuleProfile)
	blankOpts := dsl.BlankLineOptions{
		ExtraIfErrReturn: opts.Style.DSLBlankLinesExtraIfErrReturn,
	}
	rules := append([]dsl.Rule{}, dsl.BlankLineRulesWithOptions(blankOpts)...)

	// Extra readability rules for "next": when a control statement header is
	// already multiline, insert a blank line before the first statement in the
	// body/case clause.
	if profile == "next" {
		rules = append(rules,
			dsl.Rule{
				Name:     "blank_after_multiline_if_header",
				Pattern:  &dsl.NodePattern{Type: "IfStmt"},
				When:     &dsl.HasMultilineIfHeaderCond{Target: "node"},
				Priority: 9,
				Action:   &dsl.InsertBlankBeforeFirstStmtInBlockAction{Target: "node"},
			},
			dsl.Rule{
				Name:     "blank_after_multiline_for_header",
				Pattern:  &dsl.NodePattern{Type: "ForStmt"},
				When:     &dsl.HasMultilineForHeaderCond{Target: "node"},
				Priority: 9,
				Action:   &dsl.InsertBlankBeforeFirstStmtInBlockAction{Target: "node"},
			},
			dsl.Rule{
				Name:     "blank_after_multiline_case_header",
				Pattern:  &dsl.NodePattern{Type: "CaseClause"},
				When:     &dsl.HasMultilineCaseHeaderCond{Target: "node"},
				Priority: 9,
				Action:   &dsl.InsertBlankBeforeFirstStmtInBlockAction{Target: "node"},
			},
		)
	}
	return rules
}
