package formatter

import (
	formatstd "go/format"
	"os"
	"path/filepath"
	"testing"

	"github.com/lightninglabs/llformat/dsl"
	"github.com/stretchr/testify/require"
)

func TestLogsExamples(t *testing.T) {
	dir := filepath.Join("..", "testdata", "logs")
	in, err := os.ReadFile(filepath.Join(dir, "input.go"))
	if err != nil {
		t.Fatal(err)
	}
	f := NewCompactCallFormatter(Config{})
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

func TestLogsExamplesDSL(t *testing.T) {
	dir := filepath.Join("..", "testdata", "logs")
	in, err := os.ReadFile(filepath.Join(dir, "input.go"))
	if err != nil {
		t.Fatal(err)
	}

	// Use DSL formatter with only log/printf rules, wired to legacy formatter
	f := NewDSLExprFormatter(DSLExprConfig{
		ColumnLimit: 80,
		TabStop:     8,
		Rules:       dsl.LogPrintfRules(FormatCallGreedy),
	})
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
