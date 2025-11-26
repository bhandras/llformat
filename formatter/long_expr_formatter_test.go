package formatter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExpressionsGolden(t *testing.T) {
	dir := filepath.Join("..", "testdata", "expressions")
	inPath := filepath.Join(dir, "input.go")
	outPath := filepath.Join(dir, "output.go")

	if _, err := os.Stat(inPath); err != nil {
		t.Skipf("skipping: %s not present", inPath)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Skipf("skipping: %s not present", outPath)
	}

	in, err := os.ReadFile(inPath)
	if err != nil {
		t.Fatal(err)
	}

	// Use the full pipeline since expression formatting interacts with other formatters
	pipeline := NewPipeline(PipelineConfig{
		ColumnLimit: 80,
		TabStop:     8,
	})
	got := pipeline.Format(in)

	want, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}

	require.Equal(t, normalize(string(want)), normalize(string(got)))
}
