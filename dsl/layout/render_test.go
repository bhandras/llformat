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
