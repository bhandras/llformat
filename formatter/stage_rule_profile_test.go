package formatter

import (
	"testing"

	"github.com/lightninglabs/llformat/dsl"
	"github.com/stretchr/testify/require"
)

func TestDSLBundlesMultiLineStyleControlsNodeOrder(t *testing.T) {
	t.Parallel()

	_, nodeOrder := dslRulesForMultiLineCalls(StageOptions{
		Style: StageStyleOptions{DSLMultiLineStyle: "legacy"},
	})
	require.Equal(t, dsl.NodeOrderPreorder, nodeOrder)

	_, nodeOrder = dslRulesForMultiLineCalls(StageOptions{
		Style: StageStyleOptions{DSLMultiLineStyle: "layout-args"},
	})
	require.Equal(t, dsl.NodeOrderDeepestFirst, nodeOrder)

	_, nodeOrder = dslRulesForMultiLineCalls(StageOptions{
		Style: StageStyleOptions{DSLMultiLineStyle: "layout-all"},
	})
	require.Equal(t, dsl.NodeOrderDeepestFirst, nodeOrder)

	_, nodeOrder = dslRulesForMultiLineCalls(StageOptions{
		Style: StageStyleOptions{DSLMultiLineStyle: "layout-args-groups-pairs"},
	})
	require.Equal(t, dsl.NodeOrderDeepestFirst, nodeOrder)
}

func TestDSLBundlesMultiLineStyleDefaultsFromRuleProfile(t *testing.T) {
	t.Parallel()

	rules, nodeOrder := dslRulesForMultiLineCalls(StageOptions{
		Selection: StageSelectionOptions{RuleProfile: "parity"},
	})
	require.Equal(t, dsl.NodeOrderPreorder, nodeOrder)
	require.NotEmpty(t, rules)
	require.Equal(t, "legacy_multiline_scan", rules[0].Name)

	rules, nodeOrder = dslRulesForMultiLineCalls(StageOptions{
		Selection: StageSelectionOptions{RuleProfile: "modern"},
	})
	require.Equal(t, dsl.NodeOrderPreorder, nodeOrder)
	require.Len(t, rules, 2)
	require.Equal(t, "long_method_chain", rules[0].Name)
	require.Equal(t, "long_call_expr", rules[1].Name)

	rules, nodeOrder = dslRulesForMultiLineCalls(StageOptions{
		Selection: StageSelectionOptions{RuleProfile: "next"},
	})
	require.Equal(t, dsl.NodeOrderDeepestFirst, nodeOrder)
	require.Len(t, rules, 2)
	require.Equal(t, "long_method_chain", rules[0].Name)
	require.Equal(t, "long_call_expr", rules[1].Name)
}

func TestDSLBundlesSignaturesNativeAlwaysIncludesLegacyFallback(t *testing.T) {
	t.Parallel()

	rules := dslRulesForSignatures(StageOptions{
		DSL:   DSLStageOptions{UseFuncSigsNative: true},
		Style: StageStyleOptions{DSLSigsStyle: "dsl"},
	})
	require.NotEmpty(t, rules)
	require.Equal(t, "legacy_func_sig_fallback", rules[len(rules)-1].Name)
}

func TestDSLBundlesBlankLinesNativeAlwaysIncludesLegacyFallback(t *testing.T) {
	t.Parallel()

	rules := dslRulesForBlankLines(StageOptions{
		DSL: DSLStageOptions{UseBlankLinesNative: true},
	})
	require.NotEmpty(t, rules)
	require.Equal(t, "legacy_blank_lines_fallback", rules[len(rules)-1].Name)

	legacyRules := dslRulesForBlankLines(StageOptions{
		DSL: DSLStageOptions{UseBlankLinesNative: false},
	})
	require.Len(t, legacyRules, 1)
	require.Equal(t, "legacy_blank_lines_format", legacyRules[0].Name)
}
