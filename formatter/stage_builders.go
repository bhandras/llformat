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
			MoveInlineAbove: opts.CommentMoveInline,
		})
	}

	return NewDSLExprFormatter(DSLExprConfig{
		ColumnLimit:   cfg.ColumnLimit,
		TabStop:       cfg.TabStop,
		Rules:         bundle.Comments.Rules,
		Trace:         opts.TraceDSL,
		TraceReasons:  opts.TraceDSLReasons,
		NodeOrder:     bundle.Comments.NodeOrder,
		MaxIterations: bundle.Comments.MaxIterations,
		SkipGofmt:     true,
		StageName:     stageName,
		Budget:        dslBudgetForRuleProfile(opts.RuleProfile),
	})
}

func buildCompactCallStageFormatter(stageName string, cfg BaseConfig, opts StageOptions, plan StagePlan, bundle DSLBundle) Formatter {
	if plan.LogCalls != StageModeDSL {
		return NewCompactCallFormatter(Config{
			ColumnLimit:     cfg.ColumnLimit,
			TabStop:         cfg.TabStop,
			UseASTSelection: opts.CompactCallUseASTSelect,
			SkipGofmt:       true,
			ParseSafe:       opts.CompactCallParseSafe,
		})
	}

	return NewDSLExprFormatter(DSLExprConfig{
		ColumnLimit:   cfg.ColumnLimit,
		TabStop:       cfg.TabStop,
		Rules:         bundle.LogCalls.Rules,
		Trace:         opts.TraceDSL,
		TraceReasons:  opts.TraceDSLReasons,
		NodeOrder:     bundle.LogCalls.NodeOrder,
		MaxIterations: bundle.LogCalls.MaxIterations,
		SkipGofmt:     true,
		StageName:     stageName,
		Budget:        dslBudgetForRuleProfile(opts.RuleProfile),
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
			ParseSafe:        opts.LongExprParseSafe,
			UseASTSelection:  opts.LongExprUseASTSelect,
			ExcludeCallExprs: opts.LongExprExcludeCallExprs,
		})
	}

	return NewDSLExprFormatter(DSLExprConfig{
		ColumnLimit:   cfg.ColumnLimit,
		TabStop:       cfg.TabStop,
		Rules:         bundle.Expressions.Rules,
		Trace:         opts.TraceDSL,
		TraceReasons:  opts.TraceDSLReasons,
		NodeOrder:     bundle.Expressions.NodeOrder,
		MaxIterations: bundle.Expressions.MaxIterations,
		SkipGofmt:     true,
		StageName:     stageName,
		Budget:        dslBudgetForRuleProfile(opts.RuleProfile),
	})
}

func buildMultiLineCallStageFormatter(stageName string, cfg BaseConfig, opts StageOptions, plan StagePlan, bundle DSLBundle) Formatter {
	if plan.MultiLineCalls != StageModeDSL {
		return NewMultiLineCallFormatter(MultiLineConfig{
			ColumnLimit:     cfg.ColumnLimit,
			TabStop:         cfg.TabStop,
			Excludes:        opts.Excludes,
			UseASTSelection: opts.MultiLineUseASTSelect,
			SkipGofmt:       true,
			ParseSafe:       opts.MultiLineParseSafe,
		})
	}

	return NewDSLExprFormatter(DSLExprConfig{
		ColumnLimit:   cfg.ColumnLimit,
		TabStop:       cfg.TabStop,
		Rules:         bundle.MultiLineCalls.Rules,
		Trace:         opts.TraceDSL,
		TraceReasons:  opts.TraceDSLReasons,
		NodeOrder:     bundle.MultiLineCalls.NodeOrder,
		MaxIterations: bundle.MultiLineCalls.MaxIterations,
		SkipGofmt:     true,
		StageName:     stageName,
		Budget:        dslBudgetForRuleProfile(opts.RuleProfile),
		OwnedSpansFunc: func(src []byte) llast.OffsetSpanSet {
			// Use the legacy multiline stage's ownership selection (AST-based)
			// to avoid rewriting within calls that this stage will later format.
			return NewMultiLineCallFormatter(MultiLineConfig{
				ColumnLimit:     cfg.ColumnLimit,
				TabStop:         cfg.TabStop,
				Excludes:        opts.Excludes,
				UseASTSelection: true,
			}).OwnedSpans(src)
		},
	})
}

func buildSignatureStageFormatter(stageName string, cfg BaseConfig, opts StageOptions, plan StagePlan, bundle DSLBundle) Formatter {
	if plan.Signatures != StageModeDSL {
		return NewFuncSigFormatter(FuncSigConfig{
			ColumnLimit: cfg.ColumnLimit,
			TabStop:     cfg.TabStop,
		})
	}

	return NewDSLExprFormatter(DSLExprConfig{
		ColumnLimit:   cfg.ColumnLimit,
		TabStop:       cfg.TabStop,
		Rules:         bundle.Signatures.Rules,
		Trace:         opts.TraceDSL,
		TraceReasons:  opts.TraceDSLReasons,
		NodeOrder:     bundle.Signatures.NodeOrder,
		MaxIterations: bundle.Signatures.MaxIterations,
		SkipGofmt:     true,
		StageName:     stageName,
		Budget:        dslBudgetForRuleProfile(opts.RuleProfile),
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
		Trace:                       opts.TraceDSL,
		TraceReasons:                opts.TraceDSLReasons,
		NodeOrder:                   bundle.BlankLines.NodeOrder,
		MaxIterations:               bundle.BlankLines.MaxIterations,
		DisableLegacyBlankLinesShim: bundle.BlankLines.DisableLegacyBlankLinesShim,
		SkipGofmt:                   true,
		StageName:                   stageName,
		Budget:                      dslBudgetForRuleProfile(opts.RuleProfile),
	})
}
