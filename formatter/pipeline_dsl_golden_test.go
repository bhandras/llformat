package formatter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineDSLMatchesLegacy(t *testing.T) {
	cases := []string{
		"blanklines",
		"chains",
		"comments",
		"expressions",
		"logs",
		"multiline",
		"signatures",
	}

	for _, dirName := range cases {
		t.Run(dirName, func(t *testing.T) {
			dir := filepath.Join("..", "testdata", dirName)
			inData, err := os.ReadFile(filepath.Join(dir, "input.go"))
			require.NoError(t, err)

			legacy := NewPipeline(PipelineConfig{
				ColumnLimit: 80,
				TabStop:     8,
				UseDSLExpr:  false,
			})
			want := legacy.Format(inData)

			dslPipeline := NewPipeline(PipelineConfig{
				ColumnLimit: 80,
				TabStop:     8,
				UseDSLExpr:  true,
			})
			got := dslPipeline.Format(inData)

			require.Equal(t, string(want), string(got))
		})
	}
}
