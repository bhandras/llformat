package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompactCallFormatter_ASTSelectionMatchesLegacyScan_TargetsOnly(t *testing.T) {
	const in = `package p

func f() {
	// Nested targeted call inside a targeted call: the legacy scan-based
	// formatter consumes the outer call and does not format the inner call
	// separately.
	log.Infof("outer: %v", fmt.Sprintf("inner: %v", veryLongValueNameThatIsVeryLong))

	// Targeted call with a type assertion arg (historically easy to mis-scan).
	log.Infof("type assert: %v", someInterfaceValueNameThatIsVeryLong.(SomeConcreteTypeNameThatIsVeryLong))
}
`

	cfgScan := Config{
		ColumnLimit:        40,
		TabStop:            8,
		FallbackNonTargets: false,
		SkipGofmt:          true,
		UseASTSelection:    false,
	}
	cfgAST := cfgScan
	cfgAST.UseASTSelection = true

	outScan := NewCompactCallFormatter(cfgScan).FormatFile([]byte(in))
	outAST := NewCompactCallFormatter(cfgAST).FormatFile([]byte(in))
	require.Equal(t, string(outScan), string(outAST))

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", outAST, parser.AllErrors)
	require.NoError(t, err)
}

func TestCompactCallFormatter_ASTSelectionMatchesLegacyScan_AllCallsWhenFallbackEnabled(t *testing.T) {
	const in = `package p

func factory() someFactoryType { return someFactoryType{} }

type someFactoryType struct{}

func (someFactoryType) MethodThatIsVeryLongName(
	a, b, c, d, e, f, g, h int,
) int {
	return 0
}

func f() {
	// When fallback is enabled, the scan-based formatter treats *any* identifier
	// call as a "callsite to consume", even if it ends up leaving it unchanged.
	_ = factory().MethodThatIsVeryLongName(firstVeryLongArgName, secondVeryLongArgName, thirdVeryLongArgName, fourthVeryLongArgName, fifthVeryLongArgName)

	// Targeted calls are still handled by the greedy formatter.
	log.Infof("this is a long log line: %v %v %v %v", a, b, c, d)

	// Generic instantiations should not be treated as calls by the legacy scan.
	_ = someGenericFuncNameThatIsVeryLong[SomeTypeParamName](firstVeryLongArgName, secondVeryLongArgName, thirdVeryLongArgName)
}
`

	cfgScan := Config{
		ColumnLimit:        48,
		TabStop:            8,
		FallbackNonTargets: true,
		SkipGofmt:          true,
		UseASTSelection:    false,
	}
	cfgAST := cfgScan
	cfgAST.UseASTSelection = true

	outScan := NewCompactCallFormatter(cfgScan).FormatFile([]byte(in))
	outAST := NewCompactCallFormatter(cfgAST).FormatFile([]byte(in))
	require.Equal(t, string(outScan), string(outAST))

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", outAST, parser.AllErrors)
	require.NoError(t, err)
}

func TestCompactCallFormatter_ASTSelectionFallsBackWhenUnparseable(t *testing.T) {
	const in = `package p

func f() {
	log.Infof("missing close paren", a, b
}
`
	cfgScan := Config{
		ColumnLimit:        20,
		TabStop:            8,
		FallbackNonTargets: true,
		SkipGofmt:          true,
		UseASTSelection:    false,
	}
	cfgAST := cfgScan
	cfgAST.UseASTSelection = true

	outScan := NewCompactCallFormatter(cfgScan).FormatFile([]byte(in))
	outAST := NewCompactCallFormatter(cfgAST).FormatFile([]byte(in))
	require.Equal(t, string(outScan), string(outAST))
}

