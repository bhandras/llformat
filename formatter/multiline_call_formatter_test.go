package formatter

import (
	"github.com/stretchr/testify/require"
	formatstd "go/format"
	"os"
	"path/filepath"
	"testing"
)

func TestPackedMultilineViaCompactFallback(t *testing.T) {
	dir := filepath.Join("..", "testdata", "multiline")
	in, err := os.ReadFile(filepath.Join(dir, "input.go"))
	if err != nil {
		t.Fatal(err)
	}
	f := NewCompactCallFormatter(Config{FallbackNonTargets: true})
	got := f.FormatFile(in)
	want, err := os.ReadFile(filepath.Join(dir, "output.go"))
	if err != nil {
		t.Fatal(err)
	}
	if formatted, err := formatstd.Source(want); err == nil {
		want = formatted
	}
	require.Equal(t, string(want), string(got))
}
