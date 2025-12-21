package formatter

import "testing"

func TestDefaultPreset(t *testing.T) {
	preset := DefaultPreset()

	if preset.Config.ColumnLimit != DefaultColumnLimit {
		t.Errorf("ColumnLimit = %d, want %d", preset.Config.ColumnLimit,
			DefaultColumnLimit)
	}
	if preset.Config.TabStop != DefaultTabStop {
		t.Errorf("TabStop = %d, want %d", preset.Config.TabStop,
			DefaultTabStop)
	}
	if len(preset.CallRules) != 2 {
		t.Errorf("len(CallRules) = %d, want 2", len(preset.CallRules))
	}
}

func TestNewPreset(t *testing.T) {
	preset := NewPreset(100, 4)

	if preset.Config.ColumnLimit != 100 {
		t.Errorf("ColumnLimit = %d, want 100",
			preset.Config.ColumnLimit)
	}
	if preset.Config.TabStop != 4 {
		t.Errorf("TabStop = %d, want 4", preset.Config.TabStop)
	}
}

func TestPresetWithCallPatterns(t *testing.T) {
	preset := DefaultPreset()
	customBreaker := NewVerticalBreaker()

	newPreset := preset.WithCallPatterns(
		[]string{"custom.Call("}, customBreaker, 20,
	)

	// Original should be unchanged
	if len(preset.CallRules) != 2 {
		t.Errorf("Original preset modified, len = %d",
			len(preset.CallRules))
	}

	// New preset should have 3 rules
	if len(newPreset.CallRules) != 3 {
		t.Errorf("New preset len = %d, want 3",
			len(newPreset.CallRules))
	}

	// Check the new rule
	lastRule := newPreset.CallRules[len(newPreset.CallRules)-1]
	if lastRule.Patterns[0] != "custom.Call(" {
		t.Errorf("Pattern = %q, want %q", lastRule.Patterns[0], "cust"+
			"om.Call(")
	}
	if lastRule.Priority != 20 {
		t.Errorf("Priority = %d, want 20", lastRule.Priority)
	}
}

func TestPresetToRules(t *testing.T) {
	preset := DefaultPreset()
	rules := preset.ToRules()

	if len(rules) != 2 {
		t.Errorf("len(rules) = %d, want 2", len(rules))
	}

	for i, rule := range rules {
		if rule == nil {
			t.Errorf("rules[%d] is nil", i)
		}
	}
}

func TestPresetRuleMatcher(t *testing.T) {
	preset := DefaultPreset()
	matcher := preset.RuleMatcher()

	if matcher == nil {
		t.Fatal("RuleMatcher() returned nil")
	}
	if len(matcher.Rules) != 2 {
		t.Errorf("len(matcher.Rules) = %d, want 2", len(matcher.Rules))
	}

	// Test that it can match
	src := []byte("log.Infof(a)")
	matched := matcher.MatchAt(src, 0)
	if matched == nil {
		t.Error("Expected match for log.Infof")
	}
}
