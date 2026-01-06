package formatter

import (
	"testing"
)

func TestCallRuleMatch(t *testing.T) {
	rule := &CallRule{
		Patterns: []string{
			"log.Infof(",
			"log.Errorf(",
		},
		Breaker: NewLeftFlowBreaker(),
	}

	tests := []struct {
		src  string
		pos  int
		want bool
	}{
		{
			"log.Infof(a)",
			0,
			true,
		},
		{
			"log.Errorf(b)",
			0,
			true,
		},
		{
			"log.Debugf(c)",
			0,
			false,
		},
		{
			"x = log.Infof(a)",
			4,
			true,
		},
		{
			"fmt.Printf(a)",
			0,
			false,
		},
	}

	for _, tt := range tests {
		got := rule.Match([]byte(tt.src), tt.pos)
		if got != tt.want {
			t.Errorf("Match(%q, %d) = %v, want %v", tt.src, tt.pos,
				got, tt.want)
		}
	}
}

func TestCallRuleName(t *testing.T) {
	rule := &CallRule{
		Patterns: []string{
			"log.Infof(",
		},
		Breaker: NewLeftFlowBreaker(),
	}
	if rule.Name() != "call:log.Infof(" {
		t.Errorf("Name() = %q, want %q", rule.Name(), "call:log.Infof(")
	}

	emptyRule := &CallRule{Breaker: NewLeftFlowBreaker()}
	if emptyRule.Name() != "call:unknown" {
		t.Errorf("Name() = %q, want %q", emptyRule.Name(),
			"call:unknown")
	}
}

func TestCallRuleApply(t *testing.T) {
	rule := &CallRule{
		Patterns: []string{
			"log.Infof(",
		},
		Breaker: NewLeftFlowBreaker(),
	}
	cfg := NewBaseConfig(80, 8)

	// Simple case that fits on one line
	src := []byte("log.Infof(a, b)")
	replacement, consumed := rule.Apply(src, 0, cfg)
	if consumed != 15 {
		t.Errorf("consumed = %d, want 15", consumed)
	}
	if string(replacement) != "log.Infof(a, b)" {
		t.Errorf("replacement = %q, want %q", replacement,
			"log.Infof(a, b)")
	}
}

func TestCallRuleApplyNoMatch(t *testing.T) {
	rule := &CallRule{
		Patterns: []string{
			"log.Infof(",
		},
		Breaker: NewLeftFlowBreaker(),
	}
	cfg := NewBaseConfig(80, 8)

	src := []byte("log.Debugf(a)")
	replacement, consumed := rule.Apply(src, 0, cfg)
	if consumed != 0 {
		t.Errorf("consumed = %d, want 0 for non-match", consumed)
	}
	if replacement != nil {
		t.Errorf("replacement = %v, want nil for non-match",
			replacement)
	}
}

func TestMatchPrefixAt(t *testing.T) {
	tests := []struct {
		src    string
		pos    int
		prefix string
		want   bool
	}{
		{
			"hello world",
			0,
			"hello",
			true,
		},
		{
			"hello world",
			6,
			"world",
			true,
		},
		{
			"hello",
			0,
			"hello world",
			false,
		},
		{
			"hello",
			3,
			"lo",
			true,
		},
		{
			"",
			0,
			"x",
			false,
		},
	}

	for _, tt := range tests {
		got := matchPrefixAt([]byte(tt.src), tt.pos, tt.prefix)
		if got != tt.want {
			t.Errorf("matchPrefixAt(%q, %d, %q) = %v, want %v",
				tt.src, tt.pos, tt.prefix, got, tt.want)
		}
	}
}

func TestDefaultPatterns(t *testing.T) {
	logPatterns := DefaultLogPatterns()
	if len(logPatterns) != 5 {
		t.Errorf("len(DefaultLogPatterns()) = %d, want 5",
			len(logPatterns))
	}

	fmtPatterns := DefaultFmtPatterns()
	if len(fmtPatterns) != 3 {
		t.Errorf("len(DefaultFmtPatterns()) = %d, want 3",
			len(fmtPatterns))
	}

	allPatterns := DefaultCallPatterns()
	if len(allPatterns) != 8 {
		t.Errorf("len(DefaultCallPatterns()) = %d, want 8",
			len(allPatterns))
	}
}

func TestRuleMatcherMatchAt(t *testing.T) {
	rule1 := &CallRule{
		Patterns: []string{
			"log.Infof(",
		},
		Breaker: NewLeftFlowBreaker(),
	}
	rule2 := &CallRule{
		Patterns: []string{
			"fmt.Printf(",
		},
		Breaker: NewLeftFlowBreaker(),
	}

	matcher := NewRuleMatcher([]Rule{rule1, rule2}, NewBaseConfig(80, 8))

	// Should match rule1
	src := []byte("log.Infof(a)")
	matched := matcher.MatchAt(src, 0)
	if matched == nil {
		t.Error("Expected match for log.Infof")
	}
	if matched.Name() != "call:log.Infof(" {
		t.Errorf("Name() = %q, want %q", matched.Name(),
			"call:log.Infof(")
	}

	// Should match rule2
	src2 := []byte("fmt.Printf(b)")
	matched2 := matcher.MatchAt(src2, 0)
	if matched2 == nil {
		t.Error("Expected match for fmt.Printf")
	}

	// No match
	src3 := []byte("other(c)")
	matched3 := matcher.MatchAt(src3, 0)
	if matched3 != nil {
		t.Error("Expected no match for other()")
	}
}

func TestRuleMatcherApplyAt(t *testing.T) {
	rule := &CallRule{
		Patterns: []string{
			"log.Infof(",
		},
		Breaker: NewLeftFlowBreaker(),
	}
	matcher := NewRuleMatcher([]Rule{rule}, NewBaseConfig(80, 8))

	src := []byte("log.Infof(x)")
	replacement, consumed := matcher.ApplyAt(src, 0)
	if consumed == 0 {
		t.Error("Expected non-zero consumed")
	}
	if replacement == nil {
		t.Error("Expected non-nil replacement")
	}

	// No match
	src2 := []byte("other()")
	replacement2, consumed2 := matcher.ApplyAt(src2, 0)
	if consumed2 != 0 {
		t.Error("Expected zero consumed for no match")
	}
	if replacement2 != nil {
		t.Error("Expected nil replacement for no match")
	}
}

func TestDefaultOperatorRules(t *testing.T) {
	// Verify the default rules are properly ordered by priority
	rules := DefaultOperatorRules
	if len(rules) == 0 {
		t.Error("DefaultOperatorRules is empty")
	}

	// Check that comma has lowest priority (0)
	found := false
	for _, r := range rules {
		if r.Op == "," && r.Priority == 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected comma to have priority 0")
	}

	// Check that && has higher priority than ||
	var andPriority, orPriority int
	for _, r := range rules {
		if r.Op == "&&" {
			andPriority = r.Priority
		}
		if r.Op == "||" {
			orPriority = r.Priority
		}
	}
	if andPriority <= orPriority {
		t.Errorf("Expected && priority (%d) > || priority (%d)",
			andPriority, orPriority)
	}
}
