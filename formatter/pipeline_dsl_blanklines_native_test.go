package formatter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDSLBlankLinesNativeMatchesLegacyOnValidGo(t *testing.T) {
	dir := filepath.Join("..", "testdata", "blanklines")
	inData, err := os.ReadFile(filepath.Join(dir, "input.go"))
	require.NoError(t, err)

	legacy := NewPipeline(PipelineConfig{
		ColumnLimit: 80,
		TabStop:     8,
	})
	want := legacy.Format(inData)

	native := NewPipeline(PipelineConfig{
		ColumnLimit:            80,
		TabStop:                8,
		UseDSLBlankLines:       true,
		UseDSLBlankLinesNative: true,
	})
	got := native.Format(inData)

	require.Equal(t, string(want), string(got))
}
