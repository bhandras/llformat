package formatter

import "github.com/lightninglabs/llformat/dsl"

func normalizedRuleProfile(profile string) string {
	if profile == "" {
		return "parity"
	}
	return profile
}

// dslRulesForComments returns the DSL rule list for the comment stage.
// This intentionally delegates to the legacy comment formatter for parity.
func dslRulesForComments(commentMoveInline bool) []dsl.Rule {
	return dsl.LegacyCommentRules(FormatCommentsInSource, commentMoveInline)
}

// dslRulesForLogCalls returns the DSL rule list for the log/printf call stage.
// This uses the legacy call formatter implementation to ensure parity.
func dslRulesForLogCalls() []dsl.Rule {
	return dsl.LogPrintfRules(FormatCallGreedy)
}

func dslRulesForExpr(opts StageOptions) []dsl.Rule {
	profile := normalizedRuleProfile(opts.RuleProfile)

	exprRules := dsl.LongExprRules()
	if profile != "parity" || opts.AllowDSLCallArgs || opts.AutoDSLCallArgs || opts.DSLExprLogicalStyle != "" || opts.DSLExprArithmeticStyle != "" || opts.DSLExprCaseClauseStyle != "" || opts.DSLExprSelectorChainStyle != "" {
		callArgsPolicy := dsl.CallArgsPolicyOff
		if opts.AutoDSLCallArgs {
			callArgsPolicy = dsl.CallArgsPolicyAuto
		}
		if opts.AllowDSLCallArgs {
			callArgsPolicy = dsl.CallArgsPolicyForce
		}

		allowlist := opts.Excludes
		if opts.AutoDSLCallArgs {
			allowlist = append(append([]string{}, DefaultMultilineExcludes()...), opts.Excludes...)
		}

		logicalStyle := opts.DSLExprLogicalStyle
		arithStyle := opts.DSLExprArithmeticStyle
		caseClauseStyle := opts.DSLExprCaseClauseStyle
		selectorChainStyle := opts.DSLExprSelectorChainStyle
		if profile == "modern" || profile == "next" {
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
	return exprRules
}

func dslRulesForMultiLineCalls(opts StageOptions) (rules []dsl.Rule, nodeOrder dsl.NodeOrder) {
	style := opts.DSLMultiLineStyle
	if style == "" {
		switch normalizedRuleProfile(opts.RuleProfile) {
		case "modern":
			style = "packed-chain-layout"
		case "next":
			style = "layout-all"
		default:
			style = "legacy"
		}
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
	case "legacy", "legacy-scan", "scan":
		rules = dsl.LegacyMultiLineScanRulesWithOptions(
			dsl.MultiLineCallOptions{Excludes: opts.Excludes},
			FormatOneMultiLineCallInSource,
		)
	case "packed":
		rules = dsl.PackedMultiLineOnlyRulesWithOptions(
			dsl.MultiLineCallOptions{Excludes: opts.Excludes},
			FormatCallPackedMultiLine,
		)
	case "packed-chain":
		rules = dsl.MultiLineCallRulesWithOptions(
			dsl.MultiLineCallOptions{Excludes: opts.Excludes},
			FormatCallPackedMultiLine,
		)
	case "layout-args":
		// Try layout-based argument breaking, fall back to packed multiline.
		rules = dsl.MultiLineCallRulesWithOptions(
			dsl.MultiLineCallOptions{Excludes: opts.Excludes, CallArgsStyle: "layout"},
			FormatCallPackedMultiLine,
		)
	case "layout-args-groups-pairs":
		// Layout-based argument breaking with explicit pair grouping, falling
		// back to packed multiline when the layout formatter can't safely run.
		rules = dsl.MultiLineCallRulesWithOptions(
			dsl.MultiLineCallOptions{
				Excludes:         opts.Excludes,
				CallArgsStyle:    "layout",
				CallArgsGrouping: "pairs",
			},
			FormatCallPackedMultiLine,
		)
	case "packed-chain-layout", "layout-chain":
		rules = dsl.MultiLineCallRulesWithOptions(
			dsl.MultiLineCallOptions{Excludes: opts.Excludes, MethodChainStyle: "layout"},
			FormatCallPackedMultiLine,
		)
	case "layout-all":
		// Layout-based method-chain breaking + layout-based call-arg breaking.
		rules = dsl.MultiLineCallRulesWithOptions(
			dsl.MultiLineCallOptions{
				Excludes:         opts.Excludes,
				MethodChainStyle: "layout",
				CallArgsStyle:    "layout",
			},
			FormatCallPackedMultiLine,
		)
	case "layout-all-groups-pairs":
		rules = dsl.MultiLineCallRulesWithOptions(
			dsl.MultiLineCallOptions{
				Excludes:         opts.Excludes,
				MethodChainStyle: "layout",
				CallArgsStyle:    "layout",
				CallArgsGrouping: "pairs",
			},
			FormatCallPackedMultiLine,
		)
	default:
		// Unknown style: fall back to legacy parity mode.
		rules = dsl.LegacyMultiLineScanRulesWithOptions(
			dsl.MultiLineCallOptions{Excludes: opts.Excludes},
			FormatOneMultiLineCallInSource,
		)
	}

	return rules, nodeOrder
}

func dslRulesForSignatures(opts StageOptions) []dsl.Rule {
	// Signature rule selection is intentionally kept consistent with the legacy
	// stage so golden fixtures remain authoritative.
	if !opts.UseDSLFuncSigsNative {
		return dsl.LegacyFuncSigRules(FormatFuncSigsInSource)
	}

	style := opts.DSLSigsStyle
	if style == "" {
		style = "legacy"
	}

	var rules []dsl.Rule
	switch style {
	case "legacy":
		rules = append([]dsl.Rule{}, dsl.SignatureRules(dsl.SignatureConfig{
			FuncFormatter:   FormatFuncSignatureLegacy,
			MethodFormatter: FormatInterfaceMethodLegacy,
		})...)
	case "dsl":
		// Pure DSL signature formatting (fallback algorithms).
		rules = append([]dsl.Rule{}, dsl.SignatureRules()...)
	default:
		rules = append([]dsl.Rule{}, dsl.SignatureRules(dsl.SignatureConfig{
			FuncFormatter:   FormatFuncSignatureLegacy,
			MethodFormatter: FormatInterfaceMethodLegacy,
		})...)
	}

	// Always include a final fallback for cases the native rules cannot handle.
	rules = append(rules, dsl.LegacyFuncSigFallbackRules(FormatFuncSigsInSource)...)
	return rules
}

func dslRulesForBlankLines(opts StageOptions) []dsl.Rule {
	if !opts.UseDSLBlankLinesNative {
		return dsl.LegacyBlankLinesRules(FormatBlankLinesInSource)
	}

	// Native DSL blank line rules, with a legacy fallback for unparsable sources
	// (and as a last resort).
	blankOpts := dsl.BlankLineOptions{
		ExtraIfErrReturn: normalizedRuleProfile(opts.RuleProfile) == "next",
	}
	rules := append([]dsl.Rule{}, dsl.BlankLineRulesWithOptions(blankOpts)...)
	rules = append(rules, dsl.LegacyBlankLinesFallbackRules(FormatBlankLinesInSource)...)
	return rules
}
