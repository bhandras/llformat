package formatter

import (
	"github.com/lightninglabs/llformat/dsl"
)

// DSLExprFormatter uses the DSL engine to format expressions.
type DSLExprFormatter struct {
	engine *dsl.Engine
}

// DSLExprConfig holds configuration for the DSL expression formatter.
type DSLExprConfig struct {
	ColumnLimit int
	TabStop     int
	Rules       []dsl.Rule // Custom rules (if nil, uses defaults)
}

// NewDSLExprFormatter creates a new DSL-based expression formatter.
func NewDSLExprFormatter(cfg DSLExprConfig) *DSLExprFormatter {
	rules := cfg.Rules
	if rules == nil {
		rules = dsl.DefaultRules()
	}

	engine := dsl.NewEngine(rules)
	if cfg.ColumnLimit > 0 {
		engine.ColumnLimit = cfg.ColumnLimit
	}
	if cfg.TabStop > 0 {
		engine.TabStop = cfg.TabStop
	}

	return &DSLExprFormatter{engine: engine}
}

// FormatFile formats the source file using DSL rules.
func (f *DSLExprFormatter) FormatFile(src []byte) []byte {
	return f.engine.FormatFile(src)
}
