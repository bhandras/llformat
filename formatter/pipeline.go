package formatter

import (
	formatstd "go/format"
)

// PipelineConfig holds configuration for the formatting pipeline.
type PipelineConfig struct {
	ColumnLimit          int
	TabStop              int
	MoveInlineAbove      bool     // For comment formatter
	Excludes             []string // Functions to exclude from multiline formatting
	UseDSLLogCalls       bool     // Use DSL-based log/printf call formatting
	UseDSLMultiLineCalls bool     // Use DSL-based multiline call formatting
	DSLMultiLineStyle    string   // DSL multiline formatting style (empty => legacy)
	DSLCallPolicy        string   // DSL call policy bundle (empty => no override)
	UseDSLExpr           bool     // Use DSL-based expression formatter
	TraceDSL             bool     // Enable DSL rule tracing (only when UseDSLExpr)

	// AllowDSLCallArgs enables limited expression formatting within call
	// arguments when using the DSL expression stage.
	AllowDSLCallArgs bool

	// AutoDSLCallArgs enables limited expression formatting within call arguments
	// only for calls that are known to be ignored by later call-formatting stages.
	// This is less invasive than AllowDSLCallArgs but may miss some cases.
	AutoDSLCallArgs bool
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

	// Apply a policy bundle (if requested). This gives callers a single knob for
	// a coherent set of call-related DSL behaviors, while keeping legacy-parity
	// as the default and preserving golden fixtures.
	switch cfg.DSLCallPolicy {
	case "", "legacy":
		// No override.
	case "modern":
		cfg.UseDSLLogCalls = true
		cfg.UseDSLMultiLineCalls = true
		if cfg.DSLMultiLineStyle == "" || cfg.DSLMultiLineStyle == "legacy" {
			cfg.DSLMultiLineStyle = "packed-chain"
		}
		cfg.UseDSLExpr = true
	default:
		// Unknown policy: ignore (callers can still set individual toggles).
	}

	baseCfg := NewBaseConfig(cfg.ColumnLimit, cfg.TabStop)
	stages := DefaultStagesWithOptions(baseCfg, StageOptions{
		CommentMoveInline:    cfg.MoveInlineAbove,
		Excludes:             cfg.Excludes,
		UseDSLLogCalls:       cfg.UseDSLLogCalls,
		UseDSLMultiLineCalls: cfg.UseDSLMultiLineCalls,
		DSLMultiLineStyle:    cfg.DSLMultiLineStyle,
		UseDSLExpr:           cfg.UseDSLExpr,
		TraceDSL:             cfg.TraceDSL,
		AllowDSLCallArgs:     cfg.AllowDSLCallArgs,
		AutoDSLCallArgs:      cfg.AutoDSLCallArgs,
	})

	return &Pipeline{
		cfg:    cfg,
		stages: stages,
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
			out = stage.Formatter.FormatFile(out)
		}
	}

	// Final stage: Run gofmt once
	if formatted, err := formatstd.Source(out); err == nil {
		return formatted
	}
	return out
}
