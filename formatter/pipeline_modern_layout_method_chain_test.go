package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineModernLayoutBreaksMethodChains(t *testing.T) {
	const in = `package p

import "time"

func f() {
	ctx := 1
	req := 2
	_ = client.WithTimeout(30*time.Second).WithRetry(3).WithHeaders(headers).Execute(ctx, req)
}

type clientType struct{}

func (clientType) WithTimeout(time.Duration) clientType { return clientType{} }
func (clientType) WithRetry(int) clientType             { return clientType{} }
func (clientType) WithHeaders(interface{}) clientType   { return clientType{} }
func (clientType) Execute(int, int) int                 { return 0 }

var client clientType
var headers interface{}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:   60,
		TabStop:       8,
		DSLCallPolicy: "modern",
	})

	out := p.Format([]byte(in))
	outStr := string(out)

	// Dot-at-line-end chain breaking.
	require.Contains(t, outStr, "client.\n\t\tWithTimeout(")
	require.Contains(t, outStr, ".\n\t\tWithRetry(")
	require.Contains(t, outStr, ".\n\t\tWithHeaders(")
	require.Contains(t, outStr, ".\n\t\tExecute(")

	// Idempotent.
	out2 := p.Format(out)
	require.Equal(t, outStr, string(out2))

	// Parseable and AST-equivalent.
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors)
	require.NoError(t, err)
	requireASTEquivalent(t, []byte(in), out)
}
