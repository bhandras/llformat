package dsl

import "testing"

func TestHasBlockCommentIgnoresStrings(t *testing.T) {
	if hasBlockComment(`"/* not a comment */"`) {
		t.Fatalf("expected block comment inside string to be ignored")
	}
	if !hasBlockComment(`x /* comment */ y`) {
		t.Fatalf("expected block comment outside string to be detected")
	}
}

