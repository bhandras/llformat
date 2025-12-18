package formatter

import (
	formatstd "go/format"
)

// PipelineConfig holds configuration for the formatting pipeline.
type PipelineConfig struct {
	ColumnLimit            int
	TabStop                int
	MoveInlineAbove        bool     // For comment formatter
	Excludes               []string // Functions to exclude from multiline formatting
	UseDSLComments         bool     // Use DSL-based comment formatting (delegates to legacy)
	UseDSLLogCalls         bool     // Use DSL-based log/printf call formatting
	UseDSLMultiLineCalls   bool     // Use DSL-based multiline call formatting
	DSLMultiLineStyle      string   // DSL multiline formatting style (empty => legacy)
	DSLCallPolicy          string   // DSL call policy bundle (empty => no override)
	UseDSLExpr             bool     // Use DSL-based expression formatter
	UseDSLFuncSigs         bool     // Use DSL-based signature formatter (delegates to legacy)
	UseDSLFuncSigsNative   bool     // Use native DSL signature rules (fallback to legacy)
	DSLSigsStyle           string   // DSL signature style: legacy|dsl (empty => legacy)
	UseDSLBlankLines       bool     // Use DSL-based blank line formatter
	UseDSLBlankLinesNative bool     // Use native DSL blank line rules (fallback to legacy)
	TraceDSL               bool     // Enable DSL rule tracing (only when UseDSLExpr)
	TraceDSLReasons        bool     // Include "why fired/didn't fire" reasons in DSL tracing

	// Mode provides a user-facing coarse selection of pipeline behavior.
	// It is intentionally opt-in; when empty, callers can control the pipeline
	// via the individual toggles below.
	//
	// Supported values:
	// - "legacy": legacy multi-stage pipeline (no DSL stages)
	// - "dsl-parity": DSL stages enabled, parity-oriented defaults
	// - "dsl-modern": DSL stages enabled, modern policy defaults
	Mode string

	// LegacyHardening enables a recommended bundle of internal hardening/migration
	// knobs for the legacy pipeline stages. This remains opt-in to preserve
	// golden fixtures, but provides a single toggle for users who want a more
	// robust, non-oscillating formatter pipeline.
	//
	// When enabled, this will force-enable the following knobs:
	// - AST-based selection for compact/multiline call stages
	// - AST-guided selection for legacy long-expr stage
	// - Parse-safe validation for compact/multiline/long-expr stages
	LegacyHardening bool

	// UseOwnershipRegistry enables pipeline-level stage ownership boundaries.
	// When enabled, the pipeline will compute owned span sets for later stages
	// and provide them to earlier stages that support ownership-aware behavior.
	//
	// This remains opt-in to preserve golden fixtures.
	UseOwnershipRegistry bool

	// MultiLineUseASTSelect enables AST-based call selection for the legacy
	// multiline call formatter. This is an internal migration knob and is
	// intentionally opt-in to preserve golden fixtures.
	MultiLineUseASTSelect bool

	// CompactCallUseASTSelect enables AST-based call selection for the legacy
	// compact call formatter stage. This is an internal migration knob and is
	// intentionally opt-in to preserve golden fixtures.
	CompactCallUseASTSelect bool

	// CompactCallParseSafe enables parse-safe behavior for the legacy compact
	// call formatter stage: it will return the original input unchanged if the
	// candidate output does not gofmt. This is an internal hardening knob and
	// is intentionally opt-in to preserve golden fixtures.
	CompactCallParseSafe bool

	// LongExprParseSafe enables parse-safe behavior in the legacy long
	// expression formatter: it will only accept a rewrite if gofmt succeeds on
	// the candidate output. This is an internal hardening knob and is
	// intentionally opt-in to preserve golden fixtures.
	LongExprParseSafe bool

	// LongExprUseASTSelect enables AST-guided selection for the legacy long
	// expression formatter stage. This is an internal migration knob and is
	// intentionally opt-in to preserve golden fixtures.
	LongExprUseASTSelect bool

	// MultiLineParseSafe enables parse-safe behavior for the legacy multiline
	// call formatter stage: it will return the original input unchanged if the
	// candidate output does not gofmt. This is an internal hardening knob and
	// is intentionally opt-in to preserve golden fixtures.
	MultiLineParseSafe bool

	// AllowDSLCallArgs enables limited expression formatting within call
	// arguments when using the DSL expression stage.
	AllowDSLCallArgs bool

	// AutoDSLCallArgs enables limited expression formatting within call arguments
	// only for calls that are known to be ignored by later call-formatting stages.
	// This is less invasive than AllowDSLCallArgs but may miss some cases.
	AutoDSLCallArgs bool

	// DSLExprLogicalStyle controls long &&/|| chain formatting inside the DSL
	// expression stage. Empty means legacy behavior.
	DSLExprLogicalStyle string

	// DSLExprArithmeticStyle controls long arithmetic chain formatting inside the
	// DSL expression stage. Empty means legacy behavior.
	DSLExprArithmeticStyle string

	// DSLExprCaseClauseStyle controls long `case A, B, ...:` list formatting
	// inside the DSL expression stage. Empty means legacy behavior.
	DSLExprCaseClauseStyle string

	// DSLExprSelectorChainStyle controls long selector chain formatting inside
	// the DSL expression stage. Empty means legacy behavior.
	DSLExprSelectorChainStyle string

	// RuleProfile is an internal taxonomy label that describes which behavioral
	// profile is in effect. This is intended as a bridge toward cohesive
	// rule-set based configuration (parity/modern/next) without changing golden
	// fixtures.
	//
	// Supported values:
	// - "" (unspecified): inferred from Mode / DSLCallPolicy
	// - "parity": golden-parity behavior (default)
	// - "modern": opt-in improvements (stable-ish)
	// - "next": aggressive opt-in experiments
	RuleProfile string

	// StagePlanOverride forces an explicit stage selection for the pipeline.
	// When set, it overrides Mode/DSLCallPolicy-derived StagePlan selection.
	// This is intended for controlled experiments and debugging.
	StagePlanOverride *StagePlan
}

