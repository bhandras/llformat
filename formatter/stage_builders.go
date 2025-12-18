package formatter

func buildCommentStageFormatter(cfg BaseConfig, opts StageOptions, plan StagePlan, bundle DSLBundle) Formatter {
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
	})
}

func buildCompactCallStageFormatter(cfg BaseConfig, opts StageOptions, plan StagePlan, bundle DSLBundle) Formatter {
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
	})
}

func buildExpressionStageFormatter(cfg BaseConfig, opts StageOptions, plan StagePlan, bundle DSLBundle) Formatter {
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
	})
}

func buildMultiLineCallStageFormatter(cfg BaseConfig, opts StageOptions, plan StagePlan, bundle DSLBundle) Formatter {
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
	})
}

func buildSignatureStageFormatter(cfg BaseConfig, opts StageOptions, plan StagePlan, bundle DSLBundle) Formatter {
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
	})
}

func buildBlankLineStageFormatter(cfg BaseConfig, opts StageOptions, plan StagePlan, bundle DSLBundle) Formatter {
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
	})
}
