package formatter

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/lightninglabs/llformat/dsl"
	"github.com/stretchr/testify/require"
)

func TestSignaturesGolden(t *testing.T) {
	dir := filepath.Join("..", "testdata", "signatures")

	inPath := filepath.Join(dir, "input.go")
	outPath := filepath.Join(dir, "output.go")

	inData, err := os.ReadFile(inPath)
	require.NoError(t, err)

	expectedData, err := os.ReadFile(outPath)
	require.NoError(t, err)

	// Use the pipeline to format
	p := NewPipeline(PipelineConfig{
		ColumnLimit: 80,
		TabStop:     8,
	})
	actual := p.Format(inData)

	require.Equal(t, string(expectedData), string(actual))
}

func TestSignaturesGoldenDSL(t *testing.T) {
	dir := filepath.Join("..", "testdata", "signatures")

	inPath := filepath.Join(dir, "input.go")
	outPath := filepath.Join(dir, "output.go")

	inData, err := os.ReadFile(inPath)
	require.NoError(t, err)

	expectedData, err := os.ReadFile(outPath)
	require.NoError(t, err)

	// Use DSL formatter with signature rules using the legacy formatters,
	// plus blank line rules for interface methods
	sigRules := dsl.SignatureRules(dsl.SignatureConfig{
		FuncFormatter:   FormatFuncSignature,
		MethodFormatter: FormatInterfaceMethod,
	})
	blankRules := dsl.BlankLineRules()
	allRules := append(sigRules, blankRules...)
	f := NewDSLExprFormatter(DSLExprConfig{
		ColumnLimit: 80,
		TabStop:     8,
		Rules:       allRules,
	})
	actual := f.FormatFile(inData)

	require.Equal(t, string(expectedData), string(actual))
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
