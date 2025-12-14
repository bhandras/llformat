package formatter

import (
	formatstd "go/format"
)

// PipelineConfig holds configuration for the formatting pipeline.
type PipelineConfig struct {
	ColumnLimit     int
	TabStop         int
	MoveInlineAbove bool     // For comment formatter
	Excludes        []string // Functions to exclude from multiline formatting
	UseDSLExpr      bool     // Use DSL-based expression formatter
	TraceDSL        bool     // Enable DSL rule tracing (only when UseDSLExpr)
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

	baseCfg := NewBaseConfig(cfg.ColumnLimit, cfg.TabStop)
	stages := DefaultStagesWithOptions(baseCfg, StageOptions{
		CommentMoveInline: cfg.MoveInlineAbove,
		Excludes:          cfg.Excludes,
		UseDSLExpr:        cfg.UseDSLExpr,
		TraceDSL:          cfg.TraceDSL,
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
