package formatter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBlankLinesGolden(t *testing.T) {
	dir := filepath.Join("..", "testdata", "blanklines")
	inPath := filepath.Join(dir, "input.go")
	outPath := filepath.Join(dir, "output.go")

	if _, err := os.Stat(inPath); err != nil {
		t.Skipf("skipping: %s not present (add testdata/blanklines)", inPath)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Skipf("skipping: %s not present (add testdata/blanklines)", outPath)
	}

	in, err := os.ReadFile(inPath)
	if err != nil {
		t.Fatal(err)
	}
	f := NewBlankLineFormatter(BlankLineConfig{
		BeforeReturn:            true,
		BetweenCases:            true,
		BetweenInterfaceMethods: true,
	})
	got := f.FormatFile(in)

	want, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	ng, nw := normalizeBlankLines(string(got)), normalizeBlankLines(string(want))
	require.Equal(t, nw, ng)
}

func normalizeBlankLines(s string) string {
	// Normalize newlines and trim trailing newlines for a stable comparison
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimRight(s, "\n")
	return s
}
