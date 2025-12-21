package formatter

import (
	"fmt"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipeline_ASTEquivalentAcrossPolicies_ExpressionSnippets(t *testing.T) {
	// Property-style test: for a variety of valid sources, formatting must keep
	// the AST structure equivalent, regardless of which pipeline policy is used.
	//
	// This intentionally does not assert exact output strings (non-golden).
	type policy struct {
		name string
		cfg  PipelineConfig
	}

	policies := []policy{
		{name: "next", cfg: PipelineConfig{ColumnLimit: 48, TabStop: 8}},
		{
			name: "next_with_ownership",
			cfg:  PipelineConfig{ColumnLimit: 48, TabStop: 8, UseOwnershipRegistry: true},
		},
	}

	exprs := map[string]string{
		"logical_chain": "firstConditionThatIsVeryLong && secondConditionThatIsVeryLong && thirdConditionThatIsVeryLong",
		"comparison":    "veryLongLeftHandSideNameThatIsVeryLong == veryLongRightHandSideNameThatIsVeryLong",
		"arithmetic":    "someLongReceiverNameThatIsVeryLong.FieldA + someLongReceiverNameThatIsVeryLong.FieldB + someLongReceiverNameThatIsVeryLong.FieldC",
		"selector":      "someVeryLongReceiverNameThatIsVeryLong.FieldA.FieldB.FieldC.FieldD",
		"index":         "someVeryLongReceiverNameThatIsVeryLong.FieldA().FieldB().FieldC[veryLongLeftHandSideNameThatIsVeryLong]",
		"slice_full":    "someSliceNameThatIsVeryLong[veryLongLeftHandSideNameThatIsVeryLong:veryLongRightHandSideNameThatIsVeryLong]",
		"type_assert":   "someInterfaceValueNameThatIsVeryLong.(SomeConcreteTypeNameThatIsVeryLong)",
		"nested_call":   "outerFunctionNameThatIsVeryLong(innerFunctionNameThatIsVeryLong(firstConditionThatIsVeryLong && secondConditionThatIsVeryLong, 7), 42)",
		"generic_call":  "genericFunctionNameThatIsVeryLong[VeryLongTypeNameThatIsVeryLong, AnotherVeryLongTypeName](veryLongLeftHandSideNameThatIsVeryLong == veryLongRightHandSideNameThatIsVeryLong, 7)",
		"method_chain":  "someVeryLongReceiverNameThatIsVeryLong.MethodA(firstConditionThatIsVeryLong && secondConditionThatIsVeryLong, 1).MethodB(thirdConditionThatIsVeryLong || fourthConditionThatIsVeryLong)",
		"struct_kv":     "SomeStructTypeNameThatIsVeryLong{FieldNameThatIsVeryLong: firstConditionThatIsVeryLong && secondConditionThatIsVeryLong, OtherFieldName: innerFunctionNameThatIsVeryLong(thirdConditionThatIsVeryLong, 7)}",
		"map_kv":        `map[string]int{"firstVeryLongKeyName": innerFunctionNameThatIsVeryLong(firstConditionThatIsVeryLong && secondConditionThatIsVeryLong, 7), "secondVeryLongKeyName": 7}`,
		"string_concat": `"prefix: " + someVeryLongIdentifierNameThatIsVeryLong + ": " + anotherVeryLongIdentifierNameThatIsVeryLong`,
	}

	for policyIndex := range policies {
		pol := policies[policyIndex]
		t.Run(pol.name, func(t *testing.T) {
			p := NewPipeline(pol.cfg)

			for name, expr := range exprs {
				t.Run(name, func(t *testing.T) {
					in := fmt.Sprintf(`package p

func f() {
	_ = %s
	_ = outerFunctionNameThatIsVeryLong(%s, 42)
	_ = outerFunctionNameThatIsVeryLong(outerFunctionNameThatIsVeryLong(%s, 1), 42)
}
`, expr, expr, expr)

					out := p.Format([]byte(in))
					require.NotEmpty(t, out)

					// Parseable output.
					fset := token.NewFileSet()
					_, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors)
					require.NoError(t, err, "formatted output was not parseable:\n%s", string(out))

					// Semantic equivalence (ignore positions/scopes/comments).
					requireASTEquivalent(t, []byte(in), out)

					// Second pass should remain semantically equivalent and parseable.
					out2 := p.Format(out)
					_, err = parser.ParseFile(fset, "out2.go", out2, parser.AllErrors)
					require.NoError(t, err, "second pass output was not parseable:\n%s", string(out2))
					requireASTEquivalent(t, []byte(in), out2)
				})
			}
		})
	}
}
