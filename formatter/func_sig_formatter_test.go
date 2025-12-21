package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSignaturesGolden(t *testing.T) {
	t.Skip("legacy golden tests removed; next goldens are validated in TestPipelineNextGoldens")
}

func TestSignaturesGoldenDSL(t *testing.T) {
	t.Skip("legacy golden tests removed; next goldens are validated in TestPipelineNextGoldens")
}

func TestFormatFuncSigsInSource_PreservesInlineFuncBodies(t *testing.T) {
	const in = `package p

import "time"

type clientType struct{}

func (clientType) WithTimeout(time.Duration) clientType { return clientType{} }
func (clientType) WithRetry(int) clientType             { return clientType{} }
func (clientType) Execute(int, int) int                 { return 0 }

	var client clientType
`

	// Use an intentionally small column limit to force the legacy signature
	// formatter to rewrite these 1-line bodies.
	out, changed := FormatFuncSigsInSource([]byte(in), 40, 8)
	require.True(t, changed)

	outStr := string(out)
	require.Contains(t, outStr, "return clientType{}")
	require.Contains(t, outStr, "return 0")
	require.Contains(t, outStr, "var client clientType")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors)
	require.NoError(t, err)
}

func TestFormatFuncSigsInSource_PreservesInlineEmptyBodies(t *testing.T) {
	const in = `package p

func veryLongFunctionNameWithManyParameters(a, b, c, d, e, f, g, h int) {}
`

	out, changed := FormatFuncSigsInSource([]byte(in), 30, 8)
	require.True(t, changed)

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors)
	require.NoError(t, err)
}
