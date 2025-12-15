package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineModernPolicyIdempotent(t *testing.T) {
	const in = `package p

import (
	"fmt"
	"time"
)

func f(a, b, c, d, e, f2, g bool) string {
	// A long comment line that should be wrapped by the comment formatter into something more readable without breaking directives.
	result := client.WithTimeout(30*time.Second).WithRetry(3).WithHeaders(headers).Execute(ctx, req)
	_ = result

	// excluded call where auto call-args may apply
	return fmt.Sprintf("ok=%v", a && b && c && d && e && f2 && g)
}

type clientType struct{}

func (clientType) WithTimeout(time.Duration) clientType { return clientType{} }
func (clientType) WithRetry(int) clientType             { return clientType{} }
func (clientType) WithHeaders(interface{}) clientType   { return clientType{} }
func (clientType) Execute(int, int) int                 { return 0 }

var client clientType
var headers interface{}
var ctx = 1
var req = 2
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:   60,
		TabStop:       8,
		DSLCallPolicy: "modern",
	})

	out1 := p.Format([]byte(in))
	out2 := p.Format(out1)
	require.Equal(t, string(out1), string(out2))
}
