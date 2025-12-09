package formatter

import (
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
