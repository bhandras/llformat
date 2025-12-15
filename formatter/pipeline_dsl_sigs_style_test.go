package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDSLSigsStyleDSLMayDifferFromLegacy(t *testing.T) {
	const in = `package p

import "time"

// Intentionally long signature that legacy keeps fairly compact while the pure
// DSL fallback formatter may choose a different break layout.
func processInlineConfig(config struct{ Timeout time.Duration; MaxRetries int; EnableCache bool }, handler func(cfg struct{ Timeout time.Duration; MaxRetries int; EnableCache bool }) error) error {
	return nil
}
`

	legacy := NewPipeline(PipelineConfig{
		ColumnLimit: 80,
		TabStop:     8,
	})
	legacyOut := string(legacy.Format([]byte(in)))

	dslStyle := NewPipeline(PipelineConfig{
		ColumnLimit:          80,
		TabStop:              8,
		UseDSLFuncSigs:       true,
		UseDSLFuncSigsNative: true,
		DSLSigsStyle:         "dsl",
	})
	dslOut := string(dslStyle.Format([]byte(in)))

	// This test documents that the pure DSL fallback formatter is allowed to
	// diverge from legacy for now; it provides an opt-in path to evolve the
	// signature formatter without touching goldens.
	require.NotEqual(t, legacyOut, dslOut)
}
