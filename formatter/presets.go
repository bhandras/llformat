package formatter

// Preset defines a complete formatting configuration.
type Preset struct {
	Config    BaseConfig
	CallRules []CallRule
}

// SpacingOptions controls blank line formatting.
type SpacingOptions struct {
	BlankBeforeReturn   bool // Add blank line before return statements
	BlankBetweenCases   bool // Add blank line between case blocks
	BlankBetweenMethods bool // Add blank line between methods
}

// DefaultPreset returns the standard llformat configuration.
func DefaultPreset() Preset {
	return Preset{
		Config: NewBaseConfig(DefaultColumnLimit, DefaultTabStop),
		CallRules: []CallRule{
			{
				Patterns: DefaultLogPatterns(),
				Breaker: &LeftFlowBreaker{
					Separator:     ", ",
					OpenBracket:   "(",
					CloseBracket:  ")",
					TrailingComma: false,
				},
				Priority: 10,
			},
			{
				Patterns: DefaultFmtPatterns(),
				Breaker: &LeftFlowBreaker{
					Separator:     ", ",
					OpenBracket:   "(",
					CloseBracket:  ")",
					TrailingComma: false,
				},
				Priority: 10,
			},
		},
	}
}

// NewPreset creates a preset with the given configuration.
func NewPreset(columnLimit, tabStop int) Preset {
	preset := DefaultPreset()
	preset.Config = NewBaseConfig(columnLimit, tabStop)

	return preset
}

// WithCallPatterns returns a new Preset with additional call patterns.
func (p Preset) WithCallPatterns(patterns []string, breaker Breaker,
	priority int) Preset {

	rule := CallRule{
		Patterns: patterns,
		Breaker:  breaker,
		Priority: priority,
	}
	p.CallRules = append(p.CallRules, rule)

	return p
}

// ToRules converts the preset's call rules to a slice of Rule interface.
func (p Preset) ToRules() []Rule {
	rules := make([]Rule, len(p.CallRules))
	for i := range p.CallRules {
		rules[i] = &p.CallRules[i]
	}

	return rules
}

// RuleMatcher creates a RuleMatcher from this preset.
func (p Preset) RuleMatcher() *RuleMatcher {
	return NewRuleMatcher(p.ToRules(), p.Config)
}
