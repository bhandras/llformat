package formatter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineDSLMultiLineCallsMatchLegacy(t *testing.T) {
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
				ColumnLimit:          80,
				TabStop:              8,
				UseDSLLogCalls:       false,
				UseDSLMultiLineCalls: false,
				UseDSLExpr:           false,
			})
			want := legacy.Format(inData)

			dslML := NewPipeline(PipelineConfig{
				ColumnLimit:          80,
				TabStop:              8,
				UseDSLLogCalls:       false,
				UseDSLMultiLineCalls: true,
				UseDSLExpr:           false,
			})
			got := dslML.Format(inData)

			require.Equal(t, string(want), string(got))
		})
	}
}

func TestPipelineDSLMultiLineCallsIdempotent(t *testing.T) {
	dir := filepath.Join("..", "testdata", "multiline")
	inData, err := os.ReadFile(filepath.Join(dir, "input.go"))
	require.NoError(t, err)

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          80,
		TabStop:              8,
		UseDSLLogCalls:       false,
		UseDSLMultiLineCalls: true,
		UseDSLExpr:           false,
	})

	out1 := p.Format(inData)
	out2 := p.Format(out1)
	require.Equal(t, string(out1), string(out2))
}