// Pipeline orchestrates all formatters in sequence and runs gofmt once at the end.
type Pipeline struct {
	cfg    PipelineConfig
	stages []Stage
}

// NewPipeline creates a new formatting pipeline with the given config.
func NewPipeline(cfg PipelineConfig) *Pipeline {
	if cfg.ColumnLimit <= 0 {
		cfg.ColumnLimit = DefaultColumnLimit
	}
	if cfg.TabStop <= 0 {
		cfg.TabStop = DefaultTabStop
	}

	// Apply user-facing mode bundle first (if requested). This is intentionally
	// conservative and preserves the existing behavior when Mode is empty.
	switch cfg.Mode {
	case "":
		// no-op
	case "legacy":
		cfg.UseDSLComments = false
		cfg.UseDSLLogCalls = false
		cfg.UseDSLMultiLineCalls = false
		cfg.UseDSLExpr = false
		cfg.UseDSLFuncSigs = false
		cfg.UseDSLFuncSigsNative = false
		cfg.UseDSLBlankLines = false
		cfg.UseDSLBlankLinesNative = false
		cfg.DSLCallPolicy = ""
	case "dsl-parity":
		if cfg.RuleProfile == "" {
			cfg.RuleProfile = "parity"
		}
		cfg.UseDSLComments = true
		cfg.UseDSLLogCalls = true
		cfg.UseDSLMultiLineCalls = true
		cfg.UseDSLExpr = true
		cfg.UseDSLFuncSigs = true
		cfg.UseDSLBlankLines = true
		if cfg.DSLCallPolicy == "" {
			cfg.DSLCallPolicy = "legacy"
		}
	case "dsl-modern":
		if cfg.RuleProfile == "" {
			cfg.RuleProfile = "modern"
		}
		cfg.UseDSLComments = true
		cfg.UseDSLLogCalls = true
		cfg.UseDSLMultiLineCalls = true
		cfg.UseDSLExpr = true
		cfg.UseDSLFuncSigs = true
		cfg.UseDSLFuncSigsNative = true
		cfg.UseDSLBlankLines = true
		cfg.UseDSLBlankLinesNative = true
		cfg.DSLCallPolicy = "modern"
	case "next":
		if cfg.RuleProfile == "" {
			cfg.RuleProfile = "next"
		}
		// "next" is a convenience alias for the most aggressive DSL-first
		// pipeline configuration. It is intentionally opt-in so it can evolve
		// without breaking golden fixtures.
		cfg.UseDSLComments = true
		cfg.UseDSLLogCalls = true
		cfg.UseDSLMultiLineCalls = true
		cfg.UseDSLExpr = true
		cfg.UseDSLFuncSigs = true
		cfg.UseDSLFuncSigsNative = true
		cfg.UseDSLBlankLines = true
		cfg.UseDSLBlankLinesNative = true

		// Enable the modern policy defaults, then override to the most
		// layout-driven multi-line style.
		cfg.DSLCallPolicy = "modern"
		if cfg.DSLMultiLineStyle == "" || cfg.DSLMultiLineStyle == "legacy" {
			cfg.DSLMultiLineStyle = "layout-all"
		}
	default:
		// Unknown mode: ignore (callers can still set individual toggles).
	}

	// Apply legacy hardening preset (if requested) before policy bundles.
	if cfg.LegacyHardening {
		cfg.MultiLineUseASTSelect = true
		cfg.CompactCallUseASTSelect = true
		cfg.LongExprUseASTSelect = true

		cfg.CompactCallParseSafe = true
		cfg.MultiLineParseSafe = true
		cfg.LongExprParseSafe = true

		cfg.UseOwnershipRegistry = true
	}

	// Apply a policy bundle (if requested). This gives callers a single knob for
	// a coherent set of call-related DSL behaviors, while keeping legacy-parity
	// as the default and preserving golden fixtures.
	switch cfg.DSLCallPolicy {
	case "", "legacy":
		// No override.
	case "modern":
		if cfg.RuleProfile == "" {
			cfg.RuleProfile = "modern"
		}
		cfg.UseDSLComments = true
		cfg.UseDSLLogCalls = true
		cfg.UseDSLMultiLineCalls = true
		if cfg.DSLMultiLineStyle == "" || cfg.DSLMultiLineStyle == "legacy" || cfg.DSLMultiLineStyle == "packed-chain" {
			cfg.DSLMultiLineStyle = "packed-chain-layout"
		}
		cfg.UseDSLExpr = true
		if cfg.DSLExprLogicalStyle == "" {
			cfg.DSLExprLogicalStyle = "layout"
		}
		if cfg.DSLExprArithmeticStyle == "" {
			cfg.DSLExprArithmeticStyle = "layout"
		}
		if cfg.DSLExprCaseClauseStyle == "" {
			cfg.DSLExprCaseClauseStyle = "layout"
		}
		if cfg.DSLExprSelectorChainStyle == "" {
			cfg.DSLExprSelectorChainStyle = "layout"
		}
		cfg.AutoDSLCallArgs = true
		cfg.UseDSLFuncSigs = true
		cfg.UseDSLFuncSigsNative = true
		if cfg.DSLSigsStyle == "" {
			cfg.DSLSigsStyle = "legacy"
		}
		cfg.UseDSLBlankLines = true
	default:
		// Unknown policy: ignore (callers can still set individual toggles).
	}

	if cfg.RuleProfile == "" {
		cfg.RuleProfile = "parity"
	}

	// Stage ownership: when multiline formatting is explicitly configured to
	// own layout of call arguments, prefer the DSL expression stage so the legacy
	// expression formatter does not interfere inside call args. The DSL
	// expression stage is call-args-safe by default (CallArgsPolicyOff).
	if cfg.UseDSLMultiLineCalls && !cfg.UseDSLExpr {
		switch cfg.DSLMultiLineStyle {
		case "layout-args", "layout-all":
			cfg.UseDSLExpr = true
		}
	}

	// Keep DSL stages from fighting: when the DSL multiline stage is configured
	// to own call-argument layout, do not also enable call-arg breaking in the
	// DSL expression stage. This avoids non-idempotent "expr breaks args, then
	// call stage repacks args" interactions.
	if cfg.UseDSLMultiLineCalls {
		switch cfg.DSLMultiLineStyle {
		case "layout-args", "layout-all":
			cfg.AllowDSLCallArgs = false
		}
	}

	// Compute the stage plan only after all mode/policy bundles and pipeline
	// safety adjustments have been applied to cfg.
	stagePlan := stagePlanFromPipelineConfig(cfg)

	baseCfg := NewBaseConfig(cfg.ColumnLimit, cfg.TabStop)
	stages := DefaultStagesWithOptions(baseCfg, StageOptions{
		CommentMoveInline:         cfg.MoveInlineAbove,
		Excludes:                  cfg.Excludes,
		RuleProfile:               cfg.RuleProfile,
		StagePlan:                 &stagePlan,
		DSLMultiLineStyle:         cfg.DSLMultiLineStyle,
		UseDSLFuncSigsNative:      cfg.UseDSLFuncSigsNative,
		DSLSigsStyle:              cfg.DSLSigsStyle,
		UseDSLBlankLinesNative:    cfg.UseDSLBlankLinesNative,
		TraceDSL:                  cfg.TraceDSL,
		TraceDSLReasons:           cfg.TraceDSLReasons,
		MultiLineUseASTSelect:     cfg.MultiLineUseASTSelect,
		CompactCallUseASTSelect:   cfg.CompactCallUseASTSelect,
		CompactCallParseSafe:      cfg.CompactCallParseSafe,
		LongExprParseSafe:         cfg.LongExprParseSafe,
		LongExprUseASTSelect:      cfg.LongExprUseASTSelect,
		MultiLineParseSafe:        cfg.MultiLineParseSafe,
		AllowDSLCallArgs:          cfg.AllowDSLCallArgs,
		AutoDSLCallArgs:           cfg.AutoDSLCallArgs,
		DSLExprLogicalStyle:       cfg.DSLExprLogicalStyle,
		DSLExprArithmeticStyle:    cfg.DSLExprArithmeticStyle,
		DSLExprCaseClauseStyle:    cfg.DSLExprCaseClauseStyle,
		DSLExprSelectorChainStyle: cfg.DSLExprSelectorChainStyle,
	})

	return &Pipeline{
		cfg:    cfg,
		stages: stages,
	}
}

