package formatter

import (
	"testing"

	"github.com/lightninglabs/llformat/dsl"
	"github.com/stretchr/testify/require"
)

func TestNewPipeline_LayoutArgsAutoEnablesDSLExprStage(t *testing.T) {
	p := NewPipeline(PipelineConfig{
		ColumnLimit:          48,
		TabStop:              8,
		UseDSLMultiLineCalls: true,
		DSLMultiLineStyle:    "layout-args",
		// Intentionally not setting UseDSLExpr: NewPipeline should
		// enable it to avoid legacy expr formatting interacting with
		// layout-args ownership.
	})

	var exprStage Stage
	found := false
	for _, s := range p.Stages() {
		if s.Name == "expressions" {
			exprStage = s
			found = true
			break
		}
	}
	require.True(
		t, found, "expected pipeline to include an expressions stage",
	)

	_, ok := exprStage.Formatter.(*DSLExprFormatter)
	require.True(
		t, ok, "expected expressions stage to be DSL when "+
			"layout-args is enabled",
	)
}

func TestNewPipeline_LayoutArgsUsesDeepestFirstNodeOrder(t *testing.T) {
	p := NewPipeline(
		PipelineConfig{
			ColumnLimit:          48,
			TabStop:              8,
			UseDSLMultiLineCalls: true,
			DSLMultiLineStyle:    "layout-args",
		},
	)

	var multilineStage Stage
	found := false
	for _, s := range p.Stages() {
		if s.Name == "multiline-calls" {
			multilineStage = s
			found = true
			break
		}
	}
	require.True(
		t, found,
		"expected pipeline to include a multiline-calls stage",
	)

	f, ok := multilineStage.Formatter.(*DSLExprFormatter)
	require.True(
		t, ok, "expected multiline-calls stage to be DSL when "+
			"UseDSLMultiLineCalls is enabled",
	)
	require.Equal(t, dsl.NodeOrderDeepestFirst, f.engine.NodeOrder)
}

func TestNewPipeline_NonLayoutArgsKeepsDefaultNodeOrder(t *testing.T) {
	p := NewPipeline(
		PipelineConfig{
			ColumnLimit:          48,
			TabStop:              8,
			UseDSLMultiLineCalls: true,
			DSLMultiLineStyle:    "packed-chain-layout",
		},
	)

	var multilineStage Stage
	found := false
	for _, s := range p.Stages() {
		if s.Name == "multiline-calls" {
			multilineStage = s
			found = true
			break
		}
	}
	require.True(
		t, found,
		"expected pipeline to include a multiline-calls stage",
	)

	f, ok := multilineStage.Formatter.(*DSLExprFormatter)
	require.True(
		t, ok, "expected multiline-calls stage to be DSL when "+
			"UseDSLMultiLineCalls is enabled",
	)
	require.Equal(t, dsl.NodeOrderPreorder, f.engine.NodeOrder)
}
