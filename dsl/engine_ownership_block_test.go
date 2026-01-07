package dsl

import (
	"bytes"
	"io"
	"os"
	"testing"

	llast "github.com/bhandras/llformat/ast"
	"github.com/stretchr/testify/require"
)

type fixedEditAction struct {
	start   int
	end     int
	replace []byte
}

func (a fixedEditAction) Execute(caps Captures, ctx *Context) ([]byte, bool) {
	return nil, false
}

func (a fixedEditAction) ExecuteEdits(caps Captures, ctx *Context) ([]Edit, bool,
	error) {

	return []Edit{
		{
			Start:   a.start,
			End:     a.end,
			Replace: a.replace,
		},
	}, true, nil
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	fn()

	require.NoError(t, w.Close())
	os.Stderr = old

	b, err := io.ReadAll(r)
	require.NoError(t, err)

	return string(b)
}

func TestEngine_BlocksEditsOverlappingForbiddenSpans(t *testing.T) {
	t.Parallel()

	src := []byte("package p\n\nvar x = 1\n")
	start := bytes.Index(src, []byte("1"))
	require.Greater(t, start, 0)
	end := start + 1

	rule := Rule{
		Name:     "fixed_edit",
		Priority: 100,
		Pattern: &NodePattern{
			Type: "File",
		},
		When: TrueCond{},
		Action: fixedEditAction{
			start:   start,
			end:     end,
			replace: []byte("2"),
		},
	}

	engine := NewEngine([]Rule{rule})
	engine.MaxIterations = 1

	t.Run(
		"budget_rejects_large_growth",
		func(t *testing.T) {
			engine.Budget = RewriteBudget{MaxOutputBytes: len(src) - 1}
			outBudget := engine.FormatFile(src)
			require.Equal(t, string(src), string(outBudget))
		},
	)

	// Sanity: without forbidden spans, edit applies.
	engine.Budget = RewriteBudget{}
	out := engine.FormatFile(src)
	require.NotEqual(t, string(src), string(out))
	require.Contains(t, string(out), "var x = 2")

	// With forbidden spans, edit is blocked.
	engine.ForbiddenSpans = llast.NewOffsetSpanSet(
		[]llast.OffsetSpan{
			{Start: start, End: end},
		},
	)
	engine.TraceReasons = true
	engine.Trace = false

	stderr := captureStderr(
		t,
		func() {
			out2 := engine.FormatFile(src)
			require.Equal(t, string(src), string(out2))
		},
	)
	require.Contains(t, stderr, "blocked_by_ownership")
}
