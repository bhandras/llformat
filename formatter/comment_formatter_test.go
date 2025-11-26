package formatter

import (
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommentsGolden(t *testing.T) {
	dir := filepath.Join("..", "testdata", "comments")
	inPath := filepath.Join(dir, "input.go")
	outPath := filepath.Join(dir, "output.go")

	if _, err := os.Stat(inPath); err != nil {
		t.Skipf("skipping: %s not present (add testdata/comments)", inPath)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Skipf("skipping: %s not present (add testdata/comments)", outPath)
	}

	in, err := os.ReadFile(inPath)
	if err != nil {
		t.Fatal(err)
	}
	f := NewCommentFormatter(CommentConfig{})
	got := f.FormatFile(in)

	want, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	ng, nw := normalize(string(got)), normalize(string(want))
	require.Equal(t, nw, ng)
}

func normalize(s string) string {
	// Normalize newlines and trim trailing newlines for a stable comparison
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimRight(s, "\n")
	return s
}

func TestCommentsGoldenWithInline(t *testing.T) {
	dir := filepath.Join("..", "testdata", "comments")
	inPath := filepath.Join(dir, "input.go")
	outPath := filepath.Join(dir, "output_inline.go")

	if _, err := os.Stat(inPath); err != nil {
		t.Skipf("skipping: %s not present (add testdata/comments)", inPath)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Skipf("skipping: %s not present (add testdata/comments/output_inline.go)", outPath)
	}

	in, err := os.ReadFile(inPath)
	if err != nil {
		t.Fatal(err)
	}
	f := NewCommentFormatter(CommentConfig{MoveInlineAbove: true})
	got := f.FormatFile(in)

	want, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	ng, nw := normalize(string(got)), normalize(string(want))
	require.Equal(t, nw, ng)
}
