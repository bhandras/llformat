package formatter

import (
	"strings"

	"github.com/bhandras/llformat/scanner"
	"github.com/bhandras/llformat/text"
)

// Rule defines a formatting rule that can be applied to source.
type Rule interface {
	// Name returns the rule's identifier for debugging/logging.
	Name() string

	// Match returns true if this rule applies at the given position.
	Match(src []byte, pos int) bool

	// Apply formats the matched content. Returns the replacement bytes and
	// the number of source bytes consumed.
	Apply(src []byte, pos int,
		cfg BaseConfig) (replacement []byte, consumed int)
}

// CallRule formats function calls matching specific patterns.
type CallRule struct {
	// Patterns are the function call prefixes to match (e.g., "log.Infof(",
	// "fmt.Errorf(")
	Patterns []string

	// Breaker is the strategy for breaking arguments.
	Breaker Breaker

	// Splitter defines how to split the argument body into elements. If
	// nil, DefaultCommaSplitter is used.
	Splitter ElementSplitter

	// Priority determines application order (higher = applied first).
	Priority int
}

// Name implements Rule.
func (r *CallRule) Name() string {
	if len(r.Patterns) > 0 {
		return "call:" + r.Patterns[0]
	}

	return "call:unknown"
}

// Match implements Rule.
func (r *CallRule) Match(src []byte, pos int) bool {
	for _, pattern := range r.Patterns {
		if matchPrefixAt(src, pos, pattern) {
			return true
		}
	}

	return false
}

// Apply implements Rule.
func (r *CallRule) Apply(src []byte, pos int, cfg BaseConfig) ([]byte, int) {
	// Find which pattern matched
	var matched string
	for _, pattern := range r.Patterns {
		if matchPrefixAt(src, pos, pattern) {
			matched = pattern
			break
		}
	}
	if matched == "" {
		return nil, 0
	}

	// Find the opening paren
	openIdx := pos + len(matched) - 1
	if openIdx >= len(src) || src[openIdx] != '(' {
		return nil, 0
	}

	// Find the matching close paren
	closeIdx := scanner.ScanBalancedParen(src, openIdx)
	if closeIdx < 0 {
		return nil, 0
	}

	// Extract parts
	funcName := string(src[pos:openIdx])
	argsBody := string(src[openIdx+1 : closeIdx])

	// Get indentation context
	lineStart := text.LastLineStart(src, pos)
	indent := IndentFromLine(src, lineStart, cfg.TabStop)
	prefixLen := cfg.WidthFrom(0, string(src[lineStart:pos]))

	// Split arguments
	splitter := r.Splitter
	if splitter == nil {
		splitter = DefaultCommaSplitter()
	}
	elements := splitter.Split(argsBody)

	// Build break context
	ctx := BreakContext{
		Elements:   elements,
		Indent:     indent,
		CurrentCol: prefixLen + cfg.Width(funcName),
		Config:     cfg,
	}

	// Apply breaker
	result := r.Breaker.Break(ctx)

	// Combine result
	var sb strings.Builder
	sb.WriteString(funcName)
	sb.WriteString(result.Content)

	return []byte(sb.String()), closeIdx + 1 - pos
}

// OperatorRule defines how to break at a specific operator.
type OperatorRule struct {
	Op       string // The operator (e.g., "&&", "||", ",")
	Priority int    // Lower = break first (prefer breaking here)
	Context  string // Where this applies (e.g., "expr", "case", "call")
}

// DefaultOperatorRules defines standard Go formatting preferences.
var DefaultOperatorRules = []OperatorRule{
	{Op: ",", Priority: 0, Context: "case"},  // Break case lists first
	{Op: "||", Priority: 1, Context: "expr"}, // Then logical OR
	{Op: "&&", Priority: 2, Context: "expr"}, // Then logical AND
	{Op: "==", Priority: 3, Context: "expr"}, // Then comparisons
	{Op: "!=", Priority: 3, Context: "expr"},
	{Op: "+", Priority: 4, Context: "expr"}, // Then arithmetic
	{Op: "-", Priority: 4, Context: "expr"},
}

// DefaultLogPatterns returns the standard log function patterns.
func DefaultLogPatterns() []string {
	return []string{
		"log.Infof(", "log.Debugf(", "log.Tracef(",
		"log.Errorf(", "log.Warnf(",
	}
}

// DefaultFmtPatterns returns the standard fmt function patterns.
func DefaultFmtPatterns() []string {
	return []string{
		"fmt.Printf(", "fmt.Sprintf(", "fmt.Errorf(",
	}
}

// DefaultCallPatterns returns all default patterns.
func DefaultCallPatterns() []string {
	return append(DefaultLogPatterns(), DefaultFmtPatterns()...)
}

// matchPrefixAt checks if src[pos:] starts with the given prefix.
func matchPrefixAt(src []byte, pos int, prefix string) bool {
	if pos+len(prefix) > len(src) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if src[pos+i] != prefix[i] {
			return false
		}
	}

	return true
}

// RuleMatcher applies multiple rules to source code.
type RuleMatcher struct {
	Rules  []Rule
	Config BaseConfig
}

// NewRuleMatcher creates a RuleMatcher with the given rules.
func NewRuleMatcher(rules []Rule, cfg BaseConfig) *RuleMatcher {

	// Sort rules by priority if needed (higher priority first)
	return &RuleMatcher{
		Rules:  rules,
		Config: cfg,
	}
}

// MatchAt returns the first matching rule at the given position, or nil.
func (m *RuleMatcher) MatchAt(src []byte, pos int) Rule {
	for _, rule := range m.Rules {
		if rule.Match(src, pos) {
			return rule
		}
	}

	return nil
}

// ApplyAt applies the first matching rule at position. Returns nil, 0 if no
// rule matches.
func (m *RuleMatcher) ApplyAt(src []byte, pos int) ([]byte, int) {
	rule := m.MatchAt(src, pos)
	if rule == nil {
		return nil, 0
	}

	return rule.Apply(src, pos, m.Config)
}
