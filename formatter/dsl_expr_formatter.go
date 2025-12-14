package formatter

import (
	"github.com/lightninglabs/llformat/dsl"
)

// DSLExprFormatter uses the DSL engine to format expressions.
type DSLExprFormatter struct {
	engine               *dsl.Engine
	applyLegacyBlankLines bool
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

	applyLegacyBlankLines := hasDSLBlankLineRules(rules)
	if applyLegacyBlankLines {
		rules = filterDSLBlankLineRules(rules)
	}

	engine := dsl.NewEngine(rules)
	if cfg.ColumnLimit > 0 {
		engine.ColumnLimit = cfg.ColumnLimit
	}
	if cfg.TabStop > 0 {
		engine.TabStop = cfg.TabStop
	}

	return &DSLExprFormatter{
		engine:               engine,
		applyLegacyBlankLines: applyLegacyBlankLines,
	}
}

// FormatFile formats the source file using DSL rules.
func (f *DSLExprFormatter) FormatFile(src []byte) []byte {
	out := f.engine.FormatFile(src)

	if !f.applyLegacyBlankLines {
		return out
	}

	// Preserve existing llformat behavior for blank line insertion by running
	// the legacy blank-line formatter after DSL transformations.
	return NewBlankLineFormatter(BlankLineConfig{
		BeforeReturn:            true,
		BetweenCases:            true,
		BetweenInterfaceMethods: true,
	}).FormatFile(out)
}

func hasDSLBlankLineRules(rules []dsl.Rule) bool {
	for _, r := range rules {
		switch r.Name {
		case "blank_before_case", "blank_before_return", "blank_between_interface_methods":
			return true
		}
	}
	return false
}

func filterDSLBlankLineRules(rules []dsl.Rule) []dsl.Rule {
	filtered := make([]dsl.Rule, 0, len(rules))
	for _, r := range rules {
		switch r.Name {
		case "blank_before_case", "blank_before_return", "blank_between_interface_methods":
			continue
		default:
			filtered = append(filtered, r)
		}
	}
	return filtered
}
