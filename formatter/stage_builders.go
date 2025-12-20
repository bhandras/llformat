package formatter

import (
	llast "github.com/lightninglabs/llformat/ast"
	"github.com/lightninglabs/llformat/dsl"
)

func dslBudgetForRuleProfile(profile string) dsl.RewriteBudget {
	// Only "next" gets explicit safety guardrails by default; other profiles keep
	// behavior identical unless the caller opts into budgets.
	if normalizedRuleProfile(profile) != "next" {
		return dsl.RewriteBudget{}
	}

	// Large enough to never trigger for normal formatting runs, but small enough
	// to act as a safety valve against pathological rules.
	return dsl.RewriteBudget{
		MaxOutputBytes:   2 << 20, // 2 MiB
		MaxBytesIncrease: 1 << 20, // 1 MiB
	}
}

func buildCommentStageFormatter(stageName string, cfg BaseConfig, opts StageOptions, plan StagePlan, bundle DSLBundle) Formatter {
	if plan.Comments != StageModeDSL {
		return NewCommentFormatter(CommentConfig{
			ColumnLimit:     cfg.ColumnLimit,
			TabStop:         cfg.TabStop,
			MoveInlineAbove: opts.Style.CommentMoveInline,
		})
	}

	return NewDSLExprFormatter(DSLExprConfig{
		ColumnLimit:   cfg.ColumnLimit,
		TabStop:       cfg.TabStop,
		Rules:         bundle.Comments.Rules,
		Trace:         opts.DSL.Trace,
		TraceReasons:  opts.DSL.TraceReasons,
		NodeOrder:     bundle.Comments.NodeOrder,
		MaxIterations: bundle.Comments.MaxIterations,
		SkipGofmt:     true,
		StageName:     stageName,
		Budget:        dslBudgetForRuleProfile(opts.Selection.RuleProfile),
	})
}

func buildCompactCallStageFormatter(stageName string, cfg BaseConfig, opts StageOptions, plan StagePlan, bundle DSLBundle) Formatter {
	if plan.LogCalls != StageModeDSL {
		return NewCompactCallFormatter(Config{
			ColumnLimit:     cfg.ColumnLimit,
			TabStop:         cfg.TabStop,
			UseASTSelection: opts.Legacy.CompactCallUseASTSelect,
			SkipGofmt:       true,
			ParseSafe:       opts.Legacy.CompactCallParseSafe,
		})
	}

	return NewDSLExprFormatter(DSLExprConfig{
		ColumnLimit:   cfg.ColumnLimit,
		TabStop:       cfg.TabStop,
		Rules:         bundle.LogCalls.Rules,
		Trace:         opts.DSL.Trace,
		TraceReasons:  opts.DSL.TraceReasons,
		NodeOrder:     bundle.LogCalls.NodeOrder,
		MaxIterations: bundle.LogCalls.MaxIterations,
		SkipGofmt:     true,
		StageName:     stageName,
		Budget:        dslBudgetForRuleProfile(opts.Selection.RuleProfile),
		OwnedSpansFunc: func(src []byte) llast.OffsetSpanSet {
			// Align ownership boundaries with legacy call selection for now.
			return NewCompactCallFormatter(Config{
				ColumnLimit: cfg.ColumnLimit,
				TabStop:     cfg.TabStop,
				Targets:     defaultTargets(),
			}).OwnedSpans(src)
		},
	})
}

func buildExpressionStageFormatter(stageName string, cfg BaseConfig, opts StageOptions, plan StagePlan, bundle DSLBundle) Formatter {
	if plan.Expressions != StageModeDSL {
		return NewLongExprFormatter(LongExprConfig{
			ColumnLimit:      cfg.ColumnLimit,
			TabStop:          cfg.TabStop,
			ParseSafe:        opts.Legacy.LongExprParseSafe,
			UseASTSelection:  opts.Legacy.LongExprUseASTSelect,
			ExcludeCallExprs: opts.Legacy.LongExprExcludeCallExprs,
		})
	}

	return NewDSLExprFormatter(DSLExprConfig{
		ColumnLimit:   cfg.ColumnLimit,
		TabStop:       cfg.TabStop,
		Rules:         bundle.Expressions.Rules,
		Trace:         opts.DSL.Trace,
		TraceReasons:  opts.DSL.TraceReasons,
		NodeOrder:     bundle.Expressions.NodeOrder,
		MaxIterations: bundle.Expressions.MaxIterations,
		SkipGofmt:     true,
		StageName:     stageName,
		Budget:        dslBudgetForRuleProfile(opts.Selection.RuleProfile),
	})
}

