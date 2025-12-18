package formatter

import (
	"testing"

	"github.com/lightninglabs/llformat/dsl"
	"github.com/stretchr/testify/require"
)

func TestDSLBundlesMultiLineStyleControlsNodeOrder(t *testing.T) {
	t.Parallel()

	_, nodeOrder := dslRulesForMultiLineCalls(StageOptions{DSLMultiLineStyle: "legacy"})
	require.Equal(t, dsl.NodeOrderPreorder, nodeOrder)

	_, nodeOrder = dslRulesForMultiLineCalls(StageOptions{DSLMultiLineStyle: "layout-args"})
	require.Equal(t, dsl.NodeOrderDeepestFirst, nodeOrder)

	_, nodeOrder = dslRulesForMultiLineCalls(StageOptions{DSLMultiLineStyle: "layout-all"})
	require.Equal(t, dsl.NodeOrderDeepestFirst, nodeOrder)
}

func TestDSLBundlesSignaturesNativeAlwaysIncludesLegacyFallback(t *testing.T) {
	t.Parallel()

	rules := dslRulesForSignatures(StageOptions{
		UseDSLFuncSigsNative: true,
		DSLSigsStyle:         "dsl",
	})
	require.NotEmpty(t, rules)
	require.Equal(t, "legacy_func_sig_fallback", rules[len(rules)-1].Name)
}

func TestDSLBundlesBlankLinesNativeAlwaysIncludesLegacyFallback(t *testing.T) {
	t.Parallel()

	rules := dslRulesForBlankLines(StageOptions{
		UseDSLBlankLinesNative: true,
	})
	require.NotEmpty(t, rules)
	require.Equal(t, "legacy_blank_lines_fallback", rules[len(rules)-1].Name)

	legacyRules := dslRulesForBlankLines(StageOptions{
		UseDSLBlankLinesNative: false,
	})
	require.Len(t, legacyRules, 1)
	require.Equal(t, "legacy_blank_lines_format", legacyRules[0].Name)
}
