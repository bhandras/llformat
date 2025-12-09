package formatter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lightninglabs/llformat/dsl"
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

func TestExpressionsGoldenDSL(t *testing.T) {
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

	// First run comment formatter (handles comment wrapping)
	cf := NewCommentFormatter(CommentConfig{
		ColumnLimit: 80,
		TabStop:     8,
	})
	intermediate := cf.FormatFile(in)

	// Then run long expression formatter (handles string concat, etc.)
	lef := NewLongExprFormatter(LongExprConfig{
		ColumnLimit: 80,
		TabStop:     8,
	})
	intermediate = lef.FormatFile(intermediate)

	// Then run DSL formatter with expression rules
	f := NewDSLExprFormatter(DSLExprConfig{
		ColumnLimit: 80,
		TabStop:     8,
		Rules:       dsl.ExpressionRules(),
	})
	got := f.FormatFile(intermediate)

	want, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}

	require.Equal(t, normalize(string(want)), normalize(string(got)))
}
