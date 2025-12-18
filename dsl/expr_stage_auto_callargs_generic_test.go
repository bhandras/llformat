package dsl

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExprStage_AutoCallArgs_AllowlistSupportsGenericCallCallee(t *testing.T) {
	t.Parallel()

	const src = `package p

func f() {
	_ = genericCall[VeryLongTypeNameOne, VeryLongTypeNameTwo, VeryLongTypeNameThree](firstConditionThatIsVeryLong && secondConditionThatIsVeryLong && thirdConditionThatIsVeryLong)
}
`

	engine := NewEngine(LongExprRulesWithOptions(LongExprOptions{
		CallArgsPolicy:    CallArgsPolicyAuto,
		CallArgsAllowlist: []string{"genericCall"},
		LogicalChainStyle: "legacy",
	}))
	engine.ColumnLimit = 40
	engine.TabStop = 8

	out := engine.FormatFile([]byte(src))
	outStr := string(out)

	require.Contains(t, outStr, "&&\n")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors)
	require.NoError(t, err)
}
