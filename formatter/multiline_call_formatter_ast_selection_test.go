package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMultiLineCallFormatter_ASTSelectionMatchesLegacyScan(t *testing.T) {
	const in = `package p

func factory() someFactoryType { return someFactoryType{} }

type someFactoryType struct{}

func (someFactoryType) MethodThatIsVeryLongName(
	a, b, c, d, e, f, g, h int,
) int {
	return 0
}

func f() {
	_ = factory().MethodThatIsVeryLongName(firstVeryLongArgName, secondVeryLongArgName, thirdVeryLongArgName, fourthVeryLongArgName, fifthVeryLongArgName)

	// Excluded from multiline formatting by default.
	log.Infof("this is a very long log line: %v %v %v %v", a, b, c, d)

	// Generic calls are not handled by the legacy scan-based multiline formatter.
	_ = someGenericFuncNameThatIsVeryLong[SomeTypeParamName](firstVeryLongArgName, secondVeryLongArgName, thirdVeryLongArgName)

	// Not a call; should never be rewritten.
	_ = someInterfaceValueNameThatIsVeryLong.(SomeConcreteTypeNameThatIsVeryLong)
}
`

	cfgScan := MultiLineConfig{
		ColumnLimit:     60,
		TabStop:         8,
		Excludes:        nil,
		UseASTSelection: false,
		SkipGofmt:       true,
	}
	cfgAST := cfgScan
	cfgAST.UseASTSelection = true

	outScan := NewMultiLineCallFormatter(cfgScan).FormatFile([]byte(in))
	outAST := NewMultiLineCallFormatter(cfgAST).FormatFile([]byte(in))
	require.Equal(t, string(outScan), string(outAST))

	// Parseable (sanity).
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", outAST, parser.AllErrors)
	require.NoError(t, err)
}

func TestMultiLineCallFormatter_ASTSelectionFallsBackWhenUnparseable(t *testing.T) {
	// Intentionally invalid Go. The AST selector should fall back to the
	// scan-based implementation rather than failing/crashing.
	const in = `package p

func f() {
	_ = someFuncNameThatIsVeryLong(a, b,
}
`

	cfgScan := MultiLineConfig{
		ColumnLimit:     20,
		TabStop:         8,
		Excludes:        nil,
		UseASTSelection: false,
		SkipGofmt:       true,
	}
	cfgAST := cfgScan
	cfgAST.UseASTSelection = true

	outScan := NewMultiLineCallFormatter(cfgScan).FormatFile([]byte(in))
	outAST := NewMultiLineCallFormatter(cfgAST).FormatFile([]byte(in))
	require.Equal(t, string(outScan), string(outAST))
}