func stagePlanFromPipelineConfig(cfg PipelineConfig) StagePlan {
	if cfg.StagePlanOverride != nil {
		return *cfg.StagePlanOverride
	}

	// Prefer the user-facing mode/policy selection as the source of truth for
	// stage enablement. If neither are specified, fall back to explicit per-stage
	// toggles for backward compatibility.
	switch cfg.Mode {
	case "legacy":
		return StagePlan{
			Comments:       StageModeLegacy,
			LogCalls:       StageModeLegacy,
			Expressions:    StageModeLegacy,
			MultiLineCalls: StageModeLegacy,
			Signatures:     StageModeLegacy,
			BlankLines:     StageModeLegacy,
		}
	case "dsl-parity", "dsl-modern", "next":
		if plan, ok := stagePlanForRuleProfile(cfg.RuleProfile); ok {
			return plan
		}
		return allDSLStagePlan()
	}

	switch cfg.DSLCallPolicy {
	case "modern":
		if plan, ok := stagePlanForRuleProfile(cfg.RuleProfile); ok {
			return plan
		}
		return allDSLStagePlan()
	}

	// If a non-parity RuleProfile is set but no other stage-selection knobs are
	// present, treat the profile as the stage selector. This keeps the
	// out-of-the-box behavior legacy-parity while allowing internal callers to
	// opt into cohesive DSL bundles without also setting Mode/DSLCallPolicy.
	if cfg.Mode == "" &&
		cfg.DSLCallPolicy == "" &&
		normalizedRuleProfile(cfg.RuleProfile) != "parity" &&
		!cfg.UseDSLComments &&
		!cfg.UseDSLLogCalls &&
		!cfg.UseDSLMultiLineCalls &&
		!cfg.UseDSLExpr &&
		!cfg.UseDSLFuncSigs &&
		!cfg.UseDSLBlankLines {
		if plan, ok := stagePlanForRuleProfile(cfg.RuleProfile); ok {
			return plan
		}
	}

	return StagePlan{
		Comments:       stageModeFromBool(cfg.UseDSLComments),
		LogCalls:       stageModeFromBool(cfg.UseDSLLogCalls),
		Expressions:    stageModeFromBool(cfg.UseDSLExpr),
		MultiLineCalls: stageModeFromBool(cfg.UseDSLMultiLineCalls),
		Signatures:     stageModeFromBool(cfg.UseDSLFuncSigs),
		BlankLines:     stageModeFromBool(cfg.UseDSLBlankLines),
	}
}

