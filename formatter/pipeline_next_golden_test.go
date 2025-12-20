package formatter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNextGoldens(t *testing.T) {
	const placeholderMarker = "LLFORMAT_NEXT_GOLDEN_TODO"

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
			inPath := filepath.Join(dir, "input.go")
			outPath := filepath.Join(dir, "output_next.go")

			if _, err := os.Stat(inPath); err != nil {
				t.Skipf("skipping: %s not present", inPath)
			}
			if _, err := os.Stat(outPath); err != nil {
				t.Skipf("skipping: %s not present (add next goldens)", outPath)
			}

			inData, err := os.ReadFile(inPath)
			require.NoError(t, err)

			want, err := os.ReadFile(outPath)
			require.NoError(t, err)
			if strings.Contains(string(want), placeholderMarker) {
				t.Skipf("skipping: %s is a placeholder (edit it by hand and remove %q to enable)", outPath, placeholderMarker)
			}

			p := NewPipeline(PipelineConfig{
				ColumnLimit: 80,
				TabStop:     8,
				Mode:        "next",
			})
			got := p.Format(inData)

			require.Equal(t, string(want), string(got))
		})
	}
}
