package formatter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineDSLCommentStageMatchesLegacy(t *testing.T) {
	cases := []string{"blanklines", "chains", "comments", "expressions", "logs", "multiline", "signatures"}

	for _, dirName := range cases {
		t.Run(dirName, func(t *testing.T) {
			dir := filepath.Join("..", "testdata", dirName)
			inData, err := os.ReadFile(filepath.Join(dir, "input.go"))
			require.NoError(t, err)

			legacy := NewPipeline(PipelineConfig{
				ColumnLimit: 80,
				TabStop:     8,
			})
			want := legacy.Format(inData)

			dslComments := NewPipeline(PipelineConfig{
				ColumnLimit:    80,
				TabStop:        8,
				UseDSLComments: true,
			})
			got := dslComments.Format(inData)

			require.Equal(t, string(want), string(got))
		})
	}
}

func TestPipelineDSLSignatureStageMatchesLegacy(t *testing.T) {
	dir := filepath.Join("..", "testdata", "signatures")
	inData, err := os.ReadFile(filepath.Join(dir, "input.go"))
	require.NoError(t, err)

	legacy := NewPipeline(PipelineConfig{
		ColumnLimit: 80,
		TabStop:     8,
	})
	want := legacy.Format(inData)

	dslSigs := NewPipeline(PipelineConfig{
		ColumnLimit:    80,
		TabStop:        8,
		UseDSLFuncSigs: true,
	})
	got := dslSigs.Format(inData)

	require.Equal(t, string(want), string(got))
}

func TestPipelineDSLBlankLinesStageMatchesLegacy(t *testing.T) {
	dir := filepath.Join("..", "testdata", "blanklines")
	inData, err := os.ReadFile(filepath.Join(dir, "input.go"))
	require.NoError(t, err)

	legacy := NewPipeline(PipelineConfig{
		ColumnLimit: 80,
		TabStop:     8,
	})
	want := legacy.Format(inData)

	dslBlank := NewPipeline(PipelineConfig{
		ColumnLimit:      80,
		TabStop:          8,
		UseDSLBlankLines: true,
	})
	got := dslBlank.Format(inData)

	require.Equal(t, string(want), string(got))
}
