package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_LogCalls_MatchAnySelectorPrefix(t *testing.T) {
	const in = `package p

type Logger struct{}

func (Logger) Errorf(string, ...interface{}) {}

var rpcSLog Logger

func f(err error) {
	rpcSLog.Errorf("unable to lookup peer alias more: %v", err)
}
`

	// next profile should format printf-style calls even when the selector
	// prefix is not "log."/"fmt.".
	p := NewPipeline(PipelineConfig{
		ColumnLimit:    36,
		TabStop:        8,
		UseDSLLogCalls: true,
		// Keep other DSL stages off to make this test focused.
		UseDSLMultiLineCalls: false,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLFuncSigs:       false,
		UseDSLBlankLines:     false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(
		t, out, "rpcSLog.Errorf(",
		"must match any selector prefix in next profile",
	)
	require.Contains(
		t, out, "\"lookup peer \"+\n		\"alias ", "expected"+
			" a space-preserving split at the `peer ` boundary "+
			"in next formatter",
	)
	require.NotContains(
		t, out, "\"peer\"+\n		\"alias", "must not drop "+
			"the trailing space and join words across the split",
	)
}

func TestPipelineNext_LogCalls_DontSplitTinyFormatVerbTail(t *testing.T) {
	const in = `package p

import "fmt"

func f(err error) error {
	return fmt.Errorf("error parsing psbt: %v", err)
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:    34,
		TabStop:        8,
		UseDSLLogCalls: true,
		// Keep other DSL stages off to make this test focused.
		UseDSLMultiLineCalls: false,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLFuncSigs:       false,
		UseDSLBlankLines:     false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(
		t, out, "fmt.Errorf(",
		"must still treat fmt.Errorf as a printf-style call",
	)
	// With a very tight column limit, we may need to split the string, but
	// we must avoid isolating a tiny "%v" tail or splitting exactly before
	// it.
	require.Contains(
		t, out, "\"psbt: %v\"", "must keep the %v verb attached to "+
			"surrounding text (avoid tiny \"%v\" tail splits)",
	)
	require.NotContains(
		t, out, "\"psbt: \"+\n		\"%v\"",
		"must not split immediately before the format verb",
	)
	require.NotContains(
		t, out, "\"%v\"",
		"must not split into a standalone \"%v\" literal",
	)
}

func TestPipelineNext_LogCalls_FormatsErrorsNewLikePrintfCalls(t *testing.T) {
	// errors.New isn't a printf-style call, but it benefits from the same
	// "split long string without hanging-paren" behavior as other targeted
	// calls in next mode.
	const in = `package p

import "errors"

func f() (any, error) {
	return nil, errors.New("either a Single or Multi channel backup must be specified")
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:    55,
		TabStop:        8,
		UseDSLLogCalls: true,
		// Keep other DSL stages off to make this test focused.
		UseDSLMultiLineCalls: false,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLFuncSigs:       false,
		UseDSLBlankLines:     false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(
		t, out, "errors.New(", "must treat errors.New as a "+
			"targeted string call in next profile",
	)
	require.NotContains(
		t, out, "errors.New(\n",
		"must not produce a hanging-paren layout for errors.New",
	)
	require.Contains(
		t, out, "\"either a Single or ",
		"expected string splitting to preserve word boundaries",
	)
	require.Contains(
		t, out, "\" +\n		\"",
		"expected next-style string splitting for errors.New",
	)
	require.Contains(
		t, out, "\"specified\")",
		"expected final segment to remain within the call",
	)
}

func TestPipelineNext_LogCalls_FormatsNonFLoggerStringCalls(t *testing.T) {
	// Some codebases use non-printf logging methods (`.Info/.Error`) but
	// still want the same string splitting behavior as printf-style calls
	// in next mode.
	const in = `package p

type Logger struct{}

func (Logger) Error(...interface{}) {}

var rpcSLog Logger

func f() {
	rpcSLog.Error("unable to fetch channel edges by channel ID %d: %v")
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:    44,
		TabStop:        8,
		UseDSLLogCalls: true,
		// Keep other DSL stages off to make this test focused.
		UseDSLMultiLineCalls: false,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLFuncSigs:       false,
		UseDSLBlankLines:     false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(
		t, out, "rpcSLog.Error(", "must treat non-`*f` logger "+
			"calls as targeted string calls in next profile",
	)
	require.NotContains(
		t, out, "rpcSLog.Error(\n", "must not produce a "+
			"hanging-paren layout for non-`*f` logger calls",
	)
	require.Contains(
		t, out, "\"unable to fetch \" +\n		\"channel "+
			"edges by \" +\n		\"channel ID", "expe"+
			"cted next-style string splitting (word-boundary "+
			"aware) for non-`*f` logger calls",
	)
}

func TestPipelineNext_LogCalls_FlowTrailingArgsWhenTheyFit(t *testing.T) {
	const in = `package p

import "fmt"

func f(maxFee, feeRate int) error {
	return fmt.Errorf("max_fee_per_byte (%v) is less "+` + "\n" + `		"than the required fee rate (%v)", maxFee, feeRate)
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:    80,
		TabStop:        8,
		UseDSLLogCalls: true,
		// Keep other DSL stages off to make this test focused.
		UseDSLMultiLineCalls: false,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLFuncSigs:       false,
		UseDSLBlankLines:     false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(
		t, out, "fmt.Errorf(",
		"must still treat fmt.Errorf as a printf-style call",
	)
	// Prefer splitting the string further over breaking trailing args, so
	// we keep `maxFee, feeRate` together when possible.
	require.Contains(
		t, out, "maxFee, feeRate)", "expected the trailing args to "+
			"stay on the same line when they fit",
	)
	require.NotContains(
		t, out, "maxFee,\n		feeRate", "must not break "+
			"feeRate onto its own continuation line when there "+
			"is room",
	)
}

func TestPipelineNext_LogCalls_DontSplitWhenStringFitsOnContinuationLine(
	t *testing.T) {

	// Regression for cases where a printf-style call appears mid-line (e.g.
	// in a composite literal) and exceeds the column limit due to the
	// prefix, but the format string itself would fit on a continuation
	// line.
	//
	// In these cases, we should break before the string and keep it whole,
	// rather than introducing string concatenation purely to "make room"
	// for args.
	const in = `package p

import "fmt"

func f(numDeletedPayments int, failedHTLCsOnly bool) {
	_ = struct{ Status string }{
		Status: fmt.Sprintf("%v payments deleted, failed_htlcs_only=%v", numDeletedPayments, failedHTLCsOnly),
	}
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:    80,
		TabStop:        8,
		UseDSLLogCalls: true,
		// Keep other DSL stages off to make this test focused.
		UseDSLMultiLineCalls: false,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLFuncSigs:       false,
		UseDSLBlankLines:     false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(
		t, out, "fmt.Sprintf(",
		"must still treat fmt.Sprintf as a printf-style call",
	)
	require.Contains(
		t, out, "failed_htlcs_only=%v",
		"must preserve the full format string contents",
	)
	require.NotContains(
		t, out, "deleted, \"+\n", "must not split the format "+
			"string when it would fit on a continuation line",
	)
	require.NotContains(
		t, out, "failed_htlcs_only=%v\"+",
		"must not introduce concatenation for this case",
	)
}

func TestPipelineNext_LogCalls_AvoidHangingParenAfterSignature(t *testing.T) {
	// Regression: for printf-style calls, avoid breaking immediately after
	// the callee/paren (hanging paren) and instead keep the call "packed"
	// by splitting the string if necessary.
	const in = `package p

import "fmt"

func f(err error) error {
	return fmt.Errorf("unable to unpack chan backup: %v", err)
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:    50,
		TabStop:        8,
		UseDSLLogCalls: true,
		// Keep other DSL stages off to make this test focused.
		UseDSLMultiLineCalls: false,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLFuncSigs:       false,
		UseDSLBlankLines:     false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(
		t, out, "fmt.Errorf(",
		"must still treat fmt.Errorf as a printf-style call",
	)
	require.NotContains(
		t, out, "fmt.Errorf(\n", "must not break immediately after "+
			"the call signature for printf-style calls",
	)
	require.Contains(
		t, out, "\"unable to unpack \"+\n", "expected string split "+
			"to keep the call packed under the column limit",
	)
}

func TestPipelineNext_LogCalls_DontRepackArgsAfterWrappedString(t *testing.T) {
	// Regression: after splitting a printf-style format string, don't
	// "repack" arguments by forcing a break before the first arg just to
	// keep args grouped.
	//
	// Prefer greedy packing: if the first arg fits on the current line,
	// keep it there even if the following arg must break.
	const in = `package p

import "fmt"

type reqT struct{ TimeLockDelta int }

func f(req reqT, minTimeLockDelta int) error {
	if req.TimeLockDelta < minTimeLockDelta {
		return fmt.Errorf("time lock delta of %v is too small, minimum supported is %v", req.TimeLockDelta, minTimeLockDelta)
	}
	return nil
}
`

	p := NewPipeline(PipelineConfig{
		// This is intentionally tight: at 60 columns the tail segment
		// `"supported is %v"` cannot keep `req.TimeLockDelta,` on the
		// same line without overflowing once we append the trailing
		// comma to break before the next arg. At 61 columns, it should
		// fit exactly.
		ColumnLimit:    61,
		TabStop:        8,
		UseDSLLogCalls: true,
		// Keep other DSL stages off to make this test focused.
		UseDSLMultiLineCalls: false,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLFuncSigs:       false,
		UseDSLBlankLines:     false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(
		t, out, "\"supported is %v\", "+
			"req.TimeLockDelta,"+
			"\n			minTimeLockDelta)", "expecte"+
			"d greedy arg packing: keep first arg on the "+
			"string tail line and break only before the last arg",
	)
	require.NotContains(
		t, out, "\"supported is "+
			"%v\",\n			req.TimeLockDelta, "+
			"minTimeLockDelta)", "must not force a break "+
			"before the first arg and then pack args together "+
			"on the next line",
	)
}

func TestPipelineNext_LogCalls_SingleStringArg_NoHangingParen(t *testing.T) {
	// Regression: for targeted log/printf calls that take a single string
	// argument (e.g. fmt.Errorf("...") with no trailing args), avoid
	// producing a hanging-paren layout. Prefer splitting the string while
	// keeping the first segment on the signature line.
	const in = `package p

import "fmt"

func f() error {
	return fmt.Errorf("all_payments cannot be set to true while either failed_payments_only or failed_htlcs_only is also set to true")
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:    70,
		TabStop:        8,
		UseDSLLogCalls: true,
		// Keep other DSL stages off to make this test focused.
		UseDSLMultiLineCalls: false,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLFuncSigs:       false,
		UseDSLBlankLines:     false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(
		t, out, "fmt.Errorf(",
		"must still treat fmt.Errorf as a targeted call",
	)
	require.NotContains(
		t, out, "fmt.Errorf(\n", "must not break immediately after "+
			"the call signature for single-string calls",
	)
	require.Contains(
		t, out, "\" +\n		\"", "expected the long string to "+
			"be split across lines via concatenation",
	)
}

func TestPipelineNext_LogCalls_ReturnNilErrorf_PreferSplitOverHangingParen(
	t *testing.T) {

	// Regression: in a `return nil, fmt.Errorf(...)` context, avoid
	// producing `fmt.Errorf(` followed by an immediate newline when we
	// could fit a split string segment on the same line as the signature.
	const in = `package p

import "fmt"

func f(err error) (any, error) {
	if err != nil {
		return nil, fmt.Errorf("unable to flush forwarding events: %v", err)
	}
	return nil, nil
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:    55,
		TabStop:        8,
		UseDSLLogCalls: true,
		// Keep other DSL stages off to make this test focused.
		UseDSLMultiLineCalls: false,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLFuncSigs:       false,
		UseDSLBlankLines:     false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(
		t, out, "fmt.Errorf(",
		"must still treat fmt.Errorf as a targeted call",
	)
	require.NotContains(
		t, out, "fmt.Errorf(\n", "must not break immediately after "+
			"the call signature when a split segment can fit",
	)
	require.Contains(
		t, out, "\"unable to \"+\n", "expected splitting the "+
			"string to avoid the hanging-paren layout",
	)
}

func TestPipelineNext_LogCalls_PreservesExplicitConcatExpr_NoHangingParen(
	t *testing.T) {

	// Regression from real-world snippet: when the user already wrote an
	// explicit multi-line string concatenation expression as the sole
	// argument to fmt.Errorf (in a `return nil, fmt.Errorf(...)` context),
	// we must not introduce a hanging-paren layout.
	//
	// This previously happened due to reserving the closing ')' against the
	// first line of a multi-line expression, which caused a spurious
	// overflow and an early newline after `fmt.Errorf(`.
	const in = `package p

import (
	"context"
	"fmt"
)

type DeleteAllPaymentsRequest struct {
	AllPayments       bool
	FailedPaymentsOnly bool
	FailedHtlcsOnly   bool
}

type rpcServer struct{}

func (r *rpcServer) DeleteAllPayments(ctx context.Context, req *DeleteAllPaymentsRequest) (any, error) {
	switch {
	case !req.AllPayments && !req.FailedPaymentsOnly && !req.FailedHtlcsOnly:
		return nil, fmt.Errorf("at least one of the options " +
			"all_payments, failed_payments_only, or " +
			"failed_htlcs_only must be set to true")

	case req.AllPayments && (req.FailedPaymentsOnly || req.FailedHtlcsOnly):
		return nil, fmt.Errorf("all_payments cannot be set to true " +
			"while either failed_payments_only or " +
			"failed_htlcs_only is also set to true")
	}

	return nil, nil
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:    80,
		TabStop:        8,
		UseDSLLogCalls: true,
		// Keep other DSL stages off to make this test focused.
		UseDSLMultiLineCalls: false,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLFuncSigs:       false,
		UseDSLBlankLines:     false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(
		t, out, "return nil, fmt.Errorf(\"at least one of the "+
			"options \" +", "must preserve explicit "+
			"concatenation form for the single string arg",
	)
	require.NotContains(
		t, out, "fmt.Errorf(\n", "must not introduce a "+
			"hanging-paren layout for explicit multi-line "+
			"concat expr",
	)
}

func TestPipelineNext_LogCalls_DontSplitSupportedIsVerbAcrossLines(
	t *testing.T) {

	// Regression for a common pattern seen in real code: "... minimum
	// supported is %v", a, b
	//
	// When the string has to wrap, we must avoid producing awkward splits
	// like: "supported "+\n\t\t"is %v" i.e. don't detach `is %v` from the
	// preceding word.
	const in = `package p

import "fmt"

type reqT struct{ TimeLockDelta int }

func f(req reqT, minTimeLockDelta int) (any, error) {
	if req.TimeLockDelta < minTimeLockDelta {
		return nil, fmt.Errorf("time lock delta of %v is too small, "+` + "\n" + `			"minimum supported is %v", req.TimeLockDelta, minTimeLockDelta)
	}
	return nil, nil
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:    60,
		TabStop:        8,
		UseDSLLogCalls: true,
		// Keep other DSL stages off to make this test focused.
		UseDSLMultiLineCalls: false,
		UseDSLExpr:           false,
		UseDSLComments:       false,
		UseDSLFuncSigs:       false,
		UseDSLBlankLines:     false,
	})

	out := string(p.Format([]byte(in)))

	require.Contains(
		t, out, "fmt.Errorf(",
		"must still treat fmt.Errorf as a printf-style call",
	)
	require.NotContains(
		t, out, "\"supported \"+\n			\"is %v\"",
		"must not split `supported is %v` into `supported ` + `is %v`",
	)
	require.NotContains(
		t, out, "\"supported \"+\n		\"is %v\"", "must "+
			"not split `supported is %v` into `supported ` + "+
			"`is %v` (alt indent)",
	)
}
