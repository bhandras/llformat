package layout

import "testing"

func TestForceBreakForcesGroupToBreak(t *testing.T) {
	doc := G(C(
		T("a"),
		L(),
		T("b"),
		FB(),
	))

	// Without ForceBreak, this would render as "a b" under a sufficiently large
	// column limit. ForceBreak should force break mode so the Line is a newline.
	out := Render(doc, 80, 8, "")
	if out != "a\nb" {
		t.Fatalf("expected forced break output %q, got %q", "a\\nb", out)
	}
}
