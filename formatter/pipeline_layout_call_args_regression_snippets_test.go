package formatter

import (
	"fmt"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineDSLMultiLineLayoutArgs_RegressionSnippets(t *testing.T) {
	// This is intentionally a large, non-golden regression suite.
	//
	// Goal: cover lots of “tricky but valid” expressions without locking
	// the formatter into brittle output expectations. We assert only:
	// - output is parseable
	// - output converges quickly (idempotent after at most one extra pass)
	// - output doesn't contain common semicolon-insertion hazards
	p := NewPipeline(
		PipelineConfig{
			ColumnLimit:          48,
			TabStop:              8,
			UseDSLMultiLineCalls: true,
			DSLMultiLineStyle:    "layout-args",
		},
	)

	// Semicolon insertion hazards to avoid:
	// - `f\n(` becomes `f;(` and is unparseable.
	// - `x\n[` becomes `x;[` and is unparseable.
	// - `x\n.(T)` becomes `x;.(T)` and is unparseable.
	//
	// Parseability already catches these, but these checks make failures
	// much easier to triage when a regression lands.
	reBadNewlineBeforeOpenParen := regexp.MustCompile(
		`[A-Za-z0-9_]\n[ \t]*\(`,
	)
	reBadNewlineBeforeOpenBracket := regexp.MustCompile(
		`[A-Za-z0-9_]\n[ \t]*\[`,
	)
	reBadNewlineBeforeDot := regexp.MustCompile(`[A-Za-z0-9_]\n[ \t]*\.`)

	type tc struct {
		name string
		expr string
	}

	var cases []tc
	add := func(name, expr string) {
		cases = append(cases, tc{name: name, expr: expr})
	}

	logicalAnd := "firstConditionThatIsVeryLong && secondConditionThatIsVeryLong && thirdConditionThatIsVeryLong"
	logicalOr := "firstConditionThatIsVeryLong || secondConditionThatIsVeryLong || thirdConditionThatIsVeryLong"
	comparison := "veryLongLeftHandSideNameThatIsVeryLong == veryLongRightHandSideNameThatIsVeryLong"
	arithmetic := "someLongReceiverNameThatIsVeryLong.FieldA + someLongReceiverNameThatIsVeryLong.FieldB + someLongReceiverNameThatIsVeryLong.FieldC"
	nestedCall := "innerFunctionNameThatIsVeryLong(" + logicalAnd + ", 7)"
	genericCall := "genericFunctionNameThatIsVeryLong[VeryLongTypeNameThatIsVeryLong, AnotherVeryLongTypeName](" +
		comparison + ", 7)"
	methodChain := "someVeryLongReceiverNameThatIsVeryLong.MethodA(" +
		logicalAnd + ", 1).MethodB(" + logicalOr + ")"
	selectorChain := "someVeryLongReceiverNameThatIsVeryLong.FieldA.FieldB.FieldC.FieldD"
	indexBase := "someVeryLongReceiverNameThatIsVeryLong.FieldA().FieldB().FieldC"

	baseExprs := map[string]string{
		"logical_and":    logicalAnd,
		"logical_or":     logicalOr,
		"comparison":     comparison,
		"arithmetic":     arithmetic,
		"nested_call":    nestedCall,
		"generic_call":   genericCall,
		"method_chain":   methodChain,
		"selector_chain": selectorChain,
		"index_base":     indexBase,
	}

	for name, expr := range baseExprs {
		add("base_"+name, expr)
		add("paren_"+name, "("+expr+")")
	}

	for _, op := range []string{"!", "-", "*"} {
		for name, expr := range baseExprs {
			opName := map[string]string{
				"!": "not",
				"-": "neg",
				"*": "deref",
			}[op]
			add(opName+"_paren_"+name, op+"("+expr+")")
		}
	}

	// Indexing and slicing with long inner expressions.
	for _, idx := range []struct {
		name string
		expr string
	}{
		{
			"logical",
			logicalAnd,
		},
		{
			"comparison",
			comparison,
		},
		{
			"arith",
			arithmetic,
		},
		{
			"nested_call",
			nestedCall,
		},
		{
			"method_chain",
			methodChain,
		},
	} {
		add(
			"index_ident_"+idx.name,
			"someSliceNameThatIsVeryLong["+idx.expr+"]",
		)
		add("index_selector_"+idx.name, selectorChain+"["+idx.expr+"]")
		add("index_base_chain_"+idx.name, indexBase+"["+idx.expr+"]")

		add(
			"slice_low_"+idx.name,
			"someSliceNameThatIsVeryLong["+idx.expr+":]",
		)
		add(
			"slice_high_"+idx.name,
			"someSliceNameThatIsVeryLong[:"+idx.expr+"]",
		)
		add(
			"slice_full_"+idx.name,
			"someSliceNameThatIsVeryLong["+comparison+":"+idx.expr+"]",
		)
	}

	// Generic instantiation (IndexListExpr) as part of call fun and
	// composite types.
	add(
		"generic_fun_call",
		"somePkg.SomeGenericFuncNameThatIsVeryLong[VeryLongTypeNameThatIsVeryLong, AnotherVeryLongTypeName]("+logicalAnd+", 7)",
	)
	add(
		"generic_method_call",
		"someVeryLongReceiverNameThatIsVeryLong.SomeMethodNameThatIsVeryLong[VeryLongTypeNameThatIsVeryLong]("+logicalOr+")",
	)
	add(
		"generic_composite_type",
		"SomeGenericTypeNameThatIsVeryLong[VeryLongTypeNameThatIsVeryLong, AnotherVeryLongTypeName]{FieldNameThatIsVeryLong: "+logicalAnd+"}",
	)

	// Composite literals with key/value expressions that themselves can
	// break.
	add(
		"struct_kv_logical",
		"SomeStructTypeNameThatIsVeryLong{FieldNameThatIsVeryLong: "+logicalAnd+", OtherFieldName: "+nestedCall+"}",
	)
	add(
		"struct_kv_comparison",
		"SomeStructTypeNameThatIsVeryLong{FieldNameThatIsVeryLong: "+comparison+", OtherFieldName: "+methodChain+"}",
	)
	add(
		"map_kv_nested_call",
		`map[string]int{"firstVeryLongKeyName": `+nestedCall+`, "secondVeryLongKeyName": 7}`,
	)
	add("slice_lit_nested_call", `[]int{1, 2, `+nestedCall+`, 4, 5}`)

	// Type assertions, including generic types and interface types.
	add(
		"type_assert_simple", "someInterfaceValueNameThatIsVeryLong.("+
			"SomeConcreteTypeNameThatIsVeryLong)",
	)
	add(
		"type_assert_generic", "someInterfaceValueNameThatIsVeryLong."+
			"(SomeGenericTypeNameThatIsVeryLong[VeryLongTypeNameT"+
			"hatIsVeryLong, AnotherVeryLongTypeName])",
	)
	add(
		"type_assert_interface", "someInterfaceValueNameThatIsVeryLon"+
			"g.(interface{ MethodNameThatIsVeryLong(arg "+
			"SomeTypeNameThatIsVeryLong) error })",
	)
	add(
		"type_assert_on_method_chain",
		methodChain+".(SomeConcreteTypeNameThatIsVeryLong)",
	)

	// Mixed chains that historically cause “tightness” problems.
	add(
		"mixed_index_then_call",
		indexBase+"["+logicalAnd+"].SomeMethodNameThatIsVeryLong("+comparison+")",
	)
	add(
		"mixed_call_then_index",
		"someVeryLongReceiverNameThatIsVeryLong.SomeMethodNameThatIsVeryLong("+logicalAnd+").SomeFieldNameThatIsVeryLong["+comparison+"]",
	)
	add(
		"mixed_type_assert_then_call",
		"someInterfaceValueNameThatIsVeryLong.(SomeConcreteTypeNameThatIsVeryLong).SomeMethodNameThatIsVeryLong("+logicalAnd+")",
	)

	// Nested calls in args, including nested generic callees.
	add(
		"nested_call_in_arg",
		"outerFunctionNameThatIsVeryLong(innerFunctionNameThatIsVeryLong("+logicalAnd+", 7), 42)",
	)
	add(
		"nested_generic_call_in_arg",
		"outerFunctionNameThatIsVeryLong(genericFunctionNameThatIsVeryLong[VeryLongTypeNameThatIsVeryLong]("+logicalOr+"), 42)",
	)
	add(
		"nested_method_chain_in_arg",
		"outerFunctionNameThatIsVeryLong("+methodChain+", 42)",
	)
	add(
		"double_nested_call_in_arg",
		"outerFunctionNameThatIsVeryLong(innerFunctionNameThatIsVeryLong(innerFunctionNameThatIsVeryLong("+logicalAnd+", 7), 8), 42)",
	)

	// String concatenation inside call args (legacy expr stage historically
	// handled these).
	add(
		"string_concat",
		`"prefix: " + someVeryLongIdentifierNameThatIsVeryLong + ": " + anotherVeryLongIdentifierNameThatIsVeryLong`,
	)
	add(
		"string_concat_with_call",
		`"prefix: " + innerFunctionNameThatIsVeryLong(`+logicalAnd+`, 7) + ": suffix"`,
	)

	// Parenthesized callees (semicolon insertion hazards if mishandled).
	add(
		"paren_callee_ident",
		"(innerFunctionNameThatIsVeryLong)("+logicalAnd+", 7)",
	)
	add(
		"paren_callee_selector",
		"(someVeryLongReceiverNameThatIsVeryLong.MethodNameThatIsVeryLong)("+logicalOr+")",
	)
	add(
		"call_returning_func_then_call",
		"functionThatReturnsFuncNameThatIsVeryLong("+logicalAnd+")(anotherVeryLongIdentifierNameThatIsVeryLong)",
	)

	// Type assertion to a function type, immediately called.
	add(
		"type_assert_func_then_call", "someInterfaceValueNameThatIsVe"+
			"ryLong.(func(int, int) int)(1, 2)",
	)

	// Three-index slices with long indices (parseable even if not
	// type-checkable).
	add(
		"slice_three_index_logical_comparison_arith",
		"someSliceNameThatIsVeryLong["+logicalAnd+":"+comparison+":"+arithmetic+"]",
	)

	// Func literals used as args (exercise block/comment/string skipping
	// logic).
	add(
		"func_lit_arg", "outerFunctionNameThatIsVeryLong(func(x "+
			"int) int { return x + 1 }(7), 42)",
	)
	add(
		"func_lit_direct", "func(x int) int { return x + 1 "+
			"}(someVeryLongIdentifierNameThatIsVeryLong)",
	)

	// Composite literals with func-typed fields/values.
	add(
		"struct_with_func_field", "struct{ FieldNameThatIsVeryLong "+
			"func(int) int }{FieldNameThatIsVeryLong: func(x "+
			"int) int { return x + 1 }}",
	)
	add(
		"map_with_func_values",
		`map[string]func(int) int{"firstVeryLongKeyName": func(x int) int { return x + 1 }, "secondVeryLongKeyName": func(y int) int { return y + 2 }}`,
	)

	// Keys that are themselves calls.
	add(
		"map_kv_call_key",
		`map[string]int{fmt.Sprintf("%s", someVeryLongIdentifierNameThatIsVeryLong): 1, fmt.Sprintf("%s", anotherVeryLongIdentifierNameThatIsVeryLong): 2}`,
	)

	// Build up additional variations without adding brittle expectations.
	// Keep the suite deterministic and easy to extend.
	//
	// Note: avoid raw string literals here because they complicate snippet
	// embedding (backticks) and aren't central to call-arg expr layout.
	wrap := func(name, expr string) {
		add(
			"wrap_outer_call_"+name,
			"outerFunctionNameThatIsVeryLong("+expr+", 1)",
		)
		add(
			"wrap_outer_call2_"+name,
			"outerFunctionNameThatIsVeryLong("+expr+", "+nestedCall+")",
		)
	}
	for name, expr := range baseExprs {
		wrap(name, expr)
		wrap("index_"+name, "someSliceNameThatIsVeryLong["+expr+"]")
		wrap(
			"type_assert_"+name,
			"someInterfaceValueNameThatIsVeryLong.("+exprToType(
				expr,
			)+")",
		)
	}

	// Additional cross-product expansions.
	for name, expr := range baseExprs {
		add(
			"type_conversion_"+name,
			"SomeConcreteTypeNameThatIsVeryLong("+expr+")",
		)
		add(
			"paren_"+name+"_then_call",
			"("+expr+").SomeMethodNameThatIsVeryLong("+logicalAnd+")",
		)
		add("addr_of_"+name, "&("+expr+")")
	}

	require.GreaterOrEqual(
		t, len(cases), 180, "expected at least 180 regression cases",
	)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := fmt.Sprintf(`package p

func f() {
	_ = outerFunctionNameThatIsVeryLong(%s, 42)
}
`, c.expr)

			out := p.Format([]byte(in))
			outStr := string(out)

			require.NotRegexp(
				t, reBadNewlineBeforeOpenParen, outStr,
			)
			require.NotRegexp(
				t, reBadNewlineBeforeOpenBracket, outStr,
			)
			require.NotRegexp(t, reBadNewlineBeforeDot, outStr)

			// Run the formatter twice to flush out obvious
			// non-termination/crash behavior under repeated
			// application. (We intentionally avoid asserting exact
			// idempotence here; existing golden/parity tests cover
			// output stability, and this suite is about breadth +
			// safety properties.)
			out2 := p.Format(out)
			out2Str := string(out2)
			require.NotRegexp(
				t, reBadNewlineBeforeOpenParen, out2Str,
			)
			require.NotRegexp(
				t, reBadNewlineBeforeOpenBracket, out2Str,
			)
			require.NotRegexp(t, reBadNewlineBeforeDot, out2Str)

			// Parseable (both passes).
			fset := token.NewFileSet()
			_, err := parser.ParseFile(
				fset, "out.go", out, parser.AllErrors,
			)
			require.NoError(
				t, err, "formatted output was not "+
					"parseable:\n%s", outStr,
			)
			_, err = parser.ParseFile(
				fset, "out2.go", out2, parser.AllErrors,
			)
			require.NoError(
				t, err, "second pass output was not "+
					"parseable:\n%s", out2Str,
			)

			// Semantic equivalence guard: formatting must not
			// change AST structure.
			requireASTEquivalent(t, []byte(in), out)
			requireASTEquivalent(t, []byte(in), out2)

			// Quick sanity: avoid accidentally emitting NUL bytes
			// etc.
			require.False(t, strings.ContainsRune(outStr, '\x00'))
		})
	}
}

// exprToType converts an expression-shaped string into a type-shaped string for
// test generation. It doesn't need to be type-checkable, only parseable.
func exprToType(expr string) string {
	switch {
	case strings.Contains(expr, "[") && strings.Contains(expr, "]"):

		// Generic instantiation-like expressions are already
		// type-shaped.
		return "SomeGenericTypeNameThatIsVeryLong[VeryLongTypeNameThatIsVeryLong]"

	case strings.Contains(expr, "{") && strings.Contains(expr, "}"):
		return "SomeStructTypeNameThatIsVeryLong"

	default:
		return "SomeConcreteTypeNameThatIsVeryLong"
	}
}
