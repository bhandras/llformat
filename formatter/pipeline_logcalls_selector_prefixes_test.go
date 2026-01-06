package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_LogCalls_SelectorPrefixesRestrictSelection(t *testing.T) {
	const in = `package p

type Logger struct{}

func (Logger) Errorf(string, ...interface{}) {}

var rpcSLog Logger
var otherLog Logger

func f(err error) {
	rpcSLog.Errorf("unable to lookup peer alias more: %v", err)
	otherLog.Errorf("unable to lookup peer alias more: %v", err)
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:              36,
		TabStop:                  8,
		UseDSLLogCalls:           true,
		LogCallsSelectorPrefixes: []string{"rpcSLog"},
		// Keep other DSL stages off to make this test focused.
		UseDSLMultiLineCalls: false,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLFuncSigs:       false,
		UseDSLBlankLines:     false,
	})

	out := string(p.Format([]byte(in)))

	// The allowlisted logger should be formatted (string split)...
	require.Contains(
		t, out, "\"lookup peer \"+\n\t\t\"alias ",
	)

	// ...but other loggers should be left as-is.
	require.Contains(
		t, out,
		"otherLog.Errorf(\"unable to lookup peer alias more: %v\", err)",
	)
	require.NotContains(
		t, out, "otherLog.Errorf(\"unable to lookup peer "+
			"\"+\n		\"alias ",
	)
}

func TestPipelineNext_LogCalls_SelectorNamesOverrideRestrictsSelection(
	t *testing.T) {

	const in = `package p

type Logger struct{}

func (Logger) Infof(string, ...interface{}) {}
func (Logger) Errorf(string, ...interface{}) {}

var rpcSLog Logger

func f(err error) {
	rpcSLog.Infof("unable to lookup peer alias more: %v", err)
	rpcSLog.Errorf("unable to lookup peer alias more: %v", err)
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:           36,
		TabStop:               8,
		UseDSLLogCalls:        true,
		LogCallsSelectorNames: []string{"Errorf"},
		// Keep other DSL stages off to make this test focused.
		UseDSLMultiLineCalls: false,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLFuncSigs:       false,
		UseDSLBlankLines:     false,
	})

	out := string(p.Format([]byte(in)))

	// Errorf should be formatted (string split)...
	require.Contains(t, out, "rpcSLog.Errorf(")
	require.Contains(t, out, "\"lookup peer \"+\n\t\t\"alias ")

	// ...while Infof should be left as-is.
	require.Contains(
		t, out,
		"rpcSLog.Infof(\"unable to lookup peer alias more: %v\", err)",
	)
	require.NotContains(
		t, out,
		"rpcSLog.Infof(\"unable to \"+\n		\"lookup peer ",
	)
}
