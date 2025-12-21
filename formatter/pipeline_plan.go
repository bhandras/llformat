package formatter

// PipelinePlan is a best-effort summary of resolved pipeline behavior. It is
// designed for debugging and UX (e.g. CLI `--print-plan`).
type PipelinePlan struct {
	StagePlan StagePlan

	// Key style knobs
	DSLMultiLineStyle string
	DSLSigsStyle      string

	// Native toggles
	UseDSLFuncSigsNative   bool
	UseDSLBlankLinesNative bool
	// Extra blank-line behavior (native blank lines only).
	DSLBlankLinesExtraIfErrReturn bool

	// Expression style knobs
	DSLExprLogicalStyle       string
	DSLExprArithmeticStyle    string
	DSLExprCaseClauseStyle    string
	DSLExprSelectorChainStyle string

	// Call-args policies
	AllowDSLCallArgs bool
	AutoDSLCallArgs  bool
}

// ResolvePipelinePlan returns the plan that would be used by NewPipeline after
// applying mode/policy bundles and safety adjustments.
func ResolvePipelinePlan(cfg PipelineConfig) PipelinePlan {
	p := NewPipeline(cfg)
	return p.Plan()
}

// Plan returns the resolved plan for this pipeline.
func (p *Pipeline) Plan() PipelinePlan {
	plan := stagePlanFromPipelineConfig(p.cfg)
	return PipelinePlan{
		StagePlan: plan,

		DSLMultiLineStyle: p.cfg.DSLMultiLineStyle,
		DSLSigsStyle:      p.cfg.DSLSigsStyle,

		UseDSLFuncSigsNative:          p.cfg.UseDSLFuncSigsNative,
		UseDSLBlankLinesNative:        p.cfg.UseDSLBlankLinesNative,
		DSLBlankLinesExtraIfErrReturn: p.cfg.DSLBlankLinesExtraIfErrReturn,

		DSLExprLogicalStyle:       p.cfg.DSLExprLogicalStyle,
		DSLExprArithmeticStyle:    p.cfg.DSLExprArithmeticStyle,
		DSLExprCaseClauseStyle:    p.cfg.DSLExprCaseClauseStyle,
		DSLExprSelectorChainStyle: p.cfg.DSLExprSelectorChainStyle,

		AllowDSLCallArgs: p.cfg.AllowDSLCallArgs,
		AutoDSLCallArgs:  p.cfg.AutoDSLCallArgs,
	}
}