func buildMultiLineCallStageFormatter(stageName string, cfg BaseConfig, opts StageOptions, plan StagePlan, bundle DSLBundle) Formatter {
	if plan.MultiLineCalls != StageModeDSL {
		return NewMultiLineCallFormatter(MultiLineConfig{
			ColumnLimit:     cfg.ColumnLimit,
			TabStop:         cfg.TabStop,
			Excludes:        opts.Style.Excludes,
			UseASTSelection: opts.Legacy.MultiLineUseASTSelect,
			SkipGofmt:       true,
			ParseSafe:       opts.Legacy.MultiLineParseSafe,
		})
	}

	return NewDSLExprFormatter(DSLExprConfig{
		ColumnLimit:       cfg.ColumnLimit,
		TabStop:           cfg.TabStop,
		Rules:             bundle.MultiLineCalls.Rules,
		Trace:             opts.DSL.Trace,
		TraceReasons:      opts.DSL.TraceReasons,
		NodeOrder:         bundle.MultiLineCalls.NodeOrder,
		MaxIterations:     bundle.MultiLineCalls.MaxIterations,
		AutoMaxIterations: bundle.MultiLineCalls.AutoMaxIterations,
		DetectCycles:      bundle.MultiLineCalls.DetectCycles,
		SkipGofmt:         true,
		StageName:         stageName,
		Budget:            dslBudgetForRuleProfile(opts.Selection.RuleProfile),
		OwnedSpansFunc: func(src []byte) llast.OffsetSpanSet {
			// Use the legacy multiline stage's ownership selection (AST-based)
			// to avoid rewriting within calls that this stage will later format.
			return NewMultiLineCallFormatter(MultiLineConfig{
				ColumnLimit:     cfg.ColumnLimit,
				TabStop:         cfg.TabStop,
				Excludes:        opts.Style.Excludes,
				UseASTSelection: true,
			}).OwnedSpans(src)
		},
	})
}

func buildSignatureStageFormatter(stageName string, cfg BaseConfig, opts StageOptions, plan StagePlan, bundle DSLBundle) Formatter {
	if plan.Signatures != StageModeDSL {
		profile := normalizedRuleProfile(opts.Selection.RuleProfile)
		isNext := profile == "next"
		return NewFuncSigFormatter(FuncSigConfig{
			ColumnLimit: cfg.ColumnLimit,
			TabStop:     cfg.TabStop,
			// Keep legacy fixtures stable: next-specific behavior is enabled only
			// when the rule profile is explicitly "next".
			CanonicalMultilineSigLists:  isNext,
			ReserveTrailingComma:        isNext,
			PreferInlineSmallReturnList: isNext,
		})
	}

	return NewDSLExprFormatter(DSLExprConfig{
		ColumnLimit:       cfg.ColumnLimit,
		TabStop:           cfg.TabStop,
		Rules:             bundle.Signatures.Rules,
		Trace:             opts.DSL.Trace,
		TraceReasons:      opts.DSL.TraceReasons,
		NodeOrder:         bundle.Signatures.NodeOrder,
		MaxIterations:     bundle.Signatures.MaxIterations,
		AutoMaxIterations: bundle.Signatures.AutoMaxIterations,
		DetectCycles:      bundle.Signatures.DetectCycles,
		SkipGofmt:         true,
		StageName:         stageName,
		Budget:            dslBudgetForRuleProfile(opts.Selection.RuleProfile),
	})
}

func buildBlankLineStageFormatter(stageName string, cfg BaseConfig, opts StageOptions, plan StagePlan, bundle DSLBundle) Formatter {
	if plan.BlankLines != StageModeDSL {
		return NewBlankLineFormatter(BlankLineConfig{
			BeforeReturn:            true,
			BetweenCases:            true,
			BetweenInterfaceMethods: true,
		})
	}

	return NewDSLExprFormatter(DSLExprConfig{
		ColumnLimit:                 cfg.ColumnLimit,
		TabStop:                     cfg.TabStop,
		Rules:                       bundle.BlankLines.Rules,
		Trace:                       opts.DSL.Trace,
		TraceReasons:                opts.DSL.TraceReasons,
		NodeOrder:                   bundle.BlankLines.NodeOrder,
		MaxIterations:               bundle.BlankLines.MaxIterations,
		DisableLegacyBlankLinesShim: bundle.BlankLines.DisableLegacyBlankLinesShim,
		SkipGofmt:                   true,
		StageName:                   stageName,
		Budget:                      dslBudgetForRuleProfile(opts.Selection.RuleProfile),
	})
}
