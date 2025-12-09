package formatter

import (
	formatstd "go/format"
	"os"
	"path/filepath"
	"testing"

	"github.com/lightninglabs/llformat/dsl"
	"github.com/stretchr/testify/require"
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

func TestMultilineCallsDSL(t *testing.T) {
	dir := filepath.Join("..", "testdata", "multiline")
	in, err := os.ReadFile(filepath.Join(dir, "input.go"))
	if err != nil {
		t.Fatal(err)
	}

	// Use DSL formatter with multiline call rules using the legacy formatter
	f := NewDSLExprFormatter(DSLExprConfig{
		ColumnLimit: 80,
		TabStop:     8,
		Rules:       dsl.MultiLineCallRules(FormatCallPackedMultiLine),
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
