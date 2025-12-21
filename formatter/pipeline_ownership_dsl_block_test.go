package formatter

import (
	formatstd "go/format"
	"testing"

	llast "github.com/lightninglabs/llformat/ast"
	"github.com/stretchr/testify/require"
)

type noopOwningFormatter struct {
	owned llast.OffsetSpanSet
}

func (f noopOwningFormatter) FormatFile(src []byte) []byte { return src }
func (f noopOwningFormatter) OwnedSpans(src []byte) llast.OffsetSpanSet {
	return f.owned
}

func TestPipeline_OwnershipRegistryBlocksDSLExprEditsInsideOwnedCallArgs(
	t *testing.T) {

	t.Parallel()

	raw := []byte(
		`package p

func f() {
	_ = outerFunctionNameThatIsVeryLong(firstConditionThatIsVeryLong && secondConditionThatIsVeryLong && thirdConditionThatIsVeryLong && fourthConditionThatIsVeryLong)
}
`,
	)
	in, err := formatstd.Source(raw)
	require.NoError(t, err)

	// Force the DSL expression stage to break long logical chains inside
	// call arguments.
	exprRules := dslRulesForExpr(
		StageOptions{
			DSL: DSLStageOptions{
				AllowCallArgs: true,
			},
			Style: StageStyleOptions{
				DSLExprLogicalStyle: "layout",
			},
		},
	)
	expr := NewDSLExprFormatter(
		DSLExprConfig{
			ColumnLimit:   48,
			TabStop:       8,
			Rules:         exprRules,
			MaxIterations: 5,
			SkipGofmt:     true,
			StageName:     "expressions",
		},
	)

	// A later stage declares ownership of call argument lists.
	ownedArgs := llast.OwnedSpansFromSource(
		in, llast.OwnedSpanOptions{
			IncludeCallArgLists: true,
		},
	)
	require.NotEmpty(t, ownedArgs)

	stages := []Stage{
		{
			Name:      "expressions",
			Formatter: expr,
		},
		{
			Name: "call-arg-owner",
			Formatter: noopOwningFormatter{
				owned: ownedArgs,
			},
		},
	}

	// Without ownership registry, the expr stage rewrites inside call args.
	outNoRegistry := NewPipelineWithStages(
		PipelineConfig{ColumnLimit: 48, TabStop: 8},
		stages,
	).Format(in)
	require.NotEqual(t, string(in), string(outNoRegistry))
	require.Contains(
		t, string(outNoRegistry), "firstConditionThatIsVeryLong &&\n",
	)

	// With ownership registry, the expr stage should not rewrite inside
	// owned call-arg spans.
	outRegistry := NewPipelineWithStages(
		PipelineConfig{ColumnLimit: 48, TabStop: 8, UseOwnershipRegistry: true},
		stages,
	).Format(in)
	require.Equal(t, string(in), string(outRegistry))
	requireASTEquivalent(t, in, outRegistry)
}
