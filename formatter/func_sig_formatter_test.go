package formatter

import (
	"os"
	"path/filepath"
	"testing"

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
