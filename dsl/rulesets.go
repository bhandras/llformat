package dsl

// RuleSet identifies a cohesive bundle of DSL formatting rules.
//
// The goal is to make it explicit when callers want:
// - legacy-parity behavior (used by golden tests and stable CLI modes), vs.
// - modern/experimental behavior (opt-in).
type RuleSet string

const (
	// RuleSetDefault is the default full DSL rule set (signatures, calls,
	// expressions, blank lines). It is intended to reproduce legacy
	// behavior when format functions are injected via DefaultRuleOptions.
	RuleSetDefault RuleSet = "default"

	// RuleSetExpressionOnly is expression-breaking rules intended to run
	// alongside other dedicated rule sets (calls/signatures) without
	// overlap.
	RuleSetExpressionOnly RuleSet = "expression-only"

	// RuleSetExpressionCompat is a conservative expression + signature
	// subset used for tests that expect legacy-like signature wrapping.
	RuleSetExpressionCompat RuleSet = "expression-compat"
)

// RulesFor returns the rule list for a named rule set.
func RulesFor(set RuleSet, opts DefaultRuleOptions) []Rule {
	switch set {
	case RuleSetDefault:
		return DefaultRulesWithOptions(opts)

	case RuleSetExpressionOnly:
		return expressionOnlyRules()

	case RuleSetExpressionCompat:
		return ExpressionRules()

	default:

		// Unknown rule set: fall back to the default for safety.
		return DefaultRulesWithOptions(opts)
	}
}
