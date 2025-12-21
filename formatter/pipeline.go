package formatter

import (
	"bytes"
	"crypto/sha256"
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
	UseDSLExpr             bool     // Use DSL-based expression formatter
	UseDSLFuncSigs         bool     // Use DSL-based signature formatter (delegates to legacy)
	UseDSLFuncSigsNative   bool     // Use native DSL signature rules (fallback to legacy)
	DSLSigsStyle           string   // DSL signature style: legacy|dsl (empty => legacy)
	UseDSLBlankLines       bool     // Use DSL-based blank line formatter
	UseDSLBlankLinesNative bool     // Use native DSL blank line rules (fallback to legacy)
	// DSLBlankLinesExtraIfErrReturn inserts a blank line before:
	//
	//   if err != nil { return ... }
	//
	// This is intentionally opt-in because it is opinionated and may interact
	// with users' desired grouping/spacing style.
	DSLBlankLinesExtraIfErrReturn bool
	// LogCallsMinTailLen controls the minimum tail length for string splits in
	// printf/logcall formatting under the "next" profile. When 0, a profile
	// default is used.
	LogCallsMinTailLen int
	TraceDSL           bool // Enable DSL rule tracing (only when UseDSLExpr)
	TraceDSLReasons    bool // Include "why fired/didn't fire" reasons in DSL tracing

	// UseOwnershipRegistry enables pipeline-level stage ownership boundaries.
	// When enabled, the pipeline will compute owned span sets for later stages
	// and provide them to earlier stages that support ownership-aware behavior.
	//
	// This remains opt-in to preserve golden fixtures.
	UseOwnershipRegistry bool

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

	// StagePlanOverride forces an explicit stage selection for the pipeline.
	// This is intended for controlled experiments and debugging.
	StagePlanOverride *StagePlan

	// MaxPipelineIterations controls how many full pipeline passes are allowed.
	// When > 0, the pipeline will run stages + gofmt repeatedly until the output
	// stabilizes (no changes) or a cycle is detected.
	//
	// When 0, NewPipeline runs a single pass. The CLI defaults to a small
	// fixpoint search (see `--fixpoint-iters`) because it tends to produce more
	// stable results on large files.
	MaxPipelineIterations int
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

	// Enable the full DSL pipeline by default when callers did not explicitly
	// configure any stage toggles.
	if !cfg.UseDSLComments &&
		!cfg.UseDSLLogCalls &&
		!cfg.UseDSLMultiLineCalls &&
		!cfg.UseDSLExpr &&
		!cfg.UseDSLFuncSigs &&
		!cfg.UseDSLBlankLines {
		cfg.UseDSLComments = true
		cfg.UseDSLLogCalls = true
		cfg.UseDSLMultiLineCalls = true
		cfg.UseDSLExpr = true
		cfg.UseDSLFuncSigs = true
		cfg.UseDSLBlankLines = true
	}

	// Next defaults.
	if cfg.UseDSLFuncSigs {
		cfg.UseDSLFuncSigsNative = true
		if cfg.DSLSigsStyle == "" {
			cfg.DSLSigsStyle = "legacy"
		}
	}
	if cfg.UseDSLBlankLines {
		cfg.UseDSLBlankLinesNative = true
	}
	if cfg.UseDSLExpr && !cfg.AllowDSLCallArgs && !cfg.AutoDSLCallArgs {
		cfg.AutoDSLCallArgs = true
	}
	if cfg.UseDSLMultiLineCalls && cfg.DSLMultiLineStyle == "" {
		cfg.DSLMultiLineStyle = "packed-chain-layout"
	}

	// Stage ownership: when multiline formatting is explicitly configured to
	// own layout of call arguments, prefer the DSL expression stage so the legacy
	// expression formatter does not interfere inside call args. The DSL
	// expression stage is call-args-safe by default (CallArgsPolicyOff).
	if cfg.UseDSLMultiLineCalls && !cfg.UseDSLExpr {
		switch cfg.DSLMultiLineStyle {
		case "layout-args", "layout-all", "layout-args-groups-pairs", "layout-all-groups-pairs":
			cfg.UseDSLExpr = true
		}
	}

	// Keep DSL stages from fighting: when the DSL multiline stage is configured
	// to own call-argument layout, do not also enable call-arg breaking in the
	// DSL expression stage. This avoids non-idempotent "expr breaks args, then
	// call stage repacks args" interactions.
	if cfg.UseDSLMultiLineCalls && !cfg.UseOwnershipRegistry {
		switch cfg.DSLMultiLineStyle {
		case "layout-args", "layout-all", "layout-args-groups-pairs", "layout-all-groups-pairs":
			cfg.AllowDSLCallArgs = false
		}
	}

	// Compute the stage plan only after all mode/policy bundles and pipeline
	// safety adjustments have been applied to cfg.
	stagePlan := stagePlanFromPipelineConfig(cfg)

	baseCfg := NewBaseConfig(cfg.ColumnLimit, cfg.TabStop)
	stages := DefaultStagesWithOptions(baseCfg, StageOptions{
		Selection: StageSelectionOptions{
			StagePlan: &stagePlan,
		},
		Style: StageStyleOptions{
			CommentMoveInline:             cfg.MoveInlineAbove,
			Excludes:                      cfg.Excludes,
			DSLMultiLineStyle:             cfg.DSLMultiLineStyle,
			DSLSigsStyle:                  cfg.DSLSigsStyle,
			DSLLogCallsMinTailLen:         cfg.LogCallsMinTailLen,
			DSLBlankLinesExtraIfErrReturn: cfg.DSLBlankLinesExtraIfErrReturn,
			DSLExprLogicalStyle:           cfg.DSLExprLogicalStyle,
			DSLExprArithmeticStyle:        cfg.DSLExprArithmeticStyle,
			DSLExprCaseClauseStyle:        cfg.DSLExprCaseClauseStyle,
			DSLExprSelectorChainStyle:     cfg.DSLExprSelectorChainStyle,
		},
		DSL: DSLStageOptions{
			Trace:               cfg.TraceDSL,
			TraceReasons:        cfg.TraceDSLReasons,
			UseFuncSigsNative:   cfg.UseDSLFuncSigsNative,
			UseBlankLinesNative: cfg.UseDSLBlankLinesNative,
			AllowCallArgs:       cfg.AllowDSLCallArgs,
			AutoCallArgs:        cfg.AutoDSLCallArgs,
		},
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

	// Next-only default: if no explicit stage toggles are provided, run all DSL
	// stages. This keeps PipelineConfig{} useful without relying on legacy
	// formatter stages.
	if !cfg.UseDSLComments &&
		!cfg.UseDSLLogCalls &&
		!cfg.UseDSLMultiLineCalls &&
		!cfg.UseDSLExpr &&
		!cfg.UseDSLFuncSigs &&
		!cfg.UseDSLBlankLines {
		return allDSLStagePlan()
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
	maxIters := p.cfg.MaxPipelineIterations
	if maxIters <= 0 {
		maxIters = 1
	}

	// Default behavior: a single pipeline pass + one final gofmt run.
	// This is useful for debugging or for callers that want a strictly bounded
	// pass count.
	if maxIters == 1 {
		out := src

		for _, stage := range p.stages {
			if stage.Formatter == nil {
				continue
			}
			if p.cfg.UseOwnershipRegistry {
				reg := BuildOwnershipRegistry(out, p.stages)
				if aware, ok := stage.Formatter.(OwnershipAware); ok {
					aware.SetOwnershipRegistry(reg)
				}
			} else if aware, ok := stage.Formatter.(OwnershipAware); ok {
				aware.SetOwnershipRegistry(nil)
			}
			out = stage.Formatter.FormatFile(out)
		}

		if formatted, err := formatstd.Source(out); err == nil {
			return formatted
		}
		return out
	}

	out := src
	seen := make(map[[32]byte]struct{}, maxIters+1)

	for iter := 0; iter < maxIters; iter++ {
		before := out

		// Execute stages in order.
		for _, stage := range p.stages {
			if stage.Formatter == nil {
				continue
			}
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

		// gofmt after each full pass so that multi-pass convergence matches
		// user behavior (running llformat multiple times uses a gofmt-normalized
		// file as the next input).
		if formatted, err := formatstd.Source(out); err == nil {
			out = formatted
		}

		if bytes.Equal(out, before) {
			break
		}

		sum := sha256.Sum256(out)
		if _, ok := seen[sum]; ok {
			// Cycle detected (e.g. two stages fight). Stop at the last produced
			// output to avoid an infinite loop. Subsequent runs will repeat the
			// same trajectory and land in the same stable stopping point.
			break
		}
		seen[sum] = struct{}{}
	}

	return out
}
