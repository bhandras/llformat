package formatter

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDSLCallPolicyModernEnablesPackedChain(t *testing.T) {
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

	legacy := NewPipeline(PipelineConfig{
		ColumnLimit:          80,
		TabStop:              8,
		UseDSLLogCalls:       true,
		UseDSLMultiLineCalls: true,
		DSLMultiLineStyle:    "legacy",
		DSLCallPolicy:        "legacy",
		UseDSLExpr:           true,
	})
	legacyOut := string(legacy.Format([]byte(in)))

	modern := NewPipeline(PipelineConfig{
		ColumnLimit:          80,
		TabStop:              8,
		UseDSLLogCalls:       false, // policy should override
		UseDSLMultiLineCalls: false, // policy should override
		DSLMultiLineStyle:    "legacy",
		DSLCallPolicy:        "modern",
		UseDSLExpr:           false, // policy should override
	})
	modernOut := string(modern.Format([]byte(in)))

	// Legacy scan mode does not break method chains; it rewrites only generic
	// identifier-style calls and can miss selector-call chains.
	require.Contains(t, legacyOut, "WithHeaders(headers).Execute(")
	require.NotContains(t, legacyOut, ".WithHeaders(\n")

	// Modern policy enables method-chain breaking (packed-chain multiline rules).
	require.NotContains(t, modernOut, "WithHeaders(headers).Execute(ctx, req)")
	require.Contains(t, modernOut, ".WithHeaders(")
	require.Contains(t, modernOut, ").Execute(")
}

func TestDSLCallPolicyModernDoesNotRewriteInlineCommentArgs(t *testing.T) {
	const in = `package p

func f() {
	// The legacy scan-based formatter can preserve comments inside argument lists.
	// Modern packed formatting should avoid rewriting calls when it would drop
	// inline comments.
	_ = call(a, // keep
		b, c, d)
}

func call(a, b, c, d int) int { return 0 }
var a, b, c, d int
`

	modern := NewPipeline(PipelineConfig{
		ColumnLimit:          20,
		TabStop:              8,
		DSLCallPolicy:        "modern",
		UseDSLLogCalls:       true,
		UseDSLMultiLineCalls: true,
		UseDSLExpr:           true,
	})

	out := string(modern.Format([]byte(in)))
	// Ensure the inline comment survives and the call isn't rewritten into a
	// different layout that would require dropping it.
	require.Contains(t, out, "// keep")
	require.True(t, strings.Contains(out, "_ = call(a, // keep") || strings.Contains(out, "_ = call(\n\t\ta, // keep"))
}

func TestDSLCallPolicyModernEnablesAutoCallArgsForExcludedCalls(t *testing.T) {
	const in = `package p

import "fmt"

func f(a, b, c, d, e, f2, g bool) string {
	return fmt.Sprintf("ok=%v", a && b && c && d && e && f2 && g)
}
`

	legacy := NewPipeline(PipelineConfig{
		ColumnLimit:          40,
		TabStop:              8,
		DSLCallPolicy:        "legacy",
		UseDSLLogCalls:       true,
		UseDSLMultiLineCalls: true,
		UseDSLExpr:           true,
		AutoDSLCallArgs:      false,
	})
	legacyOut := string(legacy.Format([]byte(in)))

	modern := NewPipeline(PipelineConfig{
		ColumnLimit:   40,
		TabStop:       8,
		DSLCallPolicy: "modern",
	})
	modernOut := string(modern.Format([]byte(in)))

	// Legacy policy should not break inside call args.
	require.Contains(t, legacyOut, `a && b && c && d && e && f2 && g`)

	// Modern policy enables AutoDSLCallArgs and includes fmt.Sprintf in the
	// allowlist, so the logical chain should be broken across lines.
	require.Contains(t, modernOut, "a &&\n")
	require.GreaterOrEqual(t, strings.Count(modernOut, "&&\n"), 2)
	require.NotContains(t, modernOut, "a && b && c &&")
}
