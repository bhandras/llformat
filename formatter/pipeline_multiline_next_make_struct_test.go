package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_MultiLineCalls_MakeInsideStructField_BreakAfterMakeAndPackArgs(t *testing.T) {
	// Regression for "make weirdness" inside struct literals:
	// when a make() call appears as a composite literal field value and does not
	// fit on one line, we want the packed multiline formatter to break right
	// after `make(` and then pack args on continuation lines. In particular, we
	// must not keep the first arg inline as `make([]T,` and then break, which
	// produces awkward layouts.
	const in = `package p

type Invoice struct{}
type InvoiceSlice struct{ Inv []Invoice }

type Resp struct {
	Invoices []*Invoice
}

func f(invoiceSlice InvoiceSlice) {
	resp := &Resp{
		Invoices: make([]*Invoice, len(invoiceSlice.Inv)),
	}
	_ = resp
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:          60,
		TabStop:              8,
		RuleProfile:          "next",
		UseDSLMultiLineCalls: true,
		// Keep other DSL stages off to make this test focused.
		UseDSLLogCalls:    false,
		UseDSLExpr:        false,
		UseDSLComments:    false,
		UseDSLFuncSigs:    false,
		UseDSLBlankLines:  false,
		DSLMultiLineStyle: "",
	})

	out := string(p.Format([]byte(in)))

	require.Contains(t, out, "Invoices: make(\n", "expected packed multiline to break immediately after `make(`")
	require.Contains(t, out, "\t\t[]*Invoice, len(invoiceSlice.Inv),\n",
		"expected packed args to stay on the same continuation line when they fit")
	require.NotContains(t, out, "Invoices: make([]*Invoice,\n",
		"must not keep first arg inline as `make([]T,` and then break")
}
