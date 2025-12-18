package formatter

import (
	"testing"

	"github.com/lightninglabs/llformat/dsl"
	"github.com/stretchr/testify/require"
)

func TestDSLBundle_MultiLineSpecFollowsRuleProfileDefaults(t *testing.T) {
	t.Parallel()

	parity := ResolveDSLBundle(StageOptions{RuleProfile: "parity"})
	require.NotEmpty(t, parity.MultiLineCalls.Rules)
	require.Equal(t, "legacy_multiline_scan", parity.MultiLineCalls.Rules[0].Name)
	require.Equal(t, dsl.NodeOrderPreorder, parity.MultiLineCalls.NodeOrder)
	require.Equal(t, 20, parity.MultiLineCalls.MaxIterations)

	modern := ResolveDSLBundle(StageOptions{RuleProfile: "modern"})
	require.Len(t, modern.MultiLineCalls.Rules, 2)
	require.Equal(t, "long_method_chain", modern.MultiLineCalls.Rules[0].Name)
	require.Equal(t, "long_call_expr", modern.MultiLineCalls.Rules[1].Name)
	require.Equal(t, dsl.NodeOrderPreorder, modern.MultiLineCalls.NodeOrder)

	next := ResolveDSLBundle(StageOptions{RuleProfile: "next"})
	require.Len(t, next.MultiLineCalls.Rules, 2)
	require.Equal(t, "long_method_chain", next.MultiLineCalls.Rules[0].Name)
	require.Equal(t, "long_call_expr", next.MultiLineCalls.Rules[1].Name)
	require.Equal(t, dsl.NodeOrderDeepestFirst, next.MultiLineCalls.NodeOrder)
}

func TestDSLBundle_BlankLinesSpecControlsShimAndIterations(t *testing.T) {
	t.Parallel()

	legacyFallback := ResolveDSLBundle(StageOptions{
		UseDSLBlankLinesNative: false,
	})
	require.False(t, legacyFallback.BlankLines.DisableLegacyBlankLinesShim)
	require.Equal(t, 1, legacyFallback.BlankLines.MaxIterations)
	require.NotEmpty(t, legacyFallback.BlankLines.Rules)
	require.Equal(t, "legacy_blank_lines_format", legacyFallback.BlankLines.Rules[0].Name)

	native := ResolveDSLBundle(StageOptions{
		UseDSLBlankLinesNative: true,
	})
	require.True(t, native.BlankLines.DisableLegacyBlankLinesShim)
	require.Equal(t, 200, native.BlankLines.MaxIterations)
	require.NotEmpty(t, native.BlankLines.Rules)
	names := make([]string, 0, len(native.BlankLines.Rules))
	for _, r := range native.BlankLines.Rules {
		names = append(names, r.Name)
	}
	require.Contains(t, names, "blank_before_case")
	require.Contains(t, names, "blank_before_return")
	require.Contains(t, names, "blank_between_interface_methods")
	require.Equal(t, "legacy_blank_lines_fallback", native.BlankLines.Rules[len(native.BlankLines.Rules)-1].Name)
}

func TestDSLBundle_SignaturesSpecControlsIterations(t *testing.T) {
	t.Parallel()

	legacy := ResolveDSLBundle(StageOptions{UseDSLFuncSigsNative: false})
	require.Equal(t, 1, legacy.Signatures.MaxIterations)

	native := ResolveDSLBundle(StageOptions{UseDSLFuncSigsNative: true})
	require.Equal(t, 100, native.Signatures.MaxIterations)
	require.NotEmpty(t, native.Signatures.Rules)
	require.Equal(t, "legacy_func_sig_fallback", native.Signatures.Rules[len(native.Signatures.Rules)-1].Name)
}
