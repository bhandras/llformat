package dsl

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEngineKeepSimpleComparison(t *testing.T) {
	// This test verifies that simple comparisons are marked as atomic and
	// not broken across lines

	src := `package main

func foo(x int) bool {
	return x > 0
}
`

	engine := NewEngine(DefaultRules())
	result, err := engine.Format([]byte(src))
	require.NoError(t, err)

	// Should remain unchanged - x > 0 is atomic
	require.Equal(t, src, string(result))
}

func TestEngineReflowLongCall(t *testing.T) {
	src := `package main

func foo() {
	result := someVeryLongFunctionName(argumentOne, argumentTwo, argumentThree, argumentFour)
	_ = result
}
`

	engine := NewEngine(DefaultRules())
	engine.ColumnLimit = 60 // Force wrapping
	result, err := engine.Format([]byte(src))
	require.NoError(t, err)

	// Should have reformatted the call
	got := string(result)
	require.Contains(t, got, "someVeryLongFunctionName(\n")
	require.Contains(t, got, "argumentOne,")
}

func TestEngineIfCallComparison(t *testing.T) {
	src := `package main

func foo(s string) {
	if len(someLongFunctionCall(argument1, argument2, argument3)) > 10 {
		doSomething()
	}
}

func someLongFunctionCall(a, b, c string) string { return "" }
func doSomething() {}
`

	engine := NewEngine(DefaultRules())
	engine.ColumnLimit = 60
	result, err := engine.Format([]byte(src))
	require.NoError(t, err)

	got := string(result)
	// The function call should be reformatted, but > 10 should stay
	// together
	require.Contains(t, got, "> 10")
	// And the comparison shouldn't be broken
	require.NotContains(t, got, ">\n\t\t10")
}

func TestEngineLogicalChain(t *testing.T) {
	src := `package main

func foo(a, b, c, d, e bool) bool {
	return a && b && c && d && e && true && false
}
`

	engine := NewEngine(DefaultRules())
	engine.ColumnLimit = 40
	result, err := engine.Format([]byte(src))
	require.NoError(t, err)

	got := string(result)
	// Should break after && operator
	lines := strings.Split(got, "\n")
	hasBreak := false
	for _, line := range lines {
		if strings.HasSuffix(strings.TrimSpace(line), "&&") {
			hasBreak = true
			break
		}
	}
	require.True(t, hasBreak, "should break after && operator")
}

func TestEnginePriorityOrder(t *testing.T) {
	// Verify rules are sorted by priority
	engine := NewEngine(DefaultRules())

	prevPriority := 1000
	for _, rule := range engine.Rules {
		require.GreaterOrEqual(
			t, prevPriority, rule.Priority,
			"rules should be sorted by descending priority",
		)
		prevPriority = rule.Priority
	}
}

func TestEngineMaxIterations(t *testing.T) {
	// Test that engine doesn't loop forever
	src := `package main

func foo() {
	x := 1
}
`

	engine := NewEngine(DefaultRules())
	engine.MaxIterations = 3
	result, err := engine.Format([]byte(src))
	require.NoError(t, err)
	require.NotEmpty(t, result)
}

func TestEngineEmptyRules(t *testing.T) {
	src := `package main

func foo() {}
`

	engine := NewEngine([]Rule{})
	result, err := engine.Format([]byte(src))
	require.NoError(t, err)
	require.Equal(t, src, string(result))
}
