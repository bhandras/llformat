package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDSLCallPolicy_CallOnlyVariants_EnableExpectedStages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                    string
		policy                  string
		wantMultiLineStyle      string
		wantUseDSLExpr          bool
		wantExpressionsIsDSL    bool
		wantLogCallsIsDSL       bool
		wantMultiLineCallsIsDSL bool
	}{
		{
			name:                    "packed",
			policy:                  "packed",
			wantMultiLineStyle:      "packed",
			wantUseDSLExpr:          false,
			wantExpressionsIsDSL:    false,
			wantLogCallsIsDSL:       true,
			wantMultiLineCallsIsDSL: true,
		},
		{
			name:                    "packed_chain",
			policy:                  "packed-chain",
			wantMultiLineStyle:      "packed-chain",
			wantUseDSLExpr:          false,
			wantExpressionsIsDSL:    false,
			wantLogCallsIsDSL:       true,
			wantMultiLineCallsIsDSL: true,
		},
		{
			name:                    "layout_args_auto_enables_expr",
			policy:                  "layout-args",
			wantMultiLineStyle:      "layout-args",
			wantUseDSLExpr:          true, // auto-enabled for call-args ownership
			wantExpressionsIsDSL:    true,
			wantLogCallsIsDSL:       true,
			wantMultiLineCallsIsDSL: true,
		},
		{
			name:                    "layout_args_groups_pairs_auto_enables_expr",
			policy:                  "layout-args-groups-pairs",
			wantMultiLineStyle:      "layout-args-groups-pairs",
			wantUseDSLExpr:          true,
			wantExpressionsIsDSL:    true,
			wantLogCallsIsDSL:       true,
			wantMultiLineCallsIsDSL: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := NewPipeline(PipelineConfig{
				ColumnLimit:   80,
				TabStop:       8,
				DSLCallPolicy: tc.policy,
			})

			require.Equal(t, tc.wantMultiLineStyle, p.cfg.DSLMultiLineStyle)
			require.Equal(t, tc.wantUseDSLExpr, p.cfg.UseDSLExpr)

			plan := stagePlanFromPipelineConfig(p.cfg)
			require.Equal(t, tc.wantLogCallsIsDSL, plan.LogCalls == StageModeDSL)
			require.Equal(t, tc.wantMultiLineCallsIsDSL, plan.MultiLineCalls == StageModeDSL)
			require.Equal(t, tc.wantExpressionsIsDSL, plan.Expressions == StageModeDSL)

			// Call-only policies should not implicitly enable unrelated DSL stages.
			require.Equal(t, StageModeLegacy, plan.Comments)
			require.Equal(t, StageModeLegacy, plan.Signatures)
			require.Equal(t, StageModeLegacy, plan.BlankLines)
		})
	}
}
