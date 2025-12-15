package layout

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderGroupFlattensWhenItFits(t *testing.T) {
	doc := G(C(T("a,"), L(), T("b,"), L(), T("c")))
	out := Render(doc, 80, 8, "\t")
	require.Equal(t, "a, b, c", out)
}

func TestRenderGroupBreaksWhenItDoesNotFit(t *testing.T) {
	doc := G(N("\t", C(T("alpha,"), L(), T("beta,"), L(), T("gamma"))))
	out := Render(doc, 10, 8, "\t")
	require.Equal(t, "alpha,\n\t\tbeta,\n\t\tgamma", out)
}

func TestRenderSoftLineIsEmptyWhenFlat(t *testing.T) {
	doc := G(C(T("a."), SL(), T("b")))
	out := Render(doc, 80, 8, "\t")
	require.Equal(t, "a.b", out)
}

func TestRenderSoftLineBreaksWhenNotFlat(t *testing.T) {
	doc := N("\t", C(T("a."), SL(), T("b")))
	out := Render(doc, 1, 8, "\t")
	require.Equal(t, "a.\n\t\tb", out)
}

func TestRenderIfBreakChoosesVariant(t *testing.T) {
	doc := G(C(T("a"), IB(T("X"), T("Y")), T("b")))
	out := Render(doc, 80, 8, "")
	require.Equal(t, "aYb", out)

	out2 := Render(doc, 1, 8, "")
	require.Equal(t, "aXb", out2)
}

func TestRenderAlignIndentsToCurrentColumn(t *testing.T) {
	// The aligned indent should be the current column in spaces, so after
	// rendering "func(" (5 cols), the next line starts with 5 spaces.
	doc := G(C(T("func("), A(C(T("a,"), L(), T("b,"), L(), T("c"))), T(")")))
	out := Render(doc, 7, 8, "")
	require.Equal(t, "func(a,\n     b,\n     c)", out)
}

func TestRenderIndentByColsAddsSpacesOnBreaks(t *testing.T) {
	doc := G(I(2, C(T("a,"), L(), T("b"))))
	out := Render(doc, 1, 8, "\t")
	require.Equal(t, "a,\n\t  b", out)
}