// NewPipelineWithStages creates a pipeline with custom stages.
func NewPipelineWithStages(cfg PipelineConfig, stages []Stage) *Pipeline {
	if cfg.ColumnLimit <= 0 {
		cfg.ColumnLimit = DefaultColumnLimit
	}
	if cfg.TabStop <= 0 {
		cfg.TabStop = DefaultTabStop
	}
	return &Pipeline{
		cfg:    cfg,
		stages: stages,
	}
}

// Stages returns the pipeline's stages for inspection.
func (p *Pipeline) Stages() []Stage {
	return p.stages
}

// Config returns the pipeline's base configuration.
func (p *Pipeline) Config() BaseConfig {
	return NewBaseConfig(p.cfg.ColumnLimit, p.cfg.TabStop)
}

// Format applies all formatters in sequence and runs gofmt at the end.
func (p *Pipeline) Format(src []byte) []byte {
	out := src

	// Execute stages in order
	for _, stage := range p.stages {
		if stage.Formatter != nil {
			if p.cfg.UseOwnershipRegistry {
				// Ownership is computed over the current snapshot and includes
				// all stages that declare ownership. This prevents non-call
				// stages from rewriting inside regions that call formatting
				// stages may later reformat on subsequent runs (idempotence).
				reg := BuildOwnershipRegistry(out, p.stages)
				if aware, ok := stage.Formatter.(OwnershipAware); ok {
					aware.SetOwnershipRegistry(reg)
				}
			} else if aware, ok := stage.Formatter.(OwnershipAware); ok {
				// Avoid leaking a previous registry across pipeline uses.
				aware.SetOwnershipRegistry(nil)
			}
			out = stage.Formatter.FormatFile(out)
		}
	}

	// Final stage: Run gofmt once
	if formatted, err := formatstd.Source(out); err == nil {
		return formatted
	}
	return out
}
